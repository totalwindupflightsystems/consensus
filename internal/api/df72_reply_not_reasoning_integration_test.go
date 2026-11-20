package api

import (
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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// df72ReasoningMarker is distinctive text that appears ONLY in the planner's
// private reasoning (AgentOutput.internal_monologue). If it ever shows up in the
// polled session field, the harness leaked chain-of-thought into the assistant
// channel (DF-CONSENSUS-072).
const df72ReasoningMarker = "PLANNER-REASONING-MARKER-072"

// df72Reply is the assistant-facing answer the fixture model addresses to the
// user. Acceptance is exact equality with this word.
const df72Reply = "PINE"

// TestDF72ReplyIsAssistantTextNotPlannerReasoning drives the production serve
// path for a plain conversational turn. The fake upstream answers the question
// the way a real planner model does — it fills message_to_user — but omits the
// system action, and its internal_monologue carries private reasoning.
//
// On current main before this fix the turn was classified as a no-op: the reply
// was discarded, the loop burned its whole turn budget, the session settled idle
// and GET /api/v1/sessions/{id} returned last_message = null ("a plain chat turn
// never yields an assistant reply"). The only text that reached last_message in
// the pre-DF-070 design was the monologue itself, i.e. the planner's private
// reasoning verbatim.
//
// The test pins all three acceptance criteria:
//  1. the polled field carries the assistant reply (exactly the requested word),
//  2. the reply is the only assistant text_block in the session,
//  3. the reasoning appears on the debug surface (process log) and nowhere in
//     the user-facing message field.
func TestDF72ReplyIsAssistantTextNotPlannerReasoning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-binary DF-CONSENSUS-072 integration test in short mode")
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
	prompt := ""
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"df72-model"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			if llmCalls.Add(1) == 1 {
				var req struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				for _, m := range req.Messages {
					prompt += m.Content
				}
			}
			// A plain conversational turn as a real planner model answers it:
			// the reply is addressed to the user, the reasoning stays private,
			// and no system action is emitted.
			content := fmt.Sprintf(`{"internal_monologue":"%s the user demands exactly one word; I must obey and answer %s",`+
				`"memory_state_changes":[],"system_actions":[],"message_to_user":"%s","tool_requests":[],"sub_agent_spawns":[]}`,
				df72ReasoningMarker, df72Reply, df72Reply)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "df72-llm-response",
				"model": "df72-model",
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
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
	dbPath := filepath.Join(tmpDir, "df72.db")
	configPath := filepath.Join(tmpDir, "consensus.yaml")
	// logging.level: debug — the planner's private reasoning is delivered to the
	// DEBUG surface (the log), which is what criterion 3 requires; the assertion
	// below proves it lands there and not in the message field.
	config := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
  pprof_addr: ""
llm:
  default_model: df72-model
  provider: openai
  base_url: %s/v1
  api_key: df72-dummy-key
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

	question := "Reply with exactly one word: " + df72Reply
	createBody, _ := json.Marshal(map[string]string{
		"agent_name": "df72-integration",
		"goal":       question,
	})
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

	messageBody, _ := json.Marshal(map[string]string{"content": question, "type": "user_instruction"})
	message := df65APIRequest(t, client, http.MethodPost, baseURL+"/api/v1/sessions/"+created.ID+"/message", adminKey, messageBody)
	if message.StatusCode != http.StatusOK {
		t.Fatalf("post user message: HTTP %d: %s", message.StatusCode, message.Body)
	}

	var session struct {
		Status      string  `json:"status"`
		LastMessage *string `json:"last_message"`
		Iteration   int     `json:"iteration"`
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		current := df65APIRequest(t, client, http.MethodGet, baseURL+"/api/v1/sessions/"+created.ID, adminKey, nil)
		if current.StatusCode != http.StatusOK {
			t.Fatalf("poll session: HTTP %d: %s", current.StatusCode, current.Body)
		}
		if err := json.Unmarshal(current.Body, &session); err != nil {
			t.Fatalf("decode session poll: %v: %s", err, current.Body)
		}
		if session.Status == "idle" || session.Status == "completed" {
			break
		}
		if session.Status == "failed" || session.Status == "paused" {
			t.Fatalf("session reached %q instead of answering the user\n%s", session.Status, df65ReadLog(logPath))
		}
		time.Sleep(200 * time.Millisecond)
	}
	if session.Status != "idle" && session.Status != "completed" {
		t.Fatalf("session stuck in %q (LLM calls=%d)\n%s", session.Status, llmCalls.Load(), df65ReadLog(logPath))
	}

	// Criterion 1 (redirect half): the plain chat turn really reached the model
	// with the user's question — one turn, not a silent local short-circuit.
	if llmCalls.Load() == 0 {
		t.Fatalf("the binary made no LLM call; the conversational turn was never planned\n%s", df65ReadLog(logPath))
	}
	if !strings.Contains(prompt, question) {
		t.Errorf("planning prompt does not carry the user question %q", question)
	}

	// Criterion 1: the polled field carries the assistant reply, exactly.
	if session.LastMessage == nil {
		t.Fatalf("last_message = null: the plain chat turn produced no assistant reply (DF-CONSENSUS-072)\n%s", df65ReadLog(logPath))
	}
	if *session.LastMessage != df72Reply {
		t.Errorf("last_message = %q, want exactly %q", *session.LastMessage, df72Reply)
	}
	// Criterion 1 (leak half): no planner reasoning, no imperative
	// self-instruction phrasing in the user-facing message.
	if strings.Contains(*session.LastMessage, df72ReasoningMarker) {
		t.Errorf("last_message leaks the planner reasoning marker: %q", *session.LastMessage)
	}
	if strings.Contains(strings.ToLower(*session.LastMessage), "i must") {
		t.Errorf("last_message carries imperative self-instruction phrasing: %q", *session.LastMessage)
	}

	database, err := driver.Open(context.Background(), db.Config{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Criterion 2/3: the session's assistant channel holds exactly one message —
	// the reply — and no memory event of ANY type carries the reasoning marker.
	blockRows, err := database.Query(context.Background(),
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block' ORDER BY id`, created.ID)
	if err != nil {
		t.Fatalf("read memory_events text_block rows: %v", err)
	}
	if len(blockRows) != 1 {
		t.Fatalf("assistant text_block rows = %d, want 1 (only the reply is an assistant message)\n%s",
			len(blockRows), df65ReadLog(logPath))
	}
	if got := toString(blockRows[0]["content"]); got != df72Reply {
		t.Errorf("persisted assistant message = %q, want %q", got, df72Reply)
	}

	allEvents, err := database.Query(context.Background(),
		`SELECT type, content FROM memory_events WHERE session_id = $1 ORDER BY id`, created.ID)
	if err != nil {
		t.Fatalf("read memory_events rows: %v", err)
	}
	for _, row := range allEvents {
		if content := toString(row["content"]); strings.Contains(content, df72ReasoningMarker) {
			t.Errorf("planner reasoning persisted into memory_events (type=%v): %q — it belongs on the debug surface only",
				row["type"], content)
		}
	}

	// Criterion 3: the reasoning IS observable — on the debug surface (the log),
	// scrubbed and clearly labelled, never in the user-facing field.
	logText := df65ReadLog(logPath)
	if !strings.Contains(logText, df72ReasoningMarker) {
		t.Errorf("planner reasoning is not observable on the debug surface: %q absent from the serve log", df72ReasoningMarker)
	}
	if !strings.Contains(logText, "planner reasoning (debug surface, never user-facing)") {
		t.Errorf("serve log does not label the reasoning as a debug-surface record")
	}

	// One turn, one reply: the loop did not burn its budget on no-ops.
	if got := llmCalls.Load(); got != 1 {
		t.Errorf("LLM calls = %d, want 1: a plain conversational turn must resolve in a single turn, "+
			"not spin until the turn budget is gone\n%s", got, logText)
	}
}
