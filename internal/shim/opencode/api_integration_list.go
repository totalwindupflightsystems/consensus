package opencode

import "net/http"

// handleApiIntegrationList serves GET /api/integration — upstream
// v2.integration.list (ROUTE-ADD-014 / SHIM-DRIFT-001, declared responses: 200
// {location: LocationInfo, data: IntegrationInfo[]} "Success", 400
// InvalidRequestError, 401 UnauthorizedError). Previously the exact path had no
// shim route and net/http answered the default 404.
//
// ch:trace row=ROUTE-ADD-014 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.list test=internal/shim/opencode/api_integration_list_test.go evidence=docs/evidence/ROUTE-ADD-014-live-probe.md witness=none:unattended-worker-session
//
// The runtime keeps no integration registry. No migration creates an
// integration or connection store, and the provider-auth routes do not retain
// an upstream IntegrationInfo row: that declared schema requires id, name,
// methods (OAuth/key/environment descriptors), and connections. Producing any
// entry would therefore invent both an integration identity and authentication
// state the runtime does not hold. The truthful declared 200 is
// {location: <the real selected workspace>, data: []}; data is an empty,
// non-null array and never contains a fabricated row.
//
// The optional deepObject location selector is the same declared shape used by
// the sibling v2 list operations. apiV2LocationSelector enforces its
// additionalProperties:false contract and answers the declared 400
// InvalidRequestError for malformed input. The existing /api/* auth wall serves
// the declared 401 UnauthorizedError before this handler runs.
func (s *Server) handleApiIntegrationList(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
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
