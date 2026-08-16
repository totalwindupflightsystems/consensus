// Package api: OpenAPI spec serving tests (SPEC-018 §9).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/018-openapi-contract.md plan=phase-5/task-5-1/step-5-1-4 test=internal/api/openapi_test.go
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ============================================================================
// OpenAPI Serving Tests
// ============================================================================

func TestOpenAPIYAMLEndpoint(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to GET /openapi.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "yaml") {
		t.Errorf("expected YAML content type, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !strings.Contains(string(body), "openapi:") {
		t.Error("expected YAML body to contain the openapi version key")
	}
}

func TestOpenAPIJSONEndpoint(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("failed to GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content type, got %q", ct)
	}

	// Parse response
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Verify basic structure
	if doc["openapi"] == nil {
		t.Error("missing openapi version")
	}
	if doc["info"] == nil {
		t.Error("missing info section")
	}
	if doc["paths"] == nil {
		t.Error("missing paths section")
	}
}

// TestOpenAPISpecServedFromEmbeddedSpec is the DOGFOOD-103 regression test:
// the spec must be served from the copy embedded into the binary when the
// process CWD is NOT the repository root (e.g. `consensus serve` launched
// from anywhere, or the Docker image, which does not copy specs/).
func TestOpenAPISpecServedFromEmbeddedSpec(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get CWD: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWD); err != nil {
			t.Errorf("failed to restore CWD: %v", err)
		}
	}()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// YAML endpoint
	resp, err := http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to GET /openapi.yaml: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for /openapi.yaml from non-repo CWD, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "openapi:") {
		t.Error("expected embedded YAML body to contain the openapi version key")
	}

	// JSON endpoint
	resp, err = http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("failed to GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for /openapi.json from non-repo CWD, got %d", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if doc["openapi"] == nil || doc["paths"] == nil {
		t.Error("expected embedded JSON spec to have openapi version and paths")
	}
}

func TestDocAPISwaggerUIEndpoint(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// The REST API Swagger UI lives at /doc/api (DOGFOOD-103).
	resp, err := http.Get(srv.URL + "/doc/api")
	if err != nil {
		t.Fatalf("failed to GET /doc/api: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "html") {
		t.Errorf("expected HTML content type, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !strings.Contains(string(body), "/openapi.yaml") {
		t.Error("expected Swagger UI page to reference /openapi.yaml")
	}

	// The servers URL must be derived from the request Host, not a
	// hardcoded default port (DOGFOOD-103).
	host := strings.TrimPrefix(srv.URL, "http://")
	if !strings.Contains(string(body), "http://"+host) {
		t.Errorf("expected Swagger UI to derive servers URL from request Host %q", host)
	}
}

// TestBareDocNotServedByAPI verifies the REST API surface no longer claims
// /doc — that path belongs to the opencode shim's Swagger UI in full
// deployments (DOGFOOD-103). On the API-only server it must 404.
func TestBareDocNotServedByAPI(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/doc")
	if err != nil {
		t.Fatalf("failed to GET /doc: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for /doc on API-only server, got %d", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("failed to GET /api/v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("failed to parse health response: %v", err)
	}

	if health.Status != "ok" {
		t.Errorf("expected status=healthy, got %q", health.Status)
	}
	if health.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestOpenAPICORSAccess(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to GET /openapi.yaml: %v", err)
	}
	defer resp.Body.Close()

	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %q", cors)
	}
}
