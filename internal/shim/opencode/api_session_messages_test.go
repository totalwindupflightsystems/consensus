package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// ROUTE-ADD-047 focused tests — GET /api/session/{sessionID}/message
// (v2.session.messages; declared responses 200, 400, 401, 404, 500)
// ============================================================================

// TestApiSessionMessagesUnknownSessionAnswersDeclared404 pins the declared
// 404 arm (typed SessionNotFoundError, decided against the store).
func TestApiSessionMessagesUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errNoRowsInResult()})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/message")
	if status != http.StatusNotFound {
		t.Fatalf("GET message for an unknown session: got %d, want 404. Body: %s", status, body)
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
}

// TestApiSessionMessagesKnownSessionAnswersDeclared200 pins the declared
// success arm on the REAL fixture store: the {data, cursor} envelope, the
// display-state rules, and the cursor members emitted only for real pages.
func TestApiSessionMessagesKnownSessionAnswersDeclared200(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message")
	if status != http.StatusOK {
		t.Fatalf("GET message for a known session: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Data   []map[string]any `json:"data"`
		Cursor map[string]any   `json:"cursor"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 200 body: %v (%s)", err, body)
	}
	if len(got.Data) != 2 {
		t.Fatalf("message data = %d entries (%s), want 2 (hidden row omitted)", len(got.Data), body)
	}
	if got.Data[0]["memory_message_id"].(float64) != 1 || got.Data[1]["memory_message_id"].(float64) != 2 {
		t.Errorf("expected ledger ids 1 and 2; got %v", got.Data)
	}
	if got.Data[1]["display_mode"] != "compressed" || got.Data[1]["summary_text"] == "" || got.Data[1]["content"] != nil {
		t.Errorf("compressed row must carry summary_text and withhold content; got %v", got.Data[1])
	}
	// A single short page carries an empty cursor object — no invented
	// neighbor pages.
	if len(got.Cursor) != 0 {
		t.Errorf("cursor must be empty when no neighbor page exists; got %v", got.Cursor)
	}
}

// TestApiSessionMessagesPaginationEmitsRealCursor pins the pagination arm:
// limit=1 leaves a next page (cursor.next = the boundary id) and the next
// request carrying that cursor answers the remaining page, never mixing
// order and cursor.
func TestApiSessionMessagesPaginationEmitsRealCursor(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message?limit=1")
	if status != http.StatusOK {
		t.Fatalf("first page: got %d, want 200. Body: %s", status, body)
	}
	var page1 struct {
		Data   []map[string]any `json:"data"`
		Cursor map[string]any   `json:"cursor"`
	}
	if err := json.Unmarshal(body, &page1); err != nil {
		t.Fatalf("decode page 1: %v (%s)", err, body)
	}
	if len(page1.Data) != 1 || page1.Data[0]["memory_message_id"].(float64) != 1 {
		t.Fatalf("page 1 must hold id 1; got %v", page1.Data)
	}
	next, ok := page1.Cursor["next"].(string)
	if !ok || next == "" {
		t.Fatalf("page 1 must emit cursor.next (a further page exists); got %v", page1.Cursor)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message?cursor="+next)
	if status != http.StatusOK {
		t.Fatalf("second page: got %d, want 200. Body: %s", status, body)
	}
	var page2 struct {
		Data   []map[string]any `json:"data"`
		Cursor map[string]any   `json:"cursor"`
	}
	if err := json.Unmarshal(body, &page2); err != nil {
		t.Fatalf("decode page 2: %v (%s)", err, body)
	}
	if len(page2.Data) != 1 || page2.Data[0]["memory_message_id"].(float64) != 2 {
		t.Errorf("page 2 must hold id 2 (id 3 hidden); got %v", page2.Data)
	}
	prev, ok := page2.Cursor["previous"].(string)
	if !ok || prev != next {
		t.Errorf("page 2 must carry cursor.previous pointing back at the boundary; got %v", page2.Cursor)
	}
	if _, ok := page2.Cursor["next"]; ok {
		t.Errorf("page 2 is the last page; cursor.next must be absent. Got %v", page2.Cursor)
	}
}

// TestApiSessionMessagesStoreFailureAnswersDeclared500 pins the arm this
// route owns alone in the family: a store the instance cannot read answers
// the declared 500 UnknownError — never 400, never a claimed absence.
func TestApiSessionMessagesStoreFailureAnswersDeclared500(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: contextErrorBoom(), queryErr: contextErrorBoom()})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/message")
	if status != http.StatusInternalServerError {
		t.Fatalf("store failure: got %d, want 500 (the only declared 5xx in the family). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 500 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnknownError" {
		t.Errorf("500 _tag = %v, want UnknownError. Body: %s", got["_tag"], body)
	}
}

// TestApiSessionMessagesBlankSegmentAndSelectorsAnswerDeclared400 pins the
// declared 400 arms including the declared cursor rules (blank, non-token,
// cursor+order together).
func TestApiSessionMessagesBlankSegmentAndSelectorsAnswerDeclared400(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		kind  string
		field string
	}{
		{"blank sessionID segment", "/api/session/%20/message", "Path", "sessionID"},
		{"blank limit", "/api/session/" + historyFixtureSessionID + "/message?limit=", "Query", "limit"},
		{"bad limit", "/api/session/" + historyFixtureSessionID + "/message?limit=-4", "Query", "limit"},
		{"order outside the enum", "/api/session/" + historyFixtureSessionID + "/message?order=sideways", "Query", "order"},
		{"blank cursor", "/api/session/" + historyFixtureSessionID + "/message?cursor=", "Query", "cursor"},
		{"cursor combined with order", "/api/session/" + historyFixtureSessionID + "/message?cursor=2&order=desc", "Query", "cursor"},
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

// TestApiSessionMessagesForeignCursorAnswersDeclared400 pins the store-
// decided cursor rule: a token that names no memory_events row of THIS
// session is the declared invalid cursor (kind Cursor).
func TestApiSessionMessagesForeignCursorAnswersDeclared400(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message?cursor=999")
	if status != http.StatusBadRequest {
		t.Fatalf("foreign cursor: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, body)
	}
	if got["kind"] != "Cursor" || got["field"] != "cursor" {
		t.Errorf("400 body = %v, want kind Cursor field cursor", got)
	}
}

// TestApiSessionMessagesServesDeclared401 pins the v2 envelope through
// isApiSessionMessagesPath.
func TestApiSessionMessagesServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/message")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET message: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 _tag = %v, want UnauthorizedError. Body: %s", got["_tag"], body)
	}
}

// TestApiSessionMessagesUndeclaredMethodAnswers405 pins the method guard.
func TestApiSessionMessagesUndeclaredMethodAnswers405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodDelete,
		"/api/session/"+historyFixtureSessionID+"/message", "")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE message: got %d, want 405. Body: %s", status, body)
	}
}

// TestIsApiSessionMessagesPath pins the envelope-shape predicate.
func TestIsApiSessionMessagesPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/message":            true,
		"/api/session/abc/history":            false,
		"/api/session/abc/message/msg_1":      false,
		"/api/session/abc":                    false,
		"/api/session/abc/message/x":          false,
		"/api/session//message":               false,
		"/api/session/abc/agent":              false,
		"/api/provider":                       false,
	}
	for path, want := range cases {
		if got := isApiSessionMessagesPath(path); got != want {
			t.Errorf("isApiSessionMessagesPath(%q) = %v, want %v", path, got, want)
		}
	}
}
