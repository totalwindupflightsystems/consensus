package opencode

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// defaultSessionWaitPollInterval is how often POST /api/session/{sessionID}/wait
// re-reads the session's agent-loop status while the loop is still busy. The
// read is one primary-key lookup on the sessions table (migration 001 §3.2), so
// the interval trades at most one indexed query per this window per waiting
// client against how promptly the declared 204 lands after the loop goes idle.
// It is a field on the Server (sessionWaitPollInterval) rather than a bare
// const so tests can shrink it; the constructor sets this default.
const defaultSessionWaitPollInterval = 100 * time.Millisecond

// ============================================================================
// ROUTE-ADD-061 — POST /api/session/{sessionID}/wait (v2.session.wait)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session/{sessionID}/wait".post) declares v2.session.wait — summary
// "Wait for session", description "Wait for a session agent loop to become
// idle." — with declared responses
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
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED class
// (source item SHIM-DRIFT-057 in the committed artifact's positional ids; the
// comparator RENUMBERS drift ids on every regeneration, so this row, the
// served-surface row and the tests are keyed on operationId v2.session.wait —
// never on a drift id). The route is registered as its own EXACT two-segment
// pattern (server.go, mux.HandleFunc "/api/session/{sessionID}/wait", plus the
// matching MountPatterns entry so a parent chi mount reaches it); it is never an
// /api/session/* catch-all, so every sibling /api/session/{sessionID}[...]
// operation keeps the classification its own contract row is filed against, and
// the exact two-segment shape joins the v2 UnauthorizedError envelope through
// isV2EnvelopeRoutePath (server.go).
//
// Arms:
//
//	204 — the declared empty success, answered when the session's agent loop is
//	      NOT running: immediately for a session that is already idle, and
//	      after the wait for one that was busy. Unlike the sibling
//	      ROUTE-ADD-041/042/049/054 204s, this one is not a no-op placeholder —
//	      the runtime really does track an agent loop per session, so the
//	      declared condition is evaluated against real state. The sessions row
//	      (migration 001 §3.2) carries `status`, the finite-state-machine member
//	      internal/session/session.go defines (booting, idle, planning, thinking,
//	      tool_exec, executing, waiting_sub, completed, failed, paused), and the
//	      harness loop (internal/harness executor + planning) is what moves a
//	      session through the working states and back to idle. The handler polls
//	      that column and answers 204 as soon as the status leaves the working
//	      set — the shared sessionIsActive predicate (session_p1_routes.go:45)
//	      names exactly planning/thinking/tool_exec/executing/waiting_sub, the
//	      same "mid-turn" test the sibling POST prompt route uses to detect a
//	      busy session. idle, paused and the terminal states (completed, failed)
//	      have no loop running and therefore answer immediately; booting is not
//	      in the working set either (a session still being constructed claims no
//	      loop yet), and an empty/NULL status column reads as "no loop claimed".
//	      The declared success carries NO body, so the 204 asserts nothing
//	      beyond "the loop is not running now" — it echoes no identifier and
//	      invents no state.
//	400 InvalidRequestError — a present-but-blank sessionID segment (%20); the
//	      declared parameter is a plain required string, so a blank segment is
//	      the only malformed spelling a handler can observe (kind Path, field
//	      sessionID). The declared ^ses pattern is NOT enforced as a 400: the
//	      runtime's session ids are native UUIDs (the same divergence
//	      v2.session.get / v2.session.compact document), so an id of any shape
//	      resolves against the real store instead. The document declares no
//	      requestBody and no query parameter, and a handler cannot reject input
//	      the document does not declare (the handleApiSessionActive convention),
//	      so an unexpected body or query string is ignored, not refused.
//	401 UnauthorizedError — the /api/* auth wall; this exact deeper shape is
//	      admitted by isV2EnvelopeRoutePath through isApiSessionWaitPath, so the
//	      body is the declared v2 envelope rather than the shim's own v1
//	      UNAUTHENTICATED body.
//	404 SessionNotFoundError — the sessionID is not a session this instance
//	      holds ({_tag, sessionID, message}, the declared shape). The decision is
//	      read from the runtime's REAL sessions store through the shared
//	      isNoRowsResult seam (internal/db reports a missing row as its own error
//	      value, not sql.ErrNoRows — the ROUTE-ADD-041 pattern), on the first
//	      read AND on every poll inside the wait: a session deleted while a
//	      client waits is reported as it now is, never as a settled wait.
//	503 ServiceUnavailableError — THIS operation declares a 5xx arm (unlike the
//	      sibling switchAgent / switchModel / history rows, which declare none and
//	      therefore answer the declared 400 for an unreadable store), and both
//	      ways the condition cannot be evaluated are reported through it:
//	        (a) the sessions store cannot be read ({_tag, message,
//	            service: "sessions"}), at session resolution or at any poll inside
//	            the wait — absence is never claimed from a store the handler could
//	            not read;
//	        (b) the wait itself expired: the loop was still in a working state
//	            when the shim's synchronous-turn budget ran out, so no idle state
//	            was observed and the declared success is not asserted. The message
//	            names the status last read and the budget, so a client can tell a
//	            long turn from a dead one instead of receiving an invented 204.
//	            The budget is messageResponseTimeout (server.go) — the same
//	            deployment knob (CONSENSUS_MESSAGE_RESPONSE_TIMEOUT /
//	            adapters.opencode.message_response_timeout_seconds, BUG-013) that
//	            bounds every other synchronous wait-for-the-agent endpoint, so a
//	            deployment that raised it above its planning wall gets the
//	            matching wait window here; request cancellation shortens it
//	            further (a client that disconnects gets no write at all).
//	405 METHOD_NOT_ALLOWED — every method other than POST; POST is the only
//	      method the pinned document declares on this path, so no sibling
//	      operation is reclassified by this registration.
//
// ch:trace row=ROUTE-ADD-061 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.wait test=internal/shim/opencode/api_session_wait_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=docs/evidence/ROUTE-ADD-061-live-probe.md witness=none:unattended-worker-session verdict=none:foreman-tier2-pending commit=none:pre-commit memory=none:foreman-writes
//
// handleApiSessionWait serves POST /api/session/{sessionID}/wait — upstream
// v2.session.wait (ROUTE-ADD-061, declared responses 204, 400, 401, 404, 503).
// See the file doc comment for the arm-by-arm contract and for how the declared
// "wait for the agent loop to become idle" maps onto the runtime's real session
// status column.
func (s *Server) handleApiSessionWait(w http.ResponseWriter, r *http.Request) {
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
			"failed to wait for session: the shim has no session store", "sessions")
		return
	}

	budget := s.messageResponseTimeout
	if budget <= 0 {
		budget = defaultMessageResponseTimeout
	}
	poll := s.sessionWaitPollInterval
	if poll <= 0 {
		poll = defaultSessionWaitPollInterval
	}

	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	waited := false
	for {
		// Resolve the session on every pass: the declared 404/503 arms are
		// decided from the real store, and a session that disappears or a store
		// that stops answering while a client waits is reported as it now is.
		row, err := s.db.QueryRow(r.Context(), `SELECT status FROM sessions WHERE id = $1`, sessionID)
		if err != nil {
			if r.Context().Err() != nil {
				// The client is gone; there is nobody to answer. Nothing is
				// claimed about the session.
				return
			}
			if isNoRowsResult(err) {
				writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
					fmt.Sprintf("no session %q in this runtime: nothing to wait for", sessionID))
				return
			}
			writeV2ServiceUnavailable(w, "failed to resolve session: "+err.Error(), "sessions")
			return
		}
		if row == nil {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to wait for", sessionID))
			return
		}

		status := strings.TrimSpace(toString(row["status"]))
		if !sessionIsActive(status) {
			if waited {
				slog.Info("opencode-shim: session agent loop became idle while a client waited",
					"session_id", sessionID, "status", status)
			}
			// The declared success: the loop is not running now. The 204 carries
			// no body, so nothing beyond that observable fact is asserted.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !waited {
			waited = true
			slog.Info("opencode-shim: session wait is blocking on a running agent loop",
				"session_id", sessionID, "status", status, "budget", budget.String())
		}

		select {
		case <-r.Context().Done():
			// Client cancellation: no write, no claim.
			return
		case <-deadline.C:
			// The declared 5xx arm covers "the shim could not serve this wait":
			// the loop was still working when the budget ran out, so the
			// declared idle state was never observed and no 204 is invented.
			writeV2ServiceUnavailable(w, fmt.Sprintf(
				"session %q agent loop is still %q after %s: no idle state was observed, try again",
				sessionID, status, budget), "sessions")
			return
		case <-ticker.C:
		}
	}
}

// isApiSessionWaitPath admits only the declared concrete route shape
// (/api/session/{sessionID}/wait — exactly two non-empty segments, the second
// literally "wait") to the v2 UnauthorizedError auth branch. Sibling and deeper
// /api/session/* paths keep the shim's own envelope (the
// isApiSessionAgentPath / isApiSessionCompactPath precedent).
func isApiSessionWaitPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "wait"
}
