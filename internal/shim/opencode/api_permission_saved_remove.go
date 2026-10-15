package opencode

import (
	"net/http"
	"strings"
)

// handleApiPermissionSavedRemove serves DELETE /api/permission/saved/{id} —
// upstream v2.permission.saved.remove (ROUTE-ADD-025, formerly SHIM-DRIFT-001
// in the committed artifact's positional ids; declared responses: 204 No
// Content, 400 InvalidRequestError, 401 UnauthorizedError). Before this route
// was registered, net/http answered the default 404 for every request.
//
// ch:trace row=ROUTE-ADD-025 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.permission.saved.remove test=internal/shim/opencode/api_permission_saved_remove_test.go evidence=docs/evidence/ROUTE-ADD-025-live-probe.md witness=none:unattended-worker-session
//
// The runtime keeps NO saved-permission (reusable allow/deny rule) store — the
// same truth the sibling list operation answers with: the migration set
// (001..026) has no such table, and permission.reply records an "always"
// answer as the resolved approval's review_notes on the approval_requests row
// (server.go permissionReply), not as a reusable rule. No upstream id can
// therefore address anything this runtime holds. Removal is an idempotent
// no-op: the declared 204 has an empty body and fabricates neither a stored
// rule nor a deletion claim.
//
// The declared path parameter is a plain required string with NO pattern, so
// the only malformed id this handler names is a present-but-blank one (a
// whitespace-only segment such as %20; an actually-empty segment cannot match
// the {id} route pattern and keeps net/http's default 404). That degenerate
// input answers the declared InvalidRequestError 400 (writeV2InvalidRequest,
// the handleApiIntegrationRA015 spelling). This operation declares NO query
// selector — unlike its sibling list operation, which validates projectID —
// so no query parameter is validated here. The existing /api/* auth wall
// supplies the declared UnauthorizedError 401 before this handler runs,
// through the one-segment v2 envelope match.
func (s *Server) handleApiPermissionSavedRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeV2InvalidRequest(w, "Path", "id", "id must not be blank")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
