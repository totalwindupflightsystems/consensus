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

const apiIntegrationAttemptStatusPath = "/api/integration/attempt/att_oauth_1"

func newApiIntegrationRA016Server(t *testing.T, skipAuth bool) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = skipAuth
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// TestApiIntegrationAttemptStatusServesDeclared200 pins the route that was
// previously absent: a well-formed GET answers the declared {location, data}
// envelope. The runtime keeps no OAuth-attempt store, so data is an empty,
// non-null collection and never a fabricated attempt row.
func TestApiIntegrationAttemptStatusServesDeclared200(t *testing.T) {
	s, srv := newApiIntegrationRA016Server(t, true)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, apiIntegrationAttemptStatusPath)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", apiIntegrationAttemptStatusPath, status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("200 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiV2ListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("200 body is not the declared {location, data} envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != s.workdir {
		t.Errorf("location.directory = %q, want the selected workspace %q", got.Location.Directory, s.workdir)
	}
	if got.Location.Project.ID != instanceID(s.workdir) || got.Location.Project.Directory != s.workdir {
		t.Errorf("location.project = %+v, want the real selected workspace identity", got.Location.Project)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %s, want an empty non-null array: the runtime keeps no OAuth-attempt store", body)
	}
}

// TestApiIntegrationAttemptStatusMalformedInput pins the declared
// InvalidRequestError 400 for the malformed path/query shapes the declared
// request admits: a blank attemptID and malformed deepObject location selector.
func TestApiIntegrationAttemptStatusMalformedInput(t *testing.T) {
	_, srv := newApiIntegrationRA016Server(t, true)

	for _, tc := range []struct {
		name  string
		path  string
		field string
	}{
		{name: "blank attemptID", path: "/api/integration/attempt/%20", field: "attemptID"},
		{name: "blank location directory", path: apiIntegrationAttemptStatusPath + "?location%5Bdirectory%5D=", field: "location[directory]"},
		{name: "unknown location field", path: apiIntegrationAttemptStatusPath + "?location%5Btoken%5D=x", field: "location[token]"},
		{name: "non-wrk workspace", path: apiIntegrationAttemptStatusPath + "?location%5Bworkspace%5D=ses_1", field: "location[workspace]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want declared 400. Body: %s", tc.path, status, body)
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
		})
	}
}

// TestApiIntegrationAttemptStatusMethodNotAllowed pins the sibling
// METHOD_NOT_ALLOWED envelope for every method outside the merged tree's
// served set on the exact declared path: GET serves v2.integration.attempt.status
// (this handler) and DELETE serves v2.integration.attempt.cancel (the merged
// ROUTE-ADD-015 sibling), so both are excluded from the 405 arm.
func TestApiIntegrationAttemptStatusMethodNotAllowed(t *testing.T) {
	_, srv := newApiIntegrationRA016Server(t, true)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method, apiIntegrationAttemptStatusPath)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405. Body: %s", method, apiIntegrationAttemptStatusPath, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		errorBody, _ := got["error"].(map[string]any)
		if errorBody["code"] != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s 405 error.code = %v, want METHOD_NOT_ALLOWED", method, errorBody["code"])
		}
	}
}

// TestApiIntegrationAttemptStatusUnauthorized pins the declared v2 401 body.
func TestApiIntegrationAttemptStatusUnauthorized(t *testing.T) {
	_, srv := newApiIntegrationRA016Server(t, false)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, apiIntegrationAttemptStatusPath)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET: got %d, want 401. Body: %s", status, body)
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
		t.Errorf("_tag = %v, want UnauthorizedError", got["_tag"])
	}
}

// TestApiIntegrationAttemptStatusMountWiring pins both router layers: the chi
// parent receives the exact parameter pattern, the shim ServeMux receives the
// request, and no /api/* catch-all is introduced.
func TestApiIntegrationAttemptStatusMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	const mountPattern = "/api/integration/attempt/{attemptID}"
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == mountPattern {
			mounted = true
			break
		}
	}
	if !mounted {
		t.Fatalf("%s is not registered in MountPatterns — a parent chi mount would 404 ROUTE-ADD-016", mountPattern)
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, apiIntegrationAttemptStatusPath)
	if status != http.StatusOK {
		t.Errorf("chi-mounted GET: got %d, want 200. Body: %s", status, body)
	}
	for _, path := range []string{"/api/integration/attempt", apiIntegrationAttemptStatusPath + "/extra", "/api/integration/bogus"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status == http.StatusOK {
			t.Errorf("GET %s: got 200, want 404 — registration must remain exact", path)
		}
	}
	if !isV2EnvelopeRoutePath(apiIntegrationAttemptStatusPath) {
		t.Error("the declared attempt-status path is missing from the v2 UnauthorizedError envelope set")
	}
	if isV2EnvelopeRoutePath(apiIntegrationAttemptStatusPath + "/extra") {
		t.Error("a deeper undeclared integration path leaked into the exact v2-envelope membership")
	}
}

func TestApiIntegrationAttemptStatusTraceRow(t *testing.T) {
	data, err := os.ReadFile("api_integration_ra016.go")
	if err != nil {
		t.Skipf("api_integration_ra016.go not readable from package directory: %v", err)
	}
	want := "row=ROUTE-ADD-016 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.status test=internal/shim/opencode/api_integration_ra016_test.go witness=none:unattended-worker-session"
	if !strings.Contains(string(data), want) {
		t.Errorf("api_integration_ra016.go carries no ch:trace line (want %q)", want)
	}
}
