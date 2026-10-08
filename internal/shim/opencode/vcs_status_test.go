package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestVCSStatusDeclaredContract drives the live HTTP route through the same
// MountPatterns used by the production server and pins both declared outcomes.
func TestVCSStatusDeclaredContract(t *testing.T) {
	repo := makeGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	shim := NewServer(&mockDB{}, "test-key", nil, nil)
	shim.skipAuth = true
	shim.workdir = t.TempDir()
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	request := func(path string) (int, http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build GET %s: %v", path, err)
		}
		if path == "/vcs/status" {
			req.Header.Set("x-opencode-directory", repo)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read GET %s: %v", path, err)
		}
		return resp.StatusCode, resp.Header, body
	}

	t.Run("200 returns declared VcsFileStatus array", func(t *testing.T) {
		status, header, body := request("/vcs/status")
		if status != http.StatusOK {
			t.Fatalf("GET /vcs/status: got %d, want 200. Body: %s", status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got []map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("200 body is not a VcsFileStatus array: %v (%s)", err, body)
		}
		if len(got) < 1 {
			t.Fatalf("status array is empty, want the fixture's changed files")
		}
		found := false
		for _, entry := range got {
			if len(entry) != 4 {
				t.Errorf("VcsFileStatus has %d properties, want exactly the four declared properties: %v", len(entry), entry)
			}
			if _, ok := entry["file"].(string); !ok {
				t.Errorf("VcsFileStatus.file = %v, want string", entry["file"])
			}
			if _, ok := entry["additions"].(float64); !ok {
				t.Errorf("VcsFileStatus.additions = %v, want number", entry["additions"])
			}
			if _, ok := entry["deletions"].(float64); !ok {
				t.Errorf("VcsFileStatus.deletions = %v, want number", entry["deletions"])
			}
			switch entry["status"] {
			case "added", "deleted", "modified":
			default:
				t.Errorf("VcsFileStatus.status = %v, want added, deleted, or modified", entry["status"])
			}
			if entry["file"] == "untracked.txt" {
				found = true
				if entry["status"] != "added" || entry["additions"] != float64(1) || entry["deletions"] != float64(0) {
					t.Errorf("untracked.txt status = %v, want added/1/0", entry)
				}
			}
		}
		if !found {
			t.Errorf("status array omitted untracked.txt: %v", got)
		}
	})

	t.Run("400 rejects a blank declared selector", func(t *testing.T) {
		status, header, body := request("/vcs/status?directory=")
		if status != http.StatusBadRequest {
			t.Fatalf("GET /vcs/status?directory=: got %d, want 400. Body: %s", status, body)
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
			t.Fatalf("400 body is not the shim error envelope: %v (%s)", err, body)
		}
		if got.Error.Code != "INVALID_REQUEST" || !strings.Contains(got.Error.Message, "directory") {
			t.Errorf("400 error = %+v, want INVALID_REQUEST naming directory", got.Error)
		}
	})
}
