package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestInstanceDisposeAnswersDeclaredBoolean answers upstream instance.dispose
// (ROUTE-FIX-003, SHIM-DRIFT-091, declared responses: 200 boolean, 400
// BadRequest): POST /instance/dispose must return 200 with a JSON boolean —
// the shim is a singleton instance rooted at the workspace directory with no
// per-instance registry to release, so the truthful payload is the declared
// boolean true (never a string, never the pre-fix typed 501 envelope). The
// call is idempotent: a repeated POST stays 200/true, like the sibling
// POST /global/dispose (handleGlobalDispose). The operation's optional
// directory/workspace query selectors are workspace-independent input for
// this no-op: valued params are well-formed and answer 200 too.
func TestInstanceDisposeAnswersDeclaredBoolean(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		"/instance/dispose",
		"/instance/dispose?directory=/tmp/some-workspace",
		"/instance/dispose?workspace=/tmp/other-workspace",
		"/instance/dispose?directory=/tmp/a&workspace=/tmp/b",
	} {
		for i := 0; i < 2; i++ {
			status, header, body := doShimRequest(t, srv.URL, http.MethodPost, path)
			if status != http.StatusOK {
				t.Fatalf("POST %s (call %d): got %d, want 200. Body: %s", path, i+1, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("POST %s (call %d): Content-Type = %q, want application/json", path, i+1, ct)
			}
			var disposed bool
			if err := json.Unmarshal(body, &disposed); err != nil {
				t.Fatalf("POST %s (call %d) body must be the declared JSON boolean, got %q: %v", path, i+1, body, err)
			}
			if !disposed {
				t.Errorf("POST %s (call %d): boolean body = false, want true (Instance disposed)", path, i+1)
			}
		}
	}
}

// TestInstanceDisposeBlankQueryParamsAnswer400 pins the declared 400 arm: a
// present-but-blank directory or workspace query parameter is malformed input
// and answers 400 with the declared BadRequestError envelope (the upstream
// v2 SDK NamedError shape writeOpencodeBadRequest emits — name "BadRequest",
// data.kind in the declared enum; a blank query value is kind "Query"),
// naming the offending parameter — the handleSkill/handleFormatter blank
// query convention with this operation's declared body shape. A body is
// never read: the operation declares none.
func TestInstanceDisposeBlankQueryParamsAnswer400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		"/instance/dispose?directory=",
		"/instance/dispose?workspace=",
		"/instance/dispose?workspace=%20%09",
		"/instance/dispose?directory=/tmp/ok&workspace=",
	} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusBadRequest {
			t.Fatalf("POST %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("POST %s: Content-Type = %q, want application/json", path, ct)
		}
		var got struct {
			Name string `json:"name"`
			Data struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("POST %s: body not the declared BadRequestError envelope: %v (%s)", path, err, body)
		}
		if got.Name != "BadRequest" {
			t.Errorf("POST %s: name = %q, want BadRequest", path, got.Name)
		}
		if got.Data.Kind != "Query" {
			t.Errorf("POST %s: data.kind = %q, want Query", path, got.Data.Kind)
		}
		if !strings.Contains(got.Data.Message, "directory") && !strings.Contains(got.Data.Message, "workspace") {
			t.Errorf("POST %s: data.message = %q, want it to name the offending parameter", path, got.Data.Message)
		}
	}
}

// TestInstanceDisposeNonPostKeeps501 pins the neighbour behaviour the fix
// must not disturb: non-POST on /instance/dispose is not a declared method
// (405 is not in the declared response set) and keeps the pre-change answer
// from the handleInstanceSub default arm — the 501
// NOT_IMPLEMENTED envelope naming the sub-path, byte-identical in code and
// status to what the pre-fix tree answered for every method.
func TestInstanceDisposeNonPostKeeps501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, header, body := doShimRequest(t, srv.URL, method, "/instance/dispose")
		if status != http.StatusNotImplemented {
			t.Fatalf("%s /instance/dispose: got %d, want 501 (pre-change answer kept). Body: %s", method, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s /instance/dispose: Content-Type = %q, want application/json", method, ct)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s /instance/dispose: body not the NOT_IMPLEMENTED envelope: %v (%s)", method, err, body)
		}
		if got.Error.Code != "NOT_IMPLEMENTED" {
			t.Errorf("%s /instance/dispose: error.code = %q, want NOT_IMPLEMENTED", method, got.Error.Code)
		}
		if !strings.Contains(got.Error.Message, "dispose") {
			t.Errorf("%s /instance/dispose: error.message = %q, want it to name the sub-path", method, got.Error.Message)
		}
	}
}

// TestInstanceDisposeNeighboursIntact proves the new dispose case did not
// widen into a catch-all: the implemented translation reads stay 200, the
// remaining known stubs stay 501, unknown sub-paths stay 404, and a deeper
// path under /instance/dispose never reaches the dispose handler.
func TestInstanceDisposeNeighboursIntact(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	// Implemented translation reads stay 200.
	for _, probe := range []struct {
		path string
		want int
	}{
		{"/instance", http.StatusOK},
		{"/instance/path", http.StatusOK},
		{"/instance/vcs", http.StatusOK},
		{"/instance/vcs/diff", http.StatusOK},
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, probe.path)
		if status != probe.want {
			t.Errorf("GET %s: got %d, want %d. Body: %s", probe.path, status, probe.want, body)
		}
	}

	// Sibling known stubs keep the typed 501.
	for _, path := range []string{"/instance/command", "/instance/agent", "/instance/formatter", "/instance/vcs/status"} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotImplemented {
			t.Errorf("GET %s: got %d, want 501 (stub unchanged). Body: %s", path, status, body)
		}
	}
	status, _, _ := doShimRequest(t, srv.URL, http.MethodPost, "/instance/vcs/apply")
	if status != http.StatusNotImplemented {
		t.Errorf("POST /instance/vcs/apply: got %d, want 501 (stub unchanged)", status)
	}

	// Unknown sub-paths keep the 404; a deeper path under dispose is NOT the
	// dispose operation and must not be answered by the new handler.
	for _, path := range []string{"/instance/foo", "/instance/dispose/extra"} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 (no /instance/* catch-all). Body: %s", path, status, body)
		}
	}
}

// TestInstanceDisposeChiMount pins the wiring: POST /instance/dispose must
// reach the shim through the existing "/instance" + "/instance/*"
// MountPatterns — a shim-produced 200 proves the wiring, chi's 404 would
// mean the mount lost the route.
func TestInstanceDisposeChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/instance/dispose")
	if status != http.StatusOK {
		t.Fatalf("POST /instance/dispose via chi mount: got %d, want 200 — /instance/dispose must be reachable through the /instance/* MountPattern. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted POST /instance/dispose: Content-Type = %q, want application/json", ct)
	}
	var disposed bool
	if err := json.Unmarshal(body, &disposed); err != nil {
		t.Fatalf("chi-mounted POST /instance/dispose body must be a JSON boolean: %v (%s)", err, body)
	}
	if !disposed {
		t.Errorf("chi-mounted POST /instance/dispose = false, want true")
	}
}
