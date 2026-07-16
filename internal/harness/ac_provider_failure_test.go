// Package harness: provider failure test (Phase 4, Task 1).
package harness

import (
	"context"
	"fmt"
	"testing"
)

func TestProviderFailure_RecordsAuditAndReturnsError(t *testing.T) {
	th, err := newTestHarness(failingMockLLM(fmt.Errorf("provider timeout")))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	result, err := th.RunAgentIteration(context.Background(), sessionID)
	// RunAgentIteration should NOT return a Go error for LLM failures —
	// it returns the error in the result.Status field
	if err != nil {
		t.Fatalf("unexpected Go error from RunAgentIteration: %v", err)
	}

	// Verify result status
	if result.Status != "error" {
		t.Errorf("expected status 'error', got %q", result.Status)
	}
	if result.Error == nil {
		t.Error("expected non-nil result.Error")
	}
	if result.ErrorInjected == "" {
		t.Error("expected ErrorInjected to contain failure message")
	}

	// Verify audit log was written
	count, err := th.assertAuditLogCount(sessionID)
	if err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 audit log entry, got %d", count)
	}

	// Verify session still exists (not corrupted)
	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("failed to query session: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("session should still exist after provider failure")
	}
	t.Logf("session status after provider failure: %v", rows[0]["status"])
}
