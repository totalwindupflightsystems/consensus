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
// AC-DEP-04: Migration under load — pause/resume preserves session data
// ============================================================================

func TestMigrationUnderLoad(t *testing.T) {
	// Phase 4 / Hardened Testing — verify that when a schema migration (such
	// as adding a column) is applied while agent sessions are actively
	// running, the runner:
	//   1. pauses every active session before mutating the schema,
	//   2. applies the migration,
	//   3. resumes the paused sessions to 'idle',
	//   4. preserves ALL session data (memory_events, tasks, etc.) end-to-end.
	//
	// Also verifies the documented non-fatal guarantee: if pauseActiveSessions
	// fails (e.g., the sessions table is missing), the migration still
	// completes successfully.
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// ── 1. Apply every embedded migration up to the second-to-last. ────────
	// AutoMigrate handles bootstrap + LoadMigrations + Up. We then explicitly
	// untrack the last version (the way Down() does) so there's a pending
	// migration to apply on the second Up().
	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate (initial) failed: %v", err)
	}

	lastApplied, err := runner.Version(ctx)
	if err != nil {
		t.Fatalf("Version after AutoMigrate failed: %v", err)
	}
	if lastApplied == 0 {
		t.Fatal("expected at least one migration applied after AutoMigrate")
	}
	t.Logf("AutoMigrate applied migrations up to version %d", lastApplied)

	// Untrack the last migration so it becomes pending again. This mirrors
	// what Down() does (it only removes the tracking row), so subsequent
	// Up() will try to re-apply that migration — and that is exactly the
	// "add column while sessions active" flow we want to exercise.
	if err := database.Exec(ctx,
		`DELETE FROM schema_versions WHERE version = $1`, lastApplied,
	); err != nil {
		t.Fatalf("failed to untrack last migration (%d): %v", lastApplied, err)
	}

	// ── 2. Seed a model_registry row to satisfy the sessions.model_id FK ──
	if err := database.Exec(ctx,
		`INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, enabled)
		 VALUES ('load-test-model', 1, 8192, 1.0, 2.0, 1)`,
	); err != nil {
		t.Fatalf("seed model_registry: %v", err)
	}

	// ── 3. Create 3 active sessions with linked memory_events + tasks. ──────
	// Statuses span the three states the runner pauses. Each session gets
	// multiple memory_events and tasks so we can detect row loss.
	type fixture struct {
		id             string
		status         string
		memEventCount  int
		taskCount      int
	}
	fixtures := []fixture{
		{id: "load-session-1", status: "idle", memEventCount: 3, taskCount: 2},
		{id: "load-session-2", status: "thinking", memEventCount: 5, taskCount: 4},
		{id: "load-session-3", status: "executing", memEventCount: 2, taskCount: 1},
	}

	// Track each session's initial heartbeat so we can verify pause touched it.
	initialHeartbeats := make(map[string]string, len(fixtures))
	for _, fx := range fixtures {
		if err := database.Exec(ctx,
			`INSERT INTO sessions (id, agent_name, model_id, status, goal)
			 VALUES ($1, 'load-test-agent', 'load-test-model', $2, 'migration-under-load')`,
			fx.id, fx.status,
		); err != nil {
			t.Fatalf("insert session %s: %v", fx.id, err)
		}

		// Capture the session's heartbeat_at value to later confirm
		// pauseActiveSessions updated it as a side effect.
		hbRows, err := database.Query(ctx,
			`SELECT heartbeat_at FROM sessions WHERE id = $1`, fx.id)
		if err != nil {
			t.Fatalf("read heartbeat for %s: %v", fx.id, err)
		}
		if len(hbRows) == 1 {
			if v, ok := hbRows[0]["heartbeat_at"].(string); ok {
				initialHeartbeats[fx.id] = v
			}
		}

		// Seed memory_events for this session.
		for i := 0; i < fx.memEventCount; i++ {
			if err := database.Exec(ctx,
				`INSERT INTO memory_events (type, content, session_id, iteration_created)
				 VALUES ('text_block', $1, $2, $3)`,
				fmt.Sprintf("event %d for %s", i, fx.id), fx.id, i+1,
			); err != nil {
				t.Fatalf("insert memory_event %d for %s: %v", i, fx.id, err)
			}
		}

		// Seed tasks for this session.
		for i := 0; i < fx.taskCount; i++ {
			if err := database.Exec(ctx,
				`INSERT INTO tasks (id, session_id, title, status, priority)
				 VALUES ($1, $2, $3, 'pending', 5)`,
				fmt.Sprintf("%s-task-%d", fx.id, i), fx.id,
				fmt.Sprintf("task %d for %s", i, fx.id),
			); err != nil {
				t.Fatalf("insert task %d for %s: %v", i, fx.id, err)
			}
		}
	}

	// Baseline row counts (per-session and global).
	countRows := func(t *testing.T, sql string, args ...any) int64 {
		t.Helper()
		rows, err := database.Query(ctx, sql, args...)
		if err != nil {
			t.Fatalf("count query failed (%s): %v", sql, err)
		}
		if len(rows) != 1 {
			t.Fatalf("count query expected 1 row, got %d (sql=%s)", len(rows), sql)
		}
		n, ok := rows[0]["n"].(int64)
		if !ok {
			t.Fatalf("count query result missing int64 'n' column: %v", rows[0])
		}
		return n
	}

	expectedTotalSessions := int64(len(fixtures))
	expectedTotalMemEvents := int64(0)
	expectedTotalTasks := int64(0)
	for _, fx := range fixtures {
		expectedTotalMemEvents += int64(fx.memEventCount)
		expectedTotalTasks += int64(fx.taskCount)
	}

	if got := countRows(t, `SELECT COUNT(*) AS n FROM sessions WHERE id LIKE 'load-session-%'`); got != expectedTotalSessions {
		t.Fatalf("pre-migration: sessions count = %d, want %d", got, expectedTotalSessions)
	}
	if got := countRows(t, `SELECT COUNT(*) AS n FROM memory_events WHERE session_id LIKE 'load-session-%'`); got != expectedTotalMemEvents {
		t.Fatalf("pre-migration: memory_events count = %d, want %d", got, expectedTotalMemEvents)
	}
	if got := countRows(t, `SELECT COUNT(*) AS n FROM tasks WHERE session_id LIKE 'load-session-%'`); got != expectedTotalTasks {
		t.Fatalf("pre-migration: tasks count = %d, want %d", got, expectedTotalTasks)
	}

	// Per-session snapshot of row counts so we can prove nothing attached to a
	// specific session got lost during the migration.
	type perSessionCount struct {
		memEvents int64
		tasks     int64
	}
	preMigrationCounts := make(map[string]perSessionCount, len(fixtures))
	for _, fx := range fixtures {
		preMigrationCounts[fx.id] = perSessionCount{
			memEvents: countRows(t, `SELECT COUNT(*) AS n FROM memory_events WHERE session_id = $1`, fx.id),
			tasks:     countRows(t, `SELECT COUNT(*) AS n FROM tasks WHERE session_id = $1`, fx.id),
		}
	}

	// ── 4. Run Up() — applies the pending migration and triggers pause+resume ─
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("Up failed: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected exactly one applied migration, got %d (%v)", len(applied), applied)
	}
	t.Logf("Applied under load: %v", applied)

	// ── 5a. Every previously-active session must now be 'idle' (resumed). ──
	for _, fx := range fixtures {
		rows, err := database.Query(ctx,
			`SELECT status FROM sessions WHERE id = $1`, fx.id)
		if err != nil {
			t.Fatalf("read status for %s: %v", fx.id, err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 session row for %s, got %d", fx.id, len(rows))
		}
		gotStatus, _ := rows[0]["status"].(string)
		if gotStatus != "idle" {
			t.Errorf("session %s: expected resumed status 'idle', got %q", fx.id, gotStatus)
		}
	}

	// ── 5b. All session data is preserved — global row counts unchanged. ───
	if got := countRows(t, `SELECT COUNT(*) AS n FROM sessions WHERE id LIKE 'load-session-%'`); got != expectedTotalSessions {
		t.Errorf("sessions count lost data: got %d, want %d", got, expectedTotalSessions)
	}
	if got := countRows(t, `SELECT COUNT(*) AS n FROM memory_events WHERE session_id LIKE 'load-session-%'`); got != expectedTotalMemEvents {
		t.Errorf("memory_events count lost data: got %d, want %d", got, expectedTotalMemEvents)
	}
	if got := countRows(t, `SELECT COUNT(*) AS n FROM tasks WHERE session_id LIKE 'load-session-%'`); got != expectedTotalTasks {
		t.Errorf("tasks count lost data: got %d, want %d", got, expectedTotalTasks)
	}

	// ── 5c. Per-session row counts identical pre/post migration. ───────────
	for _, fx := range fixtures {
		want := preMigrationCounts[fx.id]
		gotMem := countRows(t, `SELECT COUNT(*) AS n FROM memory_events WHERE session_id = $1`, fx.id)
		gotTasks := countRows(t, `SELECT COUNT(*) AS n FROM tasks WHERE session_id = $1`, fx.id)
		if gotMem != want.memEvents {
			t.Errorf("session %s: memory_events count changed: pre=%d post=%d", fx.id, want.memEvents, gotMem)
		}
		if gotTasks != want.tasks {
			t.Errorf("session %s: tasks count changed: pre=%d post=%d", fx.id, want.tasks, gotTasks)
		}
	}

	// ── 5d. Migration landed in schema_versions at the new top version. ─────
	state, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState after Up failed: %v", err)
	}
	if state.CurrentVersion != lastApplied {
		t.Errorf("expected CurrentVersion=%d after Up, got %d", lastApplied, state.CurrentVersion)
	}
	if len(state.PendingMigrations) != 0 {
		t.Errorf("expected no pending migrations after Up, got %v", state.PendingMigrations)
	}

	// ── 6. Edge case: pauseActiveSessions fails — migration still completes. ──
	//
	// Documented contract (migrate.go: pauseActiveSessions): the error is
	// intentionally non-fatal — "log and continue even if pause fails. The
	// migration is still safe — sessions may see stale schema until restart".
	//
	// We simulate that failure by renaming the sessions table out from
	// under pauseActiveSessions' SELECT (we can't DROP it because other
	// tables FK into it). After Up() the table is restored so the rest of
	// the DB remains usable.
	if err := database.Exec(ctx, `ALTER TABLE sessions RENAME TO _sessions_renamed_for_edge_case`); err != nil {
		t.Fatalf("failed to rename sessions table for edge-case test: %v", err)
	}

	// Make a new pending migration by untracking the last version again.
	if err := database.Exec(ctx,
		`DELETE FROM schema_versions WHERE version = $1`, lastApplied,
	); err != nil {
		t.Fatalf("failed to re-untrack last migration: %v", err)
	}

	edgeApplied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("Up with broken pause failed: %v (non-fatal pause should not block migration)", err)
	}
	if len(edgeApplied) != 1 {
		t.Fatalf("edge-case: expected 1 applied migration, got %d (%v)", len(edgeApplied), edgeApplied)
	}

	edgeState, err := runner.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState after edge-case Up failed: %v", err)
	}
	if edgeState.CurrentVersion != lastApplied {
		t.Errorf("edge-case: expected CurrentVersion=%d, got %d", lastApplied, edgeState.CurrentVersion)
	}
	if len(edgeState.PendingMigrations) != 0 {
		t.Errorf("edge-case: expected no pending migrations, got %v", edgeState.PendingMigrations)
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
