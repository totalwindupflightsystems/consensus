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

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream session.todo (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/todo".get) declares exactly three responses: 200
// Array(Todo), 400 BadRequest | InvalidRequestError, 404 NotFoundError. Before
// ROUTE-FIX-039 the sub-path fell to handleSessionByID's router catch-all and
// answered an untyped 404 for EVERY request — the declared 200 (the todo list)
// was unreachable (SHIM-NARROWED-009, "router catch-all answers the declared
// 404 (no success path)").
//
// These tests are the declared-contract battery: every request to the
// operation must answer inside {200,400,404} — never 501, never an undeclared
// 5xx — for the happy path AND for each error path, with the 200 body shaped
// as the upstream Todo[] (required content, status, priority;
// additionalProperties: false) translated from the runtime's per-session
// `tasks` ledger (migrations 001/009).

// todoEntry is the upstream Todo schema used to prove the 200 body conforms
// field-for-field.
type todoEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// todoBody is the parsed 200 body plus its raw form (so the empty array can be
// distinguished from JSON null).
type todoBody struct {
	raw   string
	todos []map[string]any
}

func getTodo(t *testing.T, base, path string) (int, todoBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	var todos []map[string]any
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, &todos); err != nil {
			t.Fatalf("GET %s 200 body is not a Todo array: %v (%s)", path, err, data)
		}
	}
	return resp.StatusCode, todoBody{raw: string(data), todos: todos}
}

// todoEnvelope is the shim's error envelope, the declared
// BadRequest | InvalidRequestError / NotFoundError shape for this operation.
type todoEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeTodoEnvelope(t *testing.T, body string) todoEnvelope {
	t.Helper()
	var env todoEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("error body is not the shim envelope: %v (%s)", err, body)
	}
	return env
}

// TestSessionTodoServesDeclared200 answers the happy path: GET
// /session/{id}/todo returns 200 with the upstream Todo[] shape for a session
// that holds task rows, an empty ARRAY (never null) for a session with none,
// and the declared query selectors are accepted.
func TestSessionTodoServesDeclared200(t *testing.T) {
	_, srv, _ := newTodoStoreTestServer(t)

	t.Run("session with tasks translates every declared field", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesprobe1/todo")
		if status != http.StatusOK {
			t.Fatalf("GET /session/sesprobe1/todo: got %d, want 200 (declared). Body: %s", status, body.raw)
		}
		// 8 seeded rows: t1..t7 cover the status/priority mapping, t8 covers
		// the empty-title fallback.
		if len(body.todos) != 8 {
			t.Fatalf("todo list has %d entries, want the 8 seeded rows: %s", len(body.todos), body.raw)
		}
		want := []todoEntry{
			{Content: "wire the todo route", Status: "pending", Priority: "high"},
			{Content: "claimed arm", Status: "in_progress", Priority: "high"},
			{Content: "in progress arm", Status: "in_progress", Priority: "medium"},
			{Content: "reviewed arm", Status: "completed", Priority: "medium"},
			{Content: "published arm", Status: "completed", Priority: "low"},
			{Content: "failed arm", Status: "cancelled", Priority: "low"},
			{Content: "cancelled arm", Status: "cancelled", Priority: "medium"},
			{Content: "fallback description", Status: "pending", Priority: "medium"},
		}
		for i, entry := range body.todos {
			// additionalProperties: false — the declared Todo carries exactly
			// three keys, all of them required.
			if len(entry) != 3 {
				t.Errorf("entry %d has %d fields, want exactly content/status/priority: %v", i, len(entry), entry)
			}
			for _, field := range []string{"content", "status", "priority"} {
				if _, ok := entry[field]; !ok {
					t.Errorf("entry %d is missing the required field %q: %v", i, field, entry)
				}
			}
			got := todoEntry{
				Content:  toString(entry["content"]),
				Status:   toString(entry["status"]),
				Priority: toString(entry["priority"]),
			}
			if got != want[i] {
				t.Errorf("entry %d = %+v, want %+v", i, got, want[i])
			}
		}
	})

	t.Run("empty task list answers [] not null", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesempty/todo")
		if status != http.StatusOK {
			t.Fatalf("GET /session/sesempty/todo: got %d, want 200. Body: %s", status, body.raw)
		}
		if strings.TrimSpace(body.raw) != "[]" {
			t.Errorf("empty todo body = %q, want []", strings.TrimSpace(body.raw))
		}
	})

	t.Run("the list is scoped to the requested session", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesnotasks/todo")
		if status != http.StatusOK {
			t.Fatalf("GET /session/sesnotasks/todo: got %d, want 200. Body: %s", status, body.raw)
		}
		if len(body.todos) != 1 {
			t.Fatalf("sesnotasks todo list has %d entries, want exactly its own 1 row: %s", len(body.todos), body.raw)
		}
		if got := toString(body.todos[0]["content"]); got != "other session task" {
			t.Errorf("sesnotasks[0].content = %q, want %q", got, "other session task")
		}
	})

	t.Run("declared query selectors are accepted", func(t *testing.T) {
		for _, path := range []string{
			"/session/sesprobe1/todo?directory=/tmp",
			"/session/sesprobe1/todo?workspace=default",
			"/session/sesprobe1/todo?directory=/tmp&workspace=default",
		} {
			status, body := getTodo(t, srv.URL, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: got %d, want 200. Body: %s", path, status, body.raw)
			}
		}
	})

	// The declared ^ses pattern on the path parameter is deliberately NOT
	// enforced: Consensus mints session ids as UUIDs, so a ^ses gate would
	// refuse every real session. A UUID-shaped id must answer the declared 200,
	// not 400 — this is the regression that would make the route useless.
	t.Run("UUID session id is served, not pattern-refused", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/1a2b3c4d-0000-4000-8000-000000000000/todo")
		if status != http.StatusOK {
			t.Fatalf("GET /session/<uuid>/todo: got %d, want 200 — the declared ^ses pattern must not gate real session ids. Body: %s",
				status, body.raw)
		}
		if strings.TrimSpace(body.raw) != "[]" {
			t.Errorf("uuid session body = %q, want []", strings.TrimSpace(body.raw))
		}
	})
}

// TestSessionTodoErrorArmsAnswerDeclaredCodes answers every error path: an
// unknown session is the declared 404 NotFoundError, and a present-but-blank
// declared query selector is the declared 400 BadRequest (the operation
// declares no requestBody, so the query selectors are what keep the declared
// 400 arm reachable).
func TestSessionTodoErrorArmsAnswerDeclaredCodes(t *testing.T) {
	_, srv, _ := newTodoStoreTestServer(t)

	t.Run("unknown session answers the declared 404", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesmissing/todo")
		if status != http.StatusNotFound {
			t.Fatalf("GET /session/sesmissing/todo: got %d, want 404 (declared). Body: %s", status, body.raw)
		}
		env := decodeTodoEnvelope(t, body.raw)
		if env.Error.Code != "NOT_FOUND" {
			t.Errorf("error.code = %q, want NOT_FOUND", env.Error.Code)
		}
		if !strings.Contains(env.Error.Message, "session not found") {
			t.Errorf("error.message = %q, want it to name the missing session", env.Error.Message)
		}
	})

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"blank directory", "/session/sesprobe1/todo?directory=", "directory"},
		{"whitespace-only directory", "/session/sesprobe1/todo?directory=%20", "directory"},
		{"blank workspace", "/session/sesprobe1/todo?workspace=", "workspace"},
	} {
		t.Run(tc.name+" answers the declared 400", func(t *testing.T) {
			status, body := getTodo(t, srv.URL, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400 (declared). Body: %s", tc.path, status, body.raw)
			}
			env := decodeTodoEnvelope(t, body.raw)
			if env.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error.code = %q, want INVALID_REQUEST", env.Error.Code)
			}
			if !strings.Contains(env.Error.Message, tc.want) {
				t.Errorf("error.message = %q, want it to name %q", env.Error.Message, tc.want)
			}
		})
	}

	// Validation order: a malformed request is refused before the store read,
	// so a blank selector on an UNKNOWN session is the declared 400, not 404.
	t.Run("blank selector is refused before the session lookup", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesmissing/todo?directory=")
		if status != http.StatusBadRequest {
			t.Errorf("GET /session/sesmissing/todo?directory=: got %d, want 400. Body: %s", status, body.raw)
		}
	})

	// The error arms must not have swallowed the success path.
	t.Run("a valid session still answers 200", func(t *testing.T) {
		status, body := getTodo(t, srv.URL, "/session/sesprobe1/todo")
		if status != http.StatusOK {
			t.Fatalf("GET /session/sesprobe1/todo after the error arms: got %d, want 200. Body: %s", status, body.raw)
		}
	})

	// An undeclared method on the sub-path keeps the pre-existing router
	// default (405 is not in this operation's declared response set).
	t.Run("undeclared method keeps the pre-existing 404", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/sesprobe1/todo", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("build POST /session/sesprobe1/todo: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /session/sesprobe1/todo: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST /session/sesprobe1/todo: got %d, want 404 (only GET is declared). Body: %s", resp.StatusCode, data)
		}
	})

	// Neighbour guard: the sibling sub-paths keep their own contracts.
	t.Run("siblings untouched", func(t *testing.T) {
		if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/sesprobe1"); status != http.StatusOK {
			t.Errorf("GET /session/sesprobe1: got %d, want 200. Body: %s", status, body)
		}
		if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/sesprobe1/message"); status != http.StatusOK {
			t.Errorf("GET /session/sesprobe1/message: got %d, want 200. Body: %s", status, body)
		}
	})
}

// TestSessionTodoNeverAnswers501 is the ROUTE-FIX-039 criterion in one
// battery: on the declared operation, no request — happy path or error path —
// may answer 501 or any code outside the declared {200,400,404} set.
func TestSessionTodoNeverAnswers501(t *testing.T) {
	_, srv, _ := newTodoStoreTestServer(t)

	for _, path := range []string{
		"/session/sesprobe1/todo",
		"/session/sesnotasks/todo",
		"/session/sesprobe1/todo?directory=/tmp",
		"/session/sesprobe1/todo?workspace=",
		"/session/sesprobe1/todo?directory=",
		"/session/sesmissing/todo",
		"/session/sesmissing/todo?directory=",
	} {
		status, body := getTodo(t, srv.URL, path)
		if status == http.StatusNotImplemented {
			t.Errorf("GET %s answered 501 — SHIM-NARROWED-009 regression. Body: %s", path, body.raw)
			continue
		}
		switch status {
		case http.StatusOK, http.StatusBadRequest, http.StatusNotFound:
		default:
			t.Errorf("GET %s: got %d, want a declared code (200, 400 or 404). Body: %s", path, status, body.raw)
		}
	}
}

// TestSessionTodoChiMountMatchesProductionWiring is the BUG-009-shaped
// regression for this route: through a parent chi router mounted with
// MountPatterns (the shape cmd/consensus/main.go uses), /session/{id}/todo must
// reach the shim handler instead of chi's 404 — and must answer the declared
// contract there too.
func TestSessionTodoChiMountMatchesProductionWiring(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}
	shim := NewServer(mdb, "test-key", nil, nil)
	shim.skipAuth = true

	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/session/s1/todo", nil)
	if err != nil {
		t.Fatalf("build GET /session/s1/todo: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /session/s1/todo via chi mount: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /session/s1/todo returned 404 — MountPatterns did not pass the sub-path through, or the router catch-all answered. Body: %s", data)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /session/s1/todo via chi mount: got %d, want 200. Body: %s", resp.StatusCode, data)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("chi-mounted todo body = %q, want [] (the mock store holds no tasks)", strings.TrimSpace(string(data)))
	}
}

// newTodoStoreTestServer builds a shim server over a real SQLite store with the
// sessions and tasks schema, so the session lookup and the task-ledger
// translation both run against actual SQL.
func newTodoStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "todo.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open todo test database: %v", err)
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
			project_id TEXT,
			heartbeat_at TEXT,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			deleted_at TEXT
		)`,
		// The runtime's per-session action-item ledger (migrations 001/009).
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			parent_task_id TEXT,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'pending'
			  CHECK (status IN ('pending','claimed','in_progress','reviewed','published','failed','cancelled')),
			priority INT NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
			locked_by_agent TEXT,
			prerequisite_ids TEXT NOT NULL DEFAULT '[]',
			result_memory_id INTEGER,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			claimed_at TEXT,
			completed_at TEXT
		)`,
		// The session message store, so the sibling GET /session/{id}/message
		// neighbour probe runs against real SQL too.
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('user_message', 'todo probe', 'sesprobe1', 1)`,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('sesprobe1', 'worker', 'm', 'idle', 'probe', 0, '2026-10-03T00:00:00Z')`,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('sesnotasks', 'worker', 'm', 'idle', 'probe', 0, '2026-10-03T00:00:00Z')`,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('sesempty', 'worker', 'm', 'idle', 'probe', 0, '2026-10-03T00:00:00Z')`,
		// A UUID-shaped session id — the shape the runtime actually mints.
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('1a2b3c4d-0000-4000-8000-000000000000', 'worker', 'm', 'idle', 'probe', 0, '2026-10-03T00:00:00Z')`,
		// One row per declared mapping arm; created_at pins the ORDER BY.
		`INSERT INTO tasks (id, session_id, title, description, status, priority, created_at) VALUES
		 ('t1','sesprobe1','wire the todo route',NULL,'pending',1,'2026-10-03T00:00:01Z'),
		 ('t2','sesprobe1','claimed arm','','claimed',3,'2026-10-03T00:00:02Z'),
		 ('t3','sesprobe1','in progress arm',NULL,'in_progress',4,'2026-10-03T00:00:03Z'),
		 ('t4','sesprobe1','reviewed arm',NULL,'reviewed',7,'2026-10-03T00:00:04Z'),
		 ('t5','sesprobe1','published arm',NULL,'published',8,'2026-10-03T00:00:05Z'),
		 ('t6','sesprobe1','failed arm',NULL,'failed',10,'2026-10-03T00:00:06Z'),
		 ('t7','sesprobe1','cancelled arm',NULL,'cancelled',5,'2026-10-03T00:00:07Z'),
		 ('t8','sesprobe1','','fallback description','pending',5,'2026-10-03T00:00:08Z'),
		 ('t9','sesnotasks','other session task',NULL,'pending',1,'2026-10-03T00:00:09Z')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare todo test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}
