package harness

import (
	"os"
	"testing"
)

// skipWithoutRealLLMKey skips real-LLM E2E tests when no live provider key is
// configured. Without a real key the harness can only ever poll for 120s on a
// guaranteed-401, so a skip with a named reason is the honest outcome
// (mirrors the short-mode contract used throughout this package). The cheap
// deterministic companion covering the 401 fail-fast error surface lives in
// internal/llm/fastfail_test.go (TestLLMClient_FastFailOnInvalidKey).
func skipWithoutRealLLMKey(t *testing.T) {
	t.Helper()
	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Skip("DEEPSEEK_API_KEY not set; real-LLM E2E requires a live key")
	}
}
