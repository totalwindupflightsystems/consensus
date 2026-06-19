// Package llm: tests for LLM client abstraction.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/008-harness.md,specs/006-transactions.md plan=phase-1/task-1 test=internal/llm/client_test.go
package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// Mock Client Tests
// ============================================================================

func TestMockClient_ReturnsConfiguredOutput(t *testing.T) {
	mock := &MockClient{
		Output: &harness.AgentOutput{
			InternalMonologue: "I should insert a memory event",
			MemoryStateChanges: []string{
				"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'hello', 's-1', 1)",
			},
			SystemActions: []string{
				"UPDATE sessions SET status = 'idle' WHERE id = 's-1'",
			},
		},
	}

	output, err := mock.Call(context.Background(), []harness.Message{
		{Role: "system", Content: "test prompt"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Output.InternalMonologue != "I should insert a memory event" {
		t.Errorf("monologue = %q, want %q", output.Output.InternalMonologue, "I should insert a memory event")
	}
	if len(output.Output.MemoryStateChanges) != 1 {
		t.Errorf("memory_state_changes len = %d, want 1", len(output.Output.MemoryStateChanges))
	}
	if len(output.Output.SystemActions) != 1 {
		t.Errorf("system_actions len = %d, want 1", len(output.Output.SystemActions))
	}
}

func TestMockClient_ReturnsError(t *testing.T) {
	mock := &MockClient{
		Err: context.DeadlineExceeded,
	}

	_, err := mock.Call(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from mock client")
	}
}

func TestMockClient_DefaultNoOp(t *testing.T) {
	mock := NewMockClient()

	output, err := mock.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected non-nil output")
	}
	if len(output.Output.MemoryStateChanges) != 0 {
		t.Errorf("default mock should have empty memory_state_changes, got %d", len(output.Output.MemoryStateChanges))
	}
}

func TestMockClient_PassesMessages(t *testing.T) {
	mock := NewMockClient()
	messages := []harness.Message{
		{Role: "system", Content: "You are a test agent."},
		{Role: "user", Content: "Context: session s-1"},
	}

	// The mock ignores messages but should not error
	_, err := mock.Call(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Config Tests
// ============================================================================

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr string
	}{
		{"valid", &Config{Provider: ProviderOpenAI, Model: "gpt-4o"}, ""},
		{"valid mock", &Config{Provider: ProviderMock, Model: "mock-model"}, ""},
		{"nil config", nil, "nil"},
		{"empty provider", &Config{Model: "gpt-4o"}, "provider"},
		{"empty model", &Config{Provider: ProviderOpenAI}, "model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.wantErr != "" && err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			}
			if tt.wantErr != "" && err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != ProviderMock {
		t.Errorf("default provider = %s, want mock", cfg.Provider)
	}
	if cfg.Model == "" {
		t.Error("default model should not be empty")
	}
	if cfg.MaxTokens == 0 {
		t.Error("default max_tokens should not be zero")
	}
}

// ============================================================================
// Factory Tests
// ============================================================================

func TestNewClient_Mock(t *testing.T) {
	t.Setenv("CONSENSUS_MOCK_LLM", "1")
	client, err := NewClient(&Config{
		Provider: ProviderMock,
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_MockRejectsWithoutEnv(t *testing.T) {
	// Ensure no env var is set for this test
	t.Setenv("CONSENSUS_MOCK_LLM", "")
	_, err := NewClient(&Config{
		Provider: ProviderMock,
		Model:    "test-model",
	})
	if err == nil {
		t.Fatal("expected error when CONSENSUS_MOCK_LLM is not set")
	}
	if !strings.Contains(err.Error(), "CONSENSUS_MOCK_LLM") {
		t.Errorf("error should mention CONSENSUS_MOCK_LLM: %v", err)
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	_, err := NewClient(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewClient_UnknownProvider(t *testing.T) {
	_, err := NewClient(&Config{
		Provider: "unknown",
		Model:    "test",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention unknown provider: %v", err)
	}
}

// ============================================================================
// Client Tests (stub client removed; mock is the primary test client)
// ============================================================================

func TestNewOpenAIClient_WithoutAPIKey_StillCreates(t *testing.T) {
	// NewOpenAIClient creates a real HTTP client even without an API key.
	// It will fail on Call() but should not panic or return nil.
	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o",
	})
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// Verify it's not a mock
	if _, ok := client.(*MockClient); ok {
		t.Error("expected real client, got mock")
	}
}

func TestAnthropicClient_CreatedWithoutPanic(t *testing.T) {
	// NewAnthropicClient creates a real HTTP client even without an API key.
	// It will fail on Call() but should not panic or return nil.
	client := NewAnthropicClient(&Config{
		Provider: ProviderAnthropic,
		Model:    "claude-sonnet-4-20250514",
		MaxTokens: 1024,
	})
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// Verify it's not a mock
	if _, ok := client.(*MockClient); ok {
		t.Error("expected real anthropic client, got mock")
	}
}

func TestAnthropicClientSatisfiesInterface(t *testing.T) {
	// Compile-time check: anthropicClient must implement harness.LLMClient
	var _ harness.LLMClient = (*anthropicClient)(nil)
}

// ============================================================================
// Integration: Mock + Harness Types
// ============================================================================

func TestMockClientSatisfiesInterface(t *testing.T) {
	// Compile-time check: MockClient must implement harness.LLMClient
	var _ harness.LLMClient = (*MockClient)(nil)
}

func TestMockClient_WithToolRequests(t *testing.T) {
	mock := &MockClient{
		Output: &harness.AgentOutput{
			InternalMonologue: "Need to scrape a website",
			ToolRequests: []harness.ToolRequest{
				{ToolName: "web_scraper", Parameters: map[string]any{"url": "https://example.com"}},
			},
		},
	}

	output, err := mock.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Output.ToolRequests) != 1 {
		t.Errorf("expected 1 tool request, got %d", len(output.Output.ToolRequests))
	}
	if output.Output.ToolRequests[0].ToolName != "web_scraper" {
		t.Errorf("tool name = %q, want web_scraper", output.Output.ToolRequests[0].ToolName)
	}
}

func TestMockClient_WithSubAgentSpawns(t *testing.T) {
	mock := &MockClient{
		Output: &harness.AgentOutput{
			InternalMonologue: "I'll spawn a summarizer",
			SubAgentSpawns: []harness.SubAgentSpawn{
				{AgentName: "summarizer", Goal: "Summarize the results", ParentGoal: "research"},
			},
		},
	}

	output, err := mock.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Output.SubAgentSpawns) != 1 {
		t.Errorf("expected 1 sub-agent spawn, got %d", len(output.Output.SubAgentSpawns))
	}
	if output.Output.SubAgentSpawns[0].AgentName != "summarizer" {
		t.Errorf("agent name = %q, want summarizer", output.Output.SubAgentSpawns[0].AgentName)
	}
}
