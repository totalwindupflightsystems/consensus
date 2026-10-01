package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestGlobalConfigGetAnswersEmptyConfig answers upstream global.config.get
// (SHIM-DRIFT-087, declared responses 200 Config, 400) and
// global.config.update (ROUTE-ADD-090, SHIM-DRIFT-088, declared responses
// 200 Config, 400): GET /global/config must return 200 with a Config
// document — the shim keeps no opencode global-config store, so the truthful
// payload is the empty object (never null, never a 404) — and PATCH must
// answer 200 with the resulting Config document instead of the pre-fix
// net/http default 404.
func TestGlobalConfigGetAnswersEmptyConfig(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/global/config")
	if status != http.StatusOK {
		t.Fatalf("GET /global/config: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET /global/config: Content-Type = %q, want application/json", ct)
	}
	var cfg map[string]any
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("GET /global/config body must be a JSON object, got %q: %v", body, err)
	}
	if cfg == nil {
		t.Errorf("GET /global/config body = %s, want an object (never null)", body)
	}
	if len(cfg) != 0 {
		t.Errorf("GET /global/config = %v, want {} (the shim keeps no opencode global-config store)", cfg)
	}
}

// TestGlobalConfigPatchAnswersResultingConfig pins the declared 200 arm of
// global.config.update: the request body is an optional Config document, and
// the response is the resulting Config — the accepted patch applied to the
// empty current config, i.e. the patch itself. A body is never required (the
// pinned document does not mark requestBody required), so an absent body
// answers 200 with the empty Config.
func TestGlobalConfigPatchAnswersResultingConfig(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPatch, "/global/config",
		`{"shell":"/bin/zsh","snapshot":true,"model":"consensus/default"}`)
	if status != http.StatusOK {
		t.Fatalf("PATCH /global/config: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("PATCH /global/config: Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("PATCH /global/config body must be a Config document, got %q: %v", body, err)
	}
	if got == nil {
		t.Fatalf("PATCH /global/config body = %s, want an object (never null)", body)
	}
	if len(got) != 3 {
		t.Errorf("PATCH /global/config = %v, want the three accepted keys echoed as the resulting Config", got)
	}
	if got["shell"] != "/bin/zsh" {
		t.Errorf(`shell = %v, want "/bin/zsh"`, got["shell"])
	}
	if got["model"] != "consensus/default" {
		t.Errorf(`model = %v, want "consensus/default"`, got["model"])
	}
	if v, ok := got["snapshot"].(bool); !ok || !v {
		t.Errorf("snapshot = %v, want true", got["snapshot"])
	}

	// An absent body is well-formed (requestBody is not required) and answers
	// the empty Config document.
	status, _, body = doShimRequestBody(t, srv.URL, http.MethodPatch, "/global/config", "")
	if status != http.StatusOK {
		t.Fatalf("PATCH /global/config (no body): got %d, want 200. Body: %s", status, body)
	}
	var empty map[string]any
	if err := json.Unmarshal(body, &empty); err != nil {
		t.Fatalf("PATCH /global/config (no body) must be a JSON object, got %q: %v", body, err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("PATCH /global/config (no body) = %s, want {}", body)
	}
}

// TestGlobalConfigMalformedPatchIsBadRequest pins the declared 400 arm:
// malformed input answers the sibling INVALID_REQUEST envelope — not 200, not
// the pre-fix 404. The Config schema sets additionalProperties:false, so an
// undeclared top-level key is a contract violation, not an extension point;
// non-object bodies (arrays, scalars, an explicit null) are not Config
// documents either.
func TestGlobalConfigMalformedPatchIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"truncated json", `{"shell":`},
		{"array body", `["shell"]`},
		{"string body", `"shell"`},
		{"number body", `42`},
		{"explicit null", `null`},
		{"undeclared key", `{"shell":"/bin/sh","not_a_config_key":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, header, body := doShimRequestBody(t, srv.URL, http.MethodPatch, "/global/config", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("PATCH /global/config (%s): got %d, want 400. Body: %s", tc.name, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("PATCH /global/config (%s): Content-Type = %q, want application/json", tc.name, ct)
			}
			assertInvalidRequest(t, "/global/config", tc.body, status, body)
		})
	}

	// Non-vacuity control: the same PATCH with every declared key still
	// answers 200 — the 400 arm rejects the undeclared key, not PATCH itself.
	status, _, body := doShimRequestBody(t, srv.URL, http.MethodPatch, "/global/config", `{"shell":"/bin/sh"}`)
	if status != http.StatusOK {
		t.Errorf("PATCH /global/config {shell}: got %d, want 200 (400 must name the offending key, not refuse the verb). Body: %s", status, body)
	}
}

// TestGlobalConfigMethodGuard pins the undeclared-method behaviour: the
// declared methods (GET, PATCH) are served, so a third method reaches the
// handler and answers the sibling METHOD_NOT_ALLOWED envelope — never the
// net/http default 404 the pre-fix tree answered for the whole path.
func TestGlobalConfigMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/global/config")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /global/config: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestGlobalConfigNeighboursUntouched is the non-vacuity control: serving
// /global/config must not disturb the neighbouring surfaces — /global/health
// and /global/event stay public/streaming, the shim's own /config route keeps
// its own shape, and an unknown /global/ sub-path is still a plain 404.
func TestGlobalConfigNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/global/health")
	if status != http.StatusOK {
		t.Errorf("GET /global/health: got %d, want 200 (sibling untouched). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/global/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /global/unknown-sub: got %d, want 404 (unregistered sub-path). Body: %s", status, body)
	}
}

// TestGlobalConfigChiMount is the BUG-009 regression for the new route: a
// parent chi router mounted with MountPatterns must reach /global/config
// (the /global/* pattern already covers it, so no MountPatterns entry is
// needed) and answer the shim's 200 rather than chi's 404.
func TestGlobalConfigChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/global/config")
	if err != nil {
		t.Fatalf("GET /global/config via chi mount: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /global/config via chi mount: got %d, want 200 — /global/* must cover this route", resp.StatusCode)
	}
	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("chi-mounted /global/config body must be a JSON object: %v", err)
	}
	if cfg == nil {
		t.Errorf("chi-mounted /global/config body = null, want an object (never null)")
	}

	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/global/config", strings.NewReader(`{"shell":"/bin/sh"}`))
	if err != nil {
		t.Fatalf("build PATCH via chi mount: %v", err)
	}
	patched, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /global/config via chi mount: %v", err)
	}
	defer patched.Body.Close()
	if patched.StatusCode != http.StatusOK {
		t.Errorf("PATCH /global/config via chi mount: got %d, want 200", patched.StatusCode)
	}
}

// TestGlobalConfigDeclaredKeysMatchPinnedSchema ties globalConfigDeclaredKeys —
// the key surface the 400 arm enforces — to the pinned upstream document
// (specs/openapi/upstream/openapi-1.18.33.json, pinned by
// specs/openapi/opencode-pin.yaml). A hand-maintained key list is exactly the
// kind of table that goes stale silently, so it is asserted one-to-one against
// the contract this route answers, and the additionalProperties:false premise
// the 400 arm rests on is asserted too.
func TestGlobalConfigDeclaredKeysMatchPinnedSchema(t *testing.T) {
	doc := filepath.Join("..", "..", "..", "specs", "openapi", "upstream", "openapi-1.18.33.json")
	raw, err := os.ReadFile(doc)
	if os.IsNotExist(err) {
		t.Skipf("pinned upstream document not present at %s", doc)
	}
	if err != nil {
		t.Fatalf("read %s: %v", doc, err)
	}

	var parsed struct {
		Components struct {
			Schemas struct {
				Config struct {
					AdditionalProperties *bool          `json:"additionalProperties"`
					Properties           map[string]any `json:"properties"`
				} `json:"Config"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode %s: %v", doc, err)
	}
	schema := parsed.Components.Schemas.Config
	if len(schema.Properties) == 0 {
		t.Fatal("pinned document carries no Config properties — refusing to pass vacuously")
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Fatal("Config schema no longer sets additionalProperties:false — the 400 arm's premise changed; revisit handleGlobalConfig")
	}

	declared := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		declared = append(declared, k)
	}
	enforced := make([]string, 0, len(globalConfigDeclaredKeys))
	for k := range globalConfigDeclaredKeys {
		enforced = append(enforced, k)
	}
	sort.Strings(declared)
	sort.Strings(enforced)

	if strings.Join(declared, ",") != strings.Join(enforced, ",") {
		t.Errorf("globalConfigDeclaredKeys drifted from the pinned Config schema:\n declared: %v\n enforced: %v", declared, enforced)
	}
}
