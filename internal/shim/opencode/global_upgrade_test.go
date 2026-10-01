package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// assertUpgradeResult validates a body against the declared UpgradeResult
// union (upstream global.upgrade 200): {success:true, version:string} or
// {success:false, error:string}, both additionalProperties:false. It fails on
// any field outside the union, on a missing success flag, and on the payload
// field the arm it selected requires.
func assertUpgradeResult(t *testing.T, path, body string, data []byte) {
	t.Helper()
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("POST %s: body is not a JSON object: %v (%s)", path, err, body)
	}
	for field := range got {
		if field != "success" && field != "version" && field != "error" {
			t.Errorf("POST %s: unexpected field %q — the declared UpgradeResult arms are additionalProperties:false (%s)", path, field, body)
		}
	}
	raw, ok := got["success"]
	if !ok {
		t.Fatalf("POST %s: UpgradeResult must carry \"success\" (%s)", path, body)
	}
	var success bool
	if err := json.Unmarshal(raw, &success); err != nil {
		t.Fatalf("POST %s: \"success\" is not a boolean: %v (%s)", path, err, body)
	}
	if success {
		v, ok := got["version"]
		if !ok {
			t.Fatalf("POST %s: success=true without the required \"version\" (%s)", path, body)
		}
		if _, ok := got["error"]; ok {
			t.Errorf("POST %s: success=true arm may not carry \"error\" (%s)", path, body)
		}
		var version string
		if err := json.Unmarshal(v, &version); err != nil || strings.TrimSpace(version) == "" {
			t.Errorf("POST %s: \"version\" must be a non-empty string (%s)", path, body)
		}
		return
	}
	e, ok := got["error"]
	if !ok {
		t.Fatalf("POST %s: success=false without the required \"error\" (%s)", path, body)
	}
	if _, ok := got["version"]; ok {
		t.Errorf("POST %s: success=false arm may not carry \"version\" (%s)", path, body)
	}
	var msg string
	if err := json.Unmarshal(e, &msg); err != nil || strings.TrimSpace(msg) == "" {
		t.Errorf("POST %s: \"error\" must be a non-empty string (%s)", path, body)
	}
}

// TestGlobalUpgradeAnswersDeclaredResult answers upstream global.upgrade
// (ROUTE-ADD-092, SHIM-DRIFT-090, declared responses: 200 UpgradeResult,
// 400 BadRequest | InvalidRequestError): POST /global/upgrade must return 200
// with a schema-valid UpgradeResult — instead of the pre-fix net/http default
// 404. The shim manages no opencode installation, so the truthful arm is the
// declared failure result ({success:false, error}), never a fabricated
// version for an upgrade that did not happen.
func TestGlobalUpgradeAnswersDeclaredResult(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"named target", "/global/upgrade", `{"target":"1.18.33"}`},
		{"absent body (the document leaves requestBody optional)", "/global/upgrade", ""},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusOK {
			t.Fatalf("%s: POST %s (%q): got %d, want 200. Body: %s", tc.name, tc.path, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertUpgradeResult(t, tc.path, tc.body, body)

		var got map[string]json.RawMessage
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: POST %s: %v", tc.name, tc.path, err)
		}
		var success bool
		if err := json.Unmarshal(got["success"], &success); err != nil {
			t.Fatalf("%s: POST %s: %v", tc.name, tc.path, err)
		}
		if success {
			t.Errorf("%s: POST %s answered success=true — the shim manages no opencode installation, so the truthful UpgradeResult is the failure arm. Body: %s",
				tc.name, tc.path, body)
		}
		var msg string
		if err := json.Unmarshal(got["error"], &msg); err != nil {
			t.Fatalf("%s: POST %s: %v", tc.name, tc.path, err)
		}
		if !strings.Contains(msg, "not supported") {
			t.Errorf("%s: POST %s: error = %q, want it to say the upgrade is not supported by the shim", tc.name, tc.path, msg)
		}
	}
}

// TestGlobalUpgradeMalformedInputIsBadRequest pins the declared 400 arm: a
// body that violates the declared requestBody schema ({target*: string},
// additionalProperties:false) must answer the sibling INVALID_REQUEST
// envelope, not 200 and not the pre-fix 404. An absent body is well-formed
// (the document leaves requestBody optional) — only a body that IS present
// and does not satisfy the schema is malformed.
func TestGlobalUpgradeMalformedInputIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, body string
	}{
		{"missing required target", "/global/upgrade", `{}`},
		{"target not a string", "/global/upgrade", `{"target":123}`},
		{"target is null", "/global/upgrade", `{"target":null}`},
		{"target is an object", "/global/upgrade", `{"target":{"version":"1.18.33"}}`},
		{"blank target", "/global/upgrade", `{"target":""}`},
		{"whitespace target", "/global/upgrade", `{"target":"   "}`},
		{"unknown field (additionalProperties:false)", "/global/upgrade", `{"target":"1.18.33","force":true}`},
		{"not json", "/global/upgrade", `not json`},
		{"json array body", "/global/upgrade", `[1,2]`},
		{"json null body", "/global/upgrade", `null`},
		{"truncated json", "/global/upgrade", `{"target":`},
	} {
		status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s (%q): got %d, want 400. Body: %s", tc.name, tc.path, tc.body, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: POST %s: Content-Type = %q, want application/json", tc.name, tc.path, ct)
		}
		assertInvalidRequest(t, tc.path, tc.body, status, body)
	}
}

// TestGlobalUpgradeMethodGuard pins the non-POST behaviour: /global/upgrade is
// a registered route, so a non-POST request reaches the handler and answers
// the sibling METHOD_NOT_ALLOWED envelope — never the net/http default 404 the
// pre-fix tree answered, and never a 404 from the handler itself.
func TestGlobalUpgradeMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, body := doShimRequestBody(t, srv.URL, method, "/global/upgrade", `{"target":"1.18.33"}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /global/upgrade: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestGlobalUpgradeNeighboursUntouched is the non-vacuity control: serving
// /global/upgrade must not become a /global/* catch-all. An unknown /global
// sub-path and a deeper path under the served route stay net/http's 404 (the
// NOT-SERVED class the other declared /global/* operations are still pinned
// in), while the sibling global route GET /global/health keeps answering 200.
func TestGlobalUpgradeNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/global/unknown"},
		{http.MethodPost, "/global/upgrade/sub"},
		{http.MethodGet, "/global/upgrade/sub"},
	} {
		status, _, body := doShimRequestBody(t, srv.URL, tc.method, tc.path, `{}`)
		if status != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404 (no /global/* catch-all on the shim mux). Body: %s",
				tc.method, tc.path, status, body)
		}
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/global/health")
	if status != http.StatusOK {
		t.Fatalf("GET /global/health: got %d, want 200 (sibling global route). Body: %s", status, body)
	}
	var health map[string]any
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("GET /global/health body is not JSON: %v (%s)", err, body)
	}
	if healthy, _ := health["healthy"].(bool); !healthy {
		t.Errorf("GET /global/health: healthy = %v, want true", health["healthy"])
	}
}

// TestGlobalUpgradeChiMount is the BUG-009 regression for the new route:
// POST /global/upgrade must be reachable through a parent chi router mounted
// with MountPatterns (the shape cmd/consensus/main.go uses). The /global/*
// entry already exposes the subtree, so a chi 404 here would mean the shim's
// mux lost the exact registration. A deeper /global sub-path through the same
// mount stays 404 — MountPatterns gains no new catch-all.
func TestGlobalUpgradeChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, header, body := doShimRequestBody(t, srv.URL, http.MethodPost, "/global/upgrade", `{"target":"1.18.33"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /global/upgrade via chi mount: got %d, want 200 — /global/upgrade must be registered on the shim mux behind the /global/* MountPatterns entry. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("chi-mounted POST /global/upgrade: Content-Type = %q, want application/json", ct)
	}
	assertUpgradeResult(t, "/global/upgrade", `{"target":"1.18.33"}`, body)

	status, _, _ = doShimRequestBody(t, srv.URL, http.MethodPost, "/global/upgrade/sub", `{}`)
	if status != http.StatusNotFound {
		t.Errorf("POST /global/upgrade/sub via chi mount: got %d, want 404 (no catch-all)", status)
	}
}
