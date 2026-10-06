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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/specs"
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

	// Maximum time a synchronous message POST waits for the corresponding
	// agent response. Request cancellation can shorten this bound.
	messageResponseTimeout time.Duration

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

	// mcpHandler is the real MCP HTTP handler (internal/mcp), injected via
	// SetMCPHandler by cmd/consensus/main.go. When set, the shim's bare /mcp
	// mount delegates MCP client traffic (JSON-RPC POSTs, SSE GETs) to it
	// instead of answering the 501 stub (MCP-DIRECT-001). Optional by
	// design: shim-only harnesses that never set it keep the stub.
	mcpHandler http.Handler
}

// Service is the minimal interface the shim needs from the API service layer.
// This avoids a circular import (shim → api → shim).
type Service interface {
	CreateSession(ctx context.Context, input SessionCreateInput) (*SessionCreateResult, error)
	GetSession(ctx context.Context, id string) (*SessionResult, error)
	UpdateSession(ctx context.Context, id string, action string) error
	DeleteSession(ctx context.Context, id string) error
	SendMessage(ctx context.Context, input MessageSendInput) (*MessageSendResult, error)
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

// MessageSendResult is the assistant response produced for a submitted turn.
type MessageSendResult struct {
	Content string
}

const defaultMessageResponseTimeout = 90 * time.Second

// NewServer creates the opencode shim with all routes registered.
// svc is the API service layer — the shim calls this instead of raw DB.
// If eventBus is nil, the shim falls back to polling-based event streaming.
func NewServer(dbase db.DB, adminKey string, eventBus EventBus, svc Service) *Server {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	s := &Server{
		db:                     dbase,
		svc:                    svc,
		events:                 eventBus,
		messageResponseTimeout: defaultMessageResponseTimeout,
		adminKey:               adminKey,
		workdir:                wd,
		startedAt:              time.Now().UTC(),
	}
	mux := http.NewServeMux()
	s.mux = mux

	// Global
	mux.HandleFunc("/global/health", s.handleGlobalHealth)
	mux.HandleFunc("/global/event", s.handleGlobalEvent)
	// ROUTE-ADD-090 / SHIM-DRIFT-088: the exact /global/config path was never
	// registered, so PATCH /global/config fell through the mux to net/http's
	// default 404 — the drift artifact's NOT-SERVED class, indistinguishable
	// from "no such opencode operation". Registering the exact path routes
	// every method, and the pinned document declares this path for GET as well
	// (global.config.get, SHIM-DRIFT-087), so handleGlobalConfig serves BOTH
	// declared operations rather than lying with a 405 on the one it does not
	// route (see the handler). Exact pattern, never a /global/* catch-all.
	mux.HandleFunc("/global/config", s.handleGlobalConfig)
	// ROUTE-ADD-091 / SHIM-DRIFT-089: the exact /global/dispose path was never
	// registered, so POST /global/dispose fell through to net/http's default
	// 404. Serve the upstream global.dispose operation; the rest of the
	// /global/* family keeps its existing registrations — this is an exact
	// pattern, never a /global/* catch-all.
	mux.HandleFunc("/global/dispose", s.handleGlobalDispose)
	// ROUTE-ADD-092 / SHIM-DRIFT-090: the exact /global/upgrade path was never
	// registered, so POST /global/upgrade fell through to net/http's default
	// 404. Serve the upstream global.upgrade operation; this is an exact
	// pattern, never a /global/* catch-all — /global/dispose and every other
	// declared /global/* operation stays as it was (the /global/* entry in
	// MountPatterns only exposes this subtree to a parent chi router).
	mux.HandleFunc("/global/upgrade", s.handleGlobalUpgrade)

	// Sessions
	mux.HandleFunc("/session", s.handleSessions)
	mux.HandleFunc("/session/", s.handleSessionByID)

	// Fixed-workspace compatibility routes used by the upstream HttpApi.
	mux.HandleFunc("/path", s.handlePath)
	mux.HandleFunc("/log", s.handleLog)
	// ROUTE-ADD-112 / SHIM-DRIFT-126: the exact /sync/history path was never
	// registered, so POST /sync/history fell through to net/http's default
	// 404. Serve the upstream sync.history.list operation. Every declared
	// /sync/* operation gains its own exact pattern (history, replay, start,
	// steal) — never a /sync/* catch-all.
	mux.HandleFunc("/sync/history", s.handleSyncHistory)
	// ROUTE-ADD-113 / SHIM-DRIFT-127: same omission one path over — POST
	// /sync/replay had no shim route at all and answered net/http's default
	// 404. Serve the upstream sync.replay operation.
	mux.HandleFunc("/sync/replay", s.handleSyncReplay)
	// ROUTE-ADD-114 / SHIM-DRIFT-128: the exact /sync/start path was never
	// registered either, so POST /sync/start fell through to net/http's default
	// 404. Serve the upstream sync.start operation.
	mux.HandleFunc("/sync/start", s.handleSyncStart)
	// ROUTE-ADD-115 / SHIM-DRIFT-129: the last declared /sync/* operation —
	// POST /sync/steal — had no shim route and answered net/http's default
	// 404 (the drift artifact's NOT-SERVED class). Serve the upstream
	// sync.steal operation; with this the /sync family is one exact pattern
	// per declared operation, never a /sync/* catch-all.
	mux.HandleFunc("/sync/steal", s.handleSyncSteal)
	mux.HandleFunc("/question", s.handleQuestionList)
	mux.HandleFunc("/question/", s.handleQuestionByID)

	// Doc
	mux.HandleFunc("/doc", s.handleDoc)

	// Config/Provider/Agent
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/config/providers", s.handleConfigProviders)
	mux.HandleFunc("/provider", s.handleProvider)
	// ROUTE-ADD-099 / SHIM-DRIFT-102: the exact /provider/auth path was never
	// registered, so GET /provider/auth fell through to net/http's default
	// 404 while /provider/* sub-paths are chi-mounted. Serve the upstream
	// provider.auth operation. The exact pattern wins over the wildcard below,
	// and no pattern here is a /provider/* catch-all, so an unrelated
	// /provider/* sub-path (unknown operations) keeps the net/http default 404
	// it is classified with.
	mux.HandleFunc("/provider/auth", s.handleProviderAuth)
	// ROUTE-ADD-100 / SHIM-DRIFT-103: the declared provider.oauth.authorize
	// operation (POST /provider/{providerID}/oauth/authorize) had no shim
	// route at all — the exact /provider/auth registration above does not
	// cover it and the mux has no /provider/ subtree — so the request fell
	// through to net/http's default 404. Serve it for real. The pattern pins
	// the shape (one provider-id segment + the literal oauth/authorize
	// suffix), so it matches exactly the declared operation.
	mux.HandleFunc("/provider/{providerID}/oauth/authorize", s.handleProviderOAuthAuthorize)
	// ROUTE-ADD-101 / SHIM-DRIFT-098: the declared provider.oauth.callback
	// operation (POST /provider/{providerID}/oauth/callback) had the same gap
	// as its authorize sibling — no shim route, so it fell through to
	// net/http's default 404. Serve it for real. The pattern pins the shape
	// (one provider-id segment + the literal oauth/callback suffix), so it
	// matches exactly the declared operation; a deeper path, the two-segment
	// /provider/oauth/callback and every other /provider/* sub-path keep the
	// default 404 (no catch-all).
	mux.HandleFunc("/provider/{providerID}/oauth/callback", s.handleProviderOAuthCallback)
	mux.HandleFunc("/agent", s.handleAgent)
	mux.HandleFunc("/skill", s.handleSkill)
	// ROUTE-ADD-088 / SHIM-DRIFT-086: the exact /formatter path was never
	// registered, so GET /formatter fell through to net/http's default 404
	// (the drift row's NOT-SERVED class, indistinguishable from "no such
	// opencode operation"). Serve the upstream formatter.status operation.
	// The /instance/formatter sub-path — a different surface, the /instance/*
	// translation — keeps its 501 stub (instanceKnownSubpaths).
	mux.HandleFunc("/formatter", s.handleFormatter)

	// Tools
	mux.HandleFunc("/experimental/tool", s.handleTools)
	mux.HandleFunc("/experimental/tool/ids", s.handleToolIDs)

	// File endpoints (stubs via tool execution API)
	mux.HandleFunc("/find", s.handleFind)
	mux.HandleFunc("/find/", s.handleFindSub)
	// ROUTE-ADD-087 / SHIM-DRIFT-084: the exact /file path was never
	// registered, so GET /file (upstream file.list) fell through to net/http's
	// default 404 while the /file/* sub-paths below were served. Serve the
	// bare listing endpoint; the exact sub-path registrations are untouched
	// (ServeMux resolves the longer pattern first, so /file/content and
	// /file/status keep their handlers).
	mux.HandleFunc("/file", s.handleFileList)
	mux.HandleFunc("/file/content", s.handleFileContent)
	mux.HandleFunc("/file/status", s.handleFileStatus)

	// Permissions (HITL → permission events)
	mux.HandleFunc("/permission", s.handlePermissions)
	mux.HandleFunc("/permission/", s.handlePermissionByID)

	// PTY (ROUTE-ADD-102 / ROUTE-FIX-010): bare /pty serves the upstream
	// pty.list (GET, 200) and answers pty.create (POST) with the typed
	// not-implemented envelope. /pty/* sub-paths stay unregistered.
	mux.HandleFunc("/pty", s.handlePty)

	// TUI control (shim-only, passthrough)
	mux.HandleFunc("/tui/", s.handleTUI)

	// LSP
	mux.HandleFunc("/lsp", s.handleLSP)

	// MCP management
	mux.HandleFunc("/mcp", s.handleMCPEndpoint)
	// ROUTE-ADD-093 / SHIM-DRIFT-092: the /mcp/{name}/auth path was never
	// registered, so DELETE /mcp/{name}/auth fell through to net/http's
	// default 404 ("the shim has no such operation" was indistinguishable
	// from "this shim does not implement it"). Serve the upstream
	// mcp.auth.remove operation; every other /mcp/* shape — the rest of the
	// declared family (auth.start, auth/authenticate, auth/callback,
	// connect, disconnect) and every wrong method on the auth path — keeps
	// the byte-identical pre-change answer (http.NotFound, see handleMCPSub),
	// so the sibling artifact rows stay NOT-SERVED until their own rows serve
	// them. This is a shape check, never a /mcp/* catch-all.
	mux.HandleFunc("/mcp/", s.handleMCPSub)

	// Auth management (SPEC-017 §3.2)
	mux.HandleFunc("/auth/", s.handleAuth)

	// Standalone /event endpoint for SSE
	mux.HandleFunc("/event", s.handleGlobalEvent)

	// Project/VCS as 501 stubs (SPEC-017 §3.9); /instance is a real
	// opencode-protocol translation surface (SPEC-017 §3.10).
	// DF-CONSENSUS-38: bare GET /vcs and GET /vcs/diff are real
	// fixed-workspace compatibility routes (upstream
	// httpapi-instance.test.ts "serves path and VCS read endpoints" probes
	// them with x-opencode-directory and expects 200); the remaining /vcs/*
	// sub-paths keep the 501 stub. ServeMux resolves the longer pattern, so
	// the exact /vcs/diff registration wins over the /vcs/ subtree stub.
	// ROUTE-FIX-004: bare GET /project serves the declared project.list
	// operation (200 Project[], 400) from the runtime's projects table; the
	// 501 stub keeps every other method on the mount.
	mux.HandleFunc("/project", s.handleProject)
	// DF-CONSENSUS-47: /project/{id} sub-paths answer the upstream typed
	// ProjectNotFoundError (404) instead of the 501 stub — see §3.9.
	mux.HandleFunc("/project/", s.handleProjectByID)
	mux.HandleFunc("/vcs", s.handleVCS)
	mux.HandleFunc("/vcs/diff", s.handleVCS)
	mux.HandleFunc("/vcs/", s.handleProjectVCSSStub)
	mux.HandleFunc("/instance", s.handleInstance)
	mux.HandleFunc("/instance/", s.handleInstanceSub)

	return s
}

// SetMCPHandler wires the real MCP HTTP handler into the shim's bare /mcp
// mount (MCP-DIRECT-001): MCP client requests that reach the shim mount are
// delegated to it instead of answering the 501 stub. Passing nil (the
// zero-value default) keeps the stub answer unchanged.
func (s *Server) SetMCPHandler(h http.Handler) {
	s.mcpHandler = h
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
//
// /doc and /doc/*: /doc serves the machine-readable OpenAPI document
// (upstream opencode compatibility, DF-CONSENSUS-36); /doc/api — the native
// REST Swagger UI (SPEC-018 §9) — is NOT claimed by the shim (the shim mux
// has no /doc/api route, so chi's static-route precedence keeps the API
// server's /doc/api handler authoritative in full deployments). The patterns
// are still listed so shim sub-path requests never leak to a 404 fallback.
var MountPatterns = []string{
	"/global/*",
	"/session", "/session/*",
	"/path", "/log",
	"/question", "/question/*",
	"/sync/history",
	"/sync/replay",
	"/sync/start",
	"/sync/steal",
	"/config", "/config/*",
	"/provider", "/provider/*",
	"/agent", "/agent/*",
	"/skill", "/skill/*",
	"/formatter", "/formatter/*",
	"/experimental/*",
	"/find", "/find/*",
	"/file", "/file/*",
	"/event",
	"/permission", "/permission/*",
	"/pty",
	"/tui/*",
	"/lsp", "/lsp/*",
	"/doc", "/doc/*",
	// ROUTE-ADD-093: the exact declared param shape for mcp.auth.remove. The
	// native MCP server owns /mcp/* in the combined deployment (main.go mounts
	// it before the shim's patterns); chi prefers this deeper param route over
	// that catch-all, so only the auth sub-path is claimed from the native
	// mount — /mcp/sse and /mcp/message still reach the MCP server.
	"/mcp/{name}/auth",
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
		// DF-CONSENSUS-19: because this surface is auth-free by protocol
		// compatibility and discloses host layout, it must be served on a
		// loopback-bound listener (the default); do not expose it on a
		// non-loopback interface.
		if p := r.URL.Path; p == "/instance" || strings.HasPrefix(p, "/instance/") {
			next.ServeHTTP(w, r)
			return
		}

		// The pinned upstream HttpApi sends fixed-workspace requests without
		// Consensus credentials and identifies them with x-opencode-directory.
		// Exempt only the exact compatibility routes; all other shim and native
		// API routes retain Consensus authentication.
		if isFixedWorkspaceCompatibilityRequest(r) {
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

// handleGlobalDispose serves POST /global/dispose — upstream global.dispose
// (ROUTE-ADD-091, SHIM-DRIFT-089, declared responses: 200 boolean, 400
// BadRequest). Upstream "disposes all OpenCode instances, releasing all
// resources"; the shim keeps no per-instance registry to release — opencode
// sessions map onto Consensus rows owned by the API service, so a dispose
// request is honored as a successful no-op and answers the declared boolean
// true (the sibling handleAuthDelete boolean convention, idempotent for a
// repeated POST). The operation declares no parameters and no request body, so
// there is no input to reject against the declared 400 arm. Non-POST answers
// 405 METHOD_NOT_ALLOWED — the sibling method-guard convention
// (handleSkill, handleSyncHistory).
func (s *Server) handleGlobalDispose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	writeJSON(w, true)
}

// handleGlobalUpgrade serves POST /global/upgrade — upstream global.upgrade
// (ROUTE-ADD-092, SHIM-DRIFT-090, declared responses: 200 UpgradeResult,
// 400 BadRequest | InvalidRequestError). Upstream installs the opencode
// version named by the request body; the Consensus shim is an in-process
// protocol translation surface that manages no opencode installation, so it
// answers the declared UpgradeResult failure arm truthfully —
// {"success": false, "error": "<why>"} — instead of fabricating a version
// number for an upgrade that did not happen (a bare 404 is itself the drift
// the row closes). A parent chi mount already reaches this subtree through
// the /global/* MountPatterns entry; only the exact mux path was missing.
//
// Input handling follows the declared requestBody schema — the object
// {target*: string} with additionalProperties: false — and the sibling
// POST-with-body convention (handleSyncHistory): an absent body is well-formed
// (the document leaves requestBody optional), while a body that is present
// must satisfy the schema or it answers the declared 400 via the sibling
// writeOpencodeError INVALID_REQUEST envelope. Non-POST answers 405
// METHOD_NOT_ALLOWED — sibling method-guard convention (handleSkill,
// handleSyncHistory). No query parameter is validated: global.upgrade
// declares none.
func (s *Server) handleGlobalUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if r.Body != nil && r.ContentLength != 0 {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
			return
		}
		// A JSON null body decodes without error into a nil map
		// (encoding/json leaves the destination untouched) — still not the
		// declared {target} object.
		if body == nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				`request body must be a JSON object with a "target" field`)
			return
		}
		for field := range body {
			if field != "target" {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("unknown field %q: the declared body schema allows only \"target\"", field))
				return
			}
		}
		raw, ok := body["target"]
		if !ok {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				`required field "target" is missing`)
			return
		}
		var target string
		if err := json.Unmarshal(raw, &target); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				`field "target" must be a string`)
			return
		}
		if strings.TrimSpace(target) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				`field "target" must not be blank`)
			return
		}
	}
	writeJSON(w, map[string]any{
		"success": false,
		"error": "opencode self-upgrade is not supported by the Consensus shim: " +
			"this surface translates the opencode protocol onto the Consensus runtime and manages no opencode installation",
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
	goal := req.Goal
	if goal == "" {
		goal = req.Title
	}
	if goal == "" {
		goal = agentName
	}

	// Try service layer first (SPEC-017 §2: shim calls native API)
	if s.svc != nil {
		result, err := s.svc.CreateSession(r.Context(), SessionCreateInput{
			AgentName:     agentName,
			Goal:          goal,
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
		setFixedWorkspaceSyncFence(w, r, result.SessionID)
		writeJSON(w, resp)
		return
	}

	// Fallback: raw DB access (backwards-compatible; used when svc is nil in tests)
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

	setFixedWorkspaceSyncFence(w, r, sessionID)
	writeJSON(w, resp)
}

func setFixedWorkspaceSyncFence(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !isFixedWorkspaceCompatibilityRequest(r) {
		return
	}
	fence, err := json.Marshal(map[string]int{sessionID: 0})
	if err == nil {
		w.Header().Set("x-opencode-sync", string(fence))
	}
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

	// "status" is a reserved opencode literal, not a session id: /session/status
	// is the aggregate session-status operation (ROUTE-FIX-008 /
	// SHIM-DRIFT-114). Without this branch the id parser below would look up a
	// session literally named "status" and answer 404, telling the client the
	// operation does not exist.
	if path == "status" {
		if r.Method == http.MethodGet {
			s.sessionStatus(w, r)
		} else {
			writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
		}
		return
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
	case sub == "command" && r.Method == http.MethodPost:
		// ROUTE-FIX-009 / SHIM-DRIFT-115: the upstream session.command
		// operation (declared responses: 200, 400, 404) was served by the
		// stub-list 501 below. Serve it for real.
		s.sessionCommand(w, r, sessionID)
	case sub == "message" && r.Method == http.MethodPost:
		s.sendMessage(w, r, sessionID)
	case sub == "message" && r.Method == http.MethodGet:
		s.listMessages(w, r, sessionID)
	case strings.HasPrefix(sub, "message/") && r.Method == http.MethodGet:
		// GET /session/:id/message/:messageID
		msgID := strings.TrimPrefix(sub, "message/")
		s.getMessageByID(w, r, sessionID, msgID)
	case strings.HasPrefix(sub, "message/") && strings.Contains(sub, "/part/") && r.Method == http.MethodDelete:
		// ROUTE-FIX-036 / SHIM-NARROWED-007: the upstream part.delete
		// operation (declared responses 200, 400 BadRequest |
		// InvalidRequestError, 404 NotFoundError) had no case here, so every
		// DELETE /session/{sessionID}/message/{messageID}/part/{partID}
		// request fell to the router's default arm and answered an untyped
		// 404 — the declared error vocabulary was unreachable from outside
		// the code. Serve the declared contract truthfully (see
		// sessionPartDelete); the undeclared methods on the sub-path keep
		// the pre-existing answers, and the sibling PATCH keeps its own case
		// below.
		//
		// ch:trace row=ROUTE-FIX-036 spec=specs/openapi/upstream/openapi-1.18.33.json#part.delete wave=consensus-foreman-2026-10-04-06-15-38.json#task-1 test=TestSessionPartDeleteTruthfulArms doc=docs/evidence/ROUTE-FIX-036-live-probe.md evidence=docs/evidence/ROUTE-FIX-036-live-probe.md witness=none:self-verified-in-worktree
		msgID, partID := parseMessagePartSub(sub)
		s.sessionPartDelete(w, r, sessionID, msgID, partID)
	case strings.HasPrefix(sub, "message/") && r.Method == http.MethodDelete:
		// ROUTE-FIX-035 / SHIM-NARROWED-005: the upstream
		// session.deleteMessage operation (declared responses 200 boolean,
		// 400 BadRequest | InvalidRequestError, 404 NotFoundError, 409
		// SessionBusyError) answered an untyped 404 from this default arm
		// for every request — the declared success code was unreachable.
		// Serve the declared contract truthfully; undeclared methods on the
		// sub-path keep the pre-existing router default 404.
		msgID := strings.TrimPrefix(sub, "message/")
		s.sessionDeleteMessage(w, r, sessionID, msgID)
	case strings.HasPrefix(sub, "message/") && strings.Contains(sub, "/part/") && r.Method == http.MethodPatch:
		// ROUTE-FIX-037 / SHIM-NARROWED-006: the upstream part.update
		// operation (declared responses 200 Part "Successfully updated
		// part", 400 BadRequest | InvalidRequestError, 404 NotFoundError)
		// had no case here, so every PATCH
		// /session/{sessionID}/message/{messageID}/part/{partID} request fell
		// to the router's default arm and answered an untyped 404 — the
		// declared error vocabulary was unreachable from outside the code.
		// Serve the declared contract truthfully (see sessionPartUpdate);
		// the undeclared methods on the sub-path keep the pre-existing
		// answers, and the sibling DELETE keeps its own case above.
		//
		// ch:trace row=ROUTE-FIX-037 spec=specs/openapi/upstream/openapi-1.18.33.json#part.update wave=consensus-foreman-2026-10-03-00-46-49.json#task-1 test=TestSessionPartUpdateTruthfulArms doc=docs/evidence/ROUTE-FIX-037-live-probe.md evidence=docs/evidence/ROUTE-FIX-037-live-probe.md witness=none:self-verified-in-worktree
		msgID, partID := parseMessagePartSub(sub)
		s.sessionPartUpdate(w, r, sessionID, msgID, partID)
	case sub == "children" && r.Method == http.MethodGet:
		s.listChildren(w, r, sessionID)
	case sub == "diff" && r.Method == http.MethodGet:
		// ROUTE-FIX-010 / SHIM-DRIFT-116: the declared session.diff
		// operation (declared responses 200,400) was served by the typed 501
		// stub below. Serve it for real.
		s.sessionDiff(w, r, sessionID)
	case sub == "diff":
		// Non-GET on the sub-path keeps the pre-existing 501 answer: only GET
		// is a declared opencode operation for /session/{id}/diff, and 405 is
		// not part of that operation's declared response set.
		writeNotImplemented(w, r, "session.diff",
			"session diff is a GET operation; use GET /session/{sessionID}/diff, or GET /instance/vcs/diff for the workspace diff")
	case sub == "todo" && r.Method == http.MethodGet:
		// ROUTE-FIX-039 / SHIM-NARROWED-009: the upstream session.todo
		// operation (declared responses 200 Array(Todo), 400 BadRequest |
		// InvalidRequestError, 404 NotFoundError) had no case here, so every
		// GET /session/{sessionID}/todo request fell to the router catch-all
		// and answered an untyped 404 — the declared 200 was unreachable. Serve
		// the declared contract truthfully from the runtime's per-session
		// `tasks` ledger (see sessionTodo); undeclared methods on the sub-path
		// keep the pre-existing router default 404.
		s.sessionTodo(w, r, sessionID)
	case sub == "revert" && r.Method == http.MethodPost:
		// ROUTE-FIX-014 / SHIM-DRIFT-120: the upstream session.revert
		// operation (declared responses 200 Session, 400, 404, 409
		// SessionBusyError) was served by the stub-list 501 below. Serve the
		// declared contract truthfully; non-POST on the sub-path falls
		// through to the stub list (405 is not in the declared set).
		s.sessionRevert(w, r, sessionID)
	case sub == "share" && r.Method == http.MethodDelete:
		// ROUTE-FIX-015 / SHIM-DRIFT-121: the upstream session.unshare
		// operation (declared responses 200, 400, 404, 500) was answered by
		// the typed 501 stub. Serve the declared contract truthfully; the
		// runtime keeps no share concept, so a known session has no active
		// share to remove and answers the declared 404.
		s.sessionUnshare(w, r, sessionID)
	case sub == "share" && r.Method == http.MethodPost:
		// ROUTE-FIX-016 / SHIM-DRIFT-122: the upstream session.share
		// operation (declared responses 200, 400, 404, 500) was served by
		// the stub-list 501 below. Serve the declared contract truthfully;
		// Consensus publishes no sessions, so no share URL can be fabricated.
		s.sessionShare(w, r, sessionID)
	case sub == "shell" && r.Method == http.MethodPost:
		// ROUTE-FIX-017 / SHIM-DRIFT-123: the upstream session.shell
		// operation (declared responses 200 created message, 400, 404, 409
		// SessionBusyError) was served by the stub-list 501 below. Serve the
		// declared contract truthfully; non-POST falls through to the stub
		// list.
		s.sessionShell(w, r, sessionID)
	case sub == "summarize" && r.Method == http.MethodPost:
		// ROUTE-FIX-018 / SHIM-DRIFT-124: the upstream session.summarize
		// operation (declared responses 200 boolean, 400, 404) was served by
		// the stub-list 501 below. Serve the declared contract truthfully;
		// non-POST falls through to the stub list.
		s.sessionSummarize(w, r, sessionID)
	case sub == "init" && r.Method == http.MethodPost:
		// ROUTE-FIX-012 / SHIM-DRIFT-118: the upstream session.init operation
		// (declared responses: 200 boolean, 400, 404) was served by the
		// stub-list 501 below. Serve it for real.
		s.sessionInit(w, r, sessionID)
	case sub == "prompt_async" && r.Method == http.MethodPost:
		// ROUTE-FIX-013 / SHIM-DRIFT-113: the upstream session.prompt_async
		// operation (declared responses 204, 400, 404) was served by the
		// stub-list 501 below. Serve it for real.
		s.sessionPromptAsync(w, r, sessionID)
	case sub == "fork" && r.Method == http.MethodPost:
		// ROUTE-FIX-011 / SHIM-DRIFT-117: the upstream session.fork operation
		// (declared responses 200,400,404) was served by the untyped stub
		// below. Serve it for real.
		s.sessionFork(w, r, sessionID)
	case sub == "todo" && r.Method == http.MethodGet:
		// ROUTE-FIX-039 / SHIM-NARROWED-009: the upstream session.todo
		// operation (declared responses 200 Todo[], 400, 404) was answered by
		// an untyped 404 from this default arm — the declared success code was
		// unreachable. Serve the declared contract truthfully; non-GET falls
		// through to the router default (405 is not in the declared set).
		s.sessionTodo(w, r, sessionID)
	case sub == "unrevert" && r.Method == http.MethodPost:
		// ROUTE-FIX-040 / SHIM-NARROWED-010: the upstream session.unrevert
		// operation (declared responses 200 Session, 400, 404, 409
		// SessionBusyError) was answered by an untyped 404 from this default
		// arm. Serve the declared contract truthfully with the sessionRevert
		// handler shape; non-POST falls through to the router default.
		s.sessionUnrevert(w, r, sessionID)
	case strings.HasPrefix(sub, "permissions/") && r.Method == http.MethodPost:
		// ROUTE-FIX-038 / SHIM-NARROWED-008: the upstream permission.respond
		// operation (declared responses 200 boolean, 400, 404
		// NotFoundError | PermissionNotFoundError) was answered by an untyped
		// 404 from this default arm. Serve the declared contract truthfully,
		// mapping the declared enum onto the real approval_requests columns
		// exactly as the consent-store sidecar's POST
		// /permission/{id}/resolve does (commit 07f2f3c); non-POST falls
		// through to the router default.
		permissionID := strings.TrimPrefix(sub, "permissions/")
		s.sessionPermissionRespond(w, r, sessionID, permissionID)
	default:
		// Check for 501 exclusions. The five P1 session sub-paths now answer
		// their DECLARED methods in the cases above (ROUTE-FIX-014..018:
		// revert/share/shell/summarize POST, share DELETE); these entries
		// stay so every UNDECLARED method on the same sub-path (GET
		// /session/{id}/revert, GET /session/{id}/share, PUT
		// /session/{id}/shell, ...) keeps the pre-existing 501 — 405 is not
		// part of any of these operations' declared response sets.
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
		`SELECT id, agent_name, model_id, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
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

// sessionStatus serves GET /session/status — upstream session.status
// (ROUTE-FIX-008, SHIM-DRIFT-114, declared responses: 200 map of sessionID to
// SessionStatus, 400 BadRequest). The aggregate is derived from the same
// sessions store the single-session handler reads. SessionStatus is an anyOf
// of three objects discriminated by "type" (idle / retry / busy); the shim
// maps the Consensus session states (SPEC-011 §1) onto them:
//
//	idle, planning, thinking, tool_exec, waiting_sub, paused → {"type":"idle"}
//	booting, executing, completed                            → {"type":"busy"}
//	failed                                                   → {"type":"retry"}
//
// An empty store answers {} — the contract declares an object, never null.
func (s *Server) sessionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at
		 FROM sessions ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		// The document's only error response for this operation is 400
		// BadRequest | InvalidRequestError; answer it the way sibling
		// handlers do instead of surfacing an undeclared 500.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to read session statuses")
		return
	}

	result := make(map[string]any, len(rows))
	for _, row := range rows {
		result[toString(row["id"])] = sessionStatusEntry(toString(row["status"]), toInt64(row["iteration"]))
	}
	writeJSON(w, result)
}

// sessionStatusEntry translates a Consensus session status into the upstream
// SessionStatus shape: all active states report idle, in-flight and finished
// states report busy, and a failed session reports retry with its iteration
// count as the attempt number.
func sessionStatusEntry(status string, iteration int64) map[string]any {
	switch status {
	case "failed":
		return map[string]any{
			"type":    "retry",
			"attempt": iteration,
			"message": "session failed",
			"next":    int64(0),
		}
	case "booting", "executing", "completed":
		return map[string]any{"type": "busy"}
	default:
		// idle, planning, thinking, tool_exec, waiting_sub, paused
		return map[string]any{"type": "idle"}
	}
}

// sessionCommand serves POST /session/{id}/command — upstream session.command
// (ROUTE-FIX-009, SHIM-DRIFT-115, declared responses: 200 {info:
// AssistantMessage, parts: []Part}, 400 BadRequest | InvalidRequestError, 404
// NotFoundError). Upstream executes a slash command inside the session: the
// command text and its arguments reach the assistant as one user instruction
// and the response is the assistant turn they produce ("Send a new command to
// a session for execution by the AI assistant").
//
// The shim composes "<command> <arguments>" and sends it through the same
// synchronous message path POST /session/{id}/message uses (SPEC-017 §3.2:
// return the response produced for this turn), so the declared arms map as:
//
//	200 → the produced agent turn in the upstream {info, parts} shape
//	400 → malformed body, missing required command/arguments (the upstream
//	      requestBody requires both), or a turn that cannot be produced
//	404 → unknown session
//
// Non-POST on the sub-path keeps the pre-existing stub-list 501.
func (s *Server) sessionCommand(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		Command   string `json:"command"`
		Arguments string `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	if strings.TrimSpace(req.Command) == "" || strings.TrimSpace(req.Arguments) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "command and arguments are required")
		return
	}

	if s.svc == nil {
		// The command cannot be executed without the native service layer;
		// answer inside the declared contract instead of the stub 501.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "command execution requires the native service layer")
		return
	}

	// Resolve the session first so an unknown id answers the declared 404
	// (NotFoundError) before any turn is attempted.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	// Execute: the composed instruction goes through the same synchronous
	// turn path as POST /session/{id}/message, with the same bounded
	// response timeout (SPEC-017 §3.2).
	timeout := s.messageResponseTimeout
	if timeout <= 0 {
		timeout = defaultMessageResponseTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := s.svc.SendMessage(ctx, MessageSendInput{
		SessionID: sessionID,
		Content:   strings.TrimSpace(req.Command) + " " + strings.TrimSpace(req.Arguments),
		MsgType:   "user_instruction",
	})
	if err != nil || result == nil || strings.TrimSpace(result.Content) == "" {
		// The declared error vocabulary for this operation is 400 | 404 only;
		// a turn that cannot be produced (timeout, failed session, empty
		// response) answers 400 INVALID_REQUEST naming the reason.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "command could not be executed: "+errorMessage(err, result))
		return
	}

	writeJSON(w, s.buildAssistantMessage(result.Content))
}

// errorMessage renders the failure reason for the command 400 arm from
// whichever of the error / nil-result shapes carried it.
func errorMessage(err error, result *MessageSendResult) string {
	if err != nil {
		return err.Error()
	}
	return "the agent produced no response"
}

// sessionDiff serves GET /session/{sessionID}/diff — the upstream session.diff
// operation (ROUTE-FIX-010, formerly SHIM-DRIFT-116).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/diff".get): declared responses 200
// Array(SnapshotFileDiff) and 400 BadRequest; the optional query selectors are
// directory, workspace and messageID (declared pattern ^msg). Before this
// change the sub-path answered the typed 501 not-implemented envelope — a code
// the document does not declare for the operation.
//
// The shim has one workspace per instance, so a session's file changes ARE the
// workspace's file changes: the same git status + numstat translation
// GET /instance/vcs/diff and GET /vcs/diff report (SnapshotFileDiff: file,
// additions, deletions, status ∈ {added,deleted,modified,} — additions and
// deletions are required by the schema and always present, patch is optional
// and omitted). Consensus keeps no per-message workspace snapshot, so a
// messageID is validated and resolved against the session's message store but
// does not narrow the returned set; the declared contract is served rather
// than the stub that limitation used to justify.
//
// Workspace resolution matches the sibling VCS read routes: the
// x-opencode-directory header wins, then the declared ?directory= selector,
// then the server's configured workdir (else the process CWD).
//
// The declared error vocabulary for this operation is 400 — there is no
// declared 404/5xx — so every failure answers 400 with the sibling
// INVALID_REQUEST envelope and never a 501:
//   - the session id is unknown
//   - the session store cannot be read
//   - messageID is present but blank, or does not match the declared ^msg
//     pattern (a query-parameter violation)
//   - messageID is well-formed but names no message in that session
func (s *Server) sessionDiff(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()

	row, err := s.db.QueryRow(ctx, `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		// The declared vocabulary carries 400 only; a client must not have to
		// distinguish an undeclared 404 from a real route miss.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "unknown session: "+sessionID)
		return
	}

	if values, present := r.URL.Query()["messageID"]; present {
		messageID := ""
		if len(values) > 0 {
			messageID = strings.TrimSpace(values[0])
		}
		if messageID == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"invalid query parameter messageID: value must not be blank")
			return
		}
		if !strings.HasPrefix(messageID, "msg") {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"invalid query parameter messageID: "+messageID+" does not match the declared pattern ^msg")
			return
		}
		if !s.sessionMessageExists(ctx, sessionID, messageID) {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"unknown message: "+messageID)
			return
		}
	}

	writeJSON(w, gitFileDiffs(ctx, s.sessionDiffWorkspaceDir(r)))
}

// sessionTodo serves GET /session/{sessionID}/todo — the upstream session.todo
// operation (ROUTE-FIX-039 / SHIM-NARROWED-009 board row; declared responses:
// 200 Array(Todo), 400 BadRequest | InvalidRequestError, 404 NotFoundError).
//
// ch:trace row=ROUTE-FIX-039 spec=specs/openapi/upstream/openapi-1.18.33.json#session.todo test=TestSessionTodoServesDeclared200 doc=docs/evidence/ROUTE-FIX-039-live-probe.md evidence=docs/evidence/ROUTE-FIX-039-live-probe.md witness=none:self-verified-in-worktree
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/todo".get): the sessionID path parameter is
// declared pattern ^ses; the optional query selectors are directory and
// workspace; the declared 200 body is an ARRAY of Todo objects (required
// content, status, priority; additionalProperties: false), described as
// "Retrieve the todo list associated with a specific session, showing tasks
// and action items". Before this change the sub-path had no case in
// handleSessionByID and fell to the router catch-all, which answered an
// untyped 404 for EVERY request — the declared 200 was unreachable and a
// client could not tell "no such operation" from "this session has no todos".
//
// Translation: the runtime DOES keep per-session action items — the `tasks`
// table (migrations 001/009: session_id, title, description, status,
// priority 1..10, created_at) is a per-session work ledger, exactly the "tasks
// and action items" the operation describes. Each row is translated into the
// declared Todo shape:
//
//   - content  <- title (the task's brief description); description is the
//     fallback when a row carries an empty title (the column is NOT NULL but
//     not CHECKed non-empty);
//   - status   <- the tasks.status vocabulary (pending, claimed, in_progress,
//     reviewed, published, failed, cancelled — CHECK-constrained in both
//     dialects) mapped onto the four values the declared field describes:
//     pending -> pending; claimed/in_progress -> in_progress (a claimed task
//     has been taken, so it is no longer pending); reviewed/published ->
//     completed (both are terminal successes); failed/cancelled -> cancelled
//     (neither will complete, and the declared set has no failed bucket). A
//     value outside that CHECK vocabulary is passed through unchanged rather
//     than folded into a bucket it did not earn;
//   - priority <- the tasks.priority integer scale (1 = most urgent .. 10 =
//     least) mapped onto the three levels the declared field describes:
//     1-3 -> high, 4-7 -> medium, 8-10 -> low. A value <= 0 (unreachable under
//     the CHECK) is reported as an empty string: the required key is present,
//     but the mapper never asserts a priority it did not read.
//
// A session with no task rows answers the truthful empty ARRAY [] — never JSON
// null (the declared schema is type array) and never a fabricated entry. This
// is the same convention as GET /session/{id}/diff answering [] for a
// workspace with no changes.
//
// Validation order and the declared error arms:
//   - a present-but-blank declared query selector (directory, workspace) -> the
//     declared 400 INVALID_REQUEST naming the parameter. This operation
//     declares no requestBody, so the selectors are what keeps the declared 400
//     arm reachable (the sessionDiff / sync.steal precedent: a
//     present-but-blank declared query parameter is a contract violation, not
//     an absent one). A well-formed selector is accepted and does not narrow
//     the result — the runtime keeps one workspace per instance and the task
//     ledger is session-scoped, so neither selector scopes these rows (the
//     sibling sessionDiff messageID note);
//   - unknown session -> the declared 404 NotFoundError, via the shared
//     p1ResolveSession read (the session-row lookup every sibling session
//     sub-path uses); it is checked AFTER the query validation so a malformed
//     request is refused before any store read;
//   - the task ledger cannot be read -> the declared 400 INVALID_REQUEST
//     naming the reason, never an undeclared 5xx (the sessionDiff convention
//     for an unreadable store; the operation declares no 5xx).
//
// The declared ^ses pattern on the sessionID path parameter is deliberately
// NOT enforced: Consensus mints session ids as UUIDs
// (internal/api/sessions.go newUUID, e.g. "1a2b3c4d-..."), so a ^ses gate would
// refuse every session the runtime actually creates and leave the declared 200
// unreachable — the very defect this row fixes. The sibling session sub-path
// handlers (sessionRevert / sessionShell / sessionDeleteMessage /
// sessionPartUpdate) likewise resolve the path sessionID by row lookup and
// answer the declared 404 for an unknown id.
func (s *Server) sessionTodo(w http.ResponseWriter, r *http.Request, sessionID string) {
	// 1. A present-but-blank declared query selector is the declared 400 (this
	// operation declares no requestBody, so this is what keeps the 400 arm
	// reachable). Validation runs before the session lookup so a malformed
	// request never reaches the store.
	for _, param := range []string{"directory", "workspace"} {
		values, present := r.URL.Query()[param]
		if !present {
			continue
		}
		value := ""
		if len(values) > 0 {
			value = strings.TrimSpace(values[0])
		}
		if value == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	// 2. Unknown session -> the declared 404 NotFoundError.
	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}

	// 3. The runtime's per-session action-item ledger.
	rows, err := s.db.Query(r.Context(),
		`SELECT title, description, status, priority
		 FROM tasks WHERE session_id = $1
		 ORDER BY created_at ASC, id ASC`,
		sessionID)
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"could not read the session todo list")
		return
	}

	todos := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		content := toString(row["title"])
		if strings.TrimSpace(content) == "" {
			content = toString(row["description"])
		}
		todos = append(todos, map[string]any{
			"content":  content,
			"status":   todoStatusFromTask(toString(row["status"])),
			"priority": todoPriorityFromTask(toInt64(row["priority"])),
		})
	}

	// The declared 200 body is an array: an empty result is [], never null.
	writeJSON(w, todos)
}

// todoStatusFromTask maps the runtime's tasks.status vocabulary (migrations
// 001/009) onto the four values the declared Todo.status field describes. A
// value outside that CHECK vocabulary is returned unchanged: the declared
// field is a plain string, and folding an unknown state into a bucket it did
// not earn would be less honest than passing it through.
func todoStatusFromTask(status string) string {
	switch status {
	case "pending":
		return "pending"
	case "claimed", "in_progress":
		return "in_progress"
	case "reviewed", "published":
		return "completed"
	case "failed", "cancelled":
		return "cancelled"
	default:
		return status
	}
}

// todoPriorityFromTask maps the runtime's tasks.priority integer scale (1 =
// most urgent .. 10 = least, migrations 001/009) onto the three levels the
// declared Todo.priority field describes. A value <= 0 is unreachable under the
// column's CHECK (1..10) and is reported as an empty string rather than
// silently labelled high or low; the declared key is always present.
func todoPriorityFromTask(priority int64) string {
	switch {
	case priority <= 0:
		return ""
	case priority <= 3:
		return "high"
	case priority <= 7:
		return "medium"
	default:
		return "low"
	}
}

// sessionDiffWorkspaceDir resolves the workspace a session-diff request reads:
// the upstream fixed-workspace selector header first, then the operation's own
// declared ?directory= selector, then the server's configured workdir (else
// the process CWD) — the same order the sibling /vcs and /instance/vcs reads
// use, so every diff route agrees on which tree it is describing.
func (s *Server) sessionDiffWorkspaceDir(r *http.Request) string {
	if r != nil && strings.TrimSpace(r.Header.Get("x-opencode-directory")) == "" {
		if requested := strings.TrimSpace(r.URL.Query().Get("directory")); requested != "" {
			return filepath.Clean(requested)
		}
	}
	return s.requestWorkspaceDir(r)
}

// sessionMessageExists reports whether messageID names a message in the
// session's message store. opencode message ids are "msg-<memory_events.id>"
// (listMessages / getMessageByID), so the numeric suffix is looked up exactly
// and then by prefix, mirroring getMessageByID's resolution. As everywhere else
// in the shim, a store error and an absent row are the same answer: the id is
// not a message of this session.
func (s *Server) sessionMessageExists(ctx context.Context, sessionID, messageID string) bool {
	trimmed := strings.TrimPrefix(messageID, "msg-")
	for _, query := range []string{
		`SELECT id FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) = $2 LIMIT 1`,
		`SELECT id FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) LIKE $2 LIMIT 1`,
	} {
		arg := trimmed
		if strings.Contains(query, "LIKE") {
			arg = trimmed + "%"
		}
		if row, err := s.db.QueryRow(ctx, query, sessionID, arg); err == nil && row != nil {
			return true
		}
	}
	return false
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

// handleAuth serves PUT /auth/:id — update auth/config for a provider/session
// (SPEC-017 §3.2: HARDEN-SHIM-08 remediation) — and DELETE /auth/:id — the
// upstream auth.remove operation (SHIM-DRIFT-059), which removes the stored
// auth rows for that provider.
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/auth/")
	switch {
	case path == "":
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use PUT /auth/:id")
	case r.Method == http.MethodPut:
		s.handleAuthPut(w, r, path)
	case r.Method == http.MethodDelete:
		s.handleAuthDelete(w, r, path)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use PUT /auth/:id")
	}
}

// handleAuthPut stores the submitted auth fields as system_settings rows
// keyed auth.<providerID>.<field>.
func (s *Server) handleAuthPut(w http.ResponseWriter, r *http.Request, path string) {
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

// handleAuthDelete implements upstream auth.remove (SHIM-DRIFT-059): remove
// every auth row PUT stored for the provider (system_settings keys with
// prefix auth.<providerID>.) and answer the upstream boolean success body
// (declared responses: 200 boolean, 400 BadRequest).
func (s *Server) handleAuthDelete(w http.ResponseWriter, r *http.Request, providerID string) {
	// Escape LIKE metacharacters so a provider id containing %, _ or \
	// removes only its own rows, not a broader prefix.
	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(providerID)
	ctx := r.Context()
	if err := s.db.Exec(ctx,
		`DELETE FROM system_settings WHERE key LIKE $1 ESCAPE '\'`,
		"auth."+pattern+".%"); err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to remove auth configuration")
		return
	}
	writeJSON(w, true)
}

func isFixedWorkspaceCompatibilityRequest(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("x-opencode-directory")) == "" {
		return false
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/session":
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/path":
		return true
	case r.Method == http.MethodGet && (r.URL.Path == "/vcs" || r.URL.Path == "/vcs/diff"):
		// DF-CONSENSUS-38: upstream "serves path and VCS read endpoints"
		// probes top-level GET /vcs and /vcs/diff with x-opencode-directory
		// and no credentials. Headerless requests still 401 (the endpoint
		// smoke contract keeps its auth row for GET /vcs).
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/log":
		return true
	case r.Method == http.MethodPost && isRequestActionPath(r.URL.Path, "/permission/", "reply"):
		return true
	case r.Method == http.MethodPost && (isRequestActionPath(r.URL.Path, "/question/", "reply") || isRequestActionPath(r.URL.Path, "/question/", "reject")):
		return true
	default:
		return false
	}
}

func isRequestActionPath(path, prefix, action string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] == action
}

// handleProject serves the bare /project mount. GET is the upstream
// project.list operation (ROUTE-FIX-004; declared responses 200
// Array(Project), 400 BadRequest; optional query selectors directory and
// workspace). Before this handler the mount answered the untyped 501 stub for
// every method (SHIM-DRIFT-098, class OUTCOME-MISMATCH: "declared 200,400,
// served 501").
//
// ch:trace row=ROUTE-FIX-004 spec=specs/openapi/upstream/openapi-1.18.33.json#project.list test=TestProjectListServesDeclaredContract doc=docs/evidence/ROUTE-FIX-004-live-probe.md evidence=docs/evidence/ROUTE-FIX-004-live-probe.md witness=none:self-verified-in-worktree
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/project".get): the 200 body is an ARRAY of Project objects
// (required: id, worktree, time, sandboxes; additionalProperties: false),
// described as "Get a list of projects that have been opened with OpenCode."
//
// Translation: the runtime DOES keep that registry — the `projects` table
// (migrations 014/015: id, name, description, created_at; SPEC-004 §RBAC
// scope boundaries; sessions/tasks carry project_id). Each row is translated
// into the declared Project shape:
//
//   - id        <- projects.id
//   - name      <- projects.name
//   - worktree  <- the project's scope root: the server's configured workdir
//     (else the process CWD) — the same single-workspace translation every
//     workspace-resolving sibling uses (GET /vcs, GET /vcs/diff,
//     GET /session/{id}/diff). The column set has no per-project path, so the
//     truthful answer for each registered project is the workspace the
//     runtime serves.
//   - time      <- {created, updated} <- created_at (Unix seconds). The
//     runtime keeps no separate updated timestamp, so updated truthfully
//     repeats created (the schema requires both).
//   - sandboxes <- [] — the runtime provisions no sandboxes; the field is
//     schema-required and the empty array is the truthful value.
//
// Optional selectors (?directory=, ?workspace=) are accepted but do not
// narrow the result: the runtime has exactly one workspace (the workdir
// above) and no per-project directory/workspace columns to filter on, and
// neither value is a project id. The declared error vocabulary is 400 only —
// there is no declared 404/5xx — so a store read failure answers 400 with the
// sibling INVALID_REQUEST envelope (the same arm sessionDiff uses for its
// store reads) and never a 501. A store with NO projects table (a database
// predating migration 014) answers the declared empty array — a list with no
// rows is the honest translation there. Every other method on the mount keeps
// the pre-existing 501 stub (405 is not in the declared response set).
func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		// Not a declared operation for this mount; 405 is not in the declared
		// response set — keep the pre-existing stub answer byte-identical.
		s.handleProjectVCSSStub(w, r)
		return
	}

	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT id, name, created_at FROM projects ORDER BY created_at, id`)
	if err != nil {
		// The declared vocabulary carries 400 only; a client must not have to
		// distinguish an undeclared 503 from a real store failure. A store
		// whose projects table is absent (pre-014 database) is also the
		// declared empty list — the runtime simply has no registered projects.
		if strings.Contains(strings.ToLower(err.Error()), "no such table: projects") {
			writeJSON(w, []any{})
			return
		}
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"failed to read projects: "+err.Error())
		return
	}

	projects := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		created := projectTimeSeconds(row["created_at"])
		projects = append(projects, map[string]any{
			"id":        toString(row["id"]),
			"worktree":  s.workspaceDir(),
			"name":      toString(row["name"]),
			"time":      map[string]any{"created": created, "updated": created},
			"sandboxes": []any{},
		})
	}
	writeJSON(w, projects)
}

// projectTimeSeconds reads a projects.created_at cell as Unix seconds. The
// column is declared TIMESTAMPTZ in migrations 014/015, but SQLite type
// affinity does not force every writer to integers — a live probe stored the
// cell as TEXT ("1791098154") through strftime('%s','now'). The declared
// ProjectTime fields are integers (minimum 0), so numeric strings are parsed
// and anything unreadable answers 0 rather than a fabricated stamp.
func projectTimeSeconds(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			return 0
		}
		if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
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
			// ROUTE-FIX-006: POST /project/git/init serves real data and
			// performs a real workspace mutation, so it keeps api-key auth
			// like every other implemented route — the stub exemption was
			// justified by "NOT_IMPLEMENTED with zero data, nothing to
			// protect", which no longer holds for this sub-path.
			if path == "/project/git/init" {
				return false
			}
			return true
		}
	}
	return false
}

// handleProjectVCSSStub returns 501 for /project and /vcs paths (opencode-specific).
func (s *Server) handleProjectVCSSStub(w http.ResponseWriter, r *http.Request) {
	// SHIM-GAP-002: the declared /vcs sub-operations answer the typed
	// not-implemented envelope naming their operation id (SHIM-DRIFT-142..144)
	// instead of the untyped stub body.
	if op, ok := vcsDeclaredOps[r.URL.Path]; ok {
		writeNotImplemented(w, r, op,
			fmt.Sprintf("%s is not implemented: VCS writes and raw/whole-status views are opencode-specific; the shim serves GET /vcs (Vcs.Info) and GET /vcs/diff (FileDiff[]), or the native tool API", op))
		return
	}

	name := "project"
	if strings.Contains(r.URL.Path, "vcs") {
		name = "VCS"
	}
	writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		fmt.Sprintf("%s is opencode-specific, not supported by Consensus shim; use native tool API", name))
}

// vcsDeclaredOps maps the declared /vcs/* sub-paths the shim does not
// translate to their upstream operation ids (SHIM-GAP-002). GET /vcs and
// GET /vcs/diff are real compatibility routes (DF-CONSENSUS-38) and are not
// in this table.
var vcsDeclaredOps = map[string]string{
	"/vcs/apply":    "vcs.apply",
	"/vcs/diff/raw": "vcs.diff.raw",
	"/vcs/status":   "vcs.status",
}

// projectDeclaredSubpaths maps the literal /project sub-paths the pinned
// upstream document declares and the shim does not translate to their
// operation ids (SHIM-GAP-002). These are not project ids: without this table
// they were parsed as one and answered with the upstream ProjectNotFoundError,
// telling the client a *project named "current"* did not exist instead of
// saying the operation is not implemented. GET /project/current left the table
// in ROUTE-FIX-005 (projectCurrent serves the declared contract) and
// POST /project/git/init left it in ROUTE-FIX-006 (projectInitGit); the table
// now covers only the operations the shim does not translate.
var projectDeclaredSubpaths = map[string]string{}

// handleProjectByID serves /project/{projectID} sub-paths. Consensus has no
// project registry, so every project id is unknown and the upstream opencode
// contract (httpapi-instance.test.ts "returns typed not found bodies for
// missing projects", DF-CONSENSUS-47) expects HTTP 404 with the typed
// ProjectNotFoundError NamedError body — exactly
// {_tag, projectID, message}, no extra fields. Bare GET /project keeps the
// 501 stub (handleProjectVCSSStub).
//
// SHIM-GAP-002 carves out the sub-paths the upstream document declares
// as operations rather than ids — /project/current (project.current),
// /project/git/init (project.initGit) and /project/{projectID}/directories
// (project.directories). ROUTE-FIX-005 serves GET /project/current,
// ROUTE-FIX-006 serves POST /project/git/init and ROUTE-FIX-007 serves GET
// /project/{projectID}/directories. The bare /project/{projectID} shape keeps
// the typed 404 untouched.
func (s *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimPrefix(r.URL.Path, "/project/")

	// ROUTE-FIX-007: project.directories is a real singleton-workspace
	// translation. Only the workspace's project id can resolve; optional
	// directory/workspace selectors are accepted when nonblank, matching the
	// fixed-workspace behavior of project.current and project.initGit.
	if dirsID, ok := strings.CutSuffix(projectID, "/directories"); ok && dirsID != "" {
		if r.Method != http.MethodGet {
			writeNotImplemented(w, r, "project.directories",
				"project.directories is declared as a GET operation; the shim translates GET only")
			return
		}
		if dirsID != instanceID(s.workspaceDir()) {
			writeOpencodeBadRequest(w, r, "Payload", "unknown projectID")
			return
		}
		for _, key := range []string{"directory", "workspace"} {
			if value, present := r.URL.Query()[key]; present && (len(value) == 0 || strings.TrimSpace(value[0]) == "") {
				writeOpencodeBadRequest(w, r, "Query", key+" must not be blank when provided")
				return
			}
		}
		writeJSON(w, []map[string]string{{"directory": s.workspaceDir()}})
		return
	}

	// ROUTE-FIX-005 / SHIM-DRIFT-099: the upstream project.current operation
	// (declared responses 200 Project, 400) was served by the typed
	// not-implemented envelope from the projectDeclaredSubpaths table. Serve
	// the declared contract truthfully; non-GET keeps the typed 501 (405 is
	// not in the declared set).
	if projectID == "current" {
		if r.Method == http.MethodGet {
			s.projectCurrent(w, r)
			return
		}
		writeNotImplemented(w, r, "project.current",
			"project.current is declared as a GET operation; the shim translates GET only")
		return
	}
	if projectID == "git/init" {
		if r.Method == http.MethodPost {
			s.projectInitGit(w, r)
			return
		}
		writeNotImplemented(w, r, "project.initGit",
			"project.initGit is declared as a POST operation; the shim translates POST only")
		return
	}
	if op, ok := projectDeclaredSubpaths[projectID]; ok {
		writeNotImplemented(w, r, op,
			fmt.Sprintf("%s is not implemented: Consensus keeps no project registry, so there is no project record to resolve; use GET /instance and GET /path for the workspace and the native API for work", op))
		return
	}
	if dirsID, ok := strings.CutSuffix(projectID, "/directories"); ok && dirsID != "" {
		writeNotImplemented(w, r, "project.directories",
			"project.directories is not implemented: Consensus keeps no project registry and therefore no per-project directory list; use GET /path or GET /find?pattern= for workspace paths")
		return
	}

	if projectID == "" {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
		return
	}
	writeOpencodeNotFoundError(w, "ProjectNotFoundError", "projectID", projectID,
		"Project not found: "+projectID)
}

// writeOpencodeNotFoundError emits an upstream-shaped typed not-found body:
// {_tag: <tag>, <idField>: <id>, message: <message>} with a trailing newline
// (json.Encoder), mirroring writeUpstreamRequestNotFound.
func writeOpencodeNotFoundError(w http.ResponseWriter, tag, idField, id, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	body := map[string]any{
		"_tag":    tag,
		idField:   id,
		"message": message,
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("opencode-shim: failed to encode typed not-found body", "tag", tag)
	}
}

// ch:trace row=ROUTE-FIX-006 spec=specs/openapi/upstream/openapi-1.18.33.json#project.initGit test=TestProjectInitGitServesDeclared200 doc=docs/evidence/ROUTE-FIX-006-live-probe.md evidence=docs/evidence/ROUTE-FIX-006-live-probe.md witness=none:unattended-worker-session
//
// projectInitGit serves POST /project/git/init — the upstream project.initGit
// operation (ROUTE-FIX-006 board row, source item SHIM-DRIFT-100; declared
// responses: 200 Project, 400 BadRequest).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/project/git/init".post): the optional query selectors are directory
// and workspace; the declared 200 body is the refreshed Project object
// ("Create a git repository for the current project and return the refreshed
// project info"), and the declared error is 400 BadRequestError. Before this
// change the sub-path sat in projectDeclaredSubpaths and answered the typed
// not-implemented envelope for EVERY request — the declared 200 was
// unreachable (SHIM-DRIFT-100, "declared 200,400, served 501").
//
// Translation: unlike session.init (which translates to a native turn), this
// operation's effect is a workspace filesystem effect the shim CAN perform for
// real — upstream runs `git init` in the project's worktree. The shim does the
// Consensus-native equivalent on the single real workspace the instance
// serves (the singleton convention every /instance/* and project.current
// translation already uses, resolved by x-opencode-directory): when the
// workspace is not yet a git repository, `git init` runs in it via runGit
// (the gitEnv-stripped, ctx-bounded git helper all workspace git reads use);
// when it already is one, git init is idempotent upstream ("reinitialize" is
// not an error), so the handler skips the subprocess and reports the same
// success — the observable project state is identical. The declared Project
// body is then derived exactly the way projectCurrent derives it (same fields,
// same sources, nothing invented), which IS the "refreshed project info" the
// operation describes.
//
// Validation order and the declared error arms (the sibling projectCurrent
// convention; the operation declares no requestBody, so the query selectors
// are the client-input surface that keeps 400 reachable):
//   - a present-but-blank declared query selector (directory, workspace) → the
//     declared 400 naming the parameter, answered with the declared
//     BadRequestError envelope (writeOpencodeBadRequest, kind "Query" — the
//     instanceDispose blank-query convention). Validation runs before any git
//     call so a malformed request never touches the workspace;
//   - `git init` fails (git absent, workspace not writable) → the declared
//     400 INVALID_REQUEST naming the reason, never an undeclared 5xx and
//     never a fabricated success.
func (s *Server) projectInitGit(w http.ResponseWriter, r *http.Request) {
	// 1. A present-but-blank declared query selector is the declared 400.
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	// 2. The single workspace this instance serves.
	dir := s.requestWorkspaceDir(r)
	ctx := r.Context()

	// 3. The real effect: initialize git in the workspace when it is not one
	// already. runGit resolves the repo from dir alone (GIT_* env stripped),
	// so the init cannot leak into an ambient repository.
	if inside, err := runGit(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || inside != "true" {
		if out, err := runGit(ctx, dir, "init"); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"could not initialize git in the workspace: "+err.Error()+" "+out)
			return
		}
	}

	// 4. Declared 200 body: the refreshed Project, derived from the real
	// workspace exactly as project.current derives it (shared translation,
	// nothing invented).
	project, err := s.deriveProject(ctx, dir)
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"could not read the project timestamps: "+err.Error())
		return
	}
	writeJSON(w, project)
}

// deriveProject builds the declared upstream Project body from the single
// real workspace this instance serves — the shared translation of
// project.current (ROUTE-FIX-005) and project.initGit (ROUTE-FIX-006):
//   - id        <- instanceID(dir), the short-sha256 workspace id the
//     singleton GET /instance entry already reports;
//   - worktree  <- gitWorktree(dir): the repository top-level when the
//     workspace is a git repo, the directory itself otherwise;
//   - name      <- the workspace directory's base name;
//   - vcs       <- "git" only when `git rev-parse --is-inside-work-tree`
//     succeeds in the workspace (declared enum ["git"]; absent otherwise);
//   - commands  <- the workspace's consensus.json "commands" key when present
//     and an object; absent otherwise (optional field, no file → no claim);
//   - time      <- {created, updated} from the schema_versions ledger in
//     milliseconds since the epoch (the ProjectTime unit);
//   - sandboxes <- [] (required key): the runtime spawns no sandboxes.
//     icon is omitted (optional; nothing asserts it).
//
// An unreadable ledger returns an error — both callers answer the declared
// 400 (neither operation declares a 5xx), never a fabricated timestamp.
func (s *Server) deriveProject(ctx context.Context, dir string) (map[string]any, error) {
	project := map[string]any{
		"id":       instanceID(dir),
		"worktree": gitWorktree(ctx, dir, dir),
		"name":     filepath.Base(dir),
		// Required key; the runtime spawns no sandboxes.
		"sandboxes": []any{},
	}
	if inside, err := runGit(ctx, dir, "rev-parse", "--is-inside-work-tree"); err == nil && inside == "true" {
		project["vcs"] = "git"
	}
	if cfg := readWorkspaceCommands(dir); cfg != nil {
		project["commands"] = cfg
	}
	created, updated, err := s.migrationLedgerBounds(ctx)
	if err != nil {
		return nil, err
	}
	project["time"] = map[string]any{"created": created, "updated": updated}
	return project, nil
}

// projectCurrent serves GET /project/current — the upstream project.current
// operation (ROUTE-FIX-005 board row, source item SHIM-DRIFT-099; declared
// responses: 200 Project, 400 BadRequest).
//
// ch:trace row=ROUTE-FIX-005 spec=specs/openapi/upstream/openapi-1.18.33.json#project.current test=TestProjectCurrentServesDeclared200 doc=docs/evidence/ROUTE-FIX-005-live-probe.md evidence=docs/evidence/ROUTE-FIX-005-live-probe.md witness=none:unattended-worker-session
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/project/current".get): the optional query selectors are directory
// and workspace; the declared 200 body is a single Project object (required
// id, worktree, time, sandboxes; additionalProperties: false), described as
// "Retrieve the currently active project that OpenCode is working with."
// Before this change the sub-path was in projectDeclaredSubpaths and answered
// the typed not-implemented envelope for EVERY request — the declared 200 was
// unreachable and a client could not tell "this shim does not implement it"
// from "here is the current project".
//
// Translation: the runtime keeps exactly one workspace per instance (the
// singleton-instance convention every /instance/* translation already
// serves): the x-opencode-directory header when the request carries one
// (upstream fixed-workspace semantics, the requestWorkspaceDir convention of
// /path and /vcs), else the server workspace. The declared Project fields are
// all derived from that real workspace directory — nothing is invented:
//
//   - id        <- instanceID(dir), the short-sha256 workspace id the
//     singleton GET /instance entry already reports (a stable per-workspace
//     identity, never a fabricated registry key);
//   - worktree  <- gitWorktree(dir): the repository top-level when the
//     workspace is a git repo, the directory itself otherwise — exactly the
//     value GET /path reports as "worktree";
//   - name      <- the workspace directory's base name (the human label
//     upstream shows for a project);
//   - vcs       <- "git" only when `git rev-parse --is-inside-work-tree`
//     succeeds in the workspace (declared enum ["git"]; absent otherwise —
//     the field is optional and never asserted without evidence);
//   - commands  <- {} read from the workspace's consensus.json "commands"
//     key when present and an object; absent otherwise (optional field, no
//     file → no claim);
//   - time      <- {created, updated} from the schema_versions ledger
//     (internal/migrate bootstrapSQL): created = the earliest applied_at
//     (the workspace was initialized then), updated = the latest applied_at
//     (the schema — the project state the runtime tracks — was last touched
//     then). Milliseconds since the epoch, the unit the upstream
//     ProjectTime schema declares (integer, minimum 0). A ledger that cannot
//     be read answers 400 INVALID_REQUEST (the declared error arm), never an
//     undeclared 5xx and never a fabricated timestamp; the ledger table
//     always exists because the server auto-migrates on boot;
//   - sandboxes <- [] (required key): the runtime spawns no sandboxes.
//     icon is omitted (optional; nothing asserts it).
//
// Validation order and the declared error arms:
//   - a present-but-blank declared query selector (directory, workspace) → the
//     declared 400 INVALID_REQUEST naming the parameter. The operation
//     declares no requestBody, so the selectors are what keeps the declared
//     400 arm reachable (the sessionTodo / sync.steal precedent:
//     a present-but-blank declared query parameter is a contract violation,
//     not an absent one). A well-formed selector is accepted and does not
//     change the answer — the runtime keeps one workspace per instance, so
//     neither selector selects a different project (the sibling sessionDiff
//     note);
//   - the migration ledger cannot be read → the declared 400
//     INVALID_REQUEST naming the reason (the sessionTodo convention for an
//     unreadable store; the operation declares no 5xx and no 404).
func (s *Server) projectCurrent(w http.ResponseWriter, r *http.Request) {
	// 1. A present-but-blank declared query selector is the declared 400 (this
	// operation declares no requestBody, so this is what keeps the 400 arm
	// reachable). Validation runs before any store read so a malformed request
	// never reaches the database.
	for _, param := range []string{"directory", "workspace"} {
		values, present := r.URL.Query()[param]
		if !present {
			continue
		}
		value := ""
		if len(values) > 0 {
			value = strings.TrimSpace(values[0])
		}
		if value == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	// 2. The single workspace this instance serves, and the declared Project
	// derived from it (shared deriveProject translation — see above).
	project, err := s.deriveProject(r.Context(), s.requestWorkspaceDir(r))
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"could not read the project timestamps: "+err.Error())
		return
	}

	writeJSON(w, project)
}

// readWorkspaceCommands reads the workspace's consensus.json "commands" object
// when one exists (declared Project.commands translation). Returns nil when
// the file is absent, unreadable, or does not carry an object under
// "commands" — an optional field is never synthesized.
func readWorkspaceCommands(dir string) map[string]any {
	raw, err := os.ReadFile(filepath.Join(dir, "consensus.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		Commands map[string]any `json:"commands"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.Commands == nil {
		return nil
	}
	return cfg.Commands
}

// migrationLedgerBounds returns (created, updated) in milliseconds since the
// epoch from the schema_versions ledger: the earliest and the latest
// applied_at RFC3339 timestamps. Errors surface to the caller (declared 400)
// instead of being swallowed into a fabricated timestamp.
func (s *Server) migrationLedgerBounds(ctx context.Context) (int64, int64, error) {
	rows, err := s.db.Query(ctx, `SELECT applied_at FROM schema_versions`)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, fmt.Errorf("the schema_versions ledger holds no rows")
	}
	var oldest, newest int64
	for i, row := range rows {
		ts := toString(row["applied_at"])
		if ts == "" {
			return 0, 0, fmt.Errorf("schema_versions row %d carries a blank applied_at", i+1)
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return 0, 0, fmt.Errorf("schema_versions row %d applied_at %q is not RFC3339", i+1, ts)
		}
		ms := t.UnixMilli()
		if i == 0 || ms < oldest {
			oldest = ms
		}
		if i == 0 || ms > newest {
			newest = ms
		}
	}
	return oldest, newest, nil
}

func (s *Server) handlePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	s.instancePath(w, r)
}

// handleVCS serves the fixed-workspace VCS read compatibility routes
// (DF-CONSENSUS-38, upstream httpapi-instance.test.ts "serves path and VCS
// read endpoints"): bare GET /vcs → opencode Vcs.Info and GET /vcs/diff →
// opencode Vcs.FileDiff[] — the same translation as /instance/vcs and
// /instance/vcs/diff (SPEC-017 §3.10). The upstream pinned suite probes these
// as top-level paths with x-opencode-directory and expects 200. Non-GET
// methods and /vcs/* sub-paths stay on the 501 stub (SPEC-017 §3.9).
func (s *Server) handleVCS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	if r.URL.Path == "/vcs/diff" {
		s.instanceVCSDiff(w, r)
		return
	}
	s.instanceVCS(w, r)
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	var req struct {
		Service string         `json:"service"`
		Level   string         `json:"level"`
		Message string         `json:"message"`
		Extra   map[string]any `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed log body")
		return
	}
	if req.Service == "" || req.Message == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "service and message are required")
		return
	}
	if req.Level != "debug" && req.Level != "info" && req.Level != "warn" && req.Level != "error" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "level must be debug, info, warn, or error")
		return
	}
	writeJSON(w, true)
}

// handleQuestionList serves GET /question — upstream question.list
// (ROUTE-ADD-110, SHIM-DRIFT-113, declared responses: 200
// QuestionRequest[] "List of pending questions", 400 BadRequestError).
// Previously the path had no route at all and net/http answered the default
// 404 (the artifact's class NOT-SERVED).
//
// QuestionRequest requires id (pattern ^que), sessionID (pattern ^ses) and a
// questions array of QuestionInfo (multiple-choice prompts with labeled
// options — openapi-1.18.33.json components.schemas.QuestionRequest). The
// shim has no producer for that shape: Consensus's human-input surface is
// approval_requests (request_type/risk_level vocabulary, SPEC-014), an
// approval row cannot be truthfully reshaped into a QuestionRequest (no
// header/options, no ^que ids), and nothing in the shim or native API writes
// question rows. The handler therefore reads the pending-approval store —
// truthfully answering "no pending questions" with the declared empty list —
// and never fabricates entries. The declared 400 arm is answered the way
// sibling handlers do (writeOpencodeError INVALID_REQUEST) when the store
// cannot be read; 405 is not part of this operation's declared set, but the
// method guard keeps the sibling METHOD_NOT_ALLOWED envelope instead of
// silently reading the store.
func (s *Server) handleQuestionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	ctx := r.Context()
	_, err := s.db.Query(ctx,
		`SELECT ar.id, ar.session_id, ar.request_type, ar.risk_level, ar.description, ar.status, ar.created_at
		 FROM approval_requests ar WHERE ar.status = 'pending'`)
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to list pending questions")
		return
	}
	// Store read succeeded: no pending question requests exist in a form the
	// upstream contract can carry, so the truthful answer is the empty list
	// (QuestionRequest[] — an array, never null).
	writeJSON(w, []any{})
}

func (s *Server) handleQuestionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/question/")
	parts := strings.Split(path, "/")
	if r.Method != http.MethodPost || len(parts) != 2 || (parts[1] != "reply" && parts[1] != "reject") {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown question action")
		return
	}
	requestID := parts[0]
	if !strings.HasPrefix(requestID, "que") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid question request id")
		return
	}
	writeUpstreamRequestNotFound(w, "QuestionNotFoundError", requestID, "Question request not found: "+requestID)
}

func writeUpstreamRequestNotFound(w http.ResponseWriter, tag, requestID, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"_tag":      tag,
		"requestID": requestID,
		"message":   message,
	})
}

// ============================================================================
// Instance Endpoints (SPEC-017 §3.10) — opencode /instance/* translation
// ============================================================================
//
// The opencode server protocol (anomalyco/opencode httpapi-instance.test.ts) probes
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
// endpoints (/instance/path, /instance/vcs, /instance/vcs/diff), the
// instance.dispose operation (ROUTE-FIX-003), and the 501/404 convention for
// everything else.
func (s *Server) handleInstanceSub(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/instance/")
	switch {
	case sub == "dispose" && r.Method == http.MethodPost:
		// ROUTE-FIX-003 / SHIM-DRIFT-091: the upstream instance.dispose
		// operation (declared responses 200 boolean, 400 BadRequest) was
		// answered by the typed 501 stub below — a code the document does
		// not declare for the operation. Serve the declared contract
		// truthfully; non-POST falls through to the stub (501 is not a
		// declared response either, but it is the pre-change answer and no
		// declared code exists for a wrong method).
		s.instanceDispose(w, r)
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

// instancePath serves GET /instance/path and the fixed-workspace
// compatibility route GET /path → opencode PathInfo:
// {home, state, config, worktree, directory}.
func (s *Server) instancePath(w http.ResponseWriter, r *http.Request) {
	dir := s.requestWorkspaceDir(r)
	home, _ := os.UserHomeDir()
	writeJSON(w, map[string]any{
		"home":      home,
		"state":     filepath.Join(home, ".local", "state", "consensus"),
		"config":    filepath.Join(home, ".config", "consensus", "config.json"),
		"worktree":  gitWorktree(r.Context(), dir, dir),
		"directory": dir,
	})
}

// instanceVCS serves GET /instance/vcs and the fixed-workspace compatibility
// route GET /vcs (DF-CONSENSUS-38) → opencode Vcs.Info:
// {branch?, default_branch?}. The workspace is the x-opencode-directory
// header when present (upstream fixed-workspace semantics, same as
// instancePath), else the server workspace. Never errors — a non-git
// workspace returns {}.
func (s *Server) instanceVCS(w http.ResponseWriter, r *http.Request) {
	dir := s.requestWorkspaceDir(r)
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

// instanceVCSDiff serves GET /instance/vcs/diff and the fixed-workspace
// compatibility route GET /vcs/diff (DF-CONSENSUS-38) → opencode
// Array(Vcs.FileDiff): [{file, additions, deletions, status?}]. The workspace
// resolution follows instanceVCS (x-opencode-directory header first). patch is
// omitted (optional in the upstream schema). Never errors — a clean or
// non-git workspace returns [].
func (s *Server) instanceVCSDiff(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, gitFileDiffs(r.Context(), s.requestWorkspaceDir(r)))
}

// ch:trace row=ROUTE-FIX-003 spec=specs/openapi/upstream/openapi-1.18.33.json#instance.dispose test=TestInstanceDisposeAnswersDeclaredBoolean doc=docs/evidence/ROUTE-FIX-003-live-probe.md evidence=docs/evidence/ROUTE-FIX-003-live-probe.md witness=none:unattended-worker-session
// instanceDispose serves POST /instance/dispose — upstream instance.dispose
// (ROUTE-FIX-003, SHIM-DRIFT-091, declared responses: 200 boolean, 400
// BadRequest). Upstream "clean up and dispose the current OpenCode instance,
// releasing all resources"; the shim is ONE instance rooted at the workspace
// directory and keeps no per-instance registry to release — the Consensus
// server IS the singleton, and disposing it is not something an HTTP request
// to a compatibility shim may do. The request is therefore honored as a
// successful no-op answering the declared boolean true, the sibling
// handleGlobalDispose convention (idempotent for a repeated POST).
//
// The operation declares directory/workspace query selectors for workspace
// scoping; the no-op answer is workspace-independent, so a valued param is
// well-formed and scopes nothing, while a present-but-blank one is malformed
// input and answers the declared 400 via writeOpencodeBadRequest (the
// upstream v2 SDK NamedError envelope, kind "Query") — the
// handleSkill/handleFormatter blank query convention with this operation's
// declared body shape. The operation declares no requestBody, so none is
// read. Non-POST never reaches this handler (the dispatch case above is
// POST-only and falls through to the 501 stub).
func (s *Server) instanceDispose(w http.ResponseWriter, r *http.Request) {
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, true)
}

// requestWorkspaceDir returns the workspace directory a request operates on:
// an explicitly configured workdir or the process CWD, overridden by the
// upstream fixed-workspace selector header x-opencode-directory when the
// request carries one (DF-CONSENSUS-38: the pinned upstream suite creates a
// git workspace in a temp dir and addresses it by header, so VCS reads must
// resolve it like /path does).
func (s *Server) requestWorkspaceDir(r *http.Request) string {
	if r != nil {
		if requested := strings.TrimSpace(r.Header.Get("x-opencode-directory")); requested != "" {
			return filepath.Clean(requested)
		}
	}
	return s.workspaceDir()
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

// gitEnv returns the process environment with git-repository location
// variables stripped. Pre-commit hooks (gitreins) export GIT_DIR /
// GIT_INDEX_FILE into their children, and any test or tool process that
// shells out to git for a scratch fixture repo inherits them — the fixture
// call then resolves to the repo being committed instead of the -C target
// (DF-CONSENSUS-19 commit incident, 2026-09-25). Stripping the vars makes
// runGit/execGitStatus resolve the repo from dir/cwd alone, and makes the
// shim's workspace git calls immune to a caller's leaked git env.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="),
			strings.HasPrefix(kv, "GIT_INDEX_FILE="),
			strings.HasPrefix(kv, "GIT_OBJECT_DIRECTORY="),
			strings.HasPrefix(kv, "GIT_ALTERNATE_OBJECT_DIRECTORIES="):
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runGit runs git -C dir <args...> and returns trimmed stdout; errors are
// returned so callers can fall back to neutral shapes (never fatal).
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
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
				if adds, dels, ok := gitUntrackedStat(ctx, dir, path); ok {
					entry["additions"] = adds
					entry["deletions"] = dels
				}
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

// gitUntrackedStat counts an untracked file's additions/deletions the same
// way the pinned upstream opencode server does (DF-CONSENSUS-38):
// `git diff --no-index --numstat -- /dev/null <file>` — whole lines, so a
// file without a trailing newline still counts its final line. ok is false
// when git fails or the file is binary ("-" columns); the caller keeps 0s.
func gitUntrackedStat(ctx context.Context, dir, file string) (adds, dels int, ok bool) {
	out, _ := runGit(ctx, dir, "diff", "--no-index", "--numstat", "--", "/dev/null", file)
	// --no-index exits 1 when the compared entries differ; the output is
	// still valid, so only empty output means no stat.
	if out == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(out, "	", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	if parts[0] == "-" || parts[1] == "-" {
		return 0, 0, false
	}
	adds, errA := strconv.Atoi(parts[0])
	dels, errD := strconv.Atoi(parts[1])
	if errA != nil || errD != nil {
		return 0, 0, false
	}
	return adds, dels, true
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

	// SPEC-017 §3.2 requires this synchronous endpoint to return the response
	// produced for this turn. A shim without the native service cannot satisfy
	// that contract; fail loudly instead of returning a fabricated acknowledgement.
	if s.svc == nil {
		writeOpencodeError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "agent response service is unavailable")
		return
	}

	timeout := s.messageResponseTimeout
	if timeout <= 0 {
		timeout = defaultMessageResponseTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := s.svc.SendMessage(ctx, MessageSendInput{
		SessionID: sessionID,
		Content:   content,
		MsgType:   "user_instruction",
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeOpencodeError(w, r, http.StatusGatewayTimeout, "RESPONSE_TIMEOUT", "agent response was not produced before the request timeout")
		case errors.Is(err, context.Canceled):
			writeOpencodeError(w, r, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled before an agent response was produced")
		default:
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to send message: "+err.Error())
		}
		return
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		writeOpencodeError(w, r, http.StatusBadGateway, "EMPTY_RESPONSE", "agent turn completed but no response was produced")
		return
	}

	writeJSON(w, s.buildAssistantMessage(result.Content))
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

		// OpenCode v1.18.29 attach reads provider_default before rendering the
		// TUI. Keep the mapping present even when no default is configured, and
		// derive Consensus's entry from the same setting returned above.
		providerDefaults := make(map[string]string)
		if defaultModel := toString(settings["llm.default_model"]); defaultModel != "" {
			providerDefaults["consensus"] = defaultModel
		}
		writeJSON(w, map[string]any{
			"settings":         settings,
			"provider_default": providerDefaults,
		})
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

// handleGlobalConfig serves the upstream /global/config surface — GET
// global.config.get and PATCH global.config.update (ROUTE-ADD-090,
// SHIM-DRIFT-088; the declared GET sibling is SHIM-DRIFT-087). Declared
// responses for both operations: 200 Config, 400 BadRequest |
// InvalidRequestError (specs/openapi/upstream/openapi-1.18.33.json, pinned by
// specs/openapi/opencode-pin.yaml).
//
// Before this change the path was not registered at all, so both operations
// fell through to net/http's default 404 — the drift artifact's NOT-SERVED
// class, which a client cannot tell apart from "no such opencode operation".
// Registering the exact path routes EVERY method, so this one handler serves
// both declared operations: answering 405 for the GET arm would reclassify
// SHIM-DRIFT-087 as METHOD-MISSING (the dishonest class the declared-vs-served
// contract SHIM-GAP-002 exists to empty — a registered route may not lie with
// 404/405), and a 501 would leave a declared operation unserved for no reason
// when the shim can answer it truthfully.
//
// The runtime keeps no opencode global-config store — Consensus configuration
// lives in system_settings and is served, as a different surface, by the
// shim's own /config route — so:
//
//	GET   answers the current global config as the empty Config document {}
//	      (never null: the document declares an object). The payload is not
//	      derived from system_settings because those keys are Consensus
//	      settings, not opencode Config properties.
//	PATCH validates the optional Config body against the declared surface and
//	      answers 200 with the resulting Config document (the accepted patch
//	      applied to the empty current config = the patch itself). It is NOT
//	      persisted: there is no store to persist it to, and inventing one
//	      would fabricate state the runtime does not have. Same honesty shape
//	      as the sibling stub routes (e.g. /skill answers the declared empty
//	      array).
//
// The declared 400 arm answers the sibling writeOpencodeError INVALID_REQUEST
// envelope for malformed input: a body that is not JSON, that is not a JSON
// object (arrays, scalars and an explicit null are all not Config documents),
// or that carries a top-level key the Config schema does not declare — the
// pinned schema sets additionalProperties:false, so an undeclared key is a
// contract violation, not an extension point. An ABSENT body is well-formed
// (the document does not mark requestBody required) and answers 200. Non
// GET/PATCH methods answer 405 METHOD_NOT_ALLOWED — sibling method-guard
// convention, matching handleConfig.
func (s *Server) handleGlobalConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// The shim's global config is empty: it keeps no opencode Config store.
		writeJSON(w, map[string]any{})
		return
	case http.MethodPatch:
		var patch map[string]any
		if r.Body == nil || r.ContentLength == 0 {
			patch = map[string]any{}
		} else if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"malformed request body: "+err.Error())
			return
		}
		// A JSON null body decodes without error into a nil map (encoding/json
		// leaves the destination untouched) — still not a Config document.
		if patch == nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"request body must be a JSON object matching the Config schema")
			return
		}
		for key := range patch {
			if _, ok := globalConfigDeclaredKeys[key]; !ok {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("unknown config key %q: the Config schema declares no such property", key))
				return
			}
		}
		// The current global config is empty, so the resulting config IS the
		// accepted patch — a Config document (never null, never an envelope).
		writeJSON(w, patch)
		return
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PATCH")
	}
}

// globalConfigDeclaredKeys is the declared top-level property surface of the
// upstream Config schema (specs/openapi/upstream/openapi-1.18.33.json,
// components.schemas.Config, additionalProperties:false). It is asserted
// against that pinned document by TestGlobalConfigDeclaredKeysMatchPinnedSchema,
// so the list cannot silently drift from the contract this route answers.
var globalConfigDeclaredKeys = map[string]struct{}{
	"$schema":            {},
	"agent":              {},
	"attachment":         {},
	"autoshare":          {},
	"autoupdate":         {},
	"command":            {},
	"compaction":         {},
	"default_agent":      {},
	"disabled_providers": {},
	"enabled_providers":  {},
	"enterprise":         {},
	"experimental":       {},
	"formatter":          {},
	"instructions":       {},
	"layout":             {},
	"logLevel":           {},
	"lsp":                {},
	"mcp":                {},
	"mode":               {},
	"model":              {},
	"permission":         {},
	"plugin":             {},
	"provider":           {},
	"reference":          {},
	"references":         {},
	"server":             {},
	"share":              {},
	"shell":              {},
	"skills":             {},
	"small_model":        {},
	"snapshot":           {},
	"subagent_depth":     {},
	"tool_output":        {},
	"tools":              {},
	"username":           {},
	"watcher":            {},
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

// handleProviderAuth serves GET /provider/auth — upstream provider.auth
// (ROUTE-ADD-099, SHIM-DRIFT-102, declared responses: 200 map of providerID
// to ProviderAuthMethod[], 400 BadRequest). Upstream returns the auth methods
// available per AI provider ("Retrieve available authentication methods for
// all AI providers").
//
// The shim answers truthfully from the auth rows PUT /auth/{providerID}
// stores in system_settings (keys auth.<providerID>.<field> — SPEC-017 §3.2):
// every provider holding stored auth reports one {"type":"api"} method whose
// prompts[] carries the required key prompt (type/key/message), mirroring
// the stored-credential reality. An empty store answers {} — the contract
// declares an object, never null. The declared 400 arm is answered via the
// sibling writeOpencodeError INVALID_REQUEST envelope when the store cannot
// be read. Non-GET keeps the generic 404 (405 is not part of the declared
// response set).
//
// The method list itself comes from storedProviderAuthMethods — the same
// derivation POST /provider/{providerID}/oauth/authorize validates its
// `method` index against, so the methods the shim advertises and the methods
// it accepts cannot drift apart.
func (s *Server) handleProviderAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
		return
	}
	methods, err := s.storedProviderAuthMethods(r.Context())
	if err != nil {
		// The document's only error response for this operation is 400
		// BadRequest; answer it the way sibling handlers do instead of
		// surfacing an undeclared 500.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to read provider auth methods")
		return
	}
	writeJSON(w, methods)
}

// storedProviderAuthMethods derives the auth methods the shim advertises for
// every provider that holds stored credentials: one {"type":"api"} method per
// provider, keyed on the auth.<providerID>.<field> rows PUT /auth/{providerID}
// writes into system_settings (SPEC-017 §3.2 — the same store DELETE
// /auth/{providerID} clears). This is the shim's whole provider-method
// registry: it has no plugin/OAuth hook table, so the stored credentials are
// the only evidence a provider exists at all.
//
// It is the single derivation behind both GET /provider/auth
// (handleProviderAuth) and POST /provider/{providerID}/oauth/authorize
// (handleProviderOAuthAuthorize), so the advertised methods and the methods
// authorize accepts can never disagree.
func (s *Server) storedProviderAuthMethods(ctx context.Context) (map[string][]map[string]any, error) {
	// The LIKE pattern is a constant — the provider id is recovered from the
	// stored key below, never spliced into the query, so no escaping is
	// needed (contrast handleAuthDelete, which interpolates a user id).
	rows, err := s.db.Query(ctx,
		`SELECT key FROM system_settings WHERE key LIKE 'auth.%.%' ORDER BY key`)
	if err != nil {
		return nil, err
	}

	methods := make(map[string][]map[string]any, len(rows))
	for _, row := range rows {
		key := toString(row["key"])
		providerID := strings.TrimPrefix(key, "auth.")
		if i := strings.Index(providerID, "."); i > 0 {
			providerID = providerID[:i]
		}
		if providerID == "" {
			continue
		}
		if _, ok := methods[providerID]; ok {
			continue
		}
		methods[providerID] = []map[string]any{advertisedProviderAuthMethod("api", providerID)}
	}
	return methods, nil
}

// advertisedProviderAuthMethod is one ProviderAuthMethod the shim advertises:
// the kind ("api" here — the shim stores API keys, it never runs an OAuth
// flow), a label, and the key prompt required by the upstream Prompt schema
// (type/key/message).
func advertisedProviderAuthMethod(kind, providerID string) map[string]any {
	return map[string]any{
		"type":  kind,
		"label": "API key",
		"prompts": []map[string]any{
			{"type": "text", "key": "api_key", "message": "Enter the API key for " + providerID},
		},
	}
}

// handleProviderOAuthAuthorize serves POST /provider/{providerID}/oauth/authorize
// — upstream provider.oauth.authorize (ROUTE-ADD-100, SHIM-DRIFT-103, declared
// responses: 200 ProviderAuthAuthorization, 400 ProviderAuthError |
// InvalidRequestError). Upstream starts the OAuth authorization flow for one
// provider and answers the authorization URL plus the flow method ("Start OAuth
// authorization").
//
// The shim has no OAuth implementation and advertises no oauth method: its
// provider-method registry is derived from the stored credentials
// (storedProviderAuthMethods) and every advertised method is type "api". The
// pinned upstream runtime does exactly one thing with a method whose type is
// not "oauth" — ProviderAuth.authorize returns early
// (`if (method.type !== "oauth") return`), and the route handler serializes
// that absent result as JSON null rather than an empty body, so a client can
// still .json()-parse the response
// (packages/opencode/src/server/routes/instance/httpapi/handlers/provider.ts
// "authorizeRaw", over packages/opencode/src/provider/auth.ts, at the pinned
// commit 7945de208964a49300d7f770d1a71d078db9a4c4 / v1.18.33). The shim
// mirrors that branch: an advertised (necessarily non-oauth) method answers the
// declared 200 with a JSON null body. It never fabricates an authorization URL
// — there is no flow to start — and it never answers the pre-fix net/http
// default 404 for an operation it now serves.
//
// The declared 400 arm is answered with the document's own error shapes:
//
//   - a body that is absent, not JSON, not an object, whose declared `method`
//     field is missing or is not a non-negative integer, or whose optional
//     `inputs` field is not a map of strings answers ProviderAuthError
//     {"name":"BadRequest","data":{...}} — upstream maps a payload decode
//     failure onto the same name and data bag;
//   - a provider holding no stored credentials (so the shim advertises no
//     method for it) and a `method` index the provider does not advertise both
//     answer ProviderAuthError {"name":"BadRequest","data":{"providerID":...}}
//     — upstream indexes its hook table and faults on either; the shim answers
//     the declared code with a named reason instead;
//   - a store that cannot be read answers InvalidRequestError
//     {"_tag":"InvalidRequestError","message":...} — the document's second
//     declared 400 shape.
//
// Unlisted body fields are ignored, matching the shim's other body-taking
// handlers. Non-POST answers 405 METHOD_NOT_ALLOWED (the sibling POST-route
// method guard; 405 is not part of the declared response set, so it is never
// the answer to a well-formed POST).
func (s *Server) handleProviderOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	providerID := r.PathValue("providerID")
	if providerID == "" {
		writeProviderAuthError(w, "BadRequest", map[string]any{})
		return
	}
	methodIndex, reason, ok := parseProviderOAuthAuthorizeBody(r)
	if !ok {
		writeProviderAuthError(w, "BadRequest", map[string]any{"message": reason})
		return
	}

	methods, err := s.storedProviderAuthMethods(r.Context())
	if err != nil {
		// The document declares no 5xx for this operation; answer the declared
		// 400 with its second shape instead of surfacing an undeclared 500.
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"_tag":    "InvalidRequestError",
			"message": "failed to read provider auth methods",
		})
		return
	}

	advertised := methods[providerID]
	if len(advertised) == 0 {
		writeProviderAuthError(w, "BadRequest", map[string]any{
			"providerID": providerID,
			"message":    "no auth methods registered for provider " + providerID,
		})
		return
	}
	if methodIndex >= int64(len(advertised)) {
		writeProviderAuthError(w, "BadRequest", map[string]any{
			"providerID": providerID,
			"field":      "method",
			"message": fmt.Sprintf("auth method index %d is not advertised for provider %s (0-%d)",
				methodIndex, providerID, len(advertised)-1),
		})
		return
	}

	// The advertised method is an API-key method, never an OAuth method, so
	// there is no flow to start and no URL to hand back: mirror upstream's
	// non-oauth branch, which serializes the absent result as JSON null.
	writeJSON(w, nil)
}

// parseProviderOAuthAuthorizeBody decodes the declared request body
// ({method: <auth method index>, inputs?: {<prompt key>: <string>}} —
// components.schemas of the pinned opencode document). It returns the method
// index, a human-readable reason for a refusal, and whether the body is
// well-formed. The document does not mark requestBody required, but its only
// defined field (`method`) is required, so an absent or empty body cannot
// select a method and is refused — the same direction upstream's runtime takes
// when decoding an empty request text fails.
func parseProviderOAuthAuthorizeBody(r *http.Request) (int64, string, bool) {
	if r.Body == nil || r.ContentLength == 0 {
		return 0, "request body must carry the auth method index", false
	}
	var raw any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return 0, "malformed request body: " + err.Error(), false
	}
	// A JSON null body decodes without error into a nil interface, and an
	// array/scalar is not the declared object either.
	body, ok := raw.(map[string]any)
	if !ok {
		return 0, "request body must be a JSON object carrying the auth method index", false
	}
	value, present := body["method"]
	if !present {
		return 0, `request body is missing the required "method" field`, false
	}
	index, ok := value.(float64)
	if !ok || index != math.Trunc(index) || index < 0 {
		return 0, `"method" must be a non-negative integer auth method index`, false
	}
	if inputs, present := body["inputs"]; present && inputs != nil {
		fields, ok := inputs.(map[string]any)
		if !ok {
			return 0, `"inputs" must be an object of prompt values`, false
		}
		for key, value := range fields {
			if _, ok := value.(string); !ok {
				return 0, fmt.Sprintf("prompt value %q must be a string", key), false
			}
		}
	}
	return int64(index), "", true
}

// writeProviderAuthError answers the document's ProviderAuthError shape
// (required "name" and "data" — components.schemas.ProviderAuthError1) with a
// 400. It is the declared error form for this operation, so the handler writes
// it rather than the generic writeOpencodeError {"error":{...}} envelope the
// sibling handlers use for operations whose declared error is unspecified.
func writeProviderAuthError(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{
		"name": name,
		"data": data,
	})
}

// handleProviderOAuthCallback serves POST /provider/{providerID}/oauth/callback
// — upstream provider.oauth.callback (ROUTE-ADD-101, SHIM-DRIFT-098, declared
// responses: 200 boolean "OAuth callback processed successfully" and 400
// ProviderAuthError | InvalidRequestError). Upstream hands the authorization
// code (or the no-code arm) to the flow the matching authorize call parked in
// instance state and, when that flow reports success, stores the returned
// credential and answers the declared 200 boolean true
// (packages/opencode/src/server/routes/instance/httpapi/handlers/provider.ts
// "callback", over packages/opencode/src/provider/auth.ts `callback` — which
// faults ProviderAuthOauthMissing when no flow is pending for the providerID —
// at the pinned commit 7945de208964a49300d7f770d1a71d078db9a4c4 / v1.18.33;
// the declared surface line is
// specs/openapi/upstream/openapi-1.18.33.surface.txt:127).
//
// The shim has no OAuth implementation and parks no flow: its provider-method
// registry (storedProviderAuthMethods) advertises only "api" methods, so no
// authorize call ever starts a flow and there is no pending credential for a
// callback to complete. The truthful answer to "was an OAuth callback
// processed?" is therefore false — there was nothing to process — and the
// handler answers the declared 200 with the JSON boolean false. It never
// fabricates a processed callback (upstream's literal `true` would claim a
// credential was stored), never serializes null (the declared type is boolean,
// so a null body would violate the contract), and never answers the pre-fix
// net/http default 404 for an operation it now serves.
//
// The declared 400 arm is answered with the document's own error shapes:
//
//   - a body that is absent, not JSON, not an object, whose required `method`
//     field is missing or is not a non-negative integer, whose optional `code`
//     field is not a string, or that carries a field the contract does not
//     declare (the request schema is additionalProperties: false) answers
//     ProviderAuthError {"name":"BadRequest","data":{...}} — the name upstream
//     maps a payload decode failure onto;
//   - a provider holding no stored credentials (so the shim advertises no
//     method for it) and a `method` index the provider does not advertise both
//     answer ProviderAuthError {"name":"BadRequest","data":{"providerID":...}}
//     — upstream resolves the pending flow by providerID and faults on either;
//     the shim answers the declared code with a named reason instead;
//   - a store that cannot be read answers InvalidRequestError
//     {"_tag":"InvalidRequestError","message":...} — the document's second
//     declared 400 shape.
//
// Non-POST answers 405 METHOD_NOT_ALLOWED (the sibling POST-route method guard;
// 405 is not part of the declared response set, so it is never the answer to a
// well-formed POST).
func (s *Server) handleProviderOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	providerID := r.PathValue("providerID")
	if providerID == "" {
		writeProviderAuthError(w, "BadRequest", map[string]any{})
		return
	}
	methodIndex, reason, ok := parseProviderOAuthCallbackBody(r)
	if !ok {
		writeProviderAuthError(w, "BadRequest", map[string]any{"message": reason})
		return
	}

	methods, err := s.storedProviderAuthMethods(r.Context())
	if err != nil {
		// The document declares no 5xx for this operation; answer the declared
		// 400 with its second shape instead of surfacing an undeclared 500.
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"_tag":    "InvalidRequestError",
			"message": "failed to read provider auth methods",
		})
		return
	}

	advertised := methods[providerID]
	if len(advertised) == 0 {
		writeProviderAuthError(w, "BadRequest", map[string]any{
			"providerID": providerID,
			"message":    "no auth methods registered for provider " + providerID,
		})
		return
	}
	if methodIndex >= int64(len(advertised)) {
		writeProviderAuthError(w, "BadRequest", map[string]any{
			"providerID": providerID,
			"field":      "method",
			"message": fmt.Sprintf("auth method index %d is not advertised for provider %s (0-%d)",
				methodIndex, providerID, len(advertised)-1),
		})
		return
	}

	// The advertised method is an API-key method, never an OAuth method, and
	// this shim parks no authorize flow, so there is no callback to process and
	// no credential to store: the truthful answer to the declared boolean is
	// false, serialized as JSON false (the declared type is boolean).
	writeJSON(w, false)
}

// parseProviderOAuthCallbackBody decodes the declared request body
// ({method: <auth method index>, code?: <OAuth authorization code>} —
// components.schemas of the pinned opencode document). It returns the method
// index, a human-readable reason for a refusal, and whether the body is
// well-formed. The document does not mark requestBody required, but its only
// required field (`method`) is mandatory, so an absent or empty body cannot
// select a method and is refused — the same direction upstream's runtime takes
// when decoding an empty request text fails.
//
// Unlike the authorize body (whose undeclared fields are ignored), this
// operation's schema is additionalProperties: false, so a field the contract
// does not declare is refused with the same BadRequest shape rather than
// silently dropped.
func parseProviderOAuthCallbackBody(r *http.Request) (int64, string, bool) {
	if r.Body == nil || r.ContentLength == 0 {
		return 0, "request body must carry the auth method index", false
	}
	var raw any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return 0, "malformed request body: " + err.Error(), false
	}
	// A JSON null body decodes without error into a nil interface, and an
	// array/scalar is not the declared object either.
	body, ok := raw.(map[string]any)
	if !ok {
		return 0, "request body must be a JSON object carrying the auth method index", false
	}
	for key := range body {
		if key != "method" && key != "code" {
			return 0, fmt.Sprintf("request body carries undeclared field %q", key), false
		}
	}
	value, present := body["method"]
	if !present {
		return 0, `request body is missing the required "method" field`, false
	}
	index, ok := value.(float64)
	if !ok || index != math.Trunc(index) || index < 0 {
		return 0, `"method" must be a non-negative integer auth method index`, false
	}
	if code, present := body["code"]; present {
		if _, ok := code.(string); !ok {
			return 0, `"code" must be a string`, false
		}
	}
	return int64(index), "", true
}

// handleSkill serves GET /skill — upstream app.skills (ROUTE-ADD-111,
// SHIM-DRIFT-125, declared responses: 200 Array of Skill, 400 BadRequest).
// The upstream Skill item is {name*, description?, location*, content*};
// the Consensus runtime keeps no skill registry, so the truthful payload is
// an empty array (never null — the document declares an array). The upstream
// operation declares directory/workspace query params for workspace scoping;
// the shim's empty answer is workspace-independent, so a valued param is
// well-formed and filters nothing, while a present-but-blank one is malformed
// input and answers the declared 400 via the sibling writeOpencodeError
// INVALID_REQUEST envelope. Non-GET answers 405 METHOD_NOT_ALLOWED — sibling
// method-guard convention, matching handleAgent/handleLSP.
func (s *Server) handleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, []map[string]any{})
}

// handleFormatter serves GET /formatter — upstream formatter.status
// (ROUTE-ADD-088, SHIM-DRIFT-086, declared responses: 200 Array of
// FormatterStatus, 400 BadRequest). The upstream FormatterStatus item is
// {name*, extensions*: string[], enabled*}; the Consensus runtime keeps no
// formatter registry (it shells out to no formatter and reports none), so the
// truthful payload is an empty array (never null — the document declares an
// array), the same honest answer the sibling /skill route gives for its own
// absent registry. The operation declares directory/workspace query params
// for workspace scoping; the empty answer is workspace-independent, so a
// valued param is well-formed and filters nothing, while a present-but-blank
// one is malformed input and answers the declared 400 via the sibling
// writeOpencodeError INVALID_REQUEST envelope. Non-GET answers 405
// METHOD_NOT_ALLOWED — sibling method-guard convention, matching
// handleSkill/handleAgent/handleLSP.
func (s *Server) handleFormatter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, []map[string]any{})
}

// handleSyncHistory serves POST /sync/history — upstream sync.history.list
// (ROUTE-ADD-112, SHIM-DRIFT-126, declared responses: 200 SyncEvent[],
// 400 BadRequest | InvalidRequestError). The body is a cursor map keyed by
// aggregate ID with non-negative integer seq values; events with seq greater
// than a listed cursor are returned, and unlisted aggregates get their full
// history. The Consensus runtime keeps no sync event store, so the truthful
// payload for any well-formed cursor is an empty array (never null — the
// document declares an array). Malformed input answers the declared 400 via
// the upstream v2 SDK NamedError BadRequest body: Body identifies malformed
// or non-object JSON; Payload identifies query or cursor schema violations.
// Non-POST answers 405 METHOD_NOT_ALLOWED — sibling method-guard convention.
func (s *Server) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeBadRequest(w, r, "Payload",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	var cursors map[string]any
	if r.Body == nil || r.ContentLength == 0 {
		cursors = map[string]any{}
	} else if err := json.NewDecoder(r.Body).Decode(&cursors); err != nil {
		writeOpencodeBadRequest(w, r, "Body", "malformed request body: "+err.Error())
		return
	}
	// A JSON null body decodes without error into a nil map (encoding/json
	// leaves the destination untouched) — still not a cursor object.
	if cursors == nil {
		writeOpencodeBadRequest(w, r, "Body", "request body must be a JSON object of aggregate cursors")
		return
	}
	for agg, seq := range cursors {
		f, ok := seq.(float64)
		if !ok || f != math.Trunc(f) || f < 0 {
			writeOpencodeBadRequest(w, r, "Payload",
				fmt.Sprintf("cursor for aggregate %q must be an integer >= 0", agg))
			return
		}
	}
	writeJSON(w, []map[string]any{})
}

// handleSyncReplay serves POST /sync/replay — upstream sync.replay
// (ROUTE-ADD-113, SHIM-DRIFT-127, declared responses: 200
// ReplayedSyncEvents {sessionID}, 400 BadRequest | InvalidRequestError).
//
// Upstream validates a complete sync event history and replays it: the
// request body carries the workspace directory plus events[] (declared
// minItems 1), and a well-formed history answers the sessionID of the
// replayed session. The declared item schema is id matching ^evt_, an
// aggregateID string, an integer seq >= 0, a type and an object data payload
// — all required, with additionalProperties: false on the object and on the
// items; directory and events are the object's required keys.
//
// The Consensus runtime keeps no sync event store and has no replay engine —
// the same truthfulness limit handleSyncHistory documents — so the declared
// 200 shape is answered with the one session id the shim can honestly report:
// "" (the replay analogue of the empty SyncEvent[] /sync/history answers;
// never a fabricated session). The declared 400 arm is answered via the
// sibling writeOpencodeError INVALID_REQUEST envelope for malformed input — a
// body that is not JSON, not an object, carries an unknown top-level key, or
// violates the declared schema (missing/blank directory, missing or empty
// events, an event that is not an object or whose fields violate the declared
// shape), and a present-but-blank declared query param (directory, workspace
// — the sibling handleSkill convention). The document leaves requestBody
// optional (no required flag), so an absent body is well-formed and answers
// the declared 200, exactly as handleSyncHistory accepts one. Non-POST
// answers 405 METHOD_NOT_ALLOWED — sibling method-guard convention.
func (s *Server) handleSyncReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	if r.Body != nil && r.ContentLength != 0 {
		var body struct {
			Directory *string           `json:"directory"`
			Events    []json.RawMessage `json:"events"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
			return
		}
		// The declared object is one JSON value; a second value after it is
		// malformed input, not a body to ignore.
		var trailing json.RawMessage
		if err := dec.Decode(&trailing); err != io.EOF {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"malformed request body: unexpected data after the request object")
			return
		}
		if body.Directory == nil || strings.TrimSpace(*body.Directory) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"request body must carry a non-blank directory")
			return
		}
		if len(body.Events) == 0 {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"request body must carry at least one sync event (events declares minItems 1)")
			return
		}
		for i, raw := range body.Events {
			if reason := syncReplayEventInvalid(raw); reason != "" {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("events[%d]: %s", i, reason))
				return
			}
		}
	}
	// No sync event store, so no session is replayed and none can be named:
	// the declared shape carries the empty string rather than a fabricated id.
	writeJSON(w, map[string]any{"sessionID": ""})
}

// handleSyncStart serves POST /sync/start — upstream sync.start
// (ROUTE-ADD-114, SHIM-DRIFT-128, declared responses: 200 boolean, 400
// BadRequest). Upstream starts a sync loop for every workspace in the current
// project that has an active session; the Consensus runtime keeps no sync loop
// engine — the same truthfulness limit handleSyncHistory and handleSyncReplay
// document — so no loop is started and the declared boolean 200 is answered
// with the truthful value false ("no workspace sync started") rather than the
// fabricated true an effect-free no-op would claim (the sibling
// handleGlobalUpgrade truthfulness convention: never assert an effect that was
// not performed).
//
// The operation declares two optional query parameters (directory, workspace)
// and no request body, so the declared 400 arm is answered via the sibling
// writeOpencodeError INVALID_REQUEST envelope for a present-but-blank declared
// query param (the sibling handleSkill/handleSyncHistory convention). A request
// body is neither declared nor read: an absent body and any body are both
// well-formed, exactly as handleGlobalDispose accepts them. Non-POST answers
// 405 METHOD_NOT_ALLOWED — sibling method-guard convention.
func (s *Server) handleSyncStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	// No sync loop engine exists, so no workspace sync was started: the
	// declared boolean reports false rather than a fabricated true.
	writeJSON(w, false)
}

// handleSyncSteal serves POST /sync/steal — upstream sync.steal
// (ROUTE-ADD-115, SHIM-DRIFT-129, declared responses: 200 {sessionID},
// 400 BadRequest | InvalidRequestError). Upstream updates a session to belong
// to the current workspace through the sync event system; the Consensus
// runtime keeps no sync event store and has no cross-workspace migration
// engine — the same truthfulness limit handleSyncHistory, handleSyncReplay and
// handleSyncStart document — so no steal is performed.
//
// The request body declares one field, sessionID (required, pattern ^ses,
// additionalProperties: false). The document does not mark requestBody
// required, but sessionID is the operation's only defined field and the
// declared 200 schema requires a ^ses id: an absent or empty body cannot name
// a session and is refused — the same direction parseProviderOAuthAuthorizeBody
// takes for the identical OpenAPI shape (optional requestBody whose only
// defined field is required), and the only alternative to answering 200 with a
// session id the shim was never given.
//
// The declared 200 body names the session the caller asked to steal. The shim
// performs no migration and mints no id; it reports back the identity the
// request concerned rather than inventing one (the sibling handleSyncReplay
// "never a fabricated session" convention — here the declared ^ses pattern
// leaves no truthful empty value, so the request's own id is the answer).
//
// The declared 400 arm answers the sibling writeOpencodeError INVALID_REQUEST
// envelope for malformed input: a body that is not JSON, that is not a JSON
// object (arrays, scalars and an explicit null are all not a steal request),
// that carries an unknown top-level key or trails a second value, or whose
// sessionID is missing, non-string, blank or does not match the declared
// pattern ^ses — plus a present-but-blank declared query param (directory,
// workspace — the sibling handleSkill/handleSyncStart convention). An absent
// or empty body is the missing-sessionID class and answers 400, not 200.
// Non-POST answers 405 METHOD_NOT_ALLOWED — sibling method-guard convention.
func (s *Server) handleSyncSteal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	// sessionID is the operation's only defined field and the declared 200
	// schema requires a ^ses value, so a body-less request names no session:
	// the same refusal parseProviderOAuthAuthorizeBody answers for the
	// identical optional-requestBody/required-field shape.
	if r.Body == nil || r.ContentLength == 0 {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"request body must carry the session to steal (sessionID is required)")
		return
	}
	var body struct {
		SessionID *string `json:"sessionID"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}
	// The declared object is one JSON value; a second value after it is
	// malformed input, not a body to ignore.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"malformed request body: unexpected data after the request object")
		return
	}
	if body.SessionID == nil || strings.TrimSpace(*body.SessionID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"request body must carry a non-blank sessionID")
		return
	}
	if !strings.HasPrefix(*body.SessionID, "ses") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("sessionID %q does not match the declared pattern ^ses", *body.SessionID))
		return
	}
	// No sync event store, so no cross-workspace migration is performed: the
	// declared 200 body names the session the caller asked to steal — the only
	// session id the shim holds without minting one.
	writeJSON(w, map[string]any{"sessionID": *body.SessionID})
}

// syncReplayEventInvalid validates one declared sync.replay events[] item
// against the schema the pinned upstream document declares: id (pattern
// ^evt_), aggregateID, an integer seq >= 0, type and an object data payload —
// all required, additionalProperties: false. It returns "" when the item is
// well-formed and the reason the declared 400 arm must name otherwise.
func syncReplayEventInvalid(raw json.RawMessage) string {
	var event struct {
		ID          *string        `json:"id"`
		AggregateID *string        `json:"aggregateID"`
		Seq         *float64       `json:"seq"`
		Type        *string        `json:"type"`
		Data        map[string]any `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&event); err != nil {
		return "not a valid sync event: " + err.Error()
	}
	if event.ID == nil {
		return "missing required field id"
	}
	if !strings.HasPrefix(*event.ID, "evt_") {
		return fmt.Sprintf("id %q does not match the declared pattern ^evt_", *event.ID)
	}
	if event.AggregateID == nil {
		return "missing required field aggregateID"
	}
	if event.Seq == nil {
		return "missing required field seq"
	}
	if *event.Seq != math.Trunc(*event.Seq) || *event.Seq < 0 {
		return fmt.Sprintf("seq %v must be an integer >= 0", *event.Seq)
	}
	if event.Type == nil {
		return "missing required field type"
	}
	if event.Data == nil {
		return "missing required field data (declared type: object)"
	}
	return ""
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
	// MCP-DIRECT-001: when an MCP CLIENT (not a browser) speaks to the
	// shim's bare /mcp mount, delegate to the real MCP handler instead of
	// answering the 501 stub, so an attach attempt never dies on the shim.
	// Two probes cover the streamable-HTTP client shapes: a POST with a
	// JSON-RPC body, and an SSE-negotiating GET (Accept: text/event-stream).
	// mcpHandler is injected by cmd/consensus/main.go; when nil (shim-only
	// test harnesses) the stub answer is preserved unchanged.
	if s.mcpHandler != nil &&
		((r.Method == http.MethodPost && strings.Contains(r.Header.Get("Content-Type"), "application/json")) ||
			(r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/event-stream"))) {
		s.mcpHandler.ServeHTTP(w, r)
		return
	}
	writeOpencodeError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"MCP management via opencode shim is not implemented; use /mcp/sse directly")
}

// handleMCPSub serves the /mcp/* sub-paths. Only the upstream mcp.auth.remove
// operation — DELETE /mcp/{name}/auth (ROUTE-ADD-093, SHIM-DRIFT-092, declared
// responses: 200 {success:true}, 400 BadRequestError, 404
// McpServerNotFoundError) — is served. Every other /mcp/* shape answers
// http.NotFound, the same 404 body net/http's default handler produced before
// this route existed, so the sibling rows of the declared family
// (mcp.auth.start, auth/authenticate, auth/callback, connect, disconnect) keep
// the NOT-SERVED observation they are pinned to in the declared-vs-served
// artifact, and a non-DELETE request on this path keeps its pre-change answer
// byte-for-byte (405 is not part of this operation's declared set — the same
// choice handleProviderAuth made for its undeclared methods, ROUTE-ADD-099).
func (s *Server) handleMCPSub(w http.ResponseWriter, r *http.Request) {
	name, sub, ok := parseMCPSubPath(r.URL.Path)
	if !ok || sub != "auth" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	s.handleMCPAuthRemove(w, r, name)
}

// parseMCPSubPath splits /mcp/{name}/{sub}. ok is true only for exactly that
// shape: a non-empty name and one non-empty trailing segment (so
// /mcp/{name}/auth/callback and /mcp/{name} are not this operation).
func parseMCPSubPath(path string) (name, sub string, ok bool) {
	rest, found := strings.CutPrefix(path, "/mcp/")
	if !found {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// handleMCPAuthRemove serves DELETE /mcp/{name}/auth — upstream
// mcp.auth.remove (ROUTE-ADD-093, SHIM-DRIFT-092), "Remove OAuth credentials
// for an MCP server".
//
// The shim keeps no MCP server registry of its own: it exposes Consensus AS an
// MCP server and holds no OAuth credential store for the servers a client
// configures (that is the mcp.auth.start / auth/callback half of the family,
// still unserved). It does own a settings store — the one GET/PATCH /config
// reads and writes and the one the sibling /auth operations key
// (auth.<providerID>.<field>). An MCP server entry is therefore resolved from
// that store under the flat translation of the upstream config's
// mcp: {"<name>": ...} block: a key "mcp.<name>" or any key under "mcp.<name>.".
// A name with no stored entry names no MCP server the shim knows, so there is
// nothing whose credentials this call could remove — that is the declared 404.
//
// The declared arms map as:
//
//	200 → the stored OAuth rows for the server (mcp.<name>.oauth.<field>) are
//	      removed and the declared body {"success": true} is returned. No OAuth
//	      rows may exist — the shim has no flow that writes them yet — and the
//	      removal is still the honest answer: after the call no stored MCP OAuth
//	      credentials remain for that server (the removal is idempotent, the
//	      same shape handleAuthDelete gives upstream auth.remove).
//	400 → malformed input: a blank {name}, or a present-but-blank declared
//	      query param (directory, workspace — the sibling handleSkill
//	      convention). A settings-store read failure also answers this arm
//	      rather than an undeclared 5xx, the way handleProviderAuth does.
//	404 → typed McpServerNotFoundError {_tag, name, message} for a name with
//	      no stored MCP server entry (the declared shape, never a bare 404).
func (s *Server) handleMCPAuthRemove(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "MCP server name is required")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, present := r.URL.Query()[param]; present && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	// Escape LIKE metacharacters so a name containing %, _ or \ resolves (and
	// removes) only its own rows — the same treatment handleAuthDelete gives a
	// provider id.
	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(name)
	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT key FROM system_settings WHERE key = $1 OR key LIKE $2 ESCAPE '\'`,
		"mcp."+name, "mcp."+pattern+".%")
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to read MCP server settings")
		return
	}
	if len(rows) == 0 {
		writeOpencodeNotFoundError(w, "McpServerNotFoundError", "name", name,
			"MCP server not found: "+name)
		return
	}

	if err := s.db.Exec(ctx,
		`DELETE FROM system_settings WHERE key LIKE $1 ESCAPE '\'`,
		"mcp."+pattern+".oauth.%"); err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
			"failed to remove MCP OAuth credentials")
		return
	}
	writeJSON(w, map[string]any{"success": true})
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

	case strings.HasPrefix(path, "symbol") && r.Method == http.MethodGet:
		// ROUTE-FIX-002 / SHIM-DRIFT-085: the upstream find.symbols
		// operation (declared responses 200 Symbol[], 400 BadRequestError)
		// was answered by the untyped 501 stub below for EVERY request, so
		// neither declared code was reachable from outside. Serve the
		// declared contract truthfully (see findSymbolRoutes.go): the
		// declared 200 is the empty Symbol list — the shim has no LSP
		// integration (handleLSP reports enabled:false unconditionally), so
		// there is no symbol producer and entries would be fabricated —
		// and the 400 arm carries the sibling INVALID_REQUEST envelope.
		s.findSymbols(w, r)
	case strings.HasPrefix(path, "symbol"):
		// Undeclared methods on the sub-path keep a not-implemented answer
		// (405 is not part of find.symbols' declared response set), typed
		// per the session.diff sibling convention (ROUTE-FIX-010) naming
		// the operation and the real GET route.
		writeNotImplemented(w, r, "find.symbols",
			"symbol search is a GET operation; use GET /find/symbol?query=<name>")
	default:
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown find sub-path")
	}
}

// handleFileList serves GET /file — upstream file.list (ROUTE-ADD-087,
// SHIM-DRIFT-084, declared responses: 200 FileNode[], 400 BadRequest).
// Upstream lists the files and directories under the required ?path= query
// parameter (an optional directory/workspace pair scopes a relative path).
// The shim runs on the same host filesystem as the workspace
// (service_adapter.go), so the honest 200 payload is a real os.ReadDir
// listing: one FileNode per entry carrying all five declared required fields
// — name, path, absolute, type ("file"|"directory") and ignored. `ignored`
// is always false: the shim keeps no gitignore index, so it never claims an
// entry is ignored (the same "no registry → don't fabricate" honesty as
// handleSkill). An empty directory answers [] — never null (the document
// declares an array).
//
// Every failure is answered with the object's only declared error code, 400:
// a missing/blank ?path= (?path= is required — sibling handleFileContent
// convention), a present-but-blank directory/workspace (sibling handleSkill
// convention), and a path that cannot be listed (absent, not a directory,
// unreadable). The document declares 200 and 400 for file.list and nothing
// else, so an unlistable path is reported as a bad request rather than as an
// undeclared 500 — the same doctrine as handleProviderAuth. Non-GET answers
// 405 METHOD_NOT_ALLOWED — the sibling method-guard convention shared by
// handleSkill/handleSyncHistory/handleFileContent.
func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := q[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	target := strings.TrimSpace(q.Get("path"))
	if target == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "?path= is required")
		return
	}
	// A relative ?path= is resolved against the declared workspace scope
	// (directory first, then workspace) — the shim's workspace is the
	// process working directory, so an unprefixed relative path lists
	// against it.
	if !filepath.IsAbs(target) {
		if base := q.Get("directory"); base != "" {
			target = filepath.Join(base, target)
		} else if base := q.Get("workspace"); base != "" {
			target = filepath.Join(base, target)
		}
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("path %q cannot be listed: %v", target, err))
		return
	}
	nodes := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		child := filepath.Join(target, e.Name())
		absolute := child
		if a, err := filepath.Abs(child); err == nil {
			absolute = a
		}
		nodeType := "file"
		if e.IsDir() {
			nodeType = "directory"
		}
		nodes = append(nodes, map[string]any{
			"name":     e.Name(),
			"path":     child,
			"absolute": absolute,
			"type":     nodeType,
			"ignored":  false,
		})
	}
	writeJSON(w, nodes)
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
	case sub == "reply" && r.Method == http.MethodPost:
		if !strings.HasPrefix(permID, "per") {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid permission request id")
			return
		}
		writeUpstreamRequestNotFound(w, "PermissionNotFoundError", permID, "Permission request not found: "+permID)
	default:
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "unknown permission action")
	}
}

func (s *Server) getPermission(w http.ResponseWriter, r *http.Request, permID string) {
	// ch:trace row=ROUTE-FIX-044 spec=migrations/008_hitl_tables.sql wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
	// The approval_requests table (migrations/008_hitl_tables.sql, also the
	// canonical SPEC-014 schema in internal/hitl/hitl.go) carries target_sql,
	// review_notes and reviewed_at — there is no sql_preview / decision_reason /
	// resolved_at column. The previous SELECT referenced the nonexistent names,
	// so the query error was swallowed into a 404 for rows that EXIST (T1-D4).
	// JSON keys stay as documented; only the column mapping is corrected.
	ctx := r.Context()
	row, err := s.db.QueryRow(ctx,
		`SELECT ar.id, ar.session_id, ar.request_type, ar.risk_level,
		        ar.description, ar.target_sql, ar.status, ar.review_notes,
		        ar.created_at, ar.reviewed_at
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
		"sql_preview":     toString(row["target_sql"]),
		"status":          toString(row["status"]),
		"decision_reason": toString(row["review_notes"]),
		"created_at":      toString(row["created_at"]),
		"resolved_at":     nilOrString(row["reviewed_at"]),
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

	// ch:trace row=ROUTE-FIX-045 spec=migrations/008_hitl_tables.sql wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
	// The UPDATE must target REAL approval_requests columns (review_notes,
	// reviewed_at, reviewer_id per migrations/008_hitl_tables.sql and the
	// native ReviewApproval write in internal/hitl/hitl.go) — decision_reason /
	// resolved_at / resolved_by do not exist and made every resolve a 500 (T1-D5).
	// The db wrapper exposes no RowsAffected, so existence and the
	// status='pending' guard are checked up front (same read-then-write shape
	// as hitl.Manager.ReviewApproval). An unknown id, or a row that is no
	// longer pending, answers the declared 404 arm — the contract documents
	// only [200,400,401,404], so 409 is not available here.
	row, err := s.db.QueryRow(ctx,
		`SELECT status FROM approval_requests WHERE id = $1`, permID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "permission not found")
		return
	}
	if toString(row["status"]) != "pending" {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"permission is not pending")
		return
	}

	// Translate: opencode permission resolution → Consensus approval review
	// SPEC-017 §3.7: Maps to POST /api/v1/approvals/:id/review
	// HARDEN-SHIM-10: Emit event on resolution for SSE subscribers
	err = s.db.Exec(ctx,
		`UPDATE approval_requests
		 SET status = $1, review_notes = $2, reviewed_at = $3, reviewer_id = 'opencode-shim'
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

// tuiDeclaredOps maps the TUI sub-paths the pinned upstream document declares
// to their operation ids (SHIM-GAP-002). The shim runs no TUI process, so
// these are answered with the typed not-implemented envelope naming the
// operation instead of the 404 the router used to hand back.
//
// The remaining shim-only actions (submit-prompt, execute-command, show-toast)
// keep their pre-existing error body: they are not part of the SHIM-GAP-002
// finding set. append-prompt is served by tuiAppendPrompt below.
var tuiDeclaredOps = map[string]string{
	"clear-prompt":     "tui.clearPrompt",
	"control/next":     "tui.control.next",
	"control/response": "tui.control.response",
	"open-help":        "tui.openHelp",
	"open-models":      "tui.openModels",
	"open-sessions":    "tui.openSessions",
	"open-themes":      "tui.openThemes",
	"publish":          "tui.publish",
}

// handlePty serves the bare /pty mount (ROUTE-ADD-102 / ROUTE-FIX-010):
//
//	GET  /pty — upstream pty.list (SHIM-DRIFT-105, declared responses:
//	     200 Array(Pty), 400 BadRequest | InvalidRequestError). Consensus has
//	     no pseudo-terminal registry, so the truthful happy path is an empty
//	     array — the contract declares a list, never null. The operation's
//	     only request parameters are the optional directory/workspace query
//	     selectors; when one names a path that does not exist or is not a
//	     directory, the shim answers the declared 400 arm with the sibling
//	     writeOpencodeError INVALID_REQUEST envelope instead of silently
//	     serving a different workspace's (empty) list.
//	POST /pty — upstream pty.create (SHIM-DRIFT-106): creating terminals is
//	     opencode-specific (SPEC-017 §3.9 exclusion list); the typed
//	     not_implemented envelope names the operation. It must stay typed and
//	     never 404: registering the bare path means POST reaches this handler
//	     rather than net/http's default, and a bodyless 501 or a 404 would
//	     reintroduce the dishonesty SHIM-GAP-002 removed.
//
// Non-GET/POST methods answer the sibling METHOD_NOT_ALLOWED envelope.
// /pty/* sub-paths (pty.shells, pty.get/remove/update, connect) are NOT
// registered and stay NOT-SERVED (net/http default 404) — only the bare
// mount is claimed. Auth is untouched: /pty is not a stub path and carries
// no fixed-workspace exemption, so it keeps the standard api-key policy.
func (s *Server) handlePty(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.ptyList(w, r)
	case http.MethodPost:
		writeNotImplemented(w, r, "pty.create",
			"pty.create is not implemented: spawning pseudo-terminals is opencode-specific and Consensus keeps no pty registry; use the native shell tool inside an agent session")
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
	}
}

// ptyList answers upstream pty.list. The declared 200 body is Array(Pty)
// {id, title, command, args, cwd, status, pid}; the shim has nothing to list,
// so it serves the typed empty array. The optional directory/workspace query
// selectors are validated against the filesystem first: naming a path that
// does not exist (or is not a directory) answers the declared 400 arm.
func (s *Server) ptyList(w http.ResponseWriter, r *http.Request) {
	for _, param := range []string{"directory", "workspace"} {
		if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
			if info, err := os.Stat(v); err != nil || !info.IsDir() {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("%s %q does not exist or is not a directory", param, v))
				return
			}
		}
	}
	writeJSON(w, []map[string]any{})
}

// tuiAppendPrompt serves the declared tui.appendPrompt contract. The shim has
// no attached TUI process to receive the text, so valid input returns the
// declared boolean false rather than claiming that a prompt was processed.
func (s *Server) tuiAppendPrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text *string `json:"text"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeOpencodeBadRequest(w, r, "Body", "request body must be an object containing only a string text field")
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeBadRequest(w, r, "Body", "request body must contain exactly one JSON object")
		return
	}
	if req.Text == nil {
		writeOpencodeBadRequest(w, r, "Payload", "required field text must be a string")
		return
	}

	writeJSON(w, false)
}

func (s *Server) handleTUI(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/tui/")

	if sub == "append-prompt" {
		if r.Method != http.MethodPost {
			writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		s.tuiAppendPrompt(w, r)
		return
	}

	if op, ok := tuiDeclaredOps[sub]; ok {
		writeNotImplemented(w, r, op,
			fmt.Sprintf("TUI control %q is not implemented: no opencode TUI process is attached to this shim; use opencode's built-in TUI", sub))
		return
	}

	// TUI sub-paths: append-prompt, submit-prompt, execute-command, show-toast
	// These are shim-only and do not map to native API calls.
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

// handleDoc serves the machine-readable OpenAPI document that the upstream
// opencode suite pins at GET /doc (httpapi-instance.test.ts:59 "serves the
// OpenAPI document": 200 + content-type application/json + a parseable
// document containing /global/health and /session paths). Serving Swagger
// HTML here failed the pinned upstream contract (T6, DF-CONSENSUS-36); the
// interactive REST Swagger UI lives at /doc/api (SPEC-018 §9).
//
// The document is the same embedded bundle the REST API serves at
// /openapi.json and /openapi.yaml (SPEC-018 §9): the shim surface and the
// native REST API are two protocols over one business layer, so one served
// contract covers both. The default request (no Accept header, or no JSON
// type in Accept) is JSON; an explicit Accept: application/yaml gets the
// raw YAML bytes — mirroring the existing API serving conventions.
//
// The route is deliberately public (authMiddleware skips /doc*) — upstream
// clients fetch the contract without credentials, and the document describes
// only the protocol surface.
func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	if len(specs.BundledYAML) == 0 {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"OpenAPI spec not found. The spec is embedded at build time from specs/openapi/bundled.yaml; rebuild the binary.")
		return
	}

	if acceptsYAML(r) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(specs.BundledYAML)
		return
	}

	doc, err := specs.ParsedDocument()
	if err != nil {
		slog.Error("opencode-shim: failed to load OpenAPI spec", "error", err)
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to serve OpenAPI spec")
		return
	}
	writeJSON(w, doc)
}

// acceptsYAML reports whether the request's Accept header explicitly asks
// for YAML. Only an application/yaml preference negotiates YAML; every other
// request (including no header at all) gets the JSON default so the upstream
// client always receives machine-readable JSON at /doc.
func acceptsYAML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.TrimSpace(accept) == "" {
		return false
	}
	for _, part := range strings.Split(accept, ",") {
		if strings.HasPrefix(strings.TrimSpace(part), "application/yaml") {
			return true
		}
	}
	return false
}

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

// writeJSONStatus is writeJSON with an explicit status code, for the handlers
// whose declared response set includes an error code with a body shape of its
// own (e.g. POST /provider/{providerID}/oauth/authorize's 400 ProviderAuthError
// | InvalidRequestError) that the shared writeOpencodeError envelope does not
// carry.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data, err := json.Marshal(v); err == nil {
		w.Write(data)
	}
}

// writeOpencodeBadRequest emits the NamedError shape consumed by the upstream
// v2 SDK. kind is Body for malformed/non-object JSON and Payload for values
// that violate the operation's declared query or body schema.
func writeOpencodeBadRequest(w http.ResponseWriter, r *http.Request, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	data, _ := json.Marshal(map[string]any{
		"name": "BadRequest",
		"data": map[string]string{
			"kind":    kind,
			"message": message,
		},
	})
	w.Write(data)
	slog.Warn("opencode-shim: bad request", "method", r.Method, "path", r.URL.Path, "kind", kind)
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

// notImplementedResponse is the typed envelope every declared-but-untranslated
// opencode operation answers with (SHIM-GAP-002).
//
// The shape is deliberately flat and single-purpose: `operation` names the
// upstream operationId the client asked for and `detail` says what the shim
// cannot serve and where the supported surface is. A client — or an operator
// reading a log line — can therefore tell "this shim does not implement that
// opencode operation" from "no such route", which is exactly what a silent
// 404 from a registered route (or a bare 501) prevented.
type notImplementedResponse struct {
	Error     string `json:"error"`
	Operation string `json:"operation"`
	Detail    string `json:"detail"`
}

// writeNotImplemented answers a declared opencode operation the Consensus shim
// does not translate: HTTP 501 plus the typed notImplementedResponse envelope
// (SHIM-GAP-002, specs/017 §3.9). It is the single response path for that
// whole family — no handler inlines its own stub body.
func writeNotImplemented(w http.ResponseWriter, r *http.Request, operation, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	if data, err := json.Marshal(notImplementedResponse{
		Error:     "not_implemented",
		Operation: operation,
		Detail:    detail,
	}); err == nil {
		w.Write(data)
	}
	slog.Warn("opencode-shim: not implemented", "method", r.Method, "path", r.URL.Path, "operation", operation)
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
	cmd.Env = gitEnv()
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

func (s *Server) sessionFork(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		MessageID string `json:"messageID"`
	}
	// An absent body is a valid fork request (no explicit fork point); only a
	// body that is present and malformed is a client error.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	// Resolve the source session first: an unknown id answers the declared 404
	// (NotFoundError) before any child row is written.
	src, err := s.db.QueryRow(r.Context(),
		`SELECT id, agent_name, model_id, status, goal, context_budget
		 FROM sessions WHERE id = $1`, sessionID)
	if err != nil || src == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	// A named fork point must exist in the source session. A value that does
	// not even match the declared ^msg shape is a client contract violation
	// (400); a well-formed id the session does not hold is the declared 404.
	if req.MessageID != "" {
		if !strings.HasPrefix(req.MessageID, "msg") {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "messageID must match ^msg")
			return
		}
		if !s.sessionHasMessage(r.Context(), sessionID, req.MessageID) {
			writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "fork point message not found in session")
			return
		}
	}

	childID := newUUID()
	now := time.Now().UTC().Format(time.RFC3339)

	agentName := toString(src["agent_name"])
	modelID := toString(src["model_id"])
	goal := toString(src["goal"])
	contextBudget := toInt64(src["context_budget"])
	if contextBudget <= 0 {
		contextBudget = 128000
	}

	// A forked session is a plain Consensus session: it shares the sessions
	// store with its parent and is reachable through the ordinary
	// GET /session/{id} and GET /session/{id}/children routes. No API key is
	// minted — the declared 200 body is the upstream Session schema, which
	// carries no credential, so an unreachable key would be dead state.
	if err := s.db.Exec(r.Context(),
		`INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, context_budget, heartbeat_at, created_at)
		 VALUES ($1, $2, $3, $4, 'booting', $5, $6, $7, $7)`,
		childID, sessionID, agentName, modelID, goal, contextBudget, now,
	); err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fork session: "+err.Error())
		return
	}

	// Register the shim mapping the way createSession does, so the child is
	// addressable through the opencode bridge like any other session.
	externalID := childID
	if v := r.URL.Query().Get("external_id"); v != "" {
		externalID = v
	}
	s.db.Exec(r.Context(),
		`INSERT INTO shim_session_map (shim_type, external_id, session_id, created_at, last_used_at)
		 VALUES ('opencode', $1, $2, $3, $3)`,
		externalID, childID, now)

	resp := s.translateSessionRow(map[string]any{
		"id": childID, "agent_name": agentName, "model_id": modelID,
		"status": "booting", "goal": goal, "context_budget": contextBudget,
		"iteration": int64(0), "tokens_used_in": int64(0), "tokens_used_out": int64(0),
		"created_at": now,
	})
	// parentID is part of the upstream Session schema and the whole point of a
	// fork; surface the fork link alongside the translated fields.
	resp["parentID"] = sessionID

	setFixedWorkspaceSyncFence(w, r, childID)
	writeJSON(w, resp)
}

// sessionHasMessage reports whether messageID (the opencode "msg-<id>" form)
// resolves to a memory_event of the given session. It mirrors the resolution
// GET /session/{id}/message/{messageID} uses: strip the "msg-" prefix and match
// the numeric id, then fall back to a prefix match.

func (s *Server) sessionHasMessage(ctx context.Context, sessionID, messageID string) bool {
	trimmed := strings.TrimPrefix(messageID, "msg-")
	row, err := s.db.QueryRow(ctx,
		`SELECT id FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) = $2 LIMIT 1`,
		sessionID, trimmed)
	if err == nil && row != nil {
		return true
	}
	row, err = s.db.QueryRow(ctx,
		`SELECT id FROM memory_events WHERE session_id = $1 AND CAST(id AS TEXT) LIKE $2 LIMIT 1`,
		sessionID, trimmed+"%")
	return err == nil && row != nil
}

// patchSession handles PATCH /session/:id — update session properties (title, status, goal).
// SPEC-017 §3.2: HARDEN-SHIM-02 remediation.

// sessionInitInstruction is the directive upstream session.init delivers to the
// session's agent — verbatim from the pinned document's operation description
// (openapi-1.18.33.json paths."/session/{sessionID}/init".post.description).
const sessionInitInstruction = "Analyze the current application and create an AGENTS.md file with project-specific agent configurations."

// sessionInit serves POST /session/{sessionID}/init — upstream session.init
// (ROUTE-FIX-012, SHIM-DRIFT-118; declared responses 200 boolean,
// 400 BadRequest | InvalidRequestError, 404 NotFoundError). The pre-fix
// handler answered the untyped 501 stub from handleSessionByID's default arm,
// which contradicts the declaration the client generated against.
//
// Translation: the upstream operation analyzes the current application and
// produces project-specific agent instructions — it is a session turn, which
// is why its requestBody carries the opencode ids (modelID, providerID,
// messageID). The shim submits the same directive through the native service
// layer as one user instruction, so the analysis and the produced
// instructions land in the session's memory (SPEC-002), and answers the
// declared boolean `true` once that turn is produced. Consensus keeps its own
// bootstrapping (SPEC-017 §3.9) and the shim writes nothing into the
// workspace itself. The declared error vocabulary for this operation is
// 400 | 404 only, so a directive that cannot be executed (missing native
// service layer, timeout, failed session, empty response) answers 400
// INVALID_REQUEST naming the reason instead of an undeclared 5xx or the stub
// 501.
func (s *Server) sessionInit(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Upstream requestBody requires all three ids
	// (openapi-1.18.33.json paths."/session/{sessionID}/init".post.requestBody:
	// required ["modelID", "providerID", "messageID"]).
	var req struct {
		ModelID    string `json:"modelID"`
		ProviderID string `json:"providerID"`
		MessageID  string `json:"messageID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}
	if strings.TrimSpace(req.ModelID) == "" || strings.TrimSpace(req.ProviderID) == "" || strings.TrimSpace(req.MessageID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "modelID, providerID and messageID are required")
		return
	}

	if s.svc == nil {
		// The init directive cannot be delivered without the native service
		// layer; answer inside the declared contract instead of the stub 501.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "session init requires the native service layer")
		return
	}

	// Resolve the session first so an unknown id answers the declared 404
	// (NotFoundError) before any turn is attempted.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	// Execute: the init directive goes through the same synchronous turn path
	// as POST /session/{id}/message, with the same bounded response timeout
	// (SPEC-017 §3.2).
	timeout := s.messageResponseTimeout
	if timeout <= 0 {
		timeout = defaultMessageResponseTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result, err := s.svc.SendMessage(ctx, MessageSendInput{
		SessionID: sessionID,
		Content:   sessionInitInstruction,
		MsgType:   "user_instruction",
	})
	if err != nil || result == nil || strings.TrimSpace(result.Content) == "" {
		// A turn that cannot be produced answers the declared 400 naming the
		// reason; never a fabricated success.
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "session init could not be executed: "+errorMessage(err, result))
		return
	}

	// Declared 200 body: the operation's plain boolean.
	writeJSON(w, true)
}

// sessionPromptAsync handles POST /session/:id/prompt_async — the
// fire-and-forget variant of POST /session/:id/message. It appends the user
// message through the same message-send path the synchronous send uses
// (memory_events append + the session wake so the heartbeat loop claims the
// session) and answers the declared 204 No Content immediately, without
// waiting for an agent response.
func (s *Server) sessionPromptAsync(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	// Translate: extract text from opencode parts (same translation the
	// synchronous send uses).
	var textParts []string
	for _, p := range req.Parts {
		if p.Type == "text" && p.Text != "" {
			textParts = append(textParts, p.Text)
		}
	}
	content := strings.Join(textParts, "\n")
	if strings.TrimSpace(content) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "message content is empty")
		return
	}

	// Enqueueing a prompt requires the native message-send path; without the
	// service layer nothing can be enqueued, so answer inside the declared
	// contract instead of fabricating an acknowledgement.
	if s.svc == nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "async prompt delivery requires the native service layer")
		return
	}

	// Resolve the session first so an unknown id answers the declared 404
	// (NotFoundError) before anything is enqueued.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	// Enqueue: append the user message and wake the session. Fire-and-forget —
	// the agent's response is never awaited here. SendMessage (via the shim
	// Service) appends to memory_events and wakes the session before waiting
	// on the produced turn; run it on a detached, bounded goroutine so the 204
	// goes out at enqueue time. The response event, if one is ever produced,
	// simply stays unread — exactly the async contract.
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	go func() {
		defer cancel()
		_, _ = s.svc.SendMessage(enqueueCtx, MessageSendInput{
			SessionID: sessionID,
			Content:   content,
			MsgType:   "user_instruction",
		})
	}()

	// Declared success: 204 No Content, empty body.
	w.WriteHeader(http.StatusNoContent)
}
