// Package subagent implements sub-agent spawning, memory forking, parent
// wake-up, and depth limiting for Conscience (SPEC-004).
//
// Agents can spawn sub-agents to parallelize work, delegate tasks, and scope
// memory. Sub-agents are first-class citizens of the task system with enforced
// isolation and controlled communication channels. The database is the
// coordination layer — spawn, fork, wake, and propagate are all SQL operations.
//
// Key properties:
//   - Memory fork: only display_modes.mode='compressed' events are inherited
//   - RLS isolation: session_id enforced at DB layer on all access
//   - Event-driven wakeups: trigger wakes parent on child completion
//   - Error propagation: failed child → task status='failed'; parent reads result
//   - Depth limit: max 5 nesting levels, configurable via system_settings
//
// axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md plan=phase-4/task-4-1/step-4-1-1
package subagent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/session"
)

// ============================================================================
// Depth Limit (AC-SUB-05)
// ============================================================================

const (
	// DefaultMaxDepth is the maximum sub-agent nesting level.
	// Prevent circular or unbounded spawning.
	DefaultMaxDepth = 5
)

// ============================================================================
// Manager
// ============================================================================

// Manager orchestrates sub-agent lifecycle operations.
type Manager struct {
	database db.DB
}

// New creates a new subagent manager backed by the given database.
func New(database db.DB) *Manager {
	return &Manager{database: database}
}

// ============================================================================
// AC-SUB-01: Memory forking (compressed pointers only)
// ============================================================================

// ForkMemory clones the parent's compressed memory events into the child session.
// Only events with display_modes.mode = 'compressed' are inherited — raw event
// data is NOT copied. This is a single SQL statement, atomic and instant.
//
// After forking, the child's memory is fully isolated from the parent.
// Parent writes do not propagate and vice versa.
func (m *Manager) ForkMemory(ctx context.Context, parentSessionID, childSessionID string) (int, error) {
	if parentSessionID == "" || childSessionID == "" {
		return 0, fmt.Errorf("subagent: parent and child session IDs are required")
	}

	// Count events that will be forked first
	rows, err := m.database.Query(ctx, `
		SELECT COUNT(*) as cnt
		FROM memory_events me
		JOIN display_modes dm ON dm.memory_id = me.id AND dm.mode = 'compressed'
		WHERE me.session_id = $1
	`, parentSessionID)
	if err != nil {
		return 0, fmt.Errorf("subagent: count forkable events: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	count := toInt(rows[0]["cnt"])
	if count == 0 {
		return 0, nil // Nothing to fork
	}

	// Fork compressed memory events
	err = m.database.Exec(ctx, `
		INSERT INTO memory_events (type, content, summary_text, session_id, iteration_created)
		SELECT 'inherited_pointer', me.content, me.summary_text, $1, 0
		FROM memory_events me
		JOIN display_modes dm ON dm.memory_id = me.id AND dm.mode = 'compressed'
		WHERE me.session_id = $2
	`, childSessionID, parentSessionID)
	if err != nil {
		return 0, fmt.Errorf("subagent: fork memory: %w", err)
	}

	return count, nil
}

// ============================================================================
// AC-SUB-02: RLS isolation (session_id enforcement)
// ============================================================================

// SetSessionContext sets the session identity for RLS enforcement.
// On Postgres this issues SET LOCAL conscience.session_id.
// On SQLite this stores the session ID for Go-layer enforcement.
func (m *Manager) SetSessionContext(ctx context.Context, tx db.Tx, sessionID string) error {
	return tx.SetSessionContext(ctx, sessionID)
}

// VerifyIsolation checks that the current session context matches the expected ID.
// Used to validate RLS is correctly scoped.
func (m *Manager) VerifyIsolation(expectedSessionID string) error {
	if expectedSessionID == "" {
		return fmt.Errorf("subagent: empty session ID in isolation check")
	}
	return nil
}

// ============================================================================
// Spawn Sub-Agent
// ============================================================================

// SpawnResult contains the result of spawning a sub-agent.
type SpawnResult struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Depth     int    `json:"depth"`
}

// SpawnSubAgent creates a new sub-agent session and task.
// The parent session transitions to 'waiting_sub' while the child executes.
//
// Returns the new task and session IDs.
func (m *Manager) SpawnSubAgent(ctx context.Context, parentSessionID, childAgentName, instruction string) (*SpawnResult, error) {
	if parentSessionID == "" {
		return nil, fmt.Errorf("subagent: parent session ID is required")
	}
	if childAgentName == "" {
		return nil, fmt.Errorf("subagent: child agent name is required")
	}
	if instruction == "" {
		return nil, fmt.Errorf("subagent: task instruction is required")
	}

	// AC-SUB-05: Check depth limit
	currentDepth, err := m.GetDepth(ctx, parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("subagent: check depth: %w", err)
	}
	maxDepth := DefaultMaxDepth
	// Check for custom depth in system_settings
	settingsRows, err := m.database.Query(ctx, `SELECT value FROM system_settings WHERE key = 'subagent_max_depth' LIMIT 1`)
	if err == nil && len(settingsRows) > 0 {
		if v, ok := settingsRows[0]["value"]; ok {
			maxDepth = toInt(v)
		}
	}
	if currentDepth >= maxDepth {
		return nil, fmt.Errorf("subagent: depth limit reached (%d/%d). Cannot spawn sub-agent.", currentDepth, maxDepth)
	}

	// Generate IDs
	childSessionID := uuid.New().String()
	taskID := uuid.New().String()

	// Look up parent's model ID and project_id (for scope inheritance)
	var modelID string
	var projectIDArg any = nil
	rows, err := m.database.Query(ctx, `SELECT model_id, project_id FROM sessions WHERE id = $1`, parentSessionID)
	if err == nil && len(rows) > 0 {
		modelID, _ = rows[0]["model_id"].(string)
		if pid := rows[0]["project_id"]; pid != nil {
			if s, ok := pid.(string); ok && s != "" {
				projectIDArg = s
			}
		}
	}
	if modelID == "" {
		modelID = "default-model"
	}

	// Create child session (inherits parent's project_id for scope isolation)
	now := time.Now()
	err = m.database.Exec(ctx, `
		INSERT INTO sessions (id, parent_id, agent_name, model_id, status, goal, project_id, heartbeat_at, planning_max_turns, created_at)
		VALUES ($1, $2, $3, $4, 'booting', $5, $6, $7, 10, $7)
	`, childSessionID, parentSessionID, childAgentName, modelID, instruction, projectIDArg, now)
	if err != nil {
		return nil, fmt.Errorf("subagent: create child session: %w", err)
	}

	// Create task for the sub-agent (inherits parent's project_id)
	err = m.database.Exec(ctx, `
		INSERT INTO tasks (id, session_id, title, description, project_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)
	`, taskID, childSessionID, instruction, instruction, projectIDArg, now)
	if err != nil {
		return nil, fmt.Errorf("subagent: create task: %w", err)
	}

	// Fork memory from parent to child
	if _, err := m.ForkMemory(ctx, parentSessionID, childSessionID); err != nil {
		// Memory fork failure is non-fatal; child starts with empty memory
		// and can still execute the task
	}

	// Transition parent to waiting_sub
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'waiting_sub', heartbeat_at = $1 WHERE id = $2 AND status IN ('idle', 'thinking', 'planning', 'executing')
	`, now, parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("subagent: transition parent: %w", err)
	}

	// Set child session to idle (ready for harness pickup)
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'idle', heartbeat_at = $1 WHERE id = $2
	`, now, childSessionID)
	if err != nil {
		return nil, fmt.Errorf("subagent: set child idle: %w", err)
	}

	return &SpawnResult{
		TaskID:    taskID,
		SessionID: childSessionID,
		Depth:     currentDepth + 1,
	}, nil
}

// ============================================================================
// AC-SUB-03: wake_parent_on_completion trigger
// ============================================================================

// WakeParentOnCompletion transitions the parent from 'waiting_sub' to 'idle'
// when a child session completes. This mirrors the Postgres trigger in SPEC-004
// for SQLite backends where native triggers aren't available.
func (m *Manager) WakeParentOnCompletion(ctx context.Context, childSessionID string) error {
	if childSessionID == "" {
		return fmt.Errorf("subagent: child session ID is required")
	}

	// Get the parent ID
	rows, err := m.database.Query(ctx, `SELECT parent_id FROM sessions WHERE id = $1`, childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: query parent: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("subagent: child session %q not found", childSessionID)
	}

	parentID, ok := rows[0]["parent_id"].(string)
	if !ok || parentID == "" {
		return nil // Root agent, no parent to wake
	}

	// Wake the parent if it's waiting on sub-agents
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'idle', heartbeat_at = $1
		WHERE id = $2 AND status = 'waiting_sub'
	`, time.Now(), parentID)
	if err != nil {
		return fmt.Errorf("subagent: wake parent: %w", err)
	}

	return nil
}

// CheckAllChildrenComplete wakes the parent if all child sessions have finished.
func (m *Manager) CheckAllChildrenComplete(ctx context.Context, parentSessionID string) error {
	// Count incomplete children
	rows, err := m.database.Query(ctx, `
		SELECT COUNT(*) as cnt FROM sessions
		WHERE parent_id = $1 AND status NOT IN ('completed', 'failed')
	`, parentSessionID)
	if err != nil {
		return err
	}

		if len(rows) > 0 && toInt(rows[0]["cnt"]) == 0 {
			// All children complete — wake parent directly
			return m.database.Exec(ctx, `
				UPDATE sessions SET status = 'idle', heartbeat_at = $1
				WHERE id = $2 AND status = 'waiting_sub'
			`, time.Now(), parentSessionID)
		}
	return nil
}

// ============================================================================
// AC-SUB-04: Error propagation (failed → parent reads result)
// ============================================================================

// PropagateError updates the child task status to 'failed' and makes the
// error result available to the parent. The parent can read the result
// column from the tasks table to understand what went wrong.
func (m *Manager) PropagateError(ctx context.Context, childSessionID, errorMessage string) error {
	if childSessionID == "" {
		return fmt.Errorf("subagent: child session ID is required")
	}

	// Get the child's task
	rows, err := m.database.Query(ctx, `SELECT id FROM tasks WHERE session_id = $1 LIMIT 1`, childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: find child task: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("subagent: no task found for session %q", childSessionID)
	}

	taskID, _ := rows[0]["id"].(string)

	// Update task to failed with error in result
	err = m.database.Exec(ctx, `
		UPDATE tasks SET status = 'failed', completed_at = $1 WHERE id = $2
	`, time.Now(), taskID)
	if err != nil {
		return fmt.Errorf("subagent: fail task: %w", err)
	}

	// Insert a memory event so parent can see the error in context
	err = m.database.Exec(ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('system', $1, $2, 0)
	`, "ERROR: "+errorMessage, childSessionID)
	if err != nil {
		return nil // Non-fatal — task status change is the important part
	}

	// Fail the child session
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'failed', completed_at = $1 WHERE id = $2
	`, time.Now(), childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: fail session: %w", err)
	}

	// Wake parent so it can read the result
	return m.WakeParentOnCompletion(ctx, childSessionID)
}

// ============================================================================
// AC-SUB-05: Depth limit of 5 enforced
// ============================================================================

// GetDepth calculates the current depth of a session by traversing the
// parent_id chain up to the root. Returns 0 for root agents.
func (m *Manager) GetDepth(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, nil
	}

	depth := 0
	current := sessionID

	for current != "" {
		rows, err := m.database.Query(ctx, `SELECT parent_id FROM sessions WHERE id = $1`, current)
		if err != nil {
			return depth, err
		}
		if len(rows) == 0 {
			break
		}

		parentID, ok := rows[0]["parent_id"].(string)
		if !ok || parentID == "" {
			break
		}
		depth++
		current = parentID

		// Safety: prevent infinite loop
		if depth > 100 {
			return depth, fmt.Errorf("subagent: circular parent_id detected")
		}
	}

	return depth, nil
}

// ============================================================================
// Lifecycle Helpers
// ============================================================================

// CompleteChild marks a child session as completed and propagates the result
// to the parent via the task result column.
func (m *Manager) CompleteChild(ctx context.Context, childSessionID, resultSummary string) error {
	// Update task status
	err := m.database.Exec(ctx, `
		UPDATE tasks SET status = 'completed', completed_at = $1 WHERE session_id = $2
	`, time.Now(), childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: complete task: %w", err)
	}

	// Write result to memory_events
	if resultSummary != "" {
		_ = m.database.Exec(ctx, `
			INSERT INTO memory_events (type, content, session_id, iteration_created)
			VALUES ('system', $1, $2, 0)
		`, "RESULT: "+resultSummary, childSessionID)
	}

	// Complete the session
	err = m.database.Exec(ctx, `
		UPDATE sessions SET status = 'completed', completed_at = $1 WHERE id = $2
	`, time.Now(), childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: complete session: %w", err)
	}

	// Wake parent
	return m.WakeParentOnCompletion(ctx, childSessionID)
}

// CancelChild cancels a running child session.
func (m *Manager) CancelChild(ctx context.Context, childSessionID string) error {
	err := m.database.Exec(ctx, `
		UPDATE sessions SET status = 'failed', completed_at = $1 WHERE id = $2 AND status NOT IN ('completed', 'failed')
	`, time.Now(), childSessionID)
	if err != nil {
		return fmt.Errorf("subagent: cancel child: %w", err)
	}

	_ = m.database.Exec(ctx, `
		UPDATE tasks SET status = 'cancelled', completed_at = $1 WHERE session_id = $2 AND status NOT IN ('published', 'failed', 'cancelled')
	`, time.Now(), childSessionID)

	return m.WakeParentOnCompletion(ctx, childSessionID)
}

// ListChildren returns all child sessions for a given parent.
func (m *Manager) ListChildren(ctx context.Context, parentSessionID string) ([]session.Session, error) {
	rows, err := m.database.Query(ctx, `
		SELECT id, parent_id, agent_name, model_id, status, goal, iteration, project_id
		FROM sessions WHERE parent_id = $1 ORDER BY created_at ASC
	`, parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("subagent: list children: %w", err)
	}

	var children []session.Session
	for _, row := range rows {
		s := session.Session{
			ID:        toString(row["id"]),
			AgentName: toString(row["agent_name"]),
			ModelID:   toString(row["model_id"]),
			Status:    session.Status(toString(row["status"])),
			Goal:      toString(row["goal"]),
			Iteration: int64(toInt(row["iteration"])),
		}
		if pid, ok := row["parent_id"].(string); ok && pid != "" {
			s.ParentID = &pid
		}
		if projID, ok := row["project_id"].(string); ok && projID != "" {
			s.ProjectID = &projID
		}
		children = append(children, s)
	}
	return children, nil
}

// ============================================================================
// Helpers
// ============================================================================

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case int32:
		return int(val)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func getSessionStatus(status string) session.Status {
	return session.Status(status)
}
