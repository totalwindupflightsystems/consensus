package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const apiIntegrationConnectOauthTestPath = "/api/integration/github/connect/oauth"

type apiIntegrationConnectOauthResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data struct {
		AttemptID    string `json:"attemptID"`
		URL          string `json:"url"`
		Instructions string `json:"instructions"`
		Mode         string `json:"mode"`
		Time         struct {
			Created float64 `json:"created"`
			Expires float64 `json:"expires"`
		} `json:"time"`
	} `json:"data"`
}

func TestApiIntegrationConnectOauthRA020ServesTruthfulDeclared200(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		apiIntegrationConnectOauthTestPath, `{"methodID":"oauth","inputs":{},"label":"GitHub"}`)
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want 200. Body: %s", apiIntegrationConnectOauthTestPath, status, body)
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")
	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want IntegrationAttempt object", raw["data"])
	}
	assertExactMembers(t, data, "attemptID", "url", "instructions", "mode", "time")
	timeData, ok := data["time"].(map[string]any)
	if !ok {
		t.Fatalf("data.time = %#v, want object", data["time"])
	}
	assertExactMembers(t, timeData, "created", "expires")

	var got apiIntegrationConnectOauthResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared {location, data} response: %v (%s)", err, body)
	}
	if got.Location.Directory != s.workdir {
		t.Errorf("location.directory = %q, want real selected workspace %q", got.Location.Directory, s.workdir)
	}
	if got.Location.Project.ID != instanceID(s.workdir) || got.Location.Project.Directory != s.workdir {
		t.Errorf("location.project = %+v, want selected workspace's real id and directory", got.Location.Project)
	}
	if got.Data.AttemptID != "" || got.Data.URL != "" || got.Data.Instructions != "" || got.Data.Mode != "" ||
		got.Data.Time.Created != 0 || got.Data.Time.Expires != 0 {
		t.Errorf("data = %+v, want every required IntegrationAttempt property zero-filled; runtime created no OAuth attempt", got.Data)
	}
	for _, invented := range []string{"oauth", "GitHub", "https://", "att_"} {
		if strings.Contains(string(body), invented) {
			t.Errorf("response echoes or invents OAuth attempt state %q: %s", invented, body)
		}
	}
}

func TestApiIntegrationConnectOauthRA020Declared400Matrix(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct {
		name  string
		path  string
		body  string
		field string
	}{
		{name: "missing methodID", path: apiIntegrationConnectOauthTestPath, body: `{"inputs":{}}`, field: "methodID"},
		{name: "blank methodID", path: apiIntegrationConnectOauthTestPath, body: `{"methodID":"  ","inputs":{}}`, field: "methodID"},
		{name: "missing inputs", path: apiIntegrationConnectOauthTestPath, body: `{"methodID":"oauth"}`, field: "inputs"},
		{name: "inputs is not object", path: apiIntegrationConnectOauthTestPath, body: `{"methodID":"oauth","inputs":[]}`, field: "inputs"},
		{name: "input value is not string", path: apiIntegrationConnectOauthTestPath, body: `{"methodID":"oauth","inputs":{"port":7}}`, field: "inputs"},
		{name: "non-JSON body", path: apiIntegrationConnectOauthTestPath, body: `{`, field: "body"},
		{name: "bad location selector", path: apiIntegrationConnectOauthTestPath + "?location%5Bbogus%5D=x", body: `{"methodID":"oauth","inputs":{}}`, field: "location[bogus]"},
		{name: "unknown body field", path: apiIntegrationConnectOauthTestPath, body: `{"methodID":"oauth","inputs":{},"bogus":true}`, field: "bogus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s with %q: got %d, want 400. Body: %s", tc.path, tc.body, status, body)
			}
			if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" || got["message"] == "" {
				t.Errorf("400 body = %v, want declared InvalidRequestError with message", got)
			}
			if got["field"] != tc.field {
				t.Errorf("field = %v, want %q", got["field"], tc.field)
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, apiIntegrationConnectOauthTestPath,
		strings.NewReader(`{"methodID":"oauth","inputs":{}}`))
	req.SetPathValue("integrationID", " ")
	rec := httptest.NewRecorder()
	s.handleApiIntegrationConnectOauth(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank integrationID: got %d, want 400. Body: %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("blank integrationID body is not JSON: %v (%s)", err, rec.Body)
	}
	if got["_tag"] != "InvalidRequestError" || got["field"] != "integrationID" {
		t.Errorf("blank integrationID body = %v, want InvalidRequestError for integrationID", got)
	}
}

func TestApiIntegrationConnectOauthRA020ServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		apiIntegrationConnectOauthTestPath, `{"methodID":"oauth","inputs":{}}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST: got %d, want 401. Body: %s", status, body)
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want declared UnauthorizedError", got)
	}
}

func TestApiIntegrationConnectOauthRA020MethodAndMountWiring(t *testing.T) {
	const mountPattern = "/api/integration/{integrationID}/connect/oauth"
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == mountPattern {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("%s is not registered in MountPatterns", mountPattern)
	}
	if !isV2EnvelopeRoutePath(apiIntegrationConnectOauthTestPath) {
		t.Fatal("declared path is not registered for the v2 UnauthorizedError envelope")
	}
	for _, path := range []string{
		"/api/integration//connect/oauth",
		"/api/integration/github/connect/oauth/extra",
		"/api/integration/github/connect/bogus",
	} {
		if isV2EnvelopeRoutePath(path) {
			t.Errorf("undeclared path %q is treated as the declared v2 operation", path)
		}
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationConnectOauthTestPath,
		`{"methodID":"oauth","inputs":{}}`)
	if status != http.StatusOK {
		t.Fatalf("chi-mounted POST: got %d, want 200. Body: %s", status, body)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body = doShimRequestBody(t, srv.URL, method, apiIntegrationConnectOauthTestPath,
			`{"methodID":"oauth","inputs":{}}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s: 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s: error.code = %q, want METHOD_NOT_ALLOWED", method, got.Error.Code)
		}
	}
}

func TestApiIntegrationConnectOauthRA020TraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_integration_connect_oauth.go")
	if err != nil {
		t.Fatalf("read handler: %v", err)
	}
	want := "row=ROUTE-ADD-020 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.oauth test=internal/shim/opencode/api_integration_connect_oauth_test.go evidence=docs/evidence/ROUTE-ADD-020-live-probe.md witness=none:unattended-worker-session"
	if !strings.Contains(string(raw), want) {
		t.Errorf("handler carries no ch:trace line (want %q)", want)
	}
}
