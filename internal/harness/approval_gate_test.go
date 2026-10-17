// Package harness: APPROV-GATE-001 §4.3 + §4.2 regression tests.
//
// The live planning path never consulted the approval gate: a
// requires_approval tool produced a tool_call_ref staging hand-off with no
// approval_requests row and a session suspended in 'tool_exec' — the external
// consumer received the call straight through (findings.md §4.3, evidence
// raw/03a-planning-toolcall-handoff.txt). This test pins the gate:
//
//   - gated tool → approval_requests row + session 'paused' + NO staging row
//   - approve → session resumes
//   - reject → session stays paused; the linked tool_requests row fails with
//     a tool_result is_error=1 carrying the reviewer's reason
//
// The legacy-path (tool_requests → PollOnce) park/execute/reject cycle is
// covered in approval_gate_executor_test.go.
//
// test=internal/harness/approval_gate_test.go::TestApprovalGatedExecution
package harness

import (
	"context"
	"strings"
	"testing"
)

// planRequestingTool returns an LLM stub output that asks for a tool call —
// the shape the server actually parses into ActionToolCall.
func planRequestingTool(toolName string) *AgentOutput {
	return &AgentOutput{
		InternalMonologue: "I need the gated tool to proceed.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('header', 'requesting tool', 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1', 1)",
		},
		ToolRequests: []ToolRequest{
			{ToolName: toolName, Parameters: map[string]any{"target": "production"}},
		},
	}
}

// TestApprovalGatedExecution drives the planning loop end-to-end with a
// gated tool (approve arm) and a second session (reject arm), then asserts
// the exact DB contract for each decision.
func TestApprovalGatedExecution(t *testing.T) {
	// =========================================================================
	// Approve arm: gated tool → approval row + paused, then approval resumes.
	// =========================================================================
	th, err := newTestHarness(newMockLLM(planRequestingTool("r8_approval_gated_echo")))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	// Register the gated tool (requires_approval=true) and an ungated one to
	// prove the gate distinguishes them.
	_ = th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, requires_approval)
		 VALUES ('tool-gate-01', 'r8_approval_gated_echo', 'gated production tool', 'external', 'go_native', 'noop', 1)`)
	_ = th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref)
		 VALUES ('tool-free-01', 'free_echo', 'ungated echo', 'external', 'go_native', 'noop')`)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set thinking: %v", err)
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2

	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("RunInteractivePlanning (gated): %v", err)
	}
	if result == nil {
		t.Fatal("planning result nil on the gated arm")
	}

	// §4.3 acceptance: the approval request EXISTS.
	approvalRows, err := th.conn.Query(th.ctx,
		`SELECT id, target_tool, status FROM approval_requests WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query approval_requests: %v", err)
	}
	if len(approvalRows) != 1 {
		t.Fatalf("approval_requests rows = %d, want 1 (the gate must create a request)", len(approvalRows))
	}
	if got := toString(approvalRows[0]["target_tool"]); got != "r8_approval_gated_echo" {
		t.Errorf("approval target_tool = %q, want the gated tool", got)
	}
	if got := toString(approvalRows[0]["status"]); got != "pending" {
		t.Errorf("approval status = %q, want 'pending'", got)
	}
	approvalID := toString(approvalRows[0]["id"])

	// §4.3 acceptance: the session is PAUSED (not 'tool_exec'), so no
	// external consumer picks the call up during review.
	sessRows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if got := toString(sessRows[0]["status"]); got != "paused" {
		t.Errorf("session status = %q, want 'paused' while the approval is pending", got)
	}
	if result.NextStatus != "paused" {
		t.Errorf("result.NextStatus = %q, want 'paused'", result.NextStatus)
	}

	// §4.3 defect check: NO staging_buffer tool_call_ref was written — the
	// hand-off that leaked the call to consumers pre-fix.
	stagingRows, err := th.conn.Query(th.ctx,
		`SELECT id FROM staging_buffer WHERE session_id = $1 AND cmd_type = 'tool_call_ref'`, sessionID)
	if err != nil {
		t.Fatalf("query staging_buffer: %v", err)
	}
	if len(stagingRows) != 0 {
		t.Errorf("staging_buffer tool_call_ref rows = %d, want 0 (gated call must not be handed off)", len(stagingRows))
	}

	// The ungated tool proves the gate is selective.
	freePlan := TurnPlan{Action: ActionToolCall, ToolRequests: planRequestingTool("free_echo").ToolRequests}
	freeGated, err := th.planHasGatedTools(th.ctx, sessionID, freePlan)
	if err != nil {
		t.Fatalf("planHasGatedTools(free_echo): %v", err)
	}
	if freeGated {
		t.Error("free_echo wrongly detected as gated")
	}

	// Approve through the HITL Manager — the same wiring cmd/consensus
	// installs (approval callback re-queues the parked tool_request).
	th.hitl.SetApprovalCallback(func(ctx context.Context, aid, sid string) error {
		_, rerr := ResumeApprovedToolRequestsByApproval(ctx, th.conn, aid)
		return rerr
	})
	if err := th.hitl.ReviewApproval(th.ctx, approvalID, "approved", "admin-approver", "go ahead", ""); err != nil {
		t.Fatalf("ReviewApproval(approved): %v", err)
	}

	sessRows, err = th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("re-query session: %v", err)
	}
	if got := toString(sessRows[0]["status"]); got != "idle" {
		t.Errorf("session after approve = %q, want 'idle' (resumed)", got)
	}

	// =========================================================================
	// Reject arm: fresh session; rejection blocks and records the refusal.
	// =========================================================================
	th2, err := newTestHarness(newMockLLM(planRequestingTool("r8_approval_gated_echo")))
	if err != nil {
		t.Fatalf("create harness (reject arm): %v", err)
	}
	defer th2.close()

	_ = th2.conn.Exec(th2.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, requires_approval)
		 VALUES ('tool-gate-02', 'r8_approval_gated_echo', 'gated production tool', 'external', 'go_native', 'noop', 1)`)

	rejectSession, err := th2.createTestSession()
	if err != nil {
		t.Fatalf("create session (reject arm): %v", err)
	}
	_ = th2.conn.Exec(th2.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, rejectSession)

	if _, err := th2.RunInteractivePlanning(th2.ctx, rejectSession, cfg); err != nil {
		t.Fatalf("RunInteractivePlanning (reject arm): %v", err)
	}

	// Seed the legacy-path tool_request the operator would review, mirroring
	// production where the legacy gate parks the row with approval_request_id.
	trRows, _ := th2.conn.Query(th2.ctx,
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
		 VALUES ($1, 1, 'r8_approval_gated_echo', '{}', 'pending')
		 RETURNING id`, rejectSession)
	if len(trRows) == 0 {
		t.Fatal("seed tool_request (reject arm) failed")
	}
	toolRequestID := toInt64(trRows[0]["id"])

	// The pending approval row — the same row shape
	// tools.CreateToolApprovalRequest produces for a gated tool
	// (target_tool set, status pending).
	approval2ID := "approval-reject-arm-0001"
	_ = th2.conn.Exec(th2.ctx,
		`INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, context, target_tool, status, expires_at, created_at)
		 VALUES ($1, $2, 0, 'tool_execution', 'Execute tool "r8_approval_gated_echo"', 'medium', '{}', 'r8_approval_gated_echo', 'pending', datetime('now', '+1 hour'), datetime('now'))`,
		approval2ID, rejectSession)
	_ = th2.conn.Exec(th2.ctx,
		`UPDATE tool_requests SET status = 'awaiting_approval', approval_request_id = $1 WHERE id = $2`,
		approval2ID, toolRequestID)

	// Parked state audit: pre-reject the request holds, no tool_results row.
	preResults, err := th2.conn.Query(th2.ctx,
		`SELECT COUNT(*) AS cnt FROM tool_results WHERE request_id = $1`, toolRequestID)
	if err != nil {
		t.Fatalf("query tool_results (pre-reject): %v", err)
	}
	if toInt(preResults[0]["cnt"]) != 0 {
		t.Errorf("tool_results rows before reject = %d, want 0", toInt(preResults[0]["cnt"]))
	}

	// Run the rejection callback through the hitl Manager — the same wiring
	// cmd/consensus installs.
	th2.hitl.SetRejectionCallback(func(ctx context.Context, aid, sid, reviewer, notes string) error {
		return RejectToolRequestsForApproval(ctx, th2.conn, aid, sid, notes)
	})
	if err := th2.hitl.ReviewApproval(th2.ctx, approval2ID, "rejected", "admin-rejecter", "no production deletions", ""); err != nil {
		t.Fatalf("ReviewApproval(rejected): %v", err)
	}

	// §4.2 acceptance 1: session STAYS paused after reject.
	sessRows, err = th2.conn.Query(th2.ctx, `SELECT status FROM sessions WHERE id = $1`, rejectSession)
	if err != nil {
		t.Fatalf("re-query session (reject arm): %v", err)
	}
	if got := toString(sessRows[0]["status"]); got != "paused" {
		t.Errorf("session after reject = %q, want 'paused' (reject must block)", got)
	}

	// §4.2 acceptance 2: the tool_request is terminal-failed.
	trAfter, err := th2.conn.Query(th2.ctx,
		`SELECT status FROM tool_requests WHERE id = $1`, toolRequestID)
	if err != nil {
		t.Fatalf("query tool_requests after reject: %v", err)
	}
	if got := toString(trAfter[0]["status"]); got != "failed" {
		t.Errorf("tool_request after reject = %q, want 'failed'", got)
	}

	// §4.2 acceptance 3: tool_result with is_error=1 and the reviewer reason.
	resRows, err := th2.conn.Query(th2.ctx,
		`SELECT output, is_error, error_code FROM tool_results WHERE request_id = $1`, toolRequestID)
	if err != nil {
		t.Fatalf("query tool_results after reject: %v", err)
	}
	if len(resRows) == 0 {
		t.Fatal("no tool_result written after reject — the agent cannot learn of the refusal")
	}
	if toInt(resRows[0]["is_error"]) != 1 {
		t.Errorf("refusal tool_result is_error = %d, want 1", toInt(resRows[0]["is_error"]))
	}
	if got := toString(resRows[0]["error_code"]); got != "APPROVAL_REFUSED" {
		t.Errorf("refusal error_code = %q, want 'APPROVAL_REFUSED'", got)
	}
	if output := toString(resRows[0]["output"]); !strings.Contains(output, "no production deletions") {
		t.Errorf("refusal output %q should carry the reviewer reason", output)
	}

	t.Log("APPROV-GATE-001 PASS: gated tool paused before any execution; approve resumes; reject blocks + records refusal is_error=1")
}
