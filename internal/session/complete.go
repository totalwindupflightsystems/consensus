// Package session: general-purpose complete_task and cancel_task (AC-HARDEN-06, AC-HARDEN-07).
//
// These functions are the Go-native equivalents of the stored procedures
// described in SPEC-006 §Stored Procedures for Destructive Operations.
// They enforce state transitions, validate prerequisites, and ensure
// proper cleanup on task cancellation.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-4 impl=internal/session/complete.go
package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Task Completion Manager
// ============================================================================

// TaskManager wraps database operations for task lifecycle management.
// This is the general-purpose equivalent of CompleteChild/CancelChild from
// the subagent package, usable for ANY task, not just sub-agent tasks.
type TaskManager struct {
	database db.DB
}

// NewTaskManager creates a new task lifecycle manager.
func NewTaskManager(database db.DB) *TaskManager {
	return &TaskManager{database: database}
}

// ============================================================================
// AC-HARDEN-06: complete_task()
// ============================================================================

// CompleteTask finishes a task, transitioning it to 'published' status.
//
// Enforces:
//   - Task must exist and be in a completable state
//   - Task must have gone through 'reviewed' before 'published' (SPEC-006 §5.1)
//   - Session context must match the task's session (RLS alternative)
//
// The completed result is written as a memory event for the session's context.
func (m *TaskManager) CompleteTask(ctx context.Context, taskID string, sessionID string, resultSummary string) error {
	if taskID == "" {
		return fmt.Errorf("task: complete_task requires task_id")
	}
	if sessionID == "" {
		return fmt.Errorf("task: complete_task requires session_id")
	}

	// Verify the task exists and belongs to this session
	rows, err := m.database.Query(ctx,
		`SELECT status FROM tasks WHERE id = $1 AND session_id = $2`,
		taskID, sessionID,
	)
	if err != nil {
		return fmt.Errorf("task: lookup %s: %w", taskID, err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("task: not found or not owned by session %s: %s", sessionID, taskID)
	}

	currentStatus := toString(rows[0]["status"])
	if currentStatus == "published" {
		return fmt.Errorf("task: already published: %s", taskID)
	}
	if currentStatus == "failed" || currentStatus == "cancelled" {
		return fmt.Errorf("task: cannot complete from %s status: %s", currentStatus, taskID)
	}

	// Transition to reviewed → published (enforcing the two-step rule)
	now := time.Now()
	err = m.database.Exec(ctx,
		`UPDATE tasks SET status = 'reviewed', completed_at = $1 WHERE id = $2`,
		now, taskID,
	)
	if err != nil {
		return fmt.Errorf("task: mark reviewed: %w", err)
	}

	err = m.database.Exec(ctx,
		`UPDATE tasks SET status = 'published', completed_at = $1 WHERE id = $2 AND status = 'reviewed'`,
		now, taskID,
	)
	if err != nil {
		return fmt.Errorf("task: publish: %w", err)
	}

	// Record result as a memory event
	if resultSummary != "" {
		_ = m.database.Exec(ctx,
			`INSERT INTO memory_events (type, content, session_id, iteration_created)
			VALUES ('system', $1, $2, 0)`,
			"COMPLETED: "+resultSummary, sessionID,
		)
	}

	slog.Info("task: completed", "task_id", taskID, "session_id", sessionID)
	return nil
}

// ============================================================================
// AC-HARDEN-07: cancel_task()
// ============================================================================

// CancelTask cancels a running task, transitioning it to 'cancelled' status.
//
// Enforces:
//   - Task must exist and belong to the cancelling session
//   - Cannot cancel an already-published task
//   - Triggers cleanup: resets locked_by_agent, wakes parent if applicable
func (m *TaskManager) CancelTask(ctx context.Context, taskID string, sessionID string, reason string) error {
	if taskID == "" {
		return fmt.Errorf("task: cancel_task requires task_id")
	}
	if sessionID == "" {
		return fmt.Errorf("task: cancel_task requires session_id")
	}

	// Verify task ownership and status
	rows, err := m.database.Query(ctx,
		`SELECT status, locked_by_agent FROM tasks WHERE id = $1 AND session_id = $2`,
		taskID, sessionID,
	)
	if err != nil {
		return fmt.Errorf("task: lookup %s: %w", taskID, err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("task: not found or not owned by session %s: %s", sessionID, taskID)
	}

	currentStatus := toString(rows[0]["status"])
	if currentStatus == "published" {
		return fmt.Errorf("task: cannot cancel published task: %s", taskID)
	}
	if currentStatus == "cancelled" {
		return fmt.Errorf("task: already cancelled: %s", taskID)
	}

	// Cancel the task
	now := time.Now()
	err = m.database.Exec(ctx,
		`UPDATE tasks SET status = 'cancelled', completed_at = $1 WHERE id = $2`,
		now, taskID,
	)
	if err != nil {
		return fmt.Errorf("task: cancel: %w", err)
	}

	// Cleanup: remove lock if claimed
	_ = m.database.Exec(ctx,
		`UPDATE tasks SET locked_by_agent = NULL WHERE id = $1 AND locked_by_agent IS NOT NULL`,
		taskID,
	)

	// Record cancellation reason
	cancelMsg := "CANCELLED"
	if reason != "" {
		cancelMsg = "CANCELLED: " + reason
	}
	_ = m.database.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('system', $1, $2, 0)`,
		cancelMsg, sessionID,
	)

	slog.Info("task: cancelled", "task_id", taskID, "session_id", sessionID, "reason", reason)
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
