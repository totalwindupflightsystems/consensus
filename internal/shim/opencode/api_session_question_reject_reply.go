package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-056/057 — POST /api/session/{sessionID}/question/{requestID}/reject
// and POST /api/session/{sessionID}/question/{requestID}/reply
// (v2.session.question.reject / v2.session.question.reply)
// ============================================================================
//
// The pinned upstream contract declares 204, 400, 401 and 404 for both
// operations; reply additionally requires {answers: string[][]}. Consensus has
// no OpenCode question producer or pending-question registry. A well-formed
// request for a real session is therefore accepted as an empty 204 control
// operation: the success is bodyless and makes no fabricated claim about a
// question or answer. Session lookup still provides the declared 404, while
// malformed path/body input answers the declared InvalidRequestError 400.
//
// ch:trace row=ROUTE-ADD-056/057 spec=specs/024-shim-support-matrix.md test=internal/shim/opencode/api_session_question_reject_reply_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=none:worker-local witness=none:foreman-verifies-post-merge verdict=none:foreman-judges-post-merge commit=none:self-referential-hash memory=none:worker-no-memory

// handleApiSessionQuestionReject serves POST
// /api/session/{sessionID}/question/{requestID}/reject.
func (s *Server) handleApiSessionQuestionReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateApiSessionQuestionActionPath(w, r) {
		return
	}
	if !s.resolveApiSessionQuestionActionSession(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleApiSessionQuestionReply serves POST
// /api/session/{sessionID}/question/{requestID}/reply.
func (s *Server) handleApiSessionQuestionReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	if !validateApiSessionQuestionActionPath(w, r) {
		return
	}

	var req struct {
		Answers *[][]string `json:"answers"`
	}
	if !decodeDeclaredJSONObject(w, r, &req, "request body must be an object containing only answers") {
		return
	}
	if req.Answers == nil {
		writeV2InvalidRequest(w, "Payload", "answers", "required field answers must be an array of arrays of strings")
		return
	}
	if !s.resolveApiSessionQuestionActionSession(w, r) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validateApiSessionQuestionActionPath(w http.ResponseWriter, r *http.Request) bool {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return false
	}
	requestID := strings.TrimSpace(r.PathValue("requestID"))
	if requestID == "" || !strings.HasPrefix(requestID, "que") {
		writeV2InvalidRequest(w, "Path", "requestID", "requestID path parameter must match the declared pattern ^que")
		return false
	}
	return true
}

func (s *Server) resolveApiSessionQuestionActionSession(w http.ResponseWriter, r *http.Request) bool {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return false
	}
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no question request", sessionID))
			return false
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return false
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no question request", sessionID))
		return false
	}
	return true
}

// isApiSessionQuestionRejectPath admits exactly the declared concrete reject
// route to the v2 UnauthorizedError envelope.
func isApiSessionQuestionRejectPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && questionActionPath(tail, "reject")
}

// isApiSessionQuestionReplyPath admits exactly the declared concrete reply
// route to the v2 UnauthorizedError envelope.
func isApiSessionQuestionReplyPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && questionActionPath(tail, "reply")
}

func questionActionPath(tail, action string) bool {
	parts := strings.Split(tail, "/")
	return len(parts) == 3 && parts[0] == "question" && parts[1] != "" &&
		!strings.Contains(parts[1], "/") && parts[2] == action
}
