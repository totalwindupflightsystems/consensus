// postgres_test.go — Postgres migration bootstrap integration test.
//
// Gated by CONSENSUS_TEST_POSTGRES_URL env var. Skips gracefully when unset.
// Verifies that all embedded migration files apply cleanly against a real
// Postgres instance and that core schema tables exist post-migration.
//
// axiom:trace work_item=postgres-bootstrap-verification-01
//   spec=specs/003-database.md,specs/009-deployment.md
//   impl=internal/migrate/postgres_test.go

package migrate

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	pgpostgres "github.com/wojons/consensus/internal/db/postgres"
)

func TestPostgresBootstrap(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer database.Close()

	if database.Backend() != db.BackendPostgres {
		t.Fatalf("expected postgres backend, got %s", database.Backend())
	}

	runner := New(database)

	// Run AutoMigrate: Bootstrap → LoadMigrations → Up
	migrated, err := runner.AutoMigrate(ctx)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	t.Logf("Migrations applied: %v", migrated)

	// Verify schema_versions tracking
	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	t.Logf("Current schema version: %d", state.CurrentVersion)
	t.Logf("Applied migrations: %v", state.AppliedMigrations)

	if state.MigrationRequired {
		t.Errorf("expected no pending migrations, got %v", state.PendingMigrations)
	}

	// Verify core tables exist (from SPEC-003 §2)
	expectedTables := []string{
		"sessions", "memory_events", "display_modes", "iteration_commits",
		"memory_pages", "tasks", "tool_requests", "tool_results",
		"tools_registry", "skills_registry", "agent_billing", "workflows",
		"custom_agent_tools", "tool_files", "external_quarantine",
		"model_registry", "schema_versions", "staging_buffer",
		"agent_circuit_breakers", "agent_budget_limits", "audit_logs",
	}
	for _, table := range expectedTables {
		rows, err := database.Query(ctx,
			`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`,
			table,
		)
		if err != nil {
			t.Errorf("failed to check table %s: %v", table, err)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("table %s: no result from information_schema", table)
			continue
		}
		exists, _ := rows[0]["exists"].(bool)
		if !exists {
			t.Errorf("expected table %s to exist after migration", table)
		} else {
			t.Logf("  ✓ table %s exists", table)
		}
	}

	// Verify agent_circuit_breakers has breaker_type + unique constraint
	// (BUG-012 — this is the drifted table on the dexdat sidecar PG).
	cbCols, err := database.Query(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'agent_circuit_breakers'`)
	if err != nil {
		t.Fatalf("query agent_circuit_breakers columns: %v", err)
	}
	colSet := make(map[string]bool)
	for _, row := range cbCols {
		if c, ok := row["column_name"].(string); ok {
			colSet[c] = true
		}
	}
	for _, want := range []string{"breaker_type", "threshold", "current_count"} {
		if !colSet[want] {
			t.Errorf("agent_circuit_breakers.%s missing (BUG-012 drift)", want)
		} else {
			t.Logf("  ✓ agent_circuit_breakers.%s present", want)
		}
	}
}

// TestPostgresRepairCircuitBreakers verifies the BUG-012 repair heals a
// drifted agent_circuit_breakers table on Postgres: drop breaker_type,
// re-run AutoMigrate, assert breaker_type + unique index are restored.
// Gated by CONSENSUS_TEST_POSTGRES_URL.
func TestPostgresRepairCircuitBreakers(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres repair test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer database.Close()

	// The driver's AfterConnect hook runs SET ROLE agent_role (RLS). Once
	// migration 021 has created agent_role, the main pool runs with reduced
	// privileges (no CREATE/SELECT on tables it doesn't own). Migrations must
	// run as the table owner. Type-assert to the concrete *postgres.DB to get
	// an admin handle (no SET ROLE). Fall back to the main DB on non-pgx
	// builds (SQLite) — but this test is PG-gated, so the assertion must hold.
	adminDB := database
	if pgdb, ok := database.(*pgpostgres.DB); ok {
		adminDB = pgdb.AdminDB()
	}
	// On a DB where 021 already ran, grant agent_role the schema privileges it
	// needs for the idempotent bootstrap (CREATE TABLE IF NOT EXISTS). Uses the
	// admin pool (no SET ROLE) so the GRANT runs as table owner.
	if err := grantAgentRoleSchemaPrivs(ctx, pgURL); err != nil {
		t.Logf("skip agent_role schema grant (%v) — assuming fresh DB", err)
	}

	runner := New(adminDB)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("first AutoMigrate failed: %v", err)
	}
	if !pgCircuitBreakerHasColumn(t, adminDB, "breaker_type") {
		t.Fatal("setup: breaker_type should exist after AutoMigrate")
	}

	// Simulate drift: drop breaker_type. Postgres supports DROP COLUMN.
	if err := adminDB.Exec(ctx, `ALTER TABLE agent_circuit_breakers DROP COLUMN breaker_type`); err != nil {
		t.Fatalf("drop breaker_type: %v", err)
	}
	if pgCircuitBreakerHasColumn(t, adminDB, "breaker_type") {
		t.Fatal("setup: breaker_type should be gone after DROP")
	}

	// Heal path: re-run AutoMigrate — Up() skips v3, only repairCircuitBreakers
	// can restore the column.
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("second AutoMigrate (heal) failed: %v", err)
	}
	if !pgCircuitBreakerHasColumn(t, adminDB, "breaker_type") {
		t.Error("breaker_type still MISSING after repairCircuitBreakers heal")
	} else {
		t.Log("✓ breaker_type restored by repairCircuitBreakers")
	}
	if !pgCircuitBreakerUniqueIndexExists(t, adminDB) {
		t.Error("unique index on (session_id, breaker_type) MISSING after heal")
	} else {
		t.Log("✓ unique index on (session_id, breaker_type) present after heal")
	}
}

func pgCircuitBreakerHasColumn(t *testing.T, database db.DB, col string) bool {
	t.Helper()
	rows, err := database.Query(context.Background(),
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'agent_circuit_breakers'`)
	if err != nil {
		t.Fatalf("query agent_circuit_breakers columns: %v", err)
	}
	for _, row := range rows {
		if c, ok := row["column_name"].(string); ok && c == col {
			return true
		}
	}
	return false
}

func pgCircuitBreakerUniqueIndexExists(t *testing.T, database db.DB) bool {
	t.Helper()
	rows, err := database.Query(context.Background(),
		`SELECT indexname FROM pg_indexes WHERE tablename = 'agent_circuit_breakers'`)
	if err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	for _, row := range rows {
		name, _ := row["indexname"].(string)
		if name == "agent_circuit_breakers_session_breaker_key" {
			return true
		}
	}
	// A PRIMARY KEY on (session_id, breaker_type) also satisfies ON CONFLICT.
	pkRows, err := database.Query(context.Background(),
		`SELECT indexname FROM pg_indexes WHERE tablename = 'agent_circuit_breakers' AND indexdef LIKE '%UNIQUE%'`)
	if err == nil {
		for _, row := range pkRows {
			if name, _ := row["indexname"].(string); strings.Contains(name, "session") {
				return true
			}
		}
	}
	return false
}

// grantAgentRoleSchemaPrivs grants USAGE+CREATE on the public schema to
// agent_role so that the idempotent bootstrap (CREATE TABLE IF NOT EXISTS)
// succeeds when the migration runner's main pool is under SET ROLE
// agent_role (post-migration-021). Opens a raw connection WITHOUT the
// AfterConnect SET ROLE hook so the GRANT runs as the table owner.
// Returns an error if agent_role does not exist yet (fresh DB) — caller
// treats that as "nothing to do".
func grantAgentRoleSchemaPrivs(ctx context.Context, pgURL string) error {
	// Parse the test URL and connect with a plain pgx pool (no AfterConnect
	// hook) so we run as the connection owner, not agent_role.
	cfg, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return err
	}
	cfg.AfterConnect = nil
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx,
		`DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agent_role') THEN
				GRANT USAGE, CREATE ON SCHEMA public TO agent_role;
			END IF;
		END $$`)
	return err
}
