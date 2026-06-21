// Package api: session endpoint handlers (SPEC-015 §3.1).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-1/step-2-1-1 impl=internal/api/sessions.go
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// POST /api/v1/sessions — Create a new agent session
// ============================================================================

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "only admin keys may create sessions")
		return
	}

	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}

	result, err := s.svc.Sessions.CreateSession(r.Context(), CreateSessionInput{
		AgentName:     req.AgentName,
		Goal:          req.Goal,
		ModelID:       req.ModelID,
		ContextBudget: req.ContextBudget,
		ProjectID:     req.ProjectID,
	})
	if err != nil {
		slog.Error("api: failed to create session", "error", err)
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(CreateSessionResponse{
		ID:        result.SessionID,
		Status:    result.Status,
		APIKey:    result.APIKey,
		ModelID:   result.ModelID,
		ProjectID: result.ProjectID,
		CreatedAt: time.Now().UTC(),
	})
	w.Write(data)
}

// ============================================================================
// GET /api/v1/sessions — List sessions with optional filters
// ============================================================================

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)
	statusFilter := r.URL.Query().Get("status")

	results, err := s.svc.Sessions.ListSessions(r.Context(), statusFilter, sessionID, scope)
	if err != nil {
		slog.Error("api: failed to list sessions", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list sessions")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /api/v1/sessions/{id} — Get session details
// ============================================================================

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request, id string) {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)

	if scope == "session" && sessionID != id {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key can only access its own session")
		return
	}

	resp, err := s.svc.Sessions.GetSession(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	writeJSON(w, resp)
}

// ============================================================================
// PATCH /api/v1/sessions/{id} — Update session (pause, resume, cancel)
// ============================================================================

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request, id string) {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)

	if scope == "session" && sessionID != id {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key can only modify its own session")
		return
	}

	var req UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	if req.Status == nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "status field is required")
		return
	}

	newStatus := *req.Status

	// Validate status transition
	validTransitions := map[string][]string{
		"paused":  {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"},
		"resume":  {"paused"},
		"cancel":  {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub", "paused"},
		"idle":    {"thinking", "planning"}, // sent by harness
		"failed":  {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"}, // sent by harness
	}

	ctx := r.Context()

	// Get current status
	row, err := s.db.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	currentStatus := toString(row["status"])

	// Handle special transitions
	var targetStatus string
	switch newStatus {
	case "pause":
		allowed, ok := validTransitions["paused"]
		if !ok {
			writeError(w, r, http.StatusConflict, "CONFLICT", "invalid transition")
			return
		}
		found := false
		for _, a := range allowed {
			if a == currentStatus {
				found = true
				break
			}
		}
		if !found {
			writeError(w, r, http.StatusConflict, "CONFLICT",
				fmt.Sprintf("cannot pause session in status %q", currentStatus))
			return
		}
		targetStatus = "paused"

	case "resume":
		if currentStatus != "paused" {
			writeError(w, r, http.StatusConflict, "CONFLICT",
				fmt.Sprintf("can only resume paused sessions, current status is %q", currentStatus))
			return
		}
		targetStatus = "idle"

	case "cancel":
		targetStatus = "failed"

	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("unknown status action: %q (use pause, resume, or cancel)", newStatus))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var completedAt *string
	if targetStatus == "failed" || targetStatus == "completed" {
		completedAt = &now
	}

	var execErr error
	if completedAt != nil {
		execErr = s.db.Exec(ctx,
			`UPDATE sessions SET status = $1, heartbeat_at = $2, completed_at = $3 WHERE id = $4`,
			targetStatus, now, *completedAt, id)
	} else {
		execErr = s.db.Exec(ctx,
			`UPDATE sessions SET status = $1, heartbeat_at = $2 WHERE id = $3`,
			targetStatus, now, id)
	}

	if execErr != nil {
		slog.Error("api: failed to update session", "error", execErr)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update session")
		return
	}

	// Publish event
	s.events.PublishSessionUpdate(id, targetStatus, 0)

	// Return updated session
	row, err = s.db.QueryRow(ctx,
		`SELECT id, parent_id, agent_name, model_id, status, goal, context_budget,
		        tokens_used_in, tokens_used_out, iteration, project_id, heartbeat_at, created_at, completed_at
		 FROM sessions WHERE id = $1`, id)
	if err == nil {
		writeJSON(w, rowToSessionResponse(row))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"updated"}`))
	}
}

// ============================================================================
// DELETE /api/v1/sessions/{id} — Soft-delete session (admin only)
// ============================================================================

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request, id string) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "only admin keys may delete sessions")
		return
	}

	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	err := s.db.Exec(ctx,
		`UPDATE sessions SET status = 'failed', completed_at = $1, heartbeat_at = $1 WHERE id = $2`,
		now, id)
	if err != nil {
		slog.Error("api: failed to delete session", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}

// ============================================================================
// POST /api/v1/sessions/{id}/message — Send a message to an agent session
// ============================================================================

func (s *Server) handleSessionMessage(w http.ResponseWriter, r *http.Request, id string) {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)

	if scope == "session" && sessionID != id {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key can only message its own session")
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	if req.Content == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "content is required")
		return
	}

	msgType := req.Type
	if msgType == "" {
		msgType = "user_instruction"
	}

	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	// Check current session status, get current iteration
	row, err := s.db.QueryRow(ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	currentStatus := toString(row["status"])
	currentIteration := toInt64(row["iteration"])

	// Insert message into memory_events
	// For SQLite, we need to generate sequential IDs (memory_events uses BIGSERIAL in Postgres, but in SQLite we use INTEGER PK)
	err = s.db.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, $4)`,
		req.Content, id, currentIteration+1, now)
	if err != nil {
		slog.Error("api: failed to insert memory event", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}

	// If session is idle or booting, transition to thinking to wake it.
	// Booting sessions are newly created and haven't been processed yet —
	// receiving a message should wake them into the thinking state.
	if currentStatus == "idle" || currentStatus == "booting" {
		s.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1 WHERE id = $2`,
			now, id)
		s.events.PublishSessionUpdate(id, "thinking", currentIteration+1)
	} else if currentStatus == "paused" {
		// Message queues for next iteration, leave paused
	}

	writeJSON(w, map[string]any{
		"status":  "message_received",
		"session": id,
	})
}

// ============================================================================
// Helpers
// ============================================================================

func rowToSessionResponse(row db.Row) SessionResponse {
	resp := SessionResponse{
		ID:            toString(row["id"]),
		AgentName:     toString(row["agent_name"]),
		ModelID:       toString(row["model_id"]),
		Status:        toString(row["status"]),
		ContextBudget: toInt(row["context_budget"]),
		TokensUsedIn:  toInt64(row["tokens_used_in"]),
		TokensUsedOut: toInt64(row["tokens_used_out"]),
		Iteration:     toInt64(row["iteration"]),
		HeartbeatAt:   toString(row["heartbeat_at"]),
		CreatedAt:     toString(row["created_at"]),
	}

	if pid := row["parent_id"]; pid != nil {
		s := toString(pid)
		resp.ParentID = &s
	}
	if goal := row["goal"]; goal != nil {
		s := toString(goal)
		resp.Goal = &s
	}
	if cat := row["completed_at"]; cat != nil {
		s := toString(cat)
		resp.CompletedAt = &s
	}
	if projID := row["project_id"]; projID != nil {
		s := toString(projID)
		resp.ProjectID = &s
	}

	return resp
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version 4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "cs_sk_" + hex.EncodeToString(b)
}
