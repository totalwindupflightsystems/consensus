package harness

import (
	"strings"
	"testing"
	"time"
)

// DF-CONSENSUS-6-R2: multi-turn chat is stateless across planning runs.
//
// Turn 1 (user asks, agent answers via respond, iteration settles) is followed
// by turn 2 (a NEW user message, a NEW planning run). Pre-fix,
// buildPlanningMessages emitted only system + turn context + the pending
// question + tool exchanges: the turn-1 user_message was consumed at
// iteration 1 (display_modes 'hidden', set_by_iteration=1) and is excluded
// from every later context read, and the turn-1 assistant reply only ever
// appeared as ledger prose inside the context blob — never as the exchange the
// model is replying to. The recorded turn-2 provider request therefore lacked
// the turn-1 user text entirely, so the model could not answer
// "what is my favorite number?".
//
// Two halves, asserted separately (the reply-persistence half may already be
// satisfied by DF-CONSENSUS-084's handleRespond/handleCommitV2 text_block
// writes — the RED-proof must say which half actually fails):
//
//  1. history-carrying: the turn-2 LLM request must literally contain the
//     turn-1 user text and the turn-1 assistant reply.
//  2. reply persistence: both assistant replies are durable text_block
//     memory_events, so GET /api/v1/sessions/{id}/memory lists both turns and
//     last_message (newest text_block) is a reply, not planner text.

const (
	df006Turn1Question = "My favorite number is 41"
	// Deliberately number-free: if the turn-1 reply contained "41" it could
	// leak into the turn-2 prompt via the context blob's memory prose and make
	// the history assertion pass for the wrong reason. Only the turn-1
	// QUESTION carries the number the model must remember.
	df006Turn1Answer   = "Got it — I will remember it."
	df006Turn2Question = "what is my favorite number?"
	df006Turn2Answer   = "Your favorite number is 41."
)

// sendDF006UserMessage mirrors POST /api/v1/sessions/{id}/message for an idle
// session (internal/api/sessions.go handleSessionMessage): the ledger row is
// stamped with the next iteration and the session wakes to 'thinking' with its
// iteration bumped.
func sendDF006UserMessage(t *testing.T, th *testHarness, sessionID, content string) {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("read session iteration: %v", err)
	}
	next := toInt64(rows[0]["iteration"]) + 1
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, CURRENT_TIMESTAMP)`,
		content, sessionID, next); err != nil {
		t.Fatalf("insert user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking', iteration = $2, completed_at = NULL WHERE id = $1`,
		sessionID, next); err != nil {
		t.Fatalf("wake session: %v", err)
	}
}

func TestDF006SecondPlanningRunCarriesFirstTurnExchange(t *testing.T) {
	llm := &df084LLM{outputs: []*AgentOutput{
		{ // turn 1: answer and settle the iteration
			InternalMonologue:  "A number to remember.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      df006Turn1Answer,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		{ // turn 2: answer from the conversation, not from thin air
			InternalMonologue:  "The conversation already contains the answer.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      df006Turn2Answer,
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
	}}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness
	h.HeartbeatConfig.Interval = 2 * time.Minute

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Turn 1: ask, answer, iteration settles idle.
	sendDF006UserMessage(t, th, sessionID, df006Turn1Question)
	if _, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig()); err != nil {
		t.Fatalf("planning turn 1: %v", err)
	}
	if got := llm.callCount(); got != 1 {
		t.Fatalf("LLM calls after turn 1 = %d, want 1", got)
	}

	// Turn 2: a NEW user message after the first turn settled — a fresh
	// planning run that must still see the first exchange.
	sendDF006UserMessage(t, th, sessionID, df006Turn2Question)
	if _, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig()); err != nil {
		t.Fatalf("planning turn 2: %v", err)
	}
	if got := llm.callCount(); got != 2 {
		t.Fatalf("LLM calls after turn 2 = %d, want 2", got)
	}

	// HALF 1 — history-carrying. The recorded turn-2 request must literally
	// contain the turn-1 exchange: the user text (with the number) and the
	// assistant reply.
	prompt := llm.prompt(1)
	if !strings.Contains(prompt, df006Turn1Question) {
		t.Errorf("turn-2 provider request does not contain the turn-1 user text %q — the chat is stateless across planning runs:\n%s",
			df006Turn1Question, prompt)
	}
	if !strings.Contains(prompt, df006Turn1Answer) {
		t.Errorf("turn-2 provider request does not contain the turn-1 assistant reply %q:\n%s",
			df006Turn1Answer, prompt)
	}

	// HALF 2 — reply persistence. Both assistant replies are durable
	// text_block events, in order.
	blocks := textBlocks(t, th, sessionID)
	if len(blocks) != 2 || blocks[0] != df006Turn1Answer || blocks[1] != df006Turn2Answer {
		t.Errorf("text_block rows = %#v, want [%q %q] — a planning turn's reply never persisted",
			blocks, df006Turn1Answer, df006Turn2Answer)
	}

	// And the memory read-back lists both turns: user questions AND replies.
	turns, err := th.conn.Query(th.ctx, `
		SELECT type, content FROM memory_events
		WHERE session_id = $1 AND type IN ('user_message', 'text_block')
		ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query memory events: %v", err)
	}
	want := []struct{ typ, content string }{
		{"user_message", df006Turn1Question},
		{"text_block", df006Turn1Answer},
		{"user_message", df006Turn2Question},
		{"text_block", df006Turn2Answer},
	}
	if len(turns) != len(want) {
		t.Fatalf("persisted conversational events = %d rows, want %d: %#v", len(turns), len(want), turns)
	}
	for i, w := range want {
		if gotType, gotContent := toString(turns[i]["type"]), toString(turns[i]["content"]); gotType != w.typ || gotContent != w.content {
			t.Errorf("memory event %d = (%s, %q), want (%s, %q)", i, gotType, gotContent, w.typ, w.content)
		}
	}

	t.Logf("DF-CONSENSUS-6-R2 PASS: turn-2 request carried the turn-1 exchange; both replies persisted as text_block events")
}
