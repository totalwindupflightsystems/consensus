package opencode

import (
	"net/http"
	"strings"
)

// handleApiCredentialRemove serves DELETE /api/credential/{credentialID} —
// upstream v2.credential.remove (ROUTE-ADD-007, formerly SHIM-DRIFT-001 in the
// declared-vs-served artifact; declared responses: 204 "Credential
// removed", 400 InvalidRequestError, 401 UnauthorizedError). The exact path
// previously had no route at all, so net/http answered the default 404 (the
// artifact's NOT-SERVED class).
//
//	ch:trace row=ROUTE-ADD-007 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.credential.remove test=internal/shim/opencode/api_credential_remove_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//
// TRUTHFUL payload: the Consensus runtime keeps NO credential store for the v2
// surface — internal/db has no credentials table and no migration creates one
// (the v2 SDK credential surface is served by this shim alone) — so a removal
// request is honored as an idempotent no-op and answers the declared 204 with
// an empty body: the declared "Credential removed" acknowledged. There is no
// stored credential being deleted and none is fabricated; the same honest
// no-op the sibling v2 surface answers with wherever no store exists
// (handleGlobalDispose, the ROUTE-ADD-091 convention; the empty-collection
// precedent of the ROUTE-ADD-021 list operations). The declared 400
// InvalidRequestError arm is unreachable here by contract: the pinned upstream
// document (openapi-1.18.33.json) declares credentialID as a plain required
// string with NO pattern and the operation declares NO requestBody, so the
// shim has no declared malformed input to reject — unlike the ptyID/^pty and
// msg/^msg param families there is no shape the 400 arm attaches to, and
// inventing one would fabricate a constraint the document does not state.
//
// Path handling mirrors the pty family's single-path multi-operation shape
// (handlePtyByID): the route entry is a SUBTREE match because the declared
// path carries a path parameter, and the handler dispatches by method. The
// declared sibling PATCH (v2.credential.update) answers the typed
// not-implemented envelope (writeNotImplemented) rather than a bare 405: the
// upstream document DECLARES the operation, so a registered route may not
// silently answer it with 404/405 — the SHIM-GAP-002 contract the pty.create
// stub follows. It is recorded as an explicit intentional narrowing
// (x-shim-intentional-narrowing on its served-surface row).
//
// Unauthenticated requests answer the declared UnauthorizedError 401 (the
// writeV2Unauthorized envelope via the /api/credential/ prefix branch in the
// auth middleware, the exact-path v2EnvelopeRoutePaths convention extended to
// this parameter path). Undeclared methods on the declared path answer 405
// METHOD_NOT_ALLOWED (the apiV2RequireGet sibling convention).
func (s *Server) handleApiCredentialRemove(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		// Declared success arm, below.
	case http.MethodPatch:
		// Declared sibling operation v2.credential.update (SHIM-DRIFT-002):
		// deliberately unsupported — the runtime keeps no credential store,
		// so a declared 204 "credential updated" would fabricate a write that
		// never happened. The typed not_implemented envelope answers it
		// (the pty.create precedent, SHIM-GAP-002).
		writeNotImplemented(w, r, "v2.credential.update",
			"v2.credential.update is not implemented: the Consensus runtime keeps no v2 credential store, so there is no credential to update; this shim serves only the declared DELETE (v2.credential.remove) idempotent no-op")
		return
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use DELETE")
		return
	}

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
