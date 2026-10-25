package opencode

import (
	"net/http"
	"strconv"
	"strings"
)

// ============================================================================
// ROUTE-ADD-037 — GET /api/session (v2.session.list)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session".get) declares v2.session.list with declared responses
// 200 SessionsResponse | 400 InvalidCursorError | InvalidRequestError | 401
// UnauthorizedError, and the optional query parameters
//
//	workspace  string, pattern ^wrk
//	limit      number
//	order      string, enum asc | desc
//	search     string
//	directory  string
//	project    string
//	subpath    string
//	cursor     opaque pagination cursor returned as cursor.previous / cursor.next
//	           in a previous response
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation".
//
// The route is registered as its own EXACT pattern (server.go, MountPatterns
// "/api/session"); it is never an /api/* catch-all, so every sibling
// /api/session/* operation (v2.session.get, v2.session.active, v2.session.prompt,
// ...) keeps the classification its own contract row is filed against.
//
// Arms:
//
//	200 — the declared SessionsResponse {data: SessionV2Info[], cursor: {}}
//	400 InvalidRequestError — a query parameter violating the declared schema
//	      (blank workspace/directory/project/subpath, a workspace that does not
//	      match ^wrk, an order outside the asc|desc enum, a limit that is not a
//	      number), and an unreadable sessions store (this operation declares no
//	      5xx arm, so a store that cannot be read is reported through the
//	      declared InvalidRequestError — the handleQuestionList convention).
//	400 InvalidCursorError — a supplied `cursor`. This handler never issues a
//	      cursor (see the 200 note), so no value a client can send was issued by
//	      this surface and the declared InvalidCursorError is the honest answer.
//	401 UnauthorizedError — the /api/* auth wall (server.go
//	      v2EnvelopeRoutePaths carries this exact path).
//	405 METHOD_NOT_ALLOWED — methods the pinned document does not declare on
//	      this path at all (PUT, PATCH, DELETE).
//	501 not_implemented — POST /api/session (v2.session.create), the declared
//	      sibling method on this exact path. It is a SEPARATE declared operation
//	      with its own contract row (ROUTE-ADD-038) and is answered the typed
//	      not_implemented envelope naming v2.session.create, exactly as bare
//	      POST /pty and POST /api/pty do (SHIM-GAP-002): registering the exact
//	      path makes the declared sibling method reach this handler, and a
//	      registered route may not lie with a 404/405-free empty success. The
//	      narrowing is recorded in the served surface and in
//	      notImplementedRoutes, and it leaves with the row that serves the
//	      operation for real.
//
// Why the 200 answers an EMPTY data array: the runtime's sessions store
// (migration 001, the table the v1 /session routes and the harness read) is
// real and IS read by this handler, but no row in it can be truthfully
// reshaped into the declared SessionV2Info:
//
//	id             required, pattern ^ses — sessions.id is a native UUID; both
//	               upstream id and parentID must match ^ses, and rewriting a
//	               native id into "ses<uuid>" would fabricate the very
//	               identifier a client would then use to address the session
//	               (the handleApiPermissionRequestList reshape-refusal
//	               precedent).
//	projectID      required — the store holds no project identity for a session
//	               (sessions.project_id names an internal project row, not the
//	               declared projectID string the v2 surface addresses).
//	cost           required number — the runtime records token counts, not a
//	               monetary cost, so any number would be invented.
//	tokens         required {input, output, reasoning, cache{read, write}} —
//	               sessions carries tokens_used_in / tokens_used_out only; it
//	               records no reasoning tokens and no cache read/write split.
//	time           required {created, updated} — sessions has created_at and
//	               heartbeat_at, no updated timestamp.
//	title          required — sessions.agent_name/goal are not a session title.
//	location       required {directory} — the runtime keeps no per-session
//	               workspace directory (the shim serves one workspace per
//	               instance, resolved from the request).
//
// Emitting an entry would therefore invent at least eight of the required
// twelve members, so the declared 200 answers the empty, non-null collection
// and an empty `cursor` object (both of its members are optional in the pinned
// schema) — exactly the truthfulness rule api_v2_lists.go records for the
// sibling v2 read-only operations. Serving real SessionV2Info rows is a design
// change with its own row, not a contract detail.
//
// ch:trace row=ROUTE-ADD-037 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.list test=internal/shim/opencode/api_session_routes_test.go witness=none:unattended-worker-session

// handleApiSessionList serves GET /api/session — upstream v2.session.list
// (ROUTE-ADD-037, declared responses 200 SessionsResponse, 400
// InvalidCursorError | InvalidRequestError, 401 UnauthorizedError). See the
// file doc comment for the arm-by-arm contract and for why the declared 200
// carries an empty data array.
//
// POST is the declared sibling operation v2.session.create (ROUTE-ADD-038): it
// reaches this handler because the exact path is registered, and answers the
// typed not_implemented envelope naming that operation rather than a lying
// success (SHIM-GAP-002). Every method the document does not declare on this
// path keeps the METHOD_NOT_ALLOWED envelope.
func (s *Server) handleApiSessionList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		writeNotImplemented(w, r, "v2.session.create",
			"the Consensus shim serves no session-create contract: it does not "+
				"own the opencode session lifecycle (v2.session.create is its own "+
				"board row)")
		return
	}
	if !apiV2RequireGet(w, r) {
		return
	}
	if !apiSessionListQueryValid(w, r) {
		return
	}

	// The runtime's real session store decides the two non-input arms: a store
	// this instance cannot read is not an empty session list, and this
	// operation declares no 5xx code, so the failure is reported through the
	// declared InvalidRequestError 400 (the handleQuestionList /
	// handleApiSkillList convention) instead of a fabricated 200 [].
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to list sessions: the shim has no session store")
		return
	}
	if _, err := s.db.Query(r.Context(), `SELECT id FROM sessions`); err != nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to list sessions: "+err.Error())
		return
	}

	// Readable store, no truthfully expressible SessionV2Info row (file doc
	// comment): the declared success arm is the empty, non-null collection.
	writeJSON(w, map[string]any{
		"data":   []any{},
		"cursor": map[string]any{},
	})
}

// apiSessionListQueryValid validates the declared query parameters of
// v2.session.list against the pinned schema and writes the declared 400 arm
// itself, reporting false when the request is malformed. Every check is on a
// parameter the document declares — an undeclared query parameter is not
// refused, because the operation's parameter list does not set
// additionalProperties:false the way its response schemas do.
func apiSessionListQueryValid(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()

	// workspace — declared string with pattern ^wrk.
	if values, present := q["workspace"]; present {
		workspace := strings.TrimSpace(apiSessionFirstValue(values))
		if workspace == "" {
			writeV2InvalidRequest(w, "Query", "workspace",
				`query parameter "workspace" must not be blank`)
			return false
		}
		if !strings.HasPrefix(workspace, "wrk") {
			writeV2InvalidRequest(w, "Query", "workspace",
				`query parameter "workspace" must match the declared pattern ^wrk`)
			return false
		}
	}

	// order — declared string with enum [asc, desc].
	if values, present := q["order"]; present {
		order := strings.TrimSpace(apiSessionFirstValue(values))
		if order != "asc" && order != "desc" {
			writeV2InvalidRequest(w, "Query", "order",
				`query parameter "order" must be one of "asc" or "desc" (the declared enum)`)
			return false
		}
	}

	// limit — declared type number.
	if values, present := q["limit"]; present {
		limit := strings.TrimSpace(apiSessionFirstValue(values))
		if limit == "" {
			writeV2InvalidRequest(w, "Query", "limit",
				`query parameter "limit" must not be blank`)
			return false
		}
		if _, err := strconv.ParseFloat(limit, 64); err != nil {
			writeV2InvalidRequest(w, "Query", "limit",
				`query parameter "limit" must be a number`)
			return false
		}
	}

	// directory / project / subpath — plain declared strings naming a thing;
	// a present-but-blank value is malformed input, not "no filter" (the
	// handleVCSStatus / apiV2LocationSelector blank-selector rule).
	for _, param := range []string{"directory", "project", "subpath"} {
		values, present := q[param]
		if !present {
			continue
		}
		if strings.TrimSpace(apiSessionFirstValue(values)) == "" {
			writeV2InvalidRequest(w, "Query", param,
				"query parameter \""+param+"\" must not be blank")
			return false
		}
	}

	// search is a free-form string: the document declares no pattern, no
	// enum and no minLength, so every value (an empty one included) is
	// well-formed and there is no arm to answer.

	// cursor — opaque, and only meaningful when this surface issued it. This
	// handler's 200 never carries cursor.previous/cursor.next, so no value a
	// client can send is one of ours; the declared InvalidCursorError is the
	// truthful arm rather than an empty list that ignores the cursor.
	if _, present := q["cursor"]; present {
		writeV2InvalidCursor(w,
			`query parameter "cursor" is not a cursor issued by this server: `+
				`the session list surface returns no cursor.previous or cursor.next`)
		return false
	}

	return true
}

// apiSessionFirstValue returns the first value of a query parameter, or "" when
// the parameter was sent with no value at all (?workspace=&... arrives as a
// one-element slice; a bare ?workspace can arrive as an empty slice).
func apiSessionFirstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// writeV2InvalidCursor writes the declared InvalidCursorError body
// ({_tag, message} — the pinned schema sets additionalProperties:false and
// declares exactly those two required members).
func writeV2InvalidCursor(w http.ResponseWriter, message string) {
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{
		"_tag":    "InvalidCursorError",
		"message": message,
	})
}
