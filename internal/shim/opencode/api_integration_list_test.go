package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/integration".get,
// ROUTE-ADD-014 / SHIM-DRIFT-001): GET /api/integration, one optional
// deepObject `location` query selector, responses 200 {location: LocationInfo,
// data: IntegrationInfo[]}, 400 InvalidRequestError, 401 UnauthorizedError.
// IntegrationInfo requires id, name, methods, and connections. The runtime has
// no integration registry whose rows can truthfully provide that shape, so the
// declared 200 must carry an empty, non-null data array.

func TestApiIntegrationListServesDeclared200EmptyArray(t *testing.T) {
	s, srv, _ := newApiV2ListsServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration")
	if status != http.StatusOK {
		t.Fatalf("GET /api/integration: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiV2ListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared {location, data} envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != s.workdir {
		t.Errorf("location.directory = %q, want the real selected workspace %q", got.Location.Directory, s.workdir)
	}
	if got.Location.Project.ID != instanceID(s.workdir) {
		t.Errorf("location.project.id = %q, want instanceID(directory) = %q", got.Location.Project.ID, instanceID(s.workdir))
	}
	if got.Location.Project.Directory != got.Location.Directory {
		t.Errorf("location.project.directory = %q, want selected directory %q", got.Location.Project.Directory, got.Location.Directory)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %s, want an empty non-null array: the runtime has no IntegrationInfo-shaped store", body)
	}

	other := t.TempDir()
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet,
		"/api/integration?location%5Bdirectory%5D="+other+"&location%5Bworkspace%5D=wrk_integration")
	if status != http.StatusOK {
		t.Fatalf("GET /api/integration with location selectors: got %d, want 200. Body: %s", status, body)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("selector response is not the declared envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != other || got.Location.WorkspaceID != "wrk_integration" {
		t.Errorf("selected location = %+v, want directory %q and workspaceID wrk_integration", got.Location, other)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("selector data = %s, want an empty non-null array", body)
	}
}

func TestApiIntegrationListServesDeclared400ForMalformedLocation(t *testing.T) {
	s, srv, _ := newApiV2ListsServer(t)

	cases := []struct {
		name  string
		query string
		field string
	}{
		{name: "blank directory", query: "?location%5Bdirectory%5D=", field: "location[directory]"},
		{name: "blank workspace", query: "?location%5Bworkspace%5D=", field: "location[workspace]"},
		{name: "unknown field", query: "?location%5Bbogus%5D=x", field: "location[bogus]"},
		{name: "workspace without wrk prefix", query: "?location%5Bworkspace%5D=workspace-1", field: "location[workspace]"},
		{name: "unreadable directory", query: "?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), field: "location[directory]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/integration%s: got %d, want declared 400. Body: %s", tc.query, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			assertExactMembers(t, got, "_tag", "message", "kind", "field")
			if got["_tag"] != "InvalidRequestError" || got["kind"] != "Query" || got["field"] != tc.field {
				t.Errorf("400 body = %v, want InvalidRequestError kind Query naming %q", got, tc.field)
			}
		})
	}
}

func TestApiIntegrationListServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/integration: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message")
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want UnauthorizedError with message", got)
	}
}

func TestApiIntegrationListMethodAndMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/integration", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/integration: got %d, want 405. Body: %s", method, rec.Code, rec.Body)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Errorf("%s /api/integration 405 body is not JSON: %v (%s)", method, err, rec.Body)
		} else if got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s /api/integration error code = %q, want METHOD_NOT_ALLOWED. Body: %s", method, got.Error.Code, rec.Body)
		}
	}

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/integration" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("/api/integration is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-014)")
	}
	if !isV2EnvelopeRoutePath("/api/integration") {
		t.Error("/api/integration is not listed in v2EnvelopeRoutePaths — its declared 401 must use UnauthorizedError")
	}
	if isV2EnvelopeRoutePath("/api/integration/bogus") {
		t.Error("/api/integration/bogus is treated as a v2-envelope route; membership must be exact-path")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/integration")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/integration: got %d, want 200. Body: %s", status, body)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/integration/bogus")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/integration/bogus: got %d, want 404 (exact path, no /api/* catch-all)", status)
	}
}

func TestApiIntegrationListTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_integration_list.go")
	if err != nil {
		t.Fatalf("read api_integration_list.go: %v", err)
	}
	want := "row=ROUTE-ADD-014 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.list test=internal/shim/opencode/api_integration_list_test.go evidence=docs/evidence/ROUTE-ADD-014-live-probe.md witness=none:unattended-worker-session"
	if !strings.Contains(string(raw), want) {
		t.Errorf("api_integration_list.go carries no ch:trace line (want %q)", want)
	}
}
