package opencode

// Tests for the bare /pty mount and its shell discovery child route
// (ROUTE-ADD-102, ROUTE-ADD-103, ROUTE-ADD-104, ROUTE-FIX-010):
//
//	GET  /pty → upstream pty.list (SHIM-DRIFT-105): 200 with the Pty array —
//	     always empty, the shim keeps no pseudo-terminal registry — and the
//	     declared 400 arm for a directory/workspace selector naming a path
//	     that does not exist.
//	POST /pty → upstream pty.create (source item SHIM-DRIFT-106): the
//	     intentional narrowing declared by SPEC-017 §3.9, answered with the
//	     typed not_implemented envelope (SHIM-GAP-002 rule: a declared
//	     operation must never be answered with a bodyless 501 or a lying 404).
//	GET  /pty/shells → upstream pty.shells (source item SHIM-DRIFT-107):
//	     host-backed shell discovery with declared 200 and 400 arms. Its
//	     focused regression battery lives in pty_shells_test.go.
//
// Neighbour guards pin that the remaining /pty/{ptyID} sub-paths stay
// untouched, that non-GET/POST methods answer 405, and that /pty keeps
// Consensus auth (no stub-path exemption, no fixed-workspace exemption).

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

// TestPtyCreateIntentionalNarrowing pins the POST arm (pty.create,
// ROUTE-ADD-103 / source item SHIM-DRIFT-106): the typed not_implemented
// envelope naming the operation — never a fabricated Pty, a bare 501, or a 404
// from the registered route.
func TestPtyCreateIntentionalNarrowing(t *testing.T) {
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

// TestPtyCreateIntentionalNarrowingArtifact ties the runtime contract to the
// generated declared-vs-served artifact. The route must leave drift, but it
// must not enter full upstream coverage: typed 501 is recorded in the explicit
// intentional_narrowing bucket with the spec-owned rationale.
func TestPtyCreateIntentionalNarrowingArtifact(t *testing.T) {
	artifact := filepath.Join("..", "..", "..", "specs", "openapi", "upstream",
		"opencode-declared-vs-served-1.18.33.json")
	raw, err := os.ReadFile(artifact)
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", artifact)
	}
	if err != nil {
		t.Fatalf("read %s: %v", artifact, err)
	}

	type row struct {
		Path          string `json:"path"`
		Method        string `json:"method"`
		OperationID   string `json:"operationId"`
		ServedOutcome string `json:"served_outcome"`
		Rationale     string `json:"rationale"`
	}
	var doc struct {
		Drift                []row `json:"drift_declared_not_served"`
		IntentionalNarrowing []row `json:"intentional_narrowing"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", artifact, err)
	}

	for _, got := range doc.Drift {
		if got.Path == "/pty" && got.Method == http.MethodPost {
			t.Fatalf("POST /pty remains drift instead of explicit intentional narrowing: %+v", got)
		}
	}

	var matches []row
	for _, got := range doc.IntentionalNarrowing {
		if got.Path == "/pty" && got.Method == http.MethodPost {
			matches = append(matches, got)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("intentional_narrowing POST /pty rows = %d, want exactly 1", len(matches))
	}
	got := matches[0]
	if got.OperationID != "pty.create" {
		t.Errorf("operationId = %q, want pty.create", got.OperationID)
	}
	if got.ServedOutcome != "501-typed" {
		t.Errorf("served_outcome = %q, want 501-typed", got.ServedOutcome)
	}
	if strings.TrimSpace(got.Rationale) == "" {
		t.Error("narrowing rationale is empty")
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

// TestPtySubPathsUntouched guards the neighbours: /pty/{ptyID}
// (pty.get/remove/update), connect, and every other unregistered sub-path stay
// NOT-SERVED (net/http default 404, artifact rows SHIM-DRIFT-108..111). The
// separately declared /pty/shells operation is covered by pty_shells_test.go.
func TestPtySubPathsUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
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
// carries no compatibility exemption, so unauthenticated GET and POST requests
// answer 401 UNAUTHENTICATED exactly like the neighbouring api-key routes.
func TestPtyAuthStillRequired(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, method, "/pty")
			if status != http.StatusUnauthorized {
				t.Fatalf("%s /pty unauthenticated: got %d, want 401. Body: %s", method, status, body)
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
		})
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
