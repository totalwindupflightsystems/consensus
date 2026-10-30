package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-043 — GET /api/session/{sessionID}/context (v2.session.context)
// ============================================================================
//
// The pinned upstream operation declares 200, 400, 401, 404 and 500. The
// Consensus ledger is not keyed by OpenCode's ^msg_ identifiers, so the 200
// envelope is preserved while its entries use the runtime-native memory event
// fields, as the sibling history/messages routes do. Hidden events are omitted
// and compressed events expose their stored summary rather than fabricated
// OpenCode message objects.
//
// ch:trace row=ROUTE-ADD-043 spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.context test=internal/shim/opencode/api_session_context_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-30-00-02-15.json evidence=docs/evidence/ROUTE-ADD-043-live-probe.md witness=none:pending verdict=none:foreman-judges commit=none:pending

// handleApiSessionContext serves GET /api/session/{sessionID}/context.
func (s *Server) handleApiSessionContext(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}
	if s.db == nil {
		writeV2UnknownError(w, "the shim has no session store")
		return
	}

	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no context to return", sessionID))
			return
		}
		writeV2UnknownError(w, "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no context to return", sessionID))
		return
	}

	rows, err := s.db.Query(r.Context(), apiSessionEventsQuery+` AND COALESCE(dm.mode, 'full') != 'hidden' ORDER BY me.id ASC`, sessionID)
	if err != nil {
		writeV2UnknownError(w, "failed to read session context: "+err.Error())
		return
	}

	data := make([]map[string]any, 0, len(rows))
	for _, event := range rows {
		mode := toString(event["display_mode"])
		entry := map[string]any{
			"memory_event_id":   toInt64(event["id"]),
			"type":              toString(event["type"]),
			"iteration_created": toInt64(event["iteration_created"]),
			"created_at":        toString(event["created_at"]),
		}
		if mode == "compressed" {
			entry["display_mode"] = mode
			if summary := strings.TrimSpace(toString(event["summary_text"])); summary != "" {
				entry["summary_text"] = summary
			}
		} else {
			entry["content"] = toString(event["content"])
		}
		data = append(data, entry)
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{"data": data})
}

// isApiSessionContextPath admits only the declared concrete path shape to the
// v2 UnauthorizedError envelope. Sibling and deeper session paths retain their
// own routing and envelope behavior.
func isApiSessionContextPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "context"
}
