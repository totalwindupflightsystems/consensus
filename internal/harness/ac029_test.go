// AC-029: Auto-commit on Max Turns
// Canonical from SPEC-020 §5 (auto_commit_on_max)
//
// Verifies that the planning infrastructure handles max turns properly:
//   1. Auto-commit fires when max_turns reached with staged work
//   2. Session returns to idle after auto-commit
//   3. No-work max turns leaves session idle (no crash)

package harness

import (
	"testing"
)

func TestAC029_AutoCommitOnMaxTurns_WithWork(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Set session to 'thinking' so RunInteractivePlanning can claim it
	err = th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("AC-029: set session to thinking: %v", err)
	}

	// Create a config with max_turns=2 and auto_commit=true
	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2
	cfg.AutoCommitOnMax = true

	// Read initial iteration count
	initRows, _ := th.conn.Query(th.ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	initIter := int64(0)
	if len(initRows) > 0 {
		initIter = toInt64(initRows[0]["iteration"])
	}

	// Run interactive planning — the mock returns minimalOutput() which has
	// memory_state_changes, so it will stage work on each turn
	result, err := th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("AC-029: RunInteractivePlanning: %v", err)
	}

	if result == nil {
		t.Fatal("AC-029: result should not be nil")
	}

	// After auto-commit, session should be 'idle'
	rows, _ := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	if len(rows) == 0 {
		t.Fatalf("AC-029: session not found")
	}
	status := toString(rows[0]["status"])
	iteration := toInt64(rows[0]["iteration"])

	t.Logf("AC-029: session status=%s iteration=%d (was %d)", status, iteration, initIter)

	if status != "idle" {
		t.Errorf("AC-029: session status = %s, want 'idle' after auto-commit", status)
	}
	if iteration <= initIter {
		t.Errorf("AC-029: iteration = %d, want > %d (should have incremented on commit)", iteration, initIter)
	}
	t.Log("AC-029 PASS: auto-commit worked — session is idle, iteration incremented")
}

func TestAC029_MaxTurnsNoWork_SessionIdle(t *testing.T) {
	// Create mock with no memory changes (no-op)
	noopOutput := &AgentOutput{
		InternalMonologue: "Just thinking...",
		MemoryStateChanges: nil,
		SystemActions:     nil,
		ToolRequests:      nil,
	}
	mockNoop := newMockLLM(noopOutput)

	th, err := newTestHarness(mockNoop)
	if err != nil {
		t.Fatalf("AC-029: create harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-029: create session: %v", err)
	}

	err = th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("AC-029: set thinking: %v", err)
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2
	cfg.AutoCommitOnMax = true

	result, err := th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("AC-029: RunInteractivePlanning: %v", err)
	}

	if result == nil {
		t.Fatal("AC-029: result should not be nil")
	}

	// Should still return to idle (with a warning, not failed)
	rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	status := "unknown"
	if len(rows) > 0 {
		status = toString(rows[0]["status"])
	}

	t.Logf("AC-029: no-work scenario session status=%s, result.Status=%s", status, result.Status)

	if status != "idle" {
		t.Errorf("AC-029: session status = %s, want 'idle'", status)
	}
	t.Log("AC-029 PASS: no-work max turns leaves session idle")
}
