// Package memory: tests for duplicate user_message suppression (DF-CONSENSUS-9).
//
// axiom:trace work_item=DF-CONSENSUS-9 spec=docs/API.md plan=DF-CONSENSUS-9/task-4 test=internal/memory/dedupe_test.go
package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// newDedupeDB opens a scratch in-memory SQLite database holding the
// memory_events table the probe reads. Timestamps are inserted exactly the way
// the production writers store them: UTC RFC3339 text.
func newDedupeDB(t *testing.T) db.DB {
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
	if err := database.Exec(ctx, `CREATE TABLE IF NOT EXISTS memory_events (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		type              TEXT NOT NULL,
		content           TEXT NOT NULL,
		session_id        TEXT NOT NULL,
		iteration_created INTEGER NOT NULL DEFAULT 0,
		created_at        TEXT
	)`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return database
}

// insertUserMessage stores one 'user_message' row with an explicit timestamp,
// mirroring the production INSERT shape (UTC RFC3339 text).
func insertUserMessage(t *testing.T, database db.DB, sessionID, content string, at time.Time) {
	t.Helper()
	if err := database.Exec(context.Background(),
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 1, $3)`,
		content, sessionID, at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert user_message: %v", err)
	}
}

func TestIsDuplicateUserMessage_InWindow(t *testing.T) {
	database := newDedupeDB(t)
	first := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	insertUserMessage(t, database, "sess-1", "fix the login bug", first)

	// 2s later — inside the 5s window — the same content must be flagged.
	for _, offset := range []time.Duration{0, 2 * time.Second, UserMessageDuplicateWindow} {
		got, err := IsDuplicateUserMessage(context.Background(), database, "sess-1", "fix the login bug", first.Add(offset))
		if err != nil {
			t.Fatalf("offset %s: %v", offset, err)
		}
		if !got {
			t.Errorf("offset %s: expected duplicate, got none", offset)
		}
	}
}

func TestIsDuplicateUserMessage_OutsideWindow(t *testing.T) {
	database := newDedupeDB(t)
	first := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	insertUserMessage(t, database, "sess-1", "fix the login bug", first)

	got, err := IsDuplicateUserMessage(context.Background(), database, "sess-1", "fix the login bug", first.Add(UserMessageDuplicateWindow+time.Second))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got {
		t.Error("a repeat after the dedupe window must not be flagged as duplicate")
	}
}

func TestIsDuplicateUserMessage_DifferentContentOrSession(t *testing.T) {
	database := newDedupeDB(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	insertUserMessage(t, database, "sess-1", "fix the login bug", now)

	for _, tc := range []struct{ sessionID, content string }{
		{"sess-1", "fix the signup bug"}, // same session, different content
		{"sess-2", "fix the login bug"},  // same content, different session
	} {
		got, err := IsDuplicateUserMessage(context.Background(), database, tc.sessionID, tc.content, now.Add(time.Second))
		if err != nil {
			t.Fatalf("probe %v: %v", tc, err)
		}
		if got {
			t.Errorf("probe %+v: unexpected duplicate", tc)
		}
	}
}

func TestIsDuplicateUserMessage_UnparseableTimestampsSkipped(t *testing.T) {
	database := newDedupeDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// A writer that stored a non-RFC3339 stamp must not be treated as a
	// duplicate (and must not break the probe for rows it can parse).
	if err := database.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, 1, 'not-a-timestamp')`,
		"fix the login bug", "sess-1"); err != nil {
		t.Fatalf("insert unparseable row: %v", err)
	}
	got, err := IsDuplicateUserMessage(ctx, database, "sess-1", "fix the login bug", now)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got {
		t.Error("unparseable timestamp must be skipped, not counted as duplicate")
	}

	// The parseable row below the broken one is still honored.
	insertUserMessage(t, database, "sess-1", "fix the login bug", now.Add(-time.Second))
	got, err = IsDuplicateUserMessage(ctx, database, "sess-1", "fix the login bug", now)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !got {
		t.Error("parseable in-window row must still be detected")
	}
}

func TestIsDuplicateUserMessage_ScansBeyondMostRecentRows(t *testing.T) {
	database := newDedupeDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	content := "run the deploy"

	// The in-window row sits behind 25 newer different-content rows, past the
	// probe's LIMIT 20 scan on identical content.
	insertUserMessage(t, database, "sess-1", content, now.Add(-time.Second))
	for i := 0; i < 25; i++ {
		insertUserMessage(t, database, "sess-1", fmt.Sprintf("unrelated message %d", i), now)
	}
	got, err := IsDuplicateUserMessage(ctx, database, "sess-1", content, now)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !got {
		t.Error("in-window duplicate behind newer different rows must still be detected")
	}
}

func TestSkipDuplicateUserMessage(t *testing.T) {
	database := newDedupeDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	insertUserMessage(t, database, "sess-1", "hello", now.Add(-time.Second))

	got := SkipDuplicateUserMessage(ctx, database, "sess-1", "hello")
	if !got {
		t.Error("expected in-window duplicate to be skipped")
	}

	got = SkipDuplicateUserMessage(ctx, database, "sess-1", "a different message")
	if got {
		t.Error("different content must not be skipped")
	}

	// A broken database degrades to deliver-anyway, never to drop.
	got = SkipDuplicateUserMessage(ctx, database, "sess-1", "hello")
	_ = database.Close()
	got = SkipDuplicateUserMessage(ctx, database, "sess-1", "hello after close")
	if got {
		t.Error("probe failure must report not-duplicate (deliver anyway)")
	}

	if SkipDuplicateUserMessage(ctx, nil, "sess-1", "hello") {
		t.Error("nil database must report not-duplicate")
	}
	if SkipDuplicateUserMessage(ctx, database, "  ", "hello") {
		t.Error("blank session id must report not-duplicate")
	}
}
