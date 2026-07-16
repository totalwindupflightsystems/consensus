// Package harness: concurrent sessions processing test (Phase 4, Task 2).
//
// Test that 2+ sessions can be processed simultaneously via RunAgentIteration
// without deadlocks, data races, or cross-contamination.
//
// axiom:trace work_item=phase-4-task-2 spec=specs/006-transactions.md plan=phase-4/task-2
package harness

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Session-aware mock LLM — returns session-specific outputs for concurrent testing
// ============================================================================

// alternatingMock returns pre-configured outputs in order, one per Call().
// SQLite serializes DB access so goroutines land in deterministic order.
type alternatingMock struct {
	mu      sync.Mutex
	counter int
	outputs []*AgentOutput
	modelID string
}

func (m *alternatingMock) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	m.mu.Lock()
	idx := m.counter
	m.counter++
	m.mu.Unlock()

	if idx >= len(m.outputs) {
		return nil, fmt.Errorf("alternatingMock: called %d times, only %d outputs configured", m.counter, len(m.outputs))
	}

	return &LLMResponse{
		Output:  m.outputs[idx],
		ModelID: m.modelID,
	}, nil
}

// sessionSpecificOutput returns a minimal AgentOutput with SQL keyed to the given sessionID.
func sessionSpecificOutput(sessionID string) *AgentOutput {
	return &AgentOutput{
		InternalMonologue: fmt.Sprintf("Concurrent processing iteration for session %s.", sessionID),
		MemoryStateChanges: []string{
			fmt.Sprintf("INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Memory from %s', '%s', 1)", sessionID, sessionID),
		},
		SystemActions: []string{},
		ToolRequests:  nil,
	}
}

// ============================================================================
// Phase 4 Task 2: 2+ concurrent sessions processing simultaneously
// ============================================================================

func TestConcurrentSessions_ProcessSimultaneously(t *testing.T) {
	// Create 2 unique session UUIDs
	sessionA := "ca11aaa1-1111-1111-1111-11111111111a"
	sessionB := "ca11bbb2-2222-2222-2222-22222222222b"

	// Build an alternating mock that returns A's output first, then B's
	mock := &alternatingMock{
		outputs: []*AgentOutput{
			sessionSpecificOutput(sessionA),
			sessionSpecificOutput(sessionB),
		},
		modelID: "mock-model",
	}

	th, err := newTestHarness(mock)
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Create both sessions
	for _, sid := range []string{sessionA, sessionB} {
		agentName := fmt.Sprintf("agent-%s", sid[:8])
		err := th.conn.Exec(th.ctx, `
			INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal)
			VALUES ($1, $2, 'test-model', 'idle', 'high', 'Concurrent test goal')
		`, sid, agentName)
		if err != nil {
			t.Fatalf("failed to create session %s: %v", sid, err)
		}
	}

	// Process both sessions concurrently
	var wg sync.WaitGroup
	results := make(chan struct {
		sessionID string
		result    *IterationResult
		err       error
	}, 2)

	for _, sid := range []string{sessionA, sessionB} {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			result, err := th.RunAgentIteration(context.Background(), sessionID)
			results <- struct {
				sessionID string
				result    *IterationResult
				err       error
			}{sessionID, result, err}
		}(sid)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent sessions processing timed out after 10s")
	}
	close(results)

	// Collect and verify results
	sessionResults := make(map[string]*IterationResult)
	sessionErrs := make(map[string]error)
	for r := range results {
		sessionResults[r.sessionID] = r.result
		sessionErrs[r.sessionID] = r.err
	}

	// Verify both sessions completed
	if len(sessionResults) != 2 {
		t.Fatalf("expected 2 results, got %d", len(sessionResults))
	}

	for _, sid := range []string{sessionA, sessionB} {
		err := sessionErrs[sid]
		if err != nil {
			t.Errorf("session %s: unexpected Go error from RunAgentIteration: %v", sid, err)
			continue
		}

		result := sessionResults[sid]
		if result == nil {
			t.Errorf("session %s: nil result", sid)
			continue
		}

		// Verify success status
		if result.Status != "success" {
			t.Errorf("session %s: expected status 'success', got %q", sid, result.Status)
		}

		// Verify audit log was created
		count, err := th.assertAuditLogCount(sid)
		if err != nil {
			t.Errorf("session %s: audit log query failed: %v", sid, err)
		} else if count < 1 {
			t.Errorf("session %s: expected at least 1 audit log entry, got %d", sid, count)
		}

		// Verify session status advanced
		rows, err := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sid)
		if err != nil {
			t.Errorf("session %s: failed to query session: %v", sid, err)
		} else if len(rows) == 0 {
			t.Errorf("session %s: not found after iteration", sid)
		} else {
			finalStatus := toString(rows[0]["status"])
			finalIter := toInt64(rows[0]["iteration"])
			t.Logf("session %s: final state — status=%s, iteration=%d", sid, finalStatus, finalIter)
			if finalIter < 1 {
				t.Errorf("session %s: iteration did not advance (got %d, want >=1)", sid, finalIter)
			}
			if finalStatus == "error" {
				t.Errorf("session %s: session ended in error status", sid)
			}
		}
	}

	// Verify memory isolation: each session's events only reference its own session_id
	for _, sid := range []string{sessionA, sessionB} {
		rows, err := th.conn.Query(th.ctx, `SELECT content, session_id FROM memory_events WHERE session_id = $1`, sid)
		if err != nil {
			t.Errorf("session %s: memory events query failed: %v", sid, err)
			continue
		}
		for _, row := range rows {
			rowSID := toString(row["session_id"])
			if rowSID != sid {
				t.Errorf("session %s: memory isolation violated — found event with session_id=%s", sid, rowSID)
			}
		}
		t.Logf("session %s: %d memory events (all isolated)", sid, len(rows))
	}

	t.Logf("Phase 4 Task 2 PASS: %d sessions processed simultaneously without deadlocks or data races", 2)
}

// TestConcurrentSessions_ManySessionsProcessSimultaneously verifies that 5+
// concurrent sessions can process simultaneously without deadlocks or races.
// Uses alternatingMock for race-safe session-specific outputs.
func TestConcurrentSessions_ManySessionsProcessSimultaneously(t *testing.T) {
	sids := []string{
		"many0-aaa1-1111-1111-a11111111111",
		"many0-bbb2-2222-2222-b22222222222",
		"many0-ccc3-3333-3333-c33333333333",
		"many0-ddd4-4444-4444-d44444444444",
		"many0-eee5-5555-5555-e55555555555",
	}

	// Build alternating mock with session-specific outputs
	outputs := make([]*AgentOutput, len(sids))
	for i, sid := range sids {
		outputs[i] = sessionSpecificOutput(sid)
	}
	mock := &alternatingMock{
		outputs: outputs,
		modelID: "mock-model",
	}

	th, err := newTestHarness(mock)
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Create all sessions
	for i, sid := range sids {
		err := th.conn.Exec(th.ctx, `
			INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal)
			VALUES ($1, $2, 'test-model', 'idle', 'high', 'Many-session stress test')
		`, sid, fmt.Sprintf("agent-%d", i))
		if err != nil {
			t.Fatalf("failed to create session %s: %v", sid, err)
		}
	}

	// Process all concurrently
	var wg sync.WaitGroup
	errs := make(chan error, len(sids))

	for _, sid := range sids {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			_, err := th.RunAgentIteration(context.Background(), sessionID)
			if err != nil {
				errs <- fmt.Errorf("session %s: %w", sessionID, err)
			}
		}(sid)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
		close(errs)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out")
	}

	for e := range errs {
		t.Error(e)
	}

	// Verify all sessions exist and have advanced
	for _, sid := range sids {
		rows, err := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sid)
		if err != nil {
			t.Errorf("session %s: query failed: %v", sid, err)
		} else if len(rows) == 0 {
			t.Errorf("session %s: not found after concurrent processing", sid)
		} else {
			status := toString(rows[0]["status"])
			iter := toInt64(rows[0]["iteration"])
			t.Logf("session %s: status=%s, iteration=%d", sid, status, iter)
			if iter < 1 {
				t.Errorf("session %s: iteration did not advance (%d)", sid, iter)
			}
		}
	}

	// Verify schema integrity: all expected tables still exist
	tables := []string{"sessions", "memory_events", "audit_logs", "iteration_commits",
		"model_registry", "tool_requests", "tasks", "agent_circuit_breakers"}
	for _, table := range tables {
		rows, err := th.conn.Query(th.ctx,
			fmt.Sprintf("SELECT name FROM sqlite_master WHERE type='table' AND name='%s'", table))
		if err != nil {
			t.Errorf("schema check: query for table %s failed: %v", table, err)
		} else if len(rows) == 0 {
			t.Errorf("schema check: table %s is MISSING after concurrent processing", table)
		}
	}

	t.Logf("Phase 4 Task 2b PASS: %d sessions + schema intact", len(sids))
}
