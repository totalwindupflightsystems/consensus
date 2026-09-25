package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
)

type capturingConversationLLM struct {
	calls [][]Message
}

func (c *capturingConversationLLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
	copied := append([]Message(nil), messages...)
	c.calls = append(c.calls, copied)
	return &LLMResponse{
		Output: &AgentOutput{
			InternalMonologue:  "Answer the user's arithmetic question.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      "4",
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		ModelID: "test-model",
		Usage: LLMUsage{
			PromptTokens:     17,
			CompletionTokens: 3,
		},
	}, nil
}

type multiTurnConversationLLM struct {
	calls   [][]Message
	outputs []*AgentOutput
}

func (c *multiTurnConversationLLM) Call(_ context.Context, messages []Message) (*LLMResponse, error) {
	copied := append([]Message(nil), messages...)
	c.calls = append(c.calls, copied)
	return &LLMResponse{Output: c.outputs[len(c.calls)-1], ModelID: "test-model"}, nil
}

func TestConversationalMessagePathProjectsRepliesAndAccountsTokens(t *testing.T) {
	llm := &capturingConversationLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	adminKey := "cs_sk_conversation_test_admin_key"
	hash := sha256.Sum256([]byte(adminKey))
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('conversation-admin', $1, $2, 'admin', CURRENT_TIMESTAMP)`,
		hex.EncodeToString(hash[:]), adminKey[:8]); err != nil {
		t.Fatalf("insert API key: %v", err)
	}

	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0"})
	postMessage := func(content string) {
		t.Helper()
		body := `{"role":"user","content":` + mustJSONQuote(t, content) + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("POST message returned %d: %s", w.Code, w.Body.String())
		}
	}

	postMessage("Use the latest two instructions in order.")
	postMessage("What is 2+2? Answer with just the number.")

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 1
	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run planning: %v", err)
	}
	if result == nil || result.Status != "success" {
		t.Fatalf("planning result = %+v, want success", result)
	}

	if len(llm.calls) != 1 {
		t.Fatalf("LLM calls = %d, want 1", len(llm.calls))
	}
	messages := llm.calls[0]
	if len(messages) != 4 {
		t.Fatalf("LLM messages = %#v, want system + context + two user turns", messages)
	}
	for i, want := range []string{
		"Use the latest two instructions in order.",
		"What is 2+2? Answer with just the number.",
	} {
		got := messages[i+2]
		if got.Role != "user" || got.Content != want {
			t.Errorf("message %d = %#v, want ordered user turn %q", i+2, got, want)
		}
	}
	if strings.Contains(messages[1].Content, "What is 2+2?") {
		t.Error("user turn was duplicated inside the formatted context message")
	}

	rows, err := th.conn.Query(th.ctx, `
		SELECT s.status, s.tokens_used_in, s.tokens_used_out,
		       b.category, b.prompt_tokens, b.completion_tokens
		FROM sessions s
		JOIN agent_billing b ON b.session_id = s.id
		WHERE s.id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query accounting: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("billing rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if got := toString(row["status"]); got != "idle" {
		t.Errorf("session status = %q, want idle", got)
	}
	if got := toInt64(row["tokens_used_in"]); got != 17 {
		t.Errorf("sessions.tokens_used_in = %d, want 17", got)
	}
	if got := toInt64(row["tokens_used_out"]); got != 3 {
		t.Errorf("sessions.tokens_used_out = %d, want 3", got)
	}
	if got := toString(row["category"]); got != "cognition" {
		t.Errorf("billing category = %q, want cognition", got)
	}
	if got := toInt64(row["prompt_tokens"]); got != 17 {
		t.Errorf("billing prompt_tokens = %d, want 17", got)
	}
	if got := toInt64(row["completion_tokens"]); got != 3 {
		t.Errorf("billing completion_tokens = %d, want 3", got)
	}

	visible, err := th.conn.Query(th.ctx, `
		SELECT me.content
		FROM memory_events me
		LEFT JOIN display_modes dm ON dm.memory_id = me.id
		WHERE me.session_id = $1 AND me.type = 'user_message'
		  AND (
		      COALESCE(dm.mode, 'full') != 'hidden'
		      OR dm.set_by_iteration = (SELECT iteration FROM sessions WHERE id = $1)
		  )`, sessionID)
	if err != nil {
		t.Fatalf("query unread messages: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("unread user messages after response = %d, want 0", len(visible))
	}

	textRows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block' ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query assistant response: %v", err)
	}
	if len(textRows) != 1 || toString(textRows[0]["content"]) != "4" {
		t.Fatalf("assistant text rows = %#v, want one durable response containing 4", textRows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET session returned %d: %s", w.Code, w.Body.String())
	}
	var polled struct {
		LastMessage   *string `json:"last_message"`
		TokensUsedIn  int64   `json:"tokens_used_in"`
		TokensUsedOut int64   `json:"tokens_used_out"`
	}
	if err := json.NewDecoder(w.Body).Decode(&polled); err != nil {
		t.Fatalf("decode GET session: %v", err)
	}
	if polled.LastMessage == nil || *polled.LastMessage != "4" {
		t.Fatalf("polled last_message = %v, want 4", polled.LastMessage)
	}
	if polled.TokensUsedIn != 17 || polled.TokensUsedOut != 3 {
		t.Fatalf("polled tokens = %d/%d, want 17/3", polled.TokensUsedIn, polled.TokensUsedOut)
	}
}

func TestFailedSessionMessageResumesAndReplans(t *testing.T) {
	llm := &capturingConversationLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		UPDATE sessions
		SET status = 'failed', completed_at = CURRENT_TIMESTAMP
		WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("mark session failed: %v", err)
	}

	adminKey := "cs_sk_failed_resume_test_admin_key"
	hash := sha256.Sum256([]byte(adminKey))
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('failed-resume-admin', $1, $2, 'admin', CURRENT_TIMESTAMP)`,
		hex.EncodeToString(hash[:]), adminKey[:8]); err != nil {
		t.Fatalf("insert API key: %v", err)
	}

	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0"})
	body := `{"role":"user","content":"Retry the failed turn."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST message returned %d: %s", w.Code, w.Body.String())
	}

	row, err := th.conn.QueryRow(th.ctx, `SELECT status, completed_at FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query resumed session: %v", err)
	}
	if got := toString(row["status"]); got != "thinking" {
		t.Fatalf("status after message = %q, want thinking", got)
	}
	if row["completed_at"] != nil {
		t.Fatalf("completed_at after message = %v, want nil", row["completed_at"])
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 1
	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("re-run planning: %v", err)
	}
	if result == nil || result.Status != "success" {
		t.Fatalf("planning result = %+v, want success", result)
	}
	if len(llm.calls) != 1 {
		t.Fatalf("LLM calls = %d, want 1", len(llm.calls))
	}
	if got := llm.calls[0][len(llm.calls[0])-1].Content; got != "Retry the failed turn." {
		t.Errorf("replanned user message = %q, want retry message", got)
	}
}

func TestUserTurnRemainsVisibleAcrossPlanningTurnsThenHidesOnRollover(t *testing.T) {
	const question = "How many active tasks are in the database?"
	llm := &multiTurnConversationLLM{outputs: []*AgentOutput{
		{InternalMonologue: "I need another planning turn."},
		{
			InternalMonologue: "Answer after reviewing the question again.",
			SystemActions:     []string{"respond"},
			MessageToUser:     "There are no active tasks.",
		},
	}}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		VALUES ('user_message', $1, $2, 0, CURRENT_TIMESTAMP)`, question, sessionID); err != nil {
		t.Fatalf("insert user turn: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2
	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run planning: %v", err)
	}
	if result == nil || result.Status != "success" {
		t.Fatalf("planning result = %+v, want success", result)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("LLM calls = %d, want 2", len(llm.calls))
	}
	for turn, messages := range llm.calls {
		found := false
		for _, message := range messages {
			if message.Role == "user" && message.Content == question {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("planning turn %d did not include the user question: %#v", turn+1, messages)
		}
	}

	contextAfterRollover, err := th.ReadActiveContext(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("read context after iteration rollover: %v", err)
	}
	if len(contextAfterRollover.PendingUserMessages) != 0 {
		t.Fatalf("pending user turns after rollover = %#v, want none", contextAfterRollover.PendingUserMessages)
	}
}

func TestBuildPlanningMessagesProjectsPendingOnEveryTurn(t *testing.T) {
	h := &Harness{}
	ic := &IterationContext{
		SessionID: "conversation",
		Goal:      "answer",
		PendingUserMessages: []PendingUserMessage{
			{ID: 2, Content: "first"},
			{ID: 3, Content: "second"},
		},
	}
	cfg := DefaultPlanningConfig()
	first := h.buildPlanningMessages(ic, &StagingBuffer{}, 1, cfg, "context")
	if len(first) != 4 || first[2].Content != "first" || first[3].Content != "second" {
		t.Fatalf("first-turn messages = %#v, want ordered pending user turns", first)
	}
	second := h.buildPlanningMessages(ic, &StagingBuffer{}, 2, cfg, "context")
	if len(second) != 4 || second[2].Content != "first" || second[3].Content != "second" {
		t.Fatalf("second-turn messages = %#v, want the same ordered user turns", second)
	}
}

func TestFlushPendingUsagePersistsMissingUsageAsZero(t *testing.T) {
	th, err := newTestHarness(&capturingConversationLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	usage, err := th.captureLLMUsage(th.ctx, sessionID, 0, &LLMResponse{})
	if err != nil {
		t.Fatalf("capture zero usage: %v", err)
	}
	th.flushPendingUsage(th.ctx, sessionID, []capturedLLMUsage{usage})

	rows, err := th.conn.Query(th.ctx, `
		SELECT s.tokens_used_in, s.tokens_used_out, b.model_id,
		       b.prompt_tokens, b.completion_tokens
		FROM sessions s
		JOIN agent_billing b ON b.session_id = s.id
		WHERE s.id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query zero usage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("billing rows = %d, want 1 zero-usage response row", len(rows))
	}
	row := rows[0]
	if toInt64(row["tokens_used_in"]) != 0 || toInt64(row["tokens_used_out"]) != 0 ||
		toInt64(row["prompt_tokens"]) != 0 || toInt64(row["completion_tokens"]) != 0 {
		t.Fatalf("missing usage was not preserved as zero: %#v", row)
	}
	if got := toString(row["model_id"]); got != "unknown" {
		t.Errorf("missing model_id = %q, want unknown", got)
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON string: %v", err)
	}
	return string(encoded)
}
