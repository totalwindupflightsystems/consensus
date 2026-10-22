package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestConsoleOrgsAnswersDeclaredList answers upstream experimental.console.listOrgs
// (ROUTE-ADD-066, source item SHIM-DRIFT-063 in the committed artifact,
// SHIM-DRIFT-034 in the regenerated drift numbering, declared responses:
// 200 {orgs: ConsoleOrg[]}, 400 BadRequestError, 500 InternalServerError):
// GET /experimental/console/orgs must return 200 with the declared object
// shape — this runtime has no console-account backend at all (it logs in to no
// Console and holds no org registry), so the truthful payload is an empty,
// non-null `orgs` array (never null, never an invented org) — instead of the
// pre-fix net/http default 404.
func TestConsoleOrgsAnswersDeclaredList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/console/orgs")
	if status != http.StatusOK {
		t.Fatalf("GET /experimental/console/orgs: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("GET /experimental/console/orgs body must be a JSON object, got %q: %v", body, err)
	}
	raw, ok := payload["orgs"]
	if !ok {
		t.Fatalf("GET /experimental/console/orgs body = %s, want the declared {orgs: [...]} shape", body)
	}
	if len(payload) != 1 {
		t.Errorf("GET /experimental/console/orgs body = %s, want exactly the declared `orgs` member", body)
	}
	if string(raw) == "null" {
		t.Errorf("GET /experimental/console/orgs orgs = null, want an array (never null)")
	}
	var orgs []map[string]any
	if err := json.Unmarshal(raw, &orgs); err != nil {
		t.Fatalf("GET /experimental/console/orgs orgs must be an array, got %q: %v", raw, err)
	}
	if len(orgs) != 0 {
		t.Errorf("GET /experimental/console/orgs orgs = %v, want empty (the shim holds no console org registry)", orgs)
	}
}

// TestConsoleOrgsBlankDeclaredParamIsBadRequest pins the declared 400 arm: a
// present-but-blank declared query param (directory, workspace) is malformed
// input and must answer the declared BadRequestError envelope (name BadRequest,
// data.kind Query — the writeOpencodeBadRequest shape the sibling /command
// route serves), not 200 and not the pre-fix 404. A valued param is well-formed
// and keeps 200.
func TestConsoleOrgsBlankDeclaredParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		"/experimental/console/orgs?directory=",
		"/experimental/console/orgs?workspace=",
	} {
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
			t.Fatalf("GET %s: body not the BadRequestError envelope: %v (%s)", path, err, body)
		}
		if got.Name != "BadRequest" {
			t.Errorf("GET %s: name = %q, want BadRequest", path, got.Name)
		}
		if got.Data.Kind != "Query" {
			t.Errorf("GET %s: data.kind = %q, want Query", path, got.Data.Kind)
		}
		if got.Data.Message == "" {
			t.Errorf("GET %s: data.message empty, want the offending param named", path)
		}
	}

	// A valued param is well-formed: the route answers 200 (the shim has no
	// console org registry to scope, so the value does not change the payload).
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/console/orgs?directory=/tmp/ws")
	if status != http.StatusOK {
		t.Errorf("GET /experimental/console/orgs?directory=/tmp/ws: got %d, want 200. Body: %s", status, body)
	}
}

// TestConsoleOrgsMethodGuard pins the non-GET behaviour: /experimental/console/orgs
// is a registered route, so a non-GET request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never a 404 from the handler itself.
func TestConsoleOrgsMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/experimental/console/orgs")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /experimental/console/orgs: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestConsoleOrgsNeighboursUntouched is the non-vacuity control: serving the
// exact /experimental/console/orgs path must not disturb the neighbouring
// surfaces — the sibling /experimental/tool route (same family, different
// operation), the declared-but-unserved /experimental/console and
// /experimental/console/switch paths (their own drift rows), and an unknown
// sub-path below the new route (still a plain 404, no catch-all).
func TestConsoleOrgsNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/tool")
	if status != http.StatusOK {
		t.Errorf("GET /experimental/tool: got %d, want 200 (sibling family route). Body: %s", status, body)
	}

	for _, path := range []string{"/experimental/console", "/experimental/console/switch"} {
		status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 (declared sibling, not served by this row)", path, status)
		}
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/console/orgs/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /experimental/console/orgs/unknown-sub: got %d, want 404 (unregistered sub-path)", status)
	}
}

// TestConsoleOrgsChiMount is the BUG-009 regression for the new route:
// /experimental/* is already listed in MountPatterns, so a parent chi router
// mount must reach the shim's exact /experimental/console/orgs handler (a
// shim-produced 200 proves the wiring; chi's 404 would mean the mount lost the
// route).
func TestConsoleOrgsChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/experimental/console/orgs")
	if err != nil {
		t.Fatalf("GET /experimental/console/orgs via chi mount: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /experimental/console/orgs via chi mount: got %d, want 200 — the mount must reach the shim route", resp.StatusCode)
	}
	var payload struct {
		Orgs []map[string]any `json:"orgs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("chi-mounted /experimental/console/orgs body must be the declared object: %v", err)
	}
	if payload.Orgs == nil {
		t.Errorf("chi-mounted /experimental/console/orgs orgs = null, want an array (never null)")
	}
}
