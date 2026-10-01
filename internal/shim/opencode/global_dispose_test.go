package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestGlobalDisposeAnswersDeclaredBoolean answers upstream global.dispose
// (ROUTE-ADD-091, SHIM-DRIFT-089, declared responses: 200 boolean, 400
// BadRequest): POST /global/dispose must return 200 with a JSON boolean —
// the shim keeps no per-instance registry to release, so the truthful
// payload is the upstream boolean true (never a string, never an error,
// never the pre-fix net/http default 404). The call is idempotent: a
// repeated POST stays 200/true, like the sibling boolean DELETE
// /auth/{providerID} (handleAuthDelete).
func TestGlobalDisposeAnswersDeclaredBoolean(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for i := 0; i < 2; i++ {
		status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/global/dispose")
		if status != http.StatusOK {
			t.Fatalf("POST /global/dispose (call %d): got %d, want 200. Body: %s", i+1, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("POST /global/dispose: Content-Type = %q, want application/json", ct)
		}
		var disposed bool
		if err := json.Unmarshal(body, &disposed); err != nil {
			t.Fatalf("POST /global/dispose body must be the declared JSON boolean, got %q: %v", body, err)
		}
		if !disposed {
			t.Errorf("POST /global/dispose (call %d): boolean body = false, want true (Global disposed)", i+1)
		}
	}
}

// TestGlobalDisposeMethodGuard pins the non-POST behaviour: /global/dispose is
// a registered route, so a non-POST request reaches the handler and answers
// the sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never 404 from the handler itself.
func TestGlobalDisposeMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, header, body := doShimRequest(t, srv.URL, method, "/global/dispose")
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("%s /global/dispose: got %d, want 405. Body: %s", method, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s /global/dispose: Content-Type = %q, want application/json", method, ct)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s /global/dispose: body not the error envelope: %v (%s)", method, err, body)
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s /global/dispose: error.code = %q, want METHOD_NOT_ALLOWED", method, got.Error.Code)
		}
	}
}

// TestGlobalDisposeNeighboursUntouched is the non-vacuity control: serving
// /global/dispose must not disturb the neighbouring /global/* family — the
// public health endpoint keeps answering 200 — and the registration is an
// exact pattern, not a /global/* catch-all, so an unregistered sub-path
// still 404s.
func TestGlobalDisposeNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/global/health")
	if status != http.StatusOK {
		t.Errorf("GET /global/health: got %d, want 200 (sibling untouched). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/global/dispose/sub")
	if status != http.StatusNotFound {
		t.Errorf("POST /global/dispose/sub: got %d, want 404 (no /global/* catch-all). Body: %s", status, body)
	}
}

// TestGlobalDisposeChiMount is the BUG-009 regression for the new route:
// /global/dispose must reach the shim through a parent chi router mount. The
// existing "/global/*" MountPattern already covers every /global/* path, so no
// new pattern is required — a shim-produced 200 proves the wiring, chi's 404
// would mean the mount lost the route.
func TestGlobalDisposeChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/global/dispose")
	if status != http.StatusOK {
		t.Fatalf("POST /global/dispose via chi mount: got %d, want 200 — /global/dispose must be reachable through the /global/* MountPattern. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted POST /global/dispose: Content-Type = %q, want application/json", ct)
	}
	var disposed bool
	if err := json.Unmarshal(body, &disposed); err != nil {
		t.Fatalf("chi-mounted POST /global/dispose body must be a JSON boolean: %v (%s)", err, body)
	}
	if !disposed {
		t.Errorf("chi-mounted POST /global/dispose = false, want true")
	}

	// The sibling health endpoint stays reachable through the same mount.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/global/health")
	if status != http.StatusOK {
		t.Errorf("GET /global/health via chi mount: got %d, want 200. Body: %s", status, body)
	}
}
