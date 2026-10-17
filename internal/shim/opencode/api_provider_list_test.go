package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream contract (openapi-1.18.33.json paths."/api/provider".get =
// v2.provider.list, ROUTE-ADD-026): GET /api/provider, one optional deepObject
// `location` query selector, declared responses 200 {location: LocationInfo,
// data: ProviderV2Info[]} ("Retrieve active AI providers so clients can show
// provider availability and configuration."), 400 InvalidRequestError, 401
// UnauthorizedError, 503 ServiceUnavailableError. The path had no shim route at
// all before this change and answered net/http's default 404 (the
// declared-vs-served artifact's NOT-SERVED class).
//
// ProviderV2Info requires id, name, api and request; `api` is anyOf
// ProviderAISDK {type:"aisdk", package} | ProviderNative {type:"native",
// settings}, and `request` is {headers, body}. This runtime's provider
// evidence is model_registry.provider_id (migration 020 — the models.dev
// provider column the routing catalog carries), which names providers but
// carries no api descriptor and no request configuration. The declared 200
// therefore answers an empty, non-null data array (the api_v2_lists.go
// reshape-refusal precedent), and the catalog read decides the two arms: an
// unreadable catalog answers the declared 503 ServiceUnavailableError.
//
// Every 200 schema sets additionalProperties:false, so the tests assert the
// exact member set (an undeclared member is a contract violation, not an
// enhancement).

// apiProviderListResponse is the declared {location: LocationInfo, data:
// ProviderV2Info[]} envelope this operation answers with.
type apiProviderListResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data []json.RawMessage `json:"data"`
}

// newApiProviderListServer builds the ROUTE-ADD-026 fixture: a real SQLite
// database carrying the store the handler reads (model_registry with the
// migration-020 sync columns, including provider_id), auth bypassed, so the
// 200/400 arms are exercised against a real store rather than a mock.
func newApiProviderListServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "api-provider-list.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open api provider list test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0
		)`,
		// migration 001 §3.5 + 020 (model_registry sync source): the real
		// catalog the shim's /provider, /config/providers and /api/model
		// routes read, plus the provider_id column this handler reads.
		`CREATE TABLE model_registry (
			model_id TEXT PRIMARY KEY,
			tier INTEGER NOT NULL,
			max_context INTEGER NOT NULL,
			cost_per_m_in REAL NOT NULL,
			cost_per_m_out REAL NOT NULL,
			classifier_tags TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			sync_source TEXT NOT NULL DEFAULT 'static',
			synced_at TEXT,
			provider_id TEXT
		)`,
		// migration 001 §2.4: the credential store the sibling declared
		// operation v2.provider.get (ROUTE-ADD-027) reads as its second
		// provider-existence evidence. Present and empty here so this
		// fixture's sibling-route assertions exercise the handler's 404 arm
		// rather than an unavailable store.
		`CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TEXT
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare api provider list test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// TestApiProviderListServesDeclared200 pins v2.provider.list (ROUTE-ADD-026):
// the exact /api/provider path serves the declared operation with the declared
// 200 shape instead of the pre-fix net/http default 404 (class NOT-SERVED: no
// shim route at all).
func TestApiProviderListServesDeclared200(t *testing.T) {
	_, srv, _ := newApiProviderListServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider")
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/provider body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiProviderListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/provider body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Errorf("data = null, want the declared ProviderV2Info[] array (an empty array is not null): %s", body)
	}
	if len(got.Data) != 0 {
		t.Errorf("data = %s, want the empty array — no entry can be built without inventing the required api/request members", body)
	}
	if got.Location.Directory == "" {
		t.Errorf("location.directory is empty, want the selected workspace directory")
	}
	if got.Location.Project.ID == "" || got.Location.Project.Directory == "" {
		t.Errorf("location.project = %+v, want id and directory (LocationInfo.project required fields)", got.Location.Project)
	}
	if got.Location.Project.Directory != got.Location.Directory {
		t.Errorf("location.project.directory = %q, want the selected directory %q", got.Location.Project.Directory, got.Location.Directory)
	}
}

// TestApiProviderListNeverFabricatesProviderEntries pins the truthfulness rule
// for this operation: the runtime's real catalog names providers via
// model_registry.provider_id, but a ProviderV2Info needs an api descriptor
// (ProviderAISDK.package or ProviderNative.settings) and a request
// configuration (headers/body) that no store holds. A seeded catalog of real
// provider ids must therefore still answer the declared empty array — an entry
// built from provider_id alone would invent the two members that describe how a
// client reaches the provider.
func TestApiProviderListNeverFabricatesProviderEntries(t *testing.T) {
	_, srv, conn := newApiProviderListServer(t)
	ctx := context.Background()

	for _, row := range []struct {
		modelID    string
		tier       int
		providerID string
		enabled    int
	}{
		{"deepseek-v4-pro", 1, "deepseek", 1},
		{"anthropic/claude-sonnet-4", 2, "anthropic", 1},
		{"gpt-5.6-terra", 3, "openai", 1},
		{"disabled-provider-model", 3, "retired-provider", 0},
	} {
		if err := conn.Exec(ctx,
			`INSERT INTO model_registry
			 (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled, sync_source, provider_id)
			 VALUES (?, ?, 128000, 0.5, 1.5, ?, 'models.dev', ?)`,
			row.modelID, row.tier, row.enabled, row.providerID); err != nil {
			t.Fatalf("seed model_registry (%s): %v", row.modelID, err)
		}
	}

	// Premise: the seeded provider ids really are in the store, so the handler's
	// read has something to find and its empty answer is a refusal, not a
	// missing store.
	rows, err := conn.Query(ctx,
		`SELECT DISTINCT provider_id FROM model_registry
		 WHERE enabled = true AND provider_id IS NOT NULL AND provider_id <> ''`)
	if err != nil {
		t.Fatalf("read seeded provider ids: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("seeded catalog holds %d enabled provider ids, want 3 (premise of the no-fabrication claim)", len(rows))
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider")
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider with a seeded catalog: got %d, want 200. Body: %s", status, body)
	}
	var got apiProviderListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if len(got.Data) != 0 {
		t.Fatalf("data = %s, want the empty array: no model_registry row carries the ProviderV2Info api descriptor or request configuration (see the handler doc comment)", body)
	}
	for _, providerID := range []string{"deepseek", "anthropic", "openai"} {
		if strings.Contains(string(body), providerID) {
			t.Errorf("body echoes the seeded provider id %q — a provider id alone is not a ProviderV2Info", providerID)
		}
	}
}

// TestApiProviderListUnreadableCatalogAnswersDeclared503 pins the declared 503
// arm: when this instance genuinely cannot read the catalog that names its
// providers, the handler says so with the declared ServiceUnavailableError body
// instead of claiming an empty provider list.
func TestApiProviderListUnreadableCatalogAnswersDeclared503(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("no such table: model_registry")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/provider with an unreadable catalog: got %d, want the declared 503. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("503 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message", "service")
	if got["_tag"] != "ServiceUnavailableError" {
		t.Errorf("_tag = %v, want ServiceUnavailableError (the declared 503 body)", got["_tag"])
	}
	if got["service"] != "model_registry" {
		t.Errorf("service = %v, want the unavailable store named (model_registry)", got["service"])
	}
	if msg, _ := got["message"].(string); !strings.Contains(msg, "provider registry unavailable") {
		t.Errorf("message = %q, want it to say what could not be read", msg)
	}
}

// TestApiProviderListDeclared400MalformedSelector pins the declared
// InvalidRequestError 400 arm for the deepObject `location` selector this
// operation declares (location[directory] / location[workspace], with
// additionalProperties:false in the pinned document): a blank value, an unknown
// location[...] field, a workspace id outside the declared ^wrk pattern and an
// unreadable/non-directory location[directory] are all malformed input, and a
// malformed selector must never be answered as a successful empty provider list.
func TestApiProviderListDeclared400MalformedSelector(t *testing.T) {
	s, srv, _ := newApiProviderListServer(t)

	cases := []struct {
		name  string
		query string
		field string
	}{
		{"blank directory", "?location%5Bdirectory%5D=", "location[directory]"},
		{"blank workspace", "?location%5Bworkspace%5D=", "location[workspace]"},
		{"unknown field", "?location%5Bbogus%5D=x", "location[bogus]"},
		{"workspace without wrk prefix", "?location%5Bworkspace%5D=ws-1", "location[workspace]"},
		{"unreadable directory", "?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), "location[directory]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/provider%s: got %d, want the declared 400. Body: %s", tc.query, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("400 body is not JSON: %v (%s)", err, body)
			}
			if got["_tag"] != "InvalidRequestError" {
				t.Errorf("_tag = %v, want InvalidRequestError (the declared 400 body)", got["_tag"])
			}
			if got["message"] == "" {
				t.Error("400 body carries no message")
			}
			if field, _ := got["field"].(string); field != tc.field {
				t.Errorf("400 body names field %q, want %q (the malformed selector)", field, tc.field)
			}
		})
	}

	// A well-formed selector is answered with the declared 200 carrying the
	// requested workspace, proving the 400 arm is the malformed-input branch and
	// not a blanket refusal of the selector.
	dir := t.TempDir()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider?location%5Bdirectory%5D="+dir)
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider with location[directory]: got %d, want 200. Body: %s", status, body)
	}
	var got apiProviderListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != dir {
		t.Errorf("location.directory = %q, want the requested %q", got.Location.Directory, dir)
	}
}

// TestApiProviderListMethodNotAllowed pins the method guard: only the declared
// GET is served, and every undeclared method keeps the sibling
// METHOD_NOT_ALLOWED envelope instead of being answered as a successful list.
func TestApiProviderListMethodNotAllowed(t *testing.T) {
	_, srv, _ := newApiProviderListServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/provider")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/provider: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s /api/provider 405 body is not JSON: %v (%s)", method, err, body)
		}
	}
}

// TestApiProviderListServeDeclared401 pins the declared UnauthorizedError arm:
// an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's UNAUTHENTICATED
// body, which an unregistered /api/* path keeps.
func TestApiProviderListServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/provider: got %d, want 401. Body: %s", status, body)
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
		t.Errorf("_tag = %v, want UnauthorizedError (the declared 401 body)", got["_tag"])
	}
}

// TestApiProviderListMountWiring pins the chi mount contract: /api/provider must
// be listed in MountPatterns (cmd/consensus/main.go mounts the shim by these
// patterns — without the entry a parent chi router answers before the shim sees
// the request), the route must be reachable through that mount, no /api/*
// catch-all may be introduced, and the declared 401 envelope must be keyed by
// this exact path only.
func TestApiProviderListMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/provider"] {
		t.Errorf("/api/provider is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-026)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/provider: got %d, want 200 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: the sibling declared operation v2.provider.get
	// (/api/provider/{providerID}) has its OWN exact one-segment route
	// (ROUTE-ADD-027) and an unknown id answers THAT handler's declared 404
	// ProviderNotFoundError; a deeper sub-path still keeps the net/http default
	// 404, and /api/provider did not become an /api/* subtree.
	for _, path := range []string{"/api/provider/anthropic", "/api/provider/bogus", "/api/provider/a/b"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 — ROUTE-ADD-026 registers the exact path, never a catch-all", path, status)
		}
	}

	if !isV2EnvelopeRoutePath("/api/provider") {
		t.Error("/api/provider is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	if !isV2EnvelopeRoutePath("/api/provider/anthropic") {
		t.Error("/api/provider/anthropic is not recognized as a v2-envelope route after ROUTE-ADD-027 registered the one-segment shape")
	}
	if isV2EnvelopeRoutePath("/api/provider/anthropic/extra") {
		t.Error("/api/provider/anthropic/extra is treated as a v2-envelope route; membership must be exact-path or the one-segment shape")
	}
	if !isV2EnvelopeRoutePath("/api/model") || !isV2EnvelopeRoutePath("/api/agent") {
		t.Error("a pre-existing v2 route lost its 401 envelope")
	}
	// The v1 compatibility route is a different declared operation and must stay
	// outside the v2 envelope set.
	if isV2EnvelopeRoutePath("/provider") {
		t.Error("/provider (v1 provider.list) must not join the v2 envelope set")
	}
}

// TestApiProviderListTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId, this test file and the live-probe
// evidence, so the traceability link cannot silently drift from the served
// surface.
func TestApiProviderListTraceRow(t *testing.T) {
	raw, err := readApiProviderListSource()
	if err != nil {
		t.Skipf("api_provider_list.go not readable from the test working directory: %v", err)
	}
	want := fmt.Sprintf("row=ROUTE-ADD-026 spec=specs/openapi/upstream/openapi-1.18.33.json#%s test=internal/shim/opencode/api_provider_list_test.go", "v2.provider.list")
	if !strings.Contains(raw, want) {
		t.Errorf("api_provider_list.go carries no ch:trace line for v2.provider.list (want %q)", want)
	}
	if !strings.Contains(raw, "evidence=docs/evidence/ROUTE-ADD-026-live-probe.md") {
		t.Error("api_provider_list.go does not point at the ROUTE-ADD-026 live-probe evidence")
	}
	if !strings.Contains(raw, "witness=none:unattended-worker-session") {
		t.Error("api_provider_list.go does not record the unattended worker witness")
	}
}

func readApiProviderListSource() (string, error) {
	// The Go test binary runs in the package directory.
	data, err := os.ReadFile("api_provider_list.go")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
