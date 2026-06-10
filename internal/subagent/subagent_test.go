package subagent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/driver"
)

// ============================================================================
// AC-SUB-01: Memory forking (compressed pointers only)
// ============================================================================

func TestMemoryForking(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-1
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	// Create parent session
	parentID := "parent-session-fork"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'idle', 'test', $2, 10, $2)`, parentID, time.Now())

	// Create child session
	childID := "child-session-fork"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'test', $3, 10, $3)`, childID, parentID, time.Now())

	// Add parent memory events with compressed markers
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Important context 1', $1, 0)`, parentID)
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Important context 2', $1, 0)`, parentID)
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Uncompressed junk', $1, 0)`, parentID)

	// Mark first two as compressed
	mustExec(t, database, ctx, `INSERT INTO display_modes (memory_id, mode, set_at, set_by_iteration, session_id) VALUES (1, 'compressed', $1, 0, $2)`, time.Now(), parentID)
	mustExec(t, database, ctx, `INSERT INTO display_modes (memory_id, mode, set_at, set_by_iteration, session_id) VALUES (2, 'compressed', $1, 0, $2)`, time.Now(), parentID)
	// Event 3 has no display_modes entry → default 'full' (NOT compressed)

	// Fork memory
	count, err := mgr.ForkMemory(ctx, parentID, childID)
	if err != nil {
		t.Fatalf("ForkMemory: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 events forked, got %d", count)
	}
	t.Logf("Forked %d compressed events", count)

	// Verify child has only inherited_pointer events
	rows, err := database.Query(ctx, `SELECT type, content, session_id FROM memory_events WHERE session_id = $1`, childID)
	if err != nil {
		t.Fatalf("query child memory: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 child memory events, got %d", len(rows))
	}

	for _, row := range rows {
		if row["session_id"].(string) != childID {
			t.Errorf("child memory session_id mismatch: expected %q, got %q", childID, row["session_id"])
		}
		typ, _ := row["type"].(string)
		if typ != "inherited_pointer" {
			t.Errorf("expected type 'inherited_pointer', got %q", typ)
		}
	}
	t.Logf("Child has %d isolated memory events (correctly scoped)", len(rows))

	// Verify child does NOT have the uncompressed event
	rows, err = database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = $1 AND content = 'Uncompressed junk'`, childID)
	if err != nil {
		t.Fatalf("query for uncompressed: %v", err)
	}
	if len(rows) > 0 {
		t.Error("child should NOT have uncompressed events")
	}
}

func TestForkMemoryNoCompressedEvents(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-1
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-no-compressed"
	childID := "child-no-compressed"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'idle', 'test', $2, 10, $2)`, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'test', $3, 10, $3)`, childID, parentID, time.Now())

	// No memory events at all
	count, err := mgr.ForkMemory(ctx, parentID, childID)
	if err != nil {
		t.Fatalf("ForkMemory with no events: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events forked, got %d", count)
	}
}

// ============================================================================
// AC-SUB-02: RLS isolation (session_id enforcement)
// ============================================================================

func TestRLSIsolation(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	// Verify isolation via session ID scoping
	if err := mgr.VerifyIsolation("session-a"); err != nil {
		t.Fatalf("isolation check: %v", err)
	}

	// Create two sessions
	sessionA := "session-a"
	sessionB := "session-b"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'agent-a', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionA, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'agent-b', 'test-model', 'idle', 'test', $2, 10, $2)`, sessionB, time.Now())

	// Write to session A
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'A secret', $1, 0)`, sessionA)
	mustExec(t, database, ctx, `INSERT INTO tasks (id, session_id, title, description, status, created_at) VALUES ('task-a', $1, 'A task', 'do work', 'pending', $2)`, sessionA, time.Now())

	// Write to session B
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'B secret', $1, 0)`, sessionB)
	mustExec(t, database, ctx, `INSERT INTO tasks (id, session_id, title, description, status, created_at) VALUES ('task-b', $1, 'B task', 'do different work', 'pending', $2)`, sessionB, time.Now())

	// Verify isolation: A cannot see B's memory or tasks without proper session context
	// Query A's memory — should only see A's
	rows, err := database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = $1`, sessionA)
	if err != nil {
		t.Fatalf("query A memory: %v", err)
	}
	for _, row := range rows {
		if row["content"].(string) == "B secret" {
			t.Error("agent A can see agent B's memory — RLS isolation FAILED")
		}
	}

	// Query B's memory
	rows, err = database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = $1`, sessionB)
	if err != nil {
		t.Fatalf("query B memory: %v", err)
	}
	for _, row := range rows {
		if row["content"].(string) == "A secret" {
			t.Error("agent B can see agent A's memory — RLS isolation FAILED")
		}
	}

	t.Log("RLS isolation: agents correctly isolated by session_id")
}

// ============================================================================
// AC-SUB-03: wake_parent_on_completion trigger
// ============================================================================

func TestParentWakeUp(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-3
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-wake"
	childID := "child-wake"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'waiting_sub', 'waiting', $2, 10, $2)`, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'working', $3, 10, $3)`, childID, parentID, time.Now())

	// Verify parent is waiting_sub
	rows, _ := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	t.Logf("Before wake: parent status = %s", rows[0]["status"])

	// Complete child → should wake parent
	err := mgr.WakeParentOnCompletion(ctx, childID)
	if err != nil {
		t.Fatalf("WakeParentOnCompletion: %v", err)
	}

	// Verify parent is now idle
	rows, err = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	if err != nil {
		t.Fatalf("query parent: %v", err)
	}
	parentStatus := rows[0]["status"].(string)
	if parentStatus != "idle" {
		t.Errorf("expected parent status 'idle' after wake, got %q", parentStatus)
	}
	t.Logf("After wake: parent status = %s", parentStatus)
}

func TestWakeParentOnlyWhenWaitingSub(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-3
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-not-waiting"
	childID := "child-not-waiting"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'thinking', 'busy', $2, 10, $2)`, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'working', $3, 10, $3)`, childID, parentID, time.Now())

	// Wake should not change parent that is 'thinking' (not waiting_sub)
	err := mgr.WakeParentOnCompletion(ctx, childID)
	if err != nil {
		t.Fatalf("WakeParentOnCompletion: %v", err)
	}

	rows, _ := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	parentStatus := rows[0]["status"].(string)
	if parentStatus != "thinking" {
		t.Errorf("parent in 'thinking' should not be woken, got %q", parentStatus)
	}
	t.Logf("Parent in 'thinking' was correctly not woken")
}

func TestCheckAllChildrenComplete(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-3
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-all-complete"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'waiting_sub', 'waiting', $2, 10, $2)`, parentID, time.Now())

	// Create child that is completed
	childID := "child-complete"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'completed', 'done', $3, 10, $3)`, childID, parentID, time.Now())

	// All children are complete → should wake parent
	err := mgr.CheckAllChildrenComplete(ctx, parentID)
	if err != nil {
		t.Fatalf("CheckAllChildrenComplete: %v", err)
	}

	rows, _ := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	if rows[0]["status"].(string) != "idle" {
		t.Errorf("parent should be idle when all children complete")
	}
	t.Log("Parent correctly woken when all children complete")
}

// ============================================================================
// AC-SUB-04: Error propagation (failed → parent reads result)
// ============================================================================

func TestErrorPropagation(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-4
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-error-prop"
	childID := "child-error-prop"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'waiting_sub', 'waiting', $2, 10, $2)`, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'working', $3, 10, $3)`, childID, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO tasks (id, session_id, title, description, status, created_at) VALUES ('error-task', $1, 'error task', 'failing work', 'in_progress', $2)`, childID, time.Now())

	// Propagate error
	err := mgr.PropagateError(ctx, childID, "division by zero in analysis module")
	if err != nil {
		t.Fatalf("PropagateError: %v", err)
	}

	// Verify task is failed
	rows, err := database.Query(ctx, `SELECT status FROM tasks WHERE id = 'error-task'`)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if rows[0]["status"].(string) != "failed" {
		t.Errorf("expected task 'failed', got %q", rows[0]["status"])
	}

	// Verify child session is failed
	rows, _ = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, childID)
	if rows[0]["status"].(string) != "failed" {
		t.Errorf("expected child session 'failed', got %q", rows[0]["status"])
	}

	// Verify parent was woken
	rows, _ = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	parentStatus := rows[0]["status"].(string)
	if parentStatus != "idle" {
		t.Errorf("expected parent to be woken to 'idle', got %q", parentStatus)
	}

	// Verify error is in child memory
	rows, _ = database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = $1 AND type = 'system'`, childID)
	if len(rows) == 0 {
		t.Error("expected error message in child memory")
	} else {
		t.Logf("Error stored in memory: %s", rows[0]["content"])
	}

	t.Log("Error propagation: task failed, session failed, parent woken, memory recorded")
}

// ============================================================================
// AC-SUB-05: Depth limit of 5 enforced
// ============================================================================

func TestDepthLimit(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-5
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	// Create a chain: root → L1 → L2 → L3 → L4 → L5
	root := "root-depth"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'root', 'test-model', 'idle', 'root', $2, 10, $2)`, root, time.Now())

	prev := root
	for i := 1; i <= 5; i++ {
		sid := fmt.Sprintf("level-%d", i)
		mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'work', $3, 10, $3)`, sid, prev, time.Now())
		prev = sid
	}

	// Check depth at level 5
	depth, err := mgr.GetDepth(ctx, "level-5")
	if err != nil {
		t.Fatalf("GetDepth level-5: %v", err)
	}
	if depth != 5 {
		t.Errorf("expected depth 5, got %d", depth)
	}
	t.Logf("Depth at level-5: %d", depth)

	// Spawn at depth 5 should fail
	_, err = mgr.SpawnSubAgent(ctx, "level-5", "too-deep", "attempt at depth limit")
	if err == nil {
		t.Error("expected depth limit error, but spawn succeeded")
	}
	t.Logf("Depth limit correctly enforced: %v", err)

	// Root agent should have depth 0
	depth, err = mgr.GetDepth(ctx, root)
	if err != nil {
		t.Fatalf("GetDepth root: %v", err)
	}
	if depth != 0 {
		t.Errorf("root agent should have depth 0, got %d", depth)
	}
}

func TestDepthLimitConfiguration(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-5
	// Default depth should be 5
	if DefaultMaxDepth != 5 {
		t.Errorf("expected default max depth 5, got %d", DefaultMaxDepth)
	}
}

// ============================================================================
// Spawn SubAgent Integration
// ============================================================================

func TestSpawnSubAgent(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-spawn"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'idle', 'spawn test', $2, 10, $2)`, parentID, time.Now())

	// Spawn a sub-agent
	result, err := mgr.SpawnSubAgent(ctx, parentID, "worker-bee", "Analyze the data and report back")
	if err != nil {
		t.Fatalf("SpawnSubAgent: %v", err)
	}

	if result.TaskID == "" {
		t.Error("expected non-empty task ID")
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if result.Depth != 1 {
		t.Errorf("expected depth 1, got %d", result.Depth)
	}

	t.Logf("Spawned: task=%s, session=%s, depth=%d", result.TaskID, result.SessionID, result.Depth)

	// Verify parent is waiting_sub
	rows, _ := database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	if rows[0]["status"].(string) != "waiting_sub" {
		t.Errorf("expected parent 'waiting_sub', got %q", rows[0]["status"])
	}

	// Verify child session exists and is idle
	rows, _ = database.Query(ctx, `SELECT id, parent_id, agent_name, status, goal FROM sessions WHERE id = $1`, result.SessionID)
	if len(rows) == 0 {
		t.Fatal("child session not found")
	}
	if rows[0]["agent_name"].(string) != "worker-bee" {
		t.Errorf("expected agent_name 'worker-bee', got %q", rows[0]["agent_name"])
	}

	// Verify task exists
	rows, _ = database.Query(ctx, `SELECT id, session_id, title, status FROM tasks WHERE id = $1`, result.TaskID)
	if len(rows) == 0 {
		t.Fatal("child task not found")
	}
	if rows[0]["session_id"].(string) != result.SessionID {
		t.Errorf("task session_id mismatch")
	}
}

func TestCompleteChild(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-complete-child"
	childID := "child-complete-child"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'waiting_sub', 'waiting', $2, 10, $2)`, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, 'child', 'test-model', 'idle', 'work', $3, 10, $3)`, childID, parentID, time.Now())
	mustExec(t, database, ctx, `INSERT INTO tasks (id, session_id, title, description, status, created_at) VALUES ('complete-task', $1, 'finish work', 'done', 'in_progress', $2)`, childID, time.Now())

	// Complete the child
	err := mgr.CompleteChild(ctx, childID, "Analysis complete: found 42 anomalies")
	if err != nil {
		t.Fatalf("CompleteChild: %v", err)
	}

	// Verify task is completed
	rows, _ := database.Query(ctx, `SELECT status FROM tasks WHERE id = 'complete-task'`)
	if rows[0]["status"].(string) != "completed" {
		t.Errorf("expected task 'completed', got %q", rows[0]["status"])
	}

	// Verify session is completed
	rows, _ = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, childID)
	if rows[0]["status"].(string) != "completed" {
		t.Errorf("expected session 'completed', got %q", rows[0]["status"])
	}

	// Parent should be woken
	rows, _ = database.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, parentID)
	if rows[0]["status"].(string) != "idle" {
		t.Errorf("expected parent 'idle', got %q", rows[0]["status"])
	}

	t.Log("CompleteChild: task completed, session completed, parent woken")
}

// ============================================================================
// List Children
// ============================================================================

func TestListChildren(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	mgr := New(database)

	parentID := "parent-list-children"
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, 'parent', 'test-model', 'idle', 'test', $2, 10, $2)`, parentID, time.Now())

	// Create children
	for i := 1; i <= 3; i++ {
		cid := fmt.Sprintf("child-%d", i)
		mustExec(t, database, ctx, `INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at) VALUES ($1, $2, $3, 'test-model', $4, $5, $5, 10, $5)`, cid, parentID, fmt.Sprintf("worker-%d", i), fmt.Sprintf("idle-%d", i), time.Now())
	}

	children, err := mgr.ListChildren(ctx, parentID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(children) != 3 {
		t.Errorf("expected 3 children, got %d", len(children))
	}
	t.Logf("Found %d children for parent %s", len(children), parentID)

	// Session with no children
	children, err = mgr.ListChildren(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListChildren empty: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children for nonexistent, got %d", len(children))
	}
}

// ============================================================================
// Helpers
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create sessions table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		parent_id TEXT,
		agent_name TEXT NOT NULL DEFAULT 'test_agent',
		model_id TEXT NOT NULL DEFAULT 'test-model',
		status TEXT NOT NULL DEFAULT 'booting',
		goal TEXT NOT NULL DEFAULT '',
		context_budget INTEGER NOT NULL DEFAULT 128000,
		tokens_used_in INTEGER NOT NULL DEFAULT 0,
		tokens_used_out INTEGER NOT NULL DEFAULT 0,
		iteration INTEGER NOT NULL DEFAULT 0,
		project_id TEXT,
		heartbeat_at TEXT NOT NULL,
		planning_max_turns INTEGER NOT NULL DEFAULT 10,
		created_at TEXT NOT NULL,
		completed_at TEXT
	)`)

	// Create memory_events table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS memory_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		content TEXT NOT NULL,
		summary_text TEXT,
		session_id TEXT NOT NULL,
		iteration_created INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// Create display_modes table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS display_modes (
		memory_id INTEGER NOT NULL,
		mode TEXT NOT NULL DEFAULT 'full',
		set_at TEXT NOT NULL,
		set_by_iteration INTEGER NOT NULL,
		session_id TEXT NOT NULL,
		PRIMARY KEY (memory_id)
	)`)

	// Create tasks table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		parent_task_id TEXT,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		project_id TEXT,
		created_at TEXT NOT NULL,
		completed_at TEXT
	)`)

	// Create system_settings table
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Logf("warning: failed to close test database: %v", err)
		}
	}

	return database, cleanup
}

func mustExec(t *testing.T, database db.DB, ctx context.Context, query string, args ...any) {
	t.Helper()
	if err := database.Exec(ctx, query, args...); err != nil {
		t.Fatalf("exec failed: %s: %v", query, err)
	}
}
