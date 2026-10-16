// Sampling-parameter contract under thinking mode (spec 024 §B2, SHIM-SUPPORT-003).
//
// §B2 used to describe the provider's sampling rules; these tests are what make
// them behaviour instead of documentation. The four claims they pin:
//
//  1. thinking mode ON: temperature, presence_penalty and frequency_penalty are
//     accepted by the provider with NO effect, so the client does not put them
//     on the wire at all — and names each one it dropped at WARN, so a caller
//     who set one learns it did nothing instead of believing it did.
//  2. thinking mode OFF: the same parameters ARE honoured and pass through.
//  3. top_p is resolved, never assumed: floored at 0.95 in thinking mode, fixed
//     at 1.0 (the provider's own value) otherwise, and not sent when nobody
//     asked for it.
//  4. the rule is per THINKING MODE, not per format: all three wire surfaces
//     (OpenAI Chat Completions, Anthropic Messages, Responses) resolve it the
//     same way.
//
// The request bodies are captured from the real clients through the recorded
// fixture's own response bytes, so nothing here is a hand-built expectation of
// what the client might send.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// captureLogs redirects the default logger into a buffer for one test and
// restores it afterwards, so a WARN path can be asserted as a fact rather than
// assumed.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// inertDropParams is the documented inert set, in the order the client reports
// it: the three parameters a thinking-mode request must not carry.
var inertDropParams = []string{"temperature", "presence_penalty", "frequency_penalty"}

// TestSamplingContract_ThinkingModeRequestShape is the §B2 cell's request-shape
// assertion: a thinking-mode request that asks for temperature 0.7,
// presence_penalty 0.5, frequency_penalty 0.3 and top_p 0.5 must reach the wire
// with NONE of the inert parameters and with top_p floored to 0.95.
func TestSamplingContract_ThinkingModeRequestShape(t *testing.T) {
	logs := captureLogs(t)
	fx := loadShimFixture(t)
	// The recorded exchange carries exactly these parameters with thinking
	// DISABLED and was accepted (HTTP 200) — the provider's own bytes, so the
	// contrast between the two modes is measured, not asserted from the docs.
	ex := fx.must(t, "openai", "thinking_disabled_inert_params")
	if ex.HTTPStatus != 200 {
		t.Fatalf("fixture precondition: the recorded request must have been accepted, got %d", ex.HTTPStatus)
	}

	on := true
	half := 0.5
	client := openaiTestClient(t, "", &Config{Temperature: 0.7, Thinking: &on})
	body := callOpenAI(t, client, requestMessages(t, ex.Request), RequestOptions{
		PresencePenalty:  0.5,
		FrequencyPenalty: 0.3,
		TopP:             &half,
	}, ex)

	for _, p := range inertDropParams {
		if v, present := body[p]; present {
			t.Errorf("%s = %v went on the wire although it is inert in thinking mode: %s", p, v, mustJSON(t, body))
		}
	}
	if got := body["top_p"]; got != 0.95 {
		t.Errorf("top_p on the wire = %v, want the thinking-mode floor 0.95: %s", got, mustJSON(t, body))
	}
	if got := body["thinking"]; got == nil {
		t.Fatalf("thinking toggle missing from a thinking-mode request: %s", mustJSON(t, body))
	} else if m, ok := got.(map[string]any); !ok || m["type"] != "enabled" {
		t.Fatalf("thinking toggle = %v, want {\"type\":\"enabled\"}", got)
	}

	// ...and the drop is named, not silent: the caller who set those parameters
	// can see they were not sent.
	got := logs.String()
	if !strings.Contains(got, "level=WARN") {
		t.Fatalf("no WARN was emitted for the dropped parameters:\n%s", got)
	}
	for _, p := range inertDropParams {
		if !strings.Contains(got, p) {
			t.Errorf("the WARN log does not name %s:\n%s", p, got)
		}
	}
}

// TestSamplingContract_NonThinkingModeRequestShape is the complementary arm:
// with thinking disabled the provider honours all three, so they must reach the
// wire unchanged — and top_p must carry the fixed 1.0, never the caller's value
// that the provider ignores.
func TestSamplingContract_NonThinkingModeRequestShape(t *testing.T) {
	logs := captureLogs(t)
	fx := loadShimFixture(t)
	ex := fx.must(t, "openai", "thinking_disabled_inert_params")

	off := false
	half := 0.5
	client := openaiTestClient(t, "", &Config{Temperature: 0.7, Thinking: &off})
	body := callOpenAI(t, client, requestMessages(t, ex.Request), RequestOptions{
		PresencePenalty:  0.5,
		FrequencyPenalty: 0.3,
		TopP:             &half,
	}, ex)

	if got := body["temperature"]; got != 0.7 {
		t.Errorf("temperature = %v, want the requested 0.7 passed through: %s", got, mustJSON(t, body))
	}
	if got := body["presence_penalty"]; got != 0.5 {
		t.Errorf("presence_penalty = %v, want the requested 0.5 passed through: %s", got, mustJSON(t, body))
	}
	if got := body["frequency_penalty"]; got != 0.3 {
		t.Errorf("frequency_penalty = %v, want the requested 0.3 passed through: %s", got, mustJSON(t, body))
	}
	if got := body["top_p"]; got != 1.0 {
		t.Errorf("top_p on the wire = %v, want the fixed 1.0 (the caller's 0.5 is not sent): %s", got, mustJSON(t, body))
	}
	if m, ok := body["thinking"].(map[string]any); !ok || m["type"] != "disabled" {
		t.Errorf("thinking toggle = %v, want {\"type\":\"disabled\"}: %s", body["thinking"], mustJSON(t, body))
	}
	if got := logs.String(); strings.Contains(got, "inert while thinking mode is on") {
		t.Errorf("a non-thinking request logged a drop although nothing was dropped:\n%s", got)
	}
}

// TestSamplingContract_UnconfiguredSendsNoSamplingKnobs covers the third state:
// nothing asked for. top_p must not appear at all (an unasked-for value is not
// 1.0 by accident), and an unconfigured thinking mode — the provider's default
// is ON — must still drop a configured temperature and say so.
func TestSamplingContract_UnconfiguredSendsNoSamplingKnobs(t *testing.T) {
	fx := loadShimFixture(t)
	ex := fx.must(t, "openai", "thinking_disabled_inert_params")

	t.Run("nothing configured", func(t *testing.T) {
		body := callOpenAI(t, openaiTestClient(t, "", nil), requestMessages(t, ex.Request), RequestOptions{}, ex)
		for _, p := range []string{"temperature", "presence_penalty", "frequency_penalty", "top_p"} {
			if v, present := body[p]; present {
				t.Errorf("%s = %v was sent although nothing asked for it: %s", p, v, mustJSON(t, body))
			}
		}
	})

	t.Run("configured temperature with thinking unconfigured", func(t *testing.T) {
		logs := captureLogs(t)
		// nil thinking means "not configured"; the provider's documented default
		// is thinking ENABLED, so temperature is inert and must not be sent.
		body := callOpenAI(t, openaiTestClient(t, "", &Config{Temperature: 0.7}),
			requestMessages(t, ex.Request), RequestOptions{}, ex)
		if v, present := body["temperature"]; present {
			t.Errorf("temperature = %v was sent while the provider reasons by default: %s", v, mustJSON(t, body))
		}
		if _, present := body["thinking"]; present {
			t.Errorf("an unconfigured thinking mode must stay off the wire: %s", mustJSON(t, body))
		}
		if got := logs.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "temperature") {
			t.Errorf("the drop of a configured temperature was not named at WARN:\n%s", got)
		}
	})
}

// TestSamplingContract_CoversEveryFormat proves the rule is per thinking mode
// and not per wire format: the OpenAI Chat Completions body, the Anthropic
// Messages body and the Responses builder must all resolve it identically
// (spec 024 §B2 applies to `all` formats).
func TestSamplingContract_CoversEveryFormat(t *testing.T) {
	msgs := []harness.Message{{Role: "user", Content: "hi"}}
	half := 0.5

	onOpts := func(thinking *bool) RequestOptions {
		return RequestOptions{
			Thinking:         thinking,
			Temperature:      0.7,
			PresencePenalty:  0.5,
			FrequencyPenalty: 0.3,
			TopP:             &half,
		}
	}

	builders := map[string]func(t *testing.T, thinking *bool) map[string]any{
		"openai-chat-completions": func(t *testing.T, thinking *bool) map[string]any {
			t.Helper()
			fx := loadShimFixture(t)
			ex := fx.must(t, "openai", "thinking_disabled_inert_params")
			client := openaiTestClient(t, "", &Config{Temperature: 0.7, Thinking: thinking})
			return callOpenAI(t, client, msgs, onOpts(thinking), ex)
		},
		"anthropic-messages": func(t *testing.T, thinking *bool) map[string]any {
			t.Helper()
			client := anthropicTestClient(t, "https://api.deepseek.com/anthropic", &Config{Temperature: 0.7, Thinking: thinking})
			dial, err := MapReasoningEffort("high")
			if err != nil {
				t.Fatalf("MapReasoningEffort: %v", err)
			}
			body := client.buildRequest(msgs, onOpts(thinking), dial)
			wire, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal anthropic request: %v", err)
			}
			return decodeMap(t, wire, "anthropic request body")
		},
		"responses": func(t *testing.T, thinking *bool) map[string]any {
			t.Helper()
			body, err := BuildResponsesRequest("deepseek-flash", msgs, onOpts(thinking))
			if err != nil {
				t.Fatalf("BuildResponsesRequest: %v", err)
			}
			wire, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal responses request: %v", err)
			}
			return decodeMap(t, wire, "responses request body")
		},
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			on := true
			body := build(t, &on)
			for _, p := range inertDropParams {
				if v, present := body[p]; present {
					t.Errorf("thinking ON: %s = %v went on the wire although it is inert: %s", p, v, mustJSON(t, body))
				}
			}
			if got := body["top_p"]; got != 0.95 {
				t.Errorf("thinking ON: top_p = %v, want the floor 0.95: %s", got, mustJSON(t, body))
			}

			off := false
			body = build(t, &off)
			if got := body["temperature"]; got != 0.7 {
				t.Errorf("thinking OFF: temperature = %v, want 0.7 passed through: %s", got, mustJSON(t, body))
			}
			if got := body["presence_penalty"]; got != 0.5 {
				t.Errorf("thinking OFF: presence_penalty = %v, want 0.5 passed through: %s", got, mustJSON(t, body))
			}
			if got := body["frequency_penalty"]; got != 0.3 {
				t.Errorf("thinking OFF: frequency_penalty = %v, want 0.3 passed through: %s", got, mustJSON(t, body))
			}
			if got := body["top_p"]; got != 1.0 {
				t.Errorf("thinking OFF: top_p = %v, want the fixed 1.0: %s", got, mustJSON(t, body))
			}
		})
	}
}

// TestSamplingContract_DroppedIsSubsetOfInertSet keeps the two sets honest: what
// a request drops must be drawn from the documented inert set, and the documented
// set must not grow or shrink without this failing.
func TestSamplingContract_DroppedIsSubsetOfInertSet(t *testing.T) {
	on, off := true, false

	documented := InertThinkingParams(&on)
	if strings.Join(documented, ",") != strings.Join(inertDropParams, ",") {
		t.Fatalf("documented inert set = %v, want %v", documented, inertDropParams)
	}

	cases := []struct {
		name string
		p    SamplingParams
		want []string
	}{
		{"all three", SamplingParams{Temperature: 0.7, PresencePenalty: 0.5, FrequencyPenalty: 0.3}, inertDropParams},
		{"temperature only", SamplingParams{Temperature: 0.7}, []string{"temperature"}},
		{"none", SamplingParams{}, nil},
	}
	for _, tc := range cases {
		got := ResolveSamplingParams(tc.p, &on)
		if strings.Join(got.Dropped, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: dropped = %v, want %v", tc.name, got.Dropped, tc.want)
		}
		for _, name := range got.Dropped {
			if !contains(documented, name) {
				t.Errorf("%s: dropped %q is not in the documented inert set %v", tc.name, name, documented)
			}
		}
		if got.Temperature != nil || got.PresencePenalty != nil || got.FrequencyPenalty != nil {
			t.Errorf("%s: a thinking-mode resolution must carry no wire values: %+v", tc.name, got)
		}
	}

	// Thinking explicitly off: nothing is inert, and the requested values are
	// exactly what the resolution puts back on the wire.
	res := ResolveSamplingParams(SamplingParams{Temperature: 0.7, PresencePenalty: 0.5, FrequencyPenalty: 0.3}, &off)
	if len(res.Dropped) != 0 {
		t.Errorf("thinking off: dropped = %v, want nothing", res.Dropped)
	}
	if wireFloat(res.Temperature) != 0.7 || wireFloat(res.PresencePenalty) != 0.5 || wireFloat(res.FrequencyPenalty) != 0.3 {
		t.Errorf("thinking off: resolution = %+v, want the requested values", res)
	}
	if len(InertThinkingParams(&off)) != 0 {
		t.Errorf("thinking off must have an empty inert set: %v", InertThinkingParams(&off))
	}
}

// TestSamplingContract_TopPSemanticsAreEncoded pins the resolution table rather
// than the prose: the floor in thinking mode, the fixed 1.0 outside it, and no
// value at all when nobody asked.
func TestSamplingContract_TopPSemanticsAreEncoded(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name      string
		requested float64
		thinking  *bool
		want      float64
		send      bool
	}{
		{"unconfigured", 0, nil, 0, false},
		{"unconfigured with thinking on", 0, &on, 0, false},
		{"below the floor is floored", 0.5, &on, 0.95, true},
		{"inside the range is kept", 0.98, &on, 0.98, true},
		{"above 1.0 is clamped", 1.5, &on, 1.0, true},
		{"non-thinking sends the fixed 1.0", 0.5, &off, 1.0, true},
	}
	for _, tc := range cases {
		got, send := ResolveTopP(tc.requested, tc.thinking)
		if send != tc.send {
			t.Errorf("%s: ResolveTopP(%v) send = %v, want %v", tc.name, tc.requested, send, tc.send)
		}
		if send && got != tc.want {
			t.Errorf("%s: ResolveTopP(%v) = %v, want %v", tc.name, tc.requested, got, tc.want)
		}
	}
}

// TestSamplingContract_ThinkingRouteKeepsProducingATurn guards against a
// contract that is satisfied by breaking the call: dropping the parameters must
// not change what the client does with a real response.
func TestSamplingContract_ThinkingRouteKeepsProducingATurn(t *testing.T) {
	fx := loadShimFixture(t)
	ex := fx.must(t, "openai", "thinking_disabled_inert_params")
	srv, captured := replay(t, ex)

	on := true
	half := 0.5
	client := openaiTestClient(t, srv.URL, &Config{Temperature: 0.7, Thinking: &on})
	resp, err := client.CallWithOptions(context.Background(), requestMessages(t, ex.Request), RequestOptions{
		PresencePenalty: 0.5, TopP: &half,
	})
	// The recorded answer is the provider's own bytes ("ok"), not an AgentOutput
	// document, so that parse error is expected here (the same convention the
	// matrix helpers use). What this test pins is the call path: the request went
	// out exactly once and a response object came back — dropping the inert
	// parameters did not turn the thinking route into a failure or a retry.
	if err != nil && !strings.Contains(err.Error(), "AgentOutput JSON") {
		t.Fatalf("thinking-mode call against the recorded exchange failed: %v", err)
	}
	if resp == nil {
		t.Fatal("no response returned")
	}
	if len(*captured) != 1 {
		t.Fatalf("captured %d requests, want 1", len(*captured))
	}
	body := bodyJSON(t, (*captured)[0])
	if v, present := body["temperature"]; present {
		t.Errorf("temperature = %v went on the wire on the thinking route: %s", v, mustJSON(t, body))
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
