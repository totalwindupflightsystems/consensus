package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// ROUTE-ADD-048 focused tests — GET /api/session/{sessionID}/message/
// {messageID} (v2.session.message; declared responses 200, 400, 401, 404)
// ============================================================================

// errNoRowsInResult reproduces the internal/db driver's missing-row error —
// the seam isNoRowsResult keys on (there is no sql.ErrNoRows sentinel on
// this seam; the ROUTE-ADD-041 live-probe lesson).
func errNoRowsInResult() error {
	return errors.New("sqlite: no rows in result set")
}

// contextErrorBoom is a store failure that is NOT a missing row — the arm
// that must keep the declared 400 (history / message-get) or 500 (message
// list), never a claimed absence.
func contextErrorBoom() error {
	return errors.New("context deadline exceeded (store unavailable)")
}

// TestApiSessionMessageGetUnknownSessionAnswersSession404 pins the session
// half of the ambiguous declared 404: the session is resolved FIRST and the
// typed SessionNotFoundError names sessionID.
func TestApiSessionMessageGetUnknownSessionAnswersSession404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errNoRowsInResult()})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/message/msg_42")
	if status != http.StatusNotFound {
		t.Fatalf("GET message on unknown session: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "SessionNotFoundError" {
		t.Errorf("404 _tag = %v, want SessionNotFoundError (session resolved first). Body: %s", got["_tag"], body)
	}
	if _, ok := got["sessionID"]; !ok {
		t.Errorf("session 404 must name sessionID. Body: %s", body)
	}
	if _, ok := got["messageID"]; ok {
		t.Errorf("session 404 must NOT carry the messageID member — that is the message arm's shape. Body: %s", body)
	}
}

// TestApiSessionMessageGetUnknownMessageAnswersMessage404 pins the message
// half: an existing session with an unknown messageID answers the typed
// MessageNotFoundError {_tag, sessionID, messageID, message}.
func TestApiSessionMessageGetUnknownMessageAnswersMessage404(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message/msg-999")
	if status != http.StatusNotFound {
		t.Fatalf("GET unknown message in known session: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "MessageNotFoundError" {
		t.Errorf("404 _tag = %v, want MessageNotFoundError. Body: %s", got["_tag"], body)
	}
	if _, ok := got["messageID"]; !ok {
		t.Errorf("message 404 must name messageID. Body: %s", body)
	}
	if _, ok := got["sessionID"]; !ok {
		t.Errorf("the declared MessageNotFoundError carries sessionID too. Body: %s", body)
	}
}

// TestApiSessionMessageGetHeldMessageNeverFabricates200 pins the narrowing:
// even a message the session HOLDS answers the declared 404 naming
// messageID — the declared SessionMessage 200 (^msg_-keyed union) is never
// fabricated from the integer-keyed ledger (file doc comment), and this is
// what SHIM-NARROWED-107 records.
func TestApiSessionMessageGetHeldMessageNeverFabricates200(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message/msg-1")
	if status != http.StatusNotFound {
		t.Fatalf("GET held message: got %d, want the declared 404 (never a fabricated 200). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 404 body: %v (%s)", err, body)
	}
	if got["_tag"] != "MessageNotFoundError" {
		t.Errorf("held-message 404 _tag = %v, want MessageNotFoundError. Body: %s", got["_tag"], body)
	}
	if got["messageID"] != "msg-1" {
		t.Errorf("held-message 404 must name the addressed messageID. Body: %s", body)
	}
}

// TestApiSessionMessageGetBlankSegmentsAnswerDeclared400 pins the declared
// 400 arms: blank sessionID and blank messageID (kind Path).
func TestApiSessionMessageGetBlankSegmentsAnswerDeclared400(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		field string
	}{
		{"blank sessionID segment", "/api/session/%20/message/msg_1", "sessionID"},
		{"blank messageID segment", "/api/session/" + historyFixtureSessionID + "/message/%20", "messageID"},
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
			if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" || got["field"] != tc.field {
				t.Errorf("400 body = %v, want _tag InvalidRequestError kind Path field %s", got, tc.field)
			}
		})
	}
}

// TestApiSessionMessageGetStoreFailureAnswersDeclared400 pins the no-5xx
// rule: a store failure that is NOT a missing row keeps the declared
// InvalidRequestError 400 (this operation declares no 500 arm — the sibling
// v2.session.messages owns that arm).
func TestApiSessionMessageGetStoreFailureAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: contextErrorBoom()})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/"+historyFixtureSessionID+"/message/msg_1")
	if status != http.StatusBadRequest {
		t.Fatalf("store failure on message-get: got %d, want 400 (no declared 5xx arm). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 400 body: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("store-failure 400 _tag = %v, want InvalidRequestError. Body: %s", got["_tag"], body)
	}
}

// TestApiSessionMessageGetServesDeclared401 pins the v2 envelope through
// isApiSessionMessageGetPath.
func TestApiSessionMessageGetServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/message/msg_1")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET message/{id}: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 _tag = %v, want UnauthorizedError. Body: %s", got["_tag"], body)
	}
}

// TestApiSessionMessageGetUndeclaredMethodAnswers405 pins the method guard.
func TestApiSessionMessageGetUndeclaredMethodAnswers405(t *testing.T) {
	_, srv, cleanup := newHistoryFixtureServer(t)
	defer cleanup()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/"+historyFixtureSessionID+"/message/msg-1", "")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST message/{id}: got %d, want 405. Body: %s", status, body)
	}
}

// TestIsApiSessionMessageGetPath pins the envelope-shape predicate.
func TestIsApiSessionMessageGetPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/message/msg_1":       true,
		"/api/session/abc/message":             false,
		"/api/session/abc/message/":            false,
		"/api/session/abc/message/msg_1/extra": false,
		"/api/session//message/msg_1":          false,
		"/api/session/abc/history":             false,
		"/api/session/abc":                     false,
		"/api/provider":                        false,
	}
	for path, want := range cases {
		if got := isApiSessionMessageGetPath(path); got != want {
			t.Errorf("isApiSessionMessageGetPath(%q) = %v, want %v", path, got, want)
		}
	}
}
