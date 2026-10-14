package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths, ROUTE-ADD-009): the declared
// v2.event.subscribe operation had no shim route at all before this change and
// answered net/http's default 404 (the declared-vs-served artifact's
// NOT-SERVED class):
//
//	GET /api/event   v2.event.subscribe   200 Event stream (SSE of V2Event) | 400 InvalidRequestError | 401 UnauthorizedError
//
// The operation declares no parameters and no requestBody.
//
// Served arm (HONEST NARROWING, ROUTE-FIX-037 precedent): the declared 200 is
// NOT served — the runtime's only real event stream (v1 /event,
// handleGlobalEvent) emits v1-shaped events (message.created, tool.started,
// ...) with none of the V2Event members (^evt_ id, data.info: Message,
// durable aggregate envelope), and internal/db keeps no v2 session-event
// store, so no truthful 200 payload exists. Every well-formed subscribe —
// which is every subscribe, since there is no declared input to reject —
// answers the DECLARED 400 InvalidRequestError.

// TestApiEventSubscribeServesDeclared400Narrowing pins the narrowing arm: a
// well-formed subscribe answers 400 with the v2 InvalidRequestError envelope
// and a truthful reason naming the missing v2 event store. It runs against a
// real NewServer instance (httptest) with auth bypassed, exactly as production
// would route the request after the wall.
func TestApiEventSubscribeServesDeclared400Narrowing(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Short client timeout: if a regression ever turns this arm back into a
	// streaming 200, the request FAILS fast instead of hanging the suite.
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/api/event")
	if err != nil {
		t.Fatalf("GET /api/event: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /api/event: got %d, want 400 (the declared InvalidRequestError arm — the declared 200 is honestly narrowed, never fabricated)", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json (a narrowing must answer the declared JSON error, never SSE headers)", ct)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("400 body is not JSON: %v", err)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError (the declared 400 body)", got["_tag"])
	}
	msg, _ := got["message"].(string)
	for _, want := range []string{"v2.event.subscribe", "v2 session-event store"} {
		if !strings.Contains(msg, want) {
			t.Errorf("400 message %q does not name the truthful reason (%q)", msg, want)
		}
	}
}

// TestApiEventSubscribeUnauthenticatedFast401 pins the declared
// UnauthorizedError arm through the real auth wall: an unauthenticated request
// answers the v2 envelope quickly — never an SSE hang. The wave-recovery law
// (consensus-2026-09-26-00-13-38) makes this the load-bearing regression: a
// handler that wrote streaming headers before auth (or a route moved outside
// the /api/* wall) would turn this into an infinite stream, and the short
// client timeout FAILS the test instead of panicking the suite.
func TestApiEventSubscribeUnauthenticatedFast401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/api/event")
	if err != nil {
		t.Fatalf("unauthenticated GET /api/event must answer fast, got transport error (hang?): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/event: got %d, want 401", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("401 body is not JSON: %v", err)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError (the declared v2 401 envelope, via v2EnvelopeRoutePaths)", got["_tag"])
	}
}

// TestApiEventSubscribeMethodNotAllowed pins the sibling METHOD_NOT_ALLOWED
// envelope for every method outside the declared GET.
func TestApiEventSubscribeMethodNotAllowed(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/event")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/event: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s /api/event 405 body is not JSON: %v (%s)", method, err, body)
		}
	}
}

// TestApiEventSubscribeMountWiring pins the chi mount contract: the path is
// listed in MountPatterns (a parent chi mount reaches the shim route), the
// route is reachable through that mount, no /api/* catch-all was introduced,
// and the v2 401-envelope membership is exact-path.
func TestApiEventSubscribeMountWiring(t *testing.T) {
	if !isV2EnvelopeRoutePath("/api/event") {
		t.Error("/api/event is not listed in v2EnvelopeRoutePaths — its declared 401 body must be the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/event/bogus") {
		t.Error("/api/event/bogus is treated as a v2-envelope route; membership must be exact-path")
	}

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/event" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("/api/event is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-009)")
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(srv.URL + "/api/event")
	if err != nil {
		t.Fatalf("chi-mounted GET /api/event: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("chi-mounted GET /api/event: got %d, want 400 (the mount must reach the shim route)", resp.StatusCode)
	}

	// No catch-all: an unclaimed /api/event sub-path keeps the default 404.
	status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, "/api/event/bogus")
	if status == http.StatusOK {
		t.Error("GET /api/event/bogus: got 200, want 404 — ROUTE-ADD-009 registers the exact pattern, never an /api/* catch-all")
	}
}

// TestApiEventSubscribeTraceRow pins the ch:trace declaration in the handler
// source so the traceability link cannot silently drift from the served
// contract (same pattern as TestApiV2ListsTraceRows).
func TestApiEventSubscribeTraceRow(t *testing.T) {
	// The Go test binary runs in the package directory.
	raw, err := os.ReadFile("api_event_subscribe.go")
	if err != nil {
		t.Skipf("api_event_subscribe.go not readable from the test working directory: %v", err)
	}
	want := "row=ROUTE-ADD-009 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.event.subscribe test=internal/shim/opencode/api_event_subscribe_test.go witness=none:unattended-worker-session"
	if !strings.Contains(string(raw), want) {
		t.Errorf("api_event_subscribe.go carries no ch:trace line (want %q)", want)
	}
}
