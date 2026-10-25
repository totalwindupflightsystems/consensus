package opencode

import (
	"context"
	"encoding/json"
	"errors"
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

// Upstream contract (openapi-1.18.33.json paths."/api/session".get =
// v2.session.list, ROUTE-ADD-037): GET /api/session, declared responses 200
// SessionsResponse {data: SessionV2Info[], cursor} | 400 InvalidCursorError |
// 400 InvalidRequestError | 401 UnauthorizedError, with the optional declared
// query parameters workspace (pattern ^wrk), limit (number), order (enum
// asc|desc), search, directory, project, subpath and cursor.
//
// Before this change the exact path had no shim route at all and answered
// net/http's default 404 — the declared-vs-served artifact's NOT-SERVED class,
// indistinguishable from "no such opencode operation".
//
// Both 200 members are REQUIRED by the pinned schema and the schema sets
// additionalProperties:false, so every assertion below pins the exact member
// set (an undeclared member is a contract violation, not an enhancement).

// apiSessionListResponse is the declared SessionsResponse envelope this
// operation answers with.
type apiSessionListResponse struct {
	Data   []json.RawMessage `json:"data"`
	Cursor struct {
		Previous string `json:"previous"`
		Next     string `json:"next"`
	} `json:"cursor"`
}

// newApiSessionListServer builds the ROUTE-ADD-037 fixture: a real SQLite
// database carrying the runtime's sessions store (migration 001 §3.2, the table
// the v1 /session routes and the harness read), auth bypassed, so the 200/400
// arms are exercised against a real store rather than a mock.
func newApiSessionListServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "api-session-list.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open api session list test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// migration 001 §3.2 sessions: the real store the handler reads. The
	// columns the declared SessionV2Info would need (a ^ses id, a v2 projectID,
	// monetary cost, reasoning/cache token split, an updated timestamp) do not
	// exist here — that absence is exactly why the declared 200 answers the
	// empty collection (see the handler doc comment).
	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id UUID PRIMARY KEY,
			parent_id UUID,
			agent_name TEXT NOT NULL,
			model_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'idle',
			goal TEXT,
			tokens_used_in BIGINT NOT NULL DEFAULT 0,
			tokens_used_out BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP
		)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare api session list test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, conn
}

// TestApiSessionListServesDeclared200 pins v2.session.list (ROUTE-ADD-037): the
// exact /api/session path serves the declared operation with the declared 200
// shape instead of the pre-fix net/http default 404 (class NOT-SERVED: no shim
// route at all).
func TestApiSessionListServesDeclared200(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session")
	if status != http.StatusOK {
		t.Fatalf("GET /api/session: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("GET /api/session body is not JSON: %v (%s)", err, body)
	}
	// SessionsResponse requires both members and sets additionalProperties:false.
	assertExactMembers(t, raw, "data", "cursor")

	var got apiSessionListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /api/session body is not the declared envelope: %v (%s)", err, body)
	}
	if got.Data == nil {
		t.Errorf("data = null, want the declared SessionV2Info[] array (an empty array is not null): %s", body)
	}
	if len(got.Data) != 0 {
		t.Errorf("data = %s, want the empty array — no store row can be built into a SessionV2Info without inventing its required members", body)
	}
	if cursor, ok := raw["cursor"].(map[string]any); !ok || cursor == nil {
		t.Errorf("cursor = %v, want the declared required object ({} when no cursor is issued): %s", raw["cursor"], body)
	}
}

// TestApiSessionListNeverFabricatesSessionEntries pins the truthfulness rule:
// the runtime's real sessions store IS read, but a SessionV2Info requires a
// ^ses-matching id, a v2 projectID, a monetary cost, reasoning/cache token
// splits, an updated timestamp, a title and a workspace directory, none of
// which a sessions row carries. A seeded real session must therefore still
// answer the declared empty array — a reshaped row would fabricate the very
// identifier a client then addresses.
func TestApiSessionListNeverFabricatesSessionEntries(t *testing.T) {
	_, srv, conn := newApiSessionListServer(t)
	ctx := context.Background()

	if err := conn.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, tokens_used_in, tokens_used_out)
		 VALUES ('6f1c2f9e-0f3d-4d5e-9a51-7c2b1d3e4f60', 'consensus', 'deepseek-v4-pro',
		         'idle', 'seed a real session row', 1200, 400)`); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}

	// Premise: the row really is in the store, so the handler's read has
	// something to find and its empty answer is a refusal, not a missing store.
	rows, err := conn.Query(ctx, `SELECT id FROM sessions`)
	if err != nil {
		t.Fatalf("read seeded sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("seeded store holds %d sessions rows, want 1 (premise of the no-fabrication claim)", len(rows))
	}

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session")
	if status != http.StatusOK {
		t.Fatalf("GET /api/session with a seeded store: got %d, want 200. Body: %s", status, body)
	}
	var got apiSessionListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not the declared envelope: %v (%s)", err, body)
	}
	if len(got.Data) != 0 {
		t.Fatalf("data = %s, want the empty array: a sessions row carries none of the SessionV2Info required members (see the handler doc comment)", body)
	}
	// The native session id must not leak into the response as a fabricated
	// v2 identifier (neither verbatim nor with a ses prefix added).
	for _, fragment := range []string{"6f1c2f9e-0f3d-4d5e-9a51-7c2b1d3e4f60", "ses6f1c2f9e"} {
		if strings.Contains(string(body), fragment) {
			t.Errorf("body echoes %q — a native session id is not a declared ^ses SessionV2Info id", fragment)
		}
	}
	// A truthful empty list must not claim a cursor it never issued.
	if strings.Contains(string(body), `"previous"`) || strings.Contains(string(body), `"next"`) {
		t.Errorf("body carries a pagination cursor this surface never issued: %s", body)
	}
}

// TestApiSessionListServeDeclared401 pins the declared UnauthorizedError arm: an
// unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's v1 UNAUTHENTICATED
// body, which an unregistered /api/* path keeps.
func TestApiSessionListServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/session: got %d, want 401. Body: %s", status, body)
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

// TestApiSessionListMalformedQueryAnswersDeclared400 pins the declared 400 arms
// for every declared query parameter a client can malform: InvalidRequestError
// for a value violating the declared schema, InvalidCursorError for a cursor
// this surface never issued.
func TestApiSessionListMalformedQueryAnswersDeclared400(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	cases := []struct {
		name    string
		query   string
		wantTag string
		wantKey string
	}{
		{"blank workspace", "?workspace=%20", "InvalidRequestError", "workspace"},
		{"workspace violates ^wrk", "?workspace=abc123", "InvalidRequestError", "workspace"},
		{"order outside enum", "?order=sideways", "InvalidRequestError", "order"},
		{"limit not a number", "?limit=ten", "InvalidRequestError", "limit"},
		{"blank limit", "?limit=%20", "InvalidRequestError", "limit"},
		{"blank directory", "?directory=%20", "InvalidRequestError", "directory"},
		{"blank project", "?project=%20", "InvalidRequestError", "project"},
		{"blank subpath", "?subpath=%20", "InvalidRequestError", "subpath"},
		{"unissued cursor", "?cursor=abc", "InvalidCursorError", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session"+tc.query)
			if status != http.StatusBadRequest {
				t.Fatalf("GET /api/session%s: got %d, want 400 (declared arm). Body: %s", tc.query, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("%s: 400 body is not JSON: %v (%s)", tc.query, err, body)
			}
			if got["_tag"] != tc.wantTag {
				t.Errorf("%s: _tag = %v, want %s (the declared 400 body)", tc.query, got["_tag"], tc.wantTag)
			}
			if tc.wantKey != "" && got["field"] != tc.wantKey {
				t.Errorf("%s: field = %v, want %q naming the offending query parameter (%s)", tc.query, got["field"], tc.wantKey, body)
			}
			if _, ok := got["message"].(string); !ok {
				t.Errorf("%s: message is not a string: %s", tc.query, body)
			}
		})
	}
}

// TestApiSessionListWellFormedQueryAnswersDeclared200 is the negative arm of the
// validation tests above: a declaration-conforming value for every declared
// parameter (an empty `search` included — the document declares no pattern,
// enum or minLength for it) must NOT be refused. Without this arm a future
// tighten could reject the whole query contract and still look "tested".
func TestApiSessionListWellFormedQueryAnswersDeclared200(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	for _, query := range []string{
		"?workspace=wrk_abc",
		"?limit=10",
		"?order=asc",
		"?order=desc",
		"?search=",
		"?search=needle",
		"?directory=" + "/tmp",
		"?project=proj_1",
		"?subpath=sub/dir",
		"?workspace=wrk_abc&limit=25&order=desc&search=x&directory=%2Ftmp&project=proj_1&subpath=sub%2Fdir",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session"+query)
		if status != http.StatusOK {
			t.Errorf("GET /api/session%s: got %d, want 200 — the value conforms to the declared schema. Body: %s", query, status, body)
		}
	}
}

// TestApiSessionListUnreadableStoreAnswersDeclared400 pins the store arm: this
// operation declares no 5xx code, so a store this instance cannot read is
// reported through the declared InvalidRequestError 400 instead of a fabricated
// empty session list.
func TestApiSessionListUnreadableStoreAnswersDeclared400(t *testing.T) {
	s := NewServer(&mockDB{queryErr: errors.New("no such table: sessions")}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /api/session with an unreadable sessions store: got %d, want 400 (the declared InvalidRequestError arm). Body: %s", status, body)
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

// TestApiSessionListPostAnswersTyped501NamingSessionCreate pins the declared
// sibling method: POST /api/session is v2.session.create — a SEPARATE declared
// operation with its own contract row (ROUTE-ADD-038). Because the exact path is
// registered, that method reaches this handler, and SHIM-GAP-002 forbids a
// registered route from answering a lying 404/405 as if the operation did not
// exist: it answers the typed not_implemented envelope naming the operation.
func TestApiSessionListPostAnswersTyped501NamingSessionCreate(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session")
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /api/session: got %d, want 501 (typed not_implemented narrowing). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("POST /api/session: 501 body is not JSON: %v (%s)", err, body)
	}
	if got["error"] != "not_implemented" {
		t.Errorf("error = %v, want not_implemented", got["error"])
	}
	if got["operation"] != "v2.session.create" {
		t.Errorf("operation = %v, want v2.session.create (the declared operation this path exposes)", got["operation"])
	}
	if detail, _ := got["detail"].(string); strings.TrimSpace(detail) == "" {
		t.Error("detail must name what is missing; got empty")
	}
}

// TestApiSessionListUndeclaredMethodsAnswer405 pins the sibling method
// contract for the methods the pinned document does not declare on this path at
// all: they keep the shim's METHOD_NOT_ALLOWED envelope rather than a lying
// empty success (POST is excluded — it is a declared operation and answers the
// typed 501 narrowing above).
func TestApiSessionListUndeclaredMethodsAnswer405(t *testing.T) {
	_, srv, _ := newApiSessionListServer(t)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/session")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/session: got %d, want 405 METHOD_NOT_ALLOWED (v2.session.create is a separate declared operation). Body: %s", method, status, body)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("%s /api/session: 405 body is not JSON: %v (%s)", method, err, body)
			continue
		}
		envelope, ok := got["error"].(map[string]any)
		if !ok {
			t.Errorf("%s /api/session: 405 body carries no error envelope: %s", method, body)
			continue
		}
		if envelope["code"] != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s /api/session: error.code = %v, want METHOD_NOT_ALLOWED (%s)", method, envelope["code"], body)
		}
	}
}

// TestApiSessionListMountWiring pins the chi mount contract: /api/session must
// be listed in MountPatterns (cmd/consensus/main.go mounts the shim by these
// patterns — without the entry a parent chi router answers before the shim sees
// the request), the route must be reachable through that mount, no
// /api/session/* catch-all may be introduced, and the declared 401 envelope
// must be keyed by this exact path only.
func TestApiSessionListMountWiring(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/session"] {
		t.Errorf("/api/session is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-037)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/session")
	if status != http.StatusOK {
		t.Fatalf("chi-mounted GET /api/session: got %d, want 200 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: the sibling declared operations on /api/session/{sessionID}
	// (v2.session.get/active/prompt/...) keep their own contract rows, so this
	// exact registration must not answer them.
	for _, path := range []string{"/api/session/active", "/api/session/ses_abc", "/api/session/a/b"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404 — ROUTE-ADD-037 registers the exact path, never a catch-all", path, status)
		}
	}

	if !isV2EnvelopeRoutePath("/api/session") {
		t.Error("/api/session is not listed in v2EnvelopeRoutePaths — its declared 401 body is the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/session/active") {
		t.Error("/api/session/active is treated as a v2-envelope route; membership must stay exact-path until its own row lands")
	}
	if isV2EnvelopeRoutePath("/api/sessions") {
		t.Error("/api/sessions must not be treated as a v2-envelope route")
	}
	// The v1 /session family is a different declared operation set and must stay
	// outside the v2 envelope set.
	if isV2EnvelopeRoutePath("/session") {
		t.Error("/session (v1 session.list) must not join the v2 envelope set")
	}
}

// TestApiSessionListTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId and this test file, so the traceability
// link cannot silently drift from the served surface.
func TestApiSessionListTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_session_routes.go")
	if err != nil {
		t.Skipf("api_session_routes.go not readable from the test working directory: %v", err)
	}
	for _, want := range []string{
		"ch:trace row=ROUTE-ADD-037",
		"spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.list",
		"test=internal/shim/opencode/api_session_routes_test.go",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("api_session_routes.go is missing the traceability fragment %q", want)
		}
	}
}
