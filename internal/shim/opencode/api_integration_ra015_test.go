package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json
// paths."/api/integration/attempt/{attemptID}".delete):
// DELETE /api/integration/attempt/{attemptID}, operationId
// v2.integration.attempt.cancel, optional deepObject location selector, and
// responses 204 No Content, 400 InvalidRequestError, 401 UnauthorizedError.
// ROUTE-ADD-015 serves that exact contract instead of the pre-fix net/http
// default 404 recorded as SHIM-DRIFT-002.

func newApiIntegrationRA015Server(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// TestApiIntegrationRA015ServesDeclared204 pins the declared success arm: the
// runtime keeps no integration OAuth-attempt store, so cancellation is an
// idempotent no-op with no fabricated row and an empty 204 body.
func TestApiIntegrationRA015ServesDeclared204(t *testing.T) {
	_, srv := newApiIntegrationRA015Server(t)

	for i := 0; i < 2; i++ {
		status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
			"/api/integration/attempt/attempt-1")
		if status != http.StatusNoContent {
			t.Fatalf("attempt %d: DELETE integration attempt: got %d, want 204. Body: %s", i+1, status, body)
		}
		if len(body) != 0 {
			t.Errorf("attempt %d: 204 body = %q, want empty", i+1, body)
		}
		if ct := header.Get("Content-Type"); ct != "" {
			t.Errorf("attempt %d: 204 Content-Type = %q, want none", i+1, ct)
		}
	}
}

// TestApiIntegrationRA015MalformedSelectorAnswersDeclared400 pins the declared
// InvalidRequestError arm for malformed members of the optional deepObject
// location selector. The handler shares apiV2LocationSelector with the other v2
// routes, so it must reject unknown fields, blank values, and an invalid
// workspace id with the upstream error shape.
func TestApiIntegrationRA015MalformedSelectorAnswersDeclared400(t *testing.T) {
	_, srv := newApiIntegrationRA015Server(t)

	for _, tc := range []struct {
		name  string
		query string
		field string
	}{
		{name: "unknown location field", query: "?location%5Bbogus%5D=x", field: "location[bogus]"},
		{name: "blank directory", query: "?location%5Bdirectory%5D=", field: "location[directory]"},
		{name: "blank workspace", query: "?location%5Bworkspace%5D=", field: "location[workspace]"},
		{name: "invalid workspace id", query: "?location%5Bworkspace%5D=workspace-1", field: "location[workspace]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
				"/api/integration/attempt/attempt-1"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("DELETE malformed selector: got %d, want 400. Body: %s", status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
			}
			if got["field"] != tc.field {
				t.Errorf("field = %v, want %q", got["field"], tc.field)
			}
			if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
				t.Errorf("message = %q, want a non-empty reason", msg)
			}
		})
	}
}

// TestApiIntegrationRA015MethodAndMountWiring pins the exact chi mount, the
// sibling METHOD_NOT_ALLOWED envelope for every non-DELETE method, and the lack
// of an /api/* catch-all. GET is the separate v2.integration.attempt.status
// operation and remains outside ROUTE-ADD-015, so this handler must not serve a
// fabricated status object for it.
func TestApiIntegrationRA015MethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/integration/attempt/{attemptID}" {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatal("/api/integration/attempt/{attemptID} is not registered in MountPatterns")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete,
		"/api/integration/attempt/attempt-wired")
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted DELETE: got %d, want 204. Body: %s", status, body)
	}

	// ROUTE-ADD-015+016 merged tree: GET on this path serves the declared
	// v2.integration.attempt.status (handleApiIntegrationRA016) — see the
	// sibling ra016 test; only methods outside {GET, DELETE} stay 405.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method,
			"/api/integration/attempt/attempt-wired")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s integration attempt: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" || strings.TrimSpace(got.Error.Message) == "" {
			t.Errorf("%s 405 body = %s, want METHOD_NOT_ALLOWED with message", method, body)
		}
	}

	for _, path := range []string{
		"/api/integration/attempt/attempt-wired/deeper",
		"/api/integration/other/attempt-wired",
	} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodDelete, path)
		if status != http.StatusNotFound {
			t.Errorf("DELETE %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}
}

// TestApiIntegrationRA015ServesDeclared401 pins the v2 UnauthorizedError
// envelope supplied by the existing /api/* authentication wall.
func TestApiIntegrationRA015ServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete,
		"/api/integration/attempt/attempt-1")
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
	if got["_tag"] != "UnauthorizedError" || strings.TrimSpace(toString(got["message"])) == "" {
		t.Errorf("401 body = %v, want UnauthorizedError with message", got)
	}
	if len(got) != 2 {
		t.Errorf("401 body = %v, want only declared _tag and message", got)
	}
}
