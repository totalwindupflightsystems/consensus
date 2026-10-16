// Anthropic streaming + tool-use conformance (spec 024, SHIM-SUPPORT-004).
//
// These tests pin the two capabilities this work item adds to the Anthropic
// Messages client, in the same evidence style as sampling_contract_test.go:
//
//  1. Streaming: the client consumes the provider's SSE protocol
//     (message_start → content_block_start → content_block_delta… →
//     content_block_stop → message_delta → message_stop, plus the structural
//     ping events) and assembles EXACTLY the turn the buffered Call returns —
//     same content, same thinking block (+ signature), same tool_use blocks,
//     same usage accounting. The assembly is delegated to the same
//     buildAnthropicResponse the buffered path uses, and the equivalence test
//     below fails if the two paths ever drift.
//  2. Tool-use: the request carries the Anthropic tool shape
//     (name/description/input_schema) with the neutral tool_choice mapped onto
//     the provider vocabulary (required → any), the response's tool_use block
//     becomes a harness.ToolCall, a role:"tool" result goes back as a
//     tool_result block, and the whole round trip also works over the stream.
//     The declaration shape is pinned against the recorded live exchange in
//     testdata/shim-support-exchanges.json — the same bytes the provider
//     accepted with HTTP 200 — and against the Responses surface, so one
//     ToolDefinition means the same thing on both (responses_format.go).
//
// Malformed-stream cases fail loudly and name the defect: a stream is only
// allowed to produce a turn when message_start opened it and message_stop
// closed it, every frame carries data, and no event type outside the protocol
// is silently swallowed — a silently dropped frame would produce a turn that
// looks complete and is not.
//
// Evidence boundary: streaming transcripts here are protocol fixtures built to
// the documented event shapes, NOT provider recordings — the matrix marks the
// streaming cells `unit` evidence for exactly that reason (no live exchange in
// the fixture file carries SSE frames). The tool-use request/response shape
// cells keep `live` evidence via the recorded exchange.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// SSE test server + transcript helpers
// ============================================================================

// streamServer replays one canned SSE transcript per request, in order, and
// captures every request body the client produced. It is the streaming-side
// counterpart of newReplayServer (which serves buffered JSON): the bytes are
// per-test literals so each protocol edge is named where it is asserted.
type streamServer struct {
	srv      *httptest.Server
	captured *[][]byte
}

// newStreamServer serves the given transcripts in order. status applies to
// every response (0 = 200 with an SSE content type); an error body for a
// failure-status test is passed as a normal transcript.
func newStreamServer(t *testing.T, status int, transcripts ...string) *streamServer {
	t.Helper()
	captured := &[][]byte{}
	next := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := readAllLimited(r)
		if err != nil {
			t.Errorf("stream server: read request body: %v", err)
		}
		*captured = append(*captured, body)
		if next >= len(transcripts) {
			t.Errorf("stream server: unexpected extra request (%d transcripts)", len(transcripts))
			http.Error(w, `{"type":"error","error":{"type":"overloaded_error","message":"no transcript left"}}`, http.StatusInternalServerError)
			return
		}
		tr := transcripts[next]
		next++
		code := status
		if code == 0 {
			code = http.StatusOK
			w.Header().Set("Content-Type", "text/event-stream")
		}
		w.WriteHeader(code)
		if _, err := w.Write([]byte(tr)); err != nil {
			t.Errorf("stream server: write transcript: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return &streamServer{srv: srv, captured: captured}
}

// readAllLimited reads a request body with a small cap, so a runaway client
// cannot hang the fake.
func readAllLimited(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}

// client builds the real Anthropic client against this server.
func (s *streamServer) client(t *testing.T) *anthropicClient {
	t.Helper()
	return anthropicTestClient(t, s.srv.URL, nil)
}

// body decodes a captured request body by index. Index 0 is additionally
// asserted to be a streaming request (stream:true on the wire).
func (s *streamServer) body(t *testing.T, i int) map[string]any {
	t.Helper()
	if i >= len(*s.captured) {
		t.Fatalf("no captured request %d (have %d)", i, len(*s.captured))
	}
	body := bodyJSON(t, (*s.captured)[i])
	if i == 0 && body["stream"] != true {
		t.Fatalf("stream request carried stream=%v, want true: %s", body["stream"], mustJSON(t, body))
	}
	return body
}

// sse renders event/data pairs into an SSE transcript.
func sse(events ...[2]string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("event: " + e[0] + "\ndata: " + e[1] + "\n\n")
	}
	return b.String()
}

// deltaFrame renders one content_block_delta event's data with honest JSON
// escaping (json.Marshal, never string concatenation of user text).
func deltaFrame(index int, delta map[string]any) string {
	b, err := json.Marshal(map[string]any{"type": "content_block_delta", "index": index, "delta": delta})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Sample stream fragments. The shapes follow the documented protocol; the
// tool_use ids and signature values are fixture values, not provider state.
const streamMsgStartText = `{"type":"message_start","message":{"id":"msg_stream_text","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"usage":{"input_tokens":25,"output_tokens":1}}}`

const streamText1 = "Today's date is October 16, 2026"
const streamText2 = ", a Friday."

const streamBlockStartText = `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
const streamBlockStop0 = `{"type":"content_block_stop","index":0}`
const streamMessageDeltaEnd = `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42}}`
const streamMessageStop = `{"type":"message_stop"}`

const streamThink1 = "The user asks for today's date. "
const streamThink2 = "I should call get_date."
const streamSignature = "sig-stream-01"

const streamToolCallID = "call_00_stream01"
const streamToolInput1 = `{"day":1,`
const streamToolInput2 = `"month":10}`

// streamToolTranscript is a full tool_use turn: thinking + signature deltas,
// then a tool_use block assembled from two partial_json deltas.
func streamToolTranscript() string {
	return sse(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_stream_tool","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"usage":{"input_tokens":25,"output_tokens":1}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "thinking_delta", "thinking": streamThink1})},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "thinking_delta", "thinking": streamThink2})},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "signature_delta", "signature": streamSignature})},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"` + streamToolCallID + `","name":"get_date","input":{}}}`},
		[2]string{"content_block_delta", deltaFrame(1, map[string]any{"type": "input_json_delta", "partial_json": streamToolInput1})},
		[2]string{"content_block_delta", deltaFrame(1, map[string]any{"type": "input_json_delta", "partial_json": streamToolInput2})},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":99}}`},
		[2]string{"message_stop", streamMessageStop},
	)
}

// streamToolDefinition is the tool the transcripts' get_date calls refer to.
func streamToolDefinition() []ToolDefinition {
	return []ToolDefinition{{
		Type: "function",
		Function: ToolFunctionDef{
			Name:        "get_date",
			Description: "Get the current date",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}}
}

// ============================================================================
// Streaming conformance
// ============================================================================

// TestAnthropicConformance_StreamEventSequence pins the documented happy-path
// event sequence for a text turn: the deltas concatenate in frame order, the
// final usage arrives via message_delta, the request carried stream:true, and
// structural events (ping, content_block_stop) are accepted without effect.
func TestAnthropicConformance_StreamEventSequence(t *testing.T) {
	transcript := sse(
		[2]string{"ping", `{"type":"ping"}`},
		[2]string{"message_start", streamMsgStartText},
		[2]string{"content_block_start", streamBlockStartText},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "text_delta", "text": streamText1})},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "text_delta", "text": streamText2})},
		[2]string{"content_block_stop", streamBlockStop0},
		[2]string{"message_delta", streamMessageDeltaEnd},
		[2]string{"message_stop", streamMessageStop},
	)
	srv := newStreamServer(t, 0, transcript)
	client := srv.client(t)

	// The answer is prose, not an AgentOutput document: the turn is still real
	// (the same tolerance callAnthropic applies to recorded answers), so only
	// the AgentOutput-parse error is tolerated, never a transport/stream error.
	turn, err := client.CallWithOptions(context.Background(),
		[]harness.Message{{Role: "user", Content: "what is the date?"}},
		RequestOptions{Stream: true})
	if err != nil {
		if !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("streamed call failed: %v", err)
		}
	}
	if turn == nil {
		t.Fatal("no turn from the streamed text turn")
	}
	if got, want := turn.Content, streamText1+streamText2; got != want {
		t.Fatalf("streamed content = %q, want the concatenation of the text deltas %q", got, want)
	}
	if turn.ReasoningContent != "" {
		t.Fatalf("text-only stream produced reasoning content %q", turn.ReasoningContent)
	}
	if turn.ModelID != "deepseek-flash" {
		t.Fatalf("model id = %q, want the message_start value", turn.ModelID)
	}
	if turn.Usage.PromptTokens != 25 {
		t.Fatalf("prompt tokens = %d, want the message_start value 25", turn.Usage.PromptTokens)
	}
	if turn.Usage.CompletionTokens != 42 {
		t.Fatalf("completion tokens = %d, want the message_delta value 42", turn.Usage.CompletionTokens)
	}

	// The request must have gone out as a streaming request.
	body := srv.body(t, 0)
	if body["stream"] != true {
		t.Fatalf("request carried stream=%v, want true: %s", body["stream"], mustJSON(t, body))
	}
}

// TestAnthropicConformance_StreamThinkingAndToolUseAssemblesToolCall pins the
// streamed tool-use contract end to end: thinking and signature deltas rebuild
// the block verbatim, two partial_json deltas concatenate into the exact
// input JSON, and the tool_use block surfaces as a harness.ToolCall.
func TestAnthropicConformance_StreamThinkingAndToolUseAssemblesToolCall(t *testing.T) {
	srv := newStreamServer(t, 0, streamToolTranscript())
	client := srv.client(t)

	turn, err := client.CallWithOptions(context.Background(),
		[]harness.Message{{Role: "user", Content: "what is the date?"}},
		RequestOptions{Stream: true, Tools: streamToolDefinition()})
	if err != nil {
		t.Fatalf("streamed tool-use call failed: %v", err)
	}
	if got, want := turn.ReasoningContent, streamThink1+streamThink2; got != want {
		t.Fatalf("thinking = %q, want the concatenation of the thinking deltas %q", got, want)
	}
	if turn.ReasoningSignature != streamSignature {
		t.Fatalf("signature = %q, want %q (the signature delta must rebuild the block field)", turn.ReasoningSignature, streamSignature)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("streamed turn produced %d tool calls, want 1: %s", len(turn.ToolCalls), mustJSON(t, turn))
	}
	call := turn.ToolCalls[0]
	if call.ID != streamToolCallID || call.Function.Name != "get_date" {
		t.Fatalf("tool call = %s/%s, want %s/get_date", call.ID, call.Function.Name, streamToolCallID)
	}
	// The arguments are the byte-exact concatenation of the partial_json
	// deltas — the provider's own contract for input_json_delta.
	if got, want := call.Function.Arguments, `{"day":1,"month":10}`; got != want {
		t.Fatalf("tool arguments = %q, want the concatenated partial_json %q", got, want)
	}
	if turn.Usage.CompletionTokens != 99 {
		t.Fatalf("completion tokens = %d, want the message_delta value 99", turn.Usage.CompletionTokens)
	}
}

// TestAnthropicConformance_StreamedAndBufferedTurnsAgree is the drift guard:
// the same logical turn (thinking + signature + AgentOutput text) served once
// as SSE frames and once as the buffered JSON message must produce identical
// harness turns — content, reasoning, signature, tool calls, usage and the
// parsed AgentOutput. The streamed path delegates its final assembly to the
// buffered builder; this test fails if anyone reintroduces a parallel one.
func TestAnthropicConformance_StreamedAndBufferedTurnsAgree(t *testing.T) {
	const think = "Two sentences about the sky. "
	const sig = "sig-equiv-01"
	// Valid AgentOutput JSON: both paths must parse it, not just carry it.
	const answer = `{"internal_monologue":"m","memory_state_changes":[],"system_actions":["respond"],"message_to_user":"hello","tool_requests":[]}`

	streamTr := sse(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_equiv","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"usage":{"input_tokens":25,"output_tokens":1}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "thinking_delta", "thinking": think})},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "signature_delta", "signature": sig})},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", deltaFrame(1, map[string]any{"type": "text_delta", "text": answer})},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		[2]string{"message_delta", streamMessageDeltaEnd},
		[2]string{"message_stop", streamMessageStop},
	)
	buffered := fmt.Sprintf(`{"id":"msg_equiv","type":"message","role":"assistant","model":"deepseek-flash","content":[{"type":"thinking","thinking":%q,"signature":%q},{"type":"text","text":%s}],"stop_reason":"end_turn","usage":{"input_tokens":25,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":42}}`,
		think, sig, strconvQuote(answer))

	msgs := []harness.Message{{Role: "user", Content: "why is the sky blue?"}}
	srvS := newStreamServer(t, 0, streamTr)
	streamTurn, streamErr := srvS.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})

	srvB, _ := newReplayServer(t, json.RawMessage(buffered))
	bufClient := anthropicTestClient(t, srvB.URL, nil)
	bufTurn, bufErr := bufClient.CallWithOptions(context.Background(), msgs, RequestOptions{})

	if streamErr != nil || bufErr != nil {
		t.Fatalf("both paths must parse the AgentOutput answer: stream err=%v, buffered err=%v", streamErr, bufErr)
	}
	if streamTurn == nil || bufTurn == nil {
		t.Fatal("a path returned a nil turn without an error")
	}
	if streamTurn.Content != bufTurn.Content {
		t.Fatalf("content disagrees: streamed %q vs buffered %q", streamTurn.Content, bufTurn.Content)
	}
	if streamTurn.ReasoningContent != bufTurn.ReasoningContent {
		t.Fatalf("reasoning disagrees: streamed %q vs buffered %q", streamTurn.ReasoningContent, bufTurn.ReasoningContent)
	}
	if streamTurn.ReasoningSignature != bufTurn.ReasoningSignature {
		t.Fatalf("signature disagrees: streamed %q vs buffered %q", streamTurn.ReasoningSignature, bufTurn.ReasoningSignature)
	}
	if streamTurn.Usage != bufTurn.Usage {
		t.Fatalf("usage disagrees: streamed %+v vs buffered %+v", streamTurn.Usage, bufTurn.Usage)
	}
	if streamTurn.ModelID != bufTurn.ModelID {
		t.Fatalf("model id disagrees: %q vs %q", streamTurn.ModelID, bufTurn.ModelID)
	}
	if streamTurn.Output == nil || bufTurn.Output == nil {
		t.Fatalf("both paths must parse AgentOutput: streamed=%v buffered=%v", streamTurn.Output, bufTurn.Output)
	}
	if !reflect.DeepEqual(*streamTurn.Output, *bufTurn.Output) {
		t.Fatalf("AgentOutput disagrees: streamed %+v vs buffered %+v", *streamTurn.Output, *bufTurn.Output)
	}
}

// strconvQuote renders s as a JSON string literal (small local helper so the
// equivalence fixture above stays readable).
func strconvQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestAnthropicConformance_MalformedStreamFailsLoudly pins the refusal cases:
// a frame without data, a stream without message_start, a stream cut off
// before message_stop, and an unknown event type all fail with an error that
// names the defect instead of producing a silently incomplete turn.
func TestAnthropicConformance_MalformedStreamFailsLoudly(t *testing.T) {
	msgs := []harness.Message{{Role: "user", Content: "what is the date?"}}

	t.Run("frame_without_data", func(t *testing.T) {
		srv := newStreamServer(t, 0, sse([2]string{"message_start", ""}))
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "carries no data") {
			t.Fatalf("err = %v, want a frame-carries-no-data refusal", err)
		}
	})
	t.Run("missing_message_start", func(t *testing.T) {
		tr := sse(
			[2]string{"content_block_start", streamBlockStartText},
			[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "text_delta", "text": "hi"})},
			[2]string{"message_stop", streamMessageStop},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "message_start") {
			t.Fatalf("err = %v, want a missing-message_start refusal", err)
		}
	})
	t.Run("truncated_before_message_stop", func(t *testing.T) {
		tr := sse(
			[2]string{"message_start", streamMsgStartText},
			[2]string{"content_block_start", streamBlockStartText},
			[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "text_delta", "text": "hi"})},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "message_stop") {
			t.Fatalf("err = %v, want a stream-ended-before-message_stop refusal", err)
		}
	})
	t.Run("unknown_event_type", func(t *testing.T) {
		tr := sse(
			[2]string{"message_start", streamMsgStartText},
			[2]string{"brand_new_event", `{"type":"brand_new_event"}`},
			[2]string{"message_stop", streamMessageStop},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "unknown event type") {
			t.Fatalf("err = %v, want an unknown-event refusal", err)
		}
	})
	t.Run("delta_before_block_start", func(t *testing.T) {
		tr := sse(
			[2]string{"message_start", streamMsgStartText},
			[2]string{"content_block_delta", deltaFrame(3, map[string]any{"type": "text_delta", "text": "hi"})},
			[2]string{"message_stop", streamMessageStop},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "before its content_block_start") {
			t.Fatalf("err = %v, want a delta-before-start refusal", err)
		}
	})
	t.Run("unknown_delta_type", func(t *testing.T) {
		tr := sse(
			[2]string{"message_start", streamMsgStartText},
			[2]string{"content_block_start", streamBlockStartText},
			[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "citrus_delta", "text": "hi"})},
			[2]string{"message_stop", streamMessageStop},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "unsupported delta type") {
			t.Fatalf("err = %v, want an unsupported-delta refusal", err)
		}
	})
	t.Run("midstream_error_event", func(t *testing.T) {
		tr := sse(
			[2]string{"message_start", streamMsgStartText},
			[2]string{"error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`},
		)
		srv := newStreamServer(t, 0, tr)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "overloaded_error") || !strings.Contains(err.Error(), "Overloaded") {
			t.Fatalf("err = %v, want the stream error event surfaced with its detail", err)
		}
	})
	t.Run("http_error_before_stream", func(t *testing.T) {
		srv := newStreamServer(t, http.StatusUnauthorized,
			`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
		_, err := srv.client(t).CallWithOptions(context.Background(), msgs, RequestOptions{Stream: true})
		if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid x-api-key") {
			t.Fatalf("err = %v, want the HTTP 401 surfaced with its detail", err)
		}
	})
}

// TestAnthropicConformance_StreamConfigDefaultReachesTheWire pins the
// configuration surface: Config.Stream=true serves every request over SSE
// without a per-request option, a per-request RequestOptions{Stream:true}
// overrides an unconfigured client, and a buffered request puts NO stream
// field on the wire at all (the documented buffered default is an absent
// field, never an explicit stream:false).
func TestAnthropicConformance_StreamConfigDefaultReachesTheWire(t *testing.T) {
	srv := newStreamServer(t, 0, streamToolTranscript())
	c := anthropicTestClient(t, srv.srv.URL, &Config{Stream: true})
	if _, err := c.CallWithOptions(context.Background(),
		[]harness.Message{{Role: "user", Content: "date?"}}, RequestOptions{}); err != nil {
		t.Fatalf("configured-stream call failed: %v", err)
	}
	if body := srv.body(t, 0); body["stream"] != true {
		t.Fatalf("Config.Stream did not reach the wire: %s", mustJSON(t, body))
	}

	// A buffered request must not carry the field at all (omitempty).
	fx := loadShimFixture(t)
	ex := fx.must(t, "anthropic", "thinking_on_with_text")
	body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request), RequestOptions{}, ex)
	if _, present := body["stream"]; present {
		t.Fatalf("buffered request carried a stream field: %s", mustJSON(t, body))
	}
}

// ============================================================================
// Tool-use conformance
// ============================================================================

// TestAnthropicConformance_ToolUseRequestShape pins the request half of the
// contract against the recorded live exchange: the declared ToolDefinition
// reaches the wire as {name, description, input_schema} — the exact shape the
// provider accepted with HTTP 200 — and tool_choice maps required → any.
func TestAnthropicConformance_ToolUseRequestShape(t *testing.T) {
	fx := loadShimFixture(t)
	ex := fx.must(t, "anthropic", "tool_choice_any_thinking_on")

	tools := []ToolDefinition{{
		Type: "function",
		Function: ToolFunctionDef{
			Name:        "get_date",
			Description: "Get the current date",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}}
	body, _ := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request),
		RequestOptions{Tools: tools, ToolChoice: "required"}, ex)

	wireTools, ok := body["tools"].([]any)
	if !ok || len(wireTools) != 1 {
		t.Fatalf("wire tools = %v, want the one declared tool: %s", body["tools"], mustJSON(t, body))
	}
	tool := wireTools[0].(map[string]any)
	if tool["name"] != "get_date" {
		t.Fatalf("tool name = %v, want get_date", tool["name"])
	}
	if tool["description"] != "Get the current date" {
		t.Fatalf("tool description = %v, want the declared description", tool["description"])
	}
	schema, ok := tool["input_schema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Fatalf("input_schema = %v, want the declared parameters object", tool["input_schema"])
	}
	tc, ok := body["tool_choice"].(map[string]any)
	if !ok || tc["type"] != "any" {
		t.Fatalf("tool_choice = %v, want the Anthropic spelling {type: any} for required: %s",
			body["tool_choice"], mustJSON(t, body))
	}
}

// TestAnthropicConformance_ToolUseResponseAndResultRoundTrip pins the response
// half against the recorded exchange: a tool_use block becomes a
// harness.ToolCall with the recorded id/name/arguments, the turn's thinking
// (+ signature) rides along, and the result goes back as a tool_result block
// inside a user turn while the assistant turn is echoed with its tool_use
// block — the shape the recorded continuation was accepted with.
func TestAnthropicConformance_ToolUseResponseAndResultRoundTrip(t *testing.T) {
	fx := loadShimFixture(t)
	ex := fx.must(t, "anthropic", "tool_choice_any_thinking_on")

	_, turn := callAnthropic(t, "/anthropic", nil, requestMessages(t, ex.Request),
		RequestOptions{Tools: streamToolDefinition(), ToolChoice: "any"}, ex)
	if turn == nil {
		t.Fatal("no turn from the recorded tool-use exchange")
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("turn read %d tool calls, want 1", len(turn.ToolCalls))
	}

	// The recorded response's own values are the expectation.
	resp := decodeMap(t, ex.Response, "recorded response")
	var wantID, wantName, wantArgs string
	var wantThinking, wantSig string
	for _, b := range resp["content"].([]any) {
		m := b.(map[string]any)
		switch m["type"] {
		case "tool_use":
			wantID, _ = m["id"].(string)
			wantName, _ = m["name"].(string)
			if raw, err := json.Marshal(m["input"]); err == nil {
				wantArgs = string(raw)
			}
		case "thinking":
			wantThinking, _ = m["thinking"].(string)
			wantSig, _ = m["signature"].(string)
		}
	}
	call := turn.ToolCalls[0]
	if call.ID != wantID || call.Function.Name != wantName {
		t.Fatalf("tool call = %s/%s, want the recorded %s/%s", call.ID, call.Function.Name, wantID, wantName)
	}
	if call.Function.Arguments != wantArgs {
		t.Fatalf("arguments = %q, want the recorded input %q", call.Function.Arguments, wantArgs)
	}
	if turn.ReasoningContent != wantThinking || turn.ReasoningSignature != wantSig {
		t.Fatalf("thinking round-trip = %q/%q, want the recorded %q/%q",
			turn.ReasoningContent, turn.ReasoningSignature, wantThinking, wantSig)
	}

	// The result goes back as tool_result; the assistant turn is echoed with
	// its tool_use block (the provider requires the thinking echo with it).
	ex2 := fx.must(t, "anthropic", "tools_thinking_echo")
	cont := []harness.Message{
		{Role: "user", Content: "what is the date?"},
		{Role: "assistant", ReasoningContent: turn.ReasoningContent, ReasoningSignature: turn.ReasoningSignature, ToolCalls: turn.ToolCalls},
		{Role: "tool", ToolCallID: wantID, Content: "2026-10-16"},
	}
	body2, _ := callAnthropic(t, "/anthropic", nil, cont, RequestOptions{Tools: streamToolDefinition()}, ex2)
	sent := body2["messages"].([]any)
	last := sent[len(sent)-1].(map[string]any)
	if last["role"] != "user" {
		t.Fatalf("tool result turn role = %v, want user", last["role"])
	}
	lastBlocks := last["content"].([]any)
	res := lastBlocks[0].(map[string]any)
	if res["type"] != "tool_result" || res["tool_use_id"] != wantID {
		t.Fatalf("result block = %v, want tool_result with tool_use_id %s", res, wantID)
	}
	asst := sent[len(sent)-2].(map[string]any)
	echoBlocks := asst["content"].([]any)
	var sawToolUse bool
	for _, b := range echoBlocks {
		m := b.(map[string]any)
		if m["type"] == "tool_use" {
			sawToolUse = true
			if m["id"] != wantID {
				t.Fatalf("echoed tool_use id = %v, want %s", m["id"], wantID)
			}
		}
	}
	if !sawToolUse {
		t.Fatalf("assistant echo carries no tool_use block: %s", mustJSON(t, body2))
	}
}

// TestAnthropicConformance_ToolUseRoundTripOverStream drives the complete loop
// — tool-use turn, tool result, final answer — over the streaming surface, and
// inspects both captured request bodies: the first carries tools + stream:true,
// the second carries the tool_result block plus the echoed assistant turn with
// its thinking and tool_use input.
func TestAnthropicConformance_ToolUseRoundTripOverStream(t *testing.T) {
	answerTr := sse(
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_stream_answer","type":"message","role":"assistant","model":"deepseek-flash","content":[],"stop_reason":null,"usage":{"input_tokens":63,"output_tokens":1}}}`},
		[2]string{"content_block_start", streamBlockStartText},
		[2]string{"content_block_delta", deltaFrame(0, map[string]any{"type": "text_delta", "text": streamText1})},
		[2]string{"content_block_stop", streamBlockStop0},
		[2]string{"message_delta", streamMessageDeltaEnd},
		[2]string{"message_stop", streamMessageStop},
	)
	srv := newStreamServer(t, 0, streamToolTranscript(), answerTr)
	client := srv.client(t)
	tools := streamToolDefinition()

	turn1, err := client.CallWithOptions(context.Background(),
		[]harness.Message{{Role: "user", Content: "what is the date?"}},
		RequestOptions{Stream: true, Tools: tools})
	if err != nil {
		t.Fatalf("streamed tool-use turn failed: %v", err)
	}

	// First request: streaming request with tools declared.
	body1 := srv.body(t, 0)
	if body1["stream"] != true {
		t.Fatalf("first request stream = %v, want true", body1["stream"])
	}
	if _, ok := body1["tools"].([]any); !ok {
		t.Fatalf("first request carried no tools: %s", mustJSON(t, body1))
	}

	// The result goes back over the same streaming surface.
	turn2, err := client.CallWithOptions(context.Background(), []harness.Message{
		{Role: "user", Content: "what is the date?"},
		{Role: "assistant", ReasoningContent: turn1.ReasoningContent, ReasoningSignature: turn1.ReasoningSignature, ToolCalls: turn1.ToolCalls},
		{Role: "tool", ToolCallID: turn1.ToolCalls[0].ID, Content: "2026-10-16"},
	}, RequestOptions{Stream: true, Tools: tools})
	if err != nil {
		// Prose answer, not an AgentOutput document — the turn is real.
		if !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("streamed continuation failed: %v", err)
		}
	}
	if turn2 == nil {
		t.Fatal("no turn from the streamed continuation")
	}
	if got, want := turn2.Content, streamText1; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	if turn2.Usage.PromptTokens != 63 {
		t.Fatalf("continuation prompt tokens = %d, want its message_start value 63", turn2.Usage.PromptTokens)
	}

	// Second request: the tool result block + the echoed assistant turn.
	body2 := srv.body(t, 1)
	sent := body2["messages"].([]any)
	last := sent[len(sent)-1].(map[string]any)
	res := last["content"].([]any)[0].(map[string]any)
	if res["type"] != "tool_result" || res["tool_use_id"] != streamToolCallID {
		t.Fatalf("streamed result block = %v, want tool_result for %s", res, streamToolCallID)
	}
	asst := sent[len(sent)-2].(map[string]any)
	var sawThinking, sawToolUse bool
	for _, b := range asst["content"].([]any) {
		m := b.(map[string]any)
		switch m["type"] {
		case "thinking":
			sawThinking = m["thinking"] == streamThink1+streamThink2 && m["signature"] == streamSignature
		case "tool_use":
			sawToolUse = m["id"] == streamToolCallID
			if input, err := json.Marshal(m["input"]); err != nil || string(input) != `{"day":1,"month":10}` {
				t.Fatalf("echoed tool_use input = %s (%v), want the streamed arguments byte-exact", input, err)
			}
		}
	}
	if !sawThinking || !sawToolUse {
		t.Fatalf("echoed assistant turn incomplete (thinking=%v tool_use=%v): %s", sawThinking, sawToolUse, mustJSON(t, body2))
	}
}

// TestAnthropicConformance_ToolDefinitionCompatibilityWithResponses pins that
// one ToolDefinition is accepted by BOTH wire surfaces: the Anthropic path
// validates it in CallWithOptions exactly as ValidateToolDefinitions does, and
// BuildResponsesRequest accepts the same list and maps the same fields — so a
// caller's tool declarations mean the same thing on both (responses_format.go
// compatibility, SHIM-SUPPORT-004).
func TestAnthropicConformance_ToolDefinitionCompatibilityWithResponses(t *testing.T) {
	valid := json.RawMessage(`{"type":"object","properties":{"day":{"type":"number"}}}`)
	cases := []struct {
		name    string
		tool    ToolDefinition
		wantErr bool
	}{
		{
			name: "valid_schema_accepted_on_both",
			tool: ToolDefinition{Type: "function", Function: ToolFunctionDef{
				Name: "get_date", Description: "Get the current date", Parameters: valid}},
		},
		{
			name: "malformed_schema_still_declared_not_refused",
			// A schema-mismatched input_schema is a declaration the provider
			// sees, not a client-side violation: the name charset is what the
			// limits cell enforces, on both surfaces alike.
			tool: ToolDefinition{Type: "function", Function: ToolFunctionDef{
				Name: "odd_schema", Parameters: json.RawMessage(`{"type":"not-a-schema-kind"}`)}},
		},
		{
			name: "bad_name_charset_refused_on_both",
			tool: ToolDefinition{Type: "function", Function: ToolFunctionDef{
				Name: "bad name"}},
			wantErr: true,
		},
		{
			name: "empty_name_refused_on_both",
			tool: ToolDefinition{Type: "function", Function: ToolFunctionDef{
				Name: "", Parameters: valid}},
			wantErr: true,
		},
		{
			name: "oversize_name_refused_on_both",
			tool: ToolDefinition{Type: "function", Function: ToolFunctionDef{
				Name: strings.Repeat("a", maxToolNameLen+1)}},
			wantErr: true,
		},
	}

	msgs := []harness.Message{{Role: "user", Content: "hi"}}
	fx := loadShimFixture(t)
	off := false

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := []ToolDefinition{tc.tool}

			// The surfaces must AGREE on what is a valid tool declaration:
			// the shared validator (both clients call it) and the Responses
			// builder answer identically, so one declaration means one thing
			// on both wires. (The live-armed HTTP assertion of the shape is
			// TestAnthropicConformance_ToolUseRequestShape; this test is the
			// two-surface compatibility contract at the builder level.)
			errA := ValidateToolDefinitions(list)
			_, errB := BuildResponsesRequest("deepseek-flash", msgs, RequestOptions{Tools: list})
			if (errA != nil) != (errB != nil) {
				t.Fatalf("surfaces disagree: ValidateToolDefinitions err=%v, BuildResponsesRequest err=%v", errA, errB)
			}

			// The Anthropic client must refuse exactly what the validator
			// refuses, before any request goes out. A prose recorded answer
			// still yields a turn plus an AgentOutput-parse error — that is an
			// ACCEPTED request, not a refusal, so it counts as nil here (the
			// same tolerance callAnthropic applies).
			srv, captured := replay(t, fx.must(t, "anthropic", "thinking_on_with_text"))
			client := anthropicTestClient(t, srv.URL+"/anthropic", &Config{Thinking: &off})
			_, rawErr := client.CallWithOptions(context.Background(), msgs, RequestOptions{Tools: list})
			errC := rawErr
			if errC != nil && strings.Contains(errC.Error(), "AgentOutput JSON") {
				errC = nil
			}
			if (errC != nil) != (errA != nil) {
				t.Fatalf("anthropic call disagreed with the validator: call err=%v, validator err=%v", rawErr, errA)
			}
			if errA != nil {
				if len(*captured) != 0 {
					t.Fatalf("a request was sent despite the local refusal: %s", string((*captured)[0]))
				}
				return
			}

			// The accepted tool maps field-for-field on the Responses surface.
			reqB, errB := BuildResponsesRequest("deepseek-flash", msgs, RequestOptions{Tools: list})
			if errB != nil {
				t.Fatalf("BuildResponsesRequest failed after the validator accepted: %v", errB)
			}
			if len(reqB.Tools) != 1 || reqB.Tools[0].Name != tc.tool.Function.Name ||
				reqB.Tools[0].Description != tc.tool.Function.Description ||
				string(reqB.Tools[0].Parameters) != string(tc.tool.Function.Parameters) {
				t.Fatalf("Responses tool mapping = %+v, want the declared fields %+v", reqB.Tools[0], tc.tool.Function)
			}
		})
	}
}

// TestAnthropicConformance_MatrixRecordsStreamingCells pins the matrix state:
// SHIM-SUPPORT-004's cells exist, claim `supported`, name the streaming tests
// this file runs, and mark their evidence honestly — `unit`, because no
// recorded live exchange carries SSE frames (the tool-shape cells keep their
// live evidence via the recorded exchange).
func TestAnthropicConformance_MatrixRecordsStreamingCells(t *testing.T) {
	m := ShimSupportMatrix()
	byID := map[string]ShimCell{}
	for _, c := range m.Cells {
		byID[c.ID] = c
	}
	streamCells := []string{"B1-ANTHROPIC-STREAM", "B3-ANTHROPIC-STREAM-TOOLUSE"}
	for _, id := range streamCells {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("matrix cell %s (SHIM-SUPPORT-004) is missing", id)
		}
		if c.State != CellSupported {
			t.Fatalf("cell %s state = %q, want supported", id, c.State)
		}
		if c.Format != "anthropic" {
			t.Fatalf("cell %s format = %q, want anthropic", id, c.Format)
		}
		unit := false
		for _, e := range c.Evidence {
			if e == EvidenceUnit {
				unit = true
			}
		}
		if !unit {
			t.Fatalf("cell %s evidence %v does not include unit (streaming is not live-recorded)", id, c.Evidence)
		}
	}
}
