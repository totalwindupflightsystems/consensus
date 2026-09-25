// Package api_test: machine check that every operation declared by the SERVED
// OpenAPI spec is registered in the runtime router (DF-CONSENSUS-18).
//
// This is an EXTERNAL test package on purpose: it mounts the opencode shim and
// the MCP handler onto the API router exactly the way cmd/consensus/main.go
// does, and internal/shim/opencode imports internal/api — an in-package
// (package api) test importing the shim would be an import cycle.
//
// For every path+method the served spec declares, an httptest round-trip
// against the full-deployment handler tree must NOT answer chi's
// "404 page not found" (unregistered route) or 405 (method not registered).
// Path items marked `x-not-implemented: true` in the spec source are allowed
// to answer anything (404/405/501 included): the published contract promises
// nothing for them until the implementing board row lands (MCP-DIRECT-001
// owns the /mcp discoverable surface; the five 501 stubs are declared stubs).
//
// Evidence baseline: docs/testing/evidence/tier-sweep-2026-09-24/
// tier-sweep-v2.json measured the pre-fix drift live (route-registration
// 59/60 with '/mcp' unregistered; the five stubs surfaced as 5xx noise).
package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/mcp"
	"github.com/wojons/consensus/internal/shim/opencode"
)

// emptyDB answers every query with no rows: enough for auth middleware to
// reject with 401, health to render, and id-based handlers to return their
// standard 404 error shape. Registration (not auth) is what this check
// verifies, so an unauthenticated 401/404-by-id is a pass.
type emptyDB struct{}

func (emptyDB) BeginTx(ctx context.Context) (db.Tx, error)            { return nil, nil }
func (emptyDB) Exec(ctx context.Context, q string, args ...any) error { return nil }
func (emptyDB) Query(ctx context.Context, q string, args ...any) ([]db.Row, error) {
	return nil, nil
}
func (emptyDB) QueryRow(ctx context.Context, q string, args ...any) (db.Row, error) {
	return nil, nil
}
func (emptyDB) Backend() db.Backend { return db.BackendSQLite }
func (emptyDB) Close() error        { return nil }

// newFullDeployServer wires the API server the way cmd/consensus/main.go does:
// MCP handler on /mcp/* and the opencode shim on each of its MountPatterns, so
// the machine check exercises the real production handler tree rather than the
// bare API router (36 of the 60 declared paths live on the shim surface).
func newFullDeployServer() http.Handler {
	apiSrv := api.NewServer(api.ServerConfig{DB: emptyDB{}, Addr: ":0"})
	mux := apiSrv.Handler().(chi.Router)

	mux.Handle("/mcp/*", mcp.NewServer(emptyDB{}).Handler())
	// MCP-DIRECT-001: the bare /mcp mount is part of the full deployment —
	// chi's "/mcp/*" wildcard does not match "/mcp" itself, so it needs its
	// own registration (mirrors cmd/consensus/main.go).
	mux.Handle("/mcp", mcp.NewServer(emptyDB{}).Handler())

	shimSrv := opencode.NewServer(emptyDB{}, "", nil, opencode.NewServiceAdapter(apiSrv.Service()))
	for _, pattern := range opencode.MountPatterns {
		mux.Handle(pattern, shimSrv.Handler())
	}
	return apiSrv.Handler()
}

// specHTTPMethods are the OpenAPI operation keys that map to HTTP verbs;
// every other key on a path item (parameters, x-* extensions, summary) is not
// a routable operation.
var specHTTPMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// instantiatePath replaces every {param} segment with a UUID-length value so
// chi URL params and shim sub-path switches both receive a plausible ID.
func instantiatePath(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			segs[i] = "00000000-0000-0000-0000-000000000000"
		}
	}
	return strings.Join(segs, "/")
}

// TestServedSpecPathsAreRegisteredAtRuntime is the DF-CONSENSUS-18 machine
// check: parse the spec the server actually serves (the embedded bundled
// copy, C-GAP-039) and round-trip every declared operation through the
// full-deployment handler tree. A declared operation must never surface the
// router's "404 page not found" or a 405 unless its path item is explicitly
// marked x-not-implemented in the spec source.
func TestServedSpecPathsAreRegisteredAtRuntime(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer())
	defer srv.Close()

	// Fetch the spec the way a client would — the embedded copy the server
	// actually serves, not the on-disk bundle.
	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /openapi.json: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("served spec has no paths object")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	checked, exempt := 0, 0
	for pattern, itemAny := range paths {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		notImplemented := false
		if v, ok := item["x-not-implemented"].(bool); ok && v {
			notImplemented = true
		}
		concrete := srv.URL + instantiatePath(pattern)

		for method := range item {
			if !specHTTPMethods[method] {
				continue
			}
			checked++
			if notImplemented {
				exempt++
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), concrete, nil)
			if err != nil {
				cancel()
				t.Fatalf("build request %s %s: %v", method, pattern, err)
			}
			resp, err := client.Do(req)
			cancel()
			if err != nil {
				t.Errorf("%s %s: request failed: %v", method, pattern, err)
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()

			if resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("spec declares %s %s but the runtime answers 405 — route not registered", method, pattern)
				continue
			}
			if resp.StatusCode == http.StatusNotFound && strings.Contains(string(body), "page not found") {
				t.Errorf("spec declares %s %s but the runtime answers 404 %q — route not registered",
					method, pattern, strings.TrimSpace(string(body)))
			}
		}
	}

	if checked == 0 {
		t.Fatal("no spec operations checked — served spec declares no HTTP methods")
	}
	t.Logf("checked %d declared operations; %d on x-not-implemented path items (any answer allowed)",
		checked, exempt)
}
