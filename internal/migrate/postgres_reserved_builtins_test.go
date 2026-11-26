// postgres_builtin_repair_test.go — Postgres coverage for migration 034
// (repair_filesystem_builtin_tools), DF-CONSENSUS-081 gap 1.
//
// On Postgres the migration runner executes a whole migration in a SINGLE
// Exec (migrate.go's non-SQLite branch), so statement-by-statement SQLite
// coverage does not prove the Postgres path: only a real Postgres run of
// migration 034's full BEGIN/COMMIT body does. This test drives the real
// runner (Up), not a re-implementation.
//
// Policy under test (DF-CONSENSUS-081 gap 2): read/write/edit/glob/bash are
// reserved builtin tools owned by the product. Migration 034 INTENTIONALLY
// rewrites their dispatch fields (handler_type, handler_ref, status,
// enabled) unconditionally — an operator who deliberately disabled or
// rewired one of these five reserved builtin names is silently overridden
// on upgrade, by design — while operator-owned metadata (description,
// hemisphere, parameter schema, ...) and unrelated tool rows are preserved.
// The override semantics are documented in docs/TOOLS.md ("Reserved builtin
// tools").
//
// Skip condition: CONSENSUS_TEST_POSTGRES_URL must be set.
package migrate

import (
	"context"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	pgpostgres "github.com/wojons/consensus/internal/db/postgres"
)

func TestPostgresMigration034BuiltinRepair(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres migration-034 builtin-repair test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to open Postgres connection: %v", err)
	}
	defer func() { _ = database.Close() }()

	if database.Backend() != db.BackendPostgres {
		t.Fatalf("expected postgres backend, got %s", database.Backend())
	}

	// Migrations must run as the table owner (the AfterConnect hook of the
	// main pool runs SET ROLE agent_role once 021 exists).
	adminDB := database
	if pgdb, ok := database.(*pgpostgres.DB); ok {
		adminDB = pgdb.AdminDB()
	}
	if err := grantAgentRoleSchemaPrivs(ctx, pgURL); err != nil {
		t.Logf("skip agent_role schema grant (%v) — assuming fresh DB", err)
	}

	runner := New(adminDB)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("baseline AutoMigrate: %v", err)
	}

	names := []string{"read", "write", "edit", "glob", "bash"}
	wantRefs := map[string]string{
		"read": "read_file", "write": "write_file", "edit": "edit",
		"glob": "glob", "bash": "bash",
	}

	// Seed the EXACT population 034 repairs: an upgraded install whose five
	// reserved builtin rows carry stale dispatch fields — including an
	// operator who deliberately disabled the reserved builtin. The 034
	// policy is that these five names are product-owned: the rewrite below
	// is the state the migration must converge (and it must converge it
	// deliberately, per docs/TOOLS.md).
	for _, name := range names {
		if err := adminDB.Exec(ctx,
			`UPDATE tools_registry SET handler_type = 'subprocess', handler_ref = 'legacy_stub',
			 status = 'disabled', enabled = false, description = 'operator-owned', hemisphere = 'external'
			 WHERE name = $1`, name); err != nil {
			t.Fatalf("seed stale %s: %v", name, err)
		}
	}
	if err := adminDB.Exec(ctx,
		`INSERT INTO tools_registry (name, description, hemisphere, parameter_schema, handler_type, handler_ref, status, enabled, requires_approval)
		 VALUES ('operator_tool', 'leave me alone', 'internal', '{}', 'go_native', 'operator_handler', 'active', true, false)
		 ON CONFLICT (name) DO UPDATE SET handler_ref = EXCLUDED.handler_ref, status = EXCLUDED.status,
		 enabled = EXCLUDED.enabled, description = EXCLUDED.description`); err != nil {
		t.Fatalf("seed unrelated tool: %v", err)
	}

	// Prove the premise before running the migration: the rows really are
	// stale/disabled right now.
	staleRows, err := adminDB.Query(ctx,
		`SELECT name, handler_ref, status, enabled::INT AS enabled_int FROM tools_registry WHERE name = 'bash'`)
	if err != nil || len(staleRows) != 1 || toString(staleRows[0]["handler_ref"]) != "legacy_stub" ||
		toString(staleRows[0]["status"]) != "disabled" || toInt(staleRows[0]["enabled_int"]) != 0 {
		t.Fatalf("test premise: bash row should be stale/disabled before 034: rows=%#v err=%v", staleRows, err)
	}

	// Make 034 pending again and run it through the REAL Postgres path:
	// one whole-migration Exec of the BEGIN/COMMIT body inside Up.
	if err := adminDB.Exec(ctx, `DELETE FROM schema_versions WHERE version = 34`); err != nil {
		t.Fatalf("unrecord migration 034: %v", err)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply pending migration 034 via Up: %v", err)
	}
	if !containsMigration(applied, "034_repair_filesystem_builtin_tools.sql") {
		t.Fatalf("migration 034 was not reported as applied: %v", applied)
	}

	rows, err := adminDB.Query(ctx,
		`SELECT name, handler_ref, handler_type, status, enabled::INT AS enabled_int, description, hemisphere
		 FROM tools_registry WHERE name IN ('read', 'write', 'edit', 'glob', 'bash') ORDER BY name`)
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
		}
		if toString(row["handler_type"]) != "go_native" || toString(row["status"]) != "active" ||
			toInt(row["enabled_int"]) != 1 {
			t.Errorf("%s row = %#v, want active enabled go_native (reserved builtin converged)", name, row)
		}
		// Operator-owned metadata must be preserved even though the
		// dispatch fields were intentionally overwritten.
		if toString(row["description"]) != "operator-owned" || toString(row["hemisphere"]) != "external" {
			t.Errorf("%s metadata = %#v, want preserved operator-owned description/hemisphere", name, row)
		}
	}

	unrelated, err := adminDB.Query(ctx,
		`SELECT handler_ref, status, enabled::INT AS enabled_int, description FROM tools_registry WHERE name = 'operator_tool'`)
	if err != nil || len(unrelated) != 1 || toString(unrelated[0]["handler_ref"]) != "operator_handler" ||
		toString(unrelated[0]["description"]) != "leave me alone" || toInt(unrelated[0]["enabled_int"]) != 1 {
		t.Fatalf("unrelated tool changed: rows=%#v err=%v", unrelated, err)
	}

	// 034 re-running is a no-op: nothing new applied, converged rows stay.
	appliedAgain, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("idempotent Up: %v", err)
	}
	if len(appliedAgain) != 0 {
		t.Errorf("second Up applied %v, want none", appliedAgain)
	}

	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState after repair: %v", err)
	}
	if state.DriftDetected {
		t.Errorf("repair introduced schema drift: %s", state.DriftDetails)
	}
	if state.MigrationRequired || len(state.PendingMigrations) != 0 {
		t.Errorf("migrations still pending after repair: %v", state.PendingMigrations)
	}
}
