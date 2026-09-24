// Package harness: 100+ iteration durability test (Phase 4).
//
// Verifies that the agent harness can survive 100+ iterations without crashes,
// memory corruption, or state degradation. Each iteration produces a distinct
// memory event, audit log, and snapshot. After 100 iterations we assert
// integrity of counts, metadata, and specific historical rows.
//
// axiom:trace work_item=phase-4-durability spec=specs/006-transactions.md,specs/008-harness.md plan=phase-4
package harness

import (
	"context"
	"fmt"
	"testing"
)

// ============================================================================
// Phase 4: 100+ Iteration Durability Test
// ============================================================================

// durabilityOutputs returns n pre-generated AgentOutputs, each with a unique
// iteration number embedded in the SQL INSERT statement so that historical
// memory rows can be verified individually.
func durabilityOutputs(n int) []*AgentOutput {
	outputs := make([]*AgentOutput, n)
	for i := 0; i < n; i++ {
		iter := i + 1
		outputs[i] = &AgentOutput{
			InternalMonologue: fmt.Sprintf("Durability iteration %d. Everything stable.", iter),
			MemoryStateChanges: []string{
				fmt.Sprintf("INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Durability test iteration %d', 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1', %d)", iter, iter),
			},
			SystemActions: []string{},
			ToolRequests:  nil,
		}
	}
	return outputs
}

// TestHundredIterationDurability runs 100 consecutive agent iterations against
// a single session and verifies no crashes, correct counts, and intact state.
func TestHundredIterationDurability(t *testing.T) {
	const iterations = 100
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"

	mock := &alternatingMock{
		outputs: durabilityOutputs(iterations),
		modelID: "mock-model",
	}

	th, err := newTestHarness(mock)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	// Seed the session
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal)
		VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'Durability test goal')
	`, sessionID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Run 100 iterations. Stop immediately on any fatal error.
	for i := 0; i < iterations; i++ {
		result, err := th.RunAgentIteration(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("iteration %d: RunAgentIteration returned error: %v", i+1, err)
		}
		if result == nil {
			t.Fatalf("iteration %d: RunAgentIteration returned nil result", i+1)
		}
		if result.Status != "success" {
			t.Fatalf("iteration %d: expected status 'success', got %q (error: %v)", i+1, result.Status, result.Error)
		}
	}

	// Verify session status is not corrupted
	rows, err := th.conn.Query(th.ctx, `SELECT status, agent_name, model_id, trust_level FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session metadata: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("session %s not found after 100 iterations", sessionID)
	}
	status := toString(rows[0]["status"])
	if status == "error" || status == "" {
		t.Fatalf("session status corrupted: got %q", status)
	}

	// Verify metadata integrity (Test 2, combined)
	if got := toString(rows[0]["agent_name"]); got != "test-agent" {
		t.Fatalf("agent_name degraded: expected 'test-agent', got %q", got)
	}
	if got := toString(rows[0]["model_id"]); got != "test-model" {
		t.Fatalf("model_id degraded: expected 'test-model', got %q", got)
	}
	if got := toString(rows[0]["trust_level"]); got != "high" {
		t.Fatalf("trust_level degraded: expected 'high', got %q", got)
	}

	// Verify memory_events count
	memCount, err := assertCount(th, `SELECT COUNT(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query memory_events count: %v", err)
	}
	if memCount != iterations {
		t.Fatalf("memory_events count mismatch: expected %d, got %d", iterations, memCount)
	}

	// Verify audit_logs count
	auditCount, err := assertCount(th, `SELECT COUNT(*) as cnt FROM audit_logs WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query audit_logs count: %v", err)
	}
	if auditCount != iterations {
		t.Fatalf("audit_logs count mismatch: expected %d, got %d", iterations, auditCount)
	}

	// Verify iteration_commits count
	snapshotCount, err := assertCount(th, `SELECT COUNT(*) as cnt FROM iteration_commits WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query iteration_commits count: %v", err)
	}
	if snapshotCount != iterations {
		t.Fatalf("iteration_commits count mismatch: expected %d, got %d", iterations, snapshotCount)
	}

	t.Logf("Phase 4 Durability PASS: %d iterations, %d memory events, %d audit logs, %d snapshots", iterations, memCount, auditCount, snapshotCount)
}

// TestSessionStateNotDegraded is intentionally combined with
// TestHundredIterationDurability to avoid running the expensive 100-iteration
// loop twice. It asserts agent_name, model_id, and trust_level remain intact
// after 100 iterations.
//
// If separated, it would repeat the same loop; combining keeps the suite fast
// while still satisfying the verification requirement.
func TestSessionStateNotDegraded(t *testing.T) {
	t.Log("SessionStateNotDegraded assertions are run inside TestHundredIterationDurability")
}

// TestMemoryStabilityAcrossIterations verifies that no iteration overwrites
// another iteration's data. After 100 iterations, it checks that iteration 50's
// memory event still exists and is isolated to this session.
func TestMemoryStabilityAcrossIterations(t *testing.T) {
	const iterations = 100
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"

	mock := &alternatingMock{
		outputs: durabilityOutputs(iterations),
		modelID: "mock-model",
	}

	th, err := newTestHarness(mock)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal)
		VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'Memory stability test goal')
	`, sessionID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	for i := 0; i < iterations; i++ {
		result, err := th.RunAgentIteration(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("iteration %d: %v", i+1, err)
		}
		if result == nil || result.Status != "success" {
			t.Fatalf("iteration %d: expected success, got status %q (err %v)", i+1, result.Status, result.Error)
		}
	}

	// Verify iteration 50's specific memory event still exists
	rows, err := th.conn.Query(th.ctx, `SELECT COUNT(*) as cnt FROM memory_events WHERE session_id = $1 AND iteration_created = 50`, sessionID)
	if err != nil {
		t.Fatalf("query iteration 50 memory event: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("iteration 50 memory event query returned no rows")
	}
	count := toInt(rows[0]["cnt"])
	if count != 1 {
		t.Fatalf("expected exactly 1 memory event for iteration 50, got %d", count)
	}

	t.Logf("Phase 4 Memory Stability PASS: iteration 50 memory event preserved")
}

// assertCount is a small helper to execute a SELECT COUNT(*) query and return
// the integer count.
func assertCount(th *testHarness, query string, sessionID string) (int, error) {
	rows, err := th.conn.Query(th.ctx, query, sessionID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return toInt(rows[0]["cnt"]), nil
}
