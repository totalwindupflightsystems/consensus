// Package harness: DF-CONSENSUS-65 / DF-CONSENSUS-68 dispatch-contract
// regression.
//
// THE DEFECT (fixed by 73d3c972): the tool-call branch of
// RunInteractivePlanning rolled the planning transaction back and then only
// wrote status='tool_exec' on the session. No tool_requests row was staged, so
// the executor had nothing to claim: the tool never ran, no tool_result was
// ever written, and the session rested in 'tool_exec' forever. Because the
// planning claim only accepts status='thinking', every later heartbeat bail
// out with "already claimed by another worker" — the session was a zombie.
//
// This file pins the post-fix contract. Coverage map (see DF-CONSENSUS-068):
//
//	(1) hand-off persists exactly one pending tool_requests row per requested
//	    tool, OUTSIDE the rolled-back planning transaction (committed, so an
//	    independent connection observes them)
//	        → TestDF65PlanningHandoffPersistsPendingRequestsOutsideRolledBackTx
//	(3) after execution the owning session leaves 'tool_exec' for a terminal
//	    state ('idle') — it is never parked there
//	        → TestDF65ExecutedHandoffLeavesToolExecAndReachesIdle
//	(5) a row already claimed/executed by another claimant is NOT executed
//	    twice (and re-polling completed work is a no-op)
//	        → TestDF65ClaimedRequestIsNotExecutedTwice
//	(2) session scoping — PARTIAL, see below
//	        → TestDF65HandBackIsSessionScopedToTheOwningSession
//	(4) nil ToolExecutor is terminal ('failed') — NOT PINNED, see below
//
// DEFERRED, with the production facts that force it:
//
//   - (2) strict form ("a pending row belonging to another session is never
//     executed"). The current executor has no session-scoped entry point:
//     ToolExecutorImpl.PollOnce claims globally —
//     `SELECT ... FROM tool_requests WHERE status='pending' ORDER BY created_at
//     LIMIT 10` (internal/harness/tool_executor.go:206-212) — and
//     `dispatchPendingToolRequests` / any `ExecutePendingForSession` never
//     existed in this repository's history (`git log -S` finds nothing).
//     Asserting the strict form would fail against production, so the landed
//     test pins the part that IS the contract of the session hand-back: serving
//     one session's rows must not execute, disturb, or wake a DIFFERENT
//     session's claimed row.
//
//   - (4) nil ToolExecutor. `Harness` has no ToolExecutor field at all (only
//     ToolExecutorImpl calls the poll loop, started by cmd wiring), so "nil
//     executor ⇒ terminal 'failed'" is not representable in this code, and no
//     code path writes 'failed' for a missing executor. Pinning it would be
//     inventing behavior; it is reported as an open gap instead.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/tools"
)

// df65LLM replays a scripted list of AgentOutputs, one per Call, and records
// the prompt of each call. It never touches the network: the test is hermetic
// and runs under -short.
type df65LLM struct {
	mu      sync.Mutex
	prompts []string
	outputs []*AgentOutput
}

func (l *df65LLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	l.prompts = append(l.prompts, sb.String())

	// Past the script, replay the last turn so a buggy extra iteration shows up
	// as a wrong call count instead of a hang.
	idx := len(l.prompts) - 1
	if idx >= len(l.outputs) {
		idx = len(l.outputs) - 1
	}
	return &LLMResponse{Output: l.outputs[idx], ModelID: "df65-scripted-model"}, nil
}

func (l *df65LLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prompts)
}

// df65NewHarness builds a harness whose heartbeat loop is out of the way (the
// tests dispatch explicitly) and registers the caller's cleanup.
func df65NewHarness(t *testing.T, llm LLMClient) *testHarness {
	t.Helper()

	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	t.Cleanup(th.close)
	th.HeartbeatConfig.Interval = 2 * time.Minute
	return th
}

// df65RegisterTool registers a go_native tool and returns a counter of how many
// times its handler actually ran. Counting the handler (not the row status) is
// what makes "executed twice" observable.
func df65RegisterTool(t *testing.T, th *testHarness, id, name, handlerRef string) *atomic.Int32 {
	t.Helper()

	var calls atomic.Int32
	tools.RegisterNativeHandler(handlerRef, func(_ context.Context, params map[string]any, _ string) (string, error) {
		calls.Add(1)
		return fmt.Sprintf("%s ran with params=%v", handlerRef, params), nil
	})
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		 VALUES ($1, $2, 'DF-CONSENSUS-68 dispatch-contract tool', 'external', 'go_native', $3, 1)`,
		id, name, handlerRef); err != nil {
		t.Fatalf("register tool %s: %v", name, err)
	}
	return &calls
}

// df65CreateSession inserts a session with an explicit id (createTestSession
// hardcodes one, so a test needing two sessions must not use it).
func df65CreateSession(t *testing.T, th *testHarness, id, status string) {
	t.Helper()

	if err := th.conn.Exec(th.ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal)
		 VALUES ($1, 'test-agent', 'test-model', $2, 'high', 'DF-CONSENSUS-68 dispatch contract')`,
		id, status); err != nil {
		t.Fatalf("create session %s: %v", id, err)
	}
}

// df65SeedUserMessage writes the user_message a real turn starts from and moves
// the session to 'thinking' (the status the dispatcher claims).
func df65SeedUserMessage(t *testing.T, th *testHarness, sessionID, question string) {
	t.Helper()

	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, question, sessionID); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}
}

// df65SessionStatus reads the live status of one session.
func df65SessionStatus(t *testing.T, th *testHarness, sessionID string) string {
	t.Helper()

	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session %s status: %v", sessionID, err)
	}
	if len(rows) != 1 {
		t.Fatalf("session %s rows = %d, want 1", sessionID, len(rows))
	}
	return toString(rows[0]["status"])
}

// df65Count runs a COUNT(*) query and returns the scalar.
func df65Count(t *testing.T, th *testHarness, query string, args ...any) int {
	t.Helper()

	rows, err := th.conn.Query(th.ctx, query, args...)
	if err != nil {
		t.Fatalf("count query failed: %v (SQL: %s)", err, query)
	}
	if len(rows) == 0 {
		return 0
	}
	return toInt(rows[0]["cnt"])
}

// df65SeedRequest inserts a tool_request directly, for the pre-claimed and
// pre-completed fixtures the executor must leave alone.
func df65SeedRequest(t *testing.T, th *testHarness, sessionID, toolName, status string, executedAt any) int64 {
	t.Helper()

	if err := th.conn.Exec(th.ctx, `
		INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, executed_at)
		VALUES ($1, 1, $2, '{}', $3, $4)`, sessionID, toolName, status, executedAt); err != nil {
		t.Fatalf("seed tool_request (%s/%s): %v", status, toolName, err)
	}
	rows, err := th.conn.Query(th.ctx,
		`SELECT id FROM tool_requests WHERE session_id = $1 AND tool_name = $2 ORDER BY id DESC LIMIT 1`,
		sessionID, toolName)
	if err != nil || len(rows) == 0 {
		t.Fatalf("read back seeded tool_request: %v", err)
	}
	return toInt64(rows[0]["id"])
}

// TestDF65PlanningHandoffPersistsPendingRequestsOutsideRolledBackTx pins
// contract item 1: the tool-call branch of RunInteractivePlanning must persist
// exactly ONE pending tool_requests row per requested tool, and those rows must
// survive the planning transaction rollback — i.e. be written in the durable
// hand-off, not inside the iteration tx that is rolled back.
//
// Pre-fix 73d3c972 this test fails at the row assertion: zero rows exist while
// the session is already parked in 'tool_exec'.
func TestDF65PlanningHandoffPersistsPendingRequestsOutsideRolledBackTx(t *testing.T) {
	const (
		question = "Run both probes and report."
		toolA    = "consensus068_alpha"
		toolB    = "consensus068_beta"
		refA     = "DF65AlphaHandler"
		refB     = "DF65BetaHandler"
	)

	llm := &df65LLM{outputs: []*AgentOutput{{
		InternalMonologue:  "Two independent probes are needed.",
		MemoryStateChanges: []string{},
		SystemActions:      []string{},
		ToolRequests: []ToolRequest{
			{ToolName: toolA, Parameters: map[string]any{"q": "alpha"}},
			{ToolName: toolB, Parameters: map[string]any{"q": "beta"}},
		},
		SubAgentSpawns: []SubAgentSpawn{},
	}}}

	th := df65NewHarness(t, llm)
	h := th.Harness
	df65RegisterTool(t, th, "tool-068-01", toolA, refA)
	df65RegisterTool(t, th, "tool-068-02", toolB, refB)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	df65SeedUserMessage(t, th, sessionID, question)

	// The planning turn asks for tools, so it must hand off instead of
	// committing a reply.
	result, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig())
	if err != nil {
		t.Fatalf("planning turn: %v", err)
	}
	if result == nil {
		t.Fatal("planning turn returned nil result (session was not claimed)")
	}
	if result.NextStatus != "tool_exec" {
		t.Fatalf("planning NextStatus = %q, want 'tool_exec' (tool hand-off)", result.NextStatus)
	}
	if got := llm.callCount(); got != 1 {
		t.Fatalf("LLM calls after the hand-off turn = %d, want 1", got)
	}

	rows, err := th.conn.Query(th.ctx,
		`SELECT id, tool_name, parameters, status, iteration_id
		 FROM tool_requests WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("tool_requests rows = %d, want 2 (exactly one pending row per requested tool); "+
			"0 rows with the session already in 'tool_exec' is the pre-73d3c972 defect", len(rows))
	}

	seen := make(map[string]int, 2)
	for _, row := range rows {
		name := toString(row["tool_name"])
		seen[name]++
		if got := toString(row["status"]); got != "pending" {
			t.Errorf("tool_request %s status = %q, want 'pending' (the executor claims 'pending')", name, got)
		}
		if got := toInt64(row["iteration_id"]); got != 0 {
			t.Errorf("tool_request %s iteration_id = %d, want 0 (the SESSION iteration that issued the request, "+
				"docs/API.md §ToolRequest — s0 is the first planning iteration); SCN-CONSENSUS-010 moved this off the "+
				"planning-loop turn number, which restarts at 1 on every continuation", name, got)
		}
	}
	for _, name := range []string{toolA, toolB} {
		if seen[name] != 1 {
			t.Errorf("tool_request rows for %s = %d, want exactly 1 (got %v)", name, seen[name], seen)
		}
	}

	// Parameters are persisted verbatim, so the executor runs what the model
	// asked for rather than an empty call.
	for i, want := range []string{`"q":"alpha"`, `"q":"beta"`} {
		params := toString(rows[i]["parameters"])
		if !strings.Contains(params, want) {
			t.Errorf("tool_request[%d] parameters = %q, want it to contain %s", i, params, want)
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(params), &decoded); err != nil {
			t.Errorf("tool_request[%d] parameters are not valid JSON (%q): %v", i, params, err)
		}
	}

	// The rollback-survival proof: read the same rows through an INDEPENDENT
	// connection. Only committed data is visible to it, so rows seen here were
	// written outside the rolled-back planning transaction — a row left inside
	// that tx would have vanished with it.
	observer, err := driver.Open(th.ctx, db.Config{URL: "sqlite://" + th.tmpPath})
	if err != nil {
		t.Fatalf("open observer connection: %v", err)
	}
	defer func() { _ = observer.Close() }()

	observed, err := observer.Query(th.ctx,
		`SELECT COUNT(*) AS cnt FROM tool_requests WHERE session_id = $1 AND status = 'pending'`, sessionID)
	if err != nil {
		t.Fatalf("observer query: %v", err)
	}
	if len(observed) == 0 || toInt(observed[0]["cnt"]) != 2 {
		t.Fatalf("independent connection sees %v pending tool_requests, want 2: the hand-off rows must be "+
			"COMMITTED outside the rolled-back planning transaction", observed)
	}

	// Nothing may have executed yet: the hand-off is a queue, not an execution.
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessionID); got != 0 {
		t.Errorf("tool_results rows right after hand-off = %d, want 0", got)
	}
	if got := df65SessionStatus(t, th, sessionID); got != "tool_exec" {
		t.Errorf("session status after hand-off = %q, want 'tool_exec'", got)
	}

	t.Logf("DF-CONSENSUS-68 (1) PASS: %d tools → 2 committed pending tool_requests rows visible to an "+
		"independent connection, session parked in tool_exec awaiting execution", len(rows))
}

// TestDF65ExecutedHandoffLeavesToolExecAndReachesIdle pins contract item 3: once
// the executor has run the handed-off request, the owning session must leave
// 'tool_exec' and reach a terminal state ('idle') — it is never parked there.
//
// The full chain is exercised because 'idle' is only legitimate AFTER the
// continuation turn persists the assistant reply (DF-CONSENSUS-070): a session
// that jumps to 'idle' with the tool output dropped, or that stays in
// 'tool_exec' with zero live rows, both fail here.
func TestDF65ExecutedHandoffLeavesToolExecAndReachesIdle(t *testing.T) {
	const (
		question = "What did the probe find?"
		toolName = "consensus068_probe"
		ref      = "DF65ProbeHandler"
		reply    = "The probe found the dispatch contract intact."
	)

	llm := &df65LLM{outputs: []*AgentOutput{
		{ // turn 1: hand the tool call off
			InternalMonologue:  "I need the probe first.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests:       []ToolRequest{{ToolName: toolName, Parameters: map[string]any{"q": "probe"}}},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		{ // turn 2: continuation turn answering from the tool output
			InternalMonologue:  "I have the result now.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      reply,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
	}}

	th := df65NewHarness(t, llm)
	h := th.Harness
	calls := df65RegisterTool(t, th, "tool-068-03", toolName, ref)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	df65SeedUserMessage(t, th, sessionID, question)

	result, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig())
	if err != nil {
		t.Fatalf("planning turn 1: %v", err)
	}
	if result == nil || result.NextStatus != "tool_exec" {
		t.Fatalf("planning turn 1 = %+v, want NextStatus 'tool_exec'", result)
	}

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("PollOnce processed = %d, want 1", processed)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("tool handler invocations = %d, want 1", got)
	}

	// The park assertion, stated directly: 'tool_exec' after execution is the
	// DF-CONSENSUS-65 zombie state.
	if got := df65SessionStatus(t, th, sessionID); got == "tool_exec" {
		t.Fatalf("session status after execution = 'tool_exec': the session is parked (DF-CONSENSUS-65). " +
			"Nothing would ever re-enter planning for it")
	}

	// Drive the continuation turn and wait for the session to settle.
	h.pollAndDispatch(th.ctx)
	deadline := time.Now().Add(15 * time.Second)
	finalStatus := ""
	for time.Now().Before(deadline) {
		finalStatus = df65SessionStatus(t, th, sessionID)
		h.inFlightMu.Lock()
		inflight := len(h.inFlight)
		h.inFlightMu.Unlock()
		if finalStatus == "idle" && inflight == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if finalStatus != "idle" {
		t.Fatalf("session status after the continuation turn = %q, want 'idle' (terminal)", finalStatus)
	}

	// 'idle' must mean the turn really finished: request completed, exactly one
	// tool_result, and a durable assistant reply — not an empty park renamed.
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_requests
		WHERE session_id = $1 AND status IN ('pending','executing','awaiting_approval')`, sessionID); got != 0 {
		t.Errorf("live tool_requests after the run = %d, want 0", got)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessionID); got != 1 {
		t.Errorf("tool_results rows = %d, want 1", got)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM memory_events
		WHERE session_id = $1 AND type = 'text_block'`, sessionID); got != 1 {
		t.Errorf("assistant text_block rows = %d, want 1 (the terminal turn must persist a reply)", got)
	}
	if got := llm.callCount(); got != 2 {
		t.Errorf("LLM calls = %d, want 2 (hand-off turn + continuation turn)", got)
	}

	t.Logf("DF-CONSENSUS-68 (3) PASS: tool_request executed once → session left tool_exec → continuation " +
		"turn persisted the reply → idle (no park)")
}

// TestDF65HandBackIsSessionScopedToTheOwningSession pins the part of contract
// item 2 that the current architecture can express (see the file header for why
// the strict form — "never executes another session's pending row" — is
// deferred: PollOnce claims globally by design).
//
// What it does pin: executing one session's row must not execute, mutate, or
// wake a DIFFERENT session. Session B holds a row another claimant already
// stamped 'executing'/executed_at; that row is in flight, so B must keep its
// 'tool_exec' state and get no tool_result — the hand-back belongs to session A
// only.
func TestDF65HandBackIsSessionScopedToTheOwningSession(t *testing.T) {
	const (
		toolA   = "consensus068_owned"
		refA    = "DF65OwnedHandler"
		toolB   = "consensus068_foreign"
		refB    = "DF65ForeignHandler"
		sessA   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeee68"
		sessB   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff68"
		stamped = "2026-01-01 00:00:00"
	)

	llm := &df65LLM{outputs: []*AgentOutput{{
		InternalMonologue:  "Only the owned probe is needed.",
		MemoryStateChanges: []string{},
		SystemActions:      []string{},
		ToolRequests:       []ToolRequest{{ToolName: toolA, Parameters: map[string]any{"q": "owned"}}},
		SubAgentSpawns:     []SubAgentSpawn{},
	}}}

	th := df65NewHarness(t, llm)
	h := th.Harness
	ownedCalls := df65RegisterTool(t, th, "tool-068-04", toolA, refA)
	foreignCalls := df65RegisterTool(t, th, "tool-068-05", toolB, refB)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session A: %v", err)
	}
	if sessionID != sessA {
		// createTestSession owns the id it writes; keep the fixture honest.
		t.Logf("session A id = %s", sessionID)
	}
	df65SeedUserMessage(t, th, sessionID, "Check the owned probe.")

	result, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig())
	if err != nil || result == nil || result.NextStatus != "tool_exec" {
		t.Fatalf("planning turn for session A: result=%+v err=%v, want 'tool_exec' hand-off", result, err)
	}

	// Session B: an unrelated session whose request is already claimed by
	// another claimant (executed_at stamped, status 'executing').
	df65CreateSession(t, th, sessB, "tool_exec")
	foreignID := df65SeedRequest(t, th, sessB, toolB, "executing", stamped)

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("PollOnce processed = %d, want 1 (only session A's pending row)", processed)
	}
	if got := ownedCalls.Load(); got != 1 {
		t.Errorf("owned handler invocations = %d, want 1", got)
	}
	if got := foreignCalls.Load(); got != 0 {
		t.Errorf("foreign handler invocations = %d, want 0: session B's claimed row must not be executed", got)
	}

	// Session A ran and moved on.
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_requests
		WHERE session_id = $1 AND status = 'completed'`, sessionID); got != 1 {
		t.Errorf("session A completed tool_requests = %d, want 1", got)
	}
	if got := df65SessionStatus(t, th, sessionID); got != "thinking" {
		t.Errorf("session A status = %q, want 'thinking' (handed back for its continuation turn)", got)
	}

	// Session B is untouched: row still claimed by its own claimant, no result,
	// still holding 'tool_exec' because it has a live row.
	foreignRows, err := th.conn.Query(th.ctx,
		`SELECT status, executed_at FROM tool_requests WHERE id = $1`, foreignID)
	if err != nil {
		t.Fatalf("query session B request: %v", err)
	}
	if len(foreignRows) != 1 {
		t.Fatalf("session B request rows = %d, want 1", len(foreignRows))
	}
	if got := toString(foreignRows[0]["status"]); got != "executing" {
		t.Errorf("session B request status = %q, want 'executing' (untouched)", got)
	}
	if got := toString(foreignRows[0]["executed_at"]); got != stamped {
		t.Errorf("session B request executed_at = %q, want %q (the first claimant's stamp is authoritative)",
			got, stamped)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessB); got != 0 {
		t.Errorf("session B tool_results = %d, want 0", got)
	}
	if got := df65SessionStatus(t, th, sessB); got != "tool_exec" {
		t.Errorf("session B status = %q, want 'tool_exec' (a session with a live row must not be handed back)", got)
	}

	t.Logf("DF-CONSENSUS-68 (2, scoped form) PASS: poll executed only session A's row; session B's claimed row "+
		"(id=%d, executed_at=%s) and 'tool_exec' status were left untouched", foreignID, stamped)
}

// TestDF65ClaimedRequestIsNotExecutedTwice pins contract item 5: claiming is a
// compare-and-set on status='pending', so a row already claimed or executed by
// another claimant is never run a second time, and re-polling completed work is
// a no-op.
func TestDF65ClaimedRequestIsNotExecutedTwice(t *testing.T) {
	const (
		toolName = "consensus068_idempotent"
		ref      = "DF65IdempotentHandler"
		stamped  = "2026-01-01 00:00:00"
		sessB    = "cccccccc-dddd-eeee-ffff-000000000068"
	)

	llm := &df65LLM{outputs: []*AgentOutput{{
		InternalMonologue:  "One probe.",
		MemoryStateChanges: []string{},
		SystemActions:      []string{},
		ToolRequests:       []ToolRequest{{ToolName: toolName, Parameters: map[string]any{"q": "once"}}},
		SubAgentSpawns:     []SubAgentSpawn{},
	}}}

	th := df65NewHarness(t, llm)
	h := th.Harness
	calls := df65RegisterTool(t, th, "tool-068-06", toolName, ref)

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	df65SeedUserMessage(t, th, sessionID, "Run the probe once.")

	result, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig())
	if err != nil || result == nil || result.NextStatus != "tool_exec" {
		t.Fatalf("planning turn: result=%+v err=%v, want 'tool_exec' hand-off", result, err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("reset session status: %v", err)
	}

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("first PollOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("first PollOnce processed = %d, want 1", processed)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("handler invocations after first poll = %d, want 1", got)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessionID); got != 1 {
		t.Fatalf("tool_results after first poll = %d, want 1", got)
	}

	// Re-poll: the request is 'completed', so it must not be claimed again.
	processed, err = executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("second PollOnce: %v", err)
	}
	if processed != 0 {
		t.Errorf("second PollOnce processed = %d, want 0 (completed work must not be re-claimed)", processed)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("handler invocations after re-poll = %d, want 1: an executed request ran twice "+
			"(double execution of a completed row)", got)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessionID); got != 1 {
		t.Errorf("tool_results after re-poll = %d, want 1 (a duplicate result row means it re-executed)", got)
	}

	// And the pre-claimed case: a row another claimant stamped executed_at on is
	// NOT ours to run.
	df65CreateSession(t, th, sessB, "tool_exec")
	claimedID := df65SeedRequest(t, th, sessB, toolName, "executing", stamped)

	processed, err = executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("third PollOnce: %v", err)
	}
	if processed != 0 {
		t.Errorf("third PollOnce processed = %d, want 0 (an already-claimed row must not be claimed)", processed)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("handler invocations after polling an already-claimed row = %d, want 1 (it ran twice)", got)
	}
	claimedRows, err := th.conn.Query(th.ctx,
		`SELECT status, executed_at, completed_at FROM tool_requests WHERE id = $1`, claimedID)
	if err != nil {
		t.Fatalf("query claimed request: %v", err)
	}
	if len(claimedRows) != 1 {
		t.Fatalf("claimed request rows = %d, want 1", len(claimedRows))
	}
	if got := toString(claimedRows[0]["status"]); got != "executing" {
		t.Errorf("claimed request status = %q, want 'executing' (unchanged by the poll)", got)
	}
	if got := toString(claimedRows[0]["executed_at"]); got != stamped {
		t.Errorf("claimed request executed_at = %q, want %q (the first claimant's stamp must not be overwritten)",
			got, stamped)
	}
	if got := df65Count(t, th, `SELECT COUNT(*) AS cnt FROM tool_results WHERE session_id = $1`, sessB); got != 0 {
		t.Errorf("tool_results for the already-claimed request = %d, want 0", got)
	}

	t.Logf("DF-CONSENSUS-68 (5) PASS: pending row executed exactly once; re-poll processed 0; "+
		"already-claimed row (id=%d) left 'executing' with its original executed_at and no result", claimedID)
}
