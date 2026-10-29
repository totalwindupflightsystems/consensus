package opencode

import (
	"net/http"
)

// handleApiPtyConnectToken serves POST /api/pty/{ptyID}/connect-token — the
// declared v2 pty.connectToken operation (ROUTE-ADD-034, SHIM-DRIFT-030).
//
//	ch:trace row=ROUTE-ADD-034 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.connectToken test=internal/shim/opencode/api_pty_connect_token_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
//
// The v2 path mirrors the v1 /pty/{ptyID}/connect-token (pty_routes.go
// handlePtyConnectToken, ROUTE-ADD-109) — same declared response vocabulary
// (200 PtyTicketConnectToken, 400 BadRequestError, 403 PtyForbiddenError, 404
// PtyNotFoundError), same validation order (ptyID pattern → selector verdict →
// lookup), same truthfulness rule: the shim keeps no PTY registry and mints no
// connect tickets, so the declared 200 is never fabricated and every
// well-formed ptyID answers the declared 404 typed PtyNotFoundError.
//
// Delegation: the v2 handler calls the v1 handlePtyConnectToken so the
// validation logic, error envelopes, and log lines stay byte-identical across
// the two path prefixes. Registering the v2 route as a thin wrapper avoids
// duplicating the 40-line validation block and keeps the declared error
// contract truthful by construction.
//
// Undeclared methods keep the PRE-CHANGE answer: net/http's default 404
// (ptyMethodNotRouted). 405 is not in the declared response set.
//
// Auth is untouched: the /api/pty/{ptyID}/connect-token path is not a stub
// path and carries no fixed-workspace exemption, so it keeps the standard
// api-key policy; the declared 401 arm is the v2 UnauthorizedError envelope
// via the exact two-segment v2EnvelopeRoutePaths match (server.go).
func (s *Server) handleApiPtyConnectToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ptyMethodNotRouted(w, r)
		return
	}
	// Delegate to the v1 handler — the declared validation, error envelopes,
	// and truthfulness rule are identical. The v1 handler reads ptyID from
	// r.PathValue("ptyID"), which chi populates from the registered pattern
	// /api/pty/{ptyID}/connect-token (server.go).
	s.handlePtyConnectToken(w, r)
}
