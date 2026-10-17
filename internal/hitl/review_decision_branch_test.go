// Package hitl: APPROV-GATE-001 §4.2 — decision-branching regression tests.
//
// Pre-fix ReviewApproval resumed the session for EVERY decision
// (hitl.go:301), so a rejected approval was indistinguishable from an
// approved one at session level (findings.md §4.2, raw/03b E5→E6 paused→idle
// on reject). These tests pin the split behavior:
//
//   - rejected → session stays paused; the rejection callback fires with the
//     row's session/tool context
//   - approved → session resumes (idle) AND the approval callback fires
//   - modified → session resumes, same as approved (SPEC-014 §4.1 hook)
//
// test=internal/hitl/review_decision_branch_test.go
package hitl

import (
	"context"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// reviewBranchFixture builds a Manager + minimal schema for branch tests and
// returns a helper that records callback invocations.
type reviewBranchFixture struct {
	mgr       *Manager
	database  db.DB
	ctx       context.Context
	rejects   []string
	approvals []string
}

func newReviewBranchFixture(t *testing.T) *reviewBranchFixture {
	t.Helper()
	database, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	f := &reviewBranchFixture{mgr: New(database), database: database, ctx: ctx}
	f.mgr.SetRejectionCallback(func(_ context.Context, approvalID, sessionID, reviewerID, notes string) error {
		f.rejects = append(f.rejects, approvalID+"|"+sessionID+"|"+notes)
		return nil
	})
	f.mgr.SetApprovalCallback(func(_ context.Context, approvalID, sessionID string) error {
		f.approvals = append(f.approvals, approvalID+"|"+sessionID)
		return nil
	})
	if err := f.mgr.SetConfiguration(ctx, DefaultConfiguration()); err != nil {
		t.Fatalf("set config: %v", err)
	}
	return f
}

func (f *reviewBranchFixture) session(t *testing.T, id, status string) {
	t.Helper()
	mustExec(t, f.database, f.ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, heartbeat_at, planning_max_turns, created_at)
		 VALUES ($1, 'gate-test', 'test-model', $2, 'test', $3, 10, $3)`, id, status, time.Now())
}

func (f *reviewBranchFixture) approval(t *testing.T, id, sessionID string) {
	t.Helper()
	mustExec(t, f.database, f.ctx,
		`INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, context, target_tool, status, expires_at, created_at)
		 VALUES ($1, $2, 0, 'tool_execution', 'd', 'medium', '{}', 'gated_tool', 'pending', $3, $3)`,
		id, sessionID, time.Now())
}

func TestReviewApproval_RejectKeepsSessionPaused(t *testing.T) {
	f := newReviewBranchFixture(t)
	f.session(t, "s-reject-branch", "paused")
	f.approval(t, "ap-reject-branch", "s-reject-branch")

	if err := f.mgr.ReviewApproval(f.ctx, "ap-reject-branch", DecisionRejected, "admin", "refused: unsafe", ""); err != nil {
		t.Fatalf("review: %v", err)
	}

	rows, err := f.database.Query(f.ctx, `SELECT status FROM sessions WHERE id = 's-reject-branch'`)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("session missing")
	}
	if got := rows[0]["status"].(string); got != "paused" {
		t.Errorf("session status after reject = %q, want 'paused' (reject must block)", got)
	}
	if len(f.rejects) != 1 {
		t.Fatalf("rejection callback fired %d times, want 1", len(f.rejects))
	}
	if !contains(f.rejects[0], "ap-reject-branch|s-reject-branch|refused: unsafe") {
		t.Errorf("rejection callback payload = %q, want id|session|notes", f.rejects[0])
	}
	if len(f.approvals) != 0 {
		t.Errorf("approval callback fired on reject (%v) — must not", f.approvals)
	}

	// Double-review still blocked.
	if err := f.mgr.ReviewApproval(f.ctx, "ap-reject-branch", DecisionApproved, "admin2", "", ""); err == nil {
		t.Error("rejected approval must not be reviewable again")
	}
}

func TestReviewApproval_ApproveResumesAndFiresCallback(t *testing.T) {
	f := newReviewBranchFixture(t)
	f.session(t, "s-approve-branch", "paused")
	f.approval(t, "ap-approve-branch", "s-approve-branch")

	if err := f.mgr.ReviewApproval(f.ctx, "ap-approve-branch", DecisionApproved, "admin", "ok", ""); err != nil {
		t.Fatalf("review: %v", err)
	}

	rows, _ := f.database.Query(f.ctx, `SELECT status FROM sessions WHERE id = 's-approve-branch'`)
	if got := rows[0]["status"].(string); got != "idle" {
		t.Errorf("session status after approve = %q, want 'idle' (resumed)", got)
	}
	if len(f.approvals) != 1 {
		t.Errorf("approval callback fired %d times, want 1", len(f.approvals))
	}
	if len(f.rejects) != 0 {
		t.Errorf("rejection callback fired on approve (%v) — must not", f.rejects)
	}
}

func TestReviewApproval_ModifiedResumesSession(t *testing.T) {
	f := newReviewBranchFixture(t)
	f.session(t, "s-modify-branch", "paused")
	f.approval(t, "ap-modify-branch", "s-modify-branch")

	if err := f.mgr.ReviewApproval(f.ctx, "ap-modify-branch", DecisionModified, "admin", "safer sql", "DELETE FROM temp WHERE 1=0"); err != nil {
		t.Fatalf("review: %v", err)
	}

	rows, _ := f.database.Query(f.ctx, `SELECT status FROM sessions WHERE id = 's-modify-branch'`)
	if got := rows[0]["status"].(string); got != "idle" {
		t.Errorf("session status after modified = %q, want 'idle'", got)
	}
}

// contains is a tiny helper so the test does not import strings for one use.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOfSub(s, sub) >= 0)
}

func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
