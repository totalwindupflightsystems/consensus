// Package harness: DF-CONSENSUS-083 regression — the tool hand-off budget was
// checked against `sessions.iteration`, a LIFETIME counter bumped by EVERY
// provider call across ALL turns of a session. After ~10 ordinary multi-turn
// messages every later tool hand-off was refused with
// "planning: tool hand-off refused: session iteration budget exhausted (10/10)"
// and the session went status=failed, with no remedy (PATCH accepted only
// `status`).
//
// The fix makes the budget a PER-PLANNING-RUN window: `turn` restarts at 1 on
// every new user message, so multi-turn chat never accumulates against the
// budget, while a single planning run is hard capped at the session's
// `planning_max_turns` turns (the knob is wired into the run loop and PATCH /
// api/v1/sessions/{id} accepts planning_max_turns). `sessions.iteration` keeps
// advancing for billing/progress accounting — it just no longer gates tools.
//
// The scripted LLM is the same stand-in shape as tool_loop_progress_test.go,
// but every turn requests a DIFFERENT command so the identical-repeat guard
// (maxIdenticalToolHandoffs) never trips — the only bound under test is the
// turn budget.
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
	df083ToolName  = "df083_bash"
	df083Handler   = "DF083BashHandler"
	df083Output    = "DF083-OK: command executed"
	df083ModelName = "df083-model"
)

// df083LLM alternates: the first LLM call of each user turn requests the bash
// tool with a turn-unique command; the second call of the turn (the
// continuation after the executor ran the tool) answers. With noOpEveryTurn it
// emits a bare monologue instead — a model that produces neither tools nor
// answers, used to pin the per-run turn budget (the loop stops at the
// session's planning_max_turns and the session lands on the max-turns path,
// not on failure).
type df083LLM struct {
	mu            sync.Mutex
	calls         int
	noOpEveryTurn bool
}

func (l *df083LLM) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	l.mu.Lock()
	l.calls++
	call := l.calls
	l.mu.Unlock()

	base := AgentOutput{
		InternalMonologue:  fmt.Sprintf("df083 scripted call #%d", call),
		MemoryStateChanges: []string{},
		SubAgentSpawns:     []SubAgentSpawn{},
	}
	if l.noOpEveryTurn {
		return &LLMResponse{ModelID: df083ModelName, Output: &base}, nil
	}
	if call%2 == 1 {
		base.ToolRequests = []ToolRequest{{
			ToolName: df083ToolName,
			Parameters: map[string]any{
				"command": fmt.Sprintf("echo df083-turn-%d", (call+1)/2),
			},
		}}
		return &LLMResponse{ModelID: df083ModelName, Output: &base}, nil
	}
	base.SystemActions = []string{"respond"}
	base.MessageToUser = fmt.Sprintf("turn %d done", call/2)
	return &LLMResponse{ModelID: df083ModelName, Output: &base}, nil
}

func (l *df083LLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func df083RegisterTool(t *testing.T, th *testHarness) {
	t.Helper()
	tools.RegisterNativeHandler(df083Handler, func(_ context.Context, _ map[string]any, _ string) (string, error) {
		return df083Output, nil
	})
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		 VALUES ('tool-df083-01', $1, 'DF-CONSENSUS-083 bash stand-in', 'external', 'go_native', $2, 1)`,
		df083ToolName, df083Handler); err != nil {
		t.Fatalf("register tool: %v", err)
	}
}

// df083SeedTurn appends one user message and flips the session to thinking —
// exactly what POST /api/v1/sessions/{id}/message does — on an EXISTING
// session, so several turns hit the same session like ordinary multi-turn chat.
func df083SeedTurn(t *testing.T, th *testHarness, sessionID, instruction string) {
	t.Helper()
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, instruction, sessionID); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}
}

func df083LastAuditError(t *testing.T, th *testHarness, sessionID string) string {
	t.Helper()
	rows, err := th.conn.Query(th.ctx,
		`SELECT error_message FROM audit_logs WHERE session_id = $1 AND error_message IS NOT NULL AND error_message != ''
		 ORDER BY id DESC LIMIT 1`, sessionID)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return toString(rows[0]["error_message"])
}

// TestToolHandoffBudgetIsPerRunNotLifetime is the DF-CONSENSUS-083 scenario:
// six tool-involving user messages to ONE session. Every turn is one planning
// run costing two provider calls (tool hand-off + reply), so the LIFETIME
// session.iteration reaches the default planning_max_turns (10) during turn 5.
// The 6th message asks for a bash call — pre-fix it was refused
// ("session iteration budget exhausted (10/10)") and the session failed;
// post-fix the hand-off proceeds because the per-run window restarted at 1.
func TestToolHandoffBudgetIsPerRunNotLifetime(t *testing.T) {
	llm := &df083LLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	df083RegisterTool(t, th)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	const turns = 6
	for i := 1; i <= turns; i++ {
		df083SeedTurn(t, th, sessionID, fmt.Sprintf("Turn %d: run the turn-%d command with your bash tool, then report.", i, i))
		scn010DriveSteps(t, th, executor, sessionID, 12)
		if status := scn010SessionStatus(t, th, sessionID); status != "idle" {
			t.Fatalf("turn %d: session status = %q, want idle (a refused hand-off fails the session; "+
				"last audit error: %q)", i, status, df083LastAuditError(t, th, sessionID))
		}
	}

	rows, err := th.conn.Query(th.ctx,
		`SELECT tool_name, parameters, status FROM tool_requests WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(rows) != turns {
		t.Fatalf("tool_requests rows = %d, want %d — every turn's hand-off must execute; "+
			"last audit error: %q", len(rows), turns, df083LastAuditError(t, th, sessionID))
	}
	for i, row := range rows {
		if got := fmt.Sprint(row["parameters"]); !strings.Contains(got, fmt.Sprintf("df083-turn-%d", i+1)) {
			t.Errorf("tool_request %d parameters = %s, want the turn-%d command", i+1, got, i+1)
		}
		if got := toString(row["status"]); got != "completed" {
			t.Errorf("tool_request %d status = %q, want completed", i+1, got)
		}
	}

	// The lifetime counter kept advancing (billing/progress accounting) — it is
	// now far above the default budget and the session still worked.
	if iteration := scn010Iteration(t, th, sessionID); iteration < 2*turns {
		t.Errorf("sessions.iteration = %d, want >= %d (the counter still advances per hand-off and reply)",
			iteration, 2*turns)
	}
}

// TestPlanningRunTurnBudgetIsEnforcedWithinRun pins the other half of the fix:
// a single planning run is hard capped at the SESSION's planning_max_turns.
// The session declares 4 turns; the caller passes the default config (10) to
// prove the session row — not the default config — governs the run. A model
// that never acts runs out of turns and the session lands on the max-turns
// path (idle + warning), not on a failure. Pre-fix the run loop ignored
// planning_max_turns entirely and consumed all 10 default turns.
func TestPlanningRunTurnBudgetIsEnforcedWithinRun(t *testing.T) {
	llm := &df083LLM{noOpEveryTurn: true}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking', planning_max_turns = 4 WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set planning_max_turns: %v", err)
	}

	cfg := DefaultPlanningConfig() // MaxTurns = 10 (the spec default)
	if _, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg); err != nil {
		t.Fatalf("RunInteractivePlanning: %v", err)
	}

	if got := llm.callCount(); got != 4 {
		t.Fatalf("LLM calls = %d, want 4 (the run must stop at the session's planning_max_turns, not the caller default of 10)", got)
	}
	if status := scn010SessionStatus(t, th, sessionID); status != "idle" {
		t.Fatalf("session status = %q, want idle (a run that exhausts its turn budget is a max-turns warning, not a failure)", status)
	}
}
