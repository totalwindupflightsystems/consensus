package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ROUTE-ADD-105 / ROUTE-ADD-106 / ROUTE-ADD-107 / ROUTE-ADD-108 /
// ROUTE-ADD-109 — the declared /pty/{ptyID} operation family of the pinned
// upstream document (specs/openapi/upstream/openapi-1.18.33.json):
//
//	GET    /pty/{ptyID}              → pty.get          (ROUTE-ADD-106)
//	PUT    /pty/{ptyID}              → pty.update       (ROUTE-ADD-107)
//	DELETE /pty/{ptyID}              → pty.remove       (ROUTE-ADD-105)
//	GET    /pty/{ptyID}/connect      → pty.connect      (ROUTE-ADD-108)
//	POST   /pty/{ptyID}/connect-token → pty.connectToken (ROUTE-ADD-109)
//
// Before this change none of the three sub-paths had a shim route at all, so
// every one of them fell through the mux to net/http's default 404 — the
// declared-vs-served comparison's NOT-SERVED class (source items
// SHIM-DRIFT-091..095 in the regenerated artifact; the board rows cite
// SHIM-DRIFT-108..112 from the 2026-09-29 baseline report, and the ROUTE-ADD-108
// row additionally cited SHIM-DRIFT-104, which is /pty/{ptyID}/connect in that
// baseline — the regenerated artifact's positional ids are authoritative).
//
// TRUTHFULNESS — the decision rule this family follows (the sessionUnshare /
// sessionPartUpdate precedent, ROUTE-FIX-015 / ROUTE-FIX-037, and the
// ROUTE-FIX-031 / ROUTE-ADD-101 truthful-negative convention):
//
// The Consensus runtime keeps NO pty registry and NO pty engine. The shim
// spawns no pseudo-terminal (pty.list answers the truthful empty array,
// ROUTE-ADD-102; pty.create stays an explicit intentional narrowing,
// ROUTE-ADD-103), so no ptyID names a session-backed terminal and the four
// status-bearing operations answer the DECLARED 404 for every well-formed
// ptyID — never a fabricated success body:
//
//   - pty.get / pty.update declare a 200 Pty OBJECT the runtime cannot
//     produce: every candidate value would assert a terminal that does not
//     exist.
//   - pty.connectToken declares a 200 PtyTicketConnectToken — a short-lived
//     single-use credential. The shim mints no ticket, so a ticket value
//     would be a fabricated credential.
//   - pty.remove and pty.connect declare a 200 BOOLEAN. Unlike
//     session.deleteMessage (whose boolean can truthfully report false for a
//     message the session really holds), the boolean here is only meaningful
//     for a PTY that exists: there is no such terminal for any input, so
//     "session removed" / "connected session" cannot be reported truthfully
//     for any request, and the declared 404 PtyNotFoundError / NotFoundError
//     IS the answer (upstream itself declares that 404 for an unknown ptyID).
//
// The declared error vocabulary is therefore the whole served surface:
//
//	400  the operation declares a 400 arm AND the input is malformed —
//	     an ill-shaped ptyID (the declared path-parameter pattern is ^pty),
//	     or a present declared query selector (directory, workspace) that is
//	     blank or does not name an existing directory (the sibling ptyList /
//	     pty.shells convention), or (pty.update) a request body that is not
//	     an object / carries an undeclared key / violates the declared
//	     title/size schema.
//	403  the connect family's scope refusal (below).
//	404  every well-formed ptyID: no session-backed terminal exists.
//	405  never: it is not in any of the five declared response sets, so an
//	     undeclared method keeps the pre-change answer instead (below).
//
// 403 — the connect family's declared authorization arm. The upstream
// document scopes a 403 to exactly these two operations (no other declared
// operation in the pinned surface declares one): the PTY namespace is scoped
// to the instance's workspace, and a caller asking to open a terminal in a
// workspace this instance does not serve is refused. The shim serves exactly
// ONE workspace (its configured workdir, else the process CWD — the
// /instance/* and project.current singleton convention) and can never serve a
// terminal outside it, so a connect request naming a different workspace is
// answered inside the declared vocabulary instead of inventing an undeclared
// code:
//
//	pty.connect       — any directory/workspace selector that is blank, names
//	                    no existing directory, or names an existing directory
//	                    this instance does not serve answers the declared 403
//	                    Forbidden. This operation declares NO 400 arm, so a
//	                    selector it cannot honour is reported with the one
//	                    declared code that expresses a refusal.
//	pty.connectToken  — a blank or non-directory selector answers the declared
//	                    400 BadRequestError first (malformed input), and an
//	                    existing directory this instance does not serve answers
//	                    the declared 403 PtyForbiddenError.
//
// pty.get / pty.update / pty.remove declare no 403, so for them an existing
// foreign directory selector is not a refusal — the declared 404 (no such PTY
// in any workspace) is the answer. A blank or non-directory selector is still
// the declared 400.
//
// Undeclared methods keep the PRE-CHANGE answer. Before this change
// PATCH/POST /pty/{ptyID} and POST /pty/{ptyID}/connect fell through the mux
// to net/http's default 404; the handlers below reproduce exactly that
// (http.NotFound) for every method outside the operation's declared one,
// because 405 is not in any of the five declared response sets. The bare
// /pty mount (GET pty.list, POST the typed 501 pty.create narrowing) and
// /pty/shells keep their own handlers — ServeMux resolves the more specific
// pattern first, so registering the family below does not touch them.
//
// Auth is untouched: the /pty subtree is not a stub path and carries no
// fixed-workspace exemption, so the sub-routes keep the standard api-key
// policy exactly like the bare route.

// ptySelectorVerdict classifies the declared directory/workspace query
// selectors a request carries (upstream pty operations declare both as
// optional string selectors).
type ptySelectorVerdict int

const (
	// ptySelectorsServed: no selector was supplied, or every selector names
	// the workspace this instance serves.
	ptySelectorsServed ptySelectorVerdict = iota
	// ptySelectorMalformed: a selector is present but blank, or names a path
	// that is not an existing directory (the sibling ptyList/pty.shells rule).
	ptySelectorMalformed
	// ptySelectorForeignWorkspace: a selector names an existing directory this
	// instance does not serve.
	ptySelectorForeignWorkspace
)

// ptySelectorVerdict inspects the declared directory/workspace selectors.
// It returns the verdict plus the offending parameter and value so the caller
// can name them in the declared error body.
func (s *Server) ptySelectorVerdict(r *http.Request) (ptySelectorVerdict, string, string) {
	served := filepath.Clean(s.workspaceDir())
	for _, param := range []string{"directory", "workspace"} {
		values, present := r.URL.Query()[param]
		if !present {
			continue
		}
		if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			return ptySelectorMalformed, param, ""
		}
		value := strings.TrimSpace(values[0])
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			return ptySelectorMalformed, param, value
		}
		if filepath.Clean(value) != served {
			return ptySelectorForeignWorkspace, param, value
		}
	}
	return ptySelectorsServed, "", ""
}

// ptySelectorBadRequest answers the declared 400 arm for a malformed selector.
func ptySelectorBadRequest(w http.ResponseWriter, r *http.Request, verdict ptySelectorVerdict, param, value string) {
	if verdict == ptySelectorMalformed && value == "" {
		writeOpencodeBadRequest(w, r, "Query",
			fmt.Sprintf("query parameter %q must not be blank", param))
		return
	}
	writeOpencodeBadRequest(w, r, "Query",
		fmt.Sprintf("%s %q does not exist or is not a directory", param, value))
}

// ptyMethodNotRouted reproduces the answer an undeclared method received
// before this route family existed: net/http's default 404. 405 is not in any
// of the five declared response sets, so the pre-change answer is kept rather
// than replaced with a code the contract does not declare.
func ptyMethodNotRouted(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// ptyUnknownID answers the declared 404 for a ptyID that names no
// session-backed terminal. pty.get / pty.update / pty.remove / pty.connectToken
// declare the typed PtyNotFoundError envelope {_tag, ptyID, message};
// pty.connect declares the sibling NotFoundError NamedError shape
// {name, data:{message}} — each handler calls the matching writer.
func ptyUnknownID(ptyID string) string {
	return "PTY session not found: " + ptyID
}

// handlePtyByID serves /pty/{ptyID} (ROUTE-ADD-105/106/107): GET pty.get,
// PUT pty.update, DELETE pty.remove. The three declared operations share one
// path pattern, so the method selects the operation; every other method keeps
// the pre-change default 404 (see ptyMethodNotRouted).
//
//	ch:trace row=ROUTE-ADD-105 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.remove test=internal/shim/opencode/pty_routes_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//	ch:trace row=ROUTE-ADD-106 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.get test=internal/shim/opencode/pty_routes_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//	ch:trace row=ROUTE-ADD-107 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.update test=internal/shim/opencode/pty_routes_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
func (s *Server) handlePtyByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.ptyGet(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	case http.MethodPut:
		s.ptyUpdate(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	case http.MethodDelete:
		s.ptyRemove(w, r, strings.TrimSpace(r.PathValue("ptyID")))
	default:
		ptyMethodNotRouted(w, r)
	}
}

// ptyGet serves GET /pty/{ptyID} — upstream pty.get (ROUTE-ADD-106,
// SHIM-DRIFT-092; declared responses: 200 Pty, 400 BadRequestError,
// 404 PtyNotFoundError).
//
// Validation order: the declared path-parameter pattern ^pty (a violating
// value is the declared 400, the sessionPartUpdate ill-shaped-id convention),
// then the declared directory/workspace selectors (blank or non-directory →
// declared 400, the sibling ptyList rule), then the resource lookup. The
// runtime keeps no pty registry, so every well-formed ptyID is unknown and
// answers the declared 404 typed PtyNotFoundError — the declared 200 Pty is
// never fabricated.
func (s *Server) ptyGet(w http.ResponseWriter, r *http.Request, ptyID string) {
	if !strings.HasPrefix(ptyID, "pty") {
		writeOpencodeBadRequest(w, r, "Params",
			`path parameter "ptyID" must match the declared pattern ^pty`)
		return
	}
	if verdict, param, value := s.ptySelectorVerdict(r); verdict == ptySelectorMalformed {
		ptySelectorBadRequest(w, r, verdict, param, value)
		return
	}
	writeOpencodeNotFoundError(w, "PtyNotFoundError", "ptyID", ptyID, ptyUnknownID(ptyID))
}

// ptyUpdate serves PUT /pty/{ptyID} — upstream pty.update (ROUTE-ADD-107,
// SHIM-DRIFT-093; declared responses: 200 Pty, 400 BadRequest |
// InvalidRequestError, 404 PtyNotFoundError).
//
// The declared requestBody is the object {title?: string, size?: {rows*:
// integer > 0, cols*: integer > 0}} with additionalProperties: false. It is
// validated strictly and BEFORE the resource lookup: a non-object or
// unparseable body, an undeclared key, a title that is not a string, a size
// that is not an object, is missing rows/cols, carries an undeclared key, or
// is not > 0 answers the declared 400 (the anyOf admits the BadRequestError
// shape writeOpencodeBadRequest emits). The document marks no required member
// (the body itself is optional and every declared field is optional), so an
// absent or empty body is well-formed input and proceeds to the lookup — the
// sessionPartUpdate absent-body rule.
//
// No pty exists, so the update cannot be performed for any ptyID: the
// declared 404 typed PtyNotFoundError is the answer, never an echoed 200 Pty
// claiming the requested title/size was applied.
func (s *Server) ptyUpdate(w http.ResponseWriter, r *http.Request, ptyID string) {
	if !strings.HasPrefix(ptyID, "pty") {
		writeOpencodeBadRequest(w, r, "Params",
			`path parameter "ptyID" must match the declared pattern ^pty`)
		return
	}
	if verdict, param, value := s.ptySelectorVerdict(r); verdict == ptySelectorMalformed {
		ptySelectorBadRequest(w, r, verdict, param, value)
		return
	}
	if !s.ptyValidateUpdateBody(w, r) {
		return
	}
	writeOpencodeNotFoundError(w, "PtyNotFoundError", "ptyID", ptyID, ptyUnknownID(ptyID))
}

// ptyValidateUpdateBody enforces the declared pty.update requestBody schema.
// An absent/empty body is accepted (no declared member is required); a
// supplied body must be an object carrying only the declared fields with the
// declared value shapes.
func (s *Server) ptyValidateUpdateBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	var body struct {
		Title json.RawMessage `json:"title"`
		Size  json.RawMessage `json:"size"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeOpencodeBadRequest(w, r, "Body",
			"request body must be a JSON object carrying only the declared fields (title, size)")
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeBadRequest(w, r, "Body",
			"request body must contain exactly one JSON object")
		return false
	}
	if len(body.Title) > 0 {
		var title string
		if err := json.Unmarshal(body.Title, &title); err != nil {
			writeOpencodeBadRequest(w, r, "Payload",
				`field "title" must be a string`)
			return false
		}
	}
	if len(body.Size) > 0 {
		var size struct {
			Rows *int `json:"rows"`
			Cols *int `json:"cols"`
		}
		sizeDec := json.NewDecoder(bytes.NewReader(body.Size))
		sizeDec.DisallowUnknownFields()
		if err := sizeDec.Decode(&size); err != nil {
			writeOpencodeBadRequest(w, r, "Payload",
				`field "size" must be an object carrying only the declared fields (rows, cols)`)
			return false
		}
		if size.Rows == nil || size.Cols == nil {
			writeOpencodeBadRequest(w, r, "Payload",
				`field "size" requires both "rows" and "cols"`)
			return false
		}
		if *size.Rows <= 0 || *size.Cols <= 0 {
			writeOpencodeBadRequest(w, r, "Payload",
				`fields "size.rows" and "size.cols" must be greater than 0`)
			return false
		}
	}
	return true
}

// ptyRemove serves DELETE /pty/{ptyID} — upstream pty.remove (ROUTE-ADD-105,
// SHIM-DRIFT-091; declared responses: 200 boolean "Session removed",
// 400 BadRequestError, 404 PtyNotFoundError).
//
// The operation declares no requestBody, so any body is neither read nor
// required. Validation is the shared ptyID/selector preamble, then the
// declared 404: no pty this runtime could have created exists to remove, so
// the boolean 200 would have to report a removal that never happened — the
// declared PtyNotFoundError is the answer instead (the sessionUnshare
// no-active-share precedent).
func (s *Server) ptyRemove(w http.ResponseWriter, r *http.Request, ptyID string) {
	if !strings.HasPrefix(ptyID, "pty") {
		writeOpencodeBadRequest(w, r, "Params",
			`path parameter "ptyID" must match the declared pattern ^pty`)
		return
	}
	if verdict, param, value := s.ptySelectorVerdict(r); verdict == ptySelectorMalformed {
		ptySelectorBadRequest(w, r, verdict, param, value)
		return
	}
	writeOpencodeNotFoundError(w, "PtyNotFoundError", "ptyID", ptyID, ptyUnknownID(ptyID))
}

// handlePtyConnect serves GET /pty/{ptyID}/connect — upstream pty.connect
// (ROUTE-ADD-108, SHIM-DRIFT-094; declared responses: 200 boolean "Connected
// session", 403 Forbidden, 404 NotFoundError).
//
//	ch:trace row=ROUTE-ADD-108 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.connect test=internal/shim/opencode/pty_routes_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//
// This operation declares NO 400 arm, so an ill-shaped ptyID is never answered
// 400 — an id that cannot name a PTY is the declared 404. The declared 403 is
// the scope refusal documented on the family: any directory/workspace selector
// that is blank, names no existing directory, or names an existing directory
// this instance does not serve. The declared 200 boolean ("Connected session")
// is never fabricated: the shim opens no PTY WebSocket, so a well-formed
// ptyID answers the declared 404 NotFoundError NamedError shape.
func (s *Server) handlePtyConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ptyMethodNotRouted(w, r)
		return
	}
	if verdict, param, value := s.ptySelectorVerdict(r); verdict != ptySelectorsServed {
		ptyConnectForbidden(w, r, verdict, param, value)
		return
	}
	ptyID := strings.TrimSpace(r.PathValue("ptyID"))
	writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", ptyUnknownID(ptyID))
}

// ptyConnectForbidden answers pty.connect's declared 403 arm. Its schema is
// {_tag: "Forbidden"} with additionalProperties: false, so the body carries
// exactly that one field — the reason is logged, never added to the body.
func ptyConnectForbidden(w http.ResponseWriter, r *http.Request, verdict ptySelectorVerdict, param, value string) {
	writeJSONStatus(w, http.StatusForbidden, map[string]string{"_tag": "Forbidden"})
	slog.Warn("opencode-shim: pty connect refused outside the served workspace",
		"method", r.Method, "path", r.URL.Path, "param", param, "value", value,
		"verdict", ptySelectorVerdictName(verdict))
}

// handlePtyConnectToken serves POST /pty/{ptyID}/connect-token — upstream
// pty.connectToken (ROUTE-ADD-109, SHIM-DRIFT-095; declared responses: 200
// PtyTicketConnectToken {ticket, expires_in}, 400 BadRequestError, 403
// PtyForbiddenError, 404 PtyNotFoundError).
//
//	ch:trace row=ROUTE-ADD-109 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.connectToken test=internal/shim/opencode/pty_routes_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:self-verified-in-worktree
//
// Validation order: the declared ^pty path pattern (400, kind Params), the
// declared selectors (blank or non-directory → 400 kind Query; an existing
// directory this instance does not serve → the declared 403
// PtyForbiddenError), then the lookup. The declared 200 is a one-time
// credential the shim never mints, so it is never fabricated: a well-formed
// ptyID answers the declared 404 typed PtyNotFoundError. No requestBody is
// declared, so no body is read.
func (s *Server) handlePtyConnectToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ptyMethodNotRouted(w, r)
		return
	}
	ptyID := strings.TrimSpace(r.PathValue("ptyID"))
	if !strings.HasPrefix(ptyID, "pty") {
		writeOpencodeBadRequest(w, r, "Params",
			`path parameter "ptyID" must match the declared pattern ^pty`)
		return
	}
	switch verdict, param, value := s.ptySelectorVerdict(r); verdict {
	case ptySelectorMalformed:
		ptySelectorBadRequest(w, r, verdict, param, value)
		return
	case ptySelectorForeignWorkspace:
		writeJSONStatus(w, http.StatusForbidden, map[string]string{
			"_tag": "PtyForbiddenError",
			"message": fmt.Sprintf(
				"%s %q is not the workspace this instance serves; no PTY connect ticket can be minted for it",
				param, value),
		})
		slog.Warn("opencode-shim: pty connect-token refused outside the served workspace",
			"method", r.Method, "path", r.URL.Path, "param", param, "value", value)
		return
	}
	writeOpencodeNotFoundError(w, "PtyNotFoundError", "ptyID", ptyID, ptyUnknownID(ptyID))
}

// ptySelectorVerdictName names a verdict for log lines only.
func ptySelectorVerdictName(v ptySelectorVerdict) string {
	switch v {
	case ptySelectorsServed:
		return "served-workspace"
	case ptySelectorMalformed:
		return "malformed-selector"
	case ptySelectorForeignWorkspace:
		return "foreign-workspace"
	}
	return "unknown"
}
