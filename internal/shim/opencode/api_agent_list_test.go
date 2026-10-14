package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/agent".get =
// v2.agent.list, SHIM-DRIFT-001): GET /api/agent, one optional deepObject
// `location` query selector, responses 200 {location: LocationInfo,
// data: AgentV2Info[]} ("Retrieve currently registered agents."), 400
// InvalidRequestError, 401 UnauthorizedError.
//
// LocationInfo requires directory and project (id + directory), with an
// optional workspaceID (pattern ^wrk) — the same declared shape
// v2.fs.find's 200 carries. The shim's location surface is derived from the
// selected workspace (requestWorkspaceDir / instanceID), exactly as
// handleFSFind derives it; no location field is invented.
//
// AgentV2Info requires id, request, mode, hidden and permissions. The
// runtime has no agent-definition registry (no agent definition store
// exists in internal/db), but it DOES have a truthful agent model set:
// consensus supports exactly two agents — the primary build agent and the
// default primary agent (cmd/consensus + the harness runner run one
// primary loop each). The handler answers those two entries with their
// real configuration and never fabricates a third: `data` is
// length-pinned to 2 so a later invented agent fails this test loudly.

type apiAgentListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []struct {
		ID     string `json:"id"`
		Mode   string `json:"mode"`
		Hidden bool   `json:"hidden"`
	} `json:"data"`
}

// TestApiAgentListServesDeclared200 pins v2.agent.list (ROUTE-ADD-005,
// SHIM-DRIFT-001): the exact /api/agent path serves the declared operation
// with the declared 200 shape instead of the pre-fix net/http default 404
// (class NOT-SERVED: no shim route at all).
func TestApiAgentListServesDeclared200(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/agent")
	if status != http.StatusOK {
		t.Fatalf("GET /api/agent: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got apiAgentListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/agent body is not the declared response object: %v (%s)", err, body)
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
		t.Fatalf("data = null, want an AgentV2Info[] array")
	}
	if len(got.Data) != 2 {
		t.Fatalf("data has %d entries, want exactly the 2 agents this runtime truthfully runs (primary build + default primary)", len(got.Data))
	}
	for _, agent := range got.Data {
		if agent.ID == "" {
			t.Errorf("agent entry %+v has an empty id (AgentV2Info.id required)", agent)
		}
		switch agent.Mode {
		case "subagent", "primary", "all":
		default:
			t.Errorf("agent %q mode = %q, want a declared enum value (subagent|primary|all)", agent.ID, agent.Mode)
		}
	}
}

// TestApiAgentListDeepObjectLocation pins the v2 deepObject query spelling:
// location[directory] overrides the workspace, and a blank value is
// malformed input answered with the declared InvalidRequestError 400
// (writeV2InvalidRequest convention — the fs.find sibling's blank-param
// rule). An unknown location[...] field is refused the same way.
func TestApiAgentListDeepObjectLocation(t *testing.T) {
	dir := t.TempDir()
	s, srv := newTestServer(&mockDB{})
	s.workdir = dir
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/agent?location%5Bdirectory%5D="+dir)
	if status != http.StatusOK {
		t.Fatalf("GET /api/agent with location[directory]: got %d, want 200. Body: %s", status, body)
	}
	var got apiAgentListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared response object: %v (%s)", err, body)
	}
	if got.Location.Directory != dir {
		t.Errorf("location.directory = %q, want the requested %q", got.Location.Directory, dir)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/agent?location%5Bworkspace%5D=")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /api/agent with a blank location[workspace]: got %d, want 400. Body: %s", status, body)
	}
	var errBody map[string]any
	if err := json.Unmarshal(body, &errBody); errBody["_tag"] != "InvalidRequestError" || errBody["message"] == "" || err != nil {
		t.Errorf("400 body = %v, want InvalidRequestError with message (declared InvalidRequestError)", errBody)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/agent?location%5Bbogus%5D=x")
	if status != http.StatusBadRequest {
		t.Errorf("GET /api/agent with unknown location[bogus]: got %d, want 400. Body: %s", status, body)
	}
}

// TestApiAgentListMethodAndMountWiring pins the method guard and the chi
// mount: /api/agent must be listed in MountPatterns (cmd/consensus/main.go
// mounts the shim by these patterns — without the entry a parent chi router
// answers before the shim sees the request), non-GET keeps the sibling
// METHOD_NOT_ALLOWED envelope, and the /api/agent/* sub-paths stay
// unclaimed (default 404, no catch-all).
func TestApiAgentListMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/agent" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("/api/agent is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-005)")
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/agent", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/agent: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/agent")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/agent: got %d, want 200. Body: %s", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/agent/bogus")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/agent/bogus: got %d, want 404 (no catch-all for /api/agent sub-paths)", status)
	}
}

// TestApiAgentListServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation (the fs.find
// sibling convention).
func TestApiAgentListServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/agent")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/agent: got %d, want 401. Body: %s", status, body)
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
