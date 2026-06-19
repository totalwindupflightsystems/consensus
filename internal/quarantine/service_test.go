// Package quarantine — service layer tests for the Cognitive Firewall.
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md plan=phase-6/task-2/step-1
package quarantine

import (
	"context"
	"fmt"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// Test setup — in-memory SQLite database
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create the core tables needed for quarantine tests
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'idle',
		model_id TEXT NOT NULL DEFAULT 'default',
		context_budget INT NOT NULL DEFAULT 128000,
		tokens_used_in BIGINT NOT NULL DEFAULT 0,
		tokens_used_out BIGINT NOT NULL DEFAULT 0,
		iteration BIGINT NOT NULL DEFAULT 0,
		project_id TEXT,
		heartbeat_at TEXT NOT NULL DEFAULT (datetime('now')),
		planning_max_turns INT NOT NULL DEFAULT 10,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS memory_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		content TEXT NOT NULL,
		session_id TEXT NOT NULL,
		iteration_created BIGINT NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// Create external_quarantine matching prod schema (without Postgres-specific CHECK for SQLite compat)
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS external_quarantine (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'api_response',
		source_url TEXT,
		raw_content TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		validation_status TEXT NOT NULL DEFAULT 'pending',
		validation_notes TEXT,
		promoted_memory_id INTEGER,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// Seed a test session
	mustExec(t, database, ctx, `INSERT OR IGNORE INTO sessions (id, agent_name, status) VALUES ('test-session', 'test-agent', 'idle')`)

	cleanup := func() {
		database.Close()
	}

	return database, cleanup
}

func mustExec(t *testing.T, database db.DB, ctx context.Context, query string, args ...any) {
	t.Helper()
	if err := database.Exec(ctx, query, args...); err != nil {
		t.Fatalf("exec: %s: %v", query, err)
	}
}

// ============================================================================
// Quarantine Service Tests
// ============================================================================

func TestInsertAndListQuarantine(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	qs := NewQuarantineService(database, nil)

	// Insert a quarantined item
	item, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  `<script>alert('xss')</script>`,
		ContentHash: ContentHash(`<script>alert('xss')</script>`),
	})
	if err != nil {
		t.Fatalf("insert quarantine: %v", err)
	}
	if item.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if item.ValidationStatus != StatusPending {
		t.Errorf("expected pending status, got %s", item.ValidationStatus)
	}
	if item.SourceType != "api_response" {
		t.Errorf("expected api_response source type, got %s", item.SourceType)
	}
	t.Logf("Inserted quarantine item: id=%d, hash=%s", item.ID, item.ContentHash)

	// List all items
	items, err := qs.ListQuarantine(ctx, "")
	if err != nil {
		t.Fatalf("list quarantine: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}

	// List pending only
	pending, err := qs.ListQuarantine(ctx, StatusPending)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending item, got %d", len(pending))
	}

	// List rejected — should be empty
	rejected, err := qs.ListQuarantine(ctx, StatusRejected)
	if err != nil {
		t.Fatalf("list rejected: %v", err)
	}
	if len(rejected) != 0 {
		t.Errorf("expected 0 rejected items, got %d", len(rejected))
	}
}

func TestApproveQuarantine(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-3
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	qs := NewQuarantineService(database, nil)

	// Insert a quarantined item
	item, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "clean data that passed secondary review",
		ContentHash: ContentHash("clean data that passed secondary review"),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Approve it
	approved, err := qs.ApproveQuarantine(ctx, item.ID, "test-session")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.ValidationStatus != StatusValidated {
		t.Errorf("expected validated status, got %s", approved.ValidationStatus)
	}
	if approved.PromotedMemoryID == 0 {
		t.Errorf("expected promoted_memory_id to be set")
	}
	t.Logf("Approved item: id=%d, promoted_memory_id=%d", approved.ID, approved.PromotedMemoryID)

	// Verify the promoted memory event exists
	rows, err := database.Query(ctx, `SELECT id, type, content, session_id FROM memory_events WHERE id = $1`, approved.PromotedMemoryID)
	if err != nil {
		t.Fatalf("query promoted memory: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("promoted memory event not found")
	}
	if toString(rows[0]["session_id"]) != "test-session" {
		t.Errorf("expected session_id 'test-session', got %q", rows[0]["session_id"])
	}
	t.Logf("Promoted memory event: id=%v, content=%q", rows[0]["id"], toString(rows[0]["content"]))
}

func TestRejectQuarantine(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-4
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	qs := NewQuarantineService(database, nil)

	item, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "UNION SELECT * FROM passwords",
		ContentHash: ContentHash("UNION SELECT * FROM passwords"),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rejected, err := qs.RejectQuarantine(ctx, item.ID, "SQL injection pattern detected")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.ValidationStatus != StatusRejected {
		t.Errorf("expected rejected status, got %s", rejected.ValidationStatus)
	}
	if rejected.ValidationNotes != "SQL injection pattern detected" {
		t.Errorf("expected reason 'SQL injection pattern detected', got %q", rejected.ValidationNotes)
	}
	t.Logf("Rejected item: id=%d, reason=%q", rejected.ID, rejected.ValidationNotes)
}

func TestApproveNonPendingItem(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-5
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	qs := NewQuarantineService(database, nil)

	item, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "test content",
		ContentHash: ContentHash("test content"),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// First approve should succeed
	_, err = qs.ApproveQuarantine(ctx, item.ID, "test-session")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}

	// Second approve should fail (already validated)
	_, err = qs.ApproveQuarantine(ctx, item.ID, "test-session")
	if err == nil {
		t.Error("expected error approving already-validated item")
	}
	t.Logf("Correctly rejected double-approve: %v", err)
}

func TestScanPendingQuarantine(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	qs := NewQuarantineService(database, nil)

	// Insert clean content (should stay pending)
	_, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "normal benign webhook payload",
		ContentHash: ContentHash("normal benign webhook payload"),
	})
	if err != nil {
		t.Fatalf("insert clean: %v", err)
	}

	// Insert malicious content (should be rejected by scanner)
	_, err = qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "DROP TABLE sessions; -- malicious payload",
		ContentHash: ContentHash("DROP TABLE sessions; -- malicious payload"),
	})
	if err != nil {
		t.Fatalf("insert malicious: %v", err)
	}

	// Insert XSS content
	_, err = qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "<script>document.cookie</script>",
		ContentHash: ContentHash("<script>document.cookie</script>"),
	})
	if err != nil {
		t.Fatalf("insert XSS: %v", err)
	}

	// Scan pending items
	count, err := qs.ScanPendingQuarantine(ctx)
	if err != nil {
		t.Fatalf("scan pending: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 processed items, got %d", count)
	}

	// Verify clean item is still pending
	pending, err := qs.ListQuarantine(ctx, StatusPending)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending item (clean), got %d", len(pending))
	}
	t.Logf("Clean item still pending: id=%d", pending[0].ID)

	// Verify malicious items are rejected
	rejected, err := qs.ListQuarantine(ctx, StatusRejected)
	if err != nil {
		t.Fatalf("list rejected: %v", err)
	}
	if len(rejected) != 2 {
		t.Errorf("expected 2 rejected items (SQL + XSS), got %d", len(rejected))
	}
	for _, r := range rejected {
		t.Logf("Rejected item: id=%d, reason=%q", r.ID, r.ValidationNotes)
	}
}

// ============================================================================
// Helper: Event Publish Tracer
// ============================================================================

func TestQuarantineServiceEmitsEvents(t *testing.T) {
	// axiom:trace work_item=WI-004 plan=phase-6/task-2/step-7
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	events := make([]string, 0)
	qs := NewQuarantineService(database, func(sessionID, eventType string, eventData any) {
		events = append(events, eventType)
	})

	// Insert should emit "quarantine_pending"
	item, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
		SessionID:   "test-session",
		SourceType:  "api_response",
		RawContent:  "test event",
		ContentHash: ContentHash("test event"),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Approve should emit "quarantine_approved"
	_, err = qs.ApproveQuarantine(ctx, item.ID, "test-session")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %v", len(events), events)
	}

	if events[0] != "quarantine_pending" {
		t.Errorf("expected first event 'quarantine_pending', got %q", events[0])
	}
	if events[1] != "quarantine_approved" {
		t.Errorf("expected second event 'quarantine_approved', got %q", events[1])
	}
	t.Logf("Events emitted: %v", events)
}


