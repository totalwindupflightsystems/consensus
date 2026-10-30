package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// ROUTE-ADD-044 — GET /api/session/{sessionID}/event (v2.session.events)
// ============================================================================
//
// The pinned upstream operation declares 200 text/event-stream, 400
// InvalidRequestError, 401 UnauthorizedError, and 404 SessionNotFoundError.
// The runtime's durable event source is the append-only memory_events ledger,
// with display_modes providing the stored full/compressed/hidden view. The
// stream therefore replays real ledger rows after the optional numeric `after`
// boundary and polls the same ledger for later committed rows. It does not
// mint OpenCode evt_/msg_ identities or session.next.* discriminators.
//
// ch:trace row=ROUTE-ADD-044 spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.events test=internal/shim/opencode/api_session_event_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-30-03-42-44.json evidence=none:worker-local-unpublished witness=none:worker-local verdict=none:pending-judge commit=none:pending memory=/project/consensus/status/ticks/consensus-foreman-2026-10-30-03-42-44

const apiSessionEventPollInterval = 2 * time.Second

// handleApiSessionEvent serves GET /api/session/{sessionID}/event — upstream
// v2.session.events (ROUTE-ADD-044, declared responses 200, 400, 401, 404).
func (s *Server) handleApiSessionEvent(w http.ResponseWriter, r *http.Request) {
	if !apiV2RequireGet(w, r) {
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	after := int64(0)
	if r.URL.Query().Has("after") {
		raw := r.URL.Query().Get("after")
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if raw == "" || err != nil || parsed < 0 {
			writeV2InvalidRequest(w, "Query", "after",
				"after must be a non-negative memory_events id (the runtime resume token)")
			return
		}
		after = parsed
	}

	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "memory_events",
			"failed to read session events: the shim has no session store")
		return
	}

	// Resolve the session before sending streaming headers. A missing session
	// is a typed 404; an unreadable sessions store cannot be mistaken for an
	// absent session because this operation has no declared 5xx arm.
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: no events to stream", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: no events to stream", sessionID))
		return
	}

	query := apiSessionEventsQuery + ` AND me.id > $2 ORDER BY me.id ASC`
	rows, err := s.db.Query(r.Context(), query, sessionID, after)
	if err != nil {
		writeV2InvalidRequest(w, "Shim", "memory_events", "failed to read session events: "+err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeV2InvalidRequest(w, "Shim", "stream", "the response writer does not support event streaming")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	lastID := after
	for _, event := range rows {
		id := toInt64(event["id"])
		if id > lastID {
			lastID = id
		}
		if err := writeApiSessionEventFrame(w, flusher, event); err != nil {
			return
		}
	}

	// A stream remains open after replay so later durable commits are visible.
	// The request context is the sole shutdown signal; closing the client
	// cancels it and stops the ticker without inventing a terminal event.
	ticker := time.NewTicker(apiSessionEventPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			rows, err := s.db.Query(r.Context(), query, sessionID, lastID)
			if err != nil {
				// Headers are already committed, so an HTTP error envelope cannot
				// be added truthfully. End the stream rather than writing a
				// malformed SSE error frame outside the declared event schema.
				return
			}
			for _, event := range rows {
				id := toInt64(event["id"])
				if id > lastID {
					lastID = id
				}
				if err := writeApiSessionEventFrame(w, flusher, event); err != nil {
					return
				}
			}
		}
	}
}

func writeApiSessionEventFrame(w http.ResponseWriter, flusher http.Flusher, event map[string]any) error {
	mode := toString(event["display_mode"])
	if mode == "hidden" {
		return nil
	}

	data := map[string]any{
		"memory_event_id":   toInt64(event["id"]),
		"type":              toString(event["type"]),
		"iteration_created": toInt64(event["iteration_created"]),
		"created_at":        toString(event["created_at"]),
	}
	if mode == "compressed" {
		data["display_mode"] = mode
		if summary := strings.TrimSpace(toString(event["summary_text"])); summary != "" {
			data["summary_text"] = summary
		}
	} else {
		data["content"] = toString(event["content"])
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", toInt64(event["id"]), toString(event["type"]), encoded); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// isApiSessionEventPath admits only the declared concrete
// /api/session/{sessionID}/event shape to the v2 UnauthorizedError envelope.
func isApiSessionEventPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "event"
}
