package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// TestDF65ToolCallHandoffRealBinary drives the production serve path through
// an agent turn that requests a real registered tool. The only fake is the
// OpenAI-compatible upstream; session ingestion, planning, SQLite persistence,
// and the async tool executor all run in the real consensus binary.
func TestDF65ToolCallHandoffRealBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-binary DF-CONSENSUS-65 integration test in short mode")
	}

	projectRoot := df65ProjectRoot(t)
	binPath := filepath.Join(projectRoot, "bin", "consensus")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("create bin directory: %v", err)
	}
	build := exec.Command("go", "build", "-o", binPath, "./cmd/consensus")
	build.Dir = projectRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real consensus binary: %v\n%s", err, out)
	}

	var llmCalls atomic.Int64
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"df65-model"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			llmCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "df65-llm-response",
				"model": "df65-model",
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"internal_monologue":"request the registered read-only tool","memory_state_changes":[],"system_actions":[],"tool_requests":[{"tool_name":"list_sessions","parameters":{}}],"sub_agent_spawns":[]}`,
					},
				}},
				"usage": map[string]int64{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer llm.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve scratch port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release scratch port: %v", err)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "df65.db")
	configPath := filepath.Join(tmpDir, "consensus.yaml")
	config := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
  pprof_addr: ""
llm:
  default_model: df65-model
  provider: openai
  base_url: %s/v1
  api_key: df65-dummy-key
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 1
  max_iterations: 3
  max_consecutive_errors: 2
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: debug
compression:
  enabled: false
`, port, llm.URL, dbPath)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write scratch config: %v", err)
	}

	logPath := filepath.Join(tmpDir, "serve.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create serve log: %v", err)
	}
	serve := exec.Command(binPath, "--config", configPath, "serve")
	serve.Dir = tmpDir
	serve.Stdout = logFile
	serve.Stderr = logFile
	serve.Env = df65CleanServerEnv()
	if err := serve.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start ./bin/consensus serve: %v", err)
	}
	t.Cleanup(func() {
		_ = serve.Process.Kill()
		_ = serve.Wait()
		_ = logFile.Close()
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}
	adminKey := df65WaitReadyAndAdminKey(t, client, baseURL, logPath, serve)

	createBody := []byte(`{"agent_name":"df65-integration","goal":"Call list_sessions once and finish."}`)
	create := df65APIRequest(t, client, http.MethodPost, baseURL+"/api/v1/sessions", adminKey, createBody)
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create session: HTTP %d: %s", create.StatusCode, create.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body, &created); err != nil || created.ID == "" {
		t.Fatalf("decode create-session response: err=%v body=%s", err, create.Body)
	}

	messageBody := []byte(`{"content":"Call list_sessions once and finish.","type":"user_instruction"}`)
	message := df65APIRequest(t, client, http.MethodPost, baseURL+"/api/v1/sessions/"+created.ID+"/message", adminKey, messageBody)
	if message.StatusCode != http.StatusOK {
		t.Fatalf("post user message: HTTP %d: %s", message.StatusCode, message.Body)
	}

	deadline := time.Now().Add(30 * time.Second)
	lastStatus := ""
	for time.Now().Before(deadline) {
		current := df65APIRequest(t, client, http.MethodGet, baseURL+"/api/v1/sessions/"+created.ID, adminKey, nil)
		if current.StatusCode != http.StatusOK {
			t.Fatalf("poll session: HTTP %d: %s", current.StatusCode, current.Body)
		}
		var session struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(current.Body, &session); err != nil {
			t.Fatalf("decode session poll: %v: %s", err, current.Body)
		}
		lastStatus = session.Status
		if lastStatus == "idle" || lastStatus == "completed" {
			goto progressed
		}
		if lastStatus == "failed" || lastStatus == "paused" {
			t.Fatalf("session reached unexpected status %q instead of executing its tool\n%s", lastStatus, df65ReadLog(logPath))
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("session did not progress beyond %q within 30s (LLM calls=%d)\n%s", lastStatus, llmCalls.Load(), df65ReadLog(logPath))

progressed:
	if llmCalls.Load() == 0 {
		t.Fatal("the real binary made no LLM call; test did not exercise the tool-call planning branch")
	}

	database, err := driver.Open(context.Background(), db.Config{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	defer func() { _ = database.Close() }()
	rows, err := database.Query(context.Background(), `
		SELECT tool_name, status FROM tool_requests WHERE session_id = $1
	`, created.ID)
	if err != nil {
		t.Fatalf("read tool_requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1; session status=%q", len(rows), lastStatus)
	}
	if got := fmt.Sprint(rows[0]["tool_name"]); got != "list_sessions" {
		t.Errorf("tool_name = %q, want list_sessions", got)
	}
	if got := fmt.Sprint(rows[0]["status"]); got != "completed" {
		t.Errorf("tool request status = %q, want completed (executor must claim and run the durable row)", got)
	}
	resultRows, err := database.Query(context.Background(), `
		SELECT request_id FROM tool_results WHERE session_id = $1
	`, created.ID)
	if err != nil {
		t.Fatalf("read tool_results: %v", err)
	}
	if len(resultRows) != 1 {
		t.Fatalf("tool_results rows = %d, want 1", len(resultRows))
	}
	t.Logf("DF-CONSENSUS-65: real binary session=%s status=%s tool_request=completed tool_results=1", created.ID, lastStatus)
}

type df65HTTPResponse struct {
	StatusCode int
	Body       []byte
}

func df65APIRequest(t *testing.T, client *http.Client, method, url, key string, body []byte) df65HTTPResponse {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new %s request: %v", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s response: %v", method, url, err)
	}
	return df65HTTPResponse{StatusCode: resp.StatusCode, Body: responseBody}
}

func df65WaitReadyAndAdminKey(t *testing.T, client *http.Client, baseURL, logPath string, serve *exec.Cmd) string {
	t.Helper()
	keyPattern := regexp.MustCompile(`cs_ak_[A-Za-z0-9_-]{10,}`)
	deadline := time.Now().Add(30 * time.Second)
	ready := false
	adminKey := ""
	for time.Now().Before(deadline) {
		if serve.ProcessState != nil && serve.ProcessState.Exited() {
			t.Fatalf("consensus serve exited before readiness\n%s", df65ReadLog(logPath))
		}
		if !ready {
			resp, err := client.Get(baseURL + "/openapi.json")
			if err == nil {
				ready = resp.StatusCode == http.StatusOK
				_ = resp.Body.Close()
			}
		}
		if adminKey == "" {
			adminKey = keyPattern.FindString(df65ReadLog(logPath))
		}
		if ready && adminKey != "" {
			return adminKey
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("serve readiness/admin-key timeout (openapi_ready=%v key_found=%v)\n%s", ready, adminKey != "", df65ReadLog(logPath))
	return ""
}

func df65ReadLog(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "read serve log: " + err.Error()
	}
	return string(contents)
}

func df65ProjectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

func df65CleanServerEnv() []string {
	blocked := []string{
		"CONSENSUS_", "OPENROUTER_BASE_URL", "OPENAI_API_KEY", "DEEPSEEK_API_KEY", "LM_STUDIO_BASE_URL",
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		drop := false
		for _, prefix := range blocked {
			if strings.HasPrefix(entry, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, entry)
		}
	}
	return env
}
