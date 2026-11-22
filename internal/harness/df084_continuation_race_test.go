// Package harness: DF-CONSENSUS-084 regression.
//
// The defect (dogfood 2026-11-18, session a238a831): a continuation turn after
// a successful tool call was billed (311 completion tokens, serve log
// "llm: response received ... planning: committing turn=1") yet memory_events
// held NO text_block for the iteration, sessions.last_message stayed null, and
// a user message sent while the continuation was in flight ("go") was consumed
// at iteration 2 with no provider call of its own and no answer — it silently
// vanished.
//
// Two defects, both race-window shaped:
//
//  1. handleCommitV2 dropped plan.MessageToUser. handleRespond was the only
//     writer of the durable text_block, so a continuation that ended with
//     {"system_actions":["commit"],"message_to_user":"..."} was billed by the
//     deferred usage flush (DF-CONSENSUS-27 made billing survive the terminal
//     path) while the reply itself never landed.
//
//  2. POST /message on a BUSY session (thinking / planning / tool_exec) stored
//     the ledger row with no wake and no status change. The in-flight turn's
//     context snapshot predates the insert, so it folded the message into its
//     own prompt (window 1) or never saw it at all (window 2) — and once the
//     turn settled idle, nothing ever re-armed the session.
//
// Window 1 (arrived before the continuation read its context) is covered by
// TestRacedMessageFoldedIntoContinuationCommitStillPersistsReply: the raced
// message rides the billed continuation call, and with fix (1) its answer —
// the commit's message_to_user — is persisted.
//
// Window 2 (arrived after the snapshot) is covered by
// TestMessageArrivingMidContinuationGetsItsOwnBilledTurn: the stub inserts the
// race DURING the continuation call, the run settles idle, and the requeue
// pass flips the session back to 'thinking' so the message gets its own
// billed turn whose reply is persisted.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/tools"
)

// df084LLM replays a scripted list of outputs, records the exact prompt of
// every call (the recorder JSONL of the hermetic arm), and can run a caller
// hook per call — the hook is how the window-2 test injects the raced message
// mid-run, deterministically after the run's context snapshot.
type df084LLM struct {
	mu      sync.Mutex
	prompts []string
	outputs []*AgentOutput
	onCall  func(callIdx int)
}

func (l *df084LLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
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

	if l.onCall != nil {
		l.onCall(len(l.prompts) - 1)
	}

	// Never starve the loop: past the script, replay the last output so a
	// buggy extra turn shows up as a wrong call count instead of a hang.
	idx := len(l.prompts) - 1
	if idx >= len(l.outputs) {
		idx = len(l.outputs) - 1
	}
	return &LLMResponse{
		Output:  l.outputs[idx],
		ModelID: "df084-model",
		Usage:   LLMUsage{PromptTokens: 100, CompletionTokens: 50},
	}, nil
}

func (l *df084LLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prompts)
}

func (l *df084LLM) prompt(i int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.prompts) {
		return ""
	}
	return l.prompts[i]
}

const (
	df084Question = "Use the bash tool to compute 17*23 and report the exact stdout"
	df084Answer   = "17*23 = 391"
	df084RacedGo  = "go"
)

// insertRacedUserMessage stores a user_message the way POST /message does for
// a BUSY session: a plain ledger row with iteration_created = current + 1, no
// display_modes row, no wake, no status change.
func insertRacedUserMessage(t *testing.T, th *testHarness, sessionID, content string) {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("read session iteration: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, CURRENT_TIMESTAMP)`,
		content, sessionID, toInt64(rows[0]["iteration"])+1); err != nil {
		t.Fatalf("insert raced message: %v", err)
	}
}

// setupDF084ToolSession runs the shared preamble: register the native tool,
// create the session, seed the question, and run planning turn 1 (which hands
// off to the executor) plus one executor pass (which completes the tool and
// hands the session back to the planner for its continuation turn).
func setupDF084ToolSession(t *testing.T, llm *df084LLM) (*testHarness, *Harness, string) {
	t.Helper()

	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	h := th.Harness
	h.HeartbeatConfig.Interval = 2 * time.Minute

	const (
		handlerRef = "DF084BashHandler"
		toolName   = "df084_bash"
	)
	tools.RegisterNativeHandler(handlerRef, func(_ context.Context, params map[string]any, workspace string) (string, error) {
		return fmt.Sprintf("%s: 391 (params=%v, workspace=%s)", handlerRef, params, workspace), nil
	})
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		 VALUES ('tool-084-01', $1, 'DF-CONSENSUS-084 bash probe', 'external', 'go_native', $2, 1)`,
		toolName, handlerRef); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, df084Question, sessionID); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}

	// Planning turn 1: the model asks for the tool; the hand-off persists the
	// request and moves the session to tool_exec (iteration 0 -> 1).
	if _, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig()); err != nil {
		t.Fatalf("planning turn 1: %v", err)
	}
	if got := llm.callCount(); got != 1 {
		t.Fatalf("LLM calls after planning turn 1 = %d, want 1", got)
	}

	// The executor runs the tool and hands the session back to the planner
	// ('thinking') — the pre-continuation state the race lands in.
	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	if processed, err := executor.PollOnce(th.ctx); err != nil || processed != 1 {
		t.Fatalf("PollOnce: processed=%d err=%v, want 1/nil", processed, err)
	}
	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session status: %v", err)
	}
	if got := toString(rows[0]["status"]); got != "thinking" {
		t.Fatalf("session status after tool completion = %q, want 'thinking' (continuation queued)", got)
	}
	return th, h, sessionID
}

// settleDF084 waits until the session is idle and no dispatch goroutine holds
// it (the DF-CONSENSUS-070 convention).
func settleDF084(t *testing.T, th *testHarness, h *Harness, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
		if err != nil {
			t.Fatalf("query status: %v", err)
		}
		h.inFlightMu.Lock()
		inflight := len(h.inFlight)
		h.inFlightMu.Unlock()
		if toString(rows[0]["status"]) == "idle" && inflight == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %s never settled idle within 15s", sessionID)
}

// billingRows counts the cognition rows in agent_billing — the durable ledger
// the deferred flush writes, one per LLM call.
func billingRows(t *testing.T, th *testHarness, sessionID string) int {
	t.Helper()
	rows, err := th.conn.Query(th.ctx,
		`SELECT COUNT(*) AS cnt FROM agent_billing WHERE session_id = $1 AND category = 'cognition'`, sessionID)
	if err != nil {
		t.Fatalf("query agent_billing: %v", err)
	}
	return toInt(rows[0]["cnt"])
}

// textBlocks returns the persisted assistant replies in insertion order.
func textBlocks(t *testing.T, th *testHarness, sessionID string) []string {
	t.Helper()
	rows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block' ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query text_block rows: %v", err)
	}
	blocks := make([]string, 0, len(rows))
	for _, r := range rows {
		blocks = append(blocks, toString(r["content"]))
	}
	return blocks
}

// messageConsumed reports whether a user_message has a display_modes row —
// markUserMessagesRead stamps every message a planning run saw.
func messageConsumed(t *testing.T, th *testHarness, sessionID, content string) bool {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `
		SELECT COUNT(*) AS cnt FROM memory_events me
		JOIN display_modes dm ON dm.memory_id = me.id
		WHERE me.session_id = $1 AND me.type = 'user_message' AND me.content = $2`,
		sessionID, content)
	if err != nil {
		t.Fatalf("query consumption: %v", err)
	}
	return toInt(rows[0]["cnt"]) > 0
}

// TestRacedMessageFoldedIntoContinuationCommitStillPersistsReply is the
// DF-CONSENSUS-084 window-1 regression: the raced "go" lands between the
// executor hand-back and the continuation run's context read, so the billed
// continuation call sees it — and the continuation answers with COMMIT plus
// message_to_user. Pre-fix, handleCommitV2 dropped the reply: the iteration
// settled with a billed call, a consumed raced message, and NO text_block
// (last_message null) — the exact dogfood a238a831 shape.
func TestRacedMessageFoldedIntoContinuationCommitStillPersistsReply(t *testing.T) {
	llm := &df084LLM{outputs: []*AgentOutput{
		{ // turn 1: plan the tool call and hand it to the executor
			InternalMonologue:  "I need the compute result before answering.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests: []ToolRequest{
				{ToolName: "df084_bash", Parameters: map[string]any{"command": "echo 391"}},
			},
			SubAgentSpawns: []SubAgentSpawn{},
		},
		{ // turn 2: the continuation turn — ANSWER AND FINALIZE in one action,
			// the shape whose message_to_user handleCommitV2 used to drop.
			InternalMonologue:  "The tool printed 391; reporting and closing the iteration.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"commit"},
			MessageToUser:      df084Answer,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
	}}

	th, h, sessionID := setupDF084ToolSession(t, llm)
	defer th.close()

	// THE RACE: the user message lands while the session is 'thinking'
	// (post-hand-back, pre-continuation). POST /message stores it without a
	// wake; the continuation run's context read then folds it into its prompt.
	insertRacedUserMessage(t, th, sessionID, df084RacedGo)

	// The harness dispatches the continuation turn and it answers.
	h.pollAndDispatch(th.ctx)
	settleDF084(t, th, h, sessionID)

	if got := llm.callCount(); got != 2 {
		t.Fatalf("LLM calls = %d, want 2 (hand-off + continuation)", got)
	}

	// AC(d), window-1 arm: the raced message must have been inside the billed
	// continuation call's input — that call's persisted reply IS its answer.
	if prompt := llm.prompt(1); !strings.Contains(prompt, df084RacedGo) {
		t.Errorf("continuation prompt does not contain the raced message %q:\n%s", df084RacedGo, prompt)
	}

	// AC(a): the continuation iteration persisted a text_block — the commit
	// action's message_to_user, exactly what was dropped pre-fix.
	blocks := textBlocks(t, th, sessionID)
	if len(blocks) != 1 {
		t.Fatalf("text_block rows = %v, want exactly [%q]", blocks, df084Answer)
	}
	if blocks[0] != df084Answer {
		t.Errorf("persisted reply = %q, want %q", blocks[0], df084Answer)
	}

	// AC(b): last_message serves the reply through the API contract.
	adminKey := "cs_sk_084_admin_key"
	wakeSeedAdminKey(t, th, adminKey)
	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0", Wake: h.RequestWake})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET session: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var sess struct {
		LastMessage *string `json:"last_message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session response: %v (body=%s)", err, w.Body.String())
	}
	if sess.LastMessage == nil {
		t.Fatal("GET /api/v1/sessions/{id} last_message = null after a billed continuation turn (DF-CONSENSUS-084)")
	}
	if *sess.LastMessage != df084Answer {
		t.Errorf("last_message = %q, want %q", *sess.LastMessage, df084Answer)
	}

	// AC(a) through the memory surface: the text_block is listed for the
	// continuation iteration.
	memReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/memory?type=text_block", nil)
	memReq.Header.Set("Authorization", "Bearer "+adminKey)
	memW := httptest.NewRecorder()
	server.Handler().ServeHTTP(memW, memReq)
	if memW.Code != http.StatusOK {
		t.Fatalf("GET memory: expected 200, got %d: %s", memW.Code, memW.Body.String())
	}
	var mem []struct {
		Type             string `json:"type"`
		Content          string `json:"content"`
		IterationCreated int64  `json:"iteration_created"`
	}
	if err := json.Unmarshal(memW.Body.Bytes(), &mem); err != nil {
		t.Fatalf("decode memory response: %v (body=%s)", err, memW.Body.String())
	}
	if len(mem) != 1 || mem[0].Type != "text_block" || mem[0].Content != df084Answer {
		t.Errorf("memory text_block = %+v, want the continuation reply %q", mem, df084Answer)
	}

	// AC(c): every billed call has its durable row — agent_billing cognition
	// rows == recorder lines == 2.
	if got := billingRows(t, th, sessionID); got != llm.callCount() {
		t.Errorf("agent_billing cognition rows = %d, recorder LLM calls = %d — a billed call exists without a persisted turn", got, llm.callCount())
	}

	// The raced message was consumed by the run that answered it (not stranded).
	if !messageConsumed(t, th, sessionID, df084RacedGo) {
		t.Errorf("raced message %q has no consumption record", df084RacedGo)
	}

	// Negative control: everything was consumed by the settling run, so the
	// requeue pass must NOT arm a follow-up turn (no third call, stays idle).
	h.pollAndDispatch(th.ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
		if err != nil {
			t.Fatalf("query status: %v", err)
		}
		if toString(rows[0]["status"]) != "idle" {
			t.Fatalf("session status = %q after settle; the requeue must not arm a turn when nothing arrived", rows[0]["status"])
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := llm.callCount(); got != 2 {
		t.Errorf("LLM calls = %d after the negative-control pass, want 2 (no phantom follow-up turn)", got)
	}

	t.Logf("DF-CONSENSUS-084 window-1 PASS: raced %q folded into the billed continuation call → commit reply %q persisted, last_message non-null, billing rows %d == recorder calls %d",
		df084RacedGo, blocks[0], billingRows(t, th, sessionID), llm.callCount())
}

// TestMessageArrivingMidContinuationGetsItsOwnBilledTurn is the
// DF-CONSENSUS-084 window-2 regression: the raced message lands AFTER the
// continuation run read its context (injected during the LLM call itself), so
// the run never sees it. Pre-fix the run settled idle with the message
// unconsumed and nothing would ever dispatch the session again — a silently
// stranded message. Post-fix the requeue pass flips the idle session back to
// 'thinking' and the message gets its own billed turn whose reply persists.
func TestMessageArrivingMidContinuationGetsItsOwnBilledTurn(t *testing.T) {
	const (
		continuationReply = "The tool printed 391."
		followUpReply     = "Continuing from your go: the compute is done, stdout was 391."
	)

	llm := &df084LLM{outputs: []*AgentOutput{
		{ // turn 1: plan the tool call
			InternalMonologue:  "I need the compute result before answering.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests: []ToolRequest{
				{ToolName: "df084_bash", Parameters: map[string]any{"command": "echo 391"}},
			},
			SubAgentSpawns: []SubAgentSpawn{},
		},
		{ // turn 2: the continuation turn, answering via respond
			InternalMonologue:  "The tool printed 391; reporting.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      continuationReply,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		{ // turn 3: the follow-up turn the requeue arms, answering the race
			InternalMonologue:  "The user says go; nothing left to do — confirming.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      followUpReply,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
	}}
	// The race is injected DURING the continuation call — strictly after the
	// run read its context snapshot, which is what makes the message invisible
	// to that run.
	var thRef *testHarness
	llm.onCall = func(callIdx int) {
		if callIdx == 1 && thRef != nil {
			insertRacedUserMessage(t, thRef, sessionIDOf(t, thRef), df084RacedGo)
		}
	}

	th, h, sessionID := setupDF084ToolSession(t, llm)
	thRef = th
	defer th.close()

	// Drive the continuation turn. pollAndDispatch applies the terminal idle
	// status and then (post-fix) the requeue flips the session back to
	// 'thinking' for the follow-up turn; keep polling until the follow-up
	// dispatch has claimed and run (claimSession may skip a pass while the
	// previous goroutine is still releasing its in-flight entry).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		h.pollAndDispatch(th.ctx)
		if llm.callCount() >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := llm.callCount(); got < 3 {
		t.Fatalf("LLM calls = %d, want 3: the mid-run message never got its own billed follow-up turn", got)
	}

	// The follow-up call's prompt must carry the raced message.
	if prompt := llm.prompt(2); !strings.Contains(prompt, df084RacedGo) {
		t.Errorf("follow-up prompt does not contain the raced message %q:\n%s", df084RacedGo, prompt)
	}

	settleDF084(t, th, h, sessionID)

	// Terminal: idle, with the raced message consumed by the follow-up run —
	// no further requeue, no runaway loop.
	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query final status: %v", err)
	}
	if got := toString(rows[0]["status"]); got != "idle" {
		t.Fatalf("final session status = %q, want 'idle'", got)
	}
	if !messageConsumed(t, th, sessionID, df084RacedGo) {
		t.Errorf("raced message %q was never consumed by the follow-up run", df084RacedGo)
	}
	h.pollAndDispatch(th.ctx)
	time.Sleep(300 * time.Millisecond)
	if got := llm.callCount(); got != 3 {
		t.Errorf("LLM calls = %d after the final settle pass, want 3 (requeue must stop once the message is consumed)", got)
	}

	// AC(a): both replies persisted, in order.
	blocks := textBlocks(t, th, sessionID)
	if len(blocks) != 2 || blocks[0] != continuationReply || blocks[1] != followUpReply {
		t.Fatalf("text_block rows = %v, want [%q %q]", blocks, continuationReply, followUpReply)
	}

	// AC(b): last_message serves the follow-up reply.
	adminKey := "cs_sk_084_admin_key"
	wakeSeedAdminKey(t, th, adminKey)
	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0", Wake: h.RequestWake})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET session: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var sess struct {
		LastMessage *string `json:"last_message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session response: %v (body=%s)", err, w.Body.String())
	}
	if sess.LastMessage == nil || *sess.LastMessage != followUpReply {
		t.Fatalf("last_message = %v, want %q — the mid-run message's answer is missing", sess.LastMessage, followUpReply)
	}

	// AC(c): billing rows == recorder lines — 3 calls, 3 durable rows.
	if got := billingRows(t, th, sessionID); got != llm.callCount() {
		t.Errorf("agent_billing cognition rows = %d, recorder LLM calls = %d — a billed call exists without a persisted turn", got, llm.callCount())
	}

	t.Logf("DF-CONSENSUS-084 window-2 PASS: mid-run %q → requeue → own billed turn → reply %q persisted, billing rows %d == recorder calls %d",
		df084RacedGo, followUpReply, billingRows(t, th, sessionID), llm.callCount())
}

// sessionIDOf returns the single session id the df084 tests create.
func sessionIDOf(t *testing.T, th *testHarness) string {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT id FROM sessions LIMIT 2`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected exactly one session, got %d rows (err=%v)", len(rows), err)
	}
	return toString(rows[0]["id"])
}
