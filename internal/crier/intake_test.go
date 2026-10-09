package crier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// Happy path — a crier message becomes an agent-visible user turn
// ============================================================================

// TestIntake_CrierMessageBecomesAgentVisibleUserMessage is the CR-IN-001
// acceptance cell: a message delivered through crier's durable inbox is
// retrieved by the consensus agent, lands in the session ledger as a
// 'user_message' row stamped for the iteration that will run, carries crier
// provenance naming the message id it came from, and is acked so the inbox
// drains. A second pass over the drained inbox appends nothing.
func TestIntake_CrierMessageBecomesAgentVisibleUserMessage(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)

	const nonce = "cr-in-001-nonce-7f4c2b"
	messageID := fake.deliver("consensus", map[string]any{
		"source": "crier-producer",
		"topic":  "consensus.inbox",
		"text":   "please summarise the release notes: " + nonce,
	})

	database, sessionID := newTestLedger(t)
	intake := NewIntake(NewClient(srv.URL), &DBStore{DB: database})

	result, err := intake.Run(context.Background(), sessionID, "consensus")
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if result.Retrieved != 1 || result.Appended != 1 || result.Acked != 1 || result.Skipped != 0 {
		t.Errorf("result = %+v, want retrieved 1, appended 1, acked 1, skipped 0", result)
	}
	if len(result.MessageIDs) != 1 || result.MessageIDs[0] != messageID {
		t.Errorf("result message ids = %v, want [%s] (crier's own id)", result.MessageIDs, messageID)
	}

	rows := ledgerRows(t, database, sessionID)
	if len(rows) != 1 {
		t.Fatalf("memory_events holds %d rows, want 1", len(rows))
	}
	row := rows[0]
	if got := rowString(row, "type"); got != "user_message" {
		t.Errorf("row type = %q, want user_message (the harness projects these as LLM user turns)", got)
	}
	content := rowString(row, "content")
	if !strings.Contains(content, nonce) {
		t.Errorf("content %q does not carry the message body", content)
	}
	// Provenance: origin, the crier message id, the source agent and the
	// topic — all in the one field the ledger has room for.
	for _, want := range []string{
		ProvenanceOrigin,
		"agent=consensus",
		"msg=" + messageID,
		"from=crier-producer",
		"topic=consensus.inbox",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("content %q is missing provenance %q", content, want)
		}
	}
	if got := rowInt64(row, "iteration_created"); got != 4 {
		t.Errorf("iteration_created = %d, want 4 (the session's iteration 3 plus one)", got)
	}

	if ids := fake.ackedIDs(); len(ids) != 1 || ids[0] != messageID {
		t.Errorf("acked ids = %v, want [%s] — the lease must be released", ids, messageID)
	}
	if ids := fake.pending("consensus"); len(ids) != 0 {
		t.Errorf("inbox still holds %v after intake, want drained", ids)
	}

	// A second pass over the drained inbox is a no-op: nothing is claimed, so
	// nothing is appended (no duplicate user turn).
	second, err := intake.Run(context.Background(), sessionID, "consensus")
	if err != nil {
		t.Fatalf("second intake: %v", err)
	}
	if second.Retrieved != 0 || second.Appended != 0 || second.Acked != 0 {
		t.Errorf("second pass = %+v, want a no-op over the drained inbox", second)
	}
	if rows := ledgerRows(t, database, sessionID); len(rows) != 1 {
		t.Errorf("memory_events holds %d rows after the second pass, want 1", len(rows))
	}
}

// ============================================================================
// Negative path — another agent's message is NOT consumed
// ============================================================================

// TestIntake_DifferentAgentMessageIsNotConsumed delivers one message to
// another crier agent and one to the consuming agent, then runs intake for the
// consumer: only its own message may reach the session ledger, and the other
// agent's message must still be sitting in ITS inbox, unacked and unconsumed.
//
// The fake enforces crier's identity rules, so this is a real isolation
// assertion: the other agent's message is not merely invisible because the
// store was empty — it exists on the same server and stays pending.
func TestIntake_DifferentAgentMessageIsNotConsumed(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)

	foreignID := fake.deliver("other-agent", map[string]any{
		"source": "someone-else",
		"text":   "this belongs to another agent",
	})
	ownID := fake.deliver("consensus", map[string]any{
		"source": "crier-producer",
		"text":   "this one is for us",
	})

	database, sessionID := newTestLedger(t)
	intake := NewIntake(NewClient(srv.URL), &DBStore{DB: database})

	result, err := intake.Run(context.Background(), sessionID, "consensus")
	if err != nil {
		t.Fatalf("intake: %v", err)
	}
	if result.Retrieved != 1 || result.Appended != 1 {
		t.Fatalf("result = %+v, want exactly its own message retrieved and appended", result)
	}
	if result.MessageIDs[0] != ownID {
		t.Errorf("consumed %v, want only %s", result.MessageIDs, ownID)
	}

	rows := ledgerRows(t, database, sessionID)
	if len(rows) != 1 {
		t.Fatalf("memory_events holds %d rows, want 1", len(rows))
	}
	if content := rowString(rows[0], "content"); !strings.Contains(content, "this one is for us") {
		t.Errorf("content = %q, want the consuming agent's own message", content)
	}
	if content := rowString(rows[0], "content"); strings.Contains(content, foreignID) {
		t.Errorf("content = %q carries the other agent's message id", content)
	}

	// The other agent's message is untouched: still queued, never acked.
	if ids := fake.pending("other-agent"); len(ids) != 1 || ids[0] != foreignID {
		t.Errorf("other-agent inbox = %v, want [%s] still pending", ids, foreignID)
	}
	for _, acked := range fake.ackedIDs() {
		if acked == foreignID {
			t.Errorf("the run acked %s, a message addressed to a different agent", foreignID)
		}
	}
}

// ============================================================================
// Empty inbox — a no-op, not an error
// ============================================================================

// TestIntake_EmptyInboxIsNoOp pins the empty-inbox case: nothing retrieved,
// nothing appended, nothing acked, no error — and no ack call is made, because
// an empty read carries no lease to ack.
func TestIntake_EmptyInboxIsNoOp(t *testing.T) {
	fake := newFakeCrier()
	fake.register("consensus")
	srv := fake.start(t)

	database, sessionID := newTestLedger(t)
	intake := NewIntake(NewClient(srv.URL), &DBStore{DB: database})

	result, err := intake.Run(context.Background(), sessionID, "consensus")
	if err != nil {
		t.Fatalf("intake over an empty inbox: %v", err)
	}
	if result.Retrieved != 0 || result.Appended != 0 || result.Acked != 0 || result.Skipped != 0 {
		t.Errorf("result = %+v, want a no-op", result)
	}
	if len(result.MessageIDs) != 0 {
		t.Errorf("message ids = %v, want none", result.MessageIDs)
	}
	if rows := ledgerRows(t, database, sessionID); len(rows) != 0 {
		t.Errorf("memory_events holds %d rows, want 0", len(rows))
	}
	if got := fake.ackedIDs(); len(got) != 0 {
		t.Errorf("acked %v, want no ack call for an empty read", got)
	}
	if got := fake.retrieveCount(); got != 1 {
		t.Errorf("retrieve calls = %d, want 1", got)
	}
}

// ============================================================================
// Failure path — nothing is acked when the ledger write fails
// ============================================================================

// TestIntake_AppendFailureLeavesTheBatchUnacked proves the at-least-once
// property: a message crier handed over is never silently dropped, because a
// failed ledger write leaves the whole batch unacked and crier redelivers it
// after the lease expires.
func TestIntake_AppendFailureLeavesTheBatchUnacked(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)
	messageID := fake.deliver("consensus", map[string]any{"text": "must not be lost"})

	intake := NewIntake(NewClient(srv.URL), failingStore{err: errors.New("ledger unavailable")})

	result, err := intake.Run(context.Background(), "sess-crier-intake", "consensus")
	if err == nil {
		t.Fatal("intake must fail when the ledger write fails")
	}
	if result != nil && (result.Appended != 0 || result.Acked != 0) {
		t.Errorf("result = %+v, want nothing appended and nothing acked", result)
	}
	if ids := fake.ackedIDs(); len(ids) != 0 {
		t.Errorf("acked %v after a failed write, want nothing acked (the message must be redelivered)", ids)
	}
	if ids := fake.pending("consensus"); len(ids) != 1 || ids[0] != messageID {
		t.Errorf("inbox = %v, want [%s] still pending", ids, messageID)
	}
}

// ============================================================================
// Argument validation
// ============================================================================

func TestIntake_RequiresSessionAndAgent(t *testing.T) {
	fake := newFakeCrier()
	srv := fake.start(t)
	intake := NewIntake(NewClient(srv.URL), &DBStore{})

	if _, err := intake.Run(context.Background(), "", "consensus"); err == nil {
		t.Error("intake with no session id must fail")
	}
	if _, err := intake.Run(context.Background(), "sess-1", ""); err == nil {
		t.Error("intake with no agent name must fail")
	}
	if got := fake.retrieveCount(); got != 0 {
		t.Errorf("retrieve calls = %d, want 0 (validation precedes the read)", got)
	}
}

// ============================================================================
// Payload decoding
// ============================================================================

// TestParseInbound_Shapes is the decoding table: the documented producer
// object, a bare JSON string, an unexpected shape that must survive verbatim,
// and the two payloads that carry no agent-visible body at all.
func TestParseInbound_Shapes(t *testing.T) {
	cases := []struct {
		name       string
		payload    string
		sender     string
		wantOK     bool
		wantBody   string
		wantSource string
		wantTopic  string
	}{
		{
			name:       "producer object",
			payload:    `{"source":"muster","topic":"consensus.inbox","text":"hello from a Muster-driven integration"}`,
			wantOK:     true,
			wantBody:   "hello from a Muster-driven integration",
			wantSource: "muster",
			wantTopic:  "consensus.inbox",
		},
		{
			name:       "content key is accepted when text is absent",
			payload:    `{"content":"body via content"}`,
			sender:     "crier-recorded-sender",
			wantOK:     true,
			wantBody:   "body via content",
			wantSource: "crier-recorded-sender",
		},
		{
			name:       "bare JSON string",
			payload:    `"just a string"`,
			wantOK:     true,
			wantBody:   "just a string",
			wantSource: provenanceUnknown,
		},
		{
			name:       "unexpected shape survives verbatim",
			payload:    `{"round_trip":"cr-consensus-1"}`,
			wantOK:     true,
			wantBody:   `{"round_trip":"cr-consensus-1"}`,
			wantSource: provenanceUnknown,
		},
		{
			name:     "empty payload carries no body",
			payload:  ``,
			wantOK:   false,
			wantBody: "",
		},
		{
			name:     "null payload carries no body",
			payload:  `null`,
			wantOK:   false,
			wantBody: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := InboxEntry{ID: "msg-1", AgentID: "consensus", Payload: []byte(tc.payload), Sender: tc.sender}
			msg, ok := ParseInbound(entry)
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
			if msg.Body != tc.wantBody {
				t.Errorf("body = %q, want %q", msg.Body, tc.wantBody)
			}
			if tc.wantOK && msg.Source != tc.wantSource {
				t.Errorf("source = %q, want %q", msg.Source, tc.wantSource)
			}
			if tc.wantOK && msg.Topic != tc.wantTopic {
				t.Errorf("topic = %q, want %q", msg.Topic, tc.wantTopic)
			}
			if msg.ID != "msg-1" {
				t.Errorf("id = %q, want the crier message id msg-1", msg.ID)
			}
		})
	}
}

// TestProvenanceContent pins the marker shape and the one conditional field:
// a topic belongs to the relay, so an inbox-only producer legitimately has
// none and the marker omits it rather than printing an empty value.
func TestProvenanceContent(t *testing.T) {
	withTopic := ProvenanceContent("consensus", InboundMessage{
		ID: "msg-9", Source: "muster", Topic: "consensus.inbox", Body: "hello",
	})
	want := "[crier agent=consensus msg=msg-9 from=muster topic=consensus.inbox] hello"
	if withTopic != want {
		t.Errorf("content = %q, want %q", withTopic, want)
	}

	withoutTopic := ProvenanceContent("consensus", InboundMessage{ID: "msg-9", Source: provenanceUnknown, Body: "hello"})
	wantNoTopic := "[crier agent=consensus msg=msg-9 from=unknown] hello"
	if withoutTopic != wantNoTopic {
		t.Errorf("content = %q, want %q", withoutTopic, wantNoTopic)
	}
}

// ============================================================================
// Test fixtures
// ============================================================================

// failingStore is a Store whose write always fails.
type failingStore struct{ err error }

func (f failingStore) AppendUserMessage(context.Context, string, string) error { return f.err }

// newTestLedger creates a scratch SQLite database holding the two tables the
// intake touches and a session row at iteration 3, so an append must land at
// iteration 4. The database is in-memory and closed on cleanup — the same
// fixture shape internal/session's tests use.
func newTestLedger(t *testing.T) (db.DB, string) {
	t.Helper()
	ctx := context.Background()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	database, err := driver.Open(ctx, db.Config{
		URL: fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", name),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const sessionID = "sess-crier-intake"
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'idle',
			iteration  INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS memory_events (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			type              TEXT NOT NULL,
			content           TEXT NOT NULL,
			session_id        TEXT NOT NULL REFERENCES sessions(id),
			iteration_created INTEGER NOT NULL DEFAULT 0,
			created_at        TEXT
		)`,
		`INSERT OR IGNORE INTO sessions (id, agent_name, status, iteration)
		 VALUES ('sess-crier-intake', 'consensus', 'idle', 3)`,
	}
	for _, stmt := range statements {
		if err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
	return database, sessionID
}

// ledgerRows reads the session's memory_events rows in insertion order.
func ledgerRows(t *testing.T, database db.DB, sessionID string) []db.Row {
	t.Helper()
	rows, err := database.Query(context.Background(),
		`SELECT id, type, content, iteration_created FROM memory_events
		 WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		t.Fatalf("query memory_events: %v", err)
	}
	return rows
}

// rowString reads a text column from a driver row.
func rowString(row db.Row, key string) string {
	switch v := row[key].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

// rowInt64 reads an integer column from a driver row.
func rowInt64(row db.Row, key string) int64 {
	return asInt64(row[key])
}
