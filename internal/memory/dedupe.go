// Package memory: duplicate user_message suppression (DF-CONSENSUS-9).
//
// Shared probe used by every API-surface 'user_message' INSERT (the native
// send path, the idempotent send path, and Service.SendMessage) so a retry or
// wake race cannot land the same conversational turn twice.
//
// axiom:trace work_item=DF-CONSENSUS-9 spec=docs/API.md plan=DF-CONSENSUS-9/task-4 impl=internal/memory/dedupe.go
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// UserMessageDuplicateWindow bounds the in-flight dedupe window: a second
// 'user_message' row with byte-identical content for the same session within
// this duration of the first is dropped instead of stored.
//
// 5s covers the observed race (a client send racing the heartbeat-driven
// auto-resume re-send) with margin: it is far shorter than any legitimate
// human or agent re-sending the same instruction on purpose, and short enough
// that a genuine repeat a few seconds later is still recorded.
const UserMessageDuplicateWindow = 5 * time.Second

// querier is the read surface the dedupe probe needs. Both db.DB (the pool)
// and db.Tx (an open transaction) satisfy it, which is what lets the probe run
// on the caller's transaction instead of borrowing a second pooled connection.
//
// INT-CI-11: the probe MUST run on the transaction when the caller already has
// one open. A pooled Query while a transaction is open needs a second
// connection and, on a bounded pool, starves itself: with MaxOpenConns=4, four
// concurrent idempotent retries each held one connection for their transaction
// and then all four waited for a fifth that no goroutine could release — a
// silent, permanent deadlock (CI hung for the full 10m job timeout).
type querier interface {
	Query(ctx context.Context, query string, args ...any) ([]db.Row, error)
}

// IsDuplicateUserMessage reports whether an identical 'user_message' row
// already exists for sessionID with a created_at timestamp inside the
// UserMessageDuplicateWindow ending at now.
//
// It is a probe only: callers decide whether to skip the INSERT. The
// append-only ledger and every existing schema are untouched — suppression is
// a decision at the write seam, not a constraint on the table.
//
// Use this form only when NO transaction is open on database; otherwise use
// IsDuplicateUserMessageTx so the probe does not need a second connection.
//
// Timestamps are stored as UTC RFC3339 text by every writer (the crier
// intake, the webhook delivery, and the API paths), so the comparison parses
// created_at with time.RFC3339. Rows whose timestamp does not parse are
// skipped (treated as not duplicates) rather than guessed: an unparseable
// timestamp is evidence of an exotic writer, and dropping a real message over
// it would trade a cosmetic duplicate for silent loss.
func IsDuplicateUserMessage(ctx context.Context, database db.DB, sessionID, content string, now time.Time) (bool, error) {
	return isDuplicateUserMessage(ctx, database, sessionID, content, now)
}

// IsDuplicateUserMessageTx is IsDuplicateUserMessage evaluated on an already
// open transaction. It issues the identical query and applies the identical
// window/parse rules; only the connection it borrows differs (the
// transaction's own).
func IsDuplicateUserMessageTx(ctx context.Context, tx db.Tx, sessionID, content string, now time.Time) (bool, error) {
	return isDuplicateUserMessage(ctx, tx, sessionID, content, now)
}

// isDuplicateUserMessage is the single implementation both exported probes
// delegate to, so the pool path and the transaction path cannot drift.
func isDuplicateUserMessage(ctx context.Context, q querier, sessionID, content string, now time.Time) (bool, error) {
	rows, err := q.Query(ctx,
		`SELECT created_at FROM memory_events
		 WHERE type = 'user_message' AND session_id = $1 AND content = $2
		 ORDER BY id DESC LIMIT 20`, sessionID, content)
	if err != nil {
		return false, fmt.Errorf("dedupe probe: %w", err)
	}
	windowStart := now.Add(-UserMessageDuplicateWindow)
	for _, row := range rows {
		stored, ok := row["created_at"].(string)
		if !ok {
			continue
		}
		ts, err := time.Parse(time.RFC3339, stored)
		if err != nil {
			continue
		}
		if !ts.Before(windowStart) && !ts.After(now) {
			return true, nil
		}
	}
	return false, nil
}

// SkipDuplicateUserMessage probes for an in-window duplicate of (sessionID,
// content) and reports whether the caller must skip its INSERT. A probe
// failure never blocks delivery: it is logged and reported as not-duplicate,
// so a monitoring hiccup degrades to the old (duplicate-tolerant) behavior
// instead of losing the message. duplicated=true is the only skip signal.
//
// Use this form only when NO transaction is open on database; otherwise use
// SkipDuplicateUserMessageTx.
func SkipDuplicateUserMessage(ctx context.Context, database db.DB, sessionID, content string) bool {
	if database == nil {
		return false
	}
	return skipDuplicateUserMessage(ctx, database, sessionID, content)
}

// SkipDuplicateUserMessageTx is SkipDuplicateUserMessage evaluated on an open
// transaction: same query, same window, same failure policy, but it never
// acquires a pooled connection. Callers inside a transaction MUST use this
// variant (INT-CI-11).
func SkipDuplicateUserMessageTx(ctx context.Context, tx db.Tx, sessionID, content string) bool {
	if tx == nil {
		return false
	}
	return skipDuplicateUserMessage(ctx, tx, sessionID, content)
}

// skipDuplicateUserMessage is the shared skip-or-deliver decision both
// exported forms delegate to.
func skipDuplicateUserMessage(ctx context.Context, q querier, sessionID, content string) bool {
	if q == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	duplicate, err := isDuplicateUserMessage(ctx, q, sessionID, content, time.Now().UTC())
	if err != nil {
		slog.Warn("memory: user_message dedupe probe failed; delivering anyway",
			"session_id", sessionID, "error", err)
		return false
	}
	if duplicate {
		slog.Info("memory: dropped duplicate user_message within dedupe window",
			"session_id", sessionID, "window", UserMessageDuplicateWindow.String())
	}
	return duplicate
}
