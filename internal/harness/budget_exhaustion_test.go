// Package harness: strict budget exhaustion test (Phase 4 — Hardened Testing).
//
// Verifies that:
//  1. With budget_limit_cents=1 ($0.01), an LLM call is allowed when cost is below limit
//  2. After the cumulative cost exceeds the limit, the agent returns "blocked" and pauses
//  3. A $0 limit (no limit set) allows calls regardless of cost
//
// axiom:trace spec=specs/006-transactions.md task=phase-4-budget-exhaustion
package harness

import (
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/billing"
)

// ============================================================================
// Budget Exhaustion — Agent Stops LLM Calls When Budget Exceeded
// ============================================================================

// TestBudgetExhaustion_StrictLimit verifies that when a session has a $0.01
// budget and the cumulative cost exceeds it, the agent stops making LLM calls.
func TestBudgetExhaustion_StrictLimit(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Wire a real BillingTracker (not nil — budget enforcement requires it)
	th.Harness.BillingTracker = billing.NewTracker(th.conn)

	// Create session with $0.01 budget limit (1 cent)
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal, budget_limit_cents)
		VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'Budget exhaustion test', 1)
	`, sessionID)
	if err != nil {
		t.Fatalf("create session with budget limit: %v", err)
	}

	// Record billing at $0.02 (2 cents) — exceeds the $0.01 limit
	tracker := th.Harness.BillingTracker
	tracker.RecordBilling(th.ctx, sessionID, 1, "test-model", "cognition", 5000, 1000, 0, 0, 0.02)

	// Run iteration — should be blocked because budget is exceeded
	result, err := th.Harness.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAgentIteration returned unexpected error: %v", err)
	}

	if result.Status != "blocked" {
		t.Errorf("expected status 'blocked', got %q", result.Status)
	}
	if result.NextStatus != "paused" {
		t.Errorf("expected next_status 'paused', got %q", result.NextStatus)
	}
	if result.ErrorInjected == "" {
		t.Error("expected error message about budget exceeded, got empty")
	}
	if !strings.Contains(result.ErrorInjected, "budget exceeded") {
		t.Errorf("error message should mention budget exceeded: got %q", result.ErrorInjected)
	}

	// Verify session status was changed to paused
	rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if len(rows) > 0 {
		status := toString(rows[0]["status"])
		if status != "paused" {
			t.Errorf("expected session paused, got status %q", status)
		}
	}

	t.Logf("PASS: budget exhaustion blocks agent — %s", result.ErrorInjected)
}

// TestBudgetExhaustion_BelowLimit verifies that when the budget has NOT been
// exceeded, the agent proceeds normally.
func TestBudgetExhaustion_BelowLimit(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	th.Harness.BillingTracker = billing.NewTracker(th.conn)

	// Use same session ID as minimalOutput() so mock memory events reference live session
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal, budget_limit_cents)
		VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'Budget under limit test', 100)
	`, sessionID)
	if err != nil {
		t.Fatalf("create session with large budget: %v", err)
	}

	// Record billing at $0.01 — well under the $1.00 limit
	th.Harness.BillingTracker.RecordBilling(th.ctx, sessionID, 1, "test-model", "cognition", 5000, 1000, 0, 0, 0.01)

	// Run iteration — should NOT be blocked (budget not exceeded)
	result, err := th.Harness.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAgentIteration returned unexpected error: %v", err)
	}

	if result.Status == "blocked" {
		t.Errorf("expected iteration to proceed, got status 'blocked': %s", result.ErrorInjected)
	}
	// Should be "completed" or similar (mock LLM returns minimal output)
	t.Logf("PASS: budget under limit allows iteration — status: %s", result.Status)
}

// TestBudgetExhaustion_ZeroLimit verifies that a budget_limit_cents of 0
// (no limit) allows all calls regardless of cumulative cost.
func TestBudgetExhaustion_ZeroLimit(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	th.Harness.BillingTracker = billing.NewTracker(th.conn)

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal, budget_limit_cents)
		VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'No budget limit test', 0)
	`, sessionID)
	if err != nil {
		t.Fatalf("create session with zero budget: %v", err)
	}

	// Record billing at $100.00 — massive cost, but limit is 0 (no limit)
	th.Harness.BillingTracker.RecordBilling(th.ctx, sessionID, 1, "test-model", "cognition", 500000, 100000, 0, 0, 100.00)

	// Run iteration — should NOT be blocked (0 = no limit)
	result, err := th.Harness.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAgentIteration returned unexpected error: %v", err)
	}

	if result.Status == "blocked" {
		t.Errorf("expected iteration to proceed with zero limit, got status 'blocked': %s", result.ErrorInjected)
	}
	t.Logf("PASS: zero budget limit allows unlimited spending — status: %s", result.Status)
}

// TestBudgetExhaustion_NoBillingTracker verifies that when BillingTracker is
// nil (not configured), the agent still runs normally without budget checks.
func TestBudgetExhaustion_NoBillingTracker(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// createTestSession creates session with ID aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1
	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Set a strict budget limit — but BillingTracker is nil, so it won't be enforced
	th.conn.Exec(th.ctx, `UPDATE sessions SET budget_limit_cents = 1 WHERE id = $1`, sessionID)

	// Run iteration — should NOT be blocked (no billing tracker = no enforcement)
	result, err := th.Harness.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAgentIteration returned unexpected error: %v", err)
	}

	if result.Status == "blocked" {
		t.Errorf("expected iteration to proceed without billing tracker, got status 'blocked': %s", result.ErrorInjected)
	}
	t.Logf("PASS: nil billing tracker skips enforcement — status: %s", result.Status)
}
