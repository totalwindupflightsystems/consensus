// Package harness: SCN-CONSENSUS-009 — the LIVE dispatch path must resolve a
// requested tool name against the registry NAME and, failing that, its
// handler_ref.
//
// Measured defect (real scenario run, 2026-11-16): the model asks for
// read_file/write_file — the conventional agent vocabulary — while migrations
// 033/034 seed the public names read/write with handler_ref read_file/write_file.
// The executor looked the name up exactly, so the request failed with
// `tool "read_file" is not registered or enabled`: tool_requests showed
// read_file=failed and read=completed, and the agent could not write its
// workspace. Every assertion below is on the persisted request outcome or on
// the file system, never on the executor's own claim.
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

// queueToolRequest inserts one pending tool_request for requestedName and
// returns its id.
func queueToolRequest(t *testing.T, database db.DB, sessionID, requestedName, paramsJSON string) string {
	t.Helper()

	rows, err := database.Query(context.Background(),
		`INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, created_at)
		 VALUES ($1, 0, $2, $3, 'pending', datetime('now'))
		 RETURNING id`,
		sessionID, requestedName, paramsJSON)
	if err != nil {
		t.Fatalf("insert tool_request for %q: %v", requestedName, err)
	}
	if len(rows) == 0 {
		t.Fatalf("insert tool_request for %q returned no id", requestedName)
	}
	return toString(rows[0]["id"])
}

// seedAliasToolRequest registers a go_native row under registryName and queues a
// pending tool_request that asks for requestedName, returning the request id.
// The two names differ exactly as they do live (registry `read`, request
// `read_file`).
func seedAliasToolRequest(t *testing.T, database db.DB, sessionID, registryName, handlerRef, requestedName, paramsJSON string) string {
	t.Helper()

	if err := database.Exec(context.Background(), `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ($1, $2, 'SCN-CONSENSUS-009 alias probe', 'external', 'go_native', $3, 1)`,
		"tool-"+registryName, registryName, handlerRef); err != nil {
		t.Fatalf("register tool %q: %v", registryName, err)
	}

	return queueToolRequest(t, database, sessionID, requestedName, paramsJSON)
}

// A request under the model's handler-name vocabulary reaches the registry row
// and the real handler runs — the live defect's falsifier.
func TestToolExecutor_ResolvesHandlerRefAlias(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	const body = "alias-live-path-ran\n"
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	reqID := seedAliasToolRequest(t, database, "sess-te-01", "read", "read_file", "read_file", `{"path":"fixture.txt"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "completed" || isError != 0 {
		t.Fatalf("read_file via alias: status=%q is_error=%d output=%q; the alias must dispatch to the read handler",
			status, isError, output)
	}
	if output != body {
		t.Errorf("output = %q, want the fixture body %q read by the read_file handler", output, body)
	}
}

// Two enabled rows behind one handler_ref: the live path must refuse to guess
// and persist the reason, not execute either row.
func TestToolExecutor_AliasAmbiguousFailsClosed(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	// `read` is the row the model would reach; `zz_reader` collides on the ref.
	if err := database.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled)
		VALUES ('tool-zz', 'zz_reader', 'collider', 'external', 'go_native', 'read_file', 1)`); err != nil {
		t.Fatalf("register colliding row: %v", err)
	}
	reqID := seedAliasToolRequest(t, database, "sess-te-01", "read", "read_file", "read_file", `{"path":"fixture.txt"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "failed" || isError != 1 {
		t.Fatalf("ambiguous alias: status=%q is_error=%d output=%q; want a failed request", status, isError, output)
	}
	for _, want := range []string{"ambiguous", "[read zz_reader]"} {
		if !strings.Contains(output, want) {
			t.Errorf("failure = %q, want it to contain %q", output, want)
		}
	}
}

// An aliased request must not slip past the approval gate: the gate parks
// requests by registry NAME, so a gated row reached by handler_ref fails closed
// and nothing is written.
func TestToolExecutor_AliasedGatedToolFailsClosed(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	if err := database.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled, requires_approval)
		VALUES ('tool-write', 'write', 'gated writer', 'external', 'go_native', 'write_file', 1, 1)`); err != nil {
		t.Fatalf("register gated tool: %v", err)
	}
	reqID := queueToolRequest(t, database, "sess-te-01", "write_file",
		`{"path":"gated.txt","content":"SHOULD-NOT-EXIST"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "failed" || isError != 1 {
		t.Fatalf("gated alias: status=%q is_error=%d output=%q; want a failed request", status, isError, output)
	}
	if !strings.Contains(output, "requires approval") {
		t.Errorf("failure = %q, want it to name the approval gate", output)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "gated.txt")); statErr == nil {
		t.Error("a gated tool executed without approval and wrote gated.txt")
	}
}
