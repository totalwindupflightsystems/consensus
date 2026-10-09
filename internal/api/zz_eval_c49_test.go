package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Eval repro for DF-CONSENSUS-49 criterion 2: POST /api/v1/sessions, then a
// stream consumer on /api/v1/events WITHOUT session_id must terminate on its
// own within a bounded window.
func TestEval_C49_PostSessionThenGlobalStreamTerminates(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	old := globalStreamMaxDuration
	globalStreamMaxDuration = 200 * time.Millisecond
	defer func() { globalStreamMaxDuration = old }()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. POST /api/v1/sessions
	body := []byte(`{"model_id":"gpt-4o","agent_name":"eval-agent","goal":"eval goal","title":"eval-c49"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST sessions: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	t.Logf("POST /api/v1/sessions -> %d %s", resp.StatusCode, string(raw))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: expected 2xx, got %d: %s", resp.StatusCode, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)

	// 2. Stream consumer on /api/v1/events WITHOUT session_id
	sresp := sseConnect(t, ts, srv.adminKey, "")
	defer func() { _ = sresp.Body.Close() }()
	if sresp.StatusCode != http.StatusOK {
		t.Fatalf("global SSE connect: expected 200, got %d", sresp.StatusCode)
	}

	reader := bufio.NewReader(sresp.Body)
	if evt, _ := readSSEEvent(t, reader); evt != "connected" {
		t.Fatalf("first frame: expected connected, got %q", evt)
	}

	start := time.Now()
	done := make(chan string, 1)
	go func() {
		evt, _ := readSSEEvent(t, reader)
		done <- evt
	}()

	select {
	case evt := <-done:
		elapsed := time.Since(start)
		t.Logf("stream terminated after %v with terminal event %q", elapsed, evt)
		if evt != "stream_timeout" {
			t.Errorf("expected stream_timeout, got %q", evt)
		}
		if elapsed > 5*time.Second {
			t.Errorf("termination took too long: %v", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("global SSE stream did NOT terminate within bounded window (hang)")
	}
}
