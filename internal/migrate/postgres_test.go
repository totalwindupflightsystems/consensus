// postgres_test.go — Postgres migration bootstrap integration test.
//
// Gated by CONSCIENCE_TEST_POSTGRES_URL env var. Skips gracefully when unset.
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
	"testing"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/db/driver"
)

func TestPostgresBootstrap(t *testing.T) {
	pgURL := os.Getenv("CONSCIENCE_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSCIENCE_TEST_POSTGRES_URL not set; skipping Postgres integration test")
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
}
