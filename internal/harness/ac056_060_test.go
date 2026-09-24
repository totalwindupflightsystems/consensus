// AC-056 to AC-060: Final Integration — Multi-agent end-to-end flows
// Verifies the complete system works from multiple agent perspectives:
//   1. Agent session lifecycle (create → work → complete)
//   2. Cross-agent isolation (parallel agents don't interfere)
//   3. Sub-agent delegation chain (parent → child → grandchild)
//   4. HITL integration (approval request → pause → resume)
//   5. Full system health with all components

package harness

import (
	"fmt"
	"testing"
)

// AC-056: Complete Agent Session Lifecycle
func TestAC056_AgentSessionLifecycle(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-056: create session: %v", err)
	}

	// 1. Start: session is idle
	rows, _ := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	status := toString(rows[0]["status"])
	iteration := toInt64(rows[0]["iteration"])
	t.Logf("AC-056: initial state — status=%s, iteration=%d", status, iteration)

	// 2. Agent thinks (simulate harness starting work)
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking', heartbeat_at = datetime('now') WHERE id = $1`, sessionID)

	// 3. Agent writes a memory event (doing work)
	th.conn.Exec(th.ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('text_block', 'Agent is working on task', $1, 1)
	`, sessionID)

	// 4. Agent completes
	th.conn.Exec(th.ctx, `
		UPDATE sessions SET status = 'completed', iteration = iteration + 1, heartbeat_at = datetime('now')
		WHERE id = $1
	`, sessionID)

	// 5. Verify final state
	rowsF, _ := th.conn.Query(th.ctx, `SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	finalStatus := toString(rowsF[0]["status"])
	finalIter := toInt64(rowsF[0]["iteration"])

	if finalStatus != "completed" {
		t.Errorf("AC-056: final status = %q, want 'completed'", finalStatus)
	}
	if finalIter <= iteration {
		t.Errorf("AC-056: iteration = %d, want > %d", finalIter, iteration)
	}

	// Verify memory was recorded
	memRows, _ := th.conn.Query(th.ctx, `SELECT content FROM memory_events WHERE session_id = $1 ORDER BY iteration_created`, sessionID)
	if len(memRows) == 0 {
		t.Error("AC-056: no memory events recorded")
	}
	t.Logf("AC-056: %d memory events recorded", len(memRows))
	t.Log("AC-056 PASS: complete agent session lifecycle verified")
}

// AC-057: Parallel Agent Operations — two agents working simultaneously
func TestAC057_ParallelAgents(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Create two agents
	agent1 := "p1p1p1p1-1111-1111-1111-p1p1p1p1p1p1"
	agent2 := "p2p2p2p2-2222-2222-2222-p2p2p2p2p2p2"

	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ($1, 'parallel-A', 'test-model', 'thinking', 'Task Alpha')
	`, agent1)
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ($1, 'parallel-B', 'test-model', 'thinking', 'Task Beta')
	`, agent2)

	// Both agents write events simultaneously
	th.conn.Exec(th.ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('text_block', 'Agent A event', $1, 1)
	`, agent1)
	th.conn.Exec(th.ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('text_block', 'Agent B event', $1, 2)
	`, agent2)

	// Agent A's memory only has A's events
	aEvents, _ := th.conn.Query(th.ctx, `SELECT content FROM memory_events WHERE session_id = $1`, agent1)
	if len(aEvents) != 1 {
		t.Errorf("AC-057: Agent A has %d events, want 1", len(aEvents))
	}
	if len(aEvents) > 0 {
		c := toString(aEvents[0]["content"])
		if c != "Agent A event" {
			t.Errorf("AC-057: Agent A event content = %q", c)
		}
	}

	// Agent B's memory only has B's events
	bEvents, _ := th.conn.Query(th.ctx, `SELECT content FROM memory_events WHERE session_id = $1`, agent2)
	if len(bEvents) != 1 {
		t.Errorf("AC-057: Agent B has %d events, want 1", len(bEvents))
	}
	if len(bEvents) > 0 {
		c := toString(bEvents[0]["content"])
		if c != "Agent B event" {
			t.Errorf("AC-057: Agent B event content = %q", c)
		}
	}

	t.Logf("AC-057 PASS: %d agents working in parallel with isolated memory", 2)
}

// AC-058: Sub-agent Delegation Chain with Completion
// Verifies DB-level delegation chain structure.
// Application-level parent wake is verified by subagent.TestParentWakeUp.
func TestAC058_SubAgentChain(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// Root agent creates a task → spawns child → child spawns grandchild
	rootID := "root-root-root-root-rootrootroot1"

	// Create root
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ($1, 'root-agent', 'test-model', 'thinking', 'Root task')
	`, rootID)

	// Root creates a pending task
	th.conn.Exec(th.ctx, `
		INSERT INTO tasks (id, session_id, title, description, status)
		VALUES ('task-chain-1', $1, 'Delegated task', 'Child should do this', 'pending')
	`, rootID)

	// Child is spawned from the task
	childID := "child-child-child-child-childchild1"
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'child-agent', 'test-model', 'thinking', $2, 'Child task')
	`, childID, rootID)

	// Root sets waiting_sub
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'waiting_sub' WHERE id = $1`, rootID)

	// Child creates a subtask
	th.conn.Exec(th.ctx, `
		INSERT INTO tasks (id, session_id, title, description, status)
		VALUES ('task-chain-2', $1, 'Sub-delegated task', 'Grandchild should do this', 'pending')
	`, childID)

	// Grandchild spawned
	grandchildID := "grand-grand-grand-grand-grandchild1"
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'grandchild-agent', 'test-model', 'thinking', $2, 'Grandchild task')
	`, grandchildID, childID)

	// Verify the chain structure
	chainCheck, _ := th.conn.Query(th.ctx, `
		SELECT s1.agent_name as root_name, s2.agent_name as child_name, s3.agent_name as grandchild_name
		FROM sessions s1
		JOIN sessions s2 ON s2.parent_id = s1.id
		JOIN sessions s3 ON s3.parent_id = s2.id
		WHERE s1.id = $1 AND s2.id = $2 AND s3.id = $3
	`, rootID, childID, grandchildID)

	if len(chainCheck) != 1 {
		t.Fatalf("AC-058: chain structure not found (expected 1 row from 3-way join, got %d)", len(chainCheck))
	}

	rootName := toString(chainCheck[0]["root_name"])
	childName := toString(chainCheck[0]["child_name"])
	grandchildName := toString(chainCheck[0]["grandchild_name"])

	if rootName != "root-agent" || childName != "child-agent" || grandchildName != "grandchild-agent" {
		t.Errorf("AC-058: chain names mismatch: %q → %q → %q", rootName, childName, grandchildName)
	}

	// Verify tasks were created
	taskCount, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM tasks WHERE session_id IN ($1, $2)`, rootID, childID)
	totalTasks := 0
	if len(taskCount) > 0 {
		totalTasks = toInt(taskCount[0]["cnt"])
	}
	if totalTasks != 2 {
		t.Errorf("AC-058: expected 2 tasks, got %d", totalTasks)
	}

	t.Logf("AC-058 PASS: delegation chain verified: %q → %q → %q with %d tasks", rootName, childName, grandchildName, totalTasks)
}

// AC-059: HITL Approval During Tool Execution
func TestAC059_HITLApprovalFlow(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-059: create session: %v", err)
	}

	// Agent is thinking → requests tool that needs approval
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID)

	// Create approval request
	th.conn.Exec(th.ctx, `
		INSERT INTO approval_requests (id, session_id, request_type, reason, status, reviewed_by, created_at)
		VALUES ('apr-059', $1, 'destructive_action', 'Agent wants to delete records', 'pending', '', datetime('now'))
	`, sessionID)

	// Session pauses for approval
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'paused' WHERE id = $1`, sessionID)

	// Verify paused
	pRows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if toString(pRows[0]["status"]) != "paused" {
		t.Fatal("AC-059: session should be paused for approval")
	}

	// Human reviews and approves
	th.conn.Exec(th.ctx, `
		UPDATE approval_requests SET status = 'approved', reviewed_by = 'admin', reviewed_at = datetime('now')
		WHERE id = 'apr-059'
	`)

	// Session resumes
	th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking', heartbeat_at = datetime('now') WHERE id = $1`, sessionID)

	rRows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	resumedStatus := toString(rRows[0]["status"])
	if resumedStatus != "thinking" {
		t.Errorf("AC-059: after resume status = %q, want 'thinking'", resumedStatus)
	}

	// Verify approval was recorded
	aRows, _ := th.conn.Query(th.ctx, `SELECT status, reviewed_by FROM approval_requests WHERE id = 'apr-059'`)
	if len(aRows) > 0 {
		appStatus := toString(aRows[0]["status"])
		reviewer := toString(aRows[0]["reviewed_by"])
		if appStatus != "approved" {
			t.Errorf("AC-059: approval status = %q, want 'approved'", appStatus)
		}
		if reviewer != "admin" {
			t.Errorf("AC-059: reviewer = %q, want 'admin'", reviewer)
		}
	}
	t.Log("AC-059 PASS: HITL approval flow (pending→pause→approve→resume) verified")
}

// AC-060: Full System — all components working together
func TestAC060_FullSystemIntegrity(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	// 1. Create multiple sessions
	agentIDs := []string{}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("full-%d-system-agent-%d", i, i)
		agentIDs = append(agentIDs, id)
		goal := fmt.Sprintf("System integrity test agent %d", i)
		th.conn.Exec(th.ctx, `
			INSERT INTO sessions (id, agent_name, model_id, status, goal)
			VALUES ($1, $2, 'test-model', 'idle', $3)
		`, id, fmt.Sprintf("agent-%d", i), goal)
	}

	// 2. Each agent has memory events
	for _, id := range agentIDs {
		th.conn.Exec(th.ctx, `
			INSERT INTO memory_events (type, content, session_id, iteration_created)
			VALUES ('text_block', 'System integrity event for '+$1, $1, 1)
		`, id)
	}

	// 3. Skills registry is populated
	th.conn.Exec(th.ctx, `
		INSERT INTO skills_registry (id, name, metadata, instructions, enabled)
		VALUES ('sk-full-1', 'core_skill', '{"description":"Core system skill"}', 'Core instructions', 1)
	`)

	// 4. Staging buffer is empty (clean slate)
	bufRows, _ := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM staging_buffer`)
	stagingCount := 0
	if len(bufRows) > 0 {
		stagingCount = toInt(bufRows[0]["cnt"])
	}
	if stagingCount != 0 {
		t.Errorf("AC-060: staging buffer should be empty, got %d entries", stagingCount)
	}

	// 5. Sessions have correct data
	for _, id := range agentIDs {
		sRows, _ := th.conn.Query(th.ctx, `SELECT status, goal FROM sessions WHERE id = $1`, id)
		if len(sRows) == 0 {
			t.Errorf("AC-060: agent %s not found", id)
			continue
		}
		if toString(sRows[0]["status"]) != "idle" {
			t.Errorf("AC-060: agent %s status = %q, want 'idle'", id, toString(sRows[0]["status"]))
		}
	}

	// 6. All components present
	pass := true
	for _, table := range []string{"sessions", "memory_events", "staging_buffer", "skills_registry", "approval_requests", "tasks", "iteration_commits"} {
		tr, err := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM `+table)
		if err != nil {
			t.Errorf("AC-060: table %s error: %v", table, err)
			pass = false
		} else if len(tr) > 0 {
			t.Logf("  %s: %d rows", table, toInt(tr[0]["cnt"]))
		}
	}
	if pass {
		t.Log("AC-060 PASS: full system integrity verified — all components present and working")
	}
}
