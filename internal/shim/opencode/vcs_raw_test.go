package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestVCSDiffRawDeclaredContract drives GET /vcs/diff/raw through the same
// chi MountPatterns used by production and covers the declared text response,
// raw tracked/untracked patch, selector validation, and method guard.
func TestVCSDiffRawDeclaredContract(t *testing.T) {
	repo := makeGitRepo(t)
	shim := NewServer(&mockDB{}, "test-key", nil, nil)
	shim.skipAuth = true
	shim.workdir = t.TempDir()
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	request := func(method, path string, headerDir string) (int, http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build %s %s: %v", method, path, err)
		}
		if headerDir != "" {
			req.Header.Set("x-opencode-directory", headerDir)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read %s %s: %v", method, path, err)
		}
		return resp.StatusCode, resp.Header, body
	}

	t.Run("200 returns raw tracked and untracked patch", func(t *testing.T) {
		status, header, body := request(http.MethodGet, "/vcs/diff/raw?directory="+repo, "")
		if status != http.StatusOK {
			t.Fatalf("GET /vcs/diff/raw: got %d, want 200. Body: %s", status, body)
		}
		if got := header.Get("Content-Type"); got != "text/x-diff; charset=utf-8" {
			t.Fatalf("Content-Type = %q, want text/x-diff; charset=utf-8", got)
		}
		patch := string(body)
		for _, want := range []string{"diff --git", "dirty.txt", "untracked.txt", "+change", "+one"} {
			if !strings.Contains(patch, want) {
				t.Errorf("raw patch does not contain %q: %s", want, patch)
			}
		}
	})

	t.Run("200 clean workspace is empty text diff", func(t *testing.T) {
		clean := makeGitRepo(t)
		resetCmd := exec.Command("git", "-C", clean, "reset", "--hard", "HEAD")
		resetCmd.Env = gitEnv()
		if output, err := resetCmd.CombinedOutput(); err != nil {
			t.Fatalf("git reset: %v\n%s", err, output)
		}
		if err := os.Remove(filepath.Join(clean, "untracked.txt")); err != nil {
			t.Fatal(err)
		}
		status, header, body := request(http.MethodGet, "/vcs/diff/raw?workspace="+clean, "")
		if status != http.StatusOK || len(body) != 0 {
			t.Fatalf("clean GET /vcs/diff/raw: got status=%d body=%q, want 200 with empty body", status, body)
		}
		if got := header.Get("Content-Type"); got != "text/x-diff; charset=utf-8" {
			t.Errorf("clean Content-Type = %q, want text/x-diff; charset=utf-8", got)
		}
	})

	t.Run("400 rejects blank and invalid selectors without path disclosure", func(t *testing.T) {
		invalidPath := filepath.Join(t.TempDir(), "missing")
		for _, path := range []string{
			"/vcs/diff/raw?directory=",
			"/vcs/diff/raw?workspace=%20%09",
			"/vcs/diff/raw?directory=" + invalidPath,
			"/vcs/diff/raw?workspace=" + invalidPath,
		} {
			status, header, body := request(http.MethodGet, path, "")
			if status != http.StatusBadRequest {
				t.Errorf("GET %s: got %d, want 400. Body: %s", path, status, body)
			}
			if got := header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("GET %s Content-Type = %q, want application/json", path, got)
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Errorf("GET %s body is not JSON: %v (%s)", path, err, body)
				continue
			}
			if envelope.Error.Code != "INVALID_REQUEST" || envelope.Error.Message == "" {
				t.Errorf("GET %s error = %+v, want INVALID_REQUEST with message", path, envelope.Error)
			}
			if strings.Contains(envelope.Error.Message, invalidPath) {
				t.Errorf("GET %s error leaks filesystem path: %q", path, envelope.Error.Message)
			}
		}
	})

	t.Run("405 rejects non-GET", func(t *testing.T) {
		status, _, body := request(http.MethodPost, "/vcs/diff/raw", "")
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("POST /vcs/diff/raw: got %d, want 405. Body: %s", status, body)
		}
		if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
			t.Errorf("POST /vcs/diff/raw body = %s, want METHOD_NOT_ALLOWED", body)
		}
	})
}

// TestVCSDiffRawAuthAndRouteWiring covers the production auth distinction:
// header-selected fixed-workspace reads are compatible without credentials,
// while headerless requests remain protected.
func TestVCSDiffRawAuthAndRouteWiring(t *testing.T) {
	repo := makeGitRepo(t)
	shim := NewServer(&mockDB{}, "test-key", nil, nil)
	shim.workdir = t.TempDir()
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	unauth, err := http.Get(srv.URL + "/vcs/diff/raw")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("headerless GET /vcs/diff/raw: got %d, want 401", unauth.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/vcs/diff/raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-opencode-directory", repo)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("header-selected GET /vcs/diff/raw: got %d, want 200. Body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/x-diff; charset=utf-8" {
		t.Errorf("header-selected Content-Type = %q, want text/x-diff; charset=utf-8", got)
	}
}
