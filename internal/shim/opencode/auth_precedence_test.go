package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const fixedWorkspaceDirectory = "/tmp/opencode-fixed-workspace"

type goalCheckingService struct {
	Service
	input SessionCreateInput
}

func (s *goalCheckingService) CreateSession(_ context.Context, input SessionCreateInput) (*SessionCreateResult, error) {
	s.input = input
	if input.Goal == "" {
		return nil, errors.New("goal is required")
	}
	return &SessionCreateResult{
		SessionID: "ses_fixed_workspace",
		Status:    "booting",
		APIKey:    "session-key",
		CreatedAt: "2026-09-26T00:00:00Z",
	}, nil
}

func newAuthPrecedenceServer(t *testing.T, services ...Service) *httptest.Server {
	t.Helper()

	var service Service
	if len(services) > 0 {
		service = services[0]
	}
	shim := NewServer(&mockDB{}, "test-key", nil, service)
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func fixedWorkspaceRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s request: %v", method, url, err)
	}
	req.Header.Set("x-opencode-directory", fixedWorkspaceDirectory)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestFixedWorkspaceMutationAuthPrecedence(t *testing.T) {
	service := &goalCheckingService{}
	srv := newAuthPrecedenceServer(t, service)

	req := fixedWorkspaceRequest(t, http.MethodPost, srv.URL+"/session", `{"title":"fenced"}`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /session: got %d, want 200", resp.StatusCode)
	}
	if service.input.Goal != "fenced" {
		t.Fatalf("POST /session mapped goal = %q, want title fallback %q", service.input.Goal, "fenced")
	}
	var fence map[string]int
	if err := json.Unmarshal([]byte(resp.Header.Get("x-opencode-sync")), &fence); err != nil {
		t.Fatalf("POST /session x-opencode-sync header is not JSON: %v", err)
	}
	if len(fence) == 0 {
		t.Fatal("POST /session x-opencode-sync header must contain a mutation fence")
	}
}

func TestFixedWorkspaceReadAuthPrecedence(t *testing.T) {
	srv := newAuthPrecedenceServer(t)

	req := fixedWorkspaceRequest(t, http.MethodGet, srv.URL+"/path", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /path: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /path: got %d, want 200", resp.StatusCode)
	}
	if fence := resp.Header.Get("x-opencode-sync"); fence != "" {
		t.Fatalf("GET /path: read emitted x-opencode-sync %q", fence)
	}
	var paths map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		t.Fatalf("GET /path response is not JSON: %v", err)
	}
	if got := paths["directory"]; got != fixedWorkspaceDirectory {
		t.Fatalf("GET /path directory = %v, want %q", got, fixedWorkspaceDirectory)
	}
}

func TestFixedWorkspaceNoopMutationAuthPrecedence(t *testing.T) {
	srv := newAuthPrecedenceServer(t)

	req := fixedWorkspaceRequest(t, http.MethodPost, srv.URL+"/log", `{"service":"fence-test","level":"info","message":"noop"}`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /log: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /log: got %d, want 200", resp.StatusCode)
	}
	if fence := resp.Header.Get("x-opencode-sync"); fence != "" {
		t.Fatalf("POST /log: no-op mutation emitted x-opencode-sync %q", fence)
	}
}

func TestFixedWorkspaceRequestIDValidationPrecedesAuth(t *testing.T) {
	srv := newAuthPrecedenceServer(t)

	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "permission reply", path: "/permission/invalid-permission-id/reply", body: `{"reply":"once"}`},
		{name: "question reply", path: "/question/invalid-question-id/reply", body: `{"answers":[["Yes"]]}`},
		{name: "question reject", path: "/question/invalid-question-id/reject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := fixedWorkspaceRequest(t, http.MethodPost, srv.URL+tt.path, tt.body)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST %s: %v", tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("POST %s: got %d, want 400", tt.path, resp.StatusCode)
			}
		})
	}
}

func TestFixedWorkspaceMissingRequestReturnsTypedNotFound(t *testing.T) {
	srv := newAuthPrecedenceServer(t)

	tests := []struct {
		name      string
		path      string
		body      string
		requestID string
		tag       string
	}{
		{name: "permission reply", path: "/permission/per_missing/reply", body: `{"reply":"once"}`, requestID: "per_missing", tag: "PermissionNotFoundError"},
		{name: "question reply", path: "/question/que_missing/reply", body: `{"answers":[["Yes"]]}`, requestID: "que_missing", tag: "QuestionNotFoundError"},
		{name: "question reject", path: "/question/que_missing/reject", requestID: "que_missing", tag: "QuestionNotFoundError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := fixedWorkspaceRequest(t, http.MethodPost, srv.URL+tt.path, tt.body)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST %s: %v", tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("POST %s: got %d, want 404", tt.path, resp.StatusCode)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("POST %s response is not JSON: %v", tt.path, err)
			}
			if body["_tag"] != tt.tag || body["requestID"] != tt.requestID {
				t.Fatalf("POST %s typed error = %#v, want tag=%q requestID=%q", tt.path, body, tt.tag, tt.requestID)
			}
		})
	}
}

func TestFixedWorkspaceAuthBypassIsRouteScoped(t *testing.T) {
	srv := newAuthPrecedenceServer(t)

	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		workspace bool
	}{
		{name: "session create without workspace", method: http.MethodPost, path: "/session", body: `{"title":"protected"}`},
		{name: "session list remains protected", method: http.MethodGet, path: "/session", workspace: true},
		{name: "config remains protected", method: http.MethodGet, path: "/config", workspace: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if tt.workspace {
				req.Header.Set("x-opencode-directory", fixedWorkspaceDirectory)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tt.method, tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("%s %s: got %d, want 401", tt.method, tt.path, resp.StatusCode)
			}
		})
	}
}
