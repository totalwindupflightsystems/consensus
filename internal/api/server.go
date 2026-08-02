// Package api implements the REST API handlers for Consensus (SPEC-015).
//
// The API layer is a thin HTTP interface to the same database the agent
// harness reads from. There is no separate API state — the database is the
// single source of truth.
//
// Uses go-chi/chi/v5 router for clean route grouping, middleware, and path parameters.
// axiom:trace work_item=WI-011 spec=specs/015-api-and-mcp.md,specs/022-library-research.md plan=.memory-bank/work-items/WI-011/plan.md impl=internal/api/server.go
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/hitl"
	"github.com/wojons/consensus/internal/quarantine"
)

// startTime records when the server started, used for uptime calculation.
var startTime time.Time

// ============================================================================
// Server
// ============================================================================

// Server is the HTTP API server with middleware and endpoint routing.
type Server struct {
	db db.DB
	// admindb bypasses SET ROLE / RLS for admin queries that must always
	// succeed regardless of pool state (health checks). Falls back to db
	// when no admin pool is configured (SQLite or tests).
	admindb db.DB

	svc    *Service // service layer — shared with shims
	router chi.Router
	events *EventBus
	hitl   *hitl.Manager

	quarantineSvc *quarantine.QuarantineService // cognitive firewall (optional)

	addr string

	mu sync.RWMutex

	// apiRates maps scope name to requests-per-minute limit.
	// Initialized from ServerConfig with defaults fallback.
	apiRates map[string]int
}

// ServerConfig holds API server configuration.
type ServerConfig struct {
	Addr string // listen address, e.g. ":8090"
	DB   db.DB
	// AdminDB is an optional handle that bypasses SET ROLE / RLS (the
	// Postgres admin pool). Used for health checks and other operational
	// queries that must succeed even when the agent pool is exhausted.
	// When nil, NewServer falls back to DB for backward compatibility.
	AdminDB db.DB
	HITL    *hitl.Manager // HITL Manager for approval lifecycle (optional, defaults to nil)

	// QuarantineService is the cognitive firewall service (optional).
	// When set, enables quarantine API endpoints and SSE events.
	QuarantineService *quarantine.QuarantineService

	// Per-scope API rate limits (req/min). 0 = use default for that scope.
	AdminRate    int
	SessionRate  int
	ReadonlyRate int
	WebhookRate  int
}

// NewServer creates a new API server with all middleware and routes.
func NewServer(cfg ServerConfig) *Server {
	startTime = time.Now()
	// Health checks run through admindb to avoid the SET ROLE pool, which
	// can exhaust or hang in Docker. Fall back to the main DB when no
	// admin pool is configured (SQLite, tests, single-pool deployments).
	adminDB := cfg.AdminDB
	if adminDB == nil {
		adminDB = cfg.DB
	}
	s := &Server{
		db:            cfg.DB,
		admindb:       adminDB,
		addr:          cfg.Addr,
		events:        NewEventBus(),
		svc:           NewService(cfg.DB, nil), // events set below
		hitl:          cfg.HITL,
		quarantineSvc: cfg.QuarantineService,
		apiRates:      resolveRates(cfg),
	}
	s.svc.Sessions.events = s.events
	s.svc.Messages.events = s.events

	r := chi.NewRouter()

	// CORS middleware on all routes
	r.Use(s.allowCORSMiddleware)

	// Serve OpenAPI spec (no auth)
	s.registerOpenAPIRoutes(r)

	// Health (no auth)
	r.Get("/api/v1/health", s.handleHealth)

	// SSE event stream (no auth — session isolation via query param)
	r.Get("/api/v1/events", s.HandleSSE)

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		// Sessions
		r.Post("/api/v1/sessions", s.handleCreateSession)
		r.Get("/api/v1/sessions", s.handleListSessions)
		r.Get("/api/v1/sessions/{id}", extractSessionID(s.handleGetSession))
		r.Patch("/api/v1/sessions/{id}", extractSessionID(s.handleUpdateSession))
		r.Delete("/api/v1/sessions/{id}", extractSessionID(s.handleDeleteSession))
		r.Post("/api/v1/sessions/{id}/message", extractSessionID(s.handleSessionMessage))

		// Session sub-resources
		r.Get("/api/v1/sessions/{id}/memory", extractSessionID(s.handleListMemory))
		r.Get("/api/v1/sessions/{id}/memory/{memoryID}", extractSessionAndMemoryID(s.handleGetMemoryEvent))
		r.Get("/api/v1/sessions/{id}/context", extractSessionID(s.handleGetActiveContext))
		r.Get("/api/v1/sessions/{id}/iterations", extractSessionID(s.handleListIterations))
		r.Get("/api/v1/sessions/{id}/tasks", extractSessionID(s.handleListTasks))
		r.Post("/api/v1/sessions/{id}/tasks", extractSessionID(s.handleCreateTask))
		r.Get("/api/v1/sessions/{id}/approvals", extractSessionID(s.handleSessionApprovals))
		r.Get("/api/v1/sessions/{id}/billing", extractSessionID(s.handleGetSessionBilling))

		// Tasks
		r.Patch("/api/v1/tasks/{taskID}", extractTaskID(s.handleUpdateTask))
		r.Post("/api/v1/tasks/{taskID}/claim", extractTaskID(s.handleClaimTask))

		// Tools & Skills
		r.Get("/api/v1/tools", s.handleListTools)
		r.Get("/api/v1/skills", s.handleListSkills)
		r.Get("/api/v1/skills/{skillName}", extractSkillName(s.handleGetSkill))
		r.Post("/api/v1/tools/{toolName}/execute", extractToolName(s.handleExecuteTool))

		// Approvals
		r.Get("/api/v1/approvals", s.handleListApprovals)
		r.Get("/api/v1/approvals/{approvalID}", extractApprovalID(s.handleGetApproval))
		r.Post("/api/v1/approvals/{approvalID}/review", extractApprovalID(s.handleReviewApproval))

		// Config
		r.Get("/api/v1/config", s.handleGetConfig)

		// Metrics (also accessible with readonly scope)
		r.Get("/api/v1/metrics", s.handleGetMetrics)

		// Auth — API key management
		r.Post("/api/v1/auth/keys", s.handleCreateAPIKey)
		r.Get("/api/v1/auth/keys", s.handleListAPIKeys)
		r.Delete("/api/v1/auth/keys/{keyID}", extractKeyID(s.handleDeleteAPIKey))

		// Quarantine (Cognitive Firewall)
		r.Get("/api/v1/quarantine", s.handleListQuarantine)
		r.Post("/api/v1/quarantine/{qID}/approve", extractQuarantineID(s.handleApproveQuarantine))
		r.Post("/api/v1/quarantine/{qID}/reject", extractQuarantineID(s.handleRejectQuarantine))
	})

	s.router = r
	return s
}

// ============================================================================
// Path parameter extractors — bridge chi URL params to existing handler signatures
// ============================================================================

// uuidPattern matches a valid UUID format (8-4-4-4-12 hex digits).
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// sessionHandler is a handler that takes a session ID.
type sessionHandler func(w http.ResponseWriter, r *http.Request, sessionID string)

func extractSessionID(h sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		// If the ID is UUID-length (36 chars) but doesn't match UUID format,
		// reject with 400 instead of letting it fall through to a 404.
		// Shorter IDs (test fixtures) pass through to the DB layer.
		if len(id) == 36 && !uuidPattern.MatchString(id) {
			writeError(w, r, http.StatusBadRequest, "INVALID_UUID", "session ID must be a valid UUID: "+id)
			return
		}
		h(w, r, id)
	}
}

// sessionMemoryHandler is a handler that takes session ID and memory ID.
type sessionMemoryHandler func(w http.ResponseWriter, r *http.Request, sessionID, memoryID string)

func extractSessionAndMemoryID(h sessionMemoryHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "id"), chi.URLParam(r, "memoryID"))
	}
}

// taskHandler is a handler that takes a task ID.
type taskHandler func(w http.ResponseWriter, r *http.Request, taskID string)

func extractTaskID(h taskHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "taskID"))
	}
}

// skillHandler is a handler that takes a skill name.
func extractSkillName(h func(w http.ResponseWriter, r *http.Request, name string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "skillName"))
	}
}

// toolHandler is a handler that takes a tool name.
func extractToolName(h func(w http.ResponseWriter, r *http.Request, name string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "toolName"))
	}
}

// approvalHandler is a handler that takes an approval ID.
type approvalHandler func(w http.ResponseWriter, r *http.Request, approvalID string)

func extractApprovalID(h approvalHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "approvalID"))
	}
}

// keyHandler is a handler that takes a key ID.
func extractKeyID(h func(w http.ResponseWriter, r *http.Request, keyID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, chi.URLParam(r, "keyID"))
	}
}

// quarantineHandler is a handler that takes a quarantine ID (int64).
type quarantineHandler func(w http.ResponseWriter, r *http.Request, qID int64)

func extractQuarantineID(h quarantineHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "qID")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_ID", "quarantine ID must be an integer")
			return
		}
		h(w, r, id)
	}
}

// Start begins listening and serving HTTP.
func (s *Server) Start() error {
	slog.Info("api: starting", "addr", s.addr)
	return http.ListenAndServe(s.addr, s.router)
}

// Handler returns the underlying HTTP handler for testing and composability.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Service returns the service layer, allowing shims to share business logic.
func (s *Server) Service() *Service {
	return s.svc
}

// EventBus returns the server's event bus for publishing SSE events.
func (s *Server) EventBus() *EventBus {
	return s.events
}

// StartContext starts listening and shuts down when context is cancelled.
func (s *Server) StartContext(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.router,
	}

	go func() {
		<-ctx.Done()
		slog.Info("api: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("api: starting", "addr", s.addr)
	return srv.ListenAndServe()
}

// ============================================================================
// Auth Middleware (SPEC-015 §2)
// ============================================================================

// authMiddleware validates API keys and enforces scope restrictions.
// The health endpoint is excluded from auth via separate route registration.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractBearerToken(r)
		if key == "" {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "missing API key")
			return
		}

		prefix := key[:min(8, len(key))]
		hash := sha256Hash(key)

		ctx := r.Context()
		rows, err := s.db.Query(ctx,
			`SELECT id, scope, session_id FROM api_keys WHERE key_prefix = $1 AND key_hash = $2 AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
			prefix, hash,
		)
		if err != nil || len(rows) == 0 {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid or expired API key")
			return
		}

		// Extract scope from DB row BEFORE rate limit check so we can
		// apply scope-specific rate limits (SPEC-015 §7.1).
		scope := toString(rows[0]["scope"])

		if !s.checkRateLimit(ctx, prefix, scope) {
			writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}

		// Store auth context for downstream handlers
		ctx = context.WithValue(ctx, ctxKeyScope, scope)
		if sid := rows[0]["session_id"]; sid != nil {
			ctx = context.WithValue(ctx, ctxKeySessionID, toString(sid))
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) >= 8 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	// The Chronicle dashboard (chronicle/index.html) authenticates with an
	// X-Api-Key header (apiHeaders()). Accept it here so the UI can talk to
	// the same server it's served from.
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return k
	}
	return ""
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ============================================================================
// Rate Limiting (SPEC-015 §7)
// ============================================================================

var defaultRateLimits = map[string]int{
	"admin":    1000,
	"session":  100,
	"readonly": 200,
	"webhook":  500,
}

// resolveRates builds the active per-scope rate limit map from config,
// falling back to package-level defaults for any scope with a zero value.
func resolveRates(cfg ServerConfig) map[string]int {
	rates := make(map[string]int, 4)
	for scope, def := range defaultRateLimits {
		rates[scope] = def
	}
	if cfg.AdminRate > 0 {
		rates["admin"] = cfg.AdminRate
	}
	if cfg.SessionRate > 0 {
		rates["session"] = cfg.SessionRate
	}
	if cfg.ReadonlyRate > 0 {
		rates["readonly"] = cfg.ReadonlyRate
	}
	if cfg.WebhookRate > 0 {
		rates["webhook"] = cfg.WebhookRate
	}
	return rates
}

// scopeRateLimit returns the rate limit for a given scope, defaulting to
// "session" limits (100 req/min) for unknown scopes.
func (s *Server) scopeRateLimit(scope string) int {
	if limit, ok := s.apiRates[scope]; ok {
		return limit
	}
	return defaultRateLimits["session"]
}

func (s *Server) checkRateLimit(ctx context.Context, prefix string, scope string) bool {
	limit := s.scopeRateLimit(scope)

	rows, err := s.db.Query(ctx, `SELECT requests_count, window_start FROM api_rate_limits WHERE key_prefix = $1`, prefix)
	if err != nil {
		return true
	}

	if len(rows) == 0 {
		s.db.Exec(ctx, `INSERT INTO api_rate_limits (key_prefix, requests_count, window_start) VALUES ($1, 1, CURRENT_TIMESTAMP)`, prefix)
		return true
	}

	windowStart := toString(rows[0]["window_start"])
	count := toInt(rows[0]["requests_count"])

	t, err := time.Parse(time.RFC3339, windowStart)
	if err != nil {
		t, _ = time.Parse("2006-01-02 15:04:05", windowStart)
	}

	if time.Since(t) > time.Minute {
		s.db.Exec(ctx, `UPDATE api_rate_limits SET requests_count = 1, window_start = CURRENT_TIMESTAMP WHERE key_prefix = $1`, prefix)
		return true
	}

	if count >= limit {
		return false
	}

	s.db.Exec(ctx, `UPDATE api_rate_limits SET requests_count = requests_count + 1 WHERE key_prefix = $1`, prefix)
	return true
}

// ============================================================================
// CORS Middleware
// ============================================================================

func (s *Server) allowCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// Response Helpers
// ============================================================================

// APIError is the standard error envelope (SPEC-015 §6).
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ErrorResponse is the JSON error response body.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, _ := json.Marshal(ErrorResponse{Error: APIError{Code: code, Message: message, Details: message}})
	w.Write(data)
	slog.Warn("api: error", "method", r.Method, "path", r.URL.Path, "status", status, "code", code)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(v)
	if data != nil {
		w.Write(data)
	}
}

// ============================================================================
// Health Endpoint
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	backend := string(s.db.Backend())

	// Uptime from package-level startTime
	uptime := int64(time.Since(startTime).Seconds())

	// DB ping latency
	dbLatency := 0.0
	pingStart := time.Now()
	_, err := s.admindb.Query(ctx, "SELECT 1")
	if err == nil {
		dbLatency = float64(time.Since(pingStart).Microseconds()) / 1000.0
	}

	// DB path
	dbPath := ""
	if backend == "sqlite" {
		rows, qErr := s.admindb.Query(ctx, "SELECT file FROM pragma_database_list WHERE name = 'main'")
		if qErr == nil && len(rows) > 0 {
			dbPath = toString(rows[0]["file"])
		}
	} else {
		row, qErr := s.admindb.QueryRow(ctx, "SELECT current_database() AS db")
		if qErr == nil {
			dbPath = toString(row["db"])
		}
	}

	// DB size
	dbSizeMB := 0.0
	if backend == "sqlite" && dbPath != "" {
		if fi, statErr := os.Stat(dbPath); statErr == nil {
			dbSizeMB = float64(fi.Size()) / (1024.0 * 1024.0)
		}
	} else if backend == "postgres" {
		row, qErr := s.admindb.QueryRow(ctx, "SELECT pg_database_size(current_database()) AS size")
		if qErr == nil {
			if size, ok := row["size"].(int64); ok {
				dbSizeMB = float64(size) / (1024.0 * 1024.0)
			}
		}
	}

	// User table count
	dbTables := 0
	var tablesQuery string
	if backend == "sqlite" {
		tablesQuery = "SELECT count(*) AS cnt FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
	} else {
		tablesQuery = "SELECT count(*) AS cnt FROM information_schema.tables WHERE table_schema = 'public'"
	}
	if rows, qErr := s.admindb.Query(ctx, tablesQuery); qErr == nil && len(rows) > 0 {
		dbTables = toInt(rows[0]["cnt"])
	}

	// Migration count + schema version (table is schema_versions, not migrations)
	dbMigrations := 0
	schemaVersion := 0
	if rows, qErr := s.admindb.Query(ctx, "SELECT count(*) AS cnt FROM schema_versions"); qErr == nil && len(rows) > 0 {
		dbMigrations = toInt(rows[0]["cnt"])
	}
	if rows, qErr := s.admindb.Query(ctx, "SELECT COALESCE(MAX(version), 0) AS v FROM schema_versions"); qErr == nil && len(rows) > 0 {
		schemaVersion = toInt(rows[0]["v"])
	}

	writeJSON(w, HealthResponse{
		Status:        "ok",
		Version:       "0.1.0",
		UptimeSeconds: uptime,
		APILatencyMs:  0,
		DBLatencyMs:   dbLatency,
		LLMLatencyMs:  0,
		ErrorRatePct:  0,
		DBBackend:     backend,
		DBPath:        dbPath,
		DBSizeMB:      dbSizeMB,
		DBTables:      dbTables,
		DBMigrations:  dbMigrations,
		SchemaVersion: schemaVersion,
		ActiveConnections: ActiveConnections{
			WebSocket:          0,
			DBPoolActive:       0,
			DBPoolMax:          0,
			LLMActive:          0,
			APIRequestsLastMin: 0,
		},
		SystemLog: []string{},
	})
}

// ============================================================================
// Stub Handlers — not yet implemented
// ============================================================================

// Real handlers: tools, skills, memory, context, sessions, tasks, approvals, billing, config, metrics, auth — all in their respective .go files.
// No remaining stubs — all SPEC-015 endpoint families have real implementations.

// ============================================================================
// Session Access Check (SPEC-015 §2 scoping)
// ============================================================================

// checkSessionAccess verifies that the authenticated user can access the given session.
// Session-scoped keys can only access their own session.
// Admin/readonly keys can access any session.
func (s *Server) checkSessionAccess(w http.ResponseWriter, r *http.Request, targetSessionID string) bool {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)

	if scope == "session" && sessionID != targetSessionID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key can only access its own session")
		return false
	}
	return true
}

// ============================================================================
// Context Keys
// ============================================================================

type ctxKey string

const (
	ctxKeyScope     ctxKey = "api.scope"
	ctxKeySessionID ctxKey = "api.session_id"
)

// GetAuthScope returns the authenticated API key scope from the request context.
func GetAuthScope(r *http.Request) string {
	if v := r.Context().Value(ctxKeyScope); v != nil {
		return v.(string)
	}
	return ""
}

// GetAuthSessionID returns the session ID from the authenticated API key.
func GetAuthSessionID(r *http.Request) string {
	if v := r.Context().Value(ctxKeySessionID); v != nil {
		return v.(string)
	}
	return ""
}

// ============================================================================
// Helpers
// ============================================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// guard test
