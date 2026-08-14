// Package demo: keyless smoke test (C-GAP-019).
//
// TestSmokeKeyless runs the full agent harness loop end-to-end against a
// mocked OpenAI-compatible LLM endpoint: real server binary, real SQLite
// database, real session → heartbeat → LLM call → memory events. It needs
// no API key, no network egress, and no cost.
//
// Run: make smoke   (or: timeout 60 go test -run Smoke ./demo/)
package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// smokePort must not collide with demoPort (18885) — both tests run in the
// same package binary.
const smokePort = 18886

// TestSmokeKeyless is the keyless smoke path for a fresh checkout: it proves
// the server boots, migrates a scratch SQLite DB, serves the API, claims a
// session on the heartbeat, calls the configured LLM endpoint, and finishes
// the loop — all against a local mock, in under 60 seconds.
func TestSmokeKeyless(t *testing.T) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║     CONSENSUS — Keyless Smoke Test (mocked LLM)             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// ---- Phase 1: mock OpenAI-compatible /v1/chat/completions endpoint ----
	var mu sync.Mutex
	llmCalls := 0
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		llmCalls++
		n := llmCalls
		mu.Unlock()
		fmt.Printf("smoke: LLM request received by mock (#%d)\n", n)
		w.Header().Set("Content-Type", "application/json")
		// Minimal valid chat.completions body; content is a JSON AgentOutput
		// with no tool requests — the harness finishes the iteration idle.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "smoke-1",
			"model": "smoke-model",
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"internal_monologue":"keyless smoke ok","memory_state_changes":[],"system_actions":[],"tool_requests":[],"sub_agent_spawns":[]}`,
				},
			}},
			"usage": map[string]int64{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer mock.Close()

	// ---- Phase 2: scratch config — dummy key, mock base URL, SQLite DB ----
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "smoke.db")
	configPath := filepath.Join(tmpDir, "consensus.yaml")
	config := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
llm:
  default_model: deepseek-chat
  provider: openai
  base_url: %s/v1
  api_key: smoke-dummy-key
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 3
  max_iterations: 3
  max_consecutive_errors: 2
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: info
compression:
  enabled: false
`, smokePort, mock.URL, dbPath)
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("smoke: write config: %v", err)
	}

	// ---- Phase 3: start the real server binary (built once in TestMain) ----
	adminKey, cmd, serverLog, err := startServer(t, configPath)
	if err != nil {
		t.Fatalf("smoke: start server: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", smokePort)
	if !waitForHealth(serverURL, 20*time.Second) {
		t.Fatalf("smoke: server not healthy within 20s")
	}
	fmt.Println("smoke: server up (health OK)")

	// ---- Phase 4: create session, wake it, let the heartbeat claim it ----
	sid := createSession(t, serverURL, adminKey, "Keyless smoke: return a short plan and finish.")
	fmt.Println("smoke: session created")
	sendMsg(t, serverURL, adminKey, sid, "Start working now.")
	fmt.Println("smoke: session woken — waiting for heartbeat + mocked LLM calls...")

	// ---- Phase 5: poll until the mock answered and the session is done ----
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		calls := llmCalls
		mu.Unlock()
		status := sessionStatus(t, serverURL, adminKey, sid)
		if calls >= 1 && (status == "completed" || status == "idle") {
			fmt.Printf("smoke: session reached status %q after %d mocked LLM call(s)\n", status, calls)
			fmt.Println("smoke: PASS — keyless end-to-end harness loop works")
			return
		}
		time.Sleep(2 * time.Second)
	}

	// ---- Timeout: clear diagnostic — no raw goroutine dumps ----
	mu.Lock()
	calls := llmCalls
	mu.Unlock()
	t.Fatalf(
		"smoke: TIMEOUT after 45s — session stuck in %q with %d mocked LLM call(s).\n"+
			"Session state: %s\nServer log tail:\n%s",
		sessionStatus(t, serverURL, adminKey, sid), calls,
		sessionJSON(t, serverURL, adminKey, sid),
		logTail(serverLog, 40),
	)
}

// sessionStatus returns the session's status field ("" if absent).
func sessionStatus(t *testing.T, url, key, sid string) string {
	t.Helper()
	s := getSession(t, url, key, sid)
	if st, ok := s["status"].(string); ok {
		return st
	}
	return ""
}

// sessionJSON returns the raw session JSON for timeout diagnostics.
func sessionJSON(t *testing.T, url, key, sid string) string {
	t.Helper()
	return api(t, url, key, "GET", fmt.Sprintf("/api/v1/sessions/%s", sid), "").Body
}

// logTail returns the last n lines of a captured process log buffer.
func logTail(buf *bytes.Buffer, n int) string {
	lines := strings.Split(buf.String(), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
