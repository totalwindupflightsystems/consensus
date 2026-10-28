package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// ROUTE-ADD-042 focused tests — POST /api/session/{sessionID}/compact
// (v2.session.compact; declared responses 204, 400, 401, 404, 503)
// ============================================================================

const (
	apiSessionCompactKnownID   = "9b1c4d2e-0000-4000-8000-abcdefabcdef"
	apiSessionCompactUnknownID = "00000000-0000-4000-8000-ffffffffffff"
	apiSessionCompactPath      = "/api/session/" + apiSessionCompactKnownID + "/compact"
)

// TestApiSessionCompactKnownSessionAnswersDeclared204 pins the declared success
// arm: a session the instance holds answers the declared empty 204 — and the
// success stays bodyless, because the declared 204 carries no body and the
// runtime's compaction pipeline is not addressable from this adapter (the
// handler doc comment).
func TestApiSessionCompactKnownSessionAnswersDeclared204(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionCompactKnownID})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath)
	if status != http.StatusNoContent {
		t.Fatalf("POST /api/session/{id}/compact for a known session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 must carry no body; got %q", body)
	}
	if ct := header.Get("Content-Type"); ct != "" {
		t.Errorf("204 Content-Type = %q, want none (the declared success carries no body)", ct)
	}
}

// TestApiSessionCompactResolvesTheSessionFromTheRealStore pins that the success
// arm is only reachable when the sessions store actually answered a row — the
// handler must ask the store, not assume acceptance.
func TestApiSessionCompactResolvesTheSessionFromTheRealStore(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionCompactKnownID})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	if status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath); status != http.StatusNoContent {
		t.Fatalf("known session: got %d, want 204. Body: %s", status, body)
	}
	queried := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM sessions") {
			queried = true
		}
	}
	if !queried {
		t.Errorf("the handler never read the sessions store (queries: %v) — the declared 404/503 arms must be decided from it", mdb.queries)
	}
}

// TestApiSessionCompactUnknownSessionAnswersDeclared404 pins the declared 404
// arm in both spellings the runtime's store seam produces: a nil row and the
// driver's own "no rows in result" error. The body is the declared
// SessionNotFoundError {_tag, sessionID, message}.
func TestApiSessionCompactUnknownSessionAnswersDeclared404(t *testing.T) {
	cases := []struct {
		name string
		mdb  *mockDB
	}{
		{"nil row", &mockDB{}},
		{"no rows in result", &mockDB{queryRowErr: errors.New("sqlite: no rows in result")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(tc.mdb)
			defer srv.Close()

			status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath)
			if status != http.StatusNotFound {
				t.Fatalf("unknown session (%s): got %d, want 404. Body: %s", tc.name, status, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode 404 body: %v (%s)", err, body)
			}
			if got["_tag"] != "SessionNotFoundError" {
				t.Errorf("404 _tag = %v, want SessionNotFoundError. Body: %s", got["_tag"], body)
			}
			if got["sessionID"] != apiSessionCompactKnownID {
				t.Errorf("404 sessionID = %v, want %q. Body: %s", got["sessionID"], apiSessionCompactKnownID, body)
			}
			if _, ok := got["message"]; !ok {
				t.Errorf("404 body must carry the declared message member. Body: %s", body)
			}
		})
	}
}

// TestApiSessionCompactUnreadableStoreAnswersDeclared503 pins the declared 5xx
// arm — the one arm this operation declares and its siblings do not: a store the
// instance cannot read answers ServiceUnavailableError {_tag, message, service:
// "sessions"}, never the declared 404 (absence is not claimed from a store that
// could not be read) and never an undeclared 400/500.
func TestApiSessionCompactUnreadableStoreAnswersDeclared503(t *testing.T) {
	t.Run("no store at all", func(t *testing.T) {
		s := NewServer(nil, "test-key", nil, nil)
		s.skipAuth = true
		s.workdir = t.TempDir()
		srv := httptest.NewServer(s.Handler())
		defer srv.Close()

		assertApiSessionCompact503(t, srv.URL, apiSessionCompactPath)
	})

	t.Run("read failure that is not a missing row", func(t *testing.T) {
		_, srv := newTestServer(&mockDB{queryRowErr: errors.New("disk I/O error")})
		defer srv.Close()

		assertApiSessionCompact503(t, srv.URL, apiSessionCompactPath)
	})
}

func assertApiSessionCompact503(t *testing.T, baseURL, path string) {
	t.Helper()
	status, header, body := doShimRequest(t, baseURL, http.MethodPost, path)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("unreadable sessions store: got %d, want 503 (the declared ServiceUnavailableError). Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 503 body: %v (%s)", err, body)
	}
	if got["_tag"] != "ServiceUnavailableError" {
		t.Errorf("503 _tag = %v, want ServiceUnavailableError. Body: %s", got["_tag"], body)
	}
	if got["service"] != "sessions" {
		t.Errorf("503 service = %v, want \"sessions\" (the declared optional service member names the unavailable component). Body: %s", got["service"], body)
	}
	if _, ok := got["message"]; !ok {
		t.Errorf("503 body must carry the declared message member. Body: %s", body)
	}
}

// TestApiSessionCompactBlankSessionIDSegmentAnswersDeclared400 pins the declared
// 400 arm for a present-but-blank path parameter (a blank segment is the only
// malformed spelling of a plain required string).
func TestApiSessionCompactBlankSessionIDSegmentAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/%20/compact")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session/%%20/compact: got %d, want 400. Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 400 body: %v (%s)", err, body)
	}
	if got["_tag"] != "InvalidRequestError" {
		t.Errorf("400 _tag = %v, want InvalidRequestError. Body: %s", got["_tag"], body)
	}
	if got["kind"] != "Path" || got["field"] != "sessionID" {
		t.Errorf("400 kind/field = %v/%v, want Path/sessionID. Body: %s", got["kind"], got["field"], body)
	}
}

// TestApiSessionCompactIgnoresUndeclaredInput pins the other half of the 400
// contract: the pinned document declares NO requestBody and NO query parameter
// for this operation, so a handler cannot reject either — an unexpected body and
// an unexpected query string both leave the declared arms intact (the
// handleApiSessionActive convention). A known session still answers 204.
func TestApiSessionCompactIgnoresUndeclaredInput(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"id": apiSessionCompactKnownID})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		apiSessionCompactPath, `{"unexpected":"value"}`)
	if status != http.StatusNoContent {
		t.Errorf("undeclared request body: got %d, want 204 (the document declares no requestBody). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath+"?unexpected=1")
	if status != http.StatusNoContent {
		t.Errorf("undeclared query parameter: got %d, want 204 (the document declares no query parameter). Body: %s", status, body)
	}
}

// TestApiSessionCompactUndeclaredMethodAnswers405 pins the sibling
// METHOD_NOT_ALLOWED envelope: POST is the only method the pinned document
// declares on this path.
func TestApiSessionCompactUndeclaredMethodAnswers405(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			_, srv := newTestServer(&mockDB{})
			defer srv.Close()

			status, _, body := doShimRequest(t, srv.URL, method, apiSessionCompactPath)
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("%s on the POST-only path: got %d, want 405. Body: %s", method, status, body)
			}
			if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
				t.Errorf("405 body does not carry METHOD_NOT_ALLOWED: %s", body)
			}
		})
	}
}

// TestApiSessionCompactServesDeclared401 pins the declared UnauthorizedError
// arm: an unauthenticated request answers the v2 envelope
// ({_tag: "UnauthorizedError", message}) rather than the shim's own v1
// UNAUTHENTICATED body, through the exact two-segment shape admitted by
// isV2EnvelopeRoutePath.
func TestApiSessionCompactServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/session/{id}/compact: got %d, want 401. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 401 body: %v (%s)", err, body)
	}
	if got["_tag"] != "UnauthorizedError" {
		t.Errorf("401 _tag = %v, want UnauthorizedError (the v2 envelope). Body: %s", got["_tag"], body)
	}
}

// TestIsApiSessionCompactPath pins the envelope-shape predicate: only the
// declared two-segment "compact" shape joins the v2 UnauthorizedError branch;
// sibling session paths and deeper sub-paths keep the shim's own envelope.
func TestIsApiSessionCompactPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/compact":   true,
		"/api/session/abc/agent":     false,
		"/api/session/abc/model":     false,
		"/api/session/abc":           false,
		"/api/session/active":        false,
		"/api/session/abc/compact/x": false,
		"/api/session//compact":      false,
		"/api/session/abc/compact/":  false,
		"/api/provider":              false,
		"/session/abc/compact":       false,
	}
	for path, want := range cases {
		if got := isApiSessionCompactPath(path); got != want {
			t.Errorf("isApiSessionCompactPath(%q) = %v, want %v", path, got, want)
		}
	}
	if !isV2EnvelopeRoutePath("/api/session/abc/compact") {
		t.Error("isV2EnvelopeRoutePath(/api/session/abc/compact) = false — the declared 401 would be the shim's own body, not the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/session/abc/compact/x") {
		t.Error("isV2EnvelopeRoutePath(/api/session/abc/compact/x) = true — only the declared two-segment shape may join the v2 envelope set")
	}
}

// TestApiSessionCompactMountWiring pins the chi mount contract: the exact
// pattern must be listed in MountPatterns (a parent chi mount otherwise answers
// before the shim sees the request), the route must be reachable through that
// mount, and no catch-all may be introduced — sibling /api/session/{sessionID}
// operations keep their own answers.
func TestApiSessionCompactMountWiring(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": apiSessionCompactKnownID})}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/session/{sessionID}/compact"] {
		t.Errorf("/api/session/{sessionID}/compact is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-042)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionCompactPath)
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted POST /api/session/{id}/compact: got %d, want 204 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: the sibling declared /api/session/{sessionID}[...]
	// operations keep the answers their own contract rows are filed against.
	for _, path := range []string{"/api/session/" + apiSessionCompactKnownID + "/summarize", "/api/session/a/b"} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 — ROUTE-ADD-042 registers the exact path, never a catch-all", path, status)
		}
	}
}

// TestApiSessionCompactTraceRow pins the ch:trace declaration of the handler: it
// names this row, its own operationId and this test file, so the traceability
// link cannot silently drift from the served surface.
func TestApiSessionCompactTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_session_compact.go")
	if err != nil {
		t.Fatalf("read api_session_compact.go: %v", err)
	}
	src := string(raw)
	for _, want := range []string{
		"ch:trace row=ROUTE-ADD-042",
		"spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.compact",
		"test=internal/shim/opencode/api_session_compact_test.go",
		"doc=specs/openapi/upstream/consensus-shim-served-surface.yaml",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("api_session_compact.go trace marker is missing %q", want)
		}
	}
}

// TestApiSessionCompactDeclaredVsServedFlip pins acceptance criterion 3 at the
// artifact level: the served surface declares exactly one POST
// /api/session/{sessionID}/compact row with outcome 204, and the committed
// comparison artifact classifies that operation as COVERED with
// served_outcome "204" — never as drift. The rows are keyed by
// method+path+operationId, never by the positional SHIM-DRIFT ids (the
// comparator renumbers them on every regeneration).
func TestApiSessionCompactDeclaredVsServedFlip(t *testing.T) {
	servedRaw, err := os.ReadFile(filepath.FromSlash("../../../specs/openapi/upstream/consensus-shim-served-surface.yaml"))
	if err != nil {
		t.Fatalf("read served surface: %v", err)
	}
	var served struct {
		Routes []struct {
			Path    string `yaml:"path"`
			Method  string `yaml:"method"`
			Outcome string `yaml:"outcome"`
			Auth    string `yaml:"auth"`
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(servedRaw, &served); err != nil {
		t.Fatalf("decode served surface: %v", err)
	}
	servedRows := 0
	for _, r := range served.Routes {
		if r.Method == http.MethodPost && r.Path == "/api/session/{sessionID}/compact" {
			servedRows++
			if r.Outcome != "204" {
				t.Errorf("served-surface row outcome = %q, want \"204\" (the declared success code)", r.Outcome)
			}
			if r.Auth != "api-key" {
				t.Errorf("served-surface row auth = %q, want api-key (the /api/* auth wall)", r.Auth)
			}
		}
	}
	if servedRows != 1 {
		t.Fatalf("served surface enumerates %d POST /api/session/{sessionID}/compact rows, want exactly 1", servedRows)
	}

	artifactRaw, err := os.ReadFile(filepath.FromSlash("../../../specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json"))
	if err != nil {
		t.Fatalf("read comparison artifact: %v", err)
	}
	var doc struct {
		Covered []consistencyOp `json:"covered"`
		Drift   []consistencyOp `json:"drift_declared_not_served"`
	}
	if err := json.Unmarshal(artifactRaw, &doc); err != nil {
		t.Fatalf("decode comparison artifact: %v", err)
	}
	covered := 0
	for _, op := range doc.Covered {
		if op.Method != http.MethodPost || op.Path != "/api/session/{sessionID}/compact" {
			continue
		}
		covered++
		if op.OperationID != "v2.session.compact" {
			t.Errorf("covered row operationId = %q, want v2.session.compact", op.OperationID)
		}
		if op.ServedOutcome != "204" {
			t.Errorf("covered row served_outcome = %q, want \"204\"", op.ServedOutcome)
		}
	}
	if covered != 1 {
		t.Errorf("comparison artifact lists POST /api/session/{sessionID}/compact %d time(s) in `covered`, want exactly 1 (SHIM-DRIFT-* item for this operation flipped from not-served to covered)", covered)
	}
	for _, op := range doc.Drift {
		if op.Method == http.MethodPost && op.Path == "/api/session/{sessionID}/compact" {
			t.Errorf("comparison artifact still lists POST /api/session/{sessionID}/compact as drift (class %q) — the operation is served now", op.Class)
		}
	}
}
