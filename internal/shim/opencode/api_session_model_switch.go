package opencode

import (
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-049 — POST /api/session/{sessionID}/model (v2.session.switchModel)
// ============================================================================

// sessionModelSwitch serves POST /api/session/{sessionID}/model — upstream
// v2.session.switchModel (ROUTE-ADD-049, SHIM-DRIFT-010; declared responses:
// 204 NoContent, 400 InvalidRequestError, 401 UnauthorizedError, 404
// NotFoundError).
//
// The Consensus runtime keeps no upstream OpenCode session model-switch
// pipeline — it parks no sessions, runs no processes, and has no model
// assignment concept — so the declared 204 is answered with an empty body as a
// truthful no-op: no model was switched because no model-switch mechanism
// exists.
//
// The sessionID path parameter is required by chi; a blank segment is rejected
// by chi itself with a 400. The declared 404 NotFoundError is answered when
// the sessionID refers to a session this instance does not know.

func (s *Server) handleApiSessionModelSwitch(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"sessionID path parameter is required")
		return
	}

	// Verify the session exists in this instance's store.
	// Follows the sessionDiff session-lookup pattern: QueryRow returns (row, err),
	// and an empty sessionID path param would have already been rejected by chi.
	if s.db == nil {
		// No database means no sessions exist.
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError",
			"Session not found")
		return
	}
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError",
			"Session not found")
		return
	}

	// The runtime has no model-switch pipeline, so the truthful response is
	// 204 NoContent — the session's model state is unchanged.
	w.WriteHeader(http.StatusNoContent)
}
