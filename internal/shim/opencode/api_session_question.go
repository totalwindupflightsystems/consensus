package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-055 — GET /api/session/{sessionID}/question
//                 (v2.session.question.list)
// ============================================================================
//
// The upstream contract declares 200, 400, 401 and 404. Consensus does not
// produce OpenCode QuestionV2Request values: its human-input rows are native
// approval_requests, which do not carry the required ^que identity or the
// question/header/options fields. The truthful success response is therefore
// the declared non-null empty data array after reading the session-scoped
// approval store, matching the existing v2 question-list semantics.
//
// ch:trace row=ROUTE-ADD-055 spec=specs/024-shim-support-matrix.md#v2.session.question.list test=internal/shim/opencode/api_session_question_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=none:serial evidence=none:worker-local witness=none:unavailable-in-worker verdict=none:foreman-judge commit=none:pre-commit memory=none:foreman-closeout

// handleApiSessionQuestionList serves GET /api/session/{sessionID}/question.
func (s *Server) handleApiSessionQuestionList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return
	}

	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no question requests to list", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no question requests to list", sessionID))
		return
	}

	// Read the real pending approval store before answering. Its rows cannot be
	// losslessly reshaped into QuestionV2Request, so they remain an empty data
	// array rather than becoming fabricated question entries.
	if _, err := s.db.Query(r.Context(),
		`SELECT id, session_id, request_type, status FROM approval_requests
		 WHERE session_id = $1 AND status = 'pending'`, sessionID); err != nil {
		writeV2InvalidRequest(w, "Shim", "approval_requests",
			"failed to list session question requests: "+err.Error())
		return
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{"data": []any{}})
}

// isApiSessionQuestionPath admits only the declared concrete path shape to the
// v2 UnauthorizedError envelope. Sibling and deeper session paths retain their
// own routing and envelope behavior.
func isApiSessionQuestionPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "question"
}
