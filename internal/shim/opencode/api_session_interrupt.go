// ch:trace row=ROUTE-ADD-046 spec=specs/024-shim-support-matrix.md#v2-session-interrupt test=internal/shim/opencode/api_session_interrupt_test.go evidence=none:foreman-regenerates-artifact-post-merge doc=specs/024-shim-support-matrix.md prompt=/tmp/consensus-foreman-2026-10-30-02-18-01-ROUTE-ADD-046.txt witness=none:unattended-worker-session verdict=none:pending-judge commit=none:pending-commit memory=/project/consensus/status/ticks/consensus-foreman-2026-10-30-02-18-01
package opencode

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// handleApiSessionInterrupt serves POST /api/session/{sessionID}/interrupt —
// upstream v2.session.interrupt. Consensus does not track an interrupted
// session state, so a well-formed request for an existing session is accepted
// as a control-plane signal and answered with the declared empty 204.
func (s *Server) handleApiSessionInterrupt(w http.ResponseWriter, r *http.Request) {
	// This operation intentionally preserves the pre-route default 404 for
	// undeclared methods; the upstream document declares POST only and does not
	// declare a 405 arm.
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must be one non-blank path segment")
		return
	}

	// The middleware has already authenticated the request. Re-read the
	// session-scoped identity here so a key bound to another session cannot
	// control this session; admin keys have no session binding and are allowed.
	if !s.skipAuth {
		keySessionID, ok := s.validateAuth(r)
		if !ok {
			writeV2Unauthorized(w, "invalid credentials")
			return
		}
		if keySessionID != "" && keySessionID != sessionID {
			writeOpencodeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key is not authorized for this session")
			return
		}
	}

	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return
	}

	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to interrupt", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to interrupt", sessionID))
		return
	}

	slog.Info("opencode-shim: session interrupt accepted", "session_id", sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// isApiSessionInterruptPath admits exactly the declared concrete route shape
// to the v2 UnauthorizedError envelope.
func isApiSessionInterruptPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "interrupt"
}
