package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sessionCreateService struct {
	Service
	input SessionCreateInput
}

func (s *sessionCreateService) CreateSession(_ context.Context, input SessionCreateInput) (*SessionCreateResult, error) {
	s.input = input
	return &SessionCreateResult{SessionID: "ses_runtime_123", Status: "booting"}, nil
}

func TestApiSessionCreateReturnsRuntimeSessionID(t *testing.T) {
	service := &sessionCreateService{}
	s := NewServer(&mockDB{}, "test-key", nil, service)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session", `{"agent_name":"planner","goal":"ship feature"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /api/session: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data struct {
			ID        string `json:"id"`
			AgentName string `json:"agent_name"`
			Goal      string `json:"goal"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, body)
	}
	if got.Data.ID != "ses_runtime_123" {
		t.Errorf("data.id = %q, want the id returned by the runtime sessions service", got.Data.ID)
	}
	if got.Data.AgentName != "planner" || got.Data.Goal != "ship feature" {
		t.Errorf("created session fields = %#v, want request agent and goal", got.Data)
	}
	if service.input.AgentName != "planner" || service.input.Goal != "ship feature" {
		t.Errorf("service input = %#v, want validated create request", service.input)
	}
}

func TestApiSessionCreateRejectsMalformedBodies(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, &sessionCreateService{})
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, body := range []string{
		`{`,
		`{"agent_name":"planner"}`,
		`{"goal":"ship feature"}`,
		`{"agent_name":" ","goal":"ship feature"}`,
		`{"agent_name":"planner","goal":" "}`,
	} {
		status, _, response := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session", body)
		if status != http.StatusBadRequest {
			t.Errorf("body %s: got %d, want 400. Response: %s", body, status, response)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(response, &got); err != nil {
			t.Errorf("body %s: error response is not JSON: %v", body, err)
			continue
		}
		if got["_tag"] != "InvalidRequestError" {
			t.Errorf("body %s: _tag = %v, want InvalidRequestError", body, got["_tag"])
		}
	}
}

func TestApiSessionCreateUnauthenticatedUsesV2Envelope(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, &sessionCreateService{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session", `{"agent_name":"planner","goal":"ship feature"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/session: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 response is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError", got["_tag"])
	}
	if !isV2EnvelopeRoutePath("/api/session") {
		t.Error("/api/session missing from v2EnvelopeRoutePaths")
	}
}

func TestApiSessionCreateExactRouteRegistration(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, &sessionCreateService{})
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	// ROUTE-ADD-040 registers the declared one-segment parameter shape
	// /api/session/{sessionID} (v2.session.get, GET-only), so "extra" is no
	// longer an unrouted path: POST on it answers the session-get family's
	// METHOD_NOT_ALLOWED (GET is the only method that path declares) and the
	// create route keeps its own exact /api/session registration. A DEEPER
	// sub-path is still unrouted and keeps net/http's default 404 — the
	// original no-catch-all guarantee of ROUTE-ADD-038.
	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session/extra", `{"agent_name":"planner","goal":"ship feature"}`)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/session/extra: got %d, want 405 (the declared v2.session.get path, GET-only, was registered by ROUTE-ADD-040). Body: %s", status, body)
	}
	status, _, body = doShimRequestWithBody(t, srv.URL, http.MethodPost, "/api/session/extra/deeper", `{"agent_name":"planner","goal":"ship feature"}`)
	if status != http.StatusNotFound {
		t.Errorf("POST /api/session/extra/deeper: got %d, want 404 (exact shapes only, never a catch-all). Body: %s", status, body)
	}
}

func doShimRequestWithBody(t *testing.T, baseURL, method, path, body string) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, response.Header, data
}
