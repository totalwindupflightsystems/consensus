package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream contract (specs/openapi/upstream/openapi-1.18.33.json
// paths."/experimental/project/{projectID}/copy/refresh".post =
// v2.projectCopy.refresh): POST /experimental/project/{projectID}/copy/refresh,
// required plain-string path parameter projectID (no pattern), optional
// deepObject location selector (directory, workspace;
// additionalProperties:false), NO requestBody, and exactly two declared
// responses: 204 No Content and 400 ProjectCopyError | InvalidRequestError.
//
// ROUTE-ADD-072 serves the contract: the runtime keeps NO project-copy
// registry (the ROUTE-ADD-069 remove / ROUTE-ADD-070 create reading,
// unchanged), so refresh is the idempotent no-op of the sibling DELETE — the
// declared empty 204 fabricates neither a stored copy nor a refresh claim
// about one, and the operation declares no 404 arm an unknown id could
// answer. Before the fix the exact path had no shim route at all, so every
// request answered net/http's default 404 (the artifact's NOT-SERVED class).
//
// RED-pin: against the pre-change source every request below answers the
// net/http default 404, so TestExperimentalProjectCopyRefreshServesDeclared204
// fails on the status and the auth/malformed arms fail on answering 404
// instead of the declared envelopes. The battery pins the declared 204
// (idempotent, body-less), the declared 400 family (path + query), the v2
// UnauthorizedError 401, the 405 for undeclared methods, the exact-path
// registration under the real chi mount, and the sibling DELETE's untouched
// no-op.

func newProjectCopyRefreshServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestExperimentalProjectCopyRefreshServesDeclared204 pins the declared
// success arm: a well-formed POST answers the empty 204 and the operation is
// the idempotent no-op of the sibling DELETE (a second identical POST answers
// the same, and no body is ever written — 204 has no body by definition).
func TestExperimentalProjectCopyRefreshServesDeclared204(t *testing.T) {
	srv := newProjectCopyRefreshServer(t)
	base := "/experimental/project/prj-refresh-204/copy/refresh"

	for attempt := 1; attempt <= 2; attempt++ {
		status, _, body := doCopyRequest(t, srv.URL, http.MethodPost, base, "")
		if status != http.StatusNoContent {
			t.Fatalf("POST refresh (attempt %d): got %d, want the declared 204. Body: %s",
				attempt, status, body)
		}
		if len(body) != 0 {
			t.Errorf("POST refresh (attempt %d): body = %q, want empty (204 has no body)", attempt, body)
		}
	}
}

// TestExperimentalProjectCopyRefreshAuth pins the declared 401 arm: the exact
// path keeps the api-key wall and the v2 UnauthorizedError envelope (the
// v2.* operation rule shared with the sibling DELETE/POST through the
// <non-empty-id>/copy/refresh shape rule in isV2EnvelopeRoutePath).
func TestExperimentalProjectCopyRefreshAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-refresh-auth/copy/refresh", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST refresh: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("401 body is not JSON: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" || strings.TrimSpace(toString(got["message"])) == "" {
		t.Errorf("401 body = %s, want the v2 {_tag: UnauthorizedError, message} envelope", body)
	}
}

// TestExperimentalProjectCopyRefreshMalformedAnswersDeclared400 pins the
// declared 400 arm for every malformed input the pinned document defines:
// the only ill-shaped projectID the contract itself defines (blank, kind
// Path) and the malformed deepObject location selectors (kind Query, the
// shared apiV2LocationSelector verdict). A well-formed request still answers
// the declared 204 first, so the 400 pins are not vacuous.
func TestExperimentalProjectCopyRefreshMalformedAnswersDeclared400(t *testing.T) {
	srv := newProjectCopyRefreshServer(t)
	base := "/experimental/project/prj-refresh-400/copy/refresh"

	// Well-formed baseline (so a pass is not vacuous).
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodPost, base, "")
	if status != http.StatusNoContent {
		t.Fatalf("well-formed baseline: got %d, want 204. Body: %s", status, resp)
	}

	// Present-but-blank path parameter (kind Path).
	status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/%20/copy/refresh", "")
	assertCopyInvalidRequest(t, "blank projectID", status, resp, "Path", "projectID")

	// Malformed deepObject location selectors (kind Query).
	for _, query := range []string{
		"?location[nope]=1",
		"?location[directory]=",
		"?location[workspace]=notawk",
	} {
		status, _, resp = doCopyRequest(t, srv.URL, http.MethodPost, base+query, "")
		assertCopyInvalidRequest(t, "query "+query, status, resp, "Query", "")
	}
}

// TestExperimentalProjectCopyRefreshNoCatchAll pins the exact-path
// registration under the real chi mount (MountPatterns): the declared POST
// reaches the shim route, undeclared methods on the exact path answer the
// sibling METHOD_NOT_ALLOWED 405 envelope, the remaining declared /copy
// sibling (/copy/generate-name) and undeclared shapes keep the net/http
// default 404 their own contract rows are filed against, and the sibling
// DELETE keeps its declared 204 no-op byte-for-byte (no regression in the
// shared /experimental/* mount).
func TestExperimentalProjectCopyRefreshNoCatchAll(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()
	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	// The declared POST through the parent mount (the wiring the deployment
	// actually uses).
	status, _, body := doCopyRequest(t, srv.URL, http.MethodPost,
		"/experimental/project/prj-refresh-wired/copy/refresh", "")
	if status != http.StatusNoContent {
		t.Fatalf("mounted POST refresh: got %d, want the declared 204. Body: %s", status, body)
	}

	// Undeclared methods on the exact path keep the sibling 405 envelope.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body = doCopyRequest(t, srv.URL, method,
			"/experimental/project/prj-refresh-wired/copy/refresh", "")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s refresh: got %d, want 405 (POST is the only declared method). Body: %s",
				method, status, body)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil || got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s refresh body = %s, want the METHOD_NOT_ALLOWED envelope", method, body)
		}
	}

	// No catch-all: the remaining declared /copy sibling and undeclared
	// shapes keep the net/http default 404 they are classified with.
	for _, path := range []string{
		"/experimental/project/prj-refresh-wired/copy/generate-name",
		"/experimental/project/prj-refresh-wired/copyx",
		"/experimental/project/prj-refresh-wired/copy/deeper",
	} {
		status, _, _ := doCopyRequest(t, srv.URL, http.MethodPost, path, "")
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 (exact declared path only)", path, status)
		}
	}

	// The sibling DELETE keeps its untouched declared 204 no-op.
	status, _, resp := doCopyRequest(t, srv.URL, http.MethodDelete,
		"/experimental/project/prj-refresh-wired/copy", "")
	if status != http.StatusNoContent {
		t.Fatalf("sibling DELETE: got %d, want the untouched 204. Body: %s", status, resp)
	}
	if len(resp) != 0 {
		t.Errorf("sibling DELETE body = %q, want empty", resp)
	}
}
