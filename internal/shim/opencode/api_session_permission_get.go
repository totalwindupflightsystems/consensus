package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ROUTE-ADD-052 — GET /api/session/{sessionID}/permission/{requestID}
// (v2.session.permission.get)
//
// The upstream operation declares 200, 400, 401 and 404. Permission rows are
// stored natively in approval_requests, while the v2 permission identity and
// fields are retained in the JSON context written by the sibling create route.
// This handler resolves that protocol identity without fabricating a response
// for rows that cannot be reshaped into PermissionV2Request.
//
// ch:trace row=ROUTE-ADD-052 spec=specs/024-shim-support-matrix.md#v2-session-permission wave=consensus-foreman-2026-10-31-06-09-32#ROUTE-ADD-052 test=api_session_permission_get_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-foreman-2026-10-31-06-09-32-ROUTE-ADD-052.txt evidence=/tmp/parity-052/declared-vs-served.json witness=none:pending-foreman-verification verdict=none:pending-judge commit=<your-commit-sha> memory=none:pending-foreman-write

// handleApiSessionPermissionGet serves GET
// /api/session/{sessionID}/permission/{requestID}.
func (s *Server) handleApiSessionPermissionGet(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}
	requestID := strings.TrimSpace(r.PathValue("requestID"))
	if requestID == "" {
		writeV2InvalidRequest(w, "Path", "requestID", "requestID path parameter must not be blank")
		return
	}
	if !strings.HasPrefix(requestID, "per") {
		writeV2InvalidRequest(w, "Path", "requestID", "requestID path parameter must match the declared pattern ^per")
		return
	}

	// This operation has no declared 5xx arm. An absent or unreadable store is
	// therefore reported through the declared InvalidRequestError, never as a
	// false SessionNotFoundError.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return
	}

	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no permission request", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no permission request", sessionID))
		return
	}

	// The protocol id is stored inside context because approval_requests.id is a
	// native UUID and cannot satisfy the declared ^per identity. Read pending
	// rows for this session and resolve the protocol id in Go so the query stays
	// portable across SQLite and PostgreSQL.
	rows, err := s.db.Query(r.Context(),
		`SELECT id, session_id, context, status FROM approval_requests WHERE session_id = $1 AND status = 'pending'`,
		sessionID)
	if err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests", "failed to read permission request: "+err.Error())
		return
	}
	for _, candidate := range rows {
		var context apiSessionPermissionGetContext
		if err := json.Unmarshal([]byte(toString(candidate["context"])), &context); err != nil {
			continue
		}
		if context.ProtocolID != requestID {
			continue
		}
		if strings.TrimSpace(context.Action) == "" || context.Resources == nil {
			// A row exists, but cannot truthfully satisfy the closed response
			// schema. Use the declared permission-not-found arm.
			break
		}

		data := map[string]any{
			"id":        context.ProtocolID,
			"sessionID": sessionID,
			"action":    context.Action,
			"resources": context.Resources,
		}
		if context.Save != nil {
			data["save"] = context.Save
		}
		if len(context.Metadata) > 0 {
			var metadata map[string]any
			if err := json.Unmarshal(context.Metadata, &metadata); err == nil && metadata != nil {
				data["metadata"] = metadata
			}
		}
		if len(context.Source) > 0 {
			var source map[string]any
			if err := json.Unmarshal(context.Source, &source); err == nil && source != nil {
				data["source"] = source
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{"data": data})
		return
	}

	writeOpencodeNotFoundError(w, "PermissionNotFoundError", "requestID", requestID,
		fmt.Sprintf("no pending permission request %q in session %q", requestID, sessionID))
}

type apiSessionPermissionGetContext struct {
	ProtocolID string          `json:"protocol_id"`
	Action     string          `json:"action"`
	Resources  []string        `json:"resources"`
	Save       []string        `json:"save"`
	Metadata   json.RawMessage `json:"metadata"`
	Source     json.RawMessage `json:"source"`
}

// isApiSessionPermissionGetPath admits only the declared three-segment
// permission-detail route to the v2 UnauthorizedError envelope.
func isApiSessionPermissionGetPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	if !ok || sessionID == "" {
		return false
	}
	permissionID, requestID, ok := strings.Cut(tail, "/")
	return permissionID == "permission" && ok && requestID != "" && !strings.Contains(requestID, "/")
}
