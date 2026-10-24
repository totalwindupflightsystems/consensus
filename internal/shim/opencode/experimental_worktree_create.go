package opencode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// ROUTE-ADD-085 — POST /experimental/worktree (upstream worktree.create)
// ============================================================================

const (
	// worktreeCreateDirName is the parent directory a created sandbox worktree
	// is joined under, relative to the selected workspace (the row's declared
	// destination shape: <workspace>/.opencode-worktrees/<name>).
	worktreeCreateDirName = ".opencode-worktrees"

	// worktreeCreateNamePrefix and worktreeCreateRandBytes shape a generated
	// name: `wt-<8 hex>` from 4 crypto/rand bytes. The name is only a NAME —
	// the effect is the git worktree at the joined path, which the declared
	// 200 reports as the real directory — so a locally random suffix is the
	// same truthfulness class the upstream runtime's own generator has.
	worktreeCreateNamePrefix = "wt-"
	worktreeCreateRandBytes  = 4
)

// handleExperimentalWorktreeCreate serves POST /experimental/worktree —
// upstream worktree.create (ROUTE-ADD-085; declared responses: 200 Worktree
// {name*, branch?, directory*} and 400 WorktreeError | InvalidRequestError,
// requestBody WorktreeCreateInput {name?: string, startCommand?: string},
// specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/worktree".post). Before this route existed the exact
// path had NO shim route at all, so every method fell through to net/http's
// default 404 (the declared-vs-served artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-085 spec=specs/openapi/upstream/openapi-1.18.33.json#worktree.create test=internal/shim/opencode/experimental_worktree_create_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local
//
// Why the declared 200 is truthful: worktree.create is a git effect on the
// workspace the request routes to, not a registry read. The pinned upstream
// runtime creates the sandbox worktree with
// `git worktree add --detach <destination> HEAD`
// (packages/core/src/git.ts worktreeCreate — the same argv the sibling
// ROUTE-ADD-070 project-copy handler cites), which is exactly what this
// handler runs through runGit in the resolved workspace. The detached checkout
// is why the declared Worktree body's optional `branch` member stays ABSENT:
// --detach creates no branch, so claiming one would be a fabrication. Nothing
// else is claimed either — no id, no timestamps, no registry row.
//
// What the handler does per declared arm:
//
//   - a method other than POST — GET (upstream worktree.list) and DELETE
//     (upstream worktree.remove) are declared on this same path — answers the
//     sibling METHOD_NOT_ALLOWED 405 envelope, exactly as the /experimental/*
//     method guards do. ROUTE-ADD-084 (GET list) and ROUTE-ADD-083 (DELETE
//     remove) will extend this dispatcher in later rows; this row implements
//     POST only and claims nothing about the others;
//   - the workspace is resolved with apiV2LocationSelector (the same
//     shared selector the project-copy handler uses), whose resolved
//     directory is the operation's real source repository and the parent of
//     the created worktree. The operation ALSO declares plain `directory` and
//     `workspace` query selectors, so a present-but-blank one is malformed
//     input (declared 400 InvalidRequestError, kind Query, the parameter
//     named) and a valued `directory` SELECTS the source workspace (validated
//     as an existing directory). A valued `workspace` is accepted and
//     resolves nothing: this runtime keeps no workspace store, so it neither
//     scopes nor invents one;
//   - a body violating the declared WorktreeCreateInput (an object with
//     additionalProperties:false over {name?, startCommand?}) answers the
//     declared 400 InvalidRequestError with kind Body and the offending member
//     named in `field`. Both members are OPTIONAL, so an absent or empty body
//     is legal input and means "generate a name"; a non-object body (array,
//     string, `null`), an unknown member, a trailing second JSON value or a
//     wrong-typed member is not;
//   - an unsafe `name` (a path separator, "..", a leading "-", NUL, "." —
//     i.e. anything that could escape <workspace>/.opencode-worktrees/) is the
//     same declared 400 InvalidRequestError, field "name". No default name is
//     silently substituted for an explicit one;
//   - an absent/empty `name` is generated as `wt-<8 hex>`;
//   - a well-formed body carrying `startCommand` answers the declared 400
//     WorktreeStartCommandFailedError naming the field, BEFORE any effect —
//     the worktree is not created and the value is never executed (DF-CONSENSUS-64;
//     the pinned document declares this arm in the WorktreeError enum for
//     exactly this subject, and this runtime has no per-project start-command
//     registry, no shell and no process lifecycle to run a script "after the
//     project's start command");
//   - the workspace is not a git repository → declared 400
//     WorktreeNotGitError {name, data:{message}} (upstream's own arm);
//   - any create failure — destination already exists, `git worktree add`
//     fails (already-registered checkout, unwritable destination, locked
//     index) → declared 400 WorktreeCreateFailedError {name, data:{message}}.
//     Never an undeclared 5xx and never a fabricated success;
//   - SUCCESS → the worktree really exists on this machine (the test proves it
//     with `git worktree list` and a HEAD read), so the declared 200 carries
//     exactly {name, directory} with the absolute destination path.
//
// Auth is untouched: the api-key wall applies like every other implemented
// route. This operation declares NO 401 arm (its response set is 200 and
// 400), so an unauthenticated request keeps the shim's own UNAUTHENTICATED
// envelope instead of the v2 UnauthorizedError body — the same choice the
// sibling experimental.* handlers (e.g. /experimental/capabilities) make, and
// isV2EnvelopeRoutePath is deliberately NOT extended for this path.
func (s *Server) handleExperimentalWorktreeCreate(w http.ResponseWriter, r *http.Request) {
	// Method dispatch on the one registered pattern. GET (ROUTE-ADD-084,
	// worktree.list) and DELETE (ROUTE-ADD-083, worktree.remove) are declared on
	// this same path and are served by experimental_worktree_list_remove.go;
	// anything else keeps the honest 405 rather than a fabricated answer or a
	// 404 that would erase the fact that the route is served.
	switch r.Method {
	case http.MethodGet:
		s.handleExperimentalWorktreeList(w, r)
		return
	case http.MethodDelete:
		s.handleExperimentalWorktreeRemove(w, r)
		return
	case http.MethodPost:
		// fall through to the create arm below
	default:
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET, POST or DELETE")
		return
	}

	// The location selector is BOTH a validation arm and the operation's real
	// source: the resolved directory is the repository the worktree is created
	// from and the parent of the created worktree.
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

	name, startCommand, ok := worktreeCreateDecodeBody(w, r)
	if !ok {
		return
	}
	if startCommand != "" {
		// Declared-contract refusal (DF-CONSENSUS-64): the pinned document
		// declares WorktreeStartCommandFailedError in the WorktreeError enum
		// for exactly this subject. This runtime executes no startup scripts
		// on a request (it has no per-project start-command registry, no
		// shell, and no process lifecycle to run one "after the project's
		// start command"), so a well-formed body carrying startCommand answers
		// this declared 400 BEFORE any effect: the worktree is not created and
		// the value is never executed. The previous behaviour — accept the
		// field, disclose it only in a server-side WARN no HTTP client ever
		// reads, and answer the declared 200 anyway — was the partial truth
		// this arm removes.
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeStartCommandFailedError",
			`field "startCommand" is not supported: this runtime executes no startup scripts, so the worktree was not created`)
		return
	}
	if name == "" {
		generated, err := worktreeCreateGenerateName()
		if err != nil {
			// The declared WorktreeError enum names this failure mode, so it is
			// served as the declared 400 arm rather than an undeclared 5xx.
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeNameGenerationFailedError",
				"could not generate a worktree name: "+err.Error())
			return
		}
		name = generated
	}

	ctx := r.Context()

	// The declared WorktreeNotGitError arm: without a repository there is
	// nothing to add a worktree to. `rev-parse --git-dir` is the cheapest
	// honest probe — it succeeds in a bare or non-bare repository and fails in
	// a plain directory.
	if _, err := runGit(ctx, sourceDir, "rev-parse", "--git-dir"); err != nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeNotGitError",
			fmt.Sprintf("workspace %s is not a git repository: %v", sourceDir, err))
		return
	}

	destination := filepath.Join(sourceDir, worktreeCreateDirName, name)
	// `git worktree add` refuses a path that already exists; checking first
	// lets the declared WorktreeCreateFailedError carry the real reason instead
	// of a bare exit status.
	if info, statErr := os.Lstat(destination); statErr == nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeCreateFailedError",
			fmt.Sprintf("worktree destination already exists: %s (%s)", destination, info.Mode().Type()))
		return
	}

	// The REAL effect — the argv the pinned upstream runtime runs. runGit
	// strips GIT_* env and bounds the call by the request context, so the add
	// resolves the repository from sourceDir alone.
	if _, err := runGit(ctx, sourceDir, "worktree", "add", "--detach", destination, "HEAD"); err != nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeCreateFailedError",
			fmt.Sprintf("git worktree add %s HEAD failed in %s: %v", destination, sourceDir, err))
		return
	}

	// After the add, a destination that cannot be resolved is a real failure,
	// never a fabricated path.
	absolute, err := filepath.Abs(destination)
	if err != nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "WorktreeCreateFailedError",
			fmt.Sprintf("created worktree %s could not be resolved: %v", destination, err))
		return
	}
	if resolved, symErr := filepath.EvalSymlinks(absolute); symErr == nil {
		absolute = resolved
	}

	// The declared Worktree body: name and directory are required, branch is
	// optional and absent because --detach created no branch.
	writeJSON(w, map[string]any{
		"name":      name,
		"directory": absolute,
	})
}

// worktreeCreateDeclaredDirectory validates the operation's declared plain
// `directory` / `workspace` query selectors and returns the selected source
// directory (empty when the caller named none, in which case the location
// selector's directory stands).
//
// worktree.create declares both as optional plain strings. A present-but-blank
// selector is malformed input (declared 400 InvalidRequestError, kind Query,
// the parameter named) — the sibling experimental.* convention. A valued
// `directory` must name a readable directory, since it selects the repository
// the worktree is created from; a valued `workspace` is accepted and resolves
// nothing because this runtime keeps no workspace store.
func worktreeCreateDeclaredDirectory(w http.ResponseWriter, r *http.Request) (string, bool) {
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", param,
				fmt.Sprintf("query parameter %q must not be blank", param))
			return "", false
		}
	}

	declared := strings.TrimSpace(q.Get("directory"))
	if declared == "" {
		return "", true
	}
	absolute, err := filepath.Abs(filepath.Clean(declared))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("query parameter \"directory\" %q is not a usable path", declared))
		return "", false
	}
	info, err := os.Stat(absolute)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("query parameter \"directory\" %q cannot be read: %v", absolute, err))
		return "", false
	}
	if !info.IsDir() {
		writeV2InvalidRequest(w, "Query", "directory",
			fmt.Sprintf("query parameter \"directory\" %q is not a directory", absolute))
		return "", false
	}
	return absolute, true
}

// worktreeCreateDecodeBody decodes the declared WorktreeCreateInput
// {name?: string, startCommand?: string} (additionalProperties:false) strictly
// and BEFORE any effect. Both members are optional, so an absent, empty or {}
// body is well-formed and simply generates a name. Everything else is the
// declared 400 InvalidRequestError with kind Body and the offending member
// named in `field`: a non-object body (array, string, `null`), a second
// trailing JSON value, an undeclared member, a wrong-typed member (including a
// JSON null for a declared string) and an unsafe name.
func worktreeCreateDecodeBody(w http.ResponseWriter, r *http.Request) (name, startCommand string, ok bool) {
	if r.Body == nil {
		return "", "", true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			// No body at all: legal, both declared members are optional.
			return "", "", true
		}
		writeV2InvalidRequest(w, "Body", "",
			`request body must be a JSON object carrying only the declared fields (name, startCommand)`)
		return "", "", false
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return "", "", false
	}

	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		// Arrays, strings, numbers and booleans are not the declared object.
		writeV2InvalidRequest(w, "Body", "",
			`request body must be a JSON object carrying only the declared fields (name, startCommand)`)
		return "", "", false
	}
	if members == nil {
		// A literal JSON null unmarshals into a nil map without an error, but
		// it is not the declared object.
		writeV2InvalidRequest(w, "Body", "",
			`request body must be a JSON object carrying only the declared fields (name, startCommand)`)
		return "", "", false
	}
	for member := range members {
		if member != "name" && member != "startCommand" {
			writeV2InvalidRequest(w, "Body", member,
				fmt.Sprintf("unknown field %q (the declared body schema allows only name and startCommand)", member))
			return "", "", false
		}
	}

	if value, present := members["name"]; present {
		if strings.TrimSpace(string(value)) == "null" {
			writeV2InvalidRequest(w, "Body", "name", `field "name" must be a string, not null`)
			return "", "", false
		}
		if err := json.Unmarshal(value, &name); err != nil {
			writeV2InvalidRequest(w, "Body", "name", `field "name" must be a string`)
			return "", "", false
		}
		// An explicit empty string is the same request as an absent name.
		name = strings.TrimSpace(name)
		if name != "" {
			if reason := worktreeCreateUnsafeNameReason(name); reason != "" {
				writeV2InvalidRequest(w, "Body", "name", reason)
				return "", "", false
			}
		}
	}

	if value, present := members["startCommand"]; present {
		if strings.TrimSpace(string(value)) == "null" {
			writeV2InvalidRequest(w, "Body", "startCommand", `field "startCommand" must be a string, not null`)
			return "", "", false
		}
		if err := json.Unmarshal(value, &startCommand); err != nil {
			writeV2InvalidRequest(w, "Body", "startCommand", `field "startCommand" must be a string`)
			return "", "", false
		}
	}

	return name, startCommand, true
}

// worktreeCreateUnsafeNameReason reports why a caller-supplied name cannot be
// joined under <workspace>/.opencode-worktrees/, or "" when it can. The name is
// a single path segment: anything that could leave the worktree directory
// (separators, "..", ".") or be read as a git option (a leading "-") is
// refused with the declared 400 rather than silently rewritten.
func worktreeCreateUnsafeNameReason(name string) string {
	switch {
	case name == "." || name == "..":
		return fmt.Sprintf("field \"name\" %q is not a usable directory name", name)
	case strings.ContainsAny(name, "/\\"):
		return `field "name" must not contain a path separator (the name is one directory under the workspace)`
	case strings.Contains(name, ".."):
		return `field "name" must not contain ".."`
	case strings.HasPrefix(name, "-"):
		return `field "name" must not start with "-"`
	case strings.ContainsRune(name, 0):
		return `field "name" must not contain a NUL byte`
	}
	return ""
}

// worktreeCreateGenerateName generates the `wt-<8 hex>` name an absent (or
// empty) optional `name` is answered with.
func worktreeCreateGenerateName() (string, error) {
	var buf [worktreeCreateRandBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return worktreeCreateNamePrefix + hex.EncodeToString(buf[:]), nil
}
