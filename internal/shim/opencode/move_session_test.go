package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// moveSessionPath is the exact upstream path for
// experimental.controlPlane.moveSession (ROUTE-ADD-068, source item
// SHIM-DRIFT-036 before the regeneration).
const moveSessionPath = "/experimental/control-plane/move-session"

// assertMoveSessionInvalidRequest pins the declared InvalidRequestError arm
// (upstream components.schemas.InvalidRequestError: {_tag*: "InvalidRequestError",
// message*, kind?, field?} additionalProperties:false) — never the shim's own
// {"error":{code,message}} envelope, which the declared union does not contain.
func assertMoveSessionInvalidRequest(t *testing.T, body string, data []byte) {
	t.Helper()
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("body is not a JSON object: %v (%s)", err, body)
	}
	for field := range got {
		switch field {
		case "_tag", "message", "kind", "field":
		default:
			t.Errorf("unexpected field %q — the declared InvalidRequestError schema is additionalProperties:false (%s)", field, body)
		}
	}
	var tag string
	if err := json.Unmarshal(got["_tag"], &tag); err != nil || tag != "InvalidRequestError" {
		t.Errorf("_tag = %q (%v), want InvalidRequestError (%s)", tag, err, body)
	}
	var msg string
	if err := json.Unmarshal(got["message"], &msg); err != nil || strings.TrimSpace(msg) == "" {
		t.Errorf("message = %q (%v), want a non-empty sentence naming the offending input (%s)", msg, err, body)
	}
}

// assertMoveSessionError pins the declared MoveSessionError arm
// (components.schemas.MoveSessionError: {name*: "MoveSessionError",
// data*: {message*}} additionalProperties:false) — the shape the upstream SDK's
// throwOnError path reads.
func assertMoveSessionError(t *testing.T, body string, data []byte) {
	t.Helper()
	var got struct {
		Name string `json:"name"`
		Data *struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("body is not the MoveSessionError shape: %v (%s)", err, body)
	}
	if got.Name != "MoveSessionError" {
		t.Errorf("name = %q, want MoveSessionError (%s)", got.Name, body)
	}
	if got.Data == nil || strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("data.message = %v, want a non-empty sentence (%s)", got.Data, body)
	}
}

// TestMoveSessionServesDeclaredContract answers upstream
// experimental.controlPlane.moveSession: POST
// /experimental/control-plane/move-session must answer the declared 204 for a
// well-formed request whose destination is the single workspace this instance
// serves (the session already resides there, so the declared "Session moved" is
// the truthful idempotent no-op) — instead of the pre-fix net/http default 404.
// The declared 400 arms stay reachable (pinned by the sibling tests).
func TestMoveSessionServesDeclaredContract(t *testing.T) {
	ws := t.TempDir()
	s, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	s.workdir = ws
	defer srv.Close()

	for _, tc := range []struct {
		name, body, header string
	}{
		{
			name: "declared minimal body",
			body: `{"sessionID":"s1","destination":{"directory":` + jsonQuote(ws) + `}}`,
		},
		{
			name: "declared optional moveChanges",
			body: `{"sessionID":"s1","destination":{"directory":` + jsonQuote(ws) + `},"moveChanges":true}`,
		},
		{
			name:   "destination selected by the fixed-workspace header",
			body:   `{"sessionID":"s1","destination":{"directory":` + jsonQuote(ws) + `}}`,
			header: ws,
		},
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+moveSessionPath, strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("%s: build request: %v", tc.name, err)
		}
		if tc.header != "" {
			req.Header.Set("x-opencode-directory", tc.header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%s: read body: %v", tc.name, err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("%s: POST %s (%s): got %d, want the declared 204. Body: %s",
				tc.name, moveSessionPath, tc.body, resp.StatusCode, data)
		}
		if len(data) != 0 {
			t.Errorf("%s: the declared 204 carries no body, got %q", tc.name, data)
		}
	}
}

// TestMoveSessionMalformedInputIsDeclaredInvalidRequestError pins the declared
// 400 InvalidRequestError arm: a body that violates the declared requestBody
// schema ({sessionID*: string, destination*: {directory*: string},
// moveChanges?: boolean} additionalProperties:false) is refused with the
// declared union arm — not with a 204, not with the pre-fix 404, and not with an
// invented declared 405/404 code.
func TestMoveSessionMalformedInputIsDeclaredInvalidRequestError(t *testing.T) {
	ws := t.TempDir()
	s, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	s.workdir = ws
	defer srv.Close()

	for _, tc := range []struct{ name, body string }{
		{"absent body", ""},
		{"not json", "not json"},
		{"json array body", `[1,2]`},
		{"json null body", `null`},
		{"truncated json", `{"sessionID":`},
		{"missing sessionID", `{"destination":{"directory":"/tmp"}}`},
		{"missing destination", `{"sessionID":"s1"}`},
		{"sessionID not a string", `{"sessionID":123,"destination":{"directory":"/tmp"}}`},
		{"sessionID is null", `{"sessionID":null,"destination":{"directory":"/tmp"}}`},
		{"blank sessionID", `{"sessionID":"   ","destination":{"directory":"/tmp"}}`},
		{"destination not an object", `{"sessionID":"s1","destination":"/tmp"}`},
		{"destination is null", `{"sessionID":"s1","destination":null}`},
		{"missing destination directory", `{"sessionID":"s1","destination":{}}`},
		{"destination directory not a string", `{"sessionID":"s1","destination":{"directory":7}}`},
		{"blank destination directory", `{"sessionID":"s1","destination":{"directory":""}}`},
		{"unknown destination field", `{"sessionID":"s1","destination":{"directory":"/tmp","keepChanges":true}}`},
		{"moveChanges not a boolean", `{"sessionID":"s1","destination":{"directory":"/tmp"},"moveChanges":"yes"}`},
		{"moveChanges is null", `{"sessionID":"s1","destination":{"directory":"/tmp"},"moveChanges":null}`},
		{"unknown top-level field (additionalProperties:false)", `{"sessionID":"s1","destination":{"directory":"/tmp"},"force":true}`},
	} {
		status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, moveSessionPath, tc.body)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: POST %s (%q): got %d, want the declared 400. Body: %s",
				tc.name, moveSessionPath, tc.body, status, data)
		}
		assertMoveSessionInvalidRequest(t, tc.body, data)
	}
}

// TestMoveSessionDeclaredFailureArms pins the declared 400 MoveSessionError arm
// for the two well-formed requests this runtime cannot honour: a sessionID it
// does not hold, and a destination outside the single workspace it serves. The
// operation declares no 404, so an unknown id is never answered with an
// undeclared code, and no 204 is fabricated for a move that never happened.
func TestMoveSessionDeclaredFailureArms(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()

	t.Run("unknown session", func(t *testing.T) {
		_, srv := newTestServer(&mockDB{queryRowErr: notFoundErr{}})
		defer srv.Close()
		body := `{"sessionID":"ses_does_not_exist","destination":{"directory":` + jsonQuote(ws) + `}}`
		status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, moveSessionPath, body)
		if status != http.StatusBadRequest {
			t.Fatalf("got %d, want the declared 400 MoveSessionError. Body: %s", status, data)
		}
		assertMoveSessionError(t, body, data)
	})

	t.Run("destination outside the served workspace", func(t *testing.T) {
		s, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
		s.workdir = ws
		defer srv.Close()
		body := `{"sessionID":"s1","destination":{"directory":` + jsonQuote(other) + `}}`
		status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, moveSessionPath, body)
		if status != http.StatusBadRequest {
			t.Fatalf("got %d, want the declared 400 MoveSessionError. Body: %s", status, data)
		}
		assertMoveSessionError(t, body, data)
		if strings.Contains(strings.ToLower(string(data)), "moved") && !strings.Contains(strings.ToLower(string(data)), "cannot") {
			t.Errorf("a refused move must not claim the session moved: %s", data)
		}
	})
}

// TestMoveSessionMethodGuard pins the sibling method-guard envelope: the exact
// path is registered, so an undeclared method reaches the handler and answers
// 405 METHOD_NOT_ALLOWED — never the net/http default 404 the pre-fix tree
// answered.
func TestMoveSessionMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		status, _, data := doShimRequestBody(t, srv.URL, method, moveSessionPath, "{}")
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405. Body: %s", method, moveSessionPath, status, data)
		}
	}
}

// TestMoveSessionRequiresAuth pins the auth wall: the operation declares no 401
// arm, but every implemented shim route keeps Consensus api-key auth, so an
// unauthenticated request is answered 401 before the handler runs.
func TestMoveSessionRequiresAuth(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})}, "test-key", nil, nil)
	s.workdir = t.TempDir()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := `{"sessionID":"s1","destination":{"directory":` + jsonQuote(s.workdir) + `}}`
	status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, moveSessionPath, body)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST %s: got %d, want 401. Body: %s", moveSessionPath, status, data)
	}
}

// jsonQuote renders s as a JSON string literal for test bodies.
func jsonQuote(s string) string {
	b, err := json.Marshal(filepath.Clean(s))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// notFoundErr models the DB's "no rows" error for the session lookup.
type notFoundErr struct{}

func (notFoundErr) Error() string { return "sql: no rows in result set" }
