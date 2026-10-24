package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree"):
//
//	GET    worktree.list   -> 200 array of strings, 400 WorktreeError | InvalidRequestError
//	DELETE worktree.remove -> 200 boolean,          400 WorktreeError | InvalidRequestError
//	                          optional query parameters: directory, workspace
//
// ROUTE-ADD-084 / ROUTE-ADD-083 extend the dispatcher ROUTE-ADD-085 registered
// on the exact path. Both arms are REAL git effects: the list answers what
// `git worktree list` reports, and the remove only answers `true` after `git
// worktree remove` actually removed a REGISTERED worktree inside the selected
// workspace.
//
// ch:trace row=ROUTE-ADD-084 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.list test=internal/shim/opencode/experimental_worktree_list_remove_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:foreman-local
// ch:trace row=ROUTE-ADD-083 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.remove test=internal/shim/opencode/experimental_worktree_list_remove_test.go evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:foreman-local

// worktreeLinked creates a real linked worktree of repo and returns its path.
func worktreeLinked(t *testing.T, repo, name string) string {
	t.Helper()
	destination := filepath.Join(repo, ".opencode-worktrees", name)
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "--detach", destination, "HEAD")
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add %s: %v (%s)", destination, err, out)
	}
	return destination
}

// TestWorktreeListServesDeclared200 pins the declared success arm: the response
// is a JSON array of the repository's real registered worktree directories,
// including a linked worktree created before the request.
func TestWorktreeListServesDeclared200(t *testing.T) {
	repo := makeGitRepoNamed(t, "wt-list")
	linked := worktreeLinked(t, repo, "listed")
	srv := worktreeCreateServer(t, repo)

	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodGet, worktreeCreatePath, "")
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want the declared 200. Body: %s", worktreeCreatePath, status, body)
	}
	var got []string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET body is not a JSON array of strings: %v (%s)", err, body)
	}
	if got == nil {
		t.Fatalf("GET body is JSON null, want the declared array (never null). Body: %s", body)
	}
	foundRepo, foundLinked := false, false
	for _, p := range got {
		if sameWorkspaceDirectory(p, repo) {
			foundRepo = true
		}
		if sameWorkspaceDirectory(p, linked) {
			foundLinked = true
		}
	}
	if !foundRepo {
		t.Errorf("GET body %v does not carry the workspace %q", got, repo)
	}
	if !foundLinked {
		t.Errorf("GET body %v does not carry the linked worktree %q the repository really has (%v)",
			got, linked, gitWorktreePaths(t, repo))
	}
}

// TestWorktreeListNonGitIsDeclaredNotGitError pins the declared WorktreeError
// arm for a workspace outside any repository.
func TestWorktreeListNonGitIsDeclaredNotGitError(t *testing.T) {
	plain := t.TempDir()
	srv := worktreeCreateServer(t, plain)
	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodGet, worktreeCreatePath, "")
	worktreeCreateAssertNamedError(t, "GET on a non-git workspace", status, body, "WorktreeNotGitError")
}

// TestWorktreeListAuth keeps the api-key wall: without credentials the same
// path is unauthorized, so the 200 above is not an open endpoint.
func TestWorktreeListAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		status, header, body := worktreeCreateDoRequest(t, srv.URL, method, worktreeCreatePath, "")
		if status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %s: got %d, want 401. Body: %s", method, worktreeCreatePath, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: Content-Type = %q, want application/json", method, ct)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s 401 body is not JSON: %v (%s)", method, err, body)
		}
		if got.Error.Code != "UNAUTHENTICATED" || strings.TrimSpace(got.Error.Message) == "" {
			t.Errorf("%s 401 body = %s, want the shim {error:{code:UNAUTHENTICATED, message}} envelope", method, body)
		}
	}
}

// TestWorktreeRemoveDeclaredInputAndWorkspaceResolution covers the declared
// body/query target forms and fixed-workspace selectors.
func TestWorktreeRemoveDeclaredInputAndWorkspaceResolution(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		query      string
		workdir    string
		wantStatus int
	}{
		{name: "body only", body: "BODY", wantStatus: http.StatusOK},
		{name: "query only", query: "QUERY", wantStatus: http.StatusOK},
		{name: "workspace selector", query: "QUERY", workdir: "OUTSIDE", wantStatus: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeGitRepoNamed(t, "wt-remove-declared")
			linked := worktreeLinked(t, repo, tc.name)
			server := NewServer(&mockDB{}, "test-key", nil, nil)
			server.skipAuth = true
			server.workdir = repo
			if tc.workdir == "OUTSIDE" {
				server.workdir = t.TempDir()
			}
			srv := httptest.NewServer(server.Handler())
			defer srv.Close()
			body := tc.body
			if body != "" {
				body = fmt.Sprintf(`{"directory":%q}`, linked)
			}
			query := tc.query
			if query != "" {
				query = "?directory=" + url.QueryEscape(linked)
			}
			status, _, response := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, worktreeCreatePath+query, body)
			if status != tc.wantStatus {
				t.Fatalf("DELETE: got %d, want %d. Body: %s", status, tc.wantStatus, response)
			}
			if _, err := os.Stat(linked); !os.IsNotExist(err) {
				t.Fatalf("worktree still exists after removal: %v", err)
			}
		})
	}
}

// TestWorktreeRemoveServesDeclared200 proves the removal is REAL: the linked
// worktree exists before, answers the declared `true`, and is gone afterwards.
func TestWorktreeRemoveServesDeclared200(t *testing.T) {
	repo := makeGitRepoNamed(t, "wt-remove")
	linked := worktreeLinked(t, repo, "removable")
	srv := worktreeCreateServer(t, repo)

	path := worktreeCreatePath + "?directory=" + linked
	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, path, "")
	if status != http.StatusOK {
		t.Fatalf("DELETE %s: got %d, want the declared 200. Body: %s", path, status, body)
	}
	if strings.TrimSpace(string(body)) != "true" {
		t.Fatalf("DELETE body = %s, want the declared boolean true", body)
	}
	if _, err := os.Stat(linked); !os.IsNotExist(err) {
		t.Errorf("worktree %q still exists after the declared 200 (stat err = %v)", linked, err)
	}
	for _, wt := range gitWorktreePaths(t, repo) {
		if sameWorkspaceDirectory(wt, linked) {
			t.Errorf("%q is still registered as a worktree of %q after the removal", linked, repo)
		}
	}
}

// TestWorktreeRemoveRejectsUnsafeTargets pins the declared 400 arms: a path the
// caller merely typed is never removed, and the reason names the guard.
func TestWorktreeRemoveRejectsUnsafeTargets(t *testing.T) {
	repo := makeGitRepoNamed(t, "wt-remove-guards")
	srv := worktreeCreateServer(t, repo)

	t.Run("outside the workspace", func(t *testing.T) {
		outside := t.TempDir()
		path := worktreeCreatePath + "?directory=" + outside
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, path, "")
		worktreeCreateAssertInvalidRequest(t, "DELETE outside the workspace", status, body, "Query", "directory")
		if _, err := os.Stat(outside); err != nil {
			t.Errorf("the rejected outside directory was touched: %v", err)
		}
	})

	t.Run("inside the workspace but not registered", func(t *testing.T) {
		stray, err := os.MkdirTemp(repo, "stray-")
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := worktreeCreatePath + "?directory=" + stray
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, path, "")
		worktreeCreateAssertNamedError(t, "DELETE unregistered path", status, body, "WorktreeRemoveFailedError")
		if _, err := os.Stat(stray); err != nil {
			t.Errorf("the rejected unregistered directory was touched: %v", err)
		}
	})

	t.Run("the primary checkout", func(t *testing.T) {
		path := worktreeCreatePath + "?directory=" + repo
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, path, "")
		worktreeCreateAssertNamedError(t, "DELETE the primary checkout", status, body, "WorktreeRemoveFailedError")
		if _, err := os.Stat(repo); err != nil {
			t.Fatalf("the primary checkout was removed: %v", err)
		}
	})

	t.Run("no selector", func(t *testing.T) {
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, worktreeCreatePath, "")
		worktreeCreateAssertNamedError(t, "DELETE without a directory", status, body, "WorktreeRemoveFailedError")
	})

	t.Run("blank selector", func(t *testing.T) {
		path := worktreeCreatePath + "?directory="
		status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodDelete, path, "")
		worktreeCreateAssertInvalidRequest(t, "DELETE with a blank directory", status, body, "Query", "directory")
	})
}

// TestWorktreeMethodDispatch guards the shared registration: GET and DELETE are
// served, the declared POST arm is untouched, and an undeclared method still
// answers the honest 405.
func TestWorktreeMethodDispatch(t *testing.T) {
	repo := makeGitRepoNamed(t, "wt-dispatch")
	srv := worktreeCreateServer(t, repo)

	status, _, body := worktreeCreateDoRequest(t, srv.URL, http.MethodGet, worktreeCreatePath, "")
	if status != http.StatusOK {
		t.Errorf("GET: got %d, want 200 (list arm). Body: %s", status, body)
	}

	status, _, body = worktreeCreateDoRequest(t, srv.URL, http.MethodPatch, worktreeCreatePath, "{}")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: got %d, want the 405 method guard. Body: %s", status, body)
	}

	// The POST arm still creates a real worktree (no regression from the
	// dispatcher change).
	status, _, body = worktreeCreateDoRequest(t, srv.URL, http.MethodPost, worktreeCreatePath, `{"name":"dispatch-child"}`)
	if status != http.StatusOK {
		t.Fatalf("POST: got %d, want the declared 200. Body: %s", status, body)
	}
	var created struct {
		Name      string `json:"name"`
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("POST body is not JSON: %v (%s)", err, body)
	}
	if _, err := os.Stat(created.Directory); err != nil {
		t.Errorf("POST claims directory %q it did not create: %v", created.Directory, err)
	}
}
