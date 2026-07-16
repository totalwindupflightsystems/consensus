package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("http://example.com///", "test-key")

	if c.baseURL != "http://example.com" {
		t.Fatalf("baseURL = %q, want %q", c.baseURL, "http://example.com")
	}
	if c.apiKey != "test-key" {
		t.Fatalf("apiKey = %q, want %q", c.apiKey, "test-key")
	}
	if got, want := c.http.Timeout, 30*time.Second; got != want {
		t.Fatalf("timeout = %s, want %s", got, want)
	}
}

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("path = %s, want /api/v1/health", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"healthy",
			"version":"1.0.0",
			"uptime_seconds":42,
			"api_latency_ms":1.5,
			"db_latency_ms":2.5,
			"llm_latency_ms":3.5,
			"error_rate_pct":0.25,
			"db_backend":"sqlite",
			"db_path":"/tmp/consensus.db",
			"db_size_mb":4.5,
			"db_tables":12,
			"db_migrations":3,
			"active_connections":{"websocket":1,"db_pool_active":2,"db_pool_max":5,"llm_active":3,"api_requests_last_min":10},
			"system_log":["started"]
		}`))
	}))
	defer server.Close()

	got, err := NewClient(server.URL, "test-key").Health()
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if got.Status != "healthy" {
		t.Fatalf("Status = %q, want healthy", got.Status)
	}
	if got.UptimeSeconds != 42 {
		t.Fatalf("UptimeSeconds = %d, want 42", got.UptimeSeconds)
	}
	if got.ActiveConnections.DBPoolMax != 5 {
		t.Fatalf("DBPoolMax = %d, want 5", got.ActiveConnections.DBPoolMax)
	}
}

func TestCreateSessionReturnsTypedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions" {
			t.Errorf("request = %s %s, want POST /api/v1/sessions", r.Method, r.URL.Path)
		}
		if got, want := r.Header.Get("Content-Type"), "application/json"; got != want {
			t.Errorf("Content-Type = %q, want %q", got, want)
		}
		var req CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.AgentName != "agent" || req.Goal != "test goal" {
			t.Errorf("request = %#v, want agent/test goal", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"session-1","status":"active","api_key":"response-key","model":"model-1","created_at":"2026-07-15T12:00:00Z"}`))
	}))
	defer server.Close()

	got, err := NewClient(server.URL, "request-key").CreateSession(CreateSessionRequest{
		AgentName: "agent",
		Goal:      "test goal",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if got.ID != "session-1" || got.Status != "active" || got.APIKey != "response-key" {
		t.Fatalf("response = %#v, want typed session response", got)
	}
}

func TestListSessionsReturnsTypedSlice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions" {
			t.Errorf("path = %s, want /api/v1/sessions", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"session-1","agent_name":"agent","model_id":"model-1","status":"active","context_budget":100,"tokens_used_in":2,"tokens_used_out":3,"iteration":4,"heartbeat_at":"now","created_at":"then"}]`))
	}))
	defer server.Close()

	got, err := NewClient(server.URL, "").ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "session-1" || got[0].Iteration != 4 {
		t.Fatalf("response = %#v, want one typed session", got)
	}
}

func TestHTTP400Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"bad_request","message":"invalid input"}}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "").Health()
	if err == nil {
		t.Fatal("Health() error = nil, want HTTP 400 error")
	}
	if got, want := err.Error(), "bad_request: invalid input"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestHTTP500Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal failure"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "").Health()
	if err == nil {
		t.Fatal("Health() error = nil, want HTTP 500 error")
	}
	if !strings.Contains(err.Error(), "HTTP 500: internal failure") {
		t.Fatalf("error = %q, want HTTP 500 body", err)
	}
}
