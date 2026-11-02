package opencode

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ROUTE-ADD-074 — GET /experimental/session (experimental.session.list)
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/experimental/session".get) declares experimental.session.list with
// declared responses 200 GlobalSession[] ("List of sessions") | 400
// BadRequestError and the optional query parameters
//
//	directory  string
//	workspace  string
//	roots      boolean | string enum ["true", "false"]
//	start      number
//	cursor     number
//	search     string
//	limit      number
//	archived   boolean | string enum ["true", "false"]
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class (source item SHIM-DRIFT-014 in appendix-drift-declared.md),
// indistinguishable from "no such opencode operation".
//
// The route is registered as its own EXACT pattern (server.go, beside the
// sibling /experimental/* registrations); the /experimental/* MountPatterns
// entry already carries the subtree to a parent chi mount. It is never an
// /experimental/* catch-all, so the declared sibling
// /experimental/session/{sessionID}/background keeps the classification its
// own contract row is filed against, and deeper sub-paths keep the net/http
// default 404.
//
// Arms:
//
//	200 — the declared GlobalSession[] with the empty, non-null collection
//	      (see the reshape-refusal note below).
//	400 BadRequestError — a query parameter violating the declared schema
//	      (a present-but-blank directory/workspace selector, a roots/archived
//	      value that is neither boolean `true`/`false` nor the declared string
//	      enum form, a start/cursor/limit that is blank or not a number), and
//	      an unreadable sessions store (this operation declares no 5xx arm, so
//	      a store that cannot be read is reported through the declared
//	      BadRequestError — the handleApiSessionList no-5xx convention,
//	      answered through THIS operation's declared arm shape).
//	405 METHOD_NOT_ALLOWED — methods the pinned document does not declare on
//	      this path at all (POST, PUT, PATCH, DELETE) — sibling method-guard
//	      convention (handleExperimentalConsole).
//	401 UNAUTHENTICATED — the standard api-key wall (/experimental/* is not a
//	      v2 envelope path and the operation declares no 401 arm).
//
// Why the 200 answers an EMPTY array: the runtime's sessions store
// (migration 001, the table the v1 /session routes and the harness read) is
// real and IS read by this handler, but no row in it can be truthfully
// reshaped into the declared GlobalSession (additionalProperties: false):
//
//	id        required, pattern ^ses — sessions.id is a native UUID, and
//	          rewriting a native id into "ses<uuid>" would fabricate the very
//	          identifier a client would then use to address the session (the
//	          handleApiSessionList reshape-refusal precedent).
//	slug      required — the store holds no slug column and no slug-minting
//	          engine exists in this runtime.
//	projectID required — sessions.project_id names an internal project row,
//	          not the declared upstream projectID the surface addresses.
//	directory required — the runtime keeps no per-session project directory
//	          (the shim serves one workspace per instance, resolved from the
//	          request).
//	title     required — sessions.agent_name/goal are not a session title.
//	version   required — the store records no opencode version per session.
//	time      required {created, updated} — sessions has created_at and
//	          heartbeat_at, no updated timestamp.
//
// Emitting an entry would therefore invent most of the required members, so
// the declared 200 answers the empty, non-null collection — exactly the
// truthfulness rule the sibling v2 list operations record. Serving real
// GlobalSession rows is a design change with its own row, not a contract
// detail.
//
// ch:trace row=ROUTE-ADD-074 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.session.list test=internal/shim/opencode/api_experimental_session_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:unattended-worker-session

// handleExperimentalSession serves GET /experimental/session — upstream
// experimental.session.list (ROUTE-ADD-074, declared responses 200
// GlobalSession[], 400 BadRequestError). See the file doc comment for the
// arm-by-arm contract and for why the declared 200 carries an empty array.
func (s *Server) handleExperimentalSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	if !experimentalSessionQueryValid(w, r) {
		return
	}

	// The runtime's real session store decides the non-input arms: a store
	// this instance cannot read is not an empty session list, and this
	// operation declares no 5xx code, so the failure is reported through the
	// declared BadRequestError (the handleApiSessionList convention) instead
	// of a fabricated 200 [].
	if s.db == nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "BadRequest",
			"failed to list sessions: the shim has no session store")
		return
	}
	if _, err := s.db.Query(r.Context(), `SELECT id FROM sessions`); err != nil {
		writeOpencodeNamedError(w, r, http.StatusBadRequest, "BadRequest",
			"failed to list sessions: "+err.Error())
		return
	}

	// Readable store, no truthfully expressible GlobalSession row (file doc
	// comment): the declared success arm is the empty, non-null collection.
	writeJSON(w, []any{})
}

// experimentalSessionQueryValid validates the declared query parameters of
// experimental.session.list against the pinned schema and writes the declared
// 400 arm itself, reporting false when the request is malformed. Every check
// is on a parameter the document declares — an undeclared query parameter is
// not refused, because the operation's parameter list does not set
// additionalProperties:false the way its response schemas do.
func experimentalSessionQueryValid(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()

	// directory / workspace — declared plain strings naming a thing; a
	// present-but-blank value is malformed input, not "no filter" (the
	// handleExperimentalConsole blank-selector rule).
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return false
		}
	}

	// roots / archived — declared anyOf boolean | string enum ["true",
	// "false"]. A query value always arrives as text, so the well-formed
	// forms are exactly "true" and "false" (case-sensitive, the declared
	// enum); anything else — a blank, "1", "yes", "TRUE" — matches neither
	// declared arm.
	for _, param := range []string{"roots", "archived"} {
		values, present := q[param]
		if !present {
			continue
		}
		value := ""
		if len(values) > 0 {
			value = values[0]
		}
		if value != "true" && value != "false" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must be \"true\" or \"false\" (the declared boolean or its string-enum form)", param))
			return false
		}
	}

	// start / cursor / limit — declared type number. Unlike the v2 session
	// list (whose cursor is an opaque server-issued string), this cursor is a
	// plain declared number, so a numeric value is well-formed input and is
	// never refused; only a blank or non-numeric value is.
	for _, param := range []string{"start", "cursor", "limit"} {
		values, present := q[param]
		if !present {
			continue
		}
		value := ""
		if len(values) > 0 {
			value = strings.TrimSpace(values[0])
		}
		if value == "" {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return false
		}
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must be a number", param))
			return false
		}
	}

	// search is a free-form string: the document declares no pattern, no
	// enum and no minLength, so every value (an empty one included) is
	// well-formed and there is no arm to answer.

	return true
}
