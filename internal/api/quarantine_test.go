// Package api — quarantine API handler tests (SPEC-005 §Cognitive Firewall, WI-004).
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/015-api-and-mcp.md plan=phase-6/task-3/step-1
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/driver"
	"github.com/wojons/conscientiousness/internal/quarantine"
)

// ============================================================================
// Test setup
// ============================================================================

func setupQuarantineTest(t *testing.T) (db.DB, *quarantine.QuarantineService, *Server, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create core tables
	mustExecQ(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY, agent_name TEXT NOT NULL DEFAULT 'test',
		model_id TEXT NOT NULL DEFAULT 'default', context_budget INT NOT NULL DEFAULT 128000,
		tokens_used_in BIGINT NOT NULL DEFAULT 0, tokens_used_out BIGINT NOT NULL DEFAULT 0,
		iteration BIGINT NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'idle',
		project_id TEXT,
		heartbeat_at TEXT NOT NULL DEFAULT (datetime('now')),
		planning_max_turns INT NOT NULL DEFAULT 10,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	mustExecQ(t, database, ctx, `CREATE TABLE IF NOT EXISTS memory_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL,
		content TEXT NOT NULL, session_id TEXT NOT NULL,
		iteration_created BIGINT NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	mustExecQ(t, database, ctx, `CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY, key_hash TEXT NOT NULL UNIQUE, key_prefix TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT 'admin' CHECK (scope IN ('admin','session','readonly','webhook')),
		session_id TEXT, expires_at TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	mustExecQ(t, database, ctx, `CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix)`)

	mustExecQ(t, database, ctx, `CREATE TABLE IF NOT EXISTS api_rate_limits (
		key_prefix TEXT PRIMARY KEY, requests_count INT NOT NULL DEFAULT 0,
		window_start TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	mustExecQ(t, database, ctx, `CREATE TABLE IF NOT EXISTS external_quarantine (
		id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'api_response', source_url TEXT,
		raw_content TEXT NOT NULL, content_hash TEXT NOT NULL,
		validation_status TEXT NOT NULL DEFAULT 'pending', validation_notes TEXT,
		promoted_memory_id INTEGER,
		expires_at TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	mustExecQ(t, database, ctx, `INSERT INTO sessions (id, agent_name, status) VALUES ('test-session', 'test-agent', 'idle')`)

	// Create admin API key for auth
	keyHash := sha256Hash("test-admin-key-12345")
	mustExecQ(t, database, ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope) VALUES ('k1', $1, 'test-adm', 'admin')`, keyHash)

	qs := quarantine.NewQuarantineService(database, nil)

	srv := NewServer(ServerConfig{
		Addr:              ":0",
		DB:                database,
		QuarantineService: qs,
	})

	// Seed some quarantine data
	mustExecQ(t, database, ctx, `INSERT INTO external_quarantine (session_id, source_type, raw_content, content_hash, validation_status, expires_at, created_at)
		VALUES ('test-session', 'api_response', 'clean payload', 'hash1', 'pending', '2099-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExecQ(t, database, ctx, `INSERT INTO external_quarantine (session_id, source_type, raw_content, content_hash, validation_status, expires_at, created_at)
		VALUES ('test-session', 'api_response', 'malicious payload', 'hash2', 'pending', '2099-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExecQ(t, database, ctx, `INSERT INTO external_quarantine (session_id, source_type, raw_content, content_hash, validation_status, expires_at, created_at)
		VALUES ('test-session', 'api_response', 'already rejected', 'hash3', 'rejected', '2099-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	cleanup := func() {
		database.Close()
	}

	return database, qs, srv, cleanup
}

func mustExecQ(t *testing.T, database db.DB, ctx context.Context, query string, args ...any) {
	t.Helper()
	if err := database.Exec(ctx, query, args...); err != nil {
		t.Fatalf("exec: %s: %v", query, err)
	}
}

func quarantineAuthRequest(t *testing.T, srv *Server, method, path, body string) *http.Response {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer test-admin-key-12345")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Result()
}

// ============================================================================
// API Tests
// ============================================================================

func TestListQuarantineAll(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-2
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "GET", "/api/v1/quarantine", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body struct {
		Items []quarantine.QuarantineItem `json:"items"`
		Count int                         `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Count != 3 {
		t.Errorf("expected 3 items, got %d", body.Count)
	}
	t.Logf("List all: %d items returned", body.Count)
}

func TestListQuarantinePending(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-3
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "GET", "/api/v1/quarantine?status=pending", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body struct {
		Items []quarantine.QuarantineItem `json:"items"`
		Count int                         `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Count != 2 {
		t.Errorf("expected 2 pending items, got %d", body.Count)
	}
	t.Logf("List pending: %d items", body.Count)
}

func TestApproveQuarantineAPI(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-4
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	// Approve item with ID 1
	resp := quarantineAuthRequest(t, srv, "POST", "/api/v1/quarantine/1/approve", `{"session_id": "test-session"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body struct {
		Status string                      `json:"status"`
		Item   *quarantine.QuarantineItem  `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", body.Status)
	}
	if body.Item == nil {
		t.Fatal("expected item in response")
	}
	if body.Item.ValidationStatus != "validated" {
		t.Errorf("expected validated status, got %q", body.Item.ValidationStatus)
	}
	if body.Item.PromotedMemoryID == 0 {
		t.Errorf("expected promoted_memory_id to be set")
	}
	t.Logf("Approved quarantine item %d, promoted to memory event %d", body.Item.ID, body.Item.PromotedMemoryID)
}

func TestRejectQuarantineAPI(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-5
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "POST", "/api/v1/quarantine/2/reject", `{"reason": "Confirmed malicious payload"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	var body struct {
		Status string                      `json:"status"`
		Item   *quarantine.QuarantineItem  `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "rejected" {
		t.Errorf("expected status 'rejected', got %q", body.Status)
	}
	if body.Item == nil {
		t.Fatal("expected item in response")
	}
	if body.Item.ValidationStatus != "rejected" {
		t.Errorf("expected rejected status, got %q", body.Item.ValidationStatus)
	}
	if body.Item.ValidationNotes != "Confirmed malicious payload" {
		t.Errorf("expected reason 'Confirmed malicious payload', got %q", body.Item.ValidationNotes)
	}
	t.Logf("Rejected quarantine item %d: %s", body.Item.ID, body.Item.ValidationNotes)
}

func TestRejectQuarantineWithoutReason(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-6
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "POST", "/api/v1/quarantine/1/reject", `{"reason": ""}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for empty reason (uses default), got %d", resp.StatusCode)
	}
}

func TestApproveNonexistentQuarantine(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-7
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "POST", "/api/v1/quarantine/999/approve", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for nonexistent item, got %d", resp.StatusCode)
	}
}

func TestQuarantineInvalidID(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-8
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	resp := quarantineAuthRequest(t, srv, "POST", "/api/v1/quarantine/abc/approve", "{}")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ID, got %d", resp.StatusCode)
	}
}

func TestQuarantineUnauthenticated(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-3/step-9
	_, _, srv, cleanup := setupQuarantineTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/quarantine", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", w.Code)
	}
}
