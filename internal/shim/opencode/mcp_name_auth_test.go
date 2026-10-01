package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// mcpNavDefaultNotFoundBody is the body net/http's default handler writes for
// an unregistered path. The /mcp/* shapes this route does not serve must keep
// it byte-for-byte: the sibling rows of the declared family stay pinned to the
// NOT-SERVED class ("net/http default 404") in
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json, and their
// own route-add rows serve them.
const mcpNavDefaultNotFoundBody = "404 page not found\n"

// mcpRegistrationRow is the settings-store row that makes an MCP server name
// resolvable: the flat translation of the upstream config's mcp: {"<name>": …}
// block (handleMCPAuthRemove's doc comment).
func mcpRegistrationRow(name string) db.Row {
	return rowOf(map[string]any{"key": "mcp." + name + ".type"})
}

// TestMCPAuthRemoveAnswersDeclaredSuccess answers upstream mcp.auth.remove
// (ROUTE-ADD-093, SHIM-DRIFT-092, declared responses: 200 {success:true},
// 400 BadRequestError, 404 McpServerNotFoundError): DELETE /mcp/{name}/auth
// must return the declared 200 body for a server the shim knows — instead of
// the pre-fix net/http default 404.
func TestMCPAuthRemoveAnswersDeclaredSuccess(t *testing.T) {
	mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusOK {
		t.Fatalf("DELETE /mcp/demo/auth: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// The declared 200 schema is {success: true} with
	// additionalProperties: false — exactly one key, boolean true.
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("DELETE /mcp/demo/auth body is not JSON: %v (%s)", err, body)
	}
	if len(got) != 1 {
		t.Errorf("body = %s, want exactly {\"success\":true} (declared schema has additionalProperties:false)", body)
	}
	if got["success"] != true {
		t.Errorf("body.success = %#v, want true", got["success"])
	}

	// The removal is a real store write: the OAuth rows for the resolved server
	// are deleted, scoped to that server's own namespace.
	var deleted bool
	for _, q := range mdb.queries {
		if strings.Contains(q, "DELETE FROM system_settings") {
			deleted = true
		}
	}
	if !deleted {
		t.Errorf("no system_settings DELETE was issued; queries: %v", mdb.queries)
	}
}

// TestMCPAuthRemoveUnknownServerIsTyped404 pins the declared 404 arm: the shim
// resolves an MCP server from its settings store, so a name with no stored
// entry names no server whose credentials could be removed. The answer is the
// upstream typed body {_tag, name, message} — never a bare 404 — and nothing is
// deleted for it.
func TestMCPAuthRemoveUnknownServerIsTyped404(t *testing.T) {
	mdb := &mockDB{} // empty store: no MCP server entry resolves
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/missing/auth")
	if status != http.StatusNotFound {
		t.Fatalf("DELETE /mcp/missing/auth: got %d, want 404. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var typed map[string]any
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("typed 404 body is not JSON: %v (%s)", err, body)
	}
	if typed["_tag"] != "McpServerNotFoundError" {
		t.Errorf("_tag = %v, want McpServerNotFoundError (declared schema)", typed["_tag"])
	}
	if typed["name"] != "missing" {
		t.Errorf("name = %v, want the requested server name", typed["name"])
	}
	if msg, _ := typed["message"].(string); msg == "" {
		t.Errorf("message empty, want the declared non-empty message (%s)", body)
	}
	if len(typed) != 3 {
		t.Errorf("typed body = %s, want exactly {_tag, name, message} (additionalProperties:false)", body)
	}

	for _, q := range mdb.queries {
		if strings.Contains(q, "DELETE FROM system_settings") {
			t.Errorf("a 404 for an unknown server must not delete anything; queries: %v", mdb.queries)
		}
	}
}

// TestMCPAuthRemoveMalformedInputIsBadRequest pins the declared 400 arm: a
// present-but-blank declared query param (directory, workspace) is malformed
// input and answers the sibling INVALID_REQUEST envelope (the handleSkill
// convention). A valued param is well-formed and keeps the declared 200.
func TestMCPAuthRemoveMalformedInputIsBadRequest(t *testing.T) {
	for _, path := range []string{"/mcp/demo/auth?directory=", "/mcp/demo/auth?workspace="} {
		mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
		_, srv := newTestServer(mdb)
		status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, path)
		srv.Close()

		if status != http.StatusBadRequest {
			t.Fatalf("DELETE %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("DELETE %s: Content-Type = %q, want application/json", path, ct)
		}
		assertInvalidRequest(t, path, string(body), status, body)
		for _, q := range mdb.queries {
			if strings.Contains(q, "DELETE FROM system_settings") {
				t.Errorf("DELETE %s: malformed input must not delete anything; queries: %v", path, mdb.queries)
			}
		}
	}

	mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
	_, srv := newTestServer(mdb)
	defer srv.Close()
	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth?directory=/tmp/ws&workspace=ws1")
	if status != http.StatusOK {
		t.Errorf("DELETE /mcp/demo/auth?directory=/tmp/ws&workspace=ws1: got %d, want 200 (valued params are well-formed). Body: %s", status, body)
	}
}

// TestMCPAuthRemoveStoreReadFailureIsBadRequest keeps store failures inside the
// operation's declared error set: mcp.auth.remove declares 400, 404 and 200, so
// an unreadable settings store answers the declared 400 arm rather than an
// undeclared 5xx (the handleProviderAuth convention, ROUTE-ADD-099).
func TestMCPAuthRemoveStoreReadFailureIsBadRequest(t *testing.T) {
	mdb := &mockDB{queryErr: errors.New("settings store unavailable")}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE /mcp/demo/auth with an unreadable store: got %d, want 400. Body: %s", status, body)
	}
	assertInvalidRequest(t, "/mcp/demo/auth", string(body), status, body)
}

// TestMCPAuthRemoveDeleteFailureIsInternalError pins the write-failure arm
// against the sibling credential-removal operation handleAuthDelete
// (SHIM-DRIFT-059), which answers 500 INTERNAL_ERROR when the store write
// fails.
func TestMCPAuthRemoveDeleteFailureIsInternalError(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{mcpRegistrationRow("demo")},
		execErr:      errors.New("write locked"),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusInternalServerError {
		t.Fatalf("DELETE /mcp/demo/auth with a failing write: got %d, want 500. Body: %s", status, body)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not the error envelope: %v (%s)", err, body)
	}
	if got.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("error.code = %q, want INTERNAL_ERROR", got.Error.Code)
	}
}

// TestMCPAuthRemoveMethodGuardKeepsDefault404 pins the non-DELETE behaviour:
// 405 is not part of mcp.auth.remove's declared set, and the sibling
// POST /mcp/{name}/auth row (mcp.auth.start, SHIM-DRIFT-093 / ROUTE-ADD-094) is
// still unserved, so any other method on this path must keep the pre-change
// answer byte-for-byte — the net/http default 404 — the same choice
// handleProviderAuth (ROUTE-ADD-099) made for its undeclared methods.
func TestMCPAuthRemoveMethodGuardKeepsDefault404(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method, "/mcp/demo/auth")
		if status != http.StatusNotFound {
			t.Errorf("%s /mcp/demo/auth: got %d, want 404 (405 is not in the declared set). Body: %s", method, status, body)
		}
		if string(body) != mcpNavDefaultNotFoundBody {
			t.Errorf("%s /mcp/demo/auth: body = %q, want the unchanged net/http default %q", method, body, mcpNavDefaultNotFoundBody)
		}
	}
}

// TestMCPAuthRemoveFamilyNeighboursUntouched is the non-vacuity control: the
// route serves the declared auth shape and nothing else. The rest of the
// declared /mcp/* family (connect, disconnect, auth/authenticate,
// auth/callback) and a bare /mcp/{name} keep the net/http default 404 they
// answered before this route existed, and the MCP-DIRECT-001 delegation on the
// bare /mcp mount is untouched.
func TestMCPAuthRemoveFamilyNeighboursUntouched(t *testing.T) {
	s, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/mcp/demo/connect"},
		{http.MethodPost, "/mcp/demo/disconnect"},
		{http.MethodPost, "/mcp/demo/auth/authenticate"},
		{http.MethodPost, "/mcp/demo/auth/callback"},
		{http.MethodDelete, "/mcp/demo"},
		{http.MethodDelete, "/mcp/demo/auth/deeper"},
	} {
		status, _, body := doShimRequest(t, srv.URL, tc.method, tc.path)
		if status != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want the unchanged 404. Body: %s", tc.method, tc.path, status, body)
		}
		if string(body) != mcpNavDefaultNotFoundBody {
			t.Errorf("%s %s: body = %q, want the unchanged net/http default %q", tc.method, tc.path, body, mcpNavDefaultNotFoundBody)
		}
	}

	// The bare /mcp mount keeps its documented 501 stub for a plain browser…
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/mcp")
	if status != http.StatusNotImplemented {
		t.Errorf("GET /mcp: got %d, want 501 (unchanged stub). Body: %s", status, body)
	}

	// …and still delegates an MCP client shape (MCP-DIRECT-001).
	delegated := false
	s.SetMCPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delegated = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mcp-handler"))
	}))
	mcpReq, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	if err != nil {
		t.Fatalf("build POST /mcp: %v", err)
	}
	mcpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(mcpReq)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	if !delegated || resp.StatusCode != http.StatusOK {
		t.Errorf("POST /mcp (JSON-RPC) via the shim: status %d delegated %v, want the injected MCP handler (MCP-DIRECT-001)", resp.StatusCode, delegated)
	}
}

// TestMCPAuthRemoveRequiresAuth pins the auth policy the served-surface
// inventory records (auth: api-key): the new route is a Consensus-authenticated
// shim route, so an unauthenticated DELETE is 401 UNAUTHENTICATED, not the
// pre-fix 404.
func TestMCPAuthRemoveRequiresAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil) // no skipAuth
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE /mcp/demo/auth: got %d, want 401. Body: %s", status, body)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not the error envelope: %v (%s)", err, body)
	}
	if got.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", got.Error.Code)
	}
}

// TestMCPAuthRemoveRealStore round-trips the handler against a real SQLite
// settings store: the resolved server's stored OAuth rows are removed for real
// (the declared 200), a server with only its config entry still answers 200
// (the shim holds no OAuth rows for it), and a name with no stored entry
// answers the typed 404. The neighbouring server's rows are untouched.
func TestMCPAuthRemoveRealStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mcp-auth.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("open mcp auth test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Two statements only: the shared scratch filesystem can be very slow, and
	// every Exec is its own WAL commit.
	for _, stmt := range []string{
		`CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO system_settings (key, value) VALUES
			('mcp.demo.type', 'local'),
			('mcp.demo.oauth.access_token', 'tok-demo'),
			('mcp.demo.oauth.refresh_token', 'ref-demo'),
			('mcp.other.type', 'remote'),
			('mcp.other.oauth.access_token', 'tok-other')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare mcp auth test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusOK {
		t.Fatalf("DELETE /mcp/demo/auth (real store): got %d, want 200. Body: %s", status, body)
	}
	if string(body) != `{"success":true}` {
		t.Errorf("body = %s, want {\"success\":true}", body)
	}

	remaining := map[string]bool{}
	rows, err := conn.Query(ctx, `SELECT key FROM system_settings ORDER BY key`)
	if err != nil {
		t.Fatalf("read back settings: %v", err)
	}
	for _, row := range rows {
		remaining[toString(row["key"])] = true
	}
	if remaining["mcp.demo.oauth.access_token"] || remaining["mcp.demo.oauth.refresh_token"] {
		t.Errorf("demo OAuth rows survived the removal: %v", remaining)
	}
	if !remaining["mcp.demo.type"] {
		t.Errorf("the server's own config entry was removed too: %v", remaining)
	}
	if !remaining["mcp.other.oauth.access_token"] || !remaining["mcp.other.type"] {
		t.Errorf("the neighbouring server's rows were touched: %v", remaining)
	}

	// Registered but with no OAuth rows: the removal is still the declared 200.
	status, _, body = doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/other/auth")
	if status != http.StatusOK {
		t.Errorf("DELETE /mcp/other/auth: got %d, want 200. Body: %s", status, body)
	}

	// No stored entry at all: the declared typed 404.
	status, _, body = doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/nosuch/auth")
	if status != http.StatusNotFound {
		t.Fatalf("DELETE /mcp/nosuch/auth: got %d, want 404. Body: %s", status, body)
	}
	var typed map[string]any
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("typed 404 body is not JSON: %v (%s)", err, body)
	}
	if typed["_tag"] != "McpServerNotFoundError" || typed["name"] != "nosuch" {
		t.Errorf("typed 404 = %s, want _tag=McpServerNotFoundError name=nosuch", body)
	}
}

// TestMCPAuthRemoveChiMount is the BUG-009 regression for the new route, in the
// combined-deployment shape cmd/consensus/main.go builds: the native MCP server
// owns /mcp/* (mounted first), the shim's MountPatterns are mounted after, and
// the deeper param route must win for the auth sub-path while /mcp/sse keeps
// reaching the native MCP handler.
func TestMCPAuthRemoveChiMount(t *testing.T) {
	s := NewServer(&mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}, "test-key", nil, nil)
	s.skipAuth = true

	native := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r := chi.NewRouter()
	// Mirror main.go: the MCP server mounts /mcp/* and /mcp before the shim.
	r.Handle("/mcp/*", native)
	r.Handle("/mcp", native)
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, "/mcp/demo/auth")
	if status != http.StatusOK {
		t.Fatalf("DELETE /mcp/demo/auth via the combined chi mount: got %d, want 200 — /mcp/{name}/auth must be registered in MountPatterns. Body: %s", status, body)
	}
	if string(body) != `{"success":true}` {
		t.Errorf("chi-mounted body = %s, want {\"success\":true}", body)
	}

	// The native MCP mount keeps the rest of the subtree.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/mcp/sse")
	if status != http.StatusNoContent {
		t.Errorf("GET /mcp/sse via the combined mount: got %d, want the native MCP handler (204)", status)
	}
}
