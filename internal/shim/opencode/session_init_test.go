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

// Upstream session.init request body (openapi-1.18.33.json
// paths."/session/{sessionID}/init".post.requestBody: required
// ["modelID", "providerID", "messageID"], additionalProperties false).
func initBody(modelID, providerID, messageID string) string {
	body, err := json.Marshal(map[string]any{
		"modelID":    modelID,
		"providerID": providerID,
		"messageID":  messageID,
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// postSessionInit is the JSON-body POST helper for this row (doShimRequest
// always sends a nil body).
func postSessionInit(t *testing.T, base, path, body string) (int, http.Header, []byte) {
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

// sessionInitStoreBound bounds the wait for the init directive to reach the
// session store. It is deliberately generous: a loaded host is slow to hand
// out store connections (the sibling /session/{id}/command test's 2s timer is
// exactly the flake class QA-CONSENSUS-22 tracked), and that wait is not what
// this row tests.
const sessionInitStoreBound = 45 * time.Second

// TestSessionInitServesDeclaredBoolean answers upstream session.init
// (ROUTE-FIX-012, SHIM-DRIFT-118, declared responses: 200 boolean, 400
// BadRequest | InvalidRequestError, 404 NotFoundError): POST
// /session/{id}/init with the required {modelID, providerID, messageID} body
// must deliver the init directive to the session (it reaches the session's
// message store as a user instruction) and return the declared 200 body — a
// JSON boolean — instead of the pre-fix untyped 501 stub.
//
// The handler answers only once the init turn has been produced, so the
// request is driven from a goroutine while this goroutine plays the harness:
// it waits for the directive to land, publishes the produced instructions at
// the turn's iteration, and settles the session to idle.
func TestSessionInitServesDeclaredBoolean(t *testing.T) {
	_, srv, conn := newSessionInitStoreTestServer(t)
	ctx := context.Background()

	type httpResult struct {
		status int
		header http.Header
		body   []byte
		err    error
	}
	done := make(chan httpResult, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/s1/init",
			strings.NewReader(initBody("probe-model", "probe-provider", "msg_probe_1")))
		if err != nil {
			done <- httpResult{err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- httpResult{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		done <- httpResult{status: resp.StatusCode, header: resp.Header, body: data, err: err}
	}()

	deadline := time.Now().Add(sessionInitStoreBound)
	var content string
	for {
		rows, err := conn.Query(ctx,
			`SELECT content FROM memory_events WHERE session_id = 's1' AND type = 'user_message'`)
		if err != nil {
			t.Fatalf("read the init directive from the store: %v", err)
		}
		if len(rows) > 0 {
			content = toString(rows[0]["content"])
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the init directive never reached the session store within %s", sessionInitStoreBound)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The directive must be the upstream initialization directive, not an
	// empty or unrelated turn.
	if !strings.Contains(content, "AGENTS.md") {
		t.Errorf("init directive content = %q, want the upstream initialization directive", content)
	}

	// Publish the turn the handler is waiting for: the produced project
	// instructions at the turn's iteration, then the session settled to idle
	// (what the harness does in one transaction).
	if err := conn.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'session initialized: project instructions produced', 's1', 1)`); err != nil {
		t.Fatalf("publish agent response: %v", err)
	}
	if err := conn.Exec(ctx,
		`UPDATE sessions SET status = 'idle', iteration = 2 WHERE id = 's1'`); err != nil {
		t.Fatalf("settle session: %v", err)
	}

	res := <-done
	if res.err != nil {
		t.Fatalf("POST /session/s1/init: %v", res.err)
	}
	if res.status != http.StatusOK {
		t.Fatalf("POST /session/s1/init: got %d, want 200. Body: %s", res.status, res.body)
	}
	if ct := res.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// The declared 200 body is the operation's plain boolean.
	var got bool
	if err := json.Unmarshal(res.body, &got); err != nil {
		t.Fatalf("200 body must be the declared boolean, got %q: %v", res.body, err)
	}
	if !got {
		t.Errorf("200 body = %v, want true", got)
	}

	// The init directive must have reached the session as a user instruction,
	// not vanished: exactly one, carrying the upstream directive.
	rows, err := conn.Query(ctx,
		`SELECT content FROM memory_events WHERE session_id = 's1' AND type = 'user_message'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected exactly one stored init directive, got %d rows (err=%v)", len(rows), err)
	}
}

// TestSessionInitValidationArms pins the declared 400 arm: a malformed JSON
// body and a body missing any of the three required fields (upstream
// requestBody requires [modelID, providerID, messageID]) must answer 400 with
// the sibling error envelope — not 501, not a silent 200.
func TestSessionInitValidationArms(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	cases := []struct {
		name string
		body string
		want string // required message fragment
	}{
		{"malformed json", `{"modelID": nope`, "malformed request body"},
		{"empty body", ``, "malformed request body"},
		{"missing modelID", `{"providerID":"p","messageID":"m"}`, "modelID, providerID and messageID are required"},
		{"missing providerID", `{"modelID":"m","messageID":"msg_1"}`, "modelID, providerID and messageID are required"},
		{"missing messageID", `{"modelID":"m","providerID":"p"}`, "modelID, providerID and messageID are required"},
		{"blank modelID", `{"modelID":"  ","providerID":"p","messageID":"msg_1"}`, "modelID, providerID and messageID are required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := postSessionInit(t, srv.URL, "/session/s1/init", tc.body)
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

// TestSessionInitUnknownSession404 pins the declared 404 arm: an init against
// a session that does not exist must answer 404 with the upstream SDK
// NamedError body (NotFoundError) — the pre-fix handler answered the untyped
// 501 stub regardless of session existence (SHIM-SUITE33-001).
func TestSessionInitUnknownSession404(t *testing.T) {
	s, srv, _ := newSessionInitStoreTestServer(t) // real store: no session "missing"
	s.skipAuth = true

	status, header, body := postSessionInit(t, srv.URL, "/session/missing/init",
		initBody("m", "p", "msg_1"))
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	assertNotFoundNamedError(t, "POST /session/missing/init", status, body)
}

// TestSessionInitUnproducibleTurnAnswersDeclared400 pins the residual declared
// arm: a session that exists but whose init turn cannot be produced must
// answer inside the declared contract (400 INVALID_REQUEST naming the failure),
// never the untyped 501 stub or an undeclared 5xx.
func TestSessionInitUnproducibleTurnAnswersDeclared400(t *testing.T) {
	s, srv, _ := newSessionInitStoreTestServer(t)
	s.messageResponseTimeout = 40 * time.Millisecond // bounded no-response path

	status, _, body := postSessionInit(t, srv.URL, "/session/s1/init",
		initBody("m", "p", "msg_1"))
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

// TestSessionInitDeclaredContractOnly asserts the served status set is a
// subset of the operation's declared set (200, 400, 404) across every arm —
// the invariant SHIM-DRIFT-118 violated by answering 501.
func TestSessionInitDeclaredContractOnly(t *testing.T) {
	declared := map[int]bool{
		http.StatusOK:         true,
		http.StatusBadRequest: true,
		http.StatusNotFound:   true,
	}
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	arms := []struct {
		name string
		body string
	}{
		{"valid body", initBody("m", "p", "msg_1")},
		{"malformed body", `{"modelID": nope`},
	}
	for _, arm := range arms {
		status, _, body := postSessionInit(t, srv.URL, "/session/s1/init", arm.body)
		if !declared[status] {
			t.Errorf("%s: served %d, which the operation does not declare (200,400,404). Body: %s",
				arm.name, status, body)
		}
	}
}

// TestSessionInitNeighboursUntouched guards the neighbours: non-POST methods
// on the init sub-path keep their pre-fix stub 501, the sibling stub subs are
// unchanged, and POST /session/{id}/message keeps its own contract
// (service-unavailable on a server without the native response service).
func TestSessionInitNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/session/s1/init", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /session/s1/init: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/init: got %d, want 501 (stub-list residual)", method, resp.StatusCode)
		}
	}

	// ROUTE-FIX-011: /session/{id}/fork is now served for real (no longer a
	// 501 stub), so it is no longer listed among the sibling stubs here.
	// ROUTE-FIX-017/018: shell and summarize are served for real now too —
	// with an empty object body both answer 400 (missing required fields).
	for _, sub := range []string{"/session/s1/shell", "/session/s1/summarize"} {
		status, _, body := postSessionInit(t, srv.URL, sub, `{}`)
		if status != http.StatusBadRequest {
			t.Errorf("POST %s: got %d, want 400 (served for real, missing fields). Body: %s", sub, status, body)
		}
	}

	status, _, body := postSessionInit(t, srv.URL, "/session/s1/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Errorf("POST /session/s1/message: got %d, want 503. Body: %s", status, body)
	}
}

// TestSessionInitChiMount is the BUG-009 regression for the route: the init
// sub-path must reach the shim's own handler through a parent chi router
// mounted with MountPatterns (a shim-produced status proves the route is
// wired; chi's plain-text 404 would mean the mount lost it).
func TestSessionInitChiMount(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := postSessionInit(t, srv.URL, "/session/s1/init",
		initBody("m", "p", "msg_1"))
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/init via chi mount: got %d, want 400 (shim reached, svc missing). Body: %s", status, body)
	}
	if !strings.Contains(string(body), "INVALID_REQUEST") {
		t.Errorf("chi-mounted init body = %s, want the shim error envelope", body)
	}
}

// newSessionInitStoreTestServer builds a shim server with the real service
// adapter over a real SQLite store (full sessions schema) so the init round
// trip can be asserted against actual SQL.
func newSessionInitStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "init.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open init test database: %v", err)
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
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('s1', 'worker', 'm', 'idle', 'g', 0, '2026-09-30T00:00:00Z')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare init test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, NewServiceAdapter(api.NewService(conn, nil)))
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}
