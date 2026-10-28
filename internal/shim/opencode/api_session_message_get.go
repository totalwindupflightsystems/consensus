package opencode

// ============================================================================
// ROUTE-ADD-048 — GET /api/session/{sessionID}/message/{messageID}
//                 (v2.session.message)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."\/api/session/{sessionID}/message/{messageID}".get) declares
// v2.session.message with declared responses
//
//	200 — {data: SessionMessage} (data REQUIRED, additionalProperties:false)
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//	404 — MessageNotFoundError | SessionNotFoundError (the declared anyOf is
//	      ambiguous between unknown session and unknown message — this
//	      handler resolves the session FIRST so a client can tell the two
//	      apart)
//
// and the declared parameters: the required path sessionID (pattern ^ses)
// and messageID (pattern ^msg_). No requestBody, no query selector. GET is
// the ONLY method the document declares on this path.
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class.
//
// TRUTHFULNESS (the narrowing): the declared 200 carries a SessionMessage —
// a discriminated union whose every member requires an id matching ^msg_
// (SessionMessageUser additionally requires time.created, text, type=user;
// SessionMessageAssistant requires agent, model{ModelRef}, content[]; the
// family is closed by additionalProperties:false). The runtime's message
// rows are memory_events rows keyed by the native autoincrement integer —
// the same identifiers the v1 message routes serve — and minting a "msg_…"
// id for this response would fabricate an identifier the sibling v2
// message-list route never emits and the v1 routes never resolve (the
// ROUTE-ADD-026/039/040 reshape-refusal enumeration). Unlike those list
// operations there is no runtime-native shape that can stand in INSIDE the
// declared envelope either: the declared data member IS the SessionMessage
// object itself. The declared 200 is therefore unreachable for every real
// row and the operation is served as an HONEST ERROR-CONTRACT NARROWING
// (SHIM-GAP-004; the ROUTE-ADD-009 v2.event.subscribe precedent): a
// well-formed messageID this session does not hold answers the declared 404
// MessageNotFoundError {_tag, sessionID, messageID, message}, and so does
// one it does hold — asserting a delivered message would be the same lie as
// asserting a delivered pty. The narrowing is recorded on the served-surface
// row and in the §12.3 ledger; widening requires the runtime to key messages
// by ^msg_ identifiers (a design change with its own row, not a contract
// detail).
//
// ch:trace row=ROUTE-ADD-048 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.message test=internal/shim/opencode/api_session_message_get_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-28-07-41-25.json evidence=docs/evidence/ROUTE-ADD-045-live-probe.md witness=none:unattended-worker-session commit=none:pending-foreman-trace STATUS: complete TIER: T3 (live-probed: every declared arm curl-verified on a scratch instance; transcript committed) EVIDENCE: docs/evidence/ROUTE-ADD-045-live-probe.md
//
// Arm map:
//
//	400 InvalidRequestError — a blank sessionID (kind Path, field sessionID)
//	      or a blank messageID (kind Path, field messageID). The declared
//	      ^msg_ pattern is NOT enforced as a 400: the runtime's message ids
//	      are the integers the v1 surface serves (native ids — the same
//	      divergence v2.session.get documents for ^ses), so an unknown id of
//	      any shape is the declared 404 rather than a pattern 400.
//	401 UnauthorizedError — the v2 envelope via isApiSessionMessageGetPath.
//	404 SessionNotFoundError — the sessionID names no session (resolved
//	      FIRST so the two 404 meanings are distinguishable).
//	404 MessageNotFoundError — the session exists but the messageID names no
//	      memory_events row of it (exact numeric match after the conventional
//	      "msg-" prefix strip, the sessionHasMessage resolution, then a
//	      prefix fallback) — and the typed SessionMessage 200 is never
//	      fabricated (file doc comment), so a held message answers the same
//	      declared 404 naming messageID.
//	500 — NOT declared on this operation: a store the instance cannot read
//	      answers the declared InvalidRequestError 400 (the
//	      api_session_agent.go convention), never 500.
//	405 METHOD_NOT_ALLOWED — every method other than GET.

import (
	"fmt"
	"net/http"
	"strings"
)

// handleApiSessionMessageGet serves GET /api/session/{sessionID}/message/
// {messageID} — upstream v2.session.message (ROUTE-ADD-048, declared
// responses 200, 400, 401, 404).
func (s *Server) handleApiSessionMessageGet(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if messageID == "" {
		writeV2InvalidRequest(w, "Path", "messageID", "messageID path parameter must not be blank")
		return
	}

	// No 5xx arm is declared on this operation: an absent or unreadable
	// store is reported through the declared InvalidRequestError.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: the shim has no session store")
		return
	}

	// 1. Resolve the session (declared 404 names sessionID).
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no message to resolve", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no message to resolve", sessionID))
		return
	}

	// 2. Resolve the message inside that session (declared 404 names
	//    messageID) — the sessionHasMessage resolution: exact numeric match
	//    after the conventional "msg-" strip, then a prefix fallback.
	if !s.sessionHasMessage(r.Context(), sessionID, messageID) {
		writeV2MessageNotFoundError(w, sessionID, messageID,
			fmt.Sprintf("no message %q in session %q (or held, but the declared SessionMessage body is not producible truthfully)", messageID, sessionID))
		return
	}

	// The session holds the message row — and the declared 200
	// SessionMessage is exactly what this runtime cannot truthfully produce
	// (file doc comment). The declared 404 MessageNotFoundError names the
	// situation honestly instead of minting a ^msg_ identity.
	writeV2MessageNotFoundError(w, sessionID, messageID,
		"the session holds this message row, but the declared SessionMessage body (a ^msg_-keyed union) is not producible truthfully from this runtime's integer-keyed ledger")
}

// writeV2MessageNotFoundError answers the declared 404 MessageNotFoundError
// arm ({_tag, sessionID, messageID, message}).
func writeV2MessageNotFoundError(w http.ResponseWriter, sessionID, messageID, msg string) {
	writeJSONStatus(w, http.StatusNotFound, map[string]any{
		"_tag":      "MessageNotFoundError",
		"sessionID": sessionID,
		"messageID": messageID,
		"message":   msg,
	})
}

// isApiSessionMessageGetPath admits only the declared concrete route shape
// (/api/session/{sessionID}/message/{messageID} — session id and message id
// non-empty, second segment literally "message", exactly three segments) to
// the v2 UnauthorizedError auth branch. The sibling two-segment shapes and
// every deeper /api/session/* sub-path keep the shim's own envelope.
func isApiSessionMessageGetPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	if !ok {
		return false
	}
	if sessionID == "" {
		return false
	}
	second, messageID, ok := strings.Cut(tail, "/")
	return second == "message" && ok && messageID != "" && !strings.Contains(messageID, "/")
}

// cutApiSessionSubpath splits a concrete /api/session/<id>/<sub> path into
// the session id and the remainder after it — the shared shape check behind
// the three isApiSession*Path envelope rules added by ROUTE-ADD-045/047/048.
func cutApiSessionSubpath(path string) (sessionID, tail string, ok bool) {
	rest, ok := strings.CutPrefix(path, "/api/session/")
	if !ok {
		return "", "", false
	}
	return strings.Cut(rest, "/")
}
