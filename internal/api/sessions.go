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

	// DF-CONSENSUS-50: fail fast when no LLM API key is configured. The
	// misconfiguration is already detected at startup (C-GAP-003 warning);
	// rejecting session creation here turns it into an actionable API error
	// instead of an opaque auth failure at the session's first LLM dispatch.
	// requireLLMKey is nil when unset (tests, shims) — feature off.
	if s.requireLLMKey != nil && !s.requireLLMKey() {
		writeError(w, r, http.StatusBadRequest, "MISSING_LLM_CONFIG",
			"no LLM API key configured: set the DEEPSEEK_API_KEY (or CONSENSUS_API_KEY) "+
				"environment variable, or llm.api_key in consensus.yaml, then retry")
		return
	}

	result, err := s.svc.Sessions.CreateSession(r.Context(), CreateSessionInput{
		AgentName:        req.AgentName,
		Goal:             req.Goal,
		ModelID:          req.ModelID,
		ContextBudget:    req.ContextBudget,
		BudgetLimitCents: req.BudgetLimitCents,
		ProjectID:        req.ProjectID,
	})
	if err != nil {
		slog.Error("api: failed to create session", "error", err)
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(CreateSessionResponse{
		ID:               result.SessionID,
		Status:           result.Status,
		APIKey:           result.APIKey,
		ModelID:          result.ModelID,
		ProjectID:        result.ProjectID,
		BudgetLimitCents: result.BudgetLimitCents,
		CreatedAt:        time.Now().UTC(),
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

	// Populate last assistant message from memory_events
	row, lmErr := s.db.QueryRow(r.Context(),
		`SELECT content FROM memory_events
		 WHERE session_id = $1 AND type = 'text_block'
		 ORDER BY created_at DESC LIMIT 1`, id)
	if lmErr == nil && row != nil {
		if content := toString(row["content"]); content != "" {
			resp.LastMessage = &content
		}
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
		"paused": {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"},
		"resume": {"paused", "failed"},
		"cancel": {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub", "paused"},
		"idle":   {"thinking", "planning"},                                                  // sent by harness
		"failed": {"idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"}, // sent by harness
	}

	ctx := r.Context()

	// Get current status. A soft-deleted (tombstoned) session is API-gone:
	// it must not be pause/resume/candidate, so 404 like a missing id
	// (SPEC-015 §3.1).
	row, err := s.db.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1 AND deleted_at IS NULL`, id)
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
		allowed := validTransitions["resume"]
		found := false
		for _, candidate := range allowed {
			if candidate == currentStatus {
				found = true
				break
			}
		}
		if !found {
			writeError(w, r, http.StatusConflict, "CONFLICT",
				fmt.Sprintf("can only resume paused or failed sessions, current status is %q", currentStatus))
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
			`UPDATE sessions SET status = $1, heartbeat_at = $2, completed_at = NULL WHERE id = $3`,
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

	// Soft delete: set the deleted_at tombstone (SPEC-003 §2.1, SPEC-015 §3.1).
	// The row is never removed. The deleted_at IS NULL predicate makes this
	// idempotent — a second DELETE succeeds without changing the original
	// tombstone timestamp. A nonexistent id updates nothing and still returns
	// 200 (the route's existing semantics for missing ids).
	err := s.db.Exec(ctx,
		`UPDATE sessions SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`,
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

	if r.Header.Get("Idempotency-Key") != "" {
		s.handleIdempotentSessionMessage(w, r, id, req)
		return
	}

	ctx := r.Context()
	now := time.Now().UTC().Format(time.RFC3339)

	// Check current session status, get current iteration. A soft-deleted
	// (tombstoned) session must not be resolvable here: 410 GONE, and no
	// memory_events row is written (SPEC-015 §3.1). A session that never
	// existed keeps the route's 404 semantics.
	existsRow, err := s.db.QueryRow(ctx, `SELECT deleted_at FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	if existsRow["deleted_at"] != nil {
		writeError(w, r, http.StatusGone, "GONE", "session has been deleted")
		return
	}

	row, err := s.db.QueryRow(ctx, `SELECT status, iteration FROM sessions WHERE id = $1 AND deleted_at IS NULL`, id)
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

	// Idle and booting sessions wake normally. A new message also explicitly
	// recovers a failed session: clear its terminal timestamp and send it through
	// the same thinking -> planning claim as any other conversational turn.
	if currentStatus == "idle" || currentStatus == "booting" || currentStatus == "failed" {
		if err := s.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1, completed_at = NULL WHERE id = $2`,
			now, id); err != nil {
			slog.Error("api: failed to wake session for message", "session_id", id, "error", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to resume session")
			return
		}
		s.events.PublishSessionUpdate(id, "thinking", currentIteration+1)
		// PERF-CONSENSUS-11 fire-on-message wake: signal the harness to
		// dispatch this session now instead of waiting for the next
		// heartbeat tick. The hook is non-blocking on the harness side and
		// a no-op when unconfigured (nil).
		if s.wake != nil {
			s.wake(id)
		}
	} else if currentStatus == "paused" {
		// Message queues for next iteration, leave paused
	}

	writeJSON(w, map[string]any{
		"status":  "message_received",
		"session": id,
	})
}

func (s *Server) handleIdempotentSessionMessage(w http.ResponseWriter, r *http.Request, id string, req SendMessageRequest) {
	ctx := r.Context()
	key := r.Header.Get("Idempotency-Key")

	// Fast replay path. The durable row is the source of truth, so this also
	// serves retries after a process restart without touching the message ledger.
	if row, err := s.db.QueryRow(ctx,
		`SELECT response_message_id FROM idempotency_keys WHERE session_id = $1 AND key = $2`, id, key); err == nil {
		messageID := toInt64(row["response_message_id"])
		if messageID <= 0 {
			slog.Error("api: invalid idempotency mapping", "session_id", id)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to replay message")
			return
		}
		writeJSON(w, map[string]any{
			"status":     "message_received",
			"session":    id,
			"message_id": messageID,
		})
		return
	}

	// Preserve the route's existing missing/deleted-session errors before
	// reserving the key. The session is rechecked inside the transaction below.
	existsRow, err := s.db.QueryRow(ctx, `SELECT deleted_at FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	if existsRow["deleted_at"] != nil {
		writeError(w, r, http.StatusGone, "GONE", "session has been deleted")
		return
	}

	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		slog.Error("api: failed to begin idempotent message transaction", "session_id", id, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}
	defer func() {
		if tx.IsActive() {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.QueryRow(ctx,
		`INSERT INTO idempotency_keys (key, session_id, created_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (session_id, key) DO NOTHING
		 RETURNING key`, key, id, now); err != nil {
		// Another request may have committed this key while this request was
		// waiting on the unique index. Return that committed response.
		row, replayErr := tx.QueryRow(ctx,
			`SELECT response_message_id FROM idempotency_keys WHERE session_id = $1 AND key = $2`, id, key)
		if replayErr != nil {
			slog.Error("api: failed to reserve idempotency key", "session_id", id, "error", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
			return
		}
		messageID := toInt64(row["response_message_id"])
		if messageID <= 0 {
			slog.Error("api: invalid concurrent idempotency mapping", "session_id", id)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to replay message")
			return
		}
		_ = tx.Rollback()
		writeJSON(w, map[string]any{
			"status":     "message_received",
			"session":    id,
			"message_id": messageID,
		})
		return
	}

	row, err := tx.QueryRow(ctx,
		`SELECT status, iteration, deleted_at FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}
	if row["deleted_at"] != nil {
		writeError(w, r, http.StatusGone, "GONE", "session has been deleted")
		return
	}
	currentStatus := toString(row["status"])
	currentIteration := toInt64(row["iteration"])

	messageRow, err := tx.QueryRow(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, $4)
		 RETURNING id`, req.Content, id, currentIteration+1, now)
	if err != nil {
		slog.Error("api: failed to insert idempotent memory event", "session_id", id, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}
	messageID := toInt64(messageRow["id"])
	if messageID <= 0 {
		slog.Error("api: message insert returned invalid id", "session_id", id)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}

	wakeSession := currentStatus == "idle" || currentStatus == "booting" || currentStatus == "failed"
	if wakeSession {
		if err := tx.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1, completed_at = NULL WHERE id = $2`,
			now, id); err != nil {
			slog.Error("api: failed to wake session for idempotent message", "session_id", id, "error", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to resume session")
			return
		}
	}

	if err := tx.Exec(ctx,
		`UPDATE idempotency_keys SET response_message_id = $1 WHERE session_id = $2 AND key = $3`,
		messageID, id, key); err != nil {
		slog.Error("api: failed to complete idempotency mapping", "session_id", id, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("api: failed to commit idempotent message", "session_id", id, "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to store message")
		return
	}

	if wakeSession {
		s.events.PublishSessionUpdate(id, "thinking", currentIteration+1)
		if s.wake != nil {
			s.wake(id)
		}
	}
	writeJSON(w, map[string]any{
		"status":     "message_received",
		"session":    id,
		"message_id": messageID,
	})
}

// ============================================================================
// Helpers
// ============================================================================

func rowToSessionResponse(row db.Row) SessionResponse {
	resp := SessionResponse{
		ID:               toString(row["id"]),
		AgentName:        toString(row["agent_name"]),
		ModelID:          toString(row["model_id"]),
		Status:           toString(row["status"]),
		ContextBudget:    toInt(row["context_budget"]),
		BudgetLimitCents: toInt64(row["budget_limit_cents"]),
		TokensUsedIn:     toInt64(row["tokens_used_in"]),
		TokensUsedOut:    toInt64(row["tokens_used_out"]),
		Iteration:        toInt64(row["iteration"]),
		HeartbeatAt:      toString(row["heartbeat_at"]),
		CreatedAt:        toString(row["created_at"]),
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
