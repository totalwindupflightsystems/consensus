package opencode

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ROUTE-ADD-073: GET /experimental/resource must serve the declared
// experimental.resource.list contract — a 200 JSON OBJECT (never an array,
// never null) mapping resource names to McpResource, truthfully empty because
// this runtime operates no MCP client connections.
func TestExperimentalResourceListServesDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/resource")
	if status != http.StatusOK {
		t.Fatalf("GET /experimental/resource: got %d, want 200; body=%s", status, body)
	}
	var resources map[string]any
	if err := json.Unmarshal(body, &resources); err != nil {
		t.Fatalf("GET /experimental/resource body is not JSON: %v", err)
	}
	if resources == nil || len(resources) != 0 {
		t.Fatalf("GET /experimental/resource = %s, want an honest empty object", body)
	}
}

// The declared 400 arm answers the only malformed input the operation can
// see — a present-but-blank declared selector — and undeclared methods keep
// the sibling METHOD_NOT_ALLOWED envelope.
func TestExperimentalResourceDeclared400And405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{"/experimental/resource?directory=", "/experimental/resource?workspace="} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want 400; body=%s", path, status, body)
		}
		if !strings.Contains(string(body), "BadRequest") {
			t.Errorf("GET %s body = %s, want the declared BadRequest envelope", path, body)
		}
	}

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/experimental/resource", `{}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /experimental/resource: got %d, want 405; body=%s", status, body)
	}
	if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Fatalf("POST /experimental/resource body = %s, want METHOD_NOT_ALLOWED envelope", body)
	}
}

// The registration stays exact: a deeper /experimental/resource/* sub-path
// keeps the net/http default 404 (no catch-all), and an unauthenticated
// request is stopped at the api-key wall with the shim's 401 envelope.
func TestExperimentalResourceExactPathAndMissingAuth(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/resource/extra")
	if status != http.StatusNotFound {
		t.Fatalf("GET /experimental/resource/extra: got %d, want exact-path 404; body=%s", status, body)
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	_, unauth := newTestServerWithServer(t, s)
	defer unauth.Close()

	status, _, body = doShimRequest(t, unauth.URL, http.MethodGet, "/experimental/resource")
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /experimental/resource without key: got %d, want 401; body=%s", status, body)
	}
	// The pinned document declares no 401 arm for experimental.resource.list,
	// so the path keeps the shim's own UNAUTHENTICATED envelope (the
	// /experimental/console precedent) rather than the v2 UnauthorizedError
	// NamedError used by the declared /api/* v2 operations.
	if !strings.Contains(string(body), "UNAUTHENTICATED") {
		t.Fatalf("GET /experimental/resource without key body = %s, want UNAUTHENTICATED envelope", body)
	}
}
