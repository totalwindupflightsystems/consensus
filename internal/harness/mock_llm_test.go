// Package harness: mock LLM client for integration testing.
//
// The mock returns controlled AgentOutput payloads so integration tests
// can verify the full iteration loop end-to-end without calling a real LLM.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/008-harness.md,specs/006-transactions.md plan=phase-1/task-1 test=internal/harness/mock_llm_test.go
package harness

import (
	"context"
	"fmt"
)

// mockLLMClient returns a fixed AgentOutput for testing.
type mockLLMClient struct {
	output  *AgentOutput
	err     error
	modelID string
	usage   LLMUsage
}

// Call implements LLMClient by returning the pre-configured output.
func (m *mockLLMClient) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.output == nil {
		return nil, fmt.Errorf("mockLLM: no output configured")
	}
	modelID := m.modelID
	if modelID == "" {
		modelID = "mock-model"
	}
	return &LLMResponse{
		Output:     m.output,
		ModelID:    modelID,
		Usage:      m.usage,
		DurationMs: 0,
	}, nil
}

// newMockLLM creates a mock that returns the given output.
func newMockLLM(output *AgentOutput) *mockLLMClient {
	return &mockLLMClient{
		output:  output,
		usage:   LLMUsage{},
		modelID: "mock-model",
	}
}

// failingMockLLM creates a mock that always fails with the given error.
func failingMockLLM(err error) *mockLLMClient {
	return &mockLLMClient{err: err}
}

// minimalOutput returns a valid but minimal AgentOutput for the happy path.
func minimalOutput() *AgentOutput {
	return &AgentOutput{
		InternalMonologue: "This is a test iteration. I will record a memory event and update session status.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Test memory event from iteration', 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1', 1)",
		},
		SystemActions: []string{},
		ToolRequests:  nil,
	}
}

// outputWithToolCall returns an AgentOutput with a tool request.
func outputWithToolCall() *AgentOutput {
	return &AgentOutput{
		InternalMonologue: "I need to scrape a webpage to complete this task.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('header', 'Starting web scrape', 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1', 1)",
		},
		SystemActions: []string{},
		ToolRequests: []ToolRequest{
			{ToolName: "web_scraper", Parameters: map[string]any{"url": "https://example.com"}},
		},
	}
}
