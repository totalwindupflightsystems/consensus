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
// ROUTE-ADD-041 focused tests — POST /api/session/{sessionID}/agent
// (v2.session.switchAgent; declared responses 204, 400, 401, 404)
// ============================================================================

const apiSessionAgentBody = `{"agent":"build"}`

// TestApiSessionAgentUnknownSessionAnswersDeclared404 pins the declared 404
// arm: a sessionID this instance does not hold answers the typed
// SessionNotFoundError envelope {_tag, sessionID, message}, not a fabricated
// success and not the shim's own 404 body.
func TestApiSessionAgentUnknownSessionAnswersDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent", apiSessionAgentBody)
	if status != http.StatusNotFound {
		t.Fatalf("POST /api/session/{id}/agent for an unknown session: got %d, want 404. Body: %s", status, body)
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

// TestApiSessionAgentKnownSessionAnswersDeclared204 pins the declared success
// arm for a session the instance holds: the empty 204, with no body asserting
// an agent change the runtime cannot back.
func TestApiSessionAgentKnownSessionAnswersDeclared204(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": "9b1c4d2e-0000-4000-8000-abcdefabcdef"})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent", apiSessionAgentBody)
	if status != http.StatusNoContent {
		t.Fatalf("POST /api/session/{id}/agent for a known session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 must carry no body; got %q", body)
	}
}

// TestApiSessionAgentBodyViolationsAnswerDeclared400 pins the declared 400
// arm for every declared body rule: the body is REQUIRED, must be a single
// JSON object, must not carry undeclared fields (additionalProperties:false),
// and agent must be a present, non-blank string.
func TestApiSessionAgentBodyViolationsAnswerDeclared400(t *testing.T) {
	cases := []struct {
		name string
		body string
		kind string
		field string
	}{
		{"absent body", ``, "Body", "body"},
		{"not an object", `"build"`, "Body", "body"},
		{"null body", `null`, "Body", "body"},
		{"missing agent", `{}`, "Body", "agent"},
		{"agent not a string", `{"agent":7}`, "Body", "agent"},
		{"blank agent", `{"agent":"   "}`, "Body", "agent"},
		{"undeclared field", `{"agent":"build","model":"x"}`, "Body", "model"},
		{"multiple JSON values", `{"agent":"build"} {"agent":"plan"}`, "Body", "body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(&mockDB{})
			defer srv.Close()

			status, header, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
				"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("body %q: got %d, want 400. Body: %s", tc.body, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode 400 body: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("400 _tag = %v, want InvalidRequestError. Body: %s", got["_tag"], body)
			}
			if got["kind"] != tc.kind {
				t.Errorf("400 kind = %v, want %q. Body: %s", got["kind"], tc.kind, body)
			}
			if got["field"] != tc.field {
				t.Errorf("400 field = %v, want %q. Body: %s", got["field"], tc.field, body)
			}
		})
	}
}

// TestApiSessionAgentBlankSessionIDSegmentAnswersDeclared400 pins the declared
// 400 arm for a present-but-blank path parameter (the ROUTE-ADD-039/025
// convention: a blank segment is the only malformed spelling of a plain
// required string).
func TestApiSessionAgentBlankSessionIDSegmentAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		"/api/session/%20/agent", apiSessionAgentBody)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session/%%20/agent: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 400 body: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" || got["kind"] != "Path" {
		t.Errorf("blank sessionID: body = %s, want InvalidRequestError kind Path", body)
	}
}

// TestApiSessionAgentStoreAnswerArms pins the two store answers the LIVE probe
// distinguished (these were a real defect when the probe first ran: the driver
// reports a missing row as an error, so a bare `err != nil` would have answered
// the declared 400 for every unknown session and left the declared 404
// unreachable on a running server):
//
//	a "no rows in result" reply -> the declared 404 SessionNotFoundError
//	any other read failure      -> the declared 400 (absence is never claimed
//	                               from a store the instance could not read)
func TestApiSessionAgentStoreAnswerArms(t *testing.T) {
	t.Run("no rows in result answers 404", func(t *testing.T) {
		mdb := &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}
		_, srv := newTestServer(mdb)
		defer srv.Close()

		status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
			"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent", apiSessionAgentBody)
		if status != http.StatusNotFound {
			t.Fatalf("no-rows reply: got %d, want 404. Body: %s", status, body)
		}
		if !strings.Contains(string(body), "SessionNotFoundError") {
			t.Errorf("404 body is not the declared SessionNotFoundError: %s", body)
		}
	})

	t.Run("unreadable store answers 400", func(t *testing.T) {
		mdb := &mockDB{queryRowErr: errors.New("disk I/O error")}
		_, srv := newTestServer(mdb)
		defer srv.Close()

		status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
			"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent", apiSessionAgentBody)
		if status != http.StatusBadRequest {
			t.Fatalf("unreadable store: got %d, want 400. Body: %s", status, body)
		}
		if !strings.Contains(string(body), "InvalidRequestError") {
			t.Errorf("400 body is not the declared InvalidRequestError: %s", body)
		}
	})
}

// TestApiSessionAgentUndeclaredMethodAnswers405 pins the sibling
// METHOD_NOT_ALLOWED envelope: POST is the only method the pinned document
// declares on this path, so GET keeps the shim's own 405 envelope.
func TestApiSessionAgentUndeclaredMethodAnswers405(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET on the POST-only path: got %d, want 405. Body: %s", status, body)
	}
	if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
		t.Errorf("405 body does not carry METHOD_NOT_ALLOWED: %s", body)
	}
}

// TestApiSessionAgentServesDeclared401 pins the declared UnauthorizedError
// arm: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) rather than the shim's own v1
// UNAUTHENTICATED body, through the exact two-segment shape admitted by
// isV2EnvelopeRoutePath.
func TestApiSessionAgentServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
		"/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/session/{id}/agent: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 _tag = %v, want UnauthorizedError (the v2 envelope). Body: %s", got["_tag"], body)
	}
}

// TestIsApiSessionAgentPath pins the envelope-shape predicate: only the
// declared two-segment "agent" shape joins the v2 UnauthorizedError branch;
// sibling session paths and deeper sub-paths keep the shim's own envelope.
func TestIsApiSessionAgentPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/agent":     true,
		"/api/session/abc/compact":   false,
		"/api/session/abc":           false,
		"/api/session/active":        false,
		"/api/session/abc/agent/x":   false,
		"/api/session//agent":        false,
		"/api/session/abc/agent/":    false,
		"/api/provider":              false,
	}
	for path, want := range cases {
		if got := isApiSessionAgentPath(path); got != want {
			t.Errorf("isApiSessionAgentPath(%q) = %v, want %v", path, got, want)
		}
	}
}
