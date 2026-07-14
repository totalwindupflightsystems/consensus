// Package chronicle: E2E integration test — full Chronicle → Consensus → LLM workflow.
package chronicle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestE2E_ChronicleInvestigationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E Chronicle workflow test in short mode")
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
	}

	binPath := findBinary(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chronicle-e2e.db")
	port := 8191

	// Write consensus.yaml with API key
	configYAML := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
llm:
  default_model: deepseek-chat
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: %s
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 3
  max_iterations: 5
  max_consecutive_errors: 3
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 5
hitl:
  require_approval_for_destructive: false
  require_approval_for_schema_changes: false
logging:
  level: debug
compression:
  enabled: false
api_rate:
  admin_per_min: 1000
  session_per_min: 100
  readonly_per_min: 200
`, port, apiKey, dbPath)
	configPath := filepath.Join(tmpDir, "consensus.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Init to get admin key
	initCmd := exec.Command(binPath, "init", "--db-url", "sqlite://"+dbPath)
	initCmd.Dir = tmpDir
	initOut, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, initOut)
	}
	adminKey := extractAdminKey(t, string(initOut))
	t.Logf("admin key: %s...", adminKey[:20])

	// Start serve with config
	ctx := t.Context()
	serveCmd := exec.CommandContext(ctx, binPath, "serve", "--config", configPath, "--port", fmt.Sprintf("%d", port))
	serveCmd.Dir = tmpDir
	serveCmd.Stdout = os.Stdout
	serveCmd.Stderr = os.Stderr
	if err := serveCmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer serveCmd.Process.Kill()

	apiBase := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 30 * time.Second}

	// Wait for serve to be ready
	if !waitForHealth(t, client, apiBase+"/api/v1/health", 10*time.Second) {
		t.Fatal("consensus serve did not become healthy")
	}

	// Seed model registry
	seedModels(t, client, apiBase, adminKey)

	// Create investigation session
	sessionID, sessionKey := createSession(t, client, apiBase, adminKey,
		"Investigation Agent",
		"Investigate user query, produce THINK reasoning trace, SAYS finding with evidence, and store in memory")

	// Send investigation message
	sendMessage(t, client, apiBase, sessionID, sessionKey,
		"Investigate the following: there are reports of unusual API access patterns from IP range 10.0.0.0/8. Analyze the threat, produce a finding with evidence, and recommend action.")

	// Poll for session completion
	if !waitForSessionComplete(t, client, apiBase, sessionID, adminKey, 120*time.Second) {
		t.Fatal("session did not complete within timeout")
	}

	// Verify memory events
	memoryEvents := listMemory(t, client, apiBase, sessionID, adminKey)
	if len(memoryEvents) == 0 {
		t.Fatal("no memory events found — LLM did not produce output")
	}
	t.Logf("memory events: %d", len(memoryEvents))

	hasThink := false
	hasSays := false
	for _, ev := range memoryEvents {
		content, _ := ev["content"].(string)
		evType, _ := ev["type"].(string)
		t.Logf("  [%s] %.80s...", evType, strings.TrimSpace(content))
		if strings.Contains(strings.ToLower(content), "think") ||
			strings.Contains(strings.ToLower(content), "analy") ||
			strings.Contains(strings.ToLower(content), "threat") {
			hasThink = true
		}
		if strings.Contains(strings.ToLower(content), "finding") ||
			strings.Contains(strings.ToLower(content), "recommend") ||
			strings.Contains(strings.ToLower(content), "evidence") {
			hasSays = true
		}
	}
	if !hasThink {
		t.Log("WARNING: no THINK-like content detected (may be fine — depends on LLM output)")
	}
	if !hasSays {
		t.Log("WARNING: no SAYS-like content detected (may be fine — depends on LLM output)")
	}

	// Verify Chronicle UI serves
	chronicleBase := apiBase + "/chronicle"
	resp, err := client.Get(chronicleBase + "/")
	if err != nil {
		t.Fatalf("Chronicle UI request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Chronicle UI returned %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	for _, required := range []string{"Chronicle", "THINK", "SAYS", "design-system.css"} {
		if !strings.Contains(bodyStr, required) {
			t.Errorf("Chronicle UI missing: %s", required)
		}
	}
	t.Logf("Chronicle UI served: %d bytes", len(body))

	// Verify CSS serves
	cssResp, err := client.Get(chronicleBase + "/css/design-system.css")
	if err != nil {
		t.Fatalf("CSS request failed: %v", err)
	}
	defer cssResp.Body.Close()
	if cssResp.StatusCode != http.StatusOK {
		t.Fatalf("CSS returned %d", cssResp.StatusCode)
	}

	// Verify API session endpoint
	sessionResp, err := doAuthGet(t, client, apiBase+"/api/v1/sessions/"+sessionID, adminKey)
	if err != nil {
		t.Fatalf("session GET failed: %v", err)
	}
	defer sessionResp.Body.Close()
	var sessionData map[string]any
	json.NewDecoder(sessionResp.Body).Decode(&sessionData)
	t.Logf("session: status=%v iteration=%v tokens_in=%v tokens_out=%v",
		sessionData["status"], sessionData["iteration"],
		sessionData["tokens_used_in"], sessionData["tokens_used_out"])

	t.Logf("✅ E2E Chronicle workflow complete: LLM processed investigation, %d memory events, Chronicle UI serving", len(memoryEvents))
}

// ── Helpers ──────────────────────────────────────────────────────────

func findBinary(t *testing.T) string {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "consensus")
	// Resolve project root from test file location
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/consensus")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return outPath
}

func extractAdminKey(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "key=cs_ak_") {
			parts := strings.Split(line, "key=cs_ak_")
			if len(parts) > 1 {
				keyPart := strings.Split(parts[1], " ")[0]
				return "cs_ak_" + keyPart
			}
		}
	}
	t.Fatalf("could not extract admin key from output:\n%s", output)
	return ""
}

func waitForHealth(t *testing.T, client *http.Client, url string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func seedModels(t *testing.T, client *http.Client, apiBase, adminKey string) {
	t.Helper()
	// POST existing endpoint — may fail if already seeded, which is fine
	body := bytes.NewReader([]byte(`{"model_id":"deepseek-chat","tier":1,"max_context":128000,"cost_per_m_in":0.27,"cost_per_m_out":1.10}`))
	req, _ := http.NewRequest(http.MethodPost, apiBase+"/api/v1/models", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func doAuthGet(t *testing.T, client *http.Client, url, key string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	return client.Do(req)
}

func createSession(t *testing.T, client *http.Client, apiBase, adminKey, agentName, goal string) (string, string) {
	t.Helper()
	reqBody := map[string]any{"agent_name": agentName, "goal": goal}
	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, apiBase+"/api/v1/sessions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session returned %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	sessionID, _ := result["id"].(string)
	sessionKey, _ := result["api_key"].(string)
	if sessionID == "" {
		t.Fatal("no session ID in response")
	}
	return sessionID, sessionKey
}

func sendMessage(t *testing.T, client *http.Client, apiBase, sessionID, sessionKey, content string) {
	t.Helper()
	reqBody := map[string]any{"content": content, "type": "user_instruction"}
	b, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/api/v1/sessions/%s/message", apiBase, sessionID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send message returned %d", resp.StatusCode)
	}
}

func waitForSessionComplete(t *testing.T, client *http.Client, apiBase, sessionID, adminKey string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := apiBase + "/api/v1/sessions/" + sessionID
	for time.Now().Before(deadline) {
		resp, err := doAuthGet(t, client, url, adminKey)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		status, _ := data["status"].(string)
		iteration, _ := data["iteration"].(float64)
		t.Logf("  session status=%s iteration=%.0f", status, iteration)
		if status == "idle" && iteration > 0 {
			return true
		}
		if status == "failed" || status == "completed" {
			return status == "completed"
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func listMemory(t *testing.T, client *http.Client, apiBase, sessionID, adminKey string) []map[string]any {
	t.Helper()
	url := apiBase + "/api/v1/sessions/" + sessionID + "/memory"
	resp, err := doAuthGet(t, client, url, adminKey)
	if err != nil {
		t.Fatalf("list memory failed: %v", err)
	}
	defer resp.Body.Close()
	var events []map[string]any
	json.NewDecoder(resp.Body).Decode(&events)
	return events
}
