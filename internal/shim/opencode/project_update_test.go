package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

func newProjectUpdateTestServer(t *testing.T, workspace string) (*Server, *httptest.Server) {
	t.Helper()
	mdb := &mockDB{queryResults: []db.Row{
		rowOf(map[string]any{"applied_at": "2026-01-01T00:00:00Z"}),
		rowOf(map[string]any{"applied_at": "2026-01-02T00:00:00Z"}),
	}}
	s := NewServer(mdb, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = workspace
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

func requireProjectNotFound(t *testing.T, status int, body []byte, projectID string) {
	t.Helper()
	if status != http.StatusNotFound {
		t.Fatalf("project %q: got %d, want 404. Body: %s", projectID, status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("404 body is not JSON: %v (%s)", err, body)
	}
	want := map[string]any{
		"_tag":      "ProjectNotFoundError",
		"projectID": projectID,
		"message":   "Project not found: " + projectID,
	}
	if len(got) != len(want) {
		t.Errorf("404 body must have exactly %d fields, got %d: %v", len(want), len(got), got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("404 body field %q = %v, want %v", key, got[key], value)
		}
	}
}

func TestProjectUpdateServesDeclared200AndPersistsWorkspaceMetadata(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "consensus.json")
	if err := os.WriteFile(configPath, []byte(`{"existing":"preserved","commands":{"start":"old"}}`), 0o640); err != nil {
		t.Fatalf("seed consensus.json: %v", err)
	}
	_, srv := newProjectUpdateTestServer(t, workspace)
	projectID := instanceID(workspace)

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPatch, "/project/"+projectID,
		`{"name":"Renamed workspace","icon":{"url":"https://example.test/icon.png","override":"data:image/png;base64,abc","color":"#123456"},"commands":{"start":"make serve"}}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH /project/%s: got %d, want 200. Body: %s", projectID, status, body)
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var project map[string]any
	if err := json.Unmarshal(body, &project); err != nil {
		t.Fatalf("200 body is not a Project: %v (%s)", err, body)
	}
	if project["id"] != projectID || project["name"] != "Renamed workspace" {
		t.Errorf("updated project identity = id:%v name:%v, want %q / Renamed workspace", project["id"], project["name"], projectID)
	}
	icon, ok := project["icon"].(map[string]any)
	if !ok || icon["url"] != "https://example.test/icon.png" || icon["override"] != "data:image/png;base64,abc" || icon["color"] != "#123456" {
		t.Errorf("updated icon = %v, want all declared fields", project["icon"])
	}
	commands, ok := project["commands"].(map[string]any)
	if !ok || commands["start"] != "make serve" {
		t.Errorf("updated commands = %v, want start=make serve", project["commands"])
	}
	for _, required := range []string{"id", "worktree", "time", "sandboxes"} {
		if _, present := project[required]; !present {
			t.Errorf("declared 200 Project missing required field %q: %v", required, project)
		}
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted consensus.json: %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("persisted consensus.json is invalid: %v (%s)", err, raw)
	}
	if persisted["existing"] != "preserved" {
		t.Errorf("unrelated consensus.json field was not preserved: %v", persisted)
	}
	if persisted["name"] != "Renamed workspace" {
		t.Errorf("project metadata was not persisted: %v", persisted)
	}
	if info, err := os.Stat(configPath); err != nil {
		t.Fatalf("stat consensus.json: %v", err)
	} else if info.Mode().Perm() != 0o640 {
		t.Errorf("consensus.json mode = %o, want preserved 640", info.Mode().Perm())
	}

	// The literal current alias is resolvable too, and a partial update must
	// preserve metadata fields omitted from the patch.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPatch, "/project/current", `{"name":"Current alias"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH /project/current: got %d, want 200. Body: %s", status, body)
	}
	if err := json.Unmarshal(body, &project); err != nil {
		t.Fatalf("current-alias 200 body is not JSON: %v (%s)", err, body)
	}
	if project["id"] != projectID || project["name"] != "Current alias" {
		t.Errorf("current alias response = id:%v name:%v, want %q / Current alias", project["id"], project["name"], projectID)
	}
	if project["icon"] == nil || project["commands"] == nil {
		t.Errorf("partial update dropped existing metadata: %v", project)
	}

	// A sibling read observes the durable update; the 200 is not a response-only fabrication.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/project/current")
	if status != http.StatusOK {
		t.Fatalf("GET /project/current after update: got %d. Body: %s", status, body)
	}
	if err := json.Unmarshal(body, &project); err != nil {
		t.Fatalf("GET /project/current body is not JSON: %v (%s)", err, body)
	}
	if project["name"] != "Current alias" || project["icon"] == nil || project["commands"] == nil {
		t.Errorf("GET /project/current did not observe persisted metadata: %v", project)
	}
}

func TestProjectUpdateReturnsDeclared400Errors(t *testing.T) {
	workspace := t.TempDir()
	_, srv := newProjectUpdateTestServer(t, workspace)
	path := "/project/" + instanceID(workspace)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"name":`},
		{"null body", `null`},
		{"array body", `[]`},
		{"unknown top-level field", `{"description":"not declared"}`},
		{"null name", `{"name":null}`},
		{"wrong name type", `{"name":42}`},
		{"unknown icon field", `{"icon":{"path":"icon.png"}}`},
		{"wrong icon field type", `{"icon":{"color":42}}`},
		{"unknown commands field", `{"commands":{"stop":"make stop"}}`},
		{"wrong commands field type", `{"commands":{"start":false}}`},
	} {
		t.Run("body/"+tc.name, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPatch, path, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("got %d, want declared 400. Body: %s", status, body)
			}
			var envelope struct {
				Name string `json:"name"`
				Data struct {
					Kind    string `json:"kind"`
					Message string `json:"message"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if envelope.Name != "BadRequest" || envelope.Data.Kind != "Body" || envelope.Data.Message == "" {
				t.Errorf("400 body = %+v, want BadRequest Body envelope", envelope)
			}
		})
	}

	for _, selector := range []string{"directory", "workspace"} {
		t.Run("blank query/"+selector, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPatch, path+"?"+selector+"=", `{}`)
			if status != http.StatusBadRequest {
				t.Fatalf("got %d, want declared 400. Body: %s", status, body)
			}
			var envelope struct {
				Name string `json:"name"`
				Data struct {
					Kind string `json:"kind"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if envelope.Name != "BadRequest" || envelope.Data.Kind != "Query" {
				t.Errorf("400 body = %+v, want BadRequest Query envelope", envelope)
			}
		})
	}

	// A resolvable id with an unwritable workspace representation must fail in
	// the declared vocabulary rather than fabricate a successful update.
	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, broken := newProjectUpdateTestServer(t, notDirectory)
	status, _, body := doShimRequestBody(t, broken.URL, http.MethodPatch,
		"/project/"+instanceID(notDirectory), `{"name":"cannot persist"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("workspace write failure: got %d, want declared 400. Body: %s", status, body)
	}
	var invalid struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &invalid); err != nil {
		t.Fatalf("write-failure 400 body is not JSON: %v (%s)", err, body)
	}
	if invalid.Error.Code != "INVALID_REQUEST" || invalid.Error.Message == "" {
		t.Errorf("write-failure body = %+v, want INVALID_REQUEST with a message", invalid)
	}
}

func TestProjectUpdateMissingAndUnsupportedOperationsRemainUnchanged(t *testing.T) {
	workspace := t.TempDir()
	_, srv := newProjectUpdateTestServer(t, workspace)
	projectID := instanceID(workspace)

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPatch,
		"/project/project_missing", `{"name":"Missing"}`)
	requireProjectNotFound(t, status, body, "project_missing")

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/project/project_missing")
	requireProjectNotFound(t, status, body, "project_missing")

	// POST on a real item id is still not a declared operation on this path and
	// retains the exact typed-not-found answer it had before project.update was served.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/project/"+projectID, `{}`)
	requireProjectNotFound(t, status, body, projectID)

	// The sibling project.directories method guard remains typed 501.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPatch,
		"/project/"+projectID+"/directories", `{}`)
	if status != http.StatusNotImplemented || !strings.Contains(string(body), `"operation":"project.directories"`) {
		t.Errorf("PATCH project.directories = %d %s, want unchanged typed 501", status, body)
	}
}
