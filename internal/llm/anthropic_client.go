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
	// construction and overridable per request.
	thinking          *bool
	reasoningEffort   string
	topP              *float64
	tools             []ToolDefinition
	toolChoice        string
	allowReasoningFbk bool
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
		tools:             cfg.Tools,
		toolChoice:        cfg.ToolChoice,
		allowReasoningFbk: cfg.AllowReasoningFallback == nil || *cfg.AllowReasoningFallback,
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
// (none|auto|any|tool).
type anthropicMessageRequest struct {
	Model        string                  `json:"model"`
	MaxTokens    int                     `json:"max_tokens"`
	Temperature  float64                 `json:"temperature,omitempty"`
	TopP         *float64                `json:"top_p,omitempty"`
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
func (c *anthropicClient) CallWithOptions(ctx context.Context, messages []harness.Message, opts RequestOptions) (*harness.LLMResponse, error) {
	startTime := time.Now()

	opts = mergeRequestOptions(RequestOptions{
		Tools:           c.tools,
		ToolChoice:      c.toolChoice,
		Thinking:        c.thinking,
		ReasoningEffort: c.reasoningEffort,
		TopP:            c.topP,
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

	if inert := InertThinkingParams(opts.Thinking); len(inert) > 0 && c.temperature != 0 {
		slog.Debug("llm: parameters accepted but inert in thinking mode",
			"params", inert, "model", c.model, "format", "anthropic-messages")
	}

	return anthropicMessageRequest{
		Model:        c.model,
		MaxTokens:    c.maxTokens,
		Temperature:  c.temperature,
		TopP:         topP,
		System:       systemBlocks,
		Messages:     convMessages,
		Reasoning:    anthropicReasoningPayload(dial, opts.Thinking),
		OutputConfig: anthropicOutputConfigPayload(dial, opts.Thinking),
		Tools:        toAnthropicTools(opts.Tools),
		ToolChoice:   toAnthropicToolChoice(opts.ToolChoice),
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
	reqBody := openaiChatRequest{
		Model:           c.model,
		Messages:        toOpenAIMessages(messages, len(opts.Tools) > 0),
		MaxTokens:       c.maxTokens,
		Temperature:     c.temperature,
		TopP:            topPPayload(opts.TopP, opts.Thinking),
		Stream:          false,
		Tools:           toOpenAITools(opts.Tools),
		ToolChoice:      opts.ToolChoice,
		Thinking:        thinkingPayload(opts.Thinking),
		ReasoningEffort: effortPayload(dial, opts.Thinking),
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
