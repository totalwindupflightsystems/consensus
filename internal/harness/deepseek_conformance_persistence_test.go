// Package harness: database-backed persistence arm of the DeepSeek
// conformance matrix (spec 024 §B2, SHIM-SUPPORT-002).
package harness

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

type recordedDeepSeekTurnClient struct {
	content   string
	reasoning string
	output    *AgentOutput
}

func (c *recordedDeepSeekTurnClient) Call(context.Context, []Message) (*LLMResponse, error) {
	return &LLMResponse{
		Output:           c.output,
		Content:          c.content,
		ReasoningContent: c.reasoning,
		ModelID:          "deepseek-flash",
	}, nil
}

// TestDeepSeekConformanceMatrix_PersistsReasoningContentWithTurn drives a real
// harness iteration into a real temporary SQLite database, then reads the exact
// iteration_commits.llm_response row back. The reasoning bytes come from the
// recorded DeepSeek fixture; this is not a marshal-helper-only assertion.
func TestDeepSeekConformanceMatrix_PersistsReasoningContentWithTurn(t *testing.T) {
	raw, err := os.ReadFile("../llm/testdata/thinking-cot-turns.json")
	if err != nil {
		t.Fatalf("read recorded DeepSeek fixture: %v", err)
	}
	var fixture struct {
		ToolsAbsent struct {
			Turn1 struct {
				Response struct {
					Choices []struct {
						Message struct {
							Content          string `json:"content"`
							ReasoningContent string `json:"reasoning_content"`
						} `json:"message"`
					} `json:"choices"`
				} `json:"response"`
			} `json:"turn1"`
		} `json:"tools_absent"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode recorded DeepSeek fixture: %v", err)
	}
	if len(fixture.ToolsAbsent.Turn1.Response.Choices) != 1 {
		t.Fatalf("fixture has %d choices, want 1", len(fixture.ToolsAbsent.Turn1.Response.Choices))
	}
	recorded := fixture.ToolsAbsent.Turn1.Response.Choices[0].Message
	if recorded.ReasoningContent == "" {
		t.Fatal("fixture precondition: recorded turn has no reasoning_content")
	}

	output := minimalOutput()
	client := &recordedDeepSeekTurnClient{
		content:   recorded.Content,
		reasoning: recorded.ReasoningContent,
		output:    output,
	}
	th, err := newTestHarness(client)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	result, err := th.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("run harness iteration: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("iteration status = %q, want success", result.Status)
	}

	rows, err := th.conn.Query(th.ctx,
		`SELECT llm_response FROM iteration_commits WHERE session_id = $1 ORDER BY iteration_id DESC LIMIT 1`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("read persisted turn: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("persisted turn rows = %d, want 1", len(rows))
	}
	persistedRaw := toString(rows[0]["llm_response"])
	var persisted map[string]any
	if err := json.Unmarshal([]byte(persistedRaw), &persisted); err != nil {
		t.Fatalf("persisted llm_response is not JSON: %v\n%s", err, persistedRaw)
	}
	if got := persisted["reasoning_content"]; got != recorded.ReasoningContent {
		t.Fatalf("persisted reasoning_content = %v, want recorded bytes %q", got, recorded.ReasoningContent)
	}
	if got := persisted["internal_monologue"]; got != output.InternalMonologue {
		t.Fatalf("turn output was not persisted alongside reasoning_content: internal_monologue=%v", got)
	}
	if got := persisted["reasoning_content"]; got == persisted["internal_monologue"] {
		t.Fatal("reasoning_content was conflated with AgentOutput.internal_monologue")
	}
}
