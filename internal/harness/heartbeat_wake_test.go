// Package harness: heartbeat wake-channel and bounded in-flight tests.
//
// PERF-CONSENSUS-11: POST /message flips a session to 'thinking' but dispatch
// previously waited up to a full heartbeat tick, and a parked planning
// goroutine (or a delayed inFlight delete) could starve that session's own
// future dispatches forever. These tests pin the fire-on-message wake path,
// the non-blocking wake send, and the bounded in-flight claim lifetime.
//
// Evidence (2026-09-25 dogfood): user-visible turns of 94.7s/108.2s;
// 'harness: found active session' gaps of 6m25s/~4m for a burst-loaded
// session while a fresh session was picked up in 6.3s — poll-only dispatch
// plus an unguarded inFlight map entry.
package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/api"
)

// wakeProbeLLM counts LLM calls safely across goroutines and returns a
// minimal "respond" output so planning completes without touching tasks.
type wakeProbeLLM struct {
	mu    sync.Mutex
	calls int
}

func (w *wakeProbeLLM) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	w.mu.Lock()
	w.calls++
	w.mu.Unlock()
	return &LLMResponse{
		Output: &AgentOutput{
			InternalMonologue:  "wake probe",
			MemoryStateChanges: []string{},
			SystemActions:      []string{"respond"},
			MessageToUser:      "wake-ok",
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		ModelID: "wake-probe-model",
	}, nil
}

func (w *wakeProbeLLM) callCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

// wakeSeedAdminKey inserts an admin API key usable with the httptest server.
func wakeSeedAdminKey(t *testing.T, th *testHarness, adminKey string) {
	t.Helper()
	hash := sha256.Sum256([]byte(adminKey))
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('wake-admin', $1, $2, 'admin', CURRENT_TIMESTAMP)`,
		hex.EncodeToString(hash[:]), adminKey[:8]); err != nil {
		t.Fatalf("insert API key: %v", err)
	}
}

func wakePostMessage(t *testing.T, server *api.Server, sessionID, adminKey, content string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"role":"user","content":` + mustJSONQuote(t, content) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	return w
}

// TestHeartbeatWakeDispatchesImmediately proves the acceptance criterion:
// a message POST to an idle session triggers planning dispatch promptly,
// without waiting for the heartbeat tick (interval is set far beyond the
// test horizon, so the ticker cannot be the dispatcher).
func TestHeartbeatWakeDispatchesImmediately(t *testing.T) {
	llm := &wakeProbeLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness
	h.HeartbeatConfig.Interval = 2 * time.Minute // tick cannot fire inside this test

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	adminKey := "cs_sk_wake_test_admin_key"
	wakeSeedAdminKey(t, th, adminKey)

	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0", Wake: h.RequestWake})
	go h.StartHeartbeatLoop(th.ctx)

	postStart := time.Now()
	w := wakePostMessage(t, server, sessionID, adminKey, "wake me up")
	if w.Code != http.StatusOK {
		t.Fatalf("POST message: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if llm.callCount() >= 1 {
			elapsed := time.Since(postStart)
			if elapsed >= h.HeartbeatConfig.Interval {
				t.Fatalf("dispatch took %v — that looks like a tick, not a wake", elapsed)
			}
			return // dispatch happened promptly through the wake path
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Diagnose: did the session even reach 'thinking'?
	rows, qerr := th.conn.Query(th.ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	status := "query-error: " + fmt.Sprint(qerr)
	if qerr == nil && len(rows) > 0 {
		status = toString(rows[0]["status"])
	}
	t.Fatalf("planning was not dispatched within 10s (heartbeat interval %v); llm calls=%d, session status=%q",
		h.HeartbeatConfig.Interval, llm.callCount(), status)
}

// TestRequestWakeNeverBlocksWhenChannelFull pins the non-blocking contract:
// a caller (the HTTP handler) must never stall on a full wake channel — the
// next tick's DB scan catches dropped sessions anyway.
func TestRequestWakeNeverBlocksWhenChannelFull(t *testing.T) {
	h := New(nil, &mockLLMClient{})
	for i := 0; i < wakeChannelCapacity; i++ {
		h.wakeCh <- fmt.Sprintf("filler-%d", i)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.RequestWake("overflow-session")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RequestWake blocked on a full wake channel — the HTTP handler would stall")
	}

	got := drainWake(h.wakeCh, maxWakeBatch)
	if len(got) > maxWakeBatch {
		t.Errorf("drainWake returned %d ids, max is %d", len(got), maxWakeBatch)
	}
	seen := make(map[string]bool, len(got))
	for _, sid := range got {
		if sid == "" {
			t.Error("drainWake returned an empty session id")
		}
		if seen[sid] {
			t.Errorf("drainWake returned duplicate session id %q", sid)
		}
		seen[sid] = true
	}
}

// TestDrainWakeCoalescesDuplicates checks that repeated wake signals for the
// same session collapse into one dispatch candidate per pass; concurrent
// duplicates are deduplicated by the inFlight guard inside pollAndDispatch.
func TestDrainWakeCoalescesDuplicates(t *testing.T) {
	h := New(nil, &mockLLMClient{})
	for i := 0; i < 5; i++ {
		h.RequestWake("same-session")
	}
	h.RequestWake("other-session")

	got := drainWake(h.wakeCh, maxWakeBatch)
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct wake ids after coalescing, got %d: %v", len(got), got)
	}
	found := map[string]bool{}
	for _, sid := range got {
		found[sid] = true
	}
	if !found["same-session"] || !found["other-session"] {
		t.Errorf("expected both session ids in wake batch, got %v", got)
	}
}

// TestDrainWakeNilChannelSafe: a Harness built without a wake channel (zero
// value or future constructors) must not panic or block.
func TestDrainWakeNilChannelSafe(t *testing.T) {
	h := &Harness{}
	if got := drainWake(h.wakeCh, maxWakeBatch); len(got) != 0 {
		t.Errorf("drainWake on nil channel returned %v", got)
	}
	h.RequestWake("never-blocks") // select/default handles the nil channel
}

// TestInFlightClaimExpiryAndReclaim pins the bounded-lifetime table: fresh
// claims block redispatch, expired claims are reclaimed (with a fresh
// timestamp), missing entries claim normally.
func TestInFlightClaimExpiryAndReclaim(t *testing.T) {
	cases := []struct {
		name        string
		seedExists  bool
		seedAge     time.Duration
		wantClaimed bool
	}{
		{"fresh claim blocks redispatch", true, 0, false},
		{"expired claim allows reclaim", true, inFlightTTL + time.Minute, true},
		{"missing entry allows claim", false, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(nil, &mockLLMClient{})
			if tc.seedExists {
				h.inFlight["sess"] = time.Now().Add(-tc.seedAge)
			}
			got := h.claimSession("sess")
			if got != tc.wantClaimed {
				t.Fatalf("claimSession = %v, want %v", got, tc.wantClaimed)
			}
			if tc.wantClaimed {
				h.inFlightMu.Lock()
				claimAt, ok := h.inFlight["sess"]
				h.inFlightMu.Unlock()
				if !ok {
					t.Fatal("claimed session missing from inFlight map")
				}
				if age := time.Since(claimAt); age > time.Minute {
					t.Errorf("reclaim stored a stale claim time (age %v) — must be refreshed", age)
				}
			}
		})
	}
}

// TestEvictExpiredInFlightRemovesStaleEntries proves the housekeeping sweep:
// stale entries can never permanently block the "only claim tasks when no
// planning sessions are in-flight" gate.
func TestEvictExpiredInFlightRemovesStaleEntries(t *testing.T) {
	h := New(nil, &mockLLMClient{})
	h.inFlight["stale"] = time.Now().Add(-inFlightTTL - time.Second)
	h.inFlight["fresh"] = time.Now()

	h.evictExpiredInFlight()

	h.inFlightMu.Lock()
	_, staleOK := h.inFlight["stale"]
	_, freshOK := h.inFlight["fresh"]
	h.inFlightMu.Unlock()
	if staleOK {
		t.Error("expired inFlight entry was not evicted")
	}
	if !freshOK {
		t.Error("fresh inFlight entry was wrongly evicted")
	}
}

// TestStaleInFlightEntryAllowsRedispatch proves the end-to-end acceptance:
// a stuck claim (held longer than the TTL) can never permanently block
// redispatch of the session — the expired entry is reclaimed and planning
// runs, even though the ticker is outside the test horizon.
func TestStaleInFlightEntryAllowsRedispatch(t *testing.T) {
	llm := &wakeProbeLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create thinking session: %v", err)
	}
	// The atomic claim needs status='thinking' (see RunInteractivePlanning).
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}

	// Simulate a goroutine that parked long ago and never released.
	h.inFlight[sessionID] = time.Now().Add(-inFlightTTL - time.Minute)

	h.pollAndDispatch(th.ctx) // synchronous scan; dispatch goroutine spawned

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if llm.callCount() >= 1 {
			return // stale entry was reclaimed and planning dispatched
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stale inFlight entry blocked redispatch: llm calls=%d after %v",
		llm.callCount(), 10*time.Second)
}

// TestFreshInFlightClaimSkipsRedispatch pins the duplicate-dispatch guard:
// a fresh claim is skipped (guard intact), and the claim timestamp is not
// clobbered by a skipped pass.
func TestFreshInFlightClaimSkipsRedispatch(t *testing.T) {
	llm := &wakeProbeLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()
	h := th.Harness

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create thinking session: %v", err)
	}
	// The atomic claim needs status='thinking' (see RunInteractivePlanning).
	if err := th.conn.Exec(th.ctx,
		`UPDATE sessions SET status = 'thinking' WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("set session thinking: %v", err)
	}
	seeded := time.Now().Add(-time.Second)
	h.inFlight[sessionID] = seeded

	h.pollAndDispatch(th.ctx)

	if llm.callCount() != 0 {
		t.Fatalf("fresh inFlight claim was redispatched (llm calls=%d)", llm.callCount())
	}
	h.inFlightMu.Lock()
	claimAt := h.inFlight[sessionID]
	h.inFlightMu.Unlock()
	if !claimAt.Equal(seeded) {
		t.Errorf("skipped pass rewrote the claim time: seeded %v, now %v", seeded, claimAt)
	}
}
