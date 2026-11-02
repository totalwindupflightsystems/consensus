// Package opencode: ROUTE-ADD-074 contract tests for GET /experimental/session
// (upstream operationId experimental.session.list, declared responses
// 200 GlobalSession[] | 400 BadRequestError).
//
// ch:trace row=ROUTE-ADD-074 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.session.list test=internal/shim/opencode/api_experimental_session_test.go witness=none:unattended-worker-session
//
// Before this change the exact path had no shim route at all, so every method
// fell through to net/http's default 404 — the declared-vs-served comparison's
// NOT-SERVED class (source item SHIM-DRIFT-014 in the drift appendix),
// indistinguishable from "no such opencode operation". These tests pin the
// post-change contract arm by arm.
package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

const experimentalSessionPath = "/experimental/session"

// TestExperimentalSessionGetServesDeclared200 pins the happy path against a
// store that HOLDS session rows: the shim answers the declared 200
// GlobalSession[] with the truthful empty, non-null collection. The rows are
// real, but no sessions row can be reshaped into a GlobalSession (required id
// must match ^ses while sessions.id is a native UUID; slug, projectID,
// directory, title, version and time.updated have no truthful counterparts —
// the ROUTE-ADD-037 v2.session.list reshape-refusal precedent), and the
// declared success must be a JSON array, never null. The 200 must come AFTER a
// real read of the sessions store, not from a hardcoded answer.
func TestExperimentalSessionGetServesDeclared200(t *testing.T) {
	mdb := &mockDB{queryResults: []db.Row{
		rowOf(map[string]any{"id": "6f9619ff-8b86-d011-b42d-00c04fc964ff"}),
	}}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, experimentalSessionPath)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", experimentalSessionPath, status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var sessions []any
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatalf("body is not a JSON array (the declared GlobalSession[]): %v (%s)", err, body)
	}
	if len(sessions) != 0 {
		t.Errorf("GET %s: body = %s, want the truthful empty array — no sessions row can be truthfully reshaped into a GlobalSession", experimentalSessionPath, body)
	}
	if len(mdb.queries) == 0 {
		t.Error("no sessions-store read was issued — the 200 must be derived from a real read, not hardcoded")
	} else if !strings.Contains(mdb.queries[0], "FROM sessions") {
		t.Errorf("first query = %q, want a read of the real sessions store", mdb.queries[0])
	}
}

// TestExperimentalSessionGetValuedParamsKeep200 pins the declared query
// parameters: every parameter the document declares is optional, so well-formed
// values — string selectors, the boolean/string-enum forms of roots/archived,
// numeric start/cursor/limit (this operation's cursor is a plain declared
// number, NOT an opaque server-issued token), a free-form search — are
// well-formed input and keep the declared 200.
func TestExperimentalSessionGetValuedParamsKeep200(t *testing.T) {
	for _, path := range []string{
		experimentalSessionPath + "?directory=/tmp/ws",
		experimentalSessionPath + "?workspace=ws1",
		experimentalSessionPath + "?roots=true",
		experimentalSessionPath + "?roots=false",
		experimentalSessionPath + "?archived=true",
		experimentalSessionPath + "?start=0",
		experimentalSessionPath + "?cursor=25",
		experimentalSessionPath + "?limit=50",
		experimentalSessionPath + "?search=anything%20goes",
		experimentalSessionPath + "?directory=/tmp/ws&archived=false&limit=10&start=5&cursor=5",
	} {
		_, srv := newTestServer(&mockDB{})
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		srv.Close()
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (a well-formed declared parameter is not malformed input). Body: %s", path, status, body)
		}
	}
}

// TestExperimentalSessionGetMalformedSelectorIsDeclared400 pins the declared
// 400 arm for the plain-string selectors: a present-but-blank directory or
// workspace is malformed input and answers the declared BadRequestError body
// {name:"BadRequest", data:{kind,message}} (sibling convention,
// handleExperimentalConsole). Malformed input must not read the store at all.
func TestExperimentalSessionGetMalformedSelectorIsDeclared400(t *testing.T) {
	for _, path := range []string{
		experimentalSessionPath + "?directory=",
		experimentalSessionPath + "?workspace=",
		experimentalSessionPath + "?directory=%20",
		experimentalSessionPath + "?workspace=%20",
	} {
		mdb := &mockDB{}
		_, srv := newTestServer(mdb)
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		srv.Close()

		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}
		assertBadRequestEnvelope(t, path, body, "Query")
		if len(mdb.queries) != 0 {
			t.Errorf("GET %s: malformed input must not read the sessions store; queries: %v", path, mdb.queries)
		}
	}
}

// TestExperimentalSessionGetMalformedBooleanParamsAreDeclared400 pins the
// roots/archived schema: the parameter is declared anyOf boolean | string enum
// ["true","false"], and a query value arrives as text, so exactly "true" and
// "false" are well-formed. A blank, "1", "yes" or "TRUE" (the enum is
// case-sensitive) matches neither declared arm and answers the declared 400.
func TestExperimentalSessionGetMalformedBooleanParamsAreDeclared400(t *testing.T) {
	for _, path := range []string{
		experimentalSessionPath + "?roots=",
		experimentalSessionPath + "?roots=1",
		experimentalSessionPath + "?roots=yes",
		experimentalSessionPath + "?roots=TRUE",
		experimentalSessionPath + "?archived=",
		experimentalSessionPath + "?archived=0",
		experimentalSessionPath + "?archived=TRUE",
	} {
		mdb := &mockDB{}
		_, srv := newTestServer(mdb)
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		srv.Close()

		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		assertBadRequestEnvelope(t, path, body, "Query")
		if len(mdb.queries) != 0 {
			t.Errorf("GET %s: malformed input must not read the sessions store; queries: %v", path, mdb.queries)
		}
	}
}

// TestExperimentalSessionGetMalformedNumberParamsAreDeclared400 pins the
// numeric parameters start/cursor/limit: declared type number, so a blank or
// non-numeric value is malformed input and answers the declared 400.
func TestExperimentalSessionGetMalformedNumberParamsAreDeclared400(t *testing.T) {
	for _, path := range []string{
		experimentalSessionPath + "?start=",
		experimentalSessionPath + "?start=abc",
		experimentalSessionPath + "?cursor=",
		experimentalSessionPath + "?cursor=yesterday",
		experimentalSessionPath + "?limit=",
		experimentalSessionPath + "?limit=1.2.3",
	} {
		mdb := &mockDB{}
		_, srv := newTestServer(mdb)
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		srv.Close()

		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		assertBadRequestEnvelope(t, path, body, "Query")
		if len(mdb.queries) != 0 {
			t.Errorf("GET %s: malformed input must not read the sessions store; queries: %v", path, mdb.queries)
		}
	}
}

// TestExperimentalSessionGetUnreadableStoreIsDeclared400 pins the store-failure
// arm: the served list is derived from the runtime's real sessions store, so an
// unreadable store must not be reported as an empty list — absence is never
// claimed from a store the shim could not read. The operation declares no 5xx
// arm (only 200 and 400), so the failure answers the DECLARED 400
// BadRequestError (the handleApiSessionList no-5xx convention), never an
// undeclared 500.
func TestExperimentalSessionGetUnreadableStoreIsDeclared400(t *testing.T) {
	mdb := &mockDB{queryErr: errors.New("sessions store unavailable")}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, experimentalSessionPath)
	if status != http.StatusBadRequest {
		t.Fatalf("GET %s with an unreadable store: got %d, want 400 (the operation declares no 5xx arm). Body: %s", experimentalSessionPath, status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	assertBadRequestEnvelope(t, experimentalSessionPath, body, "")
	if len(mdb.queries) == 0 {
		t.Error("no sessions-store read was issued — the 400 arm must come from a real read failure")
	}
}

// TestExperimentalSessionGetMethodGuard pins the sibling method-guard
// convention: only GET is declared on this exact path, so every other method
// answers the 405 METHOD_NOT_ALLOWED envelope (never a catch-all, never a
// lying 404).
func TestExperimentalSessionGetMethodGuard(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		_, srv := newTestServer(&mockDB{})
		status, header, body := doShimRequest(t, srv.URL, method, experimentalSessionPath)
		srv.Close()

		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405. Body: %s", method, experimentalSessionPath, status, body)
			continue
		}
		if ct := header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", method, experimentalSessionPath, ct)
		}
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("%s %s: body is not the error envelope: %v (%s)", method, experimentalSessionPath, err, body)
			continue
		}
		if envelope.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s: error.code = %q, want METHOD_NOT_ALLOWED", method, experimentalSessionPath, envelope.Error.Code)
		}
	}
}

// assertBadRequestEnvelope decodes the declared BadRequestError body
// ({name:"BadRequest", data:{message, kind?}}) and asserts its shape: the
// declared name enum, a non-empty reason, and — when wantKind is non-empty —
// the declared data.kind carrying the input class.
func assertBadRequestEnvelope(t *testing.T, path string, body []byte, wantKind string) {
	t.Helper()
	var bad struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &bad); err != nil {
		t.Fatalf("GET %s: body is not the declared BadRequestError shape: %v (%s)", path, err, body)
	}
	if bad.Name != "BadRequest" {
		t.Errorf("GET %s: name = %q, want BadRequest (declared enum)", path, bad.Name)
	}
	if bad.Data.Message == "" {
		t.Errorf("GET %s: empty data.message, want the malformed-input or store-failure reason", path)
	}
	if wantKind != "" && bad.Data.Kind != wantKind {
		t.Errorf("GET %s: data.kind = %q, want %q", path, bad.Data.Kind, wantKind)
	}
}
