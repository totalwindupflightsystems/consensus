package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// providerAuthMethod mirrors the upstream ProviderAuthMethod schema
// (openapi-1.18.33.json components.schemas.ProviderAuthMethod): required
// "type" (oauth|api) and "label", optional "prompts".
type providerAuthMethod struct {
	Type    string           `json:"type"`
	Label   string           `json:"label"`
	Prompts []map[string]any `json:"prompts"`
}

// TestProviderAuth answers upstream provider.auth (ROUTE-ADD-099,
// SHIM-DRIFT-102, declared responses 200 map of providerID ->
// ProviderAuthMethod[] / 400 BadRequest): GET /provider/auth must return 200
// with an object keyed by provider id whose values are auth-method arrays —
// instead of the pre-fix net/http default 404 (no shim route at all).
//
// The shim derives the map truthfully from the auth rows PUT /auth/{id}
// stored in system_settings (keys auth.<providerID>.<field>): every provider
// that holds stored auth is reported with an "api" method, mirroring the
// stored-credential reality. The 200 body must remain an object — the
// contract declares an object, never null — so an empty store answers {}.
func TestProviderAuth(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key", "value": "stored"}),
			rowOf(map[string]any{"key": "auth.anthropic.api_key", "value": "stored"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Fatalf("GET /provider/auth: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string][]providerAuthMethod
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be the provider auth-method map, got %q: %v", body, err)
	}
	if len(got) != 2 {
		t.Fatalf("map carries %d entries, want 2: %v", len(got), got)
	}

	for _, providerID := range []string{"openai", "anthropic"} {
		methods, ok := got[providerID]
		if !ok {
			t.Fatalf("map missing key %q: %v", providerID, got)
		}
		if len(methods) != 1 {
			t.Fatalf("provider %q carries %d methods, want 1: %v", providerID, len(methods), methods)
		}
		if methods[0].Type != "api" {
			t.Errorf("provider %q method type = %q, want \"api\"", providerID, methods[0].Type)
		}
		if strings.TrimSpace(methods[0].Label) == "" {
			t.Errorf("provider %q method label empty (ProviderAuthMethod requires label)", providerID)
		}
		if len(methods[0].Prompts) != 1 {
			t.Fatalf("provider %q method carries %d prompts, want 1: %v", providerID, len(methods[0].Prompts), methods[0].Prompts)
		}
		for _, field := range []string{"type", "key", "message"} {
			if _, ok := methods[0].Prompts[0][field]; !ok {
				t.Errorf("provider %q prompt missing required field %q: %v", providerID, field, methods[0].Prompts[0])
			}
		}
	}

	sawAuthRead := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM system_settings") && strings.Contains(q, "auth.") {
			sawAuthRead = true
		}
	}
	if !sawAuthRead {
		t.Errorf("expected a system_settings auth read, queries: %v", mdb.queries)
	}
}

// TestProviderAuthEmptyStore pins the empty-store arm: upstream provider.auth
// returns an object — the shim must answer {} (not null, not an error) when
// no provider holds stored auth.
func TestProviderAuthEmptyStore(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Fatalf("GET /provider/auth on empty store: got %d, want 200. Body: %s", status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "{}" {
		t.Errorf("empty store body = %q, want {}", trimmed)
	}
}

// TestProviderAuthDBFailure answers the declared 400 arm the way sibling
// handlers do (writeOpencodeError INVALID_REQUEST): an auth store that
// cannot be read must not surface a 500 or an empty 200.
func TestProviderAuthDBFailure(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryErr: fmt.Errorf("disk error")})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /provider/auth with failing store: got %d, want 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "INVALID_REQUEST" {
		t.Errorf("error.code = %q, want INVALID_REQUEST", got.Error.Code)
	}
	if strings.TrimSpace(got.Error.Message) == "" {
		t.Errorf("error.message empty, want a reason")
	}
}

// TestProviderAuthMethodNotAllowed pins the method guard: the operation
// declares only GET; other methods keep the generic not-found answer instead
// of silently reading the store (405 is not part of the declared set).
func TestProviderAuthMethodNotAllowed(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/provider/auth", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /provider/auth: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s /provider/auth: got %d, want 404 (generic not-found)", method, resp.StatusCode)
		}
	}
}

// TestProviderAuthNeighboursUntouched guards the neighbours: the new exact
// pattern must not shadow GET /provider (provider.list) — it still reads
// model_registry and answers 200 — and an unknown /provider/* sub-path still
// falls through to the shim's generic not-found.
func TestProviderAuthNeighboursUntouched(t *testing.T) {
	mdb := &mockDB{
		queryRow: rowOf(map[string]any{"model_id": "m-1", "max_context": int64(128000)}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider")
	if status != http.StatusOK {
		t.Fatalf("GET /provider: got %d, want 200. Body: %s", status, body)
	}
	var listed map[string]any
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("GET /provider body is not JSON: %v (%s)", err, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/provider/oauth/authorize")
	if status != http.StatusNotFound {
		t.Errorf("GET /provider/oauth/authorize: got %d, want 404 (still NOT-SERVED). Body: %s", status, body)
	}
}

// TestProviderAuthRealStore round-trips the map against a real SQLite store:
// providers are derived from actually stored auth rows and the map keys are
// the stored provider ids.
func TestProviderAuthRealStore(t *testing.T) {
	_, srv, conn := newProviderAuthStoreTestServer(t)

	for _, ins := range []string{
		`INSERT INTO system_settings (key, value) VALUES ('auth.openai.api_key', 'stored')`,
		`INSERT INTO system_settings (key, value) VALUES ('auth.anthropic.api_key', 'stored')`,
		`INSERT INTO system_settings (key, value) VALUES ('unrelated.setting', 'kept')`,
	} {
		if err := conn.Exec(context.Background(), ins); err != nil {
			t.Fatalf("insert setting: %v", err)
		}
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Fatalf("GET /provider/auth: got %d, want 200. Body: %s", status, body)
	}
	var got map[string][]providerAuthMethod
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the provider auth-method map: %v (%s)", err, body)
	}
	if len(got) != 2 {
		t.Fatalf("map carries %d entries, want 2 (only providers with stored auth): %v", len(got), got)
	}
	for _, providerID := range []string{"openai", "anthropic"} {
		if len(got[providerID]) != 1 || got[providerID][0].Type != "api" {
			t.Errorf("provider %q methods = %v, want one api method", providerID, got[providerID])
		}
	}
	if _, leaked := got["unrelated"]; leaked {
		t.Errorf("non-auth settings leaked into the provider map: %v", got)
	}
}

// newProviderAuthStoreTestServer builds a shim server over a real database
// containing a real system_settings table so the map can be asserted against
// actual SQL.
func newProviderAuthStoreTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "provider-auth.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open provider-auth test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			key_prefix TEXT,
			scope TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare provider-auth test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}
