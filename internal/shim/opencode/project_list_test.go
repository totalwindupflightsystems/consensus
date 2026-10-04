package opencode

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// Upstream project.list (openapi-1.18.33.json paths."/project".get) declares
// exactly two responses: 200 Array(Project) and 400 BadRequest. Optional query
// selectors: directory and workspace. The bare /project mount answered the
// untyped 501 stub ("project is opencode-specific, not supported by Consensus
// shim; use native tool API", handleProjectVCSSStub) before ROUTE-FIX-004 —
// SHIM-DRIFT-098, class OUTCOME-MISMATCH ("declared 200,400, served 501").
//
// These tests are the declared-contract battery: every GET /project request
// must answer inside {200,400} — never 501, never an undeclared 404/405 — with
// the 200 body shaped as the upstream Project[] (required: id, worktree,
// time.created, time.updated, sandboxes; additionalProperties: false), and the
// 400 body shaped as the shim's INVALID_REQUEST envelope. Non-GET on the mount
// keeps the pre-existing 501 stub (405 is not in the declared response set).

// projectListResponse is the upstream Project schema surface the 200 body must
// conform to (required fields present, no undeclared fields).
type projectListResponse struct {
	Raw  string
	Rows []map[string]any
}

func getProjectList(t *testing.T, base, path string, headers map[string]string) (int, http.Header, projectListResponse) {
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
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	var rows []map[string]any
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, &rows); err != nil {
			t.Fatalf("GET %s 200 body is not a Project array: %v (%s)", path, err, data)
		}
	}
	return resp.StatusCode, resp.Header, projectListResponse{Raw: string(data), Rows: rows}
}

// newProjectListStoreTestServer builds a shim server over a real SQLite store
// carrying the projects table the same way the deployed SQLite schema has it
// after migrations 014/015 apply through the shipped migration runner: TEXT id,
// TEXT name, nullable description, integer created_at (Unix seconds — SQLite
// stores migration defaults as the raw declared types, and the runner's
// filterForSQLite keeps the DDL as written). Seeds two projects so the list
// shape is proven against real rows, not an empty table.
func newProjectListStoreTestServer(t *testing.T, skipAuth bool) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "projects.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open project list test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		// The projects table as migrations 014/015 apply it on SQLite
		// (gen_random_uuid()/now() defaults are PG-isms SQLite stores as
		// dead DDL; rows always carry an explicit id/created_at here).
		`CREATE TABLE projects (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at  INTEGER NOT NULL
		)`,
		`INSERT INTO projects (id, name, description, created_at) VALUES
			('prj-alpha', 'Alpha', 'first seeded project', 1728000000),
			('prj-beta',  'Beta',  NULL,                   1728000001)`,
		// The api_keys table the shim's validateAuth reads (Basic auth checks
		// key_hash; the Bearer arm checks key_prefix + key_hash).
		`CREATE TABLE api_keys (
			id         TEXT PRIMARY KEY,
			key_hash   TEXT NOT NULL,
			key_prefix TEXT,
			scope      TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed projects table: %v", err)
		}
	}
	// Seed the admin key row for the "test-key" server key (hex digest only —
	// no secret material reaches this statement).
	keyHash := hex.EncodeToString(sha256Hash([]byte("test-key")))
	if err := conn.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope) VALUES ('key-admin', '`+keyHash+`', 'test-key', 'admin')`); err != nil {
		t.Fatalf("seed admin api key: %v", err)
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = skipAuth
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// TestProjectListServesDeclaredContract answers the happy path: GET /project
// returns 200 with the upstream Project[] shape built from the real projects
// table — every required field present (id, worktree, time.created,
// time.updated, sandboxes), no undeclared fields (additionalProperties:
// false) — and 401 stays the unauthenticated answer, because the bare /project
// mount keeps GET auth (SPEC-017 §3.9, the shim smoke test's 401 no-auth arm).
func TestProjectListServesDeclaredContract(t *testing.T) {
	_, srv, _ := newProjectListStoreTestServer(t, true)

	t.Run("lists the projects with required fields", func(t *testing.T) {
		status, header, body := getProjectList(t, srv.URL, "/project", nil)
		if status != http.StatusOK {
			t.Fatalf("GET /project: got %d, want 200 (declared). Body: %s", status, body.Raw)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if len(body.Rows) != 2 {
			t.Fatalf("GET /project listed %d projects, want the 2 seeded rows. Body: %s", len(body.Rows), body.Raw)
		}
		byID := map[string]map[string]any{}
		for _, row := range body.Rows {
			id, _ := row["id"].(string)
			byID[id] = row
		}
		for id, wantName := range map[string]string{"prj-alpha": "Alpha", "prj-beta": "Beta"} {
			row, ok := byID[id]
			if !ok {
				t.Fatalf("project %s missing from the list: %s", id, body.Raw)
			}
			if got := row["name"]; got != wantName {
				t.Errorf("%s: name = %v, want %q", id, got, wantName)
			}
			// Required: id, worktree, time{created,updated}, sandboxes.
			if worktree, ok := row["worktree"].(string); !ok || worktree == "" {
				t.Errorf("%s: worktree = %v, want the required non-empty string", id, row["worktree"])
			}
			timeObj, ok := row["time"].(map[string]any)
			if !ok {
				t.Fatalf("%s: time = %v, want the required object", id, row["time"])
			}
			for _, field := range []string{"created", "updated"} {
				if _, ok := timeObj[field].(float64); !ok {
					t.Errorf("%s: time.%s = %v, want the required number", id, field, timeObj[field])
				}
			}
			if timeObj["created"] != float64(1728000000) && timeObj["created"] != float64(1728000001) {
				t.Errorf("%s: time.created = %v, want the seeded created_at", id, timeObj["created"])
			}
			sandboxes, ok := row["sandboxes"].([]any)
			if !ok {
				t.Errorf("%s: sandboxes = %v, want the required array", id, row["sandboxes"])
			} else if len(sandboxes) != 0 {
				t.Errorf("%s: sandboxes = %v, want an empty array (the runtime provisions none)", id, sandboxes)
			}
			// additionalProperties: false — the declared field set only.
			for key := range row {
				switch key {
				case "id", "worktree", "vcs", "name", "icon", "commands", "time", "sandboxes":
				default:
					t.Errorf("%s: undeclared field %q in the Project body (additionalProperties: false)", id, key)
				}
			}
		}
	})

	t.Run("unauthenticated still answers 401", func(t *testing.T) {
		_, authSrv, _ := newProjectListStoreTestServer(t, false)
		status, _, body := getProjectList(t, authSrv.URL, "/project", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("GET /project unauthenticated: got %d, want 401 (GET keeps auth on the bare mount). Body: %s", status, body.Raw)
		}
		// Basic auth (opencode:<admin key>) is the scheme the shim accepts for
		// api-key routes — the same arm every other authed route tests.
		req, err := http.NewRequest(http.MethodGet, authSrv.URL+"/project", nil)
		if err != nil {
			t.Fatalf("build authenticated GET /project: %v", err)
		}
		req.SetBasicAuth("opencode", "test-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /project with the admin key: %v", err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /project with the admin key: got %d, want 200. Body: %s", resp.StatusCode, data)
		}
	})
}

// TestProjectListEmptyStoreAnswersEmptyArray pins the empty result: a store
// whose projects table has no rows answers the declared array — [], never
// null, never 501.
func TestProjectListEmptyStoreAnswersEmptyArray(t *testing.T) {
	mdb := &mockDB{}
	s := NewServer(mdb, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := getProjectList(t, srv.URL, "/project", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /project (empty store): got %d, want 200. Body: %s", status, body.Raw)
	}
	if strings.TrimSpace(body.Raw) != "[]" {
		t.Errorf("empty store body = %q, want []", strings.TrimSpace(body.Raw))
	}
}

// TestProjectListParsesTextTimestamps pins the live-store finding: SQLite
// type affinity does not force created_at writers to integers (a live probe
// stored "1791098154" as TEXT through strftime('%s','now')), and the declared
// ProjectTime fields are integers — the handler must parse numeric TEXT and
// answer 0 for anything unreadable, never a fabricated stamp.
func TestProjectListParsesTextTimestamps(t *testing.T) {
	_, srv, conn := newProjectListStoreTestServer(t, true)
	ctx := context.Background()

	for _, stmt := range []string{
		`INSERT INTO projects (id, name, description, created_at) VALUES ('prj-text', 'TextStamp', NULL, '1728000123')`,
		`INSERT INTO projects (id, name, description, created_at) VALUES ('prj-junk', 'JunkStamp', NULL, 'not-a-number')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed text-stamp project: %v", err)
		}
	}

	status, _, body := getProjectList(t, srv.URL, "/project", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /project: got %d, want 200. Body: %s", status, body.Raw)
	}
	byID := map[string]map[string]any{}
	for _, row := range body.Rows {
		id, _ := row["id"].(string)
		byID[id] = row
	}
	textRow, ok := byID["prj-text"]
	if !ok {
		t.Fatalf("prj-text missing from the list: %s", body.Raw)
	}
	timeObj := textRow["time"].(map[string]any)
	if timeObj["created"] != float64(1728000123) {
		t.Errorf("TEXT created_at parsed as %v, want 1728000123", timeObj["created"])
	}
	junkRow := byID["prj-junk"]
	if junkRow == nil {
		t.Fatalf("prj-junk missing from the list: %s", body.Raw)
	}
	junkTime := junkRow["time"].(map[string]any)
	if junkTime["created"] != float64(0) {
		t.Errorf("unreadable created_at answered %v, want 0", junkTime["created"])
	}
}

// TestProjectListReadFailureAnswersDeclared400 answers the store-failure arm:
// the declared error vocabulary for this operation is 400 only (no declared
// 404/5xx), so a store read failure answers 400 INVALID_REQUEST naming the
// failure — never the pre-fix 501, never an undeclared code.
func TestProjectListReadFailureAnswersDeclared400(t *testing.T) {
	mdb := &mockDB{queryErr: errors.New("store unavailable")}
	s := NewServer(mdb, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := getProjectList(t, srv.URL, "/project", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("GET /project (store failure): got %d, want 400 (declared). Body: %s", status, body.Raw)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body.Raw), &envelope); err != nil {
		t.Fatalf("400 body is not the error envelope: %v (%s)", err, body.Raw)
	}
	if envelope.Error.Code != "INVALID_REQUEST" {
		t.Errorf("error code = %q, want INVALID_REQUEST", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, "store unavailable") {
		t.Errorf("error message = %q, want it to name the store failure", envelope.Error.Message)
	}
}

// TestProjectListNeverAnswersUndeclaredCodes sweeps the request shapes a
// bridged client can produce (bare, each declared selector, both together, an
// unknown parameter) and proves every answer stays inside the declared
// {200,400} — the OUTCOME-MISMATCH regression is a 501 anywhere in the sweep.
func TestProjectListNeverAnswersUndeclaredCodes(t *testing.T) {
	_, srv, _ := newProjectListStoreTestServer(t, true)

	for _, path := range []string{
		"/project",
		"/project?directory=/tmp",
		"/project?workspace=ws1",
		"/project?directory=/tmp&workspace=ws1",
		"/project?unknown=1",
	} {
		status, _, body := getProjectList(t, srv.URL, path, nil)
		if status == http.StatusNotImplemented {
			t.Errorf("GET %s answered 501 — SHIM-DRIFT-098 regression. Body: %s", path, body.Raw)
			continue
		}
		if status != http.StatusOK && status != http.StatusBadRequest {
			t.Errorf("GET %s: got %d, want a declared code (200 or 400). Body: %s", path, status, body.Raw)
		}
	}
}

// TestProjectListUndeclaredMethodKeepsStub pins the neighbour: non-GET on the
// bare /project mount is not a declared operation, and 405 is not in the
// declared response set — the pre-existing 501 stub answer stays.
func TestProjectListUndeclaredMethodKeepsStub(t *testing.T) {
	_, srv, _ := newProjectListStoreTestServer(t, true)

	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, srv.URL+"/project", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("build %s /project: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /project: %v", method, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s /project: got %d, want the pre-existing 501 stub. Body: %s", method, resp.StatusCode, data)
		}
	}
}

// TestProjectListChiMountMatchesProductionWiring is the BUG-009-shaped
// regression for this route: through a parent chi router mounted with
// MountPatterns (the shape cmd/consensus/main.go uses), GET /project must
// reach the shim handler and answer the declared contract there too.
func TestProjectListChiMountMatchesProductionWiring(t *testing.T) {
	mdb := &mockDB{}
	shim := NewServer(mdb, "test-key", nil, nil)
	shim.skipAuth = true
	shim.workdir = t.TempDir()

	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/project", nil)
	if err != nil {
		t.Fatalf("build GET /project: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /project via chi mount: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /project returned 404 — MountPatterns did not pass the mount through. Body: %s", data)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /project via chi mount: got %d, want 200. Body: %s", resp.StatusCode, data)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("chi-mounted project list body = %q, want []", strings.TrimSpace(string(data)))
	}
}
