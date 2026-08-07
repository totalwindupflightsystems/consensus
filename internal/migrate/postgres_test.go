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

// TestPostgresRepairCircuitBreakersOldShape replicates the full BUG-012
// scenario on a scratch Postgres database: the agent_circuit_breakers table
// pre-exists in the OLD dexdat shape (001_init.sql section 29 — id BIGSERIAL
// PK, error_type, consecutive_errors, tenant_id; no breaker_type/threshold/
// current_count, no unique (session_id, breaker_type) constraint) while
// schema_versions records v3 as applied with the legacy length checksum.
// AutoMigrate must reconcile additively (columns + unique index), rewrite
// the legacy checksum to the content hash, and the harness upsert
// (INSERT ... ON CONFLICT (session_id, breaker_type)) must succeed.
//
// All 23 versions are recorded as applied (with legacy checksums) before
// AutoMigrate, exactly like the live dexdat DB — so Up() skips everything
// and only the startup repairs run. This also makes the test work on a bare
// PG without the pgvector extension (consensus 001's CREATE EXTENSION
// vector would otherwise fail on such instances).
// Gated by CONSENSUS_TEST_POSTGRES_URL.
func TestPostgresRepairCircuitBreakersOldShape(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres old-shape repair test")
	}

	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer database.Close()

	adminDB := database
	if pgdb, ok := database.(*pgpostgres.DB); ok {
		adminDB = pgdb.AdminDB()
	}
	if err := grantAgentRoleSchemaPrivs(ctx, pgURL); err != nil {
		t.Logf("skip agent_role schema grant (%v) — assuming fresh DB", err)
	}

	runner := New(adminDB)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	// Record ALL embedded versions as applied with legacy length checksums —
	// the live dexdat schema_versions state (v1-v20 applied 2026-07-14, all
	// length-hex). Up() will skip everything; only the repairs run.
	for _, m := range runner.migrations {
		if err := adminDB.Exec(ctx,
			`INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (version) DO NOTHING`,
			m.Version, m.Filename, "2026-07-14T07:34:00Z", legacyChecksum(m.SQL)); err != nil {
			t.Fatalf("record version %d: %v", m.Version, err)
		}
	}

	// Ensure sessions exists (the old-shape table references it). On a
	// scratch DB where the dexdat bootstrap already ran it does; on a bare
	// PG, create a minimal one.
	if err := adminDB.Exec(ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		agent_name TEXT NOT NULL,
		model_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'idle',
		tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
	)`); err != nil {
		t.Fatalf("ensure sessions table: %v", err)
	}

	// Replace any existing table with the OLD dexdat shape (001_init.sql
	// section 29) — this is the drift the sidecar hit. DROP is safe here:
	// this is a scratch test DB, and the repair under test is additive by
	// design (it must NOT drop/recreate on the live DB).
	if err := adminDB.Exec(ctx, `DROP TABLE IF EXISTS agent_circuit_breakers`); err != nil {
		t.Fatalf("drop existing circuit breakers: %v", err)
	}
	if err := adminDB.Exec(ctx, `CREATE TABLE agent_circuit_breakers (
		id              BIGSERIAL PRIMARY KEY,
		session_id      UUID NOT NULL REFERENCES sessions(id),
		error_type      TEXT NOT NULL,
		consecutive_errors INT NOT NULL DEFAULT 0,
		last_error_at   TIMESTAMPTZ,
		tripped         BOOLEAN NOT NULL DEFAULT false,
		tripped_at      TIMESTAMPTZ,
		reset_at        TIMESTAMPTZ,
		tenant_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
	)`); err != nil {
		t.Fatalf("create old-shape table: %v", err)
	}

	// AutoMigrate: Up() skips all recorded versions, repairs must reconcile.
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Columns added.
	for _, col := range []string{"breaker_type", "threshold", "current_count"} {
		if !pgCircuitBreakerHasColumn(t, adminDB, col) {
			t.Errorf("column %s MISSING after repair", col)
		}
	}
	// Old columns preserved (additive repair).
	for _, col := range []string{"id", "session_id", "error_type", "consecutive_errors", "tenant_id"} {
		if !pgCircuitBreakerHasColumn(t, adminDB, col) {
			t.Errorf("old column %s LOST after repair (must be additive)", col)
		}
	}
	// Unique (session_id, breaker_type) present.
	if !pgCircuitBreakerUniqueIndexExists(t, adminDB) {
		t.Error("unique (session_id, breaker_type) index MISSING after repair")
	}

	// Legacy checksum rewritten to the content hash (one-time transition).
	rows, err := adminDB.Query(ctx, `SELECT checksum FROM schema_versions WHERE version = 3`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("query v3 checksum: %v (rows=%d)", err, len(rows))
	}
	got, _ := rows[0]["checksum"].(string)
	var sql003 string
	for _, m := range runner.migrations {
		if m.Version == 3 {
			sql003 = m.SQL
			break
		}
	}
	if sql003 == "" {
		t.Fatal("could not find migration 003 embedded SQL")
	}
	if !strings.EqualFold(got, migrationChecksum(sql003)) {
		t.Errorf("v3 checksum not rewritten to content hash: got %q want %q", got, migrationChecksum(sql003))
	}

	// Consumer path: harness-shaped upsert must succeed. Seed model_registry
	// (FK from sessions on fully-migrated scratch DBs) and a session.
	if err := adminDB.Exec(ctx,
		`INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled)
		 VALUES ('test-model', 1, 8192, 1.0, 2.0, true)
		 ON CONFLICT (model_id) DO NOTHING`); err != nil {
		t.Logf("seed model_registry skipped (%v)", err)
	}
	if err := adminDB.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, tenant_id)
		 VALUES ('00000000-0000-0000-0000-0000000000aa', 'test-agent', 'test-model', 'idle', '00000000-0000-0000-0000-000000000000')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := adminDB.Exec(ctx,
		`INSERT INTO agent_circuit_breakers (session_id, breaker_type, threshold, current_count)
		 VALUES ('00000000-0000-0000-0000-0000000000aa', 'consecutive_errors', 5, 1)
		 ON CONFLICT (session_id, breaker_type) DO UPDATE SET current_count = 4, threshold = 5`); err != nil {
		t.Fatalf("harness upsert failed after repair: %v", err)
	}
	// Conflict path must also succeed.
	if err := adminDB.Exec(ctx,
		`INSERT INTO agent_circuit_breakers (session_id, breaker_type, threshold, current_count)
		 VALUES ('00000000-0000-0000-0000-0000000000aa', 'consecutive_errors', 5, 2)
		 ON CONFLICT (session_id, breaker_type) DO UPDATE SET current_count = 4, threshold = 5`); err != nil {
		t.Fatalf("harness upsert (conflict) failed after repair: %v", err)
	}
	rows, err = adminDB.Query(ctx,
		`SELECT current_count FROM agent_circuit_breakers WHERE session_id = '00000000-0000-0000-0000-0000000000aa' AND breaker_type = 'consecutive_errors'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("query repaired row: %v (rows=%d)", err, len(rows))
	}
	if got := toInt(rows[0]["current_count"]); got != 4 {
		t.Errorf("expected current_count=4 after conflict update, got %d", got)
	}
}
