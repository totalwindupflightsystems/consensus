package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// syncReplayValidEvent is one event carrying every field the pinned upstream
// document declares for sync.replay's events[] items (id matching ^evt_,
// aggregateID, an integer seq >= 0, type, object data — all required,
// additionalProperties: false).
const syncReplayValidEvent = `{"id":"evt_1","aggregateID":"ses_a","seq":0,"type":"message.created","data":{"k":"v"}}`

// syncReplayBody builds a declared-shape sync.replay request body from raw
// events[] items so each malformed-input arm can vary exactly one thing.
func syncReplayBody(events string) string {
	return fmt.Sprintf(`{"directory":"/tmp/ws","events":[%s]}`, events)
}

// decodeSyncReplayResponse decodes the declared 200 body — the object
// {sessionID: string} (required, additionalProperties: false) — and asserts
// its shape: exactly one key, named sessionID, holding a string.
func decodeSyncReplayResponse(t *testing.T, path string, status int, header http.Header, body []byte) string {
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
	return sessionID
}

// TestSyncReplayAnswersDeclaredSessionID answers upstream sync.replay
// (ROUTE-ADD-113, SHIM-DRIFT-127, declared responses: 200 ReplayedSyncEvents
// {sessionID}, 400 BadRequest | InvalidRequestError): POST /sync/replay must
// answer 200 with the declared object instead of the pre-fix net/http default
// 404. The Consensus runtime keeps no sync event store and has no replay
// engine, so the session id the shim can honestly report is the empty string
// — the replay analogue of the empty SyncEvent[] /sync/history answers, and
// never a fabricated session.
func TestSyncReplayAnswersDeclaredSessionID(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"one declared event", "/sync/replay", syncReplayBody(syncReplayValidEvent)},
		{"several declared events", "/sync/replay", syncReplayBody(
			syncReplayValidEvent + `,{"id":"evt_2","aggregateID":"ses_a","seq":1,"type":"message.updated","data":{}}`)},
		{"valued declared params", "/sync/replay?directory=/tmp/ws&workspace=ws1",
			syncReplayBody(syncReplayValidEvent)},
		{"empty data object", "/sync/replay", syncReplayBody(
			`{"id":"evt_9","aggregateID":"agg-1","seq":42,"type":"noop","data":{}}`)},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		sessionID := decodeSyncReplayResponse(t, tc.path, status, header, body)
		if sessionID != "" {
			t.Errorf("%s: sessionID = %q, want the empty string (the shim keeps no sync event store and replays nothing)", tc.name, sessionID)
		}
	}
}

// TestSyncReplayAbsentBodyIsWellFormed pins the optional-requestBody rule the
// document declares (no required flag on the operation's requestBody, the same
// shape handleSyncHistory accepts): a body-less POST is well-formed input and
// answers the declared 200 — never a 400 for a request the contract allows.
func TestSyncReplayAbsentBodyIsWellFormed(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/replay", "")
	sessionID := decodeSyncReplayResponse(t, "/sync/replay", status, header, body)
	if sessionID != "" {
		t.Errorf("sessionID = %q, want the empty string (nothing replayed)", sessionID)
	}
}

// TestSyncReplayMalformedInputIsBadRequest pins the declared 400 arm: input
// that violates the declared schema must answer the sibling INVALID_REQUEST
// envelope, not 200 and not the pre-fix 404. Covered classes: a body that is
// not JSON, not an object, carries an unknown top-level key or trails a second
// value; a missing/blank directory; a missing, non-array or empty events; an
// events[] item that is not an object or breaks any declared item rule (id
// pattern ^evt_, aggregateID, integer seq >= 0, type, object data,
// additionalProperties: false); and a present-but-blank declared query param
// (directory, workspace — the sibling handleSkill convention).
func TestSyncReplayMalformedInputIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"not json", "/sync/replay", `not json`},
		{"json array body", "/sync/replay", `[1,2]`},
		{"json null body", "/sync/replay", `null`},
		{"empty object body", "/sync/replay", `{}`},
		{"directory not a string", "/sync/replay", `{"directory":5,"events":[` + syncReplayValidEvent + `]}`},
		{"missing directory", "/sync/replay", `{"events":[` + syncReplayValidEvent + `]}`},
		{"blank directory", "/sync/replay", `{"directory":"  ","events":[` + syncReplayValidEvent + `]}`},
		{"unknown top-level key", "/sync/replay", `{"directory":"/tmp/ws","events":[` + syncReplayValidEvent + `],"extra":1}`},
		{"missing events", "/sync/replay", `{"directory":"/tmp/ws"}`},
		{"events not an array", "/sync/replay", `{"directory":"/tmp/ws","events":{}}`},
		{"events is a string", "/sync/replay", `{"directory":"/tmp/ws","events":"nope"}`},
		{"empty events array", "/sync/replay", `{"directory":"/tmp/ws","events":[]}`},
		{"event not an object", "/sync/replay", syncReplayBody(`7`)},
		{"event is null", "/sync/replay", syncReplayBody(`null`)},
		{"event unknown key", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":0,"type":"t","data":{},"extra":1}`)},
		{"event missing id", "/sync/replay", syncReplayBody(
			`{"aggregateID":"ses_a","seq":0,"type":"t","data":{}}`)},
		{"event id misses pattern", "/sync/replay", syncReplayBody(
			`{"id":"msg_1","aggregateID":"ses_a","seq":0,"type":"t","data":{}}`)},
		{"event missing aggregateID", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","seq":0,"type":"t","data":{}}`)},
		{"event missing seq", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","type":"t","data":{}}`)},
		{"event negative seq", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":-1,"type":"t","data":{}}`)},
		{"event fractional seq", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":1.5,"type":"t","data":{}}`)},
		{"event string seq", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":"3","type":"t","data":{}}`)},
		{"event missing type", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":0,"data":{}}`)},
		{"event missing data", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":0,"type":"t"}`)},
		{"event data null", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":0,"type":"t","data":null}`)},
		{"event data not an object", "/sync/replay", syncReplayBody(
			`{"id":"evt_1","aggregateID":"ses_a","seq":0,"type":"t","data":5}`)},
		{"trailing second value", "/sync/replay", syncReplayBody(syncReplayValidEvent) + `{"x":1}`},
		{"blank directory param", "/sync/replay?directory=", syncReplayBody(syncReplayValidEvent)},
		{"blank workspace param", "/sync/replay?workspace=", syncReplayBody(syncReplayValidEvent)},
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

// TestSyncReplayMethodGuard pins the non-POST behaviour: /sync/replay is a
// registered route, so a non-POST request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestSyncReplayMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/sync/replay", "{}")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /sync/replay: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestSyncReplayNeighboursUntouched is the non-vacuity control: serving
// /sync/replay must not disturb the neighbouring /sync/* family — /sync/steal
// is served too now (ROUTE-ADD-115) and keeps answering its declared 200 for a
// well-formed body — and there is no /sync/* catch-all: a deeper sub-path stays
// 404. The sibling /sync/history route keeps answering 200.
func TestSyncReplayNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", syncStealBody(syncStealValidSessionID))
	if id := decodeSyncStealResponse(t, "/sync/steal", status, header, body); id != syncStealValidSessionID {
		t.Errorf("POST /sync/steal: sessionID = %q, want %q (sibling route intact)", id, syncStealValidSessionID)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/replay/sub", "{}")
	if status != http.StatusNotFound {
		t.Errorf("POST /sync/replay/sub: got %d, want 404 (no /sync/* catch-all). Body: %s", status, body)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/history", "{}")
	if status != http.StatusOK {
		t.Errorf("POST /sync/history: got %d, want 200 (sibling route). Body: %s", status, body)
	}
}

// TestSyncReplayChiMount is the BUG-009 regression for the new route:
// /sync/replay must be listed in MountPatterns so a parent chi router mount
// reaches the shim (a shim-produced 200 proves the wiring; chi's 404 would
// mean the mount lost the route). /sync/steal gained its own exact pattern
// with ROUTE-ADD-115 and reaches the shim through the same mount —
// MountPatterns gains exact paths, never a /sync/* catch-all (/sync/start
// gained its own exact pattern with ROUTE-ADD-114).
func TestSyncReplayChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := syncReplayBody(syncReplayValidEvent)
	status, header, respBody := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/replay", body)
	sessionID := decodeSyncReplayResponse(t, "/sync/replay via chi mount", status, header, respBody)
	if sessionID != "" {
		t.Errorf("chi-mounted /sync/replay: sessionID = %q, want the empty string", sessionID)
	}

	status, stealHeader, stealBody := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", syncStealBody(syncStealValidSessionID))
	if id := decodeSyncStealResponse(t, "/sync/steal via chi mount", status, stealHeader, stealBody); id != syncStealValidSessionID {
		t.Errorf("chi-mounted /sync/steal: sessionID = %q, want %q", id, syncStealValidSessionID)
	}
}
