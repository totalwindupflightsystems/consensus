package llm

// PERF-CONSENSUS-014 regression: a provider that answers a plain chat turn in
// PROSE (not AgentOutput JSON) used to surface as a call error from every
// client. The planning loop treated that shape error as a call failure and
// sent the session back to 'thinking', so the heartbeat re-claimed it and
// re-ran the whole full-context planning call — 2-3 sequential provider calls
// for ONE user message, ending in a circuit-breaker pause with no reply. The
// prose is the answer; it must ride message_to_user and settle the turn in
// exactly one provider call (DF-CONSENSUS-072 reply doctrine).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

func openaiCompletionBody(content string) string {
	return `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"test-model",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":` + quoteJSON(content) +
		`},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestOpenAIClient_ProseReply_IsTheAnswer_OneCall drives the red baseline
// shape: the provider answers "Hi there!" in plain prose. The call must
// succeed with the prose as message_to_user and make EXACTLY ONE provider
// request (pre-fix this returned a parse error, which re-queued the turn and
// re-sent the full context).
func TestOpenAIClient_ProseReply_IsTheAnswer_OneCall(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openaiCompletionBody("Hi there!")))
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		APIKey:   "test-key",
		Model:    "test-model",
	})

	resp, err := client.Call(context.Background(), []harness.Message{
		{Role: "user", Content: "say hi in one word"},
	})
	if err != nil {
		t.Fatalf("prose reply must not be a call error: %v", err)
	}
	if resp == nil || resp.Output == nil {
		t.Fatalf("expected a usable AgentOutput, got %+v", resp)
	}
	if got := resp.Output.MessageToUser; got != "Hi there!" {
		t.Errorf("message_to_user = %q, want the provider's prose %q", got, "Hi there!")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("provider call count = %d, want exactly 1 (no full-context retry)", n)
	}
}

// TestOpenAIClient_ToolCallTurn_WithUnparseableContent_StillErrors: a turn
// that requested tools has no user-facing reply to demote to — the error must
// survive so the tool_calls are not silently dropped.
func TestOpenAIClient_ToolCallTurn_WithUnparseableContent_StillErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"test-model",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"running the query","tool_calls":[{"id":"c1","type":"function","function":{"name":"sql","arguments":"{}"}}]},"finish_reason":"tool_calls"}],` +
			`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		APIKey:   "test-key",
		Model:    "test-model",
	})

	resp, err := client.Call(context.Background(), []harness.Message{
		{Role: "user", Content: "run the query"},
	})
	if err == nil {
		t.Fatalf("tool-call turn with unparseable content must still error, got %+v", resp)
	}
	if resp == nil || len(resp.ToolCalls) != 1 {
		t.Errorf("the tool calls must still ride the response for diagnostics, got %+v", resp)
	}
}

// TestInterpretResponsesResponse_ProseReply_IsTheAnswer covers the Responses
// format interpreter at the same seam (pure function, no transport).
func TestInterpretResponsesResponse_ProseReply_IsTheAnswer(t *testing.T) {
	raw := map[string]any{
		"id":     "resp-test",
		"status": "completed",
		"model":  "test-model",
		"output": []any{
			map[string]any{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []any{
					map[string]any{"type": "output_text", "text": "Hi there!"},
				},
			},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var rr ResponsesResponse
	if err := json.Unmarshal(blob, &rr); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	turn, err := InterpretResponsesResponse(&rr)
	if err != nil {
		t.Fatalf("prose reply must not be a call error: %v", err)
	}
	if turn == nil || turn.Output == nil {
		t.Fatalf("expected a usable AgentOutput, got %+v", turn)
	}
	if got := turn.Output.MessageToUser; got != "Hi there!" {
		t.Errorf("message_to_user = %q, want %q", got, "Hi there!")
	}
}

// TestProseFallbackOutput_EmptyContent_NoFallback guards the promotion
// boundary: empty content has nothing user-facing to carry.
func TestProseFallbackOutput_EmptyContent_NoFallback(t *testing.T) {
	for _, content := range []string{"", "   ", "\n\t"} {
		if fb := proseFallbackOutput(content); fb != nil {
			t.Errorf("proseFallbackOutput(%q) = %+v, want nil", content, fb)
		}
	}
	if fb := proseFallbackOutput("Hi"); fb == nil || fb.MessageToUser != "Hi" {
		t.Errorf("proseFallbackOutput(\"Hi\") = %+v, want MessageToUser=Hi", fb)
	}
}
