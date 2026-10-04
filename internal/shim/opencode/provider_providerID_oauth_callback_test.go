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
// paths."/provider/{providerID}/oauth/callback".post = provider.oauth.callback,
// SHIM-DRIFT-098 — note the board row ROUTE-ADD-101 cites SHIM-DRIFT-104, but
// the served/declared comparison classifies this path as SHIM-DRIFT-098 and
// SHIM-DRIFT-104 is /pty/{ptyID}/connect): POST
// /provider/{providerID}/oauth/callback, one required path param providerID
// plus optional directory/workspace query params, a requestBody
// {method: <auth method index>, code?: string} whose schema is
// additionalProperties: false, and declared responses 200 (boolean "OAuth
// callback processed successfully") and 400 (ProviderAuthError |
// InvalidRequestError).
//
// The pinned upstream handler hands the code to the flow the matching authorize
// call parked in instance state and returns the literal boolean true on success
// (packages/opencode/src/server/routes/instance/httpapi/handlers/provider.ts
// "callback"; ProviderAuth.callback faults ProviderAuthOauthMissing when no
// flow is pending). The shim has no OAuth implementation and parks no flow: its
// provider-method registry (storedProviderAuthMethods) advertises only "api"
// methods, so there is nothing for a callback to complete and the truthful
// answer is the declared boolean false — never null (the declared type is
// boolean) and never true (which would claim a credential was stored).
//
// Every arm below answers net/http's default 404 ("404 page not found") on the
// pre-fix router, which registered no route for this path.

// TestProviderOAuthCallbackAnswersDeclaredFalse answers the declared 200 for a
// provider that holds stored credentials and a well-formed body: the shim
// advertises an API-key method, parks no OAuth flow, so there is no callback to
// process. The response is the declared success code with the JSON boolean
// false — strict JSON, not null and not the pre-fix 404.
func TestProviderOAuthCallbackAnswersDeclaredFalse(t *testing.T) {
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
		{"bare method index", "/provider/openai/oauth/callback", `{"method":0}`},
		{"declared code", "/provider/openai/oauth/callback", `{"method":0,"code":"ac_authorization_code"}`},
		{"declared query params", "/provider/openai/oauth/callback?directory=/tmp/ws&workspace=ws1", `{"method":0}`},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusOK {
			t.Fatalf("%s: POST %s: got %d, want 200. Body: %s", tc.name, tc.path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: Content-Type = %q, want application/json", tc.name, ct)
		}
		// The declared content schema is boolean. The shim can only answer
		// truthfully: no OAuth flow was parked, so nothing was processed. That
		// is the exact JSON token false — a null body would not decode as the
		// declared boolean, and true would claim a credential was stored.
		if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
			t.Errorf("%s: body = %q, want false (declared boolean; no OAuth flow to process)", tc.name, trimmed)
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("%s: body is not valid JSON: %v (%s)", tc.name, err, body)
		} else if b, ok := decoded.(bool); !ok || b {
			t.Errorf("%s: decoded body = %#v, want boolean false", tc.name, decoded)
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

// TestProviderOAuthCallbackMatchesAdvertisedMethods ties the callback to the
// same method registry the sibling authorize handler validates against: the
// index /provider/auth advertises is accepted, one past the list is refused
// with the field and provider named, and a provider the shim advertises none
// for is refused. This is what keeps the advertised methods and the accepted
// methods from drifting apart across both oauth operations.
func TestProviderOAuthCallbackMatchesAdvertisedMethods(t *testing.T) {
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
	for i := range methods {
		callbackBody := `{"method":` + strconv.Itoa(i) + `}`
		status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/provider/openai/oauth/callback", callbackBody)
		if status != http.StatusOK {
			t.Errorf("advertised index %d rejected: POST /provider/openai/oauth/callback (%s): got %d, want 200. Body: %s",
				i, callbackBody, status, body)
		} else if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
			t.Errorf("advertised index %d: body = %q, want false", i, trimmed)
		}
	}

	// One index past the advertised list is not a method: refused, and the
	// refusal names the field and the provider.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/callback", `{"method":`+strconv.Itoa(len(methods))+`}`)
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

// TestProviderOAuthCallbackUnknownProviderIsBadRequest pins the other 400 arm
// of the provider lookup: a provider holding no stored credentials has no
// advertised method, so the callback cannot be attributed to one. Upstream
// resolves the pending flow by providerID and faults here; the shim answers the
// declared 400 with the provider named.
func TestProviderOAuthCallbackUnknownProviderIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{}) // empty registry
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/unknown-provider/oauth/callback", `{"method":0}`)
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

// TestProviderOAuthCallbackMalformedInputIsBadRequest pins the declared 400 arm
// for payloads the document's schema rejects: absent/empty body, non-JSON, JSON
// that is not an object, a missing/non-numeric/fractional/negative `method`, a
// `code` that is not a string, and — because the request schema is
// additionalProperties: false — any field the contract does not declare. All of
// them answer the document's ProviderAuthError shape with name "BadRequest",
// never the pre-fix 404 and never a 200.
func TestProviderOAuthCallbackMalformedInputIsBadRequest(t *testing.T) {
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
		{"null method", `{"method":null}`},
		{"code not a string", `{"method":0,"code":5}`},
		{"null code", `{"method":0,"code":null}`},
		{"undeclared extra field", `{"method":0,"provider":"openai"}`},
		{"undeclared nested field", `{"method":0,"code":"x","inputs":{}}`},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
			"/provider/openai/oauth/callback", tc.body)
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

// TestProviderOAuthCallbackStoreFailureIsDeclaredInvalidRequest answers an
// unreadable store with the document's SECOND declared 400 shape
// (InvalidRequestError) instead of an undeclared 500: this operation declares
// no 5xx, so a failure to read the method registry must still land on a
// declared code.
func TestProviderOAuthCallbackStoreFailureIsDeclaredInvalidRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryErr: errors.New("disk error")})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/callback", `{"method":0}`)
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

// TestProviderOAuthCallbackMethodGuard pins the non-POST behaviour: the path is
// registered, so a non-POST request reaches the handler and answers the sibling
// POST-route method guard (405 METHOD_NOT_ALLOWED) — never the net/http default
// 404 the pre-fix tree answered for every method. GET on the path is therefore
// consistent with the handler's own guard rather than a 404.
func TestProviderOAuthCallbackMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/provider/openai/oauth/callback", `{"method":0}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /provider/openai/oauth/callback: got %d, want 405. Body: %s", method, status, body)
		}
	}
	// GET is the arm a client is most likely to try; assert it explicitly so the
	// guard is pinned for the verb that has no body.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/provider/openai/oauth/callback")
	if status != http.StatusMethodNotAllowed {
		t.Errorf("GET /provider/openai/oauth/callback: got %d, want 405. Body: %s", status, body)
	}
}

// TestProviderOAuthCallbackNeighboursUntouched is the non-vacuity control:
// serving provider.oauth.callback must not turn /provider/* into a catch-all. A
// deeper path, the two-segment /provider/oauth/callback path and an unrelated
// /provider/* sub-path all keep the net/http default 404 body, and the
// neighbouring authorize/provider routes keep their own answers.
func TestProviderOAuthCallbackNeighboursUntouched(t *testing.T) {
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
		{"deeper path is not the declared operation", http.MethodPost, "/provider/openai/oauth/callback/extra", `{"method":0}`},
		{"declared operation needs the provider segment", http.MethodPost, "/provider/oauth/callback", `{"method":0}`},
		{"unrelated provider sub-path stays default 404", http.MethodPost, "/provider/openai/oauth/refresh", `{"method":0}`},
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

	// The sibling provider.oauth.authorize route is untouched: well-formed POST
	// still answers its own declared 200 (JSON null — non-oauth branch).
	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/openai/oauth/authorize", `{"method":0}`)
	if status != http.StatusOK {
		t.Errorf("POST /provider/openai/oauth/authorize: got %d, want 200 (sibling intact). Body: %s", status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "null" {
		t.Errorf("POST /provider/openai/oauth/authorize: body = %q, want null (sibling intact)", trimmed)
	}
	// /provider/auth keeps its own contract: GET 200, non-GET the generic 404.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/provider/auth")
	if status != http.StatusOK {
		t.Errorf("GET /provider/auth: got %d, want 200 (sibling route intact). Body: %s", status, body)
	}
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/provider/auth")
	if status != http.StatusNotFound {
		t.Errorf("POST /provider/auth: got %d, want 404 (sibling method guard intact). Body: %s", status, body)
	}
	// provider.list is untouched.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/provider")
	if status != http.StatusOK {
		t.Errorf("GET /provider: got %d, want 200 (sibling provider.list intact). Body: %s", status, body)
	}
}

// TestProviderOAuthCallbackChiMount is the BUG-009 regression for the new
// route: it must be reachable through the parent chi router's /provider/*
// mount (MountPatterns), not only through the shim's own mux. A shim-produced
// 200 with the declared boolean proves the mount reaches the wildcard pattern.
func TestProviderOAuthCallbackChiMount(t *testing.T) {
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
		"/provider/openai/oauth/callback", `{"method":0}`)
	if status != http.StatusOK {
		t.Fatalf("POST /provider/openai/oauth/callback via chi mount: got %d, want 200 — /provider/* must reach the shim. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted callback: Content-Type = %q, want application/json", ct)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
		t.Errorf("chi-mounted callback: body = %q, want false", trimmed)
	}
}

// TestProviderOAuthCallbackRealStore round-trips the handler against a real
// SQLite store: the provider registry is the real system_settings rows
// (storedProviderAuthMethods), so a provider with stored auth answers the
// declared boolean false and a provider without is refused — and an unrelated
// setting does not create a provider.
func TestProviderOAuthCallbackRealStore(t *testing.T) {
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
		"/provider/openai/oauth/callback", `{"method":0,"code":"ac_code"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /provider/openai/oauth/callback on real store: got %d, want 200. Body: %s", status, body)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "false" {
		t.Errorf("real-store callback body = %q, want false", trimmed)
	}

	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPost,
		"/provider/anthropic/oauth/callback", `{"method":0}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST for a provider with no stored auth: got %d, want 400. Body: %s", status, body)
	}
	data := assertProviderAuthError(t, "real store, unregistered provider", string(body), "BadRequest")
	if data["providerID"] != "anthropic" {
		t.Errorf("data.providerID = %v, want anthropic (%s)", data["providerID"], body)
	}
}
