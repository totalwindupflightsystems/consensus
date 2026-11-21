package opencode

// WIRE-CONSENSUS-2 — the wire law, pinned.
//
// The law: a caller must never receive a SUCCESS status for work that did not
// happen. A route the shim does not implement has exactly one honest answer —
// HTTP 501 with a body that says so — and the SERVED OpenAPI document must
// declare that state, so a generated client can see before it calls that the
// operation cannot succeed.
//
// Two directions are pinned here:
//
//	wire → spec  (TestUnimplementedRoutesAnswer501AndTheServedSpecDeclaresIt)
//	    every mounted dispatch that answers 501 answers exactly 501 (never 200)
//	    and the document served at /doc declares it: a 501 response on the
//	    operation, an x-not-implemented path item, or the x-undeclared-methods
//	    note the shim uses for sub-paths the pinned upstream document does not
//	    declare at all.
//
//	spec → body  (TestServedSpecNeverDeclaresASuccessThatDescribesAStub)
//	    no 2xx response in the served document describes itself as a stub /
//	    unimplemented / placeholder. A 200 that describes itself is the exact
//	    failure this row removed from the handlers; this half keeps the
//	    contract from re-introducing it on paper.

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// stubMarkerText matches the vocabulary a response description must never use
// for a 2xx arm. Mirrors scripts/audit/wirecheck.py's RE_STUB vocabulary.
var stubMarkerText = regexp.MustCompile(`(?i)\b(stub|not[_ ]implemented|unimplemented|placeholder)\b`)

// unimplementedRoute is one mounted dispatch that performs no work.
//
// specPath is the path template in the SERVED document that must document the
// refusal; it is the declared parent for sub-path families (a 501 on
// /session/{sessionId}/revert is documented by the /session/{sessionId} item).
type unimplementedRoute struct {
	name     string // why it is unimplemented / which operation it names
	method   string
	path     string
	body     string
	ct       string
	specPath string
	// marker is a string the wire body must carry so the caller is told, in
	// the response itself, that the operation was not performed.
	marker string
}

func unimplementedRouteTable() []unimplementedRoute {
	return []unimplementedRoute{
		{"pty.create", http.MethodPost, "/pty", "", "", "/pty", "not_implemented"},
		{"v2.pty.create", http.MethodPost, "/api/pty", "{}", "application/json", "/api/pty", "not_implemented"},
		{"mcp management (GET)", http.MethodGet, "/mcp", "", "", "/mcp", "NOT_IMPLEMENTED"},
		{"mcp management (POST)", http.MethodPost, "/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "application/json", "/mcp", "NOT_IMPLEMENTED"},
		{"vcs.apply / vcs.diff.raw (GET)", http.MethodGet, "/vcs/branches", "", "", "/vcs/{vcsId}", "NOT_IMPLEMENTED"},
		{"vcs.apply / vcs.diff.raw (POST)", http.MethodPost, "/vcs/branches", "", "", "/vcs/{vcsId}", "NOT_IMPLEMENTED"},
		{"instance sub-path command", http.MethodGet, "/instance/command", "", "", "/instance", "NOT_IMPLEMENTED"},
		{"instance sub-path vcs/status", http.MethodGet, "/instance/vcs/status", "", "", "/instance", "NOT_IMPLEMENTED"},
		{"find.symbols (non-GET)", http.MethodPost, "/find/symbol?query=x", "", "", "/find/symbol", "not_implemented"},
		{"TUI submit-prompt (GET)", http.MethodGet, "/tui/submit-prompt", "", "", "/tui/{action}", "NOT_IMPLEMENTED"},
		{"TUI submit-prompt (POST)", http.MethodPost, "/tui/submit-prompt", `{}`, "application/json", "/tui/{action}", "NOT_IMPLEMENTED"},
		{"session.prompt_async", http.MethodGet, "/session/s1/prompt_async", "", "", "/session/{sessionId}", "NOT_IMPLEMENTED"},
		{"session.revert", http.MethodGet, "/session/s1/revert", "", "", "/session/{sessionId}", "NOT_IMPLEMENTED"},
		{"session.summarize", http.MethodGet, "/session/s1/summarize", "", "", "/session/{sessionId}", "NOT_IMPLEMENTED"},
		{"session.diff (non-GET)", http.MethodPost, "/session/s1/diff", "", "", "/session/{sessionId}", "not_implemented"},
		{"session.fork (non-GET)", http.MethodPut, "/session/s1/fork", "", "", "/session/{sessionId}", "NOT_IMPLEMENTED"},
	}
}

// TestUnimplementedRoutesAnswer501AndTheServedSpecDeclaresIt is the AC2 gate:
// the wire and the served contract agree, per route, in one test.
func TestUnimplementedRoutesAnswer501AndTheServedSpecDeclaresIt(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	doc := servedDocFromShim(t, srv.URL)
	paths := specPathItems(t, doc)
	routes := unimplementedRouteTable()
	if len(routes) == 0 {
		t.Fatal("no unimplemented routes pinned — the table is the contract")
	}

	for _, rt := range routes {
		t.Run(rt.name+" "+rt.method+" "+rt.path, func(t *testing.T) {
			status, body := shimCall(t, srv.URL, rt)
			if status != http.StatusNotImplemented {
				t.Fatalf("%s %s answered %d, want 501 — a caller must never receive a success status for work that did not happen. Body: %s",
					rt.method, rt.path, status, body)
			}
			if !strings.Contains(body, rt.marker) {
				t.Fatalf("%s %s answered 501 but the body does not say the operation was not performed (want marker %q). Body: %s",
					rt.method, rt.path, rt.marker, body)
			}
			if ok, why := specDeclaresRefusal(paths, rt.specPath, rt.method); !ok {
				t.Fatalf("%s %s is answered 501 but the served document does not declare it: %s", rt.method, rt.path, why)
			}
		})
	}
}

// TestServedSpecNeverDeclaresASuccessThatDescribesAStub is the contract half:
// no 2xx arm may describe itself as a stub. This is the paper version of the
// defect the handlers carried (a 200 that describes itself).
func TestServedSpecNeverDeclaresASuccessThatDescribesAStub(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	doc := servedDocFromShim(t, srv.URL)
	checked := 0
	for path, rawItem := range specPathItems(t, doc) {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		for method, rawOp := range item {
			op, ok := rawOp.(map[string]any)
			if !ok {
				continue // x-not-implemented and friends
			}
			responses, _ := op["responses"].(map[string]any)
			for code, rawResp := range responses {
				if !strings.HasPrefix(code, "2") {
					continue
				}
				checked++
				text := responseText(rawResp)
				if m := stubMarkerText.FindString(text); m != "" {
					t.Errorf("%s %s declares a %s response whose own text says %q — a success arm that describes itself as unimplemented is exactly the lie this row removed: %q",
						strings.ToUpper(method), path, code, m, text)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("served document declares no 2xx responses — nothing was checked")
	}
	t.Logf("%d declared 2xx response(s) carry no stub language", checked)
}

// TestSessionCreateNeverFabricatesASession pins the same law on a route this
// row IMPLEMENTED rather than narrowed: POST /api/session (v2.session.create)
// runs the real create. When the runtime service is absent it must refuse with
// the declared 400 — never a 200 carrying an invented session id.
func TestSessionCreateNeverFabricatesASession(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, body := shimCall(t, srv.URL, unimplementedRoute{
		name:   "v2.session.create",
		method: http.MethodPost,
		path:   "/api/session",
		body:   `{"agent_name":"build","goal":"prove the law"}`,
		ct:     "application/json",
	})
	if status == http.StatusOK {
		t.Fatalf("POST /api/session answered 200 — a create that did no work must not report success. Body: %s", body)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session answered %d, want the declared 400 when the runtime service cannot create. Body: %s", status, body)
	}
	if strings.Contains(body, `"id"`) {
		t.Fatalf("POST /api/session refused but the body carries a session id: %s", body)
	}
	if !strings.Contains(body, "session") {
		t.Fatalf("POST /api/session refused without naming what failed: %s", body)
	}
}

// servedDocFromShim fetches the document the shim actually serves at /doc and
// decodes it, so every assertion is about the SERVED contract — not about a
// file on disk that the binary may not have been rebuilt from.
func servedDocFromShim(t *testing.T, base string) map[string]any {
	t.Helper()
	resp, err := http.Get(base + "/doc")
	if err != nil {
		t.Fatalf("GET /doc: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /doc answered %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /doc body: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode /doc as JSON: %v", err)
	}
	return doc
}

func specPathItems(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("served document declares no paths")
	}
	return paths
}

// specDeclaresRefusal answers whether the served document tells a client this
// path can refuse with 501, and names the reason when it does not.
func specDeclaresRefusal(paths map[string]any, specPath, method string) (bool, string) {
	rawItem, ok := paths[specPath]
	if !ok {
		return false, "the document declares no path item " + specPath
	}
	item, ok := rawItem.(map[string]any)
	if !ok {
		return false, "path item " + specPath + " is not an object"
	}
	if v, ok := item["x-not-implemented"].(bool); ok && v {
		return true, ""
	}
	if _, ok := item["x-undeclared-methods"]; ok {
		return true, ""
	}
	op, ok := item[strings.ToLower(method)].(map[string]any)
	if !ok {
		return false, "the document declares no " + method + " operation on " + specPath
	}
	responses, _ := op["responses"].(map[string]any)
	if _, ok := responses["501"]; ok {
		return true, ""
	}
	return false, "the " + method + " operation on " + specPath + " declares no 501 response, and the path item carries neither x-not-implemented nor x-undeclared-methods"
}

// shimCall performs one probe and returns (status, body).
func shimCall(t *testing.T, base string, rt unimplementedRoute) (int, string) {
	t.Helper()
	var reader io.Reader
	if rt.body != "" {
		reader = strings.NewReader(rt.body)
	}
	req, err := http.NewRequest(rt.method, base+rt.path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", rt.method, rt.path, err)
	}
	if rt.ct != "" {
		req.Header.Set("Content-Type", rt.ct)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", rt.method, rt.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(body)
}

// responseText flattens a declared response (description + every schema) into
// one string so a marker anywhere in the arm is caught.
func responseText(raw any) string {
	data, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(data)
}
