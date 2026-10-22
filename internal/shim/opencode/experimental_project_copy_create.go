package opencode

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ============================================================================
// ROUTE-ADD-070 — POST /experimental/project/{projectID}/copy
// (v2.projectCopy.create)
// ============================================================================

// projectCopyStrategyGitWorktree is the ONLY strategy id the pinned upstream
// runtime registers (packages/core/src/project/copy-strategies.ts:
// "git_worktree"; every other body value yields the declared ProjectCopyError
// through upstream's ProjectCopy.StrategyUnavailableError). The upstream
// document declares the member as a plain string, so the honest translation of
// "unknown strategy" is the declared 400 ProjectCopyError, not a fabricated
// worktree and not an undeclared code.
const projectCopyStrategyGitWorktree = "git_worktree"

// projectCopyNameMaxSuffix is upstream's destination-collision bound
// (packages/core/src/project/copy.ts: the slug gets `-<suffix>` up to
// suffix > 10 before DestinationExistsError).
const projectCopyNameMaxSuffix = 10

// handleExperimentalProjectCopyCreate serves the declared sibling POST on the
// exact path DELETE /experimental/project/{projectID}/copy registers —
// upstream v2.projectCopy.create (ROUTE-ADD-070; declared responses: 200
// ProjectCopyCopy {directory}, 400 ProjectCopyError | InvalidRequestError;
// specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy".post). Before this
// implementation the POST arm answered the typed 501 not_implemented envelope
// of the ROUTE-ADD-069 intentional narrowing (SHIM-NARROWED-U002), because no
// project-copy registry existed and the ROW's premise treated the registry as
// the blocker.
//
// ch:trace row=ROUTE-ADD-070 spec=specs/024-shim-support-matrix.md#project-copy test=internal/shim/opencode/experimental_project_copy_create_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local
//
// Why the declared 200 IS truthful here — the upstream source, not just the
// document: v2.projectCopy.create is NOT a registry read. The pinned upstream
// handler (packages/server/src/handlers/project-copy.ts) passes
// sourceDirectory = location.project.directory (the workspace the request
// ROUTES TO — the requestWorkspaceDir singleton this shim already serves) into
// ProjectCopy.create, whose only registered strategy ("git_worktree",
// packages/core/src/project/copy-strategies.ts) runs
// `git worktree add --detach <destination>/<name> HEAD`
// (packages/core/src/git.ts worktreeCreate) and returns
// {directory: <the created copy>} (ProjectCopy.Copy: required `directory`
// ONLY — no id, no timestamps, no registry keys). Upstream's project_directory
// TABLE is bookkeeping its refresh/GET surface uses; the declared response
// carries none of it, so serving the declared 200 requires a real filesystem
// effect this runtime can already perform (runGit, the projectInitGit
// ROUTE-FIX-006 precedent of a real workspace git effect), not a registry.
//
// What the handler does per declared arm:
//
//   - blank {projectID} → declared 400 InvalidRequestError (kind Path — the
//     sibling DELETE validation, unchanged);
//   - malformed deepObject location selector → declared 400
//     InvalidRequestError (kind Query, shared apiV2LocationSelector). The
//     RESOLVED selector directory is the operation's real sourceDirectory:
//     upstream's location.project.directory IS the routed workspace;
//   - a body violating the declared requestBody {strategy*: string,
//     directory*: string, name?: string} additionalProperties:false →
//     declared 400 InvalidRequestError (kind Body for a body that is not
//     exactly one object of declared fields, kind Payload for a wrong-typed
//     or unpaired declared member). Unlike the sibling DELETE, the body here
//     carries the REQUIRED members strategy and directory, so an absent or
//     empty body is NOT well-formed input — it is the declared 400;
//   - a strategy this runtime cannot perform → declared 400 ProjectCopyError
//     {name: "ProjectCopyError", data: {message, forceRequired?}} (upstream's
//     StrategyUnavailableError maps to exactly this arm; only "git_worktree"
//     is implemented, the only upstream-registered strategy);
//   - a destination that exists after the declared suffix attempts (upstream
//     appends -2..-10 to the name) → declared 400 ProjectCopyError
//     (upstream DestinationExistsError);
//   - create failure (source is not a git repository — the upstream
//     DirectoryUnavailableError arm — an unwritable destination, a failed
//     `git worktree add`) → declared 400 ProjectCopyError naming the git
//     failure, never an undeclared 5xx;
//   - SUCCESS → the real effect has happened (the created directory exists
//     and is a git worktree of the source repository), so the declared 200
//     ProjectsCopyCopy body {directory: <created absolute path>} names state
//     that is real on this machine. Nothing is fabricated: no registry row is
//     claimed (this runtime keeps none — the sibling DELETE stays its
//     no-regression idempotent no-op) and no id or timestamp is invented.
//
// Duplicate-worktree safety: `git worktree add` refuses an existing checkout
// of the same tree and a destination already registered as a worktree, and a
// destination inside another worktree answers the failure the command emits —
// all three surface as the declared 400 ProjectCopyError, never silence.
//
// Auth is untouched: the standard api-key wall applies (unauthenticated POST
// answers the v2 UnauthorizedError 401 through the
// /experimental/project/<id>/copy shape rule in isV2EnvelopeRoutePath, shared
// with the sibling DELETE), and every method on the path reaches this
// registration — DELETE dispatches to the unchanged sibling handler,
// undeclared methods keep the sibling METHOD_NOT_ALLOWED 405 envelope.
func (s *Server) handleExperimentalProjectCopyCreate(w http.ResponseWriter, r *http.Request,
) {
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		writeV2InvalidRequest(w, "Path", "projectID", "projectID must not be blank")
		return
	}

	// The location selector is BOTH a validation arm and the operation's real
	// source: upstream resolves sourceDirectory from
	// location.project.directory, i.e. the workspace the request routes to —
	// exactly the directory apiV2LocationSelector already resolves
	// (x-opencode-directory header, location[directory] override, server
	// workdir). A well-formed selector therefore also SELECTS the source.
	location, ok := s.apiV2LocationSelector(w, r)
	if !ok {
		return
	}

	strategy, directory, name, ok := validateProjectCopyCreateBody(w, r)
	if !ok {
		return
	}

	// Only the strategy the upstream runtime itself registers can be served.
	if strategy != projectCopyStrategyGitWorktree {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
			fmt.Sprintf("project copy strategy unavailable: %q is not implemented by this runtime (registered strategy: %q)",
				strategy, projectCopyStrategyGitWorktree))
		return
	}

	// Upstream joins the destination under the requested directory and
	// generates a slug when no name is supplied; the destination collision
	// loop appends -<suffix> up to 10 (packages/core/src/project/copy.ts).
	cleanDir := filepath.Clean(directory)
	if !filepath.IsAbs(cleanDir) {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
			fmt.Sprintf("invalid project copy directory: %q is not an absolute path", directory))
		return
	}
	lastName := strings.TrimSpace(name)
	if lastName == "" {
		var err error
		lastName, err = projectCopySlug()
		if err != nil {
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
				"could not generate a copy name: "+err.Error())
			return
		}
	}
	copyDirectory := filepath.Join(cleanDir, lastName)
	for suffix := 2; suffix <= projectCopyNameMaxSuffix; suffix++ {
		if _, statErr := os.Lstat(copyDirectory); os.IsNotExist(statErr) {
			break
		}
		copyDirectory = filepath.Join(cleanDir, fmt.Sprintf("%s-%d", lastName, suffix))
	}
	if _, statErr := os.Lstat(copyDirectory); statErr == nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
			fmt.Sprintf("project copy destination already exists: %s", copyDirectory))
		return
	}

	// The REAL effect — the same argv the pinned upstream strategy runs
	// (git worktree add --detach <directory> HEAD, cwd = the source
	// repository). runGit strips GIT_* env and bounds the call by the request
	// context, so the add resolves the repo from the source directory alone.
	ctx := r.Context()
	if _, err := runGit(ctx, location.directory, "worktree", "add", "--detach", copyDirectory, "HEAD"); err != nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
			fmt.Sprintf("project copy failed: git worktree add %s: %v", copyDirectory, err))
		return
	}

	// Canonicalize the answer the way upstream returns it (fs.resolve on the
	// created path). A created directory that cannot be resolved afterwards
	// is a real failure, never a fabricated path.
	absolute, err := filepath.Abs(cleanPathOr(copyDirectory))
	if err != nil || absolute == "" {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "ProjectCopyError",
			"created project copy could not be resolved: "+err.Error())
		return
	}
	if resolved, symErr := filepath.EvalSymlinks(absolute); symErr == nil {
		absolute = resolved
	}
	writeJSON(w, map[string]any{"directory": absolute})
}

// validateProjectCopyCreateBody validates the declared requestBody of
// v2.projectCopy.create — {strategy*: string, directory*: string, name?:
// string} with additionalProperties:false — strictly and BEFORE any effect
// (the ptyUpdate strict-body convention). The members' own schema marks
// strategy and directory required, so — unlike the sibling DELETE's optional
// body — an absent, empty or {} body is NOT well-formed input here: it answers
// the declared 400 (kind Body for a body that is not exactly one JSON object
// of declared fields, kind Payload for a wrong-typed declared member).
func validateProjectCopyCreateBody(w http.ResponseWriter, r *http.Request,
) (strategy, directory, name string, ok bool) {
	if r.Body == nil {
		writeV2InvalidRequest(w, "Body", "",
			`request body required: the declared schema carries required "strategy" and "directory"`)
		return "", "", "", false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var body struct {
		Name      json.RawMessage `json:"name"`
		Strategy  json.RawMessage `json:"strategy"`
		Directory json.RawMessage `json:"directory"`
	}
	if err := dec.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "",
			`request body must be a JSON object carrying only the declared fields (strategy, directory, name)`)
		return "", "", "", false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return "", "", "", false
	}
	if (len(body.Strategy) == 0) != (len(body.Directory) == 0) {
		writeV2InvalidRequest(w, "Payload", "",
			`fields "strategy" and "directory" are declared required and must be carried together`)
		return "", "", "", false
	}
	if len(body.Strategy) == 0 {
		writeV2InvalidRequest(w, "Body", "",
			`required field "strategy" is missing (the declared body schema carries strategy and directory)`)
		return "", "", "", false
	}
	if err := json.Unmarshal(body.Strategy, &strategy); err != nil {
		writeV2InvalidRequest(w, "Payload", "strategy", `field "strategy" must be a string`)
		return "", "", "", false
	}
	if strings.TrimSpace(strategy) == "" {
		writeV2InvalidRequest(w, "Payload", "strategy", `field "strategy" must not be blank`)
		return "", "", "", false
	}
	if err := json.Unmarshal(body.Directory, &directory); err != nil {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must be a string`)
		return "", "", "", false
	}
	if strings.TrimSpace(directory) == "" {
		writeV2InvalidRequest(w, "Payload", "directory", `field "directory" must not be blank`)
		return "", "", "", false
	}
	if len(body.Name) > 0 {
		// A JSON null unmarshals into a string without error but would be an
		// undeclared (nullable) member — the declared name is a string.
		if strings.TrimSpace(string(body.Name)) == "null" {
			writeV2InvalidRequest(w, "Payload", "name", `field "name" must be a string, not null`)
			return "", "", "", false
		}
		if err := json.Unmarshal(body.Name, &name); err != nil {
			writeV2InvalidRequest(w, "Payload", "name", `field "name" must be a string`)
			return "", "", "", false
		}
		if name != "" && filepath.Base(name) != name {
			writeV2InvalidRequest(w, "Payload", "name",
				`field "name" must be a bare name, not a path (the copy name is joined under "directory")`)
			return "", "", "", false
		}
	}
	return strategy, directory, name, true
}

// cleanPathOr cleans a filesystem path, returning the raw input when Clean
// fails (it cannot for non-empty strings, but never panic on caller data).
func cleanPathOr(path string) string {
	if path == "" {
		return path
	}
	return filepath.Clean(path)
}

// projectCopySlug generates the two-word name an absent optional `name` is
// answered with — upstream's Slug.create (adjective-noun, two random words).
// The words are only a NAME (the effect is the git worktree at the joined
// path, which the response reports as the real directory), so a locally
// random choice is the same truthfulness class upstream itself has: the copy
// location it reports is the real one either way.
func projectCopySlug() (string, error) {
	adjectives := []string{
		"brave", "calm", "clever", "cosmic", "crisp", "curious", "eager",
		"gentle", "glowing", "happy", "hidden", "jolly", "kind", "lucky",
		"mighty", "misty", "neon", "nimble", "playful", "proud", "quick",
		"quiet", "shiny", "silent", "stellar", "sunny", "swift", "tidy",
		"witty",
	}
	nouns := []string{
		"cabin", "cactus", "canyon", "circuit", "comet", "eagle", "engine",
		"falcon", "forest", "garden", "harbor", "island", "knight", "lagoon",
		"meadow", "moon", "mountain", "nebula", "orchid", "otter", "panda",
		"pixel", "planet", "river", "rocket", "sailor", "squid", "star",
		"tiger", "wizard", "wolf",
	}
	adj, err := projectCopyRandomIndex(len(adjectives))
	if err != nil {
		return "", err
	}
	noun, err := projectCopyRandomIndex(len(nouns))
	if err != nil {
		return "", err
	}
	return adjectives[adj] + "-" + nouns[noun], nil
}

func projectCopyRandomIndex(n int) (int, error) {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return int(b[0]) % n, nil
}
