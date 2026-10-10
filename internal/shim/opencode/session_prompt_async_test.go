package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// Upstream session.prompt_async request body (openapi-1.18.33.json
// paths."/session/{sessionID}/prompt_async".post.requestBody): the opencode
// message shape — {parts: [{type: "text", text: "..."}]}.
const promptAsyncBody = `{"parts":[{"type":"text","text":"run async"}]}`

// postSessionPromptAsync is the JSON-body POST helper for this row
// (doShimRequest always sends a nil body). It mirrors postCommand /
// postSessionInit: a fresh request per call, JSON content type, body read
// fully.
func postSessionPromptAsync(t *testing.T, base, path, body string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build POST %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data := make([]byte, 0, 512)
	buf := make([]byte, 512)
	for {
		n, err := resp.Body.Read(buf)
		data = append(data, buf[:n]...)
		if err != nil {
			break
		}
	}
	return resp.StatusCode, resp.Header, data
}

// TestSessionPromptAsyncEnqueuesAndAcknowledges answers upstream
// session.prompt_async (ROUTE-FIX-013, SHIM-DRIFT-113, declared responses:
// 204, 400, 404): POST /session/{id}/prompt_async is the fire-and-forget
// variant of POST /session/{id}/message — it must append the user message
// through the same message-send path (memory_events + the session wake) and
// answer 204 No Content without waiting for an agent response.
func TestSessionPromptAsyncEnqueuesAndAcknowledges(t *testing.T) {
	s, srv, conn := newSessionInitStoreTestServer(t)
	s.skipAuth = true
	ctx := context.Background()

	// The handler must not wait for an agent response: bound the whole round
	// trip well under any response-wait budget, then assert the user message
	// landed in the session's message store.
	done := make(chan struct {
		status int
		body   []byte
	}, 1)
	go func() {
		status, _, body := postSessionPromptAsync(t, srv.URL, "/session/s1/prompt_async", promptAsyncBody)
		done <- struct {
			status int
			body   []byte
		}{status, body}
	}()

	deadline := time.Now().Add(30 * time.Second)
	var content string
	for {
		rows, err := conn.Query(ctx,
			`SELECT content FROM memory_events WHERE session_id = 's1' AND type = 'user_message'`)
		if err != nil {
			t.Fatalf("read the async prompt from the store: %v", err)
		}
		if len(rows) > 0 {
			content = toString(rows[0]["content"])
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	res := <-done
	if res.status != http.StatusNoContent {
		t.Fatalf("POST /session/s1/prompt_async: got %d, want 204 (declared). Body: %s", res.status, res.body)
	}
	if len(res.body) != 0 {
		t.Errorf("204 body = %q, want empty (204 No Content)", res.body)
	}
	if content == "" {
		t.Fatalf("the async prompt never reached the session store as a user_message")
	}
	if content != "run async" {
		t.Errorf("stored prompt content = %q, want the submitted text \"run async\"", content)
	}

	// Fire-and-forget: the request was answered without any agent response in
	// the store (no text_block / agent_response was ever published).
	agentRows, err := conn.Query(ctx,
		`SELECT id FROM memory_events WHERE session_id = 's1' AND type IN ('text_block','agent_response')`)
	if err != nil {
		t.Fatalf("read agent events: %v", err)
	}
	if len(agentRows) != 0 {
		t.Errorf("prompt_async waited for or produced %d agent events, want 0 (enqueue and acknowledge)", len(agentRows))
	}

	// The same wake path the synchronous send uses: the session left 'idle'.
	// The enqueue goroutine may take a moment on a loaded host — poll briefly
	// before concluding the wake never happened.
	wakeDeadline := time.Now().Add(20 * time.Second)
	var status string
	for {
		statusRows, err := conn.Query(ctx, `SELECT status FROM sessions WHERE id = 's1'`)
		if err != nil || len(statusRows) != 1 {
			t.Fatalf("read session status: %v (rows=%d)", err, len(statusRows))
		}
		status = toString(statusRows[0]["status"])
		if status != "idle" || time.Now().After(wakeDeadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status == "idle" {
		t.Errorf("session status = idle after prompt_async, want the woken state the synchronous send sets")
	}
}

// TestSessionPromptAsyncValidationArms pins the declared 400 arm: a malformed
// JSON body and a body with no text content must answer 400 INVALID_REQUEST
// with the sibling error envelope — not 501, not a silent 204.
func TestSessionPromptAsyncValidationArms(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	cases := []struct {
		name string
		body string
		want string // required message fragment
	}{
		{"malformed json", `{"parts": nope`, "malformed request body"},
		{"empty body", ``, "malformed request body"},
		{"no parts", `{}`, "message content is empty"},
		{"no text part", `{"parts":[{"type":"file","path":"x"}]}`, "message content is empty"},
		{"blank text", `{"parts":[{"type":"text","text":"   "}]}`, "message content is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := postSessionPromptAsync(t, srv.URL, "/session/s1/prompt_async", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("got %d, want 400. Body: %s", status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error.code = %q, want INVALID_REQUEST", got.Error.Code)
			}
			if !strings.Contains(got.Error.Message, tc.want) {
				t.Errorf("error.message = %q, want it to name %q", got.Error.Message, tc.want)
			}
		})
	}
}

// TestSessionPromptAsyncUnknownSession404 pins the declared 404 arm: a prompt
// against a session that does not exist must answer 404 with the upstream SDK
// NamedError body (NotFoundError) — the pre-fix handler answered the untyped
// 501 stub regardless of session existence (SHIM-SUITE33-001).
func TestSessionPromptAsyncUnknownSession404(t *testing.T) {
	s, srv, _ := newSessionInitStoreTestServer(t) // real store: no session "missing"
	s.skipAuth = true

	status, header, body := postSessionPromptAsync(t, srv.URL, "/session/missing/prompt_async", promptAsyncBody)
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	assertNotFoundNamedError(t, "POST /session/missing/prompt_async", status, body)
}

// TestSessionPromptAsyncDeclaredContractOnly asserts the served status set is
// a subset of the operation's declared set (204, 400, 404) across every arm —
// the invariant SHIM-DRIFT-113 violated by answering 501.
func TestSessionPromptAsyncDeclaredContractOnly(t *testing.T) {
	declared := map[int]bool{
		http.StatusNoContent:  true,
		http.StatusBadRequest: true,
		http.StatusNotFound:   true,
	}
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	arms := []struct {
		name string
		path string
		body string
	}{
		{"valid body", "/session/s1/prompt_async", promptAsyncBody},
		{"malformed body", "/session/s1/prompt_async", `{"parts": nope`},
		{"unknown session", "/session/missing/prompt_async", promptAsyncBody},
	}
	for _, arm := range arms {
		status, _, body := postSessionPromptAsync(t, srv.URL, arm.path, arm.body)
		if !declared[status] {
			t.Errorf("%s: served %d, which the operation does not declare (204,400,404). Body: %s",
				arm.name, status, body)
		}
	}
}

// TestSessionPromptAsyncNeighboursUntouched guards the neighbours: non-POST
// methods on the prompt_async sub-path keep their pre-fix stub 501, the
// sibling stub subs are unchanged, and POST /session/{id}/message keeps its
// own contract (service-unavailable on a server without the native response
// service).
func TestSessionPromptAsyncNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/session/s1/prompt_async", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /session/s1/prompt_async: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/prompt_async: got %d, want 501 (stub-list residual)", method, resp.StatusCode)
		}
	}

	// ROUTE-FIX-011/012: fork and init are now served for real and are no
	// longer listed among the sibling stubs here. ROUTE-FIX-017/018: shell
	// and summarize are served for real now too — with an empty object body
	// both answer 400 (missing required fields).
	for _, sub := range []string{"/session/s1/shell", "/session/s1/summarize"} {
		status, _, body := postSessionPromptAsync(t, srv.URL, sub, `{}`)
		if status != http.StatusBadRequest {
			t.Errorf("POST %s: got %d, want 400 (served for real, missing fields). Body: %s", sub, status, body)
		}
	}

	status, _, body := postSessionPromptAsync(t, srv.URL, "/session/s1/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Errorf("POST /session/s1/message: got %d, want 503. Body: %s", status, body)
	}
}

// TestSessionPromptAsyncChiMount is the BUG-009 regression for the route: the
// prompt_async sub-path must reach the shim's own handler through a parent chi
// router mounted with MountPatterns (a shim-produced status proves the route
// is wired; chi's plain-text 404 would mean the mount lost it).
func TestSessionPromptAsyncChiMount(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// An empty body is a 400 from the shim's own validation — a shim-produced
	// status, which is the point of this arm.
	status, _, body := postSessionPromptAsync(t, srv.URL, "/session/s1/prompt_async", ``)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/prompt_async via chi mount: got %d, want 400 (shim reached, empty body). Body: %s", status, body)
	}
	if !strings.Contains(string(body), "INVALID_REQUEST") {
		t.Errorf("chi-mounted prompt_async body = %s, want the shim error envelope", body)
	}
}
