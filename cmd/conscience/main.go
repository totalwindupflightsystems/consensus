// Command conscience is the unified binary for the Conscience runtime.
//
// The binary contains the harness loop, REST API, MCP server, CLI management
// commands, and protocol shims. It connects to PostgreSQL or SQLite via a
// driver interface; one binary, two backends.
//
// When invoked with subcommands (conscience session list, conscience status),
// it acts as a thin REST client to a running server. Without subcommands,
// it starts the server (equivalent to conscience serve).
//
// axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/plan.md impl=cmd/conscience/main.go test=internal/bootstrap/admin_key_test.go evidence=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/verification.md
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/conscientiousness/internal/api"
	"github.com/wojons/conscientiousness/internal/billing"
	"github.com/wojons/conscientiousness/internal/bootstrap"
	"github.com/wojons/conscientiousness/internal/cli"
	"github.com/wojons/conscientiousness/internal/compression"
	"github.com/wojons/conscientiousness/internal/config"
	"github.com/wojons/conscientiousness/internal/db"
	dbdriver "github.com/wojons/conscientiousness/internal/db/driver"
	"github.com/wojons/conscientiousness/internal/db/postgres"
	"github.com/wojons/conscientiousness/internal/harness"
	"github.com/wojons/conscientiousness/internal/hitl"
	"github.com/wojons/conscientiousness/internal/llm"
	"github.com/wojons/conscientiousness/internal/mcp"
	"github.com/wojons/conscientiousness/internal/migrate"
	"github.com/wojons/conscientiousness/internal/quarantine"
	"github.com/wojons/conscientiousness/internal/shim/opencode"
	"github.com/wojons/conscientiousness/internal/web"
	"github.com/wojons/conscientiousness/internal/webhook"
)

func main() {
	// Wire CLI stubs to actual server functions (SPEC-016 §5.1, §5.2).
	cli.InitFunc = runInit // alias for server startup (also runs auto-migrate)
	cli.ServerFunc = runServer
	cli.MigrateFunc = runMigrate // direct DB migration mode
	cli.MCPStdioFunc = runMCPStdio // MCP stdio transport (SPEC-015 §5.4)

	// Route: if subcommands provided, use CLI client mode.
	// Otherwise, start the server.
	if len(os.Args) > 1 {
		os.Exit(cli.Execute())
	}

	runServer()
}

func runServer() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	database, err := dbdriver.Open(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Auto-migrate schema on startup (SPEC-009 §6)
	migrator := migrate.New(database)
	migrated, err := migrator.AutoMigrate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	if migrated {
		slog.Info("conscience: schema migrations applied")
	}
	adminKey, err := bootstrap.EnsureFirstAdminKey(ctx, database, bootstrap.GetBootstrapKeyTTL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		os.Exit(1)
	}
	// Bootstrap output: admin key goes to stdout (machine-parseable, SPEC-016 §3).
	// Errors and operational logs stay on stderr.
	// axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md impl=cmd/conscience/main.go
	// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-002 impl=cmd/conscience/main.go
	for _, line := range bootstrap.FormatResult(adminKey) {
		fmt.Println(line)
	}

	// Start heartbeat task poller (SPEC-008 §Heartbeat, SPEC-009 §3)
	llmCfg := &llm.Config{
		Provider:    llm.Provider(cfg.LLM.Provider),
		BaseURL:     resolveLLMBaseURL(cfg),
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.DefaultModel,
		MaxTokens:   cfg.LLM.MaxOutput,
		Temperature: 0.0,
		EnableCache: true,
	}
	if llmCfg.Model == "" {
		llmCfg.Model = "gpt-4o"
	}
	if llmCfg.MaxTokens <= 0 {
		llmCfg.MaxTokens = 16384
	}

	llmClient, err := llm.NewClient(llmCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conscience: llm client init failed: %v\n", err)
		os.Exit(1)
	}
	h := harness.New(database, llmClient)
	go h.StartHeartbeatLoop(ctx)

	// Compression Worker (WI-012, CS-GAP-001) — background memory compression pipeline.
	// SPAN: SPEC-002 §8, SPEC-011 §10
	if cfg.Compression.Enabled {
		// Embedding client uses the same API key as the main LLM client by default
		embCfg := llm.EmbeddingConfig{
			BaseURL: resolveLLMBaseURL(cfg) + "/../..", // embeddings use /v1/embeddings, not /v1/chat/completions
			APIKey:  cfg.LLM.APIKey,
			Model:   cfg.Compression.EmbeddingModel,
		}
		// Reset baseURL — embeddings endpoint is at the same base without /chat
		if cfg.LLM.BaseURL != "" {
			embCfg.BaseURL = cfg.LLM.BaseURL
		} else {
			switch cfg.LLM.Provider {
			case "openrouter":
				embCfg.BaseURL = "https://openrouter.ai/api/v1"
			default:
				embCfg.BaseURL = "https://api.openai.com/v1"
			}
		}

		embedClient := llm.NewEmbeddingClient(embCfg)

		// Summarizer uses the same API config
		summBaseURL := cfg.LLM.BaseURL
		if summBaseURL == "" {
			switch cfg.LLM.Provider {
			case "openrouter":
				summBaseURL = "https://openrouter.ai/api/v1"
			default:
				summBaseURL = "https://api.openai.com/v1"
			}
		}
		summarizer := compression.NewOpenAISummarizer(summBaseURL, cfg.LLM.APIKey)

		// Configure compression worker
		workerCfg := compression.DefaultWorkerConfig()
		workerCfg.PollInterval = time.Duration(cfg.Compression.PollIntervalSeconds) * time.Second
		workerCfg.BatchSize = cfg.Compression.BatchSize
		workerCfg.CosineThreshold = cfg.Compression.CosineThreshold
		if cfg.Compression.EmbeddingModel != "" {
			workerCfg.EmbeddingModel = cfg.Compression.EmbeddingModel
		}

		compWorker := compression.NewWorker(database, embedClient, summarizer, workerCfg)

		// Wire billing tracker
		bt := billing.NewTracker(database)
		compWorker.SetBillingTracker(func(ctx context.Context, sessionID string, iteration int64, modelID, category string, promptTokens, completionTokens int64, costUSD float64) {
			bt.RecordBilling(ctx, sessionID, iteration, modelID, category, promptTokens, completionTokens, 0, 0, costUSD)
		})

		compWorker.Start(ctx)
		slog.Info("conscience: compression worker enabled",
			"model", workerCfg.EmbeddingModel,
			"interval", workerCfg.PollInterval,
			"threshold", workerCfg.CosineThreshold,
		)
	}

	// API Server (REST endpoints + SSE) — declared early so the quarantine service
	// can reference its EventBus. Constructed fully after HITL setup.
	var apiSrv *api.Server

	// Cognitive Firewall (Quarantine Scanner) — SPEC-005 §Cognitive Firewall, WI-004
	// Wire the quarantine service into both the API server and the webhook handler.
	quarantineSvc := quarantine.NewQuarantineService(database, func(sessionID, eventType string, eventData any) {
		if apiSrv != nil && apiSrv.EventBus() != nil {
			apiSrv.EventBus().PublishQuarantineEvent(sessionID, eventType, eventData)
		}
	})

	// Wire cognitive firewall scanner into webhook handler (WI-004)
	whStore := webhook.New(database)
	scannerAdapter := quarantine.NewWebhookScannerAdapter()
	whStore.SetQuarantineScanner(scannerAdapter)
	whStore.SetQuarantineInserter(quarantine.NewQuarantineInserter(quarantineSvc))

	// Start Go-level event routing loop — polls pending events, matches rules,
	// wakes target sessions (SPEC-013 §5).
	go whStore.StartRoutingLoop(ctx, 5*time.Second)

	// HITL Manager — wire into server and start cron (SPEC-014 §5.3, sweep-017)
	hitlMgr := hitl.New(database)
	// Initialize default global HITL config (idempotent — migration 008 also inserts defaults)
	if err := hitlMgr.SetConfiguration(ctx, hitl.DefaultConfiguration()); err != nil {
		slog.Warn("conscience: failed to init default HITL config", "error", err)
	}
	// Start expiry cron — every 5 minutes, expires stale pending approvals
	hitlMgr.StartExpiryCron(ctx, 5*time.Minute)
	slog.Info("conscience: HITL manager started", "expiry_interval", "5m")

	// API Server (REST endpoints + SSE)
	apiSrv = api.NewServer(api.ServerConfig{
		Addr:              addrString(cfg.Server),
		DB:                database,
		HITL:              hitlMgr,
		QuarantineService: quarantineSvc,
		AdminRate:         cfg.APIRate.AdminLimit,
		SessionRate:       cfg.APIRate.SessionLimit,
		ReadonlyRate:      cfg.APIRate.ReadonlyLimit,
		WebhookRate:       cfg.APIRate.WebhookLimit,
	})

	// Wire real-time event streams based on backend type (SPEC-015 §4).
	// Postgres: LISTEN/NOTIFY triggers → NotificationListener → EventBus.
	// SQLite: polling goroutine → EventBus.
	switch database.Backend() {
	case db.BackendPostgres:
		// Postgres: Use NotificationListener with dedicated connection for LISTEN/NOTIFY.
		// The NotificationListener subscribes to session_events and approval_events
		// channels and forwards parsed notifications to the EventBus.
		if pgDB, ok := database.(*postgres.DB); ok {
			listener := postgres.NewNotificationListener(pgDB.Pool())
			listener.AddChannel("session_events")
			listener.AddChannel("approval_events")
			listener.SetHandler(func(n postgres.Notification) {
				handler := api.PostgresNotificationHandler(apiSrv.EventBus())
				handler(n.Channel, n.Payload)
			})
			if err := listener.Start(); err != nil {
				slog.Warn("conscience: postgres event listener failed to start", "error", err)
			} else {
				slog.Info("conscience: postgres LISTEN/NOTIFY event bridge started")
				// Clean up listener on shutdown
				go func() {
					<-ctx.Done()
					listener.Stop()
				}()
			}
		}
	case db.BackendSQLite:
		// SQLite: Start polling goroutine that checks audit_logs and approval_requests.
		// Wrap database.Query to convert []db.Row to []map[string]any for the poller.
		queryFn := func(ctx interface{}, query string, args ...any) ([]map[string]any, error) {
			rows, err := database.Query(context.Background(), query, args...)
			if err != nil {
				return nil, err
			}
			result := make([]map[string]any, len(rows))
			for i, row := range rows {
				result[i] = map[string]any(row)
			}
			return result, nil
		}
		stopPoller := api.StartSQliteEventPoller(ctx, apiSrv.EventBus(), queryFn)
		slog.Info("conscience: SQLite event polling started")
		_ = stopPoller // kept for future explicit stop if needed
	}

	// Build an EventBus bridge for the shim to use for SSE streaming.
	// This wraps the API server's EventBus so the shim gets real-time events
	// without importing the api package directly.
	shimEvents := &shimEventBridge{srv: apiSrv}

	// MCP Server (tools, resources, prompts via JSON-RPC over SSE)
	mcpSrv := mcp.NewServer(database)

	// Combined handler tree: API handles /api/..., MCP handles /mcp/...
	// chi.Router provides both ServeHTTP and Handle methods for sub-path mounting.
	apiMux := apiSrv.Handler().(chi.Router)
	apiMux.Handle("/mcp/", mcpSrv.Handler())

	// Webhook ingestion endpoint (SPEC-013 §4)
	// Mount at /webhooks/ — no API key required (HMAC signature verification instead).
	apiMux.Handle("/webhooks/", whStore)

	// Web Admin UI (SPEC-016 §12) — dark-themed dashboard for sessions, memory, health.
	// Served at /ui/ — the UI proxies API calls through its own /api/ path.
	webUI := web.NewServer("http://" + addrString(cfg.Server))
	apiMux.Handle("/ui/", http.StripPrefix("/ui", webUI.Handler()))

	// opencode Protocol Shim (SPEC-017) — translates opencode server protocol
	// into native Conscience API calls. Enabled by default.
	if cfg.Adapters.OpenCode.Enabled {
		shimService := opencode.NewServiceAdapter(apiSrv.Service())
		shimSrv := opencode.NewServer(database, cfg.Adapters.OpenCode.AdminKey, shimEvents, shimService)
		// Mount shim at root — it handles paths like /session, /config, /event, etc.
		apiMux.Handle("/global/", shimSrv.Handler())
		apiMux.Handle("/session", shimSrv.Handler())
		apiMux.Handle("/session/", shimSrv.Handler())
		apiMux.Handle("/config", shimSrv.Handler())
		apiMux.Handle("/config/", shimSrv.Handler())
		apiMux.Handle("/provider", shimSrv.Handler())
		apiMux.Handle("/provider/", shimSrv.Handler())
		apiMux.Handle("/agent", shimSrv.Handler())
		apiMux.Handle("/agent/", shimSrv.Handler())
		apiMux.Handle("/experimental/", shimSrv.Handler())
		apiMux.Handle("/find", shimSrv.Handler())
		apiMux.Handle("/find/", shimSrv.Handler())
		apiMux.Handle("/file/", shimSrv.Handler())
		apiMux.Handle("/event", shimSrv.Handler())
		apiMux.Handle("/permission", shimSrv.Handler())
		apiMux.Handle("/permission/", shimSrv.Handler())
		apiMux.Handle("/tui/", shimSrv.Handler())
		apiMux.Handle("/lsp", shimSrv.Handler())
		apiMux.Handle("/lsp/", shimSrv.Handler())
		apiMux.Handle("/doc", shimSrv.Handler())
		apiMux.Handle("/doc/", shimSrv.Handler())
		apiMux.Handle("/auth/", shimSrv.Handler())
		apiMux.Handle("/project", shimSrv.Handler())
		apiMux.Handle("/project/", shimSrv.Handler())
		apiMux.Handle("/vcs", shimSrv.Handler())
		apiMux.Handle("/vcs/", shimSrv.Handler())
		slog.Info("conscience: opencode shim enabled")
	}

	slog.Info("conscience: starting", "addr", addrString(cfg.Server))

	// Start API server (which now serves both API and MCP routes)
	go func() {
		if err := apiSrv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "conscience: %v\n", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("conscience: shutting down")
}

// runMCPStdio starts the MCP server in stdio transport mode.
// Reads JSON-RPC 2.0 from stdin, writes responses to stdout.
// Logs go to stderr.
// axiom:trace work_item=WI-015 spec=specs/015-api-and-mcp.md plan=phase-5/task-5-1/step-5-1-1 impl=cmd/conscience/main.go
func runMCPStdio() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	database, err := dbdriver.Open(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Auto-migrate schema on startup (SPEC-009 §6)
	migrator := migrate.New(database)
	migrated, err := migrator.AutoMigrate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	if migrated {
		slog.Info("conscience: schema migrations applied")
	}

	// Ensure bootstrap admin key exists
	adminKey, err := bootstrap.EnsureFirstAdminKey(ctx, database, bootstrap.GetBootstrapKeyTTL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
		os.Exit(1)
	}
	// Print admin key info to stderr (not stdout — MCP client reads stdout)
	for _, line := range bootstrap.FormatResult(adminKey) {
		slog.Info("bootstrap", "info", line)
	}

	// Create MCP server and start stdio transport
	mcpSrv := mcp.NewServer(database)
	slog.Info("conscience: starting MCP stdio transport")

	if err := mcpSrv.ServeStdio(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "conscience: mcp stdio: %v\n", err)
		os.Exit(1)
	}

	slog.Info("conscience: MCP stdio transport shut down")
}

// runMigrate runs database migrations directly against a database (specified by --db-url).
// This enables offline migration without a running server (CS-GAP-014).
// axiom:trace work_item=WI-010 spec=specs/016-cli-interface.md impl=cmd/conscience/main.go
func runMigrate(action string, dbURL string) error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if dbURL != "" {
		cfg.Database.URL = dbURL
	}

	database, err := dbdriver.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer database.Close()

	m := migrate.New(database)

	switch action {
	case "up":
		if err := m.Bootstrap(ctx); err != nil {
			return fmt.Errorf("migrate bootstrap: %w", err)
		}
		if err := m.LoadMigrations(); err != nil {
			return fmt.Errorf("migrate load: %w", err)
		}
		applied, err := m.Up(ctx)
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			fmt.Printf("Applied %d migration(s):\n", len(applied))
			for _, fn := range applied {
				fmt.Printf("  - %s\n", fn)
			}
		} else {
			fmt.Println("Schema is current — no pending migrations.")
		}

	case "down":
		result, err := m.Down(ctx)
		if err != nil {
			return err
		}
		fmt.Println(result)

	case "status":
		if err := m.Bootstrap(ctx); err != nil {
			return fmt.Errorf("migrate bootstrap: %w", err)
		}
		if err := m.LoadMigrations(); err != nil {
			return fmt.Errorf("migrate load: %w", err)
		}
		state, err := m.GetState(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Schema version: %d\n", state.CurrentVersion)
		if len(state.AppliedMigrations) > 0 {
			fmt.Printf("Applied: %d\n", len(state.AppliedMigrations))
		}
		if len(state.PendingMigrations) > 0 {
			fmt.Printf("Pending: %d\n", len(state.PendingMigrations))
		}
		if state.DriftDetected {
			fmt.Printf("DRIFT DETECTED:\n%s\n", state.DriftDetails)
		}

	default:
		return fmt.Errorf("unknown migrate action: %s (expected up, down, or status)", action)
	}

	return nil
}

// runInit is the init command implementation (SPEC-016 §5.2).
func runInit(dbURL string) error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if dbURL != "" {
		cfg.Database.URL = dbURL
	}

	database, err := dbdriver.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer database.Close()

	migrated, err := migrate.New(database).AutoMigrate(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	result, err := bootstrap.EnsureFirstAdminKey(ctx, database, bootstrap.GetBootstrapKeyTTL())
	if err != nil {
		return err
	}

	fmt.Println("conscience: init")
	fmt.Printf("Database:      %s\n", cfg.Database.URL)
	if migrated {
		fmt.Println("Migrations:    applied")
	} else {
		fmt.Println("Migrations:    current")
	}
	// Bootstrap output: admin key uses consistent machine-parseable format (SPEC-016 §3).
	// axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md impl=cmd/conscience/main.go
	// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-002 impl=cmd/conscience/main.go
	for _, line := range bootstrap.FormatResult(result) {
		fmt.Println(line)
	}
	if !result.Created {
		fmt.Println("Existing admin secrets cannot be recovered; create or rotate keys via the API if needed.")
	}
	fmt.Printf("Server URL:    http://%s\n", addrString(cfg.Server))
	fmt.Println("Config file:   ./conscience.yaml or ~/.conscience/config.yaml")
	return nil
}

func addrString(sc config.ServerConfig) string {
	return fmt.Sprintf("%s:%d", sc.Hostname, sc.Port)
}

// resolveLLMBaseURL returns the LLM provider base URL from config or environment.
// Supports OpenRouter via CONSCIENCE_LLM_BASE_URL or OPENROUTER_BASE_URL env vars.
func resolveLLMBaseURL(cfg config.Config) string {
	// Config file takes priority
	if cfg.LLM.BaseURL != "" {
		return cfg.LLM.BaseURL
	}
	// Environment variable overrides
	if v := os.Getenv("CONSCIENCE_LLM_BASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("OPENROUTER_BASE_URL"); v != "" {
		return v
	}
	// Let NewOpenAIClient pick the default based on provider
	return ""
}

// shimEventBridge adapts the api.Server's EventBus to the opencode shim's
// EventBus interface, so the shim can receive and emit real-time events
// without importing the api package directly.
type shimEventBridge struct {
	srv *api.Server
}

func (b *shimEventBridge) Listen(sessionID string, listener opencode.EventListener) func() {
	if b.srv == nil || b.srv.EventBus() == nil {
		return func() {} // no-op
	}

	id, ch := b.srv.EventBus().Subscribe(sessionID)

	// Start a goroutine to forward events to the listener
	// EventListener signature: func(sessionID string, eventType string, data any)
	go func() {
		for event := range ch {
			listener(event.SessionID, event.Type, event.Data)
		}
	}()

	return func() {
		b.srv.EventBus().Unsubscribe(sessionID, id)
	}
}

func (b *shimEventBridge) Emit(sessionID string, eventType string, data any) {
	if b.srv == nil || b.srv.EventBus() == nil {
		return
	}
	b.srv.EventBus().Publish(sessionID, api.Event{
		SessionID: sessionID,
		Type:      eventType,
		Data:      data,
	})
}
