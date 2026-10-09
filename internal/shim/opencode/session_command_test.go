package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream session.command request body (openapi-1.18.33.json
// paths."/session/{sessionID}/command".post.requestBody: required
// ["arguments", "command"]).
func commandBody(command, arguments string) string {
	body, err := json.Marshal(map[string]any{
		"command":   command,
		"arguments": arguments,
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// postCommand is the doShimRequest equivalent for requests that carry a JSON
// body (doShimRequest always sends a nil body).
func postCommand(t *testing.T, base, path, body string) (int, http.Header, []byte) {
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
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, resp.Header, data
}

// TestSessionCommandServesCommandAgainstSession answers upstream
// session.command (ROUTE-FIX-009, SHIM-DRIFT-115, declared responses: 200
// {info: AssistantMessage, parts: []Part}, 400 BadRequest, 404 NotFound):
// POST /session/{id}/command with the required {command, arguments} body must
// execute the command against the session (the composed user instruction
// reaches the session's message store) and return the upstream response shape
// {info, parts} with the produced agent response — instead of the pre-fix
// untyped 501 stub.
func TestSessionCommandServesCommandAgainstSession(t *testing.T) {
	s, srv, conn := newCommandStoreTestServer(t)
	_ = s

	// Simulate the agent turn: when the composed command lands as a
	// user_message, persist the agent response for that iteration and settle
	// the session to idle (the same race the synchronous sendMessage test
	// handles — ServiceAdapter.waitForMessageResponse polls for it).
	writeResult := make(chan error, 1)
	go func() {
		// ROUTE-FIX-039 salvage gate: a hard 2s ceiling turned this into a
		// deterministic load-flake (the 5ms poll loop + DB write routinely
		// exceeds 2s at host load 40-60). Wait on the message with a
		// generous ceiling; the test still fails if the user_message
		// never lands (30s << go test's 10m package timeout).
		deadline := time.NewTimer(30 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			rows, err := conn.Query(context.Background(),
				`SELECT id FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, "s1")
			if err != nil {
				writeResult <- err
				return
			}
			if len(rows) > 0 {
				if err := conn.Exec(context.Background(),
					`INSERT INTO memory_events (type, content, session_id, iteration_created)
					 VALUES ('text_block', 'command executed: rental summary', 's1', 1)`); err != nil {
					writeResult <- err
					return
				}
				writeResult <- conn.Exec(context.Background(),
					`UPDATE sessions SET status = 'idle', iteration = 2 WHERE id = 's1'`)
				return
			}
			select {
			case <-deadline.C:
				writeResult <- context.DeadlineExceeded
				return
			case <-ticker.C:
			}
		}
	}()

	status, header, body := postCommand(t, srv.URL, "/session/s1/command", commandBody("explain", "the rental agreement"))
	if err := <-writeResult; err != nil {
		t.Fatalf("publish agent response: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("POST /session/s1/command: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got struct {
		Info  map[string]any `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be the upstream {info, parts} shape, got %q: %v", body, err)
	}
	if got.Info["role"] != "assistant" {
		t.Errorf("info.role = %v, want assistant", got.Info["role"])
	}
	if len(got.Parts) != 1 || got.Parts[0].Type != "text" || got.Parts[0].Text != "command executed: rental summary" {
		t.Errorf("parts = %#v, want the produced agent response", got.Parts)
	}

	// The command must have reached the session as a composed user
	// instruction ("/explain the rental agreement" upstream shape), not
	// vanished.
	rows, err := conn.Query(context.Background(),
		`SELECT content FROM memory_events WHERE session_id = 's1' AND type = 'user_message'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected the composed command stored as user_message, got %d rows (err=%v)", len(rows), err)
	}
	content := toString(rows[0]["content"])
	if !strings.Contains(content, "explain") || !strings.Contains(content, "rental agreement") {
		t.Errorf("composed command content = %q, want the command and its arguments", content)
	}
}

// TestSessionCommandValidationArms pins the declared 400 arm: a malformed
// JSON body and a body missing either required field (upstream requestBody
// requires ["arguments", "command"]) must answer 400 with the sibling
// error envelope — not 501, not a silent 200.
func TestSessionCommandValidationArms(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	cases := []struct {
		name string
		body string
		want string // required message fragment
	}{
		{"malformed json", `{"command": nope`, "malformed request body"},
		{"missing command", `{"arguments":"x"}`, "command and arguments are required"},
		{"missing arguments", `{"command":"explain"}`, "command and arguments are required"},
		{"blank command", `{"command":"  ","arguments":"x"}`, "command and arguments are required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := postCommand(t, srv.URL, "/session/s1/command", tc.body)
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

// TestSessionCommandUnknownSession404 pins the declared 404 arm: a command
// against a session that does not exist must answer 404 NOT_FOUND with the
// sibling envelope — the pre-fix handler answered the untyped 501 stub
// regardless of session existence.
func TestSessionCommandUnknownSession404(t *testing.T) {
	s, srv, _ := newMessageResponseTestServer(t) // real store: no session "missing"
	s.skipAuth = true

	status, header, body := postCommand(t, srv.URL, "/session/missing/command",
		`{"command":"explain","arguments":"x"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/missing/command: got %d, want 404. Body: %s", status, body)
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
		t.Fatalf("404 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code = %q, want NOT_FOUND", got.Error.Code)
	}
}

// TestSessionCommandSendFailureAnswersDeclared400 pins the residual declared
// arm: a session that exists but whose agent response cannot be produced must
// answer inside the declared contract (400 INVALID_REQUEST naming the
// failure), never the untyped 501 stub or an undeclared 5xx.
func TestSessionCommandSendFailureAnswersDeclared400(t *testing.T) {
	s, srv, _ := newCommandStoreTestServer(t)
	s.messageResponseTimeout = 40 * time.Millisecond // bounded no-response path

	status, _, body := postCommand(t, srv.URL, "/session/s1/command",
		`{"command":"explain","arguments":"x"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400. Body: %s", status, body)
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
	if strings.TrimSpace(got.Error.Message) == "" {
		t.Errorf("error.message empty, want a reason")
	}
}

// TestSessionCommandNeighboursUntouched guards the neighbours: the sibling
// stub subs still answer their pre-fix 501 for the methods they do not serve,
// and POST /session/{id}/message keeps its own contract (service-unavailable
// on a server without the native response service).
func TestSessionCommandNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	// Non-POST on the command sub-path keeps the pre-existing stub-list 501.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/session/s1/command", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /session/s1/command: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/command: got %d, want 501 (stub-list residual)", method, resp.StatusCode)
		}
	}

	// Sibling stub subs unchanged for undeclared methods; the served siblings
	// keep their own contracts. /session/{id}/fork left the stub list in
	// ROUTE-FIX-011 (SHIM-DRIFT-117); shell, summarize and revert left it in
	// ROUTE-FIX-017/018/014 — with an empty object body each answers 400
	// (missing required fields), which proves the routes reach their
	// handlers.
	for _, sub := range []string{"/session/s1/shell", "/session/s1/summarize", "/session/s1/revert"} {
		status, _, body := postCommand(t, srv.URL, sub, `{}`)
		if status != http.StatusBadRequest {
			t.Errorf("POST %s: got %d, want 400 (served for real, missing fields). Body: %s", sub, status, body)
		}
	}

	// sendMessage contract unchanged: 503 when no native response service.
	status, _, body := postCommand(t, srv.URL, "/session/s1/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Errorf("POST /session/s1/message: got %d, want 503. Body: %s", status, body)
	}
}

// TestSessionCommandChiMount is the BUG-009 regression for the new route: the
// command sub-path must reach the shim's own handler through a parent chi
// router mounted with MountPatterns (a shim-produced status proves the route
// is wired; chi's plain-text 404 would mean the mount lost it).
func TestSessionCommandChiMount(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := postCommand(t, srv.URL, "/session/s1/command",
		`{"command":"explain","arguments":"x"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/command via chi mount: got %d, want 400 (shim reached, svc missing). Body: %s", status, body)
	}
	if !strings.Contains(string(body), "INVALID_REQUEST") {
		t.Errorf("chi-mounted command body = %s, want the shim error envelope", body)
	}
}

// newCommandStoreTestServer builds a shim server with the real service adapter
// over a real SQLite store (full sessions schema) so the command round trip
// can be asserted against actual SQL.
func newCommandStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "command.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open command test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			agent_name TEXT NOT NULL,
			model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'booting',
			goal TEXT,
			context_budget INTEGER NOT NULL DEFAULT 128000,
			budget_limit_cents INTEGER NOT NULL DEFAULT 0,
			project_id TEXT,
			tokens_used_in INTEGER NOT NULL DEFAULT 0,
			tokens_used_out INTEGER NOT NULL DEFAULT 0,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			deleted_at TEXT
		)`,
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			key_prefix TEXT,
			scope TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT
		)`,
		`CREATE TABLE shim_session_map (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			shim_type TEXT NOT NULL,
			external_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL
		)`,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('s1', 'worker', 'm', 'idle', 'g', 0, '2026-09-30T00:00:00Z')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare command test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, NewServiceAdapter(api.NewService(conn, nil)))
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}
