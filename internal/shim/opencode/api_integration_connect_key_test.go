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

const apiIntegrationConnectKeyTestPath = "/api/integration/github/connect/key"

// Upstream contract (openapi-1.18.33.json, ROUTE-ADD-019): POST
// /api/integration/{integrationID}/connect/key, operation
// v2.integration.connect.key, a required {key: string, label?: string}
// additionalProperties:false body, the optional deepObject location selector,
// and responses 204 No Content, 400 InvalidRequestError, 401 UnauthorizedError.
func TestApiIntegrationConnectKeyServesTruthfulDeclared204(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	selected := t.TempDir()
	path := apiIntegrationConnectKeyTestPath + "?location%5Bdirectory%5D=" + selected + "&location%5Bworkspace%5D=wrk_ra019"
	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, `{"key":"secret-material","label":"work"}`)
	if status != http.StatusNoContent {
		t.Fatalf("POST %s: got %d, want 204. Body: %s", path, status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 body = %q, want empty", body)
	}
	if contentType := header.Get("Content-Type"); contentType != "" {
		t.Errorf("204 Content-Type = %q, want none", contentType)
	}
}

func TestApiIntegrationConnectKeyServesDeclared400(t *testing.T) {
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
		{name: "missing key", path: apiIntegrationConnectKeyTestPath, body: `{}`, field: "key"},
		{name: "blank key", path: apiIntegrationConnectKeyTestPath, body: `{"key":"  "}`, field: "key"},
		{name: "non-json body", path: apiIntegrationConnectKeyTestPath, body: `not-json`, field: "body"},
		{name: "bad location selector", path: apiIntegrationConnectKeyTestPath + "?location%5Bbogus%5D=x", body: `{"key":"x"}`, field: "location[bogus]"},
		{name: "blank integration id", path: "/api/integration/%20/connect/key", body: `{"key":"x"}`, field: "integrationID"},
		{name: "unknown body field", path: apiIntegrationConnectKeyTestPath, body: `{"key":"x","bogus":true}`, field: "bogus"},
		{name: "label is not string", path: apiIntegrationConnectKeyTestPath, body: `{"key":"x","label":7}`, field: "label"},
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

func TestApiIntegrationConnectKeyServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationConnectKeyTestPath, `{"key":"x"}`)
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
	if got["_tag"] != "UnauthorizedError" || got["message"] == "" {
		t.Errorf("401 body = %v, want declared UnauthorizedError", got)
	}
}

func TestApiIntegrationConnectKeyMethodAndMountWiring(t *testing.T) {
	const mountPattern = "/api/integration/{integrationID}/connect/key"
	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == mountPattern {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("%s is not registered in MountPatterns", mountPattern)
	}

	serverSource, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	registration := `mux.HandleFunc("/api/integration/{integrationID}/connect/key", s.handleApiIntegrationConnectKey)`
	if !strings.Contains(string(serverSource), registration) {
		t.Fatalf("server.go does not register the exact route via mux.HandleFunc: want %s", registration)
	}

	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	req := httptest.NewRequest(http.MethodGet, apiIntegrationConnectKeyTestPath, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET %s: got %d, want 405. Body: %s", apiIntegrationConnectKeyTestPath, rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "METHOD_NOT_ALLOWED") {
		t.Errorf("GET 405 body %s does not carry METHOD_NOT_ALLOWED", rec.Body)
	}

	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, apiIntegrationConnectKeyTestPath, `{"key":"x"}`)
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted POST: got %d, want 204. Body: %s", status, body)
	}
}

func TestApiIntegrationConnectKeyTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_integration_connect_key.go")
	if err != nil {
		t.Fatalf("read api_integration_connect_key.go: %v", err)
	}
	want := "row=ROUTE-ADD-019 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.key test=internal/shim/opencode/api_integration_connect_key_test.go evidence=docs/evidence/ROUTE-ADD-019-live-probe.md witness=none:unattended-worker-session"
	if !strings.Contains(string(raw), want) {
		t.Errorf("api_integration_connect_key.go carries no ch:trace line (want %q)", want)
	}
}
