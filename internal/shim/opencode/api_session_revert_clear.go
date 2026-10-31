package opencode

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleSessionRevertClear serves POST /api/session/{sessionID}/revert/clear
// (upstream operationId v2.session.revert.clear).
// Declared responses: 204, 400, 401, 404, 500.
//
// The revert/clear operation discards any pending revert state for the session.
// This shim accepts the request and answers 204 No Content on success, matching
// the upstream contract.
func (s *Server) handleSessionRevertClear(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":{"code":"INVALID_REQUEST","message":"missing sessionID"}}`, http.StatusBadRequest)
		return
	}

	// Clear any pending revert state for this session.
	// The upstream contract is a no-arg POST that answers 204 on success.
	s.clearRevertState(sessionID)

	w.WriteHeader(http.StatusNoContent)
}

// clearRevertState removes any pending revert state for the named session.
// This is a best-effort operation; if no revert state exists, it is a no-op.
func (s *Server) clearRevertState(sessionID string) {
	// No-op: the revert subsystem is not yet wired in this shim.
	// The route exists to satisfy the declared contract (204 on success);
	// actual revert state management will be added when the revert subsystem
	// is integrated. For now, the route answers 204 as declared.
	_ = sessionID
}
