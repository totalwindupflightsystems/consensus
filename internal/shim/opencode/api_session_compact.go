package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-042 — POST /api/session/{sessionID}/compact (v2.session.compact)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session/{sessionID}/compact".post) declares v2.session.compact —
// summary "Compact session", description "Compact a session conversation." —
// with declared responses
//
//	204 — <No Content>
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//	404 — SessionNotFoundError
//	503 — ServiceUnavailableError
//
// one required path parameter sessionID (a plain string whose declared pattern
// is ^ses), NO requestBody and NO query parameters. POST is the ONLY method the
// document declares on this path.
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation". The route is
// registered as its own EXACT two-segment pattern (server.go, mux.HandleFunc
// "/api/session/{sessionID}/compact", plus the matching MountPatterns entry so a
// parent chi mount reaches it); it is never an /api/session/* catch-all, so
// every sibling /api/session/{sessionID}[...] operation keeps the
// classification its own contract row is filed against, and the exact
// two-segment shape joins the v2 UnauthorizedError envelope through
// isV2EnvelopeRoutePath (server.go).
//
// Arms:
//
//	204 — the declared empty success, answered as a truthful no-op. What the
//	      runtime actually keeps is a BACKGROUND memory-compression pipeline:
//	      internal/compression runs a queue-driven worker (cmd/consensus starts
//	      it only when compression is enabled) that summarizes a memory_event,
//	      gates the summary on an embedding cosine threshold and then writes
//	      memory_events.summary_text plus the display_modes tier — an internal
//	      display-tier policy over single memory events, driven by the queue the
//	      harness fills from its context budget. It is NOT an addressable
//	      per-session "compact this conversation" operation: this adapter (the
//	      shim Server) holds no compression worker, summarizer or embedding
//	      client, and it owns no opencode session conversation to compact — its
//	      sessions rows record Consensus agent runs. The declared success
//	      carries NO body at all, so the 204 asserts nothing the runtime cannot
//	      back: it claims no summary was produced, echoes no identifier and
//	      invents no state (the ROUTE-ADD-041 switchAgent / ROUTE-ADD-049
//	      switchModel truthful-204 precedent; contrast the v1
//	      POST /session/{sessionID}/summarize route, ROUTE-FIX-018, whose
//	      declared success body IS a boolean and which therefore answers the
//	      truthful false instead). Making this route perform a real on-demand
//	      compaction — enqueuing the session's uncompressed events onto the
//	      compression_queue — would be a design change (the protocol adapter
//	      would take a dependency on the compression pipeline and its LLM
//	      spend) with its own row, not a contract detail.
//	400 InvalidRequestError — a present-but-blank sessionID segment (%20); the
//	      declared parameter is a plain required string, so a blank segment is
//	      the only malformed spelling a handler can observe (kind Path, field
//	      sessionID). The declared 400 arm covers no other input: the document
//	      declares no requestBody and no query parameter, and a handler cannot
//	      reject input the document does not declare (the handleApiSessionActive
//	      convention), so an unexpected body or query string is ignored, not
//	      refused.
//	401 UnauthorizedError — the /api/* auth wall; this exact deeper shape is
//	      admitted by isV2EnvelopeRoutePath through isApiSessionCompactPath, so
//	      the body is the declared v2 envelope rather than the shim's own v1
//	      UNAUTHENTICATED body.
//	404 SessionNotFoundError — the sessionID is not a session this instance
//	      holds ({_tag, sessionID, message}, the declared shape). The decision is
//	      read from the runtime's REAL sessions store (migration 001 §3.2)
//	      through the shared isNoRowsResult seam (internal/db reports a missing
//	      row as its own error value, not sql.ErrNoRows — the ROUTE-ADD-041
//	      pattern), so a client distinguishes "no such session" from "the request
//	      was accepted".
//	503 ServiceUnavailableError — the sessions store cannot be read (no store on
//	      this instance, or a read failure that is NOT a missing row). THIS
//	      operation declares a 5xx arm (unlike the sibling switchAgent /
//	      switchModel / history rows, which declare none and therefore answer the
//	      declared 400 for an unreadable store), so the store failure is reported
//	      through it: {_tag, message, service: "sessions"} using the declared
//	      optional service member to name the component that is unavailable.
//	      Absence is still never claimed from a store that could not be read — a
//	      read failure is the 503, never the 404.
//	405 METHOD_NOT_ALLOWED — every method other than POST; POST is the only
//	      method the pinned document declares on this path, so no sibling
//	      operation is reclassified by this registration.
//
// ch:trace row=ROUTE-ADD-042 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.compact test=internal/shim/opencode/api_session_compact_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=docs/evidence/ROUTE-ADD-042-live-probe.md witness=none:unattended-worker-session verdict=none:foreman-tier2-pending commit=none:pre-commit memory=none:foreman-writes
//
// handleApiSessionCompact serves POST /api/session/{sessionID}/compact —
// upstream v2.session.compact (ROUTE-ADD-042, declared responses 204, 400, 401,
// 404, 503). See the file doc comment for the arm-by-arm contract and for why
// the declared 204 is the truthful answer.
func (s *Server) handleApiSessionCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	// Unlike the sibling routes this operation DOES declare a 5xx arm, so a
	// store the instance cannot read is reported through the declared
	// ServiceUnavailableError (service "sessions") rather than through the
	// declared InvalidRequestError — absence is never claimed from a store the
	// handler could not read.
	if s.db == nil {
		writeV2ServiceUnavailable(w,
			"failed to resolve session: the shim has no session store", "sessions")
		return
	}

	// Resolve the session against the runtime's real session store. The db
	// drivers in internal/db report a missing row as an error (there is no
	// sql.ErrNoRows sentinel on this seam), so the two answers are told apart
	// explicitly: a "no rows in result" reply is the declared 404, while any
	// OTHER read failure is the declared 503.
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to compact", sessionID))
			return
		}
		writeV2ServiceUnavailable(w, "failed to resolve session: "+err.Error(), "sessions")
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to compact", sessionID))
		return
	}

	// The session is one this instance holds and the request is well formed.
	// The declared 204 is the truthful answer: this adapter holds no
	// conversation-compaction pipeline (file doc comment), so no compaction ran
	// and the declared success — which carries no body — asserts nothing. The
	// status is written without a body so the empty success stays empty.
	w.WriteHeader(http.StatusNoContent)
}

// isApiSessionCompactPath admits only the declared concrete route shape
// (/api/session/{sessionID}/compact — exactly two non-empty segments, the second
// literally "compact") to the v2 UnauthorizedError auth branch. Sibling and
// deeper /api/session/* paths keep the shim's own envelope (the
// isApiSessionAgentPath / isApiSessionModelPath precedent).
func isApiSessionCompactPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/session/")
	if !ok {
		return false
	}
	sessionID, tail, ok := strings.Cut(rest, "/")
	if !ok {
		return false
	}
	return sessionID != "" && tail == "compact"
}
