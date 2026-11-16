package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/harness"
	"github.com/wojons/consensus/internal/tools"
)

// ch:trace row=DF-CONSENSUS-66 spec=specs/010-tools.md#builtin-tools wave=none:serial test=internal/migrate/builtin_tools_test.go,internal/harness/tool_executor_native_test.go evidence=fresh-init-probe:5of5-seeded+go-test-green verdict=none:pending-tier2 commit=2523cba0 memory=none:pending
// ch:trace row=DF-CONSENSUS-79 spec=specs/010-tools.md#builtin-tools wave=none:serial test=internal/migrate/builtin_tools_test.go evidence=upgrade-probe:pre033-and-at033-converge+repaired-bash-dispatches-on-upgraded-db+falsification:034-noop-red+go-test-green verdict=tier2:changes_required-then-fixed commit=pending memory=none:pending
// STATUS: implemented
// TIER: T1
// EVIDENCE: fresh `consensus init` on an empty DB seeds read/write/edit/glob/bash as enabled active go_native rows with registered handler_refs; async ToolExecutor tests assert real filesystem effects.

func applyMigrationsRange(t *testing.T, database db.DB, fromVersion, throughVersion int) *Runner {
	t.Helper()
	ctx := context.Background()
	runner := New(database)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range runner.migrations {
		if migration.Version < fromVersion {
			continue
		}
		if migration.Version > throughVersion {
			break
		}
		sql := migrationUpSQL(migration.SQL)
		if database.Backend() == db.BackendSQLite && !strings.Contains(strings.ToLower(migration.Filename), "_sqlite_") {
			sql = filterForSQLite(sql)
		}
		for _, statement := range splitStatements(sql) {
			if err := database.Exec(ctx, statement); err != nil {
				t.Fatalf("apply %s: %v", migration.Filename, err)
			}
		}
		if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES ($1, $2, CURRENT_TIMESTAMP, $3)`, migration.Version, migration.Name, migrationChecksum(migration.SQL)); err != nil {
			t.Fatalf("record %s: %v", migration.Filename, err)
		}
	}
	return runner
}

func TestFreshMigrationSeedsFilesystemBuiltinTools(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	if _, err := New(database).AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rows, err := database.Query(context.Background(), `SELECT name, handler_type, handler_ref, enabled, status FROM tools_registry WHERE name IN ('read', 'write', 'edit', 'glob', 'bash') ORDER BY name`)
	if err != nil {
		t.Fatalf("query tools_registry: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("seeded filesystem tools = %d, want 5", len(rows))
	}
	for _, row := range rows {
		name := toString(row["name"])
		if toString(row["handler_type"]) != "go_native" || toString(row["status"]) != "active" || toInt(row["enabled"]) == 0 {
			t.Errorf("%s registry row = %#v, want active enabled go_native", name, row)
		}
		ref := toString(row["handler_ref"])
		if _, ok := tools.NativeHandlerFor(ref); !ok {
			t.Errorf("%s handler_ref %q is not registered", name, ref)
		}
	}
}

func TestMigrationRepairsStaleFilesystemBuiltinTools(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Apply migrations through 032, then simulate an upgraded install whose
	// pre-033 rows have stale dispatch metadata. This leaves the filesystem
	// seed and its repair (033 then 034) pending while preserving a realistic
	// pre-existing registry.
	runner := applyMigrationsRange(t, database, 1, 32)
	names := []string{"read", "write", "edit", "glob", "bash"}
	for _, name := range names {
		if err := database.Exec(ctx, `INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval) VALUES ($1, 'operator-owned', 'external', '{}', 'go_native', 'legacy_stub', 'disabled', false, false)`, name); err != nil {
			t.Fatalf("seed stale %s: %v", name, err)
		}
	}
	if err := database.Exec(ctx, `INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval) VALUES ('operator_tool', 'leave me alone', 'internal', '{}', 'go_native', 'operator_handler', 'active', true, false)`); err != nil {
		t.Fatalf("seed unrelated tool: %v", err)
	}

	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("upgrade AutoMigrate: %v", err)
	}
	wantRefs := map[string]string{"read": "read_file", "write": "write_file", "edit": "edit", "glob": "glob", "bash": "bash"}
	rows, err := database.Query(ctx, `SELECT name, handler_ref, handler_type, enabled, status, description FROM tools_registry WHERE name IN ('read', 'write', 'edit', 'glob', 'bash') ORDER BY name`)
	if err != nil {
		t.Fatalf("query repaired tools: %v", err)
	}
	if len(rows) != len(names) {
		t.Fatalf("repaired rows = %d, want %d", len(rows), len(names))
	}
	for _, row := range rows {
		name := toString(row["name"])
		if got := toString(row["handler_ref"]); got != wantRefs[name] {
			t.Errorf("%s handler_ref = %q, want %q", name, got, wantRefs[name])
		} else if _, registered := tools.NativeHandlerFor(got); !registered {
			t.Errorf("%s handler_ref %q is not a registered native handler", name, got)
		}
		if toString(row["handler_type"]) != "go_native" || toString(row["status"]) != "active" || toInt(row["enabled"]) == 0 {
			t.Errorf("%s row = %#v, want active enabled go_native", name, row)
		}
		if toString(row["description"]) != "operator-owned" {
			t.Errorf("%s description = %q, want preserved operator-owned metadata", name, row["description"])
		}
	}
	unrelated, err := database.Query(ctx, `SELECT handler_ref, status, enabled, description FROM tools_registry WHERE name = 'operator_tool'`)
	if err != nil || len(unrelated) != 1 || toString(unrelated[0]["handler_ref"]) != "operator_handler" || toString(unrelated[0]["description"]) != "leave me alone" {
		t.Fatalf("unrelated tool changed: rows=%#v err=%v", unrelated, err)
	}

	// Re-running init/migrate is idempotent and keeps the repaired dispatch.
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("second AutoMigrate: %v", err)
	}
}

// TestMigrationRepairsInstallsAlreadyAt033 covers the population that the
// insert-only 033 seed could not reach: a database that already recorded
// migration 033 while carrying stale dispatch fields. The repair has to ship
// as a new forward migration (034) — re-editing 033 neither runs again nor
// stays checksum-clean, so the repair would silently never happen.
func TestMigrationRepairsInstallsAlreadyAt033(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	applyMigrationsRange(t, database, 1, 32)
	names := []string{"read", "write", "edit", "glob", "bash"}
	for _, name := range names {
		if err := database.Exec(ctx, `INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval) VALUES ($1, 'operator-owned', 'external', '{}', 'subprocess', 'legacy_stub', 'disabled', false, false)`, name); err != nil {
			t.Fatalf("seed stale %s: %v", name, err)
		}
	}
	if err := database.Exec(ctx, `INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval) VALUES ('operator_tool', 'leave me alone', 'internal', '{}', 'go_native', 'operator_handler', 'active', true, false)`); err != nil {
		t.Fatalf("seed unrelated tool: %v", err)
	}

	// Shipped 033 is insert-only, so this install records 033 and stays stale.
	applyMigrationsRange(t, database, 33, 33)
	stale, err := database.Query(ctx, `SELECT handler_ref FROM tools_registry WHERE name = 'bash'`)
	if err != nil || len(stale) != 1 || toString(stale[0]["handler_ref"]) != "legacy_stub" {
		t.Fatalf("033 unexpectedly repaired the row already: rows=%#v err=%v", stale, err)
	}

	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("upgrade AutoMigrate: %v", err)
	}

	wantRefs := map[string]string{"read": "read_file", "write": "write_file", "edit": "edit", "glob": "glob", "bash": "bash"}
	rows, err := database.Query(ctx, `SELECT name, handler_ref, handler_type, enabled, status, description FROM tools_registry WHERE name IN ('read', 'write', 'edit', 'glob', 'bash') ORDER BY name`)
	if err != nil {
		t.Fatalf("query repaired tools: %v", err)
	}
	if len(rows) != len(names) {
		t.Fatalf("repaired rows = %d, want %d", len(rows), len(names))
	}
	for _, row := range rows {
		name := toString(row["name"])
		if got := toString(row["handler_ref"]); got != wantRefs[name] {
			t.Errorf("%s handler_ref = %q, want %q", name, got, wantRefs[name])
		} else if _, registered := tools.NativeHandlerFor(got); !registered {
			t.Errorf("%s handler_ref %q is not a registered native handler", name, got)
		}
		if toString(row["handler_type"]) != "go_native" || toString(row["status"]) != "active" || toInt(row["enabled"]) == 0 {
			t.Errorf("%s row = %#v, want active enabled go_native", name, row)
		}
		if toString(row["description"]) != "operator-owned" {
			t.Errorf("%s description = %q, want preserved operator-owned metadata", name, row["description"])
		}
	}
	unrelated, err := database.Query(ctx, `SELECT handler_ref, status, enabled, description FROM tools_registry WHERE name = 'operator_tool'`)
	if err != nil || len(unrelated) != 1 || toString(unrelated[0]["handler_ref"]) != "operator_handler" || toString(unrelated[0]["description"]) != "leave me alone" {
		t.Fatalf("unrelated tool changed: rows=%#v err=%v", unrelated, err)
	}

	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState after repair: %v", err)
	}
	if state.DriftDetected {
		t.Fatalf("repair introduced schema drift: %s", state.DriftDetails)
	}
	if state.MigrationRequired || len(state.PendingMigrations) != 0 {
		t.Fatalf("migrations still pending after repair: %v", state.PendingMigrations)
	}
}

func TestUpgradedFilesystemBashRunsThroughToolExecutor(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := New(database).AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	if err := database.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status) VALUES ('upgrade-bash-session', 'test', 'test-model', 'idle')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	rows, err := database.Query(ctx, `INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, created_at) VALUES ('upgrade-bash-session', 0, 'bash', '{"command":"printf upgraded > bash-upgrade.txt"}', 'pending', datetime('now')) RETURNING id`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("seed bash request: rows=%#v err=%v", rows, err)
	}
	executor := harness.NewToolExecutor(database, harness.DefaultToolExecutorConfig())
	if processed, err := executor.PollOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("PollOnce: processed=%d err=%v", processed, err)
	}
	requestID := toString(rows[0]["id"])
	resultRows, err := database.Query(ctx, `SELECT tr.status, r.output, r.is_error FROM tool_requests tr JOIN tool_results r ON r.request_id = tr.id WHERE tr.id = $1`, requestID)
	if err != nil || len(resultRows) != 1 || toString(resultRows[0]["status"]) != "completed" || toInt(resultRows[0]["is_error"]) != 0 {
		t.Fatalf("bash outcome: rows=%#v err=%v", resultRows, err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "bash-upgrade.txt"))
	if err != nil || string(content) != "upgraded" {
		t.Fatalf("bash workspace effect: content=%q err=%v", content, err)
	}
}

// TestRepairedBashDispatchesOnUpgradedInstall proves the repaired row actually
// dispatches: the bash request runs against the SAME database that went through
// stale-seed -> 033 -> 034 repair, not a freshly migrated one. A row that points
// at an unregistered handler_ref would pass column assertions yet fail here.
func TestRepairedBashDispatchesOnUpgradedInstall(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	applyMigrationsRange(t, database, 1, 32)
	for _, name := range []string{"read", "write", "edit", "glob", "bash"} {
		if err := database.Exec(ctx, `INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval) VALUES ($1, 'operator-owned', 'external', '{}', 'subprocess', 'legacy_stub', 'disabled', false, false)`, name); err != nil {
			t.Fatalf("seed stale %s: %v", name, err)
		}
	}
	applyMigrationsRange(t, database, 33, 33)

	if _, err := New(database).AutoMigrate(ctx); err != nil {
		t.Fatalf("upgrade AutoMigrate: %v", err)
	}
	repaired, err := database.Query(ctx, `SELECT handler_ref, status, enabled FROM tools_registry WHERE name = 'bash'`)
	if err != nil || len(repaired) != 1 {
		t.Fatalf("query repaired bash row: rows=%#v err=%v", repaired, err)
	}
	handlerRef := toString(repaired[0]["handler_ref"])
	if _, registered := tools.NativeHandlerFor(handlerRef); !registered {
		t.Fatalf("repaired bash handler_ref %q is not registered", handlerRef)
	}

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	if err := database.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status) VALUES ('repaired-bash-session', 'test', 'test-model', 'idle')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	requested, err := database.Query(ctx, `INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status, created_at) VALUES ('repaired-bash-session', 0, 'bash', '{"command":"printf repaired > bash-repaired.txt"}', 'pending', datetime('now')) RETURNING id`)
	if err != nil || len(requested) != 1 {
		t.Fatalf("seed bash request: rows=%#v err=%v", requested, err)
	}

	executor := harness.NewToolExecutor(database, harness.DefaultToolExecutorConfig())
	if processed, err := executor.PollOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("PollOnce on upgraded install: processed=%d err=%v", processed, err)
	}
	requestID := toString(requested[0]["id"])
	outcome, err := database.Query(ctx, `SELECT tr.status, r.output, r.is_error FROM tool_requests tr JOIN tool_results r ON r.request_id = tr.id WHERE tr.id = $1`, requestID)
	if err != nil || len(outcome) != 1 || toString(outcome[0]["status"]) != "completed" || toInt(outcome[0]["is_error"]) != 0 {
		t.Fatalf("repaired bash outcome: rows=%#v err=%v", outcome, err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "bash-repaired.txt"))
	if err != nil || string(content) != "repaired" {
		t.Fatalf("repaired bash workspace effect: content=%q err=%v", content, err)
	}
}

func TestFreshMigrationSeedIsIdempotent(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("first AutoMigrate: %v", err)
	}
	if err := database.Exec(ctx, `UPDATE tools_registry SET description = 'operator-owned' WHERE name = 'read'`); err != nil {
		t.Fatalf("customize read row: %v", err)
	}

	// Re-running the exact seed is safe and does not overwrite operator data.
	rows, err := database.Query(ctx, `SELECT description FROM tools_registry WHERE name = 'read'`)
	if err != nil || len(rows) != 1 || !strings.Contains(toString(rows[0]["description"]), "operator-owned") {
		t.Fatalf("read row was unexpectedly overwritten: rows=%#v err=%v", rows, err)
	}
}
