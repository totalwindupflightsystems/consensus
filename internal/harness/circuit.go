// Package harness: circuit breaker persistence (AC-HARDEN-03, AC-HARDEN-04).
//
// Circuit breakers prevent unbounded agent loops by enforcing hard limits
// on consecutive errors, iterations, and budget. This file persists tripped
// state to the agent_circuit_breakers table so it survives process restarts.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-2 impl=internal/harness/circuit.go
package harness

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ============================================================================
// Circuit Breaker Types (SPEC-006 §Circuit Breakers)
// ============================================================================

// BreakerType identifies the kind of circuit breaker.
type BreakerType string

const (
	BreakerConsecutiveErrors BreakerType = "consecutive_errors"
	BreakerIterations        BreakerType = "iterations"
	BreakerBudget            BreakerType = "budget"
)

// ============================================================================
// Circuit Breaker Persistence (AC-HARDEN-03, AC-HARDEN-04)
// ============================================================================

// CheckCircuitBreaker checks whether a circuit breaker should trip and persists
// the tripped state to the database.
//
// Returns (tripped, error). If tripped is true, the session should be stopped.
// The tripped state is persisted so it survives harness restarts.
//
// AC-HARDEN-04: produce 3 consecutive errors, verify circuit_breaker row exists
// with tripped_at set.
func (h *Harness) CheckCircuitBreaker(ctx context.Context, sessionID string, breakerType BreakerType, currentCount int, threshold int) (bool, error) {
	if threshold <= 0 {
		return false, nil
	}

	if currentCount < threshold {
		// Not tripped — update or insert the current count for tracking
		return false, h.upsertBreakerCount(ctx, sessionID, breakerType, currentCount, threshold)
	}

	// Circuit breaker tripped
	slog.Error("harness: circuit breaker tripped",
		"session_id", sessionID,
		"type", string(breakerType),
		"count", currentCount,
		"threshold", threshold,
	)

	// Persist tripped state (AC-HARDEN-04)
	if err := h.tripBreaker(ctx, sessionID, breakerType, currentCount); err != nil {
		slog.Error("harness: failed to persist tripped circuit breaker", "session_id", sessionID, "error", err)
		// Still return tripped=true even if persistence fails — we don't want to
		// continue executing after a trip event.
	}

	return true, nil
}

// upsertBreakerCount writes or updates the current count for a breaker type.
// This is called on every iteration to keep the count in sync with the DB.
func (h *Harness) upsertBreakerCount(ctx context.Context, sessionID string, breakerType BreakerType, count int, threshold int) error {
	if h.db == nil {
		return fmt.Errorf("circuit: no database configured")
	}

	err := h.db.Exec(ctx, `
		INSERT INTO agent_circuit_breakers (session_id, breaker_type, threshold, current_count)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (session_id, breaker_type) DO UPDATE SET current_count = $4, threshold = $3
	`, sessionID, string(breakerType), threshold, count)

	if err != nil {
		return fmt.Errorf("circuit: upsert: %w", err)
	}
	return nil
}

// tripBreaker persists the tripped state with a timestamp.
func (h *Harness) tripBreaker(ctx context.Context, sessionID string, breakerType BreakerType, count int) error {
	if h.db == nil {
		return fmt.Errorf("circuit: no database configured")
	}

	now := time.Now()
	err := h.db.Exec(ctx, `
		INSERT INTO agent_circuit_breakers (session_id, breaker_type, threshold, current_count, tripped_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (session_id, breaker_type) DO UPDATE SET current_count = $4, tripped_at = $5
	`, sessionID, string(breakerType), count, count, now)

	if err != nil {
		return fmt.Errorf("circuit: trip: %w", err)
	}

	slog.Info("harness: circuit breaker tripped and persisted",
		"session_id", sessionID,
		"type", string(breakerType),
		"count", count,
		"tripped_at", now,
	)
	return nil
}

// ============================================================================
// Circuit Breaker Reset (Admin Operation)
// ============================================================================

// maxConsecutiveErrors returns the configured consecutive-errors threshold,
// falling back to the spec default of 3 when unset (SPEC-006 §Circuit
// Breakers). The value comes from harness.max_consecutive_errors config,
// wired onto the Harness at startup (DOGFOOD-003).
func (h *Harness) maxConsecutiveErrors() int {
	if h.MaxConsecutiveErrors <= 0 {
		return 3
	}
	return h.MaxConsecutiveErrors
}

// currentBreakerCount reads the persisted current_count for a breaker row.
// Returns 0 when no row exists or the read fails — the counter is
// best-effort; CheckCircuitBreaker re-persists the count on every call.
func (h *Harness) currentBreakerCount(ctx context.Context, sessionID string, breakerType BreakerType) int {
	if h.db == nil {
		return 0
	}
	rows, err := h.db.Query(ctx,
		`SELECT current_count FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(breakerType))
	if err != nil || len(rows) == 0 {
		return 0
	}
	return toInt(rows[0]["current_count"])
}

// resetConsecutiveErrors clears the consecutive-errors counter after a
// successful planning commit so stale failures don't accumulate across
// successful runs (the breaker counts CONSECUTIVE errors). No-op when no row
// exists or the count is already 0.
func (h *Harness) resetConsecutiveErrors(ctx context.Context, sessionID string) {
	if h.db == nil {
		return
	}
	if err := h.db.Exec(ctx, `
		UPDATE agent_circuit_breakers SET current_count = 0
		WHERE session_id = $1 AND breaker_type = $2 AND current_count <> 0
	`, sessionID, string(BreakerConsecutiveErrors)); err != nil {
		slog.Warn("harness: failed to reset consecutive-errors counter", "session_id", sessionID, "error", err)
	}
}

// ResetCircuitBreaker clears a tripped circuit breaker, enabling the session
// to resume execution. This is an admin operation — not available to agents.
func (h *Harness) ResetCircuitBreaker(ctx context.Context, sessionID string, breakerType BreakerType) error {
	if h.db == nil {
		return fmt.Errorf("circuit: no database configured")
	}

	err := h.db.Exec(ctx, `
		UPDATE agent_circuit_breakers
		SET current_count = 0, tripped_at = NULL, reset_at = $1
		WHERE session_id = $2 AND breaker_type = $3
	`, time.Now(), sessionID, string(breakerType))

	if err != nil {
		return fmt.Errorf("circuit: reset: %w", err)
	}

	slog.Info("harness: circuit breaker reset", "session_id", sessionID, "type", string(breakerType))
	return nil
}

// ============================================================================
// Backward Compatability — checkCircuitBreaker (original form)
// ============================================================================

// checkCircuitBreaker checks whether the session has exceeded its error budget.
// This is the original checkCircuitBreaker preserved for backward compatibility.
// New code should use CheckCircuitBreaker directly.
func (h *Harness) checkCircuitBreaker(ctx context.Context, sessionID string, consecutiveErrors, maxConsecutiveErrors int) bool {
	tripped, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, consecutiveErrors, maxConsecutiveErrors)
	if err != nil {
		slog.Error("harness: circuit breaker check failed — tripping as fail-safe",
			"session_id", sessionID,
			"error", err,
		)
		return true // fail-closed: if we can't verify safety, stop execution
	}
	return tripped
}
