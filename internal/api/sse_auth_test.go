// Package api: integration tests for SSE auth on /api/v1/events
// (DF-CONSENSUS-30, SPEC-015 §4.2).
//
// The defect: the route was registered OUTSIDE the authenticated group, so
// an unauthenticated client that knew only a session UUID streamed that
// session's live events (connected + session_update frames) — the SSE hole
// in the same wall the REST layer enforces with 403. The contract pinned
// here (specs/015-api-and-mcp.md §4.2):
//
//   - no/invalid key              -> 401 UNAUTHENTICATED (JSON envelope,
//     written BEFORE any text/event-stream header)
//   - admin key                   -> 200, streams (connected frame first)
//   - session key, own session    -> 200, streams
//   - session key, foreign session -> 403 FORBIDDEN
//   - session key, global stream  -> 403 (a global stream carries every
//     session's events, so a scoped key must not open one)
//
// Provenance: dogfood live-repro 2026-09-25 (board row DF-CONSENSUS-30).
package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mintSessionKey mints a session-scoped key via the real API and returns
// its full secret (the show-once value from the mint contract).
func mintSessionKey(t *testing.T, srv *integrationServer, sessionID string) string {
	t.Helper()
	rec, resp := mintKeyViaAPI(srv, `{"scope":"session","session_id":"`+sessionID+`"}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("mint session key: expected 2xx, got %d: %s", rec.Code, rec.Body.String())
	}
	apiKey, _ := resp["api_key"].(string)
	if apiKey == "" {
		t.Fatalf("mint session key: no api_key in response: %s", rec.Body.String())
	}
	return apiKey
}

// sseConnect opens GET /api/v1/events over a real HTTP server (streaming
// reads need a live connection, not an in-memory recorder) and returns the
// response with the body still open for frame reading.
func sseConnect(t *testing.T, ts *httptest.Server, bearer, query string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events"+query, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	return resp
}

// readSSEEvent reads one SSE frame ("event: X\ndata: Y\n\n") from the stream.
func readSSEEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var eventType, data string
	for {
		if time.Now().After(deadline) {
			t.Fatal("readSSEEvent: timeout waiting for a complete SSE frame")
		}
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE stream: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if eventType != "" {
				return eventType, data
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// TestSSE_UnauthenticatedConnect_Returns401 pins the wall: no key, no
// stream. The answer must be the standard JSON error envelope — proving the
// rejection happens before any text/event-stream header is written.
func TestSSE_UnauthenticatedConnect_Returns401(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/events?session_id=some-session", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated SSE connect: expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json error envelope, got Content-Type %q", ct)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not the JSON envelope: %v (%s)", err, w.Body.String())
	}
	if body.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("expected error code UNAUTHENTICATED, got %q", body.Error.Code)
	}
	if strings.Contains(w.Body.String(), "event:") {
		t.Errorf("SSE frames leaked past the auth wall: %s", w.Body.String())
	}
}

// TestSSE_InvalidKey_Returns401 covers the bad-token arm (not just missing).
func TestSSE_InvalidKey_Returns401(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?session_id=some-session", nil)
	req.Header.Set("Authorization", "Bearer cs_sk_totally_bogus_key_0000000000")
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid-key SSE connect: expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSSE_AdminKey_ConnectsAndStreams pins the admin arm: 200, real
// event-stream headers, connected frame first, and published session events
// still delivered on the wire.
func TestSSE_AdminKey_ConnectsAndStreams(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	sessionID := createSessionForMint(t, srv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := sseConnect(t, ts, srv.adminKey, "?session_id="+sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin SSE connect: expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream, got Content-Type %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	evtType, _ := readSSEEvent(t, reader)
	if evtType != "connected" {
		t.Errorf("first frame: expected connected, got %q", evtType)
	}

	// The stream must still deliver live events ("stream works as today").
	srv.EventBus().PublishSessionUpdate(sessionID, "thinking", 1)
	evtType, data := readSSEEvent(t, reader)
	if evtType != "session_update" {
		t.Errorf("second frame: expected session_update, got %q", evtType)
	}
	if !strings.Contains(data, `"thinking"`) || !strings.Contains(data, sessionID) {
		t.Errorf("session_update payload missing status/session: %s", data)
	}
}

// TestSSE_SessionKey_OwnSession_Streams pins the scoped-key happy path.
func TestSSE_SessionKey_OwnSession_Streams(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	sessionID := createSessionForMint(t, srv)
	key := mintSessionKey(t, srv, sessionID)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := sseConnect(t, ts, key, "?session_id="+sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("own-session SSE connect: expected 200, got %d: %s", resp.StatusCode, body)
	}
	reader := bufio.NewReader(resp.Body)
	evtType, _ := readSSEEvent(t, reader)
	if evtType != "connected" {
		t.Errorf("first frame: expected connected, got %q", evtType)
	}
}

// TestSSE_SessionKey_ForeignSession_Returns403 pins the wall the defect was
// about: a session-scoped key must NOT stream a foreign session — same
// semantics as the REST 403, and still a plain JSON error (no stream).
func TestSSE_SessionKey_ForeignSession_Returns403(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	ownID := createSessionForMint(t, srv)
	key := mintSessionKey(t, srv, ownID)
	foreignID := createSessionForMint(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?session_id="+foreignID, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign-session SSE connect: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FORBIDDEN") {
		t.Errorf("expected FORBIDDEN error code, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "event:") {
		t.Errorf("SSE frames leaked past the scope check: %s", w.Body.String())
	}
}

// TestSSE_SessionKey_GlobalStream_Returns403 pins the no-session_id arm:
// the global stream carries EVERY session's events, so a session-scoped key
// must not open one (admin/readonly still can — covered by the admin test).
func TestSSE_SessionKey_GlobalStream_Returns403(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	ownID := createSessionForMint(t, srv)
	key := mintSessionKey(t, srv, ownID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("global-stream connect with session key: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}
