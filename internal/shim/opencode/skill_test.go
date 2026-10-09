package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestSkillAnswersDeclaredList answers upstream app.skills (ROUTE-ADD-111,
// SHIM-DRIFT-125, declared responses: 200 Array of Skill, 400 BadRequest):
// GET /skill must return 200 with a JSON array — the shim keeps no skill
// registry, so the truthful payload is an empty array (never null, never an
// error) — instead of the pre-fix net/http default 404.
func TestSkillAnswersDeclaredList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/skill")
	if status != http.StatusOK {
		t.Fatalf("GET /skill: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var skills []map[string]any
	if err := json.Unmarshal(body, &skills); err != nil {
		t.Fatalf("GET /skill body must be a JSON array, got %q: %v", body, err)
	}
	if skills == nil {
		t.Errorf("GET /skill body = %s, want an array (never null)", body)
	}
	if len(skills) != 0 {
		t.Errorf("GET /skill = %v, want empty (the shim keeps no skill registry)", skills)
	}
}

// TestSkillBlankDeclaredParamIsBadRequest pins the declared 400 arm: a
// present-but-blank declared query param (directory, workspace) is malformed
// input and must answer the sibling INVALID_REQUEST envelope, not 200 and
// not the pre-fix 404. A valued param is well-formed and keeps 200.
func TestSkillBlankDeclaredParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/skill?directory=", "/skill?workspace="} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("GET %s: body not the error envelope: %v (%s)", path, err, body)
		}
		if got.Error.Code != "INVALID_REQUEST" {
			t.Errorf("GET %s: error.code = %q, want INVALID_REQUEST", path, got.Error.Code)
		}
		if got.Error.Message == "" {
			t.Errorf("GET %s: error.message empty, want the offending param named", path)
		}
	}

	// A valued param is well-formed: the route answers 200 (the shim has no
	// skill registry to filter, so the value does not change the payload).
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/skill?directory=/tmp/ws")
	if status != http.StatusOK {
		t.Errorf("GET /skill?directory=/tmp/ws: got %d, want 200. Body: %s", status, body)
	}
}

// TestSkillMethodGuard pins the non-GET behaviour: /skill is a registered
// route, so a non-GET request reaches the handler and answers the sibling
// METHOD_NOT_ALLOWED envelope — never the net/http default 404 the pre-fix
// tree answered, and never 404 from the handler itself.
func TestSkillMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/skill")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /skill: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestSkillNeighboursUntouched is the non-vacuity control: serving /skill
// must not disturb the neighbouring surfaces — GET /agent (the sibling app.*
// registry route), the /instance/skill stub (a different, upstream-stubbed
// path), and an unknown /skill sub-path (still a plain 404).
func TestSkillNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/agent")
	if status != http.StatusOK {
		t.Errorf("GET /agent: got %d, want 200 (sibling registry route). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/instance/skill")
	if status != http.StatusNotImplemented {
		t.Errorf("GET /instance/skill: got %d, want 501 (unchanged stub). Body: %s", status, body)
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/skill/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /skill/unknown-sub: got %d, want 404 (unregistered sub-path)", status)
	}
}

// TestSkillChiMount is the BUG-009 regression for the new route: /skill must
// be listed in MountPatterns so a parent chi router mount reaches the shim
// (a shim-produced 200 proves the wiring; chi's 404 would mean the mount
// lost the route).
func TestSkillChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/skill")
	if err != nil {
		t.Fatalf("GET /skill via chi mount: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /skill via chi mount: got %d, want 200 — /skill must be registered in MountPatterns", resp.StatusCode)
	}
	var skills []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&skills); err != nil {
		t.Fatalf("chi-mounted /skill body must be a JSON array: %v", err)
	}
	if skills == nil {
		t.Errorf("chi-mounted /skill body = null, want an array (never null)")
	}
}
