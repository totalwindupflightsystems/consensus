// Package harness: DF-CONSENSUS-070 regression.
//
// The defect: a user asks a question that needs a tool. Planning hands the call
// to the executor, the executor runs it and marks it completed, and the session
// was flipped straight to 'idle'. Nothing re-entered planning with the tool
// result and no assistant message was ever written, so
// GET /api/v1/sessions/{id} returned last_message = null — while docs/API.md §5
// promises "the durable assistant reply is then available in last_message".
//
// This test pins the whole chain end to end:
//
//	user message → planning hand-off (tool_request) → executor completes it
//	  → session handed back to planning ('thinking', not 'idle')
//	  → continuation turn whose prompt CARRIES the tool output
//	  → assistant text block persisted → GET last_message non-null
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

// toolContinuationLLM replays a scripted list of outputs and records the exact
// prompt of every call: call 1 plans the tool call, call 2 answers from the
// tool result.
type toolContinuationLLM struct {
	mu      sync.Mutex
	prompts []string
	outputs []*AgentOutput
}

func (l *toolContinuationLLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
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

	// Never starve the loop: past the script, replay the last output so a
	// buggy extra turn shows up as a wrong call count instead of a hang.
	idx := len(l.prompts) - 1
	if idx >= len(l.outputs) {
		idx = len(l.outputs) - 1
	}
	return &LLMResponse{Output: l.outputs[idx], ModelID: "tool-continuation-model"}, nil
}

func (l *toolContinuationLLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prompts)
}

func (l *toolContinuationLLM) prompt(i int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.prompts) {
		return ""
	}
	return l.prompts[i]
}

// TestToolResultContinuesTurnAndPersistsAssistantReply is the DF-CONSENSUS-070
// regression: a completed tool_result must drive a continuation planning turn
// that sees the output and persists the assistant reply before the session goes
// idle.
func TestToolResultContinuesTurnAndPersistsAssistantReply(t *testing.T) {
	const (
		question    = "What did the build probe report?"
		toolName    = "consensus070_probe"
		handlerRef  = "ConsensusProbeHandler"
		reply       = "The probe reports the build is green."
		toolOutMark = "ConsensusProbeHandler" // appears only in the tool output
	)

	llm := &toolContinuationLLM{outputs: []*AgentOutput{
		{ // turn 1: plan the tool call and hand it to the executor
			InternalMonologue:  "I need the build probe before I can answer.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests: []ToolRequest{
				{ToolName: toolName, Parameters: map[string]any{"query": "build-status"}},
			},
			SubAgentSpawns: []SubAgentSpawn{},
		},
		{ // turn 2: the continuation turn, answering from the tool output
			InternalMonologue:  "I have the probe result now, so I can answer.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      reply,
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
	// The dispatch below is explicit (pollAndDispatch is what the heartbeat
	// tick calls); a tiny interval only keeps StartHeartbeatLoop out of the way.
	h.HeartbeatConfig.Interval = 2 * time.Minute

	// Register the tool the plan will call. DF-CONSENSUS-077: go_native dispatch
	// now calls a registered Go handler (an unregistered ref fails the request),
	// so the output is deterministic without a subprocess or sandbox — and it
	// still carries the handler ref the continuation assertions look for.
	tools.RegisterNativeHandler(handlerRef, func(_ context.Context, params map[string]any, workspace string) (string, error) {
		return fmt.Sprintf("%s: build is green (params=%v, workspace=%s)", handlerRef, params, workspace), nil
	})
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		 VALUES ('tool-070-01', $1, 'DF-CONSENSUS-070 probe tool', 'external', 'go_native', $2, 1)`,
		toolName, handlerRef); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A real turn starts from a user message; the session is 'thinking' because
	// that is what the dispatcher queued (the API sets it on POST /message).
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, question, sessionID); err != nil {
		t.Fatalf("seed user message: %v", err)
	}
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}

	// ------------------------------------------------------------------
	// 1. Planning turn: the model asks for the tool, so the turn hands off.
	// ------------------------------------------------------------------
	result, err := h.RunInteractivePlanning(th.ctx, sessionID, DefaultPlanningConfig())
	if err != nil {
		t.Fatalf("planning turn 1: %v", err)
	}
	if result == nil {
		t.Fatal("planning turn 1 returned nil result (session was not claimed)")
	}
	if result.NextStatus != "tool_exec" {
		t.Fatalf("planning turn 1 NextStatus = %q, want 'tool_exec' (tool hand-off)", result.NextStatus)
	}
	if got := llm.callCount(); got != 1 {
		t.Fatalf("LLM calls after planning turn 1 = %d, want 1", got)
	}

	reqRows, err := th.conn.Query(th.ctx,
		`SELECT id, status FROM tool_requests WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(reqRows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1 (the hand-off must persist exactly one request)", len(reqRows))
	}
	requestID := toString(reqRows[0]["id"])
	if got := toString(reqRows[0]["status"]); got != "pending" {
		t.Fatalf("tool_request status = %q, want 'pending'", got)
	}

	// ------------------------------------------------------------------
	// 2. Executor runs the tool and completes the request.
	// ------------------------------------------------------------------
	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("PollOnce processed = %d, want 1", processed)
	}

	resRows, err := th.conn.Query(th.ctx,
		`SELECT output, is_error FROM tool_results WHERE request_id = $1`, requestID)
	if err != nil {
		t.Fatalf("query tool_results: %v", err)
	}
	if len(resRows) != 1 {
		t.Fatalf("tool_results rows = %d, want 1", len(resRows))
	}
	toolOutput := toString(resRows[0]["output"])
	if !strings.Contains(toolOutput, toolOutMark) {
		t.Fatalf("tool_result output = %q, want it to contain %q", toolOutput, toolOutMark)
	}
	if toInt(resRows[0]["is_error"]) != 0 {
		t.Fatalf("tool_result is_error = %d, want 0", toInt(resRows[0]["is_error"]))
	}

	// AC-1 preamble / AC-4: with a completed tool_request the session must NOT
	// be sitting in 'idle' — it has to be queued for the continuation turn.
	sessRows, err := th.conn.Query(th.ctx,
		`SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session status: %v", err)
	}
	statusAfterExec := toString(sessRows[0]["status"])
	if statusAfterExec != "thinking" {
		t.Fatalf("session status after tool completion = %q, want 'thinking' (continuation turn queued); "+
			"'idle' here is DF-CONSENSUS-070 — the turn ended with no assistant reply", statusAfterExec)
	}

	// ------------------------------------------------------------------
	// 3. The harness dispatches the continuation turn and it answers.
	// ------------------------------------------------------------------
	h.pollAndDispatch(th.ctx)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if llm.callCount() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := llm.callCount(); got < 2 {
		t.Fatalf("LLM calls = %d, want 2: no continuation turn ran after the tool result was stored "+
			"(AC-1: the tool output must drive another LLM turn)", got)
	}

	// AC-1: the continuation turn's prompt carries the tool output.
	continuationPrompt := llm.prompt(1)
	if !strings.Contains(continuationPrompt, toolOutMark) {
		t.Errorf("continuation prompt does not contain the tool output %q:\n%s", toolOutMark, continuationPrompt)
	}
	// ... and the user question, so the answer is grounded in the request.
	if !strings.Contains(continuationPrompt, question) {
		t.Errorf("continuation prompt does not contain the user question %q:\n%s", question, continuationPrompt)
	}

	// AC-2: the continuation turn persisted the assistant reply. Wait for the
	// dispatch goroutine to finish entirely (status applied AND the claim
	// released) so the block count below is stable.
	settleDeadline := time.Now().Add(10 * time.Second)
	finalStatus := ""
	for time.Now().Before(settleDeadline) {
		rows, qerr := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
		if qerr != nil {
			t.Fatalf("query final status: %v", qerr)
		}
		finalStatus = toString(rows[0]["status"])

		h.inFlightMu.Lock()
		inflight := len(h.inFlight)
		h.inFlightMu.Unlock()

		if finalStatus == "idle" && inflight == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if finalStatus != "idle" {
		t.Fatalf("session status after the continuation turn = %q, want 'idle'", finalStatus)
	}

	blockRows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block' ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query text_block rows: %v", err)
	}
	if len(blockRows) == 0 {
		t.Fatal("no assistant text_block persisted: the continuation turn never wrote a durable reply")
	}
	if len(blockRows) != 1 {
		t.Fatalf("assistant text_block rows = %d, want 1 (only the reply is an assistant message; "+
			"the private monologue must not be persisted as one)", len(blockRows))
	}
	if got := toString(blockRows[0]["content"]); got != reply {
		t.Errorf("persisted assistant message = %q, want %q", got, reply)
	}

	// AC-4 as an invariant: no idle session may hold a completed tool_request
	// with zero assistant blocks.
	invariantRows, err := th.conn.Query(th.ctx, `
		SELECT COUNT(*) AS cnt FROM sessions s
		WHERE s.id = $1
		  AND s.status = 'idle'
		  AND EXISTS (SELECT 1 FROM tool_requests tr WHERE tr.session_id = s.id AND tr.status = 'completed')
		  AND NOT EXISTS (SELECT 1 FROM memory_events me WHERE me.session_id = s.id AND me.type = 'text_block')
	`, sessionID)
	if err != nil {
		t.Fatalf("query idle-without-reply invariant: %v", err)
	}
	if toInt(invariantRows[0]["cnt"]) != 0 {
		t.Errorf("AC-4 violated: session is idle with a completed tool_request and no assistant block")
	}

	// docs/API.md §5: poll until idle, then last_message is available.
	adminKey := "cs_sk_070_admin_key"
	wakeSeedAdminKey(t, th, adminKey)
	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0", Wake: h.RequestWake})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET session: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		LastMessage *string `json:"last_message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v (body=%s)", err, w.Body.String())
	}
	if resp.LastMessage == nil {
		t.Fatal("GET /api/v1/sessions/{id} last_message = null after polling to idle (docs/API.md §5 promises the assistant reply)")
	}
	if *resp.LastMessage != reply {
		t.Errorf("last_message = %q, want %q", *resp.LastMessage, reply)
	}

	t.Logf("DF-CONSENSUS-070 PASS: tool_request %s → completed → continuation turn (LLM call 2 carried the tool output) → last_message=%q",
		requestID, *resp.LastMessage)
}
