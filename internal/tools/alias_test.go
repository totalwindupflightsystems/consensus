// Package tools: SCN-CONSENSUS-009 — a requested tool name resolves against the
// registry NAME first and its handler_ref second.
//
// Live defect this file is the falsifier for: the harness model calls
// read_file/write_file (the conventional agent vocabulary) while migrations
// 033/034 seed the public names read/write with handler_ref read_file/write_file.
// Dispatch did an exact name match, so every such call was rejected with
// `execute_tool: tool "read_file" is not registered or enabled` and the agent
// could not write its workspace.
//
// The tests use real SQLite (the same in-memory driver the rest of the package
// uses) because the resolution rule is expressed in SQL: a canned db.Row fake
// cannot distinguish "matched by name" from "matched by handler_ref".
package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// aliasTestTool is one tools_registry row seeded by these tests.
type aliasTestTool struct {
	name    string
	ref     string
	enabled bool
}

// setupAliasTestDB opens an in-memory database carrying the tools_registry
// columns resolution needs and seeds one row per aliasTestTool.
func setupAliasTestDB(t *testing.T, toolRows ...aliasTestTool) db.DB {
	t.Helper()
	ctx := context.Background()

	database, err := driver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.Exec(ctx, `CREATE TABLE IF NOT EXISTS tools_registry (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		hemisphere TEXT NOT NULL DEFAULT 'external',
		handler_type TEXT NOT NULL DEFAULT 'go_native',
		handler_ref TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT 'active',
		requires_approval INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create tools_registry: %v", err)
	}

	for _, row := range toolRows {
		enabled := 0
		if row.enabled {
			enabled = 1
		}
		if err := database.Exec(ctx,
			`INSERT INTO tools_registry (id, name, handler_type, handler_ref, enabled)
			 VALUES ($1, $2, 'go_native', $3, $4)`,
			"tool-"+row.name, row.name, row.ref, enabled); err != nil {
			t.Fatalf("seed tool %q: %v", row.name, err)
		}
	}

	return database
}

// seedWorkspaceFile writes a fixture into a fresh workspace and points the
// workspace resolution at it, so a dispatched read_file handler can be proven
// to have run against real bytes.
func seedWorkspaceFile(t *testing.T, name, content string) string {
	t.Helper()

	workspace := t.TempDir()
	t.Setenv(WorkspaceEnvVar, workspace)

	path := filepath.Join(workspace, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workspace fixture %q: %v", name, err)
	}
	return workspace
}

// ----------------------------------------------------------------------------
// 1. handler_ref alias resolves and reaches the real handler
// ----------------------------------------------------------------------------

func TestExecuteToolResolvesHandlerRefAlias(t *testing.T) {
	const body = "alias-resolution-ran\n"
	seedWorkspaceFile(t, "fixture.txt", body)

	// migrations/033 seed exactly this shape: the public name is `read`, the
	// Go handler and the model's vocabulary are `read_file`.
	database := setupAliasTestDB(t, aliasTestTool{name: "read", ref: "read_file", enabled: true})

	result, err := ExecuteTool(context.Background(), database, &ToolExecutionRequest{
		ID:         "req-alias-1",
		SessionID:  "sess-alias-1",
		ToolName:   "read_file",
		Parameters: map[string]any{"path": "fixture.txt"},
	})
	if err != nil {
		t.Fatalf("ExecuteTool(read_file) = error %v; the handler_ref alias must resolve to the read row", err)
	}
	if result == nil {
		t.Fatal("ExecuteTool(read_file) returned a nil result")
	}
	if result.IsError {
		t.Fatalf("ExecuteTool(read_file) failed: %q (%s)", result.Output, result.ErrorCode)
	}
	// The proof that dispatch reached the read_file handler: it returned the
	// fixture's bytes. A rejection or a stub would not.
	if result.Output != body {
		t.Errorf("output = %q, want the fixture body %q from the read_file handler", result.Output, body)
	}
}

// A name that IS a registry name must win over other rows using it as their
// handler_ref — the alias arm is a fallback, never a competing match.
func TestExecuteToolExactNameWinsOverAlias(t *testing.T) {
	const body = "exact-name-won\n"
	seedWorkspaceFile(t, "fixture.txt", body)

	database := setupAliasTestDB(t,
		aliasTestTool{name: "read", ref: "read_file", enabled: true},
		aliasTestTool{name: "read_file", ref: "read_file", enabled: true},
	)

	result, err := ExecuteTool(context.Background(), database, &ToolExecutionRequest{
		ID:         "req-alias-2",
		SessionID:  "sess-alias-2",
		ToolName:   "read_file",
		Parameters: map[string]any{"path": "fixture.txt"},
	})
	if err != nil {
		t.Fatalf("ExecuteTool(read_file) = error %v; an exact registry name must not be reported as ambiguous", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("ExecuteTool(read_file) result = %#v, want a successful read", result)
	}
	if result.Output != body {
		t.Errorf("output = %q, want the fixture body %q", result.Output, body)
	}
}

// ----------------------------------------------------------------------------
// 2. Two enabled rows behind one handler_ref → fail closed, never guess
// ----------------------------------------------------------------------------

func TestExecuteToolAliasAmbiguousFailsClosed(t *testing.T) {
	database := setupAliasTestDB(t,
		aliasTestTool{name: "read", ref: "read_file", enabled: true},
		aliasTestTool{name: "zz_reader", ref: "read_file", enabled: true},
	)

	result, err := ExecuteTool(context.Background(), database, &ToolExecutionRequest{
		ID:        "req-alias-3",
		SessionID: "sess-alias-3",
		ToolName:  "read_file",
	})
	if err == nil {
		t.Fatalf("ExecuteTool returned success for an ambiguous alias (result %#v)", result)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil for a failed dispatch", result)
	}

	// Both colliding names, sorted, so the operator can fix the registry.
	for _, want := range []string{"ambiguous", "[read zz_reader]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
	if !errors.Is(err, ErrToolAliasAmbiguous) {
		t.Errorf("error = %v, want it to wrap ErrToolAliasAmbiguous", err)
	}
}

// ----------------------------------------------------------------------------
// 3. A name matching neither column still fails closed (DF-CONSENSUS-069)
// ----------------------------------------------------------------------------

func TestExecuteToolUnknownNameStillFailsClosed(t *testing.T) {
	database := setupAliasTestDB(t, aliasTestTool{name: "read", ref: "read_file", enabled: true})

	result, err := ExecuteTool(context.Background(), database, &ToolExecutionRequest{
		ID:        "req-alias-4",
		SessionID: "sess-alias-4",
		ToolName:  "no_such_tool_anywhere",
	})
	if err == nil {
		t.Fatalf("ExecuteTool returned success for an unregistered tool (result %#v)", result)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil for a failed dispatch", result)
	}
	if !strings.Contains(err.Error(), "not registered or enabled") {
		t.Errorf("error = %q, want the unregistered-tool message", err)
	}
	if !errors.Is(err, ErrToolNotRegistered) {
		t.Errorf("error = %v, want it to wrap ErrToolNotRegistered", err)
	}
}

// ----------------------------------------------------------------------------
// 4. A DISABLED row is never resolved as an alias
// ----------------------------------------------------------------------------

func TestExecuteToolDisabledRowIsNotResolvedAsAlias(t *testing.T) {
	database := setupAliasTestDB(t, aliasTestTool{name: "read", ref: "read_file", enabled: false})

	result, err := ExecuteTool(context.Background(), database, &ToolExecutionRequest{
		ID:        "req-alias-5",
		SessionID: "sess-alias-5",
		ToolName:  "read_file",
	})
	if err == nil {
		t.Fatalf("ExecuteTool returned success for a disabled row's handler_ref (result %#v)", result)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil for a failed dispatch", result)
	}
	if !strings.Contains(err.Error(), "not registered or enabled") {
		t.Errorf("error = %q, want the unregistered-tool message", err)
	}
}

// ----------------------------------------------------------------------------
// Resolver contract — the row that won, plus how it was found
// ----------------------------------------------------------------------------

func TestResolveToolRegistryRowReportsMatchKind(t *testing.T) {
	database := setupAliasTestDB(t, aliasTestTool{name: "write", ref: "write_file", enabled: true})

	byName, alias, err := ResolveToolRegistryRow(context.Background(), database, "write")
	if err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if alias {
		t.Error("resolve by name reported an alias match")
	}
	if got := toString(byName["name"]); got != "write" {
		t.Errorf("resolved name = %q, want %q", got, "write")
	}
	if got := toString(byName["handler_type"]); got != "go_native" {
		t.Errorf("resolved handler_type = %q, want go_native (metadata must come from the winning row)", got)
	}

	byRef, alias, err := ResolveToolRegistryRow(context.Background(), database, "write_file")
	if err != nil {
		t.Fatalf("resolve by handler_ref: %v", err)
	}
	if !alias {
		t.Error("resolve by handler_ref did not report an alias match")
	}
	if got := toString(byRef["name"]); got != "write" {
		t.Errorf("alias resolved name = %q, want the registered row name %q", got, "write")
	}
	if got := toString(byRef["handler_ref"]); got != "write_file" {
		t.Errorf("alias resolved handler_ref = %q, want write_file", got)
	}

	if _, _, err := ResolveToolRegistryRow(context.Background(), nil, "write"); err == nil {
		t.Error("resolving against a nil database returned no error")
	}
}

// The ambiguity error must list every colliding name in sorted order — the
// operator needs a deterministic list to act on.
func TestToolResolveErrorMessageSortsCollidingNames(t *testing.T) {
	matches := []string{"zeta", "alpha", "mu"}
	sorted := append([]string(nil), matches...)
	sort.Strings(sorted)

	err := &ToolResolveError{Requested: "shared_handler", Matches: sorted}
	got := err.Error()
	want := `tool "shared_handler" is ambiguous: matches registry names [alpha mu zeta] by handler_ref`
	if got != want {
		t.Errorf("error = %q, want %q", got, want)
	}

	notFound := (&ToolResolveError{Requested: "ghost"}).Error()
	if wantNF := `tool "ghost" is not registered or enabled`; notFound != wantNF {
		t.Errorf("error = %q, want %q", notFound, wantNF)
	}
}
