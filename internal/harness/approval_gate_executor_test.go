// Package harness: APPROV-GATE-001 — legacy-path (tool_requests) gate tests.
//
// §4.1: on SQLite the pre-031 CHECK rejected status='awaiting_approval', the
// park write failed, the row stayed 'pending', and the same poll loop claimed
// and executed the tool while its approval was still pending (findings.md
// §4.1, raw/03b §C2 "R8-APPROVAL-GATE-BYPASS-PROOF-EXECUTED"). These tests
// pin the restored contract through ToolExecutorImpl.PollOnce:
//
//   - a requires_approval tool with a pending approval does NOT execute:
//     tool_request parks at 'awaiting_approval' with a linked approval id,
//     zero tool_results rows
//   - approve → (re-queue hook) → the SAME PollOnce executes it
//   - reject → request 'failed' + refusal tool_result is_error=1, and a
//     second PollOnce never revives it
//
// test=internal/harness/approval_gate_executor_test.go
package harness

import (
	"context"
	"strings"
	"testing"
)

// TestApprovalGated_ExecutorParkAndApprove drives the park → approve →
// execute cycle through the real poll loop.
func TestApprovalGated_ExecutorParkAndApprove(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "sess-te-01"

	// Gated tool registered as go_native → PollOnce executes it with the
	// stub handler output.
	_ = database.Exec(ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled, requires_approval)
		 VALUES ('tool-gate-101', 'gated_exec_tool', 'requires human approval', 'external', 'go_native', 'ApprovalHandler', 1, 1)`)

	reqID := seedPendingToolRequest(t, database, sessionID, "gated_exec_tool")

	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	// ---- Poll #1: the gate must PARK, not execute. ----
	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce #1: %v", err)
	}
	if count != 0 {
		t.Errorf("PollOnce #1 processed %d requests, want 0 (gated tool must not execute while approval is pending)", count)
	}

	parkRows, err := database.Query(ctx,
		`SELECT status, approval_request_id FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query parked request: %v", err)
	}
	if len(parkRows) != 1 {
		t.Fatalf("tool_request vanished after the poll (rows=%d)", len(parkRows))
	}
	if got := toString(parkRows[0]["status"]); got != "awaiting_approval" {
		t.Fatalf("tool_request status = %q, want 'awaiting_approval' (pre-fix this write FAILED on SQLite and the row stayed 'pending' → executed)", got)
	}
	approvalID := toString(parkRows[0]["approval_request_id"])
	if approvalID == "" {
		t.Fatal("approval_request_id not linked by the park — the §4.4 link gap")
	}

	// No tool_results row may exist while the approval is pending (brief
	// §4.2: "No tool_results row should be created ... until the approval
	// transitions to 'approved'").
	resRows, err := database.Query(ctx, `SELECT COUNT(*) AS cnt FROM tool_results WHERE request_id = $1`, reqID)
	if err != nil {
		t.Fatalf("count tool_results: %v", err)
	}
	if toInt(resRows[0]["cnt"]) != 0 {
		t.Errorf("tool_results rows while approval pending = %d, want 0", toInt(resRows[0]["cnt"]))
	}

	// ---- Approve: ReviewApproval marks the approval 'approved', then the
	// wired follow-through re-queues the parked request; the next poll claims it. ----
	_ = database.Exec(ctx, `UPDATE approval_requests SET status = 'approved' WHERE id = $1`, approvalID)
	if _, err := ResumeApprovedToolRequestsByApproval(ctx, database, approvalID); err != nil {
		t.Fatalf("ResumeApprovedToolRequestsByApproval: %v", err)
	}

	// The re-queued row must stay claimable: the park query must NOT flip it
	// back to 'awaiting_approval' on subsequent cycles (approved approval
	// opens the gate).
	count, err = executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce #2 (approved): %v", err)
	}
	if count != 1 {
		t.Fatalf("PollOnce #2 processed %d requests, want 1 (approved tool must execute)", count)
	}

	doneRows, err := database.Query(ctx, `SELECT status FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query after execute: %v", err)
	}
	if got := toString(doneRows[0]["status"]); got != "completed" {
		t.Errorf("tool_request after approve+poll = %q, want 'completed'", got)
	}

	okRows, _ := database.Query(ctx, `SELECT output, is_error FROM tool_results WHERE request_id = $1`, reqID)
	if len(okRows) == 0 {
		t.Fatal("no tool_result for the approved execution")
	}
	if toInt(okRows[0]["is_error"]) != 0 {
		t.Errorf("approved execution is_error = %d, want 0", toInt(okRows[0]["is_error"]))
	}

	t.Log("park → approve → execute cycle verified through PollOnce")
}

// TestApprovalGated_ExecutorParkAndReject pins the reject arm on the legacy
// path: blocked forever, terminal 'failed', refusal recorded with is_error=1.
func TestApprovalGated_ExecutorParkAndReject(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "sess-te-01"

	_ = database.Exec(ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled, requires_approval)
		 VALUES ('tool-gate-201', 'gated_exec_tool', 'requires human approval', 'external', 'go_native', 'ApprovalHandler', 1, 1)`)

	reqID := seedPendingToolRequest(t, database, sessionID, "gated_exec_tool")
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	// Park it.
	if _, err := executor.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce (park): %v", err)
	}
	parkRows, _ := database.Query(ctx,
		`SELECT status, approval_request_id FROM tool_requests WHERE id = $1`, reqID)
	if got := toString(parkRows[0]["status"]); got != "awaiting_approval" {
		t.Fatalf("premise: tool_request parked = %q, want 'awaiting_approval'", got)
	}
	approvalID := toString(parkRows[0]["approval_request_id"])
	if approvalID == "" {
		t.Fatal("premise: no approval id linked by the park")
	}

	// Simulate the reviewer's rejection exactly as ReviewApproval leaves the
	// row (decision already written by hitl before the callback fires).
	_ = database.Exec(ctx, `UPDATE approval_requests SET status = 'rejected' WHERE id = $1`, approvalID)
	if err := RejectToolRequestsForApproval(ctx, database, approvalID, sessionID, "no production deletions"); err != nil {
		t.Fatalf("RejectToolRequestsForApproval: %v", err)
	}

	// Terminal state: 'failed' with the refusal tool_result.
	failRows, err := database.Query(ctx, `SELECT status FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query after reject: %v", err)
	}
	if got := toString(failRows[0]["status"]); got != "failed" {
		t.Errorf("tool_request after reject = %q, want 'failed'", got)
	}
	refRows, err := database.Query(ctx,
		`SELECT is_error, error_code, output FROM tool_results WHERE request_id = $1`, reqID)
	if err != nil {
		t.Fatalf("query refusal result: %v", err)
	}
	if len(refRows) == 0 {
		t.Fatal("no refusal tool_result row after reject")
	}
	if toInt(refRows[0]["is_error"]) != 1 {
		t.Errorf("refusal is_error = %d, want 1", toInt(refRows[0]["is_error"]))
	}
	if got := toString(refRows[0]["error_code"]); got != "APPROVAL_REFUSED" {
		t.Errorf("refusal error_code = %q, want 'APPROVAL_REFUSED'", got)
	}
	if out := toString(refRows[0]["output"]); !strings.Contains(out, "no production deletions") {
		t.Errorf("refusal output %q should record the reviewer reason", out)
	}

	// And a later poll cycle must NEVER revive it (no resurrect-to-execute).
	if _, err := executor.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce (after reject): %v", err)
	}
	againRows, _ := database.Query(ctx, `SELECT status FROM tool_requests WHERE id = $1`, reqID)
	if got := toString(againRows[0]["status"]); got != "failed" {
		t.Errorf("tool_request after post-reject poll = %q, want 'failed' (rejection must be sticky)", got)
	}
	postRows, _ := database.Query(ctx, `SELECT COUNT(*) AS cnt FROM tool_results WHERE request_id = $1`, reqID)
	if toInt(postRows[0]["cnt"]) != 1 {
		t.Errorf("tool_results rows after post-reject poll = %d, want 1 (only the refusal)", toInt(postRows[0]["cnt"]))
	}

	t.Log("park → reject → blocked-forever + refusal is_error=1 verified through PollOnce")
}
