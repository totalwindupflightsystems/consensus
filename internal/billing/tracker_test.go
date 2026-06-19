// Package billing: tests for cost tracking and budget enforcement.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-1 test=internal/billing/tracker_test.go
package billing

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// Test DB Setup
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()
	ctx := context.Background()
	// Use file-based SQLite for persistence across queries
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Create agent_billing table
	_ = database.Exec(ctx, `CREATE TABLE IF NOT EXISTS agent_billing (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		iteration INTEGER NOT NULL,
		model_id TEXT NOT NULL,
		category TEXT NOT NULL,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		cache_read_tokens INTEGER DEFAULT 0,
		cache_write_tokens INTEGER DEFAULT 0,
		cost_usd REAL DEFAULT 0,
		recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)

	cleanup := func() {
		database.Close()
		// Clean up any file-based DB
		os.Remove(t.Name())
	}
	return database, cleanup
}

// ============================================================================
// AC-HARDEN-01: Billing row written after LLM call
// ============================================================================

func TestRecordBilling_WritesRow(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()

	// Record a billing entry
	sessionID := "test-session-abc"
	tracker.RecordBilling(ctx, sessionID, 1, "gpt-4o", "cognition", 150, 50, 10, 5, 0.000875)

	// Verify the row was written
	rows, err := database.Query(ctx, `SELECT * FROM agent_billing WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query billing: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected billing row, got none")
	}

	row := rows[0]
	if row["session_id"] != sessionID {
		t.Errorf("session_id = %v, want %s", row["session_id"], sessionID)
	}
	if row["model_id"] != "gpt-4o" {
		t.Errorf("model_id = %v, want gpt-4o", row["model_id"])
	}
	if row["category"] != "cognition" {
		t.Errorf("category = %v, want cognition", row["category"])
	}

	// Verify token counts
	promptTokens := toInt(row["prompt_tokens"])
	if promptTokens != 150 {
		t.Errorf("prompt_tokens = %d, want 150", promptTokens)
	}

	completionTokens := toInt(row["completion_tokens"])
	if completionTokens != 50 {
		t.Errorf("completion_tokens = %d, want 50", completionTokens)
	}
}

func TestRecordBilling_HandlesNilDB(t *testing.T) {
	tracker := NewTracker(nil)
	ctx := context.Background()

	// Should not panic
	tracker.RecordBilling(ctx, "test", 1, "gpt-4o", "cognition", 100, 50, 0, 0, 0.01)
}

func TestRecordBilling_HandlesEmptySessionID(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()

	// Should not panic, should log and skip
	tracker.RecordBilling(ctx, "", 1, "gpt-4o", "cognition", 100, 50, 0, 0, 0.01)
	// This is fine — empty string is logged but not fatal
}

// ============================================================================
// AC-HARDEN-02: Budget limit enforcement
// ============================================================================

func TestBudgetCheck_NotExceeded(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()
	sessionID := "test-budget-session"

	// No billing rows yet — budget should not be exceeded
	exceeded, err := tracker.BudgetCheck(ctx, sessionID, 100) // 100 cents = $1.00
	if err != nil {
		t.Fatalf("budget check: %v", err)
	}
	if exceeded {
		t.Error("budget should not be exceeded with no rows")
	}

	// Add a small cost row ($0.01)
	tracker.RecordBilling(ctx, sessionID, 1, "gpt-4o", "cognition", 100, 50, 0, 0, 0.01)
	exceeded, err = tracker.BudgetCheck(ctx, sessionID, 100)
	if err != nil {
		t.Fatalf("budget check: %v", err)
	}
	if exceeded {
		t.Error("budget should not be exceeded at $0.01")
	}
}

func TestBudgetCheck_Exceeded(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()
	sessionID := "test-budget-exceed-session"

	// Add $5.00 in costs (5.00 USD = 500 cents)
	tracker.RecordBilling(ctx, sessionID, 1, "gpt-4o", "cognition", 100000, 10000, 0, 0, 5.00)

	// Budget is 100 cents ($1.00)
	exceeded, err := tracker.BudgetCheck(ctx, sessionID, 100)
	if err == nil {
		t.Error("expected error for exceeded budget")
	}
	if !exceeded {
		t.Error("budget should be exceeded at $5.00 with $1.00 limit")
	}
}

func TestBudgetCheck_ZeroLimit(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()

	// Zero budget limit = no limit
	exceeded, err := tracker.BudgetCheck(ctx, "any-session", 0)
	if err != nil {
		t.Fatalf("budget check: %v", err)
	}
	if exceeded {
		t.Error("budget should not be exceeded with zero limit (no limit)")
	}
}

func TestGetCumulativeCost(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()
	sessionID := "test-cumulative-session"

	// Record multiple billing rows
	tracker.RecordBilling(ctx, sessionID, 1, "gpt-4o", "cognition", 100, 50, 0, 0, 0.25)
	tracker.RecordBilling(ctx, sessionID, 2, "gpt-4o", "cognition", 200, 100, 0, 0, 0.75)

	costCents, err := tracker.GetCumulativeCost(ctx, sessionID)
	if err != nil {
		t.Fatalf("cumulative cost: %v", err)
	}

	// 0.25 + 0.75 = $1.00 = 100 cents
	if costCents != 100 {
		t.Errorf("cumulative cost = %d cents, want 100", costCents)
	}
}

func TestGetCumulativeCost_NoRows(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	tracker := NewTracker(database)
	ctx := context.Background()

	costCents, err := tracker.GetCumulativeCost(ctx, "no-rows-session")
	if err != nil {
		t.Fatalf("cumulative cost: %v", err)
	}
	if costCents != 0 {
		t.Errorf("cumulative cost = %d, want 0 for session with no rows", costCents)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}
