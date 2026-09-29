// Package llm: thinking-mode chain-of-thought continuity (spec 024 §B2).
//
// The fixture in testdata/thinking-cot-turns.json holds VERBATIM provider
// exchanges recorded from api.deepseek.com/chat/completions: every request body
// in it was POSTed as written and answered HTTP 200, and the responses are the
// provider's own bytes. The tests replay the recorded responses through the real
// client and assert the request the client then builds, so both arms of the CoT
// rule (re-send with tools / omit without tools) are pinned against a body the
// provider actually accepted.
package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// recordedExchange is one request/response pair as captured on the wire.
type recordedExchange struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}

// cotFixture mirrors testdata/thinking-cot-turns.json.
type cotFixture struct {
	Recorded struct {
		Endpoint   string `json:"endpoint"`
		RecordedAt string `json:"recorded_at"`
		Model      string `json:"model"`
		HTTPStatus int    `json:"http_status"`
	} `json:"recorded"`
	ToolsPresent struct {
		Turn1 recordedExchange `json:"turn1"`
		Turn2 recordedExchange `json:"turn2"`
	} `json:"tools_present"`
	ToolsAbsent struct {
		Turn1 recordedExchange `json:"turn1"`
		Turn2 recordedExchange `json:"turn2"`
	} `json:"tools_absent"`
	ThinkingDisabled recordedExchange `json:"thinking_disabled"`
}

func loadCOTFixture(t *testing.T) *cotFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/thinking-cot-turns.json")
	if err != nil {
		t.Fatalf("read recorded fixture: %v", err)
	}
	var fx cotFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse recorded fixture: %v", err)
	}
	if fx.Recorded.HTTPStatus != 200 {
		t.Fatalf("fixture is not a recorded ACCEPTED exchange: http_status=%d", fx.Recorded.HTTPStatus)
	}
	if len(fx.ToolsPresent.Turn2.Response) == 0 || len(fx.ToolsAbsent.Turn2.Response) == 0 {
		t.Fatal("fixture is missing a recorded continuation response")
	}
	return &fx
}

// newReplayServer serves the recorded responses in order and captures every
// request body the client produced. It is the only fake in these tests: the
// bytes it returns come from the fixture file, not from the test source.
func newReplayServer(t *testing.T, responses ...json.RawMessage) (*httptest.Server, *[][]byte) {
	t.Helper()
	captured := &[][]byte{}
	next := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("replay server: read request body: %v", err)
		}
		*captured = append(*captured, body)
		if next >= len(responses) {
			t.Errorf("replay server: unexpected extra request (%d recorded responses)", len(responses))
			http.Error(w, `{"error":{"message":"no recorded response left"}}`, http.StatusInternalServerError)
			return
		}
		resp := responses[next]
		next++
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(resp); err != nil {
			t.Errorf("replay server: write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

// recordedMessages decodes the "messages" array of a recorded request through
// the same carrier type the harness passes to the client.
func recordedMessages(t *testing.T, raw json.RawMessage) []harness.Message {
	t.Helper()
	var envelope struct {
		Messages []harness.Message `json:"messages"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode recorded request messages: %v", err)
	}
	if len(envelope.Messages) == 0 {
		t.Fatal("recorded request carries no messages")
	}
	return envelope.Messages
}

// recordedTools decodes the "tools" array of a recorded request against the tool
// type this client sends, so the fixture itself proves the shape is expressible.
func recordedTools(t *testing.T, raw json.RawMessage) []ToolDefinition {
	t.Helper()
	var envelope struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode recorded request tools: %v", err)
	}
	return envelope.Tools
}

// wireMessages pulls the outgoing "messages" array out of a captured request
// body as decoded JSON, so the comparison is against the wire, not our structs.
func wireMessages(t *testing.T, body []byte) any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode outgoing body: %v\n%s", err, body)
	}
	msgs, ok := decoded["messages"]
	if !ok {
		t.Fatalf("outgoing body has no messages: %s", body)
	}
	return msgs
}

// stripReasoning returns the decoded array with every message's
// reasoning_content removed.
func stripReasoning(t *testing.T, v any) []any {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("not a message array: %T", v)
	}
	out := make([]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("message is not an object: %T", item)
		}
		clean := make(map[string]any, len(m))
		for k, val := range m {
			if k == "reasoning_content" {
				continue
			}
			clean[k] = val
		}
		out = append(out, clean)
	}
	return out
}

func mustDecodeRecorded(t *testing.T, raw json.RawMessage, as string) *openaiChatResponse {
	t.Helper()
	var resp openaiChatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode recorded %s response: %v", as, err)
	}
	if len(resp.Choices) == 0 {
		t.Fatalf("recorded %s response has no choices", as)
	}
	return &resp
}

func boolPtr(v bool) *bool { return &v }

// ============================================================================
// 1. The turn (content + CoT + tool calls) is handed back, not discarded.
// ============================================================================

// TestThinkingCOT_ToolTurnIsHandedBackWithItsReasoning pins per-turn CoT
// persistence at the layer that owns the turn: a thinking tool-calling turn has
// empty content, so the AgentOutput parse necessarily fails — but the turn must
// still come back carrying its raw content, its chain-of-thought and its tool
// calls, otherwise nothing downstream can persist it or continue the loop.
func TestThinkingCOT_ToolTurnIsHandedBackWithItsReasoning(t *testing.T) {
	fx := loadCOTFixture(t)
	recorded := mustDecodeRecorded(t, fx.ToolsPresent.Turn1.Response, "tools turn1")
	want := recorded.Choices[0].Message

	if want.Content != "" || len(want.ToolCalls) == 0 {
		t.Fatalf("fixture precondition: recorded tool turn must have empty content + tool calls (content=%q tool_calls=%d)",
			want.Content, len(want.ToolCalls))
	}

	srv, _ := newReplayServer(t, fx.ToolsPresent.Turn1.Response)
	client := NewOpenAIClient(&Config{
		Provider:        ProviderOpenAI,
		BaseURL:         srv.URL,
		Model:           "deepseek-flash",
		MaxTokens:       256,
		Thinking:        boolPtr(true),
		ReasoningEffort: "high",
		Tools:           recordedTools(t, fx.ToolsPresent.Turn1.Request),
		ToolChoice:      "auto",
	}).(*openaiClient)

	resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsPresent.Turn1.Request))
	if err == nil {
		t.Fatal("expected the AgentOutput parse to fail for a bare tool turn (empty content)")
	}
	if resp == nil {
		t.Fatal("turn was discarded on parse failure: no response returned with the error")
	}

	if resp.ReasoningContent != want.ReasoningContent {
		t.Errorf("ReasoningContent = %q, want the recorded CoT %q", resp.ReasoningContent, want.ReasoningContent)
	}
	if resp.Content != want.Content {
		t.Errorf("Content = %q, want the raw recorded content %q", resp.Content, want.Content)
	}
	if len(resp.ToolCalls) != len(want.ToolCalls) {
		t.Fatalf("ToolCalls = %d, want %d", len(resp.ToolCalls), len(want.ToolCalls))
	}
	for i, tc := range resp.ToolCalls {
		if tc.ID != want.ToolCalls[i].ID ||
			tc.Function.Name != want.ToolCalls[i].Function.Name ||
			tc.Function.Arguments != want.ToolCalls[i].Function.Arguments {
			t.Errorf("ToolCalls[%d] = %+v, want %+v", i, tc, want.ToolCalls[i])
		}
	}
	// The CoT was promoted to the answer as a last resort and could not be
	// parsed as AgentOutput — the response says so instead of quietly
	// substituting it (spec 024 §B2).
	if !resp.ReasoningPromotedToOutput {
		t.Error("ReasoningPromotedToOutput = false, want true: CoT was substituted for the empty content")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("parse")) {
		t.Errorf("error should report the parse failure, got: %v", err)
	}
}

// ============================================================================
// 2. With tools: every previous turn's reasoning_content goes back on the wire.
// ============================================================================

// TestThinkingCOT_ReSentWhenToolsPresent drives the tools branch: the outgoing
// message array must be byte-equivalent (as decoded JSON) to the recorded
// continuation request — a body the live provider accepted with HTTP 200.
func TestThinkingCOT_ReSentWhenToolsPresent(t *testing.T) {
	fx := loadCOTFixture(t)
	srv, captured := newReplayServer(t, fx.ToolsPresent.Turn1.Response, fx.ToolsPresent.Turn2.Response)

	client := NewOpenAIClient(&Config{
		Provider:        ProviderOpenAI,
		BaseURL:         srv.URL,
		Model:           "deepseek-flash",
		MaxTokens:       256,
		Thinking:        boolPtr(true),
		ReasoningEffort: "high",
		Tools:           recordedTools(t, fx.ToolsPresent.Turn1.Request),
		ToolChoice:      "auto",
	})

	msgs := recordedMessages(t, fx.ToolsPresent.Turn2.Request)
	if msgs[2].ReasoningContent == "" {
		t.Fatal("fixture precondition: recorded continuation must carry the prior turn's reasoning_content")
	}
	_, err := client.Call(t.Context(), msgs)
	if err == nil {
		t.Fatal("expected an AgentOutput parse error: the recorded continuation answer is not AgentOutput JSON")
	}
	if len(*captured) != 1 {
		t.Fatalf("captured %d requests, want 1", len(*captured))
	}

	body := (*captured)[0]
	// The CoT is present on the wire, verbatim.
	if !bytes.Contains(body, []byte(msgs[2].ReasoningContent)) {
		t.Errorf("outgoing body does not carry the previous turn's reasoning_content:\n%s", body)
	}
	// Outgoing messages == the recorded, provider-accepted request messages.
	var want map[string]any
	if err := json.Unmarshal(fx.ToolsPresent.Turn2.Request, &want); err != nil {
		t.Fatalf("decode recorded request: %v", err)
	}
	if got := wireMessages(t, body); !reflect.DeepEqual(got, want["messages"]) {
		t.Errorf("outgoing messages differ from the recorded accepted request.\n got: %s\nwant: %s",
			prettyJSON(t, got), prettyJSON(t, want["messages"]))
	}
	// And the request really carried tools, which is what licenses the re-send.
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("decode outgoing body: %v", err)
	}
	if _, ok := sent["tools"]; !ok {
		t.Errorf("outgoing body has no tools array:\n%s", body)
	}
}

// ============================================================================
// 3. Without tools: reasoning_content must not appear on the wire at all —
//    while every other part of the turn is unchanged.
// ============================================================================

// TestThinkingCOT_OmittedWhenToolsAbsent drives the no-tools branch. Both arms
// are fed the SAME recorded message list, so the only difference between them is
// the presence of tools — the discriminating variable.
func TestThinkingCOT_OmittedWhenToolsAbsent(t *testing.T) {
	fx := loadCOTFixture(t)
	srv, captured := newReplayServer(t, fx.ToolsAbsent.Turn2.Response)

	client := NewOpenAIClient(&Config{
		Provider:        ProviderOpenAI,
		BaseURL:         srv.URL,
		Model:           "deepseek-flash",
		MaxTokens:       128,
		Thinking:        boolPtr(true),
		ReasoningEffort: "high",
	})

	msgs := recordedMessages(t, fx.ToolsAbsent.Turn2.Request)
	if msgs[1].ReasoningContent == "" {
		t.Fatal("fixture precondition: recorded no-tools continuation must carry a prior turn's reasoning_content")
	}
	if _, err := client.Call(t.Context(), msgs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*captured) != 1 {
		t.Fatalf("captured %d requests, want 1", len(*captured))
	}

	body := (*captured)[0]
	if bytes.Contains(body, []byte("reasoning_content")) {
		t.Errorf("outgoing body carries reasoning_content without tools (spec 024 §B2 says it must not):\n%s", body)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("decode outgoing body: %v", err)
	}
	if _, ok := sent["tools"]; ok {
		t.Errorf("outgoing body unexpectedly carries tools:\n%s", body)
	}

	// Everything except reasoning_content is identical to the recorded input —
	// the client dropped the CoT and nothing else.
	var recorded map[string]any
	if err := json.Unmarshal(fx.ToolsAbsent.Turn2.Request, &recorded); err != nil {
		t.Fatalf("decode recorded request: %v", err)
	}
	want := stripReasoning(t, recorded["messages"])
	if got := wireMessages(t, body); !reflect.DeepEqual(got, want) {
		t.Errorf("outgoing messages differ beyond the dropped CoT.\n got: %s\nwant: %s",
			prettyJSON(t, got), prettyJSON(t, want))
	}
}

// ============================================================================
// 4. End-to-end continuity: a request rebuilt from the CLIENT's own turn-1
//    answer reproduces the recorded body the provider accepted.
// ============================================================================

func TestThinkingCOT_ContinuationRebuiltFromClientTurnMatchesRecordedRequest(t *testing.T) {
	fx := loadCOTFixture(t)
	srv, captured := newReplayServer(t, fx.ToolsPresent.Turn1.Response, fx.ToolsPresent.Turn2.Response)

	client := NewOpenAIClient(&Config{
		Provider:        ProviderOpenAI,
		BaseURL:         srv.URL,
		Model:           "deepseek-flash",
		MaxTokens:       256,
		Thinking:        boolPtr(true),
		ReasoningEffort: "high",
		Tools:           recordedTools(t, fx.ToolsPresent.Turn1.Request),
		ToolChoice:      "auto",
	})

	turn1 := recordedMessages(t, fx.ToolsPresent.Turn1.Request)
	resp1, err := client.Call(t.Context(), turn1)
	if err == nil || resp1 == nil {
		t.Fatalf("turn 1: want the turn back with a parse error, got resp=%v err=%v", resp1, err)
	}

	// The caller persists the turn and continues the conversation with tools:
	// the assistant turn it appends is built ONLY from what the client returned.
	turn2 := append([]harness.Message{}, turn1...)
	turn2 = append(turn2, harness.Message{
		Role:             "assistant",
		Content:          resp1.Content,
		ReasoningContent: resp1.ReasoningContent,
		ToolCalls:        resp1.ToolCalls,
	})
	recordedTurn2 := recordedMessages(t, fx.ToolsPresent.Turn2.Request)
	turn2 = append(turn2, recordedTurn2[3:]...) // the tool result + the follow-up, owned by the caller

	if _, err := client.Call(t.Context(), turn2); err == nil {
		t.Fatal("turn 2: expected an AgentOutput parse error from the recorded answer")
	}
	if len(*captured) != 2 {
		t.Fatalf("captured %d requests, want 2", len(*captured))
	}

	var want map[string]any
	if err := json.Unmarshal(fx.ToolsPresent.Turn2.Request, &want); err != nil {
		t.Fatalf("decode recorded request: %v", err)
	}
	if got := wireMessages(t, (*captured)[1]); !reflect.DeepEqual(got, want["messages"]) {
		t.Errorf("a continuation rebuilt from the client's own turn does not reproduce the recorded accepted request.\n got: %s\nwant: %s",
			prettyJSON(t, got), prettyJSON(t, want["messages"]))
	}
}

// ============================================================================
// 5. Explicit thinking-mode control (toggle + effort), per request and config.
// ============================================================================

func TestThinkingToggleAndEffort_ExplicitAndPerRequest(t *testing.T) {
	fx := loadCOTFixture(t)
	// The recorded disabled-thinking request carries the toggle and NO effort.
	var recorded map[string]any
	if err := json.Unmarshal(fx.ThinkingDisabled.Request, &recorded); err != nil {
		t.Fatalf("decode recorded thinking-disabled request: %v", err)
	}
	if _, hasEffort := recorded["reasoning_effort"]; hasEffort {
		t.Fatal("fixture precondition: the recorded thinking-disabled request must not carry reasoning_effort")
	}

	cases := []struct {
		name       string
		cfg        *Config
		opts       RequestOptions
		wantThink  string // "" = absent
		wantEffort string // "" = absent
	}{
		{
			name:       "config enables thinking and sets effort",
			cfg:        &Config{Thinking: boolPtr(true), ReasoningEffort: "minimal"},
			wantThink:  "enabled",
			wantEffort: "low", // minimal -> low (spec 024 §B2 mapping)
		},
		{
			name:      "config disables thinking, so no effort is sent",
			cfg:       &Config{Thinking: boolPtr(false), ReasoningEffort: "high"},
			wantThink: "disabled",
		},
		{
			name: "neither configured: nothing is sent (provider default applies)",
			cfg:  &Config{},
		},
		{
			name:       "per-request options override the config",
			cfg:        &Config{Thinking: boolPtr(false), ReasoningEffort: "max"},
			opts:       RequestOptions{Thinking: boolPtr(true), ReasoningEffort: "ultra"},
			wantThink:  "enabled",
			wantEffort: "max", // ultra -> max
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, captured := newReplayServer(t, fx.ToolsAbsent.Turn1.Response)
			tc.cfg.Provider = ProviderOpenAI
			tc.cfg.BaseURL = srv.URL
			tc.cfg.Model = "deepseek-flash"
			tc.cfg.MaxTokens = 64
			client := NewOpenAIClient(tc.cfg).(*openaiClient)

			if _, err := client.CallWithOptions(t.Context(), recordedMessages(t, fx.ToolsAbsent.Turn1.Request), tc.opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(*captured) != 1 {
				t.Fatalf("captured %d requests, want 1", len(*captured))
			}
			var sent map[string]any
			if err := json.Unmarshal((*captured)[0], &sent); err != nil {
				t.Fatalf("decode outgoing body: %v", err)
			}

			gotThink := ""
			if think, ok := sent["thinking"].(map[string]any); ok {
				gotThink, _ = think["type"].(string)
			}
			if gotThink != tc.wantThink {
				t.Errorf("thinking.type = %q, want %q (body: %s)", gotThink, tc.wantThink, (*captured)[0])
			}
			gotEffort, _ := sent["reasoning_effort"].(string)
			if gotEffort != tc.wantEffort {
				t.Errorf("reasoning_effort = %q, want %q (body: %s)", gotEffort, tc.wantEffort, (*captured)[0])
			}
		})
	}

	// The disabled toggle we emit is the one the provider accepted.
	disabledCase := cases[1]
	srv, captured := newReplayServer(t, fx.ThinkingDisabled.Response)
	disabledCase.cfg.BaseURL = srv.URL
	client := NewOpenAIClient(disabledCase.cfg).(*openaiClient)
	// The recorded answer ("consensus CoT probe") is not AgentOutput JSON, so the
	// harness-level parse error is expected; only the outgoing body matters here.
	if _, err := client.Call(t.Context(), recordedMessages(t, fx.ThinkingDisabled.Request)); err == nil {
		t.Log("note: recorded disabled-thinking answer parsed as AgentOutput (it carries no CoT)")
	}
	var sent map[string]any
	if err := json.Unmarshal((*captured)[0], &sent); err != nil {
		t.Fatalf("decode outgoing body: %v", err)
	}
	if !reflect.DeepEqual(sent["thinking"], recorded["thinking"]) {
		t.Errorf("thinking = %v, want the recorded accepted value %v", sent["thinking"], recorded["thinking"])
	}
}

// TestReasoningEffort_UnknownValueIsRefused: an effort name outside the mapping
// is an error, not a silently dropped dial.
func TestReasoningEffort_UnknownValueIsRefused(t *testing.T) {
	fx := loadCOTFixture(t)
	srv, captured := newReplayServer(t, fx.ToolsAbsent.Turn1.Response)
	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 64,
		Thinking: boolPtr(true),
	}).(*openaiClient)

	_, err := client.CallWithOptions(t.Context(), recordedMessages(t, fx.ToolsAbsent.Turn1.Request),
		RequestOptions{ReasoningEffort: "ultra-max"})
	if err == nil {
		t.Fatal("expected an error for an unknown reasoning effort")
	}
	if len(*captured) != 0 {
		t.Errorf("a refused effort must not reach the provider; captured %d requests", len(*captured))
	}

	// The mapping itself, per spec 024 §B2.
	for name, want := range map[string]string{
		"minimal": "low", "low": "low", "medium": "high", "high": "high",
		"xhigh": "high", "max": "max", "ultra": "max", "": "",
	} {
		got, err := MapReasoningEffort(name)
		if err != nil {
			t.Errorf("MapReasoningEffort(%q) returned error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("MapReasoningEffort(%q) = %q, want %q", name, got, want)
		}
	}
}

// ============================================================================
// 6. The CoT-as-answer fallback is deliberate, flagged and disableable.
// ============================================================================

// TestReasoningFallback_FlaggedAndDisableable drives the empty-content path with
// the RECORDED tool turn (content empty, CoT present but not AgentOutput JSON).
func TestReasoningFallback_FlaggedAndDisableable(t *testing.T) {
	fx := loadCOTFixture(t)

	t.Run("allowed by default: substitution is flagged", func(t *testing.T) {
		srv, _ := newReplayServer(t, fx.ToolsPresent.Turn1.Response)
		client := NewOpenAIClient(&Config{
			Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 64,
		}).(*openaiClient)

		resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsPresent.Turn1.Request))
		if err == nil {
			t.Fatal("expected the promoted CoT to fail AgentOutput parsing")
		}
		if resp == nil || !resp.ReasoningPromotedToOutput {
			t.Fatalf("promotion not reported: resp=%+v", resp)
		}
		if !bytes.Contains([]byte(err.Error()), []byte("last-resort")) {
			t.Errorf("error should name the promotion, got: %v", err)
		}
	})

	t.Run("disabled: no substitution, no request-time surprise", func(t *testing.T) {
		srv, _ := newReplayServer(t, fx.ToolsPresent.Turn1.Response)
		client := NewOpenAIClient(&Config{
			Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 64,
			AllowReasoningFallback: boolPtr(false),
		}).(*openaiClient)

		resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsPresent.Turn1.Request))
		if err == nil {
			t.Fatal("expected an error when content is empty and the fallback is disabled")
		}
		if resp == nil {
			t.Fatal("the turn must still be returned with the error")
		}
		if resp.ReasoningPromotedToOutput {
			t.Error("ReasoningPromotedToOutput = true with the fallback disabled")
		}
		if resp.Output != nil {
			t.Error("Output parsed although content was empty and the fallback was disabled")
		}
		// The CoT itself is still handed back: refused as an ANSWER, kept as a turn.
		if resp.ReasoningContent == "" {
			t.Error("the turn's reasoning_content was dropped")
		}
	})
}

func prettyJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}
