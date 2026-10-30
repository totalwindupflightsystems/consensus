package opencode

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const eventFixtureSessionID = historyFixtureSessionID

func TestApiSessionEventKnownSessionStreamsDurableRows(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	resp, err := http.Get(srv.URL + "/api/session/" + eventFixtureSessionID + "/event?after=0")
	if err != nil {
		t.Fatalf("GET session events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET session events: got %d, want 200. Body: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	lines := make([]string, 0, 4)
	for len(lines) < 4 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read first SSE frame: %v; lines=%q", err, lines)
		}
		lines = append(lines, strings.TrimSuffix(line, "\n"))
	}
	if lines[0] != "id: 1" || lines[1] != "event: user_message" {
		t.Fatalf("first SSE frame header = %q, want id/event for ledger row 1", lines[:2])
	}
	if !strings.HasPrefix(lines[2], "data: ") {
		t.Fatalf("first SSE frame has no data line: %q", lines)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[2], "data: ")), &data); err != nil {
		t.Fatalf("decode first SSE data: %v", err)
	}
	if data["memory_event_id"] != float64(1) || data["content"] != "hello from the user" {
		t.Fatalf("first SSE data = %v, want runtime-native ledger id/content", data)
	}
}

func TestApiSessionEventAfterBoundaryAndHiddenRows(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	resp, err := http.Get(srv.URL + "/api/session/" + eventFixtureSessionID + "/event?after=1")
	if err != nil {
		t.Fatalf("GET session events after boundary: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("after boundary: got %d, want 200", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	frame := make([]string, 0, 4)
	for len(frame) < 4 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read after frame: %v; lines=%q", err, frame)
		}
		frame = append(frame, strings.TrimSuffix(line, "\n"))
	}
	if frame[0] != "id: 2" || frame[1] != "event: text_block" {
		t.Fatalf("after=1 first frame = %q, want compressed row 2", frame[:2])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(frame[2], "data: ")), &data); err != nil {
		t.Fatalf("decode compressed SSE data: %v", err)
	}
	if data["display_mode"] != "compressed" || data["summary_text"] != "compact summary of event 2" || data["content"] != nil {
		t.Fatalf("compressed SSE data = %v, want summary without content", data)
	}
}

func TestApiSessionEventDeclared400Validation(t *testing.T) {
	cases := []string{
		"/api/session/%20/event",
		"/api/session/" + eventFixtureSessionID + "/event?after=",
		"/api/session/" + eventFixtureSessionID + "/event?after=not-a-number",
		"/api/session/" + eventFixtureSessionID + "/event?after=-1",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			_, srv := newTestServer(&mockDB{})
			defer srv.Close()
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode 400 body: %v", err)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Fatalf("400 body = %v, want InvalidRequestError", got)
			}
		})
	}
}

func TestApiSessionEventUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errNoRowsInResult()})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+eventFixtureSessionID+"/event")
	if status != http.StatusNotFound {
		t.Fatalf("unknown session: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v", err)
	}
	if got["_tag"] != "SessionNotFoundError" || got["sessionID"] != eventFixtureSessionID {
		t.Fatalf("404 body = %v, want typed session not found", got)
	}
}

func TestApiSessionEventServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+eventFixtureSessionID+"/event")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated events: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v", err)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Fatalf("401 body = %v, want UnauthorizedError", got)
	}
}

func TestApiSessionEventMountAndExactPath(t *testing.T) {
	if !isApiSessionEventPath("/api/session/abc/event") {
		t.Fatal("event route shape was not recognized")
	}
	for _, path := range []string{"/api/session/abc", "/api/session/abc/history", "/api/session/abc/event/deeper", "/api/session//event"} {
		if isApiSessionEventPath(path) {
			t.Errorf("isApiSessionEventPath(%q) = true, want false", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/session/abc/event") {
		t.Fatal("event route is not admitted to v2 UnauthorizedError envelope")
	}
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/session/{sessionID}/event" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("event route missing from MountPatterns")
	}

	s := NewServer(&mockDB{queryRowErr: errNoRowsInResult()}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()
	status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/abc/event")
	if status != http.StatusNotFound {
		t.Fatalf("chi-mounted unknown event session: got %d, want typed 404", status)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/session/abc/event/deeper")
	if status != http.StatusNotFound {
		t.Fatalf("undeclared deeper path: got %d, want router 404", status)
	}
}

func TestApiSessionEventUndeclaredMethodAnswers405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/"+eventFixtureSessionID+"/event")
	if status != http.StatusMethodNotAllowed || !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Fatalf("POST event: got %d %q, want 405 METHOD_NOT_ALLOWED", status, body)
	}
}
