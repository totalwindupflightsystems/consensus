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
	"sort"
	"strconv"
	"strings"
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
	// ROUTE-ADD-005 / SHIM-DRIFT-001: the declared v2.agent.list operation
	// (GET /api/agent) had no shim route at all, so the request fell through
	// to net/http's default 404. Serve it for real; the /agent compatibility
	// route above is untouched (different surface, v1 spelling).
	mux.HandleFunc("/api/agent", s.handleApiAgentList)
	// ROUTE-ADD-006 / SHIM-DRIFT-002: the declared v2.command.list operation
	// (GET /api/command) likewise had no shim route, so it fell through to
	// net/http's default 404. Serve it for real; the v1 compatibility route
	// above (/command, upstream command.list) is untouched — different
	// surface, v1 spelling.
	mux.HandleFunc("/api/command", s.handleApiCommandList)
	// ROUTE-ADD-006 / SHIM-DRIFT-008: the declared v2.health.get operation
	// (GET /api/health) had the same gap — no shim route, net/http's default
	// 404. Serve it for real; /global/health (global.health) is a separate
	// declared operation with its own body and is untouched.
	mux.HandleFunc("/api/health", s.handleApiHealth)
	// ROUTE-ADD-009 / SHIM-DRIFT-003: the declared v2.event.subscribe operation
	// (GET /api/event) had no shim route at all, so the request fell through to
	// net/http's default 404. Register it for real — the handler answers the
	// declared 400 for every well-formed subscribe (honest narrowing, see
	// api_event_subscribe.go: the runtime keeps no v2 session-event store, so
	// the declared SSE 200 is never fabricated). The v1 /event compatibility
	// route below is untouched (different surface, v1 spelling).
	mux.HandleFunc("/api/event", s.handleApiEventSubscribe)
	// ROUTE-ADD-014 / SHIM-DRIFT-001: the declared v2.integration.list
	// operation (GET /api/integration) had no shim route, so it fell through
	// to net/http's default 404. Register the exact pattern; never an /api/*
	// catch-all, so unrelated v2 operations keep their current classification.
	mux.HandleFunc("/api/integration", s.handleApiIntegrationList)
	// ROUTE-ADD-016 / SHIM-DRIFT-003: the declared
	// v2.integration.attempt.status operation (GET
	// /api/integration/attempt/{attemptID}) had no shim route, so it fell
	// through to net/http's default 404. Registration is the merged
	// method-dispatch entry near ROUTE-ADD-015 (below) — one exact pattern,
	// no /api/* or /api/integration/* catch-all.
	// ROUTE-ADD-017 / SHIM-DRIFT-004: the declared
	// v2.integration.attempt.complete operation (POST
	// /api/integration/attempt/{attemptID}/complete) had no shim route, so
	// net/http answered the default 404. Register the exact parameterized path;
	// this is not an /api/integration/* catch-all.
	mux.HandleFunc("/api/integration/attempt/{attemptID}/complete", s.handleApiIntegrationRA017)
	// ROUTE-ADD-018 / SHIM-DRIFT-005: the declared
	// GET /api/integration/{integrationID} operation (v2.integration.get) had
	// no shim route, so net/http answered its default 404. Register the exact
	// one-segment path shape; never an /api/* or /api/integration/* catch-all.
	mux.HandleFunc("/api/integration/{integrationID}", s.handleApiIntegrationRA018)
	// ROUTE-ADD-019 / formerly SHIM-DRIFT-015: exact declared POST route for
	// v2.integration.connect.key; no /api/integration/* catch-all.
	mux.HandleFunc("/api/integration/{integrationID}/connect/key", s.handleApiIntegrationConnectKey)
	// ROUTE-ADD-020: serve the declared v2.integration.connect.oauth POST on
	// its exact path. This does not claim sibling /api/integration/* routes.
	mux.HandleFunc("/api/integration/{integrationID}/connect/oauth", s.handleApiIntegrationConnectOauth)

	// ROUTE-ADD-021 / source items SHIM-DRIFT-016/017/018/019/030/031/057: the
	// declared v2 read-only list operations (v2.location.get, v2.model.list,
	// v2.reference.list, v2.skill.list, v2.permission.request.list,
	// v2.permission.saved.list, v2.question.request.list) had no shim route at
	// all, so each request fell through to net/http's default 404 — the drift
	// artifact's NOT-SERVED class, indistinguishable from "no such opencode
	// operation". Each declared path gets its own EXACT pattern (see
	// api_v2_lists.go), never an /api/* catch-all, so sibling /api/* operations
	// keep the net/http default 404 they are classified with until their own
	// contract rows land.
	mux.HandleFunc("/api/location", s.handleApiLocation)
	mux.HandleFunc("/api/model", s.handleApiModelList)
	mux.HandleFunc("/api/reference", s.handleApiReferenceList)
	mux.HandleFunc("/api/skill", s.handleApiSkillList)
	mux.HandleFunc("/api/permission/request", s.handleApiPermissionRequestList)
	mux.HandleFunc("/api/permission/saved", s.handleApiPermissionSavedList)
	// ROUTE-ADD-025 / SHIM-DRIFT-001: the declared v2.permission.saved.remove
	// operation (DELETE /api/permission/saved/{id}) had no shim route, so the
	// request fell through to net/http's default 404. Register the exact
	// one-segment path shape for real; never an /api/permission/* catch-all,
	// so the sibling list route above and any deeper unregistered sub-path
	// keep the answers they are classified with.
	mux.HandleFunc("/api/permission/saved/{id}", s.handleApiPermissionSavedRemove)
	mux.HandleFunc("/api/question/request", s.handleApiQuestionRequestList)
	// ROUTE-ADD-026: the declared v2.provider.list operation (GET /api/provider)
	// had no shim route at all, so the request fell through to net/http's
	// default 404. Register the exact path for real; never an /api/* or
	// /api/provider/* catch-all, so the v1 /provider compatibility route above
	// (provider.list, a different declared operation) is untouched.
	mux.HandleFunc("/api/provider", s.handleApiProviderList)
	// ROUTE-ADD-027: the declared v2.provider.get operation (GET
	// /api/provider/{providerID}) had no shim route either, so it fell
	// through to net/http's default 404 as well. Register the exact
	// one-segment parameter shape for real; never an /api/* or
	// /api/provider/* catch-all, so the exact list route above and any deeper
	// /api/provider/a/b sub-path keep the answers they are classified with.
	mux.HandleFunc("/api/provider/{providerID}", s.handleApiProviderGet)
	// ROUTE-ADD-028: the declared v2.pty.list operation (GET /api/pty,
	// baseline-report source item SHIM-DRIFT-024) had no shim route at all,
	// so the request fell through to net/http's default 404. Register the
	// exact path for real; never an /api/* or /api/pty/* catch-all, so the
	// v1 /pty compatibility family above (pty.list, pty.create, pty.shells
	// and the /pty/{ptyID} operations) is untouched and deeper /api/pty/a/b
	// sub-paths keep the answers they are classified with. The exact
	// registration means the declared sibling method POST /api/pty
	// (v2.pty.create) reaches the handler too, where the typed
	// not-implemented envelope answers it (SHIM-GAP-002; the ROUTE-ADD-103
	// intentional narrowing beside bare /pty).
	mux.HandleFunc("/api/pty", s.handleApiPty)
	// ROUTE-ADD-015+016 merge resolution: the shared exact parameter pattern
	// /api/integration/attempt/{attemptID} is registered ONCE and dispatched
	// by method — GET status -> handleApiIntegrationRA016 (v2.integration.attempt.status,
	// ROUTE-ADD-016), DELETE cancel -> handleApiIntegrationRA015
	// (v2.integration.attempt.cancel, ROUTE-ADD-015). A method dispatcher
	// precedes the handlers so undeclared methods keep the METHOD_NOT_ALLOWED
	// envelope. The original per-row registrations conflicted in net/http
	// (identical patterns panic at mux registration) — the wave tick that
	// merged both rows resolves to this one-entry dispatcher.
	mux.HandleFunc("/api/integration/attempt/{attemptID}", s.handleApiIntegrationAttempt)
	// ROUTE-ADD-008 / SHIM-DRIFT-002: the declared v2.credential.update
	// operation (PATCH /api/credential/{credentialID}) had no shim route at
	// all, so the request fell through to net/http's default 404 — the drift
	// artifact's NOT-SERVED class, indistinguishable from "no such opencode
	// operation". The prefix-style pattern registers the path family once (the
	// one-dispatch-entry-per-path-family convention) while the handler serves
	// ONLY the declared PATCH; DELETE on this path is v2.credential.remove,
	// ROUTE-ADD-007's scope on a sibling branch, and until its merge lands it
	// keeps the honest 405 METHOD_NOT_ALLOWED. Any other /api/credential/*
	// shape is refused inside the handler (a registered prefix must not become
	// an /api/* catch-all).
	mux.HandleFunc("/api/credential/", s.handleApiCredential)
	// ROUTE-ADD-063 / source item SHIM-DRIFT-060: the exact /command path
	// previously fell through to net/http's default 404. Serve the declared
	// command.list operation without changing the separate /instance/command
	// extension stub.
	mux.HandleFunc("/command", s.handleCommandList)
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
	// ROUTE-ADD-064: the exact /experimental/capabilities path had no shim
	// route at all, so GET fell through to net/http's default 404 (the
	// declared-vs-served artifact's NOT-SERVED class). Serve the declared
	// experimental.capabilities.get operation. Registration stays exact: the
	// /experimental/* family (already in MountPatterns) has no catch-all, so
	// the sibling declared operations keep their own rows until they land.
	mux.HandleFunc("/experimental/capabilities", s.handleExperimentalCapabilities)
	// ROUTE-ADD-065 / SHIM-DRIFT-062: the exact /experimental/console path was
	// never registered, so GET /experimental/console fell through to net/http's
	// default 404 — the drift row's NOT-SERVED class, indistinguishable from
	// "no such opencode operation". Serve the declared experimental.console.get
	// operation. A parent chi mount already reaches this subtree through the
	// /experimental/* MountPatterns entry, so only the exact mux path was
	// missing; the sibling /experimental/tool routes are separate declared
	// operations and stay untouched.
	mux.HandleFunc("/experimental/console", s.handleExperimentalConsole)
	// ROUTE-ADD-066 / source item SHIM-DRIFT-063: the exact
	// /experimental/console/orgs path had no shim route at all, so
	// GET /experimental/console/orgs (upstream experimental.console.listOrgs)
	// fell through to net/http's default 404 — the artifact's NOT-SERVED
	// class, indistinguishable from "no such opencode operation". Serve it
	// exactly; the declared sibling paths /experimental/console and
	// /experimental/console/switch are served by their own contract rows.
	mux.HandleFunc("/experimental/console/orgs", s.handleConsoleOrgs)
	// ROUTE-ADD-067 / SHIM-DRIFT-035: serve the declared experimental Console
	// org switch. Exact path only — the sibling /experimental/console/* path
	// (/console/orgs) is served by its own contract row, and a parent chi mount
	// already reaches this subtree through the /experimental/* MountPatterns
	// entry.
	mux.HandleFunc("/experimental/console/switch", s.handleConsoleSwitch)

	// ROUTE-ADD-010 / SHIM-DRIFT-006: serve the upstream v2 location-scoped
	// recursive filesystem search. Keep the registration exact so sibling v2
	// filesystem operations remain unclaimed until their own contract rows land.
	mux.HandleFunc("/api/fs/find", s.handleFSFind)
	// ROUTE-ADD-011 / SHIM-DRIFT-007 + SHIM-DRIFT-008: the remaining declared
	// v2 filesystem operations — v2.fs.list (GET /api/fs/list, exact path) and
	// v2.fs.read (GET /api/fs/read/*, the file to serve follows the prefix) —
	// had no shim route at all, so net/http answered the default 404 (the
	// artifact's NOT-SERVED class). Both are served from the real host
	// filesystem. The registrations stay exact: the /api/* sibling v2 surface
	// remains unclaimed until its own contract rows land.
	mux.HandleFunc("/api/fs/list", s.handleFSList)
	mux.HandleFunc(fsReadRoutePrefix, s.handleFSRead)

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

	// PTY (ROUTE-ADD-102 / ROUTE-ADD-103 / ROUTE-ADD-104 / ROUTE-FIX-010):
	// bare /pty serves pty.list and the intentionally narrowed pty.create;
	// /pty/shells serves the host's discoverable shell executables.
	// ROUTE-ADD-105..109 serve the declared /pty/{ptyID} operation family:
	// GET/PUT/DELETE pty.get/pty.update/pty.remove on the one-segment path and
	// GET pty.connect / POST pty.connectToken on its two declared children.
	// Each pattern pins the declared shape (one pty-id segment, optional
	// literal suffix), never a /pty/* catch-all, so any other sub-path keeps
	// the net/http default 404 it is classified with.
	mux.HandleFunc("/pty/shells", s.handlePtyShells)
	mux.HandleFunc("/pty/{ptyID}", s.handlePtyByID)
	mux.HandleFunc("/pty/{ptyID}/connect", s.handlePtyConnect)
	mux.HandleFunc("/pty/{ptyID}/connect-token", s.handlePtyConnectToken)
	mux.HandleFunc("/pty", s.handlePty)

	// TUI control (shim-only, passthrough)
	mux.HandleFunc("/tui/", s.handleTUI)

	// LSP
	mux.HandleFunc("/lsp", s.handleLSP)

	// MCP management
	mux.HandleFunc("/mcp", s.handleMCPEndpoint)
	// ROUTE-ADD-093 / SHIM-DRIFT-092 and ROUTE-ADD-094..098 / SHIM-DRIFT-085..089:
	// the /mcp/{name}/auth path was never registered, so its declared
	// operations fell through to net/http's default 404 ("the shim has no such
	// operation" was indistinguishable from "this shim does not implement
	// it"). Serve the upstream mcp family: mcp.auth.remove (DELETE
	// /mcp/{name}/auth), mcp.auth.start (POST /mcp/{name}/auth),
	// mcp.auth.authenticate, mcp.auth.callback, mcp.connect and mcp.disconnect.
	// Every other /mcp/* shape — a wrong method on a served path and every
	// shape outside the declared family — keeps the byte-identical pre-change
	// answer (http.NotFound, see handleMCPSub). This is a shape check, never a
	// /mcp/* catch-all.
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
	// ROUTE-FIX-033 / SHIM-DRIFT-144: GET /vcs/status is the declared
	// per-file status route; served via the shared git status/diff
	// translator with declared directory/workspace selector validation.
	mux.HandleFunc("/vcs/status", s.handleVCSStatus)
	// ROUTE-FIX-031 / SHIM-DRIFT-142: POST /vcs/apply is the declared
	// vcs.apply operation (declared responses 200 {applied boolean}, 400
	// VcsApplyError | InvalidRequestError); served per contract below
	// instead of the /vcs/ subtree's typed 501. ServeMux resolves the
	// longer pattern, so this exact registration wins over "/vcs/".
	mux.HandleFunc("/vcs/apply", s.handleVCSApply)
	// ROUTE-FIX-032 / SHIM-DRIFT-143: GET /vcs/diff/raw serves the declared
	// raw unified patch for the selected workspace. ServeMux resolves this
	// exact registration before the /vcs/ subtree stub.
	mux.HandleFunc("/vcs/diff/raw", s.handleVCSDiffRaw)
	// DF-CONSENSUS-47: /project/{id} sub-paths answer the upstream typed
	// ProjectNotFoundError (404) instead of the 501 stub — see §3.9.
	mux.HandleFunc("/project/", s.handleProjectByID)
	mux.HandleFunc("/vcs", s.handleVCS)
	mux.HandleFunc("/vcs/diff", s.handleVCS)
	mux.HandleFunc("/vcs/", s.handleProjectVCSSStub)
	mux.HandleFunc("/instance", s.handleInstance)
	mux.HandleFunc("/instance/", s.handleInstanceSub)
	// ROUTE-ADD-068 / SHIM-DRIFT-036: the exact control-plane move path had no
	// shim route at all, so POST /experimental/control-plane/move-session fell
	// through to net/http's default 404 (the drift row's NOT-SERVED class).
	// The /experimental/* MountPatterns entry already carries the subtree
	// through a parent chi mount; this registration is the missing exact path.
	// It stays exact so the sibling declared control-plane / project-copy
	// operations remain unclaimed until their own contract rows land.
	mux.HandleFunc("/experimental/control-plane/move-session", s.handleExperimentalMoveSession)
	// ROUTE-ADD-069 / source item SHIM-DRIFT-037 (the board row cited
	// SHIM-DRIFT-066, a positional id from an earlier baseline): the declared
	// v2.projectCopy.remove operation (DELETE
	// /experimental/project/{projectID}/copy) had no shim route at all, so
	// every request on that exact path fell through to net/http's default 404
	// — the artifact's NOT-SERVED class. Register the exact declared path
	// shape (/experimental/* is already a chi MountPattern, so a parent mount
	// reaches it); never an /experimental/project/* catch-all, so the sibling
	// declared /copy paths (v2.projectCopy.create, experimental.projectCopy
	// .generateName, v2.projectCopy.refresh) keep the drift class their own
	// contract rows are filed against.
	mux.HandleFunc("/experimental/project/{projectID}/copy", s.handleExperimentalProjectCopyRemove)

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
	// dispatchWithV2FSTraversalGuard (fs_list_read.go) sits between the auth
	// wall and the ServeMux so the declared v2.fs.read traversal arm is
	// reachable — see its doc comment.
	return s.corsMiddleware(s.authMiddleware(http.HandlerFunc(s.dispatchWithV2FSTraversalGuard)))
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
	"/command", "/command/*",
	"/skill", "/skill/*",
	"/formatter", "/formatter/*",
	"/experimental/*",
	// ROUTE-ADD-005 / SHIM-DRIFT-001: exact /api/agent (v2.agent.list) so a
	// parent chi mount reaches the shim route.
	"/api/agent",
	// ROUTE-ADD-006 / SHIM-DRIFT-002 and /api/health (v2.health.get,
	// SHIM-DRIFT-008): the same exact-pattern wiring for the two declared v2
	// operations served alongside it.
	"/api/command",
	"/api/health",
	// ROUTE-ADD-009 / SHIM-DRIFT-003: the exact /api/event path
	// (v2.event.subscribe) so a parent chi mount reaches the shim route.
	"/api/event",
	// ROUTE-ADD-014 / SHIM-DRIFT-001: exact /api/integration
	// (v2.integration.list), never an /api/* catch-all.
	"/api/integration",
	// ROUTE-ADD-016 / SHIM-DRIFT-003: exact parameter path for
	// v2.integration.attempt.status; no integration subtree catch-all.
	"/api/integration/attempt/{attemptID}",
	// ROUTE-ADD-017 / SHIM-DRIFT-004: exact parameterized path for
	// v2.integration.attempt.complete; no /api/integration/* catch-all.
	"/api/integration/attempt/{attemptID}/complete",
	// ROUTE-ADD-018 / SHIM-DRIFT-005: exact one-segment integration path.
	"/api/integration/{integrationID}",
	// ROUTE-ADD-019 / formerly SHIM-DRIFT-015: exact connect-with-key path.
	"/api/integration/{integrationID}/connect/key",
	// ROUTE-ADD-020: exact v2.integration.connect.oauth path.
	"/api/integration/{integrationID}/connect/oauth",

	// ROUTE-ADD-021: the seven declared v2 read-only list operations
	// (v2.location.get, v2.model.list, v2.reference.list, v2.skill.list,
	// v2.permission.request.list, v2.permission.saved.list,
	// v2.question.request.list) — exact paths, so a parent chi mount reaches
	// each shim route and no /api/* catch-all is introduced.
	"/api/location",
	"/api/model",
	"/api/reference",
	"/api/skill",
	"/api/permission/request",
	"/api/permission/saved",
	// ROUTE-ADD-025 / SHIM-DRIFT-001: exact declared parameter path for
	// v2.permission.saved.remove so a parent chi mount reaches the shim route;
	// no /api/permission/* catch-all is introduced.
	"/api/permission/saved/{id}",
	"/api/question/request",
	// ROUTE-ADD-026: exact /api/provider (v2.provider.list), never an /api/* or
	// /api/provider/* catch-all.
	"/api/provider",
	// ROUTE-ADD-027: exact declared parameter shape for v2.provider.get so a
	// parent chi mount reaches the shim route; no /api/provider/* catch-all.
	"/api/provider/{providerID}",
	// ROUTE-ADD-028: exact /api/pty (v2.pty.list), never an /api/* or
	// /api/pty/* catch-all; the declared sibling method POST /api/pty
	// (v2.pty.create) answers the typed 501 intentional narrowing through the
	// same registration.
	"/api/pty",
	// ROUTE-ADD-015 / SHIM-DRIFT-002: exact declared parameter path for
	// v2.integration.attempt.cancel; no /api/* catch-all is introduced.
	"/api/integration/attempt/{attemptID}",
	// ROUTE-ADD-008 / SHIM-DRIFT-002: the /api/credential/{credentialID}
	// family so a parent chi mount reaches the shim handler (v2.credential.update
	// is served here; v2.credential.remove is ROUTE-ADD-007's sibling scope).
	// The /* form is required: chi v5 matches a trailing-slash pattern ONLY
	// literally, so "/api/credential/" would 404 every /api/credential/{id}
	// request before the shim sees it (BUG-009).
	"/api/credential/*",
	"/api/fs/find",
	// ROUTE-ADD-011 / SHIM-DRIFT-007 + SHIM-DRIFT-008: the exact v2.fs.list
	// path and the v2.fs.read subtree so a parent chi mount reaches the shim
	// handlers (a missing pattern 404s exactly as it would in production).
	"/api/fs/list",
	"/api/fs/read/*",
	"/find", "/find/*",
	"/file", "/file/*",
	"/event",
	"/permission", "/permission/*",
	"/pty", "/pty/*",
	"/tui/*",
	"/lsp", "/lsp/*",
	"/doc", "/doc/*",
	// ROUTE-ADD-093 / ROUTE-ADD-094..098: the exact declared param shapes for
	// the mcp management family. The native MCP server owns /mcp/* in the
	// combined deployment (main.go mounts it before the shim's patterns); chi
	// prefers these deeper param routes over that catch-all, so only the
	// declared management sub-paths are claimed from the native mount —
	// /mcp/sse and /mcp/message still reach the MCP server.
	"/mcp/{name}/auth",
	"/mcp/{name}/auth/authenticate",
	"/mcp/{name}/auth/callback",
	"/mcp/{name}/connect",
	"/mcp/{name}/disconnect",
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
			// The v2 (v2.*) surface declares a 401 UnauthorizedError arm for
			// every operation, so these routes answer the typed v2 envelope
			// instead of the v1 UNAUTHENTICATED body (ROUTE-ADD-005/-006;
			// ROUTE-ADD-011 added the v2.fs.list path and the v2.fs.read
			// subtree; both keep api-key auth like every other implemented
			// route and are not in isFixedWorkspaceCompatibilityRequest).
			if isV2EnvelopeRoutePath(r.URL.Path) ||
				r.URL.Path == "/api/fs/list" || strings.HasPrefix(r.URL.Path, fsReadRoutePrefix) {
				writeV2Unauthorized(w, "invalid credentials")
				return
			}
			writeOpencodeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid credentials")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// v2EnvelopeRoutePaths are the /api/* paths whose declared 401 arm is the v2
// UnauthorizedError body ({_tag: "UnauthorizedError", message}) rather than the
// shim's own UNAUTHENTICATED envelope. ROUTE-ADD-005 registered the first of
// them (/api/agent) and /api/fs/find (ROUTE-ADD-010) carries the v2 spelling
// too; ROUTE-ADD-021 added the seven declared v2 read-only list operations and
// ROUTE-ADD-016 added the one-segment /api/integration/attempt/{attemptID}
// shape. Membership is exact-path or exact parameter-shape, so an unregistered
// /api/* sub-path keeps the shim's own envelope.
var v2EnvelopeRoutePaths = map[string]struct{}{
	"/api/agent":              {},
	"/api/command":            {},
	"/api/health":             {},
	"/api/fs/find":            {},
	"/api/fs/list":            {},
	"/api/integration":        {},
	"/api/location":           {},
	"/api/model":              {},
	"/api/reference":          {},
	"/api/skill":              {},
	"/api/permission/request": {},
	"/api/permission/saved":   {},
	"/api/question/request":   {},
	// ROUTE-ADD-026: the declared v2.provider.list path takes the v2
	// UnauthorizedError 401 envelope (this operation declares it); membership
	// stays exact-path, so only the registered /api/* shapes join it.
	// ROUTE-ADD-027 adds the sibling v2.provider.get one-segment shape through
	// isV2EnvelopeRoutePath below; deeper /api/provider/* sub-paths keep the
	// shim's own 401 body.
	"/api/provider": {},
	// ROUTE-ADD-028: the declared v2.pty.list path (and the declared sibling
	// method v2.pty.create on the same exact path) take the v2
	// UnauthorizedError 401 envelope; membership stays exact-path, so the
	// declared /api/pty/{ptyID} family keeps its own row and deeper
	// /api/pty/* sub-paths keep the shim's own 401 body.
	"/api/pty": {},
	// ROUTE-ADD-018 / SHIM-DRIFT-005 uses a path parameter, so
	// isV2EnvelopeRoutePath recognizes its exact one-segment shape below.
	// ROUTE-ADD-008 / SHIM-DRIFT-002: the declared v2.credential family —
	// both operations carry the same v2 UnauthorizedError 401 arm. The entry
	// is checked with strings.HasPrefix for this prefix-routed family (the
	// exact-path map membership is the default; see isV2EnvelopeRoutePath).
	"/api/credential/": {},
	"/api/event":       {},
}

// isV2EnvelopeRoutePath reports whether path is one of the declared /api/*
// operations whose 401 body is the v2 UnauthorizedError envelope. Membership is
// exact-path by default. Parameter routes are admitted only when the concrete
// path has the declared shape: the credential family uses its existing prefix
// rule, while the integration-attempt path accepts exactly one non-empty
// attemptID segment. Deeper, unregistered /api/* paths keep the shim's own
// envelope.
func isV2EnvelopeRoutePath(path string) bool {
	if strings.HasPrefix(path, "/api/credential/") {
		return true
	}
	if isApiIntegrationConnectKeyPath(path) {
		return true
	}
	if isApiIntegrationConnectOauthPath(path) {
		return true
	}
	if isApiIntegrationCompletePath(path) {
		return true
	}
	// The one-segment rule must come AFTER the /complete suffix rule: the
	// complete path's attemptID segment is followed by "/complete", so the
	// one-segment containment check would reject it first.
	if attemptID, ok := strings.CutPrefix(path, "/api/integration/attempt/"); ok {
		return attemptID != "" && !strings.Contains(attemptID, "/")
	}
	// ROUTE-ADD-018: the exact one-segment integration path
	// (/api/integration/{integrationID}) shares the one-segment shape with the
	// attempt family; both are declared v2 operations, while deeper
	// /api/integration/* sub-paths keep the shim's own envelope.
	if rest, ok := strings.CutPrefix(path, "/api/integration/"); ok {
		return rest != "" && !strings.Contains(rest, "/")
	}
	// ROUTE-ADD-025 / SHIM-DRIFT-001: the declared v2.permission.saved.remove
	// path (DELETE /api/permission/saved/{id}) carries the same v2
	// UnauthorizedError 401 arm as its sibling list operation, which the exact
	// map above already covers for the bare path. The one-segment rule admits
	// only the concrete parameter shape, so a blank or deeper
	// /api/permission/saved/* path keeps the shim's own envelope.
	if rest, ok := strings.CutPrefix(path, "/api/permission/saved/"); ok {
		return rest != "" && !strings.Contains(rest, "/")
	}
	// ROUTE-ADD-027: the declared v2.provider.get path
	// (GET /api/provider/{providerID}) declares the same v2 UnauthorizedError
	// 401 arm as its sibling v2.provider.list operation, which the exact map
	// above covers for the bare path. The one-segment rule admits only the
	// concrete parameter shape, so a blank or deeper /api/provider/* path keeps
	// the shim's own envelope.
	if rest, ok := strings.CutPrefix(path, "/api/provider/"); ok {
		return rest != "" && !strings.Contains(rest, "/")
	}
	// ROUTE-ADD-069: the declared v2.projectCopy.remove path
	// (DELETE /experimental/project/{projectID}/copy) is a v2.* operation, so
	// its unauthenticated answer is the v2 UnauthorizedError envelope rather
	// than the shim's own UNAUTHENTICATED body. The family is declared with a
	// path parameter, so only the concrete one-segment shape
	// (/experimental/project/<non-empty-id>/copy) joins the envelope; every
	// other /experimental/* path keeps the shim's own envelope.
	if rest, ok := strings.CutPrefix(path, "/experimental/project/"); ok {
		if id, ok := strings.CutSuffix(rest, "/copy"); ok {
			return id != "" && !strings.Contains(id, "/")
		}
	}
	_, ok := v2EnvelopeRoutePaths[path]
	return ok
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
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
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
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
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
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(data))
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
		_ = s.db.Exec(r.Context(),
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
		_ = s.db.Exec(ctx, `UPDATE sessions SET status = 'failed' WHERE id = $1`, sessionID)
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create api key")
		return
	}

	externalID := sessionID
	if v := r.URL.Query().Get("external_id"); v != "" {
		externalID = v
	}
	_ = s.db.Exec(ctx,
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
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
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
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
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

// authAllowedFields enumerates the credential fields PUT /auth/{providerID}
// accepts (DF-CONSENSUS-59): the upstream opencode protocol's known auth
// field names (SPEC-017 §3.2 stores credentials as system_settings rows, it
// does not enumerate fields). Without this gate any JSON object keys were
// stored verbatim as auth.<providerID>.<unknown>, minting phantom provider
// existence in GET /provider/auth and GET /api/provider/{providerID} — both
// derive which providers exist purely from those rows.
var authAllowedFields = map[string]struct{}{
	"client_id":           {},
	"client_secret":       {},
	"api_key":             {},
	"oauth_client_id":     {},
	"oauth_client_secret": {},
}

// handleAuthPut stores the submitted auth fields as system_settings rows
// keyed auth.<providerID>.<field>. Unknown fields are rejected with 400
// INVALID_REQUEST before anything is stored (DF-CONSENSUS-59), so a junk PUT
// can never mint provider existence downstream.
func (s *Server) handleAuthPut(w http.ResponseWriter, r *http.Request, path string) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	unknown := make([]string, 0)
	for k := range req {
		if _, ok := authAllowedFields[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"unknown auth field(s): "+strings.Join(unknown, ", "))
		return
	}

	// Store auth config in system_settings
	ctx := r.Context()
	for k, v := range req {
		key := fmt.Sprintf("auth.%s.%s", path, k)
		val := toString(v)
		_ = s.db.Exec(ctx,
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
	case r.Method == http.MethodGet && r.URL.Path == "/vcs/diff/raw":
		// ROUTE-FIX-032: raw diff is the same fixed-workspace read surface as
		// /vcs and /vcs/diff, so header-selected requests are public for
		// opencode compatibility while headerless requests retain auth.
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
// 501, not 401. A sub-path that has since been served per contract leaves the
// exemption (see the per-path exclusions below): its handler answers the
// declared contract, so there is real behavior to protect.
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
			// ROUTE-FIX-031: POST /vcs/apply is the declared vcs.apply
			// operation, served per contract (handleVCSApply) instead of the
			// typed 501, so it takes the same api-key auth as every other
			// served route — including its own method guard, since GET
			// /vcs/apply was already authenticated before this change
			// (only non-GET was exempt). The remaining /vcs/* non-GET
			// sub-paths keep the exemption.
			if path == "/vcs/apply" {
				return false
			}
			// ROUTE-FIX-032: GET /vcs/diff/raw is a served read operation,
			// not a non-GET stub, so every method on its exact route reaches
			// the handler's method guard and keeps the normal auth policy.
			if path == "/vcs/diff/raw" {
				return false
			}
			return true
		}
	}
	return false
}

// handleVCSStatus serves GET /vcs/status — upstream instance vcs.status
// (ROUTE-FIX-033, SHIM-DRIFT-144; declared responses: 200 File[], 400
// BadRequest). Both directory/workspace selectors are optional; a supplied
// value must name an existing directory. The workspace resolves like the
// fixed-workspace compatibility routes (x-opencode-directory header first,
// then a directory/workspace query value, then the server workspace) and the
// per-file status list is produced by the shared git status/diff translator.
func (s *Server) handleVCSStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
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
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("%s %q does not exist or is not a directory", param, value))
			return
		}
	}
	dir := s.requestWorkspaceDir(r)
	if strings.TrimSpace(r.Header.Get("x-opencode-directory")) == "" {
		for _, param := range []string{"directory", "workspace"} {
			if value := strings.TrimSpace(r.URL.Query().Get(param)); value != "" {
				dir = filepath.Clean(value)
				if param == "directory" {
					break
				}
			}
		}
	}
	writeJSON(w, gitFileDiffs(r.Context(), dir))
}

// handleVCSDiffRaw serves GET /vcs/diff/raw — upstream vcs.diff.raw
// (ROUTE-FIX-032, SHIM-DRIFT-143). The response is the raw unified patch for
// all current tracked and untracked changes in the selected workspace. A clean
// or non-git workspace is a valid empty text/x-diff response.
func (s *Server) handleVCSDiffRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	dir, invalidSelector := s.resolveVCSWorkspaceDir(r)
	if invalidSelector != "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("%s selector must name an existing directory", invalidSelector))
		return
	}

	w.Header().Set("Content-Type", "text/x-diff; charset=utf-8")
	_, _ = io.WriteString(w, gitRawDiff(r.Context(), dir))
}

// resolveVCSWorkspaceDir validates the optional directory/workspace selectors
// and applies the same precedence as the VCS status route: the
// x-opencode-directory header, then directory, then workspace, then the
// configured server workspace. It returns a selector label, rather than the
// supplied path, on validation failure so filesystem paths are not disclosed.
func (s *Server) resolveVCSWorkspaceDir(r *http.Request) (dir, invalidSelector string) {
	for _, param := range []string{"directory", "workspace"} {
		values, present := r.URL.Query()[param]
		if !present || len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			if present {
				return "", param
			}
			continue
		}
		value := strings.TrimSpace(values[0])
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			return "", param
		}
	}

	if headerDir := strings.TrimSpace(r.Header.Get("x-opencode-directory")); headerDir != "" {
		info, err := os.Stat(headerDir)
		if err != nil || !info.IsDir() {
			return "", "x-opencode-directory"
		}
		return filepath.Clean(headerDir), ""
	}
	if value := strings.TrimSpace(r.URL.Query().Get("directory")); value != "" {
		return filepath.Clean(value), ""
	}
	if value := strings.TrimSpace(r.URL.Query().Get("workspace")); value != "" {
		return filepath.Clean(value), ""
	}
	return s.workspaceDir(), ""
}

// handleVCSApply serves POST /vcs/apply — upstream instance vcs.apply
// (ROUTE-FIX-031, SHIM-DRIFT-142; declared responses: 200 object
// {applied (required boolean)}, 400 VcsApplyError | InvalidRequestError). The
// pinned document describes the operation as "Apply a raw patch to the current
// working tree."
//
// The Consensus shim has no VCS write capability, so a request that satisfies
// the declared schema answers the declared 200 with the truthful
// {"applied": false} — no patch was applied — on the ROUTE-FIX-023/029
// tuiPublish/tuiShowToast no-op precedent (those handlers answer the declared
// body for an operation a detached shim cannot perform). The shim never
// reports a patch it did not apply.
//
// Declared contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/vcs/apply".post): optional directory/workspace query selectors, a
// requestBody {patch (required string)} with additionalProperties: false, a
// 200 object {applied} with additionalProperties: false, and a 400 anyOf
// VcsApplyError | InvalidRequestError arm. Served arms:
//
//   - a body that is absent, malformed, not an object, whose required `patch`
//     field is missing or is not a non-empty string, or that carries an
//     undeclared property answers the declared 400 with the shim's BadRequest
//     NamedError envelope (decodeTUIJSONBody is the shim's shared
//     single-JSON-object decoder — its DisallowUnknownFields is the declared
//     additionalProperties: false). That payload-decode mapping is the one
//     ROUTE-ADD-100 established for a declared
//     anyOf[<domain error> | InvalidRequestError] 400
//     (handleProviderOAuthAuthorize);
//   - a present-but-blank directory/workspace selector answers the declared
//     400 with the same envelope, kind Query (the instanceDispose blank-query
//     convention). A VALUED selector is accepted as-is: this handler reads no
//     workspace, so unlike the sibling GET /vcs/status — which really runs git
//     status in the resolved directory — it does not gate on the path existing;
//   - non-POST answers 405 METHOD_NOT_ALLOWED, the sibling POST-route method
//     guard (writeOpencodeError). 405 is not in the declared response set, so
//     it is never the answer to a well-formed POST.
//
// ch:trace row=ROUTE-FIX-031 spec=specs/openapi/upstream/openapi-1.18.33.json#vcs.apply test=internal/shim/opencode/server_test.go::TestVCSApplyServesDeclaredContract doc=docs/evidence/ROUTE-FIX-031-live-probe.md evidence=docs/evidence/ROUTE-FIX-031-live-probe.md witness=none:self-verified-in-worktree
func (s *Server) handleVCSApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && len(values) > 0 && strings.TrimSpace(values[0]) == "" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	var req struct {
		Patch *string `json:"patch"`
	}
	if !decodeTUIJSONBody(w, r, &req,
		"request body must be an object containing only patch") {
		return
	}
	if req.Patch == nil || *req.Patch == "" {
		writeOpencodeBadRequest(w, r, "Payload", "required field patch must be a non-empty string")
		return
	}
	writeJSON(w, map[string]any{"applied": false})
}

// handleProjectVCSSStub returns 501 for /project and /vcs paths (opencode-specific).
func (s *Server) handleProjectVCSSStub(w http.ResponseWriter, r *http.Request) {
	// SHIM-GAP-002: the declared /vcs sub-operations the shim does not translate
	// answer the typed not-implemented envelope naming their operation id
	// (SHIM-DRIFT-143; /vcs/apply left this table in ROUTE-FIX-031, /vcs/status
	// in ROUTE-FIX-033) instead of the untyped stub body.
	if op, ok := vcsDeclaredOps[r.URL.Path]; ok {
		writeNotImplemented(w, r, op,
			fmt.Sprintf("%s is not implemented: raw/whole-status views are opencode-specific; the shim serves GET /vcs (Vcs.Info), GET /vcs/diff (FileDiff[]) and GET /vcs/status, or the native tool API", op))
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
// GET /vcs/diff are real compatibility routes (DF-CONSENSUS-38), GET
// /vcs/status is served per contract (ROUTE-FIX-033) and POST /vcs/apply is
// served per contract (ROUTE-FIX-031 / handleVCSApply) — none of them are in
// this table.
var vcsDeclaredOps = map[string]string{}

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
		if r.Method == http.MethodPatch {
			s.projectUpdate(w, r, s.requestWorkspaceDir(r))
			return
		}
		writeNotImplemented(w, r, "project.current",
			"project.current is declared as a GET operation; the shim translates GET only")
		return
	}
	if r.Method == http.MethodPatch {
		dir := s.requestWorkspaceDir(r)
		if projectID == instanceID(dir) {
			s.projectUpdate(w, r, dir)
			return
		}
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
	if metadata := readWorkspaceMetadata(dir); metadata != nil {
		if name, ok := metadata["name"].(string); ok && strings.TrimSpace(name) != "" {
			project["name"] = name
		}
		if icon, ok := metadata["icon"]; ok {
			project["icon"] = icon
		}
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

// projectUpdate serves the declared project.update operation against the
// singleton workspace represented by this shim. It persists only fields the
// upstream request declares, preserving unrelated consensus.json metadata.
func (s *Server) projectUpdate(w http.ResponseWriter, r *http.Request, dir string) {
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query", fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&raw); err != nil || raw == nil {
		writeOpencodeBadRequest(w, r, "Body", "request body must be a JSON object")
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeBadRequest(w, r, "Body", "request body must contain exactly one JSON value")
		return
	}
	for field := range raw {
		if field != "name" && field != "icon" && field != "commands" {
			writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("unknown field %q", field))
			return
		}
	}
	configPath := filepath.Join(dir, "consensus.json")
	config := map[string]any{}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(configPath); err == nil {
		mode = info.Mode().Perm()
		data, readErr := os.ReadFile(configPath)
		if readErr != nil || (len(data) > 0 && json.Unmarshal(data, &config) != nil) {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "could not read consensus.json")
			return
		}
	} else if !os.IsNotExist(err) {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "could not inspect consensus.json: "+err.Error())
		return
	}
	for _, field := range []string{"name", "icon", "commands"} {
		value, present := raw[field]
		if !present {
			continue
		}
		if field == "name" {
			var name string
			if err := json.Unmarshal(value, &name); err != nil || strings.TrimSpace(name) == "" {
				writeOpencodeBadRequest(w, r, "Body", `field "name" must be a non-empty string`)
				return
			}
			config[field] = name
			continue
		}
		var object map[string]any
		if err := json.Unmarshal(value, &object); err != nil || object == nil {
			writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("field %q must be an object", field))
			return
		}
		for key, item := range object {
			allowed := field == "icon" && (key == "url" || key == "override" || key == "color")
			if field == "commands" {
				allowed = key == "start"
			}
			if !allowed {
				writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("unknown %s field %q", field, key))
				return
			}
			if _, ok := item.(string); !ok {
				writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("%s field %q must be a string", field, key))
				return
			}
		}
		config[field] = object
	}
	if len(raw) == 0 {
		writeOpencodeBadRequest(w, r, "Body", "request body must contain at least one declared field")
		return
	}
	if err := os.WriteFile(configPath, mustJSON(config), mode); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "could not persist project metadata: "+err.Error())
		return
	}
	project, err := s.deriveProject(r.Context(), dir)
	if err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "could not read the project timestamps: "+err.Error())
		return
	}
	if icon, ok := config["icon"]; ok {
		project["icon"] = icon
	}
	if commands, ok := config["commands"]; ok {
		project["commands"] = commands
	}
	if name, ok := config["name"]; ok {
		project["name"] = name
	}
	writeJSON(w, project)
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func readWorkspaceMetadata(dir string) map[string]any {
	raw, err := os.ReadFile(filepath.Join(dir, "consensus.json"))
	if err != nil {
		return nil
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return nil
	}
	return metadata
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

// handleQuestionByID serves POST /question/{requestID}/reply (question.reply)
// and POST /question/{requestID}/reject (question.reject). Both operations
// declare responses 200 boolean, 400 BadRequest | InvalidRequestError and 404
// QuestionNotFoundError.
//
// SHIM-GAP-004 recorded the remaining narrowing explicitly. The runtime keeps
// no question producer: upstream's QuestionRequest (required id pattern ^que,
// sessionID, questions[] of QuestionInfo) has no counterpart here — the
// runtime's human-input surface is approval_requests (request_type /
// risk_level, SPEC-014), which cannot be truthfully reshaped into a
// QuestionRequest (no header/options, no ^que ids, and handleQuestionList
// answers the declared empty list rather than inventing entries). Nothing in
// the shim or the native API writes a question row, so no ^que request id can
// ever exist and the declared 200 boolean ("Question answered/rejected
// successfully") is unreachable — a fabricated true would assert that a
// question was answered when no question exists. The declared 404 typed
// QuestionNotFoundError is therefore the truthful answer for every well-formed
// requestID, and the declared 400 arms are served for what a client controls:
// an ill-shaped requestID (declared pattern ^que) and, for question.reply, a
// body that violates its declared schema. The recorded narrowing lives in
// specs/018-openapi-contract.md §12 and on the /question/{requestID}/reply and
// /question/{requestID}/reject rows of
// specs/openapi/upstream/consensus-shim-served-surface.yaml
// (x-shim-error-contract-narrowing + narrowing-rationale), so the comparison
// artifact counts it as a RECORDED narrowing instead of silently treating the
// error half as full coverage.
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
	if parts[1] == "reply" {
		// question.reply is the only one of the pair that declares a
		// requestBody: {answers*: QuestionAnswer[]}, additionalProperties
		// false, where QuestionAnswer is an array of selected-label strings.
		// question.reject declares no requestBody, so a body is neither read
		// nor required there.
		var req struct {
			Answers *[][]string `json:"answers"`
		}
		if !decodeDeclaredJSONObject(w, r, &req, "request body must be an object containing only answers") {
			return
		}
		if req.Answers == nil {
			writeOpencodeBadRequest(w, r, "Payload",
				"required field answers must be an array of arrays of strings")
			return
		}
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

// gitRawDiff returns a unified patch for tracked changes plus untracked files.
// `git diff HEAD` includes staged and unstaged tracked changes; untracked files
// are appended with git's no-index diff without mutating the index. Git's
// no-index command exits 1 for a real difference, so runGitRaw preserves its
// stdout even when the command reports that expected status.
func gitRawDiff(ctx context.Context, dir string) string {
	tracked, err := runGitRaw(ctx, dir, "diff", "HEAD", "--binary", "--no-ext-diff", "--")
	if err != nil {
		staged, _ := runGitRaw(ctx, dir, "diff", "--cached", "--binary", "--no-ext-diff", "--")
		unstaged, _ := runGitRaw(ctx, dir, "diff", "--binary", "--no-ext-diff", "--")
		tracked = staged
		if tracked != "" && unstaged != "" && !strings.HasSuffix(tracked, "\n") {
			tracked += "\n"
		}
		tracked += unstaged
	}

	untracked, err := runGitRaw(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return tracked
	}
	patch := tracked
	for _, file := range strings.Split(untracked, "\x00") {
		if file == "" {
			continue
		}
		part, _ := runGitRaw(ctx, dir, "diff", "--no-index", "--binary", "--", "/dev/null", file)
		if part == "" {
			continue
		}
		if patch != "" && !strings.HasSuffix(patch, "\n") {
			patch += "\n"
		}
		patch += part
	}
	return patch
}

// runGitRaw runs git -C dir <args...> and preserves stdout even when git
// returns a non-zero status for a meaningful diff (notably --no-index).
func runGitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
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
		// DF-CONSENSUS-54: a setting is stored as the TEXT of its row, so a
		// JSON-valued row (the mcp.<name> server rows PATCH /config writes)
		// reached a client as a JSON *string* — settings["mcp.websearch"] was
		// text a client had to re-parse, and a row written before
		// DF-CONSENSUS-53 flattened MCP blocks reached it as raw Go %v text
		// ("map[websearch:map[type:remote url:...]]"). Emit every row as the
		// JSON value it actually is: objects stay objects, plain text stays a
		// string. The store format is unchanged — this is a read-side decode.
		values := make(map[string]string, len(rows))
		settings := make(map[string]any, len(rows))
		for _, row := range rows {
			key := toString(row["key"])
			value := toString(row["value"])
			values[key] = value
			settings[key] = configSettingValue(value)
		}

		// The documented Config document carries MCP servers in an "mcp" block;
		// the flat mcp.<name> rows are that block. Expose the reconstructed
		// block alongside the flat rows (SPEC-017) so settings.mcp.<name>.url is
		// readable as JSON too. It is derived per response and never persisted:
		// the store keeps only the flat rows the /mcp/{name}/* resolver reads.
		if mcp := mcpServersFromSettings(rows); len(mcp) > 0 {
			settings["mcp"] = mcp
		}

		// OpenCode v1.18.29 attach reads provider_default before rendering the
		// TUI. Keep the mapping present even when no default is configured, and
		// derive Consensus's entry from the same setting returned above.
		providerDefaults := make(map[string]string)
		if defaultModel := values["llm.default_model"]; defaultModel != "" {
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
		settings, err := flattenedConfigSettings(req)
		if err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		if err := s.persistConfigSettings(r.Context(), settings); err != nil {
			writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update config")
			return
		}
		writeJSON(w, map[string]any{"status": "updated", "keys": len(settings)})
		return
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or PATCH")
	}
}

type configSetting struct {
	key   string
	value string
}

// flattenedConfigSettings translates opencode's documented
// {"mcp":{"<name>":{...}}} Config shape into the flat settings rows used by
// the /mcp/{name}/* resolver. Existing dotted keys remain valid. MCP config
// objects are stored as JSON so GET /config can return a readable value that
// can be PATCHed back without losing the server definition.
func flattenedConfigSettings(patch map[string]any) ([]configSetting, error) {
	flattened := make(map[string]any, len(patch))
	for key, value := range patch {
		if key != "mcp" {
			flattened[key] = value
		}
	}

	if value, ok := patch["mcp"]; ok {
		servers, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config key %q must be an object keyed by MCP server name", "mcp")
		}
		for name, server := range servers {
			// An explicitly supplied dotted key wins over its block-form twin.
			key := "mcp." + name
			if _, exists := flattened[key]; !exists {
				flattened[key] = server
			}
		}
	}

	keys := make([]string, 0, len(flattened))
	for key := range flattened {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	settings := make([]configSetting, 0, len(keys))
	for _, key := range keys {
		value := flattened[key]
		encoded := toString(value)
		if strings.HasPrefix(key, "mcp.") {
			switch value.(type) {
			case string, []byte:
			default:
				data, err := json.Marshal(value)
				if err != nil {
					return nil, fmt.Errorf("encode config key %q: %w", key, err)
				}
				encoded = string(data)
			}
		}
		settings = append(settings, configSetting{key: key, value: encoded})
	}
	return settings, nil
}

func (s *Server) persistConfigSettings(ctx context.Context, settings []configSetting) error {
	for _, setting := range settings {
		if err := s.db.Exec(ctx,
			`INSERT INTO system_settings (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = $2`,
			setting.key, setting.value,
		); err != nil {
			return err
		}
	}
	return nil
}

// globalMCPConfig reconstructs the documented block form from the flattened
// rows written by PATCH /config and PATCH /global/config. Legacy field-level
// rows (for example mcp.demo.type) are ignored because their scalar values are
// not complete MCP server config objects.
func (s *Server) globalMCPConfig(ctx context.Context) (map[string]any, error) {
	rows, err := s.db.Query(ctx,
		`SELECT key, value FROM system_settings WHERE key LIKE $1 ORDER BY key`,
		"mcp.%")
	if err != nil {
		return nil, err
	}
	return mcpServersFromSettings(rows), nil
}

// mcpServersFromSettings reconstructs the documented {"mcp":{"<name>":...}}
// block (SPEC-017) from the flat mcp.<name> system_settings rows that
// PATCH /config and PATCH /global/config persist. A row counts as a server
// definition only when its value is a JSON object: a legacy field-level row
// (mcp.<name>.<field>) carries a scalar, and a value written before
// DF-CONSENSUS-53 flattened MCP blocks is raw Go %v text — neither is a server
// definition, so neither is guessed into one.
func mcpServersFromSettings(rows []db.Row) map[string]any {
	servers := make(map[string]any)
	for _, row := range rows {
		name, ok := strings.CutPrefix(toString(row["key"]), "mcp.")
		if !ok || name == "" {
			continue
		}
		var config map[string]any
		if err := json.Unmarshal([]byte(toString(row["value"])), &config); err != nil || config == nil {
			continue
		}
		servers[name] = config
	}
	return servers
}

// configSettingValue renders one system_settings row value as the JSON value a
// client can use (DF-CONSENSUS-54). The store keeps every setting as TEXT, so a
// row whose text is a JSON object or array (the mcp.<name> server rows) is
// emitted as that native JSON — objects stay objects — while every other row
// keeps its raw text. A scalar setting such as "5" or "gpt-4o" is a string, not
// a JSON number or a JSON string literal, and text that is not JSON at all (the
// legacy raw Go %v rows) is returned unchanged rather than guessed at.
func configSettingValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return value
	}
	return decoded
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
// Consensus configuration lives in system_settings. Most global Config fields
// remain response-only because the runtime has no matching opencode store, but
// MCP servers are a shared surface: the /mcp/{name}/* route family resolves
// them from flat mcp.<name> settings rows. Therefore:
//
//	GET   reconstructs the documented {"mcp":{"<name>": ...}} block from
//	      complete flat MCP server rows and otherwise answers {} (never null).
//	PATCH validates the optional Config body, flattens each server in a
//	      top-level mcp block into one mcp.<name> settings row, and returns the
//	      accepted Config document in its original block form. Non-MCP fields
//	      remain response-only.
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
		mcp, err := s.globalMCPConfig(r.Context())
		if err != nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to read MCP config")
			return
		}
		config := make(map[string]any)
		if len(mcp) > 0 {
			config["mcp"] = mcp
		}
		writeJSON(w, config)
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
		if mcp, ok := patch["mcp"]; ok {
			settings, err := flattenedConfigSettings(map[string]any{"mcp": mcp})
			if err != nil {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
			if err := s.persistConfigSettings(r.Context(), settings); err != nil {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "failed to update MCP config")
				return
			}
		}
		// The resulting response keeps the documented block form even though its
		// MCP servers are persisted as flat mcp.<name> settings rows.
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

// handleCommandList serves GET /command — upstream command.list
// (ROUTE-ADD-063, source item SHIM-DRIFT-060, declared responses: 200
// Command[], 400 BadRequest). Command entries require name, template and hints;
// Consensus keeps no opencode slash-command registry, so the truthful payload
// is an empty array rather than fabricated commands. Valued directory/workspace
// selectors are well-formed and filter nothing; present-but-blank selectors
// answer the declared 400. Non-GET methods follow the sibling 405 convention.
//
// ch:trace row=ROUTE-ADD-063 spec=specs/openapi/upstream/openapi-1.18.33.json#command.list test=internal/shim/opencode/command_list_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:unattended-worker-session
func (s *Server) handleCommandList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, []map[string]any{})
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

// handleConsoleOrgs serves GET /experimental/console/orgs — upstream
// experimental.console.listOrgs (ROUTE-ADD-066, source item SHIM-DRIFT-063,
// declared responses: 200 {orgs: ConsoleOrg[]}, 400 BadRequestError,
// 500 InternalServerError). Before this change the path had no shim route at
// all, so GET /experimental/console/orgs fell through to net/http's default
// 404 — the declared-vs-served artifact's NOT-SERVED class, indistinguishable
// from "no such opencode operation".
//
// TRUTHFULNESS: the declared 200 orgs entries require accountID, accountEmail,
// accountUrl, orgID, orgName and active — they describe Console accounts this
// runtime is not logged in to and holds no registry for, so any entry would
// fabricate an account and an org. The truthful payload is the declared
// object shape with an empty, non-null `orgs` array (the same empty-registry
// answer the sibling /command, /skill and /formatter routes give), never null
// and never an invented org. The declared 400 is served for malformed input: a
// present-but-blank declared directory/workspace query selector answers the
// declared BadRequestError envelope (name BadRequest, data.kind Query). The
// declared 500 InternalServerError arm is a genuine-failure arm of the upstream
// Console client; this handler holds no Console backend that can fail and its
// answer is constant, so it never fabricates a 5xx — an invented 500 would be
// exactly the undeclared-behaviour lie the served-surface vocabulary forbids.
// Non-GET answers 405 METHOD_NOT_ALLOWED — sibling method-guard convention,
// matching handleCommandList/handleFormatter/handleSkill.
//
// ch:trace row=ROUTE-ADD-066 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.console.listOrgs test=internal/shim/opencode/console_orgs_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:unattended-worker-session
func (s *Server) handleConsoleOrgs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, map[string]any{"orgs": []map[string]any{}})
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

// handleExperimentalCapabilities serves GET /experimental/capabilities —
// upstream experimental.capabilities.get (ROUTE-ADD-064, declared responses:
// 200 ExperimentalCapabilities, 400 BadRequestError). The declared
// ExperimentalCapabilities object has exactly one required member,
// backgroundSubagents: boolean (additionalProperties: false).
//
// The answer is a build-level capability, not per-request data: this runtime
// DOES implement sub-agent spawning (SPEC-004) — a spawn is queued as a task
// row that the harness claim loop picks up and executes asynchronously, with
// sessions.parent_id lineage, the wake_parent_on_completion notification and
// the subagent_max_depth setting (default 5) — so the truthful value is true.
// It is not tied to a store or a runtime toggle that could make it
// instance-dependent, and no other declared member exists to report.
//
// The operation declares optional directory/workspace query selectors for
// workspace scoping; the capability answer is workspace-independent, so a
// valued selector is well-formed and filters nothing, while a
// present-but-blank one is malformed input and answers the declared
// BadRequestError body (name "BadRequest", data.kind "Query", the offending
// parameter named in data.message). Undeclared methods answer the sibling
// METHOD_NOT_ALLOWED 405 envelope (handleTools/handleFormatter convention).
//
// ch:trace row=ROUTE-ADD-064 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.capabilities.get test=internal/shim/opencode/experimental_capabilities_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:unattended-worker-session
func (s *Server) handleExperimentalCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, map[string]any{"backgroundSubagents": true})
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

// handleExperimentalConsole serves GET /experimental/console — upstream
// experimental.console.get (ROUTE-ADD-065; source item SHIM-DRIFT-062 in the
// 2026-09-29 baseline report, positional id SHIM-DRIFT-033 in the regenerated
// artifact; declared responses 200 ConsoleState, 400 BadRequestError, 500
// InternalServerError). Before this change the exact path had no shim route at
// all, so every method fell through to net/http's default 404.
//
// TRUTHFULNESS — this shim integrates with no opencode Console (the cloud
// surface that manages providers and orgs), so it reports the runtime's real
// state instead of inventing one:
//
//   - consoleManagedProviders is READ from the instance's settings store
//     (system_settings keys console.<providerID>[.<field>] — the same store and
//     naming convention PUT /auth/{providerID} uses for credentials) and
//     deduplicated per provider id. This runtime ships no Console integration,
//     so no such row exists and the list is served empty (never null, never a
//     provider the store does not hold); a binding an operator stores is
//     reported as stored.
//   - switchableOrgCount is 0: the instance joins no org directory and offers
//     no org switch, so 0 is the runtime truth rather than a store-derived
//     claim (the operation declares the member required with minimum 0).
//   - activeOrgName is optional in the pinned schema and omitted — this
//     instance has no active org, and an empty string would assert an org with
//     a blank name.
//
// An unreadable settings store answers the declared 500 InternalServerError
// {_tag:"InternalServerError"}: absence is never claimed from a store the shim
// could not read (the handleProviderGet rule), and the declared 500 schema is
// additionalProperties:false, so the body carries that one member.
//
// The operation declares the optional directory/workspace query selectors as
// plain strings, so a present-but-blank selector is malformed input and answers
// the declared BadRequestError body through writeOpencodeBadRequest (the
// sibling handleSyncHistory convention) without reading the store. A valued
// selector is well-formed and keeps the 200: the state this instance reports is
// workspace-independent.
//
// Non-GET answers 405 METHOD_NOT_ALLOWED — sibling method-guard convention
// (handleFormatter/handleSkill). Exact path only, no catch-all: the sibling
// /experimental/tool and /experimental/tool/ids routes are separate declared
// operations, and deeper /experimental/* sub-paths keep their own rows (or the
// net/http default 404).
func (s *Server) handleExperimentalConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := r.URL.Query()[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	rows, err := s.db.Query(r.Context(),
		`SELECT key FROM system_settings WHERE key LIKE 'console.%' ORDER BY key`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"_tag": "InternalServerError"})
		slog.Warn("opencode-shim: console state unavailable", "error", err)
		return
	}

	providers := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := toString(row["key"])
		providerID, ok := strings.CutPrefix(key, "console.")
		if !ok {
			continue
		}
		// console.<providerID>.<field> rows describe the same binding as the
		// console.<providerID> row itself, so only the first segment names the
		// provider.
		if i := strings.Index(providerID, "."); i >= 0 {
			providerID = providerID[:i]
		}
		if providerID == "" {
			continue
		}
		if _, dup := seen[providerID]; dup {
			continue
		}
		seen[providerID] = struct{}{}
		providers = append(providers, providerID)
	}

	writeJSON(w, map[string]any{
		"consoleManagedProviders": providers,
		"switchableOrgCount":      0,
	})
}

// handleConsoleSwitch serves POST /experimental/console/switch — upstream
// experimental.console.switchOrg (ROUTE-ADD-067, source item SHIM-DRIFT-035 in
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json; contract
// specs/024-shim-support-matrix.md §A). Upstream persists a new active Console
// account/org selection in the local OpenCode state; the Consensus shim is an
// in-process protocol translation surface that keeps no Console account/org
// store and holds no upstream Console backend, so it answers the declared 200
// with the truthful boolean false — the switch was not performed — instead of
// fabricating success (the tui.showToast / provider.oauth.authorize truthful
// boolean-false precedent). A parent chi mount already reaches this subtree
// through the /experimental/* MountPatterns entry; only the exact mux path was
// missing, so every method answered the net/http default 404 before this row.
//
// Input handling follows the declared schema. The declared requestBody is the
// object {accountID*: string, orgID*: string} with additionalProperties: false;
// the pinned document does not mark requestBody required, so an absent body is
// well-formed (the handleGlobalUpgrade convention), while a body that is
// present must decode as exactly one JSON object with only those two fields —
// unknown fields are refused through decodeDeclaredJSONObject (SHIM-GAP-004),
// and a missing, non-string or blank accountID/orgID is refused too. Malformed
// input answers the sibling 400 BadRequest envelope (writeOpencodeBadRequest,
// kind Body) rather than silently accepting a selection the caller did not
// state. The declared optional directory/workspace query selectors are
// accepted, and a present-but-blank value answers the same 400 with kind Query
// (the projectUpdate selector convention). Non-POST answers 405
// METHOD_NOT_ALLOWED — sibling method-guard convention (handleLog,
// handleGlobalUpgrade).
func (s *Server) handleConsoleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if values, present := r.URL.Query()[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query", fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	if r.Body != nil && r.ContentLength != 0 {
		var body map[string]json.RawMessage
		if !decodeDeclaredJSONObject(w, r, &body,
			`request body must be a JSON object containing only "accountID" and "orgID"`) {
			return
		}
		if body == nil {
			writeOpencodeBadRequest(w, r, "Body",
				`request body must be a JSON object containing only "accountID" and "orgID"`)
			return
		}
		for field := range body {
			if field != "accountID" && field != "orgID" {
				writeOpencodeBadRequest(w, r, "Body",
					fmt.Sprintf("unknown field %q: the declared body schema allows only \"accountID\" and \"orgID\"", field))
				return
			}
		}
		for _, field := range []string{"accountID", "orgID"} {
			raw, ok := body[field]
			if !ok {
				writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("required field %q is missing", field))
				return
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("field %q must be a string", field))
				return
			}
			if strings.TrimSpace(value) == "" {
				writeOpencodeBadRequest(w, r, "Body", fmt.Sprintf("field %q must not be blank", field))
				return
			}
		}
	}
	// Truthful answer: this shim switches no Console org (no Console backend,
	// no account/org store), so the declared boolean success marker is false.
	writeJSON(w, false)
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

// handleMCPSub serves the /mcp/* sub-paths. Six operations of the declared mcp
// family are served here, dispatched on the parsed (sub-path shape, method)
// pair:
//
//	DELETE /mcp/{name}/auth              mcp.auth.remove       (ROUTE-ADD-093)
//	POST   /mcp/{name}/auth              mcp.auth.start        (ROUTE-ADD-094)
//	POST   /mcp/{name}/auth/authenticate mcp.auth.authenticate (ROUTE-ADD-095)
//	POST   /mcp/{name}/auth/callback     mcp.auth.callback     (ROUTE-ADD-096)
//	POST   /mcp/{name}/connect           mcp.connect           (ROUTE-ADD-097)
//	POST   /mcp/{name}/disconnect        mcp.disconnect        (ROUTE-ADD-098)
//
// Every other /mcp/* shape — a wrong method on a served shape and every shape
// outside the declared family — answers http.NotFound: the same 404 body
// net/http's default handler produced before this route existed, so an
// undeclared method keeps its pre-change answer byte-for-byte (405 is not part
// of any of these operations' declared response sets — the same choice
// handleProviderAuth made for its undeclared methods, ROUTE-ADD-099). This
// stays a shape check, never a /mcp/* catch-all.
func (s *Server) handleMCPSub(w http.ResponseWriter, r *http.Request) {
	name, sub, ok := parseMCPSubPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		switch sub {
		case "auth":
			s.handleMCPAuthStart(w, r, name)
			return
		case "auth/authenticate":
			s.handleMCPAuthAuthenticate(w, r, name)
			return
		case "auth/callback":
			s.handleMCPAuthCallback(w, r, name)
			return
		case "connect":
			s.handleMCPConnect(w, r, name)
			return
		case "disconnect":
			s.handleMCPDisconnect(w, r, name)
			return
		}
	} else if sub == "auth" && r.Method == http.MethodDelete {
		s.handleMCPAuthRemove(w, r, name)
		return
	}
	http.NotFound(w, r)
}

// parseMCPSubPath splits /mcp/{name}/{sub}. ok is true only for the declared
// shapes: a non-empty name and one or two non-empty trailing segments (so
// /mcp/{name} is not one of these operations, /mcp/{name}/auth/callback/extra
// is not either, and the deeper two-segment sub-path — auth/authenticate,
// auth/callback — is what ROUTE-ADD-095/096 add to the family). The returned
// sub is the joined trailing path, so a caller dispatches on the full shape.
func parseMCPSubPath(path string) (name, sub string, ok bool) {
	rest, found := strings.CutPrefix(path, "/mcp/")
	if !found {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" {
			return "", "", false
		}
	}
	return parts[0], strings.Join(parts[1:], "/"), true
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

// ---------------------------------------------------------------------------
// MCP management: OAuth lifecycle and connection (ROUTE-ADD-094..098,
// SHIM-DRIFT-085..089)
// ---------------------------------------------------------------------------
//
// The shim keeps no MCP server registry of its own: it exposes Consensus AS an
// MCP server (the bare /mcp mount) and holds no MCP client, no OAuth flow
// engine and no connection tracker for the servers a client configures under
// the upstream config's mcp: {"<name>": ...} block. It does own a settings
// store — the one GET/PATCH /config reads and writes — so a server NAME is
// resolved exactly the way handleMCPAuthRemove established (ROUTE-ADD-093): a
// "mcp.<name>" key or any key under "mcp.<name>.". A name with no stored entry
// names no MCP server the shim knows and answers the declared typed
// McpServerNotFoundError 404.
//
// Five declared operations answer from that one resolution, each in the shape
// its own contract declares, and none of them invents a success the runtime did
// not perform:
//
//   - mcp.auth.start and mcp.auth.authenticate need a live OAuth flow: start
//     must answer a real {authorizationUrl, oauthState} pair (both required,
//     additionalProperties: false) and authenticate opens a browser and waits
//     for the callback. The shim mints no authorization URL and opens no
//     browser, so their declared 200 is unreachable — a fabricated pair would
//     be a lie the caller then hands to a browser. Both answer their declared
//     400 McpUnsupportedOAuthError {"error": ...}: the document's own shape for
//     "OAuth is not supported here".
//   - mcp.auth.callback exchanges an authorization code against the flow the
//     matching start/authenticate call parked. The shim parks no flow, so there
//     is nothing to exchange a code against: it refuses with the declared 400
//     InvalidRequestError {"_tag", "message"} — the second declared 400 shape of
//     that operation, whose 400 set carries no McpUnsupportedOAuthError.
//   - mcp.connect and mcp.disconnect answer the DECLARED 200 boolean with the
//     truthful false: the shim holds no MCP client connection, so no connect
//     and no disconnect was performed (false = "this call did not do it"), the
//     sibling handleSyncStart / tuiShowToast / provider.oauth.callback
//     convention. Never the declared literal true.
//
// The declared request surface is validated strictly before any of that: a
// present-but-blank declared query selector (directory, workspace), a
// whitespace-only {name}, and mcp.auth.callback's request body (a single JSON
// object carrying the required string `code` with no undeclared field — its
// schema is additionalProperties: false) are all refused. A settings store that
// cannot be read answers the declared 400 arm rather than an undeclared 5xx:
// none of these operations declares a 5xx.
//
// A wrong method or an out-of-family shape never reaches these handlers —
// handleMCPSub answers http.NotFound, keeping the pre-change answer
// byte-for-byte.

// resolveMCPServerName reports whether name names an MCP server the shim knows.
// The entry is resolved from the settings store under the flat translation of
// the upstream config's mcp: {"<name>": ...} block — a key "mcp.<name>" or any
// key under "mcp.<name>." — the convention handleMCPAuthRemove established
// (ROUTE-ADD-093). The second return is a store read failure, which every
// caller maps onto its operation's declared 400 arm (never an undeclared 5xx).
func (s *Server) resolveMCPServerName(ctx context.Context, name string) (bool, error) {
	// Escape LIKE metacharacters so a name containing %, _ or \ resolves only
	// its own rows — the same treatment handleMCPAuthRemove gives a name.
	pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(name)
	rows, err := s.db.Query(ctx,
		`SELECT key FROM system_settings WHERE key = $1 OR key LIKE $2 ESCAPE '\'`,
		"mcp."+name, "mcp."+pattern+".%")
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// mcpNotFoundBody answers the declared typed 404 arm shared by the whole
// family: McpServerNotFoundError {_tag, name, message} (additionalProperties:
// false), never a bare 404.
func mcpNotFoundBody(w http.ResponseWriter, name string) {
	writeOpencodeNotFoundError(w, "McpServerNotFoundError", "name", name,
		"MCP server not found: "+name)
}

// writeMCPUnsupportedOAuthError answers the declared 400 McpUnsupportedOAuthError
// shape ({"error": <message>}, additionalProperties: false) — the shape
// mcp.auth.start and mcp.auth.authenticate declare for "OAuth is not supported
// here", which is this runtime's truthful class for both: no MCP OAuth flow
// exists in the shim.
func writeMCPUnsupportedOAuthError(w http.ResponseWriter, r *http.Request, message string) {
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": message})
	slog.Warn("opencode-shim: mcp oauth unsupported", "method", r.Method, "path", r.URL.Path)
}

// writeOpencodeInvalidRequestError answers the declared 400 InvalidRequestError
// shape ({"_tag": "InvalidRequestError", "message": ...}, additionalProperties:
// false — the optional kind/field members are omitted where a refusal is not
// attributable to one named field). It is the second declared 400 shape of the
// MCP auth operations, and the shape the sibling handleProviderOAuthAuthorize /
// handleProviderOAuthCallback store-failure arms already emit.
func writeOpencodeInvalidRequestError(w http.ResponseWriter, message string) {
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{
		"_tag":    "InvalidRequestError",
		"message": message,
	})
}

// writeMCPBadRequestError answers the declared 400 BadRequestError shape
// ({"name":"BadRequest","data":{"message":...,"kind":...}}) — the only declared
// 400 shape of mcp.connect and mcp.disconnect. kind names the request part at
// fault (Params for the path-parameter {name}, Query for a blank declared
// selector) and is omitted for a refusal that is not attributable to a request
// part (a settings-store read failure); the declared schema marks kind optional.
func writeMCPBadRequestError(w http.ResponseWriter, r *http.Request, kind, message string) {
	data := map[string]any{"message": message}
	if kind != "" {
		data["kind"] = kind
	}
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{
		"name": "BadRequest",
		"data": data,
	})
	slog.Warn("opencode-shim: mcp bad request", "method", r.Method, "path", r.URL.Path, "kind", kind)
}

// blankMCPQueryParam returns the name of the first declared query selector
// (directory, workspace) that is present with a blank value, or "" when none
// is — the handleSkill/handleSyncStart convention every fixed-workspace shim
// operation follows (the declared 400 arm of an operation that declares no
// requestBody would otherwise be unreachable).
func blankMCPQueryParam(r *http.Request) string {
	for _, param := range []string{"directory", "workspace"} {
		if v, present := r.URL.Query()[param]; present && (len(v) == 0 || strings.TrimSpace(v[0]) == "") {
			return param
		}
	}
	return ""
}

// handleMCPAuthStart serves POST /mcp/{name}/auth — upstream mcp.auth.start
// (ROUTE-ADD-094, SHIM-DRIFT-085, declared responses: 200 {authorizationUrl,
// oauthState}, 400 McpUnsupportedOAuthError | InvalidRequestError, 404
// McpServerNotFoundError), "Start OAuth authentication flow for a Model Context
// Protocol (MCP) server."
//
// The declared 200 schema requires BOTH authorizationUrl and oauthState as
// strings with additionalProperties: false, and the declared 400 carries
// McpUnsupportedOAuthError — the document's own shape for "this server's OAuth
// is not supported". A flow this runtime cannot run cannot produce either
// string, so a server the shim resolves answers the declared 400
// McpUnsupportedOAuthError: there is no MCP OAuth flow here. Never a 200 with
// an invented URL, never an undeclared 5xx.
//
// Arms: a whitespace-only {name} or a present-but-blank declared query selector
// answers 400 InvalidRequestError (the operation's second declared 400 shape —
// malformed input is never reported as "OAuth unsupported"); an unreadable
// settings store answers 400 InvalidRequestError rather than an undeclared 500;
// a name with no stored MCP entry answers the typed 404 McpServerNotFoundError;
// otherwise the declared 400 McpUnsupportedOAuthError. A non-POST method never
// reaches this handler (handleMCPSub keeps the pre-change 404).
func (s *Server) handleMCPAuthStart(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeOpencodeInvalidRequestError(w, "MCP server name is required")
		return
	}
	if blank := blankMCPQueryParam(r); blank != "" {
		writeOpencodeInvalidRequestError(w, fmt.Sprintf("query parameter %q must not be blank", blank))
		return
	}
	known, err := s.resolveMCPServerName(r.Context(), name)
	if err != nil {
		writeOpencodeInvalidRequestError(w, "failed to read MCP server settings")
		return
	}
	if !known {
		mcpNotFoundBody(w, name)
		return
	}
	writeMCPUnsupportedOAuthError(w, r, fmt.Sprintf(
		"MCP OAuth is not supported for server %q: this runtime implements no MCP OAuth flow and cannot mint an authorization URL", name))
}

// handleMCPAuthAuthenticate serves POST /mcp/{name}/auth/authenticate — upstream
// mcp.auth.authenticate (ROUTE-ADD-095, SHIM-DRIFT-086, declared responses: 200
// MCPStatus, 400 McpUnsupportedOAuthError | InvalidRequestError, 404
// McpServerNotFoundError), "Start OAuth flow and wait for callback (opens
// browser)."
//
// The declared 200 is an MCPStatus — the live status of that server's MCP
// connection (connected / disabled / failed / needs_auth /
// needs_client_registration). The shim runs no MCP client and tracks no server
// connection, so it cannot report one: every MCPStatus member would be invented
// (even needs_auth asserts a state this runtime never observed), and the
// operation's own declared 400 set carries McpUnsupportedOAuthError for exactly
// the "cannot run the flow" case this operation is in — it cannot start a flow
// nor open a browser. A resolved server therefore answers the declared 400
// McpUnsupportedOAuthError. Never a fabricated status, never an undeclared 5xx.
//
// Arms are identical to handleMCPAuthStart (the same declared 200/400/404 set):
// whitespace-only name / blank declared selector → 400 InvalidRequestError;
// unreadable store → 400 InvalidRequestError; unknown name → typed 404
// McpServerNotFoundError; otherwise 400 McpUnsupportedOAuthError.
func (s *Server) handleMCPAuthAuthenticate(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeOpencodeInvalidRequestError(w, "MCP server name is required")
		return
	}
	if blank := blankMCPQueryParam(r); blank != "" {
		writeOpencodeInvalidRequestError(w, fmt.Sprintf("query parameter %q must not be blank", blank))
		return
	}
	known, err := s.resolveMCPServerName(r.Context(), name)
	if err != nil {
		writeOpencodeInvalidRequestError(w, "failed to read MCP server settings")
		return
	}
	if !known {
		mcpNotFoundBody(w, name)
		return
	}
	writeMCPUnsupportedOAuthError(w, r, fmt.Sprintf(
		"MCP OAuth is not supported for server %q: this runtime implements no MCP OAuth flow, opens no browser and tracks no connection status", name))
}

// handleMCPAuthCallback serves POST /mcp/{name}/auth/callback — upstream
// mcp.auth.callback (ROUTE-ADD-096, SHIM-DRIFT-087, declared responses: 200
// MCPStatus, 400 BadRequest | InvalidRequestError, 404
// McpServerNotFoundError), "Complete OAuth authentication for a Model Context
// Protocol (MCP) server using the authorization code."
//
// The declared 200 is an MCPStatus the shim cannot report (see
// handleMCPAuthAuthenticate) and the exchange itself needs the OAuth flow the
// matching start/authenticate call parked — which this runtime never parks. A
// well-formed callback for a resolved server therefore answers the declared 400
// InvalidRequestError naming the real reason: no flow is pending, so the code
// has nothing to be exchanged against. (Its 400 set carries no
// McpUnsupportedOAuthError, so the second declared shape is the one used.)
//
// The declared request body is validated first: a single JSON object with the
// required string `code` and no undeclared field (additionalProperties: false),
// absent/empty bodies included (the document does not mark requestBody
// required, but `code` is the operation's only declared field and it is
// required, so an absent body cannot complete anything — the
// parseProviderOAuthCallbackBody direction). The declared schema puts no
// minLength/pattern on `code`, so an empty string is schema-valid and takes the
// same no-pending-flow answer as any other well-formed code.
//
// Remaining arms: whitespace-only name / blank declared selector → 400
// InvalidRequestError; unreadable store → 400 InvalidRequestError (never an
// undeclared 5xx); unknown name → typed 404 McpServerNotFoundError; non-POST
// never reaches this handler (handleMCPSub keeps the pre-change 404).
func (s *Server) handleMCPAuthCallback(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeOpencodeInvalidRequestError(w, "MCP server name is required")
		return
	}
	if blank := blankMCPQueryParam(r); blank != "" {
		writeOpencodeInvalidRequestError(w, fmt.Sprintf("query parameter %q must not be blank", blank))
		return
	}
	if reason := parseMCPAuthCallbackBodyRejection(r); reason != "" {
		writeOpencodeInvalidRequestError(w, reason)
		return
	}
	known, err := s.resolveMCPServerName(r.Context(), name)
	if err != nil {
		writeOpencodeInvalidRequestError(w, "failed to read MCP server settings")
		return
	}
	if !known {
		mcpNotFoundBody(w, name)
		return
	}
	writeOpencodeInvalidRequestError(w, fmt.Sprintf(
		"no MCP OAuth flow is pending for server %q, so the authorization code cannot be exchanged", name))
}

// parseMCPAuthCallbackBodyRejection validates mcp.auth.callback's declared
// request body — a single JSON object carrying the required string `code` with
// no undeclared field (the pinned schema is additionalProperties: false) — and
// returns a human-readable refusal reason, or "" when the body is well-formed.
// It mirrors parseProviderOAuthCallbackBody, including the trailing-second-value
// rejection the sibling handleSyncSteal applies.
func parseMCPAuthCallbackBodyRejection(r *http.Request) string {
	if r.Body == nil || r.ContentLength == 0 {
		return `request body must carry the authorization "code"`
	}
	dec := json.NewDecoder(r.Body)
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return "malformed request body: " + err.Error()
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "request body must contain exactly one JSON value"
	}
	body, ok := raw.(map[string]any)
	if !ok {
		return "request body must be a JSON object carrying the authorization code"
	}
	for key := range body {
		if key != "code" {
			return fmt.Sprintf("request body carries undeclared field %q", key)
		}
	}
	value, present := body["code"]
	if !present {
		return `request body is missing the required "code" field`
	}
	if _, ok := value.(string); !ok {
		return `"code" must be a string`
	}
	return ""
}

// handleMCPConnect serves POST /mcp/{name}/connect — upstream mcp.connect
// (ROUTE-ADD-097, SHIM-DRIFT-088, declared responses: 200 boolean "MCP server
// connected successfully", 400 BadRequestError, 404 McpServerNotFoundError),
// "Connect an MCP server."
//
// The shim keeps no MCP client connection (it serves MCP, it does not
// consume it), so no connection was established by this call and the declared
// boolean is answered as the truthful false — the sibling handleSyncStart
// convention ("no sync was started" -> false), never the declared literal true.
// A fabricated true would claim a live connection that no code in this runtime
// holds.
//
// Arms: a whitespace-only {name} → 400 BadRequestError (name the request part:
// the path parameter); a present-but-blank declared query selector → 400
// BadRequestError kind Query; an unreadable settings store → 400
// BadRequestError (the operation declares no 5xx and no other 400 shape);
// an unknown name → typed 404 McpServerNotFoundError; otherwise 200 false.
// Non-POST never reaches this handler (handleMCPSub keeps the pre-change 404).
func (s *Server) handleMCPConnect(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeMCPBadRequestError(w, r, "Params", "MCP server name is required")
		return
	}
	if blank := blankMCPQueryParam(r); blank != "" {
		writeMCPBadRequestError(w, r, "Query", fmt.Sprintf("query parameter %q must not be blank", blank))
		return
	}
	known, err := s.resolveMCPServerName(r.Context(), name)
	if err != nil {
		writeMCPBadRequestError(w, r, "", "failed to read MCP server settings")
		return
	}
	if !known {
		mcpNotFoundBody(w, name)
		return
	}
	// No MCP client connection was established: the declared boolean reports
	// false rather than a fabricated true.
	writeJSON(w, false)
}

// handleMCPDisconnect serves POST /mcp/{name}/disconnect — upstream
// mcp.disconnect (ROUTE-ADD-098, SHIM-DRIFT-089, declared responses: 200
// boolean "MCP server disconnected successfully", 400 BadRequestError, 404
// McpServerNotFoundError), "Disconnect an MCP server."
//
// The shim holds no MCP client connection, so this call disconnected nothing
// and the declared boolean reports the truthful false — never the declared
// literal true, which would claim a teardown that no code in this runtime
// performed (the handleMCPConnect / handleSyncStart convention).
//
// Arms are identical to handleMCPConnect: whitespace-only name → 400
// BadRequestError kind Params; blank declared selector → 400 BadRequestError
// kind Query; unreadable store → 400 BadRequestError; unknown name → typed 404
// McpServerNotFoundError; otherwise 200 false. Non-POST never reaches this
// handler (handleMCPSub keeps the pre-change 404).
func (s *Server) handleMCPDisconnect(w http.ResponseWriter, r *http.Request, name string) {
	if strings.TrimSpace(name) == "" {
		writeMCPBadRequestError(w, r, "Params", "MCP server name is required")
		return
	}
	if blank := blankMCPQueryParam(r); blank != "" {
		writeMCPBadRequestError(w, r, "Query", fmt.Sprintf("query parameter %q must not be blank", blank))
		return
	}
	known, err := s.resolveMCPServerName(r.Context(), name)
	if err != nil {
		writeMCPBadRequestError(w, r, "", "failed to read MCP server settings")
		return
	}
	if !known {
		mcpNotFoundBody(w, name)
		return
	}
	// Nothing was connected by this shim, so nothing was disconnected: the
	// declared boolean reports false rather than a fabricated true.
	writeJSON(w, false)
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
		s.permissionReply(w, r, permID)
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

// permissionReply serves POST /permission/{requestID}/reply — upstream
// permission.reply (SHIM-GAP-004; declared responses: 200 boolean "Permission
// processed successfully", 400 BadRequest | InvalidRequestError, 404
// PermissionNotFoundError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/permission/{requestID}/reply".post): requestID is a path parameter
// with declared pattern ^per; the requestBody is {reply*: "once" | "always" |
// "reject", message?: string} with additionalProperties: false — reply is
// REQUIRED; the declared 200 body is a plain boolean.
//
// Before SHIM-GAP-004 this operation served ONLY the declared 404 (a typed
// PermissionNotFoundError for every well-formed id), so the declared 200 and
// 400 were unreachable from outside: a client that can only ever receive an
// error cannot use the feature, and the narrowing was invisible to the
// declared-vs-served comparison (which saw a declared code being answered). The
// runtime already owned the state and the write path — POST /permission/{id}/resolve
// (ROUTE-FIX-045) resolves the same approval_requests rows — so the declared
// verb now resolves the REAL row through the same mapping.
//
// Validation order: an ill-shaped requestID (declared pattern ^per) answers the
// declared 400; a body that is not exactly one JSON object carrying only the
// declared fields answers the declared 400; a missing/empty/non-enum reply
// answers the declared 400; an unknown id, or a row that is no longer pending,
// answers the declared 404 (the resolvePermission convention — this operation
// declares no 409). The declared enum maps onto the REAL approval_requests
// columns exactly as sessionPermissionRespond does (reject -> 'rejected';
// once/always -> 'approved' with the grant scope in review_notes — there is no
// decision_reason column), the optional declared `message` is the reviewer's
// note, a resolved row emits the same approval_resolved SSE event the sibling
// handlers emit, and only a row that was actually resolved answers the declared
// boolean true — never a no-op success. The operation declares no 5xx, so a
// failed store write answers the declared 400 InvalidRequestError naming the
// failure rather than an undeclared 500 (the mcp.auth.* convention).
func (s *Server) permissionReply(w http.ResponseWriter, r *http.Request, permID string) {
	if !strings.HasPrefix(permID, "per") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"requestID must match the declared pattern ^per")
		return
	}
	var req struct {
		Reply   *string `json:"reply"`
		Message *string `json:"message"`
	}
	if !decodeDeclaredJSONObject(w, r, &req, "request body must be an object containing only reply, message") {
		return
	}
	switch {
	case req.Reply == nil || *req.Reply == "":
		writeOpencodeBadRequest(w, r, "Payload",
			`required field "reply" must be one of "once", "always", "reject"`)
		return
	case *req.Reply != "once" && *req.Reply != "always" && *req.Reply != "reject":
		writeOpencodeBadRequest(w, r, "Payload",
			`field "reply" must be one of "once", "always", "reject"`)
		return
	}

	ctx := r.Context()
	row, err := s.db.QueryRow(ctx,
		`SELECT session_id, status FROM approval_requests WHERE id = $1`, permID)
	if err != nil || row == nil {
		writeUpstreamRequestNotFound(w, "PermissionNotFoundError", permID,
			"Permission request not found: "+permID)
		return
	}
	if toString(row["status"]) != "pending" {
		writeUpstreamRequestNotFound(w, "PermissionNotFoundError", permID,
			"Permission request is no longer pending: "+permID)
		return
	}

	decision := "approved"
	note := "granted (always)"
	switch *req.Reply {
	case "reject":
		decision = "rejected"
		note = "denied by permission.reply"
	case "once":
		note = "granted once"
	}
	if req.Message != nil && strings.TrimSpace(*req.Message) != "" {
		note += " — " + *req.Message
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.Exec(ctx,
		`UPDATE approval_requests
		 SET status = $1, review_notes = $2, reviewed_at = $3, reviewer_id = 'opencode-shim'
		 WHERE id = $4 AND status = 'pending'`,
		decision, note, now, permID,
	); err != nil {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"failed to resolve permission: "+err.Error())
		return
	}

	// The same SSE event the sibling resolve handlers emit (HARDEN-SHIM-01).
	s.emitShimEventForSession(toString(row["session_id"]), "approval_resolved", map[string]any{
		"approval_id": permID,
		"status":      decision,
	})

	// The permission WAS resolved: the declared boolean reports the truthful
	// true.
	writeJSON(w, true)
}

// decodeDeclaredJSONObject decodes a declared requestBody whose schema sets
// additionalProperties: false: exactly one JSON object, only the declared
// fields, one JSON value. message is the caller's BadRequest text; kind "Body"
// is the upstream BadRequest payload used for malformed/non-object JSON
// (writeOpencodeBadRequest documents the Body/Payload split). decodeTUIJSONBody
// is the TUI-side name for this same decoder.
func decodeDeclaredJSONObject(w http.ResponseWriter, r *http.Request, dst any, message string) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeOpencodeBadRequest(w, r, "Body", message)
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeBadRequest(w, r, "Body", "request body must contain exactly one JSON value")
		return false
	}
	return true
}

// ============================================================================
// TUI Control Endpoints (SPEC-017 §3.1 — shim-only passthrough)
// ============================================================================

// tuiDeclaredOps is retained as the typed fallback for future pinned-upstream
// TUI operations that are declared but not yet translated. Every currently
// declared operation formerly listed here is now served by a truthful handler.
//
// The shim-only actions (submit-prompt, execute-command) keep their
// pre-existing 501 error body and tuiAction stub contract. They are deliberately
// outside this map and are not part of ROUTE-FIX-023; show-toast joined the
// pinned-upstream handlers in ROUTE-FIX-029.
var tuiDeclaredOps = map[string]string{}

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
//	POST /pty — upstream pty.create (ROUTE-ADD-103, source item
//	     SHIM-DRIFT-106): creating terminals is intentionally unsupported
//	     (SPEC-017 §3.9). Consensus exposes no raw PTY lifecycle or process
//	     registry, so a declared 200 Pty body would fabricate a spawned process.
//	     The typed not_implemented envelope names the operation. It must stay
//	     typed and never 404: registering the bare path means POST reaches this
//	     handler rather than net/http's default, and a bodyless 501 or a 404
//	     would reintroduce the dishonesty SHIM-GAP-002 removed. The declared-
//	     vs-served artifact records this as an explicit intentional narrowing,
//	     not as full upstream success coverage.
//
// Non-GET/POST methods on the bare route answer the sibling
// METHOD_NOT_ALLOWED envelope. The /pty/{ptyID} sub-path family (pty.get,
// pty.update, pty.remove, pty.connect, pty.connectToken) is served by
// handlePtyByID / handlePtyConnect / handlePtyConnectToken (ROUTE-ADD-105..109,
// see pty_routes.go); /pty/shells is the other declared child route registered
// separately above. Auth is untouched: PTY routes are not stub paths and carry
// no fixed-workspace exemption, so they keep the standard api-key policy.
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

// tuiClearPrompt serves tui.clearPrompt's declared boolean response. The shim
// has no attached TUI process, so a valid request returns false rather than
// claiming that prompt state was cleared. The optional directory/workspace
// selectors are validated against the filesystem, matching ptyList.
func (s *Server) tuiClearPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
			if info, err := os.Stat(v); err != nil || !info.IsDir() {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("%s %q does not exist or is not a directory", param, v))
				return
			}
		}
	}
	writeJSON(w, false)
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

// tuiControlNext answers upstream tui.control.next. The declared 200 body is
// {path: string, body: any} with both fields required. The shim runs no TUI
// process, so there is no "next" control to select and the truthful happy path
// is the EMPTY queue representation — path "" and body null, never a
// fabricated control object. The optional directory/workspace query selectors
// are validated against the filesystem first: naming a path that does not
// exist (or is not a directory) answers the declared 400 arm with the
// INVALID_REQUEST envelope, and a non-GET method answers the sibling
// METHOD_NOT_ALLOWED (ROUTE-FIX-021, SHIM-DRIFT-132 / drift row SHIM-DRIFT-108).
func (s *Server) tuiControlNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	for _, param := range []string{"directory", "workspace"} {
		if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
			if info, err := os.Stat(v); err != nil || !info.IsDir() {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("%s %q does not exist or is not a directory", param, v))
				return
			}
		}
	}
	writeJSON(w, map[string]any{"path": "", "body": nil})
}

// validateTUISelectors applies the directory/workspace filesystem contract used
// by tui.clearPrompt and pty.list. Empty selectors preserve the pinned optional
// parameter semantics; valued selectors must name an existing directory.
func validateTUISelectors(w http.ResponseWriter, r *http.Request) bool {
	for _, param := range []string{"directory", "workspace"} {
		if v := strings.TrimSpace(r.URL.Query().Get(param)); v != "" {
			if info, err := os.Stat(v); err != nil || !info.IsDir() {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("%s %q does not exist or is not a directory", param, v))
				return false
			}
		}
	}
	return true
}

// decodeTUIJSONBody decodes exactly one JSON value and emits the upstream
// BadRequest envelope on malformed or trailing input. It is the TUI-side name
// for decodeDeclaredJSONObject (SHIM-GAP-004), which every declared
// additionalProperties:false requestBody uses.
func decodeTUIJSONBody(w http.ResponseWriter, r *http.Request, dst any, message string) bool {
	return decodeDeclaredJSONObject(w, r, dst, message)
}

// rejectUndeclaredTUIBody enforces the four open-* operations' lack of a
// requestBody declaration. A client body cannot affect a detached TUI and is
// rejected rather than silently ignored.
func rejectUndeclaredTUIBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body != nil && r.ContentLength != 0 {
		writeOpencodeBadRequest(w, r, "Body", "this operation does not declare a request body")
		return false
	}
	return true
}

// tuiControlResponse serves tui.control.response. Its request body schema is
// deliberately unconstrained, so any single valid JSON value is accepted. No
// TUI request queue is attached, therefore false is the truthful boolean result.
func (s *Server) tuiControlResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) {
		return
	}
	if r.Body != nil && r.ContentLength != 0 {
		var response any
		if !decodeTUIJSONBody(w, r, &response, "request body must contain valid JSON") {
			return
		}
	}
	writeJSON(w, false)
}

func (s *Server) tuiOpenHelp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) || !rejectUndeclaredTUIBody(w, r) {
		return
	}
	writeJSON(w, false)
}

func (s *Server) tuiOpenModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) || !rejectUndeclaredTUIBody(w, r) {
		return
	}
	writeJSON(w, false)
}

func (s *Server) tuiOpenSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) || !rejectUndeclaredTUIBody(w, r) {
		return
	}
	writeJSON(w, false)
}

func (s *Server) tuiOpenThemes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) || !rejectUndeclaredTUIBody(w, r) {
		return
	}
	writeJSON(w, false)
}

func decodeTUIEventProperties(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return errors.New("required field properties must be an object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("properties must contain exactly one JSON object")
	}
	return nil
}

// tuiPublish serves tui.publish's EventTui* union. The event is validated but
// not delivered because this shim has no attached TUI event sink; false is the
// declared boolean representation of that truthful no-op.
func (s *Server) tuiPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) {
		return
	}

	var request struct {
		Type       *string         `json:"type"`
		Properties json.RawMessage `json:"properties"`
	}
	if !decodeTUIJSONBody(w, r, &request,
		"request body must be one EventTuiPromptAppend, EventTuiCommandExecute, EventTuiToastShow, or EventTuiSessionSelect object") {
		return
	}
	if request.Type == nil || len(request.Properties) == 0 {
		writeOpencodeBadRequest(w, r, "Payload", "required fields type and properties must be present")
		return
	}

	var err error
	switch *request.Type {
	case "tui.prompt.append":
		var properties struct {
			Text *string `json:"text"`
		}
		err = decodeTUIEventProperties(request.Properties, &properties)
		if err == nil && properties.Text == nil {
			err = errors.New("required property text must be a string")
		}
	case "tui.command.execute":
		var properties struct {
			Command *string `json:"command"`
		}
		err = decodeTUIEventProperties(request.Properties, &properties)
		if err == nil && properties.Command == nil {
			err = errors.New("required property command must be a string")
		}
	case "tui.toast.show":
		var properties struct {
			Title    *string `json:"title"`
			Message  *string `json:"message"`
			Variant  *string `json:"variant"`
			Duration *int    `json:"duration"`
		}
		err = decodeTUIEventProperties(request.Properties, &properties)
		if err == nil && (properties.Message == nil || properties.Variant == nil) {
			err = errors.New("required properties message and variant must be strings")
		}
		if err == nil {
			switch *properties.Variant {
			case "info", "success", "warning", "error":
			default:
				err = errors.New("property variant must be info, success, warning, or error")
			}
		}
		if err == nil && properties.Duration != nil && *properties.Duration <= 0 {
			err = errors.New("property duration must be a positive integer")
		}
	case "tui.session.select":
		var properties struct {
			SessionID *string `json:"sessionID"`
		}
		err = decodeTUIEventProperties(request.Properties, &properties)
		if err == nil && (properties.SessionID == nil || !strings.HasPrefix(*properties.SessionID, "ses")) {
			err = errors.New("required property sessionID must match ^ses")
		}
	default:
		err = fmt.Errorf("unsupported TUI event type %q", *request.Type)
	}
	if err != nil {
		writeOpencodeBadRequest(w, r, "Payload", err.Error())
		return
	}

	writeJSON(w, false)
}

// tuiExecuteCommand serves tui.executeCommand's declared contract (ROUTE-FIX-023,
// SHIM-DRIFT-134): requestBody {command (required string), additionalProperties
// false}, optional directory/workspace query selectors, a boolean 200 and a 400
// BadRequest | InvalidRequestError arm. The shim has no attached TUI process to
// execute a command in, so a valid request returns the declared boolean false
// rather than claiming a command ran.
func (s *Server) tuiExecuteCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) {
		return
	}
	var req struct {
		Command *string `json:"command"`
	}
	if !decodeTUIJSONBody(w, r, &req,
		"request body must be an object containing only command") {
		return
	}
	if req.Command == nil || *req.Command == "" {
		writeOpencodeBadRequest(w, r, "Payload", "required field command must be a non-empty string")
		return
	}
	writeJSON(w, false)
}

// tuiShowToast serves tui.showToast's declared contract: requestBody
// {title?, message (required), variant (required, info|success|warning|error),
// duration? exclusiveMinimum 0}, additionalProperties false, a boolean 200
// and a 400 BadRequestError arm. The shim has no attached TUI process to
// display a toast, so a valid request returns the declared boolean false
// rather than claiming a toast was shown.
func (s *Server) tuiShowToast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) {
		return
	}
	var req struct {
		Title    *string `json:"title"`
		Message  *string `json:"message"`
		Variant  *string `json:"variant"`
		Duration *int    `json:"duration"`
	}
	if !decodeTUIJSONBody(w, r, &req,
		"request body must be an object containing only title, message, variant, duration") {
		return
	}
	if req.Message == nil || *req.Message == "" {
		writeOpencodeBadRequest(w, r, "Payload", "required field message must be a non-empty string")
		return
	}
	if req.Variant == nil {
		writeOpencodeBadRequest(w, r, "Payload", "required field variant must be a string")
		return
	}
	switch *req.Variant {
	case "info", "success", "warning", "error":
	default:
		writeOpencodeBadRequest(w, r, "Payload", "field variant must be info, success, warning, or error")
		return
	}
	if req.Duration != nil && *req.Duration <= 0 {
		writeOpencodeBadRequest(w, r, "Payload", "field duration must be a positive integer")
		return
	}
	writeJSON(w, false)
}

// tuiSelectSession serves tui.selectSession's declared contract (SHIM-GAP-004;
// declared responses: 200 boolean "Session selected successfully", 400
// BadRequest | InvalidRequestError, 404 NotFoundError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/tui/select-session".post): the requestBody is {sessionID*: string},
// additionalProperties: false; the declared 200 body is a plain boolean.
//
// Before SHIM-GAP-004 the bare /tui/ catch-all answered the declared 404 for
// every request, so the declared 200 and 400 were unreachable from outside. The
// operation now follows the same shape as its siblings tui.showToast and
// tui.executeCommand (ROUTE-FIX-029/-023): the declared requestBody is validated
// strictly — a non-object/unparseable body, an undeclared field or a trailing
// second value answers the declared 400 BadRequest, and so do a missing, blank
// or empty sessionID. An unknown session answers the declared 404 NotFoundError
// (p1ResolveSession's NamedError). A session that EXISTS answers the declared
// boolean false: this shim attaches no opencode TUI process, so nothing was
// navigated — the truthful false, never a claim that a TUI moved. The declared
// ^ses path claim is deliberately NOT enforced on sessionID: this runtime issues
// the session ids it accepts (sessions.id — the same ids GET /session and POST
// /session return, which are not ^ses-prefixed), so refusing them would make the
// operation unreachable for every id this server hands out. The declared 200,
// 400 and 404 are all reachable; nothing is fabricated.
func (s *Server) tuiSelectSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateTUISelectors(w, r) {
		return
	}
	var req struct {
		SessionID *string `json:"sessionID"`
	}
	if !decodeDeclaredJSONObject(w, r, &req, "request body must be an object containing only sessionID") {
		return
	}
	if req.SessionID == nil || strings.TrimSpace(*req.SessionID) == "" {
		writeOpencodeBadRequest(w, r, "Payload", "required field sessionID must be a non-empty string")
		return
	}
	if _, ok := s.p1ResolveSession(w, r, *req.SessionID); !ok {
		return // the declared 404 NotFoundError, already written
	}
	writeJSON(w, false)
}

func (s *Server) handleTUI(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/tui/")

	if sub == "clear-prompt" {
		s.tuiClearPrompt(w, r)
		return
	}

	if sub == "append-prompt" {
		if r.Method != http.MethodPost {
			writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
			return
		}
		s.tuiAppendPrompt(w, r)
		return
	}

	// control/next is a declared operation served per contract (ROUTE-FIX-021):
	// GET answers 200 {path, body} (the empty-queue representation — the shim
	// has no TUI process), invalid selectors 400, non-GET 405.
	if sub == "control/next" {
		s.tuiControlNext(w, r)
		return
	}

	// ROUTE-FIX-023: these pinned-upstream POST operations all declare a
	// boolean 200 and a 400 arm. Each handler validates its exact input schema
	// and returns false because this shim has no attached TUI process.
	switch sub {
	case "control/response":
		s.tuiControlResponse(w, r)
		return
	case "open-help":
		s.tuiOpenHelp(w, r)
		return
	case "open-models":
		s.tuiOpenModels(w, r)
		return
	case "open-sessions":
		s.tuiOpenSessions(w, r)
		return
	case "open-themes":
		s.tuiOpenThemes(w, r)
		return
	case "publish":
		s.tuiPublish(w, r)
		return
	case "show-toast":
		s.tuiShowToast(w, r)
		return
	case "select-session":
		s.tuiSelectSession(w, r)
		return
	case "execute-command":
		s.tuiExecuteCommand(w, r)
		return
	}

	if op, ok := tuiDeclaredOps[sub]; ok {
		writeNotImplemented(w, r, op,
			fmt.Sprintf("TUI control %q is not implemented: no opencode TUI process is attached to this shim; use opencode's built-in TUI", sub))
		return
	}

	// TUI sub-paths: submit-prompt, execute-command. These remain shim-only
	// and do not map to native API calls. show-toast left this switch in
	// ROUTE-FIX-029: it is a declared pinned-upstream operation, served by
	// the tuiShowToast case in the ROUTE-FIX-023 switch above.
	switch sub {
	case "submit-prompt", "execute-command":
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
		_, _ = w.Write(specs.BundledYAML)
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
		_, _ = w.Write(data)
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
		_, _ = w.Write(data)
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
	_, _ = w.Write(data)
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
	_, _ = w.Write(data)
	slog.Warn("opencode-shim: error", "method", r.Method, "path", r.URL.Path, "status", status, "code", code)
}

// writeOpencodeNamedError emits the upstream NamedError shape the v2 SDK's
// throwOnError path consumes: {"name": <name>, "data": {"message": <message>}}.
// The SDK extracts data.message into Error.message (and attaches status+body to
// cause), so a 404 with this body surfaces the server's own sentence to the
// user instead of "[object Object]" (upstream packages/opencode/test/server/
// sdk-error-shape.test.ts, "404 with NamedError body..."). Unlike
// writeOpencodeError's {"error":{code,message}} envelope — which no SDK client
// path reads — this is the declared wire shape for the 404 NotFoundError arms.
func writeOpencodeNamedError(w http.ResponseWriter, r *http.Request, status int, name, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	data, _ := json.Marshal(map[string]any{
		"name": name,
		"data": map[string]string{
			"message": message,
		},
	})
	_, _ = w.Write(data)
	slog.Warn("opencode-shim: named error", "method", r.Method, "path", r.URL.Path, "status", status, "name", name)
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
		_, _ = w.Write(data)
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
		_ = json.Unmarshal([]byte(n), &f)
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
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
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
	_ = s.db.Exec(r.Context(),
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
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
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
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
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

// handleExperimentalMoveSession serves POST
// /experimental/control-plane/move-session — upstream
// experimental.controlPlane.moveSession (ROUTE-ADD-068, source item
// SHIM-DRIFT-036 in the pre-change artifact; declared responses: 204 "Session
// moved", 400 MoveSessionError | InvalidRequestError). Before this exact route
// was registered, net/http answered the default 404 for every request (the
// artifact's NOT-SERVED class).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/control-plane/move-session".post): the requestBody is
// {sessionID*: string pattern ^ses, destination*: {directory*: string},
// moveChanges?: boolean} with additionalProperties: false; the declared success
// is an EMPTY 204 and both declared error arms are 400s.
//
// Truthfulness: this shim is a protocol translation surface over exactly ONE
// workspace (requestWorkspaceDir) and keeps no control-plane session-move
// backend — sessions are the runtime's own rows and carry no per-session
// project-directory binding to rewrite, and no upstream project-copy machinery
// exists here. So:
//   - a well-formed request whose destination resolves to the workspace this
//     instance serves ANSWERS THE DECLARED 204: the session already resides in
//     the only project directory this instance holds, so "Session moved" is the
//     truthful idempotent no-op (nothing is moved, no state is claimed to have
//     changed, and the optional moveChanges flag is moot because no directory
//     boundary is crossed);
//   - a well-formed request whose destination is any OTHER directory answers the
//     declared 400 MoveSessionError naming that this shim serves one workspace
//     and cannot move a session out of it — never a fabricated 204;
//   - a sessionID this runtime does not hold answers the same declared 400
//     MoveSessionError; the operation declares no 404, so an unknown id is never
//     answered with an undeclared code;
//   - a body that violates the declared schema answers the declared 400
//     InvalidRequestError through the sibling writeV2InvalidRequest helper,
//     whose {_tag, message, kind?, field?} shape IS components.schemas.
//     InvalidRequestError (the helper's name records the schema family, not the
//     /api/* v2 route family).
//
// The declared ^ses pattern is deliberately NOT enforced on sessionID: this
// runtime issues the ids it accepts (sessions.id — the same ids GET /session and
// POST /session return, which are not ^ses-prefixed; the tui.selectSession
// precedent), so refusing them would make the operation unreachable for every id
// this server hands out. The pinned document declares no query parameters, so
// none are read. Non-POST answers 405 METHOD_NOT_ALLOWED (the sibling
// method-guard envelope); the operation declares no 401 arm, and unauthenticated
// requests are stopped at the api-key auth wall like every implemented route.
func (s *Server) handleExperimentalMoveSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	if r.Body == nil || r.ContentLength == 0 {
		writeV2InvalidRequest(w, "Body", "sessionID",
			"request body required: the declared schema carries required sessionID and destination")
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "", "malformed request body: "+err.Error())
		return
	}
	// A JSON null body decodes without error into a nil map (encoding/json
	// leaves the destination untouched) — still not the declared object.
	if body == nil {
		writeV2InvalidRequest(w, "Body", "", "request body must be a JSON object carrying sessionID and destination")
		return
	}
	for field := range body {
		switch field {
		case "sessionID", "destination", "moveChanges":
		default:
			writeV2InvalidRequest(w, "Body", field,
				fmt.Sprintf("unknown field %q: the declared body schema is additionalProperties:false", field))
			return
		}
	}

	rawSessionID, ok := body["sessionID"]
	if !ok {
		writeV2InvalidRequest(w, "Body", "sessionID", `required field "sessionID" is missing`)
		return
	}
	var sessionID string
	if err := json.Unmarshal(rawSessionID, &sessionID); err != nil {
		writeV2InvalidRequest(w, "Body", "sessionID", `field "sessionID" must be a string`)
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		writeV2InvalidRequest(w, "Body", "sessionID", `field "sessionID" must not be blank`)
		return
	}

	rawDestination, ok := body["destination"]
	if !ok {
		writeV2InvalidRequest(w, "Body", "destination", `required field "destination" is missing`)
		return
	}
	var destination map[string]json.RawMessage
	if err := json.Unmarshal(rawDestination, &destination); err != nil {
		writeV2InvalidRequest(w, "Body", "destination", `field "destination" must be an object`)
		return
	}
	if destination == nil {
		writeV2InvalidRequest(w, "Body", "destination", `field "destination" must be an object, not null`)
		return
	}
	for field := range destination {
		if field != "directory" {
			writeV2InvalidRequest(w, "Body", "destination."+field,
				fmt.Sprintf("unknown field %q: the declared MoveSessionDestination schema allows only \"directory\"", field))
			return
		}
	}
	rawDirectory, ok := destination["directory"]
	if !ok {
		writeV2InvalidRequest(w, "Body", "destination.directory", `required field "directory" is missing`)
		return
	}
	var directory string
	if err := json.Unmarshal(rawDirectory, &directory); err != nil {
		writeV2InvalidRequest(w, "Body", "destination.directory", `field "directory" must be a string`)
		return
	}
	if strings.TrimSpace(directory) == "" {
		writeV2InvalidRequest(w, "Body", "destination.directory", `field "directory" must not be blank`)
		return
	}

	if rawMoveChanges, ok := body["moveChanges"]; ok {
		var moveChanges bool
		// A JSON null unmarshals into a bool without error (encoding/json
		// leaves the destination untouched), so refuse it explicitly: the
		// declared member is a boolean, not a nullable one.
		if strings.TrimSpace(string(rawMoveChanges)) == "null" {
			writeV2InvalidRequest(w, "Body", "moveChanges", `field "moveChanges" must be a boolean, not null`)
			return
		}
		if err := json.Unmarshal(rawMoveChanges, &moveChanges); err != nil {
			writeV2InvalidRequest(w, "Body", "moveChanges", `field "moveChanges" must be a boolean`)
			return
		}
	}

	// Resolve the session against the runtime's real session store. A lookup
	// error and a missing row are the same answer: this instance does not hold
	// the session, so nothing can be moved.
	if row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID); err != nil || row == nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "MoveSessionError",
			fmt.Sprintf("no session %q in this runtime: nothing to move", sessionID))
		return
	}

	if !sameWorkspaceDirectory(directory, s.requestWorkspaceDir(r)) {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "MoveSessionError",
			fmt.Sprintf("cannot move session %q to %q: this shim serves exactly one workspace (%q) and implements no cross-directory session move",
				sessionID, directory, s.requestWorkspaceDir(r)))
		return
	}

	// The destination is the workspace this instance already serves, so the
	// session is where the caller asked for it: the declared 204 is the
	// truthful idempotent no-op.
	w.WriteHeader(http.StatusNoContent)
}

// sameWorkspaceDirectory reports whether two directory spellings name the same
// directory: cleaned equality first, then symlink-resolved equality when both
// paths exist (the /tmp vs /private/tmp class of spelling difference).
func sameWorkspaceDirectory(a, b string) bool {
	ca, cb := filepath.Clean(strings.TrimSpace(a)), filepath.Clean(strings.TrimSpace(b))
	if ca == cb {
		return true
	}
	ra, errA := filepath.EvalSymlinks(ca)
	rb, errB := filepath.EvalSymlinks(cb)
	return errA == nil && errB == nil && ra == rb
}

// handleExperimentalProjectCopyRemove serves DELETE
// /experimental/project/{projectID}/copy — upstream v2.projectCopy.remove
// (ROUTE-ADD-069; declared responses: 204 No Content, 400 ProjectCopyError |
// InvalidRequestError). Before this route was registered, net/http answered the
// default 404 for every request on the path (the declared-vs-served artifact's
// NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-069 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.projectCopy.remove test=internal/shim/opencode/experimental_project_copy_remove_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//
// TRUTHFULNESS — the decision rule the ROUTE-ADD-025 saved-permission removal
// established: the runtime keeps NO project-copy registry. Nothing in
// internal/db or the migration set records a copied project, and the declared
// sibling projectCopy.create is not served either, so no projectID addresses a
// copy this runtime holds. Removal is therefore an idempotent no-op, and the
// declared 204 (which has no body by definition) fabricates neither a stored
// copy nor a deletion claim. The declared 400 arm answers REAL malformed input:
//
//   - a present-but-blank {projectID} segment — the pinned document declares
//     `projectID` as a plain required string with NO pattern, so blank is the
//     only ill-shaped id the contract itself defines (kind Path);
//   - a body that violates the declared requestBody {directory*: string,
//     force*: boolean} with additionalProperties:false (kind Body/Payload);
//   - a malformed deepObject `location` selector (kind Query). The operation
//     declares that selector exactly like the ROUTE-ADD-021 family, so the
//     shared apiV2LocationSelector verdict applies.
//
// The document marks the requestBody itself OPTIONAL (no requestBody.required),
// so an absent or empty body is well-formed and proceeds to the declared 204 —
// the ptyUpdate absent-body rule. Both declared 400 shapes are admitted by the
// operation's anyOf; the handler answers the InvalidRequestError arm
// (writeV2InvalidRequest), whose body is exactly {_tag, message, kind, field}.
//
// Auth is untouched: /experimental/* is not a stub path, so the standard
// api-key wall applies, and the unauthenticated 401 is the v2 UnauthorizedError
// envelope because v2.projectCopy.remove is a v2.* operation (see
// isV2EnvelopeRoutePath).
//
// The exact registration means every method on the path reaches this handler.
// DELETE is the operation this dispatcher answers; the declared sibling POST on
// the same exact path (v2.projectCopy.create) is a DIFFERENT operation whose
// contract row ROUTE-ADD-070 owns — its arm dispatches to
// handleExperimentalProjectCopyCreate (experimental_project_copy_create.go),
// which serves the declared 200/400 for real and retired this comment's
// former typed 501 intentional narrowing. Every other undeclared method
// answers the sibling METHOD_NOT_ALLOWED 405 envelope the whole v2 family
// uses.
func (s *Server) handleExperimentalProjectCopyRemove(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		// Declared operation: validate, then the idempotent no-op below.
	case http.MethodPost:
		// Declared sibling v2.projectCopy.create on this exact path. ROUTE-ADD-070
		// serves it for real (the narrowing of ROUTE-ADD-069 is gone): the
		// upstream operation is a real git-worktree effect on the location-selected
		// workspace, answerable without a project-copy registry.
		s.handleExperimentalProjectCopyCreate(w, r)
		return
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE or POST")
		return
	}

	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		writeV2InvalidRequest(w, "Path", "projectID", "projectID must not be blank")
		return
	}

	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	if !validateProjectCopyRemoveBody(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validateProjectCopyRemoveBody validates the declared requestBody of
// v2.projectCopy.remove — {directory*: string, force*: boolean} with
// additionalProperties:false — strictly and BEFORE the (no-op) removal, the
// ptyUpdate / tui.showToast / vcs.apply rule: a non-object or unparseable body,
// more than one JSON value, an undeclared key, a non-string `directory` or a
// non-boolean `force` answers the declared 400 InvalidRequestError (kind
// Body/Payload) instead of being silently ignored.
//
// The body itself is optional: the pinned document sets no requestBody.required,
// so an absent body, an empty body or `{}` is well-formed input and proceeds.
// The declared members, when present, are required by the operation's own
// schema (required: [directory, force]), so a body carrying one of them must
// carry both.
func validateProjectCopyRemoveBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		Directory json.RawMessage `json:"directory"`
		Force     json.RawMessage `json:"force"`
	}
	if err := dec.Decode(&body); err != nil {
		// io.EOF is the absent/empty body: well-formed because the document
		// marks no member required at the requestBody level.
		if err == io.EOF {
			return true
		}
		writeV2InvalidRequest(w, "Body", "",
			`request body must be a JSON object carrying only the declared fields (directory, force)`)
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return false
	}
	if len(body.Directory) == 0 && len(body.Force) == 0 {
		return true
	}
	if len(body.Directory) == 0 || len(body.Force) == 0 {
		writeV2InvalidRequest(w, "Payload", "",
			`fields "directory" and "force" are declared required and must be carried together`)
		return false
	}
	var directory string
	if err := json.Unmarshal(body.Directory, &directory); err != nil {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must be a string`)
		return false
	}
	var force bool
	if err := json.Unmarshal(body.Force, &force); err != nil {
		writeV2InvalidRequest(w, "Payload", "force", `field "force" must be a boolean`)
		return false
	}
	return true
}
