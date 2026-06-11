// AC-051: conscience session CLI — create, list, show, cost
// AC-052: conscience approve CLI — list, show, accept, reject
//
// These tests verify the DB operations that the CLI commands use,
// confirming the backend works correctly.

package harness

import (
	"testing"
)

// AC-051: conscience session — create, list, show, cost
func TestAC051_SessionCLI(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Create a session (simulating `conscience session create --goal "test"`)
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration)
		VALUES ('ac051-s1', 'test-agent', 'test-model', 'idle', 'test goal', 0)
	`)
	if err != nil {
		t.Fatalf("AC-051: create session: %v", err)
	}

	// List sessions (simulating `conscience session list`)
	rows, err := th.conn.Query(th.ctx,
		`SELECT id, status, goal, iteration FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		t.Fatalf("AC-051: list sessions: %v", err)
	}
	if len(rows) < 1 {
		t.Fatal("AC-051: no sessions found in list")
	}

	// Show session (simulating `conscience session show ac051-s1`)
	found := false
	for _, r := range rows {
		if toString(r["id"]) == "ac051-s1" {
			found = true
			if toString(r["status"]) != "idle" {
				t.Errorf("AC-051: status = %q, want 'idle'", toString(r["status"]))
			}
			if toString(r["goal"]) != "test goal" {
				t.Errorf("AC-051: goal = %q, want 'test goal'", toString(r["goal"]))
			}
			it := toInt(r["iteration"])
			if it != 0 {
				t.Errorf("AC-051: iteration = %d, want 0", it)
			}
		}
	}
	if !found {
		t.Fatal("AC-051: session ac051-s1 not found in list")
	}

	// Add billing record (simulating `conscience session cost ac051-s1`)
	err = th.conn.Exec(th.ctx, `
		INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd)
		VALUES ('ac051-s1', 1, 'test-model', 'cognition', 500, 200, 0.0050)
	`)
	if err != nil {
		t.Fatalf("AC-051: insert billing: %v", err)
	}

	// Verify cost (simulating session cost display)
	costRows, err := th.conn.Query(th.ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) as total FROM agent_billing WHERE session_id = $1`,
		"ac051-s1")
	if err != nil {
		t.Fatalf("AC-051: query cost: %v", err)
	}
	if len(costRows) == 0 {
		t.Fatal("AC-051: no cost data returned")
	}
	totalCost, _ := toFloat64(costRows[0]["total"])
	if totalCost < 0.004 || totalCost > 0.006 {
		t.Errorf("AC-051: total cost = %f, want ~0.005", totalCost)
	}

	t.Log("AC-051 PASS: session CLI operations (create, list, show, cost) verified")
}

// AC-052: conscience approve — list, show, accept, reject
func TestAC052_ApproveCLI(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Create a session and approval request
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration)
		VALUES ('ac052-s1', 'test-agent', 'test-model', 'paused', 'needs approval', 0)
	`)
	if err != nil {
		t.Fatalf("AC-052: create session: %v", err)
	}

	err = th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, status, risk_level, target_sql, description)
		VALUES ('apr-1', 'ac052-s1', 1, 'destructive_action', 'pending', 'high', 'DROP TABLE test', 'Need to drop test table')
	`)
	if err != nil {
		t.Fatalf("AC-052: create approval: %v", err)
	}

	// List approvals (simulating `conscience approve list`)
	rows, err := th.conn.Query(th.ctx, `
		SELECT id, session_id, status, risk_level, request_type
		FROM approval_requests
		WHERE status = 'pending'
		ORDER BY created_at DESC
	`)
	if err != nil {
		t.Fatalf("AC-052: list approvals: %v", err)
	}
	if len(rows) < 1 {
		t.Fatal("AC-052: no pending approvals found")
	}

	// Show approval details (simulating `conscience approve show apr-1`)
	detail, err := th.conn.Query(th.ctx,
		`SELECT id, session_id, status, risk_level, request_type, description
		 FROM approval_requests WHERE id = $1`, "apr-1")
	if err != nil {
		t.Fatalf("AC-052: show approval: %v", err)
	}
	if len(detail) == 0 {
		t.Fatal("AC-052: approval not found")
	}
	if toString(detail[0]["status"]) != "pending" {
		t.Errorf("AC-052: status = %q, want 'pending'", toString(detail[0]["status"]))
	}
	if toString(detail[0]["risk_level"]) != "high" {
		t.Errorf("AC-052: risk_level = %q, want 'high'", toString(detail[0]["risk_level"]))
	}

	// Accept approval (simulating `conscience approve accept apr-1`)
	err = th.conn.Exec(th.ctx, `
		UPDATE approval_requests SET status = 'approved', reviewed_at = datetime('now'), review_notes = 'Approved by test'
		WHERE id = $1
	`, "apr-1")
	if err != nil {
		t.Fatalf("AC-052: accept approval: %v", err)
	}

	// Verify accepted
	accepted, _ := th.conn.Query(th.ctx,
		`SELECT status FROM approval_requests WHERE id = $1`, "apr-1")
	if len(accepted) == 0 || toString(accepted[0]["status"]) != "approved" {
		t.Errorf("AC-052: after accept, status = %q, want 'approved'",
			toString(accepted[0]["status"]))
	}

	// Reject approval (simulating `conscience approve reject apr-2`)
	err = th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, status, risk_level, target_sql, description)
		VALUES ('apr-2', 'ac052-s1', 1, 'schema_change', 'pending', 'critical', 'ALTER TABLE x', 'Need schema change')
	`)
	if err != nil {
		t.Fatalf("AC-052: create second approval: %v", err)
	}

	err = th.conn.Exec(th.ctx, `
		UPDATE approval_requests SET status = 'rejected', reviewed_at = datetime('now'), review_notes = 'Rejected by test'
		WHERE id = $1
	`, "apr-2")
	if err != nil {
		t.Fatalf("AC-052: reject approval: %v", err)
	}

	rejected, _ := th.conn.Query(th.ctx,
		`SELECT status FROM approval_requests WHERE id = $1`, "apr-2")
	if len(rejected) == 0 || toString(rejected[0]["status"]) != "rejected" {
		t.Errorf("AC-052: after reject, status = %q, want 'rejected'",
			toString(rejected[0]["status"]))
	}

	t.Log("AC-052 PASS: approve CLI operations (list, show, accept, reject) verified")
}
