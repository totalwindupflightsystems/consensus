package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy".delete):
// DELETE /experimental/project/{projectID}/copy, operationId
// v2.projectCopy.remove, required plain-string path parameter `projectID` (no
// pattern), an optional deepObject `location` query selector
// (directory, workspace; additionalProperties:false), an optional requestBody
// {directory*: string, force*: boolean} (additionalProperties:false), and
// exactly two declared responses: 204 No Content and 400
// ProjectCopyError | InvalidRequestError.
//
// ROUTE-ADD-069 serves that exact contract. Before the fix every method on the
// path fell through the mux to net/http's default 404 — the declared-vs-served
// comparison's NOT-SERVED class (source item SHIM-DRIFT-037 in the committed
// artifact; the board row cited SHIM-DRIFT-066, a positional id from the
// 2026-09-29 baseline report).
//
// RED-pin: TestExperimentalProjectCopyRemoveServesDeclared204 fails with 404
// against the pre-fix tree (the route does not exist) and passes once the exact
// mux entry + handler land. The rest of the battery pins the declared 400 arm,
// the sibling METHOD_NOT_ALLOWED 405, the v2 UnauthorizedError 401 and the
// absence of any /experimental/* catch-all.

func newExperimentalProjectCopyRemoveServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// doCopyRequest is doShimRequest with an optional JSON body: the DELETE's
// declared requestBody IS part of the contract under test, so the arms below
// send one (present-but-invalid) and none (the optional-body rule).
func doCopyRequest(t *testing.T, base, method, path, body string) (int, http.Header, []byte) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data := make([]byte, 0, 1024)
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		data = append(data, buf[:n]...)
		if err != nil {
			break
		}
	}
	return resp.StatusCode, resp.Header, data
}

// assertCopyInvalidRequest pins the declared 400 arm shape: the operation
// declares ProjectCopyError | InvalidRequestError, and the handler answers the
// InvalidRequestError arm ({_tag, message, optional kind, optional field}).
func assertCopyInvalidRequest(t *testing.T, label string, status int, body []byte, wantKind, wantField string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s: got %d, want the declared 400. Body: %s", label, status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("%s: _tag = %v, want InvalidRequestError (a declared 400 arm)", label, got["_tag"])
	}
	if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("%s: message = %q, want a non-empty reason", label, got["message"])
	}
	if wantKind != "" {
		if kind, _ := got["kind"].(string); kind != wantKind {
			t.Errorf("%s: kind = %v, want %q", label, got["kind"], wantKind)
		}
	}
	if wantField != "" {
		if field, _ := got["field"].(string); field != wantField {
			t.Errorf("%s: field = %v, want %q", label, got["field"], wantField)
		}
	}
}

// TestExperimentalProjectCopyRemoveServesDeclared204 pins the declared success
// arm. The runtime keeps NO project-copy registry — nothing in internal/db or
// the migration set records a copied project, and projectCopy.create is not
// served either — so no projectID addresses a copy this runtime holds and the
// declared 204 is the empty-body idempotent no-op of the ROUTE-ADD-025
// saved-permission removal precedent: it fabricates neither a copy nor a
// deletion claim.
//
// Three well-formed shapes are pinned: no body at all (the document marks no
// requestBody.required, so a body is optional), an empty body, and the full
// declared body {directory, force}. The repeated call pins idempotence.
func TestExperimentalProjectCopyRemoveServesDeclared204(t *testing.T) {
	srv := newExperimentalProjectCopyRemoveServer(t)

	bodies := []string{
		"",
		"{}",
		`{"directory":"/tmp","force":true}`,
		`{"directory":"","force":false}`,
	}
	for _, body := range bodies {
		status, header, resp := doCopyRequest(t, srv.URL, http.MethodDelete,
			"/experimental/project/prj-copy-0001/copy", body)
		if status != http.StatusNoContent {
			t.Fatalf("DELETE copy body=%q: got %d, want 204. Body: %s", body, status, resp)
		}
		if len(resp) != 0 {
			t.Errorf("DELETE copy body=%q: 204 body = %q, want empty", body, resp)
		}
		if ct := header.Get("Content-Type"); ct != "" {
			t.Errorf("DELETE copy body=%q: 204 Content-Type = %q, want none", body, ct)
		}
	}

	// Idempotent: removing a copy nothing holds twice answers the same way.
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/prj-copy-0001/copy", "")
	if status != http.StatusNoContent {
		t.Fatalf("second DELETE copy: got %d, want 204. Body: %s", status, resp)
	}
}

// TestExperimentalProjectCopyRemoveMalformedAnswersDeclared400 pins the
// declared 400 arm for every malformed input the pinned document actually
// defines: a present-but-blank {projectID} segment (the declared parameter is a
// plain required string with NO pattern, so blank is the only ill-shaped id the
// contract itself names), a body that violates the declared
// {directory*: string, force*: boolean} additionalProperties:false schema, and
// a malformed deepObject location selector (the shared apiV2LocationSelector
// rule every v2 operation with that declared selector answers).
func TestExperimentalProjectCopyRemoveMalformedAnswersDeclared400(t *testing.T) {
	srv := newExperimentalProjectCopyRemoveServer(t)

	base := "/experimental/project/prj-copy-0001/copy"

	status, _, resp := doCopyRequest(t, srv.URL, http.MethodDelete, base, "")
	if status != http.StatusNoContent {
		t.Fatalf("well-formed baseline: got %d, want 204. Body: %s", status, resp)
	}

	// Present-but-blank path parameter (whitespace-only segment, encoded so it
	// still matches the {projectID} route pattern).
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/%20/copy", "")
	assertCopyInvalidRequest(t, "blank projectID", status, resp, "Path", "projectID")

	// Bodies violating the declared requestBody schema. The declared 400 splits
	// the two conventional kinds the sibling handlers use: Body for a body that
	// is not a single JSON object with only the declared fields, Payload for a
	// declared member with the wrong type or a missing required partner.
	badBodies := []struct {
		body     string
		wantKind string
	}{
		{`[]`, "Body"},
		{`"force"`, "Body"},
		{`{"directory":"/tmp","force":true,"extra":1}`, "Body"},
		{`{"directory":"/tmp","force":true}{"directory":"/tmp","force":true}`, "Body"},
		{`{"directory":"/tmp"}`, "Payload"},
		{`{"force":true}`, "Payload"},
		{`{"directory":1,"force":true}`, "Payload"},
		{`{"directory":"/tmp","force":"yes"}`, "Payload"},
	}
	for _, tc := range badBodies {
		status, _, resp = doCopyRequest(t, srv.URL, http.MethodDelete, base, tc.body)
		assertCopyInvalidRequest(t, "body "+tc.body, status, resp, tc.wantKind, "")
	}

	// Malformed deepObject location selectors (declared spelling:
	// location[directory] / location[workspace]; additionalProperties:false).
	for _, query := range []string{
		"?location[nope]=1",
		"?location[directory]=",
		"?location[workspace]=notawk",
	} {
		status, _, resp = doCopyRequest(t, srv.URL, http.MethodDelete, base+query, "")
		assertCopyInvalidRequest(t, "query "+query, status, resp, "Query", "")
	}

	// A well-formed selector still answers the declared 204.
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodDelete, base, "")
	if status != http.StatusNoContent {
		t.Fatalf("well-formed trailing call: got %d, want 204. Body: %s", status, resp)
	}
}

// TestExperimentalProjectCopyRemoveMethodAndMountWiring pins the exact chi
// mount (the /experimental/* MountPattern already reaches the shim), the
// sibling METHOD_NOT_ALLOWED envelope for every undeclared method, the declared
// sibling POST (v2.projectCopy.create) answered by its OWN served contract
// (ROUTE-ADD-070 retired the typed 501 narrowing — the sibling answers a
// declared 400 for the absent-body shape here, its own tests pin the 200),
// the untouched sibling /experimental/tool routes,
// and the absence of any /experimental/* catch-all.
func TestExperimentalProjectCopyRemoveMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/experimental/*" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatal("/experimental/* is not registered in MountPatterns")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, resp := doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/prj-wired/copy", "")
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted DELETE copy: got %d, want 204. Body: %s", status, resp)
	}

	// The declared sibling POST on this exact path is a different operation
	// served by its own contract row (ROUTE-ADD-070): a bodyless POST fails
	// that operation's required-body validation with the declared 400
	// InvalidRequestError — never a lying 404/405 from the registered route
	// and never the retired typed 501.
	status, header, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-wired/copy", "")
	if status != http.StatusBadRequest {
		t.Fatalf("POST copy: got %d, want the sibling contract's declared 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST copy Content-Type = %q, want application/json", ct)
	}
	assertCopyInvalidRequest(t, "POST copy absent body", status, body, "Body", "")

	// Undeclared methods keep the sibling METHOD_NOT_ALLOWED 405 envelope.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch} {
		status, _, body := doCopyRequest(t, srv.URL, method,
			"/experimental/project/prj-wired/copy", "")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s copy: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" || strings.TrimSpace(got.Error.Message) == "" {
			t.Errorf("%s 405 body = %s, want METHOD_NOT_ALLOWED with message", method, body)
		}
	}

	// Sibling /experimental/* declared operations keep their own routes.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/tool")
	if status != http.StatusOK {
		t.Errorf("sibling GET /experimental/tool: got %d, want 200. Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/tool/ids")
	if status != http.StatusOK {
		t.Errorf("sibling GET /experimental/tool/ids: got %d, want 200. Body: %s", status, body)
	}

	// No catch-all: undeclared /experimental/project* shapes keep the net/http
	// default 404 they are classified with.
	for _, path := range []string{
		"/experimental/project/prj-wired/copy/deeper",
		"/experimental/project/prj-wired/copyx",
		"/experimental/project/prj-wired/copy/generate-name",
	} {
		status, _, _ := doCopyRequest(t, srv.URL, http.MethodDelete, path, "")
		if status != http.StatusNotFound {
			t.Errorf("DELETE %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}
}

// TestExperimentalProjectCopyRemoveServesDeclared401 pins the v2
// UnauthorizedError envelope supplied by the existing authentication wall:
// v2.projectCopy.remove is a v2.* operation, so unauthenticated requests answer
// the typed {_tag: UnauthorizedError, message} body (server.go
// isV2EnvelopeRoutePath) rather than the shim's own UNAUTHENTICATED envelope.
func TestExperimentalProjectCopyRemoveServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/prj-copy-0001/copy", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE copy: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || strings.TrimSpace(toString(got["message"])) == "" {
		t.Errorf("401 body = %s, want the v2 {_tag: UnauthorizedError, message} envelope", body)
	}
}
