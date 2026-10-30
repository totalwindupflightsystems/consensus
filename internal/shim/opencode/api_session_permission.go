package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// ============================================================================
// ROUTE-ADD-050/051 — /api/session/{sessionID}/permission
// (v2.session.permission.list / v2.session.permission.create)
// ============================================================================
//
// The upstream contract declares a session-scoped permission list and create
// operation, each with 200, 400, 401 and 404 arms. Consensus owns the durable
// approval_requests store, but its native UUID/request_type schema cannot be
// losslessly reshaped into OpenCode's ^per/^ses PermissionV2Request. GET
// therefore returns the declared empty collection after reading the real
// pending store; POST records a real pending HITL approval and returns the
// protocol-shaped ask result, using the caller's protocol id when supplied.
//
// ch:trace row=ROUTE-ADD-050 spec=specs/024-shim-support-matrix.md#v2-session-permission-list test=internal/shim/opencode/api_session_permission_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/home/kara/.hermes/cache/scratch/consensus-foreman-2026-10-30-22-18-08_ROUTE-ADD-050-051.txt evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-live-witness-not-created verdict=none:pending-judge commit=none:pending memory=none:foreman-closeout
//
// The native approval row is the real side effect: no permission rule engine
// exists in this adapter, so every well-formed request is honestly parked for
// human review (effect "ask") rather than claiming allow/deny. The protocol id
// is stored in context so a later permission-detail/reply route can resolve it
// without changing the native UUID primary key.

func (s *Server) handleApiSessionPermission(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleApiSessionPermissionList(w, r)
	case http.MethodPost:
		s.handleApiSessionPermissionCreate(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
	}
}

func (s *Server) resolveSessionPermissionSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return "", false
	}
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return "", false
	}
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no permission request", sessionID))
			return "", false
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return "", false
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no permission request", sessionID))
		return "", false
	}
	return sessionID, true
}

func (s *Server) handleApiSessionPermissionList(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := s.resolveSessionPermissionSession(w, r)
	if !ok {
		return
	}

	// Read the real pending store before answering. Its rows use native UUIDs
	// and request_type/target fields, not the closed PermissionV2Request schema;
	// returning an invented ^per/^ses row would be less truthful than []{}.
	if _, err := s.db.Query(r.Context(),
		`SELECT id, session_id, request_type, risk_level, description, context, target_tool
		 FROM approval_requests WHERE session_id = $1 AND status = 'pending'
		 ORDER BY created_at`, sessionID); err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to list session permission requests: "+err.Error())
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"data": []any{}})
}

type apiSessionPermissionCreateRequest struct {
	ID        string          `json:"id"`
	Action    *string         `json:"action"`
	Resources *[]string       `json:"resources"`
	Save      []string        `json:"save"`
	Metadata  json.RawMessage `json:"metadata"`
	Source    json.RawMessage `json:"source"`
	Agent     string          `json:"agent"`
}

func (s *Server) handleApiSessionPermissionCreate(w http.ResponseWriter, r *http.Request) {
	var req apiSessionPermissionCreateRequest
	if !decodeDeclaredJSONObject(w, r, &req,
		"request body must be an object containing action and resources") {
		return
	}
	if req.Action == nil || strings.TrimSpace(*req.Action) == "" {
		writeV2InvalidRequest(w, "Body", "action", `required field "action" must be a non-blank string`)
		return
	}
	if req.Resources == nil {
		writeV2InvalidRequest(w, "Body", "resources", `required field "resources" must be an array of strings`)
		return
	}

	sessionID, ok := s.resolveSessionPermissionSession(w, r)
	if !ok {
		return
	}

	approvalID := uuid.NewString()
	protocolID := strings.TrimSpace(req.ID)
	if protocolID == "" {
		protocolID = "per" + approvalID
	} else if !strings.HasPrefix(protocolID, "per") {
		writeV2InvalidRequest(w, "Body", "id", `field "id" must match the declared pattern ^per`)
		return
	}
	contextValue := map[string]any{
		"protocol_id": protocolID,
		"action":      *req.Action,
		"resources":   *req.Resources,
	}
	if len(req.Metadata) > 0 {
		var metadata any
		if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
			writeV2InvalidRequest(w, "Body", "metadata", `field "metadata" must be an object`)
			return
		}
		if _, ok := metadata.(map[string]any); !ok {
			writeV2InvalidRequest(w, "Body", "metadata", `field "metadata" must be an object`)
			return
		}
		contextValue["metadata"] = metadata
	}
	contextJSON, err := json.Marshal(contextValue)
	if err != nil {
		writeV2InvalidRequest(w, "Body", "metadata", "failed to encode permission context: "+err.Error())
		return
	}

	if err := s.db.Exec(r.Context(),
		`INSERT INTO approval_requests
		 (id, session_id, iteration, request_type, description, risk_level, context, target_tool, status, created_at)
		 VALUES ($1, $2, 0, 'custom', $3, 'medium', $4, $5, 'pending', CURRENT_TIMESTAMP)`,
		approvalID, sessionID, *req.Action, string(contextJSON), *req.Action); err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to create permission request: "+err.Error())
		return
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":     protocolID,
			"effect": "ask",
		},
	})
}

// isApiSessionPermissionPath admits only the declared concrete two-segment
// session permission route to the v2 UnauthorizedError envelope.
func isApiSessionPermissionPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "permission"
}
