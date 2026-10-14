package opencode

import (
	"net/http"
	"strings"
)

// handleApiIntegrationRA016 serves GET /api/integration/attempt/{attemptID} —
// upstream v2.integration.attempt.status (ROUTE-ADD-016 / SHIM-DRIFT-003,
// declared responses: 200 {location: LocationInfo, data:
// IntegrationAttemptStatus}, 400 InvalidRequestError, 401 UnauthorizedError).
// Before this route was registered, net/http answered its default 404.
//
// ch:trace row=ROUTE-ADD-016 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.status test=internal/shim/opencode/api_integration_ra016_test.go witness=none:unattended-worker-session
//
// The request shape matches v2.integration.list: the optional deepObject
// location selector contains only directory/workspace and is validated by the
// shared apiV2LocationSelector. attemptID is a required plain string with no
// declared pattern, so only a blank segment is malformed.
//
// Truthfulness rule: the runtime keeps no OAuth integration-attempt store — no
// migration creates one and no shim flow persists pending/completed/failed
// attempt state. A non-empty payload could therefore only invent an attempt,
// its status, and its required created/expires timestamps. The declared 200
// consequently carries the real selected LocationInfo plus an EMPTY, non-null
// data array: absence is explicit and no IntegrationAttemptStatus row is
// fabricated. A future non-empty response must be backed by a real runtime
// store whose rows can be expressed in the declared schema.
func (s *Server) handleApiIntegrationRA016(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	attemptID := strings.TrimSpace(r.PathValue("attemptID"))
	if attemptID == "" {
		writeV2InvalidRequest(w, "Path", "attemptID", "attemptID must not be blank")
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
