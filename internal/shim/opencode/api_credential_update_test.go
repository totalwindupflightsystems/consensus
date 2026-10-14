package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/api/credential/{credentialID}", ROUTE-ADD-008):
//
//	PATCH  /api/credential/{credentialID}   v2.credential.update   204 | 400 | 401
//	DELETE /api/credential/{credentialID}   v2.credential.remove   204 | 400 | 401
//
// Before ROUTE-ADD-008 the path had no shim route at all, so every request fell
// through to net/http's default 404 (the declared-vs-served artifact's
// NOT-SERVED class, source item SHIM-DRIFT-002 for PATCH). The shim now answers
// the declared contract per its documented arm set:
//
//   - PATCH (v2.credential.update, this row): the required `label` body is
//     validated, the deepObject location selector is validated, and because the
//     runtime keeps NO credential store (no credentials table exists in
//     migrations 001..026 and the OAuth surface stores nothing) a well-formed
//     update answers the DECLARED 400 InvalidRequestError naming the missing
//     store — never a fabricated 204. This mirrors the ROUTE-FIX-037
//     part.update narrowing precedent ("declared 404 for every well-formed
//     partID — never fabricate a 200") recorded in the served-surface yaml.
//   - DELETE (v2.credential.remove) is ROUTE-ADD-007's sibling scope: until
//     that merge lands it keeps the honest 405 METHOD_NOT_ALLOWED.
//   - The declared 401 is the /api/* auth wall's v2 envelope
//     ({_tag: "UnauthorizedError", message}).
//
// The pinned document declares NO ^-style pattern for credentialID (schema is
// plain type: string, unlike ^pty / ^ses), so blank is the only ill-formed
// shape the contract itself defines.

// newApiCredentialServer builds the ROUTE-ADD-008 fixture: a shim with auth
// bypassed and a fresh temp workspace, so the 400/405 arms are exercised
// through a real NewServer + httptest.Server rather than a mock.
func newApiCredentialServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

// doCredentialRequest issues a PATCH (or any method) with the given body and
// an explicit credentialID segment.
func doCredentialRequest(t *testing.T, base, method, credentialID, body string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+"/api/credential/"+credentialID, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s /api/credential/%s: %v", method, credentialID, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s /api/credential/%s: %v", method, credentialID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	header := resp.Header
	data := readAllCredentialBody(t, resp)
	return resp.StatusCode, header, data
}

func readAllCredentialBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}

// assertCredentialInvalidRequest pins the declared 400 InvalidRequestError
// envelope {_tag, message} plus the optional kind/field members the handler
// names.
func assertCredentialInvalidRequest(t *testing.T, label string, body []byte) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("%s: 400 body _tag = %v, want InvalidRequestError (the declared 400 shape)", label, got["_tag"])
	}
	if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("%s: 400 body message empty, want a reason", label)
	}
}

// TestApiCredentialUpdateMethodGuard pins the method split: non-PATCH answers
// 405 METHOD_NOT_ALLOWED (the sibling method-guard envelope), including the
// DELETE whose real handler is ROUTE-ADD-007's sibling scope. 405 is not in
// the declared response set, so it is never the answer to a well-formed PATCH.
func TestApiCredentialUpdateMethodGuard(t *testing.T) {
	_, srv := newApiCredentialServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		status, _, body := doCredentialRequest(t, srv.URL, method, "cred-github", `{"label":"renamed"}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/credential/cred-github: got %d, want 405. Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s 405 body is not JSON: %v (%s)", method, err, body)
		}
	}
}

// TestApiCredentialUpdateWellFormedAnswersDeclared400 pins the documented
// arm: a well-formed PATCH (id + declared body) answers the DECLARED 400
// InvalidRequestError — the truthful no-credential-store answer. The declared
// 204 is never fabricated: it would echo an update that never happened (the
// ROUTE-FIX-037 precedent).
func TestApiCredentialUpdateWellFormedAnswersDeclared400(t *testing.T) {
	_, srv := newApiCredentialServer(t)

	for _, credentialID := range []string{"cred-github", "github", "0197f2a2-1c6e-7cc2-8f04-2a1b3d5e6f70"} {
		t.Run(credentialID, func(t *testing.T) {
			status, header, body := doCredentialRequest(t, srv.URL, http.MethodPatch, credentialID, `{"label":"renamed label"}`)
			if status != http.StatusBadRequest {
				t.Fatalf("PATCH /api/credential/%s: got %d, want the declared 400. Body: %s", credentialID, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertCredentialInvalidRequest(t, "well-formed PATCH", body)

			// The truthful refusal names the credential and says WHY —
			// no store exists — so the caller learns the runtime's real
			// state instead of a bare rejection.
			var got map[string]any
			_ = json.Unmarshal(body, &got)
			msg, _ := got["message"].(string)
			if !strings.Contains(msg, credentialID) || !strings.Contains(msg, "credential store") {
				t.Errorf("400 message = %q, want it to name the credential %q and the missing credential store", msg, credentialID)
			}
		})
	}
}

// TestApiCredentialUpdateMalformedInput pins the declared 400 arms for
// malformed input, validated BEFORE the no-store answer:
//
//   - a blank credentialID (the only ill-formed id shape the pinned contract
//     defines — it declares no ^-style pattern);
//   - a body violating the declared schema: not JSON, not an object, a missing
//     required `label`, a non-string label, or an undeclared key
//     (additionalProperties: false);
//   - a malformed deepObject location selector (unknown location field,
//     present-but-blank value, non-^wrk workspace).
func TestApiCredentialUpdateMalformedInput(t *testing.T) {
	_, srv := newApiCredentialServer(t)

	t.Run("blank credentialID", func(t *testing.T) {
		status, _, body := doCredentialRequest(t, srv.URL, http.MethodPatch, "", `{"label":"x"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("blank credentialID: got %d, want 400. Body: %s", status, body)
		}
		assertCredentialInvalidRequest(t, "blank credentialID", body)
		var got map[string]any
		_ = json.Unmarshal(body, &got)
		if got["kind"] != "Path" {
			t.Errorf("kind = %v, want Path", got["kind"])
		}
		if got["field"] != "credentialID" {
			t.Errorf("field = %v, want credentialID", got["field"])
		}
	})

	for _, tc := range []struct {
		name  string
		label string
		body  string
	}{
		{"not json", "label", `not json at all`},
		{"array body", "label", `["label"]`},
		{"missing label", "label", `{}`},
		{"non-string label", "label", `{"label":17}`},
		{"undeclared key", "secret", `{"label":"ok","secret":"extra"}`},
	} {
		t.Run("body: "+tc.name, func(t *testing.T) {
			status, _, body := doCredentialRequest(t, srv.URL, http.MethodPatch, "cred-github", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("%s: got %d, want 400. Body: %s", tc.name, status, body)
			}
			assertCredentialInvalidRequest(t, tc.name, body)
			var got map[string]any
			_ = json.Unmarshal(body, &got)
			if got["kind"] != "Body" {
				t.Errorf("kind = %v, want Body", got["kind"])
			}
			if got["field"] != tc.label {
				t.Errorf("field = %v, want %s", got["field"], tc.label)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		params string
	}{
		{"unknown location field", "?location[token]=x"},
		{"blank location[directory]", "?location[directory]="},
		{"blank location[workspace]", "?location[workspace]="},
		{"non-wrk workspace", "?location[workspace]=ses123"},
	} {
		t.Run("selector: "+tc.name, func(t *testing.T) {
			url := srv.URL + "/api/credential/cred-github" + tc.params
			resp, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(`{"label":"x"}`))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			out, err := http.DefaultClient.Do(resp)
			if err != nil {
				t.Fatalf("PATCH %s: %v", url, err)
			}
			defer func() { _ = out.Body.Close() }()
			data := readAllCredentialBody(t, out)
			if out.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s: got %d, want 400. Body: %s", tc.name, out.StatusCode, data)
			}
			assertCredentialInvalidRequest(t, tc.name, data)
			var got map[string]any
			_ = json.Unmarshal(data, &got)
			if got["kind"] != "Query" {
				t.Errorf("kind = %v, want Query", got["kind"])
			}
		})
	}
}

// TestApiCredentialUpdateUnauthorized pins the declared 401 arm: an
// unauthenticated request answers the v2 envelope ({_tag:
// "UnauthorizedError", message}) via the /api/* auth wall — never the shim's
// UNAUTHENTICATED body — for the declared credential/{id} shape.
func TestApiCredentialUpdateUnauthorized(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doCredentialRequest(t, srv.URL, http.MethodPatch, "cred-github", `{"label":"renamed"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated PATCH: got %d, want 401. Body: %s", status, body)
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

// TestApiCredentialMountWiring pins the mount contract: the /api/credential/
// pattern is listed in MountPatterns (cmd/consensus/main.go mounts the shim by
// these patterns — without the entry a parent chi router answers before the
// shim sees the request), the declared route is reachable through that mount,
// and the registration introduces no /api/* catch-all. It also pins the v2
// envelope membership for the family and that the prefix registration leaves
// sibling /api/* sub-paths untouched.
func TestApiCredentialMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := false
	for _, pattern := range MountPatterns {
		if pattern == "/api/credential/*" {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("/api/credential/* is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-008)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doCredentialRequest(t, srv.URL, http.MethodPatch, "cred-github", `{"label":"renamed"}`)
	if status != http.StatusBadRequest {
		t.Errorf("chi-mounted PATCH: got %d, want the served 400 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No /api/* catch-all: an unclaimed /api/* sub-path keeps the net/http
	// default 404 it is classified with (a chi mount answers it 404 before
	// reaching any sibling route).
	for _, path := range []string{"/api/credential", "/api/bogus", "/api/credential/bogus/extra/deep"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status == http.StatusOK {
			t.Errorf("GET %s: got 200, want 404 — ROUTE-ADD-008 must not introduce an /api/* catch-all", path)
		}
	}

	// The v2 envelope membership covers the declared family shapes…
	if !isV2EnvelopeRoutePath("/api/credential/cred-github") {
		t.Error("/api/credential/cred-github is not treated as a v2-envelope route; its declared 401 body is the v2 UnauthorizedError envelope")
	}
	// …without leaking onto sibling /api/* paths that keep their own envelopes.
	if isV2EnvelopeRoutePath("/api/credential") {
		t.Error("/api/credential (no trailing id segment) must not be a v2-envelope route")
	}
	if isV2EnvelopeRoutePath("/api/model/bogus") {
		t.Error("/api/model/bogus is treated as a v2-envelope route; membership must stay scoped to the credential family plus exact paths")
	}
	for _, path := range apiV2AllPaths() {
		if !isV2EnvelopeRoutePath(path) {
			t.Errorf("%s lost its v2-envelope membership (ROUTE-ADD-021 regression)", path)
		}
	}
	if !isV2EnvelopeRoutePath("/api/agent") || !isV2EnvelopeRoutePath("/api/fs/find") {
		t.Error("the pre-existing v2 routes /api/agent and /api/fs/find lost their v2 401 envelope")
	}
}

// TestApiCredentialTraceRow pins the ch:trace declaration in the handler
// source: it names this row, its operationId, this test file and the
// unattended witness, so the traceability link cannot silently drift from the
// served surface.
func TestApiCredentialTraceRow(t *testing.T) {
	raw := readCredentialUpdateSource(t)
	want := "row=ROUTE-ADD-008 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.credential.update test=internal/shim/opencode/api_credential_update_test.go witness=none:unattended-worker-session"
	if !strings.Contains(raw, want) {
		t.Errorf("api_credential_update.go carries no ch:trace line (want %q)", want)
	}
}

func readCredentialUpdateSource(t *testing.T) string {
	t.Helper()
	// The Go test binary runs in the package directory; read the handler
	// source through the same file the package compiled.
	data, err := readFileInPackage(t, "api_credential_update.go")
	if err != nil {
		t.Skipf("api_credential_update.go not readable from the test working directory: %v", err)
	}
	return string(data)
}

func readFileInPackage(t *testing.T, name string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(name)
}
