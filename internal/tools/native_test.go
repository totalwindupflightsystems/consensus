// Package tools: DF-CONSENSUS-077 — the ExecuteTool (sibling dispatcher) path
// must reach a real go_native handler, act inside the session workspace, and
// fail closed when handler_ref has no registered function.
//
// The pre-fix behaviour returned the string "tool X executed (go_native stub:
// ...)" with a nil error, so the caller recorded the request as completed while
// nothing happened. Every assertion here is on the FILE SYSTEM or on a non-nil
// error, never on the dispatcher's own success claim.
//
// axiom:trace work_item=DF-CONSENSUS-077 spec=specs/010-tools.md impl=internal/tools/native.go test=internal/tools/native_test.go
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// nativeTestDB answers the tools_registry lookup ExecuteTool performs.
type nativeTestDB struct {
	rows []map[string]any
}

func (m *nativeTestDB) BeginTx(context.Context) (db.Tx, error)     { return nil, nil }
func (m *nativeTestDB) Exec(context.Context, string, ...any) error { return nil }
func (m *nativeTestDB) Query(context.Context, string, ...any) ([]db.Row, error) {
	out := make([]db.Row, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, db.Row(row))
	}
	return out, nil
}
func (m *nativeTestDB) QueryRow(_ context.Context, _ string, _ ...any) (db.Row, error) {
	if len(m.rows) == 0 {
		return nil, nil
	}
	return db.Row(m.rows[0]), nil
}
func (m *nativeTestDB) Backend() db.Backend { return db.BackendSQLite }
func (m *nativeTestDB) Close() error        { return nil }

func goNativeTestDB(handlerRef string) *nativeTestDB {
	return &nativeTestDB{rows: []map[string]any{{
		"name":         "df077_probe",
		"handler_type": "go_native",
		"handler_ref":  handlerRef,
	}}}
}

func TestExecuteTool_GoNativeWriteFileLandsInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(WorkspaceEnvVar, workspace)

	result, err := ExecuteTool(context.Background(), goNativeTestDB("write_file"), &ToolExecutionRequest{
		ID:        "req-077-1",
		ToolName:  "df077_probe",
		SessionID: "sess-077",
		Parameters: map[string]any{
			"path":    "nested/df_probe.txt",
			"content": "DOGFOOD-OK",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteTool returned a nil result")
	}

	got, readErr := os.ReadFile(filepath.Join(workspace, "nested", "df_probe.txt"))
	if readErr != nil {
		t.Fatalf("dispatcher reported %q but wrote no file: %v", result.Output, readErr)
	}
	if string(got) != "DOGFOOD-OK" {
		t.Errorf("file content = %q, want %q", string(got), "DOGFOOD-OK")
	}
	if !strings.Contains(result.Output, "wrote") {
		t.Errorf("output = %q, want it to describe the write", result.Output)
	}
}

func TestExecuteTool_GoNativeShellRunsInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(WorkspaceEnvVar, workspace)

	result, err := ExecuteTool(context.Background(), goNativeTestDB("bash"), &ToolExecutionRequest{
		ID:         "req-077-2",
		ToolName:   "df077_probe",
		Parameters: map[string]any{"command": "printf DOGFOOD-OK > df_probe.txt && cat df_probe.txt"},
	})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}

	raw, readErr := os.ReadFile(filepath.Join(workspace, "df_probe.txt"))
	if readErr != nil {
		t.Fatalf("bash handler did not run in the workspace: %v (output %q)", readErr, result.Output)
	}
	if strings.TrimSpace(string(raw)) != "DOGFOOD-OK" {
		t.Errorf("shell wrote %q, want DOGFOOD-OK", string(raw))
	}
	if !strings.Contains(result.Output, "DOGFOOD-OK") {
		t.Errorf("output = %q, want the command's stdout", result.Output)
	}
}

func TestExecuteTool_GoNativeUnregisteredHandlerFails(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(WorkspaceEnvVar, workspace)

	result, err := ExecuteTool(context.Background(), goNativeTestDB("no_such_handler_077"), &ToolExecutionRequest{
		ID:         "req-077-3",
		ToolName:   "df077_probe",
		Parameters: map[string]any{"path": "ghost.txt", "content": "nope"},
	})
	if err == nil {
		t.Fatalf("ExecuteTool returned success for an unregistered handler (result %#v)", result)
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error = %q, want it to name the unregistered handler", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "ghost.txt")); statErr == nil {
		t.Error("an unregistered handler wrote a file")
	}
}

func TestExecuteTool_GoNativeRejectsPathEscapes(t *testing.T) {
	cases := []struct{ name, path string }{
		{"dotdot", "../escape.txt"},
		{"nested dotdot", "nested/../../escape.txt"},
		{"absolute", "/tmp/df-consensus-077-escape.txt"},
		{"home", "~/escape.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			workspace := filepath.Join(parent, "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			t.Setenv(WorkspaceEnvVar, workspace)

			result, err := ExecuteTool(context.Background(), goNativeTestDB("write_file"), &ToolExecutionRequest{
				ID:         "req-077-4",
				ToolName:   "df077_probe",
				Parameters: map[string]any{"path": tc.path, "content": "ESCAPED"},
			})
			// ExecuteTool records a handler failure as an error RESULT (the harness
			// persists it as a failed tool_result); either shape is a failure, and a
			// nil error with a success result is what must never happen.
			if err != nil {
				if !strings.Contains(err.Error(), "workspace") {
					t.Errorf("error = %q, want it to name the workspace boundary", err)
				}
			} else {
				if result == nil {
					t.Fatal("nil result and nil error")
				}
				if !result.IsError {
					t.Fatalf("path %q was accepted (output %q)", tc.path, result.Output)
				}
				if !strings.Contains(result.Output, "workspace") {
					t.Errorf("rejection = %q, want it to name the workspace boundary", result.Output)
				}
			}
			for _, outside := range []string{
				filepath.Join(parent, "escape.txt"),
				"/tmp/df-consensus-077-escape.txt",
			} {
				if _, statErr := os.Stat(outside); statErr == nil {
					t.Errorf("path %q escaped the workspace and was written to %s", tc.path, outside)
				}
			}
		})
	}
}

func TestResolveWorkspacePath_RejectsEscapesAndAcceptsNesting(t *testing.T) {
	root := t.TempDir()

	got, err := ResolveWorkspacePath(root, "a/b/c.txt")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(a/b/c.txt): %v", err)
	}
	if got != filepath.Join(root, "a", "b", "c.txt") {
		t.Errorf("resolved = %q, want %q", got, filepath.Join(root, "a", "b", "c.txt"))
	}

	for _, bad := range []string{"", ".", "..", "../x", "/etc/passwd", "~/x", "a/../../x"} {
		if _, err := ResolveWorkspacePath(root, bad); err == nil {
			t.Errorf("ResolveWorkspacePath(%q) accepted an out-of-workspace path", bad)
		}
	}
}

// TestRunNativeTool_SeededBuiltinsDispatch covers the two rows
// migrations/032_seed_builtin_tools.sql ships with handler_type='go_native':
// list_sessions and demo_approval_gate. The migration's header says they are
// implemented by the async executor's go_native handler, so a fresh install that
// routes either one through the executor must complete — not fail closed with
// "handler not registered". These assertions are on the handler OUTPUT, never on
// a success string alone.
func TestRunNativeTool_SeededBuiltinsDispatch(t *testing.T) {
	for _, ref := range []string{"list_sessions", "demo_approval_gate"} {
		if _, ok := NativeDBHandlerFor(ref); !ok {
			t.Fatalf("seeded handler_ref %q is not registered; a fresh install cannot dispatch it", ref)
		}
	}

	store := &nativeTestDB{rows: []map[string]any{{
		"id":         "sess-077",
		"agent_name": "df077-probe",
		"status":     "idle",
		"iteration":  3,
	}}}

	out, err := RunNativeTool(context.Background(), store, "sess-077", "list_sessions", "list_sessions", nil)
	if err != nil {
		t.Fatalf("list_sessions returned an error: %v", err)
	}
	if !strings.Contains(out, "df077-probe") {
		t.Errorf("list_sessions output %q does not carry the session row the database returned", out)
	}

	ack, err := RunNativeTool(context.Background(), store, "sess-077", "demo_approval_gate", "demo_approval_gate",
		map[string]any{"note": "approved by operator"})
	if err != nil {
		t.Fatalf("demo_approval_gate returned an error: %v", err)
	}
	if !strings.Contains(ack, "sess-077") || !strings.Contains(ack, "approved by operator") {
		t.Errorf("demo_approval_gate acknowledgement %q does not name the session and note", ack)
	}
}

// TestRunNativeTool_DBHandlerWithoutDatabaseFails proves the seeded handlers do
// not silently succeed when the dispatch has no database to read: fail closed is
// the doctrine, so a nil database must be an error, not an empty result.
func TestRunNativeTool_DBHandlerWithoutDatabaseFails(t *testing.T) {
	if _, err := RunNativeTool(context.Background(), nil, "sess-077", "list_sessions", "list_sessions", nil); err == nil {
		t.Fatal("list_sessions succeeded with a nil database; it must fail closed")
	}
}
