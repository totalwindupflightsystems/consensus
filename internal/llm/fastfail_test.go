package llm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/harness"
	"github.com/wojons/consensus/internal/llm"
)

// TestLLMClient_FastFailOnInvalidKey is the cheap deterministic companion to
// the real-LLM E2E tests (QA-CONSENSUS-19): with a deliberately invalid key
// against the real DeepSeek endpoint, the client must fail FAST with a 401
// auth error naming the key configuration fix, not retry-loop or hang. This
// pins the error surface the harness E2E tests would otherwise exercise at
// the cost of a 120s poll window on a key known in advance to be invalid.
func TestLLMClient_FastFailOnInvalidKey(t *testing.T) {
	if testing.Short() {
		t.Skip("live network call skipped in short mode")
	}
	client := llm.NewOpenAIClient(&llm.Config{
		Provider:  "openai",
		BaseURL:   "https://api.deepseek.com/v1",
		APIKey:    "test-fake-key-not-a-real-secret",
		Model:     "deepseek-chat",
		MaxTokens: 32,
	})

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := client.Call(ctx, []harness.Message{{Role: "user", Content: "ping"}})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected auth failure with invalid key, got nil error")
	}
	if elapsed > 30*time.Second {
		t.Errorf("invalid key should fail fast, took %s: %v", elapsed, err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected HTTP 401 in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("expected actionable key hint in error, got: %v", err)
	}
}
