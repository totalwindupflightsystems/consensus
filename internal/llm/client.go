// Package llm provides an abstract LLM client interface and provider implementations.
//
// The harness defines the LLMClient interface (harness.LLMClient) and types
// (harness.Message, harness.AgentOutput). This package implements those
// interfaces for each provider (OpenAI, Anthropic, mock).
//
// The harness does NOT import this package — wiring happens at the application
// level in cmd/consensus/.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/008-harness.md,specs/006-transactions.md plan=phase-1/task-1 impl=internal/llm/client.go
package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// Client Interface (mirrors harness.LLMClient for factory convenience)
// ============================================================================

// Client is a convenience alias for harness.LLMClient.
// All provider implementations satisfy harness.LLMClient directly.
type Client = harness.LLMClient

// ============================================================================
// Provider Type
// ============================================================================

// Provider identifies the LLM provider.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenRouter Provider = "openrouter"
	ProviderMock      Provider = "mock"
)

// ============================================================================
// Config
// ============================================================================

// ResponseFormat selects the OpenAI response_format type.
type ResponseFormat string

const (
	// ResponseFormatJSONObject requests unstructured JSON output (default).
	// The model returns valid JSON but the shape is not guaranteed.
	ResponseFormatJSONObject ResponseFormat = "json_object"

	// ResponseFormatJSONSchema requests structured output with strict schema
	// enforcement. The model must return JSON matching the provided schema.
	ResponseFormatJSONSchema ResponseFormat = "json_schema"
)

// Config holds LLM client configuration.
type Config struct {
	// Provider selects which LLM backend to use.
	Provider Provider `json:"provider" yaml:"provider"`

	// BaseURL overrides the API base URL (for OpenRouter, proxies, etc.).
	// Default: https://api.openai.com/v1 (OpenAI), https://openrouter.ai/api/v1 (OpenRouter).
	BaseURL string `json:"base_url" yaml:"base_url"`

	// APIKey is the provider API key (not logged, not stored in evidence).
	APIKey string `json:"-" yaml:"-"`

	// Model is the model identifier (e.g. "gpt-4o", "claude-sonnet-4-20250514").
	Model string `json:"model" yaml:"model"`

	// MaxTokens is the maximum output token limit.
	MaxTokens int `json:"max_tokens" yaml:"max_tokens"`

	// Temperature controls output randomness (0.0-2.0).
	Temperature float64 `json:"temperature" yaml:"temperature"`

	// EnableCache enables prompt caching where the provider supports it.
	EnableCache bool `json:"enable_cache" yaml:"enable_cache"`

	// ResponseFormat controls OpenAI structured output mode.
	// Default: "json_object". Set to "json_schema" for strict schema enforcement.
	// Only applies to OpenAI/OpenRouter providers.
	ResponseFormat ResponseFormat `json:"response_format" yaml:"response_format"`
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		Provider:       ProviderMock,
		Model:          "gpt-4o",
		MaxTokens:      16384,
		Temperature:    0.0,
		EnableCache:    true,
		ResponseFormat: ResponseFormatJSONObject,
	}
}

// Validate checks that the configuration is usable.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("llm: config is nil")
	}
	if c.Provider == "" {
		return fmt.Errorf("llm: provider is required")
	}
	if c.Model == "" {
		return fmt.Errorf("llm: model is required")
	}
	return nil
}

// ============================================================================
// Factory
// ============================================================================

// NewClient creates the appropriate LLM client based on the provider config.
//
// Production safety: when provider is "mock", CONSENSUS_MOCK_LLM=1 must
// be set explicitly to prevent accidental mock usage in production.
func NewClient(cfg *Config) (harness.LLMClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.Provider {
	case ProviderMock:
		if os.Getenv("CONSENSUS_MOCK_LLM") != "1" {
			return nil, fmt.Errorf("llm: mock provider requires CONSENSUS_MOCK_LLM=1 env var")
		}
		return NewMockClient(), nil
	case ProviderOpenAI:
		return NewOpenAIClient(cfg), nil
	case ProviderOpenRouter:
		// OpenRouter is API-compatible with OpenAI; same client, different base URL.
		return NewOpenAIClient(cfg), nil
	case ProviderAnthropic:
		return NewAnthropicClient(cfg), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q", cfg.Provider)
	}
}

// ============================================================================
// Mock Client (for deterministic testing)
// ============================================================================

// MockClient is a deterministic LLM client for testing.
// It returns pre-configured AgentOutput without making API calls.
type MockClient struct {
	Output *harness.AgentOutput
	Usage  harness.LLMUsage
	Err    error
}

// NewMockClient creates a new mock client with a default no-op output.
func NewMockClient() *MockClient {
	return &MockClient{
		Output: &harness.AgentOutput{
			InternalMonologue:  "mock reasoning",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			ToolRequests:       []harness.ToolRequest{},
			SubAgentSpawns:     []harness.SubAgentSpawn{},
		},
		Usage: harness.LLMUsage{PromptTokens: 0, CompletionTokens: 0},
	}
}

// Call implements harness.LLMClient.
func (m *MockClient) Call(ctx context.Context, messages []harness.Message) (*harness.LLMResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &harness.LLMResponse{
		Output:     m.Output,
		ModelID:    "mock-model",
		Usage:      m.Usage,
		DurationMs: 0,
	}, nil
}

// ============================================================================
// Anthropic Client (real HTTP implementation in anthropic_client.go)
// ============================================================================
// NewAnthropicClient is now defined in anthropic_client.go with the real
// Messages API HTTP implementation. The stub was removed in WI-001.
