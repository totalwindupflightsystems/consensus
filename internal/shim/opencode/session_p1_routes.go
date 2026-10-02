package opencode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
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
		w.Write(data)
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
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
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
		writeOpencodeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
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
