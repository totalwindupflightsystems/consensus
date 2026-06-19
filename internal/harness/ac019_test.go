// Package harness: server restart persistence test (AC-019).
//
// axiom:trace work_item=ac-019-server-restart-persistence spec=SPEC-008 impl=internal/harness
package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServerRestartPersistence verifies AC-019:
// Sessions and data survive server restart.
func TestServerRestartPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping server restart test in short mode")
	}

	binPath := findConscienceBinary(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "consensus-test.db")
	port := randomPort(t)

	// Write config
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		apiKey = "sk-test-fallback"
	}
	config := fmt.Sprintf(`server:
  port: %d
  bootstrap_api_key_ttl: 2160h
  admin_api_key_ttl: 2160h
llm:
  default_model: deepseek-chat
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: %s
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 5
  max_iterations: 3
  max_consecutive_errors: 3
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: debug
`, port, apiKey, dbPath)
	if err := os.WriteFile(filepath.Join(tmpDir, "consensus.yaml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// --- PHASE 1: Start server, create session ---
	t.Log("starting server (phase 1)...")
	cmd1 := exec.Command(binPath, "--config", filepath.Join(tmpDir, "consensus.yaml"), "serve")
	cmd1.Dir = tmpDir
	stdout1, _ := cmd1.StdoutPipe()
	cmd1.Stderr = os.Stderr
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer cmd1.Process.Kill()

	adminKey := parseBootstrapKey(t, bufio.NewReader(stdout1), 10*time.Second)
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForHealth(t, serverURL, 15*time.Second) {
		cmd1.Process.Kill()
		t.Fatal("server did not become healthy")
	}

	// Create session with admin key auth
	createReq, _ := http.NewRequest("POST", serverURL+"/api/v1/sessions",
		strings.NewReader(`{"agent_name":"restart-test","goal":"AC-019 persistence test"}`))
	createReq.Header.Set("Authorization", "Bearer "+adminKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		cmd1.Process.Kill()
		t.Fatalf("create session: %v", err)
	}
	defer createResp.Body.Close()

	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		cmd1.Process.Kill()
		t.Fatalf("decode: %v", err)
	}
	t.Logf("created session: id=%s status=%s", created.ID, created.Status)
	time.Sleep(1 * time.Second)

	// --- PHASE 2: Kill server ---
	t.Log("killing server...")
	cmd1.Process.Signal(os.Interrupt)
	cmd1.Wait()
	t.Log("server stopped")

	// --- PHASE 3: Restart server with SAME db ---
	t.Log("restarting server (phase 2)...")
	cmd2 := exec.Command(binPath, "--config", filepath.Join(tmpDir, "consensus.yaml"), "serve")
	cmd2.Dir = tmpDir
	stdout2, _ := cmd2.StdoutPipe()
	cmd2.Stderr = os.Stderr
	if err := cmd2.Start(); err != nil {
		t.Fatalf("restart server: %v", err)
	}
	defer cmd2.Process.Kill()

	_ = parseBootstrapKey(t, bufio.NewReader(stdout2), 10*time.Second)
	if !waitForHealth(t, serverURL, 15*time.Second) {
		cmd2.Process.Kill()
		t.Fatal("server did not become healthy on restart")
	}
	t.Log("server restarted")

	// --- PHASE 4: Verify session exists with same data ---
	t.Logf("verifying session %s still exists...", created.ID)
	getReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/sessions/%s", serverURL, created.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminKey)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != 200 {
		t.Errorf("AC-019 FAIL: session not found after restart (HTTP %d): %s", getResp.StatusCode, string(body))
		return
	}

	var restored struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Goal   string `json:"goal"`
	}
	if err := json.NewDecoder(strings.NewReader(string(body))).Decode(&restored); err != nil {
		t.Fatalf("decode restored session: %v", err)
	}

	pass := true
	if restored.ID != created.ID {
		t.Errorf("AC-019 FAIL: session ID changed: %s -> %s", created.ID, restored.ID)
		pass = false
	} else {
		t.Logf("AC-019: session ID preserved: %s", restored.ID)
	}
	if restored.Goal != "AC-019 persistence test" {
		t.Errorf("AC-019 FAIL: goal changed: %q", restored.Goal)
		pass = false
	} else {
		t.Log("AC-019: goal preserved")
	}
	if pass {
		t.Log("AC-019 PASS: session persisted across restart")
	}
}
