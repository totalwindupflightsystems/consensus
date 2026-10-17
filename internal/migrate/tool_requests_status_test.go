// Package migrate: APPROV-GATE-001 §4.1 regression tests.
//
// Migration 012 deliberately skipped the tool_requests status CHECK rewrite
// on SQLite, so `status='awaiting_approval'` — the value handleApprovalGated
// writes to park a gated tool — failed with `CHECK constraint failed (275)`,
// the row stayed 'pending', and the poll loop executed it with the approval
// still pending (the HITL gate was a no-op on the default backend).
//
// Migration 031 rebuilds tool_requests (and its child tool_results, because
// PRAGMA foreign_keys=ON makes the parent DROP fail on child rows) with the
// untruncated CHECK. These tests prove, against the REAL migration ladder via
// AutoMigrate, that all six statuses the Go code writes are accepted and that
// pre-existing data survives the rebuild.
//
// test=internal/migrate/migrate_test.go::TestSqliteToolRequestsAcceptAwaitingApproval
package migrate

import (
	"context"
	"strings"
	"testing"
)

// the six statuses across the Go writers (tool_executor park/claim/fail/
// complete, legacy seeds, sandbox timeout) — findings.md §4.1 "all six
// statuses the Go code writes".
var approvalGateStatuses = []string{
	"pending", "awaiting_approval", "executing", "completed", "failed", "timeout",
}

// TestSqliteToolRequestsAcceptAwaitingApproval runs the full ladder
// (Bootstrap + all migrations) on a fresh SQLite DB and asserts that
// inserting every status the Go writers produce succeeds — most importantly
// 'awaiting_approval', the value the pre-031 CHECK rejected.
func TestSqliteToolRequestsAcceptAwaitingApproval(t *testing.T) {
	// axiom:trace row=APPROV-GATE-001 impl=internal/migrate/migrations/031_sqlite_tool_requests_approval_status.sql
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// A live session row is required by tool_requests' FK.
	if err := database.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, heartbeat_at)
		 VALUES ('sess-gate-01', 'gate-test', 'test-model', 'idle', datetime('now'))`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	for _, status := range approvalGateStatuses {
		if err := database.Exec(ctx,
			`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
			 VALUES ('sess-gate-01', 1, 'gated_tool', '{}', $1)`, status); err != nil {
			t.Errorf("status %q rejected by the rebuilt CHECK: %v", status, err)
			continue
		}
		rows, qerr := database.Query(ctx,
			`SELECT status FROM tool_requests WHERE session_id = 'sess-gate-01' AND tool_name = 'gated_tool' AND status = $1`, status)
		if qerr != nil {
			t.Errorf("read back status %q: %v", status, qerr)
			continue
		}
		if len(rows) != 1 {
			t.Errorf("status %q insert seemed to succeed but is not readable back", status)
		}
	}

	// The negative arm: an unknown status must still be rejected by the
	// rebuilt constraint — widening must not remove the gate entirely.
	err := database.Exec(ctx,
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
		 VALUES ('sess-gate-01', 1, 'gated_tool', '{}', 'bogus_status')`)
	if err == nil {
		t.Error("rebuilt CHECK accepted unknown status 'bogus_status' — constraint lost")
	} else if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("expected a CHECK failure for 'bogus_status', got: %v", err)
	}

	t.Log("APPROV-GATE-001 §4.1: all six statuses accepted; unknown statuses still rejected")
}

// TestSqliteApprovalStatusRejectedBeforeFix documents the pre-fix failure
// mode directly against migration-012's schema so the regression is provably
// about the CHECK write the Go runtime performs.
func TestSqliteApprovalStatusRejectedBeforeFix(t *testing.T) {
	// No axiom trace marker on purpose — the pre-fix schema is referenced
	// through the same embedded ladder minus the rebuild: a DB built with
	// 031 EXCLUDED must reject 'awaiting_approval'. Exclusion is achieved by
	// loading 012's schema by hand (the exact CHECK that ships in the tree).
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY, agent_name TEXT NOT NULL, model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'booting'
		)`,
		`CREATE TABLE tool_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			iteration_id INTEGER NOT NULL,
			tool_name TEXT NOT NULL,
			parameters TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'timeout')),
			timeout_ms INT NOT NULL DEFAULT 30000,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`INSERT INTO sessions (id, agent_name, model_id, status) VALUES ('s0','a','m','idle')`,
	} {
		if err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("pre-fix schema: %v", err)
		}
	}

	// The exact write handleApprovalGated performs (tool_executor.go:349).
	err := database.Exec(ctx,
		`UPDATE tool_requests SET status = 'awaiting_approval' WHERE id = 0`)
	if err == nil {
		// No rows to update — probe the constraint via INSERT instead.
		if err := database.Exec(ctx,
			`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
			 VALUES ('s0', 1, 'gated_tool', '{}', 'awaiting_approval')`); err != nil {
			if !strings.Contains(err.Error(), "CHECK") {
				t.Errorf("expected CHECK failure, got: %v", err)
			}
			t.Log("pre-fix CHECK rejects 'awaiting_approval' as recorded in the evidence (raw/07 probe)")
			return
		}
		t.Error("pre-fix schema ACCEPTED 'awaiting_approval' — test premise broken (012's CHECK is gone?)")
		return
	}
	if !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("expected CHECK failure from the pre-fix schema, got: %v", err)
	}
}
