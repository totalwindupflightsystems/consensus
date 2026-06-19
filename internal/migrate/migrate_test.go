package migrate

import (
	"context"
	"fmt"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// AC-DEP-01: Embedded migrations + binary build
// ============================================================================

func TestEmbeddedMigrationsExist(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-1
	entries, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		t.Fatalf("failed to read embedded migrations: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no embedded migration files found")
	}

	var filenames []string
	for _, e := range entries {
		filenames = append(filenames, e.Name())
	}

	t.Logf("Embedded migrations: %v", filenames)

	found001 := false
	found002 := false
	for _, fn := range filenames {
		if fn == "001_initial_schema.sql" {
			found001 = true
		}
		if fn == "002_shim_session_map.sql" {
			found002 = true
		}
	}

	if !found001 {
		t.Error("missing embedded migration: 001_initial_schema.sql")
	}
	if !found002 {
		t.Error("missing embedded migration: 002_shim_session_map.sql")
	}
}

func TestLoadMigrations(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-1
	runner := &Runner{}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	if len(runner.migrations) == 0 {
		t.Fatal("no migrations loaded")
	}

	prevVersion := 0
	for _, m := range runner.migrations {
		if m.Version <= prevVersion {
			t.Errorf("migration versions not strictly increasing: %d after %d", m.Version, prevVersion)
		}
		if m.Name == "" {
			t.Errorf("migration version %d has empty name", m.Version)
		}
		if m.SQL == "" {
			t.Errorf("migration %s has empty SQL content", m.Filename)
		}
		prevVersion = m.Version
		t.Logf("Migration %03d: %s (%d bytes)", m.Version, m.Filename, len(m.SQL))
	}
}

// ============================================================================
// AC-DEP-02: Auto-migrate on startup; drift pauses agents
// ============================================================================

func TestBootstrapAndStateTracking(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	// Bootstrap
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Bootstrap idempotency
	for i := 0; i < 3; i++ {
		if err := runner.Bootstrap(ctx); err != nil {
			t.Fatalf("Bootstrap attempt %d failed: %v", i+1, err)
		}
	}

	// Load migrations
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	// Initial state: no migrations applied
	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.CurrentVersion != 0 {
		t.Errorf("expected initial version 0, got %d", state.CurrentVersion)
	}

	// Manually record a migration as applied
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (1, 'initial schema', '2026-05-04T00:00:00Z', 'abc123')`); err != nil {
		t.Fatalf("insert migration record: %v", err)
	}

	state, err = runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState after insert failed: %v", err)
	}
	if state.CurrentVersion != 1 {
		t.Errorf("expected version 1, got %d", state.CurrentVersion)
	}
	if len(state.AppliedMigrations) == 0 {
		t.Error("expected at least one applied migration")
	}
	t.Logf("Applied migrations: %v", state.AppliedMigrations)
}

func TestDriftDetection(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	// Insert a ghost migration that has no embedded file
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (999, 'ghost migration', '2026-05-04T00:00:00Z', 'deadbeef')`); err != nil {
		t.Fatalf("insert fake migration: %v", err)
	}

	drifted, details, err := runner.CheckDrift(ctx)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if !drifted {
		t.Error("expected drift to be detected for ghost migration version 999")
	}
	if details == "" {
		t.Error("expected drift details to be non-empty")
	}
	t.Logf("Drift detected: %s", details)
}

func TestVersionBeforeAndAfterMigrate(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	v, err := runner.Version(ctx)
	if err != nil {
		t.Fatalf("Version failed: %v", err)
	}
	if v != 0 {
		t.Errorf("expected version 0 before migration, got %d", v)
	}

	// Bootstrap and record a migration
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (1, 'first', '2026-05-04T00:00:00Z', 'abc')`); err != nil {
		t.Fatalf("insert version 1: %v", err)
	}
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (2, 'second', '2026-05-04T00:00:00Z', 'def')`); err != nil {
		t.Fatalf("insert version 2: %v", err)
	}

	v, err = runner.Version(ctx)
	if err != nil {
		t.Fatalf("Version after insert failed: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}
	t.Logf("Current version: %d", v)
}

func TestDownRollback(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Record version 1
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (1, 'first', '2026-05-04T00:00:00Z', 'abc')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Roll it back
	result, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("Down failed: %v", err)
	}
	t.Logf("Down result: %s", result)

	v, err := runner.Version(ctx)
	if err != nil {
		t.Fatalf("Version after down failed: %v", err)
	}
	if v != 0 {
		t.Errorf("expected version 0 after rollback, got %d", v)
	}
}

func TestDownNothingToRollback(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	_, err := runner.Down(ctx)
	if err == nil {
		t.Error("expected error when nothing to roll back")
	}
	t.Logf("Expected error: %v", err)
}

func TestNoMigrationsLoaded_EmptyPending(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Without loading migrations, there are none to apply
	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if state.MigrationRequired {
		t.Error("expected no migration required when no files loaded")
	}
}

// ============================================================================
// Regression: filterForSQLite MUST preserve SQLite-compatible ALTER TABLE
// ============================================================================

func TestFilterForSQLite_PreservesAlterTableAddColumn(t *testing.T) {
	// Regression: migration 013 (trust_level) was silently stripped by filterForSQLite
	// because multi-line ALTER TABLE entered mAlterTable mode with no exit condition.
	// The column was never added to sessions but migration was recorded as "applied".
	//
	// This test verifies that ALTER TABLE ADD COLUMN (SQLite-compatible) is preserved
	// and that ALTER TABLE ADD CONSTRAINT FOREIGN KEY (PG-only) is stripped.
	tests := []struct {
		name       string
		input      string
		wantContains string
		wantMissing  string
	}{
		{
			name: "ALTER TABLE ADD COLUMN (SQLite-compatible — MUST be preserved)",
			input: `BEGIN;
ALTER TABLE sessions
ADD COLUMN trust_level TEXT NOT NULL DEFAULT 'high'
CHECK (trust_level IN ('low', 'medium', 'high'));
COMMIT;`,
			wantContains: "ALTER TABLE sessions",
			wantMissing:  "",
		},
		{
			name: "ALTER TABLE ADD COLUMN IF NOT EXISTS (SQLite-compatible)",
			input: `ALTER TABLE tool_results ADD COLUMN IF NOT EXISTS exit_code INT;`,
			wantContains: "ALTER TABLE tool_results",
			wantMissing:  "",
		},
		{
			name: "ALTER TABLE ADD COLUMN with REFERENCES (SQLite-compatible)",
			input: `ALTER TABLE sessions ADD COLUMN project_id UUID REFERENCES projects(id);`,
			wantContains: "ALTER TABLE sessions ADD COLUMN project_id",
			wantMissing:  "",
		},
		{
			name: "ALTER TABLE ADD CONSTRAINT FOREIGN KEY (PG-only — MUST be stripped)",
			input: `ALTER TABLE sessions
ADD CONSTRAINT fk_sessions_model
FOREIGN KEY (model_id) REFERENCES model_registry(model_id);`,
			wantContains: "",
			wantMissing:  "ADD CONSTRAINT",
		},
		{
			name: "ALTER TABLE ENABLE ROW LEVEL SECURITY (PG-only — MUST be stripped)",
			input: `ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;`,
			wantContains: "",
			wantMissing:  "ENABLE ROW LEVEL SECURITY",
		},
		{
			name: "ALTER TABLE with BYPASSRLS (PG-only — MUST be stripped)",
			input: `ALTER TABLE sessions BYPASSRLS;`,
			wantContains: "",
			wantMissing:  "BYPASSRLS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterForSQLite(tt.input)
			if tt.wantContains != "" && !containsStr(result, tt.wantContains) {
				t.Errorf("filterForSQLite should preserve %q\ngot: %s", tt.wantContains, result)
			}
			if tt.wantMissing != "" && containsStr(result, tt.wantMissing) {
				t.Errorf("filterForSQLite should strip %q\ngot: %s", tt.wantMissing, result)
			}
			// Result must never be empty without reason
			if result == "" && tt.wantContains != "" {
				t.Errorf("filterForSQLite returned empty result but should have preserved content")
			}
		})
	}
}

// containsStr is a case-insensitive substring check.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && indexOfStr(s, substr) >= 0
}

func indexOfStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			pc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if pc >= 'A' && pc <= 'Z' {
				pc += 32
			}
			if sc != pc {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestUpBlockedByDrift(t *testing.T) {
	// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)

	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	// Insert a ghost migration to create drift
	if err := database.Exec(ctx, `INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES (999, 'ghost', '2026-05-04T00:00:00Z', 'x')`); err != nil {
		t.Fatalf("insert ghost: %v", err)
	}

	// Now the next pending migration (001) should be blocked by drift
	// The first migration IS 001_initial_schema.sql which is not yet applied
	applied, err := runner.Up(ctx)
	if err == nil {
		t.Error("expected Up to fail due to drift")
		_ = applied
	}
	t.Logf("Up correctly blocked: %v", err)
}

// ============================================================================
// Regression: AutoMigrate MUST create all columns declared in migrations
// ============================================================================

func TestAutoMigrate_CreatesTrustLevelColumn(t *testing.T) {
	// Regression: migration 013 (trust_level) was recorded as "applied" but the
	// ALTER TABLE ADD COLUMN was silently stripped by filterForSQLite.
	// This test verifies that after AutoMigrate on a fresh SQLite DB, the
	// sessions table has a trust_level column.
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)
	_, err := runner.AutoMigrate(ctx)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Verify trust_level exists on sessions table
	rows, err := database.Query(ctx, "PRAGMA table_info('sessions')")
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}

	found := false
	for _, row := range rows {
		if name, ok := row["name"].(string); ok && name == "trust_level" {
			found = true
			break
		}
	}
	if !found {
		// Dump available columns for debugging
		var cols []string
		for _, row := range rows {
			if name, ok := row["name"].(string); ok {
				cols = append(cols, name)
			}
		}
		t.Errorf("trust_level column MISSING from sessions after AutoMigrate. Available columns: %v", cols)
	}
}

// ============================================================================
// Test Helpers
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Logf("warning: failed to close test database: %v", err)
		}
	}

	return database, cleanup
}
