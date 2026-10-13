package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestCommandListAnswersDeclaredList answers upstream command.list
// (ROUTE-ADD-063, source item SHIM-DRIFT-060, declared responses: 200 Command[],
// 400 BadRequest): GET /command must return a JSON array instead of the pre-fix
// net/http default 404. Consensus has no opencode slash-command registry, so
// the truthful payload is an empty array (never null and never fabricated).
func TestCommandListAnswersDeclaredList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/command")
	if status != http.StatusOK {
		t.Fatalf("GET /command: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var commands []map[string]any
	if err := json.Unmarshal(body, &commands); err != nil {
		t.Fatalf("GET /command body must be a JSON array, got %q: %v", body, err)
	}
	if commands == nil {
		t.Errorf("GET /command body = %s, want an array (never null)", body)
	}
	if len(commands) != 0 {
		t.Errorf("GET /command = %v, want empty (the shim keeps no opencode command registry)", commands)
	}
}

// TestCommandListBlankDeclaredParamIsBadRequest pins the declared 400 arm: a
// present-but-blank directory or workspace query param is malformed input and
// must answer the sibling INVALID_REQUEST envelope. Valued params are valid and
// keep the truthful 200 response.
func TestCommandListBlankDeclaredParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/command?directory=", "/command?workspace="} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}
		var got struct {
			Name string `json:"name"`
			Data struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("GET %s: body not the error envelope: %v (%s)", path, err, body)
		}
		if got.Name != "BadRequest" || got.Data.Kind != "Query" {
			t.Errorf("GET %s: error = %+v, want BadRequest with Query kind", path, got)
		}
		// The exact wording is intentionally not pinned; the offending parameter
		// name must remain visible to clients.
		param := strings.Split(strings.Split(path, "?")[1], "=")[0]
		if !strings.Contains(got.Data.Message, param) {
			t.Errorf("GET %s: data.message = %q, want offending parameter %q named", path, got.Data.Message, param)
		}
	}

	for _, path := range []string{
		"/command?directory=/tmp/ws",
		"/command?workspace=ws1",
		"/command?directory=/tmp/ws&workspace=ws1",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200. Body: %s", path, status, body)
		}
	}
}

// TestCommandListMethodGuard pins the established non-GET behavior: a wrong
// method reaches the registered handler and answers 405 METHOD_NOT_ALLOWED.
func TestCommandListMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/command")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /command: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestCommandListNeighboursUntouched is the non-vacuity control: serving the
// bare declared route must not alter the separate /instance/command extension,
// the sibling /skill registry route, or an unknown /command sub-path.
func TestCommandListNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/skill")
	if status != http.StatusOK {
		t.Errorf("GET /skill: got %d, want 200 (sibling registry route). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/instance/command")
	if status != http.StatusNotImplemented {
		t.Errorf("GET /instance/command: got %d, want 501 (unchanged extension stub). Body: %s", status, body)
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/command/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /command/unknown-sub: got %d, want 404 (unregistered sub-path)", status)
	}
}

// TestCommandListChiMount proves the production parent-router wiring: /command
// must be present in MountPatterns or chi answers before the shim sees it.
func TestCommandListChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/command")
	if err != nil {
		t.Fatalf("GET /command via chi mount: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /command via chi mount: got %d, want 200 — /command must be registered in MountPatterns", resp.StatusCode)
	}
	var commands []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&commands); err != nil {
		t.Fatalf("chi-mounted /command body must be a JSON array: %v", err)
	}
	if commands == nil {
		t.Errorf("chi-mounted /command body = null, want an array (never null)")
	}
}
