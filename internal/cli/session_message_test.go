// Regression tests for DF-CONSENSUS-34: `consensus session message` — the
// CLI-only path to drive an agent conversation. A session never leaves
// "booting" on its own: the harness only wakes on a user_instruction message
// (POST /api/v1/sessions/{id}/message). Previously the CLI exposed no way to
// send one and `session create` presented a bare --goal as a complete flow.
//
// axiom:trace work_item=DF-CONSENSUS-34 test=internal/cli/session_message_test.go
package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMessageCapturingServer returns an httptest server that records the path
// and decoded JSON body of every request and replies with the message
// response shape `SendMessage` decodes.
func newMessageCapturingServer(t *testing.T) (*httptest.Server, *string, *map[string]any) {
	t.Helper()
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sent": true, "type": "user_instruction", "session_id": "sess-42"}`))
	}))
	return srv, &gotPath, &gotBody
}

// The subcommand must exist under `session`, POST to the message endpoint
// with the session id in the path and the joined text as content, and print
// the decoded response.
func TestSessionMessage_SendsContentAndPrintsResponse(t *testing.T) {
	captureGlobals(t)
	srv, gotPath, gotBody := newMessageCapturingServer(t)
	defer srv.Close()

	// The subcommand must be registered under `session`.
	root := NewRootCommand()
	msg, _, err := root.Find([]string{"session", "message"})
	if err != nil || msg == nil || msg.Name() != "message" {
		t.Fatalf("`session message` not registered under root: cmd=%v err=%v", msg, err)
	}

	// Full root execution: server/key via flags (root_test.go idiom — the
	// root's flag bind resets pre-set globals, DF-CONSENSUS-33).
	root.SetArgs([]string{"--server", srv.URL, "--api-key", "test-key", "session", "message", "sess-42", "wake", "up", "and", "review", "the", "spec"})
	err, stdout := captureStdout(func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *gotPath != "/api/v1/sessions/sess-42/message" {
		t.Errorf("path: got %q, want %q", *gotPath, "/api/v1/sessions/sess-42/message")
	}
	if content, _ := (*gotBody)["content"].(string); content != "wake up and review the spec" {
		t.Errorf("content: got %q, want %q", content, "wake up and review the spec")
	}
	if !strings.Contains(stdout, "user_instruction") {
		t.Errorf("expected decoded response in output, got: %s", stdout)
	}
}

// Wrong argument counts must fail with a usage error (root maps it to a
// non-zero exit).
func TestSessionMessage_WrongArgCount(t *testing.T) {
	for _, args := range [][]string{
		{"sess-42"},         // id without text
		{},                  // nothing at all
		{"just text no id"}, // one arg: could be either
	} {
		cmd := newSessionMessageCmd()
		cmd.SetArgs(args)
		err, _ := captureStdout(func() error { return cmd.Execute() })
		if err == nil {
			t.Errorf("args %v: expected an error, got nil", args)
		}
	}
}

// `session create` must tell the user the agent will not start until a
// message is sent, including the exact command to send it (DF-CONSENSUS-34).
func TestSessionCreate_PrintsWakeHint(t *testing.T) {
	ms := newMockAPIServer()
	defer ms.Close()
	defer overrideGlobals(ms.URL, "test-key", "json", false)()

	cmd := newSessionCreateCmd()
	cmd.SetArgs([]string{"--goal", "research topic"})
	err, stdout := captureStdout(func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "will not start") {
		t.Errorf("expected wake hint in create output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "consensus session message sess-001") {
		t.Errorf("expected hint naming the message command and session id, got: %s", stdout)
	}
}
