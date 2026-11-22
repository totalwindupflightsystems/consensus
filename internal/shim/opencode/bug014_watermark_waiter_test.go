package opencode

// BUG-014: the synchronous SendMessage waiter is keyed on the memory_events
// id watermark captured before the user_message row is written, not on the
// session's iteration counter. These tests pin both edges:
//
//   - a response written after the harness bumped the iteration between
//     target capture and the response write MUST still resolve (the
//     dexdat-core live case: user_message @ iteration 1, text_block @
//     iteration 3, old waiter targeted 2 -> every sync POST 504'd);
//   - a late response from an EARLIER turn MUST NOT satisfy a newer request
//     (the original correctness property the iteration-equality waiter
//     existed to guarantee).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestBUG014_IterationDriftBetweenCaptureAndResponseResolves is the drift
// proof: a client sends while the harness is still booting, the harness
// bumps the session iteration past the client's captured target, and the
// LLM answer lands under a later iteration number. The watermark-keyed
// waiter must still return it.
func TestBUG014_IterationDriftBetweenCaptureAndResponseResolves(t *testing.T) {
	s, srv, conn := newMessageResponseTestServer(t)
	s.messageResponseTimeout = 2 * time.Second

	writeResult := make(chan error, 1)
	go func() {
		ctx := context.Background()
		deadline := time.NewTimer(testPublishDeadline())
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			rows, err := conn.Query(ctx,
				`SELECT id FROM memory_events WHERE session_id = $1 AND type = 'user_message'`, "s1")
			if err != nil {
				writeResult <- err
				return
			}
			if len(rows) > 0 {
				// Simulate the boot drift: the initial-context write bumps
				// the iteration AFTER the target was captured, and a
				// non-final context block lands under that iteration.
				if err := conn.Exec(ctx,
					`UPDATE sessions SET status = 'thinking', iteration = 2 WHERE id = 's1'`); err != nil {
					writeResult <- err
					return
				}
				if err := conn.Exec(ctx,
					`INSERT INTO memory_events (type, content, session_id, iteration_created)
					 VALUES ('text_block', 'boot context snapshot', 's1', 2)`); err != nil {
					writeResult <- err
					return
				}
				// The LLM's real answer arrives under iteration 3 — two
				// away from the target the old waiter captured (1).
				if err := conn.Exec(ctx,
					`INSERT INTO memory_events (type, content, session_id, iteration_created)
					 VALUES ('text_block', 'actual agent answer', 's1', 3)`); err != nil {
					writeResult <- err
					return
				}
				writeResult <- conn.Exec(ctx,
					`UPDATE sessions SET status = 'idle', iteration = 4 WHERE id = 's1'`)
				return
			}
			select {
			case <-deadline.C:
				writeResult <- context.DeadlineExceeded
				return
			case <-ticker.C:
			}
		}
	}()

	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": "answer while booting"}},
	})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/s1/message", body)
	if err != nil {
		t.Fatalf("build POST /session/s1/message: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session/s1/message: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := <-writeResult; err != nil {
		t.Fatalf("publish drifted response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 despite iteration drift, got %d: %s", resp.StatusCode, responseBody)
	}

	var msg struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(msg.Parts) != 1 || msg.Parts[0].Type != "text" || msg.Parts[0].Text != "actual agent answer" {
		t.Fatalf("response parts = %#v, want the drifted-turn answer", msg.Parts)
	}
	// The intermediate boot-context block must NOT be the answer (it was
	// written while the session was still working, before the idle settle).
	for _, p := range msg.Parts {
		if strings.Contains(p.Text, "boot context") {
			t.Fatalf("waiter returned an intermediate turn block: %#v", msg.Parts)
		}
	}
}

// TestBUG014_StaleLateResponseFromEarlierTurnDoesNotAnswer guards the
// correctness property: a response that belongs to an EARLIER turn (it was
// created before this request was sent, i.e. its id is not newer than the
// watermark) must never satisfy the newer synchronous request. The waiter
// must time out instead of returning the stale answer.
func TestBUG014_StaleLateResponseFromEarlierTurnDoesNotAnswer(t *testing.T) {
	s, _, conn := newMessageResponseTestServer(t)
	s.messageResponseTimeout = 100 * time.Millisecond

	// A late answer from the previous turn, present in the store BEFORE the
	// request is sent (so its id sits under the watermark captured at send).
	if err := conn.Exec(context.Background(),
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('agent_response', 'late reply from the earlier turn', 's1', 0)`); err != nil {
		t.Fatalf("seed stale earlier-turn response: %v", err)
	}

	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": "new question nobody will answer"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/session/s1/message", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	recorder := httptest.NewRecorder()

	started := time.Now()
	s.Handler().ServeHTTP(recorder, req)
	// Load-aware bound (same class as TestSendMessageNoResponseReturnsConcreteError).
	bound := 250 * s.messageResponseTimeout
	if elapsed := time.Since(started); elapsed > bound {
		t.Fatalf("stale-response path took %s, want bounded timeout (bound %s)", elapsed, bound)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (the earlier-turn response must not answer): %s",
			recorder.Code, recorder.Body.String())
	}
}
