package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// decodeSyncStartResponse decodes the declared 200 body — a bare JSON boolean
// (schema {"type": "boolean"}, description "Workspace sync started") — and
// asserts its wire shape before returning the value.
func decodeSyncStartResponse(t *testing.T, path string, status int, header http.Header, body []byte) bool {
	t.Helper()
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST %s: Content-Type = %q, want application/json", path, ct)
	}
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want 200. Body: %s", path, status, body)
	}
	var got any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("POST %s body must be the declared JSON boolean: %v (%s)", path, err, body)
	}
	b, ok := got.(bool)
	if !ok {
		t.Fatalf("POST %s: body = %v (%T), want a JSON boolean (the declared 200 schema)", path, got, got)
	}
	return b
}

// TestSyncStartAnswersDeclaredBoolean answers upstream sync.start
// (ROUTE-ADD-114, SHIM-DRIFT-128, declared responses: 200 boolean, 400
// BadRequest): POST /sync/start must answer 200 with the declared boolean
// instead of the pre-fix net/http default 404. The Consensus runtime keeps no
// sync loop engine, so no workspace sync is started and the truthful value is
// false — never the fabricated true an effect-free no-op would claim (the
// sibling handleGlobalUpgrade truthfulness convention). The operation declares
// optional directory/workspace query params and no request body, so a valued
// param, an absent body and a body all answer the declared 200.
func TestSyncStartAnswersDeclaredBoolean(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"absent body", "/sync/start", ""},
		{"empty object body", "/sync/start", "{}"},
		{"valued declared params", "/sync/start?directory=/tmp/ws&workspace=ws1", "{}"},
		{"declared params without body", "/sync/start?directory=/tmp/ws", ""},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if started := decodeSyncStartResponse(t, tc.path, status, header, body); started {
			t.Errorf("%s: body = true, want false (no sync loop engine exists, so no workspace sync was started)", tc.name)
		}
	}
}

// TestSyncStartMethodGuard pins the non-POST behaviour: /sync/start is a
// registered route, so a non-POST request reaches the handler and answers the
// sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestSyncStartMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/sync/start", "{}")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /sync/start: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestSyncStartBlankQueryParamIsBadRequest pins the declared 400 arm: sync.start
// declares two optional query parameters (directory, workspace) and the shim
// answers the declared BadRequest code via the sibling writeOpencodeError
// INVALID_REQUEST envelope when either is present but blank — the sibling
// handleSkill/handleSyncHistory convention.
func TestSyncStartBlankQueryParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ name, path string }{
		{"blank directory param", "/sync/start?directory="},
		{"blank workspace param", "/sync/start?workspace="},
		{"blank directory param with valued sibling", "/sync/start?directory=&workspace=ws1"},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, "{}")
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s: got %d, want 400. Body: %s", tc.name, tc.path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertInvalidRequest(t, tc.path, "{}", status, body)
	}
}

// TestSyncStartNeighboursUntouched is the non-vacuity control: serving
// /sync/start must not disturb the neighbouring /sync/* family — /sync/steal
// stays unregistered (net/http default 404, the NOT-SERVED class it is still
// pinned in) — and there is no /sync/* catch-all: a deeper sub-path stays 404.
// The sibling /sync/history and /sync/replay routes keep answering 200.
func TestSyncStartNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", "{}")
	if status != http.StatusNotFound {
		t.Errorf("POST /sync/steal: got %d, want 404 (sibling /sync route must stay unregistered). Body: %s", status, body)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/start/sub", "{}")
	if status != http.StatusNotFound {
		t.Errorf("POST /sync/start/sub: got %d, want 404 (no /sync/* catch-all). Body: %s", status, body)
	}

	for _, tc := range []struct{ path, reqBody string }{
		{"/sync/history", "{}"},
		{"/sync/replay", syncReplayBody(syncReplayValidEvent)},
	} {
		status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.reqBody)
		if status != http.StatusOK {
			t.Errorf("POST %s: got %d, want 200 (sibling route). Body: %s", tc.path, status, body)
		}
	}
}

// TestSyncStartChiMount is the BUG-009 regression for the new route:
// /sync/start must be listed in MountPatterns so a parent chi router mount
// reaches the shim (a shim-produced 200 proves the wiring; chi's 404 would
// mean the mount lost the route). /sync/steal through the same mount stays
// chi's 404 — MountPatterns gains exact paths, never a /sync/* catch-all.
func TestSyncStartChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/start", "{}")
	if started := decodeSyncStartResponse(t, "/sync/start via chi mount", status, header, body); started {
		t.Errorf("chi-mounted /sync/start: body = true, want false (no workspace sync started)")
	}

	status, _, _ = doShimRequestBody(t, srv.URL, http.MethodPost, "/sync/steal", "{}")
	if status != http.StatusNotFound {
		t.Errorf("POST /sync/steal via chi mount: got %d, want 404 (unregistered sibling)", status)
	}
}
