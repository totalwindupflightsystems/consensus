// Package chronicle: full session lifecycle integration test through the
// Consensus OpenCode shim (SPEC-017). Proves the shim → session create →
// message send → LLM round-trip → memory event pipeline end-to-end.
//
// axiom:trace work_item=INT-001 spec=specs/017-ui-adapter-layer.md test=internal/chronicle/shim_real_llm_test.go
package chronicle

import (
	"bufio"
	"bytes"
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

// TestShimRealLLMSessionLifecycle drives a complete session through the
// Consensus OpenCode shim against a real LLM (DeepSeek). The test exercises:
//
//  1. Building the consensus binary from source.
//  2. Bootstrapping an admin key via `init --db-url sqlite://...`.
//  3. Starting `serve --adapter opencode --config <yaml>` on a high port.
//  4. Polling /global/health until the shim is ready.
//  5. Creating a session with POST /session using the OpenCode title/goal
//     body shape.
//  6. Sending a user message with POST /session/{id}/message using the
//     OpenCode parts array shape.
//  7. Polling GET /session/{id} until status == "idle" and iteration > 0
//     (or timeout at 120s).
//  8. Listing memory via GET /event and asserting at least one non-empty
//     event payload exists.
//
// The test is skipped in -short mode and when DEEPSEEK_API_KEY is unset.
func TestShimRealLLMSessionLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shim real-LLM lifecycle test in short mode")
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set — skipping shim real-LLM integration test")
	}

	binPath := findBinary(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "shim-int.db")
	port := 8198

	// ── Config: same model registry the e2e test uses, so the harness picks
	// up a chat-capable model that the OpenCode adapter can route to.
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

	// ── 1. init: bootstrap the SQLite DB and capture the admin key.
	initCmd := exec.Command(binPath, "init", "--db-url", "sqlite://"+dbPath)
	initCmd.Dir = tmpDir
	initOut, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, initOut)
	}
	adminKey := extractAdminKey(t, string(initOut))
	t.Logf("admin key: %s...", adminKey[:20])

	// ── 2. serve: launch the OpenCode shim adapter on a high port.
	ctx := t.Context()
	serveCmd := exec.CommandContext(ctx, binPath, "serve",
		"--adapter", "opencode",
		"--config", configPath,
		"--port", fmt.Sprintf("%d", port),
	)
	serveCmd.Dir = tmpDir
	serveCmd.Stdout = os.Stdout
	serveCmd.Stderr = os.Stderr
	if err := serveCmd.Start(); err != nil {
		t.Fatalf("start shim serve: %v", err)
	}
	defer serveCmd.Process.Kill()

	apiBase := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 30 * time.Second}

	// ── 3. Wait for shim health on /global/health (OpenCode protocol path).
	if !waitForShimHealth(t, client, apiBase, 30*time.Second) {
		t.Fatal("shim did not become healthy at /global/health")
	}

	// ── 4. POST /session — OpenCode protocol: {title, goal} returns
	//    {id, status, ...}. Admin key authorizes as Bearer.
	sessionID := shimCreateSession(t, client, apiBase, adminKey,
		"Shim Integration Test",
		"Verify shim integration with real LLM")

	// ── 5. POST /session/{id}/message — OpenCode parts format:
	//    {parts: [{type: "text", text: "..."}]}
	shimSendMessage(t, client, apiBase, sessionID, adminKey,
		"Say hello and tell me what model you are.")

	// ── 6. Poll GET /session/{id} until status == "idle" & iteration > 0
	//    (or 120s timeout — LLM round-trip + harness iteration).
	if !waitForShimSessionIdle(t, client, apiBase, sessionID, adminKey, 120*time.Second) {
		t.Fatal("shim session did not reach idle with iteration > 0 within 120s")
	}

	// ── 7. GET /event — memory event stream (OpenCode protocol). Verify
	//    at least one event with non-empty content exists.
	events := shimListEvents(t, client, apiBase, adminKey)
	if len(events) == 0 {
		t.Fatal("no events returned from /event — LLM produced no memory")
	}

	nonEmpty := 0
	for _, ev := range events {
		evType, _ := ev["type"].(string)
		evKind, _ := ev["event_type"].(string)
		content := firstNonEmptyString(ev, "content", "text", "payload", "body", "data")
		if content != "" {
			nonEmpty++
			t.Logf("  event type=%s kind=%s content=%.120s", evType, evKind, content)
		}
	}
	if nonEmpty == 0 {
		t.Fatalf("got %d events but none had non-empty content — pipeline broken", len(events))
	}

	t.Logf("✅ shim real-LLM lifecycle complete: session=%s events=%d (non-empty=%d)",
		sessionID, len(events), nonEmpty)
}

// ── Shim-specific helpers ──────────────────────────────────────────────
//
// These are intentionally separate from the e2e helpers because the shim
// uses the OpenCode protocol (different body shapes, /global/health, /event,
// parts-based message format). The e2e helpers above are reused only where
// the contract matches (findBinary, extractAdminKey).

// waitForShimHealth polls the OpenCode shim's /global/health endpoint until
// it returns 200 (or the deadline elapses).
func waitForShimHealth(t *testing.T, client *http.Client, apiBase string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := apiBase + "/global/health"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Logf("/global/health 200 — %s", strings.TrimSpace(string(body)))
				return true
			}
			t.Logf("  /global/health: %d", resp.StatusCode)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// shimCreateSession POSTs to /session with the OpenCode body shape
// {title, goal}. The shim accepts the admin key as Bearer auth.
func shimCreateSession(t *testing.T, client *http.Client, apiBase, adminKey, title, goal string) string {
	t.Helper()
	body := map[string]any{"title": title, "goal": goal}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, apiBase+"/session", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("POST /session returned %d: %s", resp.StatusCode, respBody)
	}
	var session map[string]any
	if err := json.Unmarshal(respBody, &session); err != nil {
		t.Fatalf("decode session: %v — body: %s", err, respBody)
	}
	id, _ := session["id"].(string)
	if id == "" {
		t.Fatalf("no session id in response: %s", respBody)
	}
	t.Logf("created shim session id=%s", id)
	return id
}

// shimSendMessage POSTs to /session/{id}/message using the OpenCode parts
// array format. The shim triggers the harness to invoke the LLM and write
// the response into memory.
func shimSendMessage(t *testing.T, client *http.Client, apiBase, sessionID, adminKey, text string) {
	t.Helper()
	body := map[string]any{
		"parts": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/session/%s/message", apiBase, sessionID)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /session/%s/message: %v", sessionID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("POST /session/%s/message returned %d: %s", sessionID, resp.StatusCode, respBody)
	}
	t.Logf("message sent to shim session %s — %d bytes response", sessionID, len(respBody))
}

// waitForShimSessionIdle polls GET /session/{id} until status == "idle" and
// iteration > 0. Returns false on timeout.
func waitForShimSessionIdle(t *testing.T, client *http.Client, apiBase, sessionID, adminKey string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("%s/session/%s", apiBase, sessionID)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+adminKey)
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Logf("  GET /session/%s: %d — %s", sessionID, resp.StatusCode, strings.TrimSpace(string(body)))
			time.Sleep(1 * time.Second)
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		status, _ := data["status"].(string)
		iter := asFloat(data["iteration"])
		t.Logf("  shim session status=%s iteration=%.0f", status, iter)
		if status == "idle" && iter > 0 {
			return true
		}
		if status == "failed" {
			t.Logf("session entered failed state: %s", string(body))
			return false
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// shimListEvents GETs the OpenCode shim /event endpoint and returns the
// event list. The shim streams text/event-stream frames (event:/data:
// blocks), which never EOF — so this helper parses frames with a bounded
// idle deadline (5s of no new data) instead of io.ReadAll. Each frame
// becomes {"type": <event type>, ...unmarshaled data JSON...}. Defensive
// JSON array/wrapped shape parsing is kept as a fallback for non-SSE
// responses.
func shimListEvents(t *testing.T, client *http.Client, apiBase, adminKey string) []map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, apiBase+"/event", nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /event returned %d: %s", resp.StatusCode, body)
	}

	// SSE: parse event:/data: frames with a bounded idle deadline.
	const sseIdleTimeout = 5 * time.Second
	events, rawBody := readSSEFrames(t, resp.Body, sseIdleTimeout)
	if len(events) > 0 {
		t.Logf("/event returned %d events (SSE frames)", len(events))
		return events
	}

	// Defensive fallback: non-SSE JSON array/wrapped shapes. rawBody holds
	// whatever bytes the frame reader saw before the stream went quiet.
	body := rawBody
	if len(body) == 0 {
		body, _ = io.ReadAll(resp.Body) // bounded by client Timeout
	}
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err == nil {
		t.Logf("/event returned %d events (array shape)", len(arr))
		return arr
	}
	var wrapped map[string]any
	if err := json.Unmarshal(body, &wrapped); err == nil {
		if evs, ok := wrapped["events"].([]any); ok {
			out := make([]map[string]any, 0, len(evs))
			for _, e := range evs {
				if m, ok := e.(map[string]any); ok {
					out = append(out, m)
				}
			}
			t.Logf("/event returned %d events (wrapped shape)", len(out))
			return out
		}
	}
	t.Logf("WARNING: /event body did not match SSE, array or wrapped shape: %s", string(body)[:min(200, len(body))])
	return nil
}

// readSSEFrames parses a text/event-stream into event maps. Lines starting
// with "event:" set the frame type; "data:" lines accumulate the JSON
// payload; a blank line terminates a frame. Returns each frame as a map with
// "type" set from the event: line and the decoded data JSON merged in.
// Reading stops after idle of no new data (the live stream never EOFs). The
// raw bytes seen are returned too, so callers can fall back to JSON parsing
// for non-SSE bodies.
func readSSEFrames(t *testing.T, r io.Reader, idle time.Duration) ([]map[string]any, []byte) {
	t.Helper()
	dr := &deadlineReader{r: r, idle: idle}
	br := bufio.NewReader(dr)
	var (
		events  []map[string]any
		raw     []byte
		evType  string
		dataLns []string
	)
	flushFrame := func() {
		if len(dataLns) == 0 && evType == "" {
			return
		}
		m := map[string]any{}
		if evType != "" {
			m["type"] = evType
		}
		payload := strings.Join(dataLns, "\n")
		if payload != "" {
			var decoded any
			if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
				if dm, ok := decoded.(map[string]any); ok {
					for k, v := range dm {
						m[k] = v
					}
				} else {
					m["data"] = decoded
				}
			} else {
				m["data"] = payload
			}
		}
		events = append(events, m)
		evType = ""
		dataLns = nil
	}
	for {
		line, err := br.ReadString('\n')
		raw = append(raw, line...)
		if err != nil {
			// Idle timeout or EOF — finalize any partial frame and return.
			flushFrame()
			return events, raw
		}
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "event:"):
			evType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		case strings.HasPrefix(trimmed, "data:"):
			dataLns = append(dataLns, strings.TrimPrefix(trimmed, "data:"))
		case trimmed == "":
			flushFrame()
		}
	}
}

// deadlineReader wraps an io.Reader so a Read that produces no data within
// idle returns os.ErrDeadlineExceeded — used to bound reads from an SSE
// stream that never EOFs. (Goroutine-per-read; acceptable in a test helper,
// and the deferred resp.Body.Close() unblocks it.)
type deadlineReader struct {
	r    io.Reader
	idle time.Duration
}

func (d *deadlineReader) Read(p []byte) (int, error) {
	ch := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := d.r.Read(p)
		ch <- struct {
			n   int
			err error
		}{n, err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(d.idle):
		return 0, os.ErrDeadlineExceeded
	}
}

// ── Tiny utilities ────────────────────────────────────────────────────

// firstNonEmptyString returns the first non-empty string value among the
// given keys in the map.
func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	// Fall back: stringify any non-nil value via JSON.
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			b, err := json.Marshal(v)
			if err == nil {
				s := strings.TrimSpace(string(b))
				if s != "" && s != "null" && s != "{}" && s != "[]" {
					return s
				}
			}
		}
	}
	return ""
}

// asFloat coerces numeric JSON values (int64, float64, int) to float64.
func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

// min returns the smaller of two ints (kept as a named helper to avoid
// relying on the builtin min for older toolchains that might parse this
// file under a different go.mod).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
