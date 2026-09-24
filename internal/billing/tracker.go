// Package billing implements per-iteration cost tracking and budget enforcement (SPEC-006).
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-1 impl=internal/billing/tracker.go
package billing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Billing Tracker
// ============================================================================

// Tracker records billing rows to the agent_billing table and enforces budget limits.
type Tracker struct {
	database db.DB
}

// NewTracker creates a new billing tracker backed by the given database.
func NewTracker(database db.DB) *Tracker {
	return &Tracker{database: database}
}

// RecordBilling writes a billing row after an LLM call completes.
//
// This is called after every LLM response, regardless of success or error.
// The billing data feeds into budget circuit breakers and cost dashboards.
//
// Security: recording failures are logged but do NOT propagate to the caller.
// Billing is append-only; a failure here should not abort the agent's work.
func (t *Tracker) RecordBilling(ctx context.Context, sessionID string, iteration int64, modelID string, category string, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int64, costUSD float64) {
	if sessionID == "" {
		slog.Error("billing: record skipped, missing session_id")
		return
	}
	if modelID == "" {
		modelID = "unknown"
	}
	if category == "" {
		category = "cognition"
	}

	if t.database == nil {
		slog.Warn("billing: skipped — no database configured", "session_id", sessionID)
		return
	}

	query := `INSERT INTO agent_billing
		(session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	err := t.database.Exec(ctx, query, sessionID, iteration, modelID, category, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens, costUSD)
	if err != nil {
		slog.Error("billing: record failed", "session_id", sessionID, "iteration", iteration, "error", err)
		// Non-fatal — billing failure does not halt the agent
		return
	}

	slog.Debug("billing: recorded", "session_id", sessionID, "iteration", iteration, "model", modelID, "cost_usd", costUSD)
}

// ============================================================================
// Budget Enforcement (AC-HARDEN-02)
// ============================================================================

// GetCumulativeCost returns the total cost in USD cents for a session.
func (t *Tracker) GetCumulativeCost(ctx context.Context, sessionID string) (int64, error) {
	if t.database == nil {
		return 0, fmt.Errorf("billing: no database configured")
	}

	rows, err := t.database.Query(ctx,
		`SELECT COALESCE(SUM(cost_usd * 100), 0) as total_cents FROM agent_billing WHERE session_id = $1`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("billing: cumulative cost: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	var cents int64
	if v, ok := rows[0]["total_cents"]; ok {
		cents = toInt64(v)
	}
	return cents, nil
}

// BudgetCheck returns true if the session has exceeded its budget limit.
// If exceeded, an error is returned with the details.
//
// This is called before each LLM call to prevent runaway spend.
func (t *Tracker) BudgetCheck(ctx context.Context, sessionID string, budgetLimitCents int64) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("billing: budget check requires session_id")
	}
	if budgetLimitCents <= 0 {
		return false, nil // No budget limit set
	}

	costCents, err := t.GetCumulativeCost(ctx, sessionID)
	if err != nil {
		return false, err
	}

	if costCents >= budgetLimitCents {
		return true, fmt.Errorf("billing: budget exceeded: %d/%d cents used", costCents, budgetLimitCents)
	}

	return false, nil
}

// ============================================================================
// Helpers
// ============================================================================

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case uint64:
		return int64(val)
	default:
		slog.Warn("billing: unexpected type in toInt64", "type", fmt.Sprintf("%T", v), "value", fmt.Sprintf("%v", v))
		return 0
	}
}
