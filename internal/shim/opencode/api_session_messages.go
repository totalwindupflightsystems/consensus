package opencode

// ============================================================================
// ROUTE-ADD-047 — GET /api/session/{sessionID}/message (v2.session.messages)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."\/api/session/{sessionID}/message".get) declares v2.session.messages
// with declared responses
//
//	200 — {data: SessionMessage[], cursor: {previous?, next?}} (data and
//	      cursor REQUIRED, additionalProperties:false)
//	400 — InvalidCursorError | InvalidRequestError
//	401 — UnauthorizedError
//	404 — SessionNotFoundError
//	500 — UnknownError — the ONLY member of this route family that declares
//	      a 5xx arm
//
// and the declared parameters: the required path sessionID (plain string,
// declared pattern ^ses) plus the OPTIONAL query selectors limit (number),
// order (enum asc|desc) and cursor (string). No requestBody. GET is the
// ONLY method the document declares on this path.
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class.
//
// What the runtime truthfully keeps: the same append-only memory_events
// ledger as history (api_session_history.go file doc comment) — a message
// IS a memory_events row, with per-event display state in display_modes.
// The declared SessionMessage anyOf requires an id matching ^msg_ on every
// member; the runtime's message identifiers are the native memory_events
// integers (the same reshape refusal as history — an id minted for one
// response would be an id every sibling route then fails to resolve). The
// DECLARED envelope is answered with the runtime-native members inside.
//
// ch:trace row=ROUTE-ADD-047 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.messages test=internal/shim/opencode/api_session_messages_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-28-07-41-25.json evidence=docs/evidence/ROUTE-ADD-045-live-probe.md witness=none:unattended-worker-session commit=none:pending-foreman-trace STATUS: complete TIER: T3 (live-probed: every declared arm curl-verified on a scratch instance; transcript committed) EVIDENCE: docs/evidence/ROUTE-ADD-045-live-probe.md
//
// Arm map:
//
//	200 — the declared {data, cursor} envelope over the session's
//	      memory_events (user_message + text_block + every other non-hidden
//	      ledger row, so a client reassembles the honest conversation view;
//	      hidden rows are omitted, compressed rows carry the stored compact
//	      view). Cursor members are emitted only when a neighbor page
//	      actually exists.
//	400 InvalidRequestError — a blank sessionID (kind Path); a present-but-
//	      blank or non-positive limit; an order outside the declared
//	      asc|desc enum; a present-but-blank cursor or one that names no
//	      memory_events row of this session (the runtime cursor token is the
//	      neighbor id, so an opaque string that never came from a previous
//	      page is a declared invalid cursor); or — this operation declares
//	      NO 500-free excuse — an unreadable store answers the declared 500
//	      UnknownError, never 400.
//	401 UnauthorizedError — the v2 envelope via isApiSessionMessagesPath.
//	404 SessionNotFoundError — unknown sessionID, decided against the real
//	      sessions store ({_tag, sessionID, message}).
//	500 UnknownError — a store the instance cannot read ({_tag:
//	      "UnknownError", message}, the declared shape) — absence is never
//	      claimed from an unreadable store, and unlike history/message-get
//	      this operation HAS a declared 5xx arm to answer with.
//	405 METHOD_NOT_ALLOWED — every method other than GET.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// handleApiSessionMessages serves GET /api/session/{sessionID}/message —
// upstream v2.session.messages (ROUTE-ADD-047, declared responses 200, 400,
// 401, 404, 500).
func (s *Server) handleApiSessionMessages(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	// Declared query selectors. Absent selectors are defaults; present-but-
	// invalid spellings are declared 400s (InvalidCursorError shares the
	// InvalidRequestError envelope body on this shim — both are {_tag,
	// message}).
	order := "asc"
	switch raw := r.URL.Query().Get("order"); {
	case !r.URL.Query().Has("order"):
		// default
	case raw == "asc", raw == "desc":
		order = raw
	default:
		writeV2InvalidRequest(w, "Query", "order", "order must be one of the declared values: asc, desc")
		return
	}

	limit := apiSessionHistoryDefaultLimit
	if r.URL.Query().Has("limit") {
		parsed, ok := apiSessionParsePositiveInt(r.URL.Query().Get("limit"))
		if !ok {
			writeV2InvalidRequest(w, "Query", "limit", "limit must be a positive number")
			return
		}
		limit = parsed
	}

	// The runtime cursor token is the boundary memory_events id carried in
	// cursor.next / cursor.previous of a previous page — the only cursor
	// strings this surface can honestly emit or consume. The declared
	// "do not combine with order" rule is enforced in the order the document
	// states it: a request that carries BOTH answers the declared 400.
	cursorID := int64(0)
	if r.URL.Query().Has("cursor") {
		raw := r.URL.Query().Get("cursor")
		if raw == "" {
			writeV2InvalidRequest(w, "Query", "cursor", "cursor must not be blank")
			return
		}
		if r.URL.Query().Has("order") {
			writeV2InvalidRequest(w, "Query", "cursor",
				"cursor must not be combined with order (the pinned document forbids it)")
			return
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			writeV2InvalidRequest(w, "Query", "cursor",
				"cursor must be the boundary memory_event_id of a previous page")
			return
		}
		cursorID = parsed
	}

	if s.db == nil {
		// Declared 500 (UnknownError): this operation declares the 5xx arm.
		writeV2UnknownError(w, "the shim has no session store")
		return
	}

	// Resolve the session first: unknown session is the declared 404 and a
	// store read failure here is the declared 500 (this route owns that
	// arm — never 400, unlike its siblings).
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no messages to list", sessionID))
			return
		}
		writeV2UnknownError(w, "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no messages to list", sessionID))
		return
	}

	// Cursor validity is decided against the store: a cursor token is the
	// boundary id of a PREVIOUS page, so it must name a real memory_events
	// row of THIS session — an opaque string that never came from one is the
	// declared invalid cursor.
	if cursorID > 0 {
		boundary, err := s.db.QueryRow(r.Context(),
			`SELECT id FROM memory_events WHERE session_id = $1 AND id = $2 LIMIT 1`,
			sessionID, cursorID)
		if err != nil {
			if isNoRowsResult(err) {
				writeV2InvalidRequest(w, "Cursor", "cursor",
					"cursor names no memory event of this session")
				return
			}
			writeV2UnknownError(w, "failed to resolve cursor: "+err.Error())
			return
		}
		if boundary == nil {
			writeV2InvalidRequest(w, "Cursor", "cursor",
				"cursor names no memory event of this session")
			return
		}
	}

	// Page the ledger. The cursor walks whichever side the boundary id sits
	// on (ids greater than the boundary when replaying forward, ids smaller
	// when paging backward); order is the declared enum, defaulting to asc.
	query := apiSessionEventsQuery
	args := []any{sessionID}
	if cursorID > 0 {
		if order == "desc" {
			query += ` AND me.id < $2`
		} else {
			query += ` AND me.id > $2`
		}
		args = append(args, cursorID)
	}
	query += ` ORDER BY me.id ` + order + ` LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, int64(limit)+1)
	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		writeV2UnknownError(w, "failed to list session messages: "+err.Error())
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	messages := make([]map[string]any, 0, len(rows))
	var firstID, lastID int64
	for i, r := range rows {
		mode := toString(r["display_mode"])
		if mode == "hidden" {
			continue
		}
		id := toInt64(r["id"])
		if i == 0 {
			firstID = id
		}
		lastID = id
		entry := map[string]any{
			// Runtime-native members (file doc comment) — memory_message_id
			// is the identifier the sibling v2.session.message route
			// resolves, never a fabricated ^msg_ id.
			"memory_message_id": id,
			"type":              toString(r["type"]),
			"iteration_created": toInt64(r["iteration_created"]),
			"created_at":        toString(r["created_at"]),
		}
		if mode == "compressed" {
			entry["display_mode"] = mode
			if summary := strings.TrimSpace(toString(r["summary_text"])); summary != "" {
				entry["summary_text"] = summary
			}
		} else {
			entry["content"] = toString(r["content"])
		}
		messages = append(messages, entry)
	}

	// Cursor members only when a neighbor page actually exists: next points
	// past the last row of an ascending page (or before the first row of a
	// descending one), previous points back at the page boundary.
	cursor := map[string]any{}
	if len(rows) > 0 {
		if hasMore {
			if order == "desc" {
				cursor["next"] = strconv.FormatInt(firstID, 10)
			} else {
				cursor["next"] = strconv.FormatInt(lastID, 10)
			}
		}
		if cursorID > 0 {
			cursor["previous"] = strconv.FormatInt(cursorID, 10)
		}
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{
		"data":   messages,
		"cursor": cursor,
	})
}

// writeV2UnknownError answers the declared 500 UnknownError arm
// ({_tag: "UnknownError", message}) — the shape only v2.session.messages in
// this family declares.
func writeV2UnknownError(w http.ResponseWriter, message string) {
	writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
		"_tag":    "UnknownError",
		"message": message,
	})
}

// isApiSessionMessagesPath admits only the declared concrete route shape
// (/api/session/{sessionID}/message — session id non-empty, second segment
// literally "message") to the v2 UnauthorizedError auth branch. The sibling
// one-segment shape, the /agent//history/model shapes, the three-segment
// message/{messageID} shape and every deeper /api/session/* sub-path keep
// the shim's own envelope.
func isApiSessionMessagesPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	if !ok {
		return false
	}
	return sessionID != "" && tail == "message"
}
