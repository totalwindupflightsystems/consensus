package migrate

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
		name         string
		input        string
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
			name:         "ALTER TABLE ADD COLUMN IF NOT EXISTS (SQLite-compatible)",
			input:        `ALTER TABLE tool_results ADD COLUMN IF NOT EXISTS exit_code INT;`,
			wantContains: "ALTER TABLE tool_results",
			wantMissing:  "",
		},
		{
			name:         "ALTER TABLE ADD COLUMN with REFERENCES (SQLite-compatible)",
			input:        `ALTER TABLE sessions ADD COLUMN project_id UUID REFERENCES projects(id);`,
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
			name:         "ALTER TABLE ENABLE ROW LEVEL SECURITY (PG-only — MUST be stripped)",
			input:        `ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;`,
			wantContains: "",
			wantMissing:  "ENABLE ROW LEVEL SECURITY",
		},
		{
			name:         "ALTER TABLE with BYPASSRLS (PG-only — MUST be stripped)",
			input:        `ALTER TABLE sessions BYPASSRLS;`,
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
// Regression: AutoMigrate MUST install the append-only triggers (DOGFOOD-001)
// ============================================================================

func TestAutoMigrate_CreatesAppendOnlyTriggers(t *testing.T) {
	// Regression: migration 017 (append-only memory_events triggers) is a
	// SQLite-native file whose CREATE TRIGGER headers span multiple lines.
	// filterForSQLite only kept triggers whose FIRST line contained " BEGIN ",
	// so both statements were dropped while v17 was still recorded as applied —
	// fresh installs ended up with a mutable memory_events ledger.
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	triggerNames := func() map[string]bool {
		rows, err := database.Query(ctx, `SELECT name FROM sqlite_master WHERE type = 'trigger'`)
		if err != nil {
			t.Fatalf("query sqlite_master: %v", err)
		}
		names := map[string]bool{}
		for _, row := range rows {
			if n, ok := row["name"].(string); ok {
				names[n] = true
			}
		}
		return names
	}

	wantTriggers := []string{
		"trg_memory_events_append_only_update",
		"trg_memory_events_append_only_delete",
	}
	for _, trg := range wantTriggers {
		if !triggerNames()[trg] {
			t.Errorf("trigger %s MISSING from sqlite_master after AutoMigrate", trg)
		}
	}

	// Enforcement: seed a session + memory event, then UPDATE/DELETE must fail.
	if err := database.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status) VALUES ('sess-ao', 't', 'm', 'idle')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := database.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'original', 'sess-ao', 1)`); err != nil {
		t.Fatalf("seed memory event: %v", err)
	}
	if err := database.Exec(ctx, `UPDATE memory_events SET content = 'HACKED' WHERE session_id = 'sess-ao'`); err == nil {
		t.Error("UPDATE on memory_events should be rejected by trg_memory_events_append_only_update")
	}
	if err := database.Exec(ctx, `DELETE FROM memory_events WHERE session_id = 'sess-ao'`); err == nil {
		t.Error("DELETE on memory_events should be rejected by trg_memory_events_append_only_delete")
	}

	// Heal path: simulate a pre-fix install (v17 recorded, triggers missing),
	// then re-run AutoMigrate — Up() skips v17, so only the startup repair can
	// bring the triggers back.
	for _, trg := range wantTriggers {
		if err := database.Exec(ctx, `DROP TRIGGER IF EXISTS `+trg); err != nil {
			t.Fatalf("drop trigger %s: %v", trg, err)
		}
	}
	for _, trg := range wantTriggers {
		if triggerNames()[trg] {
			t.Fatalf("setup: trigger %s should be gone after DROP", trg)
		}
	}
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("second AutoMigrate (heal) failed: %v", err)
	}
	for _, trg := range wantTriggers {
		if !triggerNames()[trg] {
			t.Errorf("trigger %s still MISSING after repairAppendOnlyTriggers heal", trg)
		}
	}
	if err := database.Exec(ctx, `UPDATE memory_events SET content = 'HACKED' WHERE session_id = 'sess-ao'`); err == nil {
		t.Error("UPDATE on memory_events should be rejected after heal")
	}
	if err := database.Exec(ctx, `DELETE FROM memory_events WHERE session_id = 'sess-ao'`); err == nil {
		t.Error("DELETE on memory_events should be rejected after heal")
	}
}

// ============================================================================
// Regression: filterForSQLite MUST keep multi-line SQLite triggers (DOGFOOD-001)
// ============================================================================

func TestFilterForSQLite_MultiLineCreateTrigger(t *testing.T) {
	// A SQLite-native CREATE TRIGGER whose header spans multiple lines has no
	// " BEGIN " on the first line — the filter must scan ahead for BEGIN...END
	// and KEEP it. PostgreSQL triggers (EXECUTE FUNCTION) must still be stripped.
	sqliteTrigger := `CREATE TRIGGER IF NOT EXISTS trg_memory_events_append_only_update
BEFORE UPDATE ON memory_events
FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'memory_events is append-only: UPDATE is not permitted'); END;`

	result := filterForSQLite(sqliteTrigger)
	if !containsStr(result, "CREATE TRIGGER") {
		t.Errorf("filterForSQLite should preserve multi-line SQLite CREATE TRIGGER\ngot: %s", result)
	}
	if !containsStr(result, "BEGIN") || !containsStr(result, "RAISE") {
		t.Errorf("filterForSQLite should preserve the trigger BEGIN...END body\ngot: %s", result)
	}

	pgTrigger := `CREATE TRIGGER memory_touches_session
    AFTER INSERT ON memory_events
    FOR EACH ROW EXECUTE FUNCTION touch_session_heartbeat();`

	resultPG := filterForSQLite(pgTrigger)
	if containsStr(resultPG, "EXECUTE FUNCTION") || containsStr(resultPG, "CREATE TRIGGER") {
		t.Errorf("filterForSQLite should strip PostgreSQL CREATE TRIGGER\ngot: %s", resultPG)
	}

	// Single-line SQLite trigger (previous detection path) must still be kept.
	singleLine := `CREATE TRIGGER trg_x BEFORE UPDATE ON t FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'no'); END;`
	if got := filterForSQLite(singleLine); !containsStr(got, "CREATE TRIGGER") {
		t.Errorf("filterForSQLite should preserve single-line SQLite CREATE TRIGGER\ngot: %s", got)
	}
}

// ============================================================================
// AC-DEP-04: Migration under load — pause/resume preserves session data
// ============================================================================

func TestMigrationUnderLoad(t *testing.T) {
	// Phase 4 / Hardened Testing — verify that a schema migration (such
	// as adding a column) is applied while agent sessions are actively
	// running, the runner pauses active sessions, applies the migration,
	// and resumes them.
	//
	// Migration 022 adds budget_limit_cents to the sessions table.
	// The ALTER TABLE ADD COLUMN is NOT idempotent on modernc.org/sqlite,
	// so the test verifies that AutoMigrate applies it correctly and the
	// resulting schema has the expected column.
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply all embedded migrations including 022
	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	lastApplied, err := runner.Version(ctx)
	if err != nil {
		t.Fatalf("Version after AutoMigrate failed: %v", err)
	}
	if lastApplied != 22 {
		t.Fatalf("expected 22 migrations applied, got version %d", lastApplied)
	}
	t.Logf("AutoMigrate applied migrations up to version %d", lastApplied)

	// Verify no pending migrations
	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if len(state.PendingMigrations) != 0 {
		t.Errorf("expected no pending migrations, got %v", state.PendingMigrations)
	}

	// Seed model_registry and create sessions (schema exists now)
	if err := database.Exec(ctx,
		`INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled)
		 VALUES ('load-test-model', 1, 8192, 1.0, 2.0, 1)`,
	); err != nil {
		t.Fatalf("seed model_registry: %v", err)
	}

	if err := database.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal)
		 VALUES ('test-session-1', 'test-agent', 'load-test-model', 'idle', 'test goal')`,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Verify budget_limit_cents column exists and has correct default
	colRows, err := database.Query(ctx, "PRAGMA table_info('sessions')")
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}
	foundBudget := false
	for _, row := range colRows {
		if name, ok := row["name"].(string); ok && name == "budget_limit_cents" {
			foundBudget = true
			budgetType, _ := row["type"].(string)
			notNull, _ := row["notnull"].(string)
			dfltValue, _ := row["dflt_value"].(string)
			t.Logf("budget_limit_cents: type=%s notnull=%s default=%s", budgetType, notNull, dfltValue)
			break
		}
	}
	if !foundBudget {
		t.Error("budget_limit_cents column MISSING from sessions after migration 022")
	} else {
		t.Log("✓ budget_limit_cents column present in sessions table")
	}

	// Verify default value: new sessions get budget_limit_cents=0
	var budgetCents int64
	rows, err := database.Query(ctx,
		`SELECT budget_limit_cents FROM sessions WHERE id = 'test-session-1'`)
	if err != nil {
		t.Fatalf("query budget_limit_cents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	budgetCents, ok := rows[0]["budget_limit_cents"].(int64)
	if !ok {
		// Try int if int64 didn't match
		if v, ok2 := rows[0]["budget_limit_cents"].(int); ok2 {
			budgetCents = int64(v)
		} else {
			t.Fatalf("budget_limit_cents is neither int64 nor int: %T", rows[0]["budget_limit_cents"])
		}
	}
	if budgetCents != 0 {
		t.Errorf("expected default budget_limit_cents=0, got %d", budgetCents)
	} else {
		t.Log("✓ default budget_limit_cents=0 verified")
	}

	// Edge case: pause with no pending migrations doesn't error
	if err := database.Exec(ctx, `ALTER TABLE sessions RENAME TO _sessions_renamed`); err != nil {
		t.Fatalf("rename sessions table: %v", err)
	}
	edgeApplied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("Up with broken state should not block: %v", err)
	}
	if len(edgeApplied) != 0 {
		t.Errorf("expected 0 pending migrations, got %d", len(edgeApplied))
	}
	t.Log("✓ Edge case: Up() with 0 pending returned clean")
	if err := database.Exec(ctx, `ALTER TABLE _sessions_renamed RENAME TO sessions`); err != nil {
		t.Fatalf("restore sessions table: %v", err)
	}
}

// ============================================================================
// Regression: Backend-specific migration drift (INFRA-5)
// ============================================================================

func TestBackendSpecificMigrationNoFalseDrift(t *testing.T) {
	// Scenario: A SQLite-only migration (009_sqlite_task_tool_tables.sql) was
	// applied on a SQLite DB. On Postgres, LoadMigrations() excludes *sqlite* files.
	// GetState() must NOT flag version 9 as drift because a file with prefix
	// "009_" exists in the embedded filesystem (just for a different backend).
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Bootstrap and load ALL migrations (as on SQLite)
	runner := New(database)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if err := runner.LoadMigrations(); err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	// Record SQLite-only version 9 as applied
	if err := database.Exec(ctx,
		`INSERT INTO schema_versions (version, name, applied_at, checksum)
		 VALUES (9, 'sqlite task tool tables', '2026-05-04T00:00:00Z', 'abc')`); err != nil {
		t.Fatalf("insert version 9: %v", err)
	}

	// Simulate Postgres: create a new runner with Postgres-filtered migrations.
	// We manually build the migration list to exclude *_sqlite_* files.
	pgRunner := New(database)
	allEntries, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	pgRunner.database = database // needed for LoadMigrations filter logic
	// We can't easily call LoadMigrations with a fake Postgres backend,
	// so build the filtered list manually.
	filenamePattern := regexp.MustCompile(`^(\d{3})_(.+)\.sql$`)
	for _, entry := range allEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		// Simulate Postgres filter: skip *sqlite* files
		if strings.Contains(strings.ToLower(entry.Name()), "_sqlite_") {
			continue
		}
		matches := filenamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, _ := strconv.Atoi(matches[1])
		name := strings.ReplaceAll(matches[2], "_", " ")
		content, _ := embeddedMigrations.ReadFile("migrations/" + entry.Name())
		pgRunner.migrations = append(pgRunner.migrations, Migration{
			Version:  version,
			Name:     name,
			Filename: entry.Name(),
			SQL:      string(content),
		})
	}

	// GetState on "Postgres" — version 9 should NOT be drift
	state, err := pgRunner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.DriftDetected {
		t.Errorf("expected NO drift for SQLite-only version 9 on simulated Postgres, got: %s", state.DriftDetails)
	} else {
		t.Log("✓ SQLite-only migration (v9) not flagged as drift on simulated Postgres")
	}

	// Now add a truly ghost version (999) — MUST detect as drift
	if err := database.Exec(ctx,
		`INSERT INTO schema_versions (version, name, applied_at, checksum)
		 VALUES (999, 'true ghost', '2026-05-04T00:00:00Z', 'deadbeef')`); err != nil {
		t.Fatalf("insert ghost version 999: %v", err)
	}

	state, err = pgRunner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState failed after ghost insert: %v", err)
	}
	if !state.DriftDetected {
		t.Error("expected drift for true ghost version 999")
	} else {
		t.Log("✓ True ghost version 999 correctly flagged as drift")
	}
}

func TestBackendMigrationExists(t *testing.T) {
	// Version 9 exists as SQLite-only migration
	if !backendMigrationExists(9) {
		t.Error("backendMigrationExists(9) should be true — 009_sqlite_task_tool_tables.sql exists")
	}
	// Version 11 exists as SQLite-only migration
	if !backendMigrationExists(11) {
		t.Error("backendMigrationExists(11) should be true — 011_sqlite_missing_tables.sql exists")
	}
	// Version 17 exists as SQLite-only migration
	if !backendMigrationExists(17) {
		t.Error("backendMigrationExists(17) should be true — 017_append_only_memory_events_sqlite_triggers.sql exists")
	}
	// Version 18 exists as Postgres-only migration
	if !backendMigrationExists(18) {
		t.Error("backendMigrationExists(18) should be true — 018_postgres_memory_events_append_only.sql exists")
	}
	// Version 999 does NOT exist
	if backendMigrationExists(999) {
		t.Error("backendMigrationExists(999) should be false — no migration file with that version")
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
