// Package llm: the DeepSeek harness-API conformance matrix (spec 024 §B/§D).
//
// Every cell of the matrix (shim_matrix.go) is executed here, so "we support the
// documented surface" is a checked claim rather than an intention. Cells whose
// evidence includes `live` are driven by the verbatim provider exchanges in
// testdata/shim-support-exchanges.json — recorded from api.deepseek.com on
// 2026-10-13 — through the real clients, so the assertions are about what the
// client puts on the wire and what it reads off the provider's bytes, not about
// what the code looks like.
//
// axiom:trace work_item=SHIM-SUPPORT-001 spec=specs/024-shim-support-matrix.md impl=internal/llm/shim_support_matrix_test.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// Fixture
// ============================================================================

// shimExchange is one recorded provider exchange: the body that was sent and the
// bytes that came back.
type shimExchange struct {
	URL        string          `json:"url"`
	Auth       string          `json:"auth"`
	HTTPStatus int             `json:"http_status"`
	Request    json.RawMessage `json:"request"`
	Response   json.RawMessage `json:"response"`
}

// shimFixture mirrors testdata/shim-support-exchanges.json.
type shimFixture struct {
	Recorded struct {
		Provider                string `json:"provider"`
		BaseURL                 string `json:"base_url"`
		RecordedAtFirst         string `json:"recorded_at_first"`
		RecordedAtLast          string `json:"recorded_at_last"`
		ProviderDocsRetrievedAt string `json:"provider_docs_retrieved_at"`
		Provenance              string `json:"provenance"`
		KeyMaterialStored       bool   `json:"key_material_stored"`
	} `json:"recorded"`
	OpenAI    map[string]shimExchange `json:"openai"`
	Anthropic map[string]shimExchange `json:"anthropic"`
	Responses map[string]shimExchange `json:"responses"`
	Beta      map[string]shimExchange `json:"beta"`
}

func loadShimFixture(t *testing.T) *shimFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "shim-support-exchanges.json"))
	if err != nil {
		t.Fatalf("read recorded conformance fixture: %v", err)
	}
	var fx shimFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse recorded conformance fixture: %v", err)
	}
	return &fx
}

// must returns one recorded exchange by group and name.
func (f *shimFixture) must(t *testing.T, group, name string) shimExchange {
	t.Helper()
	var table map[string]shimExchange
	switch group {
	case "openai":
		table = f.OpenAI
	case "anthropic":
		table = f.Anthropic
	case "responses":
		table = f.Responses
	case "beta":
		table = f.Beta
	default:
		t.Fatalf("unknown fixture group %q", group)
	}
	ex, ok := table[name]
	if !ok {
		t.Fatalf("fixture %s.%s is missing", group, name)
	}
	if len(ex.Request) == 0 {
		t.Fatalf("fixture %s.%s has no recorded request body", group, name)
	}
	return ex
}

// requestMessages decodes the "messages" array of a recorded request into the
// harness carrier type, tolerating both content shapes the provider accepts: a
// plain string (chat-completions, plain turns) and a block list (Anthropic
// thinking / tool_use / tool_result turns). A block-list turn is rebuilt into the
// same fields the client writes out, so a recorded request can be replayed
// through the client unchanged.
func requestMessages(t *testing.T, raw json.RawMessage) []harness.Message {
	t.Helper()
	var envelope struct {
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode recorded request messages: %v", err)
	}
	if len(envelope.Messages) == 0 {
		t.Fatal("recorded request carries no messages")
	}
	out := make([]harness.Message, 0, len(envelope.Messages))
	for _, m := range envelope.Messages {
		msg := harness.Message{Role: m.Role, ToolCallID: m.ToolCallID}
		if len(m.Content) == 0 || string(m.Content) == "null" {
			out = append(out, msg)
			continue
		}
		var text string
		if err := json.Unmarshal(m.Content, &text); err == nil {
			msg.Content = text
			out = append(out, msg)
			continue
		}
		var blocks []anthropicContentBlock
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			t.Fatalf("decode recorded message content: %v\n%s", err, m.Content)
		}
		for _, b := range blocks {
			switch b.Type {
			case "thinking":
				msg.ReasoningContent += b.Thinking
				if b.Signature != "" {
					msg.ReasoningSignature = b.Signature
				}
			case "text", "output_text", "input_text":
				msg.Content += b.Text
			case "tool_use":
				args := string(b.Input)
				if args == "" {
					args = "{}"
				}
				msg.ToolCalls = append(msg.ToolCalls, harness.ToolCall{
					ID:   b.ID,
					Type: "function",
					Function: harness.ToolCallFunction{
						Name:      b.Name,
						Arguments: args,
					},
				})
			case "tool_result":
				msg.ToolCallID = b.ToolUseID
				msg.Content += b.Content
			}
		}
		out = append(out, msg)
	}
	return out
}

// decodeMap decodes a JSON object and fails the test if it is not one.
func decodeMap(t *testing.T, raw []byte, what string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v\n%s", what, err, raw)
	}
	return out
}

// dig walks a decoded JSON object, failing loudly on a missing path.
func dig(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()
	var cur any = obj
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %q is not an object in %v", path, p, cur)
		}
		cur, ok = m[p]
		if !ok {
			t.Fatalf("path %v: key %q absent", path, p)
		}
	}
	return cur
}

// bodyJSON decodes a captured request body.
func bodyJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	return decodeMap(t, body, "captured request body")
}

// replay serves one recorded provider response and captures the request body.
// It reuses newReplayServer from thinking_cot_test.go, so there is exactly one
// fake in this package and its bytes always come from the fixture file.
func replay(t *testing.T, ex shimExchange) (*httptest.Server, *[][]byte) {
	t.Helper()
	if len(ex.Response) == 0 {
		t.Fatalf("recorded exchange %s carries no response body", ex.URL)
	}
	return newReplayServer(t, ex.Response)
}

// replayRaw serves a recorded response body that came from the older
// chain-of-thought fixture rather than the shim fixture.
func replayRaw(t *testing.T, resp json.RawMessage) (*httptest.Server, *[][]byte) {
	t.Helper()
	if len(resp) == 0 {
		t.Fatal("recorded response body is empty")
	}
	return newReplayServer(t, resp)
}

// openaiTestClient builds the real OpenAI-format client against a test base URL.
func openaiTestClient(t *testing.T, base string, cfg *Config) *openaiClient {
	t.Helper()
	t.Setenv("LM_STUDIO_BASE_URL", "")
	if cfg == nil {
		cfg = &Config{}
	}
	c := *cfg
	c.Provider = ProviderOpenAI
	c.BaseURL = base
	c.APIKey = "test-key"
	if c.Model == "" {
		c.Model = "deepseek-flash"
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 32
	}
	client, ok := NewOpenAIClient(&c).(*openaiClient)
	if !ok {
		t.Fatal("NewOpenAIClient did not return an *openaiClient")
	}
	return client
}

// anthropicTestClient builds the real Anthropic-format client against a test base.
func anthropicTestClient(t *testing.T, base string, cfg *Config) *anthropicClient {
	t.Helper()
	t.Setenv("LM_STUDIO_BASE_URL", "")
	if cfg == nil {
		cfg = &Config{}
	}
	c := *cfg
	c.Provider = ProviderAnthropic
	c.BaseURL = base
	c.APIKey = "test-key"
	if c.Model == "" {
		c.Model = "deepseek-flash"
	}
	if c.MaxTokens == 0 {
		c.MaxTokens = 32
	}
	client, ok := NewAnthropicClient(&c).(*anthropicClient)
	if !ok {
		t.Fatal("NewAnthropicClient did not return an *anthropicClient")
	}
	return client
}

// callOpenAI drives one call and returns the captured outgoing body.
func callOpenAI(t *testing.T, client *openaiClient, messages []harness.Message, opts RequestOptions, ex shimExchange) map[string]any {
	t.Helper()
	srv, captured := replay(t, ex)
	c := *client
	c.baseURL = srv.URL
	if _, err := c.CallWithOptions(context.Background(), messages, opts); err != nil {
		// A recorded answer is not always an AgentOutput document (a plain
		// sentence is not JSON); the wire assertions still hold.
		if !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("call failed against recorded exchange: %v", err)
		}
	}
	if len(*captured) == 0 {
		t.Fatal("no request was captured")
	}
	return bodyJSON(t, (*captured)[0])
}

// callAnthropic drives one Anthropic call and returns the captured body + turn.
func callAnthropic(t *testing.T, base string, cfg *Config, messages []harness.Message, opts RequestOptions, ex shimExchange) (map[string]any, *harness.LLMResponse) {
	t.Helper()
	srv, captured := replay(t, ex)
	client := anthropicTestClient(t, srv.URL+base, cfg)
	resp, err := client.CallWithOptions(context.Background(), messages, opts)
	if err != nil {
		if !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("anthropic call failed against recorded exchange: %v", err)
		}
	}
	if len(*captured) == 0 {
		t.Fatal("no anthropic request was captured")
	}
	return bodyJSON(t, (*captured)[0]), resp
}

// ============================================================================
// Cell checks
// ============================================================================

var shimChecks = map[string]func(*testing.T, *shimFixture){
	// ---- §B1 ----------------------------------------------------------------
	"B1-OPENAI-CHAT": func(t *testing.T, fx *shimFixture) {
		c := openaiTestClient(t, "https://api.deepseek.com", nil)
		if got, want := c.baseURL+"/chat/completions", "https://api.deepseek.com/chat/completions"; got != want {
			t.Fatalf("DeepSeek base+path = %q, want %q", got, want)
		}
		// The provider default is NOT DeepSeek, so the base must be configured;
		// the matrix records the documented pair, not a hope.
		if def := openaiTestClient(t, "", nil); def.baseURL == c.baseURL {
			t.Fatal("the provider default base URL already equals the DeepSeek base: check ProviderBaseURL")
		}
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded exchange is not an accepted request: http_status=%d", ex.HTTPStatus)
		}
		if want := "https://api.deepseek.com/chat/completions"; ex.URL != want {
			t.Fatalf("recorded exchange URL %q, want %q", ex.URL, want)
		}
		// The recorded provider answer must be readable by the client.
		srv, _ := replay(t, ex)
		cc := openaiTestClient(t, srv.URL, nil)
		resp, err := cc.Call(context.Background(), requestMessages(t, ex.Request))
		if resp == nil {
			t.Fatalf("client returned no turn for the recorded answer: %v", err)
		}
		wantUsage := decodeMap(t, ex.Response, "recorded response")
		wantPrompt := int64(dig(t, wantUsage, "usage", "prompt_tokens").(float64))
		if resp.Usage.PromptTokens != wantPrompt {
			t.Fatalf("prompt tokens = %d, want the recorded %d", resp.Usage.PromptTokens, wantPrompt)
		}
	},

	"B1-ANTHROPIC-MSGS": func(t *testing.T, fx *shimFixture) {
		if got, want := anthropicMessagesURL("https://api.deepseek.com/anthropic"), "https://api.deepseek.com/anthropic/v1/messages"; got != want {
			t.Fatalf("anthropic endpoint for the documented base = %q, want %q", got, want)
		}
		if got, want := anthropicMessagesURL("https://api.anthropic.com/v1"), "https://api.anthropic.com/v1/messages"; got != want {
			t.Fatalf("anthropic endpoint for a /v1 base = %q, want %q", got, want)
		}
		ex := fx.must(t, "anthropic", "messages_minimal_thinking_default")
		if ex.Auth != "x-api-key" {
			t.Fatalf("recorded exchange used auth %q, want x-api-key", ex.Auth)
		}
		var (
			sawPath, sawKey, sawVersion string
			calls                       int
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			sawPath = r.URL.Path
			sawKey = r.Header.Get("x-api-key")
			sawVersion = r.Header.Get("anthropic-version")
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(ex.Response); err != nil {
				t.Errorf("serve recorded answer: %v", err)
			}
		}))
		t.Cleanup(srv.Close)

		client := anthropicTestClient(t, srv.URL+"/anthropic", nil)
		resp, err := client.Call(context.Background(), requestMessages(t, ex.Request))
		if err != nil && !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("anthropic call against recorded answer failed: %v", err)
		}
		if calls != 1 {
			t.Fatalf("recorded answer was used for %d calls, want 1", calls)
		}
		if want := "/anthropic/v1/messages"; sawPath != want {
			t.Fatalf("request path = %q, want the documented %q", sawPath, want)
		}
		if sawKey != "test-key" {
			t.Fatalf("x-api-key header = %q, want the configured key", sawKey)
		}
		if sawVersion == "" {
			t.Fatal("anthropic-version header was not sent (documented as ignored, still sent for compatibility)")
		}
		// The recorded answer is a thinking-only turn: it must survive.
		if resp == nil {
			t.Fatal("client returned no turn for a thinking-only recorded answer")
		}
		if strings.TrimSpace(resp.ReasoningContent) == "" {
			t.Fatalf("thinking block was not read off the recorded answer: %+v", resp)
		}
		if resp.ReasoningSignature == "" {
			t.Fatal("thinking signature was not carried off the recorded answer")
		}
	},

	"B1-ANTHROPIC-MODELMAP": func(t *testing.T, fx *shimFixture) {
		// The mapping is provider-side; what our client must do is pass the
		// requested name through untouched so the documented mapping applies.
		client := anthropicTestClient(t, "https://api.deepseek.com/anthropic", &Config{Model: "claude-opus-4-20250514"})
		if client.model != "claude-opus-4-20250514" {
			t.Fatalf("client rewrote the requested model name to %q", client.model)
		}
		docs := strings.Join(ShimSupportMatrix().ProviderDocs, " ")
		if !strings.Contains(docs, "anthropic_api") {
			t.Fatal("the Anthropic API doc is not among the matrix's provider sources")
		}
	},

	"B1-RESPONSES-SHAPE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "responses", "minimal")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded Responses exchange is not accepted: %d", ex.HTTPStatus)
		}
		if want := "https://api.deepseek.com/responses"; ex.URL != want {
			t.Fatalf("recorded Responses URL %q, want %q", ex.URL, want)
		}
		if got := ResponsesEndpoint("https://api.deepseek.com"); got != ex.URL {
			t.Fatalf("ResponsesEndpoint = %q, want the recorded %q", got, ex.URL)
		}
		req, err := BuildResponsesRequest("deepseek-flash", []harness.Message{
			{Role: "system", Content: "You are terse."},
			{Role: "user", Content: "hi"},
		}, RequestOptions{})
		if err != nil {
			t.Fatalf("BuildResponsesRequest: %v", err)
		}
		wire, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal responses request: %v", err)
		}
		got := decodeMap(t, wire, "responses request")
		if _, ok := got["input"]; !ok {
			t.Fatalf("responses request has no input: %s", wire)
		}
		if got["instructions"] != "You are terse." {
			t.Fatalf("leading system message did not become instructions: %s", wire)
		}
		if _, ok := got["stream"]; !ok {
			t.Fatalf("responses request has no stream field: %s", wire)
		}
		// The recorded provider answer must be interpretable.
		var resp ResponsesResponse
		if err := json.Unmarshal(ex.Response, &resp); err != nil {
			t.Fatalf("decode recorded Responses answer: %v", err)
		}
		turn, err := InterpretResponsesResponse(&resp)
		if err != nil && !strings.Contains(err.Error(), "parse AgentOutput JSON") {
			t.Fatalf("interpret recorded Responses answer: %v", err)
		}
		if turn == nil || turn.ReasoningContent == "" {
			t.Fatalf("reasoning item was not read off the recorded answer: %+v", turn)
		}
	},

	"B1-RESPONSES-WIRING": func(t *testing.T, fx *shimFixture) {
		// The gap must be real: no provider case builds a Responses client.
		if _, err := NewClient(&Config{Provider: Provider("responses"), Model: "deepseek-flash"}); err == nil {
			t.Fatal("NewClient accepted a responses provider: the wiring row is stale")
		}
		if _, err := NewClient(&Config{Provider: Provider("deepseek-responses"), Model: "deepseek-flash"}); err == nil {
			t.Fatal("NewClient accepted a deepseek-responses provider: the wiring row is stale")
		}
		// ...while the wire layer itself is implemented and pinned above.
		if _, err := BuildResponsesRequest("deepseek-flash", []harness.Message{{Role: "user", Content: "hi"}}, RequestOptions{}); err != nil {
			t.Fatalf("responses builder unexpectedly unusable: %v", err)
		}
	},

	"B1-BETA-PREFIX": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "beta", "prefix_completion")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded prefix-completion exchange was not accepted: %d", ex.HTTPStatus)
		}
		if !strings.Contains(ex.URL, "/beta/") {
			t.Fatalf("recorded prefix-completion URL %q is not on the beta base", ex.URL)
		}
		req := decodeMap(t, ex.Request, "recorded prefix request")
		msgs, ok := req["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Fatalf("recorded prefix request carries no messages: %s", ex.Request)
		}
		last, ok := msgs[len(msgs)-1].(map[string]any)
		if !ok || last["prefix"] != true {
			t.Fatalf("recorded prefix request does not set prefix=true on the last assistant message: %s", ex.Request)
		}
		assertUnsupported(t, "B1-BETA-PREFIX", "prefix")
	},

	"B1-BETA-FIM": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "beta", "fim_completion")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded FIM exchange was not accepted: %d", ex.HTTPStatus)
		}
		req := decodeMap(t, ex.Request, "recorded FIM request")
		if _, ok := req["prompt"]; !ok {
			t.Fatalf("recorded FIM request has no prompt: %s", ex.Request)
		}
		if _, ok := req["suffix"]; !ok {
			t.Fatalf("recorded FIM request has no suffix: %s", ex.Request)
		}
		resp := decodeMap(t, ex.Response, "recorded FIM response")
		if resp["object"] != "text_completion" {
			t.Fatalf("recorded FIM response object = %v, want text_completion", resp["object"])
		}
		assertUnsupported(t, "B1-BETA-FIM", "fim")
	},

	// ---- §B2 ---------------------------------------------------------------
	"B2-OPENAI-TOGGLE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		off := false
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request), RequestOptions{Thinking: &off}, ex)
		if got := dig(t, body, "thinking", "type"); got != "disabled" {
			t.Fatalf("thinking.type = %v, want disabled", got)
		}
		on := true
		body = callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request), RequestOptions{Thinking: &on}, ex)
		if got := dig(t, body, "thinking", "type"); got != "enabled" {
			t.Fatalf("thinking.type = %v, want enabled", got)
		}
	},

	"B2-OPENAI-EFFORT": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "reasoning_effort_none")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded effort=none exchange was not accepted: %d", ex.HTTPStatus)
		}
		// The documented effect: no reasoning content came back.
		resp := decodeMap(t, ex.Response, "recorded none response")
		msg := dig(t, resp, "choices").([]any)[0].(map[string]any)["message"].(map[string]any)
		if _, present := msg["reasoning_content"]; present && msg["reasoning_content"] != nil {
			t.Fatalf("recorded effort=none answer carried reasoning_content: %v", msg["reasoning_content"])
		}
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{ReasoningEffort: "none"}, ex)
		if got := body["reasoning_effort"]; got != "none" {
			t.Fatalf("reasoning_effort = %v, want none (documented off value)", got)
		}
		if _, err := MapReasoningEffort("none"); err != nil {
			t.Fatalf("\"none\" is not part of the effort vocabulary: %v", err)
		}
	},

	"B2-ANTHROPIC-TOGGLE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "two_turn_effort_none_still_thinks")
		off := false
		body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{Thinking: &off}, ex)
		if got := dig(t, body, "reasoning", "effort"); got != "none" {
			t.Fatalf("reasoning.effort = %v, want none (documented off spelling)", got)
		}
		if _, present := body["output_config"]; present {
			t.Fatalf("output_config sent with thinking off: %s", mustJSON(t, body))
		}
	},

	"B2-ANTHROPIC-TOGGLE-EFFECT": func(t *testing.T, fx *shimFixture) {
		// This cell is not-supported because the DOCUMENTED EFFECT does not
		// happen: the recorded exchange requested effort "none" and thinking
		// still came back. The cell is only honest if that is what the bytes say.
		ex := fx.must(t, "anthropic", "two_turn_effort_none_still_thinks")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded effort=none exchange is not an accepted request: %d", ex.HTTPStatus)
		}
		req := decodeMap(t, ex.Request, "recorded request")
		if got := dig(t, req, "reasoning", "effort"); got != "none" {
			t.Fatalf("recorded request did not use the documented off spelling: %v", got)
		}
		resp := decodeMap(t, ex.Response, "recorded response")
		blocks, ok := resp["content"].([]any)
		if !ok {
			t.Fatalf("recorded response has no content blocks: %s", ex.Response)
		}
		sawThinking := false
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok && m["type"] == "thinking" {
				if s, _ := m["thinking"].(string); strings.TrimSpace(s) != "" {
					sawThinking = true
				}
			}
		}
		if !sawThinking {
			t.Fatalf("provider behaviour changed: effort \"none\" no longer returns thinking — re-derive this cell (§E) against the recorded exchange instead of trusting the old measurement")
		}
		assertUnsupported(t, "B2-ANTHROPIC-TOGGLE-EFFECT", "none")
	},

	"B2-ANTHROPIC-EFFORT": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "output_config_effort")
		if ex.HTTPStatus != 200 {
			t.Fatalf("recorded output_config exchange was not accepted: %d", ex.HTTPStatus)
		}
		on := true
		body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request),
			RequestOptions{Thinking: &on, ReasoningEffort: "max"}, ex)
		if got := dig(t, body, "output_config", "effort"); got != "max" {
			t.Fatalf("output_config.effort = %v, want max", got)
		}
		if got := dig(t, body, "reasoning", "effort"); got != "max" {
			t.Fatalf("reasoning.effort = %v, want max (toggle carries the same dial)", got)
		}
		// medium maps to high on the dial, so the wire never carries "medium".
		body, _ = callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request),
			RequestOptions{Thinking: &on, ReasoningEffort: "medium"}, ex)
		if got := dig(t, body, "output_config", "effort"); got != "high" {
			t.Fatalf("medium mapped to %v, want high", got)
		}
	},

	"B2-RESPONSES-EFFORT": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "responses", "effort_both_spellings")
		resp := decodeMap(t, ex.Response, "recorded responses answer")
		if got := dig(t, resp, "reasoning", "effort"); got != "max" {
			t.Fatalf("recorded answer echoed reasoning.effort = %v, want max", got)
		}
		on := true
		req, err := BuildResponsesRequest("deepseek-flash", []harness.Message{{Role: "user", Content: "hi"}},
			RequestOptions{Thinking: &on, ReasoningEffort: "max"})
		if err != nil {
			t.Fatalf("BuildResponsesRequest: %v", err)
		}
		if req.Reasoning == nil || req.Reasoning.Effort != "max" {
			t.Fatalf("reasoning.effort = %+v, want max", req.Reasoning)
		}
	},

	"B2-RESPONSES-OUTPUT-CONFIG": func(t *testing.T, fx *shimFixture) {
		// The spec's spelling for this format is inert provider-side. Prove the
		// measurement, then prove our builder does not send a decorative field.
		ex := fx.must(t, "responses", "effort_both_spellings")
		req := decodeMap(t, ex.Request, "recorded request")
		if _, ok := req["output_config"]; !ok {
			t.Fatal("the recorded request did not carry output_config: the divergence claim is untestable")
		}
		respRaw := string(ex.Response)
		if strings.Contains(respRaw, "output_config") {
			t.Fatal("recorded answer echoed output_config: re-derive this cell against the provider")
		}
		out, err := BuildResponsesRequest("deepseek-flash", []harness.Message{{Role: "user", Content: "hi"}},
			RequestOptions{ReasoningEffort: "low"})
		if err != nil {
			t.Fatalf("BuildResponsesRequest: %v", err)
		}
		wire, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(wire), "output_config") {
			t.Fatalf("builder sent output_config, which the provider ignores: %s", wire)
		}
		assertUnsupported(t, "B2-RESPONSES-OUTPUT-CONFIG", "output_config")
	},

	"B2-DEFAULT": func(t *testing.T, fx *shimFixture) {
		// Unconfigured: nothing about the dial goes on the wire, on either format.
		ex := fx.must(t, "anthropic", "messages_minimal_thinking_default")
		body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
		if _, present := body["reasoning"]; present {
			t.Fatalf("unconfigured client sent a reasoning toggle: %s", mustJSON(t, body))
		}
		if _, present := body["output_config"]; present {
			t.Fatalf("unconfigured client sent an effort dial: %s", mustJSON(t, body))
		}
		// ...and the provider's default is thinking ON, as the recorded answer shows.
		resp := decodeMap(t, ex.Response, "recorded response")
		found := false
		for _, b := range resp["content"].([]any) {
			if m, ok := b.(map[string]any); ok && m["type"] == "thinking" {
				found = true
			}
		}
		if !found {
			t.Fatal("recorded default request did not come back with thinking: the default claim is unsupported")
		}
		if defaultReasoningEffort != "high" {
			t.Fatalf("documented default effort = %q, want high", defaultReasoningEffort)
		}
	},

	"B2-MAX-TOKENS": func(t *testing.T, fx *shimFixture) {
		if err := ValidateMaxTokens(0); err != nil {
			t.Fatalf("unset max_tokens refused: %v", err)
		}
		if err := ValidateMaxTokens(1); err != nil {
			t.Fatalf("lower bound refused: %v", err)
		}
		if err := ValidateMaxTokens(maxTokensCeiling); err != nil {
			t.Fatalf("documented ceiling refused: %v", err)
		}
		for _, bad := range []int{-1, maxTokensCeiling + 1} {
			if err := ValidateMaxTokens(bad); err == nil {
				t.Fatalf("max_tokens %d accepted despite the documented range", bad)
			} else if !strings.Contains(err.Error(), "max_tokens") {
				t.Fatalf("refusal did not name max_tokens: %v", err)
			}
		}
		// A violating configuration must fail closed at construction...
		if _, err := NewClient(&Config{Provider: ProviderOpenAI, Model: "deepseek-flash",
			MaxTokens: maxTokensCeiling + 1}); err == nil {
			t.Fatal("NewClient accepted an out-of-range max_tokens")
		}
		// ...and a call must refuse it before any request is sent.
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		srv, captured := replay(t, ex)
		client := openaiTestClient(t, srv.URL, &Config{MaxTokens: maxTokensCeiling + 1})
		if _, err := client.Call(context.Background(), requestMessages(t, ex.Request)); err == nil {
			t.Fatal("call proceeded with an out-of-range max_tokens")
		}
		if len(*captured) != 0 {
			t.Fatal("a request was sent despite the local refusal")
		}
	},

	"B2-EFFORT-MAP": func(t *testing.T, fx *shimFixture) {
		want := map[string]string{
			"minimal": "low", "low": "low", "medium": "high", "high": "high",
			"xhigh": "high", "max": "max", "ultra": "max",
		}
		for name, dial := range want {
			got, err := MapReasoningEffort(name)
			if err != nil {
				t.Fatalf("MapReasoningEffort(%q): %v", name, err)
			}
			if got != dial {
				t.Fatalf("MapReasoningEffort(%q) = %q, want %q", name, got, dial)
			}
		}
		if got, err := MapReasoningEffort(""); err != nil || got != "" {
			t.Fatalf("empty effort = (%q, %v), want (\"\", nil)", got, err)
		}
		if got, err := MapReasoningEffort("  MAX "); err != nil || got != "max" {
			t.Fatalf("effort normalisation failed: (%q, %v)", got, err)
		}
	},

	"B2-UNKNOWN-EFFORT": func(t *testing.T, fx *shimFixture) {
		if _, err := MapReasoningEffort("bogus"); err == nil {
			t.Fatal("an unknown effort was accepted; it must be refused, never silently dropped")
		}
		// The refusal must reach the caller of a real call, not just the helper.
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		srv, _ := replay(t, ex)
		client := openaiTestClient(t, srv.URL, nil)
		if _, err := client.CallWithOptions(context.Background(), requestMessages(t, ex.Request),
			RequestOptions{ReasoningEffort: "bogus"}); err == nil {
			t.Fatal("CallWithOptions accepted an unknown effort")
		} else if !strings.Contains(err.Error(), "unknown reasoning effort") {
			t.Fatalf("refusal did not name the problem: %v", err)
		}
	},

	"B2-COT-TRANSPORT": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_tools_auto")
		resp := decodeMap(t, ex.Response, "recorded response")
		msg := dig(t, resp, "choices").([]any)[0].(map[string]any)["message"].(map[string]any)
		if _, ok := msg["reasoning_content"]; !ok {
			t.Fatal("recorded answer has no reasoning_content sibling of content")
		}
		if _, ok := msg["content"]; !ok {
			t.Fatal("recorded answer has no content field")
		}
		srv, _ := replay(t, ex)
		client := openaiTestClient(t, srv.URL, nil)
		turn, err := client.Call(context.Background(), requestMessages(t, ex.Request))
		if turn == nil {
			t.Fatalf("no turn: %v", err)
		}
		if got, want := turn.ReasoningContent, msg["reasoning_content"]; got != want {
			t.Fatalf("client read reasoning_content %q, want the recorded %v", got, want)
		}
	},

	"B2-COT-ROUNDTRIP": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_tools_auto")
		tools := recordedTools(t, ex.Request)
		if len(tools) == 0 {
			t.Fatal("recorded exchange carries no tools to drive the round-trip")
		}
		prior := []harness.Message{
			{Role: "user", Content: "question"},
			{Role: "assistant", Content: "", ReasoningContent: "earlier chain of thought"},
		}
		withTools := callOpenAI(t, openaiTestClient(t, "", nil), prior, RequestOptions{Tools: tools}, ex)
		msgs := withTools["messages"].([]any)
		if got := msgs[1].(map[string]any)["reasoning_content"]; got != "earlier chain of thought" {
			t.Fatalf("with tools, prior reasoning_content = %v, want it re-sent", got)
		}
		withoutTools := callOpenAI(t, openaiTestClient(t, "", nil), prior, RequestOptions{}, ex)
		msgs = withoutTools["messages"].([]any)
		if got, present := msgs[1].(map[string]any)["reasoning_content"]; present && got != "" {
			t.Fatalf("without tools, prior reasoning_content = %v, want it omitted", got)
		}
	},

	"B2-COT-REQUIRED-ANTHROPIC": func(t *testing.T, fx *shimFixture) {
		// The refusal arm: an assistant tool_use turn with no thinking block.
		missing := fx.must(t, "anthropic", "mid_toolcall_missing_thinking_400")
		if missing.HTTPStatus != 400 {
			t.Fatalf("recorded missing-thinking exchange http_status = %d, want the measured 400", missing.HTTPStatus)
		}
		errMsg := string(missing.Response)
		if !strings.Contains(errMsg, "thinking") {
			t.Fatalf("recorded refusal does not name thinking: %s", errMsg)
		}
		// The accepted arm: the same shape WITH its thinking block.
		echo := fx.must(t, "anthropic", "tools_tooluse_thinking_echo")
		if echo.HTTPStatus != 200 {
			t.Fatalf("recorded echoed-thinking exchange http_status = %d, want 200", echo.HTTPStatus)
		}
		tools := []ToolDefinition{{Type: "function", Function: ToolFunctionDef{Name: "get_date"}}}
		msgs := []harness.Message{
			{Role: "user", Content: "What is today's date?"},
			{Role: "assistant",
				ReasoningContent:   "I should call get_date.",
				ReasoningSignature: "sig-1",
				ToolCalls: []harness.ToolCall{{ID: "toolu_1", Type: "function",
					Function: harness.ToolCallFunction{Name: "get_date", Arguments: "{}"}}}},
			{Role: "tool", ToolCallID: "toolu_1", Content: "2026-10-13"},
		}
		body, _ := callAnthropic(t, "/anthropic", nil, msgs, RequestOptions{Tools: tools}, echo)
		blocks, ok := body["messages"].([]any)[1].(map[string]any)["content"].([]any)
		if !ok {
			t.Fatalf("assistant turn is not a block list: %s", mustJSON(t, body))
		}
		var sawThinking, sawToolUse bool
		for _, b := range blocks {
			m := b.(map[string]any)
			if m["type"] == "thinking" {
				sawThinking = true
				if m["signature"] != "sig-1" {
					t.Fatalf("thinking signature not echoed: %v", m)
				}
			}
			if m["type"] == "tool_use" {
				sawToolUse = true
			}
		}
		if !sawThinking || !sawToolUse {
			t.Fatalf("assistant turn blocks = %s, want thinking + tool_use", mustJSON(t, body))
		}
	},

	"B2-COT-PERSIST": func(t *testing.T, fx *shimFixture) {
		// The turn object handed back to the harness must carry the CoT and its
		// signature; persistence into iteration_commits.llm_response is asserted
		// by the harness package (internal/harness/turn_snapshot_test.go), which
		// owns the snapshot.
		checkField := func(v any, field, jsonTag string) {
			t.Helper()
			typ := reflect.TypeOf(v)
			f, ok := typ.FieldByName(field)
			if !ok {
				t.Fatalf("%s has no %s field: the CoT cannot ride the turn", typ, field)
			}
			if got := f.Tag.Get("json"); !strings.HasPrefix(got, jsonTag) {
				t.Fatalf("%s.%s json tag = %q, want %q (a rename would silently drop it)", typ, field, got, jsonTag)
			}
		}
		checkField(harness.LLMResponse{}, "ReasoningContent", "reasoning_content")
		checkField(harness.LLMResponse{}, "ReasoningSignature", "reasoning_signature")
		checkField(harness.Message{}, "ReasoningContent", "reasoning_content")
		checkField(harness.Message{}, "ReasoningSignature", "reasoning_signature")

		// The Anthropic client populates both from the provider's own blocks.
		ex := fx.must(t, "anthropic", "thinking_on_with_text")
		_, turn := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
		if turn == nil || turn.ReasoningContent == "" || turn.ReasoningSignature == "" {
			t.Fatalf("turn did not carry CoT + signature: %+v", turn)
		}
	},

	"B2-EFFORT-WITH-THINKING-OFF": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		off := false
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{Thinking: &off, ReasoningEffort: "max"}, ex)
		if got, present := body["reasoning_effort"]; present && got != "" {
			t.Fatalf("effort sent with thinking off: %v", got)
		}
		anth := fx.must(t, "anthropic", "messages_minimal_thinking_default")
		abody, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, anth.Request),
			RequestOptions{Thinking: &off, ReasoningEffort: "max"}, anth)
		if _, present := abody["output_config"]; present {
			t.Fatalf("anthropic effort dial sent with thinking off: %s", mustJSON(t, abody))
		}
	},

	"B2-COT-AS-ANSWER": func(t *testing.T, fx *shimFixture) {
		// A recorded answer that is a thinking block with no text at all.
		ex := fx.must(t, "anthropic", "messages_minimal_thinking_default")
		resp := decodeMap(t, ex.Response, "recorded response")
		var onlyThinking = true
		for _, b := range resp["content"].([]any) {
			if m, ok := b.(map[string]any); ok && m["type"] != "thinking" {
				onlyThinking = false
			}
		}
		if !onlyThinking {
			t.Fatal("fixture is not a thinking-only answer: re-derive this cell")
		}
		_, turn := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
		if turn == nil {
			t.Fatal("no turn returned for a thinking-only answer")
		}
		if !turn.ReasoningPromotedToOutput {
			t.Fatal("promotion was not flagged on the response")
		}
		if strings.TrimSpace(turn.Content) == "" {
			t.Fatal("promoted content is empty")
		}
		// ...and it is disableable.
		no := false
		_, turn = callAnthropic(t, "/anthropic", &Config{AllowReasoningFallback: &no},
			requestMessages(t, ex.Request), RequestOptions{}, ex)
		if turn == nil {
			t.Fatal("no turn returned with the fallback disabled")
		}
		if turn.ReasoningPromotedToOutput {
			t.Fatal("promotion happened although the fallback was disabled")
		}
		if strings.TrimSpace(turn.Content) != "" {
			t.Fatalf("content = %q, want empty when promotion is refused", turn.Content)
		}
	},

	"B2-INERT-PARAMS": func(t *testing.T, fx *shimFixture) {
		// The recorded request set all three parameters and was accepted.
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		req := decodeMap(t, ex.Request, "recorded request")
		for _, p := range []string{"temperature", "presence_penalty", "frequency_penalty"} {
			if _, ok := req[p]; !ok {
				t.Fatalf("recorded request is missing the documented-ignored parameter %q", p)
			}
		}
		if ex.HTTPStatus != 200 {
			t.Fatalf("provider rejected the ignored parameters: http_status=%d", ex.HTTPStatus)
		}
		on := true
		got := InertThinkingParams(&on)
		if strings.Join(got, ",") != "temperature,presence_penalty,frequency_penalty" {
			t.Fatalf("inert set = %v", got)
		}
		if in := InertThinkingParams(nil); len(in) != 3 {
			t.Fatalf("unconfigured thinking must be treated as the provider default (on): %v", in)
		}
		off := false
		if in := InertThinkingParams(&off); len(in) != 0 {
			t.Fatalf("parameters are not inert with thinking off: %v", in)
		}
	},

	"B2-TOP-P": func(t *testing.T, fx *shimFixture) {
		cases := []struct {
			requested float64
			thinking  *bool
			want      float64
			send      bool
		}{
			{0, nil, 0, false},
			{0.5, nil, 0.95, true},
			{0.5, boolPtr(true), 0.95, true},
			{0.98, boolPtr(true), 0.98, true},
			{1.5, boolPtr(true), 1.0, true},
			{0.5, boolPtr(false), 0, false},
		}
		for _, tc := range cases {
			got, send := ResolveTopP(tc.requested, tc.thinking)
			if send != tc.send {
				t.Fatalf("ResolveTopP(%v, %v) send = %v, want %v", tc.requested, thinkingLabel(tc.thinking), send, tc.send)
			}
			if send && got != tc.want {
				t.Fatalf("ResolveTopP(%v, %v) = %v, want %v", tc.requested, thinkingLabel(tc.thinking), got, tc.want)
			}
		}
		// ...and the clamp is what goes on the wire.
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		half := 0.5
		on := true
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{TopP: &half, Thinking: &on}, ex)
		if got := body["top_p"]; got != 0.95 {
			t.Fatalf("top_p on the wire = %v, want the clamped 0.95", got)
		}
		off := false
		body = callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{TopP: &half, Thinking: &off}, ex)
		if _, present := body["top_p"]; present {
			t.Fatalf("top_p sent in non-thinking mode: %s", mustJSON(t, body))
		}
	},

	// ---- §B3 ---------------------------------------------------------------
	"B3-TOOLS-SHAPE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_tools_auto")
		rec := decodeMap(t, ex.Request, "recorded request")
		recTools := rec["tools"].([]any)
		first := recTools[0].(map[string]any)
		fn, ok := first["function"].(map[string]any)
		if !ok {
			t.Fatalf("recorded tools are not in the function-nested shape: %s", ex.Request)
		}
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{Tools: recordedTools(t, ex.Request)}, ex)
		outTools := body["tools"].([]any)
		if len(outTools) != len(recTools) {
			t.Fatalf("tool count = %d, want %d", len(outTools), len(recTools))
		}
		gotFn := outTools[0].(map[string]any)["function"].(map[string]any)
		for _, key := range []string{"name", "description", "parameters"} {
			if _, want := fn[key]; !want {
				continue
			}
			if _, present := gotFn[key]; !present {
				t.Fatalf("outgoing tool function lost %q: %s", key, mustJSON(t, body))
			}
		}
	},

	"B3-TOOLS-ANTHROPIC-SHAPE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "tools_tooluse_thinking_echo")
		rec := decodeMap(t, ex.Request, "recorded request")
		recTool := rec["tools"].([]any)[0].(map[string]any)
		if _, nested := recTool["function"]; nested {
			t.Fatal("recorded Anthropic tool is in the OpenAI-nested shape")
		}
		for _, key := range []string{"name", "description", "input_schema"} {
			if _, ok := recTool[key]; !ok {
				t.Fatalf("recorded Anthropic tool is missing %q: %s", key, ex.Request)
			}
		}
		tools := []ToolDefinition{{Type: "function", Function: ToolFunctionDef{
			Name: "get_date", Description: "Get the current date", Parameters: json.RawMessage(`{"type":"object"}`),
		}}}
		body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{Tools: tools}, ex)
		outTool := body["tools"].([]any)[0].(map[string]any)
		if outTool["name"] != "get_date" {
			t.Fatalf("outgoing tool name = %v", outTool["name"])
		}
		if _, ok := outTool["input_schema"]; !ok {
			t.Fatalf("outgoing Anthropic tool lost input_schema: %s", mustJSON(t, body))
		}
		if _, nested := outTool["function"]; nested {
			t.Fatalf("outgoing Anthropic tool was nested under function: %s", mustJSON(t, body))
		}
	},

	"B3-TOOL-LIMITS": func(t *testing.T, fx *shimFixture) {
		long := strings.Repeat("a", maxToolNameLen)
		if err := ValidateToolDefinitions([]ToolDefinition{{Function: ToolFunctionDef{Name: long}}}); err != nil {
			t.Fatalf("%d-character name rejected: %v", maxToolNameLen, err)
		}
		tooLong := strings.Repeat("a", maxToolNameLen+1)
		if err := ValidateToolDefinitions([]ToolDefinition{{Function: ToolFunctionDef{Name: tooLong}}}); err == nil {
			t.Fatal("a name over the documented cap was accepted")
		}
		for _, bad := range []string{"has space", "has.dot", "has/slash", ""} {
			if err := ValidateToolDefinitions([]ToolDefinition{{Function: ToolFunctionDef{Name: bad}}}); err == nil {
				t.Fatalf("invalid tool name %q was accepted", bad)
			}
		}
		if err := ValidateToolDefinitions([]ToolDefinition{
			{Function: ToolFunctionDef{Name: "dup"}},
			{Function: ToolFunctionDef{Name: "dup"}},
		}); err == nil {
			t.Fatal("duplicate tool names were accepted")
		}
	},

	"B3-TOOL-CHOICE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_tools_auto")
		off := false
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request),
			RequestOptions{Tools: recordedTools(t, ex.Request), ToolChoice: "auto", Thinking: &off}, ex)
		if body["tool_choice"] != "auto" {
			t.Fatalf("openai tool_choice = %v, want auto", body["tool_choice"])
		}
		// Anthropic vocabulary mapping.
		cases := map[string]string{"none": "none", "auto": "auto", "required": "any", "get_date": "tool"}
		for in, want := range cases {
			got := toAnthropicToolChoice(in)
			if got == nil || got.Type != want {
				t.Fatalf("toAnthropicToolChoice(%q) = %+v, want type %q", in, got, want)
			}
		}
		if got := toAnthropicToolChoice("get_date"); got.Name != "get_date" {
			t.Fatalf("named choice lost its name: %+v", got)
		}
		// Responses vocabulary mapping.
		if got := toResponsesToolChoice("required"); got != "required" {
			t.Fatalf("responses required = %v", got)
		}
		if m, ok := toResponsesToolChoice("get_date").(map[string]string); !ok || m["name"] != "get_date" {
			t.Fatalf("responses named choice = %v", toResponsesToolChoice("get_date"))
		}
	},

	"B3-TOOL-CHOICE-THINKING": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "tool_choice_required_thinking_400")
		if ex.HTTPStatus != 400 {
			t.Fatalf("recorded required-in-thinking exchange http_status = %d, want the measured 400", ex.HTTPStatus)
		}
		if !strings.Contains(strings.ToLower(string(ex.Response)), "tool_choice") {
			t.Fatalf("recorded refusal does not name tool_choice: %s", ex.Response)
		}
		on := true
		if err := ValidateToolChoice("required", &on); err == nil {
			t.Fatal("required tool_choice accepted in thinking mode")
		}
		if err := ValidateToolChoice("get_date", &on); err == nil {
			t.Fatal("named tool_choice accepted in thinking mode")
		}
		if err := ValidateToolChoice("required", nil); err == nil {
			t.Fatal("required tool_choice accepted with the provider default (thinking on)")
		}
		off := false
		if err := ValidateToolChoice("required", &off); err != nil {
			t.Fatalf("required tool_choice refused outside thinking mode: %v", err)
		}
		if err := ValidateToolChoice("auto", &on); err != nil {
			t.Fatalf("auto refused in thinking mode: %v", err)
		}
		// The refusal must happen before a request is sent.
		srv, captured := replay(t, ex)
		client := openaiTestClient(t, srv.URL, nil)
		if _, err := client.CallWithOptions(context.Background(), requestMessages(t, ex.Request),
			RequestOptions{Tools: recordedTools(t, ex.Request), ToolChoice: "required", Thinking: &on}); err == nil {
			t.Fatal("CallWithOptions sent a request the provider refuses")
		}
		if len(*captured) != 0 {
			t.Fatal("a request was sent despite the local refusal")
		}
	},

	"B3-RESPONSE-SHAPE-OPENAI": func(t *testing.T, fx *shimFixture) {
		cot := loadCOTFixture(t)
		recorded := mustDecodeRecorded(t, cot.ToolsPresent.Turn1.Response, "tool-call turn")
		if len(recorded.Choices) == 0 || len(recorded.Choices[0].Message.ToolCalls) == 0 {
			t.Fatal("recorded turn carries no tool_calls")
		}
		wantCall := recorded.Choices[0].Message.ToolCalls[0]
		srv, _ := replayRaw(t, cot.ToolsPresent.Turn1.Response)
		client := openaiTestClient(t, srv.URL, nil)
		turn, err := client.Call(context.Background(), recordedMessages(t, cot.ToolsPresent.Turn1.Request))
		if turn == nil {
			t.Fatalf("no turn: %v", err)
		}
		if len(turn.ToolCalls) != 1 {
			t.Fatalf("client read %d tool calls, want 1", len(turn.ToolCalls))
		}
		if turn.ToolCalls[0].ID != wantCall.ID || turn.ToolCalls[0].Function.Name != wantCall.Function.Name {
			t.Fatalf("tool call = %+v, want the recorded %+v", turn.ToolCalls[0], wantCall)
		}
		// The result must go back as a role:"tool" message with tool_call_id.
		cont := append(recordedMessages(t, cot.ToolsPresent.Turn1.Request),
			harness.Message{Role: "assistant", ReasoningContent: turn.ReasoningContent, ToolCalls: turn.ToolCalls},
			harness.Message{Role: "tool", ToolCallID: turn.ToolCalls[0].ID, Content: "42"},
		)
		body := callOpenAI(t, openaiTestClient(t, "", nil), cont, RequestOptions{}, fx.must(t, "openai", "thinking_tools_auto"))
		msgs := body["messages"].([]any)
		last := msgs[len(msgs)-1].(map[string]any)
		if last["role"] != "tool" || last["tool_call_id"] == "" {
			t.Fatalf("tool result message = %s, want role tool + tool_call_id", mustJSON(t, body))
		}
		asst := msgs[len(msgs)-2].(map[string]any)
		if _, ok := asst["tool_calls"]; !ok {
			t.Fatalf("assistant tool_calls were not echoed back: %s", mustJSON(t, body))
		}
	},

	"B3-RESPONSE-SHAPE-ANTHROPIC": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "tools_plain_assistant")
		resp := decodeMap(t, ex.Response, "recorded response")
		var wantName, wantID string
		for _, b := range resp["content"].([]any) {
			m := b.(map[string]any)
			if m["type"] == "tool_use" {
				wantName, _ = m["name"].(string)
				wantID, _ = m["id"].(string)
			}
		}
		if wantName == "" || wantID == "" {
			t.Fatal("recorded answer carries no tool_use block")
		}
		_, turn := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
		if turn == nil || len(turn.ToolCalls) != 1 {
			t.Fatalf("client read %+v, want one tool call", turn)
		}
		if turn.ToolCalls[0].Function.Name != wantName || turn.ToolCalls[0].ID != wantID {
			t.Fatalf("tool call = %+v, want %s/%s", turn.ToolCalls[0], wantID, wantName)
		}
		if turn.ToolCalls[0].Function.Arguments == "" {
			t.Fatal("tool call arguments are empty; the provider's input object was dropped")
		}
		// A role:"tool" result must become a tool_result block.
		msgs := []harness.Message{
			{Role: "user", Content: "date?"},
			{Role: "assistant", ToolCalls: turn.ToolCalls},
			{Role: "tool", ToolCallID: wantID, Content: "2026-10-13"},
		}
		body, _ := callAnthropic(t, "/anthropic", nil, msgs, RequestOptions{Tools: []ToolDefinition{
			{Function: ToolFunctionDef{Name: wantName}},
		}}, ex)
		sent := body["messages"].([]any)
		lastBlocks := sent[len(sent)-1].(map[string]any)["content"].([]any)
		block := lastBlocks[0].(map[string]any)
		if block["type"] != "tool_result" || block["tool_use_id"] != wantID {
			t.Fatalf("tool result block = %v, want tool_result with tool_use_id %s", block, wantID)
		}
	},

	"B3-MIDCOV-CHAT-COMPLETIONS": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "openai", "thinking_disabled_inert_params")
		msgs := []harness.Message{
			{Role: "system", Content: "lead"},
			{Role: "user", Content: "u1"},
			{Role: "system", Content: "injected mid-conversation"},
			{Role: "user", Content: "u2"},
		}
		body := callOpenAI(t, openaiTestClient(t, "", nil), msgs, RequestOptions{}, ex)
		got := body["messages"].([]any)
		wantRoles := []string{"system", "user", "system", "user"}
		if len(got) != len(wantRoles) {
			t.Fatalf("message count = %d, want %d: %s", len(got), len(wantRoles), mustJSON(t, body))
		}
		for i, role := range wantRoles {
			if got[i].(map[string]any)["role"] != role {
				t.Fatalf("message %d role = %v, want %v (mid-conversation system must stay in place)",
					i, got[i].(map[string]any)["role"], role)
			}
		}
		if got[2].(map[string]any)["content"] != "injected mid-conversation" {
			t.Fatalf("mid-conversation system content was altered: %s", mustJSON(t, body))
		}
	},

	"B3-MIDCOV-INJECTED-CHAT-COMPLETIONS": func(t *testing.T, fx *shimFixture) {
		// Not-applicable is only honest if the capability lives somewhere else:
		// the sibling cells must carry it, and be supported.
		m := ShimSupportMatrix()
		byID := map[string]ShimCell{}
		for _, c := range m.Cells {
			byID[c.ID] = c
		}
		for _, sibling := range []string{"B3-MIDCOV-ANTHROPIC", "B3-MIDCOV-RESPONSES"} {
			c, ok := byID[sibling]
			if !ok {
				t.Fatalf("mid-conversation injection is not carried by %s either", sibling)
			}
			if c.State != CellSupported {
				t.Fatalf("%s is %s, so injection is supported nowhere", sibling, c.State)
			}
		}
	},

	"B3-MIDCOV-ANTHROPIC": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "mid_conversation_system")
		msgs := []harness.Message{
			{Role: "system", Content: "lead"},
			{Role: "user", Content: "u1"},
			{Role: "assistant", Content: "a1"},
			{Role: "system", Content: "injected mid-conversation"},
			{Role: "user", Content: "u2"},
		}
		body, _ := callAnthropic(t, "/anthropic", nil, msgs, RequestOptions{}, ex)
		sysBlocks := body["system"].([]any)
		if len(sysBlocks) != 1 {
			t.Fatalf("system parameter carries %d blocks, want only the leading run: %s", len(sysBlocks), mustJSON(t, body))
		}
		got := body["messages"].([]any)
		wantRoles := []string{"user", "assistant", "system", "user"}
		if len(got) != len(wantRoles) {
			t.Fatalf("message count = %d, want %d: %s", len(got), len(wantRoles), mustJSON(t, body))
		}
		for i, role := range wantRoles {
			if got[i].(map[string]any)["role"] != role {
				t.Fatalf("message %d role = %v, want %v", i, got[i].(map[string]any)["role"], role)
			}
		}
		if got[2].(map[string]any)["content"] != "injected mid-conversation" {
			t.Fatalf("mid-conversation system content was altered: %s", mustJSON(t, body))
		}
	},

	"B3-MIDCOV-RESPONSES": func(t *testing.T, fx *shimFixture) {
		msgs := []harness.Message{
			{Role: "system", Content: "lead"},
			{Role: "user", Content: "u1"},
			{Role: "agent", Content: "ignored"},
			{Role: "assistant", ToolCalls: []harness.ToolCall{{ID: "call_1", Function: harness.ToolCallFunction{Name: "t", Arguments: "{}"}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "42"},
			{Role: "system", Content: "injected mid-conversation"},
			{Role: "user", Content: "u2"},
		}
		req, err := BuildResponsesRequest("deepseek-flash", msgs, RequestOptions{})
		if err != nil {
			t.Fatalf("BuildResponsesRequest: %v", err)
		}
		if req.Instructions != "lead" {
			t.Fatalf("instructions = %q, want only the leading system message", req.Instructions)
		}
		var sawSystemMid, sawCallOutput bool
		for _, item := range req.Input {
			if item.Role == "system" && item.Content[0].Text == "injected mid-conversation" {
				sawSystemMid = true
			}
			if item.Type == "function_call_output" && item.CallID == "call_1" && item.Output == "42" {
				sawCallOutput = true
			}
			if item.Role == "agent" {
				t.Fatalf("unknown role leaked into the request: %+v", item)
			}
		}
		if !sawSystemMid || !sawCallOutput {
			t.Fatalf("responses input lost the mid-conversation system message or the tool result: %+v", req.Input)
		}
	},

	// ---- §B4 ---------------------------------------------------------------
	"B4-OPENAI-CACHE": func(t *testing.T, fx *shimFixture) {
		first := fx.must(t, "openai", "cache_turn1")
		second := fx.must(t, "openai", "cache_turn2")
		u1 := decodeMap(t, first.Response, "turn 1 response")["usage"].(map[string]any)
		u2 := decodeMap(t, second.Response, "turn 2 response")["usage"].(map[string]any)
		hit := int64(u2["prompt_cache_hit_tokens"].(float64))
		miss := int64(u2["prompt_cache_miss_tokens"].(float64))
		prompt := int64(u2["prompt_tokens"].(float64))
		if prompt != hit+miss {
			t.Fatalf("prompt_tokens %d != hit %d + miss %d", prompt, hit, miss)
		}
		if hit == 0 {
			t.Fatal("recorded cache-hit turn reports zero hit tokens: the accounting claim is untested")
		}
		nested := int64(u2["prompt_tokens_details"].(map[string]any)["cached_tokens"].(float64))
		if nested != hit {
			t.Fatalf("prompt_tokens_details.cached_tokens %d != prompt_cache_hit_tokens %d", nested, hit)
		}
		if int64(u1["prompt_cache_hit_tokens"].(float64)) != 0 {
			t.Fatalf("first turn was not a cache miss: %v", u1)
		}
		// The client must surface the hit as a cache read.
		srv, _ := replay(t, second)
		client := openaiTestClient(t, srv.URL, nil)
		turn, err := client.Call(context.Background(), requestMessages(t, second.Request))
		if turn == nil {
			t.Fatalf("no turn: %v", err)
		}
		if turn.Usage.CacheReadTokens != hit {
			t.Fatalf("CacheReadTokens = %d, want the recorded hit %d", turn.Usage.CacheReadTokens, hit)
		}
		// The provider reports the same number twice; either spelling must be
		// read, because OpenAI-compatible gateways send only one of them.
		if got := (openaiChatUsage{PromptCacheHitTokens: 42}).cacheReadTokens(); got != 42 {
			t.Fatalf("cacheReadTokens from prompt_cache_hit_tokens = %d, want 42", got)
		}
		nestedOnly := openaiChatUsage{PromptTokensDetails: &openaiPromptTokensDetails{CachedTokens: 7}}
		if got := nestedOnly.cacheReadTokens(); got != 7 {
			t.Fatalf("cacheReadTokens from prompt_tokens_details.cached_tokens = %d, want 7", got)
		}
		if got := (openaiChatUsage{}).cacheReadTokens(); got != 0 {
			t.Fatalf("cacheReadTokens with no cache fields = %d, want 0", got)
		}
	},

	"B4-ANTHROPIC-CACHE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "anthropic", "tools_tooluse_thinking_echo")
		usage := decodeMap(t, ex.Response, "recorded response")["usage"].(map[string]any)
		wantRead := int64(usage["cache_read_input_tokens"].(float64))
		wantIn := int64(usage["input_tokens"].(float64))
		wantOut := int64(usage["output_tokens"].(float64))
		if wantRead == 0 {
			t.Fatal("recorded exchange reports no cache reads: the accounting claim is untested")
		}
		_, turn := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
		if turn == nil {
			t.Fatal("no turn returned")
		}
		if turn.Usage.CacheReadTokens != wantRead {
			t.Fatalf("CacheReadTokens = %d, want the recorded %d", turn.Usage.CacheReadTokens, wantRead)
		}
		if turn.Usage.PromptTokens != wantIn || turn.Usage.CompletionTokens != wantOut {
			t.Fatalf("usage = %+v, want the recorded input %d / output %d", turn.Usage, wantIn, wantOut)
		}
		if turn.Usage.TotalTokens != wantIn+wantOut {
			t.Fatalf("total = %d, want %d", turn.Usage.TotalTokens, wantIn+wantOut)
		}
	},

	"B4-RESPONSES-CACHE": func(t *testing.T, fx *shimFixture) {
		ex := fx.must(t, "responses", "minimal")
		usage := decodeMap(t, ex.Response, "recorded response")["usage"].(map[string]any)
		var resp ResponsesResponse
		if err := json.Unmarshal(ex.Response, &resp); err != nil {
			t.Fatalf("decode recorded responses answer: %v", err)
		}
		if resp.Usage.InputTokens != int64(usage["input_tokens"].(float64)) {
			t.Fatalf("input tokens = %d, want the recorded %v", resp.Usage.InputTokens, usage["input_tokens"])
		}
		if resp.Usage.InputTokensDetails == nil {
			t.Fatal("recorded answer has no input_tokens_details: cache accounting is untestable")
		}
		turn, err := InterpretResponsesResponse(&resp)
		if err != nil && !strings.Contains(err.Error(), "parse AgentOutput JSON") {
			t.Fatalf("interpret: %v", err)
		}
		if turn == nil {
			t.Fatal("no turn")
		}
		want := int64(usage["input_tokens_details"].(map[string]any)["cached_tokens"].(float64))
		if turn.Usage.CacheReadTokens != want {
			t.Fatalf("cache read tokens = %d, want %d", turn.Usage.CacheReadTokens, want)
		}
		if turn.Usage.TotalTokens != int64(usage["total_tokens"].(float64)) {
			t.Fatalf("total tokens = %d, want the recorded %v", turn.Usage.TotalTokens, usage["total_tokens"])
		}
	},
}

// ============================================================================
// Matrix invariants + execution
// ============================================================================

// TestShimSupportMatrix runs every cell of the conformance matrix and fails on any
// malformed state, unsupported claim, or cell without a check.
func TestShimSupportMatrix(t *testing.T) {
	fx := loadShimFixture(t)
	m := ShimSupportMatrix()

	if m.DocDate != ShimDocDate || m.DocDate == "" {
		t.Fatalf("matrix doc date %q does not match the recorded constant %q", m.DocDate, ShimDocDate)
	}
	if _, err := time.Parse("2006-01-02", m.DocDate); err != nil {
		t.Fatalf("doc date %q is not an ISO date: %v", m.DocDate, err)
	}
	if len(m.ProviderDocs) == 0 {
		t.Fatal("matrix names no provider documentation")
	}
	if m.RefreshCommand == "" {
		t.Fatal("matrix does not document how to re-run itself")
	}

	seen := map[string]bool{}
	supported, notSupported, notApplicable := 0, 0, 0
	rowsToFile := map[string]string{}

	for _, cell := range m.Cells {
		if cell.ID == "" {
			t.Fatal("matrix cell with empty ID")
		}
		if seen[cell.ID] {
			t.Fatalf("duplicate matrix cell id %q", cell.ID)
		}
		seen[cell.ID] = true

		if cell.Item == "" || cell.Format == "" || cell.Doc == "" {
			t.Fatalf("cell %s is missing item/format/doc", cell.ID)
		}
		if len(cell.Evidence) == 0 {
			t.Fatalf("cell %s claims no evidence", cell.ID)
		}
		for _, e := range cell.Evidence {
			switch e {
			case EvidenceLive, EvidenceUnit, EvidenceDoc:
			default:
				t.Fatalf("cell %s has unknown evidence kind %q", cell.ID, e)
			}
		}

		switch cell.State {
		case CellSupported:
			supported++
			if cell.Row != "" {
				t.Fatalf("supported cell %s names a row (%s); states must be exclusive", cell.ID, cell.Row)
			}
		case CellNotSupported:
			notSupported++
			if !ShimRowPattern.MatchString(cell.Row) {
				t.Fatalf("not-supported cell %s must name a board row matching %s, got %q",
					cell.ID, ShimRowPattern.String(), cell.Row)
			}
			if strings.TrimSpace(cell.Reason) == "" {
				t.Fatalf("not-supported cell %s carries no reason", cell.ID)
			}
			rowsToFile[cell.Row] = cell.Item
		case CellNotApplicable:
			notApplicable++
			if strings.TrimSpace(cell.Reason) == "" {
				t.Fatalf("not-applicable cell %s carries no reason", cell.ID)
			}
			if cell.Row != "" {
				t.Fatalf("not-applicable cell %s names a row (%s); it needs a reason, not a row", cell.ID, cell.Row)
			}
		default:
			t.Fatalf("cell %s has no explicit state (got %q)", cell.ID, cell.State)
		}

		check, ok := shimChecks[cell.ID]
		if !ok {
			t.Fatalf("cell %s has no check — every cell must be exercised", cell.ID)
		}
		t.Run(cell.ID, func(t *testing.T) {
			check(t, fx)
		})
	}

	for id := range shimChecks {
		if !seen[id] {
			t.Fatalf("check for %q exists without a matrix cell", id)
		}
	}

	// The matrix is a report as much as a gate: print it so a run shows the
	// states, the evidence behind each one and any row still to be filed.
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-38s %-14s %-5s %-8s %s\n", "CELL", "FORMAT", "SEC", "STATE", "EVIDENCE")
	for _, c := range m.Cells {
		fmt.Fprintf(&b, "%-38s %-14s %-5s %-8s %s\n", c.ID, c.Format, c.Section, c.State, strings.Join(c.Evidence, "+"))
	}
	fmt.Fprintf(&b, "\nprovider docs read: %s | recorded fixture: %s\n", m.DocDate, m.RecordedFixture)
	fmt.Fprintf(&b, "cells: %d supported, %d not-supported, %d not-applicable (total %d)\n",
		supported, notSupported, notApplicable, len(m.Cells))
	if len(rowsToFile) > 0 {
		fmt.Fprintf(&b, "board rows these cells depend on: %d\n", len(rowsToFile))
		for row, item := range rowsToFile {
			fmt.Fprintf(&b, "  %s — %s\n", row, item)
		}
	}
	t.Log(b.String())
}

// TestShimMatrixSpecParity keeps specs/024 §D and the executable matrix from
// drifting: every cell must be listed in the spec table with the same state, the
// doc date must match, and the refresh command must be the one recorded.
func TestShimMatrixSpecParity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "specs", "024-shim-support-matrix.md"))
	if err != nil {
		t.Fatalf("read spec 024: %v", err)
	}
	spec := string(raw)
	m := ShimSupportMatrix()

	if !strings.Contains(spec, m.DocDate) {
		t.Fatalf("spec 024 does not record the matrix doc date %s", m.DocDate)
	}
	if !strings.Contains(spec, m.RefreshCommand) {
		t.Fatalf("spec 024 does not record the refresh command %q", m.RefreshCommand)
	}
	if !strings.Contains(spec, m.RecordedFixture) {
		t.Fatalf("spec 024 does not name the recorded fixture %s", m.RecordedFixture)
	}
	for _, doc := range m.ProviderDocs {
		if !strings.Contains(spec, doc) {
			t.Fatalf("spec 024 does not cite the provider doc %s", doc)
		}
	}

	// Parse the §D table rows: | ID | §  | item | format | state | row/reason |
	begin := strings.Index(spec, "<!-- SHIM-MATRIX-BEGIN -->")
	end := strings.Index(spec, "<!-- SHIM-MATRIX-END -->")
	if begin < 0 || end < 0 || end < begin {
		t.Fatal("spec 024 is missing the matrix table markers")
	}
	table := spec[begin:end]

	type row struct{ state string }
	inSpec := map[string]row{}
	for _, line := range strings.Split(table, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 8 {
			// A cell whose text contains a bare pipe shifts the columns; fail
			// loudly instead of reading the wrong column as the state.
			t.Fatalf("spec table row has %d columns, want 6 (state must be column 5): %q", len(cells)-2, line)
		}
		id := strings.TrimSpace(cells[1])
		if !strings.HasPrefix(id, "B") || !strings.Contains(id, "-") {
			continue
		}
		inSpec[id] = row{state: strings.TrimSpace(cells[5])}
	}

	for _, c := range m.Cells {
		specRow, ok := inSpec[c.ID]
		if !ok {
			t.Fatalf("matrix cell %s is absent from the spec table", c.ID)
		}
		if specRow.state != string(c.State) {
			t.Fatalf("cell %s state in spec = %q, in the matrix = %q", c.ID, specRow.state, c.State)
		}
	}
	for id := range inSpec {
		found := false
		for _, c := range m.Cells {
			if c.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("spec table row %s has no matrix cell", id)
		}
	}
}

// TestShimFixtureIsRecordedNotFabricated guards the fixture's honesty: it must
// say that it is a live recording, carry the provider's own bytes, and hold no
// credential material. A fixture that quietly drifts into "shaped like what we
// expect" would make every `live` cell meaningless.
func TestShimFixtureIsRecordedNotFabricated(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "shim-support-exchanges.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fx := loadShimFixture(t)

	if fx.Recorded.Provider == "" || fx.Recorded.BaseURL == "" {
		t.Fatal("fixture does not name the provider and base URL it was recorded from")
	}
	for _, want := range []string{"VERBATIM", "POSTed", "provider's own bytes"} {
		if !strings.Contains(fx.Recorded.Provenance, want) {
			t.Fatalf("fixture provenance does not state that it is a live recording (missing %q): %q",
				want, fx.Recorded.Provenance)
		}
	}
	if fx.Recorded.ProviderDocsRetrievedAt != ShimDocDate {
		t.Fatalf("fixture doc date %q does not match the matrix doc date %q",
			fx.Recorded.ProviderDocsRetrievedAt, ShimDocDate)
	}
	for _, stamp := range []string{fx.Recorded.RecordedAtFirst, fx.Recorded.RecordedAtLast} {
		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			t.Fatalf("recorded timestamp %q is not RFC3339: %v", stamp, err)
		}
	}
	if fx.Recorded.KeyMaterialStored {
		t.Fatal("fixture claims to store key material")
	}
	for _, needle := range [][]byte{[]byte("sk-"), []byte("Authorization"), []byte("Bearer ")} {
		if bytes.Contains(raw, needle) {
			t.Fatalf("fixture contains credential-shaped material (%q)", needle)
		}
	}

	total := 0
	for _, table := range []map[string]shimExchange{fx.OpenAI, fx.Anthropic, fx.Responses, fx.Beta} {
		for name, ex := range table {
			total++
			if len(ex.Request) == 0 {
				t.Fatalf("exchange %s has no request body", name)
			}
			if ex.HTTPStatus == 0 {
				t.Fatalf("exchange %s has no recorded status", name)
			}
			if ex.HTTPStatus >= 400 && len(ex.Response) == 0 {
				t.Fatalf("exchange %s records a refusal without the provider's error body", name)
			}
		}
	}
	if total < 20 {
		t.Fatalf("fixture holds only %d exchanges; the matrix's live cells need broader coverage", total)
	}
}

// ============================================================================
// Small helpers
// ============================================================================

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// assertUnsupported proves a not-supported cell still names a row that matches
// the row pattern — the matrix invariant, re-checked at the cell so a cell that
// silently loses its row fails where the change was made.
func assertUnsupported(t *testing.T, id, inReason string) {
	t.Helper()
	for _, c := range ShimSupportMatrix().Cells {
		if c.ID != id {
			continue
		}
		if c.State != CellNotSupported {
			t.Fatalf("cell %s state = %s, want not-supported", id, c.State)
		}
		if !ShimRowPattern.MatchString(c.Row) {
			t.Fatalf("cell %s names no well-formed row (got %q)", id, c.Row)
		}
		if inReason != "" && !strings.Contains(strings.ToLower(c.Reason), strings.ToLower(inReason)) {
			t.Fatalf("cell %s reason does not mention %q: %q", id, inReason, c.Reason)
		}
		return
	}
	t.Fatalf("cell %s is not in the matrix", id)
}

func thinkingLabel(v *bool) string {
	if v == nil {
		return "unset"
	}
	if *v {
		return "on"
	}
	return "off"
}
