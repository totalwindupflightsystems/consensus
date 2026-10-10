package llm

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

func TestOpenAIClient_PromptCacheUsage(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	tests := []struct {
		name           string
		usage          map[string]any
		wantCacheRead  int64
		wantCacheWrite int64
	}{
		{
			name: "nested cached tokens and cache creation tokens",
			usage: map[string]any{
				"prompt_tokens":               100,
				"completion_tokens":           20,
				"total_tokens":                120,
				"prompt_tokens_details":       map[string]any{"cached_tokens": 42},
				"cache_creation_input_tokens": 17,
			},
			wantCacheRead:  42,
			wantCacheWrite: 17,
		},
		{
			name: "omitted details default to zero",
			usage: map[string]any{
				"prompt_tokens":     80,
				"completion_tokens": 10,
				"total_tokens":      90,
			},
		},
		{
			name: "prompt cache miss tokens map to cache writes",
			usage: map[string]any{
				"prompt_tokens":            75,
				"completion_tokens":        5,
				"total_tokens":             80,
				"prompt_cache_miss_tokens": 23,
			},
			wantCacheWrite: 23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"id":    "cache-usage-response",
					"model": "cache-aware-model",
					"choices": []map[string]any{{
						"message": map[string]any{
							"role":    "assistant",
							"content": `{"internal_monologue":"ok","memory_state_changes":[],"system_actions":[],"tool_requests":[],"sub_agent_spawns":[]}`,
						},
					}},
					"usage": tt.usage,
				}); err != nil {
					t.Errorf("encode fixture response: %v", err)
				}
			}))
			defer server.Close()

			client := NewOpenAIClient(&Config{
				Provider:  ProviderOpenAI,
				BaseURL:   server.URL,
				Model:     "cache-aware-model",
				MaxTokens: 256,
			})

			response, err := client.Call(t.Context(), []harness.Message{{Role: "user", Content: "test"}})
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if response.Usage.CacheReadTokens != tt.wantCacheRead {
				t.Errorf("CacheReadTokens = %d, want %d", response.Usage.CacheReadTokens, tt.wantCacheRead)
			}
			if response.Usage.CacheWriteTokens != tt.wantCacheWrite {
				t.Errorf("CacheWriteTokens = %d, want %d", response.Usage.CacheWriteTokens, tt.wantCacheWrite)
			}
		})
	}

	if !strings.Contains(logOutput.String(), "cached_tokens=42") {
		t.Errorf("response log does not include cached_tokens=42: %s", logOutput.String())
	}
}
