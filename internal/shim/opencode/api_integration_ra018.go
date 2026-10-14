package opencode

import (
	"net/http"
	"strings"
)

// handleApiIntegrationRA018 serves GET /api/integration/{integrationID} —
// upstream v2.integration.get (ROUTE-ADD-018, SHIM-DRIFT-005, declared
// responses 200, 400 and 401). Previously the path had no shim route and
// net/http answered its default 404.
//
// ch:trace row=ROUTE-ADD-018 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.get test=internal/shim/opencode/api_integration_ra018_test.go evidence=docs/evidence/ROUTE-ADD-018-live-probe.md witness=none:unattended-worker-session
//
// The operation carries the v2 integration surface's deepObject `location`
// selector (location[directory] / location[workspace]); malformed selectors
// answer the declared InvalidRequestError 400 through apiV2LocationSelector.
// A blank integrationID also answers that declared 400, although the exact
// ServeMux pattern normally prevents an empty path segment from reaching this
// handler.
//
// Consensus keeps no integration registry: no integration table exists in the
// migration set and no runtime component registers IntegrationInfo rows. Under
// ROUTE-ADD-018's required v2.integration.list-compatible success shape, the
// truthful declared 200 is therefore {location: LocationInfo, data: []}. The
// empty array is non-null; the requested integrationID is never echoed as a
// fabricated IntegrationInfo {id, name, methods, connections} row.
func (s *Server) handleApiIntegrationRA018(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	integrationID := strings.TrimSpace(r.PathValue("integrationID"))
	if integrationID == "" {
		writeV2InvalidRequest(w, "Path", "integrationID", "integrationID must not be blank")
		return
	}

	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}
