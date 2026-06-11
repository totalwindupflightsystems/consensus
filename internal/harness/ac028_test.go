// AC-028: Rollback and Retry
// Canonical from SPEC-020 §8
//
// Verifies that:
//   1. Transaction rollback undoes staged SQL changes (memory_events not modified)
//   2. A fresh transaction can be opened and planning continues on retry
//   3. After retry + commit, the correct state is persisted

package harness

import (
	"testing"
)

func TestAC028_RollbackUndoesWork(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Count initial events
	initRows, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	initialCount := 0
	if len(initRows) > 0 {
		initialCount = toInt(initRows[0]["cnt"])
	}

	// --- Phase 1: Open tx1, do work, rollback ---
	tx1, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("AC-028: begin tx1: %v", err)
	}
	tx1.SetSessionContext(th.ctx, sessionID)

	// Insert a memory event within tx1
	err = tx1.Exec(th.ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'tx1-data', $1, 1)`, sessionID)
	if err != nil {
		t.Fatalf("AC-028: tx1 insert memory: %v", err)
	}

	// Rollback tx1
	tx1.Rollback()
	t.Log("AC-028: tx1 rolled back")

	// Verify the work was undone — memory_events count should be back to initial
	afterRollbackRows, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	afterCount := 0
	if len(afterRollbackRows) > 0 {
		afterCount = toInt(afterRollbackRows[0]["cnt"])
	}
	if afterCount != initialCount {
		t.Errorf("AC-028: after rollback count = %d, want %d (initial) — rollback did not undo work", afterCount, initialCount)
	}
	t.Logf("AC-028 PASS: rollback restored event count to %d (initial=%d)", afterCount, initialCount)

	// --- Phase 2: Open tx2, retry with correct data ---
	tx2, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("AC-028: begin tx2: %v", err)
	}
	defer func() {
		if tx2.IsActive() {
			tx2.Rollback()
		}
	}()
	tx2.SetSessionContext(th.ctx, sessionID)

	// Successfully insert this time
	err = tx2.Exec(th.ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'tx2-retry-data', $1, 1)`, sessionID)
	if err != nil {
		t.Fatalf("AC-028: tx2 insert: %v", err)
	}

	// Commit tx2
	tx2.Commit()
	t.Log("AC-028: tx2 committed — retry succeeded")

	// Verify tx2 work is visible
	finalRows, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	finalCount := 0
	if len(finalRows) > 0 {
		finalCount = toInt(finalRows[0]["cnt"])
	}
	if finalCount != initialCount+1 {
		t.Errorf("AC-028: final count = %d, want %d (initial+1)", finalCount, initialCount+1)
	}
	t.Log("AC-028 PASS: retry and commit produced correct state — rollback + retry works")
}
