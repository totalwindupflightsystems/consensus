// Package llm: OpenAI-compatible HTTP client (OpenAI, OpenRouter, etc.).
//
// This implementation replaces the stub client with a real HTTP-based
// implementation that talks to any OpenAI-compatible chat completions API.
// It supports configurable base URL for OpenRouter and other proxies.
//
// axiom:trace work_item=operationalize-01 spec=specs/008-harness.md impl=internal/llm/openai_client.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wojons/conscientiousness/internal/harness"
)

// ============================================================================
// OpenAI / OpenRouter HTTP Client
// ============================================================================

// openaiClient is a real HTTP-based OpenAI-compatible LLM client.
// It satisfies harness.LLMClient and works with OpenAI, OpenRouter,
// and any API that implements the /v1/chat/completions contract.
type openaiClient struct {
	cfg            *Config
	httpClient     *http.Client
	baseURL        string // e.g. https://api.openai.com/v1
	apiKey         string
	model          string
	maxTokens      int
	temperature    float64
	enableCache    bool
	responseFormat ResponseFormat
}

// NewOpenAIClient creates a real OpenAI-compatible HTTP client.
// The base URL defaults to OpenAI but can be overridden for OpenRouter, etc.
func NewOpenAIClient(cfg *Config) harness.LLMClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		switch cfg.Provider {
		case ProviderOpenRouter:
			baseURL = "https://openrouter.ai/api/v1"
		default:
			baseURL = "https://api.openai.com/v1"
		}
	}

	// Default response format
	responseFormat := cfg.ResponseFormat
	if responseFormat == "" {
		responseFormat = ResponseFormatJSONObject
	}

	return &openaiClient{
		cfg:            cfg,
		httpClient:     &http.Client{Timeout: 120 * time.Second},
		baseURL:        baseURL,
		apiKey:         cfg.APIKey,
		model:          cfg.Model,
		maxTokens:      cfg.MaxTokens,
		temperature:    cfg.Temperature,
		enableCache:    cfg.EnableCache,
		responseFormat: responseFormat,
	}
}

// ============================================================================
// Request / Response Types
// ============================================================================

type openaiChatRequest struct {
	Model          string               `json:"model"`
	Messages       []openaiChatMessage  `json:"messages"`
	MaxTokens      int                  `json:"max_tokens,omitempty"`
	Temperature    float64              `json:"temperature,omitempty"`
	ResponseFormat *openaiResponseFmt   `json:"response_format,omitempty"`
	Stream         bool                 `json:"stream"`
}

type openaiChatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openaiResponseFmt struct {
	Type       string              `json:"type"`
	JSONSchema *openaiJSONSchema   `json:"json_schema,omitempty"`
}

type openaiJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}

type openaiChatResponse struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Choices []openaiChatChoice  `json:"choices"`
	Usage   openaiChatUsage     `json:"usage"`
	Error   *openaiError        `json:"error,omitempty"`
}

type openaiChatChoice struct {
	Message openaiChatMessage `json:"message"`
}

type openaiChatUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// ============================================================================
// Call — main interface method
// ============================================================================

// Call sends messages to the OpenAI-compatible API, parses the JSON response
// into AgentOutput, and returns cost/usage metadata.
func (c *openaiClient) Call(ctx context.Context, messages []harness.Message) (*harness.LLMResponse, error) {
	startTime := time.Now()

	// Build request payload. Skip response_format for local providers (LM Studio,
	// Ollama) that don't support structured output constraints on all models.
	respFmt, err := c.buildResponseFormat()
	if err != nil {
		return nil, err
	}
	// LM Studio (localhost:1234) and similar local servers may reject
	// response_format on non-JSON-native models. Detect and skip.
	if c.isLocalProvider() {
		respFmt = nil
	}
	reqBody := openaiChatRequest{
		Model:          c.model,
		Messages:       toOpenAIMessages(messages),
		MaxTokens:      c.maxTokens,
		Temperature:    c.temperature,
		ResponseFormat: respFmt,
		Stream:         false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	// Build HTTP request
	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// OpenRouter-specific headers (harmless on OpenAI)
	req.Header.Set("HTTP-Referer", "https://github.com/wojons/conscientiousness")
	req.Header.Set("X-Title", "Conscience")

	slog.Info("llm: calling provider", "url", url, "model", c.model, "messages", len(reqBody.Messages))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		return nil, fmt.Errorf("llm: http request failed after %dms: %w", elapsed, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read response: %w", err)
	}

	// Parse response with tolerant error handling.
	// Some providers (LM Studio, Ollama) return error as a plain string
	// instead of {message, type, code}. Use RawMessage to handle both.
	var chatResp openaiChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		// Try alternate parse: error might be a plain string
		var rawErr struct {
			Error string `json:"error"`
		}
		if err2 := json.Unmarshal(respBytes, &rawErr); err2 == nil && rawErr.Error != "" {
			return nil, fmt.Errorf("llm: api error (status %d): %s", resp.StatusCode, rawErr.Error)
		}
		return nil, fmt.Errorf("llm: parse response (status %d): %w", resp.StatusCode, err)
	}

	// Handle API errors
	if chatResp.Error != nil {
		return nil, fmt.Errorf("llm: api error (status %d): %s (type=%s, code=%s)",
			resp.StatusCode, chatResp.Error.Message, chatResp.Error.Type, chatResp.Error.Code)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm: http %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm: no choices in response")
	}

	// Extract content — prefer Content, fall back to ReasoningContent for
	// thinking/reasoning models (Qwen thinking, DeepSeek-R1 style) that put
	// their final output in reasoning_content while leaving content empty.
	content := chatResp.Choices[0].Message.Content
	if content == "" {
		content = chatResp.Choices[0].Message.ReasoningContent
		slog.Info("llm: using reasoning_content as primary output (thinking model)",
			"model", chatResp.Model,
			"reasoning_len", len(content),
		)
	}
	content = strings.TrimSpace(content)

	// Some providers wrap JSON in markdown code blocks; strip them
	content = stripMarkdownCodeBlock(content)

	var output harness.AgentOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("llm: parse AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(content, 500))
	}

	elapsed := time.Since(startTime).Milliseconds()

	slog.Info("llm: response received",
		"model", chatResp.Model,
		"elapsed_ms", elapsed,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
	)

	return &harness.LLMResponse{
		Output:      &output,
		ModelID:     chatResp.Model,
		Usage: harness.LLMUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
		DurationMs: elapsed,
	}, nil
}

// ============================================================================
// Response Format Construction
// ============================================================================

// buildResponseFormat returns the appropriate response_format based on config.
func (c *openaiClient) buildResponseFormat() (*openaiResponseFmt, error) {
	switch c.responseFormat {
	case ResponseFormatJSONSchema:
		schema := agentOutputJSONSchema()
		schemaBytes, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("llm: marshal agent output schema: %w", err)
		}
		return &openaiResponseFmt{
			Type: "json_schema",
			JSONSchema: &openaiJSONSchema{
				Name:        "agent_output",
				Description: "Structured output from the Conscience agent cognition loop",
				Schema:      schemaBytes,
				Strict:      true,
			},
		}, nil
	default:
		// json_object mode is the safe default; it requests valid JSON but
		// does not enforce a specific schema shape.
		return &openaiResponseFmt{Type: "json_object"}, nil
	}
}

// agentOutputJSONSchema returns a JSON Schema that describes the AgentOutput
// structure for use with OpenAI json_schema mode (strict: true).
func agentOutputJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"internal_monologue": map[string]any{
				"type":        "string",
				"description": "Agent's private reasoning (never shown to user)",
			},
			"memory_state_changes": map[string]any{
				"type":        "array",
				"description": "SQL statements that modify agent memory",
				"items":       map[string]any{"type": "string"},
			},
			"system_actions": map[string]any{
				"type":        "array",
				"description": "Session-level operations (status changes)",
				"items":       map[string]any{"type": "string"},
			},
			"tool_requests": map[string]any{
				"type":        "array",
				"description": "External tool invocations",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tool_name": map[string]any{
							"type": "string",
						},
						"parameters": map[string]any{
							"type": "object",
						},
					},
					"required":             []string{"tool_name", "parameters"},
					"additionalProperties": false,
				},
			},
			"sub_agent_spawns": map[string]any{
				"type":        "array",
				"description": "Requests to fork sub-agents",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_name":  map[string]any{"type": "string"},
						"goal":        map[string]any{"type": "string"},
						"model_id":    map[string]any{"type": "string"},
						"parent_goal": map[string]any{"type": "string"},
					},
					"required":             []string{"agent_name", "goal"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"internal_monologue", "memory_state_changes", "system_actions", "tool_requests", "sub_agent_spawns"},
		"additionalProperties": false,
	}
}

// ============================================================================
// Helpers
// ============================================================================

// isLocalProvider returns true if the base URL points to a local LLM server
// (LM Studio, Ollama) that may not support response_format on all models.
func (c *openaiClient) isLocalProvider() bool {
	return strings.Contains(c.baseURL, "127.0.0.1") ||
		strings.Contains(c.baseURL, "localhost") ||
		strings.Contains(c.baseURL, "host.docker.internal")
}
func toOpenAIMessages(messages []harness.Message) []openaiChatMessage {
	out := make([]openaiChatMessage, len(messages))
	for i, m := range messages {
		out[i] = openaiChatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return out
}

// stripMarkdownCodeBlock removes ```json / ``` wrapping from LLM output.
func stripMarkdownCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	// Strip leading ```json or ```
	if strings.HasPrefix(s, "```") {
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		} else {
			s = strings.TrimPrefix(s, "```json")
			s = strings.TrimPrefix(s, "```")
		}
	}
	// Strip trailing ```
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}

// truncateStr truncates a string to maxLen characters, appending "..." if needed.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
