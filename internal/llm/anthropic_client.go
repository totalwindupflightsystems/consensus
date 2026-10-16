// Package llm: Anthropic Messages API HTTP client.
//
// This implementation replaces the stub client with a real HTTP-based
// implementation that talks to the Anthropic Messages API directly, including
// the DeepSeek Anthropic-format surface (base URL
// https://api.deepseek.com/anthropic, endpoint POST /anthropic/v1/messages,
// x-api-key auth — spec 024 §B1).
//
// It supports prompt caching via cache_control breakpoints on static system
// prompt layers, the documented thinking-mode toggle and effort spellings
// (§B2), provider-native tools and tool results in the Anthropic content-block
// shape (§B3), and the format's mid-conversation insertion rules (§B3).
//
// axiom:trace work_item=SHIM-SUPPORT-001 spec=specs/024-shim-support-matrix.md impl=internal/llm/anthropic_client.go
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

	// Thinking-mode + tool defaults (spec 024 §B2/§B3), resolved once at
	// construction and overridable per request. presencePenalty /
	// frequencyPenalty are the configured defaults for the two parameters the
	// provider ignores while it reasons; ResolveSamplingParams decides what
	// reaches the body.
	thinking          *bool
	reasoningEffort   string
	topP              *float64
	presencePenalty   float64
	frequencyPenalty  float64
	tools             []ToolDefinition
	toolChoice        string
	allowReasoningFbk bool
	stream            bool // default for the per-request Stream option (SHIM-SUPPORT-004)
}

// NewAnthropicClient creates a real Anthropic Messages API client.
func NewAnthropicClient(cfg *Config) harness.LLMClient {
	// ENV-CONSENSUS-1: provider-default mapping shared with the startup key
	// probe so both always target the same host for the same config.
	baseURL := ProviderBaseURL(cfg.Provider, cfg.BaseURL)

	return &anthropicClient{
		cfg:               cfg,
		httpClient:        &http.Client{Timeout: 120 * time.Second},
		baseURL:           baseURL,
		apiKey:            cfg.APIKey,
		model:             cfg.Model,
		maxTokens:         cfg.MaxTokens,
		temperature:       cfg.Temperature,
		enableCache:       cfg.EnableCache,
		maxRetries:        3,
		retryBackoff:      1 * time.Second,
		fallbackBaseURL:   os.Getenv("LM_STUDIO_BASE_URL"),
		thinking:          cfg.Thinking,
		reasoningEffort:   cfg.ReasoningEffort,
		topP:              cfg.TopP,
		presencePenalty:   cfg.PresencePenalty,
		frequencyPenalty:  cfg.FrequencyPenalty,
		tools:             cfg.Tools,
		toolChoice:        cfg.ToolChoice,
		allowReasoningFbk: cfg.AllowReasoningFallback == nil || *cfg.AllowReasoningFallback,
		stream:            cfg.Stream,
	}
}

// Compile-time interface check.
var _ harness.LLMClient = (*anthropicClient)(nil)

// ============================================================================
// Anthropic Messages API Types
// ============================================================================

// anthropicMessageRequest is the request body for POST /v1/messages.
//
// Field coverage is the documented Anthropic-format surface (spec 024 §B2/§B3):
// the thinking toggle is `reasoning.effort` (`none` disables) and the effort
// dial is `output_config.effort`; tools are declared in the Anthropic shape
// (name/description/input_schema) and tool_choice uses the Anthropic vocabulary
// (none|auto|any|tool). Stream selects the provider's SSE protocol; the client
// never sends stream:false, so an unset field means the documented buffered
// default rather than an explicit refusal of streaming (SHIM-SUPPORT-004).
type anthropicMessageRequest struct {
	Model       string   `json:"model"`
	MaxTokens   int      `json:"max_tokens"`
	Temperature float64  `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stream      bool     `json:"stream,omitempty"`

	// Accepted-but-inert while thinking mode is on (spec 024 §B2), so they are
	// present ONLY for a request that disabled thinking — ResolveSamplingParams
	// decides, exactly as on the OpenAI-format surface.
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`

	System       []anthropicSystemBlock  `json:"system,omitempty"`
	Messages     []anthropicMessage      `json:"messages"`
	Reasoning    *anthropicReasoning     `json:"reasoning,omitempty"`
	OutputConfig *anthropicOutputConfig  `json:"output_config,omitempty"`
	Tools        []anthropicTool         `json:"tools,omitempty"`
	ToolChoice   *anthropicToolChoiceSel `json:"tool_choice,omitempty"`
}

// anthropicReasoning is the Anthropic-format thinking toggle: `none` disables
// thinking mode, low/high/max select the effort (spec 024 §B2).
type anthropicReasoning struct {
	Effort string `json:"effort"`
}

// anthropicOutputConfig carries the Anthropic-format effort dial. Only `effort`
// is supported by the provider.
type anthropicOutputConfig struct {
	Effort string `json:"effort"`
}

// anthropicTool declares one provider-native tool in the Anthropic shape
// (spec 024 §B3).
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// anthropicToolChoiceSel is the Anthropic tool_choice object.
type anthropicToolChoiceSel struct {
	Type string `json:"type"`           // none | auto | any | tool
	Name string `json:"name,omitempty"` // set for type = tool
}

// anthropicSystemBlock represents a system prompt content block.
type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

// anthropicMessage represents a single message in the conversation. Content is
// a string for plain turns and a list of content blocks when the turn carries
// thinking, tool_use or tool_result parts (the provider's own union shape).
type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
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

// anthropicContentBlock is one content block in a message, request or response.
// A block carries only the fields its `type` defines:
//
//	thinking  → Thinking (+ Signature, echoed back verbatim)
//	text      → Text
//	tool_use  → ID, Name, Input
//	tool_result → ToolUseID, Content
type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

// anthropicUsage holds token usage statistics from the API, including the cache
// accounting that makes our cost numbers honest (spec 024 §B4).
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
// Endpoint resolution
// ============================================================================

// anthropicMessagesURL resolves the Messages endpoint for a base URL.
//
// The documented DeepSeek Anthropic base is https://api.deepseek.com/anthropic
// with the endpoint at /anthropic/v1/messages, while the Anthropic SDK
// convention is a base ending in /v1 with the endpoint at /v1/messages. Both are
// spelled correctly here so the same client serves api.anthropic.com and the
// DeepSeek Anthropic surface (spec 024 §B1).
func anthropicMessagesURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/messages"
	}
	return base + "/v1/messages"
}

// ============================================================================
// Call — main interface method
// ============================================================================

// Call sends messages to the Anthropic Messages API and parses the response.
// System messages are sent in the 'system' parameter with optional cache_control
// breakpoints. It uses the client's configured thinking mode, tools and effort;
// use CallWithOptions for the per-request surface (spec 024 §B2).
//
// On transient failures (5xx, network errors), Call retries up to maxRetries
// times with exponential backoff. After all retries are exhausted, it attempts
// an OpenAI-format fallback to LM Studio if fallbackBaseURL is configured.
func (c *anthropicClient) Call(ctx context.Context, messages []harness.Message) (*harness.LLMResponse, error) {
	return c.CallWithOptions(ctx, messages, RequestOptions{})
}

// CallWithOptions is Call with the per-request surface the harness.LLMClient
// interface does not model: provider-native tools and the thinking-mode dial
// (spec 024 §B2/§B3). Options resolve against the client Config exactly as the
// OpenAI-format client resolves them, so one RequestOptions value means the same
// thing on both surfaces.
// A request that sets Stream is served by the SSE path: the request carries
// "stream": true and the turn is assembled from the provider's event sequence
// (message_start → content_block_delta… → message_delta → message_stop) into
// the same turn shape Call returns, so a caller cannot tell them apart by the
// result (SHIM-SUPPORT-004).
func (c *anthropicClient) CallWithOptions(ctx context.Context, messages []harness.Message, opts RequestOptions) (*harness.LLMResponse, error) {
	startTime := time.Now()

	opts = mergeRequestOptions(RequestOptions{
		Tools:            c.tools,
		ToolChoice:       c.toolChoice,
		Thinking:         c.thinking,
		ReasoningEffort:  c.reasoningEffort,
		TopP:             c.topP,
		Temperature:      c.temperature,
		PresencePenalty:  c.presencePenalty,
		FrequencyPenalty: c.frequencyPenalty,
		Stream:           c.stream,
	}, opts)

	// The effort vocabulary is mapped, never passed through, and an
	// unrecognised name is refused rather than silently dropped (spec 024 §B2).
	dial, err := MapReasoningEffort(opts.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	if err := ValidateMaxTokens(c.maxTokens); err != nil {
		return nil, err
	}
	if err := ValidateToolDefinitions(opts.Tools); err != nil {
		return nil, err
	}

	reqBody := c.buildRequest(messages, opts, dial)

	// A streaming request is served by the SSE path before any retry loop: a
	// partially-consumed stream is not retryable the way a buffered request is
	// (the provider may already have billed the turn), so the stream errors
	// surface to the caller as-is (SHIM-SUPPORT-004).
	if opts.Stream {
		return c.streamAnthropic(ctx, reqBody, startTime)
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
			"fallback_format", "openai-chat-completions",
		)
		return c.anthropicFallbackToOpenAI(ctx, messages, opts, startTime)
	}

	return nil, fmt.Errorf("llm: all anthropic retries exhausted (%d attempts): %w", c.maxRetries+1, lastErr)
}

// buildRequest renders the Anthropic request body for a call, including the
// documented thinking toggle/effort spellings and the tool fields (§B2/§B3).
func (c *anthropicClient) buildRequest(messages []harness.Message, opts RequestOptions, dial string) anthropicMessageRequest {
	systemBlocks, convMessages := c.buildMessages(messages, opts)

	if c.enableCache && len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}

	// top_p carries the same documented semantics on this surface as on the
	// OpenAI-format one: thinking mode only, floored at 0.95, ignored in
	// non-thinking mode (spec 024 §B2).
	var topP *float64
	if opts.TopP != nil {
		if v, send := ResolveTopP(*opts.TopP, opts.Thinking); send {
			topP = &v
		}
	}

	// The same thinking-mode sampling contract as the OpenAI-format surface: the
	// parameters the provider ignores are dropped from the body and named at WARN
	// (spec 024 §B2 — the semantics are per thinking mode, not per format).
	sampling := ResolveSamplingParams(SamplingParams{
		Temperature:      opts.Temperature,
		PresencePenalty:  opts.PresencePenalty,
		FrequencyPenalty: opts.FrequencyPenalty,
	}, opts.Thinking)
	logDroppedSamplingParams(sampling.Dropped, c.model, "anthropic-messages")

	return anthropicMessageRequest{
		Model:            c.model,
		MaxTokens:        c.maxTokens,
		Temperature:      wireFloat(sampling.Temperature),
		TopP:             topP,
		PresencePenalty:  sampling.PresencePenalty,
		FrequencyPenalty: sampling.FrequencyPenalty,
		System:           systemBlocks,
		Messages:         convMessages,
		Reasoning:        anthropicReasoningPayload(dial, opts.Thinking),
		OutputConfig:     anthropicOutputConfigPayload(dial, opts.Thinking),
		Tools:            toAnthropicTools(opts.Tools),
		ToolChoice:       toAnthropicToolChoice(opts.ToolChoice),
	}
}

// sendAnthropic marshals and sends the Anthropic request, returning the parsed response.
func (c *anthropicClient) sendAnthropic(ctx context.Context, reqBody anthropicMessageRequest) (*anthropicMessageResponse, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic marshal request: %w", err)
	}

	url := anthropicMessagesURL(c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	// Sent for compatibility; the DeepSeek Anthropic surface documents
	// anthropic-version as ignored (spec 024 §B1).
	req.Header.Set("anthropic-version", "2023-06-01")

	slog.Info("llm: calling anthropic",
		"model", c.model,
		"url", url,
		"system_blocks", len(reqBody.System),
		"messages", len(reqBody.Messages),
		"tools", len(reqBody.Tools),
		"cache_enabled", c.enableCache,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic http request failed: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic read response: %w", err)
	}

	// Check HTTP status BEFORE attempting JSON parse (DOGFOOD-004).
	if resp.StatusCode >= 400 {
		ed := extractAnthropicErrorDetail(respBytes)
		msg := ed.actionableError(resp.StatusCode)
		slog.Error(msg, "status", resp.StatusCode, "detail", ed)
		return nil, fmt.Errorf("%s", msg)
	}

	var msgResp anthropicMessageResponse
	if err := json.Unmarshal(respBytes, &msgResp); err != nil {
		return nil, fmt.Errorf("llm: anthropic parse response (status %d): %w", resp.StatusCode, err)
	}

	if msgResp.Error != nil {
		return nil, fmt.Errorf("llm: anthropic api error (status %d): %s (type=%s)",
			resp.StatusCode, msgResp.Error.Message, msgResp.Error.Type)
	}

	if len(msgResp.Content) == 0 {
		return nil, fmt.Errorf("llm: anthropic no content in response")
	}

	return &msgResp, nil
}

// buildAnthropicResponse extracts the turn from an Anthropic response.
//
// The response's content is a block list, and the thinking block is part of the
// turn, not an alternative to it: a thinking-mode turn routinely arrives with a
// `thinking` block and, when the token budget runs out mid-reasoning, with NO
// text block at all (measured live 2026-10-13 — see
// testdata/shim-support-exchanges.json, anthropic.messages_minimal_thinking_default).
// The whole turn is therefore handed back — text, thinking (+ its signature) and
// tool calls — mirroring the OpenAI-format client, and a promotion of
// chain-of-thought into the answer stays a deliberate, logged, last resort
// (spec 024 §B2) instead of an error that loses the turn.
func (c *anthropicClient) buildAnthropicResponse(msgResp *anthropicMessageResponse, startTime time.Time) (*harness.LLMResponse, error) {
	var (
		text      string
		thinking  string
		signature string
		toolCalls []harness.ToolCall
	)
	for _, block := range msgResp.Content {
		switch block.Type {
		case "thinking":
			thinking += block.Thinking
			if block.Signature != "" {
				signature = block.Signature
			}
		case "text":
			if text == "" {
				text = block.Text
			}
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			toolCalls = append(toolCalls, harness.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: harness.ToolCallFunction{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	if text == "" && thinking == "" && len(toolCalls) == 0 {
		return nil, fmt.Errorf("llm: anthropic response carried no text, thinking or tool_use block")
	}

	elapsed := time.Since(startTime).Milliseconds()

	slog.Info("llm: anthropic response received",
		"model", msgResp.Model,
		"elapsed_ms", elapsed,
		"input_tokens", msgResp.Usage.InputTokens,
		"output_tokens", msgResp.Usage.OutputTokens,
		"cache_creation", msgResp.Usage.CacheCreationInputTokens,
		"cache_read", msgResp.Usage.CacheReadInputTokens,
		"tool_calls", len(toolCalls),
	)

	usage := harness.LLMUsage{
		PromptTokens:     msgResp.Usage.InputTokens,
		CompletionTokens: msgResp.Usage.OutputTokens,
		CacheWriteTokens: msgResp.Usage.CacheCreationInputTokens,
		CacheReadTokens:  msgResp.Usage.CacheReadInputTokens,
		TotalTokens:      msgResp.Usage.InputTokens + msgResp.Usage.OutputTokens,
	}

	resp := &harness.LLMResponse{
		Content:            text,
		ReasoningContent:   thinking,
		ReasoningSignature: signature,
		ToolCalls:          toolCalls,
		ModelID:            msgResp.Model,
		Usage:              usage,
		DurationMs:         elapsed,
	}

	content := text
	promoted := false
	if strings.TrimSpace(content) == "" && thinking != "" && len(toolCalls) == 0 {
		// CoT is evidence of thinking, not the answer (spec 024 §B2). Same
		// deliberate, logged, last-resort substitution as the OpenAI-format
		// client — the Anthropic surface simply has no `content` field to fall
		// back to, so the substitution is the only way the turn survives.
		if !c.allowReasoningFbk {
			slog.Warn("llm: anthropic content empty and thinking present — CoT NOT promoted (reasoning fallback disabled)",
				"model", msgResp.Model,
				"reasoning_len", len(thinking),
			)
		} else {
			content = thinking
			promoted = true
			resp.ReasoningPromotedToOutput = true
			slog.Warn("llm: LAST-RESORT promotion of thinking block to primary output (no text block in response)",
				"model", msgResp.Model,
				"reasoning_len", len(thinking),
			)
		}
	}

	if content == "" {
		// A pure tool_use turn: real, but it carries no AgentOutput to parse.
		return resp, nil
	}

	rawContent := strings.TrimSpace(content)
	resp.Content = rawContent
	parseTarget := stripMarkdownCodeBlock(rawContent)

	var output harness.AgentOutput
	if err := json.Unmarshal([]byte(parseTarget), &output); err != nil {
		if promoted {
			return resp, fmt.Errorf("llm: promoted reasoning content (last-resort) did not parse as AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(parseTarget, 500))
		}
		return resp, fmt.Errorf("llm: anthropic parse AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(parseTarget, 500))
	}
	resp.Output = &output
	return resp, nil
}

// anthropicFallbackToOpenAI converts Anthropic messages to OpenAI format and
// calls LM Studio as a fallback. LM Studio speaks the OpenAI-compatible API, so
// the OpenAI-format client builds and interprets the exchange; tools and the
// thinking dial are passed through rather than silently dropped.
func (c *anthropicClient) anthropicFallbackToOpenAI(ctx context.Context, messages []harness.Message, opts RequestOptions, startTime time.Time) (*harness.LLMResponse, error) {
	dial, err := MapReasoningEffort(opts.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	// The sampling contract follows the thinking mode on THIS path too: the
	// fallback speaks the OpenAI format, but the rule is per thinking mode, so
	// the ignored parameters stay out of the fallback body as well.
	sampling := ResolveSamplingParams(SamplingParams{
		Temperature:      opts.Temperature,
		PresencePenalty:  opts.PresencePenalty,
		FrequencyPenalty: opts.FrequencyPenalty,
	}, opts.Thinking)
	reqBody := openaiChatRequest{
		Model:            c.model,
		Messages:         toOpenAIMessages(messages, len(opts.Tools) > 0),
		MaxTokens:        c.maxTokens,
		Temperature:      wireFloat(sampling.Temperature),
		TopP:             topPPayload(opts.TopP, opts.Thinking),
		PresencePenalty:  sampling.PresencePenalty,
		FrequencyPenalty: sampling.FrequencyPenalty,
		Stream:           false,
		Tools:            toOpenAITools(opts.Tools),
		ToolChoice:       opts.ToolChoice,
		Thinking:         thinkingPayload(opts.Thinking),
		ReasoningEffort:  effortPayload(dial, opts.Thinking),
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
	defer func() { _ = resp.Body.Close() }()

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
// Message assembly (spec 024 §B1/§B2/§B3)
// ============================================================================

// buildMessages renders the conversation into the Anthropic shape.
//
// Insertion rules (§B3): only the LEADING run of system messages is hoisted into
// the `system` parameter, because the provider supports system messages
// mid-conversation — hoisting those too would move them out of the position the
// caller chose. A role:"tool" result becomes a `tool_result` block inside a user
// turn, and an assistant turn that carried tool calls is echoed back with its
// `tool_use` blocks.
//
// Thinking round-trip (§B2): a turn's thinking block is echoed back verbatim
// (text + signature) whenever the turn has one. The Anthropic surface hard-fails
// with HTTP 400 when a prior assistant turn that carries tool_use is missing its
// thinking block (measured 2026-10-13), and accepts the echo whether or not the
// request carries tools (also measured) — so the presence of a block decides it,
// never a guess about the request.
func (c *anthropicClient) buildMessages(messages []harness.Message, opts RequestOptions) ([]anthropicSystemBlock, []anthropicMessage) {
	var systemBlocks []anthropicSystemBlock
	var convMessages []anthropicMessage
	leadingSystem := true

	for _, m := range messages {
		if m.Role == "system" && leadingSystem {
			systemBlocks = append(systemBlocks, anthropicSystemBlock{
				Type: "text",
				Text: m.Content,
			})
			continue
		}
		leadingSystem = false

		switch m.Role {
		case "tool":
			convMessages = append(convMessages, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		case "assistant":
			blocks := make([]anthropicContentBlock, 0, 3)
			if m.ReasoningContent != "" {
				blocks = append(blocks, anthropicContentBlock{
					Type:      "thinking",
					Thinking:  m.ReasoningContent,
					Signature: m.ReasoningSignature,
				})
			}
			if m.Content != "" || len(blocks) == 0 && len(m.ToolCalls) == 0 {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(strings.TrimSpace(tc.Function.Arguments))
				if len(input) == 0 || !json.Valid(input) {
					// The provider requires a JSON object; an unparsable
					// argument string is sent as an empty object rather than
					// making the whole request malformed.
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			if len(blocks) == 1 && blocks[0].Type == "text" {
				// Plain assistant turn: keep the string form the provider
				// documents as fully supported.
				convMessages = append(convMessages, anthropicMessage{Role: "assistant", Content: blocks[0].Text})
				continue
			}
			convMessages = append(convMessages, anthropicMessage{Role: "assistant", Content: blocks})
		case "system":
			// Mid-conversation system message: the provider supports it
			// (§B3), so it keeps the role and the position the caller chose.
			convMessages = append(convMessages, anthropicMessage{Role: "system", Content: m.Content})
		default:
			role := m.Role
			if role != "user" {
				role = "user"
			}
			convMessages = append(convMessages, anthropicMessage{Role: role, Content: m.Content})
		}
	}

	return systemBlocks, convMessages
}

// anthropicReasoningPayload renders the Anthropic-format thinking toggle
// (spec 024 §B2). `none` is the documented off spelling; an enabled mode always
// names an effort, defaulting to the documented `high`.
//
// Documented-vs-live note: the provider still returns thinking blocks for
// `effort: "none"` in live traffic (measured 2026-10-13, exchange
// anthropic.two_turn_effort_none_still_thinks), so this function produces the
// documented request and the divergence is tracked by the matrix, not hidden.
func anthropicReasoningPayload(dial string, thinking *bool) *anthropicReasoning {
	if thinking == nil && dial == "" {
		return nil // not configured: the provider default (enabled, high) applies
	}
	if thinking != nil && !*thinking {
		return &anthropicReasoning{Effort: "none"}
	}
	if dial == "" {
		dial = defaultReasoningEffort
	}
	return &anthropicReasoning{Effort: dial}
}

// anthropicOutputConfigPayload renders the Anthropic-format effort dial. Like
// every other surface, the dial is meaningless with thinking off, so it is
// omitted there rather than sent as decoration (spec 024 §B2).
func anthropicOutputConfigPayload(dial string, thinking *bool) *anthropicOutputConfig {
	if thinking != nil && !*thinking {
		return nil
	}
	if dial == "" {
		return nil
	}
	return &anthropicOutputConfig{Effort: dial}
}

// toAnthropicTools converts declared tools into the Anthropic shape
// (name/description/input_schema — spec 024 §B3).
func toAnthropicTools(tools []ToolDefinition) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, len(tools))
	for i, t := range tools {
		out[i] = anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		}
	}
	return out
}

// toAnthropicToolChoice maps the neutral tool_choice vocabulary onto the
// Anthropic one: none/auto pass through, `required` becomes `any` (the
// Anthropic spelling for "must call a tool"), and any other non-empty value is
// treated as a named tool (spec 024 §B3).
func toAnthropicToolChoice(choice string) *anthropicToolChoiceSel {
	switch strings.TrimSpace(choice) {
	case "":
		return nil
	case "none":
		return &anthropicToolChoiceSel{Type: "none"}
	case "auto":
		return &anthropicToolChoiceSel{Type: "auto"}
	case "required":
		return &anthropicToolChoiceSel{Type: "any"}
	default:
		return &anthropicToolChoiceSel{Type: "tool", Name: strings.TrimSpace(choice)}
	}
}

// ============================================================================
// Helpers
// ============================================================================

// extractAnthropicErrorDetail tries to extract structured error info from an
// Anthropic error response body. Anthropic errors follow the shape:
// {"type":"error","error":{"type":"authentication_error","message":"..."}}
// Falls back to the generic OpenAI shape if Anthropic format isn't detected.
func extractAnthropicErrorDetail(body []byte) errorDetail {
	d := errorDetail{Raw: truncateStr(string(body), 200)}

	// Try Anthropic error shape: {"type":"error","error":{...}}
	var anthroErr struct {
		Type  string      `json:"type"`
		Error openaiError `json:"error"`
	}
	if err := json.Unmarshal(body, &anthroErr); err == nil && anthroErr.Error.Message != "" {
		d.Message = anthroErr.Error.Message
		d.Type = anthroErr.Error.Type
		d.Code = "" // Anthropic doesn't use codes
		return d
	}

	// Fall back to the OpenAI shape (shared errorDetail extraction).
	var chatResp openaiChatResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && chatResp.Error != nil {
		d.Message = chatResp.Error.Message
		d.Type = chatResp.Error.Type
		d.Code = chatResp.Error.Code
		return d
	}

	// Flat shape: {"error": "some message"}
	var flat struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.Error != "" {
		d.Message = flat.Error
		return d
	}

	return d
}

// ============================================================================
// Streaming (SSE) — Anthropic Messages API
// ============================================================================

// anthropicStreamEventNames are the stream event types the Anthropic Messages
// protocol documents for a successful turn (https://docs.anthropic.com/en/api/messages-streaming,
// provider docs read 2026-10-13). Each name maps to the payload struct that
// decodes its `data:` frame; the client only requires the four that carry turn
// state (message_start, content_block_delta, message_delta, message_stop) and
// treats the remaining documented names as structural (no-op) events, so a
// provider that adds framing events keeps working.
//
//	ax:trace work_item=SHIM-SUPPORT-004 spec=specs/024-shim-support-matrix.md
const (
	anthropicEvtMessageStart     = "message_start"
	anthropicEvtContentBlock     = "content_block_start"
	anthropicEvtContentBlockDel  = "content_block_delta"
	anthropicEvtContentBlockStop = "content_block_stop"
	anthropicEvtMessageDelta     = "message_delta"
	anthropicEvtMessageStop      = "message_stop"
	anthropicEvtPing             = "ping"
)

// anthropicStreamErrorNames are the stream event types that report a mid-stream
// provider failure. Either carries an error object the client surfaces as a
// failed call.
const (
	anthropicEvtError = "error"
	anthropicEvtAbort = "aborted"
)

// anthropicStreamEnvelope is one SSE `event:`/`data:` pair on the wire. The
// Anthropic protocol documents both fields; a data frame without an event name
// is malformed for this endpoint and is refused with the payload quoted.
type anthropicStreamEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// anthropicStreamDelta is the delta payload of a content_block_delta frame.
// Exactly one text-bearing field is set per frame; the deltas accumulate into
// the content blocks started by message_start / content_block_start.
type anthropicStreamDelta struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Input     string `json:"partial_json,omitempty"`
}

// anthropicStreamBlock mirrors anthropicContentBlock for the content_block_start
// frame. It reuses the response type's fields; a separate type keeps the wire
// shape exact (the stream only carries a block header: type, index and the
// fields of the block itself).
type anthropicStreamBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// anthropicStreamErrorFrame is the payload of a stream `error` frame: the same
// error envelope the buffered endpoint returns on a failure status.
type anthropicStreamErrorFrame struct {
	Type  string                     `json:"type"`
	Error *anthropicStreamErrorShape `json:"error,omitempty"`
}

// anthropicStreamErrorShape is the inner error object of an error frame.
type anthropicStreamErrorShape struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// anthropicStreamAssembler accumulates an Anthropic SSE stream into the same
// turn the non-streaming Call returns. It is deliberately provider-shape-local:
// the final assembly is delegated to buildAnthropicResponse, so a streamed turn
// and a buffered turn are identical by construction, never by parallel logic.
type anthropicStreamAssembler struct {
	msg       anthropicMessageResponse
	blocks    map[int]*anthropicContentBlock
	startTime time.Time
	started   bool
	stopped   bool
}

func newAnthropicStreamAssembler(startTime time.Time) *anthropicStreamAssembler {
	return &anthropicStreamAssembler{blocks: map[int]*anthropicContentBlock{}, startTime: startTime}
}

// addDelta applies one content_block_delta frame to the block it names.
// Unknown delta types fail loudly: a silently dropped delta would produce a
// turn that looks complete and is not.
func (a *anthropicStreamAssembler) addDelta(index int, d anthropicStreamDelta) error {
	blk, ok := a.blocks[index]
	if !ok {
		return fmt.Errorf("llm: anthropic stream delta for block %d before its content_block_start", index)
	}
	switch d.Type {
	case "text_delta":
		blk.Text += d.Text
	case "input_json_delta":
		// The provider opens a tool_use block with input {} and streams the
		// object as partial_json fragments. The opener's {} is a placeholder:
		// the FIRST fragment replaces it, later fragments concatenate onto the
		// accumulated bytes (the fragments together ARE the object).
		if string(blk.Input) == "{}" {
			blk.Input = json.RawMessage(d.Input)
		} else {
			blk.Input = append(blk.Input, d.Input...)
		}
	case "thinking_delta":
		blk.Thinking += d.Thinking
	case "signature_delta":
		blk.Signature += d.Signature
	default:
		return fmt.Errorf("llm: anthropic stream carried an unsupported delta type %q", d.Type)
	}
	return nil
}

// finalize collapses the accumulated blocks into the provider response shape
// and builds the harness turn through the same path Call uses.
func (a *anthropicStreamAssembler) finalize(c *anthropicClient) (*harness.LLMResponse, error) {
	content := make([]anthropicContentBlock, 0, len(a.blocks))
	for i := 0; i < len(a.blocks); i++ {
		content = append(content, *a.blocks[i])
	}
	a.msg.Content = content
	a.msg.Type = "message"
	a.msg.Role = "assistant"
	return c.buildAnthropicResponse(&a.msg, a.startTime)
}

// parseAnthropicSSEFrames splits an SSE byte stream into event/data envelopes.
// It tolerates CRLF and LF separators and strips one leading space per SSE
// field (the format's documented parsing rule). A frame without a `data:` field
// is malformed and is refused with the offending frame quoted, because a
// silently skipped frame would leave the assembled turn missing whatever the
// frame carried.
func parseAnthropicSSEFrames(body []byte) ([]anthropicStreamEnvelope, error) {
	var frames []anthropicStreamEnvelope
	var event string
	var data []string
	inFrame := false
	flush := func() error {
		if !inFrame {
			return nil
		}
		if event == "" {
			return fmt.Errorf("llm: anthropic SSE frame without an event name (data=%q)",
				truncateStr(strings.Join(data, "\n"), 200))
		}
		if len(data) == 0 {
			return fmt.Errorf("llm: anthropic SSE frame %q carries no data", event)
		}
		frames = append(frames, anthropicStreamEnvelope{
			Event: event,
			Data:  json.RawMessage(strings.Join(data, "\n")),
		})
		event = ""
		data = nil
		inFrame = false
		return nil
	}
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		switch {
		case line == "":
			if err := flush(); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive: ignored.
		case strings.HasPrefix(line, "event:"):
			inFrame = true
			event = strings.TrimPrefix(strings.TrimPrefix(line, "event:"), " ")
		case strings.HasPrefix(line, "data:"):
			inFrame = true
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		default:
			return nil, fmt.Errorf("llm: anthropic SSE frame carries an unrecognized line %q", truncateStr(line, 120))
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return frames, nil
}

// decodeAnthropicStreamEvent decodes one frame's data payload for the event
// types that carry turn state. The frame struct is the wire truth: `type` names
// the event, `index` the block a delta applies to, `message`/`delta`/`block`
// the payload objects the documented protocol defines.
type anthropicStreamFrame struct {
	Type    string                    `json:"type"`
	Index   int                       `json:"index"`
	Delta   *anthropicStreamDelta     `json:"delta,omitempty"`
	Message *anthropicMessageResponse `json:"message,omitempty"`
	Block   *anthropicStreamBlock     `json:"content_block,omitempty"`
	Usage   *anthropicUsage           `json:"usage,omitempty"`
}

// streamAnthropic consumes an SSE body and assembles the turn.
func (c *anthropicClient) streamAnthropic(ctx context.Context, reqBody anthropicMessageRequest, startTime time.Time) (*harness.LLMResponse, error) {
	// The request reaching this path resolved opts.Stream already (the dispatch
	// in CallWithOptions); the flag goes on the wire here so the marker lives
	// with the body builder, not the caller.
	reqBody.Stream = true
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic stream marshal request: %w", err)
	}

	url := anthropicMessagesURL(c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic stream create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	slog.Info("llm: calling anthropic (streaming)",
		"model", c.model,
		"url", url,
		"system_blocks", len(reqBody.System),
		"messages", len(reqBody.Messages),
		"tools", len(reqBody.Tools),
		"cache_enabled", c.enableCache,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic stream http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("llm: anthropic stream read error response: %w", err)
		}
		ed := extractAnthropicErrorDetail(respBytes)
		msg := ed.actionableError(resp.StatusCode)
		slog.Error(msg, "status", resp.StatusCode, "detail", ed)
		return nil, fmt.Errorf("%s", msg)
	}

	assembler := newAnthropicStreamAssembler(startTime)
	// The body is consumed as bytes and framed by parseAnthropicSSEFrames: SSE
	// event/data framing is line-based text, so a JSON decoder cannot read it,
	// and one strict parser is the single place a malformed frame is refused.
	// The stream is already fully produced server-side once the client sees the
	// bytes (the endpoint returns a complete transcript, it does not pace one),
	// so this consumes no incremental behavior a chunked reader would expose.
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic stream read body: %w", err)
	}
	frames, err := parseAnthropicSSEFrames(raw)
	if err != nil {
		return nil, err
	}
	for _, env := range frames {
		if len(env.Data) == 0 {
			return nil, fmt.Errorf("llm: anthropic stream frame %q carries no data", env.Event)
		}

		switch env.Event {
		case anthropicEvtMessageStart:
			var frame anthropicStreamFrame
			if err := json.Unmarshal(env.Data, &frame); err != nil {
				return nil, fmt.Errorf("llm: anthropic stream parse %s: %w", env.Event, err)
			}
			if frame.Message == nil {
				return nil, fmt.Errorf("llm: anthropic stream %s frame carried no message", env.Event)
			}
			assembler.msg = *frame.Message
			assembler.started = true
		case anthropicEvtContentBlock:
			var frame anthropicStreamFrame
			if err := json.Unmarshal(env.Data, &frame); err != nil {
				return nil, fmt.Errorf("llm: anthropic stream parse %s: %w", env.Event, err)
			}
			if frame.Block == nil {
				return nil, fmt.Errorf("llm: anthropic stream %s frame carried no content_block", env.Event)
			}
			blk := &anthropicContentBlock{
				Type:      frame.Block.Type,
				Text:      frame.Block.Text,
				Thinking:  frame.Block.Thinking,
				Signature: frame.Block.Signature,
				ID:        frame.Block.ID,
				Name:      frame.Block.Name,
				Input:     frame.Block.Input,
			}
			assembler.blocks[frame.Index] = blk
		case anthropicEvtContentBlockDel:
			var frame anthropicStreamFrame
			if err := json.Unmarshal(env.Data, &frame); err != nil {
				return nil, fmt.Errorf("llm: anthropic stream parse %s: %w", env.Event, err)
			}
			if frame.Delta == nil {
				return nil, fmt.Errorf("llm: anthropic stream %s frame carried no delta", env.Event)
			}
			if err := assembler.addDelta(frame.Index, *frame.Delta); err != nil {
				return nil, err
			}
		case anthropicEvtMessageDelta:
			var frame anthropicStreamFrame
			if err := json.Unmarshal(env.Data, &frame); err != nil {
				return nil, fmt.Errorf("llm: anthropic stream parse %s: %w", env.Event, err)
			}
			if frame.Usage != nil {
				if frame.Usage.InputTokens != 0 {
					assembler.msg.Usage.InputTokens = frame.Usage.InputTokens
				}
				if frame.Usage.OutputTokens != 0 {
					assembler.msg.Usage.OutputTokens = frame.Usage.OutputTokens
				}
				if frame.Usage.CacheCreationInputTokens != 0 {
					assembler.msg.Usage.CacheCreationInputTokens = frame.Usage.CacheCreationInputTokens
				}
				if frame.Usage.CacheReadInputTokens != 0 {
					assembler.msg.Usage.CacheReadInputTokens = frame.Usage.CacheReadInputTokens
				}
			}
		case anthropicEvtMessageStop:
			assembler.stopped = true
		case anthropicEvtPing, anthropicEvtContentBlockStop:
			// Structural events: no turn state.
		case anthropicEvtError, anthropicEvtAbort:
			// The error envelope is the API error shape: a top-level object
			// whose `error` field carries {type, message}.
			var e anthropicStreamErrorFrame
			if err := json.Unmarshal(env.Data, &e); err != nil {
				return nil, fmt.Errorf("llm: anthropic stream %s frame: %w", env.Event, err)
			}
			if e.Error == nil || e.Error.Message == "" {
				return nil, fmt.Errorf("llm: anthropic stream %s frame carried no error detail: %s", env.Event, truncateStr(string(env.Data), 200))
			}
			return nil, fmt.Errorf("llm: anthropic stream error (%s): %s", e.Error.Type, e.Error.Message)
		default:
			return nil, fmt.Errorf("llm: anthropic stream carried an unknown event type %q", env.Event)
		}
	}

	if !assembler.started {
		return nil, fmt.Errorf("llm: anthropic stream ended without a message_start frame")
	}
	if !assembler.stopped {
		return nil, fmt.Errorf("llm: anthropic stream ended before message_stop")
	}
	return assembler.finalize(c)
}
