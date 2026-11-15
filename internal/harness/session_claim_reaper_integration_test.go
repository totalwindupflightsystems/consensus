// Package harness: real-binary session-claim recovery integration test.
package harness

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
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

const sessionClaimBailLog = "planning: session already claimed by another worker, bailing out"

func TestSessionClaimReaperRealBinaryRecoversKilledWorker(t *testing.T) {
	if testing.Short() {
		t.Skip("real-binary lease-expiry integration test")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "consensus")
	build := exec.Command("go", "build", "-o", binary, "./cmd/consensus/")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real consensus binary: %v\n%s", err, output)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve scratch port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release scratch port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	dbPath := filepath.Join(tmpDir, "reaper.db")

	chatStarted := make(chan struct{}, 1)
	chatRelease := make(chan struct{})
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			select {
			case chatStarted <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-chatRelease:
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer func() {
		close(chatRelease)
		llmServer.CloseClientConnections()
		llmServer.Close()
	}()

	firstConfig := filepath.Join(tmpDir, "first.yaml")
	writeSessionReaperConfig(t, firstConfig, port, dbPath, "openai", llmServer.URL+"/v1", "integration-only-key")
	firstLog := filepath.Join(tmpDir, "first.log")
	first := startSessionReaperServer(t, binary, firstConfig, firstLog, false)
	adminKey := waitSessionReaperAdminKey(t, firstLog)
	waitSessionReaperReady(t, baseURL)

	sessionID := createSessionReaperSession(t, baseURL, adminKey)
	sendSessionReaperMessage(t, baseURL, adminKey, sessionID)
	select {
	case <-chatStarted:
	case <-time.After(15 * time.Second):
		t.Fatalf("claiming worker never reached the blocking LLM call; first log:\n%s", readSessionReaperLog(firstLog))
	}
	waitSessionReaperStatus(t, baseURL, adminKey, sessionID, "planning", 5*time.Second)
	stopSessionReaperServer(first)

	// The real worker was killed while holding the planning claim. Move the
	// persisted row to the tool_exec hand-off point and age its heartbeat to
	// inside the final 10s of the lease. This models the reported wedge while
	// leaving time to observe claim-bail growth before the next 30s reap tick.
	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("open killed worker database: %v", err)
	}
	staleHeartbeat := time.Now().Add(-defaultClaimVisibilityTimeout + 10*time.Second).UTC().Format(time.RFC3339)
	if err := database.Exec(ctx, `UPDATE sessions SET status = 'tool_exec', heartbeat_at = $1 WHERE id = $2`, staleHeartbeat, sessionID); err != nil {
		_ = database.Close()
		t.Fatalf("model killed worker's tool_exec claim: %v", err)
	}
	// DF-CONSENSUS-68: the dead worker had already claimed a tool request and
	// never wrote its result. Without requeue-on-reap that row rests in
	// 'executing' forever and the session can never finish the hand-off.
	staleRequestID := seedKilledWorkerToolRequest(t, ctx, database, sessionID)
	if err := database.Close(); err != nil {
		t.Fatalf("close killed worker database: %v", err)
	}

	secondConfig := filepath.Join(tmpDir, "second.yaml")
	writeSessionReaperConfig(t, secondConfig, port, dbPath, "mock", "", "")
	secondLog := filepath.Join(tmpDir, "second.log")
	startSessionReaperServer(t, binary, secondConfig, secondLog, true)
	waitSessionReaperReady(t, baseURL)

	// Before expiry, every heartbeat must rediscover the wedged session and
	// emit another claim-bail line. Without this positive control, a later
	// zero-growth assertion could pass merely because dispatch never ran.
	bailDeadline := time.Now().Add(12 * time.Second)
	for strings.Count(readSessionReaperLog(secondLog), sessionClaimBailLog) < 2 && time.Now().Before(bailDeadline) {
		time.Sleep(100 * time.Millisecond)
	}
	preExpiryBails := strings.Count(readSessionReaperLog(secondLog), sessionClaimBailLog)
	if preExpiryBails < 2 {
		t.Fatalf("claim-bail lines did not grow before lease expiry: count=%d\nsecond log:\n%s", preExpiryBails, readSessionReaperLog(secondLog))
	}
	t.Logf("pre-expiry claim-bail count=%d", preExpiryBails)

	waitSessionReaperStatus(t, baseURL, adminKey, sessionID, "idle", 45*time.Second)
	if !strings.Contains(readSessionReaperLog(secondLog), "harness: reclaimed stale session claim") {
		t.Fatalf("session reached idle without the required reaper transition; second log:\n%s", readSessionReaperLog(secondLog))
	}
	// The reaped worker's in-flight tool request must be claimable again: the
	// tool executor has to pick it up and drive it to a terminal status instead
	// of leaving 'executing' behind forever.
	waitKilledWorkerToolRequestSettled(t, dbPath, staleRequestID, 20*time.Second)
	settledBails := strings.Count(readSessionReaperLog(secondLog), sessionClaimBailLog)
	time.Sleep(3500 * time.Millisecond) // > three configured heartbeat ticks
	afterObservationBails := strings.Count(readSessionReaperLog(secondLog), sessionClaimBailLog)
	if afterObservationBails != settledBails {
		t.Fatalf("claim-bail lines kept growing after recovery: before=%d after=%d", settledBails, afterObservationBails)
	}
	t.Logf("PASS: pre-expiry bail count=%d, terminal=idle, post-recovery observation new bail lines=%d", preExpiryBails, afterObservationBails-settledBails)
}

func writeSessionReaperConfig(t *testing.T, path string, port int, dbPath, provider, baseURL, apiKey string) {
	t.Helper()
	config := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
  pprof_addr: ""
  bootstrap_api_key_ttl: 2160h
  admin_api_key_ttl: 2160h
llm:
  provider: %s
  default_model: gpt-4o
  base_url: %s
  api_key: %s
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 1
  max_consecutive_errors: 3
  planning_timeout_seconds: 180
database:
  url: "sqlite://%s"
  max_open_conns: 4
compression:
  enabled: false
logging:
  level: info
`, port, provider, baseURL, apiKey, dbPath)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write server config: %v", err)
	}
}

func startSessionReaperServer(t *testing.T, binary, configPath, logPath string, mock bool) *exec.Cmd {
	t.Helper()
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create server log: %v", err)
	}
	cmd := exec.Command(binary, "--config", configPath, "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	if mock {
		cmd.Env = append(cmd.Env, "CONSENSUS_MOCK_LLM=1")
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start real consensus server: %v", err)
	}
	t.Cleanup(func() {
		stopSessionReaperServer(cmd)
		_ = logFile.Close()
	})
	return cmd
}

func stopSessionReaperServer(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func waitSessionReaperAdminKey(t *testing.T, logPath string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, field := range strings.Fields(readSessionReaperLog(logPath)) {
			if strings.HasPrefix(field, "key=cs_ak_") {
				return strings.Trim(strings.TrimSuffix(strings.TrimPrefix(field, "key="), ","), "\"'")
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("admin key not printed; server log:\n%s", readSessionReaperLog(logPath))
	return ""
}

func waitSessionReaperReady(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/openapi.json")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server did not become ready at %s", baseURL)
}

func sessionReaperRequest(t *testing.T, method, url, adminKey, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build API request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read API response: %v", err)
	}
	return resp.StatusCode, payload
}

func createSessionReaperSession(t *testing.T, baseURL, adminKey string) string {
	t.Helper()
	status, payload := sessionReaperRequest(t, http.MethodPost, baseURL+"/api/v1/sessions", adminKey,
		`{"agent_name":"reaper-integration","model_id":"gpt-4o","goal":"prove dead worker session claims expire"}`)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", status, payload)
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &response); err != nil || response.ID == "" {
		t.Fatalf("decode created session: err=%v body=%s", err, payload)
	}
	return response.ID
}

func sendSessionReaperMessage(t *testing.T, baseURL, adminKey, sessionID string) {
	t.Helper()
	status, payload := sessionReaperRequest(t, http.MethodPost,
		baseURL+"/api/v1/sessions/"+sessionID+"/message", adminKey,
		`{"type":"user_instruction","content":"hold the planning claim until the worker is killed"}`)
	if status < 200 || status >= 300 {
		t.Fatalf("send session message status=%d body=%s", status, payload)
	}
}

func sessionReaperStatus(t *testing.T, baseURL, adminKey, sessionID string) string {
	t.Helper()
	status, payload := sessionReaperRequest(t, http.MethodGet, baseURL+"/api/v1/sessions/"+sessionID, adminKey, "")
	if status != http.StatusOK {
		t.Fatalf("get session status=%d body=%s", status, payload)
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode session status: %v body=%s", err, payload)
	}
	return response.Status
}

func waitSessionReaperStatus(t *testing.T, baseURL, adminKey, sessionID, want string, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	var got string
	for time.Now().Before(deadline) {
		got = sessionReaperStatus(t, baseURL, adminKey, sessionID)
		if got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("session status=%q, want %q within %v", got, want, budget)
}

func readSessionReaperLog(path string) string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "read log: " + err.Error()
	}
	return string(payload)
}

// seedKilledWorkerToolRequest models the DF-CONSENSUS-68 hand-off: the worker
// claimed a tool request (status 'executing') and died before writing a result.
// The tool is a go_native stub so a recovered executor can actually run it.
func seedKilledWorkerToolRequest(t *testing.T, ctx context.Context, database db.DB, sessionID string) string {
	t.Helper()
	if err := database.Exec(ctx, `
		INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ('df68-recovery-tool', 'df68_recovery_tool', 'DF-CONSENSUS-68 recovery probe', 'external', 'go_native', 'DF68Handler', 1)
	`); err != nil {
		t.Fatalf("seed killed worker tool registry row: %v", err)
	}
	rows, err := database.Query(ctx, `
		INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, created_at, executed_at)
		VALUES ($1, 0, 'df68_recovery_tool', '{}', 'executing', datetime('now'), $2)
		RETURNING id
	`, sessionID, time.Now().Add(-5*time.Minute).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed killed worker tool request: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("seed killed worker tool request returned %d rows, want 1", len(rows))
	}
	requestID := toString(rows[0]["id"])
	if requestID == "" {
		t.Fatal("seed killed worker tool request returned an empty id")
	}
	t.Logf("seeded killed worker tool_request: id=%s session=%s status=executing", requestID, sessionID)
	return requestID
}

// waitKilledWorkerToolRequestSettled asserts the recovered tool request left
// 'executing' because a live executor claimed and ran it.
func waitKilledWorkerToolRequestSettled(t *testing.T, dbPath, requestID string, budget time.Duration) {
	t.Helper()
	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("open recovered database: %v", err)
	}
	defer func() { _ = database.Close() }()

	deadline := time.Now().Add(budget)
	var status string
	for time.Now().Before(deadline) {
		rows, err := database.Query(ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
		if err != nil {
			t.Fatalf("query recovered tool_request: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("recovered tool_request rows = %d, want 1", len(rows))
		}
		status = toString(rows[0]["status"])
		if status == "completed" || status == "failed" {
			t.Logf("PASS: recovered tool_request %s reached terminal status %q", requestID, status)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("recovered tool_request %s status=%q after %v; want a terminal status (pre-fix it rests at 'executing' forever)", requestID, status, budget)
}
