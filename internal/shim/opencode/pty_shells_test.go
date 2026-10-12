package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type ptyShellResponse struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Acceptable bool   `json:"acceptable"`
}

type ptyShellErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestPtyShellsServesDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells")
	if status != http.StatusOK {
		t.Fatalf("GET /pty/shells: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if strings.TrimSpace(string(body)) == "null" {
		t.Fatal("GET /pty/shells returned null, want the declared array")
	}
	var got []ptyShellResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("200 body is not a ShellItem array: %v (%s)", err, body)
	}
	for _, shell := range got {
		if shell.Path == "" || shell.Name == "" {
			t.Errorf("shell has empty path or name: %+v", shell)
		}
		info, err := os.Stat(shell.Path)
		if err != nil {
			t.Errorf("shell path %q does not resolve: %v", shell.Path, err)
		} else if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			t.Errorf("shell path %q is not an executable regular file: mode %s", shell.Path, info.Mode())
		}
		if want := strings.ToLower(filepath.Base(shell.Path)); shell.Name != want {
			t.Errorf("shell name = %q, want basename %q", shell.Name, want)
		}
	}
}

func TestPtyShellsDeclared400Selectors(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	existing := t.TempDir()
	existingFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "missing directory", path: "/pty/shells?directory=/no/such/route-add-104"},
		{name: "file directory", path: "/pty/shells?directory=" + existingFile},
		{name: "blank workspace", path: "/pty/shells?workspace="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400. Body: %s", tc.path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			var got ptyShellErrorResponse
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatalf("400 body is not the shim error envelope: %v (%s)", err, body)
			}
			if got.Error.Code != "INVALID_REQUEST" || strings.TrimSpace(got.Error.Message) == "" {
				t.Errorf("error = %+v, want INVALID_REQUEST with a message", got.Error)
			}
		})
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells?directory="+existing)
	if status != http.StatusOK {
		t.Fatalf("GET /pty/shells with existing directory: got %d, want 200. Body: %s", status, body)
	}
}

func TestPtyShellsAuthStillRequired(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /pty/shells: got %d, want 401. Body: %s", status, body)
	}
	var got ptyShellErrorResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", got.Error.Code)
	}
}

func TestPtyShellsChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells")
	if status != http.StatusOK {
		t.Fatalf("GET /pty/shells via chi mount: got %d, want 200 (not a parent-mount 404). Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells?directory=/missing/route-add-104")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /pty/shells invalid selector via chi mount: got %d, want 400. Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/pty/shells")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /pty/shells via chi mount: got %d, want 405. Body: %s", status, body)
	}
}
