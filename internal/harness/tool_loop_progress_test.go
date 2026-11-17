// Package harness: SCN-CONSENSUS-010 regression.
//
// The defect (live, scripts/e2e/harness-scenarios.sh s3_bash, 2026-11-16 23:10):
// planning emitted a bash tool call, the executor ran it and wrote
// tool_results, the session was handed back to planning — and the model
// re-issued the IDENTICAL call 16 times in ~60s. `sessions.iteration` stayed 1
// and every continuation restarted its planning turn counter at 1, so the
// `max_turns=10` guard could never trip: a tool that SUCCEEDS but is re-planned
// without its result being seen as an action already taken is an unbounded
// loop.
//
// The two mechanisms pinned here:
//
//  1. the continuation turn carries the prior assistant tool call AND its
//     result (a transcript of what happened), not just a results blob;
//  2. a tool hand-off advances sessions.iteration, so a session can spend its
//     declared iteration budget and a repeat can be bounded at all.
//
// The scripted LLM below is a faithful stand-in for the live model: it asks for
// the same bash command until the prompt proves that command already ran. On
// the pre-fix tree it never sees its own call, so it loops — which is exactly
// the production behaviour this test turns red.
package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/wojons/consensus/internal/tools"
)

const (
	scn010Command  = "printf SCN-BASH > scn3.out"
	scn010ToolName = "consensus010_bash"
	scn010Handler  = "Consensus010BashHandler"
	scn010Output   = "SCN010-OK: bash executed exactly once"
	scn010Reply    = "DONE — the command ran and scn3.out was written."
)

// toolLoopLLM replays a model that re-issues the same tool call unless the
// conversation proves the call already ran: it treats the call as pending until
// an assistant turn carrying that exact call is followed by its tool result.
// From there it answers. Prompts are recorded so the test can assert on what
// the continuation turn actually received.
type toolLoopLLM struct {
	mu      sync.Mutex
	prompts []string
	calls   int
	// neverAnswer models a model that ignores every hint and keeps asking for
	// the same call — the shape the repetition bound must stop.
	neverAnswer bool
}

func (l *toolLoopLLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var sb strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	l.prompts = append(l.prompts, sb.String())
	l.calls++

	alreadyRan := transcriptShowsCompletedCall(messages, scn010ToolName)

	if alreadyRan && !l.neverAnswer {
		return &LLMResponse{
			ModelID: "scn010-loop-model",
			Output: &AgentOutput{
				InternalMonologue:  "The command already ran; reporting the result.",
				MemoryStateChanges: []string{},
				SystemActions:      []string{"respond"},
				MessageToUser:      scn010Reply,
				ToolRequests:       []ToolRequest{},
				SubAgentSpawns:     []SubAgentSpawn{},
			},
		}, nil
	}

	return &LLMResponse{
		ModelID: "scn010-loop-model",
		Output: &AgentOutput{
			InternalMonologue:  "I need to run the command.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests: []ToolRequest{
				{ToolName: scn010ToolName, Parameters: map[string]any{"command": scn010Command}},
			},
			SubAgentSpawns: []SubAgentSpawn{},
		},
	}, nil
}

func (l *toolLoopLLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func (l *toolLoopLLM) prompt(i int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.prompts) {
		return ""
	}
	return l.prompts[i]
}

// transcriptShowsCompletedCall reports whether the message list contains an
// assistant turn carrying the named tool call followed by a tool result for it.
// That is the only thing the model needs to stop re-issuing.
func transcriptShowsCompletedCall(messages []Message, toolName string) bool {
	seenCall := false
	for _, m := range messages {
		if m.Role == "assistant" && strings.Contains(m.Content, fmt.Sprintf(`"tool_name":%q`, toolName)) {
			seenCall = true
			continue
		}
		if seenCall && strings.Contains(m.Content, "Tool result — "+toolName) {
			return true
		}
	}
	return false
}

// scn010RegisterTool registers the deterministic go_native tool the scripted
// plan calls. A go_native handler keeps the test about loop control rather than
// sandbox plumbing, and it makes the number of EXECUTIONS observable (the count
// only advances when the harness actually hands off and the executor runs).
func scn010RegisterTool(t *testing.T, th *testHarness) {
	t.Helper()
	tools.RegisterNativeHandler(scn010Handler, func(_ context.Context, _ map[string]any, _ string) (string, error) {
		return scn010Output, nil
	})
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		 VALUES ('tool-scn010-01', $1, 'SCN-CONSENSUS-010 bash stand-in', 'external', 'go_native', $2, 1)`,
		scn010ToolName, scn010Handler); err != nil {
		t.Fatalf("register tool: %v", err)
	}
}

// scn010SeedSession seeds the user turn and the thinking status the API sets on
// POST /sessions/{id}/message.
func scn010SeedSession(t *testing.T, th *testHarness, instruction string) string {
	t.Helper()
	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, instruction, sessionID); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}
	return sessionID
}

func scn010SessionStatus(t *testing.T, th *testHarness, sessionID string) string {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("query session: %v (rows=%d)", err, len(rows))
	}
	return toString(rows[0]["status"])
}

func scn010Iteration(t *testing.T, th *testHarness, sessionID string) int64 {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("query iteration: %v (rows=%d)", err, len(rows))
	}
	return toInt64(rows[0]["iteration"])
}

// scn010DriveSteps runs the same cycle the production heartbeat owns — planning
// dispatch, then tool execution — but synchronously, so the assertions do not
// race a background goroutine. It stops on idle/failed or after maxSteps.
func scn010DriveSteps(t *testing.T, th *testHarness, executor *ToolExecutorImpl, sessionID string, maxSteps int) {
	t.Helper()
	for step := 0; step < maxSteps; step++ {
		switch scn010SessionStatus(t, th, sessionID) {
		case "idle", "failed":
			return
		case "thinking", "planning":
			if _, err := th.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig()); err != nil {
				t.Fatalf("planning step %d: %v", step, err)
			}
		case "tool_exec":
			if _, err := executor.PollOnce(th.ctx); err != nil {
				t.Fatalf("executor step %d: %v", step, err)
			}
		default:
			return
		}
	}
}

// TestContinuationCarriesToolCallAndAdvancesIteration is the SCN-CONSENSUS-010
// regression: one user instruction, one tool call, one execution, one reply.
func TestContinuationCarriesToolCallAndAdvancesIteration(t *testing.T) {
	const instruction = "Use your bash tool to run: printf SCN-BASH > scn3.out ; then report DONE."

	llm := &toolLoopLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	scn010RegisterTool(t, th)
	sessionID := scn010SeedSession(t, th, instruction)

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	scn010DriveSteps(t, th, executor, sessionID, 12)

	// 1. The tool ran exactly once. Before the fix the identical call was
	//    re-handed-off on every continuation (16x live, bounded only by the
	//    scenario timeout).
	rows, err := th.conn.Query(th.ctx,
		`SELECT id, status FROM tool_requests WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1 (the identical bash call must be executed once; "+
			"a second row is the re-issue loop — SCN-CONSENSUS-010)", len(rows))
	}
	if got := toString(rows[0]["status"]); got != "completed" {
		t.Fatalf("tool_request status = %q, want 'completed'", got)
	}

	// 2. The session progressed to a real reply instead of looping.
	status := scn010SessionStatus(t, th, sessionID)
	if status != "idle" {
		t.Fatalf("session status = %q, want 'idle' (pre-fix it never leaves thinking/tool_exec: the loop)", status)
	}
	blocks, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block' ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query text_block rows: %v", err)
	}
	if len(blocks) != 1 || toString(blocks[0]["content"]) != scn010Reply {
		t.Fatalf("assistant text_block rows = %d (content=%v), want exactly the reply %q",
			len(blocks), blocks, scn010Reply)
	}

	// 3. The continuation turn carried BOTH the assistant tool call and its
	//    result. DF-CONSENSUS-070 injected only the output; the model needs to
	//    see the call as an action it already took.
	continuation := llm.prompt(1)
	if !strings.Contains(continuation, fmt.Sprintf(`"tool_name":%q`, scn010ToolName)) {
		t.Errorf("continuation prompt is missing the prior ASSISTANT tool call:\n%s", continuation)
	}
	if !strings.Contains(continuation, scn010Command) {
		t.Errorf("continuation prompt is missing the tool call PARAMETERS (%q):\n%s", scn010Command, continuation)
	}
	if !strings.Contains(continuation, scn010Output) {
		t.Errorf("continuation prompt is missing the tool RESULT (%q):\n%s", scn010Output, continuation)
	}

	// 4. sessions.iteration advanced on the tool hand-off (0 → 1) and again on
	//    the reply (1 → 2). Pre-fix it stayed at 0 forever, which is why
	//    max_turns could never trip and GET /sessions/{id} reported no progress.
	iteration := scn010Iteration(t, th, sessionID)
	if iteration < 2 {
		t.Fatalf("sessions.iteration = %d, want >= 2 (one tool hand-off + one reply must each advance it)", iteration)
	}

	if got := llm.callCount(); got != 2 {
		t.Errorf("LLM calls = %d, want 2 (one to plan the tool call, one continuation turn to answer)", got)
	}
}

// TestRepeatedIdenticalToolHandoffFailsVisibly pins requirement 3: a model that
// ignores the transcript and keeps asking for the same call must be stopped
// explicitly — bounded executions, a named error in the audit log, and a
// 'failed' session. Never a silent 'completed' and never an endless loop.
func TestRepeatedIdenticalToolHandoffFailsVisibly(t *testing.T) {
	const instruction = "Use your bash tool to run: printf SCN-BASH > scn3.out"

	llm := &toolLoopLLM{neverAnswer: true}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	scn010RegisterTool(t, th)
	sessionID := scn010SeedSession(t, th, instruction)

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	scn010DriveSteps(t, th, executor, sessionID, 12)

	rows, err := th.conn.Query(th.ctx,
		`SELECT COUNT(*) AS cnt FROM tool_requests WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("count tool_requests: %v", err)
	}
	executed := toInt(rows[0]["cnt"])
	if executed == 0 {
		t.Fatal("no tool_requests rows: the first hand-off must still execute")
	}
	if executed > maxIdenticalToolHandoffs {
		t.Fatalf("tool_requests rows = %d, want <= %d (identical repeats must be refused, "+
			"not executed forever)", executed, maxIdenticalToolHandoffs)
	}

	if status := scn010SessionStatus(t, th, sessionID); status != "failed" {
		t.Fatalf("session status = %q, want 'failed' (a refused hand-off must fail visibly, "+
			"never resume silently)", status)
	}

	audit, err := th.conn.Query(th.ctx,
		`SELECT error_message FROM audit_logs WHERE session_id = $1 ORDER BY id DESC LIMIT 1`, sessionID)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if len(audit) == 0 {
		t.Fatal("no audit_logs row: the refusal must be recorded")
	}
	msg := toString(audit[0]["error_message"])
	if !strings.Contains(msg, "tool hand-off refused") {
		t.Fatalf("audit error_message = %q, want it to name the refused tool hand-off", msg)
	}
}

// TestCountIdenticalHandoffCycles pins the repeat-detection arithmetic,
// including the batch case (one hand-off = N rows, newest-first on read).
func TestCountIdenticalHandoffCycles(t *testing.T) {
	a := toolCallSignature{ToolName: "bash", Parameters: `{"command":"ls"}`}
	b := toolCallSignature{ToolName: "bash", Parameters: `{"command":"pwd"}`}
	c := toolCallSignature{ToolName: "read", Parameters: `{"path":"x"}`}

	cases := []struct {
		name  string
		calls []toolCallSignature
		prior []toolCallSignature // newest-first
		want  int
	}{
		{name: "no history", calls: []toolCallSignature{a}, prior: nil, want: 0},
		{name: "different call before", calls: []toolCallSignature{a}, prior: []toolCallSignature{b, a}, want: 0},
		{name: "one prior handoff", calls: []toolCallSignature{a}, prior: []toolCallSignature{a}, want: 1},
		{name: "two prior handoffs", calls: []toolCallSignature{a}, prior: []toolCallSignature{a, a}, want: 2},
		{name: "run stops at a different call", calls: []toolCallSignature{a}, prior: []toolCallSignature{a, a, b, a, a}, want: 2},
		{name: "batch prior handoff", calls: []toolCallSignature{a, b}, prior: []toolCallSignature{b, a, b, a}, want: 2},
		{name: "batch partial match", calls: []toolCallSignature{a, b}, prior: []toolCallSignature{b, c, b, a}, want: 0},
		{name: "unrelated history", calls: []toolCallSignature{a}, prior: []toolCallSignature{c, c, c}, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countIdenticalHandoffCycles(tc.calls, tc.prior); got != tc.want {
				t.Fatalf("countIdenticalHandoffCycles(%v, %v) = %d, want %d", tc.calls, tc.prior, got, tc.want)
			}
		})
	}
}
