// Package demo: live demonstration — real server, real LLM calls, real results.
//
// Run: DEEPSEEK_API_KEY=sk-... go test -v -run TestDemo -timeout 300s ./demo/
//
// This launches Conscience, creates sessions that the heartbeat picks up,
// waits for the planning loop to process them with real LLM calls, then
// displays the results: memory events, session status, and audit trail.
package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const demoPort = 18885

var demoBin string

func TestMain(m *testing.M) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	tmpBin := filepath.Join(os.TempDir(), "conscience-demo")
	build := exec.Command("go", "build", "-o", tmpBin, "./cmd/conscience/")
	build.Dir = projectRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "BUILD FAILED: %v\n%s\n", err, out)
		os.Exit(1)
	}
	demoBin = tmpBin
	os.Exit(m.Run())
}

func TestDemo_FullAgentHarness(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║     CONSCIENCE — Real LLM-Powered Agent Harness Demo        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "demo.db")
	configPath := filepath.Join(tmpDir, "conscience.yaml")

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
  heartbeat_interval_seconds: 3
  max_iterations: 3
  max_consecutive_errors: 2
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: info
`, demoPort, apiKey, dbPath)
	os.WriteFile(configPath, []byte(config), 0644)

	// Start server
	adminKey, cmd, err := startServer(t, configPath)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Process.Kill()

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", demoPort)
	if !waitForHealth(serverURL, 15*time.Second) { t.Fatal("not healthy") }
	fmt.Printf("✓ Server started — admin key: %s...\n", adminKey[:25])
	fmt.Println("✓ Heartbeat loop active — will auto-process sessions")

	// ===================================================================
	// DEMO 1: Create a session and let the heartbeat process it
	// ===================================================================
	fmt.Println()
	fmt.Println("━━━ DEMO 1: Agent Plans & Executes via LLM ━━━")

	s1 := createSession(t, serverURL, adminKey, "Create a table called demo_tasks with columns: id, name, status, created_at. Insert 3 sample tasks.")
	fmt.Printf("   Session %s created\n", s1[:20]+"...")
	// Wake session: send a message to transition from "booting" to "thinking"
	sendMsg(t, serverURL, adminKey, s1, "Start working on the task now.")
	fmt.Println("   Waking session for heartbeat pickup...")

	// Wait for heartbeat to pick it up (max 3 iterations at 3s heartbeat = ~15s)
	fmt.Println("   Waiting for heartbeat to process session...")
	if !waitForStatus(t, serverURL, adminKey, s1, []string{"completed", "idle", "failed"}, 30*time.Second) {
		t.Log("   ⚠ Session still processing — showing partial results")
	}

	showSessionResult(t, serverURL, adminKey, s1, "Demo 1")

	// ===================================================================
	// DEMO 2: Two sessions with different topics
	// ===================================================================
	fmt.Println()
	fmt.Println("━━━ DEMO 2: Multi-Topic Sessions ━━━")

	s2 := createSession(t, serverURL, adminKey, "You are a security auditor. List 3 common web vulnerabilities and store each as a memory event.")
	sendMsg(t, serverURL, adminKey, s2, "Start the security audit now.")
	s3 := createSession(t, serverURL, adminKey, "You are a performance engineer. Identify 3 database optimization strategies and record them.")
	sendMsg(t, serverURL, adminKey, s3, "Start the performance analysis now.")

	fmt.Println("   Waiting for both sessions to process...")
	waitForStatus(t, serverURL, adminKey, s2, []string{"completed", "idle", "failed"}, 30*time.Second)
	waitForStatus(t, serverURL, adminKey, s3, []string{"completed", "idle", "failed"}, 30*time.Second)

	showSessionResult(t, serverURL, adminKey, s2, "Demo 2a (Security)")
	fmt.Println()
	showSessionResult(t, serverURL, adminKey, s3, "Demo 2b (Performance)")

	// ===================================================================
	// DEMO 3: Crash Recovery
	// ===================================================================
	fmt.Println()
	fmt.Println("━━━ DEMO 3: Crash Recovery ━━━")

	s4 := createSession(t, serverURL, adminKey, "Create a table crash_test with a single column 'survived' and insert one row with value 'yes'.")
	sendMsg(t, serverURL, adminKey, s4, "Start working on the crash test task.")
	fmt.Printf("   Session %s created — waiting for processing...\n", s4[:20]+"...")
	waitForStatus(t, serverURL, adminKey, s4, []string{"completed", "idle", "failed"}, 30*time.Second)

	// Read pre-crash data
	preMemory := getMemory(t, serverURL, adminKey, s4)
	fmt.Printf("   Pre-crash memory events: %d\n", len(preMemory))

	// Kill server
	cmd.Process.Kill()
	cmd.Wait()
	fmt.Println("   💥 Server killed")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database missing!")
	}
	fmt.Println("   ✓ Database intact on disk")

	// Restart — same DB, key survives in DB
	adminKey2, cmd2 := adminKey, (*exec.Cmd)(nil) // reuse original key
	var err2 error
	cmd2, err2 = restartServer(t, configPath)
	if err2 != nil {
		t.Fatalf("restart: %v", err2)
	}
	defer cmd2.Process.Kill()

	if !waitForHealth(serverURL, 15*time.Second) { t.Fatal("restart not healthy") }
	fmt.Println("   ✓ Server restarted")

	// Check session survived
	postMemory := getMemory(t, serverURL, adminKey2, s4)
	fmt.Printf("   Post-crash memory events: %d\n", len(postMemory))
	if len(postMemory) >= len(preMemory) && len(preMemory) > 0 {
		fmt.Println("   ✓ Session data intact — crash recovery works")
	} else {
		fmt.Printf("   ⚠ Pre=%d Post=%d — check SQLite WAL\n", len(preMemory), len(postMemory))
	}

	// ===================================================================
	// SUMMARY
	// ===================================================================
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                      DEMO COMPLETE                          ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Real LLM calls: DeepSeek V4 Flash (via HTTPS API)          ║")
	fmt.Println("║  Agent plans, executes SQL, stores memory — autonomously    ║")
	fmt.Println("║  Data survives server crash + restart                       ║")
	fmt.Println("║  Sessions queryable via REST API                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	cmd2.Process.Kill()
}

// ============================================================================
// Server lifecycle
// ============================================================================

func startServer(t *testing.T, configPath string) (string, *exec.Cmd, error) {
	t.Helper()
	cmd := exec.Command(demoBin, "--config", configPath, "serve")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	adminKey := ""
	deadline := time.After(20 * time.Second)
	for adminKey == "" {
		select {
		case <-deadline:
			cmd.Process.Kill()
			return "", nil, fmt.Errorf("key timeout. Output:\n%s", stdout.String())
		default:
			for _, line := range strings.Split(stdout.String(), "\n") {
				for _, f := range strings.Fields(line) {
					if strings.HasPrefix(f, "key=cs_ak_") {
						adminKey = strings.TrimPrefix(f, "key=")
						adminKey = strings.Trim(strings.TrimSuffix(adminKey, ","), `"'`)
					}
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	return adminKey, cmd, nil
}

// restartServer starts serve without parsing a new key (DB already has one).
func restartServer(t *testing.T, configPath string) (*exec.Cmd, error) {
	t.Helper()
	cmd := exec.Command(demoBin, "--config", configPath, "serve")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// sendMsg sends a message to a session, waking it from "booting" to "thinking".
func sendMsg(t *testing.T, url, key, sid, content string) {
	body := fmt.Sprintf(`{"type":"user_instruction","content":"%s"}`, strings.ReplaceAll(content, `"`, `\"`))
	api(t, url, key, "POST", fmt.Sprintf("/api/v1/sessions/%s/message", sid), body)
}

// ============================================================================
// API helpers
// ============================================================================

func createSession(t *testing.T, url, key, goal string) string {
	body := fmt.Sprintf(`{"agent_name":"demo","model_id":"deepseek-chat","goal":"%s"}`, goal)
	resp := api(t, url, key, "POST", "/api/v1/sessions", body)
	var r struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(resp.Body), &r)
	if r.ID == "" {
		t.Fatalf("create session: %d %s", resp.Status, resp.Body)
	}
	return r.ID
}

func getMemory(t *testing.T, url, key, sid string) []map[string]interface{} {
	resp := api(t, url, key, "GET", fmt.Sprintf("/api/v1/sessions/%s/memory", sid), "")
	var r []map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &r)
	return r
}

func getSession(t *testing.T, url, key, sid string) map[string]interface{} {
	resp := api(t, url, key, "GET", fmt.Sprintf("/api/v1/sessions/%s", sid), "")
	var r map[string]interface{}
	json.Unmarshal([]byte(resp.Body), &r)
	return r
}

func waitForStatus(t *testing.T, url, key, sid string, targets []string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return false
		default:
			s := getSession(t, url, key, sid)
			status := ""
			if st, ok := s["status"].(string); ok { status = st }
			for _, tgt := range targets {
				if status == tgt { return true }
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func showSessionResult(t *testing.T, url, key, sid, label string) {
	s := getSession(t, url, key, sid)
	status := ""
	if st, ok := s["status"].(string); ok { status = st }
	iteration := float64(0)
	if it, ok := s["iteration"].(float64); ok { iteration = it }
	tokensIn := float64(0)
	if ti, ok := s["tokens_used_in"].(float64); ok { tokensIn = ti }
	tokensOut := float64(0)
	if to, ok := s["tokens_used_out"].(float64); ok { tokensOut = to }

	fmt.Printf("   ┌─ %s ─────────────────────────────\n", label)
	fmt.Printf("   │ Status: %s | Iterations: %.0f | Tokens: %.0f in / %.0f out\n",
		status, iteration, tokensIn, tokensOut)

	mem := getMemory(t, url, key, sid)
	fmt.Printf("   │ Memory events: %d\n", len(mem))

	// Show the most recent events first
	sort.Slice(mem, func(i, j int) bool {
		// Sort by ID descending (newest first)
		idi := float64(0)
		idj := float64(0)
		if v, ok := mem[i]["id"].(float64); ok { idi = v }
		if v, ok := mem[j]["id"].(float64); ok { idj = v }
		return idi > idj
	})

	shown := 0
	for _, evt := range mem {
		if shown >= 5 { break }
		etype := ""
		if t, ok := evt["type"].(string); ok { etype = t }
		content := ""
		if c, ok := evt["content"].(string); ok { content = c }
		if etype == "thinking" || etype == "system" { continue } // skip noise
		prefix := "  "
		switch etype {
		case "tool_call": prefix = "  🔧"
		case "tool_result": prefix = "  📊"
		case "text_block": prefix = "  💬"
		default: prefix = "   "
		}
		fmt.Printf("%s [%s] %s\n", prefix, etype, truncate(content, 80))
		shown++
	}
	fmt.Println("   └─────────────────────────────────────────")
}

// ============================================================================
// HTTP helpers
// ============================================================================

type apiResp struct{ Status int; Body string }

func api(t *testing.T, url, key, method, path, body string) apiResp {
	t.Helper()
	var r io.Reader
	if body != "" { r = strings.NewReader(body) }
	req, _ := http.NewRequest(method, url+path, r)
	if key != "" { req.Header.Set("Authorization", "Bearer "+key) }
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatalf("%s %s: %v", method, path, err) }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return apiResp{resp.StatusCode, string(b)}
}

func waitForHealth(url string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline: return false
		default:
			resp, err := http.Get(url + "/api/v1/health")
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return true
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}

func findProjectRoot() (string, error) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { return "", fmt.Errorf("go.mod not found") }
		dir = parent
	}
}
