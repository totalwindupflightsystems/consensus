// AC-022: Memory pages — create, resolve, deduplicate
// Canonical from SPEC-002 §5
//
// Verifies that:
//   1. Named memory pages can be created in the DB
//   2. The harness resolves pages into deduplicated event ID sets
//   3. Overlapping events across multiple pages appear only once

package harness

import (
	"fmt"
	"testing"
)

func TestAC022_MemoryPages_CreateAndResolve(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert 3 memory events for this session
	for i := 0; i < 3; i++ {
		err := th.conn.Exec(th.ctx,
			`INSERT INTO memory_events (type, content, session_id, iteration_created)
			 VALUES ('text_block', 'event', $1, 1)`, sessionID)
		if err != nil {
			t.Fatalf("insert event %d: %v", i+1, err)
		}
	}
	// Query back to get auto-increment IDs
	rows, err := th.conn.Query(th.ctx,
		`SELECT id FROM memory_events WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(rows) < 3 {
		t.Fatalf("expected 3 events, got %d", len(rows))
	}
	eventIDs := make([]int64, 3)
	for i, r := range rows {
		eventIDs[i] = toInt64(r["id"])
	}
	t.Logf("Created events with IDs: %v", eventIDs)

	// Create 2 pages with overlapping target_ids (SQLite stores as JSON text)
	// Page 1: events [ID0, ID1]
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_pages (name, target_ids, session_id)
		 VALUES ('page1', $1, $2)`,
		fmt.Sprintf("[%d,%d]", eventIDs[0], eventIDs[1]), sessionID)
	if err != nil {
		t.Fatalf("create page 1: %v", err)
	}

	// Page 2: events [ID1, ID2]  — overlaps with page1 on event ID1
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_pages (name, target_ids, session_id)
		 VALUES ('page2', $1, $2)`,
		fmt.Sprintf("[%d,%d]", eventIDs[1], eventIDs[2]), sessionID)
	if err != nil {
		t.Fatalf("create page 2: %v", err)
	}

	// Resolve page memory IDs through the harness
	pageIDs, err := th.resolvePageMemoryIDs(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("resolve page memory IDs: %v", err)
	}

	// Should have exactly 3 unique event IDs (event[1] overlaps)
	if len(pageIDs) != 3 {
		t.Errorf("AC-022: expected 3 deduplicated event IDs (got %d): %v", len(pageIDs), pageIDs)
	}

	// Each event should be present
	for _, id := range eventIDs {
		if !pageIDs[id] {
			t.Errorf("AC-022: expected event ID %d to be in page set", id)
		}
	}

	t.Logf("AC-022 PASS: 3 deduplicated events from 2 overlapping pages, IDs=%v", eventIDs)
}

func TestAC022_MemoryPages_NoOverlap(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert 2 events
	for i := 0; i < 2; i++ {
		err := th.conn.Exec(th.ctx,
			`INSERT INTO memory_events (type, content, session_id, iteration_created)
			 VALUES ('text_block', 'no-overlap', $1, 1)`, sessionID)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Query IDs
	rows, err := th.conn.Query(th.ctx,
		`SELECT id FROM memory_events WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil || len(rows) < 2 {
		t.Fatalf("query events: err=%v count=%d", err, len(rows))
	}
	id1 := toInt64(rows[0]["id"])
	id2 := toInt64(rows[1]["id"])

	// Create 2 non-overlapping pages
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_pages (name, target_ids, session_id)
		 VALUES ('page_a', $1, $2)`,
		fmt.Sprintf("[%d]", id1), sessionID)
	if err != nil {
		t.Fatalf("create page_a: %v", err)
	}
	err = th.conn.Exec(th.ctx,
		`INSERT INTO memory_pages (name, target_ids, session_id)
		 VALUES ('page_b', $1, $2)`,
		fmt.Sprintf("[%d]", id2), sessionID)
	if err != nil {
		t.Fatalf("create page_b: %v", err)
	}

	// Resolve
	pageIDs, err := th.resolvePageMemoryIDs(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(pageIDs) != 2 {
		t.Errorf("AC-022: expected 2 unique IDs from non-overlapping pages, got %d", len(pageIDs))
	}

	t.Logf("AC-022 PASS: 2 non-overlapping pages produce 2 unique IDs")
}
