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

// Upstream contract (openapi-1.18.33.json paths."/api/provider/{providerID}".get =
// v2.provider.get, ROUTE-ADD-027): GET /api/provider/{providerID}, one path
// parameter (plain required string, no pattern), one optional deepObject
// `location` selector, declared responses 200 {location: LocationInfo, data:
// ProviderV2Info} ("Retrieve a single AI provider so clients can inspect its
// availability and endpoint settings"), 400 InvalidRequestError, 401
// UnauthorizedError, 404 ProviderNotFoundError, 503 ServiceUnavailableError.
// The path had no shim route at all before this change and answered net/http's
// default 404 (the declared-vs-served artifact's NOT-SERVED class). The board
// row's source item SHIM-DRIFT-023 is POSITIONAL: in the committed artifact it
// names POST /api/session/{sessionID}/permission, while v2.provider.get carries
// SHIM-DRIFT-001 — the operation, path and declared response set are the
// stable identity and are what these tests pin.
//
// ProviderV2Info requires id, name, api and request. This runtime can source
// all four truthfully from its own provider stores (model_registry.provider_id
// and the auth.<providerID>.<field> rows in system_settings) — see the handler
// doc comment — and the tests below assert the exact served projection: id and
// name are the resolved provider id, api is the ProviderNative arm with an
// empty settings document, request declares no configured headers/body, and
// `disabled` (optional) is emitted only when the catalog carries rows for the
// provider. No member is invented and no credential value is echoed.
//
// Every 200 schema sets additionalProperties:false, so the tests assert the
// exact member set (an undeclared member is a contract violation).

// apiProviderGetResponse is the declared {location: LocationInfo, data:
// ProviderV2Info} envelope this operation answers with.
type apiProviderGetResponse struct {
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
		Project     struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"project"`
	} `json:"location"`
	Data struct {
		ID       string         `json:"id"`
		Name     string         `json:"name"`
		API      map[string]any `json:"api"`
		Request  map[string]any `json:"request"`
		Disabled *bool          `json:"disabled,omitempty"`
	} `json:"data"`
}

// newApiProviderGetServer builds the ROUTE-ADD-027 fixture: a real SQLite
// database carrying BOTH provider stores the handler reads (model_registry with
// the migration-020 provider_id column, and the system_settings keys
// PUT /auth/{providerID} writes), auth bypassed, so the 200/400/404 arms are
// exercised against real stores rather than mocks.
func newApiProviderGetServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "api-provider-get.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open api provider get test database: %v", err)
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
		// migration 001 §2.4: the store PUT /auth/{providerID} writes
		// (auth.<providerID>.<field>) and storedProviderAuthMethods derives
		// from — the second provider-existence evidence this handler reads.
		`CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TEXT
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare api provider get test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// seedApiProviderGetCatalog inserts one model_registry row per argument:
// modelID, providerID and the enabled flag.
func seedApiProviderGetCatalog(t *testing.T, conn db.DB, rows ...[3]any) {
	t.Helper()
	ctx := context.Background()
	for _, row := range rows {
		if err := conn.Exec(ctx,
			`INSERT INTO model_registry
			 (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled, sync_source, provider_id)
			 VALUES (?, 1, 128000, 0.5, 1.5, ?, 'models.dev', ?)`,
			row[0], row[1], row[2]); err != nil {
			t.Fatalf("seed model_registry (%v): %v", row[0], err)
		}
	}
}

// seedApiProviderGetCredential stores a credential row the way
// PUT /auth/{providerID} does (SPEC-017 §3.2).
func seedApiProviderGetCredential(t *testing.T, conn db.DB, providerID, value string) {
	t.Helper()
	if err := conn.Exec(context.Background(),
		`INSERT INTO system_settings (key, value, description) VALUES (?, ?, ?)`,
		"auth."+providerID+".key", value, "provider credential"); err != nil {
		t.Fatalf("seed provider credential (%s): %v", providerID, err)
	}
}

// TestApiProviderGetServesDeclared200 pins v2.provider.get (ROUTE-ADD-027): the
// exact /api/provider/{providerID} path serves the declared operation with the
// declared 200 shape instead of the pre-fix net/http default 404 (class
// NOT-SERVED: no shim route at all).
func TestApiProviderGetServesDeclared200(t *testing.T) {
	_, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCatalog(t, conn,
		[3]any{"deepseek-v4-pro", 1, "deepseek"},
		[3]any{"deepseek-v4-flash", 1, "deepseek"},
		[3]any{"gpt-5.6-terra", 3, "openai"},
	)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/deepseek")
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider/deepseek: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/provider/deepseek body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, raw, "location", "data")

	var got apiProviderGetResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/provider/deepseek body is not the declared envelope: %v (%s)", err, body)
	}

	// The declared ProviderV2Info requires id, name, api and request. `disabled`
	// is optional and this provider has enabled catalog rows, so it is present
	// and false.
	dataMembers, _ := raw["data"].(map[string]any)
	assertExactMembers(t, dataMembers, "id", "name", "api", "request", "disabled")
	if got.Data.ID != "deepseek" {
		t.Errorf("data.id = %q, want the requested provider id", got.Data.ID)
	}
	if got.Data.Name != "deepseek" {
		t.Errorf("data.name = %q, want the provider's only name (its catalog id — no display-name store exists)", got.Data.Name)
	}
	if got.Data.Disabled == nil || *got.Data.Disabled {
		t.Errorf("data.disabled = %v, want false (the catalog holds enabled models for this provider)", got.Data.Disabled)
	}
	assertExactMembers(t, got.Data.API, "type", "settings")
	if got.Data.API["type"] != "native" {
		t.Errorf("data.api.type = %v, want native (ProviderNative arm: this runtime reaches providers with its own HTTP clients, and names no AI-SDK package)", got.Data.API["type"])
	}
	if settings, ok := got.Data.API["settings"].(map[string]any); !ok || len(settings) != 0 {
		t.Errorf("data.api.settings = %v, want the empty settings document (no provider settings store exists)", got.Data.API["settings"])
	}
	assertExactMembers(t, got.Data.Request, "headers", "body")
	for _, member := range []string{"headers", "body"} {
		if v, ok := got.Data.Request[member].(map[string]any); !ok || len(v) != 0 {
			t.Errorf("data.request.%s = %v, want the empty object (no per-provider request template is configured)", member, got.Data.Request[member])
		}
	}

	// The location block is the real selected workspace, exactly like the
	// sibling v2 operations.
	if got.Location.Directory == "" {
		t.Errorf("location.directory is empty, want the selected workspace directory")
	}
	if got.Location.Project.ID == "" || got.Location.Project.Directory == "" {
		t.Errorf("location.project = %+v, want id and directory (LocationInfo.project required fields)", got.Location.Project)
	}
	if got.Location.Project.Directory != got.Location.Directory {
		t.Errorf("location.project.directory = %q, want the selected directory %q", got.Location.Project.Directory, got.Location.Directory)
	}

	// A credential in the store must never be echoed into the provider body.
	if strings.Contains(string(body), "test-key") {
		t.Errorf("body leaks a credential value: %s", body)
	}
}

// TestApiProviderGetUnknownProviderAnswersDeclared404 pins the declared 404 arm:
// a providerID that appears in NEITHER provider store is not something this
// instance can name, so the handler answers the typed ProviderNotFoundError
// body ({_tag, providerID, message}) instead of a fabricated provider.
func TestApiProviderGetUnknownProviderAnswersDeclared404(t *testing.T) {
	_, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCatalog(t, conn, [3]any{"deepseek-v4-pro", 1, "deepseek"})

	for _, providerID := range []string{"anthropic", "bogus", "DEEPSEEK"} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/"+providerID)
		if status != http.StatusNotFound {
			t.Fatalf("GET /api/provider/%s: got %d, want the declared 404. Body: %s", providerID, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("404 body is not JSON: %v (%s)", err, body)
		}
		assertExactMembers(t, got, "_tag", "providerID", "message")
		if got["_tag"] != "ProviderNotFoundError" {
			t.Errorf("_tag = %v, want ProviderNotFoundError (the declared 404 body)", got["_tag"])
		}
		if got["providerID"] != providerID {
			t.Errorf("providerID = %v, want the requested id %q echoed", got["providerID"], providerID)
		}
		if msg, _ := got["message"].(string); !strings.Contains(msg, providerID) {
			t.Errorf("message = %q, want it to name the provider", msg)
		}
	}
}

// TestApiProviderGetDisabledProviderReportsCatalogState pins the truthful
// `disabled` derivation: a provider whose catalog rows are all disabled still
// EXISTS (200, not 404 — the catalog names it), and the response reports the
// catalog state instead of pretending it is usable.
func TestApiProviderGetDisabledProviderReportsCatalogState(t *testing.T) {
	_, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCatalog(t, conn,
		[3]any{"retired-model-a", 0, "retired-provider"},
		[3]any{"retired-model-b", 0, "retired-provider"},
	)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/retired-provider")
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider/retired-provider: got %d, want 200 (the catalog names it). Body: %s", status, body)
	}
	var got apiProviderGetResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data.Disabled == nil {
		t.Fatalf("data.disabled is absent, want true — every catalog row for this provider is disabled: %s", body)
	}
	if !*got.Data.Disabled {
		t.Errorf("data.disabled = false, want true (no enabled model routes to this provider)")
	}
}

// TestApiProviderGetCredentialOnlyProvider pins the second provider-existence
// evidence: a provider registered through PUT /auth/{providerID} (stored
// credentials, no catalog rows) IS known to this instance, so it answers 200 —
// and because it has no catalog rows there is no catalog state to report, so
// the optional `disabled` member is omitted rather than guessed.
func TestApiProviderGetCredentialOnlyProvider(t *testing.T) {
	_, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCredential(t, conn, "anthropic", "sk-ant-seeded-secret")

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/anthropic")
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider/anthropic: got %d, want 200 (stored credentials register the provider). Body: %s", status, body)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	dataMembers, _ := raw["data"].(map[string]any)
	assertExactMembers(t, dataMembers, "id", "name", "api", "request")
	if dataMembers["id"] != "anthropic" {
		t.Errorf("data.id = %v, want anthropic", dataMembers["id"])
	}
	if strings.Contains(string(body), "sk-ant-seeded-secret") {
		t.Errorf("body echoes the stored credential value: %s", body)
	}
}

// TestApiProviderGetUnreadableCatalogAnswersDeclared503 pins the first declared
// 503 arm: when this instance cannot read the catalog that names its providers,
// the handler says so with the declared ServiceUnavailableError body instead of
// claiming the provider does not exist (a 404 would be a lie about absence).
func TestApiProviderGetUnreadableCatalogAnswersDeclared503(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("no such table: model_registry")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/deepseek")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/provider/deepseek with an unreadable catalog: got %d, want the declared 503. Body: %s", status, body)
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

// TestApiProviderGetUnreadableCredentialStoreAnswersDeclared503 pins the second
// declared 503 arm: a readable catalog that does not name the provider leaves
// the credential store as the only answer to "does this provider exist" — if
// that store cannot be read, the handler cannot claim absence and answers the
// declared ServiceUnavailableError naming it.
func TestApiProviderGetUnreadableCredentialStoreAnswersDeclared503(t *testing.T) {
	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "api-provider-get-nocreds.db"),
		MaxOpenConns: 2,
	})
	if err != nil {
		t.Fatalf("open api provider get test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// Only the catalog exists: the credential store is genuinely unreadable.
	if err := conn.Exec(ctx, `CREATE TABLE model_registry (
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
	)`); err != nil {
		t.Fatalf("prepare api provider get test database: %v", err)
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/anthropic")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/provider/anthropic with an unreadable credential store: got %d, want the declared 503. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("503 body is not JSON: %v (%s)", err, body)
	}
	assertExactMembers(t, got, "_tag", "message", "service")
	if got["service"] != "system_settings" {
		t.Errorf("service = %v, want the unavailable credential store named (system_settings)", got["service"])
	}
}

// TestApiProviderGetDeclared400MalformedInput pins the declared
// InvalidRequestError 400 arms: the path parameter is a plain required string
// with no pattern, so a present-but-blank id is the only malformed id, and the
// declared deepObject `location` selector (additionalProperties:false) rejects
// blank values, unknown fields, a workspace id outside the declared ^wrk shape
// and an unreadable/non-directory location[directory].
func TestApiProviderGetDeclared400MalformedInput(t *testing.T) {
	s, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCatalog(t, conn, [3]any{"deepseek-v4-pro", 1, "deepseek"})

	cases := []struct {
		name  string
		path  string
		field string
	}{
		{"blank provider id", "/api/provider/%20", "providerID"},
		{"blank directory", "/api/provider/deepseek?location%5Bdirectory%5D=", "location[directory]"},
		{"blank workspace", "/api/provider/deepseek?location%5Bworkspace%5D=", "location[workspace]"},
		{"unknown field", "/api/provider/deepseek?location%5Bbogus%5D=x", "location[bogus]"},
		{"workspace without wrk prefix", "/api/provider/deepseek?location%5Bworkspace%5D=ws-1", "location[workspace]"},
		{"unreadable directory", "/api/provider/deepseek?location%5Bdirectory%5D=" + filepath.Join(s.workdir, "does-not-exist"), "location[directory]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want the declared 400. Body: %s", tc.path, status, body)
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
				t.Errorf("400 body names field %q, want %q", field, tc.field)
			}
		})
	}

	// A well-formed selector is answered with the declared 200 carrying the
	// requested workspace, proving the 400 arm is the malformed-input branch and
	// not a blanket refusal of the selector.
	dir := t.TempDir()
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/api/provider/deepseek?location%5Bdirectory%5D="+dir)
	if status != http.StatusOK {
		t.Fatalf("GET /api/provider/deepseek with location[directory]: got %d, want 200. Body: %s", status, body)
	}
	var got apiProviderGetResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Location.Directory != dir {
		t.Errorf("location.directory = %q, want the requested %q", got.Location.Directory, dir)
	}
}

// TestApiProviderGetMethodNotAllowed pins the method guard: only the declared
// GET is served, and every undeclared method keeps the sibling
// METHOD_NOT_ALLOWED envelope instead of being answered as a provider read.
func TestApiProviderGetMethodNotAllowed(t *testing.T) {
	_, srv, conn := newApiProviderGetServer(t)
	seedApiProviderGetCatalog(t, conn, [3]any{"deepseek-v4-pro", 1, "deepseek"})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/provider/deepseek")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/provider/deepseek: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
		}
	}
}

// TestApiProviderGetServeDeclared401 pins the declared UnauthorizedError arm: an
// unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's UNAUTHENTICATED
// body, which an unregistered /api/* path keeps.
func TestApiProviderGetServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/deepseek")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/provider/deepseek: got %d, want 401. Body: %s", status, body)
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

// TestApiProviderGetMountWiring pins the chi mount contract: the parameter shape
// must be listed in MountPatterns (cmd/consensus/main.go mounts the shim by
// these patterns — without the entry a parent chi router answers before the
// shim sees the request), the route must be reachable through that mount, no
// /api/provider/* catch-all may be introduced, and the declared 401 envelope
// must be keyed by this exact one-segment shape only.
func TestApiProviderGetMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/provider/{providerID}"] {
		t.Errorf("/api/provider/{providerID} is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-027)")
	}
	if !mounted["/api/provider"] {
		t.Errorf("/api/provider (v2.provider.list) lost its MountPatterns entry")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// The mount reaches the shim route: this fixture's mock has no provider
	// rows, so the handler answers its own typed 404 ProviderNotFoundError —
	// what matters here is that the request reached the handler at all instead
	// of a chi/net-http default 404 page.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/deepseek")
	if status != http.StatusNotFound || !strings.Contains(string(body), "ProviderNotFoundError") {
		t.Fatalf("chi-mounted GET /api/provider/deepseek: got %d (%s) — the mount must reach the shim handler, whose unknown-provider answer is the typed ProviderNotFoundError", status, body)
	}

	// No catch-all: a deeper /api/provider/* path keeps net/http's default 404
	// and the exact list route keeps its own declared 200/400/503 contract.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/provider/a/b")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/provider/a/b: got %d, want 404 — ROUTE-ADD-027 registers the one-segment shape, never a catch-all. Body: %s", status, body)
	}

	if !isV2EnvelopeRoutePath("/api/provider") {
		t.Error("/api/provider is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	if !isV2EnvelopeRoutePath("/api/provider/deepseek") {
		t.Error("/api/provider/{providerID} is not recognized as a v2-envelope route — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/provider/deepseek/extra") {
		t.Error("/api/provider/deepseek/extra is treated as a v2-envelope route; membership must be the one-segment shape")
	}
	if isV2EnvelopeRoutePath("/api/provider/") {
		t.Error("the blank /api/provider/ path is treated as a v2-envelope route; membership must be a non-empty segment")
	}
	// The v1 compatibility route is a different declared operation and must stay
	// outside the v2 envelope set.
	if isV2EnvelopeRoutePath("/provider") {
		t.Error("/provider (v1 provider.list) must not join the v2 envelope set")
	}
}

// TestApiProviderGetTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId, this test file and the live-probe
// evidence, so the traceability link cannot silently drift from the served
// surface.
func TestApiProviderGetTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_provider_get.go")
	if err != nil {
		t.Skipf("api_provider_get.go not readable from the test working directory: %v", err)
	}
	src := string(raw)
	want := fmt.Sprintf("row=ROUTE-ADD-027 spec=specs/openapi/upstream/openapi-1.18.33.json#%s test=internal/shim/opencode/api_provider_get_test.go", "v2.provider.get")
	if !strings.Contains(src, want) {
		t.Errorf("api_provider_get.go carries no ch:trace line for v2.provider.get (want %q)", want)
	}
	if !strings.Contains(src, "evidence=docs/evidence/ROUTE-ADD-027-live-probe.md") {
		t.Error("api_provider_get.go does not point at the ROUTE-ADD-027 live-probe evidence")
	}
	if !strings.Contains(src, "witness=none:unattended-worker-session") {
		t.Error("api_provider_get.go does not record the unattended worker witness")
	}
}
