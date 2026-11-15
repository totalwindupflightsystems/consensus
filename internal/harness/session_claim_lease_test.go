// Package harness: session-claim lease tests (REVIEW-CONSENSUS-1).
package harness

import (
	"testing"
	"time"
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
// EVIDENCE: internal/harness/session_claim_lease_test.go::TestSessionClaimExpiryRequeuesToolAndWakesDispatcher (requeue + wake + claim/execute after expiry, RED-proven pre-fix) and internal/harness/session_claim_reaper_integration_test.go::TestSessionClaimReaperRealBinaryRecoversKilledWorker (real production binary: recovered tool_request reaches terminal status completed, session reaches idle)
// UNMET: the 8-minute live real-provider wedge repro (witness) was not run by this worker; the acceptance criterion stands unweakened and belongs to the foreman/live host.
// TestSessionClaimExpiryRequeuesToolAndWakesDispatcher proves recovery at the
// hand-off boundary: a worker can die after the tool executor claimed a request
// but before it completed it. Reaping must make that request claimable again,
// wake the normal dispatcher, and allow the real executor to complete it.
func TestSessionClaimExpiryRequeuesToolAndWakesDispatcher(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	th.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	sessionID := seedClaimedSession(t, th, "tool_exec", time.Now().Add(-5*time.Minute))
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

	reaped, err := th.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaper reclaimed %d claims, want the expired session claim", reaped)
	}
	if status := claimedSessionStatus(t, th, sessionID); status != "thinking" {
		t.Fatalf("expired session status = %q, want thinking", status)
	}

	requestRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query recovered tool request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "pending" {
		t.Fatalf("recovered tool_request status = %q, want pending", got)
	}

	select {
	case got := <-th.wakeCh:
		if got != sessionID {
			t.Fatalf("wake session = %q, want %q", got, sessionID)
		}
	default:
		t.Fatal("expired session was not woken for redispatch")
	}

	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	processed, err := executor.PollOnce(th.ctx)
	if err != nil {
		t.Fatalf("PollOnce recovered request: %v", err)
	}
	if processed != 1 {
		t.Fatalf("PollOnce processed %d recovered requests, want 1", processed)
	}
	requestRows, err = th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, requestID)
	if err != nil {
		t.Fatalf("query completed tool request: %v", err)
	}
	if got := toString(requestRows[0]["status"]); got != "completed" {
		t.Fatalf("recovered tool_request final status = %q, want completed", got)
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
