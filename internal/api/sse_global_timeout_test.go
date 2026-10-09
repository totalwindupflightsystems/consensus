package api

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSSE_GlobalStream_BoundedTerminates pins the DF-CONSENSUS-49 fix: the
// global stream (no session_id) must END within a bounded window instead of
// hanging forever, and must end cleanly with a terminal "stream_timeout"
// event after the connected frame (so EventSource auto-reconnects).
func TestSSE_GlobalStream_BoundedTerminates(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	old := globalStreamMaxDuration
	globalStreamMaxDuration = 150 * time.Millisecond
	defer func() { globalStreamMaxDuration = old }()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := sseConnect(t, ts, srv.adminKey, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("global SSE connect: expected 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	evtType, _ := readSSEEvent(t, reader)
	if evtType != "connected" {
		t.Errorf("first frame: expected connected, got %q", evtType)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		evtType, _ = readSSEEvent(t, reader)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("global SSE stream did not terminate within the bounded window")
	}

	if evtType != "stream_timeout" {
		t.Errorf("terminal frame: expected stream_timeout, got %q", evtType)
	}

	// After the terminal event the server must close the stream (EOF).
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Errorf("expected stream EOF after stream_timeout, got %v", err)
	}
}

// TestSSE_GlobalStream_EventsBeforeTimeoutStillDelivered pins that the bound
// does not change streaming behavior: events published before the timeout
// still reach the global subscriber.
func TestSSE_GlobalStream_EventsBeforeTimeoutStillDelivered(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	old := globalStreamMaxDuration
	globalStreamMaxDuration = 300 * time.Millisecond
	defer func() { globalStreamMaxDuration = old }()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := sseConnect(t, ts, srv.adminKey, "")
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	if evtType, _ := readSSEEvent(t, reader); evtType != "connected" {
		t.Fatalf("first frame: expected connected, got %q", evtType)
	}

	srv.EventBus().PublishSessionUpdate("some-session", "thinking", 1)
	evtType, _ := readSSEEvent(t, reader)
	if evtType != "session_update" {
		t.Errorf("expected session_update before timeout, got %q", evtType)
	}
}
