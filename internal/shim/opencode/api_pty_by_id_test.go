package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Upstream contract (openapi-1.18.33.json paths."/api/pty/{ptyID}" —
// v2.pty.get / v2.pty.update / v2.pty.remove, ROUTE-ADD-031 / ROUTE-ADD-032 /
// ROUTE-ADD-030): the three declared operations share the exact one-segment
// path, each declaring the error vocabulary 400 BadRequestError, 401
// UnauthorizedError, 404 PtyNotFoundError — pty.get/pty.update declare a
// declared 200 Pty body, pty.remove declares 204, and none of those is
// reachable: this runtime keeps NO pty registry and spawns no pseudo-terminals
// (the v1 /pty/{ptyID} truthfulness rule in pty_routes.go), so the served
// surface is the declared error half. Before this change the path had no shim
// route at all and answered net/http's default 404 for every method (the
// declared-vs-served artifact's NOT-SERVED class; positional id SHIM-DRIFT-001
// on DELETE v2.pty.remove). The one registration dispatches on r.Method; the
// undeclared-method answer stays the pre-change net/http 404, never a 405
// (405 is in none of the three declared response sets).
//
// ch:trace row=ROUTE-ADD-030 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.remove test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
// ch:trace row=ROUTE-ADD-031 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.get test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree
// ch:trace row=ROUTE-ADD-032 spec=specs/openapi/upstream/openapi-1.18.33.json#pty.update test=internal/shim/opencode/api_pty_by_id_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=commit witness=none:self-verified-in-worktree

// newApiPtyByIDServer builds the ROUTE-ADD-030/031/032 fixture. The handler
// reads no store — the runtime keeps no pty registry (the declared 404 IS the
// truthful answer) — so the mock DB is sufficient; auth is bypassed so the
// 400/404 arms are exercised directly. The served workspace is a fresh temp
// dir so a blank selector answer is the malformed-input branch, not a
// foreign-workspace refusal (the newPtyRouteServer convention).
func newApiPtyByIDServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	served := t.TempDir()
	s.workdir = served
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, served
}

// TestApiPtyByIDServesDeclared404And400 pins the served error contract per
// declared operation on /api/pty/{ptyID}: an ill-shaped ptyID answers the
// declared 400 BadRequestError with kind Params (the declared ^pty path
// pattern), a blank directory selector answers the declared 400 with kind
// Query (the shared ptyList selector rule), and every well-formed ptyID
// answers the declared 404 typed PtyNotFoundError {_tag, ptyID, message} —
// never a fabricated 200 Pty body or a 204 claiming a removal that never
// happened (no pty registry exists; the handlePtyByID v1 precedent). The
// declared BadRequestError wire shape is the NamedError envelope
// {name: "BadRequest", data: {kind, message}} — shared assertBadRequestKind.
func TestApiPtyByIDServesDeclared404And400(t *testing.T) {
	srv, served := newApiPtyByIDServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+"/well-formed-id", func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, method, "/api/pty/pty_abc")
			if status != http.StatusNotFound {
				t.Fatalf("%s /api/pty/pty_abc: got %d, want the declared 404. Body: %s", method, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertPtyNotFound(t, method+" /api/pty/pty_abc", body, "pty_abc")
		})
	}

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+"/ill-shaped-id", func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, method, "/api/pty/nope")
			if status != http.StatusBadRequest {
				t.Fatalf("%s /api/pty/nope: got %d, want the declared 400 kind Params. Body: %s", method, status, body)
			}
			assertBadRequestKind(t, method+" /api/pty/nope", body, "Params")
		})
	}

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+"/blank-directory-selector", func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, method, "/api/pty/pty_abc?directory=")
			if status != http.StatusBadRequest {
				t.Fatalf("%s /api/pty/pty_abc?directory=: got %d, want the declared 400 kind Query. Body: %s", method, status, body)
			}
			assertBadRequestKind(t, method+" /api/pty/pty_abc?directory=", body, "Query")
		})
		t.Run(method+"/served-directory-selector", func(t *testing.T) {
			// The served workspace directory is accepted (not malformed), so
			// the request proceeds to the lookup: the declared 404 typed
			// PtyNotFoundError — never a selector refusal.
			status, _, body := doShimRequest(t, srv.URL, method, "/api/pty/pty_abc?directory="+served)
			if status != http.StatusNotFound {
				t.Fatalf("%s /api/pty/pty_abc?directory=<served>: got %d, want the declared 404 (the served selector is accepted). Body: %s", method, status, body)
			}
			assertPtyNotFound(t, method+" /api/pty/pty_abc?directory=<served>", body, "pty_abc")
		})
	}
}

// TestApiPtyByIDUndeclaredMethodKeepsDefault404 pins the pre-change answer for
// an undeclared method (e.g. POST /api/pty/{ptyID}): net/http's default 404,
// NOT a 405 — 405 is not in any of the three declared response sets on this
// path (the ptyMethodNotRouted convention the v1 family keeps).
func TestApiPtyByIDUndeclaredMethodKeepsDefault404(t *testing.T) {
	srv, _ := newApiPtyByIDServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method, "/api/pty/pty_abc")
		if status != http.StatusNotFound {
			t.Errorf("%s /api/pty/pty_abc: got %d, want the pre-change default 404 (never a 405). Body: %s", method, status, body)
		}
		if strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
			t.Errorf("%s /api/pty/pty_abc body carries the 405 envelope; want the pre-change default 404: %s", method, body)
		}
	}
}

// TestApiPtyByIDRouteResolvesToTheByIDHandler proves the mux registration
// resolves /api/pty/pty_abc to the ptyID handler (and not the exact /api/pty
// list handler): a well-formed id answers the declared typed
// PtyNotFoundError 404, while the unregistered two-segment shape
// /api/pty/pty_abc/sub keeps the net/http default 404 of the /api/pty/*
// catch-all-free surface — and the exact /api/pty mount still serves its own
// declared 200 list.
func TestApiPtyByIDRouteResolvesToTheByIDHandler(t *testing.T) {
	srv, _ := newApiPtyByIDServer(t)

	// The one-segment path reaches the ptyID handler: the declared typed body
	// (PtyNotFoundError) is the shape only handleApiPtyByID's delegating arms
	// emit — the exact /api/pty list handler answers 200 with {location, data}.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty/pty_abc")
	if status != http.StatusNotFound {
		t.Fatalf("GET /api/pty/pty_abc: got %d, want 404 from the ptyID handler. Body: %s", status, body)
	}
	var got map[string]any
	assertPtyNotFound(t, "GET /api/pty/pty_abc (route resolution)", body, "pty_abc")
	if _, ok := got["location"]; ok {
		t.Error(`404 body carries "location" — resolved to the v2 list envelope, not the ptyID handler`)
	}

	// The exact mount is untouched: the declared 200 list still answers there.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/api/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /api/pty: got %d, want 200 — the exact list route must be untouched. Body: %s", status, body)
	}
	if !strings.Contains(string(body), `"location"`) || !strings.Contains(string(body), `"data"`) {
		t.Errorf("GET /api/pty body lost the declared list envelope: %s", body)
	}

	// Deeper sub-paths keep the catch-all-free default 404 (no /api/pty/*
	// catch-all is introduced), and the v1 family is untouched.
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/api/pty/pty_abc/sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/pty/pty_abc/sub: got %d, want the net/http default 404 (no /api/pty/* catch-all)", status)
	}
	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Errorf("GET /pty (v1 pty.list): got %d, want 200 — the v1 route must be untouched", status)
	}

	// The ptyID path is in the v2 envelope membership (it declares the v2
	// UnauthorizedError 401 arm). ROUTE-ADD-033 widened that membership to the
	// declared sibling operation v2.pty.connect (GET
	// /api/pty/{ptyID}/connect): it declares the same v2 UnauthorizedError 401
	// arm and its concrete two-segment shape is registered, so the same api-key
	// wall answers the same envelope. Membership stays SHAPE-bounded, never a
	// prefix rule — an unregistered /api/pty/* sub-path (the connect-token
	// sibling ROUTE-ADD-034 has yet to serve, any deeper segment) still keeps
	// the shim's own envelope, and a blank ptyID segment is not a path.
	if !isV2EnvelopeRoutePath("/api/pty/pty_abc") {
		t.Error("/api/pty/pty_abc is not a v2EnvelopeRoutePath member — the declared ptyID family carries the v2 401 arm")
	}
	if !isV2EnvelopeRoutePath("/api/pty/pty_abc/connect") {
		t.Error("/api/pty/pty_abc/connect is not a v2EnvelopeRoutePath member — the declared v2.pty.connect operation carries the v2 UnauthorizedError 401 arm and its shape is registered (ROUTE-ADD-033)")
	}
	for _, path := range []string{
		"/api/pty/pty_abc/sub",
		"/api/pty/pty_abc/connect-token",
		"/api/pty//connect",
		"/api/pty/a/b/connect",
	} {
		if isV2EnvelopeRoutePath(path) {
			t.Errorf("%s joins the v2 envelope; membership must stay the registered shapes only (one ptyID segment, optionally the /connect literal)", path)
		}
	}
}

// TestApiPtyByIDServeDeclared401 pins the declared UnauthorizedError arm:
// unauthenticated requests answer the v2 envelope
// ({_tag: "UnauthorizedError", message}) — never the shim's UNAUTHENTICATED
// body, which an unregistered /api/* path keeps. All three declared
// operations on the path declare the 401.
func TestApiPtyByIDServeDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, header, body := doShimRequest(t, srv.URL, method, "/api/pty/pty_abc")
		if status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s /api/pty/pty_abc: got %d, want 401. Body: %s", method, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("401 body is not JSON: %v (%s)", err, body)
		}
		if got["_tag"] != "UnauthorizedError" {
			t.Errorf("_tag = %v, want UnauthorizedError (the declared v2 401 body, not the shim's UNAUTHENTICATED envelope). Body: %s", got["_tag"], body)
		}
		if got["message"] == "" {
			t.Error("401 body carries no message")
		}
		if _, ok := got["error"]; ok {
			t.Errorf("401 body carries the shim's UNAUTHENTICATED envelope member: %s", body)
		}
	}

	// ROUTE-ADD-033: the declared v2.pty.connect operation on the registered
	// two-segment sub-path declares the same 401 arm, so the envelope
	// membership must be the envelope the auth wall really writes — not merely
	// a set entry the route can drift away from.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/api/pty/pty_abc/connect")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/pty/pty_abc/connect: got %d, want 401. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("_tag = %v, want UnauthorizedError (the declared v2 401 body on the v2.pty.connect sub-path). Body: %s", got["_tag"], body)
	}
	if _, ok := got["error"]; ok {
		t.Errorf("401 body carries the shim's UNAUTHENTICATED envelope member: %s", body)
	}
}

// TestApiPtyByIDTraceRow pins the ch:trace declaration of the handler: it
// names the three rows of the served family, this test file and the served
// surface, so the traceability link cannot silently drift from the served
// surface (the TestApiPtyListTraceRow convention).
func TestApiPtyByIDTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_pty_by_id.go")
	if err != nil {
		t.Skipf("api_pty_by_id.go not readable from the test working directory: %v", err)
	}
	source := string(raw)
	for _, tc := range []struct{ row, fragment string }{
		{"ROUTE-ADD-030", "pty.remove"},
		{"ROUTE-ADD-031", "pty.get"},
		{"ROUTE-ADD-032", "pty.update"},
	} {
		want := "row=" + tc.row + " spec=specs/openapi/upstream/openapi-1.18.33.json#" + tc.fragment +
			" test=internal/shim/opencode/api_pty_by_id_test.go"
		if !strings.Contains(source, want) {
			t.Errorf("api_pty_by_id.go carries no ch:trace line for %s (want %q)", tc.row, want)
		}
	}
	if !strings.Contains(source, "evidence=commit") {
		t.Error("api_pty_by_id.go does not record the commit evidence pointer")
	}
	if !strings.Contains(source, "witness=none:self-verified-in-worktree") {
		t.Error("api_pty_by_id.go does not record the self-verified witness")
	}
}
