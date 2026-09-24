// AC-020: memory_events is append-only — UPDATE/DELETE rejected
// Canonical from SPEC-002 §2.1, §2.4
//
// This test verifies that the SQLite trigger on memory_events prevents
// modification of existing rows while allowing INSERTs.
//
// The trigger fires BEFORE UPDATE/DELETE on memory_events, raising an
// ABORT error. This enforces the append-only invariant at the database
// layer, which is stronger than application-level enforcement (it cannot
// be bypassed by the application, even accidentally).

package harness

import (
	"strings"
	"testing"
)

func TestAC020_MemoryEventsAppendOnly_InsertAllowed(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// INSERT should succeed
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'Append-only test content', $1, 1)`, sessionID)
	if err != nil {
		t.Fatalf("INSERT on memory_events should be allowed, got: %v", err)
	}

	// Verify the row was actually written
	rows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query after insert: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one row after INSERT")
	}
	content, ok := rows[0]["content"].(string)
	if !ok {
		t.Fatalf("content field is not a string, got %T", rows[0]["content"])
	}
	if content != "Append-only test content" {
		t.Fatalf("expected 'Append-only test content', got %q", content)
	}
	t.Logf("AC-020 PASS: INSERT into memory_events allowed, content=%q", content)
}

func TestAC020_MemoryEventsAppendOnly_RejectsUpdate(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert a row first
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'Original', $1, 1)`, sessionID)
	if err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Attempt UPDATE — should be rejected by append-only trigger
	err = th.conn.Exec(th.ctx,
		`UPDATE memory_events SET content = 'Modified' WHERE session_id = $1`, sessionID)
	if err == nil {
		t.Fatal("AC-020 FAIL: UPDATE on memory_events was allowed (append-only trigger missing or not firing)")
	}

	// Verify the error message mentions append-only
	errMsg := err.Error()
	if !strings.Contains(errMsg, "append-only") {
		t.Logf("AC-020: UPDATE rejected but error message doesn't mention append-only: %s", errMsg)
	} else {
		t.Logf("AC-020 PASS: UPDATE rejected with append-only message: %s", errMsg)
	}

	// Verify the original data is unchanged
	rows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query after rejected update: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected row still present after rejected UPDATE")
	}
	content, ok := rows[0]["content"].(string)
	if !ok {
		t.Fatalf("content field is not a string, got %T", rows[0]["content"])
	}
	if content != "Original" {
		t.Fatalf("expected 'Original' (unchanged), got %q", content)
	}
	t.Logf("AC-020 PASS: original data preserved after rejected UPDATE")
}

func TestAC020_MemoryEventsAppendOnly_RejectsDelete(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert a row first
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'Delete-me', $1, 1)`, sessionID)
	if err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Attempt DELETE — should be rejected by append-only trigger
	err = th.conn.Exec(th.ctx,
		`DELETE FROM memory_events WHERE session_id = $1`, sessionID)
	if err == nil {
		t.Fatal("AC-020 FAIL: DELETE on memory_events was allowed (append-only trigger missing)")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "append-only") {
		t.Logf("AC-020: DELETE rejected but message doesn't mention append-only: %s", errMsg)
	} else {
		t.Logf("AC-020 PASS: DELETE rejected with append-only message: %s", errMsg)
	}

	// Verify the row still exists
	rows, err := th.conn.Query(th.ctx,
		`SELECT content FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query after rejected delete: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected row still present after rejected DELETE")
	}
	t.Logf("AC-020 PASS: row preserved after rejected DELETE")
}
