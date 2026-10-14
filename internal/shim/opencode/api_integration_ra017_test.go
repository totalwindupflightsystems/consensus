package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const apiIntegrationCompleteTestPath = "/api/integration/attempt/attempt-123/complete"

// Upstream contract (openapi-1.18.33.json
// paths."/api/integration/attempt/{attemptID}/complete".post =
// v2.integration.attempt.complete, SHIM-DRIFT-004): POST with a required
// request body object whose only optional property is string `code`, the same
// optional deepObject location selector used by v2.integration.list, and
// responses 204 No Content, 400 InvalidRequestError, 401 UnauthorizedError.
// Before ROUTE-ADD-017 the path had no shim route and answered net/http's
// default 404.

// TestApiIntegrationCompleteServesDeclared204 pins the declared success arm:
// both legal request-body shapes answer 204 with no response body. The runtime
// keeps no integration-attempt or integration-credential store, so completion
// is an idempotent no-op; no attempt or credential row is fabricated.
func TestApiIntegrationCompleteServesDeclared204(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "code omitted", body: `{}`},
		{name: "code supplied", body: `{"code":"oauth-code"}`},
		{name: "empty code is a string", body: `{"code":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationCompleteTestPath, tc.body)
			if status != http.StatusNoContent {
				t.Fatalf("POST %s: got %d, want 204. Body: %s", apiIntegrationCompleteTestPath, status, body)
			}
			if len(body) != 0 {
				t.Errorf("204 body = %q, want empty", body)
			}
			if contentType := header.Get("Content-Type"); contentType != "" {
				t.Errorf("204 Content-Type = %q, want none", contentType)
			}
		})
	}
}

// TestApiIntegrationCompleteDeclared400 pins every malformed part of the
// declared request: the body is required and must be one JSON object carrying
// only an optional string code; location is the declared deepObject selector.
func TestApiIntegrationCompleteDeclared400(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct {
		name  string
		path  string
		body  string
		field string
	}{
		{name: "missing body", path: apiIntegrationCompleteTestPath, body: "", field: "body"},
		{name: "null body", path: apiIntegrationCompleteTestPath, body: "null", field: "body"},
		{name: "array body", path: apiIntegrationCompleteTestPath, body: "[]", field: "body"},
		{name: "malformed body", path: apiIntegrationCompleteTestPath, body: "{", field: "body"},
		{name: "trailing value", path: apiIntegrationCompleteTestPath, body: `{} {}`, field: "body"},
		{name: "unknown body field", path: apiIntegrationCompleteTestPath, body: `{"bogus":true}`, field: "bogus"},
		{name: "code is not string", path: apiIntegrationCompleteTestPath, body: `{"code":7}`, field: "code"},
		{name: "code is null", path: apiIntegrationCompleteTestPath, body: `{"code":null}`, field: "code"},
		{name: "blank location directory", path: apiIntegrationCompleteTestPath + "?location%5Bdirectory%5D=", body: `{}`, field: "location[directory]"},
		{name: "unknown location field", path: apiIntegrationCompleteTestPath + "?location%5Bbogus%5D=x", body: `{}`, field: "location[bogus]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s with %q: got %d, want 400. Body: %s", tc.path, tc.body, status, body)
			}
			if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
			}
			if got["message"] == "" {
				t.Error("400 body carries no message")
			}
			if got["field"] != tc.field {
				t.Errorf("field = %v, want %q", got["field"], tc.field)
			}
		})
	}
}

// TestApiIntegrationCompleteServesDeclared401 drives the real auth wall and
// pins the declared v2 UnauthorizedError envelope.
func TestApiIntegrationCompleteServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationCompleteTestPath, `{}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST: got %d, want 401. Body: %s", status, body)
	}
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError", got["_tag"])
	}
}

// TestApiIntegrationCompleteMethodAndMountWiring pins the exact route shape,
// production chi mount, and sibling METHOD_NOT_ALLOWED envelope.
func TestApiIntegrationCompleteMethodAndMountWiring(t *testing.T) {
	const mountPattern = "/api/integration/attempt/{attemptID}/complete"
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == mountPattern {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("%s is not registered in MountPatterns", mountPattern)
	}
	if !isV2EnvelopeRoutePath(apiIntegrationCompleteTestPath) {
		t.Fatal("declared path is not registered for the v2 UnauthorizedError envelope")
	}
	for _, path := range []string{
		"/api/integration/attempt//complete",
		"/api/integration/attempt/attempt-123/complete/extra",
	} {
		if isV2EnvelopeRoutePath(path) {
			t.Errorf("undeclared path %q is treated as the declared v2 operation", path)
		}
	}
	// ROUTE-ADD-015+016 merged tree: the bare one-segment attempt path IS a
	// declared v2 operation now (v2.integration.attempt.status, ROUTE-ADD-016),
	// so it legitimately receives the v2 envelope.
	if !isV2EnvelopeRoutePath("/api/integration/attempt/attempt-123") {
		t.Error("declared one-segment attempt path lost its v2 envelope membership after the ROUTE-ADD-016 merge")
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationCompleteTestPath, `{}`)
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted POST: got %d, want 204. Body: %s", status, body)
	}

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequestBody(t, srv.URL, method, apiIntegrationCompleteTestPath, `{}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s: 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		if got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s: error.code = %q, want METHOD_NOT_ALLOWED", method, got.Error.Code)
		}
	}

	for _, path := range []string{
		"/api/integration/attempt/attempt-123/complete/extra",
		"/api/integration/bogus",
	} {
		status, _, _ := doShimRequestBody(t, srv.URL, http.MethodPost, path, `{}`)
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 (no /api/integration/* catch-all)", path, status)
		}
	}
	// ROUTE-ADD-015+016 merged tree: POST to the bare one-segment attempt path
	// is NOT declared — the merged dispatcher answers the sibling
	// METHOD_NOT_ALLOWED envelope (GET status / DELETE cancel are the declared
	// arms), which is still a truthful non-404 classification.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, "/api/integration/attempt/attempt-123", `{}`)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/integration/attempt/attempt-123: got %d, want 405 (merged dispatcher, no catch-all)", status)
	}
	_ = body
}

// TestApiIntegrationCompleteTraceRow pins the source-to-contract link.
func TestApiIntegrationCompleteTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_integration_ra017.go")
	if err != nil {
		t.Skipf("api_integration_ra017.go not readable: %v", err)
	}
	want := "row=ROUTE-ADD-017 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.complete test=internal/shim/opencode/api_integration_ra017_test.go witness=none:unattended-worker-session"
	if !strings.Contains(string(raw), want) {
		t.Errorf("api_integration_ra017.go carries no ch:trace line (want %q)", want)
	}
}
