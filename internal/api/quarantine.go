// Package api — quarantine API handlers (SPEC-005 §Cognitive Firewall).
//
// These endpoints allow operators to view and manage quarantined external data:
//   - GET  /api/v1/quarantine?status=pending  — list quarantined items
//   - POST /api/v1/quarantine/:id/approve      — approve and promote to memory
//   - POST /api/v1/quarantine/:id/reject       — reject with reason
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/015-api-and-mcp.md plan=phase-3/task-1/step-1
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/wojons/consensus/internal/quarantine"
)

// ============================================================================
// Quarantine API Handlers
// ============================================================================

// handleListQuarantine returns quarantined items, optionally filtered by status.
// GET /api/v1/quarantine?status=pending
func (s *Server) handleListQuarantine(w http.ResponseWriter, r *http.Request) {
	qs, ok := s.getQuarantineService()
	if !ok {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "quarantine service not configured")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	items, err := qs.ListQuarantine(r.Context(), statusFilter)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"items": items,
		"count": len(items),
	})
}

// handleApproveQuarantine approves a quarantined item, promoting it to agent memory.
// POST /api/v1/quarantine/:id/approve
//
// Request body (optional): {"session_id": "uuid"}
// If session_id is omitted, the session from the quarantine item is used.
func (s *Server) handleApproveQuarantine(w http.ResponseWriter, r *http.Request, id int64) {
	qs, ok := s.getQuarantineService()
	if !ok {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "quarantine service not configured")
		return
	}

	// Parse optional session_id from body
	sessionID := ""
	var body struct {
		SessionID string `json:"session_id"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			sessionID = body.SessionID
		}
	}

	item, err := qs.ApproveQuarantine(r.Context(), id, sessionID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "QUARANTINE_ERROR", err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"status":  "approved",
		"item":    item,
	})
}

// handleRejectQuarantine rejects a quarantined item with a reason.
// POST /api/v1/quarantine/:id/reject
//
// Request body: {"reason": "why this was rejected"}
func (s *Server) handleRejectQuarantine(w http.ResponseWriter, r *http.Request, id int64) {
	qs, ok := s.getQuarantineService()
	if !ok {
		writeError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "quarantine service not configured")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	if body.Reason == "" {
		body.Reason = "Rejected by operator"
	}

	item, err := qs.RejectQuarantine(r.Context(), id, body.Reason)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "QUARANTINE_ERROR", err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"status":  "rejected",
		"item":    item,
	})
}

// ============================================================================
// Helpers
// ============================================================================

// getQuarantineService returns the quarantine service if configured.
func (s *Server) getQuarantineService() (*quarantine.QuarantineService, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.quarantineSvc != nil {
		return s.quarantineSvc, true
	}
	return nil, false
}

// parseQuarantineID extracts a quarantine item ID from the URL path.
// Path format: /api/v1/quarantine/123/approve or /api/v1/quarantine/123/reject
func parseQuarantineID(path, prefix string) (int64, string, error) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 {
		return 0, "", http.ErrNotSupported
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", err
	}
	return id, parts[1], nil
}
