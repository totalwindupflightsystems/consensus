package opencode

import (
	"net/http"
)

// handleApiIntegrationAttempt is the ROUTE-ADD-015+016 merge resolution
// dispatcher for the shared exact pattern /api/integration/attempt/{attemptID}.
// net/http panics at mux registration on two identical patterns, so the merged
// tree registers this entry once and routes by method:
//
//	DELETE -> handleApiIntegrationRA015 (v2.integration.attempt.cancel, ROUTE-ADD-015)
//	GET    -> handleApiIntegrationRA016 (v2.integration.attempt.status, ROUTE-ADD-016)
//
// Any other method keeps the sibling METHOD_NOT_ALLOWED envelope (the
// ROUTE-ADD-005 convention), and the v2 401 envelope membership for the path
// is unchanged (isV2EnvelopeRoutePath one-segment rule).
func (s *Server) handleApiIntegrationAttempt(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		s.handleApiIntegrationRA015(w, r)
	case http.MethodGet:
		s.handleApiIntegrationRA016(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or DELETE")
	}
}
