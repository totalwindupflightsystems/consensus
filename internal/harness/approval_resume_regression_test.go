package harness

import (
	"context"
	"testing"
)

func TestPlanningApprovalExecutesAndContinues(t *testing.T) {
	const name = "approval_resume_probe"
	llm := &df65LLM{outputs: []*AgentOutput{
		{InternalMonologue: "Run probe", ToolRequests: []ToolRequest{{ToolName: name, Parameters: map[string]any{"q": "probe"}}}},
		{InternalMonologue: "Done", SystemActions: []string{"respond"}, MessageToUser: "Probe completed."},
	}}
	th := df65NewHarness(t, llm)
	calls := df65RegisterTool(t, th, "approval-resume-probe", name, "ApprovalResumeProbeHandler")
	if err := th.conn.Exec(th.ctx, `UPDATE tools_registry SET requires_approval = true WHERE name = $1`, name); err != nil {
		t.Fatal(err)
	}
	sid, err := th.createTestSession()
	if err != nil {
		t.Fatal(err)
	}
	df65SeedUserMessage(t, th, sid, "Run the probe")
	result, err := th.RunInteractivePlanning(th.ctx, sid, DefaultPlanningConfig())
	if err != nil || result == nil || result.NextStatus != "paused" {
		t.Fatalf("park: %+v %v", result, err)
	}
	rows, err := th.conn.Query(th.ctx, `SELECT id, approval_request_id, parameters FROM tool_requests WHERE session_id = $1 AND status = 'awaiting_approval'`, sid)
	if err != nil || len(rows) != 1 {
		t.Fatalf("parked request: %+v %v", rows, err)
	}
	aid := toString(rows[0]["approval_request_id"])
	if aid == "" || calls.Load() != 0 {
		t.Fatalf("approval=%q calls=%d", aid, calls.Load())
	}
	th.hitl.SetApprovalCallback(func(ctx context.Context, aid, sid string) error {
		_, err := ResumeApprovedToolRequestsByApproval(ctx, th.conn, aid)
		return err
	})
	if err := th.hitl.ReviewApproval(th.ctx, aid, "approved", "reviewer", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := df65SessionStatus(t, th, sid); got != "tool_exec" {
		t.Fatalf("after review: %s", got)
	}
	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	n, err := executor.PollOnce(th.ctx)
	if err != nil || n != 1 || calls.Load() != 1 {
		t.Fatalf("execution: processed=%d calls=%d err=%v", n, calls.Load(), err)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_requests q JOIN tool_results r ON r.request_id=q.id WHERE q.session_id=$1 AND q.status='completed' AND r.is_error=0`, sid); got != 1 {
		t.Fatalf("successful persisted results: %d", got)
	}
	result, err = th.RunInteractivePlanning(th.ctx, sid, DefaultPlanningConfig())
	if err != nil || result == nil {
		t.Fatalf("continuation: %+v %v", result, err)
	}
	if got := df65SessionStatus(t, th, sid); got != "idle" {
		t.Fatalf("final status: %s", got)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM memory_events WHERE session_id=$1 AND type='text_block'`, sid); got != 1 {
		t.Fatalf("assistant replies: %d", got)
	}
}
