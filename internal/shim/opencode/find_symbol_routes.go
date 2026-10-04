package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// findSymbols serves GET /find/symbol — upstream find.symbols
// (ROUTE-FIX-002; SHIM-DRIFT-085 in
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json, declared
// responses: 200 Symbol[] "Symbols", 400 BadRequestError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/find/symbol".get): the required ?query= parameter names the
// workspace symbols to search (functions, classes, variables via LSP), with
// optional ?directory= / ?workspace= selectors scoping the search.
//
// Truthfulness — why the declared 200 is answered with the EMPTY list. The
// Consensus shim has no LSP integration: handleLSP reports
// {"enabled":false,"status":"unavailable"} unconditionally, no language
// server is spawned or wired anywhere in the runtime, and there is no
// symbol table (no gopls/ctags index, no store) to query. A non-empty
// response would therefore fabricate Symbol entries (name, kind,
// location.uri, location.range) that no tool produced — the exact
// fabrication the handleSyncStart / handleGlobalUpgrade truthfulness
// convention (13189b1) and the question.list precedent (ROUTE-ADD-110)
// forbid. The handler validates the request, reports no symbol producer,
// and answers [] — an array, never null (the document declares an array).
//
// Every malformed request answers the object's only declared error code,
// 400: a missing/blank ?query= (sibling handleFind ?pattern= convention)
// and a present-but-blank directory/workspace (sibling handleFileList
// convention). The declared query selectors are validated but do not
// narrow the (empty) result — the singleton-instance convention shared by
// session.todo (ROUTE-FIX-039).
//
// The dispatch in server.go keeps a 501 for undeclared methods on the
// sub-path (405 is not part of this operation's declared response set),
// typed per the session.diff sibling convention (ROUTE-FIX-010).
//
// ch:trace row=ROUTE-FIX-002 spec=specs/openapi/upstream/openapi-1.18.33.json#find.symbols test=TestFindSymbols* doc=docs/evidence/ROUTE-FIX-002-live-probe.md evidence=docs/evidence/ROUTE-FIX-002-live-probe.md witness=none:self-verified-in-worktree
func (s *Server) findSymbols(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if v, ok := q[param]; ok && strings.TrimSpace(v[0]) == "" {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	if strings.TrimSpace(q.Get("query")) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "?query= is required")
		return
	}
	// No LSP integration → no symbol producer → the truthful empty list.
	writeJSON(w, []any{})
}
