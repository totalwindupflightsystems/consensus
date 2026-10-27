package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ============================================================================
// ROUTE-ADD-040 focused contract tests: GET /api/session/{sessionID}
// (v2.session.get, declared responses 200, 400, 401, 404; GET the only
// declared method).
//
// Upstream contract (openapi-1.18.33.json paths."/api/session/{sessionID}".get):
// the 200 envelope requires `data` (additionalProperties:false), and the 404
// is the NamedError shape {name: "NotFoundError", data: {message: "Session
// not found"}} — the exact body the pinned upstream sdk-error-shape suite
// extracts. Every assertion below pins the member set or the exact body shape.
// ============================================================================

// TestApiSessionGetServesDeclared200 pins the declared success arm: a known
// session answers 200 with the {data: {...}} envelope carrying the
// runtime-native members the sessions row holds — the same member set
// v2.session.create (ROUTE-ADD-038) answers with, so create-then-get sees
// the same names for the same row.
func TestApiSessionGetServesDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{
		"id":         "11111111-2222-3333-4444-555555555555",
		"agent_name": "research-agent",
		"goal":       "summarize the pinned suites",
		"status":     "idle",
	})})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/session/11111111-2222-3333-4444-555555555555")
	if status != http.StatusOK {
		t.Fatalf("GET /api/session/{id}: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("200 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "data")
	data, _ := got["data"].(map[string]any)
	assertExactMembers(t, data, "id", "agent_name", "goal", "status")
	if data["id"] != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("data.id = %v, want the addressed session's runtime id", data["id"])
	}
	if data["status"] != "idle" {
		t.Errorf("data.status = %v, want the row's real status", data["status"])
	}
}

// TestApiSessionGetServesDeclared400 pins the declared InvalidRequestError
// arm: a present-but-blank sessionID segment (%20) answers the v2
// InvalidRequestError envelope naming the field, and a sessions store this
// instance cannot read answers the same declared arm (absence is never
// claimed from an unreadable store; the operation declares no 5xx).
func TestApiSessionGetServesDeclared400(t *testing.T) {
	t.Run("blank sessionID", func(t *testing.T) {
		_, srv := newTestServer(&mockDB{})
		defer srv.Close()

		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/%20")
		if status != http.StatusBadRequest {
			t.Fatalf("GET /api/session/%%20: got %d, want 400. Body: %s", status, body)
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
	})

	t.Run("unreadable sessions store", func(t *testing.T) {
		_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: disk I/O error")})
		defer srv.Close()

		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_abc")
		if status != http.StatusBadRequest {
			t.Fatalf("GET /api/session/ses_abc with unreadable store: got %d, want 400. Body: %s", status, body)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("400 body is not JSON: %v (%s)", err, body)
		}
		if got["_tag"] != "InvalidRequestError" {
			t.Errorf("_tag = %v, want InvalidRequestError (body %v)", got["_tag"], got)
		}
	})

	t.Run("no-rows reply is the declared 404, not a store failure", func(t *testing.T) {
		// internal/db reports a missing row as its own error value ("sqlite:
		// no rows in result" — there is no sql.ErrNoRows sentinel on this
		// seam). That reply means the store READ FINE and holds no such
		// session, so it is the declared NotFoundError 404, never the 400 the
		// pre-merge branch handler answered (the defect the INT-BOARD-1 live
		// probe caught: every unknown session answered the InvalidRequestError
		// 400 and the declared 404 arm was unreachable on a live server).
		_, srv := newTestServer(&mockDB{queryRowErr: errors.New("sqlite: no rows in result")})
		defer srv.Close()

		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_abc")
		if status != http.StatusNotFound {
			t.Fatalf("GET /api/session/ses_abc with a no-rows reply: got %d, want 404. Body: %s", status, body)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("404 body is not JSON: %v (%s)", err, body)
		}
		assertNotFoundNamedErrorBody(t, "GET /api/session/ses_abc (no rows)", status, got)
	})
}

// TestApiSessionGetServesDeclared401 pins the declared UnauthorizedError arm
// ON THE LIVE MIDDLEWARE: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's v1
// UNAUTHENTICATED body, which an unregistered /api/* path keeps. This is the
// arm the pattern-literal v2EnvelopeRoutePaths entry could not serve (the map
// is keyed by concrete request paths); the one-segment isV2EnvelopeRoutePath
// rule serves it.
func TestApiSessionGetServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_abc")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/session/ses_abc: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError (the declared 401 body)", got["_tag"])
	}

	// Membership stays one-segment: deeper /api/session/* paths keep the
	// shim's own envelope (they are not this operation).
	if isV2EnvelopeRoutePath("/api/session/a/b") {
		t.Error("/api/session/a/b must not be treated as a v2-envelope route; the rule admits one segment only")
	}
	if !isV2EnvelopeRoutePath("/api/session/ses_abc") {
		t.Error("/api/session/ses_abc must be treated as a v2-envelope route (the declared v2.session.get 401 arm)")
	}
}

// TestApiSessionGetServesDeclared404 pins the declared NotFoundError arm on
// the exact upstream body shape the pinned sdk-error-shape suite extracts:
// 404 with {name: "NotFoundError", data: {message: "Session not found"}} and
// no retired {"error": ...} envelope key.
func TestApiSessionGetServesDeclared404(t *testing.T) {
	_, srv := newTestServer(&mockDB{}) // QueryRow returns (nil, nil): no row
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_no_such")
	if status != http.StatusNotFound {
		t.Fatalf("GET /api/session/ses_no_such: got %d, want 404. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("404 body is not JSON: %v (%s)", err, body)
	}
	assertNotFoundNamedErrorBody(t, "GET /api/session/ses_no_such", status, got)
}

// TestApiSessionGetUndeclaredMethod pins the method contract: GET is the only
// method the pinned document declares on this path, so every other method
// answers METHOD_NOT_ALLOWED 405 (never a success payload), and no sibling
// operation is reclassified by this registration.
func TestApiSessionGetUndeclaredMethod(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{
		"id": "ses123", "agent_name": "a", "goal": "g", "status": "idle",
	})})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodPut} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/session/ses123")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/session/ses123: got %d, want 405 (GET is the only declared method). Body: %s",
				method, status, body)
		}
	}
}

// TestApiSessionGetMountWiring pins the chi mount contract: the declared
// parameter shape must be listed in MountPatterns (cmd/consensus/main.go
// mounts the shim by these patterns — without the entry a parent chi router
// answers before the shim sees the request), the route must be reachable
// through that mount, and no /api/session/* catch-all may be introduced (the
// deeper declared /api/session/{sessionID}/... operations keep their own
// answers; /api/session/active keeps its own route).
func TestApiSessionGetMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/session/{sessionID}"] {
		t.Errorf("/api/session/{sessionID} is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-040)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// The mount reaches the shim route: this fixture's mock has no session
	// row, so the handler answers its own typed 404 NotFoundError — what
	// matters is the request reached the handler, not a default 404 page.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/ses_abc")
	if status != http.StatusNotFound || !strings.Contains(string(body), "NotFoundError") {
		t.Fatalf("chi-mounted GET /api/session/ses_abc: got %d (%s) — the mount must reach the shim handler, whose unknown-session answer is the typed NotFoundError", status, body)
	}

	// No catch-all: a deeper path keeps net/http's default 404, and the
	// exact /api/session/active route keeps its own declared 200 contract.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/session/a/b")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/session/a/b: got %d, want 404 — ROUTE-ADD-040 registers the one-segment shape, never a catch-all", status)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusOK {
		t.Errorf("GET /api/session/active: got %d, want 200 (its own route, ROUTE-ADD-039). Body: %s", status, body)
	}
}

// TestApiSessionGetTraceRow pins the ch:trace declaration of the handler:
// it names this row, its own operationId and this test file, so the
// traceability link cannot silently drift from the served surface.
func TestApiSessionGetTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_session_get.go")
	if err != nil {
		t.Skipf("api_session_get.go not readable from the test working directory: %v", err)
	}
	for _, want := range []string{
		"ch:trace row=ROUTE-ADD-040",
		"spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.get",
		"test=internal/shim/opencode/api_session_get_test.go",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("api_session_get.go is missing the traceability fragment %q", want)
		}
	}
}
