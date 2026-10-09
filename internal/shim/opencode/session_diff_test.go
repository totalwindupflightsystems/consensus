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
	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream session.diff (openapi-1.18.33.json
// paths."/session/{sessionID}/diff".get) declares exactly two responses: 200
// Array(SnapshotFileDiff) and 400 BadRequest. The shim answered the typed 501
// not-implemented envelope from handleSessionByID's `sub == "diff"` arm
// (SHIM-DRIFT-116: class OUTCOME-MISMATCH, "declared 200,400, served 501")
// before ROUTE-FIX-010.
//
// These tests are the declared-contract battery: every request to the
// operation must answer inside {200,400} — never 501, never an undeclared
// 404/405 — for the happy path AND for each error path, with the 200 body
// shaped as the upstream SnapshotFileDiff[] and with the required schema
// fields present.

// diffBody is the parsed 200 body plus its raw form (so an empty array can be
// distinguished from JSON null).
type diffBody struct {
	raw   string
	files []map[string]any
}

func getDiff(t *testing.T, base, path string, headers map[string]string) (int, http.Header, diffBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	var files []map[string]any
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, &files); err != nil {
			t.Fatalf("GET %s 200 body is not a SnapshotFileDiff array: %v (%s)", path, err, data)
		}
	}
	return resp.StatusCode, resp.Header, diffBody{raw: string(data), files: files}
}

// TestSessionDiffServesDeclaredContract answers the happy path: GET
// /session/{id}/diff returns 200 with the upstream SnapshotFileDiff[] shape for
// the workspace the request names (the x-opencode-directory header, then the
// operation's declared ?directory= selector), and an empty array — never null,
// never 501 — for a workspace with no changes.
func TestSessionDiffServesDeclaredContract(t *testing.T) {
	s, srv, _ := newDiffStoreTestServer(t)
	repo := makeGitRepo(t)
	// The server's own workdir is deliberately NOT the fixture repo: both the
	// header and the ?directory= selector must resolve the requested tree.
	s.workdir = t.TempDir()

	t.Run("header selects the workspace", func(t *testing.T) {
		status, header, body := getDiff(t, srv.URL, "/session/s1/diff", map[string]string{"x-opencode-directory": repo})
		if status != http.StatusOK {
			t.Fatalf("GET /session/s1/diff: got %d, want 200 (declared). Body: %s", status, body.raw)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		byFile := map[string]map[string]any{}
		for _, f := range body.files {
			name, _ := f["file"].(string)
			byFile[name] = f
		}
		for _, want := range []string{"dirty.txt", "untracked.txt"} {
			entry, ok := byFile[want]
			if !ok {
				t.Fatalf("%s missing from the session diff: %v", want, body.files)
			}
			// SnapshotFileDiff: additions and deletions are required numbers,
			// status is the declared enum, and additionalProperties is false.
			for _, field := range []string{"additions", "deletions"} {
				if _, ok := entry[field].(float64); !ok {
					t.Errorf("%s: %s = %v, want the required number field", want, field, entry[field])
				}
			}
			switch entry["status"] {
			case "added", "deleted", "modified":
			default:
				t.Errorf("%s: status = %v, want one of added|deleted|modified", want, entry["status"])
			}
			for key := range entry {
				switch key {
				case "file", "patch", "additions", "deletions", "status":
				default:
					t.Errorf("%s: undeclared field %q in the SnapshotFileDiff body (additionalProperties: false)", want, key)
				}
			}
		}
		if got := byFile["dirty.txt"]["additions"]; got != float64(1) {
			t.Errorf("dirty.txt additions = %v, want 1", got)
		}
	})

	t.Run("declared directory selector selects the workspace", func(t *testing.T) {
		status, _, body := getDiff(t, srv.URL, "/session/s1/diff?directory="+repo, nil)
		if status != http.StatusOK {
			t.Fatalf("GET /session/s1/diff?directory=: got %d, want 200. Body: %s", status, body.raw)
		}
		found := false
		for _, f := range body.files {
			if f["file"] == "dirty.txt" {
				found = true
			}
		}
		if !found {
			t.Errorf("?directory= did not select the fixture workspace: %v", body.files)
		}
	})

	t.Run("clean workspace answers an empty array", func(t *testing.T) {
		status, _, body := getDiff(t, srv.URL, "/session/s1/diff", nil)
		if status != http.StatusOK {
			t.Fatalf("GET /session/s1/diff (clean workspace): got %d, want 200. Body: %s", status, body.raw)
		}
		// The contract declares an array: an empty result is [], not null.
		if strings.TrimSpace(body.raw) != "[]" {
			t.Errorf("clean workspace body = %q, want []", strings.TrimSpace(body.raw))
		}
	})
}

// TestSessionDiffErrorPathsAnswerDeclared400 answers every error path: the
// operation declares 400 (there is no declared 404/405), so an unknown
// session and each invalid/unknown ?messageID= must answer 400 with the shim's
// INVALID_REQUEST envelope naming the offending parameter — and never the
// pre-fix 501.
func TestSessionDiffErrorPathsAnswerDeclared400(t *testing.T) {
	_, srv, conn := newDiffStoreTestServer(t)

	var messageID string
	{
		rows, err := conn.Query(context.Background(),
			`SELECT id FROM memory_events WHERE session_id = 's1' ORDER BY id LIMIT 1`)
		if err != nil || len(rows) == 0 {
			t.Fatalf("read seeded message id: %v (%d rows)", err, len(rows))
		}
		messageID = toString(rows[0]["id"])
	}
	if messageID == "" {
		t.Fatal("seeded message row has an empty id")
	}

	unknownSession := func(t *testing.T, path string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build GET %s: %v", path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}

	for _, tc := range []struct {
		name    string
		path    string
		wantSub string
	}{
		{"unknown session", "/session/ses_missing/diff", "unknown session"},
		{"blank messageID", "/session/s1/diff?messageID=", "messageID"},
		{"messageID pattern violation", "/session/s1/diff?messageID=abc123", "^msg"},
		{"unknown messageID", "/session/s1/diff?messageID=msg-999999", "unknown message"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := unknownSession(t, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400 (declared). Body: %s", tc.path, status, body)
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(body), &envelope); err != nil {
				t.Fatalf("400 body is not the shim envelope: %v (%s)", err, body)
			}
			if envelope.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error.code = %q, want INVALID_REQUEST", envelope.Error.Code)
			}
			if !strings.Contains(envelope.Error.Message, tc.wantSub) {
				t.Errorf("error.message = %q, want it to name %q", envelope.Error.Message, tc.wantSub)
			}
		})
	}

	// The error paths above must not have swallowed the success path: with a
	// message id that exists in this session's store, the operation answers the
	// declared 200.
	t.Run("known messageID still answers 200", func(t *testing.T) {
		status, _, body := getDiff(t, srv.URL, "/session/s1/diff?messageID=msg-"+messageID, nil)
		if status != http.StatusOK {
			t.Fatalf("GET /session/s1/diff?messageID=msg-%s: got %d, want 200. Body: %s", messageID, status, body.raw)
		}
	})

	// Neighbour guards: the sibling sub-paths keep their own contracts.
	t.Run("siblings untouched", func(t *testing.T) {
		if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/message"); status != http.StatusOK {
			t.Errorf("GET /session/s1/message: got %d, want 200. Body: %s", status, body)
		}
		if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/session/s1"); status != http.StatusOK {
			t.Errorf("GET /session/s1: got %d, want 200. Body: %s", status, body)
		}
		// Non-GET on the sub-path is not a declared operation: it keeps the
		// pre-existing untyped 501 stub (405 is not in the declared set).
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/s1/diff", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("build POST /session/s1/diff: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /session/s1/diff: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("POST /session/s1/diff: got %d, want 501 (only GET is declared)", resp.StatusCode)
		}
	})
}

// TestSessionDiffNeverAnswers501 is the ROUTE-FIX-010 criterion in one battery:
// on the declared operation, no request — happy path or error path — may answer
// 501 (the pre-fix answer) or any code outside the declared {200,400} set.
func TestSessionDiffNeverAnswers501(t *testing.T) {
	_, srv, _ := newDiffStoreTestServer(t)
	repo := makeGitRepo(t)

	for _, path := range []string{
		"/session/s1/diff",
		"/session/s1/diff?directory=" + repo,
		"/session/s1/diff?messageID=msg-1",
		"/session/s1/diff?messageID=not-a-message-id",
		"/session/s1/diff?messageID=",
		"/session/unknown-session/diff",
		"/session/s1/diff?directory=",
	} {
		status, _, body := getDiff(t, srv.URL, path, nil)
		if status == http.StatusNotImplemented {
			t.Errorf("GET %s answered 501 — SHIM-DRIFT-116 regression. Body: %s", path, body.raw)
			continue
		}
		if status != http.StatusOK && status != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want a declared code (200 or 400). Body: %s", path, status, body.raw)
		}
	}
}

// TestSessionDiffChiMountMatchesProductionWiring is the BUG-009-shaped
// regression for this route: through a parent chi router mounted with
// MountPatterns (the shape cmd/consensus/main.go uses), /session/{id}/diff must
// reach the shim handler instead of chi's 404 — and must answer the declared
// contract there too.
func TestSessionDiffChiMountMatchesProductionWiring(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}
	shim := NewServer(mdb, "test-key", nil, nil)
	shim.skipAuth = true
	shim.workdir = t.TempDir() // non-git: the diff is deterministically empty

	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/session/s1/diff", nil)
	if err != nil {
		t.Fatalf("build GET /session/s1/diff: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /session/s1/diff via chi mount: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /session/s1/diff returned 404 — MountPatterns did not pass the sub-path through. Body: %s", data)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /session/s1/diff via chi mount: got %d, want 200. Body: %s", resp.StatusCode, data)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("chi-mounted diff body = %q, want []", strings.TrimSpace(string(data)))
	}
}

// newDiffStoreTestServer builds a shim server over a real SQLite store with the
// full sessions schema so the session lookup, the messageID resolution and the
// diff translation all run against actual SQL.
func newDiffStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "diff.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open diff test database: %v", err)
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
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('user_message', 'make the change', 's1', 1)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare diff test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, NewServiceAdapter(api.NewService(conn, nil)))
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}
