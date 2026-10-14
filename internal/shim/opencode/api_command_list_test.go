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

// Upstream contract (openapi-1.18.33.json paths."/api/command".get =
// v2.command.list, SHIM-DRIFT-002): GET /api/command, one optional deepObject
// `location` query selector, responses 200 {location: LocationInfo, data:
// CommandV2Info[]} ("Retrieve currently registered commands."), 400
// InvalidRequestError, 401 UnauthorizedError.
//
// LocationInfo requires directory and project (id + directory), with an
// optional workspaceID (pattern ^wrk) — the declared shape v2.agent.list's 200
// carries as well. The shim's location surface is derived from the real
// selected workspace (requestWorkspaceDir / instanceID, the handleApiAgentList
// and handleFSFind convention); no location field is invented.
//
// CommandV2Info requires name and template. The Consensus runtime keeps NO v2
// command registry (internal/db has no commands store and no migration creates
// one), so the truthful `data` is an EMPTY array — never null and never a
// fabricated entry, the same honest answer the sibling v1 GET /command route
// (handleCommandList, upstream command.list) gives for its own absent registry
// and the question.list precedent (empty list 200). Both the length pin (0)
// and the raw `"data":[]` assertion below fail loudly if a later change
// invents an entry.

type apiCommandListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []struct {
		Name     string `json:"name"`
		Template string `json:"template"`
	} `json:"data"`
}

// TestApiCommandListServesDeclared200 pins v2.command.list (ROUTE-ADD-006,
// SHIM-DRIFT-002): the exact /api/command path serves the declared operation
// with the declared 200 shape instead of the pre-fix net/http default 404
// (class NOT-SERVED: no shim route at all). The httptest server built here is
// the live probe — no `go run` and no external process is involved.
func TestApiCommandListServesDeclared200(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/command")
	if status != http.StatusOK {
		t.Fatalf("GET /api/command: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	// The declared array member is an EMPTY CommandV2Info[]: pin the raw bytes
	// so a null (or an invented entry) cannot hide behind a struct decode.
	if !strings.Contains(string(body), `"data":[]`) {
		t.Errorf(`GET /api/command body = %s, want the declared "data":[] (an empty CommandV2Info[], never null)`, body)
	}

	var got apiCommandListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/command body is not the declared response object: %v (%s)", err, body)
	}
	if got.Location.Directory == "" {
		t.Errorf("location.directory is empty, want the selected workspace directory")
	}
	if got.Location.Project.ID == "" || got.Location.Project.Directory == "" {
		t.Errorf("location.project = %+v, want id and directory (LocationInfo.project required fields)", got.Location.Project)
	}
	if got.Location.Project.Directory != got.Location.Directory {
		t.Errorf("location.project.directory = %q, want the selected directory %q", got.Location.Project.Directory, got.Location.Directory)
	}
	if got.Data == nil {
		t.Fatalf("data = null, want a CommandV2Info[] array")
	}
	if len(got.Data) != 0 {
		t.Fatalf("data has %d entries, want exactly 0 — this runtime keeps no v2 command registry, so no entry may be invented", len(got.Data))
	}
}

// TestApiCommandListDeepObjectLocation pins the v2 deepObject query spelling:
// location[directory] overrides the workspace, and each malformed selector
// shape is answered with the declared InvalidRequestError 400
// (writeV2InvalidRequest convention, the handleApiAgentList sibling).
func TestApiCommandListDeepObjectLocation(t *testing.T) {
	dir := t.TempDir()
	s, srv := newTestServer(&mockDB{})
	s.workdir = dir
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/command?location%5Bdirectory%5D="+dir)
	if status != http.StatusOK {
		t.Fatalf("GET /api/command with location[directory]: got %d, want 200. Body: %s", status, body)
	}
	var got apiCommandListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared response object: %v (%s)", err, body)
	}
	if got.Location.Directory != dir {
		t.Errorf("location.directory = %q, want the requested %q", got.Location.Directory, dir)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/command?location%5Bworkspace%5D=wrk_S1")
	if status != http.StatusOK {
		t.Fatalf("GET /api/command with a ^wrk location[workspace]: got %d, want 200. Body: %s", status, body)
	}
	var withWorkspace apiCommandListResponse
	if err := json.Unmarshal(body, &withWorkspace); err != nil {
		t.Fatalf("body is not the declared response object: %v (%s)", err, body)
	}
	if withWorkspace.Location.WorkspaceID != "wrk_S1" {
		t.Errorf("location.workspaceID = %q, want the requested wrk_S1", withWorkspace.Location.WorkspaceID)
	}

	missing := filepath.Join(dir, "definitely-not-here")
	cases := []struct {
		name string
		path string
	}{
		{"blank directory", "/api/command?location%5Bdirectory%5D="},
		{"blank workspace", "/api/command?location%5Bworkspace%5D="},
		{"unknown location field", "/api/command?location%5Bbogus%5D=x"},
		{"unreadable directory", "/api/command?location%5Bdirectory%5D=" + missing},
		{"non-^wrk workspace", "/api/command?location%5Bworkspace%5D=ws1"},
	}
	for _, tc := range cases {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
		if status != http.StatusBadRequest {
			t.Errorf("GET %s (%s): got %d, want the declared 400. Body: %s", tc.path, tc.name, status, body)
			continue
		}
		var errBody map[string]any
		if err := json.Unmarshal(body, &errBody); err != nil {
			t.Errorf("GET %s (%s): 400 body is not JSON: %v (%s)", tc.path, tc.name, err, body)
			continue
		}
		if errBody["_tag"] != "InvalidRequestError" || errBody["message"] == "" {
			t.Errorf("GET %s (%s): 400 body = %v, want the declared InvalidRequestError with message", tc.path, tc.name, errBody)
		}
	}

	// A location[directory] naming a FILE (not a directory) is malformed too.
	file := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed %s: %v", file, err)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/command?location%5Bdirectory%5D="+file)
	if status != http.StatusBadRequest {
		t.Errorf("GET /api/command with a file as location[directory]: got %d, want the declared 400. Body: %s", status, body)
	}
}

// TestApiCommandListMethodAndMountWiring pins the method guard and the chi
// mount: /api/command must be listed in MountPatterns (cmd/consensus/main.go
// mounts the shim by these patterns — without the entry a parent chi router
// answers before the shim sees the request), non-GET keeps the sibling
// METHOD_NOT_ALLOWED envelope, and /api/command/* sub-paths stay unclaimed
// (default 404, no catch-all).
func TestApiCommandListMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/command" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("/api/command is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-006)")
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/command", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/command: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/command")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/command: got %d, want 200. Body: %s", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/command/bogus")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/command/bogus: got %d, want 404 (no catch-all for /api/command sub-paths)", status)
	}
	// Non-vacuity control: the v1 route and the /api sibling are untouched.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/command")
	if status != http.StatusOK {
		t.Errorf("GET /command (v1 command.list): got %d, want 200 (unchanged sibling). Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/agent")
	if status != http.StatusOK {
		t.Errorf("GET /api/agent (v2.agent.list): got %d, want 200 (unchanged sibling). Body: %s", status, body)
	}
}

// TestApiCommandListServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation (the /api/agent and
// /api/fs/find convention), not the v1 UNAUTHENTICATED envelope.
func TestApiCommandListServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/command")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/command: got %d, want 401. Body: %s", status, body)
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
