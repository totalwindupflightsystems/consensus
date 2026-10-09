// Package api: integration tests for session endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-1/step-2-1-2 test=internal/api/sessions_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/hitl"
)

// ============================================================================
// Integration Test Server Setup
// ============================================================================

type integrationServer struct {
	*Server
	conn     db.DB
	adminKey string // valid admin API key for tests
}

func newIntegrationServer(t *testing.T) *integrationServer {
	t.Helper()

	ctx := context.Background()
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://file:api-" + dbName + "?mode=memory&cache=shared"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Run migration
	if err := runIntegrationMigration(ctx, conn); err != nil {
		_ = conn.Close()
		t.Fatalf("migration: %v", err)
	}

	// Seed model_registry (required FK)
	if err := conn.Exec(ctx, `INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out) VALUES ('gpt-4o', 1, 128000, 2.50, 10.00)`); err != nil {
		_ = conn.Close()
		t.Fatalf("seed model: %v", err)
	}

	// Create an admin API key
	adminKey := "cs_ak_admin_test_1234567890_abcdef"
	hash := sha256Hash(adminKey)
	prefix := "cs_ak_ad"

	if err := conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-admin-1', $1, $2, 'admin', datetime('now'))`, hash, prefix); err != nil {
		_ = conn.Close()
		t.Fatalf("create admin key: %v", err)
	}

	srv := NewServer(ServerConfig{
		Addr: ":0",
		DB:   conn,
		HITL: hitl.New(conn),
	})
	// Initialize default HITL config (idempotent)
	_ = hitl.New(conn).SetConfiguration(ctx, hitl.DefaultConfiguration())
	return &integrationServer{Server: srv, conn: conn, adminKey: adminKey}
}

func (is *integrationServer) close() {
	_ = is.conn.Close()
}

func runIntegrationMigration(ctx context.Context, conn db.DB) error {
	// Read migration from harness testdata
	path := "../harness/testdata/migration_test.sql"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	statements := splitMigrationSQL(string(data))
	for _, stmt := range statements {
		if err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	// Keep the API integration fixture aligned with migration 025 without
	// coupling every handler test to the full production migration ladder.
	if err := conn.Exec(ctx, `CREATE TABLE idempotency_keys (
		key TEXT NOT NULL,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		response_message_id INTEGER REFERENCES memory_events(id),
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}
	return conn.Exec(ctx, `CREATE UNIQUE INDEX uq_idempotency_keys_session_key
		ON idempotency_keys(session_id, key)`)
}

func splitMigrationSQL(sqlText string) []string {
	var result []string
	var current strings.Builder
	lines := strings.Split(sqlText, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(current.String())
			stmt = strings.TrimSuffix(stmt, ";")
			if stmt != "" {
				result = append(result, stmt)
			}
			current.Reset()
		}
	}
	return result
}

// ============================================================================
// Create Session Tests
// ============================================================================

func TestCreateSession_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"agent_name":"research-agent","goal":"Analyze Q4 revenue data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if resp.Status != "booting" {
		t.Errorf("expected status 'booting', got %q", resp.Status)
	}
	if resp.APIKey == "" {
		t.Error("expected non-empty API key")
	}
	if !strings.HasPrefix(resp.APIKey, "cs_sk_") {
		t.Errorf("expected API key prefix 'cs_sk_', got %q", resp.APIKey[:min(6, len(resp.APIKey))])
	}
	if resp.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}

	// Verify session exists in DB
	ctx := context.Background()
	rows, err := srv.conn.Query(ctx, `SELECT agent_name, model_id, status, goal FROM sessions WHERE id = $1`, resp.ID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("session not found in DB: %v", err)
	}
	if toString(rows[0]["agent_name"]) != "research-agent" {
		t.Errorf("expected agent_name 'research-agent', got %q", toString(rows[0]["agent_name"]))
	}
	if toString(rows[0]["status"]) != "booting" {
		t.Errorf("expected status 'booting', got %q", toString(rows[0]["status"]))
	}
	if toString(rows[0]["model_id"]) != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", toString(rows[0]["model_id"]))
	}

	// Verify API key exists in DB
	apiRows, err := srv.conn.Query(ctx, `SELECT scope, session_id FROM api_keys WHERE session_id = $1`, resp.ID)
	if err != nil || len(apiRows) == 0 {
		t.Fatalf("API key not found in DB: %v", err)
	}
	if toString(apiRows[0]["scope"]) != "session" {
		t.Errorf("expected scope 'session', got %q", toString(apiRows[0]["scope"]))
	}
}

func TestCreateSession_WithSpecificModel(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"agent_name":"coder","goal":"Write tests","model_id":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify model stored correctly
	var resp CreateSessionResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	ctx := context.Background()
	rows, _ := srv.conn.Query(ctx, `SELECT model_id FROM sessions WHERE id = $1`, resp.ID)
	if toString(rows[0]["model_id"]) != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", toString(rows[0]["model_id"]))
	}
}

func TestCreateSession_MissingRequiredFields(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	tests := []struct {
		name string
		body string
	}{
		{"no agent_name", `{"goal":"Do something"}`},
		{"no goal", `{"agent_name":"test"}`},
		{"empty object", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+srv.adminKey)
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// DF-CONSENSUS-50: when no LLM API key is configured, session creation must
// fail fast with an actionable 400 instead of returning 201 and letting the
// session die opaquely at its first LLM dispatch. With the gate unset (nil,
// the pre-existing wiring), creation keeps succeeding — the legacy behavior
// existing tests and shims rely on.
func TestCreateSession_MissingLLMKey(t *testing.T) {
	body := `{"agent_name":"probe-agent","goal":"DF-CONSENSUS-50"}`

	t.Run("gate_on_returns_400_with_guidance", func(t *testing.T) {
		srv := newIntegrationServer(t)
		defer srv.close()
		srv.requireLLMKey = func() bool { return false } // no LLM API key configured

		req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+srv.adminKey)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json Content-Type, got %q", ct)
		}
		if w.Body.Len() == 0 {
			t.Fatal("expected non-empty JSON error body")
		}

		var resp ErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error body %q: %v", w.Body.String(), err)
		}
		if resp.Error.Code != "MISSING_LLM_CONFIG" {
			t.Errorf("expected code MISSING_LLM_CONFIG, got %q", resp.Error.Code)
		}
		for _, want := range []string{"DEEPSEEK_API_KEY", "CONSENSUS_API_KEY", "llm.api_key"} {
			if !strings.Contains(resp.Error.Message, want) {
				t.Errorf("message %q does not name %q", resp.Error.Message, want)
			}
		}

		// Fail fast: nothing must have been persisted.
		ctx := context.Background()
		rows, _ := srv.conn.Query(ctx, `SELECT id FROM sessions WHERE agent_name = 'probe-agent'`)
		if len(rows) != 0 {
			t.Errorf("expected no session row, found %d", len(rows))
		}
	})

	t.Run("gate_off_keeps_legacy_201", func(t *testing.T) {
		srv := newIntegrationServer(t)
		defer srv.close()
		// requireLLMKey left nil — the pre-existing wiring.

		req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+srv.adminKey)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected legacy 201 with gate unset, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestCreateSession_NonAdminKey(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	// Create a session so the key has a valid session_id to reference
	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('test-session-id', 'existing', 'gpt-4o', 'idle', 'Existing', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_session_test_key_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-sess-1', $1, $2, 'session', 'test-session-id', datetime('now'))`, hash, prefix)

	body := `{"agent_name":"hacker","goal":"steal data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// List Sessions Tests
// ============================================================================

func TestListSessions_Admin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	// Seed two sessions
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-1', 'agent-a', 'gpt-4o', 'idle', 'Goal A', datetime('now'), datetime('now'))`)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-2', 'agent-b', 'gpt-4o', 'thinking', 'Goal B', datetime('now'), datetime('now'))`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessions []SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) < 2 {
		t.Errorf("expected at least 2 sessions, got %d", len(sessions))
	}
}

func TestListSessions_WithStatusFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-1', 'a', 'gpt-4o', 'idle', 'Goal', datetime('now'), datetime('now'))`)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-2', 'b', 'gpt-4o', 'thinking', 'Goal', datetime('now'), datetime('now'))`)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-3', 'c', 'gpt-4o', 'failed', 'Goal', datetime('now'), datetime('now'))`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?status=idle,thinking", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessions []SessionResponse
	_ = json.NewDecoder(w.Body).Decode(&sessions)

	for _, s := range sessions {
		if s.Status != "idle" && s.Status != "thinking" {
			t.Errorf("unexpected status %q in filtered results", s.Status)
		}
	}
}

func TestListSessions_SessionScoped(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	// Create session + key
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'my-agent', 'gpt-4o', 'idle', 'My goal', datetime('now'), datetime('now'))`)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-other', 'other-agent', 'gpt-4o', 'thinking', 'Other goal', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_mine_test_key_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-mine', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessions []SessionResponse
	_ = json.NewDecoder(w.Body).Decode(&sessions)

	// Session-scoped key should only see own session
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions) > 0 && sessions[0].ID != "sess-mine" {
		t.Errorf("expected sess-mine, got %q", sessions[0].ID)
	}
}

// ============================================================================
// Get Session Tests
// ============================================================================

func TestGetSession_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := "2026-05-04T00:00:00Z"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, context_budget, iteration, created_at, heartbeat_at) VALUES ('sess-1', 'test', 'gpt-4o', 'idle', 'My goal', 64000, 3, $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-1", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.ID != "sess-1" {
		t.Errorf("expected sess-1, got %q", resp.ID)
	}
	if resp.AgentName != "test" {
		t.Errorf("expected agent 'test', got %q", resp.AgentName)
	}
	if resp.ContextBudget != 64000 {
		t.Errorf("expected budget 64000, got %d", resp.ContextBudget)
	}
	if resp.Iteration != 3 {
		t.Errorf("expected iteration 3, got %d", resp.Iteration)
	}
	if resp.Goal == nil || *resp.Goal != "My goal" {
		t.Errorf("expected goal 'My goal', got %v", resp.Goal)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/00000000-0000-4000-8000-000000000000", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetSession_InvalidUUID(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/00000000-0000-0000-0000-gggggggggggg", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSession_SessionScoped_CanAccessOwn(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'mine', 'gpt-4o', 'idle', 'Mine', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_own_test_key_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-own', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-mine", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSession_SessionScoped_CannotAccessOther(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'mine', 'gpt-4o', 'idle', 'Mine', datetime('now'), datetime('now'))`)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-other', 'other', 'gpt-4o', 'idle', 'Other', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_mine_test_xyz_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-mine2', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-other", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Update Session Tests (pause, resume, cancel)
// ============================================================================

func TestUpdateSession_Pause(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-pause', 'test', 'gpt-4o', 'thinking', 'Goal', datetime('now'), datetime('now'))`)

	body := `{"status":"pause"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-pause", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify status in DB
	rows, _ := srv.conn.Query(ctx, `SELECT status FROM sessions WHERE id = 'sess-pause'`)
	if toString(rows[0]["status"]) != "paused" {
		t.Errorf("expected 'paused', got %q", toString(rows[0]["status"]))
	}
}

func TestUpdateSession_Resume(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-resume', 'test', 'gpt-4o', 'paused', 'Goal', datetime('now'), datetime('now'))`)

	body := `{"status":"resume"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-resume", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	rows, _ := srv.conn.Query(ctx, `SELECT status FROM sessions WHERE id = 'sess-resume'`)
	if toString(rows[0]["status"]) != "idle" {
		t.Errorf("expected 'idle', got %q", toString(rows[0]["status"]))
	}
}

func TestUpdateSession_ResumeFailed(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, completed_at, created_at, heartbeat_at) VALUES ('sess-recover', 'test', 'gpt-4o', 'failed', 'Goal', datetime('now'), datetime('now'), datetime('now'))`)

	body := `{"status":"resume"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-recover", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	row, err := srv.conn.QueryRow(ctx, `SELECT status, completed_at FROM sessions WHERE id = 'sess-recover'`)
	if err != nil {
		t.Fatalf("query resumed session: %v", err)
	}
	if got := toString(row["status"]); got != "idle" {
		t.Errorf("expected 'idle', got %q", got)
	}
	if row["completed_at"] != nil {
		t.Errorf("expected completed_at cleared, got %v", row["completed_at"])
	}
}

func TestUpdateSession_Cancel(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-cancel', 'test', 'gpt-4o', 'thinking', 'Goal', datetime('now'), datetime('now'))`)

	body := `{"status":"cancel"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	rows, _ := srv.conn.Query(ctx, `SELECT status, completed_at FROM sessions WHERE id = 'sess-cancel'`)
	if toString(rows[0]["status"]) != "failed" {
		t.Errorf("expected 'failed', got %q", toString(rows[0]["status"]))
	}
	if rows[0]["completed_at"] == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestUpdateSession_InvalidTransition(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-boot', 'test', 'gpt-4o', 'booting', 'Goal', datetime('now'), datetime('now'))`)

	body := `{"status":"pause"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-boot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateSession_PauseResumeRoundTrip is the DOGFOOD-002 contract test:
// the CLI sends action verbs, so the API must accept {"status":"pause"} on a
// running session (→ 200, status "paused") and then {"status":"resume"}
// (→ 200, status "idle") on the same session.
func TestUpdateSession_PauseResumeRoundTrip(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-rt', 'test', 'gpt-4o', 'thinking', 'Goal', datetime('now'), datetime('now'))`)

	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-rt", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+srv.adminKey)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		return w
	}
	status := func() string {
		rows, _ := srv.conn.Query(ctx, `SELECT status FROM sessions WHERE id = 'sess-rt'`)
		if len(rows) == 0 {
			return "missing"
		}
		return toString(rows[0]["status"])
	}

	if w := patch(`{"status":"pause"}`); w.Code != http.StatusOK {
		t.Fatalf("pause: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := status(); got != "paused" {
		t.Fatalf("after pause: expected 'paused', got %q", got)
	}

	if w := patch(`{"status":"resume"}`); w.Code != http.StatusOK {
		t.Fatalf("resume: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := status(); got != "idle" {
		t.Fatalf("after resume: expected 'idle', got %q", got)
	}

	// Target states are NOT valid actions — the old CLI sent these and got 400.
	if w := patch(`{"status":"paused"}`); w.Code != http.StatusBadRequest {
		t.Errorf("target-state payload: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Delete Session Tests
// ============================================================================

func TestDeleteSession_Admin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-del', 'test', 'gpt-4o', 'thinking', 'Goal', datetime('now'), datetime('now'))`)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/sess-del", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the soft-delete tombstone (DF-CONSENSUS-28). The OLD contract
	// pinned status=='failed' here — that was the defect: 'failed' is a crash
	// semantic, so the session stayed visible and messageable. The NEW
	// contract (SPEC-015 §3.1, SPEC-003 §2.1) tombstones via deleted_at; the
	// property still protected is that the row SURVIVES (soft delete, never a
	// hard DELETE) and the session is terminal from the API's perspective.
	rows, _ := srv.conn.Query(ctx, `SELECT status, completed_at, deleted_at FROM sessions WHERE id = 'sess-del'`)
	if len(rows) != 1 {
		t.Fatalf("expected row to survive soft delete, got %d rows", len(rows))
	}
	if rows[0]["deleted_at"] == nil {
		t.Error("expected deleted_at tombstone to be set")
	}
}

func TestDeleteSession_NonAdmin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-del2', 'test', 'gpt-4o', 'idle', 'Goal', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_sess_test_del_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-del', $1, $2, 'session', 'sess-del2', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/sess-del2", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Send Message Tests
// ============================================================================

func TestSendMessage_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at) VALUES ('sess-msg', 'test', 'gpt-4o', 'idle', 'Goal', 0, datetime('now'), datetime('now'))`)

	body := `{"content":"Focus on international markets"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-msg/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify memory event created
	memRows, _ := srv.conn.Query(ctx, `SELECT type, content FROM memory_events WHERE session_id = 'sess-msg' AND type = 'user_message'`)
	if len(memRows) == 0 {
		t.Fatal("expected user_message in memory_events")
	}

	// Session should transition to 'thinking'
	sessRows, _ := srv.conn.Query(ctx, `SELECT status, iteration FROM sessions WHERE id = 'sess-msg'`)
	if toString(sessRows[0]["status"]) != "thinking" {
		t.Errorf("expected status 'thinking', got %q", toString(sessRows[0]["status"]))
	}
}

func postSessionMessage(t *testing.T, handler http.Handler, adminKey, sessionID, key, content string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(SendMessageRequest{Content: content})
	if err != nil {
		t.Fatalf("marshal message request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func responseMessageID(t *testing.T, w *httptest.ResponseRecorder) int64 {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode message response %q: %v", w.Body.String(), err)
	}
	id, ok := resp["message_id"].(float64)
	if !ok || id <= 0 {
		t.Fatalf("response missing positive message_id: %s", w.Body.String())
	}
	return int64(id)
}

func TestSendMessage_IdempotencyReplaySurvivesServerRestart(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at)
		VALUES ('sess-idem', 'test', 'gpt-4o', 'idle', 'Goal', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	first := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem", "retry-key", "apply once")
	if first.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d: %s", first.Code, first.Body.String())
	}
	firstID := responseMessageID(t, first)

	// Recreate the HTTP server over the same durable database. The replay must
	// come from SQLite, not process-local state.
	restarted := NewServer(ServerConfig{Addr: ":0", DB: srv.conn, HITL: hitl.New(srv.conn)})
	replay := postSessionMessage(t, restarted.router, srv.adminKey, "sess-idem", "retry-key", "must not apply")
	if replay.Code != http.StatusOK {
		t.Fatalf("replay: expected 200, got %d: %s", replay.Code, replay.Body.String())
	}
	if replayID := responseMessageID(t, replay); replayID != firstID {
		t.Fatalf("replay message_id=%d, want original %d", replayID, firstID)
	}
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("replay body differs from original:\nfirst:  %s\nreplay: %s", first.Body.String(), replay.Body.String())
	}

	ledger, err := srv.conn.Query(ctx,
		`SELECT id, content FROM memory_events WHERE session_id = 'sess-idem' AND type = 'user_message'`)
	if err != nil {
		t.Fatalf("query memory ledger: %v", err)
	}
	if len(ledger) != 1 {
		t.Fatalf("idempotent retry wrote %d memory ledger rows, want 1", len(ledger))
	}
	if got := toString(ledger[0]["content"]); got != "apply once" {
		t.Fatalf("stored content=%q, want original request", got)
	}
	keys, err := srv.conn.Query(ctx,
		`SELECT response_message_id FROM idempotency_keys WHERE session_id = 'sess-idem' AND key = 'retry-key'`)
	if err != nil || len(keys) != 1 {
		t.Fatalf("durable idempotency mapping rows=%d err=%v, want 1", len(keys), err)
	}
	if got := toInt64(keys[0]["response_message_id"]); got != firstID {
		t.Fatalf("stored response_message_id=%d, want %d", got, firstID)
	}
}

func TestSendMessage_ConcurrentIdempotencyRetryAppliesOnce(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at)
		VALUES ('sess-idem-race', 'test', 'gpt-4o', 'paused', 'Goal', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	const retries = 4
	start := make(chan struct{})
	responses := make([]*httptest.ResponseRecorder, retries)
	var wg sync.WaitGroup
	wg.Add(retries)
	for i := 0; i < retries; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			responses[i] = postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-race", "concurrent-key", "apply once")
		}(i)
	}
	close(start)
	wg.Wait()

	var originalID int64
	for i, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("retry %d: expected 200, got %d: %s", i, response.Code, response.Body.String())
		}
		messageID := responseMessageID(t, response)
		if i == 0 {
			originalID = messageID
		} else if messageID != originalID {
			t.Fatalf("retry %d message_id=%d, want original %d", i, messageID, originalID)
		}
	}

	row, err := srv.conn.QueryRow(ctx,
		`SELECT COUNT(*) AS count FROM memory_events WHERE session_id = 'sess-idem-race' AND type = 'user_message'`)
	if err != nil {
		t.Fatalf("count memory ledger: %v", err)
	}
	if got := toInt64(row["count"]); got != 1 {
		t.Fatalf("concurrent retries wrote %d memory ledger rows, want 1", got)
	}
}

func TestSendMessage_DifferentOrAbsentIdempotencyKeyAppliesAgain(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at)
		VALUES ('sess-idem-distinct', 'test', 'gpt-4o', 'paused', 'Goal', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	first := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-distinct", "key-a", "first")
	second := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-distinct", "key-b", "second")
	withoutKey := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-distinct", "", "third")
	for name, w := range map[string]*httptest.ResponseRecorder{"first": first, "second": second, "without key": withoutKey} {
		if w.Code != http.StatusOK {
			t.Fatalf("%s request: expected 200, got %d: %s", name, w.Code, w.Body.String())
		}
	}
	if firstID, secondID := responseMessageID(t, first), responseMessageID(t, second); firstID == secondID {
		t.Fatalf("different keys returned the same message_id %d", firstID)
	}

	var noKeyBody map[string]any
	if err := json.Unmarshal(withoutKey.Body.Bytes(), &noKeyBody); err != nil {
		t.Fatalf("decode no-key response: %v", err)
	}
	if _, present := noKeyBody["message_id"]; present {
		t.Fatalf("request without Idempotency-Key changed legacy response body: %s", withoutKey.Body.String())
	}
	if noKeyBody["status"] != "message_received" || noKeyBody["session"] != "sess-idem-distinct" {
		t.Fatalf("request without key returned unexpected legacy body: %s", withoutKey.Body.String())
	}

	ledger, err := srv.conn.Query(ctx,
		`SELECT id FROM memory_events WHERE session_id = 'sess-idem-distinct' AND type = 'user_message'`)
	if err != nil {
		t.Fatalf("query memory ledger: %v", err)
	}
	if len(ledger) != 3 {
		t.Fatalf("different/no keys wrote %d memory ledger rows, want 3", len(ledger))
	}
}

func TestSendMessage_IdempotencyKeyIsScopedPerSession(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	for _, sessionID := range []string{"sess-idem-a", "sess-idem-b"} {
		if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at)
			VALUES ($1, 'test', 'gpt-4o', 'paused', 'Goal', 0, datetime('now'), datetime('now'))`, sessionID); err != nil {
			t.Fatalf("seed session %s: %v", sessionID, err)
		}
	}

	first := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-a", "shared-key", "for a")
	second := postSessionMessage(t, srv.router, srv.adminKey, "sess-idem-b", "shared-key", "for b")
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("same key across sessions failed: a=%d %s b=%d %s",
			first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	if firstID, secondID := responseMessageID(t, first), responseMessageID(t, second); firstID == secondID {
		t.Fatalf("same key across sessions returned the same message_id %d", firstID)
	}

	for _, sessionID := range []string{"sess-idem-a", "sess-idem-b"} {
		row, err := srv.conn.QueryRow(ctx,
			`SELECT COUNT(*) AS count FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, sessionID)
		if err != nil {
			t.Fatalf("count messages for %s: %v", sessionID, err)
		}
		if got := toInt64(row["count"]); got != 1 {
			t.Fatalf("session %s has %d messages, want 1", sessionID, got)
		}
	}
	rows, err := srv.conn.Query(ctx, `SELECT session_id FROM idempotency_keys WHERE key = 'shared-key'`)
	if err != nil || len(rows) != 2 {
		t.Fatalf("same key should persist once per session: rows=%d err=%v", len(rows), err)
	}
}

func TestSendMessage_EmptyContent(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-empty', 'test', 'gpt-4o', 'idle', 'Goal', datetime('now'), datetime('now'))`)

	body := `{"content":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-empty/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

// unused but kept for test utility
var _ = bytes.Buffer{}
