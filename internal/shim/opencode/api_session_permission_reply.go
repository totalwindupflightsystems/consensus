package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// handleApiSessionPermissionReply serves POST
// /api/session/{sessionID}/permission/{requestID}/reply — upstream
// v2.session.permission.reply (ROUTE-ADD-053). A pending approval request is
// resolved through the real approval_requests store; the declared success is
// bodyless, so the handler never fabricates a response payload.
//
// Declared responses are 204, 400, 401 and 404. The request body is the closed
// object {reply: "once"|"always"|"reject", message?: string}; the path
// parameters must match ^ses and ^per respectively.
//
// ch:trace row=ROUTE-ADD-053 spec=specs/024-shim-support-matrix.md#v2-session-permission wave=consensus-foreman-2026-10-31-06-09-32#ROUTE-ADD-053 test=api_session_permission_reply_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-foreman-2026-10-31-06-09-32-ROUTE-ADD-053.txt evidence=/tmp/parity-053/declared-vs-served.json witness=none:pending-foreman-verification verdict=none:pending-judge commit=<your-commit-sha> memory=none:pending-foreman-write
func (s *Server) handleApiSessionPermissionReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" || !strings.HasPrefix(sessionID, "ses") || strings.Contains(sessionID, "/") {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must match the declared pattern ^ses")
		return
	}
	requestID := strings.TrimSpace(r.PathValue("requestID"))
	if requestID == "" || !strings.HasPrefix(requestID, "per") || strings.Contains(requestID, "/") {
		writeV2InvalidRequest(w, "Path", "requestID", "requestID path parameter must match the declared pattern ^per")
		return
	}

	var req struct {
		Reply   *string `json:"reply"`
		Message *string `json:"message"`
	}
	if !decodeDeclaredJSONObject(w, r, &req,
		"request body must be an object containing only reply and message") {
		return
	}
	if req.Reply == nil || (*req.Reply != "once" && *req.Reply != "always" && *req.Reply != "reject") {
		writeV2InvalidRequest(w, "Payload", "reply",
			`required field "reply" must be one of "once", "always", "reject"`)
		return
	}

	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to resolve permission: the shim has no approval request store")
		return
	}

	// Resolve both resources in one read. A request is addressable through this
	// route only when its owning session exists, it belongs to this session, and
	// it is still pending. The combined lookup keeps the session/request
	// relationship atomic at the decision point and gives the mock and live
	// stores the same observable contract.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT ar.session_id, ar.status
		 FROM approval_requests ar
		 INNER JOIN sessions s ON s.id = ar.session_id
		 WHERE s.id = $1 AND ar.id = $2`, sessionID, requestID)
	if err != nil || row == nil {
		writeOpencodeNotFoundError(w, "PermissionNotFoundError", "requestID", requestID,
			fmt.Sprintf("permission request %q was not found for session %q", requestID, sessionID))
		return
	}
	if owner := strings.TrimSpace(toString(row["session_id"])); owner != "" && owner != sessionID {
		writeOpencodeNotFoundError(w, "PermissionNotFoundError", "requestID", requestID,
			fmt.Sprintf("permission request %q was not found for session %q", requestID, sessionID))
		return
	}
	if toString(row["status"]) != "pending" {
		writeOpencodeNotFoundError(w, "PermissionNotFoundError", "requestID", requestID,
			fmt.Sprintf("permission request %q is not pending", requestID))
		return
	}

	decision := "approved"
	note := "granted (always)"
	switch *req.Reply {
	case "once":
		note = "granted once"
	case "reject":
		decision = "rejected"
		note = "denied by v2.session.permission.reply"
	}
	if req.Message != nil && strings.TrimSpace(*req.Message) != "" {
		note += " — " + *req.Message
	}

	if err := s.db.Exec(r.Context(),
		`UPDATE approval_requests
		 SET status = $1, review_notes = $2, reviewed_at = CURRENT_TIMESTAMP,
		     reviewer_id = 'opencode-shim'
		 WHERE id = $3 AND session_id = $4 AND status = 'pending'`,
		decision, note, requestID, sessionID); err != nil {
		// This operation declares no 5xx arm. Report an unreadable/unwritable
		// approval store through its declared InvalidRequestError instead.
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to resolve permission: "+err.Error())
		return
	}

	s.emitShimEventForSession(sessionID, "approval_resolved", map[string]any{
		"approval_id": requestID,
		"status":      decision,
	})
	w.WriteHeader(http.StatusNoContent)
}

// isApiSessionPermissionReplyPath admits exactly the declared concrete route
// shape to the v2 UnauthorizedError envelope.
func isApiSessionPermissionReplyPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/session/")
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) == 4 && parts[0] != "" && parts[1] == "permission" &&
		parts[2] != "" && parts[3] == "reply"
}
