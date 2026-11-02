package opencode

import (
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-072 — POST /experimental/project/{projectID}/copy/refresh
// (v2.projectCopy.refresh)
// ============================================================================

// handleExperimentalProjectCopyRefresh serves the declared project-copy
// refresh operation on its own exact path — upstream v2.projectCopy.refresh
// (ROUTE-ADD-072; declared responses: 204 No Content, 400 ProjectCopyError |
// InvalidRequestError; declared parameters: the required plain-string
// {projectID} path segment and the optional deepObject location selector
// {directory, workspace}, additionalProperties:false; the pinned document
// declares NO requestBody —
// specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy/refresh".post). Before this
// implementation the exact path had no shim route at all, so every request
// fell through to net/http's default 404 (the declared-vs-served artifact's
// NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-072 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.projectCopy.refresh test=internal/shim/opencode/api_project_copy_refresh_test.go doc=none:runtime-route-change evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local
//
// Why the declared 204 IS truthful here — the sibling DELETE precedent, not a
// shortcut: v2.projectCopy.refresh upstream re-validates the project-copy
// bookkeeping its project_directory registry holds (the same registry the
// sibling rows read), and this runtime keeps NO project-copy registry —
// nothing in internal/db or the migration set records a copied project (the
// ROUTE-ADD-069 remove and ROUTE-ADD-070 create readings, unchanged). With no
// registry there is no projectID that addresses a held copy, so there is
// nothing to re-validate and refresh is the idempotent no-op of the sibling
// DELETE (the ROUTE-ADD-025 saved-permission removal rule): the declared 204
// — which has no body by definition — fabricates neither a stored copy nor a
// refresh claim about one. The operation declares NO 404 arm, so an id this
// runtime does not hold is never answered with an undeclared code; upstream's
// registry-dependent error arms (an unknown id, a vanished directory) have no
// reachable input here because no id resolves to a held copy in the first
// place. The declared 400 arm answers REAL malformed input:
//
//   - a present-but-blank {projectID} segment — the pinned document declares
//     `projectID` as a plain required string with NO pattern, so blank is the
//     only ill-shaped id the contract itself defines (kind Path, the sibling
//     DELETE validation, unchanged);
//   - a malformed deepObject location selector — an unknown selector key, a
//     present-but-blank value, or a workspace value outside the declared
//     shape (kind Query, the shared apiV2LocationSelector verdict, the same
//     declared selector the sibling create/remove operations carry).
//
// The document declares no requestBody, so — the /global/dispose and
// session.active rule — a request body is not input this contract defines and
// is neither read nor refused; the only declared 400 shapes are the two arms
// above, both emitted as the InvalidRequestError arm
// ({_tag, message, kind, field}) of the operation's declared anyOf through
// writeV2InvalidRequest.
//
// Auth is untouched: the standard api-key wall applies (/experimental/* is
// not a stub path), and the unauthenticated 401 is the v2 UnauthorizedError
// envelope because v2.projectCopy.refresh is a v2.* operation — the
// <non-empty-id>/copy/refresh shape rule in isV2EnvelopeRoutePath, shared
// with the sibling DELETE/POST. The exact registration means every method on
// the path reaches this handler: POST is the only method the document
// declares, so every other method answers the sibling METHOD_NOT_ALLOWED 405
// envelope, and no /experimental/project/* catch-all is introduced — the
// remaining declared /copy sibling (experimental.projectCopy.generateName)
// and undeclared shapes keep the net/http default 404 their own contract
// rows are filed against.
func (s *Server) handleExperimentalProjectCopyRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		writeV2InvalidRequest(w, "Path", "projectID", "projectID must not be blank")
		return
	}

	// The declared deepObject location selector is validated exactly like the
	// sibling create/remove operations; refresh reads nothing from it (there
	// is no registry to re-validate) but the declared 400 kind Query arm is
	// real either way.
	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}

	// The declared success: an empty 204. Nothing is created, nothing is
	// deleted, no registry row is claimed — the idempotent no-op of the
	// sibling DELETE, because the runtime keeps no project-copy registry.
	w.WriteHeader(http.StatusNoContent)
}
