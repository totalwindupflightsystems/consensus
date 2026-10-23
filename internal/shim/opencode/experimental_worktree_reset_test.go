package opencode

import (
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
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree/reset".post = worktree.reset):
//   - optional query selectors directory / workspace (the shared deepObject
//     location selector);
//   - requestBody WorktreeResetInput {directory*: string}
//     (additionalProperties:false);
//   - exactly two declared responses: 200 (a BARE boolean, description
//     "Worktree reset") and 400 anyOf[WorktreeError, InvalidRequestError],
//     where WorktreeError = {name: <WorktreeNotGitError |
//     WorktreeNameGenerationFailedError | WorktreeCreateFailedError |
//     WorktreeStartCommandFailedError | WorktreeRemoveFailedError |
//     WorktreeResetFailedError | WorktreeListFailedError>, data: {message}}.
//
// ROUTE-ADD-086 serves it for real. Before the route existed the exact path had
// NO shim route at all and answered net/http's default 404 (the
// declared-vs-served artifact's NOT-SERVED class, indistinguishable from "no
// such opencode operation"), so every arm below is a RED pin against the
// pre-change source: the success case 404s instead of 200, the malformed arms
// 404 instead of the declared 400, and the method guard 404s instead of 405.
//
// The success arm is verified by STATE, not by the status code: a tracked file
// dirtied inside a real `git worktree add` checkout is compared byte-for-byte
// against its committed content after the call, an untracked file is proven
// gone, and `git status --porcelain` in the worktree is proven empty. A handler
// that answered `true` without running git could not satisfy those assertions.

// worktreeResetGit runs git in dir, failing the test on a non-zero exit.
func worktreeResetGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// worktreeResetFixture builds the REAL shape the operation acts on: a git
// workspace and a worktree registered to it, nested inside the workspace so the
// request's routed workspace contains the reset target (the containment rule
// the handler enforces). It returns the workspace, the registered worktree and
// the tracked file inside that worktree.
func worktreeResetFixture(t *testing.T) (repo, worktree, tracked string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo = makeGitRepoNamed(t, "reset-workspace")
	worktree = filepath.Join(repo, ".worktrees", "wt1")
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		t.Fatalf("mkdir worktree parent: %v", err)
	}
	worktreeResetGit(t, repo, "worktree", "add", worktree, "HEAD")
	return repo, worktree, filepath.Join(worktree, "README.md")
}

// worktreeResetServer starts a shim whose routed workspace is repo (the
// configured workdir apiV2LocationSelector resolves when no selector is sent).
func worktreeResetServer(t *testing.T, repo string) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = repo
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// worktreeResetBody renders the declared requestBody for a directory.
func worktreeResetBody(directory string) string {
	encoded, err := json.Marshal(map[string]string{"directory": directory})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// assertWorktreeResetInvalidRequest pins the declared 400 InvalidRequestError
// arm on this operation: {_tag: "InvalidRequestError", message, kind?, field?}.
func assertWorktreeResetInvalidRequest(t *testing.T, label string, status int, body []byte, wantKind, wantField string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s: got %d, want the declared 400 InvalidRequestError. Body: %s", label, status, body)
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
		t.Errorf("%s: _tag = %q, want InvalidRequestError. Body: %s", label, got.Tag, body)
	}
	if strings.TrimSpace(got.Message) == "" {
		t.Errorf("%s: message empty, want the offending input named. Body: %s", label, body)
	}
	if wantKind != "" && got.Kind != wantKind {
		t.Errorf("%s: kind = %q, want %q. Body: %s", label, got.Kind, wantKind, body)
	}
	if wantField != "" && got.Field != wantField {
		t.Errorf("%s: field = %q, want %q. Body: %s", label, got.Field, wantField, body)
	}
}

// assertWorktreeResetWorktreeError pins the declared 400 WorktreeError arm:
// {name: <declared enum member>, data: {message}}.
func assertWorktreeResetWorktreeError(t *testing.T, label string, status int, body []byte, wantName string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("%s: got %d, want the declared 400 WorktreeError. Body: %s", label, status, body)
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
		t.Errorf("%s: name = %q, want %q (a declared WorktreeError enum member). Body: %s", label, got.Name, wantName, body)
	}
	if strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("%s: data.message empty, want git's own reason. Body: %s", label, body)
	}
}

// TestWorktreeResetRestoresRegisteredWorktree is the declared-200 pin: a real
// registered worktree with a dirtied tracked file and an untracked file is reset
// to HEAD, and the response body is the BARE boolean literal `true` — not an
// object wrapper (the declared schema is a bare boolean).
func TestWorktreeResetRestoresRegisteredWorktree(t *testing.T) {
	repo, worktree, tracked := worktreeResetFixture(t)
	_, srv := worktreeResetServer(t, repo)

	const committed = "hello\n"
	original, err := os.ReadFile(tracked)
	if err != nil {
		t.Fatalf("read tracked file before the reset: %v", err)
	}
	if string(original) != committed {
		t.Fatalf("fixture tracked file = %q, want %q before dirtying", original, committed)
	}
	if err := os.WriteFile(tracked, []byte("DIRTY working tree content\n"), 0o644); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}
	untracked := filepath.Join(worktree, "scratch-untracked.txt")
	if err := os.WriteFile(untracked, []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("create untracked file: %v", err)
	}

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, worktreeResetBody(worktree))
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want the declared 200. Body: %s", worktreeResetPath, status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "true" {
		t.Errorf("declared 200 body = %q, want the bare boolean literal true", trimmed)
	}
	var flag bool
	if err := json.Unmarshal(body, &flag); err != nil || !flag {
		t.Errorf("200 body must decode as the declared boolean: err %v, value %v (body %s)", err, flag, body)
	}

	// STATE, not the status code: the tracked file carries its committed
	// content again and the untracked file is gone.
	restored, err := os.ReadFile(tracked)
	if err != nil {
		t.Fatalf("read tracked file after the reset: %v", err)
	}
	if string(restored) != committed {
		t.Errorf("tracked file after reset = %q, want the committed content %q — the 200 would be fabricated", restored, committed)
	}
	if _, statErr := os.Stat(untracked); !os.IsNotExist(statErr) {
		t.Errorf("untracked file %q survived the reset (stat err = %v), want it removed by git clean -fd", untracked, statErr)
	}
	cmd := exec.Command("git", "-C", worktree, "status", "--porcelain")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status in the worktree: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("worktree is not clean after the reset: %q", out)
	}
}

// TestWorktreeResetRequiresAuth pins the served-surface auth row (api-key): an
// unauthenticated POST is 401 UNAUTHENTICATED — never the pre-fix 404, and never
// an unauthenticated 400 from the body validation.
func TestWorktreeResetRequiresAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil) // no skipAuth
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, `{"directory":"/tmp"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST %s: got %d, want 401. Body: %s", worktreeResetPath, status, body)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not the shim error envelope: %v (%s)", err, body)
	}
	if got.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", got.Error.Code)
	}
}

// TestWorktreeResetMalformedBodyIsInvalidRequest pins the declared 400
// InvalidRequestError arm for the requestBody WorktreeResetInput
// {directory*: string} additionalProperties:false — and proves the refusal
// happens BEFORE any destructive effect (each case carries a real worktree path
// where the schema allows one, and the worktree is checked to be untouched).
func TestWorktreeResetMalformedBodyIsInvalidRequest(t *testing.T) {
	repo, worktree, tracked := worktreeResetFixture(t)
	_, srv := worktreeResetServer(t, repo)

	const dirty = "DIRTY working tree content\n"
	if err := os.WriteFile(tracked, []byte(dirty), 0o644); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}

	cases := []struct {
		name      string
		body      string
		wantKind  string
		wantField string
	}{
		{
			name:      "empty object",
			body:      `{}`,
			wantKind:  "Body",
			wantField: "directory",
		},
		{
			name:      "blank directory",
			body:      `{"directory":""}`,
			wantKind:  "Payload",
			wantField: "directory",
		},
		{
			name:      "whitespace directory",
			body:      `{"directory":"   "}`,
			wantKind:  "Payload",
			wantField: "directory",
		},
		{
			name:     "unknown member",
			body:     fmt.Sprintf(`{"directory":%q,"force":true}`, worktree),
			wantKind: "Body",
		},
		{
			name:      "wrong type",
			body:      `{"directory":42}`,
			wantKind:  "Payload",
			wantField: "directory",
		},
		{
			name:      "null directory",
			body:      `{"directory":null}`,
			wantKind:  "Payload",
			wantField: "directory",
		},
		{
			name:     "not an object",
			body:     `"reset-me"`,
			wantKind: "Body",
		},
		{
			name:     "two objects",
			body:     fmt.Sprintf(`{"directory":%q}{"directory":%q}`, worktree, worktree),
			wantKind: "Body",
		},
		{
			name:     "empty body",
			body:     ``,
			wantKind: "Body",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, tc.body)
			assertWorktreeResetInvalidRequest(t, tc.name, status, body, tc.wantKind, tc.wantField)
		})
	}

	// Nothing destructive ran: the dirty tracked file in the registered
	// worktree still carries the dirty content.
	got, err := os.ReadFile(tracked)
	if err != nil {
		t.Fatalf("read tracked file after the refused requests: %v", err)
	}
	if string(got) != dirty {
		t.Errorf("tracked file = %q after refusals, want the untouched dirty content %q", got, dirty)
	}
}

// TestWorktreeResetRejectsDirectoryOutsideWorkspace pins the containment guard:
// reset is destructive, so a caller-typed path outside the routed workspace —
// absolute sibling, or a `..` traversal that lexically starts inside it — is the
// declared 400 InvalidRequestError, and NOTHING outside is touched.
func TestWorktreeResetRejectsDirectoryOutsideWorkspace(t *testing.T) {
	repo, worktree, _ := worktreeResetFixture(t)
	_, srv := worktreeResetServer(t, repo)

	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("stage outside file: %v", err)
	}

	escapes := []struct {
		name      string
		directory string
	}{
		{name: "absolute sibling", directory: outside},
		{name: "dotdot traversal", directory: filepath.Join(repo, "..", filepath.Base(outside))},
		{name: "nested dotdot traversal", directory: filepath.Join(filepath.Dir(worktree), "..", "..", "..", filepath.Base(outside))},
		{name: "relative path", directory: ".worktrees/wt1"},
	}
	for _, tc := range escapes {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, worktreeResetBody(tc.directory))
			assertWorktreeResetInvalidRequest(t, tc.name, status, body, "Payload", "directory")
		})
	}

	// The out-of-workspace file is untouched and the workspace's own
	// registered worktree is still a checkout.
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep\n" {
		t.Errorf("outside file %q = %q (err %v) after the refused requests, want it untouched", keep, data, err)
	}
	if info, err := os.Stat(worktree); err != nil || !info.IsDir() {
		t.Errorf("registered worktree %q is gone after the refused requests: %v", worktree, err)
	}
}

// TestWorktreeResetUnregisteredDirectoryIsWorktreeError pins the second guard:
// a directory that exists inside the workspace but is NOT a registered git
// worktree answers the declared 400 WorktreeError
// (WorktreeResetFailedError) — resetting it would silently reset its parent
// repository, so the handler refuses instead of acting on a typed path — and
// the parent repository is left untouched.
func TestWorktreeResetUnregisteredDirectoryIsWorktreeError(t *testing.T) {
	repo, _, _ := worktreeResetFixture(t)
	_, srv := worktreeResetServer(t, repo)

	plain := filepath.Join(repo, "plain-subdir")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatalf("mkdir plain subdir: %v", err)
	}
	trackedInRepo := filepath.Join(repo, "README.md")
	const dirty = "DIRTY working tree content\n"
	if err := os.WriteFile(trackedInRepo, []byte(dirty), 0o644); err != nil {
		t.Fatalf("dirty repository file: %v", err)
	}

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, worktreeResetBody(plain))
	assertWorktreeResetWorktreeError(t, "unregistered directory", status, body, "WorktreeResetFailedError")

	got, err := os.ReadFile(trackedInRepo)
	if err != nil {
		t.Fatalf("read repository file after the refused request: %v", err)
	}
	if string(got) != dirty {
		t.Errorf("repository file = %q after the refusal, want the untouched dirty content %q — the handler must not reset an unregistered directory", got, dirty)
	}
}

// TestWorktreeResetMethodNotAllowed pins the sibling 405 envelope: only POST is
// declared, so every other method on the exact path answers METHOD_NOT_ALLOWED
// (never a 404 for GET, never an implicit 200).
func TestWorktreeResetMethodNotAllowed(t *testing.T) {
	repo, _, _ := worktreeResetFixture(t)
	_, srv := worktreeResetServer(t, repo)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, method, worktreeResetPath)
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s: got %d, want 405. Body: %s", method, worktreeResetPath, status, body)
			}
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("%s: 405 body is not the shim error envelope: %v (%s)", method, err, body)
			}
			if got.Error.Code != "METHOD_NOT_ALLOWED" {
				t.Errorf("%s: error.code = %q, want METHOD_NOT_ALLOWED", method, got.Error.Code)
			}
			if strings.TrimSpace(got.Error.Message) == "" {
				t.Errorf("%s: error.message empty, want the accepted method named", method)
			}
		})
	}
}

// TestWorktreeResetReachableThroughChiMount drives the exact path through a
// parent chi router mounted with MountPatterns (the production wiring), so a
// missing or misregistered pattern answers 404 exactly as it would in
// production (BUG-009 class).
func TestWorktreeResetReachableThroughChiMount(t *testing.T) {
	repo, worktree, tracked := worktreeResetFixture(t)
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = repo
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	if err := os.WriteFile(tracked, []byte("DIRTY working tree content\n"), 0o644); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, worktreeResetPath, worktreeResetBody(worktree))
	if status != http.StatusOK {
		t.Fatalf("POST %s through the chi mount: got %d, want the declared 200. Body: %s", worktreeResetPath, status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "true" {
		t.Errorf("declared 200 body = %q, want the bare boolean literal true", trimmed)
	}
	restored, err := os.ReadFile(tracked)
	if err != nil || string(restored) != "hello\n" {
		t.Errorf("tracked file after the chi-mount reset = %q (err %v), want the committed content", restored, err)
	}
}
