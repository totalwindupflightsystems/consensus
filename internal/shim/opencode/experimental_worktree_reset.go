package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// ROUTE-ADD-086 — POST /experimental/worktree/reset (worktree.reset)
// ============================================================================

// worktreeResetPath is the exact declared path this file serves. Registration
// in NewServer uses the same literal; the constant exists so the handler's
// error sentences and the route cannot drift apart.
const worktreeResetPath = "/experimental/worktree/reset"

// worktreeResetErrorName kinds the pinned document declares for this
// operation (components.schemas.WorktreeError.data.name enum members that this
// handler can truthfully answer).
const (
	worktreeResetNameNotGit  = "WorktreeNotGitError"
	worktreeResetNameFailed  = "WorktreeResetFailedError"
	worktreeResetMaxBodySize = 1 << 20
)

// handleExperimentalWorktreeReset serves POST /experimental/worktree/reset —
// upstream worktree.reset (ROUTE-ADD-086; declared responses: 200 boolean,
// 400 WorktreeError | InvalidRequestError;
// specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree/reset".post). Before this implementation the
// exact path had NO shim route at all, so every request fell through to
// net/http's default 404 (the declared-vs-served artifact's NOT-SERVED class,
// indistinguishable from "no such opencode operation").
//
// ch:trace row=ROUTE-ADD-086 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.reset test=internal/shim/opencode/experimental_worktree_reset_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local
//
// The declared contract, arm by arm:
//
//   - requestBody WorktreeResetInput {directory*: string},
//     additionalProperties:false — strictly decoded BEFORE any effect, so an
//     absent body, an empty object, an unknown member (the security-relevant
//     case: extra members must not be silently ignored), a null and a
//     wrong-typed or blank directory all answer the declared 400
//     InvalidRequestError (kind Body when the body is not exactly one object
//     carrying the declared member, kind Payload when the member violates its
//     declared type);
//   - the declared query selectors (directory, workspace) are the shared
//     deepObject location selector: a malformed/blank one answers the declared
//     400 InvalidRequestError (kind Query) through apiV2LocationSelector, the
//     same arm every sibling v2 operation answers;
//   - response 200 is a BARE boolean (not an object): the handler writes the
//     JSON literal `true` only after the reset actually happened on disk;
//   - response 400 anyOf[WorktreeError, InvalidRequestError]: every git
//     failure answers WorktreeError
//     {name: <WorktreeNotGitError|WorktreeResetFailedError>, data: {message}},
//     never an undeclared 5xx.
//
// TRUTHFULNESS — what "reset" means here, and why the 200 is not fabricated.
// The pinned upstream summary is "Reset a worktree branch to the primary
// default branch"; the effect this shim performs is the destructive pair the
// ROUTE-ADD-086 contract specifies, run IN the caller-named worktree:
//
//	git -C <directory> reset --hard HEAD
//	git -C <directory> clean -fd
//
// i.e. the worktree is returned to its own committed HEAD and its untracked
// files are removed. That is a real state change on this machine — the tests
// assert the restored file CONTENT, not the status code — so the declared 200
// reports state that exists. The shim does NOT claim to move the worktree onto
// the repository default branch; it reports exactly the reset it performed, and
// this paragraph is the honest limit of the translation.
//
// SAFETY — reset is destructive, and `directory` is caller-typed data. Before
// any git write the handler proves that the path is
//
//	(1) absolute and (after symlink resolution) INSIDE the workspace the
//	    request routes to — a `..` escape or a symlink pointing out of the
//	    workspace answers the declared 400 InvalidRequestError, so nothing
//	    outside the routed workspace can be touched; and
//	(2) a directory registered as a git worktree of that repository
//	    (`git -C <workspace> worktree list --porcelain`) — a mere subdirectory
//	    of the repository is NOT a worktree, and resetting it would silently
//	    reset its parent, so an unregistered path answers the declared 400
//	    WorktreeError (WorktreeResetFailedError) instead of acting on a typed
//	    path.
//
// Auth is untouched: the route is an api-key shim route, so an unauthenticated
// POST answers the shim's own 401 UNAUTHENTICATED envelope (this operation
// declares no 401 arm, so it is not a v2-envelope path). Every method on the
// exact path reaches this registration; undeclared methods keep the sibling
// 405 METHOD_NOT_ALLOWED envelope.
func (s *Server) handleExperimentalWorktreeReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	directory, ok := worktreeResetDecodeBody(w, r)
	if !ok {
		return
	}

	// (1) containment: an absolute path that resolves inside the routed
	// workspace. Both the lexical and the symlink-resolved spellings must be
	// contained, so neither "../.." nor a symlink can point the destructive
	// command outside.
	workspace := filepath.Clean(s.worktreeDeclaredWorkspace(r))
	target := filepath.Clean(directory)
	if !filepath.IsAbs(target) {
		writeV2InvalidRequest(w, "Payload", "directory",
			fmt.Sprintf(`field "directory" must be an absolute path, got %q`, directory))
		return
	}
	if !worktreeResetWithinWorkspace(workspace, target) {
		writeV2InvalidRequest(w, "Payload", "directory",
			fmt.Sprintf("field \"directory\" %q is outside the selected workspace %q: worktree reset is destructive and only acts inside the workspace the request routes to",
				directory, workspace))
		return
	}
	resolvedTarget := worktreeResetResolve(target)
	if !worktreeResetWithinWorkspace(worktreeResetResolve(workspace), resolvedTarget) {
		writeV2InvalidRequest(w, "Payload", "directory",
			fmt.Sprintf("field \"directory\" %q resolves to %q, outside the selected workspace %q",
				directory, resolvedTarget, workspace))
		return
	}

	info, statErr := os.Stat(resolvedTarget)
	if statErr != nil || !info.IsDir() {
		writeWorktreeResetError(w, r, worktreeResetNameFailed,
			fmt.Sprintf("worktree reset failed: %q is not a readable directory: %v", resolvedTarget, statErr))
		return
	}

	ctx := r.Context()

	// (2) registered-worktree guard: the path must be a worktree of the
	// repository the request routes to, not merely a directory inside it.
	registered, listErr := worktreeResetRegisteredWorktrees(ctx, workspace)
	if listErr != nil {
		writeWorktreeResetError(w, r, worktreeResetNameForGitFailure(ctx, workspace),
			fmt.Sprintf("worktree reset failed: cannot list the git worktrees of the workspace %q: %v", workspace, listErr))
		return
	}
	if !worktreeResetIsRegistered(registered, resolvedTarget) {
		writeWorktreeResetError(w, r, worktreeResetNameFailed,
			fmt.Sprintf("worktree reset failed: %q is not a registered git worktree of %q (registered worktrees: %s)",
				resolvedTarget, workspace, worktreeResetJoinPaths(registered)))
		return
	}

	// The REAL effect, in the caller-named worktree.
	if out, err := runGit(ctx, resolvedTarget, "reset", "--hard", "HEAD"); err != nil {
		writeWorktreeResetError(w, r, worktreeResetNameForGitFailure(ctx, resolvedTarget),
			worktreeResetGitFailure("git reset --hard HEAD", resolvedTarget, out, err))
		return
	}
	if out, err := runGit(ctx, resolvedTarget, "clean", "-fd"); err != nil {
		writeWorktreeResetError(w, r, worktreeResetNameForGitFailure(ctx, resolvedTarget),
			worktreeResetGitFailure("git clean -fd", resolvedTarget, out, err))
		return
	}

	// The declared response is a BARE boolean literal.
	writeJSON(w, true)
}

// worktreeResetDecodeBody strictly decodes the declared requestBody
// WorktreeResetInput {directory*: string} (additionalProperties:false) and
// returns the raw directory string. Every violation answers the declared 400
// InvalidRequestError and reports false: kind Body when the body is not exactly
// one JSON object carrying only the declared member, kind Payload when the
// declared member's value violates its type or is blank. Passing an empty
// string through is not possible — the schema marks directory required.
func worktreeResetDecodeBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Body == nil {
		writeV2InvalidRequest(w, "Body", "directory",
			`request body required: the declared WorktreeResetInput carries the required member "directory"`)
		return "", false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, worktreeResetMaxBodySize))
	dec.DisallowUnknownFields()
	var body struct {
		Directory json.RawMessage `json:"directory"`
	}
	if err := dec.Decode(&body); err != nil {
		// An unknown member is the additionalProperties:false arm; the
		// decoder's own sentence names it.
		writeV2InvalidRequest(w, "Body", "",
			fmt.Sprintf(`request body must be one JSON object carrying only the declared field "directory" (WorktreeResetInput, additionalProperties:false): %v`, err))
		return "", false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return "", false
	}
	if len(body.Directory) == 0 {
		writeV2InvalidRequest(w, "Body", "directory",
			`required field "directory" is missing (WorktreeResetInput requires directory)`)
		return "", false
	}
	// A JSON null unmarshals into a string without error but is not the
	// declared (non-nullable) string.
	if strings.TrimSpace(string(body.Directory)) == "null" {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must be a string, not null`)
		return "", false
	}
	var directory string
	if err := json.Unmarshal(body.Directory, &directory); err != nil {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must be a string`)
		return "", false
	}
	if strings.TrimSpace(directory) == "" {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must not be blank`)
		return "", false
	}
	return directory, true
}

// worktreeResetWithinWorkspace reports whether target (already cleaned) is the
// workspace itself or a path strictly inside it. filepath.Rel is the containment
// proof: any `..` escape yields a relative path that starts with "..", and a
// different volume/drive yields an absolute one.
func worktreeResetWithinWorkspace(workspace, target string) bool {
	if workspace == "" || target == "" {
		return false
	}
	rel, err := filepath.Rel(workspace, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// worktreeResetResolve returns the symlink-resolved spelling of path when it can
// be resolved, and the cleaned input otherwise (a path that does not exist yet
// still has to be judged lexically).
func worktreeResetResolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
		return resolved
	}
	return filepath.Clean(path)
}

// worktreeResetRegisteredWorktrees lists the worktree paths git reports for the
// repository the workspace belongs to (`git worktree list --porcelain`, whose
// entries are the "worktree <path>" lines). The error is returned so the caller
// can answer the declared WorktreeError instead of guessing.
func worktreeResetRegisteredWorktrees(ctx context.Context, workspace string) ([]string, error) {
	out, err := runGit(ctx, workspace, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(unquoteGitPath(strings.TrimSpace(rest))); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths, nil
}

// worktreeResetIsRegistered reports whether target names one of the listed
// worktrees, comparing both the cleaned and the symlink-resolved spellings.
func worktreeResetIsRegistered(registered []string, target string) bool {
	for _, candidate := range registered {
		if sameWorkspaceDirectory(candidate, target) {
			return true
		}
	}
	return false
}

// worktreeResetJoinPaths renders the registered worktree paths for an error
// sentence, so the caller learns what WAS registered next to what was refused.
func worktreeResetJoinPaths(paths []string) string {
	if len(paths) == 0 {
		return "(none)"
	}
	return strings.Join(paths, ", ")
}

// worktreeResetGitFailure builds the WorktreeError message for a failed git
// command, carrying git's own sentence when it produced one.
func worktreeResetGitFailure(command, directory, output string, err error) string {
	detail := strings.TrimSpace(output)
	if detail == "" {
		return fmt.Sprintf("worktree reset failed: %s in %q: %v", command, directory, err)
	}
	return fmt.Sprintf("worktree reset failed: %s in %q: %v: %s", command, directory, err, detail)
}

// worktreeResetNameForGitFailure maps a git failure onto the declared
// WorktreeError.data.name enum: WorktreeNotGitError when the directory is not
// inside a git repository, WorktreeResetFailedError otherwise. The probe is a
// real `git rev-parse --git-dir` rather than string matching on git's output, so
// the answer is the repository's own verdict.
func worktreeResetNameForGitFailure(ctx context.Context, directory string) string {
	if _, err := runGit(ctx, directory, "rev-parse", "--git-dir"); err != nil {
		return worktreeResetNameNotGit
	}
	return worktreeResetNameFailed
}

// writeWorktreeResetError emits the declared 400 WorktreeError arm:
// {name: <declared enum member>, data: {message}} — the shape
// components.schemas.WorktreeError declares and the upstream NamedError
// envelope the SDK reads.
func writeWorktreeResetError(w http.ResponseWriter, r *http.Request, name, message string) {
	writeOpencodeNamedError(w, r, http.StatusBadRequest, name, message)
}
