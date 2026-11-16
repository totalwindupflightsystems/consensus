// Package harness: DF-CONSENSUS-077 — go_native and subprocess dispatch must
// actually act inside the session workspace.
//
// These tests are the falsifier for the pre-fix behaviour: a go_native tool call
// was answered with the string "tool X executed (go_native stub: ...)" and
// NOTHING happened, so "the requested file exists with the requested content"
// was unsatisfiable. Every assertion below is on the FILE SYSTEM or on the
// persisted failure, never on the executor's own happy-path claim.
//
// axiom:trace work_item=DF-CONSENSUS-077 spec=specs/010-tools.md impl=internal/tools/native.go test=internal/harness/tool_executor_native_test.go
package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/tools"
)

// seedWorkspaceToolRequest registers a tool row and queues one pending
// tool_request carrying paramsJSON, returning the request id.
func seedWorkspaceToolRequest(t *testing.T, database db.DB, sessionID, toolName, handlerType, handlerRef, paramsJSON string) string {
	t.Helper()
	ctx := context.Background()

	if err := database.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ($1, $2, 'DF-CONSENSUS-077 workspace probe', 'external', $3, $4, 1)`,
		"tool-"+toolName, toolName, handlerType, handlerRef); err != nil {
		t.Fatalf("register tool %q: %v", toolName, err)
	}

	rows, err := database.Query(ctx,
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, created_at)
		 VALUES ($1, 0, $2, $3, 'pending', datetime('now'))
		 RETURNING id`,
		sessionID, toolName, paramsJSON)
	if err != nil {
		t.Fatalf("insert tool_request for %q: %v", toolName, err)
	}
	if len(rows) == 0 {
		t.Fatalf("insert tool_request for %q returned no id", toolName)
	}
	return toString(rows[0]["id"])
}

// requestOutcome reads the persisted status plus the failure-tolerant result row.
func requestOutcome(t *testing.T, database db.DB, reqID string) (status, output string, isError int) {
	t.Helper()
	ctx := context.Background()

	statusRows, err := database.Query(ctx, `SELECT status FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query tool_request %s: %v", reqID, err)
	}
	if len(statusRows) != 1 {
		t.Fatalf("tool_request %s: got %d rows, want 1", reqID, len(statusRows))
	}
	status = toString(statusRows[0]["status"])

	resultRows, err := database.Query(ctx,
		`SELECT output, is_error FROM tool_results WHERE request_id = $1`, reqID)
	if err != nil {
		t.Fatalf("query tool_results for %s: %v", reqID, err)
	}
	if len(resultRows) != 1 {
		t.Fatalf("tool_results for %s: got %d rows, want 1", reqID, len(resultRows))
	}
	return status, toString(resultRows[0]["output"]), toInt(resultRows[0]["is_error"])
}

// ============================================================================
// go_native write_file — the capability the measured repro could not reach
// ============================================================================

func TestToolExecutor_FreshFilesystemBuiltinsExecuteRealEffects(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	fixture := filepath.Join(workspace, "fixture.txt")
	if err := os.WriteFile(fixture, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())
	poll := func(name, handlerRef, params string) string {
		t.Helper()
		requestID := seedWorkspaceToolRequest(t, database, "sess-te-01", name, "go_native", handlerRef, params)
		count, err := executor.PollOnce(ctx)
		if err != nil || count != 1 {
			t.Fatalf("PollOnce %s: count=%d err=%v", name, count, err)
		}
		status, output, isError := requestOutcome(t, database, requestID)
		if status != "completed" || isError != 0 {
			t.Fatalf("%s: status=%q is_error=%d output=%q; real handler must execute", name, status, isError, output)
		}
		if strings.Contains(output, "go_native stub") {
			t.Fatalf("%s: output contains the removed stub success marker: %q", name, output)
		}
		return output
	}

	writeOutput := poll("write", "write_file", `{"path":"nested/hello.py","content":"def greet():\n    return \"Hello, bakeoff!\"\n\n\nprint(greet())\n"}`)
	if !strings.Contains(writeOutput, "wrote") {
		t.Errorf("write output = %q, want write confirmation", writeOutput)
	}
	editOutput := poll("edit", "edit", `{"path":"fixture.txt","old_string":"before","new_string":"after"}`)
	if !strings.Contains(editOutput, "edited") {
		t.Errorf("edit output = %q, want edit confirmation", editOutput)
	}
	readOutput := poll("read", "read_file", `{"path":"fixture.txt"}`)
	if !strings.Contains(readOutput, "after") {
		t.Errorf("read output = %q, want edited fixture body", readOutput)
	}
	globOutput := poll("glob", "glob", `{"pattern":"nested/*.py"}`)
	if !strings.Contains(globOutput, "nested/hello.py") {
		t.Errorf("glob output = %q, want the written Python file", globOutput)
	}
	bashOutput := poll("bash", "bash", `{"command":"python3 nested/hello.py"}`)
	if !strings.Contains(bashOutput, "Hello, bakeoff!") {
		t.Errorf("bash output = %q, want the executed script's stdout", bashOutput)
	}

	written, err := os.ReadFile(filepath.Join(workspace, "nested", "hello.py"))
	if err != nil {
		t.Fatalf("write did not create hello.py: %v", err)
	}
	if !strings.Contains(string(written), "def greet()") {
		t.Errorf("hello.py = %q, want a greet() function in the bakeoff fixture", string(written))
	}
	edited, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read edited fixture: %v", err)
	}
	if string(edited) != "after\n" {
		t.Errorf("edited fixture = %q, want after", string(edited))
	}
}

func TestToolExecutor_GoNativeWriteFile_CreatesFileInWorkspace(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
		"write_probe", "go_native", "write_file",
		`{"path":"nested/df_probe.txt","content":"DOGFOOD-OK"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Errorf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "completed" {
		t.Fatalf("tool_request status = %q (output %q), want completed", status, output)
	}
	if isError != 0 {
		t.Fatalf("tool_result is_error = %d, want 0", isError)
	}

	// The whole point: the file is on disk, with exactly the requested content.
	got, readErr := os.ReadFile(filepath.Join(workspace, "nested", "df_probe.txt"))
	if readErr != nil {
		t.Fatalf("DF-CONSENSUS-077: tool reported %q but wrote no file: %v", output, readErr)
	}
	if string(got) != "DOGFOOD-OK" {
		t.Errorf("file content = %q, want %q", string(got), "DOGFOOD-OK")
	}

	t.Logf("write_file landed %q in the session workspace (%d bytes reported: %q)",
		filepath.Join(workspace, "nested", "df_probe.txt"), len(got), output)
}

func TestToolExecutor_GoNativeCreateFileAndFileWriteAliases(t *testing.T) {
	for _, handlerRef := range []string{"create_file", "file_write"} {
		t.Run(handlerRef, func(t *testing.T) {
			database, cleanup := setupToolExecutorTestDB(t)
			defer cleanup()

			workspace := t.TempDir()
			t.Setenv(tools.WorkspaceEnvVar, workspace)

			ctx := context.Background()
			executor := NewToolExecutor(database, DefaultToolExecutorConfig())

			toolName := "probe_" + handlerRef
			reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
				toolName, "go_native", handlerRef,
				`{"path":"`+handlerRef+`.txt","content":"ALIAS-OK"}`)

			if _, err := executor.PollOnce(ctx); err != nil {
				t.Fatalf("PollOnce: %v", err)
			}

			status, output, _ := requestOutcome(t, database, reqID)
			if status != "completed" {
				t.Fatalf("status = %q (output %q), want completed", status, output)
			}
			got, readErr := os.ReadFile(filepath.Join(workspace, handlerRef+".txt"))
			if readErr != nil {
				t.Fatalf("%s: no file written: %v", handlerRef, readErr)
			}
			if string(got) != "ALIAS-OK" {
				t.Errorf("%s content = %q, want ALIAS-OK", handlerRef, string(got))
			}
		})
	}
}

// ============================================================================
// go_native shell handler — runs in the workspace cwd
// ============================================================================

func TestToolExecutor_GoNativeShellRunsInWorkspace(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
		"shell_probe", "go_native", "bash",
		`{"command":"pwd > shell_pwd.txt && cat shell_pwd.txt"}`)

	if _, err := executor.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "completed" {
		t.Fatalf("status = %q (output %q), want completed", status, output)
	}
	if isError != 0 {
		t.Fatalf("is_error = %d, want 0 (output %q)", isError, output)
	}

	pwdFile := filepath.Join(workspace, "shell_pwd.txt")
	raw, readErr := os.ReadFile(pwdFile)
	if readErr != nil {
		t.Fatalf("bash handler did not run in the workspace: %v", readErr)
	}
	gotPwd := strings.TrimSpace(string(raw))
	// t.TempDir() may hand back a symlinked path (/tmp vs /private/...) — compare
	// against the resolved workspace instead of the literal string.
	wantPwd, evalErr := filepath.EvalSymlinks(workspace)
	if evalErr != nil {
		wantPwd = workspace
	}
	gotResolved, evalGotErr := filepath.EvalSymlinks(gotPwd)
	if evalGotErr != nil {
		gotResolved = gotPwd
	}
	if gotResolved != wantPwd {
		t.Errorf("bash cwd = %q, want the session workspace %q", gotResolved, wantPwd)
	}
	if !strings.Contains(output, wantPwd) && !strings.Contains(output, workspace) {
		t.Errorf("bash output = %q, want the workspace path echoed back", output)
	}
}

// ============================================================================
// Fail closed: an unregistered go_native ref must FAIL, never report success
// ============================================================================

func TestToolExecutor_GoNativeUnregisteredHandlerFailsRequest(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
		"ghost_tool", "go_native", "no_such_native_handler",
		`{"path":"ghost.txt","content":"should never be written"}`)

	if _, err := executor.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "failed" {
		t.Fatalf("status = %q, want failed — a stub MUST NOT report success (output %q)", status, output)
	}
	if isError != 1 {
		t.Errorf("is_error = %d, want 1", isError)
	}
	if !strings.Contains(output, "not registered") {
		t.Errorf("failure reason = %q, want it to name the unregistered handler", output)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "ghost.txt")); statErr == nil {
		t.Error("an unregistered handler wrote a file")
	}
}

// ============================================================================
// Path traversal guard — the handler, not the caller, must reject escapes
// ============================================================================

func TestToolExecutor_GoNativeRejectsPathsOutsideWorkspace(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "dotdot", path: "../escape.txt"},
		{name: "nested dotdot", path: "nested/../../escape.txt"},
		{name: "absolute", path: "/tmp/df-consensus-077-escape.txt"},
		{name: "home relative", path: "~/escape.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, cleanup := setupToolExecutorTestDB(t)
			defer cleanup()

			parent := t.TempDir()
			workspace := filepath.Join(parent, "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			t.Setenv(tools.WorkspaceEnvVar, workspace)

			ctx := context.Background()
			executor := NewToolExecutor(database, DefaultToolExecutorConfig())

			reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
				"escape_probe", "go_native", "write_file",
				`{"path":"`+tc.path+`","content":"ESCAPED"}`)

			if _, err := executor.PollOnce(ctx); err != nil {
				t.Fatalf("PollOnce: %v", err)
			}

			status, output, isError := requestOutcome(t, database, reqID)
			if status != "failed" {
				t.Fatalf("status = %q for path %q, want failed (output %q)", status, tc.path, output)
			}
			if isError != 1 {
				t.Errorf("is_error = %d, want 1", isError)
			}
			if !strings.Contains(output, "workspace") {
				t.Errorf("failure reason = %q, want it to name the workspace boundary", output)
			}

			// Nothing may appear outside the workspace.
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

// ============================================================================
// subprocess — session workspace cwd + parameters on stdin
// ============================================================================

func TestToolExecutor_SubprocessRunsInWorkspaceWithParameters(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	// `cat > sub_probe.txt` writes whatever the handler received on stdin into a
	// file RELATIVE to the cwd — so the file landing proves both halves of the
	// fix: cwd is the workspace AND the parameters reached the process.
	reqID := seedWorkspaceToolRequest(t, database, "sess-te-01",
		"stdin_probe", "subprocess", `sh -c "cat > sub_probe.txt"`,
		`{"message":"hello from harness"}`)

	if _, err := executor.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "completed" {
		t.Fatalf("status = %q (output %q), want completed", status, output)
	}
	if isError != 0 {
		t.Fatalf("is_error = %d, want 0 (output %q)", isError, output)
	}

	raw, readErr := os.ReadFile(filepath.Join(workspace, "sub_probe.txt"))
	if readErr != nil {
		t.Fatalf("subprocess tool did not run in the session workspace: %v", readErr)
	}
	payload := string(raw)
	if !strings.Contains(payload, `"message":"hello from harness"`) {
		t.Errorf("stdin payload = %q, want the request parameters as JSON", payload)
	}
}
