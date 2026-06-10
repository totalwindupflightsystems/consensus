// Package tools: tests for approval gating (WI-005).
//
// axiom:trace work_item=WI-005 spec=specs/014-hitl-interrupt-state.md plan=phase-4/task-2 test=internal/tools/approval_test.go
package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/driver"
)

// setupApprovalTestDB creates an in-memory database for approval testing.
func setupApprovalTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL DEFAULT 'test',
			status TEXT NOT NULL DEFAULT 'idle',
			project_id TEXT,
			heartbeat_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tools_registry (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			requires_approval INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS approval_requests (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			request_type TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			risk_level TEXT NOT NULL DEFAULT 'medium',
			context TEXT NOT NULL DEFAULT '{}',
			target_tool TEXT,
			target_sql TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			reviewer_id TEXT,
			review_notes TEXT,
			modified_sql TEXT,
			created_at TEXT,
			reviewed_at TEXT,
			expires_at TEXT
		)`,
	} {
		if err := database.Exec(ctx, stmt); err != nil {
			database.Close()
			t.Fatalf("create table: %v", err)
		}
	}

	_ = database.Exec(ctx, `INSERT INTO sessions (id) VALUES ('sess-ap-01')`)
	_ = database.Exec(ctx, `INSERT INTO sessions (id) VALUES ('sess-ap-02')`)

	cleanup := func() { database.Close() }
	return database, cleanup
}

// ============================================================================
// ToolRequiresApproval Tests
// ============================================================================

func TestToolRequiresApproval_NotRequired(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Tool without requires_approval
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, requires_approval) VALUES ('ap-na', 'safe_tool', 1, 0)`)

	required, err := ToolRequiresApproval(ctx, database, "safe_tool")
	if err != nil {
		t.Fatalf("ToolRequiresApproval: %v", err)
	}
	if required {
		t.Error("expected requires_approval = false for safe_tool")
	}
}

func TestToolRequiresApproval_Required(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Tool with requires_approval
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, requires_approval) VALUES ('ap-req', 'dangerous_tool', 1, 1)`)

	required, err := ToolRequiresApproval(ctx, database, "dangerous_tool")
	if err != nil {
		t.Fatalf("ToolRequiresApproval: %v", err)
	}
	if !required {
		t.Error("expected requires_approval = true for dangerous_tool")
	}
}

func TestToolRequiresApproval_Disabled(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Disabled tool — doesn't require approval (can't be executed)
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, requires_approval) VALUES ('ap-dis', 'disabled_tool', 0, 1)`)

	required, err := ToolRequiresApproval(ctx, database, "disabled_tool")
	if err != nil {
		t.Fatalf("ToolRequiresApproval: %v", err)
	}
	if required {
		t.Error("disabled tool should not require approval")
	}
}

func TestToolRequiresApproval_Unknown(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	required, err := ToolRequiresApproval(ctx, database, "nonexistent")
	if err != nil {
		t.Fatalf("ToolRequiresApproval: %v", err)
	}
	if required {
		t.Error("unknown tool should not require approval")
	}
}

func TestToolRequiresApproval_NilDB(t *testing.T) {
	_, err := ToolRequiresApproval(context.Background(), nil, "test_tool")
	if err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("expected 'no database' error, got: %v", err)
	}
}

// ============================================================================
// CreateToolApprovalRequest Tests
// ============================================================================

func TestCreateToolApprovalRequest_Success(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	params := map[string]any{"url": "https://example.com", "timeout": 30}
	approvalID, err := CreateToolApprovalRequest(ctx, database, "sess-ap-01", "web_scraper", params)
	if err != nil {
		t.Fatalf("CreateToolApprovalRequest: %v", err)
	}

	if approvalID == "" {
		t.Fatal("expected non-empty approval ID")
	}

	// Verify approval_request was inserted
	rows, err := database.Query(ctx, `SELECT id, session_id, request_type, target_tool, status FROM approval_requests WHERE id = $1`, approvalID)
	if err != nil {
		t.Fatalf("query approval: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("approval request not found")
	}
	if toString(rows[0]["session_id"]) != "sess-ap-01" {
		t.Errorf("session_id = %q, want sess-ap-01", toString(rows[0]["session_id"]))
	}
	if toString(rows[0]["request_type"]) != "tool_execution" {
		t.Errorf("request_type = %q, want tool_execution", toString(rows[0]["request_type"]))
	}
	if toString(rows[0]["target_tool"]) != "web_scraper" {
		t.Errorf("target_tool = %q, want web_scraper", toString(rows[0]["target_tool"]))
	}
	if toString(rows[0]["status"]) != "pending" {
		t.Errorf("status = %q, want pending", toString(rows[0]["status"]))
	}

	t.Logf("Approval request created: id=%s", approvalID)
}

func TestCreateToolApprovalRequest_PausesSession(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	params := map[string]any{"command": "rm -rf /"}
	_, err := CreateToolApprovalRequest(ctx, database, "sess-ap-02", "dangerous_tool", params)
	if err != nil {
		t.Fatalf("CreateToolApprovalRequest: %v", err)
	}

	// Verify session was paused
	rows, err := database.Query(ctx, `SELECT status FROM sessions WHERE id = 'sess-ap-02'`)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if len(rows) > 0 {
		status := toString(rows[0]["status"])
		if status != "paused" {
			t.Errorf("session status = %q, want 'paused'", status)
		}
	}
}

func TestCreateToolApprovalRequest_NilDB(t *testing.T) {
	_, err := CreateToolApprovalRequest(context.Background(), nil, "sess-nil", "test_tool", nil)
	if err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("expected 'no database' error, got: %v", err)
	}
}

func TestCreateToolApprovalRequest_EmptyParams(t *testing.T) {
	database, cleanup := setupApprovalTestDB(t)
	defer cleanup()
	ctx := context.Background()

	approvalID, err := CreateToolApprovalRequest(ctx, database, "sess-ap-01", "simple_tool", nil)
	if err != nil {
		t.Fatalf("CreateToolApprovalRequest: %v", err)
	}
	if approvalID == "" {
		t.Fatal("expected non-empty approval ID")
	}

	rows, err := database.Query(ctx, `SELECT count(*) as cnt FROM approval_requests WHERE id = $1`, approvalID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) > 0 && toInt(rows[0]["cnt"]) != 1 {
		t.Error("approval request not found")
	}
}
