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

// The five declared POST /mcp/* operations this file covers (ROUTE-ADD-094..098,
// source items SHIM-DRIFT-085..089 in
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json):
//
//	mcp.auth.start        POST /mcp/{name}/auth               (ROUTE-ADD-094)
//	mcp.auth.authenticate POST /mcp/{name}/auth/authenticate  (ROUTE-ADD-095)
//	mcp.auth.callback     POST /mcp/{name}/auth/callback      (ROUTE-ADD-096)
//	mcp.connect           POST /mcp/{name}/connect            (ROUTE-ADD-097)
//	mcp.disconnect        POST /mcp/{name}/disconnect         (ROUTE-ADD-098)
//
// Each declares 200 / 400 / 404 and each resolves its {name} against the
// settings store under the mcp.<name> key convention ROUTE-ADD-093 established.
// The arms are pinned per operation: the OAuth-flow operations answer their
// declared 400 error shape (this runtime implements no MCP OAuth flow, so the
// declared 200 — a real {authorizationUrl, oauthState} pair, or an MCPStatus the
// shim has no tracker for — is unreachable), connect/disconnect answer the
// declared 200 boolean as the truthful false.

// mcp400Shape names the declared 400 shape an operation answers for malformed
// input: the MCP auth operations declare McpUnsupportedOAuthError |
// InvalidRequestError (malformed input is the InvalidRequestError class), while
// mcp.connect / mcp.disconnect declare BadRequestError only.
type mcp400Shape string

const (
	mcp400InvalidRequest mcp400Shape = "invalid-request"
	mcp400BadRequest     mcp400Shape = "bad-request"
)

// mcpDeclaredPostOp is one of the five declared POST operations under test.
type mcpDeclaredPostOp struct {
	row   string // board row lineage
	op    string // operationId
	path  string // request path with {name} substituted ("/mcp/demo/…")
	body  string // request body, "" for a bodyless POST
	shape mcp400Shape

	// resolvedCode/shape is the arm answered for a name the settings store
	// resolves; resolvedBoolean false means the declared 200 boolean is answered.
	resolvedCode    int
	resolvedBoolean bool
	// resolvedShape names the body shape of the resolved-server arm.
	resolvedShape string // "unsupported-oauth" | "invalid-request" | "boolean"
}

func mcpDeclaredPostOps() []mcpDeclaredPostOp {
	return []mcpDeclaredPostOp{
		{
			row: "ROUTE-ADD-094", op: "mcp.auth.start", path: "/mcp/demo/auth",
			shape: mcp400InvalidRequest, resolvedCode: http.StatusBadRequest,
			resolvedShape: "unsupported-oauth",
		},
		{
			row: "ROUTE-ADD-095", op: "mcp.auth.authenticate", path: "/mcp/demo/auth/authenticate",
			shape: mcp400InvalidRequest, resolvedCode: http.StatusBadRequest,
			resolvedShape: "unsupported-oauth",
		},
		{
			row: "ROUTE-ADD-096", op: "mcp.auth.callback", path: "/mcp/demo/auth/callback",
			body: `{"code":"auth-code"}`, shape: mcp400InvalidRequest,
			resolvedCode: http.StatusBadRequest, resolvedShape: "invalid-request",
		},
		{
			row: "ROUTE-ADD-097", op: "mcp.connect", path: "/mcp/demo/connect",
			shape: mcp400BadRequest, resolvedCode: http.StatusOK, resolvedBoolean: true,
			resolvedShape: "boolean",
		},
		{
			row: "ROUTE-ADD-098", op: "mcp.disconnect", path: "/mcp/demo/disconnect",
			shape: mcp400BadRequest, resolvedCode: http.StatusOK, resolvedBoolean: true,
			resolvedShape: "boolean",
		},
	}
}

// assertMCPUnsupportedOAuthShape pins the declared 400 McpUnsupportedOAuthError
// body: {"error": <non-empty string>} and nothing else (additionalProperties:
// false).
func assertMCPUnsupportedOAuthShape(t *testing.T, path string, data []byte) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: body not JSON: %v (%s)", path, err, data)
	}
	if len(got) != 1 {
		t.Errorf("%s: body = %s, want exactly {\"error\": ...} (declared schema has additionalProperties:false)", path, data)
	}
	if msg, _ := got["error"].(string); msg == "" {
		t.Errorf("%s: body.error empty, want the declared non-empty message (%s)", path, data)
	}
}

// assertMCPInvalidRequestShape pins the declared 400 InvalidRequestError body:
// {_tag: "InvalidRequestError", message: <non-empty>} — the optional kind/field
// members are omitted by these handlers.
func assertMCPInvalidRequestShape(t *testing.T, path string, data []byte) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: body not JSON: %v (%s)", path, err, data)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("%s: body._tag = %v, want InvalidRequestError (declared schema)", path, got["_tag"])
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Errorf("%s: body.message empty, want the declared non-empty message (%s)", path, data)
	}
	if len(got) != 2 {
		t.Errorf("%s: body = %s, want exactly {_tag, message}", path, data)
	}
}

// assertMCPBadRequestShape pins the declared 400 BadRequestError body:
// {name: "BadRequest", data: {message: <non-empty>, kind?: <wantKind>}}.
func assertMCPBadRequestShape(t *testing.T, path, wantKind string, data []byte) {
	t.Helper()
	var got struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: body not the BadRequestError envelope: %v (%s)", path, err, data)
	}
	if got.Name != "BadRequest" {
		t.Errorf("%s: name = %q, want BadRequest (declared schema)", path, got.Name)
	}
	if got.Data.Message == "" {
		t.Errorf("%s: data.message empty, want the declared non-empty message (%s)", path, data)
	}
	if got.Data.Kind != wantKind {
		t.Errorf("%s: data.kind = %q, want %q", path, got.Data.Kind, wantKind)
	}
}

// assertMCPTypedNotFoundShape pins the declared 404 McpServerNotFoundError body:
// {_tag, name, message} and nothing else, naming the requested server.
func assertMCPTypedNotFoundShape(t *testing.T, path, wantName string, data []byte) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: typed 404 body not JSON: %v (%s)", path, err, data)
	}
	if got["_tag"] != "McpServerNotFoundError" {
		t.Errorf("%s: _tag = %v, want McpServerNotFoundError", path, got["_tag"])
	}
	if got["name"] != wantName {
		t.Errorf("%s: name = %v, want %q", path, got["name"], wantName)
	}
	if msg, _ := got["message"].(string); msg == "" {
		t.Errorf("%s: message empty, want the declared non-empty message (%s)", path, data)
	}
	if len(got) != 3 {
		t.Errorf("%s: typed body = %s, want exactly {_tag, name, message}", path, data)
	}
}

// TestMCPDeclaredPostOpsResolvedServerAnswersDeclaredArm answers the central
// truthfulness question per operation: for an MCP server the shim resolves, the
// declared code is served with a body its own contract declares and no
// fabricated success. mcp.auth.start / mcp.auth.authenticate answer the declared
// 400 McpUnsupportedOAuthError (an authorization URL cannot be minted, so the
// declared 200 is unreachable), mcp.auth.callback answers the declared 400
// InvalidRequestError (no flow is pending to exchange a code against), and
// mcp.connect / mcp.disconnect answer the declared 200 boolean false (no
// connection was established or torn down).
func TestMCPDeclaredPostOpsResolvedServerAnswersDeclaredArm(t *testing.T) {
	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
			_, srv := newTestServer(mdb)
			defer srv.Close()

			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, op.path, op.body)
			if status != op.resolvedCode {
				t.Fatalf("%s %s (%s): got %d, want %d. Body: %s", http.MethodPost, op.path, op.op, status, op.resolvedCode, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") && op.resolvedShape != "boolean" {
				t.Errorf("%s: Content-Type = %q, want application/json", op.path, ct)
			}
			switch op.resolvedShape {
			case "unsupported-oauth":
				assertMCPUnsupportedOAuthShape(t, op.path, body)
			case "invalid-request":
				assertMCPInvalidRequestShape(t, op.path, body)
			case "boolean":
				if string(body) != "false" {
					t.Errorf("%s: body = %s, want the truthful JSON false (no connection was established/torn down)", op.path, body)
				}
			default:
				t.Fatalf("%s: unknown expected shape %q", op.path, op.resolvedShape)
			}

			// None of these operations mutates state: the resolved-server arm
			// reads the settings store and answers.
			for _, q := range mdb.queries {
				if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(q)), "SELECT") {
					t.Errorf("%s: unexpected non-read query %q", op.path, q)
				}
			}
		})
	}
}

// TestMCPDeclaredPostOpsUnknownServerIsTyped404 pins the declared 404 arm for
// every operation: the shim resolves an MCP server from its settings store, so a
// name with no stored entry names no server and answers the typed
// McpServerNotFoundError body — never a bare 404, and never a fabricated
// success.
func TestMCPDeclaredPostOpsUnknownServerIsTyped404(t *testing.T) {
	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			mdb := &mockDB{} // empty store: no MCP server entry resolves
			_, srv := newTestServer(mdb)
			defer srv.Close()

			path := strings.Replace(op.path, "/mcp/demo/", "/mcp/missing/", 1)
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, op.body)
			if status != http.StatusNotFound {
				t.Fatalf("%s: got %d, want 404. Body: %s", path, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("%s: Content-Type = %q, want application/json", path, ct)
			}
			assertMCPTypedNotFoundShape(t, path, "missing", body)
		})
	}
}

// TestMCPDeclaredPostOpsMalformedInputIsDeclared400 pins the declared 400 arm
// for malformed input on every operation: a present-but-blank declared query
// selector (directory, workspace) and a whitespace-only {name} are refused in
// the shape each operation declares — InvalidRequestError for the MCP auth
// operations, BadRequestError for connect/disconnect — and never reported as an
// unsupported OAuth flow.
func TestMCPDeclaredPostOpsMalformedInputIsDeclared400(t *testing.T) {
	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			for _, param := range []string{"directory", "workspace"} {
				mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
				_, srv := newTestServer(mdb)
				path := op.path + "?" + param + "="
				status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, op.body)
				srv.Close()

				if status != http.StatusBadRequest {
					t.Fatalf("POST %s: got %d, want 400. Body: %s", path, status, body)
				}
				switch op.shape {
				case mcp400InvalidRequest:
					assertMCPInvalidRequestShape(t, path, body)
				case mcp400BadRequest:
					assertMCPBadRequestShape(t, path, "Query", body)
				}
			}

			// A whitespace-only {name} is malformed input too: parseMCPSubPath
			// accepts it as a segment, and the handler refuses it rather than
			// resolving a stranger.
			mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
			_, srv := newTestServer(mdb)
			defer srv.Close()
			path := strings.Replace(op.path, "/mcp/demo/", "/mcp/%20%20/", 1)
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, path, op.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s: got %d, want 400 for a whitespace-only name. Body: %s", path, status, body)
			}
			switch op.shape {
			case mcp400InvalidRequest:
				assertMCPInvalidRequestShape(t, path, body)
			case mcp400BadRequest:
				assertMCPBadRequestShape(t, path, "Params", body)
			}

			// A valued selector is well-formed: the operation answers its
			// resolved-server arm instead of a 400.
			mdb = &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
			_, srv = newTestServer(mdb)
			defer srv.Close()
			status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, op.path+"?directory=/tmp/ws&workspace=w1", op.body)
			if status != op.resolvedCode {
				t.Errorf("POST %s with valued selectors: got %d, want %d. Body: %s", op.path, status, op.resolvedCode, body)
			}
		})
	}
}

// TestMCPDeclaredPostOpsStoreFailureIsDeclared400 keeps store failures inside
// each operation's declared response set: none of these operations declares a
// 5xx, so an unreadable settings store answers the operation's declared 400
// shape rather than an undeclared 500.
func TestMCPDeclaredPostOpsStoreFailureIsDeclared400(t *testing.T) {
	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			mdb := &mockDB{queryErr: errors.New("settings store unavailable")}
			_, srv := newTestServer(mdb)
			defer srv.Close()

			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, op.path, op.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s with an unreadable store: got %d, want 400. Body: %s", op.path, status, body)
			}
			switch op.shape {
			case mcp400InvalidRequest:
				assertMCPInvalidRequestShape(t, op.path, body)
			case mcp400BadRequest:
				// A store read failure is not attributable to a request part, so
				// the optional kind member is omitted (the declared schema marks
				// it optional).
				assertMCPBadRequestShape(t, op.path, "", body)
			}
		})
	}
}

// TestMCPDeclaredPostOpsWrongMethodKeepsDefault404 pins the shape-check
// contract: 405 is not part of any of these operations' declared response sets,
// so a method the operation does not declare keeps the byte-identical
// pre-change answer — the net/http default 404 body — on both the served paths
// and the DELETE-only sibling (which keeps POST served, so only GET/PUT/PATCH
// stay default).
func TestMCPDeclaredPostOpsWrongMethodKeepsDefault404(t *testing.T) {
	unserved := []struct{ method, path string }{
		{http.MethodGet, "/mcp/demo/auth"},
		{http.MethodPut, "/mcp/demo/auth"},
		{http.MethodPatch, "/mcp/demo/auth"},
		{http.MethodGet, "/mcp/demo/auth/authenticate"},
		{http.MethodDelete, "/mcp/demo/auth/authenticate"},
		{http.MethodGet, "/mcp/demo/auth/callback"},
		{http.MethodDelete, "/mcp/demo/auth/callback"},
		{http.MethodGet, "/mcp/demo/connect"},
		{http.MethodDelete, "/mcp/demo/connect"},
		{http.MethodPut, "/mcp/demo/connect"},
		{http.MethodGet, "/mcp/demo/disconnect"},
		{http.MethodDelete, "/mcp/demo/disconnect"},
		{http.MethodPut, "/mcp/demo/disconnect"},
	}
	_, srv := newTestServer(&mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}})
	defer srv.Close()

	for _, tc := range unserved {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, tc.method, tc.path, "")
			if status != http.StatusNotFound {
				t.Errorf("%s %s: got %d, want 404 (405 is not in the declared set). Body: %s", tc.method, tc.path, status, body)
			}
			if string(body) != mcpNavDefaultNotFoundBody {
				t.Errorf("%s %s: body = %q, want the unchanged net/http default %q", tc.method, tc.path, body, mcpNavDefaultNotFoundBody)
			}
		})
	}
}

// TestMCPDeclaredPostOpsOutOfFamilyShapesKeepDefault404 is the non-vacuity
// control for the shape check: the route serves the six declared shapes and
// nothing else. A bare /mcp/{name}, an unknown sub-path, a deeper path and a
// sub-path one segment too deep all keep the net/http default 404 they answered
// before these routes existed — the route never became a /mcp/* catch-all.
func TestMCPDeclaredPostOpsOutOfFamilyShapesKeepDefault404(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}})
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/mcp/demo"},
		{http.MethodDelete, "/mcp/demo"},
		{http.MethodPost, "/mcp/demo/unknown"},
		{http.MethodPost, "/mcp/demo/auth/deeper"},
		{http.MethodPost, "/mcp/demo/auth/authenticate/extra"},
		{http.MethodPost, "/mcp/demo/connect/extra"},
		{http.MethodPost, "/mcp/demo/"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, tc.method, tc.path, `{"code":"x"}`)
			if status != http.StatusNotFound {
				t.Errorf("%s %s: got %d, want the unchanged 404. Body: %s", tc.method, tc.path, status, body)
			}
			if string(body) != mcpNavDefaultNotFoundBody {
				t.Errorf("%s %s: body = %q, want the unchanged net/http default %q", tc.method, tc.path, body, mcpNavDefaultNotFoundBody)
			}
		})
	}
}

// TestMCPAuthCallbackBodySchemaStrict pins mcp.auth.callback's declared request
// schema (a single JSON object carrying the required string `code`,
// additionalProperties: false): every violation is refused with the declared 400
// InvalidRequestError before the server is even resolved, while a schema-valid
// body reaches the no-pending-flow answer — and the two arms are distinguishable
// by their message, so the schema gate is not a vacuous alias of the flow gate.
func TestMCPAuthCallbackBodySchemaStrict(t *testing.T) {
	rejected := []struct{ name, body string }{
		{"absent body", ""},
		{"not JSON", "definitely not json"},
		{"JSON null", "null"},
		{"JSON array", `[{"code":"x"}]`},
		{"JSON scalar", `"code"`},
		{"empty object", `{}`},
		{"code not a string", `{"code":1}`},
		{"undeclared field", `{"code":"x","extra":true}`},
		{"trailing second value", `{"code":"x"} {"code":"y"}`},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
			_, srv := newTestServer(mdb)
			defer srv.Close()

			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/mcp/demo/auth/callback", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST /mcp/demo/auth/callback (%s): got %d, want 400. Body: %s", tc.name, status, body)
			}
			assertMCPInvalidRequestShape(t, "/mcp/demo/auth/callback", body)
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("%s: body not JSON: %v", tc.name, err)
			}
			if msg, _ := got["message"].(string); strings.Contains(msg, "no MCP OAuth flow is pending") {
				t.Errorf("%s: a schema violation must be refused as malformed input, not reported as the no-pending-flow arm (%s)", tc.name, body)
			}
			for _, q := range mdb.queries {
				if strings.Contains(strings.ToUpper(q), "SELECT") {
					t.Errorf("%s: the store was read for a body that violates the declared schema; queries: %v", tc.name, mdb.queries)
				}
			}
		})
	}

	// The declared schema puts no minLength/pattern on `code`, so an empty
	// string is schema-valid: it takes the no-pending-flow arm (the same arm a
	// real code takes) rather than being misreported as malformed input.
	for _, body := range []string{`{"code":""}`, `{"code":"auth-code"}`} {
		mdb := &mockDB{queryResults: []db.Row{mcpRegistrationRow("demo")}}
		_, srv := newTestServer(mdb)
		status, _, respBody := doShimRequestBody(t, srv.URL, http.MethodPost, "/mcp/demo/auth/callback", body)
		srv.Close()
		if status != http.StatusBadRequest {
			t.Fatalf("POST /mcp/demo/auth/callback (%s): got %d, want the declared 400 no-pending-flow arm. Body: %s", body, status, respBody)
		}
		assertMCPInvalidRequestShape(t, "/mcp/demo/auth/callback", respBody)
		var got map[string]any
		if err := json.Unmarshal(respBody, &got); err != nil {
			t.Fatalf("%s: body not JSON: %v", body, err)
		}
		if msg, _ := got["message"].(string); !strings.Contains(msg, "no MCP OAuth flow is pending") {
			t.Errorf("%s: message = %q, want the no-pending-flow reason (%s)", body, msg, respBody)
		}
	}
}

// TestMCPDeclaredPostOpsRequireAuth pins the auth policy the served-surface
// inventory records (auth: api-key) for all five routes: they are
// Consensus-authenticated shim routes, so an unauthenticated POST is 401
// UNAUTHENTICATED, not the pre-fix 404 and not an unauthenticated 400.
func TestMCPDeclaredPostOpsRequireAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil) // no skipAuth
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, op.path, op.body)
			if status != http.StatusUnauthorized {
				t.Fatalf("unauthenticated POST %s: got %d, want 401. Body: %s", op.path, status, body)
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
		})
	}
}

// TestMCPDeclaredPostOpsChiMount is the BUG-009 regression for the five new
// routes in the combined-deployment shape cmd/consensus/main.go builds: the
// native MCP server owns /mcp/* (mounted first), the shim's MountPatterns are
// mounted after, and the deeper param routes must win for the declared
// management sub-paths while /mcp/sse keeps reaching the native MCP handler.
func TestMCPDeclaredPostOpsChiMount(t *testing.T) {
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

	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, op.path, op.body)
			if status != op.resolvedCode {
				t.Fatalf("POST %s via the combined chi mount: got %d, want %d — every declared /mcp/* shape must be registered in MountPatterns. Body: %s",
					op.path, status, op.resolvedCode, body)
			}
		})
	}

	// The native MCP mount keeps the rest of the subtree.
	status, _, _ := doShimRequestBody(t, srv.URL, http.MethodGet, "/mcp/sse", "")
	if status != http.StatusNoContent {
		t.Errorf("GET /mcp/sse via the combined mount: got %d, want the native MCP handler (204)", status)
	}
}

// TestMCPDeclaredPostOpsRealStore round-trips the handlers against a real SQLite
// settings store: a stored MCP entry makes the name resolvable for every
// operation (each answering its declared arm), and a name with no stored entry
// answers the declared typed 404. It is the end-to-end check that the arms above
// are not mock artifacts.
func TestMCPDeclaredPostOpsRealStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mcp-ops.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("open mcp ops test database: %v", err)
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
			('mcp.demo.type', 'remote'),
			('mcp.demo.url', 'https://example.invalid/mcp')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare mcp ops test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, op := range mcpDeclaredPostOps() {
		t.Run(op.op, func(t *testing.T) {
			status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, op.path, op.body)
			if status != op.resolvedCode {
				t.Fatalf("POST %s (real store): got %d, want %d. Body: %s", op.path, status, op.resolvedCode, body)
			}
			unknown := strings.Replace(op.path, "/mcp/demo/", "/mcp/nosuch/", 1)
			status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost, unknown, op.body)
			if status != http.StatusNotFound {
				t.Fatalf("POST %s (real store): got %d, want 404. Body: %s", unknown, status, body)
			}
			assertMCPTypedNotFoundShape(t, unknown, "nosuch", body)
		})
	}
}
