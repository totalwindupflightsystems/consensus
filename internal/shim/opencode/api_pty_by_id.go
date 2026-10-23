package opencode

import (
	"net/http"
	"strings"
)

// handleApiPtyByID serves the exact /api/pty/{ptyID} path — the declared v2
// PTY operation family of the pinned upstream document
// (specs/openapi/upstream/openapi-1.18.33.json):
//
//	GET    /api/pty/{ptyID} → v2.pty.get    (ROUTE-ADD-031)
//	PUT    /api/pty/{ptyID} → v2.pty.update (ROUTE-ADD-032)
//	DELETE /api/pty/{ptyID} → v2.pty.remove (ROUTE-ADD-030; the committed
//	                         artifact's positional id SHIM-DRIFT-001)
//
// Before this change the path had no shim route at all, so every method fell
// through the mux to net/http's default 404 — the declared-vs-served
// comparison's NOT-SERVED class. The three declared operations share one exact
// path pattern, so ONE registration dispatches on r.Method, exactly like the
// v1 handlePtyByID family (pty_routes.go, ROUTE-ADD-105/106/107); registering
// each method separately would panic (duplicate mux pattern) and a partial
// registration would reclassify the declared siblings on the same path as
// METHOD-MISSING, which TestNotImplementedTableMatchesDeclaredSurfaceArtifact
// forbids.
//
// ch:trace row=ROUTE-ADD-030 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.remove test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
// ch:trace row=ROUTE-ADD-031 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.get test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
// ch:trace row=ROUTE-ADD-032 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.update test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
//
// TRUTHFULNESS — the decision rule the v1 /pty/{ptyID} family follows
// (pty_routes.go top comment): the Consensus runtime keeps NO pty registry and
// spawns no pseudo-terminal, so the declared success bodies are never
// fabricated and the declared typed 404 is the answer for every well-formed
// ptyID. The three v2 arms therefore delegate to the v1 handlers
// (s.ptyGet / s.ptyUpdate / s.ptyRemove), which serve the shared declared
// error vocabulary: an ill-shaped ptyID (the declared path-parameter pattern
// ^pty) answers the declared 400 BadRequestError with kind Params, a blank or
// non-directory directory/workspace selector answers the declared 400 with
// kind Query, and every well-formed id answers the declared 404 typed
// PtyNotFoundError {_tag, ptyID, message}. The declared success codes (pty.get
// / pty.update 200 Pty, pty.remove 204) stay unserved by design — the
// documented error-contract narrowing (x-shim-error-contract-narrowing on the
// served-surface rows), because no PTY registry exists to name, update or
// remove.
//
// Undeclared methods keep the PRE-CHANGE answer: net/http's default 404
// (ptyMethodNotRouted). 405 is not in any of the three declared response sets
// on this path.
//
// Auth is untouched: the /api/pty/{ptyID} path is not a stub path and carries
// no fixed-workspace exemption, so it keeps the standard api-key policy; the
// declared 401 arm is the v2 UnauthorizedError envelope via the exact
// one-segment v2EnvelopeRoutePaths match (server.go).
//
// No /api/pty/* catch-all is introduced: the declared sibling sub-paths
// /api/pty/{ptyID}/connect and /api/pty/{ptyID}/connect-token keep their own
// rows (ROUTE-ADD-033/034) and stay NOT-SERVED, and deeper /api/pty/* sub-paths
// keep the net/http default 404.
func (s *Server) handleApiPtyByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.ptyGet(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	case http.MethodPut:
		s.ptyUpdate(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	case http.MethodDelete:
		s.ptyRemove(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	default:
		ptyMethodNotRouted(w, r)
	}
}
