package opencode

import (
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-040 — GET /api/session/{sessionID} (v2.session.get)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session/{sessionID}".get) declares v2.session.get with declared
// responses
//
//	200 — {data: SessionV2Info} (data REQUIRED; the schema sets
//	      additionalProperties:false; SessionV2Info requires id, projectID,
//	      cost, tokens, time, title, location)
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//	404 — NotFoundError
//
// and exactly one parameter: the required path sessionID with pattern ^ses.
// GET is the ONLY method the document declares on this path.
//
// Before ROUTE-ADD-040 the path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation". The route is
// registered as its own EXACT one-segment pattern; never an /api/session/*
// catch-all, so the deeper declared /api/session/{sessionID}/... operations
// keep the answers their own contract rows are filed against.
//
// ch:trace row=ROUTE-ADD-040 spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.get wave=none:serial worker brief=/tmp/consensus-foreman-2026-10-27-08-30-23-ROUTE-ADD-040-brief.txt test=internal/shim/opencode/api_session_get_test.go evidence=internal/shim/opencode/api_session_get_test.go witness=none:pending verdict=none:pending commit=none:pending memory=none:pending
//
// Arms:
//
//	200 — the declared {data: {...}} envelope carrying the runtime-native
//	      members the sessions row actually holds (id, agent_name, goal,
//	      status — exactly the member set the merged v2.session.create
//	      response ROUTE-ADD-038 answers with, so a client that creates via
//	      v2 and gets via v2 sees the same names for the same row). The
//	      declared SessionV2Info members beyond those have no truthful
//	      source in this runtime: the ^ses-patterned id cannot be rewritten
//	      from the native UUID without fabricating the identifier a client
//	      then addresses, and projectID/cost/tokens/time/location have no
//	      counterpart columns (the ROUTE-ADD-037 handleApiSessionList
//	      reshape-refusal enumeration) — invented members are never emitted
//	      (ROUTE-ADD-094/095/096: no fabricated declared success members).
//	400 InvalidRequestError — a present-but-blank sessionID segment (%20).
//	      The declared ^ses pattern is NOT enforced against the store
//	      lookup: this runtime's session ids are the ids it mints
//	      (native UUIDs — the same divergence v2.session.create's 200
//	      already answers truthfully), and an id this instance does not
//	      know is the declared 404 whatever its shape. A sessions store
//	      that cannot be read answers the same 400 (the
//	      handleApiSessionActive convention: absence is never claimed from
//	      a store the instance could not read, and this operation declares
//	      no 5xx arm).
//	401 UnauthorizedError — the /api/* auth wall's v2 envelope (server.go
//	      isV2EnvelopeRoutePath admits the exact one-segment
//	      /api/session/{sessionID} shape via the /api/session/ rule, the
//	      /api/provider and /api/pty family precedent).
//	404 NotFoundError — the sessionID names no session in this instance's
//	      real sessions store (migration 001 §3.2, the table the v1
//	      /session routes and the harness read); the typed NamedError body
//	      {name: "NotFoundError", data: {message: "Session not found"}} is
//	      the shape the pinned upstream sdk-error-shape suite extracts
//	      (TestSessionNotFoundNamedError404).
//	405 METHOD_NOT_ALLOWED — every other method; GET is the only method
//	      the pinned document declares on this path, so no sibling
//	      operation is reclassified by this registration.
//
// The document declares no query parameters and no requestBody for this
// operation, so undeclared query parameters are ignored, not refused (the
// handleGlobalDispose / handleApiSessionList convention: a handler cannot
// reject input the document does not declare).
func (s *Server) handleApiSessionGet(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Session", "sessionID",
			"sessionID path parameter is required")
		return
	}

	// The real sessions store decides the remaining arms: an unreadable
	// store is not proof a session is absent (400 above), and a readable
	// store with no such row is the declared 404 below.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to read session: the shim has no session store")
		return
	}
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id, agent_name, goal, status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			// The store read and answered: this instance holds no such
			// session — the declared NotFoundError 404 (the
			// TestApiSessionGetServesDeclared404 contract).
			writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError",
				"Session not found")
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to read session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError",
			"Session not found")
		return
	}

	// Readable store, real row: the declared 200 envelope carrying the
	// runtime-native members the row holds (file doc comment) — the same
	// member set v2.session.create (ROUTE-ADD-038) answers with.
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"id":         toString(row["id"]),
			"agent_name": toString(row["agent_name"]),
			"goal":       toString(row["goal"]),
			"status":     toString(row["status"]),
		},
	})
}
