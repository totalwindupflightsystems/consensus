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
	"os"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/harness"
)

// ============================================================================
// OpenAI / OpenRouter HTTP Client
// ============================================================================

// openaiClient is a real HTTP-based OpenAI-compatible LLM client.
// It satisfies harness.LLMClient and works with OpenAI, OpenRouter,
// and any API that implements the /v1/chat/completions contract.
type openaiClient struct {
	cfg             *Config
	httpClient      *http.Client
	baseURL         string // e.g. https://api.openai.com/v1
	apiKey          string
	model           string
	maxTokens       int
	temperature     float64
	enableCache     bool
	responseFormat  ResponseFormat
	maxRetries      int           // max retry attempts on transient failures
	retryBackoff    time.Duration // base backoff between retries
	fallbackBaseURL string        // LM Studio fallback URL (empty = disabled)

	// Thinking-mode defaults (spec 024 §B2), resolved once at construction.
	thinking          *bool
	reasoningEffort   string
	tools             []ToolDefinition
	toolChoice        string
	allowReasoningFbk bool
}

// NewOpenAIClient creates a real OpenAI-compatible HTTP client.
// The base URL defaults to OpenAI but can be overridden for OpenRouter, etc.
func NewOpenAIClient(cfg *Config) harness.LLMClient {
	// ENV-CONSENSUS-1: provider-default mapping shared with the startup key
	// probe so both always target the same host for the same config.
	baseURL := ProviderBaseURL(cfg.Provider, cfg.BaseURL)

	// Default response format
	responseFormat := cfg.ResponseFormat
	if responseFormat == "" {
		responseFormat = ResponseFormatJSONObject
	}

	return &openaiClient{
		cfg:               cfg,
		httpClient:        &http.Client{Timeout: 120 * time.Second},
		baseURL:           baseURL,
		apiKey:            cfg.APIKey,
		model:             cfg.Model,
		maxTokens:         cfg.MaxTokens,
		temperature:       cfg.Temperature,
		enableCache:       cfg.EnableCache,
		responseFormat:    responseFormat,
		maxRetries:        3,
		retryBackoff:      1 * time.Second,
		fallbackBaseURL:   os.Getenv("LM_STUDIO_BASE_URL"),
		thinking:          cfg.Thinking,
		reasoningEffort:   cfg.ReasoningEffort,
		tools:             cfg.Tools,
		toolChoice:        cfg.ToolChoice,
		allowReasoningFbk: cfg.AllowReasoningFallback == nil || *cfg.AllowReasoningFallback,
	}
}

// ============================================================================
// Thinking mode + provider-native tools (spec 024 §B2/§B3)
// ============================================================================

// ToolDefinition is a provider-native tool declared on a request.
type ToolDefinition struct {
	Type     string          `json:"type"` // "function"
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef describes one callable function.
type ToolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// RequestOptions carries the per-request surface that the LLMClient interface
// does not model: provider-native tools and the thinking-mode dial
// (spec 024 §B2). Zero values mean "use the client Config".
type RequestOptions struct {
	// Tools is the provider-native tool list for this request. A NON-EMPTY
	// list is what decides the CoT round-trip: every prior turn's
	// reasoning_content is re-sent. With no tools it is omitted entirely.
	Tools []ToolDefinition

	// ToolChoice is the provider tool_choice value
	// (none|auto|required|<function-name>). Empty leaves it unset.
	ToolChoice string

	// Thinking is the thinking-mode toggle for this request. nil defers to the
	// client Config (which may itself be "not configured").
	Thinking *bool

	// ReasoningEffort is the effort NAME (minimal|low|medium|high|xhigh|max|
	// ultra); it is mapped onto the provider dial. Empty defers to the client
	// Config.
	ReasoningEffort string
}

// effortDial maps the documented effort vocabulary onto the dial the provider
// accepts (spec 024 §B2 effort mapping).
var effortDial = map[string]string{
	"minimal": "low",
	"low":     "low",
	"medium":  "high",
	"high":    "high",
	"xhigh":   "high",
	"max":     "max",
	"ultra":   "max",
}

// MapReasoningEffort maps an effort name onto the provider dial.
// An empty name means "not configured" (returns ""); an unrecognised name is an
// error, because silently dropping the dial would hide a caller mistake
// (spec 024 §B2).
func MapReasoningEffort(effort string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(effort))
	if name == "" {
		return "", nil
	}
	dial, ok := effortDial[name]
	if !ok {
		return "", fmt.Errorf("llm: unknown reasoning effort %q (valid: minimal, low, medium, high, xhigh, max, ultra)", effort)
	}
	return dial, nil
}

// resolveOptions merges per-request options over the client's configured
// defaults. Thinking mode is never inherited silently from the provider: what
// comes back from here is exactly what goes on the wire.
func (c *openaiClient) resolveOptions(opts RequestOptions) RequestOptions {
	if opts.Thinking == nil {
		opts.Thinking = c.thinking
	}
	if strings.TrimSpace(opts.ReasoningEffort) == "" {
		opts.ReasoningEffort = c.reasoningEffort
	}
	if len(opts.Tools) == 0 {
		opts.Tools = c.tools
	}
	if opts.ToolChoice == "" {
		opts.ToolChoice = c.toolChoice
	}
	return opts
}

// thinkingPayload renders the thinking toggle for the wire, or nil when the
// mode is not configured at all (provider default applies).
func thinkingPayload(thinking *bool) *openaiThinking {
	if thinking == nil {
		return nil
	}
	if *thinking {
		return &openaiThinking{Type: "enabled"}
	}
	return &openaiThinking{Type: "disabled"}
}

// effortPayload returns the effort dial to send: an explicit thinking-off
// request carries no effort, because the dial is meaningless with thinking off
// (spec 024 §B2).
func effortPayload(dial string, thinking *bool) string {
	if thinking != nil && !*thinking {
		return ""
	}
	return dial
}

// ============================================================================
// Request / Response Types
// ============================================================================

type openaiChatRequest struct {
	Model          string              `json:"model"`
	Messages       []openaiChatMessage `json:"messages"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Temperature    float64             `json:"temperature,omitempty"`
	ResponseFormat *openaiResponseFmt  `json:"response_format,omitempty"`
	Stream         bool                `json:"stream"`

	// Provider-native tools (spec 024 §B3). Their presence on the wire is what
	// decides the CoT round-trip (§B2): with tools, every prior turn's
	// reasoning_content is re-sent; without them it is omitted.
	Tools      []openaiTool `json:"tools,omitempty"`
	ToolChoice string       `json:"tool_choice,omitempty"`

	// Thinking mode (spec 024 §B2). Sent at the top level of the body (the
	// provider's `extra_body` merges there).
	Thinking        *openaiThinking `json:"thinking,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiToolFunc `json:"function"`
}

type openaiToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openaiThinking struct {
	Type string `json:"type"` // "enabled" | "disabled"
}

type openaiChatMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type,omitempty"`
	Function openaiToolCallFunction `json:"function"`
}

type openaiToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiResponseFmt struct {
	Type       string            `json:"type"`
	JSONSchema *openaiJSONSchema `json:"json_schema,omitempty"`
}

type openaiJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}

type openaiChatResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Choices []openaiChatChoice `json:"choices"`
	Usage   openaiChatUsage    `json:"usage"`
	Error   *openaiError       `json:"error,omitempty"`
}

type openaiChatChoice struct {
	Message openaiChatMessage `json:"message"`
}

type openaiChatUsage struct {
	PromptTokens             int64                      `json:"prompt_tokens"`
	CompletionTokens         int64                      `json:"completion_tokens"`
	TotalTokens              int64                      `json:"total_tokens"`
	PromptTokensDetails      *openaiPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	CacheCreationInputTokens int64                      `json:"cache_creation_input_tokens"`
	PromptCacheMissTokens    int64                      `json:"prompt_cache_miss_tokens"`
}

type openaiPromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

func (u openaiChatUsage) cacheReadTokens() int64 {
	if u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

func (u openaiChatUsage) cacheWriteTokens() int64 {
	if u.CacheCreationInputTokens != 0 {
		return u.CacheCreationInputTokens
	}
	return u.PromptCacheMissTokens
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
//
// It uses the client's configured thinking mode, tools and effort. Use
// CallWithOptions for the per-request surface (spec 024 §B2).
//
// On transient failures (5xx status codes, network errors), Call retries up to
// maxRetries times with exponential backoff. After all retries are exhausted, it
// attempts a fallback call to LM Studio if fallbackBaseURL is configured.
func (c *openaiClient) Call(ctx context.Context, messages []harness.Message) (*harness.LLMResponse, error) {
	return c.CallWithOptions(ctx, messages, RequestOptions{})
}

// CallWithOptions is Call with the per-request surface the harness.LLMClient
// interface does not model: provider-native tools and the thinking-mode dial
// (spec 024 §B2/§B3).
//
// CoT continuity (§B2): when the request carries tools, every prior turn's
// reasoning_content is re-sent with its turn; when it does not, reasoning
// content is omitted from the wire entirely.
func (c *openaiClient) CallWithOptions(ctx context.Context, messages []harness.Message, opts RequestOptions) (*harness.LLMResponse, error) {
	startTime := time.Now()

	opts = c.resolveOptions(opts)

	// Thinking mode is resolved explicitly, and an unrecognised effort is
	// refused rather than silently dropped (spec 024 §B2).
	dial, err := MapReasoningEffort(opts.ReasoningEffort)
	if err != nil {
		return nil, err
	}

	// Build request payload. Skip response_format for local providers (LM Studio,
	// Ollama) that don't support structured output constraints on all models.
	respFmt, err := c.buildResponseFormat()
	if err != nil {
		return nil, err
	}
	if c.isLocalProvider() {
		respFmt = nil
	}
	reqBody := openaiChatRequest{
		Model:           c.model,
		Messages:        toOpenAIMessages(messages, len(opts.Tools) > 0),
		MaxTokens:       c.maxTokens,
		Temperature:     c.temperature,
		ResponseFormat:  respFmt,
		Stream:          false,
		Tools:           toOpenAITools(opts.Tools),
		ToolChoice:      opts.ToolChoice,
		Thinking:        thinkingPayload(opts.Thinking),
		ReasoningEffort: effortPayload(dial, opts.Thinking),
	}

	// Send with retry + backoff
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.retryBackoff * time.Duration(1<<uint(attempt-1))
			slog.Info("llm: retrying provider call",
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

		_, chatResp, err := c.sendAndParse(ctx, reqBody)
		if err == nil {
			return c.buildResponse(chatResp, startTime)
		}

		if !isRetryableLLMError(err) {
			return nil, err
		}
		lastErr = err
	}

	// All retries exhausted — try LM Studio fallback
	if c.fallbackBaseURL != "" {
		slog.Info("llm: attempting LM Studio fallback",
			"fallback_url", c.fallbackBaseURL,
			"primary_error", lastErr,
		)
		_, chatResp, fallbackErr := c.sendToURL(ctx, reqBody, c.fallbackBaseURL+"/chat/completions")
		if fallbackErr == nil {
			return c.buildResponse(chatResp, startTime)
		}
		slog.Warn("llm: LM Studio fallback also failed", "fallback_error", fallbackErr)
	}

	return nil, fmt.Errorf("llm: all retries exhausted (%d attempts): %w", c.maxRetries+1, lastErr)
}

// sendAndParse sends the request to the primary base URL and parses the response.
func (c *openaiClient) sendAndParse(ctx context.Context, reqBody openaiChatRequest) (*http.Response, *openaiChatResponse, error) {
	return c.sendToURL(ctx, reqBody, c.baseURL+"/chat/completions")
}

// sendToURL sends the request to a specific URL, parses the response, and checks for errors.
func (c *openaiClient) sendToURL(ctx context.Context, reqBody openaiChatRequest, url string) (*http.Response, *openaiChatResponse, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("llm: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// OpenRouter-specific headers (harmless on OpenAI)
	req.Header.Set("HTTP-Referer", "https://github.com/wojons/consensus")
	req.Header.Set("X-Title", "Consensus")

	slog.Info("llm: calling provider", "url", url, "model", c.model, "messages", len(reqBody.Messages))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: http request failed: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return resp, nil, fmt.Errorf("llm: read response: %w", err)
	}

	// Check HTTP status BEFORE attempting JSON parse (DOGFOOD-004).
	// Non-2xx responses (especially 401/403 auth errors) may have non-JSON
	// bodies (plaintext, HTML). Attempting json.Unmarshal first produces
	// cryptic errors instead of surfacing the real authentication failure.
	if resp.StatusCode >= 400 {
		// Try to extract a structured error from the body for diagnostics.
		errDetail := c.extractErrorDetail(respBytes)
		msg := errDetail.actionableError(resp.StatusCode)
		slog.Error(msg, "status", resp.StatusCode, "detail", errDetail)
		return resp, nil, fmt.Errorf("%s", msg)
	}

	var chatResp openaiChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return resp, nil, fmt.Errorf("llm: parse response (status %d): %w", resp.StatusCode, err)
	}

	if chatResp.Error != nil {
		return resp, nil, fmt.Errorf("llm: api error (status %d): %s (type=%s, code=%s)",
			resp.StatusCode, chatResp.Error.Message, chatResp.Error.Type, chatResp.Error.Code)
	}

	if len(chatResp.Choices) == 0 {
		return resp, nil, fmt.Errorf("llm: no choices in response")
	}

	return resp, &chatResp, nil
}

// buildResponse extracts the turn from a parsed chat response and builds
// LLMResponse.
//
// The whole turn is handed back — raw content, chain-of-thought and tool calls —
// so the caller can persist it and, when the conversation continues with tools,
// re-send the chain-of-thought (spec 024 §B2). A turn whose content is empty is
// still a real turn (a tool call), so the turn is returned together with the
// AgentOutput parse error instead of being discarded.
func (c *openaiClient) buildResponse(chatResp *openaiChatResponse, startTime time.Time) (*harness.LLMResponse, error) {
	msg := chatResp.Choices[0].Message
	elapsed := time.Since(startTime).Milliseconds()
	cacheReadTokens := chatResp.Usage.cacheReadTokens()
	cacheWriteTokens := chatResp.Usage.cacheWriteTokens()

	resp := &harness.LLMResponse{
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        toHarnessToolCalls(msg.ToolCalls),
		ModelID:          chatResp.Model,
		Usage: harness.LLMUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			CacheReadTokens:  cacheReadTokens,
			CacheWriteTokens: cacheWriteTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
		DurationMs: elapsed,
	}

	content := msg.Content
	promoted := false
	if strings.TrimSpace(content) == "" && msg.ReasoningContent != "" {
		// CoT is evidence of thinking, not the answer (spec 024 §B2). Promoting
		// it into the output is a deliberate, logged, last-resort substitution.
		if !c.allowReasoningFbk {
			slog.Warn("llm: content empty and reasoning_content present — CoT NOT promoted (reasoning fallback disabled)",
				"model", chatResp.Model,
				"reasoning_len", len(msg.ReasoningContent),
			)
		} else {
			content = msg.ReasoningContent
			promoted = true
			resp.ReasoningPromotedToOutput = true
			slog.Warn("llm: LAST-RESORT promotion of reasoning_content to primary output (content was empty)",
				"model", chatResp.Model,
				"reasoning_len", len(msg.ReasoningContent),
				"tool_calls", len(msg.ToolCalls),
			)
		}
	}
	content = stripMarkdownCodeBlock(strings.TrimSpace(content))

	var output harness.AgentOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		if promoted {
			return resp, fmt.Errorf("llm: promoted reasoning_content (last-resort) did not parse as AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(content, 500))
		}
		return resp, fmt.Errorf("llm: parse AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(content, 500))
	}
	resp.Output = &output

	slog.Info("llm: response received",
		"model", chatResp.Model,
		"elapsed_ms", elapsed,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"cached_tokens", cacheReadTokens,
		"reasoning_promoted", promoted,
	)

	return resp, nil
}

// isRetryableLLMError returns true if the error is transient and worth retrying.
func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Match both old format ("status 5") and new format ("HTTP 5").
	if strings.Contains(errStr, "status 5") || strings.Contains(errStr, "HTTP 5") {
		return true
	}
	if strings.Contains(errStr, "status 4") || strings.Contains(errStr, "HTTP 4") {
		return false
	}
	if strings.Contains(errStr, "http request failed") {
		return true
	}
	if strings.Contains(errStr, "context") {
		return false
	}
	return false
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
				Description: "Structured output from the Consensus agent cognition loop",
				Schema:      schemaBytes,
				Strict:      true,
			},
		}, nil
	default:
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
			"message_to_user": map[string]any{
				"type":        []string{"string", "null"},
				"description": "User-visible response; required when system_actions contains respond",
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
		"required":             []string{"internal_monologue", "memory_state_changes", "system_actions", "message_to_user", "tool_requests", "sub_agent_spawns"},
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

// toOpenAIMessages converts harness messages to the wire shape.
//
// includeReasoning implements the CoT continuity rule from spec 024 §B2: the
// per-turn reasoning_content of EVERY previous turn goes back on the wire when
// the request carries tools, and is omitted entirely when it does not (without
// tools the provider ignores it, so sending it is pure payload bloat).
func toOpenAIMessages(messages []harness.Message, includeReasoning bool) []openaiChatMessage {
	out := make([]openaiChatMessage, len(messages))
	for i, m := range messages {
		msg := openaiChatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  toOpenAIToolCalls(m.ToolCalls),
			ToolCallID: m.ToolCallID,
		}
		if includeReasoning {
			msg.ReasoningContent = m.ReasoningContent
		}
		out[i] = msg
	}
	return out
}

// toOpenAITools converts declared tools to the wire shape.
func toOpenAITools(tools []ToolDefinition) []openaiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openaiTool, len(tools))
	for i, t := range tools {
		typ := t.Type
		if typ == "" {
			typ = "function"
		}
		out[i] = openaiTool{
			Type: typ,
			Function: openaiToolFunc{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		}
	}
	return out
}

// toOpenAIToolCalls converts a turn's tool calls to the wire shape.
func toOpenAIToolCalls(calls []harness.ToolCall) []openaiToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openaiToolCall, len(calls))
	for i, c := range calls {
		out[i] = openaiToolCall{
			ID:   c.ID,
			Type: c.Type,
			Function: openaiToolCallFunction{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		}
	}
	return out
}

// toHarnessToolCalls converts response tool calls into the turn shape the
// caller persists and re-sends.
func toHarnessToolCalls(calls []openaiToolCall) []harness.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]harness.ToolCall, len(calls))
	for i, c := range calls {
		typ := c.Type
		if typ == "" {
			typ = "function"
		}
		out[i] = harness.ToolCall{
			ID:   c.ID,
			Type: typ,
			Function: harness.ToolCallFunction{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		}
	}
	return out
}

// stripMarkdownCodeBlock removes ```json / ``` wrapping from LLM output.
func stripMarkdownCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		} else {
			s = strings.TrimPrefix(s, "```json")
			s = strings.TrimPrefix(s, "```")
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// truncateStr truncates a string to maxLen characters, appending "..." if needed.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ============================================================================
// HTTP Error Detail Extraction (DOGFOOD-004)
// ============================================================================

// errorDetail captures structured error information extracted from a non-2xx
// LLM provider response body. Fields are populated on a best-effort basis;
// not all providers return JSON error bodies.
type errorDetail struct {
	Message string // human-readable error message
	Type    string // error type (e.g. "authentication_error")
	Code    string // error code (e.g. "invalid_api_key")
	Raw     string // first 200 chars of raw body for diagnostics
}

// extractErrorDetail tries to pull structured error info from a response body.
// It handles the OpenAI error shape: {"error":{"message":"...","type":"...","code":"..."}}
// and the simpler shape: {"error":"..."}.
// On any failure, Raw is populated with a truncated view of the body.
func (c *openaiClient) extractErrorDetail(body []byte) errorDetail {
	d := errorDetail{Raw: truncateStr(string(body), 200)}

	// First try the OpenAI structured error shape.
	var chatResp openaiChatResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && chatResp.Error != nil {
		d.Message = chatResp.Error.Message
		d.Type = chatResp.Error.Type
		d.Code = chatResp.Error.Code
		return d
	}

	// Then try the flat shape: {"error": "some message"}
	var flat struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Error != "" {
		d.Message = flat.Error
		return d
	}

	return d
}

// actionableError returns an actionable error message for an HTTP status code.
// On 401/403, it includes a hint to check API key configuration.
func (d errorDetail) actionableError(statusCode int) string {
	base := fmt.Sprintf("llm: HTTP %d", statusCode)

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg := fmt.Sprintf("%s — LLM auth failed: check your API key (e.g. DEEPSEEK_API_KEY or OPENAI_API_KEY env var)", base)
		if d.Message != "" {
			msg += fmt.Sprintf(" (%s)", d.Message)
		}
		return msg
	case http.StatusTooManyRequests:
		msg := fmt.Sprintf("%s — LLM rate limited", base)
		if d.Message != "" {
			msg += fmt.Sprintf(": %s", d.Message)
		}
		return msg
	case http.StatusBadRequest:
		msg := fmt.Sprintf("%s — LLM bad request", base)
		if d.Message != "" {
			msg += fmt.Sprintf(": %s", d.Message)
		}
		return msg
	default:
		msg := base
		if d.Message != "" {
			msg += fmt.Sprintf(": %s", d.Message)
		} else if d.Raw != "" {
			msg += fmt.Sprintf(": %s", d.Raw)
		}
		return msg
	}
}
