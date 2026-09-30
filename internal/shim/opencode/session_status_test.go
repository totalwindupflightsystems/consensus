package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// TestSessionStatus answers upstream session.status (ROUTE-FIX-008,
// SHIM-DRIFT-114, declared responses 200 map / 400 BadRequest): GET
// /session/status must return 200 with the upstream aggregate map
// {sessionID -> {type: idle|busy|retry}} over the same session store the
// single-session handler reads, an empty object when no sessions exist
// (upstream returns an object, never null), and the sibling-handler 400
// envelope for a malformed request — instead of the pre-fix typed 501.
//
// SessionStatus is an anyOf of three objects discriminated by `type`
// (upstream openapi-1.18.33.json components.schemas.SessionStatus):
//
//	idle  — {}
//	busy  — {}
//	retry — {attempt >= 0, message, next >= 0}
//
// The shim derives it from the sessions table status (SPEC-011 §1): idle,
// planning, thinking, tool_exec, waiting_sub, paused → {type: idle}; booting,
// executing, completed → {type: busy}; failed → {type: retry}.
func TestSessionStatus(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "s-idle", "agent_name": "worker", "status": "idle", "goal": "g",
				"iteration": int64(2), "tokens_used_in": int64(10), "tokens_used_out": int64(20),
				"created_at": "2026-09-30T00:00:00Z",
			}),
			rowOf(map[string]any{
				"id": "s-failed", "agent_name": "worker", "status": "failed", "goal": "g",
				"iteration": int64(3), "tokens_used_in": int64(0), "tokens_used_out": int64(0),
				"created_at": "2026-09-30T00:00:01Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/status")
	if status != http.StatusOK {
		t.Fatalf("GET /session/status: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be the aggregate status map, got %q: %v", body, err)
	}
	if len(got) != 2 {
		t.Fatalf("map carries %d entries, want 2: %v", len(got), got)
	}

	idle, ok := got["s-idle"]
	if !ok {
		t.Fatalf("map missing key s-idle: %v", got)
	}
	if idle["type"] != "idle" {
		t.Errorf("s-idle status = %v, want {\"type\":\"idle\"}", idle)
	}

	retry, ok := got["s-failed"]
	if !ok {
		t.Fatalf("map missing key s-failed: %v", got)
	}
	if retry["type"] != "retry" {
		t.Errorf("s-failed status = %v, want {\"type\":\"retry\",...}", retry)
	}
	for _, field := range []string{"attempt", "message", "next"} {
		if _, ok := retry[field]; !ok {
			t.Errorf("retry status missing required field %q (SessionStatus anyOf retry arm): %v", field, retry)
		}
	}
	if attempt, _ := retry["attempt"].(float64); attempt < 0 {
		t.Errorf("retry attempt = %v, must be >= 0", retry["attempt"])
	}
	if next, _ := retry["next"].(float64); next < 0 {
		t.Errorf("retry next = %v, must be >= 0", retry["next"])
	}
	if msg, _ := retry["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("retry message empty, want a reason string")
	}

	sawStatusQuery := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM sessions") && strings.Contains(q, "status") {
			sawStatusQuery = true
		}
	}
	if !sawStatusQuery {
		t.Errorf("expected a sessions store read, queries: %v", mdb.queries)
	}
}

// TestSessionStatusEmptyStore pins the empty-store arm: upstream session.status
// returns an object — the shim must answer {} (not null, not an error) when no
// sessions exist.
func TestSessionStatusEmptyStore(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/status")
	if status != http.StatusOK {
		t.Fatalf("GET /session/status on empty store: got %d, want 200. Body: %s", status, body)
	}
	if body := strings.TrimSpace(string(body)); body != "{}" {
		t.Errorf("empty store body = %q, want {}", body)
	}
}

// TestSessionStatusMethodNotAllowed pins that the reserved literal only serves
// GET: other methods keep the pre-existing behaviour of the generic not-found
// switch instead of silently reading the store.
func TestSessionStatusMethodNotAllowed(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/session/status", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /session/status: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s /session/status: got %d, want 404 (generic not-found switch)", method, resp.StatusCode)
		}
	}
}

// TestSessionStatusDBFailure answers the declared 400 arm the way sibling
// handlers do (writeOpencodeError INVALID_REQUEST): a session store that
// cannot be read must not surface a 500 or an empty 200.
func TestSessionStatusDBFailure(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryErr: fmt.Errorf("disk error")})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/status")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /session/status with failing store: got %d, want 400. Body: %s", status, body)
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
	if strings.TrimSpace(got.Error.Message) == "" {
		t.Errorf("error.message empty, want a reason")
	}
}

// TestSessionStatusRealStore round-trips the aggregate against a real SQLite
// sessions table: the map keys are the stored session ids and every stored
// status translates to a valid SessionStatus arm.
func TestSessionStatusRealStore(t *testing.T) {
	s, srv, conn := newStatusStoreTestServer(t)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, ins := range []string{
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('11111111-1111-4111-8111-111111111111', 'worker', 'm', 'idle', 'g', 1, '` + now + `')`,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('22222222-2222-4222-8222-222222222222', 'worker', 'm', 'failed', 'g', 2, '` + now + `')`,
	} {
		if err := conn.Exec(context.Background(), ins); err != nil {
			t.Fatalf("insert session: %v", err)
		}
	}

	_ = s
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/status")
	if status != http.StatusOK {
		t.Fatalf("GET /session/status: got %d, want 200. Body: %s", status, body)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the aggregate map: %v (%s)", err, body)
	}
	if len(got) != 2 {
		t.Fatalf("map carries %d entries, want 2: %v", len(got), got)
	}
	if got["11111111-1111-4111-8111-111111111111"]["type"] != "idle" {
		t.Errorf("idle row translated to %v", got["11111111-1111-4111-8111-111111111111"])
	}
	if got["22222222-2222-4222-8222-222222222222"]["type"] != "retry" {
		t.Errorf("failed row translated to %v", got["22222222-2222-4222-8222-222222222222"])
	}
}

// TestSessionStatusSingleSessionUntouched guards the neighbour: the reserved
// literal must not leak into real session ids — GET /session/<id> still reads
// that session, and a session literally named "status" is unreachable by
// design (opencode declares /session/status as the aggregate operation).
func TestSessionStatusSingleSessionUntouched(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{
			"id": "s-real", "agent_name": "w", "model_id": "m", "status": "idle", "goal": "g",
			"context_budget": int64(128000), "iteration": int64(0),
			"tokens_used_in": int64(0), "tokens_used_out": int64(0),
			"created_at": "2026-09-30T00:00:00Z", "heartbeat_at": "2026-09-30T00:00:00Z",
			"completed_at": nil,
		}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/s-real")
	if status != http.StatusOK {
		t.Fatalf("GET /session/s-real: got %d, want 200. Body: %s", status, body)
	}
	var single map[string]any
	if err := json.Unmarshal(body, &single); err != nil {
		t.Fatalf("body is not a session object: %v (%s)", err, body)
	}
	if single["id"] != "s-real" {
		t.Errorf("GET /session/{id} broken by the status route: %v", single)
	}
}

// newStatusStoreTestServer builds a shim server over a real database containing
// a real sessions table so the aggregate map can be asserted against actual SQL.
func newStatusStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "status.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open status test database: %v", err)
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
			tokens_used_in INTEGER NOT NULL DEFAULT 0,
			tokens_used_out INTEGER NOT NULL DEFAULT 0,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			created_at TEXT NOT NULL,
			completed_at TEXT
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
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare status test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

