package opencode

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

// Upstream contract (openapi-1.18.33.json paths."/api/fs/read/*".get =
// v2.fs.read, ROUTE-ADD-011, source item SHIM-DRIFT-008 of
// docs/reports/shim-source-002-baseline-diff-2026-09-29.md): GET
// /api/fs/read/*, an optional deepObject `location` selector, and responses
// 200 application/octet-stream (binary string, "Serve one file relative to the
// requested location."), 400 InvalidRequestError, 401 UnauthorizedError.
//
// The file is addressed by the URL suffix after /api/fs/read/ and must stay
// inside the requested location: traversal out of the workspace — lexical
// (`..`), absolute, or through a symlink — is answered with the declared 400 and
// the content is never served. The operation declares no 404, so an absent file
// and a directory are refused through the same declared 400 arm.
//
// The negative-control corpus below (outside-secret.txt beside the workspace
// and a symlink pointing at it) exists so the traversal arms would return the
// secret if the guard were removed — the 400 assertions are not vacuous.

const fsReadSecret = "OUTSIDE-SECRET"

func assertDeclaredInvalidRequest(t *testing.T, label, path string, status int, header http.Header, body []byte) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s GET %s: got %d, want the declared 400. Body: %s", label, path, status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("%s Content-Type = %q, want application/json", label, ct)
	}
	if strings.Contains(string(body), fsReadSecret) {
		t.Fatalf("%s GET %s leaked outside-workspace content: %s", label, path, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got["_tag"] != "InvalidRequestError" || got["message"] == "" {
		t.Errorf("%s 400 body = %v, want InvalidRequestError with message", label, got)
	}
	for key := range got {
		if key != "_tag" && key != "message" && key != "kind" && key != "field" {
			t.Errorf("%s 400 body has undeclared property %q: %v", label, key, got)
		}
	}
}

// TestFSReadServesDeclared200 pins v2.fs.read (ROUTE-ADD-011, source item
// SHIM-DRIFT-008): the /api/fs/read/* subtree serves the declared operation —
// the file's real bytes as the declared application/octet-stream binary — for a
// path relative to the selected workspace. Before the fix the whole subtree had
// no route and net/http answered the default 404.
func TestFSReadServesDeclared200(t *testing.T) {
	root := fsListReadFixture(t)
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0o644); err != nil {
		t.Fatalf("write empty.txt: %v", err)
	}
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	cases := []struct {
		name     string
		path     string
		wantBody string
	}{
		{"root file", "/api/fs/read/alpha.txt", "alpha"},
		{"nested file", "/api/fs/read/docs/guide.md", "guide"},
		{"empty file", "/api/fs/read/empty.txt", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", tc.path, status, body)
			}
			if ct := header.Get("Content-Type"); ct != "application/octet-stream" {
				t.Errorf("Content-Type = %q, want the declared application/octet-stream", ct)
			}
			if string(body) != tc.wantBody {
				t.Errorf("GET %s body = %q, want %q", tc.path, body, tc.wantBody)
			}
		})
	}
}

// TestFSReadServesBinaryBytes pins the declared media type: the 200 body is the
// file's bytes, not a JSON envelope and not a re-encoded string. A byte range
// that is not valid UTF-8 and contains 0x00 must round-trip exactly.
func TestFSReadServesBinaryBytes(t *testing.T) {
	root := fsListReadFixture(t)
	blob := make([]byte, 256)
	for i := range blob {
		blob[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), blob, 0o644); err != nil {
		t.Fatalf("write blob.bin: %v", err)
	}

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/blob.bin")
	if status != http.StatusOK {
		t.Fatalf("GET /api/fs/read/blob.bin: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want the declared application/octet-stream", ct)
	}
	if len(body) != len(blob) {
		t.Fatalf("body length %d, want %d (a JSON or UTF-8 round-trip would change it)", len(body), len(blob))
	}
	for i := range blob {
		if body[i] != blob[i] {
			t.Fatalf("body[%d] = 0x%02x, want 0x%02x — the binary content was re-encoded", i, body[i], blob[i])
		}
	}
}

// TestFSReadLocationSelector pins the declared deepObject location selector on
// the read path: location[directory] selects the workspace the relative file
// path is resolved against.
func TestFSReadLocationSelector(t *testing.T) {
	root := fsListReadFixture(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "other.txt"), []byte("other"), 0o644); err != nil {
		t.Fatalf("write other.txt: %v", err)
	}

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	path := "/api/fs/read/other.txt?location%5Bdirectory%5D=" + url.QueryEscape(other)
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", path, status, body)
	}
	if string(body) != "other" {
		t.Errorf("GET %s body = %q, want %q", path, body, "other")
	}

	// The same relative name must NOT resolve against the configured workdir
	// once the selector names another location.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/other.txt")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /api/fs/read/other.txt (workdir scope): got %d, want 400 (the file is not in the selected workspace). Body: %s", status, body)
	}

	// A malformed selector is refused before the file is touched.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/other.txt?location%5Bbogus%5D=x")
	if status != http.StatusBadRequest {
		t.Fatalf("GET with an unknown location field: got %d, want 400. Body: %s", status, body)
	}
}

// TestFSReadRejectsTraversal is the core contract of this surface: a path that
// escapes the requested location is answered with the declared 400 and its
// content is never served. The negative control (outside-secret.txt beside the
// workspace, reachable by each of these spellings) proves the arm is real.
func TestFSReadRejectsTraversal(t *testing.T) {
	root := fsListReadFixture(t)

	// Negative control 1: a file reachable by walking up out of the workspace.
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "outside-secret.txt"), []byte(fsReadSecret), 0o644); err != nil {
		t.Fatalf("write outside-secret.txt: %v", err)
	}
	// Negative control 2: a symlink inside the workspace pointing outside it.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte(fsReadSecret), 0o644); err != nil {
		t.Fatalf("write outside/outside.txt: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(root, "escape-link.txt")); err != nil {
		t.Fatalf("symlink escape-link.txt: %v", err)
	}

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	cases := []struct {
		name string
		path string
	}{
		{"plain parent reference", "/api/fs/read/../outside-secret.txt"},
		{"percent-encoded parent reference", "/api/fs/read/%2e%2e/outside-secret.txt"},
		{"encoded-slash parent reference", "/api/fs/read/..%2Foutside-secret.txt"},
		{"nested parent reference", "/api/fs/read/docs/..%2F..%2Foutside-secret.txt"},
		{"trailing parent reference", "/api/fs/read/src/..%2F..%2Foutside-secret.txt"},
		{"percent-encoded absolute path", "/api/fs/read/%2Fetc%2Fhostname"},
		{"symlink escaping the workspace", "/api/fs/read/escape-link.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			assertDeclaredInvalidRequest(t, tc.name, tc.path, status, header, body)
		})
	}
}

// TestFSReadRefusesMalformedAndUnreadable covers the remaining declared 400
// arms: an empty file suffix, an absent file and a directory — the operation
// declares no 404, so these are refused through the declared error arm rather
// than an undeclared status (the handleFileList doctrine).
func TestFSReadRefusesMalformedAndUnreadable(t *testing.T) {
	root := fsListReadFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	for _, tc := range []struct {
		name string
		path string
	}{
		{"empty file suffix", "/api/fs/read/"},
		{"absent file", "/api/fs/read/does-not-exist.txt"},
		{"absent nested file", "/api/fs/read/docs/missing.md"},
		{"directory", "/api/fs/read/docs"},
		{"directory with trailing slash", "/api/fs/read/docs/"},
		{"NUL byte in the path", "/api/fs/read/bad%00name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			assertDeclaredInvalidRequest(t, tc.name, tc.path, status, header, body)
		})
	}
}

// TestFSReadServesSymlinkInsideTheWorkspace is the positive control for the
// symlink guard: a link whose real target is inside the workspace is served
// normally, so the guard refuses escapes rather than every symlink.
func TestFSReadServesSymlinkInsideTheWorkspace(t *testing.T) {
	root := fsListReadFixture(t)
	if err := os.Symlink(filepath.Join(root, "alpha.txt"), filepath.Join(root, "inside-link.txt")); err != nil {
		t.Fatalf("symlink inside-link.txt: %v", err)
	}

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/inside-link.txt")
	if status != http.StatusOK {
		t.Fatalf("GET /api/fs/read/inside-link.txt: got %d, want 200 (the real target is inside the workspace). Body: %s", status, body)
	}
	if string(body) != "alpha" {
		t.Errorf("body = %q, want %q", body, "alpha")
	}
}

// TestFSReadMethodMountAndAuthWiring pins the method guard, the chi mount and
// the auth arm for the read subtree.
func TestFSReadMethodMountAndAuthWiring(t *testing.T) {
	root := fsListReadFixture(t)
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = root

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/fs/read/alpha.txt", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/fs/read/alpha.txt: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/docs/guide.md")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/fs/read/docs/guide.md: got %d, want 200. Body: %s", status, body)
	}
	if string(body) != "guide" {
		t.Errorf("chi-mounted nested read body = %q, want %q", body, "guide")
	}
	// The traversal guard is reachable through the production mount too.
	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/..%2Foutside-secret.txt")
	assertDeclaredInvalidRequest(t, "chi-mounted traversal", "/api/fs/read/..%2Foutside-secret.txt", status, header, body)
}
