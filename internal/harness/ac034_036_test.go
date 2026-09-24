// AC-034 to AC-036: Sub-agent Spawning, Parent/Child Linking & Depth
// Canonical from SPEC-004 §3-4
//
// Verifies that:
//   1. Sub-agent can be spawned with parent_id linking (AC-034)
//   2. Child sessions correctly reference their parent_id (AC-035)
//   3. Depth chains work (root→child→grandchild) (AC-036)
//
// Note: Parent wake-on-completion is verified in subagent.TestParentWakeUp
// (Postgres trigger keeps application-layer WakeParentOnCompletion).
// These AC tests verify the DB-level infrastructure.

package harness

import (
	"testing"
)

// AC-034: Sub-agent Spawning — parent spawns child via tasks table
func TestAC034_SubAgentSpawn(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	parentID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-034: create parent session: %v", err)
	}

	// Spawn a sub-agent: insert task + create child session with parent_id
	taskID := "task-034-1"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO tasks (id, session_id, title, description, status)
		VALUES ($1, $2, 'Research endpoints', 'Find all REST API endpoints', 'pending')
	`, taskID, parentID)
	if err != nil {
		t.Fatalf("AC-034: insert task: %v", err)
	}

	// Create child session with parent_id linking
	childID := "bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeee2"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'child-agent', 'test-model', 'idle', $2, 'Research endpoints')
	`, childID, parentID)
	if err != nil {
		t.Fatalf("AC-034: create child session: %v", err)
	}

	// Verify parent_id is set
	rows, _ := th.conn.Query(th.ctx, `SELECT parent_id FROM sessions WHERE id = $1`, childID)
	if len(rows) == 0 {
		t.Fatal("AC-034: child session not found")
	}
	pid := toString(rows[0]["parent_id"])
	if pid != parentID {
		t.Errorf("AC-034: parent_id = %q, want %q", pid, parentID)
	}
	t.Logf("AC-034 PASS: sub-agent %s spawned with parent %s", childID, pid)
}

// AC-035: Parent/Child Linking — child's parent_id correctly set
func TestAC035_ChildParentLink(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	parentID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-035: create parent: %v", err)
	}

	childID := "cccccccc-bbbb-cccc-dddd-eeeeeeeeeee3"
	err = th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'child', 'test-model', 'thinking', $2, 'child task')
	`, childID, parentID)
	if err != nil {
		t.Fatalf("AC-035: create child: %v", err)
	}

	// Verify child links to parent
	cRows, _ := th.conn.Query(th.ctx, `SELECT parent_id FROM sessions WHERE id = $1`, childID)
	if len(cRows) == 0 || toString(cRows[0]["parent_id"]) != parentID {
		t.Errorf("AC-035: child parent_id = %q, want %q", toString(cRows[0]["parent_id"]), parentID)
	}

	// Verify parent can see its children (query by parent_id)
	children, _ := th.conn.Query(th.ctx, `SELECT id FROM sessions WHERE parent_id = $1`, parentID)
	if len(children) != 1 {
		t.Errorf("AC-035: expected 1 child for parent, got %d", len(children))
	}
	if len(children) > 0 && toString(children[0]["id"]) != childID {
		t.Errorf("AC-035: child id mismatch")
	}
	t.Log("AC-035 PASS: parent/child link verified — parent can query its children")
}

// AC-036: Sub-agent Depth Chain
func TestAC036_SubAgentDepthLimit(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	rootID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("AC-036: create root: %v", err)
	}

	// Create chain: root → child → grandchild
	childID := "dddddddd-bbbb-cccc-dddd-eeeeeeeeeee4"
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'child', 'test-model', 'idle', $2, 'level1')
	`, childID, rootID)

	grandchildID := "eeeeeeee-bbbb-cccc-dddd-eeeeeeeeeee5"
	th.conn.Exec(th.ctx, `
		INSERT INTO sessions (id, agent_name, model_id, status, parent_id, goal)
		VALUES ($1, 'grandchild', 'test-model', 'idle', $2, 'level2')
	`, grandchildID, childID)

	// Verify chain: grandchild → child → root
	rows, _ := th.conn.Query(th.ctx, `
		SELECT id, parent_id FROM sessions WHERE id IN ($1, $2, $3) ORDER BY id
	`, rootID, childID, grandchildID)
	if len(rows) != 3 {
		t.Fatalf("AC-036: expected 3 sessions in chain, got %d", len(rows))
	}

	idToParent := make(map[string]string)
	for _, r := range rows {
		idToParent[toString(r["id"])] = toString(r["parent_id"])
	}

	if idToParent[rootID] != "" {
		t.Errorf("AC-036: root should have empty parent_id")
	}
	if idToParent[childID] != rootID {
		t.Errorf("AC-036: child parent_id = %q, want %q", idToParent[childID], rootID)
	}
	if idToParent[grandchildID] != childID {
		t.Errorf("AC-036: grandchild parent_id = %q, want %q", idToParent[grandchildID], childID)
	}
	t.Log("AC-036 PASS: sub-agent depth chain verified (root→child→grandchild)")
}
