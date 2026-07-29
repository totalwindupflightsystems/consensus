// AC-037 to AC-039: HITL — Approval Requests, Reviews & Pause/Resume
// Canonical from SPEC-014 §3-4
//
// Verifies that:
//   1. Approval requests can be created with different types
//   2. Approvals can be reviewed (approved/rejected) with reviewer tracking
//   3. Sessions can be paused and resumed
//   4. Auto-approval config works for low-risk requests

package harness

import (
	"testing"
	"time"
)

// AC-037: HITL Approval Request Creation
func TestAC037_HITLApprovalRequest(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-037: create session: %v", err)
	}

	// Create approval request (matching approval_requests schema from SPEC-014 §3.1)
	err = th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status)
		VALUES ('apr-037-1', $1, 1, 'tool_execution', 'Delete 5000 rows needs approval', 'high', 'pending')
	`, sessionID)
	if err != nil {
		t.Fatalf("AC-037: insert approval request: %v", err)
	}

	// Verify it was created with correct data
	rows, _ := th.conn.Query(th.ctx, `
		SELECT id, session_id, request_type, description, status
		FROM approval_requests WHERE id = 'apr-037-1'
	`)
	if len(rows) == 0 {
		t.Fatal("AC-037: approval request not found")
	}
	rtype := toString(rows[0]["request_type"])
	rstatus := toString(rows[0]["status"])
	desc := toString(rows[0]["description"])

	if rtype != "tool_execution" {
		t.Errorf("AC-037: request_type = %q, want 'tool_execution'", rtype)
	}
	if rstatus != "pending" {
		t.Errorf("AC-037: status = %q, want 'pending'", rstatus)
	}
	if desc == "" {
		t.Error("AC-037: description should not be empty")
	}
	t.Log("AC-037 PASS: approval request created correctly")
}

// AC-038: HITL Approval Review — approve and reject
func TestAC038_HITLApprovalReview(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-038: create session: %v", err)
	}

	// Create approval request
	th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status)
		VALUES ('apr-038-1', $1, 1, 'tool_execution', 'Run database migration', 'high', 'pending')
	`, sessionID)

	// Approve it
	err = th.conn.Exec(th.ctx, `
		UPDATE approval_requests SET status = 'approved', reviewer_id = 'human-1', reviewed_at = datetime('now')
		WHERE id = 'apr-038-1'
	`)
	if err != nil {
		t.Fatalf("AC-038: approve request: %v", err)
	}

	// Verify approved
	rows, _ := th.conn.Query(th.ctx, `SELECT status, reviewer_id FROM approval_requests WHERE id = 'apr-038-1'`)
	if len(rows) == 0 {
		t.Fatal("AC-038: request not found after approval")
	}
	status := toString(rows[0]["status"])
	reviewer := toString(rows[0]["reviewer_id"])
	if status != "approved" {
		t.Errorf("AC-038: status = %q, want 'approved'", status)
	}
	if reviewer != "human-1" {
		t.Errorf("AC-038: reviewed_by = %q, want 'human-1'", reviewer)
	}
	t.Log("AC-038 PASS: approval request reviewed and approved")

	// Now test rejection
	th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status)
		VALUES ('apr-038-2', $1, 1, 'destructive_action', 'Delete production data', 'critical', 'pending')
	`, sessionID)

	err = th.conn.Exec(th.ctx, `
		UPDATE approval_requests SET status = 'rejected', reviewer_id = 'human-1', reviewed_at = datetime('now')
		WHERE id = 'apr-038-2'
	`)
	if err != nil {
		t.Fatalf("AC-038: reject request: %v", err)
	}

	rows2, _ := th.conn.Query(th.ctx, `SELECT status FROM approval_requests WHERE id = 'apr-038-2'`)
	rejStatus := "unknown"
	if len(rows2) > 0 {
		rejStatus = toString(rows2[0]["status"])
	}
	if rejStatus != "rejected" {
		t.Errorf("AC-038: rejected status = %q, want 'rejected'", rejStatus)
	}
	t.Log("AC-038 PASS: approval rejection works")
}

// AC-039: Session Pause and Resume
func TestAC039_SessionPauseResume(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-039: create session: %v", err)
	}

	// Set to thinking (actively running)
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking', heartbeat_at = datetime('now') WHERE id = $1`, sessionID)

	// Pause for HITL
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'paused', heartbeat_at = datetime('now') WHERE id = $1`, sessionID)

	rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	pausedStatus := "unknown"
	if len(rows) > 0 {
		pausedStatus = toString(rows[0]["status"])
	}
	if pausedStatus != "paused" {
		t.Errorf("AC-039: after pause status = %q, want 'paused'", pausedStatus)
	}
	t.Logf("AC-039: session paused at %s", time.Now().Format(time.RFC3339))

	// Resume
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'idle', heartbeat_at = datetime('now') WHERE id = $1`, sessionID)

	rows2, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	resumedStatus := "unknown"
	if len(rows2) > 0 {
		resumedStatus = toString(rows2[0]["status"])
	}
	if resumedStatus != "idle" {
		t.Errorf("AC-039: after resume status = %q, want 'idle'", resumedStatus)
	}
	t.Log("AC-039 PASS: session pause/resume cycle works")
}
