package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type fsFindResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"data"`
}

func fsFindFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"alpha-dir", "docs", "src"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for name, body := range map[string]string{
		"alpha-dir/nested.txt": "nested",
		"docs/alpha.md":        "alpha docs",
		"src/alpha.go":         "package alpha",
		"src/beta.go":          "package beta",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func decodeFSFindResponse(t *testing.T, body []byte) fsFindResponse {
	t.Helper()
	var got fsFindResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/fs/find body is not the declared response object: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Fatalf("GET /api/fs/find data = null, want an array")
	}
	return got
}

// TestFSFindServesDeclared200 pins v2.fs.find (ROUTE-ADD-010,
// SHIM-DRIFT-006): the route recursively searches the selected workspace and
// returns the declared LocationInfo + FileSystemEntry[] shape.
func TestFSFindServesDeclared200(t *testing.T) {
	root := fsFindFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/find?query=alpha")
	if status != http.StatusOK {
		t.Fatalf("GET /api/fs/find?query=alpha: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	got := decodeFSFindResponse(t, body)
	if got.Location.Directory != root || got.Location.Project.Directory != root {
		t.Errorf("location = %+v, want directory and project.directory %q", got.Location, root)
	}
	if got.Location.Project.ID != instanceID(root) {
		t.Errorf("location.project.id = %q, want %q", got.Location.Project.ID, instanceID(root))
	}
	if got.Location.WorkspaceID != "" {
		t.Errorf("location.workspaceID = %q, want omitted for the singleton workspace", got.Location.WorkspaceID)
	}

	entries := map[string]string{}
	for _, entry := range got.Data {
		if entry.Path == "" {
			t.Errorf("FileSystemEntry.path is blank: %+v", entry)
		}
		if entry.Type != "file" && entry.Type != "directory" {
			t.Errorf("FileSystemEntry.type = %q, want file|directory", entry.Type)
		}
		if filepath.IsAbs(entry.Path) {
			t.Errorf("FileSystemEntry.path = %q, want a workspace-relative path", entry.Path)
		}
		entries[filepath.ToSlash(entry.Path)] = entry.Type
	}
	for path, kind := range map[string]string{
		"alpha-dir/":    "directory",
		"docs/alpha.md": "file",
		"src/alpha.go":  "file",
	} {
		if entries[path] != kind {
			t.Errorf("entry %q = %q, want %q; all entries: %+v", path, entries[path], kind, entries)
		}
	}
	if _, ok := entries["src/beta.go"]; ok {
		t.Errorf("non-matching src/beta.go was returned: %+v", entries)
	}
}

// TestFSFindFiltersAndLimits pins the declared type enum and positive limit.
func TestFSFindFiltersAndLimits(t *testing.T) {
	root := fsFindFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	for _, tc := range []struct {
		name     string
		path     string
		wantType string
		wantLen  int
	}{
		{"files", "/api/fs/find?query=alpha&type=file", "file", 3},
		{"directories", "/api/fs/find?query=alpha&type=directory", "directory", 1},
		{"limit", "/api/fs/find?query=alpha&type=file&limit=1", "file", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", tc.path, status, body)
			}
			got := decodeFSFindResponse(t, body)
			if len(got.Data) != tc.wantLen {
				t.Fatalf("GET %s returned %d entries, want %d: %s", tc.path, len(got.Data), tc.wantLen, body)
			}
			for _, entry := range got.Data {
				if entry.Type != tc.wantType {
					t.Errorf("GET %s entry %+v, want type %q", tc.path, entry, tc.wantType)
				}
			}
		})
	}
}

// TestFSFindLocationSelector pins the deepObject location query used by the
// generated v2 SDK: location[directory] selects the recursive search root.
func TestFSFindLocationSelector(t *testing.T) {
	root := fsFindFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	path := "/api/fs/find?query=beta&location%5Bdirectory%5D=" + url.QueryEscape(root) +
		"&location%5Bworkspace%5D=wrk-test"
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", path, status, body)
	}
	got := decodeFSFindResponse(t, body)
	if got.Location.Directory != root || got.Location.WorkspaceID != "wrk-test" {
		t.Errorf("location = %+v, want directory=%q workspaceID=wrk-test", got.Location, root)
	}
	if len(got.Data) != 1 || filepath.ToSlash(got.Data[0].Path) != "src/beta.go" {
		t.Errorf("data = %+v, want only src/beta.go", got.Data)
	}
}

// TestFSFindServesDeclared400 verifies every malformed declared query arm uses
// the v2 InvalidRequestError shape, rather than the generic shim envelope.
func TestFSFindServesDeclared400(t *testing.T) {
	root := fsFindFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	file := filepath.Join(root, "src", "alpha.go")
	missing := filepath.Join(root, "missing")
	cases := []struct {
		name string
		path string
	}{
		{"missing query", "/api/fs/find"},
		{"blank query", "/api/fs/find?query=%20"},
		{"invalid type", "/api/fs/find?query=a&type=socket"},
		{"zero limit", "/api/fs/find?query=a&limit=0"},
		{"non-numeric limit", "/api/fs/find?query=a&limit=many"},
		{"blank directory", "/api/fs/find?query=a&location%5Bdirectory%5D="},
		{"blank workspace", "/api/fs/find?query=a&location%5Bworkspace%5D="},
		{"missing directory", "/api/fs/find?query=a&location%5Bdirectory%5D=" + url.QueryEscape(missing)},
		{"non-directory", "/api/fs/find?query=a&location%5Bdirectory%5D=" + url.QueryEscape(file)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400. Body: %s", tc.path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" || got["message"] == "" {
				t.Errorf("400 body = %v, want InvalidRequestError with message", got)
			}
			for key := range got {
				if key != "_tag" && key != "message" && key != "kind" && key != "field" {
					t.Errorf("400 body has undeclared property %q: %v", key, got)
				}
			}
		})
	}
}

// TestFSFindServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation.
func TestFSFindServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/find?query=alpha")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/fs/find: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want UnauthorizedError with message", got)
	}
	if len(got) != 2 {
		t.Errorf("401 body = %v, want only declared _tag and message properties", got)
	}
}

func TestFSFindOrderingIsStable(t *testing.T) {
	root := fsFindFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/find?query=alpha")
	if status != http.StatusOK {
		t.Fatalf("GET /api/fs/find: got %d, want 200. Body: %s", status, body)
	}
	got := decodeFSFindResponse(t, body)
	paths := make([]string, 0, len(got.Data))
	for _, entry := range got.Data {
		paths = append(paths, entry.Path)
	}
	want := append([]string(nil), paths...)
	sort.SliceStable(want, func(i, j int) bool {
		bi := strings.TrimSuffix(filepath.Base(filepath.Clean(want[i])), string(filepath.Separator))
		bj := strings.TrimSuffix(filepath.Base(filepath.Clean(want[j])), string(filepath.Separator))
		if len(bi) != len(bj) {
			return len(bi) < len(bj)
		}
		return want[i] < want[j]
	})
	// All fixture matches are prefix/whole-basename matches. Within that rank,
	// shorter basenames then lexical path make repeated responses deterministic.
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Errorf("paths are not deterministically ranked:\n got %v\nwant %v", paths, want)
	}
}

func TestFSFindMethodAndMountWiring(t *testing.T) {
	root := fsFindFixture(t)
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = root

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/fs/find?query=alpha", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/fs/find: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/find?query=beta")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/fs/find?query=beta: got %d, want 200. Body: %s", status, body)
	}
	// ROUTE-ADD-011 claimed the sibling v2 filesystem routes, so /api/fs/list
	// is served through the production mount instead of the 404 it answered
	// while it was unclaimed.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/list")
	if status != http.StatusOK {
		t.Errorf("GET /api/fs/list: got %d, want 200 (ROUTE-ADD-011 serves the sibling). Body: %s", status, body)
	}
}
