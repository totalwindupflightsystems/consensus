// AC-026: Staging Buffer Persistence
// Canonical from SPEC-020 §4
//
// Verifies that:
//   1. staging_buffer table exists and accepts INSERT
//   2. INSERTed commands can be queried back with correct fields
//   3. loadStagingBuffer returns entries in turn/seq order
//   4. Only active (staged/executed) entries are loaded — committed entries excluded

package harness

import (
	"encoding/json"
	"testing"
)

func TestAC026_StagingBuffer_InsertAndQuery(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert 3 staged commands into the staging_buffer
	payloads := []string{"SELECT 1", "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'test', '" + sessionID + "', 1)", "SELECT count(*) FROM memory_events"}
	descs := []string{"Simple select", "Insert memory event", "Verify count"}
	cmds := []string{"sql", "sql", "sql"}

	for i, p := range payloads {
		payloadJSON, _ := json.Marshal(p)
		err := th.conn.Exec(th.ctx, `
			INSERT INTO staging_buffer (session_id, iteration, turn, seq, cmd_type, payload, description, status, created_at)
			VALUES ($1, 1, $2, 1, $3, $4, $5, 'staged', datetime('now'))
		`, sessionID, i+1, cmds[i], string(payloadJSON), descs[i])
		if err != nil {
			t.Fatalf("AC-026: insert staging entry %d: %v", i, err)
		}
	}
	t.Log("AC-026: 3 staging entries inserted")

	// Query them back
	rows, err := th.conn.Query(th.ctx, `
		SELECT id, turn, seq, cmd_type, description, status
		FROM staging_buffer
		WHERE session_id = $1
		ORDER BY turn, seq
	`, sessionID)
	if err != nil {
		t.Fatalf("AC-026: query staging_buffer: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("AC-026: expected 3 staging entries, got %d", len(rows))
	}
	t.Logf("AC-026 PASS: 3 entries found in staging_buffer")

	// Verify loadStagingBuffer works
	buffer, err := th.loadStagingBuffer(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("AC-026: loadStagingBuffer: %v", err)
	}
	if len(buffer.Entries) != 3 {
		t.Fatalf("AC-026: expected 3 entries from loadStagingBuffer, got %d", len(buffer.Entries))
	}
	if !buffer.IsActive {
		t.Error("AC-026: buffer should be active")
	}
	if buffer.Entries[0].Description != "Simple select" {
		t.Errorf("AC-026: entry 0 description = %q, want 'Simple select'", buffer.Entries[0].Description)
	}
	if buffer.Entries[1].CmdType != CmdSQL {
		t.Errorf("AC-026: entry 1 cmd_type = %s, want sql", buffer.Entries[1].CmdType)
	}
	if buffer.Entries[2].Status != BufferStaged {
		t.Errorf("AC-026: entry 2 status = %s, want staged", buffer.Entries[2].Status)
	}
	t.Log("AC-026 PASS: loadStagingBuffer returns correct entries in order")
}

func TestAC026_StagingBuffer_ExcludesCommitted(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Insert one staged and one committed entry
	pay1, _ := json.Marshal("SELECT 1")
	pay2, _ := json.Marshal("SELECT 2")
	th.conn.Exec(th.ctx, `
		INSERT INTO staging_buffer (session_id, iteration, turn, seq, cmd_type, payload, description, status, created_at)
		VALUES ($1, 1, 1, 1, 'sql', $2, 'active', 'staged', datetime('now'))
	`, sessionID, string(pay1))
	th.conn.Exec(th.ctx, `
		INSERT INTO staging_buffer (session_id, iteration, turn, seq, cmd_type, payload, description, status, created_at)
		VALUES ($1, 1, 2, 1, 'sql', $2, 'finalized', 'committed', datetime('now'))
	`, sessionID, string(pay2))

	buffer, err := th.loadStagingBuffer(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("AC-026: loadStagingBuffer: %v", err)
	}
	if len(buffer.Entries) != 1 {
		t.Fatalf("AC-026: expected 1 active entry, got %d", len(buffer.Entries))
	}
	if buffer.Entries[0].Description != "active" {
		t.Errorf("AC-026: expected 'active' entry, got %q", buffer.Entries[0].Description)
	}
	t.Log("AC-026 PASS: committed entries excluded from staging buffer")
}
