// Package harness: real LLM integration test — proves Conscience works end-to-end
// with LM Studio's qwen model.
//
// axiom:trace work_item=real-llm-integration-tests-01 spec=specs/008-harness.md,specs/015-api-and-mcp.md,specs/000-north-star.md plan=.memory-bank/work-items/real-llm-integration-tests-01/plan.yaml
package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealLLMIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real LLM integration test in short mode")
	}

	binPath := findConscienceBinary(t)
	port := randomPort(t)

	// Use DeepSeek by default, fall back to LM Studio if DEEPSEEK_KEY not set
	model := "deepseek-chat"
	baseURL := "https://api.deepseek.com/v1"
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		apiKey = "sk-5e64bd0e1d7f442a95a951b751e80d67" // fallback from config
	}
	if !apiReachable(t, baseURL, apiKey) {
		// Fall back to LM Studio
		t.Log("DeepSeek not reachable, falling back to LM Studio")
		if !lmStudioReachable(t) {
			t.Skip("neither DeepSeek nor LM Studio reachable — skipping")
		}
		model = "qwen/qwen3.5-9b"
		baseURL = "http://127.0.0.1:1234/v1"
		apiKey = "lm-studio"
	}

	// Write temp config with the selected provider
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "conscience-test-llm.db")
	os.WriteFile(filepath.Join(tmpDir, "conscience.yaml"), []byte(fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
llm:
  default_model: %s
  provider: openai
  base_url: %s
  api_key: %s
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 5
  max_iterations: 5
  max_consecutive_errors: 3
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 5
hitl:
  auto_pause_on_error_threshold: 10
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
  webhook_per_min: 500
`, port, model, baseURL, apiKey, dbPath)), 0644)

	// Start conscience — capture stdout for bootstrap admin key
	cmd := exec.Command(binPath, "--config", filepath.Join(tmpDir, "conscience.yaml"), "serve")
	cmd.Dir = tmpDir
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	t.Logf("starting conscience on port %d...", port)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start conscience: %v", err)
	}
	defer func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
	}()

	// Parse bootstrap admin key
	adminKey := parseBootstrapKey(t, stdoutPipe, 10*time.Second)
	if adminKey == "" {
		cmd.Process.Kill()
		t.Fatal("could not find bootstrap admin key")
	}
	t.Logf("bootstrap admin key: %s...", adminKey[:16])

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForHealth(t, serverURL, 15*time.Second) {
		cmd.Process.Kill()
		t.Fatal("conscience did not become healthy")
	}
	t.Logf("conscience healthy")

	// Step 1: Create session
	createResp := apiPost(t, serverURL+"/api/v1/sessions",
		`{"agent_name":"real-llm-test","goal":"Prove real LLM integration. Respond with a valid AgentOutput containing a memory_state_change INSERT into memory_events."}`, adminKey)
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create session: HTTP %d: %s", createResp.StatusCode, string(body))
	}
	var created struct {
		ID        string `json:"id"`
		APIKey    string `json:"api_key"`
		Status    string `json:"status"`
		Iteration int64  `json:"iteration"`
	}
	json.NewDecoder(createResp.Body).Decode(&created)
	sessionID := created.ID
	sessionKey := created.APIKey
	t.Logf("session created: id=%s status=%s api_key=%s...", sessionID, created.Status, sessionKey[:16])
	if sessionID == "" {
		t.Fatal("missing session ID")
	}
	if sessionKey == "" || len(sessionKey) < 12 {
		t.Fatalf("invalid session API key: %q", sessionKey)
	}

	// Step 2: Send message to trigger harness loop
	msgResp := apiPost(t, serverURL+"/api/v1/sessions/"+sessionID+"/message",
		`{"content":"Respond with a valid AgentOutput JSON containing a memory_state_change that inserts into memory_events."}`, sessionKey)
	msgResp.Body.Close()
	if msgResp.StatusCode != http.StatusOK && msgResp.StatusCode != http.StatusCreated {
		t.Fatalf("send message: HTTP %d", msgResp.StatusCode)
	}
	t.Logf("message sent (HTTP %d)", msgResp.StatusCode)

	// Step 3: Poll for completion. The session transitions:
	//   booting → thinking → planning → idle (or complete)
	// The heartbeat picks up 'thinking' → runs planning → touches status.
	t.Log("waiting for LLM processing to complete...")
	finalStatus := ""
	terminalStates := map[string]bool{
		"idle": true, "complete": true, "done": true,
		"error": true, "failed": true, "paused": true,
	}
	deadline := time.After(240 * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

pollLoop:
	for {
		select {
		case <-deadline:
			t.Log("deadline reached, stopping poll")
			break pollLoop
		case <-ticker.C:
			resp, err := httpGet(serverURL+"/api/v1/sessions/"+sessionID, sessionKey)
			if err != nil {
				t.Logf("  poll: %v", err)
				continue
			}
			if resp == nil || resp.StatusCode == http.StatusNotFound {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var s struct {
				Status    string `json:"status"`
				Iteration int64  `json:"iteration"`
			}
			json.Unmarshal(body, &s)
			t.Logf("  status=%s iteration=%d (HTTP %d)", s.Status, s.Iteration, resp.StatusCode)
			if terminalStates[s.Status] || s.Iteration > 0 && s.Status == "planning" {
				// If the harness hasn't finished but iteration advanced, give it more time.
				// Only break on truly terminal states.
				if terminalStates[s.Status] {
					finalStatus = s.Status
					break pollLoop
				}
			}
		}
	}
	t.Logf("final session status: %s", finalStatus)

	// Step 4: Verify memory events were committed by the LLM.
	// The memory endpoint returns a JSON array of memory events (not wrapped in {"events": ...}).
	memResp := apiGet(t, serverURL+"/api/v1/sessions/"+sessionID+"/memory", sessionKey)
	defer memResp.Body.Close()

	var memoryEvents []map[string]any
	if err := json.NewDecoder(memResp.Body).Decode(&memoryEvents); err != nil {
		t.Logf("memory decode (expected raw array): %v (HTTP %d)", err, memResp.StatusCode)
	} else {
		t.Logf("memory events: %d", len(memoryEvents))
		for _, ev := range memoryEvents {
			t.Logf("  [%v] %v", ev["type"], ev["content"])
		}
	}

	// Step 5: Verify iteration advanced (proof harness picked up the session).
	sessResp := apiGet(t, serverURL+"/api/v1/sessions/"+sessionID, sessionKey)
	defer sessResp.Body.Close()
	var finalSess struct {
		Status    string `json:"status"`
		Iteration int64  `json:"iteration"`
	}
	body, _ := io.ReadAll(sessResp.Body)
	json.Unmarshal(body, &finalSess)
	t.Logf("final: status=%s iteration=%d", finalSess.Status, finalSess.Iteration)

	// Assertions
	if finalSess.Status == "" || finalSess.Status == "thinking" {
		t.Error("session is stuck in thinking — harness never picked it up or planning timed out")
	}
	if finalSess.Iteration == 0 {
		t.Error("iteration is 0 — harness never advanced the session")
	}
	if len(memoryEvents) == 0 {
		t.Error("no memory events — LLM didn't produce memory_state_changes")
	}

	t.Logf("result: status=%s iteration=%d memory_events=%d", finalSess.Status, finalSess.Iteration, len(memoryEvents))
}

// parseBootstrapKey reads stdout looking for the bootstrap admin key.
func parseBootstrapKey(t *testing.T, r io.Reader, timeout time.Duration) string {
	t.Helper()
	ch := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "conscience: first_admin_key created=true key=") {
				for _, p := range strings.Fields(line) {
					if strings.HasPrefix(p, "key=") && !strings.HasPrefix(p, "key_prefix=") {
						ch <- strings.TrimPrefix(p, "key=")
						return
					}
				}
			}
		}
	}()
	select {
	case key := <-ch:
		return key
	case <-time.After(timeout):
		return ""
	}
}

func findConscienceBinary(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"../conscience", filepath.Join("..", "..", "conscience")} {
		if abs, _ := filepath.Abs(p); true {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	t.Skip("binary not found — run 'go build -o conscience ./cmd/conscience'")
	return ""
}

func lmStudioReachable(t *testing.T) bool {
	t.Helper()
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:1234/v1/models")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func apiReachable(t *testing.T, baseURL, apiKey string) bool {
	t.Helper()
	body := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`
	req, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func randomPort(t *testing.T) int {
	t.Helper()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForHealth(t *testing.T, baseURL string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return false
		default:
			resp, err := http.Get(baseURL + "/api/v1/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
}

func apiPost(t *testing.T, url, body, apiKey string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func apiGet(t *testing.T, url, apiKey string) *http.Response {
	t.Helper()
	resp, err := httpGet(url, apiKey)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func httpGet(url, apiKey string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return (&http.Client{Timeout: 30 * time.Second}).Do(req)
}
