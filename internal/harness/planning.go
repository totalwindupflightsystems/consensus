// Package harness: interactive multi-turn planning (SPEC-020).
//
// The agent works through multiple turns within a single database transaction:
// staging SQL, executing batches, reviewing results, requesting tools, and
// finally committing or rolling back. The staging buffer is persisted to the
// database so that crash recovery works reliably.
//
// Key SPEC-020 contract changes from earlier implementation:
//   - Single long-running transaction for all planning turns (not per-turn auto-commit)
//   - Staging buffer persisted to staging_buffer table (not in-memory)
//   - respond action: reply to user without committing
//   - Command type system: sql, file_write, file_edit, file_delete, memory_write, tool_call_ref
//   - Transaction timeout: 60s (not 10min)
//   - Crash recovery via orphan staging cleanup
//
// axiom:trace work_item=spec-020-hardening-01 spec=specs/020-multi-turn-planning.md plan=phase-1~6 impl=internal/harness/planning.go
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/session"
)

// ============================================================================
// Command Types (SPEC-020 §4 — canonical staging_buffer.cmd_type)
// ============================================================================

// CmdType distinguishes the type of staged command for dispatch and execution.
type CmdType string

const (
	CmdSQL         CmdType = "sql"
	CmdFileWrite   CmdType = "file_write"
	CmdFileEdit    CmdType = "file_edit"
	CmdFileDelete  CmdType = "file_delete"
	CmdMemoryWrite CmdType = "memory_write"
	CmdToolCallRef CmdType = "tool_call_ref"
)

// ============================================================================
// Buffer Status (SPEC-020 §4)
// ============================================================================

// BufferStatus tracks the lifecycle of a staging buffer entry.
type BufferStatus string

const (
	BufferStaged     BufferStatus = "staged"
	BufferExecuted   BufferStatus = "executed"
	BufferCommitted  BufferStatus = "committed"
	BufferRolledBack BufferStatus = "rolled_back"
	BufferFailed     BufferStatus = "failed"
)

// ============================================================================
// Staging Types
// ============================================================================

// StagingEntry represents a single command in the staging buffer.
// Populated from the staging_buffer database table.
type StagingEntry struct {
	ID          int64            `json:"id"`
	SessionID   string           `json:"session_id"`
	Turn        int              `json:"turn"`
	Seq         int              `json:"seq"`
	CmdType     CmdType          `json:"cmd_type"`
	Payload     []byte           `json:"payload"`
	Description string           `json:"description"`
	Executed    bool             `json:"executed"`
	Result      *json.RawMessage `json:"result,omitempty"`
	Status      BufferStatus     `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
}

// StagingBuffer represents the complete state of the agent's staging area.
type StagingBuffer struct {
	Entries  []*StagingEntry `json:"entries"`
	Turn     int             `json:"turn"`
	IsActive bool            `json:"is_active"`
}

// ============================================================================
// Planning Actions (SPEC-020 §3)
// ============================================================================

// PlanAction describes the agent's action for the current turn.
type PlanAction string

const (
	ActionStageExec PlanAction = "stage_and_execute"
	ActionStageOnly PlanAction = "stage_only"
	ActionToolCall  PlanAction = "tool_call"
	ActionCommit    PlanAction = "commit"
	ActionRollback  PlanAction = "rollback"
	ActionRespond   PlanAction = "respond"
	ActionNoOp      PlanAction = "no_op"
)

// TurnPlan represents the agent's selected action and payload for one turn.
type TurnPlan struct {
	Action             PlanAction    `json:"action"`
	StagedCommands     []StagedCmd   `json:"staged_commands,omitempty"`
	ToolRequests       []ToolRequest `json:"tool_requests,omitempty"`
	MemoryStateChanges []string      `json:"memory_state_changes,omitempty"`
	MessageToUser      string        `json:"message_to_user,omitempty"`
	Monologue          string        `json:"internal_monologue"`
	EndIteration       bool          `json:"end_iteration,omitempty"`
}

// StagedCmd describes a command the agent wants to stage.
type StagedCmd struct {
	CmdType     CmdType `json:"type"`
	Payload     string  `json:"payload"`
	Description string  `json:"description"`
}

// ============================================================================
// Planning Configuration (SPEC-020 §11)
// ============================================================================

// PlanningConfig controls the interactive planning loop.
type PlanningConfig struct {
	MaxTurns          int           `json:"max_turns"`        // max turns before auto-commit (default: 10)
	Timeout           time.Duration `json:"timeout"`           // max time for entire planning session (default: 60s)
	MaxRollbacks      int           `json:"max_rollbacks"`     // max rollbacks before forcing failure (default: 3)
	MaxStagedCommands int           `json:"max_staged_cmds"`   // max commands in staging buffer per iteration (default: 50)
	AutoCommitOnMax   bool          `json:"auto_commit_on_max"` // auto-commit when max turns hit with staged work
}

// DefaultPlanningConfig returns spec-aligned defaults.
func DefaultPlanningConfig() *PlanningConfig {
	return &PlanningConfig{
		MaxTurns:          10,
		Timeout:           180 * time.Second, // Local LLMs (LM Studio, Ollama) need more time than cloud APIs
		MaxRollbacks:      3,
		MaxStagedCommands: 50,    // SPEC-020 §11
		AutoCommitOnMax:   true,  // SPEC-020 §11
	}
}

// ============================================================================
// RunInteractivePlanning — the main entry point (SPEC-020 §5)
// ============================================================================

// RunInteractivePlanning executes the multi-turn planning loop within a single
// long-running database transaction.
//
// The loop runs until:
//   - commit: agent finalizes all staged work, transaction commits
//   - rollback + end_iteration: agent gives up, iteration ends
//   - respond: agent replies to user without committing
//   - max turns: auto-commit (if work done) or error (if idle)
//   - timeout: transaction rolled back, session marked failed
//   - max rollbacks: session terminated
func (h *Harness) RunInteractivePlanning(ctx context.Context, sessionID string, config *PlanningConfig) (*IterationResult, error) {
	if config == nil {
		config = DefaultPlanningConfig()
	}

	slog.Info("planning: starting interactive session",
		"session_id", sessionID,
		"max_turns", config.MaxTurns,
		"timeout", config.Timeout,
	)

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// Debug-level logging to trace atomic claim
	slog.Debug("planning: attempting atomic claim", "session_id", sessionID)

	// Atomic claim: transition from thinking→planning.
	// This runs BEFORE opening the planning transaction so the claim uses
	// an unpinned pool connection. On SQLite in-memory (tests), each pool
	// connection has its own private database — running the claim inside
	// the transaction would use a separate connection and fail with
	// "no such table: sessions".
	// The WHERE status='thinking' clause prevents duplicate goroutines
	// from processing the same session on concurrent heartbeat ticks.
	if err := h.db.Exec(ctx,
		`UPDATE sessions SET status = 'planning', heartbeat_at = datetime('now') WHERE id = $1 AND status = 'thinking'`,
		sessionID); err != nil {
		slog.Error("planning: atomic claim failed", "session_id", sessionID, "error", err)
		return nil, fmt.Errorf("planning: claim session: %w", err)
	}

	// Verify we actually claimed it — another goroutine may have won the race.
	rows, err := h.db.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	actualStatus := "unknown"
	if err == nil && len(rows) > 0 {
		actualStatus = toString(rows[0]["status"])
	}
	slog.Debug("planning: claim result", "session_id", sessionID, "status", actualStatus, "query_err", err)
	if actualStatus != "planning" {
		slog.Info("planning: session already claimed by another worker, bailing out",
			"session_id", sessionID, "actual_status", actualStatus)
		return nil, nil // another goroutine got there first
	}

	// Open single long-running transaction (HARDEN-PLAN-01)
	tx, err := h.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("planning: begin tx: %w", err)
	}
	defer func() {
		if tx.IsActive() {
			tx.Rollback()
		}
	}()

	if err := tx.SetSessionContext(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("planning: set session context: %w", err)
	}

	// Track state
	rollbackCount := 0
	turnsWithWork := 0
	memoryStateChanges := make([]string, 0)

	// Read initial context
	ic, err := h.ReadActiveContext(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("planning: initial context: %w", err)
	}

	// Main planning loop
	for turn := 1; turn <= config.MaxTurns; turn++ {
		// Check timeout
		select {
		case <-ctx.Done():
			return h.handlePlanningTimeout(ctx, tx, sessionID, turn, ctx.Err())
		default:
		}

		// Load staging buffer from database for this turn's context
		buffer, err := h.loadStagingBuffer(ctx, sessionID)
		if err != nil {
			return h.handlePlanningError(ctx, tx, sessionID, err)
		}
		buffer.Turn = turn

		// Format turn context and call LLM
		turnContext := h.formatTurnContextV2(ic, buffer, turn, config, memoryStateChanges)

		messages := []Message{
			{Role: "system", Content: h.formatPlanningSystemPromptV2(ic, buffer, turn, config)},
			{Role: "user", Content: turnContext},
		}

		output, err := h.LLMClient.Call(ctx, messages)
		if err != nil {
			slog.Error("planning: LLM call failed", "turn", turn, "error", err)
			return h.handlePlanningError(ctx, tx, sessionID, err)
		}

		// Parse into TurnPlan
		plan := h.outputToTurnPlanV2(output.Output)

		// Record billing after each LLM call
		if h.BillingTracker != nil {
			h.BillingTracker.RecordBilling(ctx, sessionID, ic.Iteration, output.ModelID, "planning",
				output.Usage.PromptTokens, output.Usage.CompletionTokens,
				output.Usage.CacheReadTokens, output.Usage.CacheWriteTokens,
				0.0)

			// Check budget after recording
			exceeded, bErr := h.BillingTracker.BudgetCheck(ctx, sessionID, ic.BudgetLimitCents)
			if bErr != nil {
				slog.Error("planning: budget check failed", "session_id", sessionID, "error", bErr)
			}
			if exceeded {
				slog.Warn("planning: budget exceeded", "session_id", sessionID)
				if err := h.db.Exec(ctx,
					`UPDATE sessions SET status = 'paused', heartbeat_at = datetime('now') WHERE id = $1`,
					sessionID); err != nil {
					slog.Error("planning: failed to pause session for budget", "session_id", sessionID, "error", err)
				}
				return nil, nil
			}
		}

		// Save monologue as audit entry
		h.WriteAuditLog(ctx, &AuditEntry{
			SessionID:   sessionID,
			Monologue:   plan.Monologue,
			SQLExecuted: nil,
			Result:      "planning_turn",
		})

		// Dispatch based on action
		switch plan.Action {
		case ActionStageExec, ActionStageOnly:
			if len(plan.StagedCommands) == 0 {
				slog.Warn("planning: no commands staged", "turn", turn)
				continue
			}

			// Enforce buffer limit (HARDEN-PLAN-BUF)
			currentCount := len(buffer.Entries)
			if currentCount+len(plan.StagedCommands) > config.MaxStagedCommands {
				slog.Error("planning: buffer limit hit", "current", currentCount, "adding", len(plan.StagedCommands), "max", config.MaxStagedCommands)
				return h.handlePlanningError(ctx, tx, sessionID,
					fmt.Errorf("staging buffer limit exceeded (%d)", config.MaxStagedCommands))
			}

			// Insert into staging_buffer table
			exec := plan.Action == ActionStageExec
			for i, cmd := range plan.StagedCommands {
				entry := &StagingEntry{
					SessionID:   sessionID,
					Turn:        turn,
					Seq:         i + 1,
					CmdType:     cmd.CmdType,
					Payload:     []byte(cmd.Payload),
					Description: cmd.Description,
					Status:      BufferStaged,
				}

				if err := h.insertStagingEntry(ctx, tx, sessionID, entry, config); err != nil {
					return h.handlePlanningError(ctx, tx, sessionID, fmt.Errorf("insert staging entry: %w", err))
				}

				if exec {
					// Execute within the open transaction
					result, execErr := h.executeStagedEntry(ctx, tx, entry, ic.Iteration)
					if execErr != nil {
						// Don't fail the whole session — mark entry as failed, let agent decide
						h.updateStagingStatus(ctx, tx, entry.ID, BufferFailed, &json.RawMessage{})
						slog.Warn("planning: staged command failed", "turn", turn, "cmd_type", cmd.CmdType, "error", execErr)

						// If the error breaks the transaction, rollback and fail
						if !tx.IsActive() {
							return h.handlePlanningError(ctx, tx, sessionID, execErr)
						}
					} else {
						h.updateStagingResult(ctx, tx, entry.ID, result)
					}
				}
			}

			turnsWithWork++
			slog.Info("planning: commands staged", "turn", turn, "count", len(plan.StagedCommands), "executed", exec)

		case ActionToolCall:
			turnsWithWork++
			slog.Info("planning: tool call requested", "turn", turn, "tools", len(plan.ToolRequests))
			// Record tool_call_ref entries in staging buffer so agent sees them next turn
			for i, tr := range plan.ToolRequests {
				payload, _ := json.Marshal(tr)
				entry := &StagingEntry{
					SessionID:   sessionID,
					Turn:        turn,
					Seq:         i + 1,
					CmdType:     CmdToolCallRef,
					Payload:     payload,
					Description: fmt.Sprintf("tool_call: %s", tr.ToolName),
					Status:      BufferExecuted,
				}
				h.insertStagingEntry(ctx, tx, sessionID, entry, config)
			}
			// Suspend transaction, execute tools outside, then return control
			return h.handleToolCallDuringPlanning(ctx, tx, sessionID, plan, turn)

		case ActionCommit:
			// Apply memory_state_changes before commit (HARDEN-PLAN-MEM)
			if len(plan.MemoryStateChanges) > 0 {
				memoryStateChanges = append(memoryStateChanges, plan.MemoryStateChanges...)
			}
			if err := h.applyMemoryStateChanges(ctx, tx, sessionID, ic.TrustLevel, memoryStateChanges); err != nil {
				return h.handlePlanningError(ctx, tx, sessionID, fmt.Errorf("commit: memory state changes: %w", err))
			}

			return h.handleCommitV2(ctx, tx, sessionID, turn, memoryStateChanges)

		case ActionRollback:
			rollbackCount++
			if rollbackCount > config.MaxRollbacks {
				slog.Error("planning: max rollbacks exceeded", "count", rollbackCount, "max", config.MaxRollbacks)
				return h.handleMaxRollbacksV2(ctx, tx, sessionID, turn)
			}
			// Rollback and reset in-memory state; staging_buffer rows remain for audit
			slog.Info("planning: rolling back", "turn", turn, "rollback_count", rollbackCount)

			if plan.EndIteration {
				memoryStateChanges = nil
				return h.handleRollbackAndEnd(ctx, tx, sessionID, turn, plan.MessageToUser)
			}

			// Re-open fresh transaction for retry
			tx.Rollback()
			tx, err = h.db.BeginTx(ctx)
			if err != nil {
				return nil, fmt.Errorf("planning: re-open tx: %w", err)
			}
			tx.SetSessionContext(ctx, sessionID)
			memoryStateChanges = nil // clear pending changes

		case ActionRespond:
			// Reply to user without committing (HARDEN-PLAN-03)
			tx.Rollback()
			slog.Info("planning: responding to user", "turn", turn)
			return &IterationResult{
				Status:     "success",
				NextStatus: string(session.StatusIdle),
				AuditEntry: AuditEntry{
					SessionID: sessionID,
					Monologue: plan.Monologue,
					Result:    "responded",
				},
			}, nil

		case ActionNoOp:
			slog.Info("planning: no-op turn", "turn", turn)
		}

		// Refresh context for next turn
		ic, _ = h.ReadActiveContext(ctx, sessionID)
	}

	// Max turns reached
	if turnsWithWork > 0 && config.AutoCommitOnMax {
		slog.Info("planning: max turns reached, auto-committing", "turns_with_work", turnsWithWork)
		if err := h.applyMemoryStateChanges(ctx, tx, sessionID, ic.TrustLevel, memoryStateChanges); err != nil {
			return h.handlePlanningError(ctx, tx, sessionID, err)
		}
		return h.handleCommitV2(ctx, tx, sessionID, config.MaxTurns, memoryStateChanges)
	}

	slog.Warn("planning: max turns reached with no productive work")

	// Deferred cleanup: ensure session always ends up idle regardless of which path
	// is taken below. Without this, if handleCommitV2 fails (e.g., transaction state
	// is compromised), the session stays stuck in its current status forever.
	defer func() {
		if err := h.db.Exec(ctx,
			`UPDATE sessions SET status = 'idle', heartbeat_at = datetime('now') WHERE id = $1 AND status NOT IN ('idle','failed','paused')`,
			sessionID); err != nil {
			slog.Error("planning: failed to update session status to idle after max turns",
				"session_id", sessionID, "error", err)
		}
	}()

	// Apply any memory_state_changes and commit (even if no staged commands —
	// the agent might have intended to commit pure memory changes)
	var finalMsg string
	if len(memoryStateChanges) > 0 {
		if err := h.applyMemoryStateChanges(ctx, tx, sessionID, ic.TrustLevel, memoryStateChanges); err != nil {
			return h.handlePlanningError(ctx, tx, sessionID, err)
		}
		result, err := h.handleCommitV2(ctx, tx, sessionID, config.MaxTurns, memoryStateChanges)
		if err != nil {
			return result, err
		}
		finalMsg = result.ErrorInjected
	} else {
		tx.Rollback()
		// BUG FIX: tx is rolled back, so we must update session status outside the transaction.
		// Without this, the session remains stuck in "planning" forever.
		if err := h.db.Exec(ctx, `UPDATE sessions SET status = 'idle', heartbeat_at = datetime('now') WHERE id = $1`, sessionID); err != nil {
			slog.Error("planning: failed to update session status to idle after no productive work", "session_id", sessionID, "error", err)
		}
	}

	return &IterationResult{
		Status:        "warning",
		ErrorInjected: fmt.Sprintf("max turns (%d) reached: %s", config.MaxTurns, finalMsg),
		NextStatus:    string(session.StatusIdle),
	}, nil
}

// ============================================================================
// Staging Buffer Persistence (HARDEN-PLAN-02)
// ============================================================================

func (h *Harness) insertStagingEntry(ctx context.Context, tx db.Tx, sessionID string, entry *StagingEntry, config *PlanningConfig) error {
	payloadJSON, err := json.Marshal(string(entry.Payload))
	if err != nil {
		payloadJSON, _ = json.Marshal(entry.Payload)
	}

	return tx.Exec(ctx, `
		INSERT INTO staging_buffer
			(session_id, iteration, turn, seq, cmd_type, payload, description, status, created_at)
		VALUES ($1, 0, $2, $3, $4, $5, $6, $7, datetime('now'))
	`, sessionID, entry.Turn, entry.Seq, string(entry.CmdType),
		string(payloadJSON), entry.Description, string(entry.Status))
}

func (h *Harness) updateStagingStatus(ctx context.Context, tx db.Tx, entryID int64, status BufferStatus, result *json.RawMessage) error {
	return tx.Exec(ctx, `
		UPDATE staging_buffer SET status = $1, executed = $2, executed_at = datetime('now')
		WHERE id = $3
	`, string(status), status == BufferExecuted, entryID)
}

func (h *Harness) updateStagingResult(ctx context.Context, tx db.Tx, entryID int64, result *json.RawMessage) error {
	return tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'executed', result = $1, executed = true, executed_at = datetime('now')
		WHERE id = $2
	`, string(*result), entryID)
}

func (h *Harness) executeStagedEntry(ctx context.Context, tx db.Tx, entry *StagingEntry, iteration int64) (*json.RawMessage, error) {
	switch entry.CmdType {
	case CmdSQL:
		// For SQL commands, execute directly
		sqlStr := string(entry.Payload)
		if sqlStr == "" {
			return nil, fmt.Errorf("empty SQL payload")
		}
		if err := tx.Exec(ctx, sqlStr); err != nil {
			return nil, fmt.Errorf("sql: %w", err)
		}
		result, _ := json.Marshal(map[string]string{"status": "ok"})
		return (*json.RawMessage)(&result), nil

	case CmdFileWrite, CmdFileEdit, CmdFileDelete:
		// File operations go through tool_registry
		result, _ := json.Marshal(map[string]string{"status": "staged", "note": "file operations executed via tool_registry"})
		return (*json.RawMessage)(&result), nil

	case CmdMemoryWrite:
		// Memory writes deferred to commit time
		result, _ := json.Marshal(map[string]string{"status": "will_apply_at_commit"})
		return (*json.RawMessage)(&result), nil

	case CmdToolCallRef:
		// Tool calls are executed outside the transaction
		result, _ := json.Marshal(map[string]string{"status": "executing"})
		return (*json.RawMessage)(&result), nil

	default:
		return nil, fmt.Errorf("unknown command type: %s", entry.CmdType)
	}
}

// loadStagingBuffer reads all staging entries for this session from the database.
func (h *Harness) loadStagingBuffer(ctx context.Context, sessionID string) (*StagingBuffer, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, session_id, turn, seq, cmd_type, payload, description, executed, result, status, created_at
		FROM staging_buffer
		WHERE session_id = $1 AND status IN ('staged', 'executed')
		ORDER BY turn, seq
	`, sessionID)
	if err != nil {
		if isTableNotFound(err) {
			return &StagingBuffer{IsActive: true}, nil
		}
		return nil, fmt.Errorf("load staging: %w", err)
	}

	buffer := &StagingBuffer{IsActive: true}
	for _, r := range rows {
		entry := &StagingEntry{
			ID:          toInt64(r["id"]),
			SessionID:   toString(r["session_id"]),
			Turn:        toIntHelper(r["turn"]),
			Seq:         toIntHelper(r["seq"]),
			CmdType:     CmdType(toString(r["cmd_type"])),
			Description: toString(r["description"]),
			Status:      BufferStatus(toString(r["status"])),
		}
		if raw, ok := r["payload"]; ok {
			if b, ok := raw.([]byte); ok {
				entry.Payload = b
			} else {
				entry.Payload, _ = json.Marshal(raw)
			}
		}
		if exec, ok := r["executed"]; ok {
			entry.Executed = toBoolHelper(exec)
		}
		if raw, ok := r["result"]; ok && raw != nil {
			result, _ := json.Marshal(raw)
			entry.Result = (*json.RawMessage)(&result)
		}
		buffer.Entries = append(buffer.Entries, entry)
	}
	return buffer, nil
}

// applyMemoryStateChanges executes memory_state_changes within the transaction before commit.
func (h *Harness) applyMemoryStateChanges(ctx context.Context, tx db.Tx, sessionID string, trustLevel string, changes []string) error {
	if len(changes) == 0 {
		return nil
	}
	for _, stmt := range changes {
		if err := h.executeStatement(ctx, tx, stmt, sessionID, trustLevel); err != nil {
			return fmt.Errorf("memory_state_change %q: %w", stmt, err)
		}
	}
	return nil
}

// ============================================================================
// Action Handlers (with tx parameter)
// ============================================================================

func (h *Harness) handleCommitV2(ctx context.Context, tx db.Tx, sessionID string, turn int, memoryStateChanges []string) (*IterationResult, error) {
	slog.Info("planning: committing", "turn", turn)

	// Mark all staging entries as committed
	if err := tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'committed'
		WHERE session_id = $1 AND status IN ('staged', 'executed')
	`, sessionID); err != nil {
		return nil, fmt.Errorf("commit: update staging: %w", err)
	}

	// Update session to idle
	if err := tx.Exec(ctx, `
		UPDATE sessions SET status = 'idle', heartbeat_at = datetime('now'), iteration = iteration + 1
		WHERE id = $1
	`, sessionID); err != nil {
		return nil, fmt.Errorf("commit: update session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &IterationResult{
		Status:     "success",
		NextStatus: string(session.StatusIdle),
	}, nil
}

func (h *Harness) handleRollbackAndEnd(ctx context.Context, tx db.Tx, sessionID string, turn int, messageToUser string) (*IterationResult, error) {
	// Mark staging entries as rolled_back
	tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'rolled_back'
		WHERE session_id = $1 AND status IN ('staged', 'executed')
	`, sessionID)

	tx.Rollback()

	return &IterationResult{
		Status:     "success",
		NextStatus: string(session.StatusIdle),
	}, nil
}

func (h *Harness) handleMaxRollbacksV2(ctx context.Context, tx db.Tx, sessionID string, turn int) (*IterationResult, error) {
	// Mark buffer as failed
	tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'failed'
		WHERE session_id = $1 AND status IN ('staged', 'executed')
	`, sessionID)

	if err := tx.Exec(ctx, `UPDATE sessions SET status = 'failed' WHERE id = $1`, sessionID); err != nil {
		return nil, err
	}

	return &IterationResult{
		Status:        "error",
		ErrorInjected: "max rollbacks exceeded — session terminated",
		NextStatus:    string(session.StatusFailed),
	}, nil
}

func (h *Harness) handlePlanningTimeout(ctx context.Context, tx db.Tx, sessionID string, turn int, err error) (*IterationResult, error) {
	slog.Error("planning: timeout", "session_id", sessionID, "turn", turn)

	tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'failed'
		WHERE session_id = $1 AND status IN ('staged', 'executed')
	`, sessionID)

	if err := h.db.Exec(ctx, `UPDATE sessions SET status = 'failed' WHERE id = $1`, sessionID); err != nil {
		slog.Error("planning: failed to update session on timeout", "error", err)
	}

	tx.Rollback()

	return &IterationResult{
		Status:        "error",
		ErrorInjected: fmt.Sprintf("planning timeout after turn %d: %v", turn, err),
		NextStatus:    string(session.StatusFailed),
	}, nil
}

func (h *Harness) handlePlanningError(ctx context.Context, tx db.Tx, sessionID string, err error) (*IterationResult, error) {
	slog.Error("planning: error", "error", err)

	if tx.IsActive() {
		tx.Rollback()
	}

	// Mark staging buffer as failed and transition session to failed.
	// Without the status update, the session stays stuck in "planning" forever.
	tx.Exec(ctx, `
		UPDATE staging_buffer SET status = 'failed'
		WHERE session_id = $1 AND status IN ('staged', 'executed')
	`, sessionID)

	if err := h.db.Exec(ctx, `UPDATE sessions SET status = 'failed', heartbeat_at = datetime('now') WHERE id = $1`, sessionID); err != nil {
		slog.Error("planning: failed to update session status to failed after error", "session_id", sessionID, "error", err)
	}

	return &IterationResult{
		Status:        "error",
		Error:         err,
		ErrorInjected: err.Error(),
	}, nil
}

func (h *Harness) handleToolCallDuringPlanning(ctx context.Context, tx db.Tx, sessionID string, plan TurnPlan, turn int) (*IterationResult, error) {
	// Suspend the open transaction — transition to tool_exec
	tx.Rollback() // Close the planning tx; tool execution will re-open a new one

	if err := h.db.Exec(ctx, `UPDATE sessions SET status = 'tool_exec' WHERE id = $1`, sessionID); err != nil {
		return nil, err
	}

	return &IterationResult{
		Status:     "success",
		NextStatus: string(session.StatusToolExec),
		AuditEntry: AuditEntry{
			SessionID: sessionID,
			Monologue: plan.Monologue,
			Result:    "tool_exec_requested",
		},
	}, nil
}

// ============================================================================
// Context Formatting (SPEC-020 §6)
// ============================================================================

func (h *Harness) formatTurnContextV2(ic *IterationContext, buffer *StagingBuffer, turn int, config *PlanningConfig, memoryStateChanges []string) string {
	var memSection string
	if len(memoryStateChanges) > 0 {
		memSection = fmt.Sprintf("\n### Pending Memory Changes (applied at commit)\n%s\n", formatSQLList(memoryStateChanges))
	}

	return fmt.Sprintf(
		`# Transaction Window — Turn %d / %d

**Session:** %s
**Goal:** %s
**Status:** %s | Iteration: %d

## Transaction Contents
%s
%s

## Available Actions
- **stage_and_execute**: Stage commands and execute them. You'll see results next turn.
- **stage_only**: Stage commands without executing (queue for later).
- **tool_call**: Run an external tool. Results appear next turn.
- **commit**: Commit the transaction. All staged commands become permanent.
- **rollback**: Roll back everything. You can retry or give up.
- **respond**: Reply to the user without committing.

What do you want to do?`,
		turn, config.MaxTurns,
		ic.SessionID,
		ic.Goal, ic.Status, ic.Iteration,
		h.formatBufferStateV2(buffer),
		memSection,
	)
}

func (h *Harness) formatBufferStateV2(buffer *StagingBuffer) string {
	if len(buffer.Entries) == 0 {
		return "(empty — no staged commands yet)\n"
	}

	var result string
	for _, e := range buffer.Entries {
		icon := "…"
		switch e.Status {
		case BufferExecuted:
			icon = "✓"
		case BufferFailed:
			icon = "✗"
		case BufferRolledBack:
			icon = "↩"
		}
		result += fmt.Sprintf("%s [Turn %d] %s: %s", icon, e.Turn, e.CmdType, e.Description)
		if e.Result != nil && len(*e.Result) > 0 {
			result += fmt.Sprintf("\n    Result: %s", truncateJSON(*e.Result, 200))
		}
		result += "\n"
	}
	return result
}

// ============================================================================
// Planning System Prompt v2
// ============================================================================

func (h *Harness) formatPlanningSystemPromptV2(ic *IterationContext, buffer *StagingBuffer, turn int, config *PlanningConfig) string {
	// Build schema section — LLM needs column info to generate valid SQL
	// (e.g., memory_events.type, not memory_events.event_type).
	var schemaBuf strings.Builder
	schemaBuf.WriteString("## Database Schema\n\n")
	schemaBuf.WriteString("You can read from and (where allowed) write to the following tables:\n\n")
	schemaBuf.WriteString("| Table | Columns | Writable |\n")
	schemaBuf.WriteString("|---|---|---|\n")
	coreWritable := map[string]string{
		"memory_events":      "INSERT only (append-only)",
		"display_modes":      "Full CRUD",
		"iteration_commits":  "INSERT only",
		"memory_pages":       "Full CRUD (scoped)",
		"tasks":              "Full CRUD (scoped)",
		"tool_requests":      "INSERT only",
		"tool_results":       "INSERT only",
		"agent_billing":      "INSERT only",
		"staging_buffer":     "Full CRUD (scoped)",
		"audit_logs":         "INSERT only",
		"agent_messages":     "INSERT only",
		"compression_queue":  "INSERT only",
		"custom_agent_tools": "INSERT only",
	}
	for _, name := range coreTableNames {
		writable := coreWritable[name]
		if writable == "" {
			writable = "Read only"
		}
		cols := coreTableColumns[name]
		if cols == "" {
			cols = "(unknown)"
		}
		schemaBuf.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", name, cols, writable))
	}
	schemaBuf.WriteString("\n")
	schemaBuf.WriteString("**Key:** `memory_events.type` (NOT event_type) — valid values: 'header', 'text_block', 'tool_call', 'tool_result', 'thinking', 'system', 'inherited_pointer', 'user_message'. `memory_events.content` (NOT payload). Use ONLY the columns shown above.\n")
	schemaBuf.WriteString("**SQLite:** No gen_random_uuid(). No SERIAL/BIGSERIAL — use INTEGER PRIMARY KEY AUTOINCREMENT. No ::type casts. No JSONB operators.\n")
	schemaSection := schemaBuf.String()

	return fmt.Sprintf(`You are a Conscience agent in interactive multi-turn planning mode (SPEC-020).

Goal: %s
Session: %s
Turn: %d / %d

%s

You have an open database transaction. You can stage commands, execute them,
see results, and decide what to do next — just like an engineer in an interactive
psql session.

**Rules:**
- Write SQL to memory_state_changes: your persistent ledger of everything you do.
- Use system_actions for session-level operations: ["commit"], ["rollback"], ["respond"], or ["respond", "end"].
- Request external tools via tool_requests (transaction suspended while tools run).
- All SQL is executed atomically. On failure, everything rolls back.
- memory_events is append-only — you can INSERT but not UPDATE or DELETE.
- Only access tables scoped to your session_id.
- For schema changes (CREATE TABLE, ALTER TABLE), put the SQL directly in memory_state_changes.
  The harness executes DDL immediately (before the transaction) and retries within the transaction.
- If there's a SQL error, the harness injects the error into the next context for recovery.
- **SQLite notes:** No gen_random_uuid() — the harness rewrites it. No ::type casts. No JSONB operators.
  Use INTEGER PRIMARY KEY AUTOINCREMENT for auto-incrementing IDs (not SERIAL/BIGSERIAL).
  DEFAULT (datetime('now')) for timestamps — parentheses required. CHECK constraints for validation.
  **Use CREATE TABLE IF NOT EXISTS** — SQLite auto-commits DDL, so repeated CREATE TABLE fails.
  **Use INSERT OR IGNORE** for idempotent inserts when you're unsure if rows already exist.

**Output format (valid JSON only):**
{
  "internal_monologue": "your reasoning — what you plan to do and why",
  "memory_state_changes": [
    "SQL statement 1",
    "SQL statement 2"
  ],
  "system_actions": [],
  "tool_requests": [],
  "sub_agent_spawns": []
}

**memory_state_changes:** Each entry is a full SQL statement (CREATE TABLE, INSERT, SELECT, etc.).
Put DDL (CREATE TABLE, ALTER TABLE) before DML (INSERT, UPDATE) in separate entries.
**IMPORTANT:** memory_state_changes IS where SQL code goes — not text descriptions, not prose.
Each entry is raw executable SQL. Example of creating a table (session_id is in the turn context):
  "memory_state_changes": [
    "CREATE TABLE IF NOT EXISTS my_table (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, created_at TEXT DEFAULT (datetime('now')))",
    "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Created my_table', '<your session id>', 0)"
  ]
**system_actions:** ["commit"] to finalize and end the session. ["rollback"] to undo.
["respond"] to reply without committing. ["respond", "end"] to reply and end the session.
Empty array [] means continue planning.
**tool_requests:** [{"tool_name": "name", "parameters": {...}}] — external tool calls.
**sub_agent_spawns:** [{"agent_name": "...", "goal": "...", "model": "..."}] — fork a sub-agent.`, ic.Goal, ic.SessionID, turn, config.MaxTurns, schemaSection)
}

// ============================================================================
// Output Parsing
// ============================================================================

func (h *Harness) outputToTurnPlanV2(output *AgentOutput) TurnPlan {
	plan := TurnPlan{
		Action:    ActionNoOp,
		Monologue: output.InternalMonologue,
	}

	// Check for tool requests first (they take priority)
	if len(output.ToolRequests) > 0 {
		plan.Action = ActionToolCall
		plan.ToolRequests = output.ToolRequests
		plan.MemoryStateChanges = output.MemoryStateChanges
		return plan
	}

	// Check system_actions for commit/rollback/respond intent
	for _, sa := range output.SystemActions {
		switch {
		case containsWord(sa, "commit"):
			plan.Action = ActionCommit
			plan.MemoryStateChanges = output.MemoryStateChanges
			return plan
		case containsWord(sa, "rollback"):
			plan.Action = ActionRollback
			plan.EndIteration = containsWord(sa, "end") || containsWord(sa, "give_up")
			return plan
		case containsWord(sa, "respond"):
			plan.Action = ActionRespond
			return plan
		}
	}

	// Memory state changes → stage_and_execute
	if len(output.MemoryStateChanges) > 0 && plan.Action == ActionNoOp {
		plan.Action = ActionStageExec
		for _, msc := range output.MemoryStateChanges {
			plan.StagedCommands = append(plan.StagedCommands, StagedCmd{
				CmdType:     CmdSQL,
				Payload:     msc,
				Description: "memory_state_change",
			})
		}
		plan.MemoryStateChanges = nil // we converted them to staged commands
	}

	return plan
}

// ============================================================================
// Crash Recovery — Orphan Staging Cleanup (HARDEN-PLAN-07)
// ============================================================================

// CleanupOrphanedStaging marks staging entries as failed for sessions that
// are no longer in planning/executing/tool_exec status.
// This should run as a goroutine on a periodic timer.
func (h *Harness) CleanupOrphanedStaging(ctx context.Context) error {
	return h.db.Exec(ctx, `
		UPDATE staging_buffer
		SET status = 'failed'
		WHERE status IN ('staged', 'executed')
		  AND session_id IN (
		    SELECT id FROM sessions WHERE status IN ('failed', 'completed')
		  )
	`)
}

// ============================================================================
// Helpers
// ============================================================================

func containsWord(s, word string) bool {
	lower := toLower(s)
	w := toLower(word)
	return len(lower) >= len(w) && indexOf(lower, w) >= 0
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b[i] = s[i] + 32
		} else {
			b[i] = s[i]
		}
	}
	return string(b)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func formatSQLList(sqls []string) string {
	var out string
	for i, s := range sqls {
		out += fmt.Sprintf("- [%d] %s\n", i+1, truncateSQL(s, 120))
	}
	return out
}

func truncateSQL(sql string, maxLen int) string {
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen-3] + "..."
}

func truncateJSON(raw json.RawMessage, maxLen int) string {
	s := string(raw)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func isTableNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsWord(msg, "does not exist") || containsWord(msg, "no such table")
}

// toIntHelper converts any to int (avoids redeclaring toInt from context.go).
func toIntHelper(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

// toBoolHelper converts any to bool.
func toBoolHelper(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case string:
		return val == "true" || val == "1"
	}
	return false
}

