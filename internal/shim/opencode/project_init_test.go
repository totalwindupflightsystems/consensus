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

// Upstream project.initGit (specs/openapi/upstream/openapi-1.18.33.json
// paths."/project/git/init".post) declares exactly two responses: 200 Project
// ("Create a git repository for the current project and return the refreshed
// project info"), 400 BadRequest. Before ROUTE-FIX-006 the sub-path sat in
// projectDeclaredSubpaths and answered the typed not-implemented envelope for
// EVERY request — the declared 200 was unreachable (SHIM-DRIFT-100,
// "declared 200,400, served 501").
//
// These tests are the declared-contract battery: every request to the
// operation must answer inside {200,400} — never 501, never an undeclared
// 4xx/5xx — for the happy path AND for each error path, and the 200 body must
// be the same declared Project shape project.current serves. The happy path
// must also PROVE the effect: a non-git workspace becomes a git repository
// (.git exists afterwards), and an already-initialized workspace is left
// byte-identical (git init is idempotent; no HEAD is created on a re-init).

// badRequestEnvelope is the declared BadRequestError shape this operation's
// 400 arm answers with (writeOpencodeBadRequest): {name:"BadRequest",
// data:{kind,message}}.
type badRequestEnvelope struct {
	Name string `json:"name"`
	Data struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"data"`
}

// errorEnvelope is the shim's {error:{code,message}} envelope, the declared
// 400 shape for the init-failure and unreadable-ledger arms (the operation's
// 400 declares BadRequestError; the sibling project.current battery accepts
// both envelopes inside its declared 400s, and so do these tests).
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type initResponse struct {
	status  int
	raw     string
	project map[string]any
	badReq  badRequestEnvelope
	errEnv  errorEnvelope
}

func postProjectGitInit(t *testing.T, base, path string) initResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+path, nil)
	if err != nil {
		t.Fatalf("build POST %s: %v", path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST %s: %v", path, err)
	}
	out := initResponse{status: resp.StatusCode, raw: string(data)}
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.Unmarshal(data, &out.project); err != nil {
			t.Fatalf("POST %s 200 body is not a JSON object (declared Project): %v (%s)", path, err, data)
		}
	case http.StatusBadRequest:
		// The declared 400 carries the BadRequestError NamedError envelope;
		// decode both envelope shapes so the assertions can be exact.
		_ = json.Unmarshal(data, &out.badReq)
		_ = json.Unmarshal(data, &out.errEnv)
	}
	return out
}

// newProjectInitGitTestServer builds a shim server over a real SQLite store
// whose schema_versions ledger carries two known migrations, and points the
// workspace at a real (empty, non-git) directory so the happy path can prove
// git init actually runs there.
func newProjectInitGitTestServer(t *testing.T) (*Server, *httptest.Server, db.DB, string) {
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

	dir := t.TempDir() // deliberately NOT a git repo yet
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
	initWantCreatedMilli = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	initWantUpdatedMilli = time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC).UnixMilli()
)

func TestProjectInitGitServesDeclared200(t *testing.T) {
	_, srv, _, dir := newProjectInitGitTestServer(t)

	t.Run("happy path initializes git and answers the declared Project", func(t *testing.T) {
		resp := postProjectGitInit(t, srv.URL, "/project/git/init")
		if resp.status != http.StatusOK {
			t.Fatalf("POST /project/git/init: got %d, want 200 (declared). Body: %s", resp.status, resp.raw)
		}
		// The real effect: the workspace is now a git repository.
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Errorf("workspace still has no .git after POST /project/git/init: %v", err)
		}

		// additionalProperties: false — only declared keys may appear.
		for key := range resp.project {
			switch key {
			case "id", "worktree", "name", "vcs", "icon", "commands", "time", "sandboxes":
			default:
				t.Errorf("project carries undeclared key %q (additionalProperties: false): %v", key, resp.project)
			}
		}
		for _, field := range []string{"id", "worktree", "time", "sandboxes"} {
			if _, ok := resp.project[field]; !ok {
				t.Errorf("project is missing the required field %q: %v", field, resp.project)
			}
		}
		if got, _ := resp.project["worktree"].(string); got != dir {
			t.Errorf("worktree = %q, want the workspace directory %q", got, dir)
		}
		if got, _ := resp.project["name"].(string); got != filepath.Base(dir) {
			t.Errorf("name = %q, want the workspace base name %q", got, filepath.Base(dir))
		}
		if got, _ := resp.project["vcs"].(string); got != "git" {
			t.Errorf("vcs = %v, want \"git\" (the workspace was just initialized)", resp.project["vcs"])
		}
		if got := instanceID(dir); got != resp.project["id"] {
			t.Errorf("id = %v, want the singleton instance id %q", resp.project["id"], got)
		}
		sandboxes, ok := resp.project["sandboxes"].([]any)
		if !ok || len(sandboxes) != 0 {
			t.Errorf("sandboxes = %v, want [] (the runtime spawns no sandboxes)", resp.project["sandboxes"])
		}
	})

	t.Run("time comes from the schema_versions ledger in millis", func(t *testing.T) {
		resp := postProjectGitInit(t, srv.URL, "/project/git/init")
		if resp.status != http.StatusOK {
			t.Fatalf("POST /project/git/init: got %d, want 200. Body: %s", resp.status, resp.raw)
		}
		tm, ok := resp.project["time"].(map[string]any)
		if !ok {
			t.Fatalf("time = %v, want an object", resp.project["time"])
		}
		created, createdOK := tm["created"].(float64)
		updated, updatedOK := tm["updated"].(float64)
		if !createdOK || !updatedOK {
			t.Fatalf("time = %v, want numeric created/updated", tm)
		}
		if int64(created) != initWantCreatedMilli {
			t.Errorf("time.created = %.0f, want %d (earliest applied_at as epoch millis)", created, initWantCreatedMilli)
		}
		if int64(updated) != initWantUpdatedMilli {
			t.Errorf("time.updated = %.0f, want %d (latest applied_at as epoch millis)", updated, initWantUpdatedMilli)
		}
	})

	t.Run("already-initialized workspace answers 200 without a re-init (HEAD untouched)", func(t *testing.T) {
		// First POST initialized git in the fixture (no commit yet, HEAD
		// absent). Record the refs the re-init must not create or change.
		before := gitDirState(t, dir)
		resp := postProjectGitInit(t, srv.URL, "/project/git/init")
		if resp.status != http.StatusOK {
			t.Fatalf("POST /project/git/init (re-init): got %d, want 200. Body: %s", resp.status, resp.raw)
		}
		after := gitDirState(t, dir)
		if before != after {
			t.Errorf("re-initializing changed the git state:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("declared query selectors are accepted", func(t *testing.T) {
		for _, path := range []string{
			"/project/git/init?directory=/tmp",
			"/project/git/init?workspace=default",
			"/project/git/init?directory=/tmp&workspace=default",
		} {
			resp := postProjectGitInit(t, srv.URL, path)
			if resp.status != http.StatusOK {
				t.Errorf("POST %s: got %d, want 200. Body: %s", path, resp.status, resp.raw)
			}
		}
	})

	// The singleton convention: x-opencode-directory selects the workspace
	// the init runs in, exactly as it selects the workspace every read uses.
	t.Run("x-opencode-directory header selects the workspace", func(t *testing.T) {
		other := t.TempDir() // not a git repo
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/project/git/init", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("x-opencode-directory", other)
		respDo, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /project/git/init: %v", err)
		}
		defer func() { _ = respDo.Body.Close() }()
		data, _ := io.ReadAll(respDo.Body)
		if respDo.StatusCode != http.StatusOK {
			t.Fatalf("POST /project/git/init (header workspace): got %d, want 200. Body: %s", respDo.StatusCode, data)
		}
		if _, err := os.Stat(filepath.Join(other, ".git")); err != nil {
			t.Errorf("header workspace still has no .git after the init: %v", err)
		}
		var project map[string]any
		if err := json.Unmarshal(data, &project); err != nil {
			t.Fatalf("body not JSON: %v (%s)", err, data)
		}
		if got, _ := project["worktree"].(string); got != other {
			t.Errorf("worktree = %q, want the header workspace %q", got, other)
		}
	})
}

func TestProjectInitGitErrorArmsAnswerDeclaredCodes(t *testing.T) {
	_, srv, _, _ := newProjectInitGitTestServer(t)

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"blank directory", "/project/git/init?directory=", "directory"},
		{"whitespace-only directory", "/project/git/init?directory=%20", "directory"},
		{"blank workspace", "/project/git/init?workspace=", "workspace"},
	} {
		t.Run(tc.name+" answers the declared 400", func(t *testing.T) {
			resp := postProjectGitInit(t, srv.URL, tc.path)
			if resp.status != http.StatusBadRequest {
				t.Fatalf("POST %s: got %d, want 400 (declared). Body: %s", tc.path, resp.status, resp.raw)
			}
			// The declared BadRequestError envelope, exactly.
			if resp.badReq.Name != "BadRequest" {
				t.Errorf("name = %q, want BadRequest (declared BadRequestError)", resp.badReq.Name)
			}
			if resp.badReq.Data.Kind != "Query" {
				t.Errorf("data.kind = %q, want Query", resp.badReq.Data.Kind)
			}
			if !strings.Contains(resp.badReq.Data.Message, tc.want) {
				t.Errorf("data.message = %q, want it to name %q", resp.badReq.Data.Message, tc.want)
			}
		})
	}

	// A git init that cannot run is the declared 400, never an undeclared
	// 5xx and never a fabricated success. The workspace is pointed at a
	// directory git refuses to initialize in (a FILE, not a directory).
	t.Run("failed git init answers the declared 400", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not available")
		}
		s := NewServer(nil, "test-key", nil, nil)
		s.skipAuth = true
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		s.workdir = file
		srv2 := httptest.NewServer(s.Handler())
		t.Cleanup(srv2.Close)

		resp := postProjectGitInit(t, srv2.URL, "/project/git/init")
		if resp.status != http.StatusBadRequest {
			t.Fatalf("POST /project/git/init (init fails): got %d, want 400 (declared). Body: %s", resp.status, resp.raw)
		}
		if resp.errEnv.Error.Code != "INVALID_REQUEST" {
			t.Errorf("error.code = %q, want INVALID_REQUEST", resp.errEnv.Error.Code)
		}
		if !strings.Contains(resp.errEnv.Error.Message, "git") {
			t.Errorf("error.message = %q, want it to name the git failure", resp.errEnv.Error.Message)
		}
	})

	// The error arms must not have swallowed the success path.
	t.Run("a valid request still answers 200", func(t *testing.T) {
		resp := postProjectGitInit(t, srv.URL, "/project/git/init")
		if resp.status != http.StatusOK {
			t.Fatalf("POST /project/git/init: got %d, want 200. Body: %s", resp.status, resp.raw)
		}
	})
}

// TestProjectInitGitNeverAnswers501 pins the fix: no arm of the operation
// answers the typed (or untyped) 501 stub any more.
func TestProjectInitGitNeverAnswers501(t *testing.T) {
	_, srv, _, _ := newProjectInitGitTestServer(t)
	for _, path := range []string{
		"/project/git/init",
		"/project/git/init?directory=",
		"/project/git/init?workspace=",
	} {
		resp := postProjectGitInit(t, srv.URL, path)
		if resp.status == http.StatusNotImplemented {
			t.Errorf("POST %s answered 501 (the pre-fix stub); every arm must answer inside the declared {200,400}", path)
		}
	}
}

// gitDirState summarizes the refs and HEAD of the workspace's .git so a
// re-init can be asserted state-preserving.
func gitDirState(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "HEAD").CombinedOutput()
	head := "absent"
	if err == nil {
		head = strings.TrimSpace(string(out))
	}
	branch, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		t.Fatalf("git branch: %v", err)
	}
	return "HEAD=" + head + " branch=" + strings.TrimSpace(string(branch))
}
