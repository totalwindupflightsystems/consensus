// Package tools: go_native handler registry and the workspace-bound built-in
// handlers (DF-CONSENSUS-077).
//
// A tools_registry row with handler_type='go_native' names a Go function through
// handler_ref. Before this file the dispatch was a stub that returned a success
// string without calling anything, so a tool_request was marked 'completed'
// while no effect happened — the model asked for write_file/bash and the file
// never appeared. Dispatch now goes through a process-wide registry, and a ref
// with no registered function is an ERROR: a handler that did not run must never
// report success (fail-closed, the DF-CONSENSUS-069 doctrine).
//
// The built-ins are the file/shell tools the model actually asks for. Every one
// of them is bound to the SESSION WORKSPACE: writes resolve relative paths under
// the workspace root, reject absolute paths and '..' escapes, and the shell
// handler runs with the workspace as its working directory.
//
// axiom:trace work_item=DF-CONSENSUS-077 spec=specs/010-tools.md impl=internal/tools/native.go
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wojons/consensus/internal/db"
)

// ErrNativeHandlerNotRegistered marks a go_native dispatch whose handler_ref has
// no registered Go function. Callers must treat it as a failure of the REQUEST —
// never as a successful no-op.
var ErrNativeHandlerNotRegistered = errors.New("go_native handler not registered")

// NativeHandler executes a go_native tool.
//
// params is the decoded tool_requests.parameters object and workspace is the
// session workspace root the handler must stay inside. A nil error means the
// effect HAPPENED, because the caller records the request as 'completed' on a
// nil error; a handler that cannot do the work must return an error.
type NativeHandler func(ctx context.Context, params map[string]any, workspace string) (string, error)

var (
	nativeHandlersMu sync.RWMutex
	nativeHandlers   = map[string]NativeHandler{}
)

// NativeDBHandler executes a go_native tool that needs the database it runs
// against. It is a separate registry from NativeHandler because the
// workspace-bound handlers are pure functions of (params, workspace) and safe to
// register once at boot, while these are bound to ONE database and session.
type NativeDBHandler func(ctx context.Context, database db.DB, sessionID string, params map[string]any) (string, error)

// nativeDBHandlers holds the handlers for the rows that
// migrations/032_seed_builtin_tools.sql ships with handler_type='go_native'.
// That migration's header promises list_sessions and demo_approval_gate are
// "implemented by ... the async executor's go_native handler"; these entries keep
// that promise. Without them the seeded tools fail closed on every fresh install,
// which is how DF-CONSENSUS-077's fail-closed executor surfaced the gap.
var nativeDBHandlers = map[string]NativeDBHandler{
	"list_sessions":      nativeListSessions,
	"demo_approval_gate": nativeDemoApprovalGate,
}

// NativeDBHandlerFor reports the database-bound handler registered for ref.
func NativeDBHandlerFor(handlerRef string) (NativeDBHandler, bool) {
	fn, ok := nativeDBHandlers[strings.TrimSpace(handlerRef)]
	return fn, ok
}

// RegisterNativeHandler binds handlerRef to fn. Registering the same ref twice
// replaces the previous function (tests and boot-time wiring both rely on it
// being idempotent). An empty ref or nil function is ignored.
func RegisterNativeHandler(handlerRef string, fn NativeHandler) {
	ref := strings.TrimSpace(handlerRef)
	if ref == "" || fn == nil {
		return
	}
	nativeHandlersMu.Lock()
	defer nativeHandlersMu.Unlock()
	nativeHandlers[ref] = fn
}

// NativeHandlerFor resolves a registered handler by handler_ref.
func NativeHandlerFor(handlerRef string) (NativeHandler, bool) {
	nativeHandlersMu.RLock()
	defer nativeHandlersMu.RUnlock()
	fn, ok := nativeHandlers[strings.TrimSpace(handlerRef)]
	return fn, ok
}

// RegisteredNativeHandlers lists the registered refs, sorted, for diagnostics.
func RegisteredNativeHandlers() []string {
	nativeHandlersMu.RLock()
	defer nativeHandlersMu.RUnlock()
	refs := make([]string, 0, len(nativeHandlers))
	for ref := range nativeHandlers {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

// RunNativeTool is the single go_native dispatch shared by both execution paths
// (internal/harness/tool_executor.go and ExecuteTool in this package).
//
// handlerRef wins; a registry row with an empty handler_ref falls back to the
// tool name, which is the shape an operator gets from
// `INSERT ... handler_type='go_native'` without a ref.
//
// A ref with no registered function returns an error wrapping
// ErrNativeHandlerNotRegistered — the request then becomes 'failed' with that
// reason instead of a completed row that did nothing.
func RunNativeTool(ctx context.Context, database db.DB, sessionID, toolName, handlerRef string, params map[string]any) (string, error) {
	ref := strings.TrimSpace(handlerRef)
	if ref == "" {
		ref = toolName
	}

	// The seeded built-ins need the database this dispatch runs against, so they
	// are looked up before the workspace-bound registry and need no workspace
	// (list_sessions is read-only bookkeeping).
	if dbHandler, ok := NativeDBHandlerFor(ref); ok {
		if params == nil {
			params = map[string]any{}
		}
		output, err := dbHandler(ctx, database, sessionID, params)
		if err != nil {
			return "", fmt.Errorf("go_native %q: %w", ref, err)
		}
		return output, nil
	}

	fn, ok := NativeHandlerFor(ref)
	if !ok {
		return "", fmt.Errorf("%w: %q for tool %q (registered: %s)",
			ErrNativeHandlerNotRegistered, ref, toolName, strings.Join(RegisteredNativeHandlers(), ", "))
	}

	workspace, err := ResolveWorkspace(ctx, database, sessionID)
	if err != nil {
		return "", err
	}
	if params == nil {
		params = map[string]any{}
	}

	output, err := fn(ctx, params, workspace)
	if err != nil {
		return "", fmt.Errorf("go_native %q: %w", ref, err)
	}
	return output, nil
}

// ============================================================================
// Session workspace
// ============================================================================

// WorkspaceEnvVar names the operator-level workspace root used for sessions that
// do not carry a workspace of their own. It is used VERBATIM (one shared
// directory), which is what an operator running a single agent against one
// checkout wants.
const WorkspaceEnvVar = "CONSENSUS_TOOL_WORKSPACE"

// fallbackWorkspaceDir is the documented last-resort root: a per-session
// directory under the OS temp dir. It is ephemeral by design — a deployment that
// needs workspace files to survive a restart must set the session's workspace
// column or CONSENSUS_TOOL_WORKSPACE.
const fallbackWorkspaceDir = "consensus-workspaces"

// sessionWorkspaceColumns are the column names a deployment may use to pin a
// session's workspace directory. The first non-empty one wins. The shipped
// schema (migrations/001 §3) has none of them yet, so the env/temp fallbacks are
// what run today; adding the column later is enough to take over, no code change.
var sessionWorkspaceColumns = []string{"workspace", "workspace_root", "directory", "workdir", "cwd"}

// ResolveWorkspace returns the absolute workspace directory for a session,
// creating it when missing.
//
// Precedence:
//  1. the session's own workspace column (if the deployment has one),
//  2. CONSENSUS_TOOL_WORKSPACE,
//  3. <os.TempDir()>/consensus-workspaces/<session-id> (documented fallback).
//
// SELECT * is deliberate: naming a workspace column would make every dispatch
// fail against a schema that has not got one.
func ResolveWorkspace(ctx context.Context, database db.DB, sessionID string) (string, error) {
	if database != nil && sessionID != "" {
		if row, err := database.QueryRow(ctx, `SELECT * FROM sessions WHERE id = $1`, sessionID); err == nil && row != nil {
			for _, column := range sessionWorkspaceColumns {
				value, ok := row[column]
				if !ok {
					continue
				}
				if dir := strings.TrimSpace(toString(value)); dir != "" {
					return ensureWorkspaceDir(dir)
				}
			}
		}
	}

	if root := strings.TrimSpace(os.Getenv(WorkspaceEnvVar)); root != "" {
		return ensureWorkspaceDir(root)
	}

	name := sanitizeName(sessionID)
	if name == "" {
		name = "default"
	}
	return ensureWorkspaceDir(filepath.Join(os.TempDir(), fallbackWorkspaceDir, name))
}

// ensureWorkspaceDir makes dir absolute and creates it.
func ensureWorkspaceDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve %q: %w", dir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("workspace: create %q: %w", abs, err)
	}
	return abs, nil
}

// ResolveWorkspacePath maps a model-supplied relative path into the workspace.
//
// Absolute paths, '..' escapes, NUL bytes and '~' are rejected: a tool may only
// touch what is inside the session workspace. The joined result is re-checked
// against the cleaned root so a path that cleans differently than it reads
// cannot slip out.
func ResolveWorkspacePath(workspace, relative string) (string, error) {
	relative = strings.TrimSpace(relative)
	if relative == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(relative, 0) {
		return "", fmt.Errorf("path %q contains a NUL byte", relative)
	}
	if filepath.IsAbs(relative) || strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, `\`) ||
		(len(relative) > 1 && relative[1] == ':') {
		return "", fmt.Errorf("path %q is absolute; paths must be relative to the session workspace", relative)
	}
	if relative == "~" || strings.HasPrefix(relative, "~/") {
		return "", fmt.Errorf("path %q is not a workspace-relative path", relative)
	}

	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." {
		return "", fmt.Errorf("path %q names the workspace root, not a file", relative)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the session workspace", relative)
	}

	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("workspace %q: %w", workspace, err)
	}
	full := filepath.Join(root, cleaned)
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the session workspace", relative)
	}
	return full, nil
}

// ============================================================================
// Built-in handlers
// ============================================================================

// nativeTextParam extracts an optional string parameter under any of names.
// The bool reports PRESENCE (an explicitly empty string is present), so a
// handler can tell "content omitted" from "content is empty".
func nativeTextParam(params map[string]any, names ...string) (string, bool, error) {
	for _, name := range names {
		value, ok := params[name]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return "", true, fmt.Errorf("parameter %q must be a string, got %T", name, value)
		}
		return text, true, nil
	}
	return "", false, nil
}

// requiredPathParam resolves the model's path parameter (path/file_path/filename)
// into an absolute workspace path.
func requiredPathParam(params map[string]any, workspace, handler string) (string, error) {
	raw, present, err := nativeTextParam(params, "path", "file_path", "filename")
	if err != nil {
		return "", fmt.Errorf("%s: %w", handler, err)
	}
	if !present || strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s: parameter %q is required", handler, "path")
	}
	full, err := ResolveWorkspacePath(workspace, raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", handler, err)
	}
	return full, nil
}

// nativeWriteFile writes the model's content to the model's path inside the
// workspace, creating parent directories. This is the tool the measured repro
// could not get to happen at all.
func nativeWriteFile(_ context.Context, params map[string]any, workspace string) (string, error) {
	full, err := requiredPathParam(params, workspace, "write_file")
	if err != nil {
		return "", err
	}

	content, present, err := nativeTextParam(params, "content", "contents", "text", "data")
	if err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if !present {
		return "", fmt.Errorf("write_file: parameter %q is required", "content")
	}

	if dir := filepath.Dir(full); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("write_file: create %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: write %q: %w", full, err)
	}

	relative, relErr := filepath.Rel(workspace, full)
	if relErr != nil {
		relative = filepath.Base(full)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), filepath.ToSlash(relative)), nil
}

// nativeReadFile returns a workspace file's contents — the read half of the
// write/verify loop a model needs to satisfy "the file exists with this
// content" without shelling out.
func nativeReadFile(_ context.Context, params map[string]any, workspace string) (string, error) {
	full, err := requiredPathParam(params, workspace, "read_file")
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return string(content), nil
}

// nativeShell runs a shell command with the workspace as its working directory
// and returns the combined stdout/stderr.
//
// The workspace is the cwd, NOT a jail: a command can still reach absolute paths
// on the host. Callers that need containment run the tool as a subprocess tool
// with its own sandbox, not as a go_native handler.
func nativeShell(ctx context.Context, params map[string]any, workspace string) (string, error) {
	command, present, err := nativeTextParam(params, "command", "cmd", "script")
	if err != nil {
		return "", fmt.Errorf("shell: %w", err)
	}
	if !present || strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("shell: parameter %q is required", "command")
	}

	cfg := DefaultExternalToolConfig()
	cfg.WorkDir = workspace

	result, err := ExecuteExternalTool(ctx, "sh", []string{"-c", command}, cfg)
	if err != nil {
		return "", fmt.Errorf("shell: %w", err)
	}
	if result.ExitCode != 0 {
		// The output is the only diagnostic the model gets, so it travels in the
		// error the executor records.
		return "", fmt.Errorf("shell: exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Output))
	}
	return result.Output, nil
}

// ============================================================================
// Seeded built-ins (migrations/032_seed_builtin_tools.sql)
// ============================================================================

// nativeListSessions is the go_native implementation of the seeded
// `list_sessions` row. It mirrors the REST direct-execute path in
// internal/api/tools.go (same columns, newest first) so a tool request routed
// through the async executor returns what the synchronous route returns.
func nativeListSessions(ctx context.Context, database db.DB, sessionID string, params map[string]any) (string, error) {
	if database == nil {
		return "", fmt.Errorf("list_sessions: no database bound to this dispatch")
	}
	limit := 50
	if raw, ok := params["limit"]; ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw))); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	rows, err := database.Query(ctx, fmt.Sprintf(
		`SELECT id, agent_name, status, iteration FROM sessions ORDER BY created_at DESC LIMIT %d`, limit))
	if err != nil {
		return "", fmt.Errorf("list_sessions: %w", err)
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("list_sessions: encode rows: %w", err)
	}
	return string(encoded), nil
}

// nativeDemoApprovalGate is the go_native implementation of the seeded
// `demo_approval_gate` row: an acknowledgement with no side effect, which is what
// the migration documents ("no-op acknowledgement on approval"). The approval
// decision itself happens BEFORE this handler runs (internal/tools/approval.go),
// so a successful return here means the human approved and the tool completed.
func nativeDemoApprovalGate(ctx context.Context, database db.DB, sessionID string, params map[string]any) (string, error) {
	note := strings.TrimSpace(fmt.Sprint(params["note"]))
	if note == "" || note == "<nil>" {
		note = "(no note)"
	}
	return fmt.Sprintf("approval gate acknowledged for session %s: %s", sessionID, note), nil
}

func init() {
	// The file-writing aliases are the names the model actually requested in the
	// measured repro; all three are the same handler.
	RegisterNativeHandler("write_file", nativeWriteFile)
	RegisterNativeHandler("create_file", nativeWriteFile)
	RegisterNativeHandler("file_write", nativeWriteFile)

	RegisterNativeHandler("read_file", nativeReadFile)
	RegisterNativeHandler("file_read", nativeReadFile)

	RegisterNativeHandler("shell", nativeShell)
	RegisterNativeHandler("bash", nativeShell)
	RegisterNativeHandler("exec", nativeShell)
}
