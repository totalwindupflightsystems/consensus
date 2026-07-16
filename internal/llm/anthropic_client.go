// Package llm: Anthropic Messages API HTTP client.
//
// This implementation replaces the stub client with a real HTTP-based
// implementation that talks to the Anthropic Messages API directly.
// It supports prompt caching via cache_control breakpoints on static
// system prompt layers.
//
// axiom:trace work_item=WI-001-remove-mock-llm spec=specs/022-library-research.md impl=internal/llm/anthropic_client.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// Anthropic HTTP Client
// ============================================================================

// anthropicClient is a real HTTP-based Anthropic Messages API client.
// It satisfies harness.LLMClient.
type anthropicClient struct {
	cfg             *Config
	httpClient      *http.Client
	baseURL         string
	apiKey          string
	model           string
	maxTokens       int
	temperature     float64
	enableCache     bool
	maxRetries      int           // max retry attempts on transient failures
	retryBackoff    time.Duration // base backoff between retries
	fallbackBaseURL string        // LM Studio fallback URL (empty = disabled)
}

// NewAnthropicClient creates a real Anthropic Messages API client.
func NewAnthropicClient(cfg *Config) harness.LLMClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	return &anthropicClient{
		cfg:             cfg,
		httpClient:      &http.Client{Timeout: 120 * time.Second},
		baseURL:         baseURL,
		apiKey:          cfg.APIKey,
		model:           cfg.Model,
		maxTokens:       cfg.MaxTokens,
		temperature:     cfg.Temperature,
		enableCache:     cfg.EnableCache,
		maxRetries:      3,
		retryBackoff:    1 * time.Second,
		fallbackBaseURL: os.Getenv("LM_STUDIO_BASE_URL"),
	}
}

// Compile-time interface check.
var _ harness.LLMClient = (*anthropicClient)(nil)

// ============================================================================
// Anthropic Messages API Types
// ============================================================================

// anthropicMessageRequest is the request body for POST /v1/messages.
type anthropicMessageRequest struct {
	Model       string                 `json:"model"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature,omitempty"`
	System      []anthropicSystemBlock `json:"system,omitempty"`
	Messages    []anthropicMessage     `json:"messages"`
}

// anthropicSystemBlock represents a system prompt content block.
type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicMessage represents a single message in the conversation.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicCacheControl enables prompt caching on a content block.
type anthropicCacheControl struct {
	Type string `json:"type"`
}

// anthropicMessageResponse is the response from POST /v1/messages.
type anthropicMessageResponse struct {
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
	Model   string                  `json:"model"`
	Usage   anthropicUsage          `json:"usage"`
	Error   *anthropicAPIError      `json:"error,omitempty"`
}

// anthropicContentBlock is a content block in the response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// anthropicUsage holds token usage statistics from the API.
type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// anthropicAPIError represents an API-level error response.
type anthropicAPIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ============================================================================
// Call — main interface method
// ============================================================================

// Call sends messages to the Anthropic Messages API and parses the response.
// System messages (role="system") are extracted and sent in the 'system'
// parameter with optional cache_control breakpoints.
//
// On transient failures (5xx, network errors), Call retries up to maxRetries
// times with exponential backoff. After all retries are exhausted, it attempts
// an OpenAI-format fallback to LM Studio if fallbackBaseURL is configured.
func (c *anthropicClient) Call(ctx context.Context, messages []harness.Message) (*harness.LLMResponse, error) {
	startTime := time.Now()

	systemBlocks, convMessages := c.splitMessages(messages)

	if c.enableCache && len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}

	reqBody := anthropicMessageRequest{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		System:      systemBlocks,
		Messages:    convMessages,
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.retryBackoff * time.Duration(1<<uint(attempt-1))
			slog.Info("llm: retrying anthropic call",
				"attempt", attempt,
				"backoff_ms", backoff.Milliseconds(),
				"model", c.model,
			)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("llm: context cancelled during backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		msgResp, err := c.sendAnthropic(ctx, reqBody)
		if err == nil {
			return c.buildAnthropicResponse(msgResp, startTime)
		}

		if !isRetryableLLMError(err) {
			return nil, err
		}
		lastErr = err
	}

	// All retries exhausted — try LM Studio fallback (OpenAI format)
	if c.fallbackBaseURL != "" {
		slog.Info("llm: attempting LM Studio fallback from anthropic",
			"fallback_url", c.fallbackBaseURL,
			"primary_error", lastErr,
		)
		return c.anthropicFallbackToOpenAI(ctx, messages, startTime)
	}

	return nil, fmt.Errorf("llm: all anthropic retries exhausted (%d attempts): %w", c.maxRetries+1, lastErr)
}

// sendAnthropic marshals and sends the Anthropic request, returning the parsed response.
func (c *anthropicClient) sendAnthropic(ctx context.Context, reqBody anthropicMessageRequest) (*anthropicMessageResponse, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic marshal request: %w", err)
	}

	url := c.baseURL + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	slog.Info("llm: calling anthropic",
		"model", c.model,
		"system_blocks", len(reqBody.System),
		"messages", len(reqBody.Messages),
		"cache_enabled", c.enableCache,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic http request failed: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic read response: %w", err)
	}

	var msgResp anthropicMessageResponse
	if err := json.Unmarshal(respBytes, &msgResp); err != nil {
		return nil, fmt.Errorf("llm: anthropic parse response (status %d): %w", resp.StatusCode, err)
	}

	if msgResp.Error != nil {
		return nil, fmt.Errorf("llm: anthropic api error (status %d): %s (type=%s)",
			resp.StatusCode, msgResp.Error.Message, msgResp.Error.Type)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm: anthropic http %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("llm: anthropic no content in response")
	}

	return &msgResp, nil
}

// buildAnthropicResponse extracts AgentOutput from an Anthropic response.
func (c *anthropicClient) buildAnthropicResponse(msgResp *anthropicMessageResponse, startTime time.Time) (*harness.LLMResponse, error) {
	var contentText string
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			contentText = block.Text
			break
		}
	}
	if contentText == "" {
		return nil, fmt.Errorf("llm: anthropic no text content in response")
	}

	contentText = strings.TrimSpace(contentText)
	contentText = stripMarkdownCodeBlock(contentText)

	var output harness.AgentOutput
	if err := json.Unmarshal([]byte(contentText), &output); err != nil {
		return nil, fmt.Errorf("llm: anthropic parse AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(contentText, 500))
	}

	elapsed := time.Since(startTime).Milliseconds()

	slog.Info("llm: anthropic response received",
		"model", msgResp.Model,
		"elapsed_ms", elapsed,
		"input_tokens", msgResp.Usage.InputTokens,
		"output_tokens", msgResp.Usage.OutputTokens,
		"cache_creation", msgResp.Usage.CacheCreationInputTokens,
		"cache_read", msgResp.Usage.CacheReadInputTokens,
	)

	usage := harness.LLMUsage{
		PromptTokens:     msgResp.Usage.InputTokens,
		CompletionTokens: msgResp.Usage.OutputTokens,
		CacheWriteTokens: msgResp.Usage.CacheCreationInputTokens,
		CacheReadTokens:  msgResp.Usage.CacheReadInputTokens,
		TotalTokens:      msgResp.Usage.InputTokens + msgResp.Usage.OutputTokens,
	}

	return &harness.LLMResponse{
		Output:     &output,
		ModelID:    msgResp.Model,
		Usage:      usage,
		DurationMs: elapsed,
	}, nil
}

// anthropicFallbackToOpenAI converts Anthropic messages to OpenAI format and
// calls LM Studio as a fallback. LM Studio speaks OpenAI-compatible API.
func (c *anthropicClient) anthropicFallbackToOpenAI(ctx context.Context, messages []harness.Message, startTime time.Time) (*harness.LLMResponse, error) {
	openaiMsgs := toOpenAIMessages(messages)
	reqBody := openaiChatRequest{
		Model:     c.model,
		Messages:  openaiMsgs,
		MaxTokens: c.maxTokens,
		Stream:    false,
	}

	url := c.fallbackBaseURL + "/chat/completions"
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic fallback marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic fallback create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("llm: anthropic fallback failed", "error", err)
		return nil, fmt.Errorf("llm: anthropic fallback request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic fallback read response: %w", err)
	}

	var chatResp openaiChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("llm: anthropic fallback parse (status %d): %w", resp.StatusCode, err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("llm: anthropic fallback api error (status %d): %s", resp.StatusCode, chatResp.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm: anthropic fallback http %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm: anthropic fallback no choices in response")
	}

	// Use the OpenAI response builder
	oaClient := &openaiClient{}
	return oaClient.buildResponse(&chatResp, startTime)
}

// ============================================================================
// Helpers
// ============================================================================

// splitMessages separates system messages from user/assistant messages.
func (c *anthropicClient) splitMessages(messages []harness.Message) ([]anthropicSystemBlock, []anthropicMessage) {
	var systemBlocks []anthropicSystemBlock
	var convMessages []anthropicMessage

	for _, m := range messages {
		switch m.Role {
		case "system":
			systemBlocks = append(systemBlocks, anthropicSystemBlock{
				Type: "text",
				Text: m.Content,
			})
		default:
			role := m.Role
			if role != "user" && role != "assistant" {
				role = "user"
			}
			convMessages = append(convMessages, anthropicMessage{
				Role:    role,
				Content: m.Content,
			})
		}
	}

	return systemBlocks, convMessages
}
