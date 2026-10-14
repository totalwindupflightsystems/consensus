// Package llm: DeepSeek Responses API format (spec 024 §B1).
//
// The provider serves a third request format — the Responses API, at the same
// base URL as Chat Completions (POST /responses) — and the harness must speak
// its structure rather than ignore it. This file holds the pure wire layer: the
// request builder and the response interpreter. It is deliberately free of
// transport so the shape is testable against recorded exchanges; wiring a
// provider that selects it is a separate, tracked step (see the conformance
// matrix cell B1-RESPONSES-WIRING in specs/024 §D).
//
// Measured on the live surface 2026-10-13 (testdata/shim-support-exchanges.json):
//   - `reasoning.effort` is the honoured effort spelling; the response echoes it
//     back under `reasoning`.
//   - The `output_config` spelling that spec 024 §B2 listed for this format is
//     silently ignored here (no echo, no error), so this builder sends
//     `reasoning.effort` and records the divergence instead of miming an inert
//     parameter.
//   - Unsupported parameters are silently ignored by the provider, which is
//     exactly why the builder emits only fields it means.
//
// axiom:trace work_item=SHIM-SUPPORT-001 spec=specs/024-shim-support-matrix.md impl=internal/llm/responses_format.go
package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wojons/consensus/internal/harness"
)

// ResponsesPath is the endpoint path of the Responses API, relative to the
// provider base URL (spec 024 §B1: base https://api.deepseek.com).
const ResponsesPath = "/responses"

// ResponsesEndpoint returns the absolute Responses endpoint for a base URL.
func ResponsesEndpoint(base string) string {
	return strings.TrimRight(base, "/") + ResponsesPath
}

// ResponsesRequest is the Responses request body.
type ResponsesRequest struct {
	Model           string                 `json:"model"`
	Input           []ResponsesInputItem   `json:"input"`
	Instructions    string                 `json:"instructions,omitempty"`
	Reasoning       *ResponsesReasoning    `json:"reasoning,omitempty"`
	Tools           []ResponsesTool        `json:"tools,omitempty"`
	ToolChoice      any                    `json:"tool_choice,omitempty"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Temperature     float64                `json:"temperature,omitempty"`
	TopP            *float64               `json:"top_p,omitempty"`
	Stream          bool                   `json:"stream"`
	Metadata        map[string]any         `json:"metadata,omitempty"`
	Text            *ResponsesTextControls `json:"text,omitempty"`
}

// ResponsesReasoning carries the effort dial for this format (`summary` is
// accepted by the provider but no summary is generated, so it is not sent).
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// ResponsesTextControls carries the output-format controls. `format` is fully
// supported by the provider; `verbosity` has no effect and is not modelled.
type ResponsesTextControls struct {
	Format *openaiResponseFmt `json:"format,omitempty"`
}

// ResponsesTool declares a tool in the Responses shape (flat, unlike the Chat
// Completions shape that nests under `function`).
type ResponsesTool struct {
	Type        string          `json:"type"` // "function"
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

// ResponsesInputItem is one input item: a message (role + content parts), a
// function_call replayed from a prior turn, or a function_call_output carrying a
// tool result.
type ResponsesInputItem struct {
	Type      string                 `json:"type,omitempty"` // function_call | function_call_output
	Role      string                 `json:"role,omitempty"`
	Content   []ResponsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Output    string                 `json:"output,omitempty"`
}

// ResponsesContentPart is an input_text / output_text / reasoning_text part.
type ResponsesContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ResponsesResponse is the provider's response object (OpenAI-Responses
// compatible; fields depending on unsupported capabilities take fixed values).
type ResponsesResponse struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"`
	Model   string                `json:"model"`
	Status  string                `json:"status"`
	Output  []ResponsesOutputItem `json:"output"`
	Usage   ResponsesUsage        `json:"usage"`
	Error   *openaiError          `json:"error,omitempty"`
	Store   bool                  `json:"store"`
	Reason  *ResponsesReasoning   `json:"reasoning,omitempty"`
	Created int64                 `json:"created_at"`
}

// ResponsesOutputItem is one output item: reasoning, message or function_call.
type ResponsesOutputItem struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Content   []ResponsesContentPart `json:"content,omitempty"`
	Summary   []ResponsesContentPart `json:"summary,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
}

// ResponsesUsage holds token usage for this format, including cache details.
type ResponsesUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details,omitempty"`
}

// BuildResponsesRequest renders a conversation into a Responses request.
//
// Insertion rules (§B3): this format supports system messages mid-conversation,
// so only the LEADING run of system messages becomes `instructions` (the
// provider inserts instructions as the first system message) and any later
// system message stays an input item in place. Tool results become
// function_call_output items paired by call_id, and a prior assistant turn's
// tool calls are replayed as function_call items.
func BuildResponsesRequest(model string, messages []harness.Message, opts RequestOptions) (ResponsesRequest, error) {
	dial, err := MapReasoningEffort(opts.ReasoningEffort)
	if err != nil {
		return ResponsesRequest{}, err
	}
	if err := ValidateToolDefinitions(opts.Tools); err != nil {
		return ResponsesRequest{}, err
	}

	req := ResponsesRequest{
		Model:       model,
		Stream:      false,
		Reasoning:   responsesReasoningPayload(dial, opts.Thinking),
		Tools:       toResponsesTools(opts.Tools),
		ToolChoice:  toResponsesToolChoice(opts.ToolChoice),
		Temperature: 0,
	}

	if opts.TopP != nil {
		if v, send := ResolveTopP(*opts.TopP, opts.Thinking); send {
			req.TopP = &v
		}
	}

	var instructions []string
	leadingSystem := true
	for _, m := range messages {
		if m.Role == "system" && leadingSystem {
			instructions = append(instructions, m.Content)
			continue
		}
		leadingSystem = false

		switch m.Role {
		case "tool":
			req.Input = append(req.Input, ResponsesInputItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		case "assistant":
			if m.Content != "" {
				req.Input = append(req.Input, ResponsesInputItem{
					Role:    "assistant",
					Content: []ResponsesContentPart{{Type: "output_text", Text: m.Content}},
				})
			}
			for _, tc := range m.ToolCalls {
				args := strings.TrimSpace(tc.Function.Arguments)
				if args == "" || !json.Valid([]byte(args)) {
					args = "{}"
				}
				req.Input = append(req.Input, ResponsesInputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				})
			}
		case "system":
			// Mid-conversation system message: supported by this format, so it
			// keeps its position as an input item (§B3).
			req.Input = append(req.Input, ResponsesInputItem{
				Role:    "system",
				Content: []ResponsesContentPart{{Type: "input_text", Text: m.Content}},
			})
		default:
			role := m.Role
			if role != "user" {
				role = "user"
			}
			req.Input = append(req.Input, ResponsesInputItem{
				Role:    role,
				Content: []ResponsesContentPart{{Type: "input_text", Text: m.Content}},
			})
		}
	}

	req.Instructions = strings.Join(instructions, "\n\n")
	return req, nil
}

// responsesReasoningPayload renders the effort dial for this format. As on every
// other surface, an explicitly disabled thinking mode carries no dial (§B2).
func responsesReasoningPayload(dial string, thinking *bool) *ResponsesReasoning {
	if thinking != nil && !*thinking {
		return nil
	}
	if dial == "" {
		return nil
	}
	return &ResponsesReasoning{Effort: dial}
}

// toResponsesTools converts declared tools into the Responses shape. The
// provider supports the `function` tool type and ignores other types, so only
// function tools are emitted.
func toResponsesTools(tools []ToolDefinition) []ResponsesTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ResponsesTool, len(tools))
	for i, t := range tools {
		out[i] = ResponsesTool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
			Strict:      t.Function.Strict,
		}
	}
	return out
}

// toResponsesToolChoice maps the neutral tool_choice vocabulary onto this
// format's shape: none/auto/required go as strings, a named tool becomes
// {"type":"function","name":...}.
func toResponsesToolChoice(choice string) any {
	switch strings.TrimSpace(choice) {
	case "":
		return nil
	case "none", "auto", "required":
		return strings.TrimSpace(choice)
	default:
		return map[string]string{"type": "function", "name": strings.TrimSpace(choice)}
	}
}

// InterpretResponsesResponse turns a Responses response into the harness turn.
//
// The output is a mixed item list: `reasoning` items carry the chain-of-thought,
// `message` items carry the text, `function_call` items carry tool calls. All of
// them belong to the turn, so all of them are returned; a turn whose text is
// empty because it is a tool call or a reasoning-only item is not an error.
func InterpretResponsesResponse(resp *ResponsesResponse) (*harness.LLMResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("llm: responses: nil response")
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("llm: responses api error: %s (type=%s, code=%s)",
			resp.Error.Message, resp.Error.Type, resp.Error.Code)
	}

	var (
		text      string
		thinking  string
		toolCalls []harness.ToolCall
	)
	for _, item := range resp.Output {
		switch item.Type {
		case "reasoning":
			for _, part := range item.Content {
				if part.Type == "reasoning_text" || part.Type == "text" {
					thinking += part.Text
				}
			}
			for _, part := range item.Summary {
				thinking += part.Text
			}
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" || part.Type == "text" {
					text += part.Text
				}
			}
		case "function_call":
			args := strings.TrimSpace(item.Arguments)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, harness.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: harness.ToolCallFunction{
					Name:      item.Name,
					Arguments: args,
				},
			})
		}
	}

	if text == "" && thinking == "" && len(toolCalls) == 0 {
		return nil, fmt.Errorf("llm: responses: response carried no reasoning, message or function_call item (status=%q)", resp.Status)
	}

	var cacheRead int64
	if resp.Usage.InputTokensDetails != nil {
		cacheRead = resp.Usage.InputTokensDetails.CachedTokens
	}

	turn := &harness.LLMResponse{
		Content:          text,
		ReasoningContent: thinking,
		ToolCalls:        toolCalls,
		ModelID:          resp.Model,
		Usage: harness.LLMUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			CacheReadTokens:  cacheRead,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	content := strings.TrimSpace(text)
	if content == "" {
		if len(toolCalls) > 0 {
			// A pure tool call: a real turn with no AgentOutput to parse.
			return turn, nil
		}
		if thinking == "" {
			return turn, nil
		}
		// No output item we can read as an answer. The caller decides whether
		// CoT may stand in for one — this interpreter never promotes silently
		// (§B2), it just reports what the response contained.
		return turn, nil
	}
	turn.Content = content

	var output harness.AgentOutput
	if err := json.Unmarshal([]byte(stripMarkdownCodeBlock(content)), &output); err != nil {
		return turn, fmt.Errorf("llm: responses parse AgentOutput JSON: %w\nRaw content: %s", err, truncateStr(content, 500))
	}
	turn.Output = &output
	return turn, nil
}
