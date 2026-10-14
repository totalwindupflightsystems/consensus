package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/credential/{credentialID}".delete =
// v2.credential.remove, SHIM-DRIFT-001): DELETE /api/credential/{credentialID},
// responses 204 ("Credential removed"), 400 InvalidRequestError, 401
// UnauthorizedError. The pinned document declares credentialID as a plain
// required string (NO pattern) and the operation declares NO requestBody, so
// the declared 400 arm has no malformed input to attach to and the shim
// answers the declared 204 for every well-formed credentialID.
//
// TRUTHFULNESS (the same rule handleApiCommandList / handleGlobalDispose
// answer with): the Consensus runtime keeps NO v2 credential store — internal/db
// has no credentials table and no migration creates one — so the 204 is an
// idempotent removal of nothing, never a fabricated "stored credential was
// deleted". The 204 body is empty and the 401/405 bodies below are pinned
// property-by-property so a later change cannot silently alter the envelope.

// TestApiCredentialRemoveServesDeclared204 pins the declared success arm
// (ROUTE-ADD-007, SHIM-DRIFT-001): the previously un-routed path serves the
// declared operation with a 204 and an EMPTY body instead of the pre-fix
// net/http default 404 (class NOT-SERVED: no shim route at all). The httptest
// server built here is the live probe — no handler-direct call only.
func TestApiCredentialRemoveServesDeclared204(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, "/api/credential/crd_1")
	if status != http.StatusNoContent {
		t.Fatalf("DELETE /api/credential/crd_1: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 body = %q, want empty (declared 204 No Content)", body)
	}
	if ct := header.Get("Content-Type"); ct != "" {
		t.Errorf("204 Content-Type = %q, want none (no body was written)", ct)
	}
}

// TestApiCredentialRemoveIdempotent pins the honest no-op semantics: two
// removals of the same id both answer 204 — there is no store to hold a
// deleted entry, so a repeat cannot 404 or 400.
func TestApiCredentialRemoveIdempotent(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	s.workdir = t.TempDir()
	defer srv.Close()

	for i := 0; i < 2; i++ {
		status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/api/credential/crd_same")
		if status != http.StatusNoContent {
			t.Fatalf("repeat %d DELETE: got %d, want 204. Body: %s", i+1, status, body)
		}
		if len(body) != 0 {
			t.Errorf("repeat %d: 204 body = %q, want empty", i+1, body)
		}
	}
}

// TestApiCredentialRemoveMethodAndMountWiring pins the wiring and the 405 arm:
// the route is mounted (chi pattern + MountPatterns entry), undeclared methods
// answer the sibling METHOD_NOT_ALLOWED envelope (apiV2RequireGet convention),
// the declared sibling PATCH (v2.credential.update, its own contract row) keeps
// the pre-change net/http default 404, and unchanged siblings still answer.
func TestApiCredentialRemoveMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/credential/{credentialID}" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("/api/credential/{credentialID} is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-007)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Declared path: DELETE answers 204 through the chi mount (wiring proof).
	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/api/credential/crd_wired")
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted DELETE: got %d, want 204. Body: %s", status, body)
	}

	// Undeclared methods on the declared path answer 405 with the sibling
	// METHOD_NOT_ALLOWED envelope.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/credential/crd_wired")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/credential/crd_wired: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s: 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got["error"].(map[string]any)["code"] != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s: 405 body = %v, want METHOD_NOT_ALLOWED envelope", method, got)
		}
	}

	// The declared sibling PATCH operation (v2.credential.update,
	// SHIM-DRIFT-002) is deliberately unsupported: it answers the typed
	// not_implemented envelope (the pty.create precedent, SHIM-GAP-002) —
	// never a bare 405/404, which is what the artifact-consistency guard
	// below enforces — and its served-surface row records it as an explicit
	// intentional narrowing.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodPatch, "/api/credential/crd_wired")
	if status != http.StatusNotImplemented {
		t.Errorf("PATCH /api/credential/crd_wired (declared v2.credential.update): got %d, want 501 typed not_implemented (its own contract row is an explicit intentional narrowing)", status)
	}

	// Undeclared sub-paths and the bare path keep the net/http default 404 —
	// no catch-all was introduced beyond the declared parameter shape.
	for _, path := range []string{"/api/credential/", "/api/credential/crd_1/sub"} {
		status, _, _ = doShimRequest(t, srv.URL, http.MethodDelete, path)
		if status != http.StatusNotFound {
			t.Errorf("DELETE %s: got %d, want 404 (no declared operation on this path shape)", path, status)
		}
	}

	// Non-vacuity controls: the v2 siblings are untouched.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/agent")
	if status != http.StatusOK {
		t.Errorf("GET /api/agent (v2.agent.list): got %d, want 200 (unchanged sibling). Body: %s", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/command")
	if status != http.StatusOK {
		t.Errorf("GET /api/command (v2.command.list): got %d, want 200 (unchanged sibling). Body: %s", status, body)
	}
}

// TestApiCredentialRemoveServesDeclared401 verifies auth rejection uses the v2
// UnauthorizedError contract declared for this operation (the writeV2Unauthorized
// envelope, the exact-path v2EnvelopeRoutePaths convention), not the v1
// UNAUTHENTICATED envelope.
func TestApiCredentialRemoveServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, "/api/credential/crd_1")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE: got %d, want 401. Body: %s", status, body)
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
