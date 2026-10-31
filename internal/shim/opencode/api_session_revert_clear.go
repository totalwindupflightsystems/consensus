package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// handleSessionRevertClear serves POST /api/session/{sessionID}/revert/clear
// (upstream operationId v2.session.revert.clear).
// Declared responses: 204, 400, 401, 404, 500.
//
// The revert/clear operation discards any pending revert state for the session.
// This shim accepts the request and answers 204 No Content on success, matching
// the upstream contract.
func (s *Server) handleSessionRevertClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must be one non-blank path segment")
		return
	}

	// The middleware has already authenticated the request. Re-read the
	// session-scoped identity so a key bound to another session cannot control
	// this session; admin keys have no session binding and are allowed.
	if !s.skipAuth {
		keySessionID, ok := s.validateAuth(r)
		if !ok {
			writeV2Unauthorized(w, "invalid credentials")
			return
		}
		if keySessionID != "" && keySessionID != sessionID {
			writeOpencodeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key is not authorized for this session")
			return
		}
	}

	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return
	}
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to clear", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to clear", sessionID))
		return
	}

	// Consensus has no addressable per-session revert state. The declared
	// success is bodyless, so this is an honest idempotent no-op.
	w.WriteHeader(http.StatusNoContent)
}
