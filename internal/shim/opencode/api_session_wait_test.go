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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// ROUTE-ADD-061 focused tests — POST /api/session/{sessionID}/wait
// (v2.session.wait; declared responses 204, 400, 401, 404, 503)
// ============================================================================

const (
	apiSessionWaitKnownID   = "9b1c4d2e-0000-4000-8000-abcdefabcde1"
	apiSessionWaitUnknownID = "00000000-0000-4000-8000-fffffffffffe"
	apiSessionWaitPath      = "/api/session/" + apiSessionWaitKnownID + "/wait"
)

// newSessionWaitStore opens a real SQLite database carrying the sessions table
// shape the handler reads (`status` per migration 001 §3.2) and returns it with
// a cleanup. The wait contract is decided from that column, so the transition
// tests must drive a store that can actually change, not a fixed mock row.
func newSessionWaitStore(t *testing.T) db.DB {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session-wait.db")
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + path, MaxOpenConns: 4})
	if err != nil {
		t.Fatalf("open session-wait database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.Exec(ctx, `CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		heartbeat_at TEXT,
		created_at TEXT
	)`); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if err := conn.Exec(ctx,
		`INSERT INTO sessions (id, status) VALUES ($1, $2)`,
		apiSessionWaitKnownID, "thinking"); err != nil {
		t.Fatalf("seed sessions row: %v", err)
	}
	return conn
}

// waitTestServer builds a shim over the given store with auth bypassed and the
// production poll interval replaced by a short one so a status transition is
// observable without making the suite slow.
func waitTestServer(store db.DB) (*Server, *httptest.Server) {
	s := NewServer(store, "test-key", nil, nil)
	s.skipAuth = true
	s.sessionWaitPollInterval = 5 * time.Millisecond
	return s, httptest.NewServer(s.Handler())
}

// TestApiSessionWaitIdleSessionAnswersDeclared204 pins the declared success arm
// for a session whose agent loop is not running: the answer is the declared
// empty 204, and it is immediate (nothing to wait for).
func TestApiSessionWaitIdleSessionAnswersDeclared204(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	start := time.Now()
	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	elapsed := time.Since(start)

	if status != http.StatusNoContent {
		t.Fatalf("POST /api/session/{id}/wait for an idle session: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 must carry no body; got %q", body)
	}
	if ct := header.Get("Content-Type"); ct != "" {
		t.Errorf("204 Content-Type = %q, want none (the declared success carries no body)", ct)
	}
	if elapsed > time.Second {
		t.Errorf("an idle session took %s to answer — the 204 must not block when no loop is running", elapsed)
	}
}

// TestApiSessionWaitResolvesTheSessionFromTheRealStore pins that the success arm
// is only reachable when the sessions store actually answered a row — the
// handler must ask the store for the agent-loop status, not assume one.
func TestApiSessionWaitResolvesTheSessionFromTheRealStore(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	if status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath); status != http.StatusNoContent {
		t.Fatalf("idle session: got %d, want 204. Body: %s", status, body)
	}
	queried := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM sessions") && strings.Contains(q, "status") {
			queried = true
		}
	}
	if !queried {
		t.Errorf("the handler never read the session status from the sessions store (queries: %v) — the declared 204 is a claim about that column", mdb.queries)
	}
}

// TestApiSessionWaitBlocksUntilRunningLoopGoesIdle is the core semantic test:
// a session whose agent loop IS running must NOT get the declared 204 up front.
// Against a real SQLite store the row is flipped to idle while the request is
// in flight, and the 204 must land only after that transition.
func TestApiSessionWaitBlocksUntilRunningLoopGoesIdle(t *testing.T) {
	store := newSessionWaitStore(t)
	_, srv := waitTestServer(store)
	defer srv.Close()

	ctx := context.Background()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = store.Exec(ctx, `UPDATE sessions SET status = 'idle' WHERE id = $1`, apiSessionWaitKnownID)
	}()

	start := time.Now()
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	elapsed := time.Since(start)

	if status != http.StatusNoContent {
		t.Fatalf("running loop that goes idle: got %d, want 204. Body: %s", status, body)
	}
	if len(body) != 0 {
		t.Errorf("204 must carry no body; got %q", body)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("answered %s after the request started — the handler did not wait for the running loop to go idle (the row was flipped at ~150ms)", elapsed)
	}
}

// TestApiSessionWaitEveryWorkingStatusBlocks pins the working set the handler
// treats as "loop not idle yet": every status the runtime's harness genuinely
// works through must block (and then answer 204 once the store settles), while
// the non-working statuses answer immediately. A handler that inverted this test
// would report "idle" for a session that is mid-planning.
func TestApiSessionWaitEveryWorkingStatusBlocks(t *testing.T) {
	working := []string{"planning", "thinking", "tool_exec", "executing", "waiting_sub"}
	for _, status := range working {
		t.Run("working/"+status, func(t *testing.T) {
			store := newSessionWaitStore(t)
			if err := store.Exec(context.Background(),
				`UPDATE sessions SET status = $1 WHERE id = $2`, status, apiSessionWaitKnownID); err != nil {
				t.Fatalf("set status %q: %v", status, err)
			}
			_, srv := waitTestServer(store)
			defer srv.Close()

			go func() {
				time.Sleep(120 * time.Millisecond)
				_ = store.Exec(context.Background(),
					`UPDATE sessions SET status = 'idle' WHERE id = $1`, apiSessionWaitKnownID)
			}()

			start := time.Now()
			code, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
			elapsed := time.Since(start)
			if code != http.StatusNoContent {
				t.Fatalf("%s -> idle: got %d, want 204. Body: %s", status, code, body)
			}
			if elapsed < 80*time.Millisecond {
				t.Errorf("%s answered in %s — a working status must block until the loop settles", status, elapsed)
			}
		})
	}

	for _, status := range []string{"idle", "paused", "completed", "failed", "booting"} {
		t.Run("settled/"+status, func(t *testing.T) {
			mdb := &mockDB{queryRow: rowOf(map[string]any{"status": status})}
			_, srv := newTestServer(mdb)
			defer srv.Close()

			start := time.Now()
			code, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
			if code != http.StatusNoContent {
				t.Fatalf("%s: got %d, want 204 (no agent loop is running). Body: %s", status, code, body)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("%s took %s — a settled session must answer immediately", status, elapsed)
			}
		})
	}
}

// TestApiSessionWaitBudgetExpiryAnswersDeclared503 pins the second use of the
// declared 5xx arm: a loop that never goes idle inside the shim's synchronous
// turn budget answers ServiceUnavailableError naming the status it last saw,
// never an invented 204.
func TestApiSessionWaitBudgetExpiryAnswersDeclared503(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"status": "thinking"})}
	s, srv := waitTestServer(mdb)
	s.messageResponseTimeout = 60 * time.Millisecond
	// Close and rebuild the listener so the shortened budget is in place before
	// the first request is served.
	srv.Close()
	srv = httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("loop that never goes idle: got %d, want 503 (the declared ServiceUnavailableError). Body: %s", status, body)
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
		t.Errorf("503 service = %v, want \"sessions\". Body: %s", got["service"], body)
	}
	msg, _ := got["message"].(string)
	if !strings.Contains(msg, "thinking") {
		t.Errorf("503 message = %q, want it to name the status last observed so a client can tell a long turn from a dead one", msg)
	}
}

// TestApiSessionWaitUnknownSessionAnswersDeclared404 pins the declared 404 arm in
// both spellings the runtime's store seam produces: a nil row and the driver's
// own "no rows in result" error.
func TestApiSessionWaitUnknownSessionAnswersDeclared404(t *testing.T) {
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

			status, header, body := doShimRequest(t, srv.URL, http.MethodPost,
				"/api/session/"+apiSessionWaitUnknownID+"/wait")
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
			if got["sessionID"] != apiSessionWaitUnknownID {
				t.Errorf("404 sessionID = %v, want %q. Body: %s", got["sessionID"], apiSessionWaitUnknownID, body)
			}
		})
	}
}

// TestApiSessionWaitSessionDeletedMidWaitAnswersDeclared404 pins that the wait
// re-resolves the session: a row deleted while a client is waiting is reported
// as gone (the declared 404), never as a settled wait.
func TestApiSessionWaitSessionDeletedMidWaitAnswersDeclared404(t *testing.T) {
	store := newSessionWaitStore(t)
	_, srv := waitTestServer(store)
	defer srv.Close()

	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = store.Exec(context.Background(),
			`DELETE FROM sessions WHERE id = $1`, apiSessionWaitKnownID)
	}()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	if status != http.StatusNotFound {
		t.Fatalf("session deleted mid-wait: got %d, want 404. Body: %s", status, body)
	}
	if !strings.Contains(string(body), "SessionNotFoundError") {
		t.Errorf("404 body does not carry the declared SessionNotFoundError: %s", body)
	}
}

// TestApiSessionWaitUnreadableStoreAnswersDeclared503 pins the first use of the
// declared 5xx arm: a store the instance cannot read answers
// ServiceUnavailableError {_tag, message, service: "sessions"}, never a claimed
// absence.
func TestApiSessionWaitUnreadableStoreAnswersDeclared503(t *testing.T) {
	t.Run("no store at all", func(t *testing.T) {
		s := NewServer(nil, "test-key", nil, nil)
		s.skipAuth = true
		s.workdir = t.TempDir()
		srv := httptest.NewServer(s.Handler())
		defer srv.Close()

		assertApiSessionWait503(t, srv.URL, apiSessionWaitPath)
	})

	t.Run("read failure that is not a missing row", func(t *testing.T) {
		_, srv := newTestServer(&mockDB{queryRowErr: errors.New("disk I/O error")})
		defer srv.Close()

		assertApiSessionWait503(t, srv.URL, apiSessionWaitPath)
	})
}

func assertApiSessionWait503(t *testing.T, baseURL, path string) {
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
		t.Errorf("503 service = %v, want \"sessions\". Body: %s", got["service"], body)
	}
	if _, ok := got["message"]; !ok {
		t.Errorf("503 body must carry the declared message member. Body: %s", body)
	}
}

// TestApiSessionWaitBlankSessionIDSegmentAnswersDeclared400 pins the declared 400
// arm for a present-but-blank path parameter.
func TestApiSessionWaitBlankSessionIDSegmentAnswersDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/api/session/%20/wait")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /api/session/%%20/wait: got %d, want 400. Body: %s", status, body)
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

// TestApiSessionWaitIgnoresUndeclaredInput pins the other half of the 400
// contract: the pinned document declares NO requestBody and NO query parameter
// for this operation, so neither can be refused.
func TestApiSessionWaitIgnoresUndeclaredInput(t *testing.T) {
	mdb := &mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequestWithBody(t, srv.URL, http.MethodPost,
		apiSessionWaitPath, `{"unexpected":"value"}`)
	if status != http.StatusNoContent {
		t.Errorf("undeclared request body: got %d, want 204 (the document declares no requestBody). Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath+"?unexpected=1")
	if status != http.StatusNoContent {
		t.Errorf("undeclared query parameter: got %d, want 204 (the document declares no query parameter). Body: %s", status, body)
	}
}

// TestApiSessionWaitUndeclaredMethodAnswers405 pins the sibling
// METHOD_NOT_ALLOWED envelope: POST is the only method the pinned document
// declares on this path.
func TestApiSessionWaitUndeclaredMethodAnswers405(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			_, srv := newTestServer(&mockDB{
				queryRow: rowOf(map[string]any{"status": "thinking"}),
			})
			defer srv.Close()

			status, _, body := doShimRequest(t, srv.URL, method, apiSessionWaitPath)
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("%s on the POST-only path: got %d, want 405 (and it must not block on the running loop). Body: %s", method, status, body)
			}
			if !strings.Contains(string(body), "METHOD_NOT_ALLOWED") {
				t.Errorf("405 body does not carry METHOD_NOT_ALLOWED: %s", body)
			}
		})
	}
}

// TestApiSessionWaitServesDeclared401 pins the declared UnauthorizedError arm:
// an unauthenticated request answers the v2 envelope rather than the shim's own
// v1 UNAUTHENTICATED body, through the exact two-segment shape admitted by
// isV2EnvelopeRoutePath.
func TestApiSessionWaitServesDeclared401(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST /api/session/{id}/wait: got %d, want 401. Body: %s", status, body)
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

// TestIsApiSessionWaitPath pins the envelope-shape predicate: only the declared
// two-segment "wait" shape joins the v2 UnauthorizedError branch.
func TestIsApiSessionWaitPath(t *testing.T) {
	cases := map[string]bool{
		"/api/session/abc/wait":    true,
		"/api/session/abc/compact": false,
		"/api/session/abc/agent":   false,
		"/api/session/abc":         false,
		"/api/session/wait":        false,
		"/api/session/abc/wait/x":  false,
		"/api/session//wait":       false,
		"/api/session/abc/wait/":   false,
		"/session/abc/wait":        false,
	}
	for path, want := range cases {
		if got := isApiSessionWaitPath(path); got != want {
			t.Errorf("isApiSessionWaitPath(%q) = %v, want %v", path, got, want)
		}
	}
	if !isV2EnvelopeRoutePath("/api/session/abc/wait") {
		t.Error("isV2EnvelopeRoutePath(/api/session/abc/wait) = false — the declared 401 would be the shim's own body, not the v2 UnauthorizedError envelope")
	}
	if isV2EnvelopeRoutePath("/api/session/abc/wait/x") {
		t.Error("isV2EnvelopeRoutePath(/api/session/abc/wait/x) = true — only the declared two-segment shape may join the v2 envelope set")
	}
}

// TestApiSessionWaitMountWiring pins the chi mount contract: the exact pattern
// must be listed in MountPatterns, the route must be reachable through that
// mount, and no catch-all may be introduced.
func TestApiSessionWaitMountWiring(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"status": "idle"})}, "test-key", nil, nil)
	s.skipAuth = true
	s.workdir = t.TempDir()

	mounted := map[string]bool{}
	for _, pattern := range MountPatterns {
		mounted[pattern] = true
	}
	if !mounted["/api/session/{sessionID}/wait"] {
		t.Errorf("/api/session/{sessionID}/wait is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-061)")
	}

	r := chi.NewRouter()
	for _, pattern := range MountPatterns {
		r.Handle(pattern, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, apiSessionWaitPath)
	if status != http.StatusNoContent {
		t.Fatalf("chi-mounted POST /api/session/{id}/wait: got %d, want 204 (the mount must reach the shim route). Body: %s", status, body)
	}

	// No catch-all: sibling /api/session/{sessionID}[...] operations keep the
	// answers their own contract rows are filed against.
	for _, path := range []string{
		"/api/session/" + apiSessionWaitKnownID + "/summarize",
		"/api/session/a/b",
	} {
		status, _, _ := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusNotFound {
			t.Errorf("POST %s: got %d, want 404 — ROUTE-ADD-061 registers the exact path, never a catch-all", path, status)
		}
	}
}

// TestApiSessionWaitTraceRow pins the ch:trace declaration of the handler.
func TestApiSessionWaitTraceRow(t *testing.T) {
	raw, err := os.ReadFile("api_session_wait.go")
	if err != nil {
		t.Fatalf("read api_session_wait.go: %v", err)
	}
	src := string(raw)
	for _, want := range []string{
		"ch:trace row=ROUTE-ADD-061",
		"spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.wait",
		"test=internal/shim/opencode/api_session_wait_test.go",
		"doc=specs/openapi/upstream/consensus-shim-served-surface.yaml",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("api_session_wait.go trace marker is missing %q", want)
		}
	}
}

// TestApiSessionWaitDeclaredVsServedFlip pins acceptance criterion 3 at the
// artifact level: the served surface declares exactly one POST
// /api/session/{sessionID}/wait row with outcome 204, and the committed
// comparison artifact classifies that operation as COVERED with
// served_outcome "204" — never as drift. Rows are keyed by
// method+path+operationId, never by the positional SHIM-DRIFT ids.
func TestApiSessionWaitDeclaredVsServedFlip(t *testing.T) {
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
		if r.Method == http.MethodPost && r.Path == "/api/session/{sessionID}/wait" {
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
		t.Fatalf("served surface enumerates %d POST /api/session/{sessionID}/wait rows, want exactly 1", servedRows)
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
		if op.Method != http.MethodPost || op.Path != "/api/session/{sessionID}/wait" {
			continue
		}
		covered++
		if op.OperationID != "v2.session.wait" {
			t.Errorf("covered row operationId = %q, want v2.session.wait", op.OperationID)
		}
		if op.ServedOutcome != "204" {
			t.Errorf("covered row served_outcome = %q, want \"204\"", op.ServedOutcome)
		}
	}
	if covered != 1 {
		t.Errorf("comparison artifact lists POST /api/session/{sessionID}/wait %d time(s) in `covered`, want exactly 1 (the SHIM-DRIFT-057 item flipped from not-served to covered)", covered)
	}
	for _, op := range doc.Drift {
		if op.Method == http.MethodPost && op.Path == "/api/session/{sessionID}/wait" {
			t.Errorf("comparison artifact still lists POST /api/session/{sessionID}/wait as drift (class %q) — the operation is served now", op.Class)
		}
	}
}
