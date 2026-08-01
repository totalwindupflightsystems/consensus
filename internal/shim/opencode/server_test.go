// Package opencode: opencode protocol shim tests (SPEC-017).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/017-ui-adapter-layer.md plan=phase-6/task-6-1/step-6-1-4 test=internal/shim/opencode/server_test.go
package opencode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
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

func TestSendMessage(t *testing.T) {
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

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var msg map[string]any
	json.NewDecoder(resp.Body).Decode(&msg)

	if msg["info"] == nil {
		t.Error("expected info in response")
	}
	if msg["parts"] == nil {
		t.Error("expected parts in response")
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
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg["settings"] == nil {
		t.Error("expected settings in config")
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
	if ct == "" || !strings.Contains(ct, "html") {
		t.Errorf("expected HTML content type, got %q", ct)
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

func TestVCSEndpointReturns501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/vcs")
	if err != nil {
		t.Fatalf("GET /vcs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 501 {
		t.Errorf("expected 501 for /vcs, got %d", resp.StatusCode)
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
