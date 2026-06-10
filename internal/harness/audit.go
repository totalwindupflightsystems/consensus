// Package harness: audit log and iteration snapshot writer (SPEC-006, SPEC-008).
//
// After each successful iteration, the harness writes:
//  1. An audit_logs row recording monologue, SQL executed, and result
//  2. An iteration_commits snapshot with llm_response and sql_executed
//
// Both writes happen in a single transaction alongside the agent's SQL.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md plan=phase-1/task-1-1/step-1-1-5 impl=internal/harness/audit.go
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================================
// Audit Log Writer
// ============================================================================

// WriteAuditLog inserts an audit_logs row recording the outcome of this iteration.
//
// The audit log captures:
//   - session_id and iteration number
//   - The agent's internal monologue (private reasoning)
//   - All SQL statements that were executed
//   - Whether the transaction committed or rolled back
//   - Any error message (if rolled back)
//
// Security: the monologue is scrubbed of secrets before being stored.
func (h *Harness) WriteAuditLog(ctx context.Context, entry *AuditEntry) error {
	// Scrub secrets from the monologue before persisting to audit trail
	if h.secretStore != nil && entry.Monologue != "" {
		entry.Monologue = h.secretStore.Scrub(entry.Monologue)
	}
	if h.secretStore != nil && entry.ErrorMessage != "" {
		entry.ErrorMessage = h.secretStore.Scrub(entry.ErrorMessage)
	}
	if entry.SessionID == "" {
		return fmt.Errorf("audit: missing session_id")
	}
	if entry.Result != "committed" && entry.Result != "rolled_back" {
		return fmt.Errorf("audit: invalid result %q (expected committed or rolled_back)", entry.Result)
	}

	// Convert sql_executed to PostgreSQL TEXT[] literal via JSON
	sqlJSON, err := json.Marshal(entry.SQLExecuted)
	if err != nil {
		return fmt.Errorf("audit: marshal sql_executed: %w", err)
	}

	query := `
		INSERT INTO audit_logs (session_id, iteration, monologue, sql_executed, result, error_message)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	errMsg := ""
	if entry.ErrorMessage != "" {
		errMsg = entry.ErrorMessage
	}

	err = h.db.Exec(ctx, query,
		entry.SessionID,
		entry.Iteration,
		entry.Monologue,
		string(sqlJSON),
		entry.Result,
		nullIfEmpty(errMsg),
	)

	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}

	return nil
}

// ============================================================================
// Iteration Snapshot Writer
// ============================================================================

// WriteIterationSnapshot saves an iteration_commits row capturing the full
// state of this iteration.
//
// The snapshot captures:
//   - llm_response (the full JSON the LLM returned, scrubbed of secrets)
//   - sql_executed (list of SQL statements executed in this iteration)
//   - rows_affected (count of rows modified)
//
// If the agent's SQL already created an iteration_commits row (with
// active_pointers and display_rules), this method updates it. If no row
// exists yet, it inserts one with minimal defaults.
//
// This enables time-travel debugging: every iteration is replayable.
func (h *Harness) WriteIterationSnapshot(ctx context.Context, sessionID string, iteration int64, llmResponseJSON []byte, sqlExecuted []string, rowsAffected int) error {
	if sessionID == "" {
		return fmt.Errorf("snapshot: missing session_id")
	}
	if iteration <= 0 {
		return fmt.Errorf("snapshot: invalid iteration %d", iteration)
	}

	sqlJSON, err := json.Marshal(sqlExecuted)
	if err != nil {
		return fmt.Errorf("snapshot: marshal sql_executed: %w", err)
	}

	// Try UPDATE first — the agent's SQL may have already INSERTed the row.
	err = h.db.Exec(ctx,
		`UPDATE iteration_commits
		 SET llm_response = $1, sql_executed = $2, rows_affected = $3
		 WHERE session_id = $4 AND iteration_id = $5`,
		string(llmResponseJSON), string(sqlJSON), rowsAffected, sessionID, iteration,
	)
	if err != nil {
		return fmt.Errorf("snapshot: update: %w", err)
	}

	// Check if any row was updated — if not, INSERT a new one.
	checkRows, cerr := h.db.Query(ctx,
		`SELECT COUNT(*) as cnt FROM iteration_commits WHERE session_id = $1 AND iteration_id = $2`,
		sessionID, iteration,
	)
	if cerr != nil {
		return fmt.Errorf("snapshot: check: %w", cerr)
	}
	if len(checkRows) > 0 && toInt(checkRows[0]["cnt"]) > 0 {
		return nil
	}

	// No row existed — INSERT one with defaults.
	emptyArray := "[]"
	emptyObject := "{}"
	return h.db.Exec(ctx,
		`INSERT INTO iteration_commits (session_id, iteration_id, active_pointers, display_rules, llm_response, sql_executed, rows_affected)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID, iteration, emptyArray, emptyObject,
		string(llmResponseJSON), string(sqlJSON), rowsAffected,
	)
}

// ============================================================================
// Combined Writer — called after successful COMMIT
// ============================================================================

// FinalizeIteration writes both audit log and iteration snapshot in a single
// post-commit phase. This is called after the agent's SQL transaction has
// been committed.
//
// If either write fails, the error is logged but does not undo the committed
// transaction — the audit trail is best-effort.
func (h *Harness) FinalizeIteration(ctx context.Context, sessionID string, iteration int64, monologue string, sqlExecuted []string, llmResponseJSON []byte, rowsAffected int, result string, errMsg string) []error {
	var errs []error

	// Write audit log
	auditEntry := &AuditEntry{
		SessionID:    sessionID,
		Iteration:    iteration,
		Monologue:    monologue,
		SQLExecuted:  sqlExecuted,
		Result:       result,
		ErrorMessage: errMsg,
	}
	if err := h.WriteAuditLog(ctx, auditEntry); err != nil {
		errs = append(errs, fmt.Errorf("audit: %w", err))
	}

	// Write iteration snapshot
	if err := h.WriteIterationSnapshot(ctx, sessionID, iteration, llmResponseJSON, sqlExecuted, rowsAffected); err != nil {
		errs = append(errs, fmt.Errorf("snapshot: %w", err))
	}

	return errs
}

// ============================================================================
// SQL Statement Formatter for Audit
// ============================================================================

// FormatSQLStatements joins a list of SQL statements into a single human-readable
// string for audit display. Statements are separated by semicolons and newlines.
func FormatSQLStatements(statements []string) string {
	if len(statements) == 0 {
		return "(no SQL executed)"
	}

	var sb strings.Builder
	for i, stmt := range statements {
		if i > 0 {
			sb.WriteString(";\n")
		}
		sb.WriteString(stmt)
	}
	return sb.String()
}

// ============================================================================
// Helpers
// ============================================================================

// nullIfEmpty returns nil if the string is empty, otherwise returns the string.
// Used for SQL NULL handling on optional TEXT columns.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
