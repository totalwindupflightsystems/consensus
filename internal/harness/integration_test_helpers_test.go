// Package harness: regression test for SQLite test-harness teardown cleanup.
//
// The test harness opens its temp database in WAL mode (the sqlite driver
// forces journal_mode=WAL), which materializes <tmpPath>-wal and
// <tmpPath>-shm sidecars next to the main database file. close() must
// remove all three, not only the .db file.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/008-harness.md
package harness

import (
	"os"
	"testing"
)

// TestTestHarnessClose_RemovesSQLiteSidecars proves that testHarness.close()
// cleans up the WAL and SHM sidecars SQLite creates beside the temp database,
// in addition to the temp database itself.
//
// Non-vacuity: the premise establishes that the sidecar paths are actually
// occupied at close() time by holding an open read transaction across the
// close (SQLite's clean last-close checkpoint deletes the sidecars itself
// when every connection is closed — the leak this regression test guards
// against only manifests while a pool connection is still live, the same
// shape as a test failing mid-transaction and Goexit'ing past its deferred
// close).
func TestTestHarnessClose_RemovesSQLiteSidecars(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}

	// Premise: commit real work so the WAL sidecar materializes on disk.
	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	if _, err := th.RunAgentIteration(th.ctx, sessionID); err != nil {
		t.Fatalf("RunAgentIteration failed: %v", err)
	}

	// Hold one pool connection open across close() so SQLite's clean
	// last-close sidecar removal cannot fire and mask a broken close().
	// This mirrors the real leak shape: a test that fails mid-transaction
	// reaches t.Cleanup/defer close with a connection still checked out.
	tx, err := th.conn.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("failed to begin premise transaction: %v", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
			tx = nil
		}
	}()
	if err := tx.Exec(th.ctx, `INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'premise-agent', 'test-model', 'idle', 'high', 'premise tx')`, "bbbbbbbb-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("premise transaction write failed: %v", err)
	}

	dbPath := th.tmpPath
	walPath := th.tmpPath + "-wal"
	shmPath := th.tmpPath + "-shm"

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("premise: temp db %s missing before close: %v", dbPath, err)
	}
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("premise: WAL sidecar %s missing before close (WAL-mode writes should materialize it): %v", walPath, err)
	}
	if _, err := os.Stat(shmPath); err != nil {
		t.Logf("note: SHM sidecar %s not present before close: %v", shmPath, err)
	}

	th.close()
	tx = nil // close() owns teardown from here; nothing to roll back

	// After close(): none of the three files may remain. os.Stat must report
	// "not exist" — any other error is also a failure.
	for _, tc := range []struct {
		path  string
		label string
	}{
		{dbPath, "temp db"},
		{walPath, "WAL sidecar"},
		{shmPath, "SHM sidecar"},
	} {
		if _, err := os.Stat(tc.path); err == nil {
			t.Errorf("close() left %s behind: %s", tc.label, tc.path)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat %s (%s) after close: %v", tc.path, tc.label, err)
		}
	}
}
