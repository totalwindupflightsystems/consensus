package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/health".get =
// v2.health.get, SHIM-DRIFT-008): GET /api/health, no parameters, responses
// 200 {healthy: boolean} ("Check whether the API server is ready to accept
// requests."), 400 InvalidRequestError, 401 UnauthorizedError.
//
// The declared 200 object requires `healthy` and sets additionalProperties:
// false with enum [true]; the operation is the HttpApi's process-readiness
// probe, so the truthful answer of a server that is routing the request is
// exactly {"healthy": true}. That pin is asserted below, including the "no
// extra member" half: the sibling /global/health route legitimately carries a
// `version` member, but v2.health.get's schema does not declare one, so adding
// it here would violate the declared contract.

type apiHealthResponse struct {
	Healthy bool `json:"healthy"`
}

// TestApiHealthServesDeclared200 pins v2.health.get (ROUTE-ADD-006,
// SHIM-DRIFT-008): the exact /api/health path serves the declared operation
// with the declared 200 shape instead of the pre-fix net/http default 404
// (class NOT-SERVED: no shim route at all). The httptest server built here is
// the live probe — no `go run` and no external process is involved.
func TestApiHealthServesDeclared200(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/health")
	if status != http.StatusOK {
		t.Fatalf("GET /api/health: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/health body is not the declared response object: %v (%s)", err, body)
	}
	if raw["healthy"] != true {
		t.Errorf("healthy = %v, want the declared enum value true", raw["healthy"])
	}
	if len(raw) != 1 {
		t.Errorf("GET /api/health body = %s, want only the declared required `healthy` property (additionalProperties: false)", body)
	}

	var got apiHealthResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared response object: %v (%s)", err, body)
	}
	if !got.Healthy {
		t.Errorf("healthy = false, want true (the declared enum is [true])")
	}
}

// TestApiHealthMethodAndMountWiring pins the method guard and the chi mount:
// /api/health must be listed in MountPatterns (cmd/consensus/main.go mounts
// the shim by these patterns — without the entry a parent chi router answers
// before the shim sees the request), non-GET keeps the sibling
// METHOD_NOT_ALLOWED envelope, /api/health/* sub-paths stay unclaimed (default
// 404, no catch-all), and the declared global.health sibling is untouched.
func TestApiHealthMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/health" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("/api/health is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-006)")
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/health", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/health: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
		}
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/health")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/health: got %d, want 200. Body: %s", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/health/bogus")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/health/bogus: got %d, want 404 (no catch-all for /api/health sub-paths)", status)
	}

	// Non-vacuity control: the declared global.health operation keeps its own
	// route and its own (version-carrying) body.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/global/health")
	if status != http.StatusOK {
		t.Fatalf("GET /global/health: got %d, want 200 (unchanged sibling). Body: %s", status, body)
	}
	var global map[string]any
	if err := json.Unmarshal(body, &global); err != nil {
		t.Fatalf("GET /global/health body is not JSON: %v (%s)", err, body)
	}
	if global["healthy"] != true {
		t.Errorf("GET /global/health healthy = %v, want true (unchanged)", global["healthy"])
	}
}

// TestApiHealthServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation (the /api/agent and
// /api/fs/find convention), not the v1 UNAUTHENTICATED envelope.
func TestApiHealthServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/health")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/health: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want UnauthorizedError with message", got)
	}
	if len(got) != 2 {
		t.Errorf("401 body = %v, want only declared _tag and message properties", got)
	}
}
