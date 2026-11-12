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
