package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestFormatterAnswersDeclaredList answers upstream formatter.status
// (ROUTE-ADD-088, SHIM-DRIFT-086, declared responses: 200 Array of
// FormatterStatus, 400 BadRequest): GET /formatter must return 200 with a JSON
// array — the shim keeps no formatter registry, so the truthful payload is an
// empty array (never null, never an error) — instead of the pre-fix net/http
// default 404.
func TestFormatterAnswersDeclaredList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/formatter")
	if status != http.StatusOK {
		t.Fatalf("GET /formatter: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var formatters []map[string]any
	if err := json.Unmarshal(body, &formatters); err != nil {
		t.Fatalf("GET /formatter body must be a JSON array, got %q: %v", body, err)
	}
	if formatters == nil {
		t.Errorf("GET /formatter body = %s, want an array (never null)", body)
	}
	if len(formatters) != 0 {
		t.Errorf("GET /formatter = %v, want empty (the shim keeps no formatter registry)", formatters)
	}
}

// TestFormatterBlankDeclaredParamIsBadRequest pins the declared 400 arm: a
// present-but-blank declared query param (directory, workspace) is malformed
// input and must answer the sibling INVALID_REQUEST envelope, not 200 and not
// the pre-fix 404. A valued param is well-formed and keeps 200.
func TestFormatterBlankDeclaredParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/formatter?directory=", "/formatter?workspace="} {
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
	// formatter registry to filter, so the value does not change the payload).
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/formatter?directory=/tmp/ws")
	if status != http.StatusOK {
		t.Errorf("GET /formatter?directory=/tmp/ws: got %d, want 200. Body: %s", status, body)
	}
}

// TestFormatterMethodGuard pins the non-GET behaviour: /formatter is a
// registered route, so a non-GET request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestFormatterMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/formatter")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /formatter: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestFormatterNeighboursUntouched is the non-vacuity control: serving the
// bare /formatter must not disturb the neighbouring surfaces — GET /skill (the
// sibling registry route that shares the empty-array shape), the
// /instance/formatter stub (a different, upstream-stubbed path on the
// /instance/* translation surface), and an unknown /formatter sub-path (still
// a plain 404).
func TestFormatterNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/skill")
	if status != http.StatusOK {
		t.Errorf("GET /skill: got %d, want 200 (sibling registry route). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/instance/formatter")
	if status != http.StatusNotImplemented {
		t.Errorf("GET /instance/formatter: got %d, want 501 (unchanged stub). Body: %s", status, body)
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/formatter/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /formatter/unknown-sub: got %d, want 404 (unregistered sub-path)", status)
	}
}

// TestFormatterChiMount is the BUG-009 regression for the new route: /formatter
// must be listed in MountPatterns so a parent chi router mount reaches the shim
// (a shim-produced 200 proves the wiring; chi's 404 would mean the mount lost
// the route).
func TestFormatterChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/formatter")
	if err != nil {
		t.Fatalf("GET /formatter via chi mount: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /formatter via chi mount: got %d, want 200 — /formatter must be registered in MountPatterns", resp.StatusCode)
	}
	var formatters []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&formatters); err != nil {
		t.Fatalf("chi-mounted /formatter body must be a JSON array: %v", err)
	}
	if formatters == nil {
		t.Errorf("chi-mounted /formatter body = null, want an array (never null)")
	}
}
