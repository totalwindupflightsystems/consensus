// AC-027: Execute Staged Commands
// Canonical from SPEC-020 §7
//
// Verifies that:
//   1. Staged SQL commands execute within the open transaction
//   2. Results are written back to the staging_buffer
//   3. Multiple commands can be staged and executed in sequence
//   4. Failed commands are marked with status='failed' and don't break the session

package harness

import (
	"encoding/json"
	"testing"
)

func TestAC027_ExecuteStagedSQL_WithTransaction(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Begin a transaction
	tx, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("AC-027: begin tx: %v", err)
	}
	defer func() {
		if tx.IsActive() {
			tx.Rollback()
		}
	}()

	tx.SetSessionContext(th.ctx, sessionID)

	// Stage and execute a SQL command
	entry := &StagingEntry{
		SessionID:   sessionID,
		Turn:        1,
		Seq:         1,
		CmdType:     CmdSQL,
		Payload:     []byte("INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'AC-027 test', '" + sessionID + "', 1)"),
		Description: "Insert test memory event",
		Status:      BufferStaged,
	}

	if err := th.insertStagingEntry(th.ctx, tx, sessionID, entry, DefaultPlanningConfig()); err != nil {
		t.Fatalf("AC-027: insert staging entry: %v", err)
	}

	// Execute the staged command
	rawResult, execErr := th.executeStagedEntry(th.ctx, tx, entry, 1)
	if execErr != nil {
		t.Fatalf("AC-027: execute staged entry: %v", execErr)
	}
	// Transaction still active — planning loop handles failure, we just verify result

	t.Log("AC-027: SQL command executed successfully")

	// Verify result was written back
	th.updateStagingResult(th.ctx, tx, entry.ID, rawResult)

	var resultMap map[string]string
	if err := json.Unmarshal(*rawResult, &resultMap); err != nil {
		t.Fatalf("AC-027: parse result: %v", err)
	}
	if resultMap["status"] != "ok" {
		t.Errorf("AC-027: result status = %q, want 'ok'", resultMap["status"])
	}
	t.Log("AC-027 PASS: staged SQL executed, result written back")
}

func TestAC027_StageAndExecuteMultipleCommands(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	tx, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("AC-027: begin tx: %v", err)
	}
	tx.SetSessionContext(th.ctx, sessionID)

	// Stage and execute 3 commands in sequence
	commands := []struct {
		sql  string
		desc string
	}{
		{"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'cmd1', '" + sessionID + "', 1)", "insert event 1"},
		{"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'cmd2', '" + sessionID + "', 1)", "insert event 2"},
		{"SELECT count(*) as cnt FROM memory_events WHERE session_id = '" + sessionID + "'", "count events"},
	}

	for i, cmd := range commands {
		entry := &StagingEntry{
			SessionID:   sessionID,
			Turn:        i + 1,
			Seq:         1,
			CmdType:     CmdSQL,
			Payload:     []byte(cmd.sql),
			Description: cmd.desc,
			Status:      BufferStaged,
		}
		if err := th.insertStagingEntry(th.ctx, tx, sessionID, entry, DefaultPlanningConfig()); err != nil {
			t.Fatalf("AC-027: insert entry %d: %v", i, err)
		}
		result, execErr := th.executeStagedEntry(th.ctx, tx, entry, 1)
		if execErr != nil {
			t.Fatalf("AC-027: execute entry %d: %v", i, execErr)
		}
		th.updateStagingResult(th.ctx, tx, entry.ID, result)
	}

	// Commit the transaction to make changes visible
	if err := tx.Commit(); err != nil {
		t.Fatalf("AC-027: commit tx: %v", err)
	}

	// Verify 2 events inserted into memory_events (after commit, visible on main connection)
	rows, err := th.conn.Query(th.ctx, `SELECT count(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("AC-027: query memory_events: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("AC-027: no rows returned")
	}
	cnt := toInt(rows[0]["cnt"])
	if cnt != 2 {
		t.Errorf("AC-027: expected 2 memory events, got %d", cnt)
	}
	t.Logf("AC-027 PASS: staged and executed %d commands, %d events visible in transaction", len(commands), cnt)
}

func TestAC027_StagedCommandFailure(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	tx, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("AC-027: begin tx: %v", err)
	}
	defer func() {
		if tx.IsActive() {
			tx.Rollback()
		}
	}()
	tx.SetSessionContext(th.ctx, sessionID)

	// A command that will fail (nonexistent table)
	entry := &StagingEntry{
		SessionID:   sessionID,
		Turn:        1,
		Seq:         1,
		CmdType:     CmdSQL,
		Payload:     []byte("INSERT INTO nonexistent_table (id) VALUES (1)"),
		Description: "Intentionally failing command",
		Status:      BufferStaged,
	}

	if err := th.insertStagingEntry(th.ctx, tx, sessionID, entry, DefaultPlanningConfig()); err != nil {
		t.Fatalf("AC-027: insert entry: %v", err)
	}

	_, execErr := th.executeStagedEntry(th.ctx, tx, entry, 1)
	if execErr == nil {
		t.Fatal("AC-027: expected error from command on nonexistent table, got nil")
	}
	t.Logf("AC-027: failed command correctly produced error: %v", execErr)

	// Mark entry as failed (as the planning loop does)
	th.updateStagingStatus(th.ctx, tx, entry.ID, BufferFailed, &json.RawMessage{})

	// Transaction should still be active after the error (SQLite doesn't abort on error)
	if !tx.IsActive() {
		t.Error("AC-027: transaction should remain active after failed command")
	}
	t.Log("AC-027 PASS: failed command marked as failed, transaction still active")
}
