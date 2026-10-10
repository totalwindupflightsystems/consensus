package opencode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// P1 session sub-routes (ROUTE-FIX-014..018)
//
// Five upstream session operations that were routed through
// handleSessionByID but answered the stub-list 501 instead of their declared
// contracts. Each handler below serves the declared response vocabulary
// truthfully: the Consensus runtime keeps no session-publishing, no
// shell-execution, no revert engine and no summarize pipeline, so no handler
// ever asserts an effect that was not performed (the handleSyncStart /
// handleGlobalUpgrade truthfulness convention, commit 13189b1).
// ============================================================================

// sessionBusyError writes the declared 409 SessionBusyError body — the typed
// upstream shape {"_tag":"SessionBusyError","sessionID":...,"message":...}.
// It is answered only for the analogous Consensus condition that genuinely
// exists: the sessions row is mid-turn (status planning/thinking/tool_exec/
// executing/waiting_sub — internal/session.Status.IsActive), the same state
// the sibling GET /session/status maps to {"type":"busy"}.
func (s *Server) sessionBusyError(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	if data, err := json.Marshal(map[string]any{
		"_tag":      "SessionBusyError",
		"sessionID": sessionID,
		"message":   "session is mid-turn; the operation requires an idle session",
	}); err == nil {
		_, _ = w.Write(data)
	}
}

// sessionIsActive reports whether the sessions row is mid-turn — the runtime's
// real analog of the upstream "prompt in flight" condition. Unknown or
// unreadable status answers false (the 409 arm must never be invented).
func sessionIsActive(status string) bool {
	switch status {
	case "planning", "thinking", "tool_exec", "executing", "waiting_sub":
		return true
	}
	return false
}

// p1ResolveSession looks up the sessions row (id, status) and answers the
// declared 404 NotFoundError itself when the id is unknown; ok is false in
// that case and the caller must return.
func (s *Server) p1ResolveSession(w http.ResponseWriter, r *http.Request, sessionID string) (status string, ok bool) {
	rows, err := s.db.QueryRow(r.Context(),
		`SELECT id, status FROM sessions WHERE id = $1`, sessionID)
	if err != nil || rows == nil {
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
		return "", false
	}
	return toString(rows["status"]), true
}

// p1DecodeBody decodes the declared JSON object body. Per the sibling
// POST-with-body convention (sessionFork/handleGlobalUpgrade): an absent body
// is a client contract violation when the operation declares required fields
// (the 400 message says so), and a body that is present but not a JSON object
// is malformed. The returned reader-position error is surfaced as malformed.
func p1DecodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"request body required: the declared schema carries required fields")
		return false
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"request body required: the declared schema carries required fields")
			return false
		}
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return false
	}
	return true
}

// ============================================================================
// ROUTE-FIX-014 — POST /session/{sessionID}/revert (session.revert)
// ============================================================================

// sessionRevert serves POST /session/{sessionID}/revert — upstream
// session.revert (ROUTE-FIX-014, SHIM-DRIFT-120 board row; declared responses:
// 200 Session, 400 BadRequest | InvalidRequestError, 404 NotFoundError, 409
// SessionBusyError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths." /session/{sessionID}/revert".post): the requestBody is the object
// {messageID*: string pattern ^msg, partID?: string pattern ^prt} with
// additionalProperties: false — messageID is REQUIRED. The declared 200 body
// is the (updated) Session object.
//
// Truthfulness: the Consensus runtime keeps no revert engine — nothing is
// reverted, no turn is dispatched, no state is rewritten — so the declared 200
// is answered with the session AS IT IS (translateSessionRow over the row
// unchanged) rather than a claimed reverted state, and the boolean `false` a
// no-op fork-paraphrase would return is not part of this operation's declared
// 200 schema at all. Validation order mirrors sessionFork: malformed body ->
// 400, missing/ill-shaped messageID -> 400 (the declared pattern is ^msg),
// unknown session -> 404, a well-formed messageID the session does not hold ->
// 404 via sessionHasMessage.
//
// The declared 409 SessionBusyError is answered ONLY for the analogous
// Consensus condition that genuinely exists in the sessions table: the row is
// mid-turn (status planning/thinking/tool_exec/executing/waiting_sub — the
// same states internal/session.Status.IsActive calls active, and the same
// states sibling GET /session/status maps to {"type":"busy"}). A revert while
// a turn is in flight is exactly the race upstream's SessionBusyError
// describes, and a busy session has no consistent revert point, so the
// declared refusal is truthful here. Idle/booting/completed/failed/paused
// sessions answer the declared 200.
func (s *Server) sessionRevert(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		MessageID string `json:"messageID"`
		PartID    string `json:"partID"`
	}
	if !p1DecodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.MessageID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required field "messageID" is missing`)
		return
	}
	if !strings.HasPrefix(req.MessageID, "msg") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"messageID must match the declared pattern ^msg")
		return
	}

	status, ok := s.p1ResolveSession(w, r, sessionID)
	if !ok {
		return
	}

	// The declared 409 analog: a session mid-turn has no consistent revert
	// point. Only the runtime's real in-flight states answer it (see
	// sessionIsActive) — never invented for idle rows.
	if sessionIsActive(status) {
		s.sessionBusyError(w, r, sessionID)
		return
	}

	// A well-formed messageID the session does not hold is the declared 404.
	if !s.sessionHasMessage(r.Context(), sessionID, req.MessageID) {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"revert point message not found in session")
		return
	}

	// No revert engine exists: nothing is reverted. The declared 200 body is
	// the Session — answered as the row stands, never claiming a reverted
	// state.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id, parent_id, agent_name, model_id, status, goal, context_budget,
		        tokens_used_in, tokens_used_out, iteration, project_id, heartbeat_at, created_at, completed_at
		 FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
		return
	}
	writeJSON(w, s.translateSessionRow(row))
}

// ============================================================================
// ROUTE-FIX-015 — DELETE /session/{sessionID}/share (session.unshare)
// ============================================================================

// sessionUnshare serves DELETE /session/{sessionID}/share — upstream
// session.unshare (ROUTE-FIX-015, SHIM-DRIFT-121 board row; declared
// responses: 200 Session, 400 Bad request, 404 NotFoundError, 500
// InternalServerError).
//
// Truthfulness: the Consensus runtime keeps no share concept at all — there is
// no shares table and no session-publishing pipeline (export is native,
// SPEC-015) — so every known session truthfully has no active share, and the
// declared 404 NotFoundError IS the answer: there is nothing to unshare. The
// declared 200 Session would assert that a share was removed that never
// existed, and 500 would assert a runtime error that did not happen; neither
// is answered. The operation declares no requestBody (null), so a body is
// neither read nor required. Order: unknown session 404 first, then the
// no-active-share 404 for every known session.
func (s *Server) sessionUnshare(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}
	// Known session, but the runtime keeps no share concept: no active share
	// exists to remove, which is the declared 404 (NotFoundError).
	writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
		"session has no active share to remove")
}

// ============================================================================
// ROUTE-FIX-016 — POST /session/{sessionID}/share (session.share)
// ============================================================================

// sessionShare serves POST /session/{sessionID}/share — upstream session.share
// (ROUTE-FIX-016, SHIM-DRIFT-122 board row; declared responses: 200 Session
// carrying share {url}, 400 Bad request, 404 NotFoundError, 500
// InternalServerError).
//
// Truthfulness: upstream publishes the session and returns it with a share
// URL. Consensus publishes nothing (export is native, SPEC-015) and a
// fabricated URL is forbidden (the handleSyncReplay precedent: answer the
// declared shape with the one value the runtime can honestly report, never a
// fabricated identifier) — so the declared 200 cannot be answered truthfully
// for any session. The refusal is answered inside the DECLARED error
// vocabulary: 400 Bad request via the sibling writeOpencodeError
// INVALID_REQUEST envelope naming the limitation (the sibling sessionCommand /
// handleGlobalUpgrade convention), never an undeclared 501 and never 500
// unless the runtime actually errored. Unknown session answers the declared
// 404 first. The operation declares no requestBody (null), so a body is
// neither read nor required.
func (s *Server) sessionShare(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}
	writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
		"session sharing is an opencode feature; Consensus does not publish sessions (export is native, SPEC-015), so no share URL can be created")
}

// ============================================================================
// ROUTE-FIX-017 — POST /session/{sessionID}/shell (session.shell)
// ============================================================================

// sessionShell serves POST /session/{sessionID}/shell — upstream session.shell
// (ROUTE-FIX-017, SHIM-DRIFT-123 board row; declared responses: 200 created
// message {info, parts}, 400 BadRequest | InvalidRequestError, 404
// NotFoundError, 409 SessionBusyError).
//
// Upstream contract: the requestBody is {agent*: string, command*: string,
// messageID?: ^msg, model?: {providerID*, modelID*}} with
// additionalProperties: false; the declared 200 body is the created message
// {info: Message, parts: Part[]}.
//
// Truthfulness: the Consensus runtime keeps no shell-execution engine — no
// command is run and no assistant message is produced — so the declared
// "Created message" body is never fabricated. Validation order mirrors
// sessionCommand: malformed body -> 400, missing required agent/command ->
// 400, unknown session -> 404, then the same declared 409 SessionBusyError
// analog sessionRevert answers (a shell command must not be attributed to a
// session whose turn is already in flight), and finally the declared 400
// naming the missing execution engine (the sessionCommand convention of
// answering inside the declared contract instead of the stub 501).
func (s *Server) sessionShell(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		Agent   string `json:"agent"`
		Command string `json:"command"`
	}
	if !p1DecodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Agent) == "" || strings.TrimSpace(req.Command) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required fields "agent" and "command" must not be blank`)
		return
	}

	status, ok := s.p1ResolveSession(w, r, sessionID)
	if !ok {
		return
	}

	// The declared 409 analog: a session mid-turn cannot take a shell
	// command without interleaving two in-flight turns.
	if sessionIsActive(status) {
		s.sessionBusyError(w, r, sessionID)
		return
	}

	// No shell-execution engine exists: no command was run and no message
	// was created. The declared 400 names the limitation — never a
	// fabricated created-message 200.
	writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
		"shell command execution is not available in this runtime: no shell-execution engine is wired to the session")
}

// ============================================================================
// ROUTE-FIX-018 — POST /session/{sessionID}/summarize (session.summarize)
// ============================================================================

// sessionSummarize serves POST /session/{sessionID}/summarize — upstream
// session.summarize (ROUTE-FIX-018, SHIM-DRIFT-124 board row; declared
// responses: 200 boolean ("Summarized session"), 400 BadRequest |
// InvalidRequestError, 404 NotFoundError).
//
// Upstream contract: the requestBody is {providerID*: string, modelID*:
// string, auto?: boolean} with additionalProperties: false; the declared 200
// body is a plain boolean.
//
// Truthfulness: the runtime keeps no summarize/compaction pipeline, so no
// summary is produced and the declared boolean is answered with the truthful
// false ("no summary produced") — the sibling handleSyncStart truthfulness
// convention: never assert an effect that was not performed. Validation order:
// malformed body -> 400, missing required providerID/modelID -> 400, unknown
// session -> 404.
func (s *Server) sessionSummarize(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req struct {
		ProviderID string `json:"providerID"`
		ModelID    string `json:"modelID"`
	}
	if !p1DecodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ProviderID) == "" || strings.TrimSpace(req.ModelID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required fields "providerID" and "modelID" must not be blank`)
		return
	}

	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}

	// No summarize pipeline exists: no summary was produced. The declared
	// boolean reports the truthful false — never a fabricated true.
	writeJSON(w, false)
}

// ============================================================================
// ROUTE-FIX-035 — DELETE /session/{sessionID}/message/{messageID}
// (session.deleteMessage)
// ============================================================================

// sessionDeleteMessage serves DELETE /session/{sessionID}/message/{messageID}
// — upstream session.deleteMessage (ROUTE-FIX-035, SHIM-NARROWED-005 board
// row; declared responses: 200 boolean ("Successfully deleted message"), 400
// BadRequest | InvalidRequestError, 404 NotFoundError, 409 SessionBusyError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/message/{messageID}".delete): sessionID and
// messageID are path parameters (patterns ^ses and ^msg); the operation
// declares no requestBody (null); the declared 200 body is a plain boolean
// described as "Successfully deleted message" — a permanent deletion of the
// message and all of its parts.
//
// Truthfulness: a message IS a memory_events row (opencode message ids are
// "msg-<memory_events.id>"; GET /session/{id}/message/{messageID},
// sessionHasMessage and this handler all resolve that same row), and that
// ledger is APPEND-ONLY by construction — SPEC-002 §2.1, enforced by the
// triggers in migrations 017 (SQLite) and 018 (Postgres), which ABORT every
// UPDATE and DELETE. The runtime keeps no delete engine, no message tombstone
// and no separate parts store (parts are synthesized from
// memory_events.content), and the shim never deletes memory_events anywhere
// (no "DELETE FROM memory_events" over internal/shim; the cognitive-firewall
// classifier treats such a statement as a poisoning attempt), so there is no
// genuine delete analog to perform. The one message-level state transition the
// runtime does keep — display_modes mode='hidden' (how the harness consumes
// user turns) — is not an analog either: the shim's own GET
// /session/{id}/message/{messageID} reads memory_events directly and does not
// honour display_modes, so a hidden message would remain observable and the
// "deletion" would be a no-op.
//
// Per the sibling sessionSummarize precedent (ROUTE-FIX-018, d8f1d59) — the
// other operation whose declared 200 body is a plain boolean and whose engine
// the runtime does not keep — the declared boolean is answered with the
// truthful false ("no message was deleted"): the truthfulness convention
// (handleSyncStart / handleGlobalUpgrade, 13189b1) forbids asserting an effect
// that was not performed, and 200-false is inside the declared contract, so
// the declared success code IS reachable. Validation order mirrors
// sessionRevert/sessionShell: unknown session -> 404; a session mid-turn
// (sessionIsActive — the genuine analog of upstream's "prompt in flight") ->
// the declared 409 SessionBusyError, checked before message resolution; an
// absent/blank or ill-shaped messageID (the declared pattern is ^msg) -> the
// declared 400 (this is what makes the declared 400 arm reachable, since the
// operation declares no requestBody); a well-formed messageID the session does
// not hold -> 404 via sessionHasMessage. The operation declares no
// requestBody, so no body is read or required.
func (s *Server) sessionDeleteMessage(w http.ResponseWriter, r *http.Request, sessionID, messageID string) {
	status, ok := s.p1ResolveSession(w, r, sessionID)
	if !ok {
		return
	}

	// The declared 409 analog: a session mid-turn has a prompt in flight,
	// which is exactly the condition upstream's SessionBusyError describes.
	if sessionIsActive(status) {
		s.sessionBusyError(w, r, sessionID)
		return
	}

	// messageID is required and the declared pattern is ^msg (the
	// sessionRevert validation precedent).
	if strings.TrimSpace(messageID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required path parameter "messageID" is missing`)
		return
	}
	if !strings.HasPrefix(messageID, "msg") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"messageID must match the declared pattern ^msg")
		return
	}

	// A well-formed messageID the session does not hold is the declared 404.
	if !s.sessionHasMessage(r.Context(), sessionID, messageID) {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"message not found in session")
		return
	}

	// No delete engine exists: messages live in the append-only memory_events
	// ledger (SPEC-002 §2.1) and no tombstone/parts store exists, so nothing
	// was deleted. The declared boolean reports the truthful false — never a
	// fabricated true.
	writeJSON(w, false)
}

// ============================================================================
// ROUTE-FIX-040 — POST /session/{sessionID}/unrevert (session.unrevert)
// ============================================================================
//
// sessionUnrevert serves POST /session/{sessionID}/unrevert — upstream
// session.unrevert (ROUTE-FIX-040, board row source item SHIM-NARROWED-010 in
// the 2026-09-29 baseline report; declared responses: 200 Session "Updated
// session", 400 BadRequest | InvalidRequestError, 404 NotFoundError, 409
// SessionBusyError).
//
// Upstream contract: the operation declares NO requestBody (parameters are
// sessionID plus the optional directory/workspace query parameters), so a body
// is neither read nor required. The declared 200 body is the (updated) Session
// object. Handler shape mirrors sessionRevert (ROUTE-FIX-014, commit d8f1d59):
// unknown session -> declared 404; a session mid-turn (sessionIsActive — the
// runtime's real in-flight state) -> the declared 409 SessionBusyError
// (restoring messages while a turn is in flight is exactly the race upstream's
// SessionBusyError describes); otherwise the declared 200 with the session AS
// IT IS.
//
// Truthfulness: sessionRevert is already a truthful no-op in this runtime (no
// revert engine exists — nothing is ever moved out of the message list, so
// there is nothing to restore), and unrevert inherits that: no turn is
// dispatched and no state is rewritten, so the declared 200 is answered with
// translateSessionRow over the row unchanged, never a claimed restored state.
func (s *Server) sessionUnrevert(w http.ResponseWriter, r *http.Request, sessionID string) {
	status, ok := s.p1ResolveSession(w, r, sessionID)
	if !ok {
		return
	}

	// The declared 409 analog: restoring messages into a session mid-turn
	// would interleave with the in-flight prompt (the sessionRevert
	// precedent — same declared SessionBusyError vocabulary).
	if sessionIsActive(status) {
		s.sessionBusyError(w, r, sessionID)
		return
	}

	// No revert engine exists: nothing was ever reverted, so nothing is
	// restored. The declared 200 body is the Session — answered as the row
	// stands, never claiming a restored state.
	row, err := s.db.QueryRow(r.Context(),
		`SELECT id, parent_id, agent_name, model_id, status, goal, context_budget,
		        tokens_used_in, tokens_used_out, iteration, project_id, heartbeat_at, created_at, completed_at
		 FROM sessions WHERE id = $1`, sessionID)
	if err != nil || row == nil {
		writeOpencodeNamedError(w, r, http.StatusNotFound, "NotFoundError", "Session not found")
		return
	}
	writeJSON(w, s.translateSessionRow(row))
}

// ============================================================================
// ROUTE-FIX-038 — POST /session/{sessionID}/permissions/{permissionID}
// (permission.respond)
// ============================================================================

// sessionPermissionRespond serves POST
// /session/{sessionID}/permissions/{permissionID} — upstream permission.respond
// (ROUTE-FIX-038, board row source item SHIM-NARROWED-008 in the 2026-09-29
// baseline report; declared responses: 200 boolean "Permission processed
// successfully", 400 BadRequest | InvalidRequestError, 404 NotFoundError |
// PermissionNotFoundError; the operation is marked deprecated upstream but the
// shim serves it as declared).
//
// Upstream contract: permissionID is a path parameter with declared pattern
// ^per; the requestBody is {response*: "once" | "always" | "reject"} with
// additionalProperties: false — response is REQUIRED; the declared 200 body is
// a plain boolean. The operation is the session-scoped twin of the consent
// sidecar's POST /permission/{id}/resolve (ROUTE-FIX-045, commit 07f2f3c), so
// the field mapping mirrors that handler onto the REAL approval_requests
// columns: response "reject" -> status 'rejected'; "once"/"always" -> status
// 'approved' (both grant the pending action — the scope recorded in
// review_notes, 'granted once' vs 'granted (always)'); review_notes carries the
// resolution note, reviewed_at the timestamp and reviewer_id 'opencode-shim' —
// there is no decision_reason column (migrations/008_hitl_tables.sql, the same
// mapping getPermission/resolvePermission already use).
//
// Validation order mirrors the sibling P1 handlers: malformed body -> 400,
// missing/invalid response value -> 400, unknown session -> 404, unknown or
// already-resolved permission -> 404 (the resolvePermission convention: the
// contract declares only 200/400/404 here, so a non-pending row answers the
// declared 404, never an undeclared 409). A resolved permission emits the same
// SSE approval_resolved event the sidecar resolve handler emits
// (HARDEN-SHIM-01). The declared 200 boolean reports true ONLY when the row was
// actually resolved (the truthful-delete precedent: never a fabricated
// success), which is every request that passes the guards above.
func (s *Server) sessionPermissionRespond(w http.ResponseWriter, r *http.Request, sessionID, permissionID string) {
	var req struct {
		Response string `json:"response"`
	}
	if !p1DecodeBody(w, r, &req) {
		return
	}
	switch req.Response {
	case "once", "always", "reject":
		// declared enum values
	default:
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required field "response" must be one of "once", "always", "reject"`)
		return
	}
	if !strings.HasPrefix(permissionID, "per") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"permissionID must match the declared pattern ^per")
		return
	}

	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}

	ctx := r.Context()
	// Existence + pending guard up front (the resolvePermission convention —
	// the db wrapper exposes no RowsAffected).
	row, err := s.db.QueryRow(ctx,
		`SELECT session_id, status FROM approval_requests WHERE id = $1`, permissionID)
	if err != nil || row == nil {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "permission not found")
		return
	}
	if rowSession := toString(row["session_id"]); rowSession != "" && rowSession != sessionID {
		// The permission exists but belongs to another session: from this
		// session-scoped path it is indistinguishable from a missing one.
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"permission not found in session")
		return
	}
	if toString(row["status"]) != "pending" {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"permission is not pending")
		return
	}

	// Map the declared enum onto the REAL approval_requests columns (the
	// resolvePermission / ROUTE-FIX-045 mapping — no decision_reason column
	// exists): reject -> 'rejected', once/always -> 'approved' with the
	// grant scope in review_notes.
	decision := "approved"
	note := "granted (always)"
	switch req.Response {
	case "reject":
		decision = "rejected"
		note = "denied by permission.respond"
	case "once":
		note = "granted once"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.Exec(ctx,
		`UPDATE approval_requests
		 SET status = $1, review_notes = $2, reviewed_at = $3, reviewer_id = 'opencode-shim'
		 WHERE id = $4 AND status = 'pending'`,
		decision, note, now, permissionID,
	); err != nil {
		writeOpencodeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
			"failed to resolve permission: "+err.Error())
		return
	}

	// Emit the same SSE event the sidecar resolve handler emits
	// (HARDEN-SHIM-01: permission.resolved for SSE subscribers).
	s.emitShimEventForSession(sessionID, "approval_resolved", map[string]any{
		"approval_id": permissionID,
		"status":      decision,
	})

	// The permission WAS resolved: the declared boolean reports the truthful
	// true — never a no-op success.
	writeJSON(w, true)
}

// ============================================================================
// ROUTE-FIX-037 — PATCH /session/{sessionID}/message/{messageID}/part/{partID}
// (part.update)
//
// ch:trace row=ROUTE-FIX-037 spec=specs/openapi/upstream/openapi-1.18.33.json#part.update wave=consensus-foreman-2026-10-03-00-46-49.json#task-1 test=TestSessionPartUpdateTruthfulArms doc=docs/evidence/ROUTE-FIX-037-live-probe.md evidence=docs/evidence/ROUTE-FIX-037-live-probe.md witness=none:self-verified-in-worktree
// ============================================================================

// parseMessagePartSub splits the handleSessionByID sub-path
// "message/{messageID}/part/{partID}" into its two path parameters. It splits
// on the first "/part/" separator, so a messageID half that happens to be
// empty (or a trailing "/part/" with no partID) still yields both halves and
// the handler answers the declared 400 for the missing parameter; a sub-path
// that is not of this shape at all (impossible for the router case that calls
// this — it requires a "/part/" segment) yields an empty partID and takes the
// same declared 400 arm rather than an undeclared code.
func parseMessagePartSub(sub string) (messageID, partID string) {
	rest := strings.TrimPrefix(sub, "message/")
	halves := strings.SplitN(rest, "/part/", 2)
	if len(halves) != 2 {
		return rest, ""
	}
	return halves[0], halves[1]
}

// sessionPartUpdate serves PATCH
// /session/{sessionID}/message/{messageID}/part/{partID} — upstream part.update
// (ROUTE-FIX-037; the board row's SOURCE ITEM SHIM-NARROWED-007 in
// docs/reports/shim-source-002-baseline-diff-2026-09-29.md, and
// SHIM-NARROWED-006 in the positional ids of
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json, which
// renumbered after ROUTE-FIX-035; declared responses: 200 Part "Successfully
// updated part", 400 BadRequest | InvalidRequestError, 404 NotFoundError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/message/{messageID}/part/{partID}".patch):
// sessionID, messageID and partID are path parameters with declared patterns
// ^ses, ^msg and ^prt; the requestBody is a Part (anyOf of the twelve part
// variants — TextPart, FilePart, ToolPart, ...) and the declared 200 body is
// the (updated) Part.
//
// Truthfulness — why the declared 200 is NOT served. The Consensus runtime
// keeps no message-part editing engine and no part store:
//
//   - A message IS a memory_events row (opencode "msg-<id>" ids resolve to
//     that ledger; config, session_*) and that ledger is APPEND-ONLY by
//     construction — SPEC-002 §2.1, enforced by the triggers in migrations 017
//     (SQLite) and 018 (Postgres), which ABORT every UPDATE and DELETE. The
//     shim never writes memory_events in place anywhere.
//   - There is no parts table in any migration (migrations/ has sessions,
//     memory_events, display_modes, tool_requests, ... and no part store), so
//     parts are SYNTHESIZED on read: GET /session/{id}/message/{messageID}
//     returns exactly one part, {"type":"text","text":<memory_events.content>},
//     which carries NO id field at all (getMessageByID). No "prt-..." id is
//     ever issued by this runtime, so no part identified by partID exists to
//     update, and no part-editing engine is wired to the session.
//   - A truthful 200 is impossible inside the declared schema: its body is a
//     Part object, and every value that could be returned would assert an
//     update that was not performed (the part as it currently is does not
//     reflect the requested Part; echoing the requested Part would claim
//     "Successfully updated part" while the synthesized part still reports the
//     old text — the exact fabrication the handleSyncStart / handleGlobalUpgrade
//     truthfulness convention, 13189b1 / d8f1d59, forbids). Unlike
//     session.deleteMessage (ROUTE-FIX-035), whose declared 200 body is a plain
//     boolean that can report the truthful false ("no message was deleted"),
//     this operation declares an OBJECT the runtime cannot produce truthfully.
//
// Per the sessionUnshare precedent (ROUTE-FIX-015, SHIM-DRIFT-121) — the other
// operation that targets a resource the runtime keeps no concept of, where the
// declared 404 NotFoundError IS the truthful answer — the missing resource is
// reported inside the DECLARED vocabulary: 404 NotFoundError for an unknown
// part, never an undeclared 501 and never a fabricated 200. The declared 400
// arm is reached by malformed input, exactly as in the sibling handlers.
//
// Validation order mirrors the sibling on this sub-path
// (sessionDeleteMessage, ROUTE-FIX-035) plus the task's ordering:
//
//  1. unknown session -> declared 404 (p1ResolveSession);
//  2. absent/blank messageID -> declared 400, ill-shaped messageID (declared
//     pattern ^msg) -> declared 400 (sessionRevert/sessionDeleteMessage
//     validation precedent);
//  3. a well-formed messageID the session does not hold -> declared 404;
//  4. absent/blank partID -> declared 400, ill-shaped partID (declared pattern
//     ^prt) -> declared 400;
//  5. a Part request body that IS supplied must be a JSON object: unparseable
//     bytes or a non-object shape (array, string, null) -> declared 400 via
//     the p1DecodeBody conventions. An ABSENT body is not a contract violation
//     for this operation — its requestBody marks no required member (the whole
//     body is a bare $ref to Part) — so the request proceeds to the resource
//     lookup instead of inventing a 400 the declaration does not support;
//  6. no part store exists, so every well-formed prt id is an unknown part ->
//     the declared 404 NotFoundError.
//
// The declared response set for part.update is 200, 400, 404 — it declares NO
// 409 SessionBusyError, so unlike sessionRevert/sessionShell/
// sessionDeleteMessage this handler has no mid-turn arm and never calls
// sessionIsActive (409 is not in the declared set; inventing it would answer
// outside the contract).
func (s *Server) sessionPartUpdate(w http.ResponseWriter, r *http.Request, sessionID, messageID, partID string) {
	// 1. Unknown session -> the declared 404 NotFoundError.
	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}

	// 2. messageID is a required path parameter and the declared pattern is
	// ^msg (the sessionRevert / sessionDeleteMessage validation precedent).
	if strings.TrimSpace(messageID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required path parameter "messageID" is missing`)
		return
	}
	if !strings.HasPrefix(messageID, "msg") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"messageID must match the declared pattern ^msg")
		return
	}

	// 3. A well-formed messageID the session does not hold -> the declared 404.
	if !s.sessionHasMessage(r.Context(), sessionID, messageID) {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"message not found in session")
		return
	}

	// 4. partID is a required path parameter and the declared pattern is ^prt.
	if strings.TrimSpace(partID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required path parameter "partID" is missing`)
		return
	}
	if !strings.HasPrefix(partID, "prt") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"partID must match the declared pattern ^prt")
		return
	}

	// 5. The declared requestBody is a Part. When a body IS supplied it must be
	// a Part object — unparseable bytes or a non-object shape is the declared
	// 400 (the p1DecodeBody convention). An absent body is tolerated: the
	// operation's requestBody carries no required member, so a request without
	// one is not a contract violation and proceeds to the resource lookup.
	if r.Body != nil && r.ContentLength != 0 {
		var part map[string]any
		if !p1DecodeBody(w, r, &part) {
			return
		}
		if part == nil {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				"request body must be a Part object")
			return
		}
	}

	// 6. No part store and no part-editing engine exist (see the truthfulness
	// note above): parts are synthesized from memory_events.content and carry
	// no id, so no part with this id exists to update. The declared 404
	// NotFoundError is the truthful answer for every well-formed partID —
	// never a fabricated 200 Part asserting an update that did not happen.
	writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
		"part not found in message")
}

// sessionPartDelete serves DELETE
// /session/{sessionID}/message/{messageID}/part/{partID} — upstream part.delete
// (ROUTE-FIX-036; declared responses: 200, 400 BadRequest |
// InvalidRequestError, 404 NotFoundError).
//
// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/session/{sessionID}/message/{messageID}/part/{partID}".delete):
// sessionID, messageID and partID are path parameters with declared patterns
// ^ses, ^msg and ^prt, exactly as for the sibling part.update operation
// (ROUTE-FIX-037). Unlike part.update the operation declares NO requestBody.
//
// Truthfulness — the same reasoning as sessionPartUpdate applies verbatim and
// is not restated in full here: the Consensus runtime keeps no message-part
// editing engine and no part store (parts are SYNTHESIZED on read from
// memory_events.content and carry no id — memory_events is APPEND-ONLY by
// construction, SPEC-002 §2.1, enforced by the triggers in migrations 017
// (SQLite) and 018 (Postgres)), so there is no part identified by partID to
// delete, no tombstone can be recorded, and no truthful 200 exists to serve.
// Per the sessionUnshare precedent (ROUTE-FIX-015) the missing resource is
// reported inside the DECLARED vocabulary: a well-formed partID always gets
// the declared 404 NotFoundError "part not found in message" — never an
// undeclared 501 and never a fabricated 200.
//
// Validation order mirrors sessionPartUpdate (the sibling on this sub-path)
// and the task's ordering:
//
//  1. unknown session -> declared 404 (p1ResolveSession);
//  2. absent/blank messageID -> declared 400, ill-shaped messageID (declared
//     pattern ^msg) -> declared 400;
//  3. a well-formed messageID the session does not hold -> declared 404;
//  4. absent/blank partID -> declared 400, ill-shaped partID (declared
//     pattern ^prt) -> declared 400;
//  5. no part store exists, so every well-formed prt id is an unknown part ->
//     the declared 404 NotFoundError.
//
// The declared response set for part.delete is 200, 400, 404 — it declares NO
// 409 SessionBusyError, so this handler has no mid-turn arm and never calls
// sessionIsActive.
func (s *Server) sessionPartDelete(w http.ResponseWriter, r *http.Request, sessionID, messageID, partID string) {
	// 1. Unknown session -> the declared 404 NotFoundError.
	if _, ok := s.p1ResolveSession(w, r, sessionID); !ok {
		return
	}

	// 2. messageID is a required path parameter and the declared pattern is
	// ^msg (the sessionPartUpdate validation precedent).
	if strings.TrimSpace(messageID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required path parameter "messageID" is missing`)
		return
	}
	if !strings.HasPrefix(messageID, "msg") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"messageID must match the declared pattern ^msg")
		return
	}

	// 3. A well-formed messageID the session does not hold -> the declared 404.
	if !s.sessionHasMessage(r.Context(), sessionID, messageID) {
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"message not found in session")
		return
	}

	// 4. partID is a required path parameter and the declared pattern is ^prt.
	if strings.TrimSpace(partID) == "" {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			`required path parameter "partID" is missing`)
		return
	}
	if !strings.HasPrefix(partID, "prt") {
		writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			"partID must match the declared pattern ^prt")
		return
	}

	// 5. No part store exists (see the truthfulness note above): parts are
	// synthesized from memory_events.content and carry no id, so no part with
	// this id exists to delete. The declared 404 NotFoundError is the truthful
	// answer for every well-formed partID — never a fabricated 200.
	writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND",
		"part not found in message")
}
