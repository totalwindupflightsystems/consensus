// Package harness: session-claim lease tests (REVIEW-CONSENSUS-1).
package harness

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/tools"
)

func seedClaimedSession(t *testing.T, th *testHarness, status string, heartbeat time.Time) string {
	t.Helper()
	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		UPDATE sessions SET status = $1, heartbeat_at = $2 WHERE id = $3
	`, status, heartbeat.UTC().Format(time.RFC3339), sessionID); err != nil {
		t.Fatalf("seed claimed session: %v", err)
	}
	return sessionID
}

func claimedSessionStatus(t *testing.T, th *testHarness, sessionID string) string {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query claimed session: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("claimed session row count = %d, want 1", len(rows))
	}
	return toString(rows[0]["status"])
}

func TestSessionClaimLeaseExpiry(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-5*time.Minute))
	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the one expired session claim", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "thinking" {
		t.Fatalf("expired session status = %q, want thinking so the normal dispatcher can retry", status)
	}
}

func TestSessionClaimLeaseRenewal(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = time.Hour

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now())
	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 0 {
		t.Fatalf("reaper reclaimed %d claims; a fresh session lease must survive", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "tool_exec" {
		t.Fatalf("fresh session status = %q, want tool_exec", status)
	}
}

// ch:trace row=DF-CONSENSUS-68 spec=none:product-contract wave=none:serial test=internal/harness/session_claim_lease_test.go::TestSessionClaimExpiryRequeuesToolAndWakesDispatcher doc=none evidence=internal/harness/session_claim_reaper_integration_test.go::TestSessionClaimReaperRealBinaryRecoversKilledWorker witness=none:live-probe-not-run-by-worker verdict=none:foreman-judge-required commit=none:foreman-will-bind memory=none:foreman-will-write
// STATUS: partial
// TIER: T1
// EVIDENCE: internal/harness/session_claim_lease_test.go::TestSessionClaimExpiryRequeuesToolAndWakesDispatcher, ::TestSessionClaimExpiryResumesPendingToolRequests, ::TestSessionClaimExpiryHandsBackToPlannerWhenToolWorkIsDone, ::TestSessionClaimExpiryParksUnclaimableToolWork and internal/harness/session_claim_reaper_integration_test.go::TestSessionClaimReaperRealBinaryRecoversKilledWorker (real production binary)
// UNMET: the 8-minute live real-provider wedge repro (witness) was not run by this worker; the acceptance criterion stands unweakened and belongs to the foreman/live host.
// TestSessionClaimExpiryRequeuesToolAndWakesDispatcher proves recovery at the
// hand-off boundary: a worker can die after the tool executor claimed a request
// but before it completed it. Reaping must make that request claimable again,
// hand the session to the executor (the owner of outstanding tool work) instead
// of the planner, and allow the real executor to complete it — which is what
// wakes the planner for the continuation turn.
func TestSessionClaimExpiryRequeuesToolAndWakesDispatcher(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-5*time.Minute))
	// DF-CONSENSUS-077: a go_native tool only completes if its handler_ref is
	// registered — the dispatch fails closed otherwise.
	tools.RegisterNativeHandler("RecoveryHandler", func(_ context.Context, params map[string]any, workspace string) (string, error) {
		return fmt.Sprintf("RecoveryHandler ran with %d params in %s", len(params), workspace), nil
	})
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ('recovery-tool', 'recovery_tool', 'claim expiry recovery tool', 'external', 'go_native', 'RecoveryHandler', 1)
	`); err != nil {
		t.Fatalf("register recovery tool: %v", err)
	}
	requestID := seedPendingToolRequest(t, th.conn, sessionID, "recovery_tool")
	if err := th.conn.Exec(th.ctx,
		`UPDATE tool_requests SET status = 'executing', executed_at = $1 WHERE id = $2`,
		time.Now().Add(-5*time.Minute).UTC().Format(time.RFC3339), requestID); err != nil {
		t.Fatalf("model claimed tool request: %v", err)
	}
	toolWaker := &toolWakeRecorder{}
	th.ToolExecutor = toolWaker

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the expired session claim", reaped)
	}
	// Outstanding tool work means the EXECUTOR owns the next step. Handing the
	// session to the planner here is the wedge: the continuation turn runs
	// without the tool result and the executor's hand-back guard then no-ops.
	if status := claimedSessionStatus(t, th, sessionID); status != "tool_exec" {
		t.Fatalf("expired session status = %q, want tool_exec while a tool request is outstanding", status)
	}
	expectNoPlannerWake(t, th, "reaped session with outstanding tool work")
	if got := toolWaker.count(); got != 1 {
		t.Fatalf("executor wake count = %d, want 1 so the requeued request is claimed now", got)
	}

	requestRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query recovered tool request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "pending" {
		t.Fatalf("recovered tool_request status = %q, want pending", got)
	}

	plannerWakes := runRecoveredExecutor(t, th, sessionID, requestID)
	if len(plannerWakes) != 1 || plannerWakes[0] != sessionID {
		t.Fatalf("planner wakes after hand-back = %v, want [%s]", plannerWakes, sessionID)
	}
}

// TestSessionClaimExpiryPreservesApprovalPause ensures stale-claim recovery
// does not turn an approval pause into an executable tool request or wake.
func TestSessionClaimExpiryPreservesApprovalPause(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "paused", time.Now().Add(-5*time.Minute))
	requestID := seedPendingToolRequest(t, th.conn, sessionID, "approval_tool")
	if err := th.conn.Exec(th.ctx, `
		UPDATE tool_requests SET status = 'awaiting_approval'
		WHERE id = $1
	`, requestID); err != nil {
		t.Fatalf("park approval-gated request: %v", err)
	}

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 0 {
		t.Fatalf("reaper reclaimed %d claims, want no paused approval claim", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "paused" {
		t.Fatalf("approval-paused session status = %q, want paused", status)
	}
	requestRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query approval-gated request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "awaiting_approval" {
		t.Fatalf("approval-gated request status = %q, want awaiting_approval", got)
	}
	select {
	case got := <-th.wakeCh:
		t.Fatalf("approval-paused session was woken as %q", got)
	default:
	}
}

// ch:trace row=DF-CONSENSUS-68 spec=none:product-contract wave=none:serial test=internal/harness/session_claim_lease_test.go::TestSessionClaimExpiryResumesPendingToolRequests doc=none evidence=internal/harness/session_claim_reaper_integration_test.go::TestSessionClaimReaperRealBinaryRecoversKilledWorker witness=none:live-probe-not-run-by-worker verdict=none:foreman-judge-required commit=none:foreman-will-bind memory=none:foreman-will-write
// TestSessionClaimExpiryResumesPendingToolRequests covers the shape the live
// wedge actually produced. The DF-65 producer fix commits the tool_requests
// row as 'pending' atomically BEFORE the session switches to tool_exec, so the
// request a dying worker never reached is still 'pending' — never 'executing' —
// when the claim lease expires. Recovery therefore must not depend on the
// request having been claimed:
//
//  1. the row stays claimable,
//  2. the session must NOT be parked ('paused' is the measured failure: it
//     rested there at iteration 1 with the requested file never created),
//  3. the session must stay with the EXECUTOR, which owns outstanding tool
//     work — handing a 'thinking' session to the planner runs a continuation
//     turn with no tool result and the model simply re-issues the call,
//  4. the executor is woken immediately rather than up to a poll interval
//     later, and
//  5. when the tool result lands the session is handed back to the planner and
//     the planner is woken for the continuation turn.
func TestSessionClaimExpiryResumesPendingToolRequests(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-5*time.Minute))
	// DF-CONSENSUS-077: a go_native tool only runs if its handler_ref is
	// registered — the dispatch fails closed otherwise.
	registerRecoveryTool(t, th, "pending-recovery-tool", "pending_recovery_tool",
		"PendingRecoveryHandler", func(_ context.Context, params map[string]any, workspace string) (string, error) {
			return fmt.Sprintf("PendingRecoveryHandler ran with %d params in %s", len(params), workspace), nil
		})
	// DF-65 producer shape: the row is committed as 'pending' before the
	// session status moves to tool_exec, so nothing ever claimed it.
	requestID := seedPendingToolRequest(t, th.conn, sessionID, "pending_recovery_tool")
	toolWaker := &toolWakeRecorder{}
	th.ToolExecutor = toolWaker

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the expired session claim", reaped)
	}

	status := claimedSessionStatus(t, th, sessionID)
	if status == "paused" {
		t.Fatalf("expired session status = %q; recovery must not park the session while a tool request is resumable", status)
	}
	if status != "tool_exec" {
		t.Fatalf("expired session status = %q, want tool_exec: the executor owns a session with outstanding tool work", status)
	}
	expectNoPlannerWake(t, th, "reaped session with a pending tool request")
	if got := toolWaker.count(); got != 1 {
		t.Fatalf("executor wake count = %d, want 1 so the pending request is claimed now", got)
	}

	requestRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query recovered tool request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "pending" {
		t.Fatalf("recovered pending tool_request status = %q, want pending (claimable)", got)
	}

	plannerWakes := runRecoveredExecutor(t, th, sessionID, requestID)
	if len(plannerWakes) != 1 || plannerWakes[0] != sessionID {
		t.Fatalf("planner wakes after hand-back = %v, want [%s]", plannerWakes, sessionID)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "thinking" {
		t.Fatalf("session status after the tool result landed = %q, want thinking (continuation turn is dispatchable)", status)
	}
}

// TestSessionClaimExpiryHandsBackToPlannerWhenToolWorkIsDone is the negative
// control for the ownership decision: a dead worker can also die AFTER its tool
// result landed. Nothing is outstanding then, so recovery must go straight to
// the planner (continuation turn carries the result) and must not fire an
// executor wake that no request needs.
func TestSessionClaimExpiryHandsBackToPlannerWhenToolWorkIsDone(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-5*time.Minute))
	requestID := seedPendingToolRequest(t, th.conn, sessionID, "finished_tool")
	if err := th.conn.Exec(th.ctx,
		`UPDATE tool_requests SET status = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = $1`,
		requestID); err != nil {
		t.Fatalf("settle tool request: %v", err)
	}
	toolWaker := &toolWakeRecorder{}
	th.ToolExecutor = toolWaker

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the expired session claim", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "thinking" {
		t.Fatalf("expired session status = %q, want thinking when no tool work is outstanding", status)
	}
	select {
	case got := <-th.wakeCh:
		if got != sessionID {
			t.Fatalf("planner wake session = %q, want %q", got, sessionID)
		}
	default:
		t.Fatal("recovered session with no outstanding tool work was not woken for the continuation turn")
	}
	if got := toolWaker.count(); got != 0 {
		t.Fatalf("executor wake count = %d, want 0: nothing is claimable", got)
	}
}

// TestSessionClaimExpiryParksUnclaimableToolWork bounds recovery: a request
// nothing can claim (no live executor, no registered handler) must not re-arm
// the session at the lease cadence forever. Past the recovery window the
// session is parked for an operator — the pre-DF-CONSENSUS-68 behaviour — and
// nothing is woken.
func TestSessionClaimExpiryParksUnclaimableToolWork(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-2*time.Hour))
	requestID := seedPendingToolRequest(t, th.conn, sessionID, "unclaimable_tool")
	if err := th.conn.Exec(th.ctx,
		`UPDATE tool_requests SET created_at = datetime('now', '-45 minutes') WHERE id = $1`,
		requestID); err != nil {
		t.Fatalf("age tool request: %v", err)
	}
	toolWaker := &toolWakeRecorder{}
	th.ToolExecutor = toolWaker

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the expired session claim", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "paused" {
		t.Fatalf("session with unclaimable tool work = %q, want paused after the recovery window", status)
	}
	expectNoPlannerWake(t, th, "parked session with unclaimable tool work")
	if got := toolWaker.count(); got != 0 {
		t.Fatalf("executor wake count = %d, want 0: the request is presumed unclaimable", got)
	}
	requestRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query parked tool request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "pending" {
		t.Fatalf("parked tool_request status = %q, want pending (unchanged)", got)
	}
}

// ============================================================================
// DF-CONSENSUS-68 test helpers
// ============================================================================

// toolWakeRecorder is a ToolExecutor stub that counts Wake() calls so the
// reaper's ownership decision is observable without starting a poll loop.
type toolWakeRecorder struct {
	mu    sync.Mutex
	wakes int
}

func (w *toolWakeRecorder) PollOnce(context.Context) (int, error) { return 0, nil }
func (w *toolWakeRecorder) Start(context.Context)                 {}
func (w *toolWakeRecorder) Stop()                                 {}
func (w *toolWakeRecorder) Wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wakes++
}

func (w *toolWakeRecorder) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wakes
}

// registerRecoveryTool registers a go_native tool and its handler so the real
// executor can dispatch it (DF-CONSENSUS-077 fails closed without both).
func registerRecoveryTool(t *testing.T, th *testHarness, id, name, handlerRef string,
	handler func(context.Context, map[string]any, string) (string, error)) {
	t.Helper()
	tools.RegisterNativeHandler(handlerRef, handler)
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ($1, $2, $3, 'external', 'go_native', $4, 1)
	`, id, name, name+" claim expiry recovery tool", handlerRef); err != nil {
		t.Fatalf("register recovery tool: %v", err)
	}
}

// expectNoPlannerWake fails when the reaper handed a recovered session to the
// planner, which would run a continuation turn before its tool work ran.
func expectNoPlannerWake(t *testing.T, th *testHarness, context string) {
	t.Helper()
	select {
	case got := <-th.wakeCh:
		t.Fatalf("%s: session %q was woken for planning, want the executor to own this step", context, got)
	default:
	}
}

// runRecoveredExecutor drives the recovered hand-off to completion with the
// real tool executor and returns the session wake signals the executor
// produced when it handed the session back to the planner.
func runRecoveredExecutor(t *testing.T, th *testHarness, sessionID, requestID string) []string {
	t.Helper()
	var wakes []string
	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	executor.SetSessionWake(func(sid string) { wakes = append(wakes, sid) })

	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("PollOnce recovered request: %v", err)
	}
	if processed != 1 {
		t.Fatalf("PollOnce processed %d recovered requests, want 1", processed)
	}

	rows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query completed tool request: %v", err)
	}
	if got := toString(rows[0]["status"]); got != "completed" {
		t.Fatalf("recovered tool_request final status = %q, want completed", got)
	}
	resultRows, err := th.conn.Query(th.ctx, `SELECT output FROM tool_results WHERE request_id = $1`, requestID)
	if err != nil {
		t.Fatalf("query recovered tool result: %v", err)
	}
	if len(resultRows) != 1 {
		t.Fatalf("recovered tool result rows = %d, want 1 (the tool never ran)", len(resultRows))
	}
	if out := toString(resultRows[0]["output"]); out == "" {
		t.Fatalf("recovered tool result output is empty; the registered go_native handler did not run")
	}
	return wakes
}
