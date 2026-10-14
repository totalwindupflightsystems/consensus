package opencode

import "net/http"

// handleApiHealth serves GET /api/health — upstream v2.health.get
// (ROUTE-ADD-006, source item SHIM-DRIFT-008, declared responses: 200
// {healthy: boolean} "Check whether the API server is ready to accept
// requests.", 400 InvalidRequestError, 401 UnauthorizedError). The exact path
// previously had no route at all, so net/http answered the default 404 (the
// artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-006 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.health.get test=internal/shim/opencode/api_health_test.go witness=none:unattended-worker-session
//
// The declared 200 schema is a single required boolean `healthy` whose enum is
// [true] with additionalProperties: false, and the upstream summary is "Check
// whether the API server is ready to accept requests" — a process-level
// readiness probe. A server that is running and routing THIS request is ready
// to accept requests, so the truthful body is exactly {"healthy": true}: the
// declared enum admits no other value (a fabricated false arm would violate
// the declared contract), and no extra member may be added — the sibling
// /global/health route carries a `version` member, but global.health's own
// schema declares it while v2.health.get's does not. The operation declares no
// parameters and no request body, so there is no input to reject against the
// declared 400 arm. Unauthenticated requests answer the declared
// UnauthorizedError 401 (the middleware /api/health branch, mirroring
// /api/agent and /api/fs/find), and undeclared methods keep the sibling
// METHOD_NOT_ALLOWED envelope.
func (s *Server) handleApiHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	writeJSON(w, map[string]any{"healthy": true})
}
