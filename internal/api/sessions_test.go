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
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Run migration
	if err := runIntegrationMigration(ctx, conn); err != nil {
		conn.Close()
		t.Fatalf("migration: %v", err)
	}

	// Seed model_registry (required FK)
	if err := conn.Exec(ctx, `INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out) VALUES ('gpt-4o', 1, 128000, 2.50, 10.00)`); err != nil {
		conn.Close()
		t.Fatalf("seed model: %v", err)
	}

	// Create an admin API key
	adminKey := "cs_ak_admin_test_1234567890_abcdef"
	hash := sha256Hash(adminKey)
	prefix := "cs_ak_ad"

	if err := conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-admin-1', $1, $2, 'admin', datetime('now'))`, hash, prefix); err != nil {
		conn.Close()
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
	is.conn.Close()
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
	return nil
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
	json.NewDecoder(w.Body).Decode(&resp)

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

func TestCreateSession_NonAdminKey(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	// Create a session so the key has a valid session_id to reference
	ctx := context.Background()
	srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('test-session-id', 'existing', 'gpt-4o', 'idle', 'Existing', datetime('now'), datetime('now'))`)

	sessionKey := "cs_sk_session_test_key_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-sess-1', $1, $2, 'session', 'test-session-id', datetime('now'))`, hash, prefix)

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
	json.NewDecoder(w.Body).Decode(&sessions)

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
	json.NewDecoder(w.Body).Decode(&sessions)

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

	// Verify session is now failed/completed
	rows, _ := srv.conn.Query(ctx, `SELECT status, completed_at FROM sessions WHERE id = 'sess-del'`)
	if toString(rows[0]["status"]) != "failed" {
		t.Errorf("expected 'failed', got %q", toString(rows[0]["status"]))
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
