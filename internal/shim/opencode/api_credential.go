package opencode

import (
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-007 + ROUTE-ADD-008 — the /api/credential/{credentialID} family
// (v2.credential.remove, v2.credential.update)
//
// ch:trace row=ROUTE-ADD-007 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.credential.remove test=internal/shim/opencode/api_credential_remove_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
// ch:trace row=ROUTE-ADD-008 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.credential.update test=internal/shim/opencode/api_credential_update_test.go witness=none:unattended-worker-session
// ============================================================================

// handleApiCredential dispatches the /api/credential/{credentialID} path —
// the declared v2 credential family (upstream operationIds v2.credential.remove
// and v2.credential.update; declared responses per operation: 204, 400
// InvalidRequestError, 401 UnauthorizedError). Before this wave the path had no
// shim route at all, so net/http answered the default 404 (the drift artifact's
// NOT-SERVED class). The route entry is a SUBTREE match because the declared
// path carries a path parameter, and the handler dispatches by method (the
// handlePtyByID shape; the one-dispatch-entry-per-path-family convention).
func (s *Server) handleApiCredential(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		s.credentialRemove(w, r)
	case http.MethodPatch:
		s.credentialUpdate(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE or PATCH")
	}
}

// credentialRemove serves DELETE /api/credential/{credentialID} — upstream
// v2.credential.remove (ROUTE-ADD-007, formerly SHIM-DRIFT-001; declared
// responses: 204 "Credential removed", 400 InvalidRequestError, 401
// UnauthorizedError).
//
// TRUTHFUL payload: the Consensus runtime keeps NO credential store for the v2
// surface — internal/db has no credentials table and no migration creates one —
// so a removal request is honored as an idempotent no-op and answers the
// declared 204 with an empty body. There is no stored credential being deleted
// and none is fabricated (the handleGlobalDispose / ROUTE-ADD-091 convention;
// the empty-collection precedent of the ROUTE-ADD-021 list operations). The
// declared 400 InvalidRequestError arm is unreachable here by contract: the
// pinned upstream document declares credentialID as a plain required string
// with NO pattern and the operation declares NO requestBody, so the shim has no
// declared malformed input to reject — inventing one would fabricate a
// constraint the document does not state.
//
// The declared sibling PATCH (v2.credential.update) is dispatched above to
// credentialUpdate (api_credential_update.go, ROUTE-ADD-008's honest 400
// narrowing).
func (s *Server) credentialRemove(w http.ResponseWriter, r *http.Request) {
	credentialID := strings.TrimPrefix(r.URL.Path, "/api/credential/")
	// The declared path carries exactly one non-empty {credentialID} segment;
	// a blank id (/api/credential/) or a deeper undeclared sub-path was never
	// a declared operation, so it keeps the net/http default 404 instead of
	// being answered inside the declared 204/400 contract.
	if credentialID == "" || strings.Contains(credentialID, "/") {
		http.NotFound(w, r)
		return
	}

	// Declared success: 204 No Content, empty body — idempotent removal of
	// nothing (no credential store exists to hold an entry).
	w.WriteHeader(http.StatusNoContent)
}
