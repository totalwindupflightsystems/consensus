// Package session: tests for complete_task and cancel_task (AC-HARDEN-06, AC-HARDEN-07).
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-4 test=internal/session/complete_test.go
package session

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// setupTMTestDB creates a test database with tasks, sessions, and memory_events tables.
func setupTMTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()
	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Create required tables
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			agent_name TEXT NOT NULL,
			model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'idle',
			goal TEXT,
			iteration INTEGER DEFAULT 0,
			project_id TEXT,
			heartbeat_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			locked_by_agent TEXT,
			completed_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			iteration_created INTEGER NOT NULL DEFAULT 0
		)`,
	} {
		if err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	cleanup := func() {
		database.Close()
		os.Remove(t.Name())
	}
	return database, cleanup
}

func seedTask(database db.DB, id, sessionID, status string) {
	ctx := context.Background()
	_ = database.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status) VALUES ($1, 'test-agent', 'gpt-4o', 'thinking')`,
		sessionID,
	)
	_ = database.Exec(ctx,
		`INSERT INTO tasks (id, session_id, title, status) VALUES ($1, $2, $3, $4)`,
		id, sessionID, "test task", status,
	)
}

// ============================================================================
// AC-HARDEN-06: complete_task()
// ============================================================================

func TestCompleteTask_Success(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-001"
	sessionID := "session-001"

	seedTask(database, taskID, sessionID, "in_progress")

	err := mgr.CompleteTask(ctx, taskID, sessionID, "Task finished successfully")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Verify task is now published
	rows, err := database.Query(ctx, `SELECT status FROM tasks WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("task not found")
	}
	status := rows[0]["status"].(string)
	if status != "published" {
		t.Errorf("status = %q, want published", status)
	}
}

func TestCompleteTask_RecordsMemoryEvent(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-002"
	sessionID := "session-002"

	seedTask(database, taskID, sessionID, "in_progress")

	err := mgr.CompleteTask(ctx, taskID, sessionID, "Summary of completed work")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Verify memory event was recorded
	rows, err := database.Query(ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND type = 'system'`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("query memory: %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected memory event, got none")
	}
}

func TestCompleteTask_NotFound(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()

	err := mgr.CompleteTask(ctx, "nonexistent", "any-session", "result")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestCompleteTask_AlreadyPublished(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-003"
	sessionID := "session-003"

	seedTask(database, taskID, sessionID, "published")

	err := mgr.CompleteTask(ctx, taskID, sessionID, "result")
	if err == nil {
		t.Error("expected error for already-published task")
	}
}

func TestCompleteTask_EmptyIDs(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()

	if err := mgr.CompleteTask(ctx, "", "sid", "r"); err == nil {
		t.Error("expected error for empty task_id")
	}
	if err := mgr.CompleteTask(ctx, "tid", "", "r"); err == nil {
		t.Error("expected error for empty session_id")
	}
}

// ============================================================================
// AC-HARDEN-07: cancel_task()
// ============================================================================

func TestCancelTask_Success(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-010"
	sessionID := "session-010"

	seedTask(database, taskID, sessionID, "in_progress")

	err := mgr.CancelTask(ctx, taskID, sessionID, "User cancelled")
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	// Verify status
	rows, err := database.Query(ctx, `SELECT status FROM tasks WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	status := rows[0]["status"].(string)
	if status != "cancelled" {
		t.Errorf("status = %q, want cancelled", status)
	}
}

func TestCancelTask_RecordsMemoryEvent(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-011"
	sessionID := "session-011"

	seedTask(database, taskID, sessionID, "in_progress")

	err := mgr.CancelTask(ctx, taskID, sessionID, "Budget exceeded")
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	// Verify memory event
	rows, err := database.Query(ctx,
		`SELECT content FROM memory_events WHERE session_id = $1 AND content LIKE '%CANCELLED%'`,
		sessionID,
	)
	if err != nil {
		t.Fatalf("query memory: %v", err)
	}
	if len(rows) == 0 {
		t.Error("expected CANCELLED memory event")
	}
}

func TestCancelTask_CannotCancelPublished(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()
	taskID := "task-012"
	sessionID := "session-012"

	seedTask(database, taskID, sessionID, "published")

	err := mgr.CancelTask(ctx, taskID, sessionID, "reason")
	if err == nil {
		t.Error("expected error cancelling published task")
	}
}

func TestCancelTask_NotFound(t *testing.T) {
	database, cleanup := setupTMTestDB(t)
	defer cleanup()

	mgr := NewTaskManager(database)
	ctx := context.Background()

	err := mgr.CancelTask(ctx, "nonexistent", "any-session", "reason")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}
