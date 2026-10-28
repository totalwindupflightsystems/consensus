package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-049 — POST /api/session/{sessionID}/model (v2.session.switchModel)
// (merged from wt/ROUTE-ADD-049, 3e0b67f; merge completed on main by
// INT-BOARD-1 — see the merge-completion notes below)
// ============================================================================

// sessionModelSwitch serves POST /api/session/{sessionID}/model — upstream
// v2.session.switchModel (ROUTE-ADD-049, SHIM-DRIFT-010; declared responses:
// 204 NoContent, 400 InvalidRequestError, 401 UnauthorizedError, 404
// NotFoundError).
//
// The Consensus runtime keeps no upstream OpenCode session model-switch
// pipeline — it parks no sessions, runs no processes, and has no model
// assignment concept — so the declared 204 is answered with an empty body as a
// truthful no-op: no model was switched because no model-switch mechanism
// exists.
//
// The declared 401 arm is served through the isApiSessionModelPath rule in
// server.go isV2EnvelopeRoutePath: the branch commit registered the path in
// v2EnvelopeRoutePaths, but that map is keyed by concrete request paths and a
// literal "/api/session/{sessionID}/model" entry can never match the
// parameterized request "/api/session/<id>/model", so the map entry alone
// served no unauthenticated request the v2 envelope. The rule admits only the
// declared two-segment shape, so sibling and deeper /api/session/* paths keep
// the shim's own envelope.
//
// The sessionID path parameter decides the declared 404 NotFoundError against
// the runtime's REAL sessions store (migration 001 §3.2): a blank segment
// answers the declared 400 InvalidRequestError, an id this instance does not
// hold answers the declared 404, and a well-formed request for a held session
// answers the declared 204 — the truthful answer, because there is no
// model-switch pipeline to run.
//
// MERGE COMPLETION (INT-BOARD-1, 2026-10-27): the branch commit's
// session-existence check read `row, err := s.db.QueryRow(...)` and answered
// 404 whenever `err != nil || row == nil`. On this tree internal/db reports a
// missing row as its OWN error value (there is no sql.ErrNoRows sentinel on
// this seam), so that check answered the declared 404 for BOTH "no such
// session" AND "the store could not be read" — the store-read failure was
// misreported as session absence. The merged handler resolves the session
// through the shared isNoRowsResult seam introduced by ROUTE-ADD-041
// (api_session_agent.go): a "no rows in result" reply is the declared 404,
// every OTHER read failure answers the declared InvalidRequestError 400
// (absence is never claimed from a store the instance could not read; this
// operation declares no 5xx arm).
//
// ch:trace row=ROUTE-ADD-049 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.switchModel wave=none:int-board-1-merge test=internal/shim/opencode/api_session_model_switch_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree verdict=none:foreman-will-run-after-merge commit=none:pre-commit memory=none:pending
func (s *Server) handleApiSessionModelSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	// An absent store cannot answer the 404 question, and this operation
	// declares no 5xx arm: report the unreadable store through the declared
	// InvalidRequestError instead of claiming the session does not exist.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: the shim has no session store")
		return
	}

	// Resolve the session against the runtime's real session store. The db
	// drivers in internal/db report a missing row as an error (there is no
	// sql.ErrNoRows sentinel on this seam), so the two answers are told apart
	// explicitly: a "no rows in result" reply is the declared 404, while any
	// OTHER read failure is reported through the declared InvalidRequestError.
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to switch", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to switch", sessionID))
		return
	}

	// The session is one this instance holds and the request is well formed.
	// The declared 204 is the truthful answer: this adapter holds no model
	// assignment for a Consensus session (file doc comment), so there is no
	// model binding to change and no data-bearing success to fabricate. The
	// declared success carries no body, so no claim is made.
	w.WriteHeader(http.StatusNoContent)
}

// isApiSessionModelPath admits only the declared concrete route shape
// (/api/session/{sessionID}/model — exactly two non-empty segments, the second
// literally "model") to the v2 UnauthorizedError auth branch. Sibling and
// deeper /api/session/* paths keep the shim's own envelope (the
// isApiSessionAgentPath precedent).
func isApiSessionModelPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/session/")
	if !ok {
		return false
	}
	sessionID, tail, ok := strings.Cut(rest, "/")
	if !ok {
		return false
	}
	return sessionID != "" && tail == "model"
}
