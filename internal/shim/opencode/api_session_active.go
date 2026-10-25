package opencode

import (
	"net/http"
)

// ============================================================================
// ROUTE-ADD-039 — GET /api/session/active (v2.session.active)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session/active".get) declares v2.session.active — "Retrieve
// foreground Session drains currently owned by this OpenCode process.
// Sessions absent from the result are inactive." — with declared responses
//
//	200 — {data: {<session id matching ^ses>: SessionActive{type: "running"}}}
//	      (data is REQUIRED; the schema sets additionalProperties:false, and
//	      every key of the data object must match the ^ses pattern, its value
//	      the SessionActive schema whose only declared member is the required
//	      enum `type: running`)
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//
// The document declares GET as the ONLY method on this path and an EMPTY
// parameters list (no query parameters, no requestBody), so there is no
// sibling method to dispatch and no declared query input to validate (the
// handleGlobalDispose convention: a handler cannot reject input the document
// does not declare — inventing a constraint would fabricate one).
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation". The route is
// registered as its own EXACT pattern (server.go, MountPatterns
// "/api/session/active"); it is never an /api/session/* catch-all, so every
// sibling /api/session/{sessionID}[...] operation keeps the classification
// its own contract row is filed against.
//
// Why the 200 answers an EMPTY data object rather than one entry per running
// session:
//
//   - The declared result is keyed by session ids matching ^ses, and the
//     runtime's real sessions store (migration 001 §3.2, the table the v1
//     /session routes and the harness read) carries native UUID ids.
//     Emitting a row under a rewritten "ses<uuid>" key would fabricate the
//     very identifier a client then uses to address the session — the
//     ROUTE-ADD-037 handleApiSessionList / handleApiPermissionRequestList
//     reshape-refusal precedent.
//   - More fundamentally, the operation's declared semantics scope the result
//     to foreground Session drains owned by THIS OpenCode process. The
//     Consensus shim is a protocol adapter over the Consensus API service,
//     not an OpenCode process: it owns no foreground session drains. The
//     sessions table records Consensus agent runs whose status vocabulary
//     (booting/idle/planning/thinking/tool_exec/executing/waiting_sub/
//     paused/completed/failed) describes a harness lifecycle, and no row in
//     it IS an upstream OpenCode session drain. The truthful active set owned
//     by this process is therefore empty, and the declared 200 answers the
//     empty, non-null object {"data": {}} — exactly the truthfulness rule
//     api_v2_lists.go records for the sibling v2 read-only operations whose
//     collections have no truthful source.
//
// Arms:
//
//	200 — the declared {data: {…^ses…}} envelope with the empty, non-null
//	      data object.
//	400 InvalidRequestError — an unreadable sessions store: this operation
//	      declares no 5xx arm, so a store that cannot be read is reported
//	      through the declared InvalidRequestError (the handleApiSessionList /
//	      handleQuestionList convention) instead of claiming absence from a
//	      store the instance could not read.
//	401 UnauthorizedError — the /api/* auth wall (server.go
//	      v2EnvelopeRoutePaths carries this exact path).
//	405 METHOD_NOT_ALLOWED — every other method; GET is the only method the
//	      pinned document declares on this path, so no sibling operation is
//	      reclassified by this registration (contrast /api/session, whose
//	      declared sibling POST is v2.session.create).
//
// ch:trace row=ROUTE-ADD-039 spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.active wave=.coding-hermes/waves/consensus-foreman-2026-10-25-19-22-41.json#ROUTE-ADD-039 test=internal/shim/opencode/api_session_active_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-foreman-2026-10-25-19-22-41-ROUTE-ADD-039-brief.txt evidence=docs/evidence/ROUTE-ADD-039-live-probe.md witness=none:worker-local-until-foreman-verifies verdict=none:foreman-pending commit=none:worker-pending memory=none:foreman-pending
//
// handleApiSessionActive serves GET /api/session/active — upstream
// v2.session.active (ROUTE-ADD-039, declared responses 200 {data:
// {^ses: SessionActive}}, 400 InvalidRequestError, 401 UnauthorizedError).
// See the file doc comment for the arm-by-arm contract and for why the
// declared 200 carries an empty data object.
func (s *Server) handleApiSessionActive(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	// The runtime's real sessions store decides the one non-auth failure
	// arm: a store this instance cannot read is not proof that no session is
	// active, and this operation declares no 5xx code, so the failure is
	// reported through the declared InvalidRequestError 400 (the
	// handleApiSessionList / handleApiSkillList convention) instead of a
	// fabricated empty 200.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to list active sessions: the shim has no session store")
		return
	}
	if _, err := s.db.Query(r.Context(), `SELECT id, status FROM sessions`); err != nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to list active sessions: "+err.Error())
		return
	}

	// Readable store, no truthfully expressible active entry (file doc
	// comment): the declared success arm is the empty, non-null object.
	writeJSON(w, map[string]any{
		"data": map[string]any{},
	})
}
