// Package api: OpenAPI spec serving tests (SPEC-018 §9).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/018-openapi-contract.md plan=phase-5/task-5-1/step-5-1-4 test=internal/api/openapi_test.go
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/wojons/consensus/specs"
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

// normalizeOpenAPIPath collapses path parameters ({id}, {sessionId}, {qID})
// and chi wildcards (*) to a fixed token so router patterns compare against
// spec paths regardless of the parameter name chosen on each side.
func normalizeOpenAPIPath(p string) string {
	re := regexp.MustCompile(`\{[^}]*\}|\*`)
	return re.ReplaceAllString(p, "{}")
}

// specPaths returns the set of path keys in a parsed OpenAPI document.
func specPaths(t *testing.T, doc map[string]any) map[string]bool {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec has no paths section")
	}
	out := make(map[string]bool, len(paths))
	for p := range paths {
		out[p] = true
	}
	return out
}

// embeddedSpecPaths returns the set of path keys in specs.BundledYAML.
func embeddedSpecPaths(t *testing.T) map[string]bool {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(specs.BundledYAML, &doc); err != nil {
		t.Fatalf("failed to parse embedded spec: %v", err)
	}
	return specPaths(t, doc)
}

// TestOpenAPISpecIgnoresDecoyOnDisk is the C-GAP-039 contract test: the
// served spec MUST come from the binary's embedded copy (specs.BundledYAML),
// never from an on-disk specs/openapi/*.yaml. A decoy bundle in the process
// CWD must not shadow the embedded repo spec — GET /openapi.yaml must be
// byte-equal to specs.BundledYAML and /openapi.json must expose exactly the
// embedded spec's paths.
func TestOpenAPISpecIgnoresDecoyOnDisk(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get CWD: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWD); err != nil {
			t.Errorf("failed to restore CWD: %v", err)
		}
	}()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	// Decoy spec on disk. Under the old disk-first resolution this would
	// shadow the embedded bundle; the decoy is a valid-looking OpenAPI doc
	// with a marker path so any leakage is unmistakable.
	decoy := []byte("openapi: \"3.1.0\"\ninfo:\n  title: DECOY\n  version: \"0.0.0\"\npaths:\n  /decoy/only:\n    get:\n      responses:\n        \"200\":\n          description: decoy\n")
	if err := os.MkdirAll("specs/openapi", 0o755); err != nil {
		t.Fatalf("failed to create decoy specs dir: %v", err)
	}
	for _, name := range []string{"bundled.yaml", "openapi.yaml"} {
		if err := os.WriteFile("specs/openapi/"+name, decoy, 0o644); err != nil {
			t.Fatalf("failed to write decoy %s: %v", name, err)
		}
	}

	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// YAML endpoint: byte-for-byte the embedded spec, not the decoy.
	resp, err := http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to GET /openapi.yaml: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read /openapi.yaml body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for /openapi.yaml, got %d", resp.StatusCode)
	}
	if !bytes.Equal(body, specs.BundledYAML) {
		t.Errorf("GET /openapi.yaml body != specs.BundledYAML (%d bytes vs %d); a decoy on disk was served instead of the embedded spec", len(body), len(specs.BundledYAML))
	}

	// JSON endpoint: paths must match the embedded spec's paths exactly.
	resp, err = http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("failed to GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	var served map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&served); err != nil {
		t.Fatalf("failed to parse /openapi.json: %v", err)
	}
	wantPaths := embeddedSpecPaths(t)
	gotPaths := specPaths(t, served)
	if len(gotPaths) != len(wantPaths) {
		t.Fatalf("served /openapi.json has %d paths, embedded spec has %d (decoy leaked?)", len(gotPaths), len(wantPaths))
	}
	for p := range wantPaths {
		if _, ok := gotPaths[p]; !ok {
			t.Errorf("served /openapi.json is missing embedded spec path %q", p)
		}
	}
}

// TestOpenAPIRoutesReconciledWithServedSpec is the C-GAP-039 contract test:
// every route registered on the API server's chi router (native API routes
// plus the four doc-serving routes, param-normalized) must exist in the spec
// served at /openapi.json, so the router and the served contract cannot
// drift.
//
// Reconciliation is contract-first: specs/openapi/ is the contract. A route
// that exists in code but is absent from the spec must be ADDED to
// specs/openapi/paths/* (then re-bundled and re-embedded), not deleted from
// the router to force this test green. Deliberate exclusions are documented
// here:
//
//   - /openapi.yaml, /openapi.json, /doc/api, /doc/api/* — the four
//     doc-serving routes (SPEC-018 §9) are the spec/UI serving surface
//     itself, not API operations; they are intentionally absent from the
//     spec's paths.
//   - /api/v1/events — SSE event stream (SPEC-015), not a REST operation;
//     intentionally excluded from the OpenAPI contract.
//   - /api/v1/quarantine, /api/v1/quarantine/{qID}/approve,
//     /api/v1/quarantine/{qID}/reject — cognitive firewall endpoints
//     (SPEC-005, optional QuarantineService); intentionally excluded until
//     they are documented in specs/openapi/paths/*.
//
// Path-count baseline: the served spec has 60 paths. Live walk 2026-08-31
// corrects the breakdown from refs/openapi-serving-disk-first.md's
// "31 native + 4 doc + 25 MCP/shim" to 24 native (/api/v1/*) + 0 doc-serving
// + 36 MCP/shim (opencode-shim surface, incl. /doc) — the doc-serving routes
// are the serving surface, not spec paths. Bump this snapshot deliberately
// when the contract grows.
func TestOpenAPIRoutesReconciledWithServedSpec(t *testing.T) {
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Fetch the spec the way a client would — exercises the real serving
	// path (embedded spec, not disk).
	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("failed to GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("failed to parse /openapi.json: %v", err)
	}
	servedPaths := specPaths(t, doc)

	// Snapshot: 60 paths total = 24 native + 0 doc-serving + 36 MCP/shim.
	if got := len(servedPaths); got != 60 {
		t.Fatalf("served spec has %d paths, want 60 (24 native + 0 doc + 36 MCP/shim)", got)
	}
	native, shim := 0, 0
	for p := range servedPaths {
		if strings.HasPrefix(p, "/api/v1/") {
			native++
		} else {
			shim++
		}
	}
	t.Logf("served spec path breakdown: %d native (/api/v1/*) + %d MCP/shim = %d", native, shim, native+shim)
	if native != 24 {
		t.Errorf("native (/api/v1/*) path count = %d, want 24 (corrected live-walk baseline)", native)
	}
	if shim != 36 {
		t.Errorf("MCP/shim path count = %d, want 36 (corrected live-walk baseline)", shim)
	}

	// Every chi route pattern (native + 4 doc routes) must exist in the
	// served spec, modulo the documented exclusions above.
	normalizedSpec := make(map[string]bool, len(servedPaths))
	for p := range servedPaths {
		normalizedSpec[normalizeOpenAPIPath(p)] = true
	}
	excluded := map[string]bool{
		"/openapi.yaml":                 true, // doc-serving surface (SPEC-018 §9)
		"/openapi.json":                 true, // doc-serving surface (SPEC-018 §9)
		"/doc/api":                      true, // doc-serving surface (SPEC-018 §9)
		"/doc/api/{}":                   true, // doc-serving surface (SPEC-018 §9)
		"/api/v1/events":                true, // SSE stream, not REST (SPEC-015)
		"/api/v1/quarantine":            true, // cognitive firewall (SPEC-005)
		"/api/v1/quarantine/{}/approve": true, // cognitive firewall (SPEC-005)
		"/api/v1/quarantine/{}/reject":  true, // cognitive firewall (SPEC-005)
	}

	walked := 0
	err = chi.Walk(s.router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		walked++
		norm := normalizeOpenAPIPath(route)
		if excluded[norm] {
			return nil
		}
		if !normalizedSpec[norm] {
			t.Errorf("route %s %s (normalized %q) is registered on the router but missing from the served OpenAPI spec; reconcile contract-first in specs/openapi/paths/* or document the exclusion", method, route, norm)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk failed: %v", err)
	}
	if walked < 30 {
		t.Fatalf("chi.Walk visited only %d routes; expected the full native + doc surface", walked)
	}
	t.Logf("reconciled %d chi routes (native + doc) against the served spec", walked)
}
