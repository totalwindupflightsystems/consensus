// Package harness: regression test for DF-CONSENSUS-28 — a soft-deleted
// (tombstoned) session must never be claimed by the harness dispatch loop.
package harness

import (
	"context"
	"testing"
)

// TestFindActiveSessionsExcludesSoftDeleted proves findActiveSessions skips
// sessions carrying a deleted_at tombstone, even while their status is still
// 'thinking' (deleted mid-iteration). A live thinking session is still found.
func TestFindActiveSessionsExcludesSoftDeleted(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	ctx := context.Background()

	// findActiveSessions filters on heartbeat_at and deleted_at; the minimal
	// context-test schema has neither.
	mustExec(t, database, `ALTER TABLE sessions ADD COLUMN heartbeat_at TEXT`)
	mustExec(t, database, `ALTER TABLE sessions ADD COLUMN deleted_at TEXT`)

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, heartbeat_at)
		VALUES ('h-softdel-live', 'harness-test', 'mock', 'thinking', datetime('now'))`)
	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, heartbeat_at, deleted_at)
		VALUES ('h-softdel-dead', 'harness-test', 'mock', 'thinking', datetime('now'), datetime('now'))`)

	h := New(database, nil)
	ids, err := h.findActiveSessions(ctx)
	if err != nil {
		t.Fatalf("findActiveSessions: %v", err)
	}

	foundLive, foundDead := false, false
	for _, id := range ids {
		switch id {
		case "h-softdel-live":
			foundLive = true
		case "h-softdel-dead":
			foundDead = true
		}
	}
	if !foundLive {
		t.Errorf("expected live thinking session to be claimed; got %v", ids)
	}
	if foundDead {
		t.Errorf("soft-deleted session was claimed for dispatch (deleted_at tombstone ignored); got %v", ids)
	}
}
