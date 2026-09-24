package harness

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestHandlePlanningErrorSetsFailed verifies that handlePlanningError
// properly transitions a session to status='failed'.
func TestHandlePlanningErrorSetsFailed(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	ctx := context.Background()

	// Add heartbeat_at to the test session table (setupTestDB doesn't include it)
	database.Exec(ctx, `ALTER TABLE sessions ADD COLUMN heartbeat_at TEXT`)

	// Create a session
	sessionID := "ac017-direct-test"
	database.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at)
		VALUES ('ac017-direct-test', 'ac017-test', 'mock', 'planning', 'test handlePlanningError', datetime('now'))`)

	// Create harness
	h := New(database, failingMockLLM(fmt.Errorf("should not be called")))

	// Call handlePlanningError directly
	tx, err := h.db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	result, err := h.handlePlanningError(ctx, tx, sessionID, fmt.Errorf("direct test error"))
	if err != nil {
		t.Fatalf("handlePlanningError returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("handlePlanningError returned nil result")
	}

	time.Sleep(100 * time.Millisecond)

	// Verify session status was set to 'failed'
	rows, err := database.Query(ctx, `SELECT status FROM sessions WHERE id = 'ac017-direct-test'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("session not found")
	}
	status := rows[0]["status"]
	t.Logf("session status after handlePlanningError: %v", status)

	if status != "failed" {
		t.Errorf("AC-017 FAIL: expected status='failed', got '%v'", status)
	} else {
		t.Log("AC-017 PASS: status=failed")
	}
}
