package webhook

import (
	"context"
	"strings"
	"testing"
)

// DF-CONSENSUS-46: a webhook routed to a paused session must do more than
// flip its status. The harness heartbeat loop dispatches only sessions in
// 'thinking'/'planning'/'tool_exec', so the old paused→'idle' flip stranded
// the session with unanswered input and a payload no agent ever saw. The
// routed wake must deliver the payload as a 'user_message' memory event
// (the same shape the message API uses — context.go projects those rows as
// LLM user turns), flip the session to 'thinking' with an iteration bump,
// and fire the wake signal so dispatch is immediate, not next-tick.
func TestRoutedWakeDeliversPayloadAndDispatchesIteration(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	// Production-shaped fixtures: sessions with every column the wake
	// touches (status/iteration/heartbeat_at/deleted_at), memory_events
	// (message ledger), routing_rules (SPEC-013 §5).
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'idle',
		iteration BIGINT NOT NULL DEFAULT 0,
		heartbeat_at TEXT NOT NULL DEFAULT (datetime('now')),
		deleted_at TEXT
	)`)
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS memory_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		content TEXT NOT NULL,
		summary_text TEXT,
		session_id TEXT NOT NULL,
		iteration_created BIGINT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS routing_rules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, source_pattern TEXT,
		event_type_pattern TEXT, payload_pattern TEXT,
		target_session_id TEXT, target_workflow_id TEXT,
		priority INTEGER NOT NULL DEFAULT 5, enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`)

	// A paused session sitting on an unanswered user message (the board
	// row's exact shape), iteration 3.
	mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, status, iteration) VALUES ('paused-target', 'test-agent', 'paused', 3)`)
	mustExec(t, database, ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('user_message', 'unanswered earlier question', 'paused-target', 3)`)

	// Routing rule: webhook/ping → paused-target; one pending ping event.
	mustExec(t, database, ctx, `INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, target_session_id, priority, enabled, created_at) VALUES ('r1', 'wake-rule', 'webhook', 'ping', 'paused-target', 1, 1, datetime('now'))`)
	mustExec(t, database, ctx, `INSERT INTO external_events (source, source_id, event_type, payload, signature_valid, status, session_id, workflow_id) VALUES ('webhook', 'wake-test-1', 'ping', '{"event":"ping","message":"deploy finished"}', 1, 'pending', NULL, NULL)`)

	// Wake recorder — the seam main.go wires to Harness.RequestWake.
	var woken []string
	store.SetWake(func(sessionID string) { woken = append(woken, sessionID) })

	if err := store.routePendingEvents(ctx); err != nil {
		t.Fatalf("routing: %v", err)
	}

	// The event itself is routed with its target attached.
	eRows, err := database.Query(ctx, `SELECT status, session_id FROM external_events WHERE source_id = 'wake-test-1'`)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if len(eRows) != 1 || eRows[0]["status"] != "routed" || eRows[0]["session_id"] != "paused-target" {
		t.Fatalf("event not routed to paused-target: %+v", eRows)
	}

	// 1. Dispatch-eligible: status 'thinking', iteration bumped 3 → 4.
	sRows, err := database.Query(ctx, `SELECT status, iteration FROM sessions WHERE id = 'paused-target'`)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if len(sRows) != 1 {
		t.Fatalf("sessions has %d rows for paused-target, want 1", len(sRows))
	}
	if got := toString(sRows[0]["status"]); got != "thinking" {
		t.Errorf("status after routed wake = %q, want thinking (heartbeat loop only dispatches thinking/planning/tool_exec)", got)
	}
	if got := toInt64(sRows[0]["iteration"]); got != 4 {
		t.Errorf("iteration after routed wake = %d, want 4", got)
	}

	// 2. The payload is visible to the agent: a new user_message row
	// carrying the webhook content, stamped for the iteration that runs.
	mRows, err := database.Query(ctx, `SELECT type, content, iteration_created FROM memory_events WHERE session_id = 'paused-target' ORDER BY id`)
	if err != nil {
		t.Fatalf("query memory_events: %v", err)
	}
	if len(mRows) != 2 {
		t.Fatalf("memory_events has %d rows, want 2 (unanswered message + webhook payload)", len(mRows))
	}
	delivered := mRows[1]
	if got := toString(delivered["type"]); got != "user_message" {
		t.Errorf("delivered event type = %q, want user_message", got)
	}
	if got := toString(delivered["content"]); !strings.Contains(got, "deploy finished") {
		t.Errorf("delivered content %q does not carry the webhook payload", got)
	}
	if got := toInt64(delivered["iteration_created"]); got != 4 {
		t.Errorf("delivered iteration_created = %d, want 4 (the iteration that will run)", got)
	}

	// 3. The wake signal fired so dispatch is immediate.
	if len(woken) != 1 || woken[0] != "paused-target" {
		t.Errorf("wake signals = %v, want [paused-target]", woken)
	}

	// 4. Second wake while the session is already 'thinking' (the board row's
	// repeat case): the event still routes and the NEW payload is still
	// delivered (the running iteration's successor must see it), but there
	// is no double dispatch, no iteration churn.
	mustExec(t, database, ctx, `INSERT INTO external_events (source, source_id, event_type, payload, signature_valid, status, session_id, workflow_id) VALUES ('webhook', 'wake-test-2', 'ping', '{"event":"ping","message":"second"}', 1, 'pending', NULL, NULL)`)
	woken = nil
	if err := store.routePendingEvents(ctx); err != nil {
		t.Fatalf("second routing: %v", err)
	}
	eRows, err = database.Query(ctx, `SELECT status FROM external_events WHERE source_id = 'wake-test-2'`)
	if err != nil {
		t.Fatalf("query second event: %v", err)
	}
	if len(eRows) != 1 || eRows[0]["status"] != "routed" {
		t.Errorf("second event not routed: %+v", eRows)
	}
	sRows, err = database.Query(ctx, `SELECT status, iteration FROM sessions WHERE id = 'paused-target'`)
	if err != nil {
		t.Fatalf("query session after second wake: %v", err)
	}
	if got := toString(sRows[0]["status"]); got != "thinking" {
		t.Errorf("status after second wake = %q, want unchanged thinking", got)
	}
	if got := toInt64(sRows[0]["iteration"]); got != 4 {
		t.Errorf("iteration after second wake = %d, want unchanged 4 (no double dispatch)", got)
	}
	mRows, err = database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = 'paused-target' ORDER BY id`)
	if err != nil {
		t.Fatalf("query memory_events after second wake: %v", err)
	}
	if len(mRows) != 3 {
		t.Errorf("memory_events has %d rows after second wake, want 3 (both payloads delivered)", len(mRows))
	} else if got := toString(mRows[2]["content"]); !strings.Contains(got, "second") {
		t.Errorf("second delivered content %q does not carry the second payload", got)
	}
	if len(woken) != 0 {
		t.Errorf("wake signals after second wake = %v, want none (session already dispatching)", woken)
	}
}
