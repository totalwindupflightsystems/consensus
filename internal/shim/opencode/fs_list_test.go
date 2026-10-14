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

// Upstream contract (openapi-1.18.33.json paths."/api/fs/list".get = v2.fs.list,
// ROUTE-ADD-011, source item SHIM-DRIFT-007 of
// docs/reports/shim-source-002-baseline-diff-2026-09-29.md): GET /api/fs/list,
// an optional deepObject `location` selector, an optional string `path`, and
// responses 200 {location: LocationInfo, data: FileSystemEntry[]} ("List direct
// children of one directory relative to the requested location."), 400
// InvalidRequestError, 401 UnauthorizedError.
//
// LocationInfo requires directory and project {id, directory} with an optional
// workspaceID (pattern ^wrk). FileSystemEntry requires exactly path and type
// (enum file|directory). The shim derives the location from the selected
// workspace (requestWorkspaceDir / instanceID, the handleFSFind convention) and
// answers from a real os.ReadDir of the host filesystem — no entry is invented,
// and an empty directory answers [] rather than null.

type fsListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []map[string]any `json:"data"`
}

// fsListReadFixture builds a small real workspace: three directories (one of
// them empty), two root files and two nested files. Every arm of both
// /api/fs/list and /api/fs/read is exercised against it.
func fsListReadFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"docs", "empty", "src"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for name, body := range map[string]string{
		"alpha.txt":     "alpha",
		"notes.md":      "notes",
		"docs/guide.md": "guide",
		"src/main.go":   "package main",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func decodeFSListResponse(t *testing.T, body []byte) fsListResponse {
	t.Helper()
	var got fsListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/fs/list body is not the declared response object: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Fatalf("GET /api/fs/list data = null, want a FileSystemEntry[] array (declared array): %s", body)
	}
	return got
}

// fsListEntrySet validates every declared FileSystemEntry property and returns
// path -> type. It fails on an undeclared property, an empty path, an absolute
// path (entries are location-relative) and an out-of-enum type.
func fsListEntrySet(t *testing.T, entries []map[string]any) map[string]string {
	t.Helper()
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		for key := range entry {
			if key != "path" && key != "type" {
				t.Errorf("FileSystemEntry carries undeclared property %q: %v", key, entry)
			}
		}
		path, _ := entry["path"].(string)
		kind, _ := entry["type"].(string)
		if path == "" {
			t.Errorf("FileSystemEntry.path is blank: %v", entry)
		}
		if filepath.IsAbs(path) {
			t.Errorf("FileSystemEntry.path = %q, want a location-relative path", path)
		}
		if kind != "file" && kind != "directory" {
			t.Errorf("FileSystemEntry.type = %q, want file|directory (declared enum)", kind)
		}
		if _, dup := out[path]; dup {
			t.Errorf("duplicate FileSystemEntry.path %q", path)
		}
		out[path] = kind
	}
	return out
}

// TestFSListServesDeclared200 pins v2.fs.list (ROUTE-ADD-011, source item
// SHIM-DRIFT-007): the exact /api/fs/list path serves the declared operation —
// the real direct children of the selected workspace — instead of the pre-fix
// net/http default 404 (class NOT-SERVED: no shim route at all).
func TestFSListServesDeclared200(t *testing.T) {
	root := fsListReadFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/list")
	if status != http.StatusOK {
		t.Fatalf("GET /api/fs/list: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	got := decodeFSListResponse(t, body)
	if got.Location.Directory != root || got.Location.Project.Directory != root {
		t.Errorf("location = %+v, want directory and project.directory %q", got.Location, root)
	}
	if got.Location.Project.ID != instanceID(root) {
		t.Errorf("location.project.id = %q, want %q", got.Location.Project.ID, instanceID(root))
	}
	if got.Location.WorkspaceID != "" {
		t.Errorf("location.workspaceID = %q, want omitted for the singleton workspace", got.Location.WorkspaceID)
	}

	entries := fsListEntrySet(t, got.Data)
	want := map[string]string{
		"alpha.txt": "file",
		"notes.md":  "file",
		"docs/":     "directory",
		"empty/":    "directory",
		"src/":      "directory",
	}
	if len(entries) != len(want) {
		t.Errorf("data has %d entries, want exactly the %d direct children: %v", len(entries), len(want), entries)
	}
	for path, kind := range want {
		if entries[path] != kind {
			t.Errorf("entry %q = %q, want %q; all entries: %v", path, entries[path], kind, entries)
		}
	}
	// Direct children only: a listing must not recurse.
	for _, nested := range []string{"docs/guide.md", "src/main.go"} {
		if _, ok := entries[nested]; ok {
			t.Errorf("nested entry %q was returned by a direct-children listing: %v", nested, entries)
		}
	}
}

// TestFSListPathSelector pins the declared optional `path` query: it names one
// directory relative to the requested location, and the entries it returns stay
// location-relative (so a list result feeds fs.read directly).
func TestFSListPathSelector(t *testing.T) {
	root := fsListReadFixture(t)
	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	cases := []struct {
		name string
		path string
		want map[string]string
	}{
		{
			name: "subdirectory",
			path: "/api/fs/list?path=docs",
			want: map[string]string{"docs/guide.md": "file"},
		},
		{
			name: "subdirectory with trailing slash",
			path: "/api/fs/list?path=docs/",
			want: map[string]string{"docs/guide.md": "file"},
		},
		{
			name: "empty subdirectory",
			path: "/api/fs/list?path=empty",
			want: map[string]string{},
		},
		{
			name: "location itself",
			path: "/api/fs/list?path=.",
			want: map[string]string{
				"alpha.txt": "file", "notes.md": "file",
				"docs/": "directory", "empty/": "directory", "src/": "directory",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", tc.path, status, body)
			}
			got := decodeFSListResponse(t, body)
			if got.Location.Directory != root {
				t.Errorf("location.directory = %q, want the selected workspace %q", got.Location.Directory, root)
			}
			entries := fsListEntrySet(t, got.Data)
			if len(entries) != len(tc.want) {
				t.Fatalf("GET %s: data = %v, want %v", tc.path, entries, tc.want)
			}
			for path, kind := range tc.want {
				if entries[path] != kind {
					t.Errorf("GET %s: entry %q = %q, want %q (all: %v)", tc.path, path, entries[path], kind, entries)
				}
			}
		})
	}
}

// TestFSListDeepObjectLocation pins the v2 deepObject location query spelling:
// location[directory] selects the listed workspace and location[workspace] is
// echoed only when the caller names one (handleFSFind convention).
func TestFSListDeepObjectLocation(t *testing.T) {
	root := fsListReadFixture(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "other.txt"), []byte("other"), 0o644); err != nil {
		t.Fatalf("write other.txt: %v", err)
	}

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	path := "/api/fs/list?location%5Bdirectory%5D=" + url.QueryEscape(other) +
		"&location%5Bworkspace%5D=wrk-test"
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", path, status, body)
	}
	got := decodeFSListResponse(t, body)
	if got.Location.Directory != other || got.Location.Project.Directory != other {
		t.Errorf("location = %+v, want the requested directory %q", got.Location, other)
	}
	if got.Location.WorkspaceID != "wrk-test" {
		t.Errorf("location.workspaceID = %q, want the echoed wrk-test", got.Location.WorkspaceID)
	}
	entries := fsListEntrySet(t, got.Data)
	if len(entries) != 1 || entries["other.txt"] != "file" {
		t.Errorf("data = %v, want exactly other.txt of the selected location", entries)
	}
}

// TestFSListServesDeclared400 verifies every malformed arm answers the declared
// InvalidRequestError, including the traversal arms: a `path` that escapes the
// workspace root is refused, never listed.
func TestFSListServesDeclared400(t *testing.T) {
	root := fsListReadFixture(t)
	file := filepath.Join(root, "alpha.txt")
	missing := filepath.Join(root, "missing")
	outside := t.TempDir()

	s, srv := newTestServer(&mockDB{})
	s.workdir = root
	defer srv.Close()

	cases := []struct {
		name string
		path string
	}{
		{"blank location[directory]", "/api/fs/list?location%5Bdirectory%5D="},
		{"blank location[workspace]", "/api/fs/list?location%5Bworkspace%5D="},
		{"unknown location field", "/api/fs/list?location%5Bbogus%5D=x"},
		{"missing location directory", "/api/fs/list?location%5Bdirectory%5D=" + url.QueryEscape(missing)},
		{"location is a file", "/api/fs/list?location%5Bdirectory%5D=" + url.QueryEscape(file)},
		{"non-wrk workspace id", "/api/fs/list?location%5Bworkspace%5D=workspace-1"},
		{"blank path", "/api/fs/list?path=%20"},
		{"absolute path", "/api/fs/list?path=" + url.QueryEscape(filepath.Join(outside, "escape.txt"))},
		{"parent reference", "/api/fs/list?path=.."},
		{"nested parent reference", "/api/fs/list?path=" + url.QueryEscape("docs/../..")},
		{"parent reference to a sibling", "/api/fs/list?path=" + url.QueryEscape("../"+filepath.Base(outside))},
		{"path is a file", "/api/fs/list?path=alpha.txt"},
		{"missing path", "/api/fs/list?path=missing"},
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

// TestFSListMethodAndMountWiring pins the method guard and the chi mount:
// /api/fs/list must be listed in MountPatterns (cmd/consensus/main.go mounts
// the shim by these patterns — without the entry a parent chi router answers
// before the shim sees the request), non-GET keeps the sibling
// METHOD_NOT_ALLOWED envelope, and the chi mount reaches both routes of this
// task (the exact list path and the read subtree).
func TestFSListMethodAndMountWiring(t *testing.T) {
	root := fsListReadFixture(t)
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = root

	for _, pattern := range []string{"/api/fs/list", "/api/fs/read/*"} {
		mounted := false
		for _, candidate := range MountPatterns {
			if candidate == pattern {
				mounted = true
			}
		}
		if !mounted {
			t.Fatalf("%s is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-011)", pattern)
		}
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		for _, path := range []string{"/api/fs/list", "/api/fs/read/alpha.txt"} {
			req := httptest.NewRequest(method, path, nil)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: got %d, want 405. Body: %s", method, path, rec.Code, rec.Body)
			}
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/list")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/fs/list: got %d, want 200. Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/fs/read/alpha.txt")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/fs/read/alpha.txt: got %d, want 200. Body: %s", status, body)
	}
	if string(body) != "alpha" {
		t.Errorf("chi-mounted GET /api/fs/read/alpha.txt body = %q, want %q", body, "alpha")
	}
}

// TestFSListServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation, and that the
// traversal guard sits BEHIND the auth wall — an unauthenticated traversal
// request is answered 401, not 400 (the auth arm wins, as it does for every
// other arm of the surface).
func TestFSListServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/api/fs/list", "/api/fs/read/alpha.txt", "/api/fs/read/../outside.txt"} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated GET %s: got %d, want 401. Body: %s", path, status, body)
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
}
