package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (openapi-1.18.33.json paths."/api/session/active".get =
// v2.session.active, ROUTE-ADD-039): GET /api/session/active — "Retrieve
// foreground Session drains currently owned by this OpenCode process" — with
// declared responses 200 {data: {^ses: SessionActive{type: running}}} | 400
// InvalidRequestError | 401 UnauthorizedError, an EMPTY declared parameters
// list, and GET as the ONLY declared method on the path.
//
// Before this change the exact path had no shim route at all and answered
// net/http's default 404 — the declared-vs-served artifact's NOT-SERVED class,
// indistinguishable from "no such opencode operation".
//
// The 200 schema sets additionalProperties:false on the envelope and requires
// `data`, so every assertion below pins the exact member set (an undeclared
// member is a contract violation, not an enhancement).

// apiSessionActiveResponse is the declared envelope this operation answers
// with: data is a map keyed by session ids matching ^ses, each value a
// SessionActive {type: "running"}.
type apiSessionActiveResponse struct {
	Data map[string]struct {
		Type string `json:"type"`
	} `json:"data"`
}

// TestApiSessionActiveServesDeclared200 pins v2.session.active
// (ROUTE-ADD-039): the exact /api/session/active path serves the declared
// operation with the declared 200 shape instead of the pre-fix net/http
// default 404 (class NOT-SERVED: no shim route at all).
func TestApiSessionActiveServesDeclared200(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusOK {
		t.Fatalf("GET /api/session/active: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/session/active body is not JSON: %v (%s)", err, body)
	}
	// The envelope requires `data` and sets additionalProperties:false.
	assertExactMembers(t, raw, "data")

	var got apiSessionActiveResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/session/active body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Errorf("data = null, want the declared {^ses: SessionActive} object (an empty object is not null): %s", body)
	}
	if len(got.Data) != 0 {
		t.Errorf("data = %s, want the empty object — this process owns no OpenCode foreground session drains (see the handler doc comment)", body)
	}
}

// TestApiSessionActiveNeverFabricatesActiveEntries pins the truthfulness rule:
// the runtime's real sessions store IS read, but a data entry must be keyed by
// a session id matching the declared ^ses pattern while sessions.id is a
// native UUID — and upstream scopes the result to foreground Session drains
// owned by this OpenCode process, a concept the Consensus harness lifecycle
// has no counterpart for. A seeded real session — even one in an actively
// running harness state — must therefore still answer the declared empty
// object: an entry would fabricate both the identifier and the drain.
func TestApiSessionActiveNeverFabricatesActiveEntries(t *testing.T) {
	_, srv, conn := newApiSessionListServer(t)
	ctx := context.Background()

	if err := conn.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, tokens_used_in, tokens_used_out)
		 VALUES ('7a2d3f1e-1a4e-5e6f-ab62-8d3c2e4f5171', 'consensus', 'deepseek-v4-pro',
		         'thinking', 'seed a running-state session row', 800, 200)`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	// Premise: the row really is in the store in an actively-running harness
	// state, so the handler's read has something to find and its empty answer
	// is a refusal, not a missing store.
	rows, err := conn.Query(ctx, `SELECT id, status FROM sessions`)
	if err != nil {
		t.Fatalf("read seeded sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("seeded store holds %d sessions rows, want 1 (premise of the no-fabrication claim)", len(rows))
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusOK {
		t.Fatalf("GET /api/session/active with a seeded running-state session: got %d, want 200. Body: %s", status, body)
	}
	var got apiSessionActiveResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if len(got.Data) != 0 {
		t.Fatalf("data = %s, want the empty object: a sessions row is not an OpenCode foreground drain and its native UUID id is not a declared ^ses key (see the handler doc comment)", body)
	}
	// The native session id must not leak into the response as a fabricated
	// v2 identifier (neither verbatim nor with a ses prefix added).
	for _, fragment := range []string{"7a2d3f1e-1a4e-5e6f-ab62-8d3c2e4f5171", "ses7a2d3f1e", "running"} {
		if strings.Contains(string(body), fragment) {
			t.Errorf("body echoes %q — no SessionActive entry may be fabricated from a Consensus sessions row", fragment)
		}
	}
}

// TestApiSessionActiveServesDeclared401 pins the declared UnauthorizedError
// arm: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's v1 UNAUTHENTICATED
// body, which an unregistered /api/* path keeps.
func TestApiSessionActiveServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/session/active: got %d, want 401. Body: %s", status, body)
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

// TestApiSessionActiveUnreadableStoreAnswersDeclared400 pins the declared
// InvalidRequestError arm: this operation declares no 5xx code, so a store
// this instance cannot read is reported through the declared 400 instead of a
// fabricated empty active set (absence is never claimed from an unreadable
// store).
func TestApiSessionActiveUnreadableStoreAnswersDeclared400(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("no such table: sessions")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /api/session/active with an unreadable sessions store: got %d, want 400 (the declared InvalidRequestError arm). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("_tag = %v, want InvalidRequestError", got["_tag"])
	}
	if msg, _ := got["message"].(string); !strings.Contains(msg, "sessions") {
		t.Errorf("message = %q, want it to name the store that could not be read", msg)
	}
}

// TestApiSessionActiveUndeclaredMethodsAnswer405 pins the method contract:
// GET is the ONLY method the pinned document declares on
// /api/session/active, so every other method keeps the shim's
// METHOD_NOT_ALLOWED envelope (unlike sibling /api/session, no declared
// sibling method exists here to reclassify).
func TestApiSessionActiveUndeclaredMethodsAnswer405(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/session/active")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/session/active: got %d, want 405 METHOD_NOT_ALLOWED (GET is the only declared method on this path). Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s /api/session/active: 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		envelope, ok := got["error"].(map[string]any)
		if !ok {
			t.Errorf("%s /api/session/active: 405 body carries no error envelope: %s", method, body)
			continue
		}
		if envelope["code"] != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s /api/session/active: error.code = %v, want METHOD_NOT_ALLOWED (%s)", method, envelope["code"], body)
		}
	}
}

// TestApiSessionActiveIgnoresUndeclaredQuery pins the input contract boundary:
// the pinned document declares an EMPTY parameters list for this operation and
// does not set additionalProperties:false on it, so a query parameter is not
// refused — inventing a constraint would fabricate one the document does not
// state (the apiSessionListQueryValid undeclared-parameter rule). Without this
// arm a future tighten could reject undeclared input and still look "tested".
func TestApiSessionActiveIgnoresUndeclaredQuery(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	for _, query := range []string{"?anything=1", "?directory=%2Ftmp", "?workspace=wrk_abc"} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active"+query)
		if status != http.StatusOK {
			t.Errorf("GET /api/session/active%s: got %d, want 200 — the document declares no query parameters for this operation. Body: %s", query, status, body)
		}
	}
}

// TestApiSessionActiveMountWiring pins the chi mount contract:
// /api/session/active must be listed in MountPatterns (cmd/consensus/main.go
// mounts the shim by these patterns — without the entry a parent chi router
// answers before the shim sees the request), the route must be reachable
// through that mount, no catch-all may be introduced, and the declared 401
// envelope must be keyed by this exact path only.
func TestApiSessionActiveMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/session/active"] {
		t.Errorf("/api/session/active is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-039)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session/active")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/session/active: got %d, want 200 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: the declared sibling operations on
	// /api/session/{sessionID}[...] keep their own contract rows, so this
	// exact registration must not answer them.
	for _, path := range []string{"/api/session/active/extra", "/api/session/ses_abc", "/api/session/a/b"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 — ROUTE-ADD-039 registers the exact path, never a catch-all", path, status)
		}
	}

	if !isV2EnvelopeRoutePath("/api/session/active") {
		t.Error("/api/session/active is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/session/active/extra") {
		t.Error("/api/session/active/extra must not be treated as a v2-envelope route; membership stays exact-path")
	}
}

// TestApiSessionActiveTraceRow pins the ch:trace declaration of the handler:
// it names this row, its own operationId and this test file, so the
// traceability link cannot silently drift from the served surface.
func TestApiSessionActiveTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_session_active.go")
	if err != nil {
		t.Skipf("api_session_active.go not readable from the test working directory: %v", err)
	}
	for _, want := range []string{
		"ch:trace row=ROUTE-ADD-039",
		"spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.active",
		"test=internal/shim/opencode/api_session_active_test.go",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("api_session_active.go is missing the traceability fragment %q", want)
		}
	}
}
