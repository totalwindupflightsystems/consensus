package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// syncStealValidSessionID is a session id that satisfies the declared pattern
// ^ses on both the sync.steal request body and its 200 response.
const syncStealValidSessionID = "ses_abc123"

// syncStealBody builds a declared-shape sync.steal request body — the object
// {sessionID: string} — from a raw session id, JSON-quoting it so each call
// site passes the id itself rather than a pre-encoded literal.
func syncStealBody(sessionID string) string {
	quoted, _ := json.Marshal(sessionID)
	return `{"sessionID":` + string(quoted) + `}`
}

// decodeSyncStealResponse decodes the declared 200 body — the object
// {sessionID: string, pattern ^ses} (required, additionalProperties: false) —
// and asserts its shape before returning the id.
func decodeSyncStealResponse(t *testing.T, path string, status int, header http.Header, body []byte) string {
	t.Helper()
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST %s: Content-Type = %q, want application/json", path, ct)
	}
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want 200. Body: %s", path, status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("POST %s body must be the declared JSON object: %v (%s)", path, err, body)
	}
	if len(got) != 1 {
		t.Errorf("POST %s: body carries %d keys, want exactly sessionID (declared additionalProperties: false): %v", path, len(got), got)
	}
	raw, ok := got["sessionID"]
	if !ok {
		t.Fatalf("POST %s: body %s is missing the required sessionID property", path, body)
	}
	sessionID, ok := raw.(string)
	if !ok {
		t.Fatalf("POST %s: sessionID = %v (%T), want a string", path, raw, raw)
	}
	if !strings.HasPrefix(sessionID, "ses") {
		t.Errorf("POST %s: sessionID = %q, want a value matching the declared pattern ^ses", path, sessionID)
	}
	return sessionID
}

// TestSyncStealAnswersDeclaredSessionID answers upstream sync.steal
// (ROUTE-ADD-115, SHIM-DRIFT-129, declared responses: 200 {sessionID},
// 400 BadRequest | InvalidRequestError): POST /sync/steal must answer 200 with
// the declared object instead of the pre-fix net/http default 404. The
// Consensus runtime keeps no sync event store and no cross-workspace migration
// engine, so no steal is performed; the session id the shim can report is the
// one the caller named (never a minted or fabricated id — the sibling
// handleSyncReplay convention), which is also the only value the declared ^ses
// response pattern admits. The operation declares optional directory/workspace
// query params alongside the body, so a valued param answers the same 200.
func TestSyncStealAnswersDeclaredSessionID(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, sessionID string
	}{
		{"typical session id", "/sync/steal", syncStealValidSessionID},
		{"minimal ^ses prefix", "/sync/steal", "ses"},
		{"distinct session id", "/sync/steal", "ses_9f8e7d6c5b4a"},
		{"valued declared params", "/sync/steal?directory=/tmp/ws&workspace=ws1", syncStealValidSessionID},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, syncStealBody(tc.sessionID))
		if id := decodeSyncStealResponse(t, tc.path, status, header, body); id != tc.sessionID {
			t.Errorf("%s: sessionID = %q, want %q (the session the caller asked to steal)", tc.name, id, tc.sessionID)
		}
	}
}

// TestSyncStealAbsentBodyIsBadRequest pins the required-input rule: the
// document does not mark the operation's requestBody required, but sessionID is
// its only defined field and the declared 200 schema requires a ^ses value — an
// absent (or empty-object) body names no session, so it answers the declared
// 400 rather than a 200 the shim could only reach by minting an id it was never
// given. This follows parseProviderOAuthAuthorizeBody, the shim's existing
// refusal for the identical optional-requestBody/required-field shape.
func TestSyncStealAbsentBodyIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ name, path, body string }{
		{"absent body", "/sync/steal", ""},
		{"empty object body", "/sync/steal", "{}"},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s (%q): got %d, want 400. Body: %s", tc.name, tc.path, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertInvalidRequest(t, tc.path, tc.body, status, body)
	}
}

// TestSyncStealMalformedInputIsBadRequest pins the declared 400 arm: input that
// violates the declared schema must answer the sibling INVALID_REQUEST envelope,
// not 200 and not the pre-fix 404. Covered classes: a body that is not JSON,
// not an object (array/scalar/null), carries an unknown top-level key or trails
// a second value; a sessionID that is missing, non-string, null, blank or does
// not match the declared pattern ^ses; and a present-but-blank declared query
// param (directory, workspace — the sibling handleSkill/handleSyncStart
// convention).
func TestSyncStealMalformedInputIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	valid := syncStealBody(syncStealValidSessionID)
	for _, tc := range []struct {
		name, path, body string
	}{
		{"not json", "/sync/steal", `not json`},
		{"json array body", "/sync/steal", `[1,2]`},
		{"json scalar body", "/sync/steal", `7`},
		{"json null body", "/sync/steal", `null`},
		{"missing sessionID", "/sync/steal", `{}`},
		{"sessionID not a string", "/sync/steal", `{"sessionID":5}`},
		{"sessionID is null", "/sync/steal", `{"sessionID":null}`},
		{"blank sessionID", "/sync/steal", `{"sessionID":"   "}`},
		{"empty sessionID", "/sync/steal", `{"sessionID":""}`},
		{"sessionID misses pattern", "/sync/steal", `{"sessionID":"msg_1"}`},
		{"unknown top-level key", "/sync/steal", `{"sessionID":"ses_a","extra":1}`},
		{"trailing second value", "/sync/steal", valid + `{"x":1}`},
		{"blank directory param", "/sync/steal?directory=", valid},
		{"blank workspace param", "/sync/steal?workspace=", valid},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s (%q): got %d, want 400. Body: %s", tc.name, tc.path, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertInvalidRequest(t, tc.path, tc.body, status, body)
	}
}

// TestSyncStealMethodGuard pins the non-POST behaviour: /sync/steal is a
// registered route, so a non-POST request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestSyncStealMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	valid := syncStealBody(syncStealValidSessionID)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/sync/steal", valid)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /sync/steal: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestSyncStealChiMount is the BUG-009 regression for the new route:
// /sync/steal must be listed in MountPatterns so a parent chi router mount
// reaches the shim (a shim-produced 200 proves the wiring; chi's 404 would mean
// the mount lost the route). MountPatterns gains the exact path, never a
// /sync/* catch-all: a deeper sub-path stays chi's 404.
func TestSyncStealChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	valid := syncStealBody(syncStealValidSessionID)
	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", valid)
	if id := decodeSyncStealResponse(t, "/sync/steal via chi mount", status, header, body); id != syncStealValidSessionID {
		t.Errorf("chi-mounted /sync/steal: sessionID = %q, want %q", id, syncStealValidSessionID)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal/sub", valid)
	if status != http.StatusNotFound {
		t.Errorf("POST /sync/steal/sub via chi mount: got %d, want 404 (no /sync/* catch-all). Body: %s", status, body)
	}
}
