package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Upstream contract (openapi-1.18.33.json paths."/api/pty".post =
// v2.pty.create, ROUTE-ADD-029): one optional deepObject `location` selector
// (location[directory] / location[workspace], additionalProperties:false), a
// REQUIRED application/json requestBody — object, additionalProperties:false,
// members command (string), args (string[]), cwd (string), title (string),
// env (object of string), NONE of them required — and declared responses
// 200 {location: LocationInfo, data: Pty}, 400 InvalidRequestError,
// 401 UnauthorizedError.
//
// These tests pin what this shim ANSWERS on that surface:
//
//   - 401 UnauthorizedError for an unauthenticated request (the /api/* v2
//     envelope, before any input handling — the auth wall sits in front of the
//     mux, so a malformed body cannot be used to probe the route);
//   - 400 InvalidRequestError for a request that violates the declared input
//     contract: absent body (the operation marks requestBody required),
//     unparseable JSON, a non-object JSON value, more than one JSON value, an
//     undeclared member (additionalProperties:false), a declared member of the
//     wrong JSON type, and a malformed `location` selector;
//   - the typed 501 not_implemented narrowing for every WELL-FORMED request.
//
// The declared 200 {location, data: Pty} is DELIBERATELY not served: it needs
// a real Pty (id ^pty, title, command, args, cwd, status, pid) and this
// runtime keeps no pty registry and spawns no pseudo-terminals (SPEC-017
// §3.9, the ROUTE-ADD-102/ROUTE-ADD-028 truth rule). The 200 arm is pinned as
// UNREACHABLE below, so a later change cannot start fabricating a spawned
// terminal without failing a test.

// newApiPtyCreateServer builds the ROUTE-ADD-029 fixture with auth bypassed so
// the declared 400 arm and the narrowing are exercised directly. The handler
// reads no store (there is no pty registry), so the mock DB is sufficient.
func newApiPtyCreateServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// assertV2InvalidRequest asserts the declared 400 InvalidRequestError body
// (the v2 NamedError shape the sibling GET arm emits): exact member set, a
// non-empty message, and — when the request named one — the offending
// kind/field.
func assertV2InvalidRequest(t *testing.T, body []byte, wantKind, wantField string) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError (the declared 400 body). Body: %s", got["_tag"], body)
	}
	if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("400 body carries no message. Body: %s", body)
	}
	if wantKind != "" {
		if got["kind"] != wantKind {
			t.Errorf("kind = %v, want %q. Body: %s", got["kind"], wantKind, body)
		}
	}
	if wantField != "" {
		if got["field"] != wantField {
			t.Errorf("field = %v, want %q (the offending input). Body: %s", got["field"], wantField, body)
		}
	}
}

// TestApiPtyCreateDeclared401Unauthorized pins the declared UnauthorizedError
// arm: an unauthenticated POST answers 401 with the v2 envelope
// {_tag: UnauthorizedError, message} — proven with a malformed body too, so
// the auth wall is shown to run BEFORE body validation (an unauthenticated
// caller cannot learn whether its payload would have been accepted).
func TestApiPtyCreateDeclared401Unauthorized(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"no body", ""},
		{"valid body", `{"command":"/bin/sh","args":["-l"],"cwd":"/tmp","title":"demo","env":{"TERM":"xterm"}}`},
		{"malformed body", `{"bogus":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var status int
			var header http.Header
			var body []byte
			if tc.body == "" {
				status, header, body = doShimRequest(t, srv.URL, http.MethodPost, "/api/pty")
			} else {
				status, header, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty", tc.body)
			}
			if status != http.StatusUnauthorized {
				t.Fatalf("unauthenticated POST /api/pty (%s): got %d, want the declared 401. Body: %s", tc.name, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("401 body is not JSON: %v (%s)", err, body)
			}
			assertExactMembers(t, got, "_tag", "message")
			if got["_tag"] != "UnauthorizedError" {
				t.Errorf("_tag = %v, want UnauthorizedError (the declared 401 body)", got["_tag"])
			}
		})
	}
}

// TestApiPtyCreateDeclared400MalformedBody pins the declared
// InvalidRequestError 400 arm for the required v2.pty.create requestBody. Each
// case is input the pinned schema refuses: the body is required by the
// operation, must be exactly one JSON OBJECT, may carry only the five declared
// members, and each member must have its declared JSON type (a JSON null is a
// type violation — no member declares a null arm).
func TestApiPtyCreateDeclared400MalformedBody(t *testing.T) {
	_, srv := newApiPtyCreateServer(t)

	cases := []struct {
		name      string
		body      string
		wantKind  string
		wantField string
	}{
		{"unparseable JSON", `{"command":`, "Body", ""},
		{"empty string body", " ", "Body", ""},
		{"JSON string instead of object", `"create a terminal"`, "Body", ""},
		{"JSON array instead of object", `["/bin/sh"]`, "Body", ""},
		{"JSON null body", `null`, "Body", ""},
		{"two JSON values", `{} {}`, "Body", ""},
		{"undeclared member", `{"bogus":true}`, "Payload", "bogus"},
		{"undeclared member beside a valid one", `{"command":"/bin/sh","shell":"/bin/zsh"}`, "Payload", "shell"},
		{"undeclared member beside a valid one (cols)", `{"command":"/bin/sh","cols":80}`, "Payload", "cols"},
		{"command wrong type", `{"command":42}`, "Payload", "command"},
		{"command null", `{"command":null}`, "Payload", "command"},
		{"cwd wrong type", `{"cwd":["/tmp"]}`, "Payload", "cwd"},
		{"title wrong type", `{"title":false}`, "Payload", "title"},
		{"args not an array", `{"args":"-l"}`, "Payload", "args"},
		{"args element wrong type", `{"args":["-l",7]}`, "Payload", "args[1]"},
		{"args element null", `{"args":[null]}`, "Payload", "args[0]"},
		{"env not an object", `{"env":["TERM=xterm"]}`, "Payload", "env"},
		{"env value wrong type", `{"env":{"TERM":123}}`, "Payload", "env.TERM"},
		{"env value null", `{"env":{"TERM":null}}`, "Payload", "env.TERM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST /api/pty (%s): got %d, want the declared 400. Body: %s", tc.name, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertV2InvalidRequest(t, body, tc.wantKind, tc.wantField)
		})
	}
}

// TestApiPtyCreateDeclared400AbsentBody pins the one body arm the operation's
// own declaration makes: v2.pty.create marks requestBody `"required": true`
// (not merely its members), so a POST with no body at all is the declared 400
// InvalidRequestError — never a 200, never the 501 narrowing (which states
// something about the runtime's capability and must not stand in for input the
// shim never received). An EMPTY OBJECT is the smallest well-formed body: no
// member is required, so `{}` reaches the narrowing.
func TestApiPtyCreateDeclared400AbsentBody(t *testing.T) {
	_, srv := newApiPtyCreateServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/pty")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/pty with no body: got %d, want the declared 400 (requestBody is required). Body: %s", status, body)
	}
	assertV2InvalidRequest(t, body, "Body", "")

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty", `{}`)
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /api/pty with `{}`: got %d, want the typed 501 narrowing (an empty object is well-formed: no member is required). Body: %s", status, body)
	}
}

// TestApiPtyCreateDeclared400MalformedSelector pins the declared 400 arm for
// the `location` deepObject selector this operation declares — the same
// validation the sibling GET arm serves, so a POST cannot smuggle a malformed
// workspace selection past the list route's contract. A valid body is sent so
// the refusal is attributable to the selector and not the body.
func TestApiPtyCreateDeclared400MalformedSelector(t *testing.T) {
	s, srv := newApiPtyCreateServer(t)

	cases := []struct{ name, query, field string }{
		{"blank directory", "?location%5Bdirectory%5D=", "location[directory]"},
		{"blank workspace", "?location%5Bworkspace%5D=", "location[workspace]"},
		{"unknown field", "?location%5Bbogus%5D=x", "location[bogus]"},
		{"workspace without wrk prefix", "?location%5Bworkspace%5D=ws-1", "location[workspace]"},
		{"unreadable directory", "?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), "location[directory]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty"+tc.query, `{"command":"/bin/sh"}`)
			if status != http.StatusBadRequest {
				t.Fatalf("POST /api/pty%s: got %d, want the declared 400. Body: %s", tc.query, status, body)
			}
			assertV2InvalidRequest(t, body, "Query", tc.field)
		})
	}
}

// TestApiPtyCreateWellFormedAnswersTyped501AndNeverFabricatesPty pins both
// halves of the truth for a well-formed request: the answer is the typed
// not_implemented envelope naming v2.pty.create (SHIM-GAP-002 — never 404/405
// from a registered route, never a bare 501), and the declared 200
// {location, data: Pty} is NOT produced. The second half is the guard against
// "finishing" this row by inventing a terminal: no response may carry a Pty
// member or a pty-shaped id, because no process was spawned.
func TestApiPtyCreateWellFormedAnswersTyped501AndNeverFabricatesPty(t *testing.T) {
	_, srv := newApiPtyCreateServer(t)

	bodies := []struct{ name, body string }{
		{"empty object", `{}`},
		{"command only", `{"command":"/bin/sh"}`},
		{
			"every declared member",
			`{"command":"/bin/sh","args":["-l","-c","echo hi"],"cwd":"/tmp","title":"demo","env":{"TERM":"xterm-256color","LANG":"C"}}`,
		},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty", tc.body)
			if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
				t.Fatalf("POST /api/pty answered %d — a registered route may not lie with 404/405. Body: %s", status, body)
			}
			if status != http.StatusNotImplemented {
				t.Fatalf("POST /api/pty (%s): got %d, want the typed 501 narrowing. Body: %s", tc.name, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("501 body is not JSON: %v (%s)", err, body)
			}
			assertExactMembers(t, got, "error", "operation", "detail")
			if got["error"] != "not_implemented" {
				t.Errorf("error = %v, want \"not_implemented\"", got["error"])
			}
			if got["operation"] != "v2.pty.create" {
				t.Errorf("operation = %v, want v2.pty.create (the declared operation the envelope must name)", got["operation"])
			}
			if detail, _ := got["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Error("detail must name what is missing; got empty")
			}

			// The fabrication guard: no location/data envelope, no pty object.
			for _, fabricated := range []string{"location", "data", `"pid"`, `"status":"running"`} {
				if strings.Contains(string(body), fabricated) {
					t.Errorf("501 body mentions %s — a well-formed request must not produce the declared Pty success shape without a spawned terminal. Body: %s", fabricated, body)
				}
			}
		})
	}
}

// TestApiPtyCreateMethodGuard pins the sibling method envelope: POST and GET
// are the only methods the exact path answers, and the narrowing did not turn
// the path into a catch-all for other verbs.
func TestApiPtyCreateMethodGuard(t *testing.T) {
	_, srv := newApiPtyCreateServer(t)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/api/pty", `{"command":"/bin/sh"}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/pty: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s /api/pty 405 body is not JSON: %v (%s)", method, err, body)
		}
	}
}

// TestApiPtyCreateV1RouteUntouched pins the boundary of ROUTE-ADD-029: only
// the v2 /api/pty POST gained the declared 400 arm. The v1 sibling routes keep
// their pre-existing answers (bare POST /pty still narrows with the typed 501
// and no body validation, bare GET /pty still lists).
func TestApiPtyCreateV1RouteUntouched(t *testing.T) {
	_, srv := newApiPtyCreateServer(t)

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/pty", `{"bogus":true}`)
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /pty (v1 pty.create): got %d, want the unchanged typed 501. Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /pty (v1 pty.list): got %d, want the unchanged 200. Body: %s", status, body)
	}
}

// TestApiPtyCreateTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId, this test file and the live-probe
// evidence, so the traceability link cannot silently drift from the served
// surface.
func TestApiPtyCreateTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_pty_create.go")
	if err != nil {
		t.Skipf("api_pty_create.go not readable from the test working directory: %v", err)
	}
	source := string(raw)
	want := fmt.Sprintf("row=ROUTE-ADD-029 spec=specs/openapi/upstream/openapi-1.18.33.json#%s test=internal/shim/opencode/api_pty_create_test.go", "v2.pty.create")
	if !strings.Contains(source, want) {
		t.Errorf("api_pty_create.go carries no ch:trace line for v2.pty.create (want %q)", want)
	}
	if !strings.Contains(source, "evidence=docs/evidence/ROUTE-ADD-029-live-probe.md") {
		t.Error("api_pty_create.go does not point at the ROUTE-ADD-029 live-probe evidence")
	}
	if !strings.Contains(source, "witness=none:unattended-worker-session") {
		t.Error("api_pty_create.go does not record the unattended worker witness")
	}
}
