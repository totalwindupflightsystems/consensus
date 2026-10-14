// Package llm: focused, executable DeepSeek behavior matrix for SHIM-SUPPORT-002.
//
// These tests replay recorded provider responses through the real OpenAI-format
// client and inspect the actual request bytes captured by httptest. No test in
// this file calls a live provider.
package llm

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools proves both
// arms of spec 024's round-trip rule against the same conversation: every prior
// reasoning_content value is re-sent when tools are present, and every value is
// omitted when tools are absent.
func TestDeepSeekConformanceMatrix_ResendsAllReasoningOnlyWithTools(t *testing.T) {
	fx := loadCOTFixture(t)
	messages := recordedMessages(t, fx.ToolsPresent.Turn2.Request)

	// Add a second prior reasoning turn using another recorded provider response.
	// This makes "all previous turns" observable rather than accidentally proving
	// only the single-turn case.
	extraRecorded := mustDecodeRecorded(t, fx.ToolsAbsent.Turn1.Response, "no-tools turn1").Choices[0].Message
	if extraRecorded.ReasoningContent == "" {
		t.Fatal("fixture precondition: second recorded turn has no reasoning_content")
	}
	extra := harness.Message{
		Role:             "assistant",
		Content:          extraRecorded.Content,
		ReasoningContent: extraRecorded.ReasoningContent,
	}
	insertAt := len(messages) - 1
	messages = append(messages, harness.Message{})
	copy(messages[insertAt+1:], messages[insertAt:])
	messages[insertAt] = extra

	wantReasoning := map[int]string{}
	for i, msg := range messages {
		if msg.ReasoningContent != "" {
			wantReasoning[i] = msg.ReasoningContent
		}
	}
	if len(wantReasoning) < 2 {
		t.Fatalf("fixture setup produced %d reasoning turns, want at least 2", len(wantReasoning))
	}

	call := func(t *testing.T, tools []ToolDefinition) map[string]any {
		t.Helper()
		srv, captured := newReplayServer(t, fx.ToolsPresent.Turn2.Response)
		client := NewOpenAIClient(&Config{
			Provider:  ProviderOpenAI,
			BaseURL:   srv.URL,
			Model:     "deepseek-flash",
			MaxTokens: 256,
		}).(*openaiClient)
		_, err := client.CallWithOptions(t.Context(), messages, RequestOptions{Tools: tools})
		if err != nil && !strings.Contains(err.Error(), "AgentOutput JSON") {
			t.Fatalf("recorded replay failed before request-shape assertion: %v", err)
		}
		if len(*captured) != 1 {
			t.Fatalf("captured %d requests, want 1", len(*captured))
		}
		return bodyJSON(t, (*captured)[0])
	}

	t.Run("tools_present_resends_every_prior_reasoning_turn", func(t *testing.T) {
		body := call(t, recordedTools(t, fx.ToolsPresent.Turn2.Request))
		sent := body["messages"].([]any)
		for i, want := range wantReasoning {
			got, ok := sent[i].(map[string]any)["reasoning_content"].(string)
			if !ok || got != want {
				t.Errorf("messages[%d].reasoning_content = %q (present=%v), want %q", i, got, ok, want)
			}
		}
		if _, ok := body["tools"]; !ok {
			t.Fatal("tools-present arm did not put tools on the wire")
		}
	})

	t.Run("tools_absent_omits_every_prior_reasoning_turn", func(t *testing.T) {
		body := call(t, nil)
		sent := body["messages"].([]any)
		for i, item := range sent {
			if got, ok := item.(map[string]any)["reasoning_content"]; ok {
				t.Errorf("messages[%d] retained reasoning_content without tools: %v", i, got)
			}
		}
		if _, ok := body["tools"]; ok {
			t.Fatal("tools-absent arm unexpectedly put tools on the wire")
		}
	})
}

// TestDeepSeekConformanceMatrix_ThinkingControlIsExplicitPerRequest proves the
// toggle and effort are resolved from each request, not silently inherited from
// provider defaults or frozen at client construction.
func TestDeepSeekConformanceMatrix_ThinkingControlIsExplicitPerRequest(t *testing.T) {
	fx := loadCOTFixture(t)
	srv, captured := newReplayServer(t,
		fx.ToolsAbsent.Turn1.Response,
		fx.ToolsAbsent.Turn1.Response,
		fx.ToolsAbsent.Turn1.Response,
	)
	client := NewOpenAIClient(&Config{
		Provider:        ProviderOpenAI,
		BaseURL:         srv.URL,
		Model:           "deepseek-flash",
		MaxTokens:       128,
		Thinking:        boolPtr(false),
		ReasoningEffort: "max",
	}).(*openaiClient)
	messages := recordedMessages(t, fx.ToolsAbsent.Turn1.Request)

	cases := []struct {
		name       string
		opts       RequestOptions
		wantToggle string
		wantEffort string
	}{
		{name: "request_enables_low", opts: RequestOptions{Thinking: boolPtr(true), ReasoningEffort: "minimal"}, wantToggle: "enabled", wantEffort: "low"},
		{name: "request_disables_and_omits_effort", opts: RequestOptions{Thinking: boolPtr(false), ReasoningEffort: "high"}, wantToggle: "disabled"},
		{name: "next_request_enables_max", opts: RequestOptions{Thinking: boolPtr(true), ReasoningEffort: "ultra"}, wantToggle: "enabled", wantEffort: "max"},
	}

	for _, tc := range cases {
		if _, err := client.CallWithOptions(t.Context(), messages, tc.opts); err != nil {
			t.Fatalf("%s: recorded replay failed: %v", tc.name, err)
		}
	}
	if len(*captured) != len(cases) {
		t.Fatalf("captured %d requests, want %d", len(*captured), len(cases))
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := bodyJSON(t, (*captured)[i])
			thinking, ok := body["thinking"].(map[string]any)
			if !ok || thinking["type"] != tc.wantToggle {
				t.Fatalf("thinking = %v, want type=%q", body["thinking"], tc.wantToggle)
			}
			gotEffort, _ := body["reasoning_effort"].(string)
			if gotEffort != tc.wantEffort {
				t.Fatalf("reasoning_effort = %q, want %q", gotEffort, tc.wantEffort)
			}
		})
	}
}

// TestDeepSeekConformanceMatrix_ReasoningFallbackIsLoggedFlaggedAndLastResort
// proves the empty-content fallback is an observable emergency path: it emits a
// WARN, sets the response flag, and does not run when real content exists.
func TestDeepSeekConformanceMatrix_ReasoningFallbackIsLoggedFlaggedAndLastResort(t *testing.T) {
	fx := loadCOTFixture(t)
	oldLogger := slog.Default()
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	t.Run("empty_content_promotes_with_warn_and_flag", func(t *testing.T) {
		logs.Reset()
		srv, _ := newReplayServer(t, fx.ToolsPresent.Turn1.Response)
		client := NewOpenAIClient(&Config{
			Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 128,
		}).(*openaiClient)
		resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsPresent.Turn1.Request))
		if err == nil {
			t.Fatal("fixture's promoted reasoning is not AgentOutput JSON; expected the parse failure")
		}
		if resp == nil || !resp.ReasoningPromotedToOutput {
			t.Fatalf("fallback was not flagged on the response: %+v", resp)
		}
		if resp.Content != "" || resp.ReasoningContent == "" {
			t.Fatalf("fixture no longer drives the last-resort precondition: content=%q reasoning=%q", resp.Content, resp.ReasoningContent)
		}
		if got := logs.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "LAST-RESORT promotion") {
			t.Fatalf("fallback did not emit the named WARN path:\n%s", got)
		}
	})

	t.Run("nonempty_content_never_promotes", func(t *testing.T) {
		logs.Reset()
		srv, _ := newReplayServer(t, fx.ToolsAbsent.Turn1.Response)
		client := NewOpenAIClient(&Config{
			Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 128,
		}).(*openaiClient)
		resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsAbsent.Turn1.Request))
		if err != nil {
			t.Fatalf("recorded non-empty response failed: %v", err)
		}
		if resp == nil || resp.ReasoningPromotedToOutput {
			t.Fatalf("non-empty content took the fallback path: %+v", resp)
		}
		if strings.Contains(logs.String(), "LAST-RESORT promotion") {
			t.Fatalf("non-empty content emitted the fallback warning:\n%s", logs.String())
		}
	})

	t.Run("disabled_fallback_keeps_reasoning_but_does_not_promote", func(t *testing.T) {
		logs.Reset()
		srv, _ := newReplayServer(t, fx.ToolsPresent.Turn1.Response)
		client := NewOpenAIClient(&Config{
			Provider: ProviderOpenAI, BaseURL: srv.URL, Model: "deepseek-flash", MaxTokens: 128,
			AllowReasoningFallback: boolPtr(false),
		}).(*openaiClient)
		resp, err := client.Call(t.Context(), recordedMessages(t, fx.ToolsPresent.Turn1.Request))
		if err == nil {
			t.Fatal("expected empty content to remain unparsable when fallback is disabled")
		}
		if resp == nil || resp.ReasoningPromotedToOutput || resp.ReasoningContent == "" {
			t.Fatalf("disabled fallback response = %+v", resp)
		}
		if got := logs.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "CoT NOT promoted") {
			t.Fatalf("disabled fallback did not emit its named WARN path:\n%s", got)
		}
	})
}

// TestDeepSeekConformanceMatrix_ProducesVerifiableScenarioTable requires the
// existing ShimSupportMatrix to expose the focused scenario table and prints it
// in the test output, making each coverage claim inspectable.
func TestDeepSeekConformanceMatrix_ProducesVerifiableScenarioTable(t *testing.T) {
	rows := ShimSupportMatrix().BehaviorScenarios
	if len(rows) != 5 {
		t.Fatalf("BehaviorScenarios has %d rows, want 5 explicit request/response scenarios", len(rows))
	}
	specRaw, err := os.ReadFile(filepath.Join("..", "..", "specs", "024-shim-support-matrix.md"))
	if err != nil {
		t.Fatalf("read spec 024: %v", err)
	}
	spec := string(specRaw)

	var report strings.Builder
	report.WriteString("\nSCENARIO | FIXTURE | EXPECTED REQUEST SHAPE | EXPECTED RESPONSE HANDLING | TEST\n")
	seen := map[string]bool{}
	for i, row := range rows {
		values := []string{row.ID, row.Fixture, row.ExpectedRequestShape, row.ExpectedResponseHandling, row.Test}
		for j, value := range values {
			if strings.TrimSpace(value) == "" {
				t.Fatalf("BehaviorScenarios[%d] field %d is empty", i, j)
			}
		}
		if seen[row.ID] {
			t.Fatalf("duplicate behavior scenario %q", row.ID)
		}
		seen[row.ID] = true

		testName := strings.SplitN(row.Test, "/", 2)[0]
		if !conformanceTestFunctionExists(t, testName) {
			t.Fatalf("scenario %s names missing test function %s", row.ID, testName)
		}
		if !strings.Contains(spec, "| "+row.ID+" |") || !strings.Contains(spec, "`"+row.Test+"`") {
			t.Fatalf("scenario %s is not documented with executable test %s in spec 024", row.ID, row.Test)
		}
		report.WriteString(strings.Join(values, " | ") + "\n")
	}

	for _, id := range []string{"TURN-PERSIST", "TOOLS-RESEND", "NO-TOOLS-OMIT", "THINKING-PER-REQUEST", "FALLBACK-LAST-RESORT"} {
		if !seen[id] {
			t.Errorf("behavior matrix is missing %s", id)
		}
	}
	t.Log(report.String())
}

func conformanceTestFunctionExists(t *testing.T, name string) bool {
	t.Helper()
	for _, dir := range []string{".", filepath.Join("..", "harness")} {
		paths, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatalf("list test files in %s: %v", dir, err)
		}
		for _, path := range paths {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse test functions in %s: %v", path, err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Recv == nil && fn.Name.Name == name {
					return true
				}
			}
		}
	}
	return false
}
