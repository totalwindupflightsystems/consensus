package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// ROUTE-ADD-049 focused contract tests: POST /api/session/{sessionID}/model
// (v2.session.switchModel, declared responses 204, 400, 401, 404; POST the
// only declared method).
//
// Merge-completion note (INT-BOARD-1): the original branch tests asserted
// only "404 for an unknown session" against the pre-merge handler whose
// existence check could not tell "no such session" from "the store could not
// be read" (internal/db reports a missing row as its own error value on this
// tree). These tests pin the full declared arm set, including the two answers
// the merged isNoRowsResult seam exists to tell apart.
// ============================================================================

// TestApiSessionModelSwitchUnknownSessionIsDeclared404 pins the declared
// NotFoundError arm: an id this instance does not hold answers 404 with the
// typed SessionNotFoundError body (the ROUTE-ADD-041 shape).
func TestApiSessionModelSwitchUnknownSessionIsDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "POST", "/api/session/nonexistent/model")
	if status != http.StatusNotFound {
		t.Fatalf("POST /api/session/nonexistent/model: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("404 body is not JSON: %v (%s)", err, body)
	}
	// The v2 404 carries the typed error shape with the addressed id
	// ({_tag, sessionID, message} — the shared writeOpencodeNotFoundError
	// shape the sibling ROUTE-ADD-041 404 answers with).
	if got["_tag"] != "SessionNotFoundError" {
		t.Errorf("_tag = %v, want SessionNotFoundError (body %v)", got["_tag"], got)
	}
	if got["sessionID"] != "nonexistent" {
		t.Errorf("sessionID = %v, want the addressed id (body %v)", got["sessionID"], got)
	}
	if !strings.Contains(string(body), "nonexistent") {
		t.Errorf("404 body %s does not name the addressed sessionID", body)
	}
}

// TestApiSessionModelSwitchUnreadableStoreIsDeclared400 pins the arm the
// pre-merge handler could not answer: a store this instance cannot read is
// NOT proof a session is absent, so it answers the declared
// InvalidRequestError 400 (the operation declares no 5xx arm) instead of a
// fabricated 404.
func TestApiSessionModelSwitchUnreadableStoreIsDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: disk I/O error")})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "POST", "/api/session/11111111-2222-3333-4444-555555555555/model")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session/{id}/model with unreadable store: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError (body %v)", got["_tag"], got)
	}
}

// TestApiSessionModelSwitchKnownSessionIsDeclared204 pins the declared
// success arm: a well-formed request for a session this instance holds
// answers 204 with an empty body — the truthful no-op (no model-switch
// pipeline exists to run).
func TestApiSessionModelSwitchKnownSessionIsDeclared204(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "11111111-2222-3333-4444-555555555555"})})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, "POST", "/api/session/11111111-2222-3333-4444-555555555555/model")
	if status != http.StatusNoContent {
		t.Fatalf("POST /api/session/{id}/model for a held session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 body = %q, want empty (the declared success carries no body)", body)
	}
	if ct := header.Get("Content-Type"); ct != "" {
		t.Errorf("204 Content-Type = %q, want none (no body is written)", ct)
	}
}

// TestApiSessionModelSwitchDeclared401 pins the declared UnauthorizedError arm
// ON THE LIVE MIDDLEWARE: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — the arm the branch commit's literal
// v2EnvelopeRoutePaths entry could never serve (the map is keyed by concrete
// request paths), which the merged isApiSessionModelPath rule serves.
func TestApiSessionModelSwitchDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "POST", "/api/session/ses_abc/model")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/session/ses_abc/model: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError (the declared 401 body)", got["_tag"])
	}

	// Membership stays two-segment: the sibling one-segment v2.session.get
	// shape and deeper paths keep the shim's own envelope.
	if !isApiSessionModelPath("/api/session/ses_abc/model") {
		t.Error("/api/session/{id}/model must be admitted to the v2 envelope (the declared 401 arm)")
	}
	if isApiSessionModelPath("/api/session/ses_abc") {
		t.Error("/api/session/{id} is v2.session.get's shape, not the model-switch shape")
	}
	if isApiSessionModelPath("/api/session/ses_abc/model/deeper") {
		t.Error("deeper /api/session/{id}/model/* paths must keep the shim's own envelope")
	}
	if isApiSessionModelPath("/api/session//model") {
		t.Error("a blank sessionID segment must not be admitted (it is the declared 400)")
	}
}

// TestApiSessionModelSwitchUndeclaredMethod pins the method contract: POST is
// the only method the pinned document declares on this path, so every other
// method answers METHOD_NOT_ALLOWED 405 — never a success payload, never a
// session lookup.
func TestApiSessionModelSwitchUndeclaredMethod(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "ses123abc"})})
	defer srv.Close()

	for _, method := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/session/ses123abc/model")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/session/ses123abc/model: got %d, want 405 (POST is the only declared method). Body: %s",
				method, status, body)
		}
	}
}

// TestApiSessionModelSwitchBlankSessionID pins the declared
// InvalidRequestError arm: a present-but-blank sessionID segment (%20)
// answers the v2 400 envelope naming the field.
func TestApiSessionModelSwitchBlankSessionID(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, "POST", "/api/session/%20/model")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session/%%20/model: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError (body %v)", got["_tag"], got)
	}
	if got["field"] != "sessionID" {
		t.Errorf("field = %v, want sessionID (body %v)", got["field"], got)
	}
}
