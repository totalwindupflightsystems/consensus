// tenant_id_defaults_test.go — regression test for BUG-008.
//
// BUG-008: POST /api/v1/sessions/{id}/message returned HTTP 500
// (SQLSTATE 42501) because memory_events.tenant_id is UUID NOT NULL with no
// column default: the message handler's INSERT omits tenant_id, and under
// FORCE ROW LEVEL SECURITY the tenant_isolation WITH CHECK rejects the
// resulting NULL.
//
// The fix (migration 023_tenant_id_defaults.sql) sets the zero-UUID column
// default on every table that carries tenant_id without a default, mirroring
// the established sessions.tenant_id pattern.
//
// Why this test exists (and where the API-level repro can't): the internal/api
// integration tests run on SQLite, which has no RLS and whose schema has no
// tenant_id columns — the bug cannot reproduce there. This test drives the
// real Postgres migration artifact instead.
//
// Gated by CONSENSUS_TEST_POSTGRES_URL (same convention as postgres_test.go
// and postgres_full_test.go). Requires a DEDICATED scratch database — the
// test creates fixture tables in the public schema and will fail loudly if
// they already exist (i.e. the URL points at a non-scratch database).
//
// The admin DB runs as the table owner (no SET ROLE agent_role), exactly
// like the runtime migration path (internal/cli/models.go:
// migrate.New(dbdriver.AdminDB(database))). On a scratch database the
// agent_role grants from migrations 002/021 do not exist, so DDL and the
// role-independent SQL-level INSERT assertions belong here; the full RLS
// path is verified end-to-end on the live stack via the API repro.
//
// axiom:trace work_item=bug-008 spec=specs/003-database.md
package migrate

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

const zeroTenantUUID = "00000000-0000-0000-0000-000000000000"

// fixtureTables mirror the live alembic-managed schema shape: tenant_id
// UUID NOT NULL with no column default (the BUG-008 precondition). The set
// covers the tables the message handler touches plus a spread of other
// consensus tables carrying tenant_id.
var fixtureTables = []string{
	"memory_events",
	"tasks",
	"tool_requests",
	"tool_results",
	"agent_messages",
	"sessions",
}

// TestTenantIDDefaultsMigration asserts that migration 023:
//  1. is embedded in the binary (auto-applied on startup),
//  2. fixes the NOT NULL tenant_id violation for handler-shaped INSERTs,
//  3. covers every existing tenant_id column without a default,
//  4. is idempotent,
//  5. also covers tenant_id columns added AFTER it has run (the bug class is
//     covered by definition, not by a hardcoded table list).
func TestTenantIDDefaultsMigration(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres regression test")
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

	adminDB := driver.AdminDB(database)

	// ---- 1. The migration under test must be embedded ---------------------
	runner := New(adminDB)
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}
	var migrationSQL string
	for _, m := range runner.migrations {
		if m.Filename == "023_tenant_id_defaults.sql" {
			migrationSQL = m.SQL
			break
		}
	}
	if migrationSQL == "" {
		t.Fatal("migration 023_tenant_id_defaults.sql not found in embedded migrations")
	}

	// ---- 2. Set up the BUG-008 precondition -------------------------------
	// Plain CREATE TABLE (no IF NOT EXISTS): pointing this test at a
	// non-scratch database fails loudly instead of mutating real tables.
	if err := adminDB.Exec(ctx, "CREATE TABLE sessions (id uuid PRIMARY KEY, tenant_id uuid NOT NULL)"); err != nil {
		t.Fatalf("create fixture sessions: %v", err)
	}
	if err := adminDB.Exec(ctx,
		`CREATE TABLE memory_events (
			id BIGSERIAL PRIMARY KEY,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id UUID NOT NULL REFERENCES sessions(id),
			iteration_created BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			tenant_id UUID NOT NULL
		)`); err != nil {
		t.Fatalf("create fixture memory_events: %v", err)
	}
	for _, table := range fixtureTables {
		if table == "sessions" || table == "memory_events" {
			continue
		}
		if err := adminDB.Exec(ctx, "CREATE TABLE "+table+" (id uuid PRIMARY KEY, tenant_id uuid NOT NULL)"); err != nil {
			t.Fatalf("create fixture %s: %v", table, err)
		}
	}

	// A session row so the handler-shaped INSERT can reference it.
	sessionID := "11111111-1111-1111-1111-111111111111"
	if err := adminDB.Exec(ctx,
		`INSERT INTO sessions (id, tenant_id) VALUES ($1, $2)`, sessionID, "22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatalf("seed fixture session: %v", err)
	}

	// Handler-shaped INSERT (internal/api/sessions.go handleSessionMessage):
	// omits tenant_id entirely. Before the fix this must FAIL (NULL violates
	// tenant_id NOT NULL; under RLS on the live stack this is SQLSTATE 42501).
	handlerInsert := `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		VALUES ('user_message', $1, $2, $3, $4)`
	if err := adminDB.Exec(ctx, handlerInsert, "hello", sessionID, int64(1), "2026-08-06T00:00:00Z"); err == nil {
		t.Fatal("expected handler-shaped INSERT to fail before migration 023 (tenant_id NOT NULL, no default)")
	} else {
		t.Logf("pre-migration INSERT correctly rejected: %v", err)
	}

	// ---- 3. Apply migration 023 and assert the defaults -------------------
	if err := adminDB.Exec(ctx, migrationSQL); err != nil {
		t.Fatalf("applying migration 023 failed: %v", err)
	}

	// Every fixture table's tenant_id now carries the zero-UUID default.
	for _, table := range fixtureTables {
		rows, err := adminDB.Query(ctx,
			`SELECT column_default FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'tenant_id'`,
			table)
		if err != nil {
			t.Fatalf("query default for %s: %v", table, err)
		}
		if len(rows) != 1 {
			t.Errorf("expected exactly one tenant_id column on %s, got %d", table, len(rows))
			continue
		}
		def, _ := rows[0]["column_default"].(string)
		if !strings.Contains(def, zeroTenantUUID) {
			t.Errorf("table %s: tenant_id default = %q, want zero UUID %q", table, def, zeroTenantUUID)
		}
	}

	// ---- 4. The handler-shaped INSERT now succeeds and persists -----------
	if err := adminDB.Exec(ctx, handlerInsert, "hello", sessionID, int64(1), "2026-08-06T00:00:00Z"); err != nil {
		t.Fatalf("handler-shaped INSERT failed after migration 023: %v", err)
	}
	rows, err := adminDB.Query(ctx,
		`SELECT tenant_id::text AS tenant_id FROM memory_events WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query persisted memory_events row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 persisted memory_events row, got %d", len(rows))
	}
	tenantID, _ := rows[0]["tenant_id"].(string)
	if tenantID != zeroTenantUUID {
		t.Errorf("persisted tenant_id = %q, want zero UUID %q", tenantID, zeroTenantUUID)
	}

	// ---- 5. Idempotent: a second run is a clean no-op ---------------------
	if err := adminDB.Exec(ctx, migrationSQL); err != nil {
		t.Fatalf("re-applying migration 023 failed (must be idempotent): %v", err)
	}

	// ---- 6. Dynamic coverage: tables created AFTER 023 also get the fix ---
	if err := adminDB.Exec(ctx, "CREATE TABLE tenant_late_add (id uuid PRIMARY KEY, tenant_id uuid NOT NULL)"); err != nil {
		t.Fatalf("create tenant_late_add: %v", err)
	}
	if err := adminDB.Exec(ctx, migrationSQL); err != nil {
		t.Fatalf("applying migration 023 after late table creation failed: %v", err)
	}
	rows, err = adminDB.Query(ctx,
		`SELECT column_default FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'tenant_late_add' AND column_name = 'tenant_id'`)
	if err != nil {
		t.Fatalf("query default for tenant_late_add: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one tenant_id column on tenant_late_add, got %d", len(rows))
	}
	def, _ := rows[0]["column_default"].(string)
	if !strings.Contains(def, zeroTenantUUID) {
		t.Errorf("tenant_late_add: tenant_id default = %q, want zero UUID %q", def, zeroTenantUUID)
	}
}
