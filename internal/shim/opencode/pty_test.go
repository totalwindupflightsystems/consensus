package opencode

// Tests for the bare /pty mount (ROUTE-ADD-102, ROUTE-FIX-010):
//
//	GET  /pty → upstream pty.list (SHIM-DRIFT-105): 200 with the Pty array —
//	     always empty, the shim keeps no pseudo-terminal registry — and the
//	     declared 400 arm for a directory/workspace selector naming a path
//	     that does not exist.
//	POST /pty → upstream pty.create (SHIM-DRIFT-106): the typed
//	     not_implemented envelope (SHIM-GAP-002 rule: a declared operation
//	     must never be answered with a bodyless 501 or a lying 404).
//
// Neighbour guards pin that the mount does not claim the /pty/* sub-paths
// (pty.shells, pty.get/remove/update stay NOT-SERVED), that non-GET/POST
// methods answer 405, and that /pty keeps Consensus auth (no stub-path
// exemption, no fixed-workspace exemption).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestPtyList pins the declared 200 arm of GET /pty (pty.list): an
// application/json array, empty — the shim keeps no pty registry, and the
// upstream contract declares an array, never null.
func TestPtyList(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /pty: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got []map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be a Pty array, got %q: %v", body, err)
	}
	if len(got) != 0 {
		t.Errorf("pty list = %v, want [] (the shim keeps no pseudo-terminal registry)", got)
	}
}

// TestPtyListSelector pins the declared 400 arm: a directory/workspace
// selector naming a path that does not exist (or is not a directory) answers
// the sibling writeOpencodeError INVALID_REQUEST envelope; a selector naming
// a real directory answers 200 with the empty list.
func TestPtyListSelector(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	existing := t.TempDir()
	existingFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, query string
		wantStatus  int
	}{
		{"existing directory", "directory=" + url.QueryEscape(existing), http.StatusOK},
		{"existing workspace", "workspace=" + url.QueryEscape(existing), http.StatusOK},
		{"missing directory", "directory=" + url.QueryEscape(filepath.Join(existing, "missing")), http.StatusBadRequest},
		{"missing workspace", "workspace=/nonexistent/route-add-102", http.StatusBadRequest},
		{"selector names a file", "directory=" + url.QueryEscape(existingFile), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty?"+tc.query)
			if status != tc.wantStatus {
				t.Fatalf("GET /pty?%s: got %d, want %d. Body: %s", tc.query, status, tc.wantStatus, body)
			}
			if tc.wantStatus != http.StatusBadRequest {
				return
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error.code = %q, want INVALID_REQUEST", got.Error.Code)
			}
			if strings.TrimSpace(got.Error.Message) == "" {
				t.Error("error.message empty, want a reason")
			}
		})
	}
}

// TestPtyCreateNotImplemented pins the POST arm (pty.create, SHIM-DRIFT-106):
// the typed not_implemented envelope naming the operation — never a bare 501
// and never a 404 from the newly registered route.
func TestPtyCreateNotImplemented(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/pty")
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /pty: got %d, want 501. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("501 body is not JSON: %v (%s)", err, body)
	}
	if len(got) != 3 {
		t.Errorf("envelope must carry exactly 3 fields (error, operation, detail), got %d: %v", len(got), got)
	}
	if got["error"] != "not_implemented" {
		t.Errorf("error = %v, want \"not_implemented\"", got["error"])
	}
	if got["operation"] != "pty.create" {
		t.Errorf("operation = %v, want \"pty.create\"", got["operation"])
	}
	if detail, _ := got["detail"].(string); strings.TrimSpace(detail) == "" {
		t.Error("detail must name what is missing; got empty")
	}
}

// TestPtyMethodNotAllowed pins that methods outside the declared GET/POST
// pair answer the sibling METHOD_NOT_ALLOWED envelope.
func TestPtyMethodNotAllowed(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/pty")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /pty: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestPtySubPathsUntouched guards the neighbours: only the bare /pty mount is
// registered — /pty/shells (pty.shells), /pty/{ptyID} (pty.get/remove/update)
// and every other sub-path stay NOT-SERVED (net/http default 404, artifact
// rows SHIM-DRIFT-107..110 untouched).
func TestPtySubPathsUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/pty/shells"},
		{http.MethodGet, "/pty/pty_1"},
		{http.MethodDelete, "/pty/pty_1"},
		{http.MethodGet, "/pty/pty_1/connect"},
	} {
		status, _, body := doShimRequest(t, srv.URL, tc.method, tc.path)
		if status != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404 (sub-path must stay NOT-SERVED). Body: %s", tc.method, tc.path, status, body)
		}
	}
}

// TestPtyAuthStillRequired pins the auth policy: /pty is not a stub path and
// carries no compatibility exemption, so an unauthenticated request answers
// 401 UNAUTHENTICATED exactly like the neighbouring api-key routes.
func TestPtyAuthStillRequired(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /pty unauthenticated: got %d, want 401. Body: %s", status, body)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", got.Error.Code)
	}
}

// TestPtyChiMount drives /pty through a parent chi router mounted with
// MountPatterns (the production wiring) so a missing or misregistered
// pattern 404s exactly as it would in production (BUG-009 class).
func TestPtyChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /pty via chi mount: got %d, want 200. Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/pty")
	if status != http.StatusNotImplemented {
		t.Errorf("POST /pty via chi mount: got %d, want 501. Body: %s", status, body)
	}
}
