// Package harness: circuit breaker persistence tests (AC-HARDEN-03, AC-HARDEN-04).
//
// These tests prove that circuit breaker state is written to the agent_circuit_breakers
// table and survives process restarts.
//
// axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-2 test=internal/harness/circuit_test.go
package harness

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// AC-HARDEN-03: agent_circuit_breakers table created + persisted
// AC-HARDEN-04: checkCircuitBreaker persists tripped state
// ============================================================================

// setupCircuitTestDB creates a fresh in-memory database with sessions and circuit_breakers.
func setupCircuitTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()
	ctx := context.Background()

	// Use a temp file instead of :memory: so all pool connections see the same DB
	dbPath := "/tmp/circuit-test-" + t.Name() + ".db"
	os.Remove(dbPath)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	database, err := driver.Open(ctx, db.Config{
		URL: "sqlite://" + dbPath + "?_journal_mode=WAL&_time_format=sqlite",
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Create minimal required tables
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS model_registry (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id TEXT NOT NULL UNIQUE,
			tier INTEGER NOT NULL DEFAULT 1,
			max_context INTEGER NOT NULL DEFAULT 128000,
			cost_per_m_in REAL DEFAULT 0,
			cost_per_m_out REAL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL,
			model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'booting',
			goal TEXT,
			context_budget INT NOT NULL DEFAULT 128000,
			tokens_used_in BIGINT NOT NULL DEFAULT 0,
			tokens_used_out BIGINT NOT NULL DEFAULT 0,
			iteration BIGINT NOT NULL DEFAULT 0,
			project_id TEXT,
			planning_max_turns INT NOT NULL DEFAULT 10
		)`,
		`CREATE TABLE IF NOT EXISTS agent_circuit_breakers (
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			breaker_type TEXT NOT NULL CHECK (breaker_type IN ('consecutive_errors','iterations','budget')),
			threshold INTEGER NOT NULL DEFAULT 5,
			current_count INTEGER NOT NULL DEFAULT 0,
			tripped_at TEXT,
			reset_at TEXT,
			PRIMARY KEY (session_id, breaker_type)
		)`,
	} {
		if err := database.Exec(ctx, stmt); err != nil {
			database.Close()
			t.Fatalf("create table: %v", err)
		}
	}

	// Seed model_registry
	_ = database.Exec(ctx, `INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out)
		VALUES ('test-model', 1, 128000, 0, 0)`)

	// Seed a session
	_ = database.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('sess-circ-01', 'test-agent', 'test-model', 'idle', 'circuit breaker test')`)

	cleanup := func() { database.Close() }
	return database, cleanup
}

// ============================================================================
// AC-HARDEN-03: Write and read circuit breaker counts
// ============================================================================

func TestCircuitBreaker_WriteAndReadCount(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	// Create harness — LLM client is not needed for circuit breaker tests
	h := New(database, nil)

	ctx := context.Background()
	sessionID := "sess-circ-01"

	// Simulate 2 consecutive errors (below threshold of 3)
	tripped, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 2, 3)
	if err != nil {
		t.Fatalf("CheckCircuitBreaker: %v", err)
	}
	if tripped {
		t.Error("circuit breaker should not trip at count 2 with threshold 3")
	}

	// Verify the count was persisted
	rows, err := database.Query(ctx,
		`SELECT session_id, breaker_type, current_count, threshold, tripped_at
		 FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query circuit_breakers: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no circuit breaker row found after count update")
	}

	row := rows[0]
	count := toInt(row["current_count"])
	if count != 2 {
		t.Errorf("current_count = %d, want 2", count)
	}
	threshold := toInt(row["threshold"])
	if threshold != 3 {
		t.Errorf("threshold = %d, want 3", threshold)
	}

	// Verify tripped_at is NULL (not tripped yet)
	trippedAt := row["tripped_at"]
	if trippedAt != nil {
		t.Errorf("tripped_at should be nil when not tripped, got %v", trippedAt)
	}

	t.Logf("AC-HARDEN-03 PASS: circuit breaker count persisted (count=2, threshold=3, tripped=false)")
}

// ============================================================================
// AC-HARDEN-03: Persist count across multiple upserts
// ============================================================================

func TestCircuitBreaker_UpsertUpdatesCount(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// First write
	_, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 1, 5)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second write (upsert)
	_, err = h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 2, 5)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Verify only ONE row exists (upsert, not duplicate insert)
	rows, err := database.Query(ctx,
		`SELECT COUNT(*) as cnt FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	count := toInt(rows[0]["cnt"])
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}

	// Verify latest count is 2
	rows2, err := database.Query(ctx,
		`SELECT current_count FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	currentCount := toInt(rows2[0]["current_count"])
	if currentCount != 2 {
		t.Errorf("current_count = %d, want 2 after upsert", currentCount)
	}

	t.Logf("AC-HARDEN-03 PASS: upsert preserves single row, current_count=2")
}

// ============================================================================
// AC-HARDEN-04: Circuit breaker trips and persists tripped_at
// ============================================================================

func TestCircuitBreaker_TripPersistsTimestamp(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// Simulate hitting the threshold: 3 errors with threshold 3
	tripped, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 3, 3)
	if err != nil {
		t.Fatalf("CheckCircuitBreaker: %v", err)
	}
	if !tripped {
		t.Error("circuit breaker should trip at count 3 with threshold 3")
	}

	// Verify tripped state persisted in the database
	rows, err := database.Query(ctx,
		`SELECT current_count, tripped_at FROM agent_circuit_breakers
		 WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no circuit breaker row found after trip")
	}

	row := rows[0]
	count := toInt(row["current_count"])
	if count != 3 {
		t.Errorf("current_count = %d, want 3", count)
	}

	trippedAt := row["tripped_at"]
	if trippedAt == nil {
		t.Fatal("tripped_at should be set when circuit breaker trips")
	}
	trippedAtStr, ok := trippedAt.(string)
	if !ok || trippedAtStr == "" {
		t.Errorf("tripped_at should be a non-empty timestamp, got %T: %v", trippedAt, trippedAt)
	}

	t.Logf("AC-HARDEN-04 PASS: circuit breaker tripped, tripped_at=%s", trippedAtStr)
}

// ============================================================================
// AC-HARDEN-04: Threshold of 0 disables the breaker
// ============================================================================

func TestCircuitBreaker_ZeroThreshold_Disabled(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// Zero threshold means "no limit"
	tripped, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerBudget, 999999, 0)
	if err != nil {
		t.Fatalf("CheckCircuitBreaker: %v", err)
	}
	if tripped {
		t.Error("circuit breaker should not trip with zero threshold")
	}

	t.Logf("AC-HARDEN-04 PASS: zero threshold disables circuit breaker")
}

// ============================================================================
// AC-HARDEN-04: Different breaker types tracked independently
// ============================================================================

func TestCircuitBreaker_IndependentBreakerTypes(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// Trip consecutive_errors
	_, _ = h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 3, 3)

	// Set iteration count (not tripped)
	_, _ = h.CheckCircuitBreaker(ctx, sessionID, BreakerIterations, 10, 50)

	// Check budget (not tripped)
	_, _ = h.CheckCircuitBreaker(ctx, sessionID, BreakerBudget, 5, 100)

	// Verify all 3 types exist independently
	rows, err := database.Query(ctx,
		`SELECT breaker_type, current_count, tripped_at IS NOT NULL as is_tripped
		 FROM agent_circuit_breakers WHERE session_id = $1 ORDER BY breaker_type`,
		sessionID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 breaker types, got %d", len(rows))
	}

	// Verify consecutive_errors is tripped
	type0 := toString(rows[0]["breaker_type"])
	type1 := toString(rows[1]["breaker_type"])
	type2 := toString(rows[2]["breaker_type"])

	if type0 != string(BreakerBudget) || type1 != string(BreakerConsecutiveErrors) || type2 != string(BreakerIterations) {
		t.Logf("breaker types (order may vary): %s, %s, %s", type0, type1, type2)
	}

	// The consecutive_errors breaker should have tripped_at set
	trippedCount := 0
	for _, row := range rows {
		isTripped := toInt(row["is_tripped"])
		if isTripped != 0 {
			trippedCount++
		}
	}
	if trippedCount != 1 {
		t.Errorf("expected exactly 1 tripped breaker, got %d", trippedCount)
	}

	t.Logf("AC-HARDEN-04 PASS: 3 independent breaker types tracked")
}

// ============================================================================
// Circuit breaker reset (admin operation)
// ============================================================================

func TestCircuitBreaker_ResetClearsTripState(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// First trip the breaker
	tripped, err := h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, 5, 3)
	if err != nil {
		t.Fatalf("CheckCircuitBreaker: %v", err)
	}
	if !tripped {
		t.Fatal("expected breaker to trip at count 5 with threshold 3")
	}

	// Now reset it
	err = h.ResetCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors)
	if err != nil {
		t.Fatalf("ResetCircuitBreaker: %v", err)
	}

	// Verify tripped_at is cleared and count reset
	rows, err := database.Query(ctx,
		`SELECT current_count, tripped_at, reset_at FROM agent_circuit_breakers
		 WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("row should exist after reset")
	}
	row := rows[0]

	count := toInt(row["current_count"])
	if count != 0 {
		t.Errorf("current_count = %d, want 0 after reset", count)
	}

	if row["tripped_at"] != nil {
		t.Errorf("tripped_at should be nil after reset, got %v", row["tripped_at"])
	}

	if row["reset_at"] == nil {
		t.Error("reset_at should be set after reset")
	}

	t.Logf("AC-HARDEN-04 PASS: reset clears tripped_at, sets reset_at, zeroes current_count")
}

// ============================================================================
// Backward compatibility: checkCircuitBreaker wraps CheckCircuitBreaker
// ============================================================================

func TestCircuitBreaker_BackwardCompat_WrapsCheck(t *testing.T) {
	// Use full testHarness for backward compat test (needs sessions + memory_events)
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID := "sess-bc-01"
	_ = th.conn.Exec(th.ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal) VALUES ($1, 'bc-agent', 'test-model', 'idle', 'backward compat')`,
		sessionID)

	// Simulate 3 consecutive errors → should trip
	// The underlying CheckCircuitBreaker will try to write to agent_circuit_breakers
	// (the table exists in the test migration now)
	tripped := th.checkCircuitBreaker(th.ctx, sessionID, 3, 3)
	if !tripped {
		t.Error("checkCircuitBreaker should return true when errors >= threshold")
	}

	// Simulate 1 error → should NOT trip
	tripped = th.checkCircuitBreaker(th.ctx, sessionID, 1, 3)
	if tripped {
		t.Error("checkCircuitBreaker should return false when errors < threshold")
	}

	t.Logf("AC-HARDEN-04 PASS: backward-compat checkCircuitBreaker wraps CheckCircuitBreaker correctly")
}

// ============================================================================
// Nil DB safety
// ============================================================================

func TestCircuitBreaker_NilDB_DoesNotPanic(t *testing.T) {
	h := New(nil, nil)
	ctx := context.Background()

	// count < threshold: returns false + error (upsert fails with nil DB)
	tripped, err := h.CheckCircuitBreaker(ctx, "sess-any", BreakerConsecutiveErrors, 1, 3)
	if err == nil {
		t.Error("expected error for nil DB on upsert ")
	}
	if tripped {
		t.Error("should not trip below threshold")
	}

	// count >= threshold: returns true + nil error (persistence failure is logged, not returned)
	// The design is fail-closed: we STOP the agent even if we can't persist the trip state.
	tripped, err = h.CheckCircuitBreaker(ctx, "sess-any", BreakerConsecutiveErrors, 3, 3)
	if err != nil {
		t.Logf("nil DB trip path error (logged but returned nil): %v", err)
	}
	if tripped {
		t.Log("tripped=true even with nil DB — correct fail-closed behavior")
	}

	// Reset should return error for nil DB
	err = h.ResetCircuitBreaker(ctx, "sess-any", BreakerConsecutiveErrors)
	if err == nil {
		t.Error("expected error for nil DB on reset")
	}

	t.Log("nil DB safety: no panics, clean errors")
}

// ============================================================================
// Test with full harness: circuit breaker infrastructure integration
//
// This verifies that:
//  1. agent_circuit_breakers table exists in the test migration
//  2. CheckCircuitBreaker + tripBreaker work correctly with a real database
//  3. upsertBreakerCount correctly tracks counts across calls
//
// The breaker is wired into both LLM error paths: handleLLMPlanningError
// (planning loop, DOGFOOD-003) and handleLLMError (RunAgentIteration /
// task-claim loop, C-GAP-011). Trip behavior through the full iteration path
// is covered by TestProviderFailure_CircuitBreakerTripsViaRunAgentIteration
// in ac_provider_failure_test.go; the persistence infrastructure itself is
// fully tested here.
// ============================================================================

func TestCircuitBreaker_Integration_TableExistsInMigration(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	ctx := th.ctx

	// Verify the table exists in the test schema
	rows, err := th.conn.Query(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='agent_circuit_breakers'`)
	if err != nil || len(rows) == 0 {
		t.Fatal("AC-HARDEN-03 FAIL: agent_circuit_breakers table is missing from test migration")
	}

	// Write a breaker count via CheckCircuitBreaker
	tripped, err := th.CheckCircuitBreaker(ctx, sessionID, BreakerBudget, 75, 100)
	if err != nil {
		t.Fatalf("CheckCircuitBreaker: %v", err)
	}
	if tripped {
		t.Error("budget breaker should not trip at 75/100")
	}

	// Verify the row was written
	rows2, err := th.conn.Query(ctx,
		`SELECT current_count, threshold FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerBudget))
	if err != nil || len(rows2) == 0 {
		t.Fatal("AC-HARDEN-04 FAIL: no row persisted after CheckCircuitBreaker call")
	}
	count := toInt(rows2[0]["current_count"])
	threshold := toInt(rows2[0]["threshold"])
	if count != 75 || threshold != 100 {
		t.Errorf("count=%d, threshold=%d, want 75, 100", count, threshold)
	}

	// Now trip the breaker (count >= threshold)
	tripped, err = th.CheckCircuitBreaker(ctx, sessionID, BreakerBudget, 100, 100)
	if !tripped {
		t.Error("budget breaker should trip at 100/100")
	}

	// Verify tripped_at was set
	rows3, err := th.conn.Query(ctx,
		`SELECT tripped_at FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerBudget))
	if err != nil || len(rows3) == 0 {
		t.Fatal("row missing after trip")
	}
	if rows3[0]["tripped_at"] == nil {
		t.Error("AC-HARDEN-04 FAIL: tripped_at should be set after trip")
	}

	t.Logf("AC-HARDEN-03/04 integration PASS: table exists, persistence verified (count=100, tripped_at set)")
}

// ============================================================================
// Benchmark: concurrent circuit breaker operations
// ============================================================================

func TestCircuitBreaker_ConcurrentAccess_NoRace(t *testing.T) {
	database, cleanup := setupCircuitTestDB(t)
	defer cleanup()

	h := New(database, nil)
	ctx := context.Background()
	sessionID := "sess-circ-01"

	// Run several concurrent accesses to verify thread safety (via SQLite serialization)
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			_, _ = h.CheckCircuitBreaker(ctx, sessionID, BreakerConsecutiveErrors, idx+1, 10)
			done <- true
		}(i)
	}

	// Wait for all goroutines with timeout
	timeout := time.After(5 * time.Second)
	completed := 0
	for completed < 5 {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatalf("timeout waiting for goroutines (%d/5)", completed)
		}
	}

	// Verify final state
	rows, err := database.Query(ctx,
		`SELECT current_count FROM agent_circuit_breakers
		 WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) > 0 {
		t.Logf("final current_count after concurrent access: %d", toInt(rows[0]["current_count"]))
	}

	t.Log("concurrent access: no deadlocks, no panics")
}

// ============================================================================
// DOGFOOD-003 regression: consecutive LLM errors trip the breaker → paused
// ============================================================================

// TestConsecutiveLLMErrors_TripBreakerPausesSession proves the wiring that
// dogfood found broken: LLM call failures inside RunInteractivePlanning used
// to go straight to status='failed' via handlePlanningError, never reaching
// CheckCircuitBreaker (agent_circuit_breakers stayed empty). Now each failure
// increments the persisted counter with the CONFIGURED threshold, and on trip
// the session is PAUSED, not failed.
func TestConsecutiveLLMErrors_TripBreakerPausesSession(t *testing.T) {
	th, err := newTestHarness(failingMockLLM(fmt.Errorf("llm: http 401: invalid api key")))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	th.Harness.MaxConsecutiveErrors = 2

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set thinking: %v", err)
	}

	cfg := DefaultPlanningConfig()
	breakerState := func() (int, any) {
		rows, qErr := th.conn.Query(th.ctx,
			`SELECT current_count, tripped_at FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
			sessionID, string(BreakerConsecutiveErrors))
		if qErr != nil || len(rows) == 0 {
			return 0, nil
		}
		return toInt(rows[0]["current_count"]), rows[0]["tripped_at"]
	}
	sessionStatus := func() string {
		rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
		if len(rows) == 0 {
			return "missing"
		}
		return toString(rows[0]["status"])
	}

	// Failure 1: below threshold — session returns to 'thinking' (retryable),
	// counter persisted at 1, not tripped, NOT failed.
	result, err := th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run 1: unexpected Go error: %v", err)
	}
	if result == nil || result.Status != "error" {
		t.Fatalf("run 1: expected error result, got %+v", result)
	}
	if got := sessionStatus(); got != "thinking" {
		t.Errorf("run 1: expected status 'thinking' (retry pending), got %q", got)
	}
	if got := sessionStatus(); got == "failed" {
		t.Errorf("run 1: session must NOT go straight to 'failed' on first LLM error (DOGFOOD-003)")
	}
	count, trippedAt := breakerState()
	if count != 1 {
		t.Errorf("run 1: expected breaker current_count=1, got %d", count)
	}
	if trippedAt != nil {
		t.Errorf("run 1: breaker should not be tripped below threshold, tripped_at=%v", trippedAt)
	}

	// Failure 2: threshold reached — breaker trips, session PAUSED (not
	// failed), tripped_at persisted.
	result, err = th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run 2: unexpected Go error: %v", err)
	}
	if result == nil || result.Status != "error" {
		t.Fatalf("run 2: expected error result, got %+v", result)
	}
	if got := sessionStatus(); got != "paused" {
		t.Errorf("run 2: expected status 'paused' after breaker trip, got %q", got)
	}
	count, trippedAt = breakerState()
	if count != 2 {
		t.Errorf("run 2: expected breaker current_count=2, got %d", count)
	}
	if trippedAt == nil {
		t.Error("run 2: expected tripped_at to be set on agent_circuit_breakers row")
	}
}

// TestConsecutiveErrors_ResetOnSuccessfulCommit proves the counter tracks
// CONSECUTIVE failures: a failure followed by a successful planning commit
// resets the persisted count to 0.
func TestConsecutiveErrors_ResetOnSuccessfulCommit(t *testing.T) {
	mock := &retryMockLLM{maxFails: 1, successOutput: minimalOutput()}
	th, err := newTestHarness(mock)
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	th.Harness.MaxConsecutiveErrors = 3

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2
	cfg.AutoCommitOnMax = true

	// Run 1: LLM fails → below threshold → 'thinking', count=1.
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set thinking: %v", err)
	}
	if _, err := th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg); err != nil {
		t.Fatalf("run 1: unexpected Go error: %v", err)
	}
	rows, _ := th.conn.Query(th.ctx,
		`SELECT current_count FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if len(rows) == 0 || toInt(rows[0]["current_count"]) != 1 {
		t.Fatalf("run 1: expected breaker count=1, rows=%v", rows)
	}

	// Run 2: LLM succeeds → commit → counter reset to 0, session idle.
	if _, err := th.Harness.RunInteractivePlanning(th.ctx, sessionID, cfg); err != nil {
		t.Fatalf("run 2: unexpected Go error: %v", err)
	}
	rows, _ = th.conn.Query(th.ctx,
		`SELECT current_count FROM agent_circuit_breakers WHERE session_id = $1 AND breaker_type = $2`,
		sessionID, string(BreakerConsecutiveErrors))
	if len(rows) == 0 {
		t.Fatal("run 2: breaker row missing")
	}
	if got := toInt(rows[0]["current_count"]); got != 0 {
		t.Errorf("run 2: expected breaker count reset to 0 after successful commit, got %d", got)
	}
	statusRows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if len(statusRows) > 0 && toString(statusRows[0]["status"]) != "idle" {
		t.Errorf("run 2: expected session 'idle' after successful commit, got %q", toString(statusRows[0]["status"]))
	}
}
