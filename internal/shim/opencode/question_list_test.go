package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// Upstream contract (openapi-1.18.33.json paths."/question".get =
// question.list, SHIM-DRIFT-113): GET /question, optional `directory` and
// `workspace` query params (plain strings, no declared constraints), responses
// 200 QuestionRequest[] ("List of pending questions") and 400 BadRequestError.
//
// QuestionRequest requires id (pattern ^que), sessionID (pattern ^ses) and a
// questions array of QuestionInfo (multiple-choice prompts with labeled
// options). Consensus has no producer for that shape: the HITL surface is
// approval_requests (request_type/risk_level vocabulary, SPEC-014), nothing in
// the shim or native API writes question rows, and an approval row cannot be
// truthfully reshaped into a QuestionRequest (no header/options). The shim
// therefore answers the declared 200 with an empty list — never a fabricated
// queue — and answers the declared 400 arm the way sibling handlers do
// (writeOpencodeError INVALID_REQUEST) when the store cannot be read.

// TestQuestionList answers upstream question.list: GET /question must return
// 200 with the declared QuestionRequest[] JSON array — instead of the pre-fix
// net/http default 404 (SHIM-DRIFT-113 was class NOT-SERVED: no route at all).
func TestQuestionList(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"id": "apr_pending", "status": "pending"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/question")
	if status != http.StatusOK {
		t.Fatalf("GET /question: got %d, want 200. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got []any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be the declared QuestionRequest[] array, got %q: %v", body, err)
	}
	if len(got) != 0 {
		t.Fatalf("shim has no upstream question producer; body must be [], got %v", got)
	}

	sawStoreRead := false
	for _, q := range mdb.queries {
		if strings.Contains(q, "FROM approval_requests") {
			sawStoreRead = true
		}
	}
	if !sawStoreRead {
		t.Errorf("expected a pending-approval store read backing the empty answer, queries: %v", mdb.queries)
	}
}

// TestQuestionListEmptyStore pins the empty-store arm: the declared 200 is an
// array, so an empty store answers [] (never null, never an error).
func TestQuestionListEmptyStore(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/question")
	if status != http.StatusOK {
		t.Fatalf("GET /question on empty store: got %d, want 200. Body: %s", status, body)
	}
	if body := strings.TrimSpace(string(body)); body != "[]" {
		t.Errorf("empty store body = %q, want []", body)
	}
}

// TestQuestionListPendingApprovalsDoNotFabricateQuestions pins the truthful
// 200: pending approval_requests rows exist in the store, but the shim must
// NOT reshape them into upstream QuestionRequest entries (id ^que + labeled
// options) — the answer stays the empty declared array.
func TestQuestionListPendingApprovalsDoNotFabricateQuestions(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{
				"id": "apr_1", "session_id": "ses_1", "request_type": "custom",
				"risk_level": "low", "description": "I want to drop the temp_cache table",
				"status": "pending", "created_at": "2026-09-30T00:00:00Z",
			}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/question")
	if status != http.StatusOK {
		t.Fatalf("GET /question with pending approvals: got %d, want 200. Body: %s", status, body)
	}
	var got []map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body must be a JSON array: %v (%s)", err, body)
	}
	if len(got) != 0 {
		t.Errorf("approval_requests rows must not be reshaped into QuestionRequest entries, got %v", got)
	}
}

// TestQuestionListParamsAccepted pins the declared optional query params: the
// upstream `directory` / `workspace` params are unconstrained strings, so a
// request carrying them is not malformed and must answer the declared 200.
func TestQuestionListParamsAccepted(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/question?directory=/tmp/ws&workspace=/tmp/ws")
	if status != http.StatusOK {
		t.Fatalf("GET /question with declared params: got %d, want 200. Body: %s", status, body)
	}
	if body := strings.TrimSpace(string(body)); body != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

// TestQuestionListDBFailure answers the declared 400 arm the way sibling
// handlers do (writeOpencodeError INVALID_REQUEST): a store that cannot be
// read must not surface a 500 or an empty 200.
func TestQuestionListDBFailure(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryErr: context.DeadlineExceeded})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/question")
	if status != http.StatusBadRequest {
		t.Fatalf("GET /question with failing store: got %d, want 400. Body: %s", status, body)
	}
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("400 body is not JSON: %v (%s)", err, body)
	}
	if got.Error.Code != "INVALID_REQUEST" {
		t.Errorf("error.code = %q, want INVALID_REQUEST", got.Error.Code)
	}
	if strings.TrimSpace(got.Error.Message) == "" {
		t.Errorf("error.message empty, want a reason")
	}
}

// TestQuestionListMethodNotAllowed pins the method guard: 405 is not part of
// question.list's declared set, but the handler must answer the sibling
// METHOD_NOT_ALLOWED envelope rather than silently reading the store.
func TestQuestionListMethodNotAllowed(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, srv.URL+"/question", nil)
		if err != nil {
			t.Fatalf("build %s: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /question: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s /question: got %d, want 405 METHOD_NOT_ALLOWED", method, resp.StatusCode)
		}
	}
}

// TestQuestionSubRoutesUntouched guards the neighbours: the new exact
// /question registration must not change the /question/{requestID} actions —
// reply/reject keep their id validation, typed 404 and method guard.
func TestQuestionSubRoutesUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	// GET on an action sub-path is not the declared POST → generic not-found.
	status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, "/question/que_x/reply")
	if status != http.StatusNotFound {
		t.Errorf("GET /question/que_x/reply: got %d, want 404 (sub-path method guard)", status)
	}

	// Non-que id → 400 INVALID_REQUEST (sub-path id validation, unchanged).
	status, _, body := doShimRequest(t, srv.URL, http.MethodPost, "/question/bad/reply")
	if status != http.StatusBadRequest {
		t.Errorf("POST /question/bad/reply: got %d, want 400. Body: %s", status, body)
	}

	// Well-formed id → typed QuestionNotFoundError (unchanged).
	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/question/que_missing/reject")
	if status != http.StatusNotFound {
		t.Errorf("POST /question/que_missing/reject: got %d, want 404. Body: %s", status, body)
	}
	var typed map[string]any
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("typed 404 body is not JSON: %v (%s)", err, body)
	}
	if typed["_tag"] != "QuestionNotFoundError" || typed["requestID"] != "que_missing" {
		t.Errorf("typed error = %v, want _tag=QuestionNotFoundError requestID=que_missing", typed)
	}
}

// TestQuestionListRealStore round-trips the handler against a real SQLite
// database containing a real approval_requests table with a pending row: the
// store read succeeds, so the answer is the declared 200 with the empty list.
func TestQuestionListRealStore(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "question.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open question test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE approval_requests (
			id TEXT PRIMARY KEY,
			session_id TEXT,
			request_type TEXT NOT NULL,
			description TEXT,
			risk_level TEXT NOT NULL DEFAULT 'low',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			deleted_at TEXT
		)`,
		`CREATE TABLE api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL,
			key_prefix TEXT,
			scope TEXT NOT NULL,
			session_id TEXT,
			expires_at TEXT
		)`,
		`INSERT INTO approval_requests (id, session_id, request_type, description, risk_level, status)
		 VALUES ('apr_1', 'ses_1', 'custom', 'drop temp_cache', 'low', 'pending')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare question test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/question")
	if status != http.StatusOK {
		t.Fatalf("GET /question on real store: got %d, want 200. Body: %s", status, body)
	}
	var got []any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, body)
	}
	if len(got) != 0 {
		t.Errorf("real pending approvals must not fabricate QuestionRequest entries, got %v", got)
	}
}
