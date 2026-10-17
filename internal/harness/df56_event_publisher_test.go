// Package harness: regression tests for harness-applied status transitions
// publishing session_update events (DF-CONSENSUS-56).
//
// The defect: the harness applies terminal status transitions by SQL only —
// executor.go's NextStatus apply (pollAndDispatch), the consecutive-error
// breaker pause, and the planning-fail pause — need no event publisher.
// Every published session_update came from internal/api; an SSE client on
// GET /api/v1/events saw turn-start 'thinking' only and waited forever for
// the terminal transition.
//
// Fix direction (row DF-CONSENSUS-56): publish session_update when the
// harness applies NextStatus (idle AND paused). The publisher is injected
// as an optional EventPublisher interface (nil = disabled, tests unaffected).
//
// RED proof (pre-fix tree, commit 2a13a0f): the mechanism did not exist at
// all — no EventPublisher, no publish call sites (git grep on 2a13a0f).
// Behavioural RED with symbols retained: RED-PROOF DISABLE in publishStatus
// suppresses ALL publishes while the API stays intact; the two async
// dispatch tests fail with exactly the defect's surface (no publish /
// terminal SSE event never observed). See commit body.
//
// axiom:trace row=DF-CONSENSUS-56 fix=publish-executor-transitions
package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/hitl"
)

// recordingPublisher captures PublishSessionUpdate calls for assertions.
// Same method signature as internal/api.EventBus's convenience method —
// TestEventPublisher_InterfaceSatisfiedWithApiEventBus pins the implicit
// satisfaction contract used at the wiring site (cmd/consensus/main.go).
type recordingPublisher struct {
	mu       sync.Mutex
	session  string
	status   string
	iterated int64
	calls    int
}

func (r *recordingPublisher) PublishSessionUpdate(sessionID, status string, iteration int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.session = sessionID
	r.status = status
	r.iterated = iteration
	r.calls++
}

func (r *recordingPublisher) snapshot() (session, status string, iteration int64, calls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.session, r.status, r.iterated, r.calls
}

// waitFor polls until pred holds, running tick between attempts (proves
// liveness of the poll itself against a stuck predicate).
func waitFor(t *testing.T, timeout time.Duration, tick time.Duration, pred func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(tick)
	}
	return pred()
}

// TestEventPublisher_NilIsNoop: nil publisher (default construction, tests,
// CLI paths) must be a no-op — no panic, no side effects.
func TestEventPublisher_NilIsNoop(t *testing.T) {
	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	if th.EventPublisher != nil {
		t.Fatal("expected nil EventPublisher after harness.New (nil-safe default)")
	}

	sid, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	th.publishStatus(sid, "idle", 1) // must not panic
}

// TestExecutorPublishesTerminalIdle drives the REAL async dispatch path
// (pollAndDispatch goroutine): a successful planning turn applies
// NextStatus='idle' by SQL and must publish session_update idle on the
// injected publisher — the defect's main seam (executor.go ~:822).
func TestExecutorPublishesTerminalIdle(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	rec := &recordingPublisher{}
	th.EventPublisher = rec

	sid, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sid); err != nil {
		t.Fatalf("set thinking: %v", err)
	}

	// pollAndDispatch finds the thinking session, dispatches
	// RunInteractivePlanning in a goroutine, applies NextStatus and
	// publishes. The outer sweep is synchronous; the goroutine applies
	// asynchronously — poll the publisher and the row with a deadline.
	th.pollAndDispatch(th.ctx)

	ok := waitFor(t, 15*time.Second, 100*time.Millisecond, func() bool {
		_, _, _, calls := rec.snapshot()
		if calls == 0 {
			return false
		}
		rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sid)
		return len(rows) > 0 && toString(rows[0]["status"]) == "idle"
	})
	if !ok {
		t.Fatalf("dispatch never published/applied terminal idle (publisher=%+v)", rec)
	}

	session, status, iteration, calls := rec.snapshot()
	if calls != 1 {
		t.Fatalf("expected exactly 1 publish, got %d", calls)
	}
	if session != sid || status != "idle" {
		t.Errorf("expected publish (%s, idle), got (%s, %s)", sid, session, status)
	}
	if iteration < 0 {
		t.Errorf("expected non-negative iteration, got %d", iteration)
	}
}

// TestExecutorPublishesPausedOnBreakerTrip drives the SAME async dispatch
// path through the planning-fail breaker trip (executor.go ~:815): below
// threshold publishes NOTHING (retryable 'thinking' reset is api-owned),
// the trip publishes exactly one paused event.
func TestExecutorPublishesPausedOnBreakerTrip(t *testing.T) {
	th, err := newTestHarness(failingMockLLM(fmt.Errorf("llm: http 401: invalid api key")))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	rec := &recordingPublisher{}
	th.EventPublisher = rec
	th.MaxConsecutiveErrors = 2

	sid, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sid); err != nil {
		t.Fatalf("set thinking: %v", err)
	}

	// Failure 1 stays below the threshold and must NOT publish; failure 2
	// trips and must publish exactly one paused event. The dispatch
	// goroutine needs the inFlight claim released between runs, so re-sweep
	// until trip or deadline.
	deadline := time.Now().Add(20 * time.Second)
	for {
		th.pollAndDispatch(th.ctx)
		_, status, _, _ := rec.snapshot()
		if status == "paused" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("breaker trip never published paused (publisher=%+v)", rec)
		}
		rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sid)
		got := "missing"
		if len(rows) > 0 {
			got = toString(rows[0]["status"])
		}
		if got == "paused" {
			t.Fatalf("session paused but publisher recorded %q", status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	rows, _ := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sid)
	if len(rows) == 0 || toString(rows[0]["status"]) != "paused" {
		status := "missing"
		if len(rows) > 0 {
			status = toString(rows[0]["status"])
		}
		t.Fatalf("expected session row status=paused after breaker trip, got %q", status)
	}
	session, status, _, calls := rec.snapshot()
	if calls != 1 {
		t.Errorf("expected exactly 1 publish (trip only; below-threshold retries publish nothing), got %d", calls)
	}
	if session != sid || status != "paused" {
		t.Errorf("expected publish (%s, paused), got (%s, %s)", sid, session, status)
	}
}

// TestEventPublisher_InterfaceSatisfiedWithApiEventBus pins the implicit
// satisfaction contract used at the wiring site (cmd/consensus/main.go:
// h.EventPublisher = apiSrv.EventBus()).
func TestEventPublisher_InterfaceSatisfiedWithApiEventBus(t *testing.T) {
	var ep EventPublisher = api.NewEventBus()              // compile-time: bus satisfies the harness interface
	ep.PublishSessionUpdate("sa4023-probe-sid", "idle", 0) // consume: signature match at the seam
}

// TestEventPublisher_RecordingMatchesSignature keeps the fake pinned to the
// interface shape so a signature drift fails both, not only production.
func TestEventPublisher_RecordingMatchesSignature(t *testing.T) {
	rec := &recordingPublisher{}
	var ep EventPublisher = rec
	ep.PublishSessionUpdate("sa4023-probe-sid", "idle", 0)
	if _, _, _, calls := rec.snapshot(); calls != 1 {
		t.Fatalf("expected 1 call through the interface, got %d", calls)
	}
}

// TestSSEReaderObservesTerminalIdleEvent is the row's acceptance criterion:
// an SSE reader on GET /api/v1/events observes a TERMINAL (idle) event for a
// full turn — streams used to end at turn-start 'thinking' only.
//
// Wiring proven end to end: harness pollAndDispatch publish → api.EventBus →
// HandleSSE wire — the same publisher instance cmd/consensus/main.go wires
// in production (apiServer.EventBus()).
func TestSSEReaderObservesTerminalIdleEvent(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	sid, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed admin API key for the authenticated SSE stream (sse_auth_test.go
	// arm pattern, inlined to keep this file self-contained).
	adminKey := "cs_sk_admin_df56_sse_terminal_key"
	_ = th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-df56', $1, 'cs_sk_ad', 'admin', datetime('now'))`,
		sha256Hash(adminKey))

	// api.NewServer on the harness's DB (end_to_end_test.go pattern) and wire
	// the harness's publisher to its EventBus — the production wiring. The
	// Wake hook mirrors main.go's PERF-CONSENSUS-11 wiring: without it a
	// message POST would wait for the next heartbeat tick before dispatch.
	apiServer := api.NewServer(api.ServerConfig{DB: th.conn, HITL: hitl.New(th.conn), Wake: th.RequestWake})
	th.EventPublisher = apiServer.EventBus()

	ts := httptest.NewServer(apiServer.Handler())
	defer ts.Close()

	url := ts.URL + "/api/v1/events?session_id=" + sid
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("SSE connect: expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Drive a FULL TURN through the real dispatch path: thinking → planning
	// → idle apply + publish, exactly as pollAndDispatch does in production.
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'thinking' WHERE id = $1`, sid); err != nil {
		t.Fatalf("set thinking: %v", err)
	}
	th.pollAndDispatch(th.ctx)

	// Read frames until the terminal event. The connected frame must arrive
	// first; the terminal session_update is the acceptance assertion.
	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(15 * time.Second)
	haveConnected, haveTerminal := false, false
	terminalData := ""
	for !haveConnected || !haveTerminal {
		if time.Now().After(deadline) {
			break
		}
		var eventType, data string
		// Read one frame: lines until blank.
		sawFrameContent := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("reading SSE stream: %v (event=%q data=%q)", err, eventType, data)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if sawFrameContent {
					break
				}
				continue
			}
			sawFrameContent = true
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		switch eventType {
		case "connected":
			haveConnected = true
		case "session_update":
			// HandleSSE marshals the FULL Event envelope as the frame's
			// data line (events.go: json.Marshal(event)), not the bare
			// SessionUpdateEvent payload — decode the nested shape.
			var envelope struct {
				Data struct {
					Status    string `json:"status"`
					Iteration int64  `json:"iteration"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(data), &envelope); err == nil && envelope.Data.Status == "idle" {
				haveTerminal = true
				terminalData = data
			}
		}
	}

	if !haveConnected {
		t.Fatal("SSE stream never sent the connected frame")
	}
	if !haveTerminal {
		t.Fatalf("SSE reader never observed the terminal session_update idle event for the turn (last seen data: %q)", terminalData)
	}
	if !strings.Contains(terminalData, `"iteration"`) {
		t.Errorf("terminal frame missing iteration field: %s", terminalData)
	}
	t.Logf("SSE reader observed terminal idle frame: %s", terminalData)
}
