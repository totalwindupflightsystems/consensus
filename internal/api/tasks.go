// Package api: task endpoint handlers (SPEC-015 §3.3).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-3/step-2-3-1 impl=internal/api/tasks.go
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// POST /api/v1/sessions/{id}/tasks — Create a task for a session
// ============================================================================

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}

	if req.Title == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "title is required")
		return
	}
	if req.Priority == 0 {
		req.Priority = 5
	}
	if req.Priority < 1 || req.Priority > 10 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "priority must be between 1 and 10")
		return
	}

	taskID := newUUID()
	now := time.Now().UTC().Format(time.RFC3339)

	ctx := r.Context()

	// Marshal prerequisite_ids to JSON array string (SQLite stores as TEXT)
	prereqJSON := "[]"
	if len(req.PrerequisiteIDs) > 0 {
		data, err := json.Marshal(req.PrerequisiteIDs)
		if err == nil {
			prereqJSON = string(data)
		}
	}

	err := s.db.Exec(ctx,
		`INSERT INTO tasks (id, session_id, title, description, status, priority, prerequisite_ids, created_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7)`,
		taskID, sessionID, req.Title, req.Description, req.Priority, prereqJSON, now,
	)
	if err != nil {
		slog.Error("api: failed to create task", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create task: "+err.Error())
		return
	}

	// Get the created task
	row, err := s.db.QueryRow(ctx,
		`SELECT id, session_id, parent_task_id, title, description, status, priority,
		        locked_by_agent, prerequisite_ids, result_memory_id,
		        created_at, claimed_at, completed_at
		 FROM tasks WHERE id = $1`, taskID)
	if err != nil || row == nil {
		writeJSON(w, map[string]any{"id": taskID, "status": "pending"})
		return
	}

	writeJSON(w, rowToTaskResponse(row))
}

// ============================================================================
// GET /api/v1/sessions/{id}/tasks — List tasks for a session
// ============================================================================

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.checkSessionAccess(w, r, sessionID) {
		return
	}

	ctx := r.Context()

	// Optional status filter
	statusFilter := r.URL.Query().Get("status")
	limit := 100

	var rows []db.Row
	var err error

	if statusFilter != "" {
		rows, err = s.db.Query(ctx,
			`SELECT id, session_id, parent_task_id, title, description, status, priority,
			        locked_by_agent, prerequisite_ids, result_memory_id,
			        created_at, claimed_at, completed_at
			 FROM tasks
			 WHERE session_id = $1 AND status = $2
			 ORDER BY priority DESC, created_at ASC
			 LIMIT $3`,
			sessionID, statusFilter, limit,
		)
	} else {
		rows, err = s.db.Query(ctx,
			`SELECT id, session_id, parent_task_id, title, description, status, priority,
			        locked_by_agent, prerequisite_ids, result_memory_id,
			        created_at, claimed_at, completed_at
			 FROM tasks
			 WHERE session_id = $1
			 ORDER BY priority DESC, created_at ASC
			 LIMIT $2`,
			sessionID, limit,
		)
	}

	if err != nil {
		slog.Error("api: failed to list tasks", "error", err, "session", sessionID)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list tasks")
		return
	}

	results := make([]TaskResponse, 0, len(rows))
	for _, row := range rows {
		results = append(results, rowToTaskResponse(row))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// PATCH /api/v1/tasks/{tid} — Update task status
// ============================================================================

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx := r.Context()

	// Look up the task's session to enforce scope
	row, err := s.db.QueryRow(ctx,
		`SELECT id, session_id, parent_task_id, title, description, status, priority,
		        locked_by_agent, prerequisite_ids, result_memory_id,
		        created_at, claimed_at, completed_at
		 FROM tasks WHERE id = $1`, taskID)
	if err != nil || row == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}

	// Enforce session-scoped access
	if !s.checkSessionAccess(w, r, toString(row["session_id"])) {
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body")
		return
	}

	if req.Status == nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "status field is required")
		return
	}

	newStatus := *req.Status
	currentStatus := toString(row["status"])
	now := time.Now().UTC().Format(time.RFC3339)

	// Validate state transitions
	validTransitions := map[string][]string{
		"claimed":     {"pending"},
		"in_progress": {"pending", "claimed"},
		"reviewed":    {"in_progress"},
		"published":   {"reviewed"},
		"failed":      {"pending", "claimed", "in_progress"},
		"cancelled":   {"pending", "claimed", "in_progress"},
	}

	allowed, ok := validTransitions[newStatus]
	if !ok {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
			fmt.Sprintf("unknown status %q", newStatus))
		return
	}

	valid := false
	for _, a := range allowed {
		if a == currentStatus {
			valid = true
			break
		}
	}
	if !valid {
		writeError(w, r, http.StatusConflict, "CONFLICT",
			fmt.Sprintf("cannot transition task from %q to %q", currentStatus, newStatus))
		return
	}

	// Apply the update
	var execErr error
	switch newStatus {
	case "claimed":
		claimedAt := now
		execErr = s.db.Exec(ctx,
			`UPDATE tasks SET status = $1, claimed_at = $2 WHERE id = $3`,
			newStatus, claimedAt, taskID)
	case "published", "failed", "cancelled":
		completedAt := now
		execErr = s.db.Exec(ctx,
			`UPDATE tasks SET status = $1, completed_at = $2 WHERE id = $3`,
			newStatus, completedAt, taskID)
	default:
		execErr = s.db.Exec(ctx,
			`UPDATE tasks SET status = $1 WHERE id = $2`,
			newStatus, taskID)
	}

	if execErr != nil {
		slog.Error("api: failed to update task", "error", execErr)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update task")
		return
	}

	// Return updated task
	row, err = s.db.QueryRow(ctx,
		`SELECT id, session_id, parent_task_id, title, description, status, priority,
		        locked_by_agent, prerequisite_ids, result_memory_id,
		        created_at, claimed_at, completed_at
		 FROM tasks WHERE id = $1`, taskID)
	if err == nil && row != nil {
		writeJSON(w, rowToTaskResponse(row))
	} else {
		writeJSON(w, map[string]any{"id": taskID, "status": newStatus})
	}
}

// ============================================================================
// POST /api/v1/tasks/{tid}/claim — Claim a task
// ============================================================================

func (s *Server) handleClaimTask(w http.ResponseWriter, r *http.Request, taskID string) {
	ctx := r.Context()
	authSessionID := GetAuthSessionID(r)

	// Look up the task
	row, err := s.db.QueryRow(ctx,
		`SELECT id, session_id, status, priority, locked_by_agent
		 FROM tasks WHERE id = $1`, taskID)
	if err != nil || row == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "task not found")
		return
	}

	taskStatus := toString(row["status"])
	taskLockedBy := row["locked_by_agent"]

	// Only pending tasks can be claimed
	if taskStatus != "pending" {
		writeError(w, r, http.StatusConflict, "CONFLICT",
			fmt.Sprintf("task status is %q, cannot be claimed", taskStatus))
		return
	}

	// Cannot claim a task that is already locked by another agent
	if taskLockedBy != nil && toString(taskLockedBy) != "" && toString(taskLockedBy) != authSessionID {
		writeError(w, r, http.StatusConflict, "CONFLICT",
			"task is already locked by another agent")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Claim the task: set status to claimed and lock to the requesting session
	execErr := s.db.Exec(ctx,
		`UPDATE tasks SET status = 'claimed', locked_by_agent = $1, claimed_at = $2 WHERE id = $3`,
		authSessionID, now, taskID,
	)
	if execErr != nil {
		slog.Error("api: failed to claim task", "error", execErr)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to claim task")
		return
	}

	// Return updated task
	row, err = s.db.QueryRow(ctx,
		`SELECT id, session_id, parent_task_id, title, description, status, priority,
		        locked_by_agent, prerequisite_ids, result_memory_id,
		        created_at, claimed_at, completed_at
		 FROM tasks WHERE id = $1`, taskID)
	if err == nil && row != nil {
		writeJSON(w, rowToTaskResponse(row))
	} else {
		writeJSON(w, map[string]any{"id": taskID, "status": "claimed", "locked_by": authSessionID})
	}
}

// ============================================================================
// Row Mapper
// ============================================================================

func rowToTaskResponse(row db.Row) TaskResponse {
	resp := TaskResponse{
		ID:              toString(row["id"]),
		SessionID:       toString(row["session_id"]),
		Title:           toString(row["title"]),
		Status:          toString(row["status"]),
		Priority:        toInt(row["priority"]),
		PrerequisiteIDs: parseStringArray(toString(row["prerequisite_ids"])),
		CreatedAt:       toString(row["created_at"]),
	}

	if pid := row["parent_task_id"]; pid != nil {
		s := toString(pid)
		resp.ParentTaskID = &s
	}
	if desc := row["description"]; desc != nil {
		s := toString(desc)
		resp.Description = &s
	}
	if lba := row["locked_by_agent"]; lba != nil {
		s := toString(lba)
		if s != "" {
			resp.LockedByAgent = &s
		}
	}
	if rmid := row["result_memory_id"]; rmid != nil {
		n := toInt64(rmid)
		resp.ResultMemoryID = &n
	}
	if ca := row["claimed_at"]; ca != nil {
		s := toString(ca)
		resp.ClaimedAt = &s
	}
	if coa := row["completed_at"]; coa != nil {
		s := toString(coa)
		resp.CompletedAt = &s
	}

	return resp
}
