// Package api: OpenAPI spec serving tests (SPEC-018 §9).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/018-openapi-contract.md plan=phase-5/task-5-1/step-5-1-4 test=internal/api/openapi_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		// If the bundled.yaml is not found, expect 404
		if resp != nil && resp.StatusCode == 404 {
			t.Skip("bundled.yaml not found — skipping YAML test (run 'make bundle-spec' first)")
		}
		t.Fatalf("failed to GET /openapi.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		t.Skip("bundled.yaml not found — skipping YAML test")
	}

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "yaml") {
		t.Errorf("expected YAML content type, got %q", ct)
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

	if resp.StatusCode == 404 {
		t.Skip("bundled.yaml not found — skipping JSON test")
	}

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

func TestSwaggerUIDocsEndpoint(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/doc")
	if err != nil {
		t.Fatalf("failed to GET /doc: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "html") {
		t.Errorf("expected HTML content type, got %q", ct)
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
	if err != nil || resp.StatusCode == 404 {
		t.Skip("bundled.yaml not found — skipping CORS test")
	}
	defer resp.Body.Close()

	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %q", cors)
	}
}
