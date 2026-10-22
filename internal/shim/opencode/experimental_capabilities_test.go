package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ROUTE-ADD-064 / source item SHIM-DRIFT-032 (the board brief cited
// SHIM-DRIFT-061, which this artifact assigns to a different operation): the
// exact GET /experimental/capabilities path had NO shim route at all, so it
// answered net/http's default 404 — the declared-vs-served artifact's
// NOT-SERVED class, indistinguishable from "no such opencode operation". The
// pinned document declares experimental.capabilities.get with responses 200
// (ExperimentalCapabilities: {backgroundSubagents*: boolean},
// additionalProperties false) and 400 (BadRequestError), plus optional
// directory/workspace query selectors.
//
// These tests pin the served contract: the declared 200 (red before the route
// was registered), the declared 400 for a present-but-blank declared selector,
// the sibling 405 method guard, chi-mount reachability, and that the
// neighbouring /experimental/* surfaces are untouched (no catch-all).
//
// ch:trace row=ROUTE-ADD-064 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.capabilities.get test=TestExperimentalCapabilitiesAnswersDeclared200 evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:unattended-worker-session

// TestExperimentalCapabilitiesAnswersDeclared200 asserts the happy path: GET
// /experimental/capabilities answers the declared 200 with the single declared
// member. Before ROUTE-ADD-064 the path fell through to the net/http default
// 404; the test is kept as the regression pin for the registration itself.
func TestExperimentalCapabilitiesAnswersDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/capabilities")
	if status != http.StatusOK {
		t.Fatalf("GET /experimental/capabilities: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	// The declared schema requires exactly one member (backgroundSubagents) and
	// is additionalProperties: false, so decode strictly into that shape and
	// refuse anything else.
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("GET /experimental/capabilities body must be a JSON object, got %q: %v", body, err)
	}
	if len(got) != 1 {
		t.Errorf("GET /experimental/capabilities = %v, want exactly one member (backgroundSubagents)", got)
	}
	value, ok := got["backgroundSubagents"]
	if !ok {
		t.Fatalf("GET /experimental/capabilities = %v, want the required backgroundSubagents member", got)
	}
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("backgroundSubagents = %v (%T), want a boolean", value, value)
	}
	// The value is a build-level capability answer, not a per-request one: the
	// Consensus runtime implements sub-agent spawning (SPEC-004) — a spawn
	// queues a task that the harness claim loop executes asynchronously, with
	// sessions.parent_id lineage, the wake_parent_on_completion trigger and the
	// subagent_max_depth setting (default 5) — so the truthful answer is true.
	if !flag {
		t.Errorf("backgroundSubagents = false, want true: this runtime implements sub-agent spawning (SPEC-004, internal/subagent)")
	}
}

// TestExperimentalCapabilitiesBlankDeclaredParamIsBadRequest pins the declared
// 400 arm: a present-but-blank declared query selector (directory, workspace)
// is malformed input and answers the DECLARED BadRequestError body
// ({name: "BadRequest", data: {kind: "Query", message}}), never 200 and never
// the pre-fix 404. A valued selector is well-formed and keeps the 200 (the
// capability answer is workspace-independent).
func TestExperimentalCapabilitiesBlankDeclaredParamIsBadRequest(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		"/experimental/capabilities?directory=",
		"/experimental/capabilities?workspace=",
	} {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}
		var got struct {
			Name string `json:"name"`
			Data struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("GET %s: body not the declared BadRequestError envelope: %v (%s)", path, err, body)
		}
		if got.Name != "BadRequest" {
			t.Errorf("GET %s: name = %q, want BadRequest (the declared BadRequestError schema)", path, got.Name)
		}
		if got.Data.Kind != "Query" {
			t.Errorf("GET %s: data.kind = %q, want Query (the declared enum member)", path, got.Data.Kind)
		}
		if got.Data.Message == "" {
			t.Errorf("GET %s: data.message empty, want the offending param named", path)
		}
	}

	// A valued selector is well-formed and must not be refused.
	status, _, body := doShimRequest(t, srv.URL, http.MethodGet,
		"/experimental/capabilities?directory=%2Ftmp%2Fws&workspace=wrk_0000000000000000000000000000")
	if status != http.StatusOK {
		t.Errorf("GET /experimental/capabilities with valued selectors: got %d, want 200. Body: %s", status, body)
	}
}

// TestExperimentalCapabilitiesMethodGuard pins the undeclared-method arm:
// /experimental/capabilities is an exact registered route, so a non-GET request
// reaches the handler and answers the sibling METHOD_NOT_ALLOWED envelope —
// never the net/http default 404 the pre-fix tree answered.
func TestExperimentalCapabilitiesMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, body := doShimRequest(t, srv.URL, method, "/experimental/capabilities")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s /experimental/capabilities: got %d, want 405. Body: %s", method, status, body)
		}
	}
}

// TestExperimentalCapabilitiesNeighboursUntouched is the non-vacuity control:
// serving the exact /experimental/capabilities path must not turn the
// /experimental/* family into a catch-all. The declared tool routes keep their
// own handlers and an equally deep undeclared sibling still answers the default
// 404.
func TestExperimentalCapabilitiesNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/experimental/tool")
	if status != http.StatusOK {
		t.Errorf("GET /experimental/tool: got %d, want 200 (sibling route unchanged). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/tool/ids")
	if status != http.StatusOK {
		t.Errorf("GET /experimental/tool/ids: got %d, want 200 (sibling route unchanged). Body: %s", status, body)
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/capabilities/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /experimental/capabilities/unknown-sub: got %d, want 404 (unregistered sub-path)", status)
	}

	status, _, _ = doShimRequest(t, srv.URL, http.MethodGet, "/experimental/not-a-declared-op")
	if status != http.StatusNotFound {
		t.Errorf("GET /experimental/not-a-declared-op: got %d, want 404 (no /experimental catch-all)", status)
	}
}

// TestExperimentalCapabilitiesChiMount is the BUG-009 regression for the new
// route: a parent chi router mounting the shim patterns must reach
// GET /experimental/capabilities (a shim-produced 200 proves the wiring;
// chi's 404 would mean the mount lost the family).
func TestExperimentalCapabilitiesChiMount(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/experimental/capabilities")
	if err != nil {
		t.Fatalf("GET /experimental/capabilities via chi mount: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /experimental/capabilities via chi mount: got %d, want 200 — the /experimental/* mount pattern must reach the shim route",
			resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("chi-mounted /experimental/capabilities body must be a JSON object: %v", err)
	}
	if _, ok := got["backgroundSubagents"]; !ok {
		t.Errorf("chi-mounted /experimental/capabilities = %v, want the declared backgroundSubagents member", got)
	}
}
