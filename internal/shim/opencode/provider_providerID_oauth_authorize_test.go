package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/consensus/internal/db"
)

// Upstream contract (openapi-1.18.33.json
// paths."/provider/{providerID}/oauth/authorize".post = provider.oauth.authorize,
// SHIM-DRIFT-103): POST /provider/{providerID}/oauth/authorize, one required
// path param providerID plus optional directory/workspace query params, a
// requestBody {method: <auth method index>, inputs?: {key: string}} and
// declared responses 200 (ProviderAuthAuthorization: url/method(auto|code)/
// instructions) and 400 (ProviderAuthError | InvalidRequestError).
//
// The shim advertises API-key methods only — its provider-method registry is
// derived from the auth.<providerID>.<field> rows PUT /auth/{providerID}
// stores (storedProviderAuthMethods, the same source /provider/auth answers
// from) — so no advertised method is an OAuth method. The pinned upstream
// runtime's behaviour for a method whose type is not "oauth" is to return
// early and serialize the absent result as JSON null (ProviderAuth.authorize
// `if (method.type !== "oauth") return` over provider.ts "authorizeRaw"):
// the shim mirrors that branch. It never fabricates an authorization URL.
//
// Every arm below answers net/http's default 404 ("404 page not found") on
// the pre-fix router, which registered no route for this path.

// assertProviderAuthError decodes the document's ProviderAuthError shape
// (components.schemas.ProviderAuthError1: required "name" and "data") and
// asserts the declared name with a non-empty message in the data bag.
func assertProviderAuthError(t *testing.T, context, raw string, wantName string) map[string]any {
	t.Helper()
	var got struct {
		Name string `json:"name"`
		Data map[string]any
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("%s: body is not the declared ProviderAuthError shape: %v (%s)", context, err, raw)
	}
	if got.Name != wantName {
		t.Errorf("%s: error.name = %q, want %q (body %s)", context, got.Name, wantName, raw)
	}
	if got.Data == nil {
		t.Fatalf("%s: error.data missing — ProviderAuthError requires it (body %s)", context, raw)
	}
	return got.Data
}

// TestProviderOAuthAuthorizeAnswersDeclaredSuccess answers the declared 200
// for a provider that holds stored credentials: the request is well-formed,
// the selected method index is one the provider advertises, and that method is
// an API-key method — so there is no OAuth flow to start. The response is the
// declared success code with the JSON null body upstream's own non-oauth
// branch serializes (never a fabricated ProviderAuthAuthorization url, never
// the pre-fix 404).
func TestProviderOAuthAuthorizeAnswersDeclaredSuccess(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"bare method index", "/provider/openai/oauth/authorize", `{"method":0}`},
		{"declared inputs", "/provider/openai/oauth/authorize", `{"method":0,"inputs":{"api_key":"sk-not-a-real-key"}}`},
		{"declared query params", "/provider/openai/oauth/authorize?directory=/tmp/ws&workspace=ws1", `{"method":0}`},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusOK {
			t.Fatalf("%s: POST %s: got %d, want 200. Body: %s", tc.name, tc.path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: Content-Type = %q, want application/json", tc.name, ct)
		}
		// The declared 200 content schema (ProviderAuthAuthorization) is
		// unreachable in this runtime: it requires a url, and no OAuth flow
		// exists to authorise. Upstream serializes the absent result as null,
		// so null — not a fabricated object, not an empty body — is the
		// contract answer.
		if trimmed := strings.TrimSpace(string(body)); trimmed != "null" {
			t.Errorf("%s: body = %q, want null (upstream's non-oauth branch; no authorization URL to return)", tc.name, trimmed)
		}
	}

	// Non-vacuity: the success path really did consult the shim's provider
	// method registry rather than short-circuiting.
	registryRead := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM system_settings") && strings.Contains(q, "auth.") {
			registryRead = true
		}
	}
	if !registryRead {
		t.Errorf("expected an auth-registry read, queries: %v", mdb.queries)
	}
}

// TestProviderOAuthAuthorizeMatchesAdvertisedMethods ties the two provider
// surfaces together: the method index /provider/auth advertises for a provider
// is exactly the index authorize accepts, and a provider the shim advertises
// none for is refused. This is what stops the advertised methods and the
// accepted methods from drifting apart.
func TestProviderOAuthAuthorizeMatchesAdvertisedMethods(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Fatalf("GET /provider/auth: got %d, want 200. Body: %s", status, body)
	}
	var advertised map[string][]providerAuthMethod
	if err := json.Unmarshal(body, &advertised); err != nil {
		t.Fatalf("GET /provider/auth: body is not the method map: %v (%s)", err, body)
	}
	methods, ok := advertised["openai"]
	if !ok || len(methods) == 0 {
		t.Fatalf("GET /provider/auth did not advertise openai: %v", advertised)
	}
	for i, method := range methods {
		if method.Type == "oauth" {
			t.Fatalf("this runtime advertises no OAuth method, got %v", method)
		}
		authorizeBody := `{"method":` + strconv.Itoa(i) + `}`
		status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/provider/openai/oauth/authorize", authorizeBody)
		if status != http.StatusOK {
			t.Errorf("advertised index %d rejected: POST /provider/openai/oauth/authorize (%s): got %d, want 200. Body: %s",
				i, authorizeBody, status, body)
		}
	}

	// One index past the advertised list is not a method: refused, and the
	// refusal names the field and the provider.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/authorize", `{"method":`+strconv.Itoa(len(methods))+`}`)
	if status != http.StatusBadRequest {
		t.Fatalf("unadvertised index: got %d, want 400. Body: %s", status, body)
	}
	data := assertProviderAuthError(t, "unadvertised index", string(body), "BadRequest")
	if data["field"] != "method" {
		t.Errorf("unadvertised index: data.field = %v, want \"method\" (%s)", data["field"], body)
	}
	if data["providerID"] != "openai" {
		t.Errorf("unadvertised index: data.providerID = %v, want openai (%s)", data["providerID"], body)
	}
	if msg, _ := data["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("unadvertised index: data.message empty, want the offending index named (%s)", body)
	}
}

// TestProviderOAuthAuthorizeUnknownProviderIsBadRequest pins the other 400 arm
// of the provider lookup: a provider holding no stored credentials has no
// advertised method, so the request cannot select one. Upstream indexes its
// hook table and faults here; the shim answers the declared 400 with the
// provider named.
func TestProviderOAuthAuthorizeUnknownProviderIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{}) // empty registry
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/unknown-provider/oauth/authorize", `{"method":0}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST for an unregistered provider: got %d, want 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	data := assertProviderAuthError(t, "unregistered provider", string(body), "BadRequest")
	if data["providerID"] != "unknown-provider" {
		t.Errorf("data.providerID = %v, want unknown-provider (%s)", data["providerID"], body)
	}
	if msg, _ := data["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("data.message empty, want the provider named (%s)", body)
	}
}

// TestProviderOAuthAuthorizeMalformedInputIsBadRequest pins the declared 400
// arm for payloads that cannot select a method: absent/empty body, non-JSON,
// JSON that is not an object, a missing/non-numeric/fractional/negative
// `method`, and an `inputs` bag that is not a map of strings. All of them
// answer the document's ProviderAuthError shape with name "BadRequest" — the
// name upstream maps a payload decode failure onto — never the pre-fix 404 and
// never a 200.
func TestProviderOAuthAuthorizeMalformedInputIsBadRequest(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	for _, tc := range []struct {
		name, body string
	}{
		{"no body at all", ""},
		{"not json", "not json"},
		{"json array", `[0]`},
		{"json null", `null`},
		{"missing method", `{}`},
		{"string method", `{"method":"0"}`},
		{"negative method", `{"method":-1}`},
		{"fractional method", `{"method":0.5}`},
		{"inputs not an object", `{"method":0,"inputs":5}`},
		{"inputs value not a string", `{"method":0,"inputs":{"api_key":1}}`},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
			"/provider/openai/oauth/authorize", tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST (%q): got %d, want 400. Body: %s", tc.name, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: Content-Type = %q, want application/json", tc.name, ct)
		}
		data := assertProviderAuthError(t, tc.name, string(body), "BadRequest")
		if msg, _ := data["message"].(string); strings.TrimSpace(msg) == "" {
			t.Errorf("%s: data.message empty, want the offending input named (%s)", tc.name, body)
		}
	}
}

// TestProviderOAuthAuthorizeStoreFailureIsDeclaredInvalidRequest answers an
// unreadable store with the document's SECOND declared 400 shape
// (InvalidRequestError) instead of an undeclared 500: this operation declares
// no 5xx, so a failure to read the method registry must still land on a
// declared code.
func TestProviderOAuthAuthorizeStoreFailureIsDeclaredInvalidRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryErr: errors.New("disk error")})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/authorize", `{"method":0}`)
	if status != http.StatusBadRequest {
		t.Fatalf("failing store: got %d, want 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got struct {
		Tag     string `json:"_tag"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not the declared InvalidRequestError shape: %v (%s)", err, body)
	}
	if got.Tag != "InvalidRequestError" {
		t.Errorf("_tag = %q, want InvalidRequestError (%s)", got.Tag, body)
	}
	if strings.TrimSpace(got.Message) == "" {
		t.Errorf("message empty, want a reason (%s)", body)
	}
}

// TestProviderOAuthAuthorizeMethodGuard pins the non-POST behaviour: the path
// is registered, so a non-POST request reaches the handler and answers the
// sibling POST-route method guard (405 METHOD_NOT_ALLOWED) — never the net/http
// default 404 the pre-fix tree answered for every method.
func TestProviderOAuthAuthorizeMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/provider/openai/oauth/authorize", `{"method":0}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /provider/openai/oauth/authorize: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestProviderOAuthAuthorizeNeighboursUntouched is the non-vacuity control:
// serving provider.oauth.authorize must not turn /provider/* into a catch-all.
// A deeper path does not match the wildcard, the two-segment
// /provider/oauth/authorize path is not the declared operation, and the
// neighbouring /provider and /provider/auth routes keep their own answers. The
// sibling oauth/callback operation used to be one of the unregistered
// neighbours here; ROUTE-ADD-101 now serves it for real, so it is asserted
// against its own handler below instead of the default 404.
func TestProviderOAuthAuthorizeNeighboursUntouched(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key"}),
		},
		queryRow: rowOf(map[string]any{"model_id": "m-1", "max_context": int64(128000)}),
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	for _, tc := range []struct {
		name, method, path, body string
	}{
		{"deeper path is not the declared operation", http.MethodPost, "/provider/openai/oauth/authorize/extra", `{"method":0}`},
		{"declared operation needs the provider segment", http.MethodPost, "/provider/oauth/authorize", `{"method":0}`},
	} {
		status, _, body := doShimRequestBody(t, srv.URL, tc.method, tc.path, tc.body)
		if status != http.StatusNotFound {
			t.Errorf("%s: %s %s: got %d, want 404. Body: %s", tc.name, tc.method, tc.path, status, body)
		}
		if !strings.Contains(string(body), "404 page not found") {
			t.Errorf("%s: %s %s body = %q, want the net/http default 404 (no catch-all route).",
				tc.name, tc.method, tc.path, body)
		}
	}

	// /provider/auth keeps its own contract: GET 200, non-GET the generic 404.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Errorf("GET /provider/auth: got %d, want 200 (sibling route intact). Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/provider/auth")
	if status != http.StatusNotFound {
		t.Errorf("POST /provider/auth: got %d, want 404 (sibling method guard intact). Body: %s", status, body)
	}
	// GET on the authorize path reaches the handler's method guard, not a 404.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/provider/openai/oauth/authorize")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET /provider/openai/oauth/authorize: got %d, want 405. Body: %s", status, body)
	}
	// provider.list is untouched.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/provider")
	if status != http.StatusOK {
		t.Errorf("GET /provider: got %d, want 200 (sibling provider.list intact). Body: %s", status, body)
	}
	// ROUTE-ADD-101 serves the sibling provider.oauth.callback operation on the
	// same /provider/{providerID}/oauth/ subtree: a well-formed POST for a
	// provider holding stored auth reaches the callback handler and answers the
	// declared 200 with the boolean false (this shim parks no OAuth flow), so
	// the path is no longer one of the neighbours that fall through to the
	// default 404 above.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/callback", `{"method":0}`)
	if status != http.StatusOK {
		t.Errorf("POST /provider/openai/oauth/callback: got %d, want 200 (ROUTE-ADD-101 serves it). Body: %s", status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
		t.Errorf("POST /provider/openai/oauth/callback: body = %q, want false", trimmed)
	}
}

// TestProviderOAuthAuthorizeChiMount is the BUG-009 regression for the new
// route: it must be reachable through the parent chi router's /provider/*
// mount (MountPatterns), not only through the shim's own mux. A shim-produced
// 200 proves the mount reaches the wildcard pattern; the sibling
// provider.oauth.callback operation ROUTE-ADD-101 serves on the same subtree
// answers its own 200 through the mount too, so the mount carries both
// wildcard routes rather than being narrowed to one.
func TestProviderOAuthAuthorizeChiMount(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "auth.openai.api_key"}),
		},
	}
	s := NewServer(mdb, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/authorize", `{"method":0}`)
	if status != http.StatusOK {
		t.Fatalf("POST /provider/openai/oauth/authorize via chi mount: got %d, want 200 — /provider/* must reach the shim. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted authorize: Content-Type = %q, want application/json", ct)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "null" {
		t.Errorf("chi-mounted authorize: body = %q, want null", trimmed)
	}

	status, header, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/callback", `{"method":0}`)
	if status != http.StatusOK {
		t.Errorf("chi-mounted sibling callback: got %d, want 200 (ROUTE-ADD-101 serves it through the mount). Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted callback: Content-Type = %q, want application/json", ct)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
		t.Errorf("chi-mounted callback: body = %q, want false", trimmed)
	}
}

// TestProviderOAuthAuthorizeRealStore round-trips the handler against a real
// SQLite store: the provider registry is the real system_settings rows
// (storedProviderAuthMethods), so a provider with stored auth authorizes and a
// provider without is refused — and an unrelated setting does not create a
// provider.
func TestProviderOAuthAuthorizeRealStore(t *testing.T) {
	_, srv, conn := newProviderAuthStoreTestServer(t)

	for _, ins := range []string{
		`INSERT INTO system_settings (key, value) VALUES ('auth.openai.api_key', 'stored')`,
		`INSERT INTO system_settings (key, value) VALUES ('unrelated.setting', 'kept')`,
	} {
		if err := conn.Exec(t.Context(), ins); err != nil {
			t.Fatalf("insert setting: %v", err)
		}
	}

	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/authorize", `{"method":0}`)
	if status != http.StatusOK {
		t.Fatalf("POST /provider/openai/oauth/authorize on real store: got %d, want 200. Body: %s", status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "null" {
		t.Errorf("real-store authorize body = %q, want null", trimmed)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/anthropic/oauth/authorize", `{"method":0}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST for a provider with no stored auth: got %d, want 400. Body: %s", status, body)
	}
	data := assertProviderAuthError(t, "real store, unregistered provider", string(body), "BadRequest")
	if data["providerID"] != "anthropic" {
		t.Errorf("data.providerID = %v, want anthropic (%s)", data["providerID"], body)
	}
}
