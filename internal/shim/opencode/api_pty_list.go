package opencode

import (
	"net/http"
)

// handleApiPty serves the exact /api/pty mount — upstream v2.pty.list (GET)
// and v2.pty.create (POST) of the pinned upstream document
// (specs/openapi/upstream/openapi-1.18.33.json):
//
//	GET  /api/pty — v2.pty.list   (ROUTE-ADD-028; baseline-report source item
//	                               SHIM-DRIFT-024, the committed artifact's
//	                               positional id SHIM-DRIFT-001)
//	POST /api/pty — v2.pty.create (baseline-report source item SHIM-DRIFT-025)
//
// Before this change the path had no shim route at all, so every method fell
// through the mux to net/http's default 404 — the declared-vs-served
// comparison's NOT-SERVED class, indistinguishable from "no such opencode
// operation".
//
// ch:trace row=ROUTE-ADD-028 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.pty.list test=internal/shim/opencode/api_pty_list_test.go evidence=docs/evidence/ROUTE-ADD-028-live-probe.md witness=none:unattended-worker-session
//
// TRUTHFULNESS — the decision rule the bare /pty mount follows (ROUTE-ADD-102
// pty.list / ROUTE-ADD-103 pty.create):
//
// GET v2.pty.list answers its full declared 200 {location: LocationInfo,
// data: Pty[]}. The declared Pty requires id (pattern ^pty), title, command,
// args, cwd, status (running|exited) and pid — and this runtime keeps NO pty
// registry and spawns no pseudo-terminals (the same truth ROUTE-ADD-102
// records for the v1 pty.list), so the truthful data payload is the empty,
// non-null array. Any entry would assert a terminal that does not exist.
//
// POST v2.pty.create is served by apiPtyCreate (api_pty_create.go,
// ROUTE-ADD-029): the declared 400 InvalidRequestError arm is answered for a
// request that violates the declared input contract (location selector, or the
// required requestBody/its schema), and a well-formed request keeps the
// explicit intentional narrowing — the typed not_implemented envelope naming
// pty.create, exactly as bare POST /pty does. The declared 200 Pty body stays
// unserved because it would fabricate a spawned process: this runtime keeps no
// pty registry (SPEC-017 §3.9).
//
// Every other method keeps the sibling METHOD_NOT_ALLOWED envelope (the
// ROUTE-ADD-005 convention shared by the whole v2 list family).
//
// The declared deepObject `location` selector is validated by the shared
// apiV2LocationSelector, so malformed input answers the declared
// InvalidRequestError 400; unauthenticated requests answer the declared
// UnauthorizedError 401 through the /api/* auth wall's v2 envelope
// (server.go v2EnvelopeRoutePaths). Auth is otherwise untouched: the /api/pty
// path is not a stub path and carries no fixed-workspace exemption, so it
// keeps the standard api-key policy exactly like the v1 /pty routes.
//
// Exact path only — the declared /api/pty/{ptyID} family (v2.pty.get /
// v2.pty.update / v2.pty.remove / v2.pty.connect / v2.pty.connectToken) keeps
// its own rows; deeper /api/pty/* sub-paths keep the net/http default 404 they
// are classified with.
func (s *Server) handleApiPty(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.apiPtyList(w, r)
	case http.MethodPost:
		s.apiPtyCreate(w, r)
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET or POST")
	}
}

// apiPtyList answers upstream v2.pty.list (ROUTE-ADD-028). The declared 200
// body is {location: LocationInfo, data: Pty[]}; the shim has no pty registry
// and spawns no pseudo-terminals, so it serves the typed empty array — never
// null, never an invented terminal (the ROUTE-ADD-102 v1 pty.list rule). The
// optional deepObject location selector is validated first by the shared
// apiV2LocationSelector: a blank value, an unknown location[...] field, a
// non-^wrk workspace or an unreadable/non-directory location[directory]
// answers the declared InvalidRequestError 400, so a malformed request is
// never served as a successful (empty) list for a workspace nobody selected.
func (s *Server) apiPtyList(w http.ResponseWriter, r *http.Request) {
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{
		"location": location.payload,
		"data":     []any{},
	})
}
