// BUG-014 (part b): the max-turns auto-commit branch used to call
// handleCommitV2 with an empty replyToUser, so the iteration settled with
// ZERO response events and a synchronous waiter could never return — even
// after part (a) re-keyed the waiter on the event-id watermark. This file
// proves the auto-commit path now persists a text_block reply (the last
// turn's monologue, or the deterministic fallback) that a watermark-keyed
// waiter can pick up.
//
// The DF-CONSENSUS-072 respond-promotion path is untouched and still owns
// turns whose message_to_user is non-empty.

package harness

import (
	"strings"
	"testing"
)

func TestBUG014_MaxTurnsAutoCommitPersistsReply(t *testing.T) {
	// The staged memory_state_change deliberately writes OUTSIDE the test
	// session (createTestSession reuses eeeeeeeeeee1, so reusing
	// minimalOutput() would add an in-session text_block that a
	// memory_state_change — not the commit reply — produced). With this
	// output, the ONLY text_block above the watermark in the test session
	// can be the reply handleCommitV2 persists.
	out := &AgentOutput{
		InternalMonologue: "BUG-014 planning monologue: staged the memory write.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('header', 'BUG-014 staged audit row', 'zzzz-not-the-test-session', 1)",
		},
	}
	th, err := newTestHarness(newMockLLM(out))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("BUG-014: set session to thinking: %v", err)
	}

	// Watermark a (a)-shaped waiter would capture before the request:
	// only events with id > this value are eligible answers.
	wmRows, err := th.conn.Query(th.ctx, `SELECT COALESCE(MAX(id), 0) AS max_id FROM memory_events`)
	if err != nil {
		t.Fatalf("BUG-014: read watermark: %v", err)
	}
	watermark := int64(0)
	if len(wmRows) > 0 {
		watermark = toInt64(wmRows[0]["max_id"])
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 2
	cfg.AutoCommitOnMax = true

	if _, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg); err != nil {
		t.Fatalf("BUG-014: RunInteractivePlanning: %v", err)
	}

	// After the auto-commit the session settles idle, exactly the state the
	// shim waiter reads when choosing a settled text_block.
	rows, err := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil || len(rows) == 0 {
		t.Fatalf("BUG-014: read session status: %v", err)
	}
	if status := toString(rows[0]["status"]); status != "idle" {
		t.Fatalf("BUG-014: session status = %q, want idle after auto-commit", status)
	}

	// The waiter-shaped filter: text_block events created after the
	// watermark for this session. With the pre-fix empty replyToUser no
	// such row exists and the assertion below fails (RED on pre-fix code).
	replyRows, err := th.conn.Query(th.ctx,
		`SELECT id, type, content FROM memory_events
		 WHERE session_id = $1 AND id > $2 AND type = 'text_block'
		 ORDER BY id`, sessionID, watermark)
	if err != nil {
		t.Fatalf("BUG-014: query post-watermark replies: %v", err)
	}
	if len(replyRows) == 0 {
		t.Fatalf("BUG-014: no text_block reply persisted after max-turns auto-commit (watermark %d): a synchronous waiter would time out", watermark)
	}
	content := strings.TrimSpace(toString(replyRows[len(replyRows)-1]["content"]))
	if content == "" {
		t.Fatalf("BUG-014: persisted reply content is empty: %+v", replyRows[len(replyRows)-1])
	}
	eligible := strings.Contains(content, "max turns") ||
		strings.Contains(content, out.InternalMonologue)
	if !eligible {
		t.Fatalf("BUG-014: reply = %q, want the last turn's monologue or the deterministic max-turns summary", content)
	}
	t.Logf("BUG-014: persisted reply id > %d: %q", watermark, content)
}
