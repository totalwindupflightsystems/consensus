package opencode

import (
	"context"
	"encoding/json"
	"io"
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

// Upstream project.current (specs/openapi/upstream/openapi-1.18.33.json
// paths."/project/current".get) declares exactly two responses: 200 Project,
// 400 BadRequest. Before ROUTE-FIX-005 the sub-path sat in
// projectDeclaredSubpaths and answered the typed not-implemented envelope for
// EVERY request — the declared 200 was unreachable (SHIM-DRIFT-099,
// "declared 200,400, served 501").
//
// These tests are the declared-contract battery: every request to the
// operation must answer inside {200,400} — never 501, never an undeclared
// 4xx/5xx — for the happy path AND for each error path, with the 200 body
// shaped as the upstream Project (required id, worktree, time, sandboxes;
// additionalProperties: false) derived from the real workspace and the real
// schema_versions ledger.

// projectEnvelope is the shim's error envelope, the declared BadRequest shape
// for this operation (writeOpencodeError carries the shim's {error:{code,
// message}} vocabulary).
type projectEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type projectBody struct {
	raw     string
	project map[string]any
}

func getProjectCurrent(t *testing.T, base, path string) (int, projectBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
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
	body := projectBody{raw: string(data)}
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, &body.project); err != nil {
			t.Fatalf("GET %s 200 body is not a JSON object (declared Project): %v (%s)", path, err, data)
		}
	}
	return resp.StatusCode, body
}

func decodeProjectEnvelope(t *testing.T, body string) projectEnvelope {
	t.Helper()
	var env projectEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("error body is not the shim envelope: %v (%s)", err, body)
	}
	return env
}

// newProjectCurrentTestServer builds a shim server over a real SQLite store
// whose schema_versions ledger carries two known migrations, and points the
// workspace at a real git repo (makeGitRepo) so worktree/vcs are derived, not
// asserted.
func newProjectCurrentTestServer(t *testing.T) (*Server, *httptest.Server, db.DB, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "project.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open project test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		// The migration ledger (internal/migrate bootstrapSQL) — the project
		// clock. Two rows with known RFC3339 stamps pin created/updated.
		`CREATE TABLE schema_versions (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL,
			checksum TEXT NOT NULL
		)`,
		`INSERT INTO schema_versions (version, name, applied_at, checksum)
		 VALUES (1, '001 baseline', '2026-09-01T10:00:00Z', 'sha'),
		        (2, '002 tasks',    '2026-09-05T12:30:00Z', 'sha')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare project test database: %v", err)
		}
	}

	dir := makeGitRepo(t)
	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = dir
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn, dir
}

// wantCreatedMilli / wantUpdatedMilli are the exact millis the seeded ledger
// must produce (created = earliest applied_at, updated = latest).
var (
	wantCreatedMilli = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	wantUpdatedMilli = time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC).UnixMilli()
)

func TestProjectCurrentServesDeclared200(t *testing.T) {
	_, srv, conn, dir := newProjectCurrentTestServer(t)

	t.Run("happy path answers the declared Project", func(t *testing.T) {
		status, body := getProjectCurrent(t, srv.URL, "/project/current")
		if status != http.StatusOK {
			t.Fatalf("GET /project/current: got %d, want 200 (declared). Body: %s", status, body.raw)
		}
		// additionalProperties: false — only declared keys may appear.
		for key := range body.project {
			switch key {
			case "id", "worktree", "name", "vcs", "icon", "commands", "time", "sandboxes":
			default:
				t.Errorf("project carries undeclared key %q (additionalProperties: false): %v", key, body.project)
			}
		}
		for _, field := range []string{"id", "worktree", "time", "sandboxes"} {
			if _, ok := body.project[field]; !ok {
				t.Errorf("project is missing the required field %q: %v", field, body.project)
			}
		}
		if got, _ := body.project["worktree"].(string); got != dir {
			t.Errorf("worktree = %q, want the git toplevel %q", got, dir)
		}
		if got, _ := body.project["name"].(string); got != filepath.Base(dir) {
			t.Errorf("name = %q, want the workspace base name %q", got, filepath.Base(dir))
		}
		if got, _ := body.project["vcs"].(string); got != "git" {
			t.Errorf("vcs = %v, want \"git\" (the workspace is a git repo)", body.project["vcs"])
		}
		sandboxes, ok := body.project["sandboxes"].([]any)
		if !ok {
			t.Fatalf("sandboxes = %v, want an array", body.project["sandboxes"])
		}
		if len(sandboxes) != 0 {
			t.Errorf("sandboxes = %v, want [] (the runtime spawns no sandboxes)", sandboxes)
		}
		if got := instanceID(dir); got != body.project["id"] {
			t.Errorf("id = %v, want the singleton instance id %q", body.project["id"], got)
		}
	})

	t.Run("time comes from the schema_versions ledger in millis", func(t *testing.T) {
		_, body := getProjectCurrent(t, srv.URL, "/project/current")
		tm, ok := body.project["time"].(map[string]any)
		if !ok {
			t.Fatalf("time = %v, want an object", body.project["time"])
		}
		created, createdOK := tm["created"].(float64)
		updated, updatedOK := tm["updated"].(float64)
		if !createdOK || !updatedOK {
			t.Fatalf("time = %v, want numeric created/updated", tm)
		}
		if int64(created) != wantCreatedMilli {
			t.Errorf("time.created = %.0f, want %d (earliest applied_at as epoch millis)", created, wantCreatedMilli)
		}
		if int64(updated) != wantUpdatedMilli {
			t.Errorf("time.updated = %.0f, want %d (latest applied_at as epoch millis)", updated, wantUpdatedMilli)
		}
		// Upstream ProjectTime: created <= updated, both integer >= 0.
		if created > updated {
			t.Errorf("time.created %.0f > time.updated %.0f", created, updated)
		}
	})

	t.Run("commands present only when consensus.json declares them", func(t *testing.T) {
		// No consensus.json in the fixture repo → no commands key (optional,
		// never synthesized).
		_, body := getProjectCurrent(t, srv.URL, "/project/current")
		if _, present := body.project["commands"]; present {
			t.Errorf("commands = %v, want the key ABSENT with no consensus.json in the workspace", body.project["commands"])
		}

		cfg := `{"commands":{"start":"consensus serve"}}`
		if err := os.WriteFile(filepath.Join(dir, "consensus.json"), []byte(cfg), 0o644); err != nil {
			t.Fatalf("write consensus.json fixture: %v", err)
		}
		status, body := getProjectCurrent(t, srv.URL, "/project/current")
		if status != http.StatusOK {
			t.Fatalf("GET /project/current with consensus.json: got %d, want 200. Body: %s", status, body.raw)
		}
		commands, ok := body.project["commands"].(map[string]any)
		if !ok {
			t.Fatalf("commands = %v, want the consensus.json object", body.project["commands"])
		}
		if got, _ := commands["start"].(string); got != "consensus serve" {
			t.Errorf("commands.start = %v, want \"consensus serve\"", commands["start"])
		}
	})

	t.Run("declared query selectors are accepted", func(t *testing.T) {
		for _, path := range []string{
			"/project/current?directory=/tmp",
			"/project/current?workspace=default",
			"/project/current?directory=/tmp&workspace=default",
		} {
			status, body := getProjectCurrent(t, srv.URL, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: got %d, want 200. Body: %s", path, status, body.raw)
			}
		}
	})

	// A non-git workspace must not claim a VCS: the field is optional and the
	// enum only knows "git".
	t.Run("non-git workspace omits vcs and keeps 200", func(t *testing.T) {
		s, srv2, _, plain := newProjectCurrentTestServer(t)
		plainDir := t.TempDir() // not a git repo
		s.workdir = plainDir
		status, body := getProjectCurrent(t, srv2.URL, "/project/current")
		if status != http.StatusOK {
			t.Fatalf("GET /project/current (plain dir): got %d, want 200. Body: %s", status, body.raw)
		}
		if _, present := body.project["vcs"]; present {
			t.Errorf("vcs = %v, want the key ABSENT for a non-git workspace", body.project["vcs"])
		}
		if got, _ := body.project["worktree"].(string); got != plainDir {
			t.Errorf("worktree = %q, want the plain directory %q (gitWorktree fallback)", got, plainDir)
		}
		_ = plain
	})

	// x-opencode-directory selects the workspace, the fixed-workspace
	// convention of /path and /vcs.
	t.Run("x-opencode-directory header selects the workspace", func(t *testing.T) {
		other := makeGitRepoNamed(t, "other-ws")
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/project/current", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("x-opencode-directory", other)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /project/current: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /project/current (header workspace): got %d, want 200. Body: %s", resp.StatusCode, data)
		}
		var project map[string]any
		if err := json.Unmarshal(data, &project); err != nil {
			t.Fatalf("body not JSON: %v (%s)", err, data)
		}
		if got, _ := project["worktree"].(string); got != other {
			t.Errorf("worktree = %q, want the header workspace %q", got, other)
		}
		_ = conn
	})
}

func TestProjectCurrentErrorArmsAnswerDeclaredCodes(t *testing.T) {
	_, srv, _, _ := newProjectCurrentTestServer(t)

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"blank directory", "/project/current?directory=", "directory"},
		{"whitespace-only directory", "/project/current?directory=%20", "directory"},
		{"blank workspace", "/project/current?workspace=", "workspace"},
	} {
		t.Run(tc.name+" answers the declared 400", func(t *testing.T) {
			status, body := getProjectCurrent(t, srv.URL, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400 (declared). Body: %s", tc.path, status, body.raw)
			}
			env := decodeProjectEnvelope(t, body.raw)
			if env.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error.code = %q, want INVALID_REQUEST", env.Error.Code)
			}
			if !strings.Contains(env.Error.Message, tc.want) {
				t.Errorf("error.message = %q, want it to name %q", env.Error.Message, tc.want)
			}
		})
	}

	// An unreadable migration ledger is the declared 400, never an undeclared
	// 5xx and never a fabricated timestamp.
	t.Run("unreadable ledger answers the declared 400", func(t *testing.T) {
		ctx := context.Background()
		conn, err := driver.Open(ctx, db.Config{
			URL:          "sqlite://" + filepath.Join(t.TempDir(), "ledger.db"),
			MaxOpenConns: 4,
		})
		if err != nil {
			t.Fatalf("open ledger test database: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		for _, stmt := range []string{
			`CREATE TABLE schema_versions (
				version INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				applied_at TEXT NOT NULL,
				checksum TEXT NOT NULL
			)`,
			// A row whose applied_at is not RFC3339 — the ledger cannot be
			// turned into truthful timestamps.
			`INSERT INTO schema_versions (version, name, applied_at, checksum)
			 VALUES (1, 'broken', 'not-a-timestamp', 'sha')`,
		} {
			if err := conn.Exec(ctx, stmt); err != nil {
				t.Fatalf("prepare ledger test database: %v", err)
			}
		}
		s := NewServer(conn, "test-key", nil, nil)
		s.skipAuth = true
		s.workdir = t.TempDir()
		srv2 := httptest.NewServer(s.Handler())
		t.Cleanup(srv2.Close)

		status, body := getProjectCurrent(t, srv2.URL, "/project/current")
		if status != http.StatusBadRequest {
			t.Fatalf("GET /project/current (broken ledger): got %d, want 400 (declared). Body: %s", status, body.raw)
		}
		env := decodeProjectEnvelope(t, body.raw)
		if env.Error.Code != "INVALID_REQUEST" {
			t.Errorf("error.code = %q, want INVALID_REQUEST", env.Error.Code)
		}
		if !strings.Contains(env.Error.Message, "applied_at") && !strings.Contains(env.Error.Message, "ledger") {
			t.Errorf("error.message = %q, want it to name the ledger problem", env.Error.Message)
		}
	})

	// The error arms must not have swallowed the success path.
	t.Run("a valid request still answers 200", func(t *testing.T) {
		status, _ := getProjectCurrent(t, srv.URL, "/project/current")
		if status != http.StatusOK {
			t.Fatalf("GET /project/current after the error arms: got %d, want 200", status)
		}
	})

	// POST is not a declared method; 405 is not in the declared set either, so
	// the pre-existing typed 501 keeps the surface honest.
	t.Run("undeclared method keeps the typed 501", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/project/current", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("build POST /project/current: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /project/current: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("POST /project/current: got %d, want 501 (only GET is declared; 405 is not). Body: %s", resp.StatusCode, data)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("POST body not JSON: %v (%s)", err, data)
		}
		if got["operation"] != "project.current" || got["error"] != "not_implemented" {
			t.Errorf("POST body = %v, want the typed not-implemented envelope naming project.current", got)
		}
	})
}

// TestProjectCurrentNeverAnswers501 is the battery-level non-vacuity pin: no
// arm of the operation may answer 501 any more.
func TestProjectCurrentNeverAnswers501(t *testing.T) {
	_, srv, _, _ := newProjectCurrentTestServer(t)
	for _, path := range []string{
		"/project/current",
		"/project/current?directory=/tmp",
		"/project/current?directory=",
		"/project/current?workspace=",
	} {
		status, body := getProjectCurrent(t, srv.URL, path)
		if status == http.StatusNotImplemented {
			t.Errorf("GET %s answered 501 — the declared 200 was reachable before, it must stay reachable", path)
		}
		if status != http.StatusOK && status != http.StatusBadRequest {
			t.Errorf("GET %s answered %d, want a declared code (200 or 400). Body: %s", path, status, body.raw)
		}
	}
}

// Neighbour guard: the sibling /project sub-paths keep their own contracts
// (typed 404 for bare /project/{id}; the declared project operations answer
// only their declared 200/400 responses).
func TestProjectCurrentSiblingsUntouched(t *testing.T) {
	_, srv, _, _ := newProjectCurrentTestServer(t)

	if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/project/project_missing"); status != http.StatusNotFound {
		t.Errorf("GET /project/project_missing: got %d, want 404 (typed ProjectNotFoundError). Body: %s", status, body)
	}
	if status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/project/git/init"); status != http.StatusOK && status != http.StatusBadRequest {
		// ROUTE-FIX-006 serves project.initGit now; the arm must stay inside
		// the declared {200,400} and never answer the old typed 501.
		t.Errorf("POST /project/git/init: got %d, want a declared code (200 or 400, ROUTE-FIX-006). Body: %s", status, body)
	}
	if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/project/project_missing/directories"); status != http.StatusBadRequest {
		t.Errorf("GET /project/project_missing/directories: got %d, want 400 BadRequest for unknown project id (ROUTE-FIX-007). Body: %s", status, body)
	}
	if status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/project/current/unknown-sub"); status != http.StatusNotFound {
		t.Errorf("GET /project/current/unknown-sub: got %d, want 404 (non-vacuity control). Body: %s", status, body)
	}
}

// makeGitRepoNamed is makeGitRepo with a chosen base directory name, so the
// x-opencode-directory arm can prove the workspace actually switched.
func makeGitRepoNamed(t *testing.T, name string) string {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = gitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")
	return dir
}
