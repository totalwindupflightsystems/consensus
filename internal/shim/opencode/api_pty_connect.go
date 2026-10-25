package opencode

import (
	"net/http"
	"strings"
)

// handleApiPtyConnect serves GET /api/pty/{ptyID}/connect (v2.pty.connect).
// The shim has no PTY registry or websocket connector, so a well-formed
// request returns the declared typed 404 rather than fabricating success.
// ch:trace row=ROUTE-ADD-033 spec=specs/024-shim-support-matrix.md wave=consensus-2026-10-24-23-58-46#serial test=internal/shim/opencode/api_pty_connect_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-2026-10-24-23-58-46-ROUTE-ADD-033-brief.txt evidence=none:pending witness=none:self-verified-in-worktree verdict=none:pending commit=none:pending memory=none:pending
func (s *Server) handleApiPtyConnect(w http.ResponseWriter, r *http.Request) {
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
