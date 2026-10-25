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

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/pty".get = v2.pty.list,
// ROUTE-ADD-028): GET /api/pty, one optional deepObject `location` query
// selector (location[directory] / location[workspace],
// additionalProperties:false in the pinned document), declared responses
// 200 {location: LocationInfo, data: Pty[]} ("List PTY sessions for a
// location, including exited sessions retained until removal."), 400
// InvalidRequestError, 401 UnauthorizedError. The path had no shim route at
// all before this change and answered net/http's default 404 (the
// declared-vs-served artifact's NOT-SERVED class; baseline-report id
// SHIM-DRIFT-024, the committed artifact's positional id SHIM-DRIFT-001).
//
// The declared sibling method on the same exact path, POST /api/pty
// (v2.pty.create), is reached by the registration and answers the typed
// not-implemented envelope (SHIM-GAP-002) — the ROUTE-ADD-103 intentional
// narrowing beside bare /pty (baseline id SHIM-DRIFT-025).
//
// Pty requires id (pattern ^pty), title, command, args, cwd, status
// (running|exited) and pid; this runtime keeps no pty registry and spawns no
// pseudo-terminals (the ROUTE-ADD-102 pty.list truthfulness rule), so the
// declared 200 answers an empty, non-null data array — never an invented
// terminal. Every 200 schema sets additionalProperties:false, so the tests
// assert the exact member set (an undeclared member is a contract violation,
// not an enhancement).

// apiPtyListResponse is the declared {location: LocationInfo, data: Pty[]}
// envelope this operation answers with.
type apiPtyListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []json.RawMessage `json:"data"`
}

// newApiPtyListServer builds the ROUTE-ADD-028 fixture. The handler reads no
// store — the runtime keeps no pty registry (the empty list IS the truthful
// answer) — so the mock DB is sufficient; auth is bypassed so the 200/400/501
// arms are exercised directly.
func newApiPtyListServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// TestApiPtyListServesDeclared200 pins v2.pty.list (ROUTE-ADD-028): the exact
// /api/pty path serves the declared operation with the declared 200 shape
// instead of the pre-fix net/http default 404 (class NOT-SERVED: no shim route
// at all). data must be the empty, non-null Pty[] — the runtime spawns no
// pseudo-terminals — and location must be the real selected workspace.
func TestApiPtyListServesDeclared200(t *testing.T) {
	_, srv := newApiPtyListServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /api/pty: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/pty body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiPtyListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/pty body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Errorf("data = null, want the declared Pty[] array (an empty array is not null): %s", body)
	}
	if len(got.Data) != 0 {
		t.Errorf("data = %s, want the empty array — the runtime spawns no pseudo-terminals, so any entry would be an invented terminal", body)
	}
	if got.Location.Directory == "" {
		t.Errorf("location.directory is empty, want the selected workspace directory")
	}
	if got.Location.Project.ID == "" || got.Location.Project.Directory == "" {
		t.Errorf("location.project = %+v, want id and directory (LocationInfo.project required fields)", got.Location.Project)
	}
	if got.Location.Project.Directory != got.Location.Directory {
		t.Errorf("location.project.directory = %q, want the selected directory %q", got.Location.Project.Directory, got.Location.Directory)
	}
}

// TestApiPtyListServesSelectedWorkspace pins the deepObject location selector
// override: a well-formed location[directory] is honoured in the declared
// location envelope (the same derivation the ROUTE-ADD-021 list family uses),
// which proves the 400 arm below is the malformed-input branch and not a
// blanket refusal of the selector.
func TestApiPtyListServesSelectedWorkspace(t *testing.T) {
	_, srv := newApiPtyListServer(t)

	dir := t.TempDir()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty?location%5Bdirectory%5D="+dir)
	if status != http.StatusOK {
		t.Fatalf("GET /api/pty with location[directory]: got %d, want 200. Body: %s", status, body)
	}
	var got apiPtyListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != dir {
		t.Errorf("location.directory = %q, want the requested %q", got.Location.Directory, dir)
	}
	if len(got.Data) != 0 {
		t.Errorf("data = %s, want the empty array for the requested workspace too", body)
	}
}

// TestApiPtyListDeclared400MalformedSelector pins the declared
// InvalidRequestError 400 arm for the deepObject `location` selector this
// operation declares (location[directory] / location[workspace], with
// additionalProperties:false in the pinned document): a blank value, an unknown
// location[...] field, a workspace id outside the declared ^wrk pattern and an
// unreadable/non-directory location[directory] are all malformed input, and a
// malformed selector must never be answered as a successful empty pty list.
func TestApiPtyListDeclared400MalformedSelector(t *testing.T) {
	s, srv := newApiPtyListServer(t)

	cases := []struct {
		name  string
		query string
		field string
	}{
		{"blank directory", "?location%5Bdirectory%5D=", "location[directory]"},
		{"blank workspace", "?location%5Bworkspace%5D=", "location[workspace]"},
		{"unknown field", "?location%5Bbogus%5D=x", "location[bogus]"},
		{"workspace without wrk prefix", "?location%5Bworkspace%5D=ws-1", "location[workspace]"},
		{"unreadable directory", "?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), "location[directory]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/pty%s: got %d, want the declared 400. Body: %s", tc.query, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError (the declared 400 body)", got["_tag"])
			}
			if got["message"] == "" {
				t.Error("400 body carries no message")
			}
			if field, _ := got["field"].(string); field != tc.field {
				t.Errorf("400 body names field %q, want %q (the malformed selector)", field, tc.field)
			}
		})
	}
}

// TestApiPtyListCreateAnswersTyped501 pins the SHIM-GAP-002 contract for the
// declared sibling method on the same exact path: POST /api/pty is
// v2.pty.create, and once the path is registered that method reaches the
// handler — where a WELL-FORMED request must answer the typed not-implemented
// envelope naming pty.create (the ROUTE-ADD-103 intentional narrowing beside
// bare /pty), never a lying 404 from the registered route and never a bare
// 501. The request carries the declared required requestBody as `{}` (valid:
// no member is required, additionalProperties:false) because ROUTE-ADD-029
// serves the declared 400 arm for a body that violates the schema — the 400
// arms themselves are pinned in api_pty_create_test.go.
func TestApiPtyListCreateAnswersTyped501(t *testing.T) {
	_, srv := newApiPtyListServer(t)

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/api/pty", `{}`)
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/pty answered %d — a registered route may not lie with 404/405 (SHIM-GAP-002). Body: %s", status, body)
	}
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /api/pty: got %d, want 501. Body: %s", status, body)
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
}

// TestApiPtyListMethodNotAllowed pins the method guard: GET and the narrowed
// POST are the only methods the exact path answers; every undeclared method
// keeps the sibling METHOD_NOT_ALLOWED envelope instead of being answered as a
// successful list or an untyped stub.
func TestApiPtyListMethodNotAllowed(t *testing.T) {
	_, srv := newApiPtyListServer(t)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/pty")
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

// TestApiPtyListServeDeclared401 pins the declared UnauthorizedError arm:
// unauthenticated requests answer the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's UNAUTHENTICATED
// body, which an unregistered /api/* path keeps. Both declared operations on
// the path declare the 401, so POST is asserted too (before its 501: the auth
// wall sits in front of the mux).
func TestApiPtyListServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		status, header, body := doShimRequest(t, srv.URL, method, "/api/pty")
		if status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s /api/pty: got %d, want 401. Body: %s", method, status, body)
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
	}
}

// TestApiPtyListMountWiring pins the chi mount contract: /api/pty must be
// listed in MountPatterns (cmd/consensus/main.go mounts the shim by these
// patterns — without the entry a parent chi router answers before the shim
// sees the request), the route must be reachable through that mount, no
// /api/pty/* catch-all may be introduced, and the declared 401 envelope must
// be keyed by this exact path only.
func TestApiPtyListMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/pty"] {
		t.Errorf("/api/pty is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-028)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/pty: got %d, want 200 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: registering the bare /api/pty mount (and since
	// ROUTE-ADD-030 the exact one-segment /api/pty/{ptyID} route) must not
	// swallow deeper sub-paths — /api/pty/a/b keeps the net/http default 404
	// it is classified with. (GET /api/pty/pty1 was asserted 404 here before
	// ROUTE-ADD-030 served the ptyID family; that row now answers the
	// declared 404 typed PtyNotFoundError — same code, a different reason —
	// so the catch-all check moved to the unregistered two-segment shape.)
	for _, path := range []string{"/api/pty/a/b"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 — ROUTE-ADD-028 registers the exact paths, never a catch-all", path, status)
		}
	}

	// The v1 pty surface is a different declared operation family and must be
	// untouched: bare GET /pty (pty.list) keeps its own declared 200.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Errorf("GET /pty (v1 pty.list): got %d, want 200 — the v1 route must be untouched by the v2 registration", status)
	}

	if !isV2EnvelopeRoutePath("/api/pty") {
		t.Error("/api/pty is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	// ROUTE-ADD-030: the declared /api/pty/{ptyID} family (v2.pty.get/update/
	// remove) carries the declared 401 too, so the concrete one-segment shape
	// joins the envelope; deeper /api/pty/* sub-paths keep the shim's own body.
	if !isV2EnvelopeRoutePath("/api/pty/pty_abc") {
		t.Error("/api/pty/pty_abc is not treated as a v2-envelope route — the declared ptyID family carries the v2 UnauthorizedError 401 arm (ROUTE-ADD-030/031/032)")
	}
	// ROUTE-ADD-033 widened the same family by one REGISTERED shape: the
	// declared v2.pty.connect operation on /api/pty/{ptyID}/connect declares
	// the same v2 401 arm, so that concrete sub-path joins the envelope. The
	// membership is still shape-bounded rather than rest-of-prefix: the
	// connect-token sibling (ROUTE-ADD-034, not yet registered under /api) and
	// every other deeper segment keep the shim's own envelope.
	if !isV2EnvelopeRoutePath("/api/pty/pty1/connect") {
		t.Error("/api/pty/pty1/connect is not treated as a v2-envelope route — the declared v2.pty.connect operation carries the v2 UnauthorizedError 401 arm and the shape is registered (ROUTE-ADD-033)")
	}
	for _, path := range []string{"/api/pty/pty1/connect-token", "/api/pty/pty1/extra", "/api/pty/pty1/connect/extra"} {
		if isV2EnvelopeRoutePath(path) {
			t.Errorf("%s is treated as a v2-envelope route; membership must stay the registered shapes (one ptyID segment, optionally the /connect literal)", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/provider") {
		t.Error("a pre-existing v2 route lost its 401 envelope")
	}
}

// TestApiPtyListTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId, this test file and the live-probe
// evidence, so the traceability link cannot silently drift from the served
// surface.
func TestApiPtyListTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_pty_list.go")
	if err != nil {
		t.Skipf("api_pty_list.go not readable from the test working directory: %v", err)
	}
	source := string(raw)
	want := fmt.Sprintf("row=ROUTE-ADD-028 spec=specs/openapi/upstream/openapi-1.18.33.json#%s test=internal/shim/opencode/api_pty_list_test.go", "v2.pty.list")
	if !strings.Contains(source, want) {
		t.Errorf("api_pty_list.go carries no ch:trace line for v2.pty.list (want %q)", want)
	}
	if !strings.Contains(source, "evidence=docs/evidence/ROUTE-ADD-028-live-probe.md") {
		t.Error("api_pty_list.go does not point at the ROUTE-ADD-028 live-probe evidence")
	}
	if !strings.Contains(source, "witness=none:unattended-worker-session") {
		t.Error("api_pty_list.go does not record the unattended worker witness")
	}
}
