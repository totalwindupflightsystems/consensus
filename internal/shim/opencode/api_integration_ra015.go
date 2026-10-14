package opencode

import (
	"net/http"
	"strings"
)

// handleApiIntegrationRA015 serves DELETE
// /api/integration/attempt/{attemptID} — upstream
// v2.integration.attempt.cancel (ROUTE-ADD-015, formerly SHIM-DRIFT-002;
// declared responses: 204 No Content, 400 InvalidRequestError, 401
// UnauthorizedError). Before this route was registered, net/http answered the
// default 404 for every request.
//
// ch:trace row=ROUTE-ADD-015 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.cancel test=internal/shim/opencode/api_integration_ra015_test.go evidence=docs/evidence/ROUTE-ADD-015-live-probe.md witness=none:unattended-worker-session
//
// The runtime keeps no integration OAuth-attempt store and no resource keyed by
// the upstream attemptID. Cancellation is therefore an idempotent no-op: the
// declared 204 has an empty body and does not claim a stored attempt existed or
// that any fabricated row was deleted. The declared optional deepObject
// location selector is still validated through apiV2LocationSelector; malformed
// selector input answers the declared InvalidRequestError 400. The existing
// /api/* auth wall supplies the declared UnauthorizedError 401 before this
// handler runs.
func (s *Server) handleApiIntegrationRA015(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
		return
	}

	attemptID := strings.TrimSpace(r.PathValue("attemptID"))
	if attemptID == "" {
		writeV2InvalidRequest(w, "Path", "attemptID", "attemptID must not be blank")
		return
	}

	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
