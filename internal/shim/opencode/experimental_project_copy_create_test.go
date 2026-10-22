package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy".post = v2.projectCopy.create):
// POST /experimental/project/{projectID}/copy, required plain-string path
// parameter projectID (no pattern), optional deepObject location selector
// (directory, workspace; additionalProperties:false), requestBody
// {strategy*: string, directory*: string, name?: string}
// (additionalProperties:false), and exactly two declared responses: 200
// ProjectCopyCopy {directory} and 400 ProjectCopyError | InvalidRequestError.
//
// ROUTE-ADD-070 serves the contract for real: the pinned upstream source makes
// the operation a git-worktree effect
// (worktreeCreate: `git worktree add --detach <directory>/<name> HEAD`) on the
// location-selected source workspace, so the declared 200 {directory} reports
// a directory the handler really created. Before the fix the POST arm answered
// the typed 501 intentional narrowing of ROUTE-ADD-069 (SHIM-NARROWED-U002).
//
// RED-pin: against the pre-change source every POST below answers 501 (the
// narrowing), so TestExperimentalProjectCopyCreateServesDeclared200 fails on
// both the status and the body; the malformed arms fail on answering 501
// instead of the declared 400. The battery pins auth, the declared 400 family
// (path/body/query/strategy), the real-worktree success — verified with
// `git worktree list`, not by trusting the response — the sibling DELETE's
// untouched no-op, and the no-catch-all wiring.

func newProjectCopyCreateServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	repo := makeGitRepoNamed(t, "copy-source")
	s.workdir = repo
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, repo
}

func gitWorktreePaths(t *testing.T, repo string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	paths := make([]string, 0, 8)
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, rest)
		}
	}
	return paths
}

// assertProjectCopyNamedError pins the ProjectCopyError arm body:
// {name: ProjectCopyError, data: {message, forceRequired?}}.
func assertProjectCopyNamedError(t *testing.T, label string, status int, body []byte) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s: got %d, want the declared 400. Body: %s", label, status, body)
	}
	var got struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got.Name != "ProjectCopyError" {
		t.Errorf("%s: name = %q, want ProjectCopyError (a declared 400 arm)", label, got.Name)
	}
	if strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("%s: data.message empty, want a non-empty reason. Body: %s", label, body)
	}
}

// assertCreatedWorktree proves the copy is REAL state: the returned directory
// exists and `git worktree list` of the source repository carries it. The 200
// would be a fabrication otherwise. wantPath, when non-empty, is the exact
// expected path (already joined by the caller).
func assertCreatedWorktree(t *testing.T, label, directory, repo, wantPath string) {
	t.Helper()
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("%s: declared directory %q does not exist as a directory: %v", label, directory, err)
	}
	if wantPath != "" && filepath.Clean(directory) != filepath.Clean(wantPath) {
		t.Errorf("%s: directory %q is not the body-produced path %q", label, directory, wantPath)
	}
	found := false
	for _, wt := range gitWorktreePaths(t, repo) {
		if sameWorkspaceDirectory(wt, directory) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s: copy %q is NOT a git worktree of the source %q (%v) — the 200 would fabricate a copy",
			label, directory, repo, gitWorktreePaths(t, repo))
	}
	// The worktree is a real checkout: it has a HEAD.
	cmd := exec.Command("git", "-C", directory, "rev-parse", "HEAD")
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%s: created worktree has no HEAD: %v (%s)", label, err, out)
	}
}

// TestExperimentalProjectCopyCreateServesDeclared200 pins the declared success
// arm against the REAL effect.
func TestExperimentalProjectCopyCreateServesDeclared200(t *testing.T) {
	srv, repo := newProjectCopyCreateServer(t)
	destParent := t.TempDir()

	status, header, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-create/copy",
		`{"strategy":"git_worktree","directory":"`+destParent+`","name":"named-copy"}`)
	if status != http.StatusOK {
		t.Fatalf("POST copy: got %d, want the declared 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("200 body is not JSON: %v (%s)", err, body)
	}
	if len(got) != 1 {
		t.Errorf("200 body carries %d fields (%v), want exactly {directory} (additionalProperties: false)", len(got), got)
	}
	directory, _ := got["directory"].(string)
	if directory == "" {
		t.Fatalf("200 body = %s, want {directory: <created copy>}", body)
	}
	assertCreatedWorktree(t, "named copy", directory, repo,
		filepath.Join(filepath.Clean(destParent), "named-copy"))
}

// TestExperimentalProjectCopyCreateGeneratesNameForAbsentOptional pins the
// absent-name arm: upstream falls back to a two-word slug (Slug.create), and
// the response still reports a REAL created worktree directory. A second
// create in the same destination must not collide destructively — the suffix
// loop (upstream -2..-10) keeps serving.
func TestExperimentalProjectCopyCreateGeneratesNameForAbsentOptional(t *testing.T) {
	srv, repo := newProjectCopyCreateServer(t)
	destParent := t.TempDir()

	status, _, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-create/copy",
		`{"strategy":"git_worktree","directory":"`+destParent+`"}`)
	if status != http.StatusOK {
		t.Fatalf("POST copy without name: got %d, want the declared 200 (name is optional). Body: %s", status, body)
	}
	var got struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Directory == "" {
		t.Fatalf("200 body = %s, want {directory}; decode err: %v", body, err)
	}
	base := filepath.Base(got.Directory)
	if base == filepath.Base(destParent) {
		t.Errorf("generated copy %q has no distinct name under %q", got.Directory, destParent)
	}
	assertCreatedWorktree(t, "generated copy", got.Directory, repo, "")
}

// TestExperimentalProjectCopyCreateAuth pins the declared 401 arm: the exact
// path keeps the api-key wall and the v2 UnauthorizedError envelope (the v2.*
// operation rule shared with the sibling DELETE).
func TestExperimentalProjectCopyCreateAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-auth/copy",
		`{"strategy":"git_worktree","directory":"/tmp"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST copy: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || strings.TrimSpace(toString(got["message"])) == "" {
		t.Errorf("401 body = %s, want the v2 {_tag: UnauthorizedError, message} envelope", body)
	}
}

// TestExperimentalProjectCopyCreateMalformedAnswersDeclared400 pins the
// declared 400 arm for every malformed input the pinned document defines.
func TestExperimentalProjectCopyCreateMalformedAnswersDeclared400(t *testing.T) {
	srv, _ := newProjectCopyCreateServer(t)
	base := "/experimental/project/prj-copy-400/copy"

	// Well-formed baseline (so a pass is not vacuous).
	destParent := t.TempDir()
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodPost, base,
		`{"strategy":"git_worktree","directory":"`+destParent+`","name":"baseline-copy"}`)
	if status != http.StatusOK {
		t.Fatalf("well-formed baseline: got %d, want 200. Body: %s", status, resp)
	}

	// Present-but-blank path parameter (kind Path).
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/%20/copy", `{"strategy":"git_worktree","directory":"/tmp"}`)
	assertCopyInvalidRequest(t, "blank projectID", status, resp, "Path", "projectID")

	// Bodies violating the declared {strategy*, directory*, name?} schema.
	badBodies := []struct {
		body     string
		wantKind string
	}{
		{``, "Body"},   // required members absent entirely
		{`{}`, "Body"}, // required members absent
		{`{"strategy":"git_worktree"}`, "Payload"},                             // unpaired required member
		{`{"directory":"/tmp"}`, "Payload"},                                    // unpaired required member
		{`{"strategy":1,"directory":"/tmp"}`, "Payload"},                       // wrong type
		{`{"strategy":"git_worktree","directory":2}`, "Payload"},               // wrong type
		{`{"strategy":"git_worktree","directory":"/tmp","name":3}`, "Payload"}, // wrong type
		{`{"strategy":"git_worktree","directory":"/tmp","extra":1}`, "Body"},   // undeclared key
		{`[]`, "Body"},  // not an object
		{`"x"`, "Body"}, // not an object
		{`{"strategy":"git_worktree","directory":"/tmp","name":"/etc/escape"}`, "Payload"}, // name carries a path
		{`{"strategy":"","directory":"/tmp"}`, "Payload"},                                  // blank required member
		{`{"strategy":"git_worktree","directory":""}`, "Payload"},                          // blank required member
	}
	for _, tc := range badBodies {
		status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost, base, tc.body)
		assertCopyInvalidRequest(t, "body "+tc.body, status, resp, tc.wantKind, "")
	}

	// Malformed deepObject location selectors (kind Query).
	for _, query := range []string{
		"?location[nope]=1",
		"?location[directory]=",
		"?location[workspace]=notawk",
	} {
		status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost, base+query,
			`{"strategy":"git_worktree","directory":"/tmp"}`)
		assertCopyInvalidRequest(t, "query "+query, status, resp, "Query", "")
	}

	// Unknown strategy: the pinned upstream runtime registers exactly one
	// strategy (git_worktree); any other word answers the declared
	// ProjectCopyError (upstream StrategyUnavailableError) — never a
	// fabricated worktree and never an undeclared code.
	destOther := t.TempDir()
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost, base,
		`{"strategy":"mystery","directory":"`+destOther+`"}`)
	assertProjectCopyNamedError(t, "unknown strategy", status, resp)
}

// TestExperimentalProjectCopyCreateErrorArms pins the remaining
// ProjectCopyError arms: (a) a source workspace that is NOT a git repository
// (upstream DirectoryUnavailableError — the strategy cannot run) and (b) a
// destination whose requested name is taken by a non-worktree directory that
// the suffix loop cannot pass — both answer the declared 400
// ProjectCopyError, never an undeclared 5xx and never a fabricated success.
func TestExperimentalProjectCopyCreateErrorArms(t *testing.T) {
	srv, _ := newProjectCopyCreateServer(t)

	// (a) non-git source: point the selector at a plain directory.
	plain := t.TempDir()
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-err/copy?location[directory]="+plain,
		`{"strategy":"git_worktree","directory":"`+t.TempDir()+`"}`)
	assertProjectCopyNamedError(t, "non-git source", status, resp)

	// (b) taken destination: name exists AND name-2..name-10 all exist, so
	// upstream's suffix loop exhausts (DestinationExistsError).
	destParent := t.TempDir()
	for _, n := range []string{"taken", "taken-2", "taken-3", "taken-4", "taken-5",
		"taken-6", "taken-7", "taken-8", "taken-9", "taken-10"} {
		if err := os.Mkdir(filepath.Join(destParent, n), 0o755); err != nil {
			t.Fatalf("seed destination %s: %v", n, err)
		}
	}
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-err/copy",
		`{"strategy":"git_worktree","directory":"`+destParent+`","name":"taken"}`)
	assertProjectCopyNamedError(t, "exhausted destination", status, resp)
}

// TestExperimentalProjectCopyCreateSelectorSelectsSource pins the
// selector-as-source behavior: the resolved location[directory] (a DIFFERENT
// real repo), not the server workdir, is the repository the worktree is
// created from — upstream's location.project.directory semantics.
func TestExperimentalProjectCopyCreateSelectorSelectsSource(t *testing.T) {
	srv, _ := newProjectCopyCreateServer(t)
	destParent := t.TempDir()
	other := makeGitRepoNamed(t, "other-source")

	status, _, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-copy-sel/copy?location[directory]="+other,
		`{"strategy":"git_worktree","directory":"`+destParent+`","name":"sel-copy"}`)
	if status != http.StatusOK {
		t.Fatalf("selector-sourced POST: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Directory == "" {
		t.Fatalf("200 body = %s, want {directory}; decode err: %v", body, err)
	}
	found := false
	for _, wt := range gitWorktreePaths(t, other) {
		if sameWorkspaceDirectory(wt, got.Directory) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("copy %q is not a worktree of the SELECTED source %q — the selector resolved nothing",
			got.Directory, other)
	}
}

// TestExperimentalProjectCopyCreateSiblingDeleteUntouched pins the co-existence
// contract: the sibling DELETE keeps its declared 204 no-op byte-for-byte (no
// regression in the shared registration), and the no-registry truthfulness of
// the DELETE is preserved even though create now performs real effects.
func TestExperimentalProjectCopyCreateSiblingDeleteUntouched(t *testing.T) {
	srv, _ := newProjectCopyCreateServer(t)
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/prj-copy-sibling/copy", "")
	if status != http.StatusNoContent {
		t.Fatalf("sibling DELETE: got %d, want the untouched 204. Body: %s", status, resp)
	}
	if len(resp) != 0 {
		t.Errorf("sibling DELETE body = %q, want empty", resp)
	}
}

// TestExperimentalProjectCopyCreateNoCatchAll pins the exact-path
// registration: /copy/generate-name, /copy/refresh, /copyx and deeper shapes
// keep the net/http default 404 they are classified with, and undeclared
// methods on the exact path keep the 405 sibling envelope.
func TestExperimentalProjectCopyCreateNoCatchAll(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{
		"/experimental/project/prj-catch/copy/deeper",
		"/experimental/project/prj-catch/copyx",
		"/experimental/project/prj-catch/copy/generate-name",
	} {
		status, _, _ := doCopyRequest(t, srv.URL, http.MethodPost, path, `{}`)
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}
	status, _, body := doCopyRequest(t, srv.URL, http.MethodPut,
		"/experimental/project/prj-catch/copy", `{}`)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("PUT copy: got %d, want 405. Body: %s", status, body)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("PUT copy body = %s, want the METHOD_NOT_ALLOWED envelope", body)
	}
}

// TestExperimentalProjectCopyCreateRealStore proves the success arm against a
// REAL SQLite-backed server: the declared 200 comes from the effect + the
// filesystem, not from any seeded registry row — the store starts EMPTY and
// the shim reads nothing from it on this path (the no-registry claim is real).
func TestExperimentalProjectCopyCreateRealStore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	conn, err := driver.Open(context.Background(), db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "copy.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open copy test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	repo := makeGitRepoNamed(t, "store-source")
	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = repo
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	destParent := t.TempDir()
	status, _, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-store/copy",
		fmt.Sprintf(`{"strategy":"git_worktree","directory":%q,"name":"store-copy"}`, destParent))
	if status != http.StatusOK {
		t.Fatalf("real-store POST: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Directory == "" {
		t.Fatalf("200 body = %s, want {directory}", body)
	}
	assertCreatedWorktree(t, "real-store copy", got.Directory, repo, "")
}
