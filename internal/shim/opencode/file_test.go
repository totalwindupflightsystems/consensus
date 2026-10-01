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

// fileListFixture builds a scratch directory holding one file and one
// directory, plus the directory that nests it, so a listing has both node
// types to report.
func fileListFixture(t *testing.T) (root, sub, file string) {
	t.Helper()
	root = t.TempDir()
	sub = filepath.Join(root, "alpha")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}
	file = filepath.Join(root, "beta.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	return root, sub, file
}

// decodeFileNodes decodes a GET /file body as an array of FileNode maps,
// failing if the body is not a JSON array (a null array included).
func decodeFileNodes(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var nodes []map[string]any
	if err := json.Unmarshal(body, &nodes); err != nil {
		t.Fatalf("GET /file body must be a JSON array, got %q: %v", body, err)
	}
	if nodes == nil {
		t.Fatalf("GET /file body = %s, want an array (never null)", body)
	}
	return nodes
}

// assertFileNodeShape pins the FileNode contract (all five properties are
// required, type is the declared enum, ignored is a boolean).
func assertFileNodeShape(t *testing.T, node map[string]any, dir string) {
	t.Helper()
	name, _ := node["name"].(string)
	if name == "" {
		t.Errorf("FileNode.name = %v, want a non-empty string", node["name"])
		return
	}
	path, _ := node["path"].(string)
	if want := filepath.Join(dir, name); path != want {
		t.Errorf("FileNode.path = %q, want %q", path, want)
	}
	absolute, _ := node["absolute"].(string)
	if !filepath.IsAbs(absolute) {
		t.Errorf("FileNode.absolute = %q, want an absolute path", absolute)
	} else if want, err := filepath.Abs(filepath.Join(dir, name)); err == nil && absolute != want {
		t.Errorf("FileNode.absolute = %q, want %q", absolute, want)
	}
	kind, _ := node["type"].(string)
	if kind != "file" && kind != "directory" {
		t.Errorf("FileNode.type = %q, want the declared enum file|directory", kind)
	}
	if _, ok := node["ignored"].(bool); !ok {
		t.Errorf("FileNode.ignored = %v (%T), want a boolean", node["ignored"], node["ignored"])
	}
}

// TestFileListAnswersDeclaredList answers upstream file.list (ROUTE-ADD-087,
// SHIM-DRIFT-084, declared responses: 200 FileNode[], 400 BadRequest):
// GET /file?path=<dir> must return 200 with a JSON array of FileNode objects
// — name, path, absolute, type, ignored on every entry — instead of the
// pre-fix net/http default 404.
func TestFileListAnswersDeclaredList(t *testing.T) {
	root, sub, _ := fileListFixture(t)

	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/file?path="+url.QueryEscape(root))
	if status != http.StatusOK {
		t.Fatalf("GET /file: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	nodes := decodeFileNodes(t, body)
	if len(nodes) != 2 {
		t.Fatalf("GET /file?path=%s returned %d nodes, want 2 (alpha/, beta.txt): %s",
			root, len(nodes), body)
	}
	byName := map[string]map[string]any{}
	for _, n := range nodes {
		assertFileNodeShape(t, n, root)
		byName[n["name"].(string)] = n
	}
	if _, ok := byName["beta.txt"]; !ok {
		t.Errorf("listing %s missing the file entry: %s", root, body)
	} else if byName["beta.txt"]["type"] != "file" {
		t.Errorf("beta.txt type = %v, want file", byName["beta.txt"]["type"])
	}
	if _, ok := byName["alpha"]; !ok {
		t.Errorf("listing %s missing the directory entry: %s", root, body)
	} else if byName["alpha"]["type"] != "directory" {
		t.Errorf("alpha type = %v, want directory", byName["alpha"]["type"])
	}
	// The shim keeps no gitignore index, so no entry is ever claimed ignored.
	for _, n := range nodes {
		if n["ignored"] != false {
			t.Errorf("%v ignored = %v, want false (no gitignore index)", n["name"], n["ignored"])
		}
	}
	// The listed sub-directory is itself listable through the same route.
	status, _, subBody := doShimRequest(t, srv.URL, http.MethodGet,
		"/file?path="+url.QueryEscape(sub))
	if status != http.StatusOK {
		t.Fatalf("GET /file?path=%s: got %d, want 200. Body: %s", sub, status, subBody)
	}
	subNodes := decodeFileNodes(t, subBody)
	if len(subNodes) != 0 {
		t.Errorf("empty dir %s returned %s, want []", sub, subBody)
	}
}

// TestFileListEmptyDirectoryIsEmptyArray pins the empty payload: an empty
// directory answers [] — an array, never null (the document declares an
// array, and a null would break generated clients).
func TestFileListEmptyDirectoryIsEmptyArray(t *testing.T) {
	empty := t.TempDir()

	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/file?path="+url.QueryEscape(empty))
	if status != http.StatusOK {
		t.Fatalf("GET /file on an empty dir: got %d, want 200. Body: %s", status, body)
	}
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("GET /file on an empty dir = %s, want []", body)
	}
}

// TestFileListMissingPathIsBadRequest pins the required-parameter arm: ?path=
// is declared required, so a missing or blank value is malformed input and
// must answer the declared 400 via the sibling INVALID_REQUEST envelope —
// not the pre-fix 404, and not a 200 over the process working directory.
func TestFileListMissingPathIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/file", "/file?path=", "/file?path=%20"} {
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
		if !strings.Contains(got.Error.Message, "path") {
			t.Errorf("GET %s: error.message = %q, want the required parameter named", path, got.Error.Message)
		}
	}
}

// TestFileListUnlistablePathIsBadRequest pins the failure arm inside the
// declared response set: file.list declares only 200 and 400, so an absent
// path or a path that is not a directory answers 400 (never an undeclared
// 500, the handleProviderAuth doctrine).
func TestFileListUnlistablePathIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, _, file := fileListFixture(t)

	for _, target := range []string{missing, file} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
			"/file?path="+url.QueryEscape(target))
		if status != http.StatusBadRequest {
			t.Fatalf("GET /file?path=%s: got %d, want 400 (only 200/400 are declared). Body: %s",
				target, status, body)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("GET /file?path=%s: body not the error envelope: %v (%s)", target, err, body)
		}
		if got.Error.Code != "INVALID_REQUEST" {
			t.Errorf("GET /file?path=%s: error.code = %q, want INVALID_REQUEST", target, got.Error.Code)
		}
	}
}

// TestFileListBlankDeclaredParamIsBadRequest pins the sibling handleSkill
// convention for the other declared query params: a present-but-blank
// directory/workspace is malformed input (400), while a valued directory is
// well-formed and scopes a relative ?path=.
func TestFileListBlankDeclaredParamIsBadRequest(t *testing.T) {
	root, sub, _ := fileListFixture(t)

	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	valid := "/file?path=" + url.QueryEscape(root)
	for _, path := range []string{
		valid + "&directory=",
		valid + "&workspace=",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if !strings.Contains(string(body), "INVALID_REQUEST") {
			t.Errorf("GET %s: body = %s, want the INVALID_REQUEST envelope", path, body)
		}
	}

	// A relative path scoped by a valued directory param resolves against it
	// and lists the sub-directory named by the relative path.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/file?directory="+url.QueryEscape(root)+"&path=alpha")
	if status != http.StatusOK {
		t.Fatalf("GET /file?directory=%s&path=alpha: got %d, want 200. Body: %s", root, status, body)
	}
	if nodes := decodeFileNodes(t, body); len(nodes) != 0 {
		t.Errorf("listing alpha (=%s) returned %s, want []", sub, body)
	}
}

// TestFileListMethodGuard pins the sibling method-guard convention: /file is
// a GET-only read endpoint, so every other method answers 405
// METHOD_NOT_ALLOWED (not the pre-fix 404, and not 200).
func TestFileListMethodGuard(t *testing.T) {
	root, _, _ := fileListFixture(t)

	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	path := "/file?path=" + url.QueryEscape(root)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method, path)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /file: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
			t.Errorf("%s /file: body = %s, want the METHOD_NOT_ALLOWED envelope", method, body)
		}
	}
}

// TestFileListNeighboursUntouched is the non-vacuity control: serving the
// bare /file listing must not disturb the sibling file surfaces — the exact
// /file/content route still reads a file, and an unknown /file sub-path (and
// the trailing-slash form of the bare path) stays a plain 404.
func TestFileListNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	_, _, file := fileListFixture(t)
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/file/content?path="+url.QueryEscape(file))
	if status != http.StatusOK {
		t.Errorf("GET /file/content: got %d, want 200 (sibling route untouched). Body: %s", status, body)
	}
	if !strings.Contains(string(body), "hello") {
		t.Errorf("GET /file/content body = %s, want the file contents", body)
	}

	for _, path := range []string{"/file/unknown-sub", "/file/"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 (unregistered sub-path, no /file/* catch-all)", path, status)
		}
	}
}

// TestFileListChiMount is the BUG-009 regression for the new route: /file
// must be listed in MountPatterns so a parent chi router mount reaches the
// shim (a shim-produced 200 proves the wiring; chi's 404 would mean the mount
// lost the bare pattern while keeping the /file/* sub-paths).
func TestFileListChiMount(t *testing.T) {
	root, _, _ := fileListFixture(t)

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/file?path="+url.QueryEscape(root))
	if status != http.StatusOK {
		t.Fatalf("GET /file via chi mount: got %d, want 200 — /file must be registered in MountPatterns. Body: %s",
			status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted GET /file: Content-Type = %q, want application/json", ct)
	}
	if nodes := decodeFileNodes(t, body); len(nodes) != 2 {
		t.Errorf("chi-mounted GET /file returned %d nodes, want 2: %s", len(nodes), body)
	}
}
