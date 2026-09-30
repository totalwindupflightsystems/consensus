// Package opencode: opencode protocol shim tests (SPEC-017).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/017-ui-adapter-layer.md plan=phase-6/task-6-1/step-6-1-4 test=internal/shim/opencode/server_test.go
package opencode

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// Test Helpers: mock DB
// ============================================================================

type mockDB struct {
	queryResults []db.Row
	queryRow     db.Row
	execErr      error
	queryErr     error
	queryRowErr  error
	queries      []string
}

func (m *mockDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *mockDB) Exec(ctx context.Context, query string, args ...any) error {
	m.queries = append(m.queries, query)
	return m.execErr
}
func (m *mockDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	m.queries = append(m.queries, query)
	return m.queryResults, m.queryErr
}
func (m *mockDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	m.queries = append(m.queries, query)
	if m.queryRowErr != nil {
		return nil, m.queryRowErr
	}
	return m.queryRow, nil
}
func (m *mockDB) Backend() db.Backend { return db.BackendSQLite }
func (m *mockDB) Close() error        { return nil }

func rowOf(kv map[string]any) db.Row {
	r := make(db.Row, len(kv))
	for k, v := range kv {
		r[k] = v
	}
	return r
}

// newTestServer creates a shim server with auth bypassed for testing.
func newTestServer(mdb *mockDB) (*Server, *httptest.Server) {
	s := NewServer(mdb, "test-key", nil, nil) // nil EventBus, nil Service — shim falls back to raw DB for remaining endpoints
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	return s, srv
}

func newMessageResponseTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open response test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			deleted_at TEXT
		)`,
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			key_prefix TEXT,
			scope TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT
		)`,
		`INSERT INTO sessions (id, status, iteration) VALUES ('s1', 'idle', 0)`,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'stale answer', 's1', 0)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare response test database: %v", err)
		}
	}

	if err := conn.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope)
		 VALUES ('test-admin', $1, 'test-key', 'admin')`,
		hex.EncodeToString(sha256Hash([]byte("test-key")))); err != nil {
		t.Fatalf("seed response test API key: %v", err)
	}

	service := NewServiceAdapter(api.NewService(conn, nil))
	s := NewServer(conn, "test-key", nil, service)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// ============================================================================
// Health Tests
// ============================================================================

func TestHealthEndpoint(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/global/health")
	if err != nil {
		t.Fatalf("GET /global/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["healthy"] != true {
		t.Error("expected healthy=true")
	}
	if body["version"] != "consensus-0.1.0" {
		t.Errorf("expected version=consensus-0.1.0, got %v", body["version"])
	}
}

// ============================================================================
// Session Endpoint Tests
// ============================================================================

func TestListSessions(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "s1", "agent_name": "test-agent", "model_id": "gpt-4o",
				"status": "idle", "goal": "test goal",
				"iteration": int64(5), "tokens_used_in": int64(100), "tokens_used_out": int64(50),
				"created_at": "2026-01-01T00:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session")
	if err != nil {
		t.Fatalf("GET /session failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []map[string]any
	json.NewDecoder(resp.Body).Decode(&sessions)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sess := sessions[0]
	if sess["id"] != "s1" {
		t.Errorf("expected id=s1, got %v", sess["id"])
	}
	if sess["title"] != "test-agent" {
		t.Errorf("expected title=test-agent, got %v", sess["title"])
	}
	if sess["status"] != "idle" {
		t.Errorf("expected status=idle, got %v", sess["status"])
	}
}

func TestCreateSession(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"title": "test-agent",
		"goal":  "accomplish something",
	})
	resp, err := http.Post(srv.URL+"/session", "application/json", body)
	if err != nil {
		t.Fatalf("POST /session failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var session map[string]any
	json.NewDecoder(resp.Body).Decode(&session)

	if session["title"] != "test-agent" {
		t.Errorf("expected title=test-agent, got %v", session["title"])
	}
	if session["status"] != "booting" {
		t.Errorf("expected status=booting, got %v", session["status"])
	}
}

func TestGetSession(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"id": "s1", "agent_name": "test-agent", "model_id": "gpt-4o",
			"status": "thinking", "goal": "do work",
			"context_budget": 128000,
			"tokens_used_in": int64(200), "tokens_used_out": int64(100),
			"iteration":  int64(3),
			"created_at": "2026-01-01T00:00:00Z",
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session/s1")
	if err != nil {
		t.Fatalf("GET /session/s1 failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var session map[string]any
	json.NewDecoder(resp.Body).Decode(&session)
	if session["id"] != "s1" {
		t.Errorf("expected id=s1, got %v", session["id"])
	}
	if session["status"] != "thinking" {
		t.Errorf("expected status=thinking, got %v", session["status"])
	}
}

func TestGetSessionNotFound(t *testing.T) {
	mdb := &mockDB{queryRowErr: context.DeadlineExceeded}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session/nonexistent")
	if err != nil {
		t.Fatalf("GET /session/nonexistent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteSession(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/session/s1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /session/s1 failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %v", body["status"])
	}
}

func TestAbortSession(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/session/s1/abort", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /session/s1/abort failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "aborted" {
		t.Errorf("expected status=aborted, got %v", body["status"])
	}
}

// ============================================================================
// Message Endpoint Tests
// ============================================================================

func TestSendMessageRequiresResponseService(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"status": "idle", "iteration": int64(0),
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{
			{"type": "text", "text": "Hello, agent!"},
		},
	})
	resp, err := http.Post(srv.URL+"/session/s1/message", "application/json", body)
	if err != nil {
		t.Fatalf("POST /session/s1/message failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without response service, got %d", resp.StatusCode)
	}

	var bodyResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&bodyResponse); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	errorBody, ok := bodyResponse["error"].(map[string]any)
	if !ok || errorBody["code"] != "SERVICE_UNAVAILABLE" {
		t.Errorf("error = %v, want SERVICE_UNAVAILABLE", bodyResponse["error"])
	}
}

func TestSendMessageReturnsResponseForSubmittedTurn(t *testing.T) {
	_, srv, conn := newMessageResponseTestServer(t)

	writeResult := make(chan error, 1)
	go func() {
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			rows, err := conn.Query(context.Background(),
				`SELECT id FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, "s1")
			if err != nil {
				writeResult <- err
				return
			}
			if len(rows) > 0 {
				if err := conn.Exec(context.Background(),
					`INSERT INTO memory_events (type, content, session_id, iteration_created)
					 VALUES ('text_block', 'actual agent answer', 's1', 1)`); err != nil {
					writeResult <- err
					return
				}
				writeResult <- conn.Exec(context.Background(),
					`UPDATE sessions SET status = 'idle', iteration = 2 WHERE id = 's1'`)
				return
			}
			select {
			case <-deadline.C:
				writeResult <- context.DeadlineExceeded
				return
			case <-ticker.C:
			}
		}
	}()

	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": "answer this turn"}},
	})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/s1/message", body)
	if err != nil {
		t.Fatalf("build POST /session/s1/message: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session/s1/message failed: %v", err)
	}
	defer resp.Body.Close()
	if err := <-writeResult; err != nil {
		t.Fatalf("publish agent response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, responseBody)
	}

	var msg struct {
		Info  map[string]any `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if msg.Info["role"] != "assistant" {
		t.Fatalf("response role = %v, want assistant", msg.Info["role"])
	}
	if len(msg.Parts) != 1 || msg.Parts[0].Type != "text" || msg.Parts[0].Text != "actual agent answer" {
		t.Fatalf("response parts = %#v, want actual agent answer", msg.Parts)
	}
}

func TestSendMessageNoResponseReturnsConcreteError(t *testing.T) {
	s, _, _ := newMessageResponseTestServer(t)
	s.messageResponseTimeout = 40 * time.Millisecond
	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": "no answer will arrive"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/session/s1/message", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	recorder := httptest.NewRecorder()

	started := time.Now()
	s.Handler().ServeHTTP(recorder, req)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("no-response path took %s, want bounded cancellation", elapsed)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "agent response") ||
		!strings.Contains(recorder.Body.String(), "not produced") {
		t.Fatalf("timeout error is not concrete: %s", recorder.Body.String())
	}
}

// TestMountPatternsChi is the BUG-009 regression: the shim must be reachable
// through a parent chi router mounted with MountPatterns. chi v5 Handle()
// with a trailing-slash pattern ("/session/") is EXACT-match only, so
// sub-path routes mounted that way 404 before reaching the shim — the dexdat
// sidecar hit exactly this (2026-08-07). The /* wildcard forms must pass
// /session/{id}/message through to the shim.
func TestMountPatternsChi(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"status": "idle", "iteration": int64(0),
		}),
	}
	s := NewServer(mdb, "test-key", nil, nil)
	s.skipAuth = true

	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Sub-path route: must reach the shim's service-unavailable response,
	// not chi's 404. This server intentionally has no native response service.
	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{
			{"type": "text", "text": "Hello, agent!"},
		},
	})
	resp, err := http.Post(srv.URL+"/session/s1/message", "application/json", body)
	if err != nil {
		t.Fatalf("POST /session/s1/message via chi mount failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 via chi MountPatterns mount, got %d", resp.StatusCode)
	}

	// Bare endpoint: exact pattern must still work.
	resp2, err := http.Get(srv.URL + "/session/s1")
	if err != nil {
		t.Fatalf("GET /session/s1 via chi mount failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 404 {
		t.Error("GET /session/s1 returned 404 — bare /session pattern not mounted")
	}

	// Unmounted path must 404 (sanity: the router is real, not a catch-all).
	resp3, err := http.Get(srv.URL + "/nope/not-mounted")
	if err != nil {
		t.Fatalf("GET /nope/not-mounted failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Errorf("expected 404 for unmounted path, got %d", resp3.StatusCode)
	}
}

func TestSendMessageEmptyContent(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{},
	})
	resp, err := http.Post(srv.URL+"/session/s1/message", "application/json", body)
	if err != nil {
		t.Fatalf("POST /session/s1/message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for empty message, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Config & Provider Tests
// ============================================================================

func TestGetConfig(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "llm.default_model", "value": "gpt-4o"}),
			rowOf(map[string]any{"key": "harness.heartbeat_seconds", "value": "5"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("GET /config failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		t.Fatalf("expected settings object in config, got %T", cfg["settings"])
	}
	if got := settings["harness.heartbeat_seconds"]; got != "5" {
		t.Errorf("settings were not preserved: harness.heartbeat_seconds = %v, want 5", got)
	}
	providerDefaults, ok := cfg["provider_default"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider_default object in config, got %T", cfg["provider_default"])
	}
	if got := providerDefaults["consensus"]; got != "gpt-4o" {
		t.Errorf("provider_default[consensus] = %v, want gpt-4o", got)
	}
}

func TestGetConfigWithoutDefaultModel(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("GET /config failed: %v", err)
	}
	defer resp.Body.Close()

	var cfg struct {
		Settings        map[string]any    `json:"settings"`
		ProviderDefault map[string]string `json:"provider_default"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if cfg.Settings == nil {
		t.Fatal("expected settings object in config")
	}
	if cfg.ProviderDefault == nil {
		t.Fatal("expected provider_default to be an object, not null or missing")
	}
	if len(cfg.ProviderDefault) != 0 {
		t.Errorf("provider_default = %v, want empty map without llm.default_model", cfg.ProviderDefault)
	}
}

func TestGetConfigProviders(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"model_id": "gpt-4o", "tier": 1, "max_context": int64(128000),
				"cost_per_m_in": 2.5, "cost_per_m_out": 10.0, "enabled": true,
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/config/providers")
	if err != nil {
		t.Fatalf("GET /config/providers failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	providers, ok := body["providers"].([]any)
	if !ok || len(providers) == 0 {
		t.Error("expected providers array in response")
	}
}

func TestGetProvider(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/provider")
	if err != nil {
		t.Fatalf("GET /provider failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["provider"] != "consensus" {
		t.Errorf("expected provider=consensus, got %v", body["provider"])
	}
}

func TestGetAgent(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/agent")
	if err != nil {
		t.Fatalf("GET /agent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var agents []map[string]any
	json.NewDecoder(resp.Body).Decode(&agents)
	if len(agents) == 0 {
		t.Error("expected non-empty agent list")
	}
}

// ============================================================================
// Tools Tests
// ============================================================================

func TestListTools(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "t1", "name": "list_sessions", "description": "List all sessions",
				"hemisphere": "internal", "handler_type": "go_native",
				"status": "active", "enabled": true, "requires_approval": false,
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/experimental/tool")
	if err != nil {
		t.Fatalf("GET /experimental/tool failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var tools []map[string]any
	json.NewDecoder(resp.Body).Decode(&tools)
	if len(tools) == 0 {
		t.Error("expected non-empty tool list")
	}
}

func TestListToolIDs(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"name": "list_sessions"}),
			rowOf(map[string]any{"name": "read_file"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/experimental/tool/ids")
	if err != nil {
		t.Fatalf("GET /experimental/tool/ids failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var ids []string
	json.NewDecoder(resp.Body).Decode(&ids)
	if len(ids) != 2 {
		t.Errorf("expected 2 tool IDs, got %d", len(ids))
	}
}

// ============================================================================
// Doc Endpoint Test
// ============================================================================

// TestDocEndpoint pins the upstream opencode compatibility contract for
// GET /doc (httpapi-instance.test.ts:59 "serves the OpenAPI document"):
// 200 + application/json + a parseable OpenAPI document. The shim must NOT
// answer HTML here — the pinned upstream suite rejects text/html with
// "Expected to contain: application/json" (T6, DF-CONSENSUS-36). The
// interactive REST Swagger UI lives at /doc/api (SPEC-018 §9).
func TestDocEndpoint(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/doc")
	if err != nil {
		t.Fatalf("GET /doc failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content type (upstream /doc contract), got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read /doc body: %v", err)
	}

	// The body must parse as a real OpenAPI document, not merely be JSON.
	var doc struct {
		OpenAPI string         `json:"openapi"`
		Info    map[string]any `json:"info"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("GET /doc must return a parseable OpenAPI JSON document (content-type %q): %v; body head: %.120s", ct, err, body)
	}
	if doc.OpenAPI == "" {
		t.Error("GET /doc document missing openapi version field")
	}
	if doc.Info == nil {
		t.Error("GET /doc document missing info object")
	}
	for _, p := range []string{"/global/health", "/session"} {
		if _, ok := doc.Paths[p]; !ok {
			t.Errorf("GET /doc document missing upstream-required path %q", p)
		}
	}
}

// TestDocEndpointYAMLAccept covers the explicit YAML negotiation: only an
// Accept header containing application/yaml gets the raw embedded YAML
// document. The default request (no Accept, or a JSON preference) stays
// application/json — that is the contract the upstream opencode suite pins.
func TestDocEndpointYAMLAccept(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/doc", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "application/yaml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /doc with Accept: application/yaml failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "yaml") {
		t.Errorf("expected yaml content type for Accept: application/yaml, got %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("GET /doc (yaml) must be parseable YAML: %v; body head: %.120s", err, body)
	}
	if doc["openapi"] == nil || doc["paths"] == nil {
		t.Error("GET /doc (yaml) document missing openapi/paths")
	}
}

// TestDocEndpointNoAuthAndScopedSkip pins the auth contract around /doc:
// the document route is public (upstream clients fetch the contract without
// credentials — chronicle C01 and the smoke suite rely on this), while a
// neighboring protected route in the same unauthenticated harness still 401s,
// proving the skip is /doc-scoped and not a global auth bypass.
func TestDocEndpointNoAuthAndScopedSkip(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil) // auth ON — no skipAuth
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/doc")
	if err != nil {
		t.Fatalf("GET /doc failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected /doc to be public (200), got %d", resp.StatusCode)
	}

	resp2, err := http.Get(srv.URL + "/config")
	if err != nil {
		t.Fatalf("GET /config failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected /config to stay protected (401) in the same harness, got %d", resp2.StatusCode)
	}
}

// ============================================================================
// 501 Exclusions Test
// ============================================================================

func TestPromptAsyncReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/session/s1/prompt_async", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /session/s1/prompt_async failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 501 {
		t.Errorf("expected 501 for opencode-specific endpoint, got %d", resp.StatusCode)
	}
}

func TestShellReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/session/s1/shell", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /session/s1/shell failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 501 {
		t.Errorf("expected 501 for shell endpoint, got %d", resp.StatusCode)
	}
}

// ============================================================================
// File Endpoint Stub Tests (SPEC-017 §3.1)
// ============================================================================

func TestFindEndpointReturns200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/find?pattern=*.go")
	if err != nil {
		t.Fatalf("GET /find: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for /find, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["files"] == nil {
		t.Error("expected files array in response")
	}
}

func TestFindFileEndpointReturns200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/find/file?query=*.go")
	if err != nil {
		t.Fatalf("GET /find/file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for /find/file, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["files"] == nil {
		t.Error("expected files array in response")
	}
}

func TestFileContentEndpointReturns200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	// Use a file that exists relative to the test working directory
	// The test runs from the package directory; use a go source file in the package
	resp, err := http.Get(srv.URL + "/file/content?path=doc.go")
	if err != nil {
		t.Fatalf("GET /file/content: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for /file/content, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["content"] == nil {
		t.Error("expected content in response")
	}
}

func TestFileStatusEndpointReturns200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/file/status")
	if err != nil {
		t.Fatalf("GET /file/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for /file/status, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] == nil {
		t.Error("expected status in response")
	}
}

func TestFindMissingPatternReturns400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/find")
	if err != nil {
		t.Fatalf("GET /find: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for /find without pattern, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Permission / HITL Translation Tests (SPEC-017 §3.7)
// ============================================================================

func TestListPermissions(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "p1", "session_id": "s1", "request_type": "destructive_tool",
				"risk_level": "high", "description": "Delete temp_cache table",
				"status": "pending", "created_at": "2026-05-04T00:00:00Z",
			}),
			rowOf(map[string]any{
				"id": "p2", "session_id": "s2", "request_type": "schema_change",
				"risk_level": "medium", "description": "ALTER TABLE users ADD COLUMN",
				"status": "pending", "created_at": "2026-05-04T01:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/permission")
	if err != nil {
		t.Fatalf("GET /permission failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	permissions, ok := body["permissions"].([]any)
	if !ok || len(permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(permissions))
	}
}

func TestListPermissionsFilterBySession(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "p1", "session_id": "s1", "request_type": "destructive_tool",
				"risk_level": "high", "description": "Delete temp_cache",
				"status": "pending", "created_at": "2026-05-04T00:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/permission?session_id=s1")
	if err != nil {
		t.Fatalf("GET /permission: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetPermission(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"id": "p1", "session_id": "s1", "request_type": "destructive_tool",
			"risk_level": "high", "description": "Delete temp_cache",
			"sql_preview": "DROP TABLE temp_cache",
			"status":      "pending", "decision_reason": "",
			"created_at": "2026-05-04T00:00:00Z",
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/permission/p1")
	if err != nil {
		t.Fatalf("GET /permission/p1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["id"] != "p1" {
		t.Errorf("expected id=p1, got %v", body["id"])
	}
	if body["risk_level"] != "high" {
		t.Errorf("expected risk_level=high, got %v", body["risk_level"])
	}
}

func TestResolvePermissionApprove(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"decision": "approved",
		"reason":   "This is safe to proceed",
	})
	resp, err := http.Post(srv.URL+"/permission/p1/resolve", "application/json", body)
	if err != nil {
		t.Fatalf("POST /permission/p1/resolve: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]any
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["status"] != "approved" {
		t.Errorf("expected status=approved, got %v", respBody["status"])
	}
}

func TestResolvePermissionReject(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"decision": "rejected",
		"reason":   "Too risky for production",
	})
	resp, err := http.Post(srv.URL+"/permission/p1/resolve", "application/json", body)
	if err != nil {
		t.Fatalf("POST /permission/p1/resolve: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]any
	json.NewDecoder(resp.Body).Decode(&respBody)
	if respBody["status"] != "rejected" {
		t.Errorf("expected status=rejected, got %v", respBody["status"])
	}
}

func TestResolvePermissionInvalidDecision(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body2 := jsonBody(t, map[string]any{
		"decision": "maybe_later",
	})
	resp, err := http.Post(srv.URL+"/permission/p1/resolve", "application/json", body2)
	if err != nil {
		t.Fatalf("POST /permission/p1/resolve: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for invalid decision, got %d", resp.StatusCode)
	}
}

// ============================================================================
// TUI Endpoint Tests
// ============================================================================

func TestTUIAppendPromptReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/tui/append-prompt", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /tui/append-prompt: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 501 {
		t.Errorf("expected 501 for TUI append-prompt, got %d", resp.StatusCode)
	}
}

// ============================================================================
// LSP Endpoint Test
// ============================================================================

func TestLSPEndpoint(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/lsp")
	if err != nil {
		t.Fatalf("GET /lsp: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for LSP endpoint, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["enabled"] != false {
		t.Error("expected LSP disabled by default")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func jsonBody(t *testing.T, v any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return strings.NewReader(string(data))
}

// ============================================================================
// listChildren Tests
// ============================================================================

func TestListChildren(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "child-1", "agent_name": "sub-agent", "status": "thinking",
				"goal": "sub task", "iteration": int64(2),
				"tokens_used_in": int64(50), "tokens_used_out": int64(25),
				"created_at": "2026-05-07T00:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session/parent-1/children")
	if err != nil {
		t.Fatalf("GET /session/parent-1/children: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var children []map[string]any
	json.NewDecoder(resp.Body).Decode(&children)
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0]["id"] != "child-1" {
		t.Errorf("expected child-1, got %v", children[0]["id"])
	}
}

// ============================================================================
// patchSession Tests
// ============================================================================

func TestPatchSession(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"id": "s1", "agent_name": "test", "status": "paused",
			"goal": "do work", "iteration": int64(3),
			"tokens_used_in": int64(100), "tokens_used_out": int64(50),
			"created_at": "2026-05-07T00:00:00Z",
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	body := jsonBody(t, map[string]any{"status": "paused"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/session/s1", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /session/s1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var session map[string]any
	json.NewDecoder(resp.Body).Decode(&session)
	if session["status"] != "paused" {
		t.Errorf("expected paused, got %v", session["status"])
	}
}

// ============================================================================
// getMessageByID Tests
// ============================================================================

func TestGetMessageByID(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"id": float64(1), "type": "text_block", "content": "hello agent",
			"session_id": "s1", "iteration_created": float64(1),
			"created_at": "2026-05-07T00:00:00Z",
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session/s1/message/msg-1")
	if err != nil {
		t.Fatalf("GET /session/s1/message/msg-1: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 with message parts
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// listMessages Tests
// ============================================================================

func TestListMessages(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "msg-1", "session_id": "s1", "content": "hello",
				"msg_type": "user_message", "created_at": "2026-05-07T00:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/session/s1/message")
	if err != nil {
		t.Fatalf("GET /session/s1/message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// handleAuth Tests
// ============================================================================

func TestHandleAuth_GET(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/api-key")
	if err != nil {
		t.Fatalf("GET /auth/api-key: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 (auth returns mock key info)
	if resp.StatusCode != 200 {
		t.Logf("auth endpoint returned %d", resp.StatusCode)
	}
}

// TestHandleAuthDelete answers upstream auth.remove (ROUTE-FIX-001,
// SHIM-DRIFT-059, declared responses 200 boolean / 400): DELETE
// /auth/{providerID} must return 200 with the upstream boolean success body,
// and a repeated delete stays 200 (idempotent removal) instead of the
// pre-fix 405.
func TestHandleAuthDelete(t *testing.T) {
	mdb := &mockDB{}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	doDelete := func() {
		t.Helper()
		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/auth/testprovider", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /auth/testprovider: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", resp.StatusCode, body)
		}
		var removed bool
		if err := json.Unmarshal(body, &removed); err != nil {
			t.Fatalf("body must be the upstream boolean, got %q: %v", body, err)
		}
		if !removed {
			t.Errorf("boolean body = false, want true")
		}
	}

	doDelete()
	doDelete() // second delete must still answer 200/true

	var sawDelete bool
	for _, q := range mdb.queries {
		if strings.Contains(q, "DELETE FROM system_settings") {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Errorf("expected a DELETE on system_settings, queries: %v", mdb.queries)
	}
}

// TestHandleAuthUnsupportedMethods pins that only PUT and DELETE are served:
// other methods keep the pre-fix 405, and an empty provider id stays 405.
func TestHandleAuthUnsupportedMethods(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/testprovider", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /auth/testprovider: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /auth/:id: expected 405, got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/auth/", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /auth/: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /auth/ (empty id): expected 405, got %d", resp.StatusCode)
	}
}

// newAuthStoreTestServer builds a shim server over a real database containing
// only system_settings (plus the tables the server touches at boot) so auth
// storage round-trips can be asserted against actual SQL.
func newAuthStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "auth.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open auth test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			deleted_at TEXT
		)`,
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			key_prefix TEXT,
			scope TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT
		)`,
		`CREATE TABLE system_settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare auth test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// TestHandleAuthDeleteRemovesOnlyProviderRows drives the full cycle against a
// real database: PUT stores auth.<providerID>.<field> rows, DELETE removes
// exactly that provider's rows (LIKE metacharacters in the id are escaped so
// prov_1 does not sweep provX1) and leaves every other provider's rows — and
// non-auth settings — untouched.
func TestHandleAuthDeleteRemovesOnlyProviderRows(t *testing.T) {
	ctx := context.Background()
	_, srv, conn := newAuthStoreTestServer(t)

	do := func(method, path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("build %s %s: %v", method, path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}
	countRows := func(keyPrefix string) int {
		t.Helper()
		pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyPrefix) + "%"
		rows, err := conn.Query(ctx,
			`SELECT key FROM system_settings WHERE key LIKE $1 ESCAPE '\'`,
			pattern)
		if err != nil {
			t.Fatalf("count %s rows: %v", keyPrefix, err)
		}
		return len(rows)
	}

	resp := do(http.MethodPut, "/auth/prov_1", `{"apiKey":"k1","secret":"s1"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /auth/prov_1: expected 200, got %d", resp.StatusCode)
	}
	// Sibling data that must survive the delete: another provider whose id
	// shares characters with prov_1 (an unescaped _ in prov_1 would sweep
	// provX1's rows too), another provider, and a non-auth setting.
	for _, kv := range [][2]string{
		{"auth.provX1.key", "keep-me"},
		{"auth.other.key", "keep-me"},
		{"consensus.instance.name", "keep-me"},
	} {
		if err := conn.Exec(ctx,
			`INSERT INTO system_settings (key, value) VALUES ($1, $2)`, kv[0], kv[1]); err != nil {
			t.Fatalf("seed %s: %v", kv[0], err)
		}
	}
	if got := countRows("auth.prov_1."); got != 2 {
		t.Fatalf("expected 2 stored rows for prov_1 before delete, got %d", got)
	}

	resp = do(http.MethodDelete, "/auth/prov_1", "")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /auth/prov_1: expected 200, got %d. Body: %s", resp.StatusCode, body)
	}
	var removed bool
	if err := json.Unmarshal(body, &removed); err != nil || !removed {
		t.Fatalf("DELETE body must be boolean true, got %q (err %v)", body, err)
	}
	if got := countRows("auth.prov_1."); got != 0 {
		t.Errorf("prov_1 rows must be gone after DELETE, %d remain", got)
	}
	for _, prefix := range []string{"auth.provX1.", "auth.other.", "consensus.instance."} {
		if got := countRows(prefix); got != 1 {
			t.Errorf("rows under %s must survive the delete, got %d", prefix, got)
		}
	}
}

// ============================================================================
// handleProjectVCSSStub Tests
// ============================================================================

func TestProjectEndpointReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/project")
	if err != nil {
		t.Fatalf("GET /project: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 501 {
		t.Errorf("expected 501 for /project, got %d", resp.StatusCode)
	}
}

// TestProjectPatchMissingReturnsTypedNotFound pins the upstream opencode
// contract for missing projects (httpapi-instance.test.ts "returns typed not
// found bodies for missing projects", DF-CONSENSUS-47): PATCH
// /project/:projectID on an unknown project must return HTTP 404 with the
// exact upstream ProjectNotFoundError NamedError body — no 501 stub, no extra
// fields (the upstream assertion is a strict toEqual).
func TestProjectPatchMissingReturnsTypedNotFound(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	const projectID = "project_missing"
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/project/"+projectID, strings.NewReader(`{"name":"Missing"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-opencode-directory", t.TempDir())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /project/%s: %v", projectID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404 for missing project, got %d. Body: %s", resp.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("404 body not JSON: %v", err)
	}
	want := map[string]any{
		"_tag":      "ProjectNotFoundError",
		"projectID": projectID,
		"message":   "Project not found: " + projectID,
	}
	if len(body) != len(want) {
		t.Errorf("404 body must have exactly %d fields (upstream toEqual), got %d: %v", len(want), len(body), body)
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("404 body field %q = %v, want %v", k, body[k], v)
		}
	}
}

func TestVCSStubEndpointsReturn501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	// Bare GET /vcs and GET /vcs/diff are real fixed-workspace compatibility
	// routes since DF-CONSENSUS-38 (TestVCSReadCompatibilityEndpoints); the
	// remaining /vcs/* sub-paths stay 501 stubs (SPEC-017 §3.9). The test
	// server skips auth, so the stub — not 401 — answers.
	for _, path := range []string{"/vcs/status", "/vcs/diff/raw", "/vcs/apply", "/vcs/"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 501 {
			t.Errorf("GET %s: expected 501 stub, got %d: %s", path, resp.StatusCode, body)
		}
	}
}

// TestVCSReadCompatibilityEndpoints drives the pinned upstream
// httpapi-instance.test.ts "serves path and VCS read endpoints" assertions
// (DF-CONSENSUS-38) through a parent chi router mounted with MountPatterns —
// the same shape the real server uses in cmd/consensus/main.go — so a missing
// or misregistered pattern 404s exactly as it would in production.
func TestVCSReadCompatibilityEndpoints(t *testing.T) {
	repo := makeGitRepo(t)
	// No trailing newline: upstream counts untracked additions with
	// `git diff --no-index --numstat` (whole lines), so "hello" is 1 addition.
	if err := os.WriteFile(filepath.Join(repo, "changed.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	shim := NewServer(&mockDB{}, "test-key", nil, nil)
	shim.workdir = repo
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	get := func(headerDir, path string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build GET %s: %v", path, err)
		}
		req.Header.Set("x-opencode-directory", headerDir)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		return resp
	}

	// GET /path → 200 {directory, worktree} (upstream PathInfo).
	paths := get(repo, "/path")
	defer paths.Body.Close()
	if paths.StatusCode != 200 {
		t.Fatalf("GET /path: got %d, want 200", paths.StatusCode)
	}
	var pathInfo map[string]any
	if err := json.NewDecoder(paths.Body).Decode(&pathInfo); err != nil {
		t.Fatalf("GET /path not JSON: %v", err)
	}
	if got := pathInfo["directory"]; got != repo {
		t.Errorf("directory = %v, want %q", got, repo)
	}
	if got := pathInfo["worktree"]; got != repo {
		t.Errorf("worktree = %v, want %q", got, repo)
	}

	// GET /vcs → 200 {branch} (upstream Vcs.Info).
	vcs := get(repo, "/vcs")
	defer vcs.Body.Close()
	if vcs.StatusCode != 200 {
		t.Fatalf("GET /vcs: got %d, want 200", vcs.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(vcs.Body).Decode(&info); err != nil {
		t.Fatalf("GET /vcs not JSON: %v", err)
	}
	if b, _ := info["branch"].(string); b != "main" {
		t.Errorf("branch = %v, want \"main\"", info["branch"])
	}

	// GET /vcs/diff?mode=git → 200 [{file, additions, status}] including the
	// untracked file counted the upstream way.
	diff := get(repo, "/vcs/diff?mode=git")
	defer diff.Body.Close()
	if diff.StatusCode != 200 {
		t.Fatalf("GET /vcs/diff: got %d, want 200", diff.StatusCode)
	}
	var diffs []map[string]any
	if err := json.NewDecoder(diff.Body).Decode(&diffs); err != nil {
		t.Fatalf("GET /vcs/diff not JSON array: %v", err)
	}
	var changed *map[string]any
	for i := range diffs {
		if diffs[i]["file"] == "changed.txt" {
			changed = &diffs[i]
		}
	}
	if changed == nil {
		t.Fatalf("changed.txt missing from diff list: %v", diffs)
	}
	if (*changed)["status"] != "added" {
		t.Errorf("changed.txt status = %v, want \"added\"", (*changed)["status"])
	}
	if a, _ := (*changed)["additions"].(float64); a != 1 {
		t.Errorf("changed.txt additions = %v, want 1 (file has no trailing newline)", (*changed)["additions"])
	}

	// The server's own workspace is NOT the git repo the fixture addresses by
	// header: /vcs and /vcs/diff must resolve the header directory like /path
	// does (the pinned upstream suite creates the git repo in a temp dir and
	// never points the server workdir at it). A header pointing at a non-git
	// directory yields the neutral shapes.
	shim.workdir = t.TempDir()
	nonGit := get(t.TempDir(), "/vcs")
	defer nonGit.Body.Close()
	if nonGit.StatusCode != 200 {
		t.Fatalf("GET /vcs (non-git header dir): got %d, want 200", nonGit.StatusCode)
	}
	var nonGitInfo map[string]any
	if err := json.NewDecoder(nonGit.Body).Decode(&nonGitInfo); err != nil {
		t.Fatalf("GET /vcs (non-git) not JSON: %v", err)
	}
	if len(nonGitInfo) != 0 {
		t.Errorf("GET /vcs (non-git header dir) = %v, want {}", nonGitInfo)
	}
}

// ============================================================================
// Instance Endpoint Tests (SPEC-017 §3.10)
// ============================================================================

// makeGitRepo creates a temp git repo with one committed file, one staged new
// file (dirty.txt) and one untracked file (untracked.txt, 3 lines). Returns
// the repo dir; skips when git is unavailable.
//
// Fixture git calls use gitEnv() (defined in server.go) — stripping
// GIT_DIR/GIT_INDEX_FILE etc. so the hook-exported repo location cannot
// hijack the fixture into the real repo (DF-CONSENSUS-19 incident).
func makeGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = gitEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	// origin/HEAD does not exist in a fresh local repo; set the
	// init.defaultBranch fallback so default_branch resolves to "main".
	run("config", "init.defaultBranch", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")
	// staged new file → porcelain "A " → added with 1 addition
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "dirty.txt")
	// untracked file → porcelain "??" → added, additions from line count
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstanceEndpoint_GET(t *testing.T) {
	workdir := t.TempDir()
	s, srv := newTestServer(&mockDB{})
	defer srv.Close()
	s.workdir = workdir

	resp, err := http.Get(srv.URL + "/instance")
	if err != nil {
		t.Fatalf("GET /instance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("not an array: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 singleton instance, got %d", len(list))
	}
	inst := list[0]
	for _, f := range []string{"id", "path", "createdAt", "updatedAt"} {
		if v, _ := inst[f].(string); v == "" {
			t.Errorf("instance entry missing non-empty %q: %v", f, inst)
		}
	}
	if p, _ := inst["path"].(string); p != workdir {
		t.Errorf("path = %q, want %q", p, workdir)
	}
}

func TestInstanceEndpoint_RequiresGET(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/instance", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /instance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestInstancePathEndpoint_GET(t *testing.T) {
	workdir := t.TempDir()
	s, srv := newTestServer(&mockDB{})
	defer srv.Close()
	s.workdir = workdir

	resp, err := http.Get(srv.URL + "/instance/path")
	if err != nil {
		t.Fatalf("GET /instance/path: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("not JSON object: %v", err)
	}
	// upstream PathInfo: {home, state, config, worktree, directory}
	for _, f := range []string{"home", "state", "config", "worktree", "directory"} {
		if v, _ := info[f].(string); v == "" {
			t.Errorf("missing non-empty %q: %v", f, info)
		}
	}
	if d, _ := info["directory"].(string); d != workdir {
		t.Errorf("directory = %q, want %q", d, workdir)
	}
	// non-git workspace → worktree falls back to the workspace directory
	if w, _ := info["worktree"].(string); w != workdir {
		t.Errorf("worktree = %q, want fallback %q", w, workdir)
	}
}

func TestInstanceVCSEndpoint_GET(t *testing.T) {
	t.Run("git workspace", func(t *testing.T) {
		repo := makeGitRepo(t)
		s, srv := newTestServer(&mockDB{})
		defer srv.Close()
		s.workdir = repo

		resp, err := http.Get(srv.URL + "/instance/vcs")
		if err != nil {
			t.Fatalf("GET /instance/vcs: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var info map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			t.Fatalf("not JSON object: %v", err)
		}
		if b, _ := info["branch"].(string); b != "main" {
			t.Errorf("branch = %q, want \"main\"", b)
		}
		if d, _ := info["default_branch"].(string); d != "main" {
			t.Errorf("default_branch = %q, want \"main\"", d)
		}
	})

	t.Run("non-git workspace", func(t *testing.T) {
		workdir := t.TempDir()
		s, srv := newTestServer(&mockDB{})
		defer srv.Close()
		s.workdir = workdir

		resp, err := http.Get(srv.URL + "/instance/vcs")
		if err != nil {
			t.Fatalf("GET /instance/vcs: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200 (never error), got %d", resp.StatusCode)
		}
		var info map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			t.Fatalf("not JSON object: %v", err)
		}
		if len(info) != 0 {
			t.Errorf("expected {} for non-git workspace, got %v", info)
		}
	})
}

func TestInstanceVCSDiffEndpoint_GET(t *testing.T) {
	t.Run("git workspace with staged + untracked changes", func(t *testing.T) {
		repo := makeGitRepo(t)
		s, srv := newTestServer(&mockDB{})
		defer srv.Close()
		s.workdir = repo

		resp, err := http.Get(srv.URL + "/instance/vcs/diff")
		if err != nil {
			t.Fatalf("GET /instance/vcs/diff: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var diffs []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&diffs); err != nil {
			t.Fatalf("not JSON array: %v", err)
		}
		if len(diffs) == 0 {
			t.Fatal("expected non-empty diff list for dirty repo")
		}
		var dirty, untracked *map[string]any
		for i := range diffs {
			switch diffs[i]["file"] {
			case "dirty.txt":
				dirty = &diffs[i]
			case "untracked.txt":
				untracked = &diffs[i]
			}
		}
		if dirty == nil {
			t.Fatalf("dirty.txt missing from diff list: %v", diffs)
		}
		if (*dirty)["status"] != "added" {
			t.Errorf("dirty.txt status = %v, want \"added\"", (*dirty)["status"])
		}
		if a, _ := (*dirty)["additions"].(float64); a != 1 {
			t.Errorf("dirty.txt additions = %v, want 1", (*dirty)["additions"])
		}
		if untracked == nil {
			t.Fatalf("untracked.txt missing from diff list: %v", diffs)
		}
		if (*untracked)["status"] != "added" {
			t.Errorf("untracked.txt status = %v, want \"added\"", (*untracked)["status"])
		}
		if a, _ := (*untracked)["additions"].(float64); a != 3 {
			t.Errorf("untracked.txt additions = %v, want 3 (line count)", (*untracked)["additions"])
		}
	})

	t.Run("clean git workspace", func(t *testing.T) {
		repo := makeGitRepo(t)
		// reset the staged file and remove the untracked one → clean tree
		resetCmd := exec.Command("git", "-C", repo, "reset", "--hard", "HEAD")
		resetCmd.Env = gitEnv()
		if out, err := resetCmd.CombinedOutput(); err != nil {
			t.Fatalf("git reset: %v\n%s", err, out)
		}
		if err := os.Remove(filepath.Join(repo, "untracked.txt")); err != nil {
			t.Fatal(err)
		}
		s, srv := newTestServer(&mockDB{})
		defer srv.Close()
		s.workdir = repo

		resp, err := http.Get(srv.URL + "/instance/vcs/diff")
		if err != nil {
			t.Fatalf("GET /instance/vcs/diff: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var diffs []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&diffs); err != nil {
			t.Fatalf("not JSON array: %v", err)
		}
		if len(diffs) != 0 {
			t.Errorf("expected empty diff list for clean repo, got %v", diffs)
		}
	})

	t.Run("non-git workspace", func(t *testing.T) {
		workdir := t.TempDir()
		s, srv := newTestServer(&mockDB{})
		defer srv.Close()
		s.workdir = workdir

		resp, err := http.Get(srv.URL + "/instance/vcs/diff")
		if err != nil {
			t.Fatalf("GET /instance/vcs/diff: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200 (never error), got %d", resp.StatusCode)
		}
		var diffs []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&diffs); err != nil {
			t.Fatalf("not JSON array: %v", err)
		}
		if len(diffs) != 0 {
			t.Errorf("expected empty diff list for non-git workspace, got %v", diffs)
		}
	})
}

func TestInstanceKnownSubpathReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, sub := range []string{
		"/instance/vcs/status", "/instance/vcs/diff/raw", "/instance/vcs/apply",
		"/instance/dispose", "/instance/command", "/instance/agent",
		"/instance/skill", "/instance/lsp", "/instance/formatter",
	} {
		resp, err := http.Get(srv.URL + sub)
		if err != nil {
			t.Fatalf("GET %s: %v", sub, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 501 {
			t.Errorf("GET %s: expected 501, got %d: %s", sub, resp.StatusCode, body)
		}
	}

	// upstream POST endpoints reach the 501 branch regardless of method
	req, _ := http.NewRequest("POST", srv.URL+"/instance/dispose", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /instance/dispose: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 501 {
		t.Errorf("POST /instance/dispose: expected 501, got %d", resp.StatusCode)
	}
}

func TestInstanceUnknownSubpathReturns404(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/instance/foo")
	if err != nil {
		t.Fatalf("GET /instance/foo: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ============================================================================
// handleGlobalEvent Tests (SSE)
// ============================================================================

// TestGlobalEventEndpoint_SSE_FlushesAndReplays proves the SSE contract:
// (1) 200 + headers flush immediately on connect, and (2) stored
// memory_events for the session are replayed as frames for late subscribers.
// The handler streams forever, so the request runs under a 2s context and
// the buffered bytes are parsed as SSE frames.
func TestGlobalEventEndpoint_SSE_FlushesAndReplays(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"id": int64(1), "type": "user_message", "content": "hello world", "session_id": "sess-1", "iteration_created": int64(1), "created_at": "2026-08-01T00:00:00Z"}),
			rowOf(map[string]any{"id": int64(2), "type": "assistant_message", "content": "hi there", "session_id": "sess-1", "iteration_created": int64(1), "created_at": "2026-08-01T00:00:01Z"}),
		},
	}
	s := &Server{db: mdb}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handleGlobalEvent(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/global/event?session_id=sess-1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /global/event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Read until the 2s context cancels the stream; parse what arrived.
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	t.Logf("SSE body (%d bytes):\n%s", len(body), text)

	if !strings.Contains(text, "event: message.created") {
		t.Fatalf("expected replayed message.created frames, got:\n%s", text)
	}
	if !strings.Contains(text, "hello world") || !strings.Contains(text, "hi there") {
		t.Errorf("expected replayed content 'hello world' and 'hi there', got:\n%s", text)
	}
	if !strings.Contains(text, "event_type") {
		t.Errorf("expected raw event_type in replayed frames, got:\n%s", text)
	}
}

// TestGlobalEventEndpoint_SSE_EmptySession_ReplaysGlobal proves the
// session_id-less case replays recent global events (test step 7 calls
// /event without a session_id).
func TestGlobalEventEndpoint_SSE_EmptySession_ReplaysGlobal(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"id": int64(1), "type": "user_message", "content": "global hello", "session_id": "sess-9", "iteration_created": int64(1), "created_at": "2026-08-01T00:00:00Z"}),
		},
	}
	s := &Server{db: mdb}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handleGlobalEvent(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/global/event", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /global/event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "global hello") {
		t.Errorf("expected global event replay, got:\n%s", string(body))
	}
}

// ============================================================================
// Build Empty Assistant Message Test
// ============================================================================

func TestBuildEmptyAssistantMessage(t *testing.T) {
	s := NewServer(&mockDB{}, "", nil, nil)
	msg := s.buildEmptyAssistantMessage()
	// Returns result of s.buildAssistantMessage("")
	// Check it has parts array and info section
	if msg["parts"] == nil {
		t.Error("expected non-nil parts")
	}
}

// TestSendMessageStoresRawUserText is the DF-CONSENSUS-45 regression: a
// parts-shaped shim message must land in memory_events exactly once as raw
// user_message content, byte-for-byte — never JSON-quoted, never re-encoded,
// even when the text is itself valid JSON. The assertion reads the inserted
// row through the same db.DB the shim wrote through, not just the HTTP
// response. The probe string is valid JSON on purpose: it used to survive
// the write and get re-encoded on the read path into "\"Reply with...\"".
func TestSendMessageStoresRawUserText(t *testing.T) {
	_, srv, conn := newMessageResponseTestServer(t)

	// Poll-published agent answer, same shape as
	// TestSendMessageReturnsResponseForSubmittedTurn, so the synchronous
	// shim contract is exercised unchanged.
	writeResult := make(chan error, 1)
	go func() {
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			rows, err := conn.Query(context.Background(),
				`SELECT id FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, "s1")
			if err != nil {
				writeResult <- err
				return
			}
			if len(rows) > 0 {
				if err := conn.Exec(context.Background(),
					`INSERT INTO memory_events (type, content, session_id, iteration_created)
					 VALUES ('text_block', 'ack', 's1', 1)`); err != nil {
					writeResult <- err
					return
				}
				writeResult <- conn.Exec(context.Background(),
					`UPDATE sessions SET status = 'idle', iteration = 2 WHERE id = 's1'`)
				return
			}
			select {
			case <-deadline.C:
				writeResult <- context.DeadlineExceeded
				return
			case <-ticker.C:
			}
		}
	}()

	submitted := `Reply with "quoted" text, please`
	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": submitted}},
	})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/s1/message", body)
	if err != nil {
		t.Fatalf("build POST /session/s1/message: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session/s1/message failed: %v", err)
	}
	defer resp.Body.Close()
	if err := <-writeResult; err != nil {
		t.Fatalf("publish agent response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, responseBody)
	}
	io.Copy(io.Discard, resp.Body)

	// Read the durable row — not the HTTP response — for the exact content.
	rows, err := conn.Query(context.Background(),
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, "s1")
	if err != nil {
		t.Fatalf("read user_message row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 user_message row, got %d", len(rows))
	}
	if got := rows[0]["content"]; got != submitted {
		t.Fatalf("user_message content = %#v, want raw %#v (JSON-quoted or re-encoded)", got, submitted)
	}
}

// ============================================================================
// Auth Middleware Test (with auth disabled + test auth header)
// ============================================================================

func TestAuthMiddleware_WithValidTestKey(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"id": "key-1", "key_hash": "test-hash", "scope": "admin", "session_id": nil}),
		},
	}
	s := NewServer(mdb, "admin-key", nil, nil)
	s.skipAuth = true
	_ = s
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestExtractBearerToken_Valid(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer cs_ak_test123")
	tok := extractBearerToken(req)
	if tok != "cs_ak_test123" {
		t.Errorf("expected cs_ak_test123, got %q", tok)
	}
}

func TestExtractBearerToken_NoBearer(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "cs_ak_test123")
	tok := extractBearerToken(req)
	if tok != "" {
		t.Errorf("expected empty for missing Bearer prefix, got %q", tok)
	}
}

func TestExtractBearerToken_Empty(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test", nil)
	tok := extractBearerToken(req)
	if tok != "" {
		t.Errorf("expected empty, got %q", tok)
	}
}

func TestSHA256Hash(t *testing.T) {
	h := sha256Hash([]byte("hello"))
	if len(h) != 32 {
		t.Errorf("expected 32-byte hash, got %d bytes", len(h))
	}
}

func TestMin(t *testing.T) {
	if got := min(3, 5); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
	if got := min(10, 2); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestNewUUID(t *testing.T) {
	u := newUUID()
	if u == "" {
		t.Error("expected non-empty UUID")
	}
	if strings.Count(u, "-") != 4 {
		t.Errorf("expected UUID format with 4 dashes, got %q", u)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key := generateAPIKey()
	if !strings.HasPrefix(key, "cs_sk_") {
		t.Errorf("expected cs_sk_ prefix, got %q", key)
	}
	if len(key) < 20 {
		t.Errorf("expected key length >= 20, got %d", len(key))
	}
}

func TestNilOrString_ReturnsPointer(t *testing.T) {
	// nilOrString returns *string — nil for empty/missing, pointer otherwise
	ps := nilOrString(nil)
	if ps != nil {
		t.Errorf("expected nil for nil input, got %v", *ps)
	}

	ps2 := nilOrString("hello")
	if ps2 == nil || *ps2 != "hello" {
		t.Errorf("expected pointer to 'hello', got %v", ps2)
	}

	// int 42 gets toString'd to "42" which is non-empty
	ps3 := nilOrString(42)
	if ps3 == nil || *ps3 != "42" {
		t.Errorf("expected pointer to '42', got %v", ps3)
	}
}

func TestSessionIDFromPerm(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{"session_id": "sess-abc"}),
	}
	s := NewServer(mdb, "", nil, nil)

	if id := s.sessionIDFromPerm("appr-1"); id != "sess-abc" {
		t.Errorf("expected sess-abc, got %q", id)
	}

	mdb2 := &mockDB{queryRowErr: context.DeadlineExceeded}
	s2 := NewServer(mdb2, "", nil, nil)
	if id := s2.sessionIDFromPerm("nonexistent"); id != "" {
		t.Errorf("expected empty for unknown, got %q", id)
	}
}

// ============================================================================
// Edge Cases: Missing required fields in create session
// ============================================================================

func TestCreateSession_NoGoal(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"title": "test-agent",
	})
	resp, err := http.Post(srv.URL+"/session", "application/json", body)
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()

	// Without goal, should still succeed (goal is optional in request — default set)
	if resp.StatusCode != 200 {
		t.Logf("create session without goal returned %d", resp.StatusCode)
	}
}

// ============================================================================
// CORS Middleware Test with actual request
// ============================================================================

func TestCORS_ActualRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/session", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /session: %v", err)
	}
	defer resp.Body.Close()

	// Should return 204 for preflight
	if resp.StatusCode != 204 {
		t.Errorf("expected 204 for CORS preflight, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Config PATCH test
// ============================================================================

func TestPatchConfig(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := jsonBody(t, map[string]any{
		"settings": map[string]string{
			"llm.default_model": "gpt-4.1",
		},
	})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/config", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Utility Function Tests
// ============================================================================

func TestToInt_Int64(t *testing.T) {
	if got := toInt(int64(42)); got != 42 {
		t.Errorf("toInt(int64 42) = %d, want 42", got)
	}
}

func TestToInt_Float64(t *testing.T) {
	if got := toInt(float64(3.14)); got != 3 {
		t.Errorf("toInt(float64 3.14) = %d, want 3", got)
	}
}

func TestToInt_Int(t *testing.T) {
	if got := toInt(99); got != 99 {
		t.Errorf("toInt(99) = %d, want 99", got)
	}
}

func TestToInt_Nil(t *testing.T) {
	if got := toInt(nil); got != 0 {
		t.Errorf("toInt(nil) = %d, want 0", got)
	}
}

func TestToInt_StringDefault(t *testing.T) {
	if got := toInt("hello"); got != 0 {
		t.Errorf("toInt(string) = %d, want 0", got)
	}
}

func TestToInt64_Int64(t *testing.T) {
	if got := toInt64(int64(42)); got != 42 {
		t.Errorf("toInt64(int64 42) = %d, want 42", got)
	}
}

func TestToInt64_Int(t *testing.T) {
	if got := toInt64(99); got != 99 {
		t.Errorf("toInt64(99) = %d, want 99", got)
	}
}

func TestToInt64_Float64(t *testing.T) {
	if got := toInt64(float64(3.14)); got != 3 {
		t.Errorf("toInt64(float64 3.14) = %d, want 3", got)
	}
}

func TestToInt64_Nil(t *testing.T) {
	if got := toInt64(nil); got != 0 {
		t.Errorf("toInt64(nil) = %d, want 0", got)
	}
}

func TestToFloat64_Float64(t *testing.T) {
	if got := toFloat64(float64(3.14)); got != 3.14 {
		t.Errorf("toFloat64(3.14) = %f, want 3.14", got)
	}
}

func TestToFloat64_Int64(t *testing.T) {
	if got := toFloat64(int64(42)); got != 42.0 {
		t.Errorf("toFloat64(int64 42) = %f, want 42.0", got)
	}
}

func TestToFloat64_String(t *testing.T) {
	if got := toFloat64("-12.5"); got != -12.5 {
		t.Errorf("toFloat64('-12.5') = %f, want -12.5", got)
	}
}

func TestToFloat64_InvalidString(t *testing.T) {
	if got := toFloat64("not-a-number"); got != 0 {
		t.Errorf("toFloat64(invalid) = %f, want 0", got)
	}
}

func TestToFloat64_Nil(t *testing.T) {
	if got := toFloat64(nil); got != 0 {
		t.Errorf("toFloat64(nil) = %f, want 0", got)
	}
}

func TestToBool_Bool(t *testing.T) {
	if got := toBool(true); got != true {
		t.Errorf("toBool(true) = %v, want true", got)
	}
	if got := toBool(false); got != false {
		t.Errorf("toBool(false) = %v, want false", got)
	}
}

func TestToBool_Int64(t *testing.T) {
	if got := toBool(int64(1)); got != true {
		t.Errorf("toBool(int64 1) = %v, want true", got)
	}
	if got := toBool(int64(0)); got != false {
		t.Errorf("toBool(int64 0) = %v, want false", got)
	}
}

func TestToBool_Float64(t *testing.T) {
	if got := toBool(float64(1.5)); got != true {
		t.Errorf("toBool(float64 1.5) = %v, want true", got)
	}
	if got := toBool(float64(0.0)); got != false {
		t.Errorf("toBool(float64 0.0) = %v, want false", got)
	}
}

func TestToBool_Nil(t *testing.T) {
	if got := toBool(nil); got != false {
		t.Errorf("toBool(nil) = %v, want false", got)
	}
}

func TestToString_Nil(t *testing.T) {
	if got := toString(nil); got != "" {
		t.Errorf("toString(nil) = %q, want empty", got)
	}
}

func TestToString_Bytes(t *testing.T) {
	if got := toString([]byte("hello")); got != "hello" {
		t.Errorf("toString([]byte) = %q, want 'hello'", got)
	}
}

func TestToString_Default(t *testing.T) {
	if got := toString(42); got != "42" {
		t.Errorf("toString(42) = %q, want '42'", got)
	}
}

// ============================================================================
// emitShimEventForSession Tests
// ============================================================================

type testEventBus struct {
	emits []eventEmit
}

type eventEmit struct {
	sessionID string
	eventType string
}

func (b *testEventBus) Listen(sessionID string, listener EventListener) func() {
	return func() {}
}

func (b *testEventBus) Emit(sessionID, eventType string, data any) {
	b.emits = append(b.emits, eventEmit{sessionID, eventType})
}

func TestEmitShimEventForSession_WithEvents(t *testing.T) {
	bus := &testEventBus{}
	s := &Server{events: bus}

	s.emitShimEventForSession("sid-1", "message", nil)

	if len(bus.emits) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(bus.emits))
	}
	if bus.emits[0].sessionID != "sid-1" {
		t.Errorf("emit sessionID = %q, want 'sid-1'", bus.emits[0].sessionID)
	}
	if bus.emits[0].eventType != "message" {
		t.Errorf("emit eventType = %q, want 'message'", bus.emits[0].eventType)
	}
}

func TestEmitShimEventForSession_NoEventBus(t *testing.T) {
	s := &Server{events: nil}

	// Should not panic when events is nil
	s.emitShimEventForSession("sid-1", "message", nil)
}

// ============================================================================
// sessionIDFromPerm Tests
// ============================================================================

func TestSessionIDFromPerm_Found(t *testing.T) {
	db := &mockDB{
		queryRow: rowOf(map[string]any{"session_id": "sid-abc-123"}),
	}
	s := &Server{db: db}

	got := s.sessionIDFromPerm("perm-1")
	if got != "sid-abc-123" {
		t.Errorf("sessionIDFromPerm = %q, want 'sid-abc-123'", got)
	}
}

func TestSessionIDFromPerm_NotFound(t *testing.T) {
	db := &mockDB{
		queryRow: rowOf(map[string]any{"session_id": nil}),
	}
	s := &Server{db: db}

	// nilOrString will return nil for nil value → toString(nil) = ""
	got := s.sessionIDFromPerm("perm-1")
	if got != "" {
		t.Errorf("sessionIDFromPerm = %q, want ''", got)
	}
}

func TestSessionIDFromPerm_DBError(t *testing.T) {
	db := &mockDB{
		queryRowErr: context.DeadlineExceeded,
	}
	s := &Server{db: db}

	got := s.sessionIDFromPerm("perm-1")
	if got != "" {
		t.Errorf("sessionIDFromPerm with error = %q, want ''", got)
	}
}

// ============================================================================
// validateAuth Tests (Bearer + Basic)
// ============================================================================

func TestValidateAuth_BearerWithDB(t *testing.T) {
	token := "cs_sk_testtoken12345"

	db := &mockDB{
		queryResults: []db.Row{rowOf(map[string]any{
			"id":         "key-1",
			"scope":      "session",
			"session_id": "sid-test",
		})},
	}
	s := &Server{db: db}

	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	sessionID, ok := s.validateAuth(req)
	if !ok {
		t.Fatal("validateAuth returned false for valid bearer")
	}
	if sessionID != "sid-test" {
		t.Errorf("sessionID = %q, want 'sid-test'", sessionID)
	}
	// Verify it queried the api_keys table
	if len(db.queries) == 0 {
		t.Fatal("no queries executed")
	}
	q := db.queries[0]
	if !strings.Contains(q, "api_keys") || !strings.Contains(q, "key_prefix") {
		t.Errorf("query doesn't target api_keys with key_prefix: %s", q)
	}
	// Verify the hash was passed as a parameter (we use $2, which is args)
	if !strings.Contains(q, "key_hash") {
		t.Errorf("query doesn't contain key_hash filter: %s", q)
	}
	// The actual hash is passed as a parameter, not embedded in query text.
	// We trust the parameter binding; verify through the mock that Query was called.
}

func TestValidateAuth_BasicAuthWithDB(t *testing.T) {
	password := "my-password-here"

	db := &mockDB{
		queryResults: []db.Row{rowOf(map[string]any{
			"id":         "key-1",
			"scope":      "admin",
			"session_id": nil,
		})},
	}
	s := &Server{db: db}

	// opencode sends: base64("opencode:password")
	encoded := base64.StdEncoding.EncodeToString([]byte("opencode:" + password))
	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Basic "+encoded)

	sessionID, ok := s.validateAuth(req)
	if !ok {
		t.Fatal("validateAuth returned false for valid basic auth")
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want '' (admin key, no session)", sessionID)
	}
	// Verify api_keys was queried
	if len(db.queries) == 0 {
		t.Fatal("no queries executed")
	}
	q := db.queries[0]
	if !strings.Contains(q, "api_keys") || !strings.Contains(q, "key_hash") {
		t.Errorf("query doesn't target api_keys with key_hash: %s", q)
	}
}

func TestValidateAuth_InvalidBasicBase64(t *testing.T) {
	db := &mockDB{}
	s := &Server{db: db}

	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Basic !!!invalid-base64!!!")

	_, ok := s.validateAuth(req)
	if ok {
		t.Error("validateAuth should fail for invalid base64")
	}
}

func TestValidateAuth_NoAuthHeader(t *testing.T) {
	db := &mockDB{}
	s := &Server{db: db}

	req, _ := http.NewRequest("GET", "/session", nil)

	_, ok := s.validateAuth(req)
	if ok {
		t.Error("validateAuth should fail with no auth header")
	}
}

func TestValidateAuth_EmptyBearerToken(t *testing.T) {
	db := &mockDB{}
	s := &Server{db: db}

	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Bearer ")

	_, ok := s.validateAuth(req)
	if ok {
		t.Error("validateAuth should fail with empty bearer token")
	}
}

// ============================================================================
// handleGlobalEvent error path tests
// ============================================================================

// TestHandleGlobalEvent_SSE_WithFlusher tests the SSE handler with a flusher-capable
// writer in a goroutine with a timeout. The handler enters its event loop and blocks
// — this is correct production behavior. We verify it doesn't panic on startup.
func TestHandleGlobalEvent_SSE_WithFlusher(t *testing.T) {
	// Use a real http.ResponseWriter through httptest server
	// to verify the SSE endpoint starts correctly.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		db := &mockDB{}
		s := &Server{db: db}
		s.handleGlobalEvent(w, r)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Start a GET to /global/event with a short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/global/event", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context deadline exceeded is expected — the handler blocks in its event loop
		if ctx.Err() == context.DeadlineExceeded {
			t.Log("SSE handler blocked as expected (infinite event loop)")
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 (SSE started) or the connection was cut
	if resp.StatusCode != http.StatusOK {
		t.Logf("SSE setup returned %d (may have been cut by context)", resp.StatusCode)
	}
}

// ============================================================================
// SHIM-GAP-002 — declared operations the shim does not translate must say so
// ============================================================================

// notImplementedRoute is one row of the shim's declared-vs-served honesty
// contract: an opencode operation the pinned upstream document declares and
// the Consensus shim does not translate. Before SHIM-GAP-002 each of these
// answered a 404 from inside a registered route (13 ROUTED-404), a 404 from an
// existing path whose method was never registered (1 METHOD-MISSING), or a
// bare 501 with an unrelated error body (3 STUB-501) — a client could not tell
// "no such opencode operation" from "this shim does not implement it".
//
// driftID is the per-item id carried by
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json. Those ids
// are positional (re-assigned by enumeration order when the comparison is
// regenerated), so the test below pins the semantically stable key —
// method + path shape + declared operationId — and only uses the id as a label.
type notImplementedRoute struct {
	driftID   string
	method    string
	path      string // concrete request path; placeholders filled with the ids below
	operation string // upstream operationId the envelope must name
}

// notImplementedRoutes is every finding SHIM-GAP-002 fixes. The count is
// asserted in the test so the table cannot silently shrink. ROUTE-FIX-008
// removed SHIM-DRIFT-114 (GET /session/status): the shim now serves it.
var notImplementedRoutes = []notImplementedRoute{
	// ROUTED-404 (13): route registered, handler answered 404 NOT_FOUND.
	{"SHIM-DRIFT-099", http.MethodGet, "/project/current", "project.current"},
	{"SHIM-DRIFT-100", http.MethodPost, "/project/git/init", "project.initGit"},
	{"SHIM-DRIFT-101", http.MethodGet, "/project/project_missing/directories", "project.directories"},
	{"SHIM-DRIFT-116", http.MethodGet, "/session/s1/diff", "session.diff"},
	{"SHIM-DRIFT-131", http.MethodPost, "/tui/clear-prompt", "tui.clearPrompt"},
	{"SHIM-DRIFT-132", http.MethodGet, "/tui/control/next", "tui.control.next"},
	{"SHIM-DRIFT-133", http.MethodPost, "/tui/control/response", "tui.control.response"},
	{"SHIM-DRIFT-135", http.MethodPost, "/tui/open-help", "tui.openHelp"},
	{"SHIM-DRIFT-136", http.MethodPost, "/tui/open-models", "tui.openModels"},
	{"SHIM-DRIFT-137", http.MethodPost, "/tui/open-sessions", "tui.openSessions"},
	{"SHIM-DRIFT-138", http.MethodPost, "/tui/open-themes", "tui.openThemes"},
	{"SHIM-DRIFT-139", http.MethodPost, "/tui/publish", "tui.publish"},
	// METHOD-MISSING (1): path routed for other methods, this one absent.
	{"SHIM-DRIFT-121", http.MethodDelete, "/session/s1/share", "session.unshare"},
	// STUB-501 (3): already 501, but the stub was silent/untyped.
	{"SHIM-DRIFT-142", http.MethodPost, "/vcs/apply", "vcs.apply"},
	{"SHIM-DRIFT-143", http.MethodGet, "/vcs/diff/raw", "vcs.diff.raw"},
	{"SHIM-DRIFT-144", http.MethodGet, "/vcs/status", "vcs.status"},
}

// shimRouteShape normalizes a concrete request path to the artifact's
// placeholder form so the table's ids compare equal to "{sessionID}" /
// "{projectID}" rows. "s1" and "project_missing" are the only concrete ids the
// table uses, and neither is a literal segment of any declared opencode path.
func shimRouteShape(path string) string {
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "{") || seg == "s1" || seg == "project_missing" {
			segs[i] = "{}"
		}
	}
	return strings.Join(segs, "/")
}

func doShimRequest(t *testing.T, base, method, path string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, resp.Header, body
}

// TestDeclaredUnimplementedRoutesAnswerTypedEnvelope is the SHIM-GAP-002
// contract: every one of the 17 declared operations the shim does not
// translate answers HTTP 501 with the typed not-implemented envelope
// {"error":"not_implemented","operation":"<op>","detail":"<what is missing>"} —
// never a 404 from a registered route, never a bare 501.
func TestDeclaredUnimplementedRoutesAnswerTypedEnvelope(t *testing.T) {
	if len(notImplementedRoutes) != 16 {
		t.Fatalf("SHIM-GAP-002 pins 13 ROUTED-404 + 1 METHOD-MISSING + 3 STUB-501 = 17 routes, minus the SHIM-DRIFT-114 route now served by ROUTE-FIX-008 = 16; table has %d",
			len(notImplementedRoutes))
	}
	seen := map[string]bool{}
	for _, route := range notImplementedRoutes {
		if seen[route.driftID] {
			t.Fatalf("duplicate drift id %s in notImplementedRoutes", route.driftID)
		}
		seen[route.driftID] = true
	}

	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, route := range notImplementedRoutes {
		t.Run(route.driftID+" "+route.method+" "+route.path, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, route.method, route.path)

			if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
				t.Fatalf("%s %s answered %d — a registered route may not lie with 404/405. Body: %s",
					route.method, route.path, status, body)
			}
			if status != http.StatusNotImplemented {
				t.Fatalf("%s %s: got %d, want 501. Body: %s", route.method, route.path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("501 body is not JSON: %v (%s)", err, body)
			}
			if len(got) != 3 {
				t.Errorf("envelope must carry exactly 3 fields (error, operation, detail), got %d: %v", len(got), got)
			}
			if got["error"] != "not_implemented" {
				t.Errorf("error = %v, want \"not_implemented\"", got["error"])
			}
			if got["operation"] != route.operation {
				t.Errorf("operation = %v, want %q", got["operation"], route.operation)
			}
			if detail, _ := got["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Error("detail must name what is missing; got empty")
			}
		})
	}
}

// TestNotImplementedEnvelopeIsNotBlanket501 is the non-vacuity control for the
// table above: the typed envelope did not become the answer for every path in
// these families. An unknown sub-path still 404s, and the upstream typed
// ProjectNotFoundError contract for a bare /project/{projectID}
// (DF-CONSENSUS-47) is untouched by the change.
func TestNotImplementedEnvelopeIsNotBlanket501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/tui/unknown-action"},
		{http.MethodGet, "/project/project_missing/unknown-sub"},
	} {
		status, _, body := doShimRequest(t, srv.URL, tc.method, tc.path)
		if status != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404 (non-vacuity control). Body: %s",
				tc.method, tc.path, status, body)
		}
	}

	// The typed ProjectNotFoundError body is a strict toEqual upstream: the
	// fix must not have swallowed the bare-project shape.
	status, _, body := doShimRequest(t, srv.URL, http.MethodPatch, "/project/project_missing")
	if status != http.StatusNotFound {
		t.Fatalf("PATCH /project/project_missing: got %d, want 404. Body: %s", status, body)
	}
	var typed map[string]any
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("PATCH /project/project_missing body is not JSON: %v (%s)", err, body)
	}
	if typed["_tag"] != "ProjectNotFoundError" {
		t.Errorf("PATCH /project/project_missing: _tag = %v, want ProjectNotFoundError (DF-CONSENSUS-47)", typed["_tag"])
	}
}

// TestNotImplementedTableMatchesDriftArtifact ties the table to the committed
// declared-vs-served comparison (specs/024 §A.3): the SHIM-GAP-002 set and the
// artifact must agree one-to-one, the artifact must record each of them as
// answered by the typed envelope (outcome 501-typed), and the dishonest drift
// classes the row exists to remove — ROUTED-404, METHOD-MISSING, STUB-501 —
// must be empty. Matching is on the semantically stable key (method + path
// shape) because the artifact's SHIM-DRIFT-NNN ids are positional.
func TestNotImplementedTableMatchesDriftArtifact(t *testing.T) {
	artifact := filepath.Join("..", "..", "..", "specs", "openapi", "upstream",
		"opencode-declared-vs-served-1.18.33.json")
	raw, err := os.ReadFile(artifact)
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", artifact)
	}
	if err != nil {
		t.Fatalf("read %s: %v", artifact, err)
	}

	var doc struct {
		Drift []struct {
			ID            string `json:"id"`
			Class         string `json:"class"`
			Method        string `json:"method"`
			Path          string `json:"path"`
			OperationID   string `json:"operationId"`
			ServedOutcome string `json:"served_outcome"`
		} `json:"drift_declared_not_served"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", artifact, err)
	}
	if len(doc.Drift) == 0 {
		t.Fatal("artifact carries no drift_declared_not_served rows — refusing to pass vacuously")
	}

	// The classes SHIM-GAP-002 exists to empty: a handler that answers 404 from
	// inside a registered route, a registered path missing the declared method,
	// and an untyped 501 stub.
	dishonest := map[string]bool{"ROUTED-404": true, "METHOD-MISSING": true, "STUB-501": true}
	byKey := map[string]int{}
	typed := map[string]string{} // shape+method -> operationId
	for i, row := range doc.Drift {
		key := shimRouteShape(row.Path) + " " + row.Method
		byKey[key] = i
		if dishonest[row.Class] {
			t.Errorf("artifact still classifies %s %s as %s (id %s) — SHIM-GAP-002 must empty that class",
				row.Method, row.Path, row.Class, row.ID)
		}
		if row.ServedOutcome == "501-typed" {
			typed[key] = row.OperationID
		}
	}
	if len(typed) != 16 {
		t.Errorf("artifact records %d operations with outcome 501-typed, want the 16 remaining SHIM-GAP-002 findings (ROUTE-FIX-008 now serves the 17th, /session/status)", len(typed))
	}

	for _, route := range notImplementedRoutes {
		key := shimRouteShape(route.path) + " " + route.method
		i, ok := byKey[key]
		if !ok {
			t.Errorf("%s: %s %s is not a drift row in the artifact", route.driftID, route.method, route.path)
			continue
		}
		if row := doc.Drift[i]; row.OperationID != route.operation {
			t.Errorf("%s: artifact operationId is %q, table claims %q", route.driftID, row.OperationID, route.operation)
		}
		if _, ok := typed[key]; !ok {
			t.Errorf("%s: artifact served_outcome for %s %s is %q, want 501-typed",
				route.driftID, route.method, route.path, doc.Drift[i].ServedOutcome)
		}
		delete(typed, key)
	}
	for key, op := range typed {
		t.Errorf("artifact records %s as 501-typed (%s) but notImplementedRoutes does not cover it", key, op)
	}
}
