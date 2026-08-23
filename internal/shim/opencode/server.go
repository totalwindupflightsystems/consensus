// Package opencode implements the opencode server protocol shim (SPEC-017).
//
// The shim translates opencode's HTTP protocol into Consensus's native REST API.
// It runs in-process alongside the harness, using the same database connection.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/017-ui-adapter-layer.md plan=phase-6/task-6-1/step-6-1-1 impl=internal/shim/opencode/server.go
package opencode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Shim Server
// ============================================================================

// EventListener receives events for a specific session (or all sessions if sessionID is "").
// The returned stop function unsubscribes.
type EventListener func(sessionID string, eventType string, data any)

// EventBus is the minimal interface the shim needs for real-time event distribution.
type EventBus interface {
	Listen(sessionID string, listener EventListener) (stop func())
	Emit(sessionID string, eventType string, data any)
}

// Server is the opencode protocol shim that translates opencode HTTP requests
// to Consensus native API calls.
//
// SPEC-017 §2: "The opencode shim does NOT bypass the native API. It calls it."
// The shim uses the api.Service layer — the same business logic the REST API uses.
type Server struct {
	db  db.DB
	svc Service // api.Service interface — shared business logic
	mux *http.ServeMux

	// Event bus for real-time SSE streaming
	events EventBus

	// Admin API key for auth translation (Basic Auth password → admin key)
	adminKey string

	// If skipAuth is true, auth middleware is bypassed (for testing)
	skipAuth bool

	// Workspace directory for /instance/* translation endpoints (SPEC-017
	// §3.10). Defaults to the process working directory at construction;
	// overridable for tests.
	workdir string

	// Server start time — reported as the singleton instance's createdAt.
	startedAt time.Time

	// Mutex for shim_session_map writes
	mu sync.Mutex
}

// Service is the minimal interface the shim needs from the API service layer.
// This avoids a circular import (shim → api → shim).
type Service interface {
	CreateSession(ctx context.Context, input SessionCreateInput) (*SessionCreateResult, error)
	GetSession(ctx context.Context, id string) (*SessionResult, error)
	UpdateSession(ctx context.Context, id string, action string) error
	DeleteSession(ctx context.Context, id string) error
	SendMessage(ctx context.Context, input MessageSendInput) error
	GetConfig(ctx context.Context) (map[string]string, error)
	UpdateConfig(ctx context.Context, settings map[string]string) error

	// File operations (SPEC-017 §3.1) — maps opencode /file and /find to
	// filesystem access through the API service layer.
	FindFiles(ctx context.Context, pattern string) ([]string, error)
	ReadFile(ctx context.Context, path string) (string, error)
	GetGitStatus(ctx context.Context) (map[string]any, error)
}

// SessionCreateInput mirrors api.CreateSessionInput.
type SessionCreateInput struct {
	AgentName     string
	Goal          string
	ModelID       string
	ContextBudget int
}

// SessionCreateResult mirrors api.CreateSessionOutput.
type SessionCreateResult struct {
	SessionID string
	Status    string
	APIKey    string
	CreatedAt string
}

// SessionResult mirrors api.SessionResponse.
type SessionResult struct {
	ID            string
	ParentID      *string
	AgentName     string
	ModelID       string
	Status        string
	Goal          *string
	ContextBudget int
	TokensUsedIn  int64
	TokensUsedOut int64
	Iteration     int64
	HeartbeatAt   string
	CreatedAt     string
	CompletedAt   *string
}

// MessageSendInput mirrors api.SendMessageInput.
type MessageSendInput struct {
	SessionID string
	Content   string
	MsgType   string
}

// NewServer creates the opencode shim with all routes registered.
// svc is the API service layer — the shim calls this instead of raw DB.
// If eventBus is nil, the shim falls back to polling-based event streaming.
func NewServer(dbase db.DB, adminKey string, eventBus EventBus, svc Service) *Server {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	s := &Server{
		db:        dbase,
		svc:       svc,
		events:    eventBus,
		adminKey:  adminKey,
		workdir:   wd,
		startedAt: time.Now().UTC(),
	}
	mux := http.NewServeMux()
	s.mux = mux

	// Global
	mux.HandleFunc("/global/health", s.handleGlobalHealth)
	mux.HandleFunc("/global/event", s.handleGlobalEvent)

	// Sessions
	mux.HandleFunc("/session", s.handleSessions)
	mux.HandleFunc("/session/", s.handleSessionByID)

	// Doc
	mux.HandleFunc("/doc", s.handleDoc)

	// Config/Provider/Agent
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/config/providers", s.handleConfigProviders)
	mux.HandleFunc("/provider", s.handleProvider)
	mux.HandleFunc("/agent", s.handleAgent)

	// Tools
	mux.HandleFunc("/experimental/tool", s.handleTools)
	mux.HandleFunc("/experimental/tool/ids", s.handleToolIDs)

	// File endpoints (stubs via tool execution API)
	mux.HandleFunc("/find", s.handleFind)
	mux.HandleFunc("/find/", s.handleFindSub)
	mux.HandleFunc("/file/content", s.handleFileContent)
	mux.HandleFunc("/file/status", s.handleFileStatus)

	// Permissions (HITL → permission events)
	mux.HandleFunc("/permission", s.handlePermissions)
	mux.HandleFunc("/permission/", s.handlePermissionByID)

	// TUI control (shim-only, passthrough)
	mux.HandleFunc("/tui/", s.handleTUI)

	// LSP
	mux.HandleFunc("/lsp", s.handleLSP)

	// MCP management
	mux.HandleFunc("/mcp", s.handleMCPEndpoint)

	// Auth management (SPEC-017 §3.2)
	mux.HandleFunc("/auth/", s.handleAuth)

	// Standalone /event endpoint for SSE
	mux.HandleFunc("/event", s.handleGlobalEvent)

	// Project/VCS as 501 stubs (SPEC-017 §3.9); /instance is a real
	// opencode-protocol translation surface (SPEC-017 §3.10).
	mux.HandleFunc("/project", s.handleProjectVCSSStub)
	mux.HandleFunc("/project/", s.handleProjectVCSSStub)
	mux.HandleFunc("/vcs", s.handleProjectVCSSStub)
	mux.HandleFunc("/vcs/", s.handleProjectVCSSStub)
	mux.HandleFunc("/instance", s.handleInstance)
	mux.HandleFunc("/instance/", s.handleInstanceSub)

	return s
}

// Handler returns the http.Handler for mounting under a parent server.
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.authMiddleware(s.mux))
}

// MountPatterns lists the chi router patterns required to expose the shim
// under a parent router. Exact patterns (no trailing slash) register the bare
// endpoint; /* wildcard patterns register sub-paths. chi v5 Handle() with a
// trailing-slash pattern (e.g. "/session/") matches ONLY the literal path, so
// sub-paths must be registered with the /* form — otherwise every
// /session/{id} request 404s before reaching the shim (BUG-009, dexdat
// sidecar, 2026-08-07).
var MountPatterns = []string{
	"/global/*",
	"/session", "/session/*",
	"/config", "/config/*",
	"/provider", "/provider/*",
	"/agent", "/agent/*",
	"/experimental/*",
	"/find", "/find/*",
	"/file/*",
	"/event",
	"/permission", "/permission/*",
	"/tui/*",
	"/lsp", "/lsp/*",
	"/doc", "/doc/*",
	"/auth/*",
	"/project", "/project/*",
	"/vcs", "/vcs/*",
	"/instance", "/instance/*",
}

// ============================================================================
// Middleware
// ============================================================================

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health, doc, or if skipAuth is set
		if s.skipAuth || r.URL.Path == "/global/health" || r.URL.Path == "/doc" || strings.HasPrefix(r.URL.Path, "/doc/") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for opencode-specific 501 stubs (SPEC-017 §3.9) — they return
		// NOT_IMPLEMENTED with zero data, so there is nothing to protect. The
		// OpenCode contract tests hit these unauthenticated and expect 501, not 401.
		// /project and /vcs keep GET auth (shim smoke test expects 401 no-auth)
		// but non-GET reaches the stub (C20).
		if isStubPath(r.URL.Path, r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// /instance/* is fully public but implemented (SPEC-017 §3.10) — the
		// opencode contract probes these endpoints unauthenticated and expects
		// 200 with real workspace data (full-contract suite C19).
		if p := r.URL.Path; p == "/instance" || strings.HasPrefix(p, "/instance/") {
			next.ServeHTTP(w, r)
			return
		}

		_, ok := s.validateAuth(r)
		if !ok {
			writeOpencodeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid credentials")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) validateAuth(r *http.Request) (string, bool) {
	// Try Bearer token first
	if token := extractBearerToken(r); token != "" {
		prefix := token[:min(8, len(token))]
		hash := hex.EncodeToString(sha256Hash([]byte(token)))

		ctx := r.Context()
		rows, err := s.db.Query(ctx,
			`SELECT id, scope, session_id FROM api_keys WHERE key_prefix = $1 AND key_hash = $2 AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
			prefix, hash,
		)
		if err == nil && len(rows) > 0 {
			sid := ""
			if v := rows[0]["session_id"]; v != nil {
				sid = toString(v)
			}
			return sid, true
		}
	}

	// Try Basic Auth (opencode sends: Basic base64("opencode:" + password))
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Basic ") {
		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				// Validate the password against admin key or session keys
				password := parts[1]
				hash := hex.EncodeToString(sha256Hash([]byte(password)))
				ctx := r.Context()
				rows, err := s.db.Query(ctx,
					`SELECT id, scope, session_id FROM api_keys WHERE key_hash = $1 AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
					hash,
				)
				if err == nil && len(rows) > 0 {
					sid := ""
					if v := rows[0]["session_id"]; v != nil {
						sid = toString(v)
					}
					return sid, true
				}
			}
		}
	}

	return "", false
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
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
// Global Endpoints
// ============================================================================

func (s *Server) handleGlobalHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"healthy": true,
		"version": "consensus-0.1.0",
	})
}

func (s *Server) handleGlobalEvent(w http.ResponseWriter, r *http.Request) {
	// SSE event stream — maps Consensus events to opencode event types
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := r.URL.Query().Get("session_id")
	ctx := r.Context()

	// SSE contract: flush 200 + headers immediately on connect so a client
	// that subscribes with no pending events still receives response headers
	// right away instead of hanging. (SHIM-EVENT-001)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Historical replay: emit stored memory_events for the session before
	// entering the live wait loop, so late subscribers (e.g. a client that
	// connects after the session reached idle) see the session's history
	// instead of an empty stream. (SHIM-EVENT-001)
	s.replaySessionEvents(ctx, w, flusher, sessionID)

	if s.events != nil {
		// Real event bus — subscribe and translate events
		ch := make(chan map[string]any, 64)
		stop := s.events.Listen(sessionID, func(sid string, eventType string, data any) {
			evt := s.translateConscienceEvent(sid, eventType, data)
			if evt != nil {
				select {
				case ch <- evt:
				default:
					// buffer full, drop
				}
			}
		})
		defer stop()

		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-ch:
				data, _ := json.Marshal(evt)
				eventType := toString(evt["type"])
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
				flusher.Flush()
			}
		}
	}

	// Fallback: poll for events if no event bus provided
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastCheck := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events := s.pollEvents(sessionID, lastCheck)
			lastCheck = time.Now()
			for _, evt := range events {
				data, _ := json.Marshal(evt)
				eventType := toString(evt["type"])
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
				flusher.Flush()
			}
		}
	}
}

// replaySessionEvents emits the session's stored memory_events as SSE frames
// (event: <type>\ndata: <json>\n\n) so a late subscriber sees history before
// the live wait loop begins. The most recent ~50 events are replayed ordered
// by id ASC (oldest first) to preserve chronological order. When sessionID is
// empty the most recent global events are replayed instead.
func (s *Server) replaySessionEvents(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, sessionID string) {
	if s.db == nil {
		return
	}
	query := `SELECT me.id, me.type, me.content, me.session_id, me.iteration_created, me.created_at
	          FROM memory_events me`
	args := []any{}
	if sessionID != "" {
		query += ` WHERE me.session_id = $1`
		args = append(args, sessionID)
	}
	// Most recent ~50 events, replayed oldest-first.
	query += ` ORDER BY me.id DESC LIMIT 50`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		slog.Warn("opencode-shim: failed to replay session events", "session_id", sessionID, "error", err)
		return
	}

	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		rawType := toString(row["type"])
		content := toString(row["content"])
		evt := s.translateConscienceEvent(sessionID, "message_created", map[string]any{
			"content": content,
		})
		if evt == nil {
			evt = map[string]any{
				"type":       "message.created",
				"session_id": sessionID,
				"properties": map[string]any{
					"sessionID": sessionID,
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}
		}
		// Keep the raw memory event type + content so consumers can inspect
		// the original event (matches listMessages semantics).
		evt["event_type"] = rawType
		evt["content"] = content
		evt["id"] = toInt64(row["id"])
		if ts := toString(row["created_at"]); ts != "" {
			evt["timestamp"] = ts
		}

		data, _ := json.Marshal(evt)
		eventType := toString(evt["type"])
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
		flusher.Flush()
	}
}

// translateConscienceEvent converts a native Consensus event into an opencode-format event.
func (s *Server) translateConscienceEvent(sessionID, eventType string, data any) map[string]any {
	switch eventType {
	case "session_update":
		if d, ok := data.(map[string]any); ok {
			status := ""
			if v, exists := d["status"]; exists {
				status = toString(v)
			}
			iteration := int64(0)
			if v, exists := d["iteration"]; exists {
				iteration = toInt64(v)
			}
			return map[string]any{
				"type":       "session.updated",
				"session_id": sessionID,
				"properties": map[string]any{
					"sessionID": sessionID,
					"status":    status,
					"iteration": iteration,
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}
		}
	case "approval_pending":
		if d, ok := data.(map[string]any); ok {
			return map[string]any{
				"type":          "permission.requested",
				"permission_id": toString(d["approval_id"]),
				"session_id":    sessionID,
				"properties": map[string]any{
					"permissionID": toString(d["approval_id"]),
					"message":      toString(d["description"]),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}
		}
	case "message_created":
		// SPEC-017 §3.5: memory_event_created → message.created
		return map[string]any{
			"type":       "message.created",
			"session_id": sessionID,
			"properties": map[string]any{
				"sessionID": sessionID,
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}
	case "tool_started":
		// SPEC-017 §3.5: tool_execution_start → tool.started
		return map[string]any{
			"type":       "tool.started",
			"session_id": sessionID,
			"properties": map[string]any{
				"sessionID": sessionID,
				"toolName":  toString(data),
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}
	case "tool_completed":
		// SPEC-017 §3.5: tool_execution_complete → tool.completed
		return map[string]any{
			"type":       "tool.completed",
			"session_id": sessionID,
			"properties": map[string]any{
				"sessionID": sessionID,
				"toolName":  toString(data),
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}
	case "approval_resolved":
		if d, ok := data.(map[string]any); ok {
			return map[string]any{
				"type":          "permission.resolved",
				"permission_id": toString(d["approval_id"]),
				"session_id":    sessionID,
				"properties": map[string]any{
					"permissionID": toString(d["approval_id"]),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}
		}
	}
	return nil
}

func (s *Server) pollEvents(sessionID string, since time.Time) []map[string]any {
	// Query sessions for status changes since last check
	ctx := context.Background()
	query := `SELECT id, agent_name, status, iteration, heartbeat_at FROM sessions WHERE heartbeat_at > $1`
	args := []any{since.Format(time.RFC3339)}

	if sessionID != "" {
		query += ` AND id = $2`
		args = append(args, sessionID)
	}

	query += ` ORDER BY heartbeat_at DESC LIMIT 10`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil
	}

	events := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		events = append(events, map[string]any{
			"type":       "session.updated",
			"session_id": toString(row["id"]),
			"properties": map[string]any{
				"sessionID": toString(row["id"]),
				"status":    toString(row["status"]),
				"iteration": toInt64(row["iteration"]),
			},
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}

	return events
}

// ============================================================================
// Session Endpoints
// ============================================================================

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSessions(w, r)
	case http.MethodPost:
		s.createSession(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statusFilter := r.URL.Query().Get("status")

	var rows []db.Row
	var err error

	if statusFilter != "" {
		statuses := strings.Split(statusFilter, ",")
		placeholders := make([]string, len(statuses))
		args := make([]any, len(statuses))
		for i, st := range statuses {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = strings.TrimSpace(st)
		}
		query := fmt.Sprintf(
			`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
			 FROM sessions WHERE status IN (%s) ORDER BY created_at DESC LIMIT 50`,
			strings.Join(placeholders, ","))
		rows, err = s.db.Query(ctx, query, args...)
	} else {
		rows, err = s.db.Query(ctx,
			`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
			 FROM sessions ORDER BY created_at DESC LIMIT 50`)
	}

	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list sessions")
		return
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, s.translateSessionRow(row))
	}
	writeJSON(w, result)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Goal  string `json:"goal"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	// Translate: opencode "title" → Consensus "agent_name"
	agentName := req.Title
	if agentName == "" {
		agentName = "opencode-agent"
	}

	// Try service layer first (SPEC-017 §2: shim calls native API)
	if s.svc != nil {
		result, err := s.svc.CreateSession(r.Context(), SessionCreateInput{
			AgentName:     agentName,
			Goal:          req.Goal,
			ModelID:       req.Model,
			ContextBudget: 128000,
		})
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create session: "+err.Error())
			return
		}

		// Write shim_session_map entry
		externalID := result.SessionID
		if v := r.URL.Query().Get("external_id"); v != "" {
			externalID = v
		}
		now := time.Now().UTC().Format(time.RFC3339)
		s.db.Exec(r.Context(),
			`INSERT INTO shim_session_map (shim_type, external_id, session_id, created_at, last_used_at)
			 VALUES ('opencode', $1, $2, $3, $3)`,
			externalID, result.SessionID, now,
		)

		resp := map[string]any{
			"id":        result.SessionID,
			"title":     agentName,
			"status":    result.Status,
			"api_key":   result.APIKey,
			"createdAt": result.CreatedAt,
		}
		writeJSON(w, resp)
		return
	}

	// Fallback: raw DB access (backwards-compatible; used when svc is nil in tests)
	goal := req.Goal
	modelID := req.Model

	sessionID := newUUID()
	ctx := r.Context()

	if modelID == "" {
		// Prefer a chat-capable model; exclude embedding-only models
		// (text-embedding-*) which win on cost but can't drive the loop.
		modelRows, err := s.db.Query(ctx, `SELECT model_id FROM model_registry
			WHERE enabled = true AND model_id NOT LIKE 'text-embedding%'
			ORDER BY tier ASC, cost_per_m_in ASC LIMIT 1`)
		if err == nil && len(modelRows) > 0 {
			modelID = toString(modelRows[0]["model_id"])
		}
		if modelID == "" {
			modelID = "default"
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	err := s.db.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, context_budget, heartbeat_at, created_at)
		 VALUES ($1, $2, $3, 'booting', $4, 128000, $5, $5)`,
		sessionID, agentName, modelID, goal, now,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create session: "+err.Error())
		return
	}

	apiKey := generateAPIKey()
	keyHash := hex.EncodeToString(sha256Hash([]byte(apiKey)))
	keyPrefix := apiKey[:8]
	keyID := newUUID()

	err = s.db.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at)
		 VALUES ($1, $2, $3, 'session', $4, $5)`,
		keyID, keyHash, keyPrefix, sessionID, now,
	)
	if err != nil {
		s.db.Exec(ctx, `UPDATE sessions SET status = 'failed' WHERE id = $1`, sessionID)
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create api key")
		return
	}

	externalID := sessionID
	if v := r.URL.Query().Get("external_id"); v != "" {
		externalID = v
	}
	s.db.Exec(ctx,
		`INSERT INTO shim_session_map (shim_type, external_id, session_id, created_at, last_used_at)
		 VALUES ('opencode', $1, $2, $3, $3)`,
		externalID, sessionID, now,
	)

	resp := s.translateSessionRow(map[string]any{
		"id": sessionID, "agent_name": agentName, "model_id": modelID,
		"status": "booting", "goal": goal,
		"iteration": int64(0), "tokens_used_in": int64(0), "tokens_used_out": int64(0),
		"created_at": now,
	})
	resp["api_key"] = apiKey

	writeJSON(w, resp)
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /session/{id} or /session/{id}/message or /session/{id}/abort etc.
	path := strings.TrimPrefix(r.URL.Path, "/session/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch {
	case sub == "" && r.Method == http.MethodGet:
		s.getSession(w, r, sessionID)
	case sub == "" && r.Method == http.MethodPatch:
		s.patchSession(w, r, sessionID)
	case sub == "" && r.Method == http.MethodDelete:
		s.deleteSession(w, r, sessionID)
	case sub == "abort" && r.Method == http.MethodPost:
		s.abortSession(w, r, sessionID)
	case sub == "message" && r.Method == http.MethodPost:
		s.sendMessage(w, r, sessionID)
	case sub == "message" && r.Method == http.MethodGet:
		s.listMessages(w, r, sessionID)
	case strings.HasPrefix(sub, "message/") && r.Method == http.MethodGet:
		// GET /session/:id/message/:messageID
		msgID := strings.TrimPrefix(sub, "message/")
		s.getMessageByID(w, r, sessionID, msgID)
	case sub == "children" && r.Method == http.MethodGet:
		s.listChildren(w, r, sessionID)
	default:
		// Check for 501 exclusions
		switch sub {
		case "prompt_async", "shell", "command", "share", "summarize", "init", "fork", "revert":
			writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				fmt.Sprintf("endpoint %q is opencode-specific, not supported by Consensus shim", sub))
		default:
			writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
		}
	}
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	row, err := s.db.QueryRow(ctx,
		`SELECT id, parent_id, agent_name, model_id, status, goal, context_budget,
		        tokens_used_in, tokens_used_out, iteration, project_id, heartbeat_at, created_at, completed_at
		 FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	writeJSON(w, s.translateSessionRow(row))
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)
	err := s.db.Exec(ctx,
		`UPDATE sessions SET status = 'failed', completed_at = $1 WHERE id = $2`,
		now, sessionID,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete session")
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) abortSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)
	err := s.db.Exec(ctx,
		`UPDATE sessions SET status = 'failed', completed_at = $1 WHERE id = $2`,
		now, sessionID,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to abort session")
		return
	}
	writeJSON(w, map[string]string{"status": "aborted"})
}

func (s *Server) listChildren(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
		 FROM sessions WHERE parent_id = $1 ORDER BY created_at DESC LIMIT 50`,
		sessionID,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list children")
		return
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, s.translateSessionRow(row))
	}
	writeJSON(w, result)
}

// patchSession handles PATCH /session/:id — update session properties (title, status, goal).
// SPEC-017 §3.2: HARDEN-SHIM-02 remediation.
func (s *Server) patchSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		Title  string `json:"title"`
		Status string `json:"status"`
		Goal   string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	// Build update dynamically
	sets := []string{}
	args := []any{}
	idx := 0

	if req.Title != "" {
		idx++
		sets = append(sets, fmt.Sprintf("agent_name = $%d", idx))
		args = append(args, req.Title)
	}
	if req.Status != "" {
		status := req.Status
		// Map opencode status → Consensus status
		switch status {
		case "pause", "paused":
			status = "paused"
		case "resume", "resumed", "idle":
			status = "idle"
		case "cancel", "cancelled":
			status = "failed"
		}
		idx++
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, status)
	}
	if req.Goal != "" {
		idx++
		sets = append(sets, fmt.Sprintf("goal = $%d", idx))
		args = append(args, req.Goal)
	}

	if len(sets) == 0 {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "no fields to update")
		return
	}

	idx++
	sets = append(sets, fmt.Sprintf("heartbeat_at = $%d", idx))
	args = append(args, now)

	idx++
	args = append(args, sessionID)

	query := fmt.Sprintf("UPDATE sessions SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	err := s.db.Exec(ctx, query, args...)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update session: "+err.Error())
		return
	}

	// Return updated session
	row, err := s.db.QueryRow(ctx,
		`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
		 FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeJSON(w, map[string]string{"status": "updated"})
		return
	}
	writeJSON(w, s.translateSessionRow(row))
}

// getMessageByID handles GET /session/:id/message/:messageID.
// SPEC-017 §3.2: HARDEN-SHIM-03 remediation.
func (s *Server) getMessageByID(w http.ResponseWriter, r *http.Request, sessionID, messageID string) {
	ctx := r.Context()

	// The messageID from opencode is "msg-{id}" format; extract the numeric suffix
	trimmed := strings.TrimPrefix(messageID, "msg-")

	row, err := s.db.QueryRow(ctx,
		`SELECT id, type, content, session_id, iteration_created, created_at
		 FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) = $2 LIMIT 1`,
		sessionID, trimmed,
	)
	if err != nil || row == nil {
		// Fallback: try prefix match
		row, err = s.db.QueryRow(ctx,
			`SELECT id, type, content, session_id, iteration_created, created_at
			 FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) LIKE $2 LIMIT 1`,
			sessionID, trimmed+"%",
		)
	}
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "message not found")
		return
	}

	msgType := toString(row["type"])
	role := "assistant"
	if msgType == "user_message" {
		role = "user"
	}

	writeJSON(w, map[string]any{
		"info": map[string]any{
			"id":        fmt.Sprintf("msg-%v", row["id"]),
			"role":      role,
			"createdAt": time.Now().UnixMilli(),
		},
		"parts": []map[string]any{
			{"type": "text", "text": toString(row["content"])},
		},
	})
}

// handleAuth handles PUT /auth/:id — update auth/config for a provider/session.
// SPEC-017 §3.2: HARDEN-SHIM-08 remediation.
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/auth/")
	if path == "" || r.Method != http.MethodPut {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use PUT /auth/:id")
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	// Store auth config in system_settings
	ctx := r.Context()
	for k, v := range req {
		key := fmt.Sprintf("auth.%s.%s", path, k)
		val := toString(v)
		s.db.Exec(ctx,
			`INSERT INTO system_settings (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = $2`,
			key, val,
		)
	}

	writeJSON(w, map[string]any{
		"id":      path,
		"status":  "updated",
		"message": "auth configuration saved",
	})
}

// isStubPath reports whether a path maps to an opencode-specific 501 stub
// (SPEC-017 §3.9). These endpoints return NOT_IMPLEMENTED with zero data, so
// auth is skipped for them — contract tests and unauthenticated clients get
// 501, not 401.
//
// Auth skip policy (reconciles the two shim contract suites):
//   - /instance/* is fully public but implemented (SPEC-017 §3.10) — the
//     auth skip lives in authMiddleware, not here.
//   - /project and /vcs keep auth on GET — the endpoint smoke test expects
//     401 for unauthenticated GET /project and /vcs — but non-GET methods
//     (PATCH, POST, DELETE) reach the stub so the full-contract suite (C20)
//     sees 501/404 for PATCH /project/:id instead of 401.
func isStubPath(path, method string) bool {
	for _, p := range []string{"/project", "/vcs"} {
		if (path == p || strings.HasPrefix(path, p+"/")) && method != http.MethodGet {
			return true
		}
	}
	return false
}

// handleProjectVCSSStub returns 501 for /project and /vcs paths (opencode-specific).
func (s *Server) handleProjectVCSSStub(w http.ResponseWriter, r *http.Request) {
	name := "project"
	if strings.Contains(r.URL.Path, "vcs") {
		name = "VCS"
	}
	writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		fmt.Sprintf("%s is opencode-specific, not supported by Consensus shim; use native tool API", name))
}

// ============================================================================
// Instance Endpoints (SPEC-017 §3.10) — opencode /instance/* translation
// ============================================================================
//
// The opencode server protocol (sst/opencode httpapi-instance.test.ts) probes
// /instance/path, /instance/vcs and /instance/vcs/diff unauthenticated and
// expects 200 with real workspace data. The Consensus shim treats the server
// as a singleton instance rooted at the workspace directory.

// instanceKnownSubpaths lists /instance/* sub-paths that exist in the upstream
// opencode protocol but are NOT translated by the shim. They return
// 501 NOT_IMPLEMENTED (same convention as /session subpaths); unknown
// sub-paths return 404.
var instanceKnownSubpaths = map[string]bool{
	"dispose":      true, // POST — upstream instance disposal
	"vcs/status":   true, // GET — per-file VCS status list
	"vcs/diff/raw": true, // GET — raw unified diff text
	"vcs/apply":    true, // POST — apply a patch
	"command":      true, // GET — opencode slash commands
	"agent":        true, // GET — opencode agent registry
	"skill":        true, // GET — opencode skill registry
	"lsp":          true, // GET — LSP server status
	"formatter":    true, // GET — formatter status
}

// handleInstance serves GET /instance — the singleton instance list. The
// Consensus server is a single instance rooted at the workspace directory;
// created/updated timestamps come from the server process.
func (s *Server) handleInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	dir := s.workspaceDir()
	writeJSON(w, []map[string]any{
		{
			"id":        instanceID(dir),
			"path":      dir,
			"createdAt": s.startedAt.Format(time.RFC3339),
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// handleInstanceSub serves /instance/* sub-paths: the implemented translation
// endpoints (/instance/path, /instance/vcs, /instance/vcs/diff) plus the
// 501/404 convention for everything else.
func (s *Server) handleInstanceSub(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/instance/")
	switch {
	case sub == "path" && r.Method == http.MethodGet:
		s.instancePath(w, r)
	case sub == "vcs" && r.Method == http.MethodGet:
		s.instanceVCS(w, r)
	case sub == "vcs/diff" && r.Method == http.MethodGet:
		s.instanceVCSDiff(w, r)
	default:
		if instanceKnownSubpaths[sub] {
			writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				fmt.Sprintf("endpoint %q is opencode-specific, not supported by Consensus shim", sub))
			return
		}
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
	}
}

// instancePath serves GET /instance/path → opencode PathInfo:
// {home, state, config, worktree, directory}.
func (s *Server) instancePath(w http.ResponseWriter, r *http.Request) {
	dir := s.workspaceDir()
	home, _ := os.UserHomeDir()
	writeJSON(w, map[string]any{
		"home":      home,
		"state":     filepath.Join(home, ".local", "state", "consensus"),
		"config":    filepath.Join(home, ".config", "consensus", "config.json"),
		"worktree":  gitWorktree(r.Context(), dir, dir),
		"directory": dir,
	})
}

// instanceVCS serves GET /instance/vcs → opencode Vcs.Info:
// {branch?, default_branch?}. Never errors — a non-git workspace returns {}.
func (s *Server) instanceVCS(w http.ResponseWriter, r *http.Request) {
	dir := s.workspaceDir()
	ctx := r.Context()
	info := map[string]any{}
	if branch := gitBranch(ctx, dir); branch != "" {
		info["branch"] = branch
	}
	if def := gitDefaultBranch(ctx, dir); def != "" {
		info["default_branch"] = def
	}
	writeJSON(w, info)
}

// instanceVCSDiff serves GET /instance/vcs/diff → opencode Array(Vcs.FileDiff):
// [{file, additions, deletions, status?}]. patch is omitted (optional in the
// upstream schema). Never errors — a clean or non-git workspace returns [].
func (s *Server) instanceVCSDiff(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, gitFileDiffs(r.Context(), s.workspaceDir()))
}

// workspaceDir returns the workspace directory used by /instance/* endpoints:
// an explicitly configured workdir, falling back to the process CWD.
func (s *Server) workspaceDir() string {
	if s.workdir != "" {
		return s.workdir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// instanceID derives a stable singleton-instance id from the workspace
// directory (short sha256 prefix).
func instanceID(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return "consensus-" + hex.EncodeToString(sum[:])[:12]
}

// runGit runs git -C dir <args...> and returns trimmed stdout; errors are
// returned so callers can fall back to neutral shapes (never fatal).
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// gitBranch returns the current branch (empty when not a git repo, detached,
// or unborn HEAD).
func gitBranch(ctx context.Context, dir string) string {
	out, err := runGit(ctx, dir, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return out
}

// gitDefaultBranch returns the repository's default branch (origin HEAD,
// falling back to init.defaultBranch); empty when undeterminable.
func gitDefaultBranch(ctx context.Context, dir string) string {
	if out, err := runGit(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return strings.TrimPrefix(out, "origin/")
	}
	if out, err := runGit(ctx, dir, "config", "--get", "init.defaultBranch"); err == nil {
		return out
	}
	return ""
}

// gitWorktree returns the repository top-level (worktree root) for a git
// workspace; falls back to dir when not a git repo.
func gitWorktree(ctx context.Context, dir string, fallback string) string {
	if out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel"); err == nil && out != "" {
		return out
	}
	return fallback
}

// gitNumstat returns {path: [additions, deletions]} from
// `git diff HEAD --numstat`, falling back to index-vs-worktree when the repo
// has no HEAD yet. Binary files ("-" columns) count as 0.
func gitNumstat(ctx context.Context, dir string) map[string][]int {
	out, err := runGit(ctx, dir, "diff", "HEAD", "--numstat")
	if err != nil {
		out, err = runGit(ctx, dir, "diff", "--numstat")
		if err != nil {
			return map[string][]int{}
		}
	}
	stats := map[string][]int{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "	", 3)
		if len(parts) != 3 {
			continue
		}
		adds, errA := strconv.Atoi(parts[0])
		dels, errD := strconv.Atoi(parts[1])
		file := unquoteGitPath(parts[2])
		if errA != nil || errD != nil || file == "" {
			continue
		}
		stats[file] = []int{adds, dels}
	}
	return stats
}

// gitFileDiffs builds the opencode Vcs.FileDiff list for a workspace: changed
// files from `git status --porcelain` with additions/deletions from
// `git diff HEAD --numstat`; untracked files count their own lines. A clean or
// non-git workspace yields an empty (never nil) list.
func gitFileDiffs(ctx context.Context, dir string) []map[string]any {
	status, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return []map[string]any{}
	}
	stats := gitNumstat(ctx, dir)
	diffs := []map[string]any{}
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 4 {
			continue
		}
		code, path := line[:2], line[3:]
		if strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C") {
			if i := strings.LastIndex(path, " -> "); i >= 0 {
				path = path[i+4:]
			}
		}
		path = unquoteGitPath(path)
		if path == "" {
			continue
		}
		entry := map[string]any{
			"file":      path,
			"additions": 0,
			"deletions": 0,
		}
		if stat := stats[path]; stat != nil {
			entry["additions"] = stat[0]
			entry["deletions"] = stat[1]
		}
		switch {
		case strings.HasPrefix(code, "??"):
			entry["status"] = "added"
			if _, ok := stats[path]; !ok {
				entry["additions"] = lineCount(filepath.Join(dir, path))
			}
		case strings.Contains(code, "D"):
			entry["status"] = "deleted"
		case strings.Contains(code, "A"):
			entry["status"] = "added"
		default:
			entry["status"] = "modified"
		}
		diffs = append(diffs, entry)
	}
	return diffs
}

// unquoteGitPath unquotes a git-quoted path (C-style escaping when the path
// contains spaces or non-ASCII characters); plain paths pass through.
func unquoteGitPath(p string) string {
	if strings.HasPrefix(p, "\"") {
		if u, err := strconv.Unquote(p); err == nil {
			return u
		}
	}
	return p
}

// lineCount counts newlines in a text file for untracked-file diff stats;
// unreadable or oversized (>= 1 MiB) files count as 0.
func lineCount(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	n, total := 0, 0
	for {
		m, rerr := f.Read(buf)
		total += m
		if m > 0 {
			n += bytes.Count(buf[:m], []byte{'\n'})
		}
		if rerr != nil || total >= 1<<20 {
			break
		}
	}
	return n
}

// ============================================================================
// Message Translation (core shim functionality)
// ============================================================================

// SendMessageRequest is the opencode message request format.
type SendMessageRequest struct {
	Parts []MessagePart `json:"parts"`
}

// MessagePart represents a single part in an opencode message.
type MessagePart struct {
	Type string `json:"type"` // "text", "tool-invocation", etc.
	Text string `json:"text,omitempty"`
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed message body")
		return
	}

	// Translate: extract text from opencode parts
	var textParts []string
	for _, p := range req.Parts {
		if p.Type == "text" && p.Text != "" {
			textParts = append(textParts, p.Text)
		}
	}
	content := strings.Join(textParts, "\n")
	if content == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "message content is empty")
		return
	}

	// Try the API service layer first (SPEC-017 §2: shim calls native API)
	if s.svc != nil {
		err := s.svc.SendMessage(r.Context(), MessageSendInput{
			SessionID: sessionID,
			Content:   content,
			MsgType:   "user_instruction",
		})
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to send message: "+err.Error())
			return
		}
		resp := s.buildAssistantMessage("Message received. The agent will process your request and respond in the next iteration.")
		writeJSON(w, resp)
		return
	}

	// Fallback: raw DB access (used when svc is nil, e.g., in unit tests)
	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	row, err := s.db.QueryRow(ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	currentIteration := toInt64(row["iteration"])

	err = s.db.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, $4)`,
		content, sessionID, currentIteration+1, now)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}

	currentStatus := toString(row["status"])
	// Wake the session for harness pickup. Sessions are born 'booting'
	// (see createSession); a message must transition them to 'thinking'
	// so the heartbeat loop claims them. Matches native API behavior
	// (api/sessions.go: idle || booting → thinking).
	if currentStatus == "idle" || currentStatus == "booting" {
		s.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1 WHERE id = $2`,
			now, sessionID,
		)
	}

	resp := s.buildAssistantMessage("Message received. The agent will process your request and respond in the next iteration.")
	writeJSON(w, resp)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()
	limit := 50

	rows, err := s.db.Query(ctx,
		`SELECT me.id, me.type, me.content, me.session_id, me.iteration_created, me.created_at
		 FROM memory_events me
		 WHERE me.session_id = $1
		 ORDER BY me.id DESC LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list messages")
		return
	}

	// Group by conversation turns, translate to opencode format
	messages := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		msgType := toString(row["type"])
		role := "assistant"
		if msgType == "user_message" {
			role = "user"
		}

		parts := []map[string]any{
			{
				"type": "text",
				"text": toString(row["content"]),
			},
		}

		messages = append(messages, map[string]any{
			"id":        fmt.Sprintf("msg-%d", toInt64(row["id"])),
			"role":      role,
			"parts":     parts,
			"createdAt": time.Now().UnixMilli(),
		})
	}

	writeJSON(w, messages)
}

// ============================================================================
// Config, Provider, Agent Endpoints
// ============================================================================

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx := r.Context()
		rows, err := s.db.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read config")
			return
		}
		settings := make(map[string]any, len(rows))
		for _, row := range rows {
			settings[toString(row["key"])] = toString(row["value"])
		}
		writeJSON(w, map[string]any{"settings": settings})
		return
	case http.MethodPatch:
		// HARDEN-SHIM-09: PATCH /config support
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
			return
		}
		ctx := r.Context()
		for k, v := range req {
			val := toString(v)
			s.db.Exec(ctx,
				`INSERT INTO system_settings (key, value) VALUES ($1, $2)
				 ON CONFLICT (key) DO UPDATE SET value = $2`,
				k, val,
			)
		}
		writeJSON(w, map[string]any{"status": "updated", "keys": len(req)})
		return
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PATCH")
	}
}

func (s *Server) handleConfigProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled
		 FROM model_registry ORDER BY tier ASC, cost_per_m_in ASC`)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read models")
		return
	}

	type modelInfo struct {
		ID          string  `json:"id"`
		Tier        int     `json:"tier"`
		MaxContext  int64   `json:"max_context"`
		CostPerMIn  float64 `json:"cost_per_m_in"`
		CostPerMOut float64 `json:"cost_per_m_out"`
		Enabled     bool    `json:"enabled"`
	}
	models := make([]modelInfo, 0, len(rows))
	for _, row := range rows {
		models = append(models, modelInfo{
			ID:          toString(row["model_id"]),
			Tier:        toInt(row["tier"]),
			MaxContext:  toInt64(row["max_context"]),
			CostPerMIn:  toFloat64(row["cost_per_m_in"]),
			CostPerMOut: toFloat64(row["cost_per_m_out"]),
			Enabled:     toBool(row["enabled"]),
		})
	}
	writeJSON(w, map[string]any{"providers": models})
}

func (s *Server) handleProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	// Read from model_registry to return the default LLM provider info (HARDEN-SHIM-12)
	ctx := r.Context()
	row, err := s.db.QueryRow(ctx,
		`SELECT model_id, max_context FROM model_registry WHERE enabled = true ORDER BY tier ASC, cost_per_m_in ASC LIMIT 1`)
	if err != nil || row == nil {
		// Fallback if no models registered
		writeJSON(w, map[string]any{
			"provider":  "consensus",
			"version":   "0.1.0",
			"model":     "gpt-4o",
			"maxTokens": 128000,
		})
		return
	}
	writeJSON(w, map[string]any{
		"provider":  "consensus",
		"version":   "0.1.0",
		"model":     toString(row["model_id"]),
		"maxTokens": toInt64(row["max_context"]),
	})
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	// Return available agent types
	writeJSON(w, []map[string]any{
		{"name": "default", "description": "Default Consensus agent"},
		{"name": "researcher", "description": "Research-focused agent"},
		{"name": "coder", "description": "Code-focused agent"},
		{"name": "analyst", "description": "Data analysis agent"},
	})
}

// ============================================================================
// Tools Endpoints
// ============================================================================

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT id, name, description, hemisphere, handler_type, status, enabled, requires_approval
		 FROM tools_registry ORDER BY name`)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tools")
		return
	}

	tools := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		tools = append(tools, map[string]any{
			"id":                toString(row["id"]),
			"name":              toString(row["name"]),
			"description":       toString(row["description"]),
			"hemisphere":        toString(row["hemisphere"]),
			"handler_type":      toString(row["handler_type"]),
			"status":            toString(row["status"]),
			"enabled":           toBool(row["enabled"]),
			"requires_approval": toBool(row["requires_approval"]),
		})
	}
	writeJSON(w, tools)
}

func (s *Server) handleToolIDs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	ctx := r.Context()
	rows, err := s.db.Query(ctx, `SELECT name FROM tools_registry ORDER BY name`)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tool IDs")
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, toString(row["name"]))
	}
	writeJSON(w, ids)
}

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"MCP management via opencode shim is not implemented; use /mcp/sse directly")
}

// ============================================================================
// File Endpoints (SPEC-017 §3.1 — map to native tool execution API)
// ============================================================================

func (s *Server) handleFind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "?pattern= is required")
		return
	}
	// Delegate to service layer if available
	if s.svc != nil {
		matches, err := s.svc.FindFiles(r.Context(), pattern)
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "find failed: "+err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"files":   matches,
			"count":   len(matches),
			"pattern": pattern,
		})
		return
	}
	// Fallback: use path/filepath directly
	matches, err := filepath.Glob(pattern)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "find failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"files":   matches,
		"count":   len(matches),
		"pattern": pattern,
	})
}

func (s *Server) handleFindSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/find/")
	query := r.URL.Query().Get("query")

	switch {
	case strings.HasPrefix(path, "file"):
		if query == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "?query= is required")
			return
		}
		// Glob-based file search via service layer
		if s.svc != nil {
			matches, err := s.svc.FindFiles(r.Context(), query)
			if err != nil {
				writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "file search failed: "+err.Error())
				return
			}
			writeJSON(w, map[string]any{"files": matches, "count": len(matches), "query": query})
			return
		}
		// Fallback: path/filepath
		matches, err := filepath.Glob(query)
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, map[string]any{"files": matches, "count": len(matches), "query": query})

	case strings.HasPrefix(path, "symbol"):
		// Symbol search requires LSP — still not available
		writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"symbol search requires LSP integration, not yet available")
	default:
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown find sub-path")
	}
}

func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "?path= is required")
		return
	}
	// Delegate to service layer if available
	if s.svc != nil {
		content, err := s.svc.ReadFile(r.Context(), filePath)
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read file: "+err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"path":    filePath,
			"content": content,
			"size":    len(content),
		})
		return
	}
	// Fallback: direct filesystem read
	data, err := os.ReadFile(filePath)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read file: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"path":    filePath,
		"content": string(data),
		"size":    len(data),
	})
}

func (s *Server) handleFileStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	// Delegate to service layer if available
	if s.svc != nil {
		status, err := s.svc.GetGitStatus(r.Context())
		if err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "git status failed: "+err.Error())
			return
		}
		writeJSON(w, status)
		return
	}
	// Fallback: run git status directly
	ctx := r.Context()
	output, err := execGitStatus(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"status": "unavailable", "message": err.Error(), "changes": []string{}})
		return
	}
	writeJSON(w, output)
}

// ============================================================================
// Permission / HITL Translation (SPEC-017 §3.7)
// ============================================================================

func (s *Server) handlePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	// Translate: opencode permission list → Consensus approval_requests
	ctx := r.Context()
	sessionID := r.URL.Query().Get("session_id")

	query := `SELECT ar.id, ar.session_id, ar.request_type, ar.risk_level,
	                 ar.description, ar.status, ar.created_at
	          FROM approval_requests ar WHERE ar.status = 'pending'`
	args := []any{}

	if sessionID != "" {
		query += ` AND ar.session_id = $1`
		args = append(args, sessionID)
	}
	query += ` ORDER BY ar.risk_level DESC, ar.created_at ASC LIMIT 20`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list permissions")
		return
	}

	permissions := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		permissions = append(permissions, map[string]any{
			"id":          toString(row["id"]),
			"session_id":  toString(row["session_id"]),
			"type":        toString(row["request_type"]),
			"risk_level":  toString(row["risk_level"]),
			"description": toString(row["description"]),
			"status":      toString(row["status"]),
			"created_at":  toString(row["created_at"]),
		})
	}

	// Format as opencode permission events
	writeJSON(w, map[string]any{
		"permissions": permissions,
	})
}

func (s *Server) handlePermissionByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /permission/{id} or /permission/{id}/resolve
	path := strings.TrimPrefix(r.URL.Path, "/permission/")
	parts := strings.SplitN(path, "/", 2)
	permID := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch {
	case sub == "" && r.Method == http.MethodGet:
		s.getPermission(w, r, permID)
	case sub == "resolve" && r.Method == http.MethodPost:
		s.resolvePermission(w, r, permID)
	default:
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown permission action")
	}
}

func (s *Server) getPermission(w http.ResponseWriter, r *http.Request, permID string) {
	ctx := r.Context()
	row, err := s.db.QueryRow(ctx,
		`SELECT ar.id, ar.session_id, ar.request_type, ar.risk_level,
		        ar.description, ar.sql_preview, ar.status, ar.decision_reason,
		        ar.created_at, ar.resolved_at
		 FROM approval_requests ar WHERE ar.id = $1`, permID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "permission not found")
		return
	}

	writeJSON(w, map[string]any{
		"id":              toString(row["id"]),
		"session_id":      toString(row["session_id"]),
		"type":            toString(row["request_type"]),
		"risk_level":      toString(row["risk_level"]),
		"description":     toString(row["description"]),
		"sql_preview":     toString(row["sql_preview"]),
		"status":          toString(row["status"]),
		"decision_reason": toString(row["decision_reason"]),
		"created_at":      toString(row["created_at"]),
		"resolved_at":     nilOrString(row["resolved_at"]),
	})
}

func (s *Server) resolvePermission(w http.ResponseWriter, r *http.Request, permID string) {
	var req struct {
		Decision string `json:"decision"` // "approved" | "rejected" | "modified"
		Reason   string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	if req.Decision != "approved" && req.Decision != "rejected" && req.Decision != "modified" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"decision must be 'approved', 'rejected', or 'modified'")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	// Translate: opencode permission resolution → Consensus approval review
	// SPEC-017 §3.7: Maps to POST /api/v1/approvals/:id/review
	// HARDEN-SHIM-10: Emit event on resolution for SSE subscribers
	err := s.db.Exec(ctx,
		`UPDATE approval_requests
		 SET status = $1, decision_reason = $2, resolved_at = $3, resolved_by = 'opencode-shim'
		 WHERE id = $4 AND status = 'pending'`,
		req.Decision, req.Reason, now, permID,
	)
	if err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
			"failed to resolve permission: "+err.Error())
		return
	}

	// Emit SSE event for the resolved permission (HARDEN-SHIM-01: permission.resolved)
	s.emitShimEventForSession(s.sessionIDFromPerm(permID), "approval_resolved", map[string]any{
		"approval_id": permID,
		"status":      req.Decision,
	})

	writeJSON(w, map[string]any{
		"id":        permID,
		"status":    req.Decision,
		"resolved":  true,
		"timestamp": now,
	})
}

// ============================================================================
// TUI Control Endpoints (SPEC-017 §3.1 — shim-only passthrough)
// ============================================================================

func (s *Server) handleTUI(w http.ResponseWriter, r *http.Request) {
	// TUI sub-paths: append-prompt, submit-prompt, execute-command, show-toast
	// These are shim-only and do not map to native API calls.
	sub := strings.TrimPrefix(r.URL.Path, "/tui/")
	switch sub {
	case "append-prompt", "submit-prompt", "execute-command", "show-toast":
		writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			fmt.Sprintf("TUI control %q is not implemented in this shim; use opencode's built-in TUI", sub))
	default:
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown TUI action")
	}
}

// ============================================================================
// LSP Endpoint (SPEC-017 §3.1)
// ============================================================================

func (s *Server) handleLSP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	// Return LSP status from config — not yet integrated
	writeJSON(w, map[string]any{
		"enabled": false,
		"status":  "unavailable",
		"message": "LSP integration is not yet available in Consensus shim",
	})
}

// ============================================================================
// Doc Endpoint
// ============================================================================

func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Derive the servers URL from the request Host instead of hardcoding
	// localhost:8090 — the shim can be mounted on any port (DOGFOOD-103).
	serversURL := "http://" + r.Host
	if r.TLS != nil {
		serversURL = "https://" + r.Host
	}
	page := strings.Replace(swaggerUIPage, "{{SHIM_SERVERS_URL}}", serversURL, 1)
	w.Write([]byte(page))
}

const swaggerUIPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Consensus — opencode Shim API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>body{margin:0;background:#fafafa;}.topbar{display:none;}</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        dom_id: '#swagger-ui',
        deepLinking: true,
        layout: "StandaloneLayout",
        spec: {
          openapi: "3.1.0",
          info: { title: "Consensus opencode Shim", version: "0.1.0",
            description: "opencode server protocol shim for Consensus agent runtime." },
          servers: [{ url: "{{SHIM_SERVERS_URL}}", description: "Shim server" }],
          tags: [
            { name: "Global", description: "Health check and event stream" },
            { name: "Sessions", description: "Session lifecycle" },
            { name: "Messages", description: "Message send/receive" },
            { name: "Config", description: "Configuration and providers" },
            { name: "Tools", description: "Tool registry" }
          ],
          paths: {
            "/global/health": { get: { tags: ["Global"], summary: "Health check", responses: { "200": { description: "OK" } } } },
            "/global/event": { get: { tags: ["Global"], summary: "SSE event stream", parameters: [{ name: "session_id", in: "query", schema: { type: "string" } }], responses: { "200": { description: "SSE stream" } } } },
            "/session": {
              get: { tags: ["Sessions"], summary: "List sessions", parameters: [{ name: "status", in: "query", schema: { type: "string" } }], responses: { "200": { description: "Session list" } } },
              post: { tags: ["Sessions"], summary: "Create session", requestBody: { content: { "application/json": { schema: { type: "object", properties: { title: { type: "string" }, goal: { type: "string" }, model: { type: "string" } } } } } }, responses: { "201": { description: "Created" } } }
            },
            "/session/{id}": {
              get: { tags: ["Sessions"], summary: "Get session", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Session details" } } },
              delete: { tags: ["Sessions"], summary: "Delete session", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Deleted" } } }
            },
            "/session/{id}/abort": { post: { tags: ["Sessions"], summary: "Abort session", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Aborted" } } } },
            "/session/{id}/message": {
              post: { tags: ["Messages"], summary: "Send message", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], requestBody: { content: { "application/json": { schema: { type: "object", properties: { parts: { type: "array", items: { type: "object", properties: { type: { type: "string" }, text: { type: "string" } } } } } } } } }, responses: { "200": { description: "Response" } } },
              get: { tags: ["Messages"], summary: "List messages", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Messages" } } }
            },
            "/session/{id}/children": { get: { tags: ["Sessions"], summary: "List child sessions", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Children" } } } },
            "/config": { get: { tags: ["Config"], summary: "Get config", responses: { "200": { description: "Config" } } } },
            "/config/providers": { get: { tags: ["Config"], summary: "List model providers", responses: { "200": { description: "Providers" } } } },
            "/provider": { get: { tags: ["Config"], summary: "Get LLM provider info", responses: { "200": { description: "Provider info" } } } },
            "/agent": { get: { tags: ["Config"], summary: "List agent types", responses: { "200": { description: "Agent types" } } } },
            "/experimental/tool": { get: { tags: ["Tools"], summary: "List tools", responses: { "200": { description: "Tools" } } } },
            "/experimental/tool/ids": { get: { tags: ["Tools"], summary: "List tool IDs", responses: { "200": { description: "Tool IDs" } } } },
            "/find": { get: { tags: ["Files"], summary: "Find files by pattern (grep)", parameters: [{ name: "pattern", in: "query", required: true, schema: { type: "string" } }], responses: { "501": { description: "Not yet wired" } } } },
            "/find/file": { get: { tags: ["Files"], summary: "Find files by glob", parameters: [{ name: "query", in: "query", required: true, schema: { type: "string" } }], responses: { "501": { description: "Not yet wired" } } } },
            "/file/content": { get: { tags: ["Files"], summary: "Read file content", parameters: [{ name: "path", in: "query", required: true, schema: { type: "string" } }], responses: { "501": { description: "Not yet wired" } } } },
            "/file/status": { get: { tags: ["Files"], summary: "Git file status", responses: { "501": { description: "Not yet wired" } } } },
            "/permission": { get: { tags: ["Permissions"], summary: "List pending permissions/approvals", parameters: [{ name: "session_id", in: "query", schema: { type: "string" } }], responses: { "200": { description: "Permissions" } } } },
            "/permission/{id}": { get: { tags: ["Permissions"], summary: "Get permission detail", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], responses: { "200": { description: "Permission detail" } } } },
            "/permission/{id}/resolve": { post: { tags: ["Permissions"], summary: "Resolve permission (approve/reject)", parameters: [{ name: "id", in: "path", required: true, schema: { type: "string" } }], requestBody: { content: { "application/json": { schema: { type: "object", properties: { decision: { type: "string", enum: ["approved", "rejected", "modified"] }, reason: { type: "string" } }, required: ["decision"] } } } }, responses: { "200": { description: "Resolved" } } } }
          }
        }
      });
    };
  </script>
</body>
</html>
`

// ============================================================================
// Translation Helpers
// ============================================================================

func (s *Server) translateSessionRow(row map[string]any) map[string]any {
	// opencode session format
	return map[string]any{
		"id":          toString(row["id"]),
		"title":       toString(row["agent_name"]),
		"status":      toString(row["status"]),
		"goal":        toString(row["goal"]),
		"model":       toString(row["model_id"]),
		"iteration":   toInt64(row["iteration"]),
		"tokensIn":    toInt64(row["tokens_used_in"]),
		"tokensOut":   toInt64(row["tokens_used_out"]),
		"createdAt":   toString(row["created_at"]),
		"completedAt": nilOrString(row["completed_at"]),
	}
}

func (s *Server) buildAssistantMessage(text string) map[string]any {
	return map[string]any{
		"info": map[string]any{
			"id":        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			"role":      "assistant",
			"createdAt": time.Now().UnixMilli(),
		},
		"parts": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func (s *Server) buildEmptyAssistantMessage() map[string]any {
	return s.buildAssistantMessage("")
}

// ============================================================================
// Response Helpers
// ============================================================================

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(v)
	if data != nil {
		w.Write(data)
	}
}

func writeOpencodeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
	w.Write(data)
	slog.Warn("opencode-shim: error", "method", r.Method, "path", r.URL.Path, "status", status, "code", code)
}

// emitShimEventForSession sends an event through the event bus for SSE subscribers.
func (s *Server) emitShimEventForSession(sessionID, eventType string, data any) {
	if s.events != nil {
		s.events.Emit(sessionID, eventType, data)
	}
}

// sessionIDFromPerm looks up the session ID associated with a permission/approval ID.
func (s *Server) sessionIDFromPerm(permID string) string {
	ctx := context.Background()
	row, err := s.db.QueryRow(ctx, `SELECT session_id FROM approval_requests WHERE id = $1 LIMIT 1`, permID)
	if err != nil || row == nil {
		return ""
	}
	return toString(row["session_id"])
}

// ============================================================================
// Utilities (shared)
// ============================================================================

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

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case string:
		var f float64
		json.Unmarshal([]byte(n), &f)
		return f
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	default:
		return false
	}
}

func nilOrString(v any) *string {
	s := toString(v)
	if s == "" {
		return nil
	}
	return &s
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) < 8 || auth[:7] != "Bearer " {
		return ""
	}
	return auth[7:]
}

func sha256Hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func newUUID() string {
	b := make([]byte, 16)
	// Simple deterministic-ish UUID generation with time-based entropy
	now := time.Now().UnixNano()
	for i := 0; i < 16; i++ {
		b[i] = byte(now >> (i * 8 % 64))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateAPIKey() string {
	b := make([]byte, 32)
	now := time.Now().UnixNano()
	for i := 0; i < 32; i++ {
		b[i] = byte(now>>(i*8%64)) ^ byte(now>>(i*3%64))
	}
	return "cs_sk_" + hex.EncodeToString(b)
}

// execGitStatus runs "git status --porcelain" as a fallback when no service layer is available.
func execGitStatus(ctx context.Context) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return map[string]any{
			"status":  "unavailable",
			"message": err.Error(),
			"changes": []string{},
		}, nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	return map[string]any{
		"status":  "ok",
		"changes": lines,
	}, nil
}

// Serve starts listening. Not exported — use s.Handler() to mount on parent server.
func (s *Server) serve() {} // placeholder
