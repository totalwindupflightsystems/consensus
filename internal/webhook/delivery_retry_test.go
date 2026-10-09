package webhook

// REVIEW-CONSENSUS-5: routed-payload delivery used to be a single
// best-effort INSERT — a handler/DB failure was logged and the payload was
// dropped, silently losing the delivery. The delivery now retries with
// bounded exponential backoff (up to deliveryMaxAttempts, 200ms doubling,
// capped) and persists the terminal failure on the external_events row
// (status → 'failed', last_error → final attempt error).
//
// These tests stub the deliverPayload seam (package-level function var), so
// no test sleeps through the real 200/400ms backoff — the suite stays fast
// and deterministic.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubDelivery replaces the deliverPayload seam for the duration of one test
// and restores it via cleanup.
func stubDelivery(t *testing.T, stub func(ctx context.Context, s *Store, content, sessionID string, iteration int64) error) {
	t.Helper()
	prev := deliverPayload
	deliverPayload = stub
	t.Cleanup(func() { deliverPayload = prev })
}

// stubSleep replaces the deliverSleep seam with a recorder that appends each
// requested backoff delay to delays and restores it via cleanup.
func stubSleep(t *testing.T, delays *[]time.Duration) {
	t.Helper()
	prev := deliverSleep
	deliverSleep = func(d time.Duration) { *delays = append(*delays, d) }
	t.Cleanup(func() { deliverSleep = prev })
}

// TestRoutedPayloadDeliveryRetry is the table-driven behavior suite for
// deliverRoutedPayload + the routing loop's terminal-failure persistence.
func TestRoutedPayloadDeliveryRetry(t *testing.T) {
	ctx := context.Background()
	deliveryErr := errors.New("boom: memory_events insert failed")

	tests := []struct {
		name string

		// failFirst is the number of INITIAL delivery attempts the stub
		// fails before succeeding (deliveryMaxAttempts = never succeed).
		failFirst int

		// wantAttempts is the exact number of delivery attempts.
		wantAttempts int

		// wantDelivered is true when the payload must land in memory_events.
		wantDelivered bool

		// wantEventStatus / wantLastError: the persisted outcome on the
		// external_events row.
		wantEventStatus string
		wantLastError   string

		// wantDelays is the exact sequence of backoff sleeps the loop must
		// request between attempts (nil = none). Capturing the requested
		// delays — not the wall clock — is what proves both the schedule
		// (200ms→400ms) and that the success path never sleeps.
		wantDelays []time.Duration
	}{
		{
			name:            "success on first attempt: 1 attempt, delivered, no retry, no sleep",
			failFirst:       0,
			wantAttempts:    1,
			wantDelivered:   true,
			wantEventStatus: "routed",
			wantLastError:   "",
			wantDelays:      nil,
		},
		{
			name:            "first attempt fails then success: exactly 2 attempts, delivered",
			failFirst:       1,
			wantAttempts:    2,
			wantDelivered:   true,
			wantEventStatus: "routed", // stays routed; failure was transient
			wantLastError:   "",
			wantDelays:      []time.Duration{200 * time.Millisecond},
		},
		{
			name:            "all attempts fail: exactly 3 attempts, failure persisted on event",
			failFirst:       deliveryMaxAttempts,
			wantAttempts:    3,
			wantDelivered:   false,
			wantEventStatus: EventStatusFailed,
			wantLastError:   deliveryErr.Error(),
			wantDelays:      []time.Duration{200 * time.Millisecond, 400 * time.Millisecond},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, cleanup := setupTestDB(t)
			defer cleanup()

			store := New(database)

			// minimal fixture: session + routing rule + one pending event
			mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY, agent_name TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'idle',
				iteration BIGINT NOT NULL DEFAULT 0,
				heartbeat_at TEXT NOT NULL DEFAULT (datetime('now')))
			`)
			mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS memory_events (
				id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL,
				content TEXT NOT NULL, session_id TEXT NOT NULL,
				iteration_created BIGINT NOT NULL,
				created_at TEXT NOT NULL DEFAULT (datetime('now')))
			`)
			// migration 026 adds last_error to external_events; setupTestDB's
			// fixture mirrors migration 007 (pre-026 shape), so add it here.
			// (CREATE TABLE IF NOT EXISTS would be a no-op — the table
			// already exists at this point.)
			mustExec(t, database, ctx, `ALTER TABLE external_events ADD COLUMN last_error TEXT`)
			mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS routing_rules (
				id TEXT PRIMARY KEY, name TEXT NOT NULL, source_pattern TEXT,
				event_type_pattern TEXT, payload_pattern TEXT,
				target_session_id TEXT, target_workflow_id TEXT,
				priority INTEGER NOT NULL DEFAULT 5, enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL)
			`)
			mustExec(t, database, ctx, `INSERT INTO sessions (id, agent_name, status, iteration) VALUES ('retry-target', 'test-agent', 'paused', 7)`)
			mustExec(t, database, ctx, `INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, target_session_id, priority, enabled, created_at)
				VALUES ('r1', 'retry-rule', 'webhook', 'ping', 'retry-target', 1, 1, datetime('now'))`)
			mustExec(t, database, ctx, `INSERT INTO external_events (source, source_id, event_type, payload, signature_valid, status)
				VALUES ('webhook', 'retry-e1', 'ping', '{"event":"ping"}', 1, 'pending')`)

			var (
				mu       sync.Mutex
				attempts int
			)
			// Capture the default implementation BEFORE stubbing so the
			// stub's success path still performs the real INSERT into
			// memory_events (the stub only decides fail vs. succeed).
			realDeliver := deliverPayload
			stubDelivery(t, func(ctx context.Context, s *Store, content, sessionID string, iteration int64) error {
				mu.Lock()
				attempts++
				fail := attempts <= tt.failFirst
				mu.Unlock()
				if fail {
					return deliveryErr
				}
				return realDeliver(ctx, s, content, sessionID, iteration)
			})

			// Intercept backoff sleeps: assert the exact requested-delay
			// sequence below instead of waiting out real 200/400ms sleeps.
			var delays []time.Duration
			stubSleep(t, &delays)

			if err := store.routePendingEvents(ctx); err != nil {
				t.Fatalf("routePendingEvents: %v", err)
			}

			if attempts != tt.wantAttempts {
				t.Errorf("delivery attempts = %d, want %d", attempts, tt.wantAttempts)
			}
			if len(delays) != len(tt.wantDelays) {
				t.Errorf("backoff sleeps = %v, want %v", delays, tt.wantDelays)
			} else {
				for i, d := range tt.wantDelays {
					if delays[i] != d {
						t.Errorf("backoff sleep[%d] = %v, want %v", i, delays[i], d)
					}
				}
			}

			// Delivered payloads land as user_message rows carrying the
			// event payload.
			rows, err := database.Query(ctx, `SELECT content FROM memory_events WHERE session_id = 'retry-target'`)
			if err != nil {
				t.Fatalf("query memory_events: %v", err)
			}
			if tt.wantDelivered {
				if len(rows) != 1 {
					t.Fatalf("memory_events rows = %d, want 1 (payload delivered)", len(rows))
				}
				if !strings.Contains(toString(rows[0]["content"]), `"event":"ping"`) {
					t.Errorf("delivered content %q does not carry the event payload", rows[0]["content"])
				}
			} else if len(rows) != 0 {
				t.Errorf("memory_events rows = %d, want 0 (payload must be dropped after terminal failure)", len(rows))
			}

			// Persisted outcome on the event row.
			eRows, err := database.Query(ctx, `SELECT status, last_error, session_id, processed_at FROM external_events WHERE source_id = 'retry-e1'`)
			if err != nil {
				t.Fatalf("query external_events: %v", err)
			}
			if len(eRows) != 1 {
				t.Fatalf("external_events rows = %d, want 1", len(eRows))
			}
			row := eRows[0]
			if got := row["status"]; got != tt.wantEventStatus {
				t.Errorf("event status = %q, want %q", got, tt.wantEventStatus)
			}
			if got := toString(row["last_error"]); got != tt.wantLastError {
				t.Errorf("last_error = %q, want %q", got, tt.wantLastError)
			}
			if tt.wantEventStatus == EventStatusFailed {
				// Failure observable: routing target preserved, failure time
				// stamped, final error recorded.
				if row["session_id"] != "retry-target" {
					t.Errorf("failed event lost its routing target session_id: %v", row["session_id"])
				}
				if toString(row["processed_at"]) == "" {
					t.Errorf("failed event has no processed_at stamp; want the failure time")
				}
			} else if toString(row["last_error"]) != "" {
				t.Errorf("transient path persisted last_error %q, want empty", row["last_error"])
			}
		})
	}
}

// TestDeliverRoutedPayloadShape exercises the delivery function directly:
// error propagation, attempt exhaustion, and ctx cancellation — without the
// routing loop around it.
func TestDeliverRoutedPayloadShape(t *testing.T) {
	ctx := context.Background()

	t.Run("error propagates verbatim after exhausting attempts", func(t *testing.T) {
		database, cleanup := setupTestDB(t)
		defer cleanup()
		store := New(database)

		sentinel := errors.New("persistent failure")
		calls := 0
		stubDelivery(t, func(ctx context.Context, s *Store, content, sessionID string, iteration int64) error {
			calls++
			return sentinel
		})
		var delays []time.Duration
		stubSleep(t, &delays)

		err := store.deliverRoutedPayload(ctx, "c", "sess", 1)
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want the sentinel error", err)
		}
		if calls != deliveryMaxAttempts {
			t.Errorf("attempts = %d, want %d", calls, deliveryMaxAttempts)
		}
	})

	t.Run("already-cancelled ctx abandons remaining attempts after one try", func(t *testing.T) {
		database, cleanup := setupTestDB(t)
		defer cleanup()
		store := New(database)

		cctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		stubDelivery(t, func(ctx context.Context, s *Store, content, sessionID string, iteration int64) error {
			calls++
			return errors.New("always fails")
		})
		var delays []time.Duration
		stubSleep(t, &delays)

		_ = store.deliverRoutedPayload(cctx, "c", "sess", 1)
		if calls != 1 {
			t.Errorf("attempts = %d, want exactly 1 (first attempt in flight, then abandon on cancelled ctx)", calls)
		}
	})

	t.Run("backoff schedule doubles and is capped", func(t *testing.T) {
		// Direct check of the shift arithmetic used between attempts.
		if d := deliveryInitialBackoff << 0; d != 200*time.Millisecond {
			t.Errorf("first backoff = %v, want 200ms", d)
		}
		if d := deliveryInitialBackoff << 1; d != 400*time.Millisecond {
			t.Errorf("second backoff = %v, want 400ms", d)
		}
		if capped := deliveryInitialBackoff << 10; capped < deliveryMaxBackoff {
			t.Errorf("cap check broken: shift overflow %v < cap %v", capped, deliveryMaxBackoff)
		}
	})
}
