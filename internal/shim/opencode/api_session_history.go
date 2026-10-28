package opencode

// ============================================================================
// ROUTE-ADD-045 — GET /api/session/{sessionID}/history (v2.session.history)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."\/api/session/{sessionID}/history".get) declares v2.session.history
// with declared responses
//
//	200 — {data: SessionDurableEvent[], hasMore: boolean} (both members
//	      REQUIRED, additionalProperties:false)
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//	404 — SessionNotFoundError
//
// and three declared parameters: the required path sessionID (plain string,
// declared pattern ^ses) plus the OPTIONAL query selectors limit and after
// (both declared as plain strings). No requestBody. GET is the ONLY method
// the document declares on this path.
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation".
//
// What the runtime truthfully keeps (the HISTORY SEMANTICS): the runtime's
// append-only ledger is memory_events (migration 001 §3.1 — types header,
// text_block, tool_call, tool_result, thinking, system, inherited_pointer,
// user_message), with per-event display state in the separate display_modes
// table (full / compressed / hidden; memory_events carries NO display_mode
// column by design). Compressed rows keep their compact view in
// memory_events.summary_text. The declared SessionDurableEvent oneOf (each
// member requires an id matching ^evt_ and a type from the
// session.next.* discriminator enum) has NO truthful counterpart: memory ids
// are the native autoincrement integers a client addresses, minting "evt_…"
// ids would fabricate the very identifiers the rest of this surface then
// fails to resolve, and no ledger row carries a session.next.* discriminator.
// Per the reshape-refusal convention (ROUTE-ADD-037 list, ROUTE-ADD-040 get,
// ROUTE-ADD-026 provider list) the DECLARED envelope is answered —
// {data, hasMore, additionalProperties:false} — while the entries inside
// data are the runtime-native members the rows actually hold. An empty
// ledger answers the declared 200 with an empty non-null data array.
//
// ch:trace row=ROUTE-ADD-045 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.history test=internal/shim/opencode/api_session_history_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-28-07-41-25.json evidence=docs/evidence/ROUTE-ADD-045-live-probe.md witness=none:unattended-worker-session commit=none:pending-foreman-trace STATUS: complete TIER: T3 (live-probed: every declared arm curl-verified on a scratch instance; transcript committed) EVIDENCE: docs/evidence/ROUTE-ADD-045-live-probe.md
//
// Arm map (mirrors api_session_agent.go, ROUTE-ADD-041):
//
//	200 — the declared {data, hasMore} envelope over an honest replay of the
//	      session's memory_events ordered by id ASC: full rows carry content,
//	      compressed rows carry summary_text (content withheld — the compact
//	      view IS the stored truth), hidden rows are omitted from the listing
//	      (display state respected, the handleSessionMessages convention).
//	400 InvalidRequestError — a present-but-blank sessionID segment (%20)
//	      (kind Path, field sessionID); a present-but-blank or non-positive
//	      limit selector (kind Query, field limit); a present-but-blank or
//	      non-numeric after selector (kind Query, field after — the runtime
//	      resume token is the last-seen memory_events id, so the declared
//	      "after" selector must be a non-negative integer); or an unreadable
//	      store (this operation declares no 5xx arm, so absence is never
//	      claimed from a store the instance could not read).
//	401 UnauthorizedError — the /api/* auth wall; this exact deeper shape is
//	      admitted by isV2EnvelopeRoutePath through isApiSessionHistoryPath,
//	      so the body is the declared v2 envelope rather than the shim's own
//	      v1 UNAUTHENTICATED body.
//	404 SessionNotFoundError — the sessionID is not a session this instance
//	      holds ({_tag, sessionID, message}, the declared shape). The
//	      decision is read from the runtime's REAL sessions store with the
//	      shared isNoRowsResult seam, so a client distinguishes "no such
//	      session" from "an empty history".
//	405 METHOD_NOT_ALLOWED — every method other than GET; GET is the only
//	      method the pinned document declares on this path, so no sibling
//	      operation is reclassified by this registration.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// apiSessionEventsQuery replays a session's memory_events ledger rows with
// their per-event display state, oldest first. The LEFT JOIN keeps rows with
// no display_modes entry (mode defaults to full — migration 001 §3.2's
// DEFAULT) and selects the compact view (summary_text) the compression
// worker maintains alongside content.
const apiSessionEventsQuery = `SELECT me.id, me.type, me.content, me.summary_text, ` +
	`me.iteration_created, me.created_at, COALESCE(dm.mode, 'full') AS display_mode ` +
	`FROM memory_events me ` +
	`LEFT JOIN display_modes dm ON dm.memory_id = me.id ` +
	`WHERE me.session_id = $1`

// apiSessionHistoryDefaultLimit is the replay window when the declared limit
// selector is absent — the replaySessionEvents convention (the shim's other
// history-shaped read replays ~50 events).
const apiSessionHistoryDefaultLimit = 50

// handleApiSessionHistory serves GET /api/session/{sessionID}/history —
// upstream v2.session.history (ROUTE-ADD-045, declared responses 200, 400,
// 401, 404). See the file doc comment for the arm-by-arm contract and for
// what the runtime truthfully keeps as session history.
func (s *Server) handleApiSessionHistory(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	// The declared optional query selectors are the only query input the
	// document declares, so each present-but-invalid spelling is a declared
	// contract violation (400), while absent selectors are simply defaults.
	limit := apiSessionHistoryDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" || r.URL.Query().Has("limit") {
		parsed, ok := apiSessionParsePositiveInt(raw)
		if !ok {
			writeV2InvalidRequest(w, "Query", "limit",
				"limit must be a positive integer")
			return
		}
		limit = parsed
	}
	after := int64(0)
	if r.URL.Query().Has("after") {
		raw := r.URL.Query().Get("after")
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if raw == "" || err != nil || parsed < 0 {
			writeV2InvalidRequest(w, "Query", "after",
				"after must be a non-negative memory_events id (the runtime resume token)")
			return
		}
		after = parsed
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
	// sql.ErrNoRows sentinel on this seam), so the two answers are told
	// apart explicitly: a "no rows in result" reply is the declared 404,
	// while any OTHER read failure keeps the declared 400 (absence is never
	// claimed from a store the instance could not read).
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no history to report", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no history to report", sessionID))
		return
	}

	// Read the ledger. A store that resolved the session but cannot read its
	// events keeps the declared 400 (no 5xx arm on this operation).
	query := apiSessionEventsQuery
	args := []any{sessionID}
	if after > 0 {
		query += ` AND me.id > $2`
		args = append(args, after)
	}
	// One extra row decides hasMore without a second query.
	query += ` ORDER BY me.id ASC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, int64(limit)+1)
	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		writeV2InvalidRequest(w, "Shim", "memory_events",
			"failed to read session history: "+err.Error())
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	events := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		mode := toString(r["display_mode"])
		if mode == "hidden" {
			// Display state respected: a hidden event is not part of the
			// consumable history view (the harness consumes user turns
			// through exactly this state, migration 001 §3.2).
			continue
		}
		entry := map[string]any{
			// Runtime-native members the row really holds — never a
			// fabricated ^evt_ id or session.next.* discriminator (file doc
			// comment). memory_event_id is the identifier a client resumes
			// the declared `after` selector with.
			"memory_event_id":   toInt64(r["id"]),
			"type":              toString(r["type"]),
			"iteration_created": toInt64(r["iteration_created"]),
			"created_at":        toString(r["created_at"]),
		}
		if mode == "compressed" {
			// The compact view IS the stored truth for a compressed row:
			// summary_text when the store holds one, and the content is
			// withheld — never re-fabricated from the summary.
			entry["display_mode"] = mode
			if summary := strings.TrimSpace(toString(r["summary_text"])); summary != "" {
				entry["summary_text"] = summary
			}
		} else {
			entry["content"] = toString(r["content"])
		}
		events = append(events, entry)
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{
		"data":    events,
		"hasMore": hasMore,
	})
}

// apiSessionParsePositiveInt parses a declared count selector: a non-empty
// base-10 positive integer. A present-but-blank selector is the declared
// InvalidRequestError (a handler cannot distinguish blank from absent on a
// query key the document declares), as is any non-numeric or non-positive
// spelling.
func apiSessionParsePositiveInt(raw string) (int, bool) {
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 || parsed > int64(int(^uint(0)>>1)) {
		return 0, false
	}
	return int(parsed), true
}

// isApiSessionHistoryPath admits only the declared concrete route shape
// (/api/session/{sessionID}/history — session id non-empty, second segment
// literally "history") to the v2 UnauthorizedError auth branch. The sibling
// one-segment /api/session/{sessionID} shape, the sibling two-segment
// /agent and /model shapes and every deeper /api/session/* sub-path keep the
// shim's own envelope.
func isApiSessionHistoryPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	if !ok {
		return false
	}
	return sessionID != "" && tail == "history"
}
