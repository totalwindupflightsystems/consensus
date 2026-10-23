package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree".post = worktree.create):
// POST /experimental/worktree, optional plain directory/workspace query
// selectors, requestBody WorktreeCreateInput {name?: string, startCommand?:
// string} (additionalProperties:false), and exactly two declared responses:
// 200 Worktree {name*, branch?, directory*} (additionalProperties:false) and
// 400 WorktreeError | InvalidRequestError.
//
// ROUTE-ADD-085 serves the contract for real: the pinned upstream runtime
// creates the sandbox worktree with `git worktree add --detach
// <destination> HEAD`, so the declared 200 {name, directory} reports a
// directory the handler really created — verified below with `git worktree
// list`, never by trusting the response. Before the fix the exact path had NO
// shim route at all, so every request answered net/http's default 404.
//
// RED-pin: against the pre-change tree (route unregistered) every arm below
// answers 404, so TestWorktreeCreateServesDeclared200 fails on the status and
// the malformed arms fail on answering 404 instead of the declared 400. The
// battery pins auth, the declared 400 family (body/name/query), the
// real-worktree success, the generated name, the method guard and the
// no-catch-all wiring.
//
// ch:trace row=ROUTE-ADD-085 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.create test=internal/shim/opencode/experimental_worktree_create_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local

const worktreeCreatePath = "/experimental/worktree"

// worktreeCreateDoRequest issues a request against a running shim test server.
// A non-empty body is sent as application/json; an empty one sends no body at
// all (the declared both-members-optional case).
func worktreeCreateDoRequest(t *testing.T, base, method, path, body string) (int, http.Header, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, resp.Header, payload
}

// worktreeCreateServer starts a shim whose workspace is the given repository
// with auth bypassed.
func worktreeCreateServer(t *testing.T, workspace string) *httptest.Server {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = workspace
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// worktreeCreateAssertNamedError pins the declared 400 WorktreeError body:
// {name: <enum member>, data: {message}}.
func worktreeCreateAssertNamedError(t *testing.T, label string, status int, body []byte, wantName string) {
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
	if got.Name != wantName {
		t.Errorf("%s: name = %q, want %q (a declared WorktreeError member)", label, got.Name, wantName)
	}
	if strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("%s: data.message empty, want a non-empty reason. Body: %s", label, body)
	}
}

// worktreeCreateAssertInvalidRequest pins the declared 400
// InvalidRequestError body ({_tag, message, kind?, field?}).
func worktreeCreateAssertInvalidRequest(t *testing.T, label string, status int, body []byte, wantKind, wantField string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s: got %d, want the declared 400. Body: %s", label, status, body)
	}
	var got struct {
		Tag     string `json:"_tag"`
		Message string `json:"message"`
		Kind    string `json:"kind"`
		Field   string `json:"field"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got.Tag != "InvalidRequestError" {
		t.Errorf("%s: _tag = %q, want InvalidRequestError (the declared 400 arm)", label, got.Tag)
	}
	if strings.TrimSpace(got.Message) == "" {
		t.Errorf("%s: message empty, want a non-empty reason. Body: %s", label, body)
	}
	if wantKind != "" && got.Kind != wantKind {
		t.Errorf("%s: kind = %q, want %q", label, got.Kind, wantKind)
	}
	if wantField != "" && got.Field != wantField {
		t.Errorf("%s: field = %q, want %q (the offending member named)", label, got.Field, wantField)
	}
}

// worktreeCreateAssertRealWorktree proves the 200 is REAL state: the returned
// directory exists and `git worktree list` of the source repository carries it,
// and the checkout has a HEAD (it is a real detached checkout, not an empty
// directory).
func worktreeCreateAssertRealWorktree(t *testing.T, label, directory, repo, wantName string) {
	t.Helper()
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("%s: declared directory %q does not exist as a directory: %v", label, directory, err)
	}
	wantPath := filepath.Join(filepath.Clean(repo), worktreeCreateDirName, wantName)
	if filepath.Clean(directory) != filepath.Clean(wantPath) {
		t.Errorf("%s: directory %q is not %q", label, directory, wantPath)
	}
	found := false
	for _, wt := range gitWorktreePaths(t, repo) {
		if sameWorkspaceDirectory(wt, directory) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s: %q is NOT a git worktree of %q (%v) — the 200 would fabricate one",
			label, directory, repo, gitWorktreePaths(t, repo))
	}
	cmd := exec.Command("git", "-C", directory, "rev-parse", "HEAD")
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%s: created worktree has no HEAD: %v (%s)", label, err, out)
	}
}

// TestWorktreeCreateServesDeclared200 pins the declared success arm against the
// REAL effect: the body carries exactly {name, directory} (additionalProperties
// false, and `branch` absent because --detach creates no branch) and the
// directory is a registered git worktree of the workspace.
func TestWorktreeCreateServesDeclared200(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-source")
	srv := worktreeCreateServer(t, repo)

	status, header, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"row-085-named"}`)
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want the declared 200. Body: %s", worktreeCreatePath, status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("200 body is not JSON: %v (%s)", err, body)
	}
	if len(got) != 2 {
		t.Errorf("200 body carries %d members (%v), want exactly {name, directory}: the declared Worktree "+
			"schema is additionalProperties:false and `branch` is absent for a --detach checkout", len(got), got)
	}
	if _, present := got["branch"]; present {
		t.Errorf("200 body = %v, want NO branch member: --detach created no branch", got)
	}
	name, _ := got["name"].(string)
	if name != "row-085-named" {
		t.Errorf("name = %q, want the supplied name row-085-named", name)
	}
	directory, _ := got["directory"].(string)
	if directory == "" {
		t.Fatalf("200 body = %s, want {name, directory}", body)
	}
	if !filepath.IsAbs(directory) {
		t.Errorf("directory = %q, want an absolute path", directory)
	}
	worktreeCreateAssertRealWorktree(t, "named worktree", directory, repo, "row-085-named")
}

// TestWorktreeCreateGeneratesNameForAbsentAndEmptyBody pins the optional-members
// arm: both declared members are optional, so no body and {} are legal and
// mean "generate a name" (wt-<8 hex>), and the response still reports a REAL
// created worktree.
func TestWorktreeCreateGeneratesNameForAbsentAndEmptyBody(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-generate")
	srv := worktreeCreateServer(t, repo)

	generated := regexp.MustCompile(`^wt-[0-9a-f]{8}$`)
	for _, tc := range []struct{ label, body string }{
		{"absent body", ""},
		{"empty object", `{}`},
		{"explicit empty name", `{"name":""}`},
	} {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost, worktreeCreatePath, tc.body)
		if status != http.StatusOK {
			t.Fatalf("%s: got %d, want the declared 200 (both members are optional). Body: %s", tc.label, status, body)
		}
		var got struct {
			Name      string `json:"name"`
			Directory string `json:"directory"`
		}
		if err := json.Unmarshal(body, &got); err != nil || got.Name == "" || got.Directory == "" {
			t.Fatalf("%s: 200 body = %s, want {name, directory}; decode err: %v", tc.label, body, err)
		}
		if !generated.MatchString(got.Name) {
			t.Errorf("%s: generated name = %q, want wt-<8 hex>", tc.label, got.Name)
		}
		if filepath.Base(got.Directory) != got.Name {
			t.Errorf("%s: directory %q does not end in the reported name %q", tc.label, got.Directory, got.Name)
		}
		worktreeCreateAssertRealWorktree(t, tc.label, got.Directory, repo, got.Name)
	}
}

// TestWorktreeCreateAuth pins the api-key wall: an unauthenticated POST answers
// 401. worktree.create declares no 401 arm (its response set is 200 and 400),
// so the shim's own UNAUTHENTICATED envelope is kept — the sibling
// experimental.* convention — instead of the v2 UnauthorizedError body.
func TestWorktreeCreateAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"unauth"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST %s: got %d, want 401. Body: %s", worktreeCreatePath, status, body)
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
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "UNAUTHENTICATED" || strings.TrimSpace(got.Error.Message) == "" {
		t.Errorf("401 body = %s, want the shim {error:{code:UNAUTHENTICATED, message}} envelope "+
			"(worktree.create declares no 401 arm)", body)
	}
	// Nothing may be created by a refused request.
	if entries, err := os.ReadDir(filepath.Join(s.workdir, worktreeCreateDirName)); err == nil && len(entries) != 0 {
		t.Errorf("unauthorized POST created %d entries under %s", len(entries), worktreeCreateDirName)
	}
}

// TestWorktreeCreateNonGitWorkspaceIsDeclaredNotGitError pins the declared
// WorktreeNotGitError arm: a workspace that is not a git repository cannot host
// a worktree, so it answers the declared 400 rather than an undeclared 5xx.
func TestWorktreeCreateNonGitWorkspaceIsDeclaredNotGitError(t *testing.T) {
	srv := worktreeCreateServer(t, t.TempDir())

	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"not-git"}`)
	worktreeCreateAssertNamedError(t, "non-git workspace", status, body, "WorktreeNotGitError")
}

// TestWorktreeCreateDestinationCollisionIsDeclaredCreateFailed pins the create
// failure arm: a destination that already exists answers the declared 400
// WorktreeCreateFailedError (never a silent overwrite, never an undeclared 5xx).
func TestWorktreeCreateDestinationCollisionIsDeclaredCreateFailed(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-collision")
	taken := filepath.Join(repo, worktreeCreateDirName, "taken")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatalf("seed destination %s: %v", taken, err)
	}
	srv := worktreeCreateServer(t, repo)

	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"taken"}`)
	worktreeCreateAssertNamedError(t, "destination collision", status, body, "WorktreeCreateFailedError")

	// The seeded directory must be untouched (a failed create changes nothing).
	entries, err := os.ReadDir(taken)
	if err != nil {
		t.Fatalf("seeded destination vanished: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("failed create wrote %d entries into the existing destination", len(entries))
	}
}

// TestWorktreeCreateMalformedAnswersDeclared400 pins the declared 400
// InvalidRequestError arm for every malformed input the pinned document
// defines, plus the unsafe-name refusals.
func TestWorktreeCreateMalformedAnswersDeclared400(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-400")
	srv := worktreeCreateServer(t, repo)

	// Well-formed baseline, so a pass is not vacuous.
	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"baseline-ok"}`)
	if status != http.StatusOK {
		t.Fatalf("well-formed baseline: got %d, want 200. Body: %s", status, body)
	}

	badBodies := []struct {
		body      string
		wantField string
	}{
		{`{"nope":1}`, "nope"},                    // undeclared member
		{`{"name":1}`, "name"},                    // wrong type
		{`{"name":null}`, "name"},                 // null for a declared string
		{`{"startCommand":true}`, "startCommand"}, // wrong type
		{`{"startCommand":null}`, "startCommand"}, // null for a declared string
		{`{"name":"../escape"}`, "name"},          // parent traversal
		{`{"name":"nested/segment"}`, "name"},     // path separator
		{`{"name":".."}`, "name"},                 // traversal segment
		{`{"name":"-rf"}`, "name"},                // option-looking name
		{`[]`, ""},                                // not an object
		{`"x"`, ""},                               // not an object
		{`null`, ""},                              // not an object
		{`{"name":"a"}{"name":"b"}`, ""},          // two JSON values
	}
	for _, tc := range badBodies {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost, worktreeCreatePath, tc.body)
		worktreeCreateAssertInvalidRequest(t, "body "+tc.body, status, body, "Body", tc.wantField)
	}

	// Malformed declared query selectors (plain strings on this operation).
	queryCases := []struct {
		query     string
		wantField string
	}{
		{"?directory=", "directory"},
		{"?workspace=", "workspace"},
		{"?directory=" + filepath.Join(t.TempDir(), "does-not-exist"), "directory"},
	}
	for _, tc := range queryCases {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
			worktreeCreatePath+tc.query, `{"name":"query-case"}`)
		worktreeCreateAssertInvalidRequest(t, "query "+tc.query, status, body, "Query", tc.wantField)
	}

	// None of the refusals may have created a worktree.
	if entries, err := os.ReadDir(filepath.Join(repo, worktreeCreateDirName)); err == nil {
		for _, entry := range entries {
			if entry.Name() == "query-case" || entry.Name() == ".." || strings.Contains(entry.Name(), "escape") {
				t.Errorf("a refused request created %q", entry.Name())
			}
		}
	}
}

// TestWorktreeCreateSelectorSelectsSource pins the declared plain `directory`
// selector: the selected directory (a DIFFERENT real repo), not the server
// workdir, is the repository the worktree is created from.
func TestWorktreeCreateSelectorSelectsSource(t *testing.T) {
	workdirRepo := makeGitRepoNamed(t, "worktree-workdir")
	selectedRepo := makeGitRepoNamed(t, "worktree-selected")
	srv := worktreeCreateServer(t, workdirRepo)

	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath+"?directory="+selectedRepo, `{"name":"selected-source"}`)
	if status != http.StatusOK {
		t.Fatalf("selector-sourced POST: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Name      string `json:"name"`
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Directory == "" {
		t.Fatalf("200 body = %s, want {name, directory}; decode err: %v", body, err)
	}
	worktreeCreateAssertRealWorktree(t, "selector source", got.Directory, selectedRepo, "selected-source")

	// The unselected workspace must gain nothing.
	if _, err := os.Stat(filepath.Join(workdirRepo, worktreeCreateDirName)); !os.IsNotExist(err) {
		t.Errorf("the workdir %s gained a %s directory although the selector named %s",
			workdirRepo, worktreeCreateDirName, selectedRepo)
	}
}

// TestWorktreeCreateMethodGuard pins the undeclared-method arm: GET
// (worktree.list), DELETE (worktree.remove) and every other method on the
// exact path answer the sibling METHOD_NOT_ALLOWED 405 envelope. ROUTE-ADD-084
// and ROUTE-ADD-083 will extend the dispatcher and change this expectation.
func TestWorktreeCreateMethodGuard(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-methods")
	srv := worktreeCreateServer(t, repo)

	// GET and DELETE are declared on this path and are now served by their own
	// rows (ROUTE-ADD-084 worktree.list, ROUTE-ADD-083 worktree.remove), so the
	// guard here is the UNDECLARED-method guard only. The two declared arms are
	// pinned by experimental_worktree_list_remove_test.go.
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, method, worktreeCreatePath, "")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405. Body: %s", method, worktreeCreatePath, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil || got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s body = %s, want the METHOD_NOT_ALLOWED envelope", method, worktreeCreatePath, body)
		}
	}
	// A refused method creates nothing.
	if entries, err := os.ReadDir(filepath.Join(repo, worktreeCreateDirName)); err == nil && len(entries) != 0 {
		t.Errorf("a non-POST method created %d worktrees", len(entries))
	}
}

// TestWorktreeCreateNeighboursUntouched is the non-vacuity control: serving the
// exact /experimental/worktree path must not turn the /experimental/* family
// into a catch-all, and the declared siblings keep their behaviour. It mounts
// the shim under a parent chi router (the production wiring) so the
// /experimental/* MountPatterns entry is part of the proof.
func TestWorktreeCreateNeighboursUntouched(t *testing.T) {
	repo := makeGitRepoNamed(t, "worktree-neighbours")
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = repo
	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// The served sibling declared operations keep their own answers.
	for _, path := range []string{"/experimental/tool", "/experimental/tool/ids", "/experimental/capabilities"} {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodGet, path, "")
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (sibling route unchanged). Body: %s", path, status, body)
		}
	}

	// Deeper and lookalike shapes stay unregistered: no catch-all.
	for _, path := range []string{
		worktreeCreatePath + "/unknown-sub",
		worktreeCreatePath + "x",
	} {
		status, _, _ := worktreeCreateDoRequest(t, srv.URL, http.MethodPost, path, `{"name":"catch-all"}`)
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}

	// The chi-mounted registration really reaches the handler.
	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodPost,
		worktreeCreatePath, `{"name":"chi-mounted"}`)
	if status != http.StatusOK {
		t.Fatalf("chi-mounted POST %s: got %d, want 200. Body: %s", worktreeCreatePath, status, body)
	}
	var got struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.Directory == "" {
		t.Fatalf("chi-mounted 200 body = %s, want {name, directory}; decode err: %v", body, err)
	}
	worktreeCreateAssertRealWorktree(t, "chi-mounted worktree", got.Directory, repo, "chi-mounted")
}
