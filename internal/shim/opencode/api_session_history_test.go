package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// ROUTE-ADD-045 focused tests — GET /api/session/{sessionID}/history
// (v2.session.history; declared responses 200, 400, 401, 404)
// ============================================================================

// historyFixtureSessionID is the session the fixture store seeds.
const historyFixtureSessionID = "9b1c4d2e-0000-4000-8000-abcdefabcdef"

// newHistoryFixtureServer opens a REAL sqlite store (the migration-001
// §3.1/§3.2 shape: memory_events with summary_text, a separate
// display_modes table), seeds one session plus a ledger carrying a full, a
// compressed and a hidden row (ids 1..3 in insertion order), and serves the
// shim over it with auth bypassed.
func newHistoryFixtureServer(t *testing.T) (*Server, *httptest.Server, func()) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "history.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open history fixture store: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'idle',
			iteration INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			summary_text TEXT,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE display_modes (
			memory_id INTEGER NOT NULL PRIMARY KEY,
			mode TEXT NOT NULL DEFAULT 'full',
			set_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			set_by_iteration INTEGER NOT NULL DEFAULT 0,
			session_id TEXT NOT NULL
		)`,
		`INSERT INTO sessions (id, status) VALUES ('` + historyFixtureSessionID + `', 'idle')`,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('user_message', 'hello from the user', '` + historyFixtureSessionID + `', 0)`,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'the long original answer text', '` + historyFixtureSessionID + `', 0)`,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('text_block', 'third event', '` + historyFixtureSessionID + `', 0)`,
		`INSERT INTO display_modes (memory_id, mode, session_id) VALUES (2, 'compressed', '` + historyFixtureSessionID + `')`,
		`UPDATE memory_events SET summary_text = 'compact summary of event 2' WHERE id = 2`,
		`INSERT INTO display_modes (memory_id, mode, session_id) VALUES (3, 'hidden', '` + historyFixtureSessionID + `')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed history fixture store: %v (%s)", err, stmt)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	return s, srv, func() { _ = conn.Close(); srv.Close() }
}

// TestApiSessionHistoryUnknownSessionAnswersDeclared404 pins the declared
// 404 arm: a sessionID this instance does not hold answers the typed
// SessionNotFoundError envelope {_tag, sessionID, message}.
func TestApiSessionHistoryUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errNoRowsInResult()})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/history")
	if status != http.StatusNotFound {
		t.Fatalf("GET history for an unknown session: got %d, want 404. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "SessionNotFoundError" {
		t.Errorf("404 _tag = %v, want SessionNotFoundError. Body: %s", got["_tag"], body)
	}
	if _, ok := got["sessionID"]; !ok {
		t.Errorf("404 body must carry the declared sessionID member. Body: %s", body)
	}
	if _, ok := got["message"]; !ok {
		t.Errorf("404 body must carry the declared message member. Body: %s", body)
	}
}

// TestApiSessionHistoryKnownSessionAnswersDeclared200 pins the declared
// success arm against the REAL fixture store: the declared {data, hasMore}
// envelope, oldest first, hidden rows omitted, compressed rows carrying
// summary_text and withholding content, full rows carrying content, and no
// fabricated ^evt_ identity anywhere in an entry.
func TestApiSessionHistoryKnownSessionAnswersDeclared200(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/history")
	if status != http.StatusOK {
		t.Fatalf("GET history for a known session: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data []map[string]any `json:"data"`
		Has  bool             `json:"hasMore"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 200 body: %v (%s)", err, body)
	}
	if len(got.Data) != 2 {
		t.Fatalf("history data = %d entries (%s), want 2 (hidden row omitted)", len(got.Data), body)
	}
	first, second := got.Data[0], got.Data[1]
	// The seeded display_modes row marks the third insert (id 3) hidden, so
	// the surviving pair must be ids 1 and 2 — ASC order proves the ORDER BY,
	// a third entry would prove the hidden row leaked.
	if first["memory_event_id"].(float64) != 1 || second["memory_event_id"].(float64) != 2 {
		t.Errorf("expected ledger ids 1 and 2 after hiding id 3; got %v", got.Data)
	}
	if first["content"] != "hello from the user" {
		t.Errorf("full row must carry its content verbatim; got %v", first["content"])
	}
	if second["display_mode"] != "compressed" {
		t.Errorf("compressed row must name its display_mode; got %v", second)
	}
	if second["summary_text"] == "" || second["content"] != nil {
		t.Errorf("compressed row must carry summary_text and withhold content; got %v", second)
	}
	if got.Has {
		t.Errorf("hasMore must be false for a page shorter than the limit")
	}
	for _, entry := range got.Data {
		for key := range entry {
			if strings.HasPrefix(key, "evt_") {
				t.Errorf("fabricated ^evt_ identity %q leaked into the history entry", key)
			}
		}
	}
}

// TestApiSessionHistoryBlankSegmentAndSelectorsAnswerDeclared400 pins the
// declared 400 arms: a present-but-blank sessionID segment (%20) and the
// present-but-invalid declared query selectors.
func TestApiSessionHistoryBlankSegmentAndSelectorsAnswerDeclared400(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		kind  string
		field string
	}{
		{"blank sessionID segment", "/api/session/%20/history", "Path", "sessionID"},
		{"blank limit", "/api/session/" + historyFixtureSessionID + "/history?limit=", "Query", "limit"},
		{"non-numeric limit", "/api/session/" + historyFixtureSessionID + "/history?limit=many", "Query", "limit"},
		{"zero limit", "/api/session/" + historyFixtureSessionID + "/history?limit=0", "Query", "limit"},
		{"blank after", "/api/session/" + historyFixtureSessionID + "/history?after=", "Query", "after"},
		{"non-numeric after", "/api/session/" + historyFixtureSessionID + "/history?after=evt_later", "Query", "after"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(&mockDB{})
			defer srv.Close()

			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400. Body: %s", tc.path, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode 400 body: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" || got["kind"] != tc.kind || got["field"] != tc.field {
				t.Errorf("400 body = %v, want _tag InvalidRequestError kind %s field %s", got, tc.kind, tc.field)
			}
		})
	}
}

// TestApiSessionHistoryServesDeclared401 pins the declared UnauthorizedError
// arm: an unauthenticated request answers the v2 envelope through the
// isApiSessionHistoryPath shape, not the shim's own v1 body.
func TestApiSessionHistoryServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/history")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET history: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 _tag = %v, want UnauthorizedError (the v2 envelope). Body: %s", got["_tag"], body)
	}
}

// TestApiSessionHistoryUndeclaredMethodAnswers405 pins the method guard.
func TestApiSessionHistoryUndeclaredMethodAnswers405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/"+historyFixtureSessionID+"/history", "")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST history: got %d, want 405. Body: %s", status, body)
	}
}

// TestApiSessionHistoryStoreAnswerArms separates the two real store answers
// on the REAL fixture store: the unknown-session 404 and the happy 200 both
// come from the same seam (the mockDB pair cannot distinguish them — the
// ROUTE-ADD-041 live-probe lesson).
func TestApiSessionHistoryStoreAnswerArms(t *testing.T) {
	s, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/00000000-0000-4000-8000-ffffffffffff/history")
	if status != http.StatusNotFound || !strings.Contains(string(body), "SessionNotFoundError") {
		t.Fatalf("unknown session on real store: got %d, want 404 SessionNotFoundError. Body: %s", status, body)
	}

	if err := s.db.Exec(context.Background(),
		`INSERT INTO sessions (id, status) VALUES ('b7e2c3d4-0000-4000-8000-abcdefabcdef', 'idle')`); err != nil {
		t.Fatalf("seed empty session: %v", err)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/b7e2c3d4-0000-4000-8000-abcdefabcdef/history")
	if status != http.StatusOK {
		t.Fatalf("empty-ledger history: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode empty 200: %v (%s)", err, body)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Errorf("empty ledger must answer an empty non-null data array; got %s", body)
	}
}

// TestApiSessionHistoryAfterSelectorPinsResumeToken pins the declared after
// selector: entries strictly after the boundary id, hasMore false.
func TestApiSessionHistoryAfterSelectorPinsResumeToken(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/history?after=1")
	if status != http.StatusOK {
		t.Fatalf("history after=1: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode after=1 body: %v (%s)", err, body)
	}
	if len(got.Data) != 1 || got.Data[0]["memory_event_id"].(float64) != 2 {
		t.Errorf("after=1 must replay only id 2 (id 3 hidden); got %v", got.Data)
	}
}

// TestIsApiSessionHistoryPath pins the envelope-shape predicate.
func TestIsApiSessionHistoryPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/history":   true,
		"/api/session/abc/message":   false,
		"/api/session/abc":           false,
		"/api/session/active":        false,
		"/api/session/abc/history/x": false,
		"/api/session//history":      false,
		"/api/session/abc/agent":     false,
		"/api/session/abc/model":     false,
		"/api/provider":              false,
	}
	for path, want := range cases {
		if got := isApiSessionHistoryPath(path); got != want {
			t.Errorf("isApiSessionHistoryPath(%q) = %v, want %v", path, got, want)
		}
	}
}
