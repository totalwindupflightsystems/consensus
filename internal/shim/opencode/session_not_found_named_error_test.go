package opencode

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// assertNotFoundNamedErrorBody asserts the upstream v2 SDK NamedError shape on a
// decoded 404 body: name=NotFoundError, data.message containing the exact
// upstream sentence, and NO retired {"error":...} envelope key.
func assertNotFoundNamedErrorBody(t *testing.T, label string, status int, body map[string]any) {
	t.Helper()
	if status != http.StatusNotFound {
		t.Fatalf("%s: got %d, want 404 (body %v)", label, status, body)
	}
	// The retired shim envelope must be gone — its presence is what made the
	// SDK's wrapClientError miss the message.
	if _, ok := body["error"]; ok {
		t.Errorf("%s: body carries the retired \"error\" envelope key: %v", label, body)
	}
	if name, _ := body["name"].(string); name != "NotFoundError" {
		t.Errorf("%s: body.name = %v, want %q (body %v)", label, body["name"], "NotFoundError", body)
	}
	data, _ := body["data"].(map[string]any)
	if msg, _ := data["message"].(string); !strings.Contains(msg, "Session not found") {
		t.Errorf("%s: body.data.message = %q, want it to contain %q (body %v)", label, msg, "Session not found", body)
	}
}

// assertNotFoundNamedError is the raw-body twin of assertNotFoundNamedErrorBody.
func assertNotFoundNamedError(t *testing.T, label string, status int, raw []byte) {
	t.Helper()
	var body map[string]any
	if len(raw) == 0 {
		t.Fatalf("%s: empty 404 body, want a NamedError (status %d)", label, status)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("%s: 404 body is not a JSON object: %v (%s)", label, err, raw)
	}
	assertNotFoundNamedErrorBody(t, label, status, body)
}

// TestSessionNotFoundNamedError404 pins the upstream v2 SDK error contract for
// GET /session/{missingID} (SHIM-SUITE33-001, upstream pin v1.18.33).
//
// Upstream packages/opencode/test/server/sdk-error-shape.test.ts
// ("404 with NamedError body throws a real Error carrying the server message")
// drives sdk.session.get({sessionID:"ses_no_such"}, {throwOnError:true}) and
// asserts err.message contains "Session not found" plus cause.body matching
// {name:"NotFoundError", data:{message: stringContaining(...)}}. The SDK only
// extracts data.message when the body IS a NamedError; the shim's pre-fix
// {"error":{"code","message"}} envelope left cause.body without name/data, so
// err.message was empty and F1 sdk-error-shape failed.
//
// This test asserts the served wire shape directly: 404, name=NotFoundError,
// data.message carrying the exact upstream sentence, and the absence of the
// retired "error" envelope key.
func TestSessionNotFoundNamedError404(t *testing.T) {
	s, srv, _ := newMessageResponseTestServer(t) // real store: no session "ses_no_such"
	defer srv.Close()
	s.skipAuth = true

	status, header, raw := doShimRequest(t, srv.URL, http.MethodGet, "/session/ses_no_such")
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	assertNotFoundNamedError(t, "GET /session/ses_no_such", status, raw)
}

// TestSessionNotFoundNamedErrorAcrossSites checks the conversion reached every
// session-not-found 404 arm named by SHIM-SUITE33-001 — not just GET
// /session/{id} — so a future edit cannot regress one site back to the retired
// envelope. Each path resolves the session before doing any work, so an unknown
// id must answer the NamedError 404 without touching the store; the bodies
// carry whatever the operation's own pre-session validation requires, so the
// request reaches the session lookup instead of a 400.
func TestSessionNotFoundNamedErrorAcrossSites(t *testing.T) {
	s, srv, _ := newMessageResponseTestServer(t) // real store: no session "ses_no_such"
	defer srv.Close()
	s.skipAuth = true

	cases := []struct {
		name string
		path string
		body string
	}{
		{"init", "/session/ses_no_such/init", `{"modelID":"m","providerID":"p","messageID":"msg_1"}`},
		{"fork", "/session/ses_no_such/fork", `{}`},
		{"revert", "/session/ses_no_such/revert", `{"messageID":"msg_1"}`},
		{"unrevert", "/session/ses_no_such/unrevert", `{}`},
		{"shell", "/session/ses_no_such/shell", `{"agent":"a","command":"ls"}`},
		{"summarize", "/session/ses_no_such/summarize", `{"providerID":"p","modelID":"m"}`},
		{"command", "/session/ses_no_such/command", `{"command":"explain","arguments":"x"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, raw := postCommand(t, srv.URL, tc.path, tc.body)
			assertNotFoundNamedError(t, "POST "+tc.path, status, raw)
		})
	}
}
