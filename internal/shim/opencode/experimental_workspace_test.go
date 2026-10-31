package opencode

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ROUTE-ADD-076 / ROUTE-ADD-077: both methods must be served by one exact
// registration. The test also pins the declared error envelopes and prevents
// the route from claiming future /experimental/workspace/* siblings.
func TestExperimentalWorkspaceListAndCreate(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/workspace")
	if status != http.StatusOK {
		t.Fatalf("GET /experimental/workspace: got %d, want 200; body=%s", status, body)
	}
	var workspaces []any
	if err := json.Unmarshal(body, &workspaces); err != nil {
		t.Fatalf("GET /experimental/workspace body is not JSON: %v", err)
	}
	if workspaces == nil || len(workspaces) != 0 {
		t.Fatalf("GET /experimental/workspace = %s, want an honest empty array", body)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/experimental/workspace", `{"type":"unknown"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /experimental/workspace: got %d, want declared 400; body=%s", status, body)
	}
	if !strings.Contains(string(body), "WorkspaceCreateError") {
		t.Fatalf("POST /experimental/workspace body = %s, want WorkspaceCreateError", body)
	}
}

func TestExperimentalWorkspaceDeclared400AndExactPath(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/experimental/workspace?directory=", "/experimental/workspace?workspace="} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want 400; body=%s", path, status, body)
		}
		if !strings.Contains(string(body), "InvalidRequestError") && !strings.Contains(string(body), "BadRequest") {
			t.Errorf("GET %s body = %s, want a declared bad-request envelope", path, body)
		}
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/workspace/adapter")
	if status != http.StatusNotFound {
		t.Fatalf("GET /experimental/workspace/adapter: got %d, want exact-path 404; body=%s", status, body)
	}
}

func TestExperimentalWorkspaceMissingAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, srv := newTestServerWithServer(t, s)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/workspace")
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /experimental/workspace without key: got %d, want 401; body=%s", status, body)
	}
	if !strings.Contains(string(body), "UnauthorizedError") {
		t.Fatalf("GET /experimental/workspace without key body = %s, want UnauthorizedError envelope", body)
	}
}
