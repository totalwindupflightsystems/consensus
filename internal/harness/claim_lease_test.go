// Package harness: claim-lease visibility timeout tests (REVIEW-CONSENSUS-1).
//
// The reaper resets 'in_progress' tasks whose claimed_at lease has expired
// back to 'pending', so a worker that dies mid-task cannot block its row
// forever. These tests drive the real reaper (runClaimReaper's reap pass)
// against the real SQLite test migration.
package harness

import (
	"context"
	"sync"
	"testing"
	"time"
)

// stubLLM is a no-op LLM client — the reaper path never calls the LLM.
type stubLLM struct {
	mu       sync.Mutex
	callFrom *time.Time
}

func (s *stubLLM) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &LLMResponse{Output: &AgentOutput{}, ModelID: "test-model"}, nil
}

// seedClaimedTask inserts a session plus one task in status 'in_progress'
// with the given claimed_at anchor, mirroring a claim made by
// ClaimNextReadyTask (executor.go stamps claimed_at in RFC3339 UTC).
func seedClaimedTask(t *testing.T, th *testHarness, taskID string, claimedAt time.Time) {
	t.Helper()
	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO tasks (id, session_id, title, description, status, claimed_at)
		VALUES ($1, $2, 'lease task', 'claim lease visibility timeout test', 'in_progress', $3)
	`, taskID, sessionID, claimedAt.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert claimed task: %v", err)
	}
}

// taskState returns (status, claimed_at raw value) for the given task.
func taskState(t *testing.T, th *testHarness, taskID string) (string, any) {
	t.Helper()
	rows, err := th.conn.Query(th.ctx, `SELECT status, claimed_at FROM tasks WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("task %q not found", taskID)
	}
	return toString(rows[0]["status"]), rows[0]["claimed_at"]
}

// TestClaimLeaseExpiry: a task claimed longer ago than the visibility
// timeout must be reset to 'pending' with claimed_at cleared by the
// reaper's reap pass, and the running reaper loop must perform the reset.
func TestClaimLeaseExpiry(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness

	h.HeartbeatConfig.Interval = 2 * time.Minute          // polling tick cannot fire inside this test
	h.HeartbeatConfig.ClaimVisibilityTimeout = 100 * time.Millisecond

	seedClaimedTask(t, th, "lease-expired", time.Now().Add(-5*time.Minute))

	// Start the real reaper goroutine (runClaimReaper) and wait for its
	// first tick (claimReaperInterval is fixed at 30s, so wait a bounded
	// margin for the loop to notice the stale claim).
	ctx, cancel := context.WithCancel(th.ctx)
	defer cancel()
	go h.runClaimReaper(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := taskState(t, th, "lease-expired")
		if status == "pending" {
			// Reclaimed by the live loop — now pin the claimed_at side effect.
			_, claimedAt := taskState(t, th, "lease-expired")
			if claimedAt != nil {
				t.Fatalf("expected claimed_at cleared after reclaim, got %v (%T)", claimedAt, claimedAt)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("reaper did not reclaim expired claim within %v (status still %q)",
		5*time.Second, mustStatus(t, th, "lease-expired"))
}

// mustStatus fetches just the task status, for error messages.
func mustStatus(t *testing.T, th *testHarness, taskID string) string {
	t.Helper()
	status, _ := taskState(t, th, taskID)
	return status
}

// TestClaimLeaseRenewal: a task whose claimed_at is fresh (renewed) must be
// left untouched by the reaper — the lease is still valid.
func TestClaimLeaseRenewal(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness

	h.HeartbeatConfig.ClaimVisibilityTimeout = 1 * time.Hour

	seedClaimedTask(t, th, "lease-fresh", time.Now())

	reaped, err := h.reapStaleClaims(th.ctx)
	if err != nil {
		t.Fatalf("reapStaleClaims: %v", err)
	}
	if reaped != 0 {
		t.Fatalf("reaper reclaimed %d task(s); a fresh lease must never be touched", reaped)
	}
	status, claimedAt := taskState(t, th, "lease-fresh")
	if status != "in_progress" {
		t.Fatalf("expected fresh-lease task to stay 'in_progress', got %q", status)
	}
	if claimedAt == nil {
		t.Fatal("expected claimed_at to survive on a valid lease, got NULL")
	}
}

// TestClaimLeaseConfig: the default visibility timeout is 5 minutes when
// unconfigured, and the effective value honours an explicit override.
func TestClaimLeaseConfig(t *testing.T) {
	h := &Harness{} // zero-value HeartbeatConfig: ClaimVisibilityTimeout unset

	if got := h.claimVisibilityTimeout(); got != 5*time.Minute {
		t.Fatalf("default ClaimVisibilityTimeout = %v, want 5m", got)
	}
	if got := defaultClaimVisibilityTimeout; got != 5*time.Minute {
		t.Fatalf("defaultClaimVisibilityTimeout const = %v, want 5m", got)
	}

	h.HeartbeatConfig.ClaimVisibilityTimeout = 90 * time.Second
	if got := h.claimVisibilityTimeout(); got != 90*time.Second {
		t.Fatalf("configured ClaimVisibilityTimeout = %v, want 90s", got)
	}

	// New() must wire the default so an out-of-the-box harness reclaims at 5m.
	if got := New(nil, nil).HeartbeatConfig.ClaimVisibilityTimeout; got != 5*time.Minute {
		t.Fatalf("New() ClaimVisibilityTimeout = %v, want 5m", got)
	}
}

// TestClaimNextReadyTaskStampsClaimedAt pins the lease anchor: claiming a
// task must set claimed_at (otherwise the reaper has no expiry reference).
func TestClaimNextReadyTaskStampsClaimedAt(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO tasks (id, session_id, title, description, status)
		VALUES ('lease-anchor', $1, 'anchor task', 'claimed_at anchor', 'pending')
	`, sessionID); err != nil {
		t.Fatalf("insert pending task: %v", err)
	}

	before := time.Now().Add(-time.Second)
	task, err := th.ClaimNextReadyTask(th.ctx)
	if err != nil {
		t.Fatalf("ClaimNextReadyTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected a task to be claimed")
	}

	_, claimedAt := taskState(t, th, "lease-anchor")
	if claimedAt == nil {
		t.Fatal("expected claimed_at to be stamped by ClaimNextReadyTask, got NULL")
	}
	stamped, ok := parseClaimedAt(toString(claimedAt))
	if !ok {
		t.Fatalf("claimed_at %v (%T) is not parseable", claimedAt, claimedAt)
	}
	if stamped.Before(before) || stamped.After(time.Now().Add(time.Second)) {
		t.Fatalf("claimed_at %v not within [%v, now+1s]", stamped, before)
	}
}

// TestReaperRespectsContextCancellation: runClaimReaper must exit promptly
// when its context is cancelled.
func TestReaperRespectsContextCancellation(t *testing.T) {
	th, err := newTestHarness(&stubLLM{})
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.runClaimReaper(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// reaper exited promptly
	case <-time.After(3 * time.Second):
		t.Fatal("runClaimReaper did not exit within 3s of context cancellation")
	}
}
