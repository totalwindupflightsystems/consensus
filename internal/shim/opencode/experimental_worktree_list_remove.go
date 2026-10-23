package opencode

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// ROUTE-ADD-084 — GET /experimental/worktree  (upstream worktree.list)
// ROUTE-ADD-083 — DELETE /experimental/worktree (upstream worktree.remove)
// ============================================================================
//
// Both operations are declared on the exact path the ROUTE-ADD-085 dispatcher
// already registers, so they EXTEND that dispatcher rather than adding a second
// mux entry (a second registration of the same pattern panics).
//
// Declared arms (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree"):
//
//	GET  worktree.list   -> 200 array of strings ("List of worktree directories"), 400 WorktreeError | InvalidRequestError
//	DEL  worktree.remove -> 200 boolean ("Worktree removed"),                      400 WorktreeError | InvalidRequestError
//	                        optional query parameters: directory, workspace
//
// Truthfulness: both arms are REAL git effects against the selected workspace,
// not registry reads. worktree.list answers what `git worktree list` reports on
// this machine; worktree.remove runs `git worktree remove` and only answers the
// declared `true` once that command succeeded. Nothing is fabricated: no path
// is invented for a repository that has none, and no removal is claimed for a
// directory this runtime did not itself have registered.

// worktreeRegisteredPaths returns the absolute worktree paths `git worktree
// list --porcelain` reports for the repository containing dir.
func worktreeRegisteredPaths(ctx context.Context, dir string) ([]string, error) {
	out, err := runGit(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 4)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		if p := strings.TrimSpace(strings.TrimPrefix(line, "worktree ")); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// worktreeNotGit reports whether dir is outside any git repository, which is
// the declared WorktreeNotGitError arm rather than a list/remove failure.
func worktreeNotGit(ctx context.Context, dir string) bool {
	if _, err := runGit(ctx, dir, "rev-parse", "--git-dir"); err != nil {
		return true
	}
	return false
}

// handleExperimentalWorktreeList serves GET /experimental/worktree — upstream
// worktree.list (ROUTE-ADD-084; declared responses: 200 array of strings,
// 400 WorktreeError | InvalidRequestError).
//
// ch:trace row=ROUTE-ADD-084 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.list test=internal/shim/opencode/experimental_worktree_list_remove_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:foreman-local
//
// Served arms:
//   - the selected workspace comes from apiV2LocationSelector, and a valued
//     `directory` query parameter selects the repository to list (the same
//     declared selector rule the sibling POST uses); a present-but-blank
//     selector is the declared 400 InvalidRequestError (kind Query);
//   - a workspace outside any git repository answers the declared 400
//     WorktreeNotGitError;
//   - a failing `git worktree list` answers the declared 400
//     WorktreeListFailedError — never an undeclared 5xx;
//   - SUCCESS answers the declared 200 with the repository's real registered
//     worktree directories. The array is never null: a repository with no
//     linked worktrees answers `[]` (its primary checkout is still a
//     worktree, so in practice the primary path is present).
func (s *Server) handleExperimentalWorktreeList(w http.ResponseWriter, r *http.Request) {
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	sourceDir := location.directory
	if declared, ok := worktreeCreateDeclaredDirectory(w, r); !ok {
		return
	} else if declared != "" {
		sourceDir = declared
	}

	ctx := r.Context()
	paths, err := worktreeRegisteredPaths(ctx, sourceDir)
	if err != nil {
		if worktreeNotGit(ctx, sourceDir) {
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeNotGitError",
				fmt.Sprintf("workspace %s is not a git repository: %v", sourceDir, err))
			return
		}
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeListFailedError",
			fmt.Sprintf("git worktree list failed in %s: %v", sourceDir, err))
		return
	}
	writeJSON(w, paths)
}

// handleExperimentalWorktreeRemove serves DELETE /experimental/worktree —
// upstream worktree.remove (ROUTE-ADD-083; declared responses: 200 boolean,
// 400 WorktreeError | InvalidRequestError; optional query parameters
// `directory` and `workspace`).
//
// ch:trace row=ROUTE-ADD-083 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.remove test=internal/shim/opencode/experimental_worktree_list_remove_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:foreman-local
//
// Served arms:
//   - the workspace is resolved with apiV2LocationSelector; a present-but-blank
//     `workspace` or `directory` selector answers the declared 400
//     InvalidRequestError (kind Query, the parameter named);
//   - the target is the `directory` query parameter, which must resolve INSIDE
//     the selected workspace (a path escape is the declared 400
//     InvalidRequestError) and must be a REGISTERED git worktree of that
//     repository — a path the caller merely typed is never removed;
//   - absent `directory` answers the declared 400 WorktreeRemoveFailedError:
//     without a selector the operation would name the primary checkout, and
//     this runtime refuses to remove its own primary worktree;
//   - the only real effect is `git worktree remove <target>`; its failure (for
//     example a dirty worktree) answers the declared 400
//     WorktreeRemoveFailedError carrying the git message;
//   - SUCCESS answers the declared 200 `true`, and only then — the directory
//     really is gone.
func (s *Server) handleExperimentalWorktreeRemove(w http.ResponseWriter, r *http.Request) {
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}
	workspace := location.directory

	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", param,
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}

	ctx := r.Context()
	registered, err := worktreeRegisteredPaths(ctx, workspace)
	if err != nil {
		if worktreeNotGit(ctx, workspace) {
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeNotGitError",
				fmt.Sprintf("workspace %s is not a git repository: %v", workspace, err))
			return
		}
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeRemoveFailedError",
			fmt.Sprintf("git worktree list failed in %s: %v", workspace, err))
		return
	}

	target := strings.TrimSpace(q.Get("directory"))
	if target == "" {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeRemoveFailedError",
			"worktree.remove needs an explicit directory query parameter: without one the only candidate is the primary checkout, which this runtime refuses to remove")
		return
	}

	absolute, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("query parameter \"directory\" %q is not a usable path", target))
		return
	}
	root, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("selected workspace %q is not a usable path", workspace))
		return
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("query parameter \"directory\" %q resolves outside the selected workspace %q", absolute, root))
		return
	}
	if relative == "." {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeRemoveFailedError",
			fmt.Sprintf("%q is the primary checkout of %s; worktree.remove only removes linked worktrees", absolute, root))
		return
	}

	if !worktreePathRegistered(registered, absolute) {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeRemoveFailedError",
			fmt.Sprintf("%q is not a registered git worktree of %s", absolute, root))
		return
	}

	if out, err := runGit(ctx, root, "worktree", "remove", absolute); err != nil {
		message := strings.TrimSpace(out)
		if message == "" {
			message = err.Error()
		}
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeRemoveFailedError",
			fmt.Sprintf("git worktree remove %s failed in %s: %s", absolute, root, message))
		return
	}

	writeJSON(w, true)
}

// worktreePathRegistered reports whether target is one of the registered
// worktree paths, comparing both the recorded and the symlink-resolved forms
// (git records the path it was given, which may differ from an EvalSymlinks
// answer on a machine with a symlinked temp root).
func worktreePathRegistered(registered []string, target string) bool {
	resolvedTarget := target
	if r, err := filepath.EvalSymlinks(target); err == nil {
		resolvedTarget = r
	}
	for _, candidate := range registered {
		if candidate == target || candidate == resolvedTarget {
			return true
		}
		if r, err := filepath.EvalSymlinks(candidate); err == nil && r == resolvedTarget {
			return true
		}
	}
	return false
}
