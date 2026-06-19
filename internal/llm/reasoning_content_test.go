// Package llm: tests for thinking-model reasoning_content fallback.
package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// TestOpenAIClient_ReasoningContentFallback verifies that when a thinking/reasoning
// model (Qwen, DeepSeek-R1 style) returns an empty "content" field with output in
// "reasoning_content", the client extracts from reasoning_content as a fallback.
func TestOpenAIClient_ReasoningContentFallback(t *testing.T) {
	// Fake server that returns a thinking-model response: content="" but reasoning_content has the JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiChatResponse{
			ID:    "test-reasoning-id",
			Model: "qwen-thinking",
			Choices: []openaiChatChoice{
				{
					Message: openaiChatMessage{
						Role:             "assistant",
						Content:          "", // thinking models leave this empty
						ReasoningContent: `{"internal_monologue":"I am thinking through this step by step","memory_state_changes":[],"system_actions":[],"tool_requests":[],"sub_agent_spawns":[]}`,
					},
				},
			},
			Usage: openaiChatUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		Model:    "qwen-thinking",
		MaxTokens: 1024,
	})

	resp, err := client.Call(t.Context(), []harness.Message{
		{Role: "user", Content: "Think about this."},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Output == nil {
		t.Fatal("expected non-nil output")
	}
	if resp.Output.InternalMonologue != "I am thinking through this step by step" {
		t.Errorf("monologue = %q, want %q", resp.Output.InternalMonologue, "I am thinking through this step by step")
	}
}

// TestOpenAIClient_ReasoningContentFallback_EmptyBoth verifies that when both
// content and reasoning_content are empty, we get a parse error (not a panic).
func TestOpenAIClient_ReasoningContentFallback_EmptyBoth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiChatResponse{
			ID:    "test-empty-id",
			Model: "broken-model",
			Choices: []openaiChatChoice{
				{
					Message: openaiChatMessage{
						Role:             "assistant",
						Content:          "",
						ReasoningContent: "",
					},
				},
			},
			Usage: openaiChatUsage{PromptTokens: 1, CompletionTokens: 0, TotalTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		Model:    "broken-model",
		MaxTokens: 1024,
	})

	_, err := client.Call(t.Context(), []harness.Message{
		{Role: "user", Content: "test"},
	})
	if err == nil {
		t.Fatal("expected error for empty content + empty reasoning_content")
	}
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "unexpected") {
		t.Logf("error (acceptable): %v", err)
	}
}
