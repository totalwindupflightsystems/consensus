// Package migrate implements the schema migration runner for Consensus.
//
// Migration SQL files are embedded in the binary and auto-applied on startup.
// A schema_versions table tracks which migrations have been applied.
// Drift detection ensures the database schema matches expectations.
//
// axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-1/step-1-1-1
package migrate

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

//go:embed migrations/*
var embeddedMigrations embed.FS

// Migration represents a single schema migration file.
type Migration struct {
	Version  int
	Name     string
	Filename string
	SQL      string
}

// State represents the current schema state.
type State struct {
	CurrentVersion    int
	AppliedMigrations []string
	PendingMigrations []string
	DriftDetected     bool
	DriftDetails      string
	MigrationRequired bool
}

// Runner manages schema migrations.
type Runner struct {
	database   db.DB
	migrations []Migration
}

// New creates a new migration runner.
func New(database db.DB) *Runner {
	return &Runner{
		database: database,
	}
}

// ============================================================================
// Bootstrap — Create schema_versions table if it doesn't exist
// ============================================================================

const bootstrapSQL = `
CREATE TABLE IF NOT EXISTS schema_versions (
	version     INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	applied_at  TEXT NOT NULL,
	checksum    TEXT NOT NULL
);
`

// Bootstrap creates the schema_versions tracking table if needed.
// This is safe to call on every startup; it's idempotent.
func (r *Runner) Bootstrap(ctx context.Context) error {
	return r.database.Exec(ctx, bootstrapSQL)
}

// ============================================================================
// Loading Migrations
// ============================================================================

// LoadMigrations reads embedded migration files and parses them into Migration structs.
// Files must follow the naming convention: NNN_description.sql
func (r *Runner) LoadMigrations() error {
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: failed to read embedded migrations: %w", err)
	}

	var migrations []Migration
	filenamePattern := regexp.MustCompile(`^(\d{3})_(.+)\.sql$`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Skip SQLite-specific migrations when running on Postgres.
		// Files named *_sqlite_*.sql are SQLite-only schema additions
		// (e.g., trigger emulation, SQLite-specific tables).
		if r.database != nil && db.DetectBackendFromDB(r.database) == db.BackendPostgres {
			if strings.Contains(strings.ToLower(entry.Name()), "_sqlite_") {
				continue
			}
		}

		// Skip Postgres-specific migrations when running on SQLite.
		// Files named *_postgres_*.sql are Postgres-only schema additions
		// (e.g., PL/pgSQL trigger functions, Postgres-specific extensions).
		if r.database != nil && db.DetectBackendFromDB(r.database) == db.BackendSQLite {
			if strings.Contains(strings.ToLower(entry.Name()), "_postgres_") {
				continue
			}
		}

		matches := filenamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue // Skip files that don't match naming convention
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		name := strings.ReplaceAll(matches[2], "_", " ")

		content, err := embeddedMigrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("migrate: failed to read migration file %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version:  version,
			Name:     name,
			Filename: entry.Name(),
			SQL:      string(content),
		})
	}

	// Sort by version ascending
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	r.migrations = migrations
	return nil
}

// ============================================================================
// Status
// ============================================================================

// GetState returns the current schema migration state.
func (r *Runner) GetState(ctx context.Context) (*State, error) {
	state := &State{}

	// Get applied migrations from tracking table
	rows, err := r.database.Query(ctx, `SELECT version, name, checksum FROM schema_versions ORDER BY version ASC`)
	if err != nil {
		// If the table doesn't exist yet, it means no migrations have been applied
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "no such table") {
			state.CurrentVersion = 0
			for _, m := range r.migrations {
				state.PendingMigrations = append(state.PendingMigrations, m.Filename)
			}
			state.MigrationRequired = len(state.PendingMigrations) > 0
			return state, nil
		}
		return nil, fmt.Errorf("migrate: failed to query schema_versions: %w", err)
	}

	var appliedVersions []int
	appliedChecksums := make(map[int]string, len(rows))
	for _, row := range rows {
		v, ok := row["version"]
		if !ok {
			continue
		}
		version := toInt(v)
		name, _ := row["name"].(string)
		checksum, _ := row["checksum"].(string)
		appliedVersions = append(appliedVersions, version)
		appliedChecksums[version] = checksum
		state.AppliedMigrations = append(state.AppliedMigrations, fmt.Sprintf("%03d_%s", version, name))

		if version > state.CurrentVersion {
			state.CurrentVersion = version
		}
	}

	// Find pending migrations
	for _, m := range r.migrations {
		found := false
		for _, av := range appliedVersions {
			if av == m.Version {
				found = true
				break
			}
		}
		if !found {
			state.PendingMigrations = append(state.PendingMigrations, m.Filename)
		}
	}

	// Build a lookup of embedded migrations by version for checksum comparison.
	embeddedByVersion := make(map[int]Migration, len(r.migrations))
	for _, m := range r.migrations {
		embeddedByVersion[m.Version] = m
	}

	// Drift detection: are there applied migrations that don't have embedded files?
	for _, av := range appliedVersions {
		m, found := embeddedByVersion[av]
		if found {
			// Checksum comparison: the stored checksum must match either the
			// current content hash (migrationChecksum) OR the legacy
			// length-based hex (legacyChecksum) — otherwise the embedded file
			// content differs from what was recorded (BUG-012).
			stored, _ := appliedChecksums[av]
			if stored != "" &&
				!strings.EqualFold(stored, migrationChecksum(m.SQL)) &&
				!strings.EqualFold(stored, legacyChecksum(m.SQL)) {
				state.DriftDetected = true
				state.DriftDetails += fmt.Sprintf(
					"Applied migration %03d (checksum %s) does not match embedded file checksum %s (recorded-but-different-content)\n",
					av, stored, migrationChecksum(m.SQL))
			}
			continue
		}
		// No matching embedded file for this applied version.
		// Check if a file with this version exists for a different backend.
		// When migrating from SQLite to Postgres (or vice versa), the other
		// backend's migrations are filtered out by LoadMigrations() but the
		// version is still recorded in schema_versions. This is NOT drift.
		if backendMigrationExists(av) {
			continue
		}
		state.DriftDetected = true
		state.DriftDetails += fmt.Sprintf("Applied migration version %d has no matching embedded file\n", av)
	}

	state.MigrationRequired = len(state.PendingMigrations) > 0
	return state, nil
}

// ============================================================================
// Up (Apply)
// ============================================================================

// Up applies all pending migrations in order.
// Before migration, active agent sessions are paused to prevent drift-related
// data races. After migration completes, paused sessions are resumed.
func (r *Runner) Up(ctx context.Context) ([]string, error) {
	state, err := r.GetState(ctx)
	if err != nil {
		return nil, err
	}

	if state.DriftDetected && len(state.PendingMigrations) > 0 {
		return nil, fmt.Errorf("migrate: schema drift detected; cannot apply new migrations until drift is resolved:\n%s", state.DriftDetails)
	}

	if !state.MigrationRequired {
		return nil, nil // Already up to date
	}

	// Pause all active agent sessions before migration to prevent data races
	pausedSessions, err := r.pauseActiveSessions(ctx)
	if err != nil {
		// Non-fatal: log and continue even if pause fails
		// The migration is still safe — sessions may see stale schema until restart
	}

	var applied []string
	for _, m := range r.migrations {
		// Check if already applied
		alreadyApplied := false
		for _, fn := range state.AppliedMigrations {
			if fn == m.Filename || strings.HasSuffix(fn, m.Filename) || strings.HasPrefix(m.Filename, fn[:3]) {
				alreadyApplied = true
				break
			}
		}
		if alreadyApplied {
			continue
		}

		sql := m.SQL
		if db.DetectBackendFromDB(r.database) == db.BackendSQLite {
			// SQLite-native migrations (filename contains "_sqlite_") are written
			// for SQLite by construction — the same convention LoadMigrations uses
			// to include them only on SQLite. Skip the PG→SQLite filter for them:
			// filterForSQLite strips CREATE TRIGGER statements whose header spans
			// multiple lines, which silently dropped migration 017's append-only
			// triggers while still recording v17 as applied (DOGFOOD-001).
			if !strings.Contains(strings.ToLower(m.Filename), "_sqlite_") {
				sql = filterForSQLite(sql)
			}
			// SQLite cannot execute multiple ;-separated statements in one Exec call.
			// Split and execute each statement individually.
			for i, stmt := range splitStatements(sql) {
				if err := r.database.Exec(ctx, stmt); err != nil {
					return applied, fmt.Errorf("migrate: failed to apply migration %s (version %d, statement %d): %w", m.Filename, m.Version, i+1, err)
				}
			}
		} else {
			if err := r.database.Exec(ctx, sql); err != nil {
				return applied, fmt.Errorf("migrate: failed to apply migration %s (version %d): %w", m.Filename, m.Version, err)
			}
		}

		// Record the migration.
		// Checksum is a SHA-256 content hash of the embedded SQL so that
		// recorded-but-different-content drift (BUG-012) is detectable on the
		// next GetState. Legacy installs stored a length-based hex; GetState
		// normalizes those so they don't read as false drift.
		checksum := migrationChecksum(m.SQL)
		if err := r.database.Exec(ctx,
			`INSERT INTO schema_versions (version, name, applied_at, checksum) VALUES ($1, $2, $3, $4)`,
			m.Version, m.Name, time.Now().Format(time.RFC3339), checksum,
		); err != nil {
			return applied, fmt.Errorf("migrate: failed to record migration %s: %w", m.Filename, err)
		}

		applied = append(applied, m.Filename)
	}

	// Resume paused sessions that were active before migration
	if len(pausedSessions) > 0 {
		if resumeErr := r.resumePausedSessions(ctx, pausedSessions); resumeErr != nil {
			// Non-fatal: log — sessions may remain paused but migration is done
		}
	}

	return applied, nil
}

// ============================================================================
// Down (Rollback)
// ============================================================================

// Down rolls back the most recently applied migration.
// Note: migration files must contain rollback SQL for this to be effective.
// Currently, this only removes the tracking record since we don't parse rollback sections.
func (r *Runner) Down(ctx context.Context) (string, error) {
	state, err := r.GetState(ctx)
	if err != nil {
		return "", err
	}

	if state.CurrentVersion == 0 {
		return "", fmt.Errorf("migrate: no migrations to roll back")
	}

	lastVersion := state.CurrentVersion

	// Remove the tracking record
	if err := r.database.Exec(ctx,
		`DELETE FROM schema_versions WHERE version = $1`, lastVersion,
	); err != nil {
		return "", fmt.Errorf("migrate: failed to rollback version %d: %w", lastVersion, err)
	}

	return fmt.Sprintf("Rolled back migration version %d", lastVersion), nil
}

// ============================================================================
// Version
// ============================================================================

// Version returns the current schema version number.
func (r *Runner) Version(ctx context.Context) (int, error) {
	state, err := r.GetState(ctx)
	if err != nil {
		return 0, err
	}
	return state.CurrentVersion, nil
}

// ============================================================================
// Auto-Migrate (for startup)
// ============================================================================

// AutoMigrate is the startup migration flow:
// 1. Bootstrap the schema_versions table
// 2. Load embedded migrations
// 3. Check for drift
// 4. Apply pending migrations
//
// Returns true if migrations were applied, false if already current.
func (r *Runner) AutoMigrate(ctx context.Context) (bool, error) {
	if err := r.Bootstrap(ctx); err != nil {
		return false, fmt.Errorf("migrate: bootstrap failed: %w", err)
	}

	if err := r.LoadMigrations(); err != nil {
		return false, fmt.Errorf("migrate: load failed: %w", err)
	}

	applied, err := r.Up(ctx)
	if err != nil {
		return false, err
	}

	// Repair: migration 013 had a bug (filterForSQLite stripped valid SQLite
	// ALTER TABLE ADD COLUMN). DBs initialized before the fix have migration 013
	// recorded but trust_level missing. Detect and repair silently.
	if err := r.repairTrustLevel(ctx); err != nil {
		// Non-fatal: the repair is best-effort. The harness will catch the
		// missing column in its planning phase and report a clear error.
	}

	// Repair: migration 017's append-only triggers were silently stripped by
	// filterForSQLite (multi-line CREATE TRIGGER header without " BEGIN " on
	// the first line). SQLite DBs initialized before the fix have v17 recorded
	// as applied but no triggers in sqlite_master (DOGFOOD-001).
	if err := r.repairAppendOnlyTriggers(ctx); err != nil {
		// Non-fatal: best-effort, same policy as repairTrustLevel.
	}

	// Repair: migration 008 was recorded as applied on some installs without
	// its DDL landing (recorded-but-not-executed — same family as 013/017),
	// leaving approval_requests / hitl_configuration / notification_log
	// missing while schema_versions says v8 applied. Every startup then warns
	// "failed to initialize HITL defaults: relation hitl_configuration does
	// not exist" (BUG-010, dexdat sidecar 2026-08-07).
	if err := r.repairHitlConfiguration(ctx); err != nil {
		// Non-fatal: best-effort. HITL flows surface the missing table with a
		// clear error at first use.
	}

	// Repair: migration 003 drifted on the dexdat sidecar PG —
	// agent_circuit_breakers was created without breaker_type (and possibly
	// threshold/current_count) while schema_versions records v3 as applied.
	// The harness upserts via ON CONFLICT (session_id, breaker_type), which
	// fails without the column + unique constraint. Reconcile additively
	// (BUG-012, recorded-but-different-content).
	if err := r.repairCircuitBreakers(ctx); err != nil {
		// Non-fatal: best-effort. The harness will surface the missing column
		// as a query error on first circuit-breaker write.
	}

	// One-time transition: pre-BUG-012 installs recorded length-based hex
	// checksums (e.g. "2e7" for migration 003). Rewrite those rows to the
	// SHA-256 content hash of the current embedded SQL so content-hash drift
	// detection engages for them on the next GetState. Rows whose stored
	// checksum matches NEITHER the legacy form NOR the content hash are
	// genuine recorded-but-different-content drift and are left untouched
	// (GetState flags them).
	if err := r.normalizeLegacyChecksums(ctx); err != nil {
		// Non-fatal: best-effort. GetState still accepts legacy checksums, so
		// an un-normalized install simply keeps the old scheme until the
		// next successful startup.
	}

	return len(applied) > 0, nil
}

// normalizeLegacyChecksums rewrites schema_versions rows that still carry the
// pre-BUG-012 length-based hex checksum to the SHA-256 content hash of the
// current embedded migration SQL. This is the one-time transition that lets
// content-hash drift detection (recorded-but-different-content) engage for
// existing installs — including the live dexdat sidecar PG, whose v3 row
// stores "2e7" (hex of len(003 SQL) = 743).
func (r *Runner) normalizeLegacyChecksums(ctx context.Context) error {
	rows, err := r.database.Query(ctx, `SELECT version, checksum FROM schema_versions`)
	if err != nil {
		// schema_versions unavailable (pre-bootstrap) — nothing to normalize.
		return nil
	}

	embeddedByVersion := make(map[int]Migration, len(r.migrations))
	for _, m := range r.migrations {
		embeddedByVersion[m.Version] = m
	}

	for _, row := range rows {
		v, ok := row["version"]
		if !ok {
			continue
		}
		version := toInt(v)
		stored, _ := row["checksum"].(string)
		if stored == "" {
			continue
		}
		m, found := embeddedByVersion[version]
		if !found {
			continue // backend-filtered or ghost migration — leave alone
		}
		contentHash := migrationChecksum(m.SQL)
		if strings.EqualFold(stored, contentHash) {
			continue // already the content hash — nothing to do
		}
		if !strings.EqualFold(stored, legacyChecksum(m.SQL)) {
			// Matches neither the legacy length form nor the content hash —
			// genuine drift. Leave it for GetState to flag.
			continue
		}
		// Legacy length-based checksum → rewrite to the content hash.
		if err := r.database.Exec(ctx,
			`UPDATE schema_versions SET checksum = $1 WHERE version = $2`,
			contentHash, version); err != nil {
			return fmt.Errorf("migrate: normalize legacy checksum for version %d: %w", version, err)
		}
		slog.Info("migrate: normalized legacy length-based checksum to content hash",
			"version", version, "checksum", contentHash)
	}
	return nil
}

// repairTrustLevel adds sessions.trust_level if missing (migration 013 filterForSQLite bug).
// Safe to call on any DB: uses PRAGMA table_info (SQLite) or
// information_schema.columns (Postgres) before ALTER TABLE.
func (r *Runner) repairTrustLevel(ctx context.Context) error {
	var cols []db.Row
	var err error

	backend := db.DetectBackendFromDB(r.database)
	if backend == db.BackendPostgres {
		cols, err = r.database.Query(ctx,
			`SELECT column_name AS name FROM information_schema.columns WHERE table_name = 'sessions'`)
	} else {
		cols, err = r.database.Query(ctx, "PRAGMA table_info(sessions)")
	}
	if err != nil {
		// Table might not exist yet (first-run, not yet auto-migrated)
		return nil
	}
	for _, row := range cols {
		if name, ok := row["name"].(string); ok && name == "trust_level" {
			return nil // Already present — nothing to repair
		}
	}
	// Column missing — add it (migration 013 recorded but never executed)
	return r.database.Exec(ctx, "ALTER TABLE sessions ADD COLUMN trust_level TEXT NOT NULL DEFAULT 'high' CHECK (trust_level IN ('low', 'medium', 'high'))")
}

// repairAppendOnlyTriggers ensures the SQLite append-only triggers on
// memory_events (migration 017) actually exist. filterForSQLite used to strip
// CREATE TRIGGER statements whose header spans multiple lines, so SQLite DBs
// initialized before the fix have migration 017 recorded in schema_versions
// but zero triggers in sqlite_master — leaving memory_events mutable despite
// the append-only invariant (DOGFOOD-001). Re-applying 017 is idempotent
// (CREATE TRIGGER IF NOT EXISTS). Postgres is unaffected: append-only
// enforcement there lives in migration 018.
func (r *Runner) repairAppendOnlyTriggers(ctx context.Context) error {
	if db.DetectBackendFromDB(r.database) != db.BackendSQLite {
		return nil
	}

	rows, err := r.database.Query(ctx,
		`SELECT name, type FROM sqlite_master WHERE type IN ('table', 'trigger')`)
	if err != nil {
		// sqlite_master unavailable — best-effort repair, don't fail startup
		return nil
	}
	haveMemoryEvents := false
	triggers := map[string]bool{}
	for _, row := range rows {
		name, _ := row["name"].(string)
		typ, _ := row["type"].(string)
		if typ == "table" && name == "memory_events" {
			haveMemoryEvents = true
		}
		if typ == "trigger" {
			triggers[name] = true
		}
	}
	if !haveMemoryEvents {
		// First-run (memory_events not created yet) — the normal migration
		// path will create the table and the triggers together.
		return nil
	}
	if triggers["trg_memory_events_append_only_update"] && triggers["trg_memory_events_append_only_delete"] {
		return nil // Both present — nothing to repair
	}

	// Re-apply migration 017 (idempotent: CREATE TRIGGER IF NOT EXISTS).
	content, err := embeddedMigrations.ReadFile("migrations/017_append_only_memory_events_sqlite_triggers.sql")
	if err != nil {
		return fmt.Errorf("migrate: read 017 repair SQL: %w", err)
	}
	for i, stmt := range splitStatements(string(content)) {
		if err := r.database.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: repair append-only triggers (statement %d): %w", i+1, err)
		}
	}
	return nil
}

// repairHitlConfiguration ensures the HITL tables (migration 008) actually
// exist. Some installs have version 8 recorded in schema_versions while the
// DDL never landed (recorded-but-not-executed — same family as 013/017),
// leaving approval_requests / hitl_configuration / notification_log missing
// and every startup warning "failed to initialize HITL defaults: relation
// hitl_configuration does not exist" (BUG-010, dexdat sidecar 2026-08-07).
// Re-applying 008 is idempotent (CREATE TABLE IF NOT EXISTS + guarded
// default-row insert).
func (r *Runner) repairHitlConfiguration(ctx context.Context) error {
	backend := db.DetectBackendFromDB(r.database)

	var rows []db.Row
	var err error
	if backend == db.BackendPostgres {
		rows, err = r.database.Query(ctx,
			`SELECT table_name AS name FROM information_schema.tables WHERE table_name = 'hitl_configuration'`)
	} else {
		rows, err = r.database.Query(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'hitl_configuration'`)
	}
	if err != nil {
		// Table metadata unavailable (pre-bootstrap) — best-effort repair, don't fail startup.
		return nil
	}
	if len(rows) > 0 {
		return nil // Already present — nothing to repair
	}

	// Table missing — re-apply migration 008 (idempotent), through the same
	// backend filters the normal Up() path applies (filterForSQLite strips
	// PG-only constructs like gen_random_uuid()/BIGSERIAL on SQLite).
	content, err := embeddedMigrations.ReadFile("migrations/008_hitl_tables.sql")
	if err != nil {
		return fmt.Errorf("migrate: read 008 repair SQL: %w", err)
	}
	if backend == db.BackendSQLite {
		content = []byte(filterForSQLite(string(content)))
		for i, stmt := range splitStatements(string(content)) {
			if err := r.database.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("migrate: repair hitl (statement %d): %w", i+1, err)
			}
		}
		return nil
	}
	if err := r.database.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("migrate: repair hitl: %w", err)
	}
	return nil
}

// repairCircuitBreakers reconciles a drifted agent_circuit_breakers table
// (BUG-012). Migration 003 was recorded as applied but its content drifted
// on the dexdat sidecar PG — the table exists but lacks breaker_type (and
// possibly threshold/current_count), so the harness upsert
// `ON CONFLICT (session_id, breaker_type)` fails. This repair:
//  1. If the table is missing entirely but sessions exists, re-apply 003
//     (idempotent: CREATE TABLE IF NOT EXISTS).
//  2. If the table exists, add any missing columns among breaker_type /
//     threshold / current_count additively (never drop/recreate — RLS and
//     existing data are preserved).
//  3. Ensure a UNIQUE constraint on (session_id, breaker_type) so the
//     harness ON CONFLICT clause works.
//
// Safe to call on any DB (SQLite or PG) at any migration state. All DDL is
// additive and guarded. Non-fatal on failure.
func (r *Runner) repairCircuitBreakers(ctx context.Context) error {
	backend := db.DetectBackendFromDB(r.database)

	// --- 1. Does agent_circuit_breakers exist? ---
	var rows []db.Row
	var err error
	if backend == db.BackendPostgres {
		rows, err = r.database.Query(ctx,
			`SELECT table_name AS name FROM information_schema.tables WHERE table_name = 'agent_circuit_breakers'`)
	} else {
		rows, err = r.database.Query(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'agent_circuit_breakers'`)
	}
	if err != nil {
		// Table metadata unavailable (pre-bootstrap) — best-effort, don't fail startup.
		return nil
	}
	breakersExists := len(rows) > 0

	if !breakersExists {
		// If sessions doesn't exist either, this is a fresh DB pre-migration —
		// the normal Up() path will create everything. Bail out silently.
		if !r.tableExists(ctx, "sessions") {
			return nil
		}
		// sessions exists but agent_circuit_breakers is missing while v3 is
		// recorded as applied → re-apply 003 idempotently.
		content, rerr := embeddedMigrations.ReadFile("migrations/003_circuit_breakers.sql")
		if rerr != nil {
			return fmt.Errorf("migrate: read 003 repair SQL: %w", rerr)
		}
		if backend == db.BackendSQLite {
			filtered := filterForSQLite(string(content))
			for i, stmt := range splitStatements(filtered) {
				if eerr := r.database.Exec(ctx, stmt); eerr != nil {
					return fmt.Errorf("migrate: repair circuit breakers (statement %d): %w", i+1, eerr)
				}
			}
			return nil
		}
		if eerr := r.database.Exec(ctx, string(content)); eerr != nil {
			return fmt.Errorf("migrate: repair circuit breakers: %w", eerr)
		}
		return nil
	}

	// --- 2. Table exists — reconcile columns additively. ---
	cols, cerr := r.columnNames(ctx, "agent_circuit_breakers")
	if cerr != nil {
		// Cannot introspect — best-effort, don't fail startup.
		return nil
	}
	have := make(map[string]bool, len(cols))
	for _, c := range cols {
		have[strings.ToLower(c)] = true
	}

	// Each missing column is added with a DEFAULT so existing rows stay valid.
	type colDef struct {
		name string
		pg   string // full ADD COLUMN clause for Postgres (CHECK allowed)
		sql  string // full ADD COLUMN clause for SQLite (CHECK omitted if needed)
	}
	defs := []colDef{
		{
			name: "breaker_type",
			pg:   `ALTER TABLE agent_circuit_breakers ADD COLUMN IF NOT EXISTS breaker_type TEXT NOT NULL DEFAULT 'consecutive_errors' CHECK (breaker_type IN ('consecutive_errors', 'iterations', 'budget'))`,
			sql:  `ALTER TABLE agent_circuit_breakers ADD COLUMN breaker_type TEXT NOT NULL DEFAULT 'consecutive_errors' CHECK (breaker_type IN ('consecutive_errors', 'iterations', 'budget'))`,
		},
		{
			name: "threshold",
			pg:   `ALTER TABLE agent_circuit_breakers ADD COLUMN IF NOT EXISTS threshold INTEGER NOT NULL DEFAULT 5`,
			sql:  `ALTER TABLE agent_circuit_breakers ADD COLUMN threshold INTEGER NOT NULL DEFAULT 5`,
		},
		{
			name: "current_count",
			pg:   `ALTER TABLE agent_circuit_breakers ADD COLUMN IF NOT EXISTS current_count INTEGER NOT NULL DEFAULT 0`,
			sql:  `ALTER TABLE agent_circuit_breakers ADD COLUMN current_count INTEGER NOT NULL DEFAULT 0`,
		},
	}
	for _, d := range defs {
		if have[d.name] {
			continue
		}
		stmt := d.pg
		if backend == db.BackendSQLite {
			stmt = d.sql
		}
		if eerr := r.database.Exec(ctx, stmt); eerr != nil {
			if backend == db.BackendSQLite && d.name == "breaker_type" {
				// Some SQLite builds reject a CHECK constraint on ADD COLUMN.
				// Retry without the CHECK (keep the DEFAULT so rows are valid).
				fallback := `ALTER TABLE agent_circuit_breakers ADD COLUMN breaker_type TEXT NOT NULL DEFAULT 'consecutive_errors'`
				if ferr := r.database.Exec(ctx, fallback); ferr != nil {
					return fmt.Errorf("migrate: repair circuit breakers (add breaker_type): %w", ferr)
				}
				continue
			}
			return fmt.Errorf("migrate: repair circuit breakers (add %s): %w", d.name, eerr)
		}
	}

	// The old dexdat shape declares error_type TEXT NOT NULL with NO default,
	// and the harness upsert (internal/harness/circuit.go) never supplies it —
	// so even after the 003 columns land, the consumer INSERT would fail with
	// a NOT NULL violation. Give it the canonical default (PG only: SQLite
	// cannot ALTER COLUMN SET DEFAULT, and SQLite installs never carry the old
	// shape — they get the canonical 003 DDL directly). Idempotent: setting
	// the same default twice is a no-op.
	if backend == db.BackendPostgres && have["error_type"] {
		if eerr := r.database.Exec(ctx,
			`ALTER TABLE agent_circuit_breakers ALTER COLUMN error_type SET DEFAULT 'consecutive_errors'`); eerr != nil {
			return fmt.Errorf("migrate: repair circuit breakers (error_type default): %w", eerr)
		}
	}

	// --- 3. Ensure unique (session_id, breaker_type) for ON CONFLICT. ---
	// The harness upserts with `ON CONFLICT (session_id, breaker_type) DO UPDATE`.
	// CREATE UNIQUE INDEX IF NOT EXISTS is supported by both PG and SQLite and
	// is additive (does not disturb an existing PK or RLS policies).
	//
	// Skip when a PRIMARY KEY on (session_id, breaker_type) already exists
	// (the canonical 003 shape) — the PK provides the ON CONFLICT arbiter, and
	// adding a redundant unique index would duplicate the constraint. The
	// drifted dexdat shape has a PK on id only, so the index is created there.
	if r.hasPrimaryKeyOn(ctx, "agent_circuit_breakers", "session_id", "breaker_type") {
		return nil
	}
	uniqueIdx := `CREATE UNIQUE INDEX IF NOT EXISTS agent_circuit_breakers_session_breaker_key ON agent_circuit_breakers (session_id, breaker_type)`
	if eerr := r.database.Exec(ctx, uniqueIdx); eerr != nil {
		// A PK on (session_id, breaker_type) already satisfies the uniqueness
		// contract; a duplicate-index error is non-fatal. Only fail on
		// unrelated errors.
		if !isDuplicateIndexErr(eerr) {
			return fmt.Errorf("migrate: repair circuit breakers (unique index): %w", eerr)
		}
	}
	return nil
}

// hasPrimaryKeyOn reports whether the named table has a PRIMARY KEY whose
// column set is exactly the given columns (order-insensitive). Used by
// repairCircuitBreakers to avoid creating a redundant unique index when the
// canonical (session_id, breaker_type) PK already provides the ON CONFLICT
// arbiter (BUG-012). Postgres introspects pg_index; SQLite parses the
// table's CREATE statement from sqlite_master.
func (r *Runner) hasPrimaryKeyOn(ctx context.Context, table string, cols ...string) bool {
	want := make(map[string]bool, len(cols))
	for _, c := range cols {
		want[strings.ToLower(c)] = true
	}

	if db.DetectBackendFromDB(r.database) == db.BackendPostgres {
		rows, err := r.database.Query(ctx,
			`SELECT a.attname
			   FROM pg_index i
			   JOIN pg_class t ON t.oid = i.indrelid
			   JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(i.indkey)
			  WHERE t.relname = $1 AND i.indisprimary
			  ORDER BY a.attnum`, table)
		if err != nil {
			return false
		}
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			if n, ok := row["attname"].(string); ok {
				got[strings.ToLower(n)] = true
			}
		}
		if len(got) != len(want) {
			return false
		}
		for c := range want {
			if !got[c] {
				return false
			}
		}
		return true
	}

	// SQLite: the composite PK appears in the table's CREATE statement as
	// "PRIMARY KEY (session_id, breaker_type)" (column order as declared).
	rows, err := r.database.Query(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = $1`, table)
	if err != nil || len(rows) == 0 {
		return false
	}
	createSQL, _ := rows[0]["sql"].(string)
	lower := strings.ToLower(createSQL)
	idx := strings.Index(lower, "primary key (")
	if idx < 0 {
		return false
	}
	end := strings.Index(lower[idx:], ")")
	if end < 0 {
		return false
	}
	got := make(map[string]bool)
	for _, c := range strings.Split(lower[idx+len("primary key ("):idx+end], ",") {
		got[strings.TrimSpace(c)] = true
	}
	if len(got) != len(want) {
		return false
	}
	for c := range want {
		if !got[c] {
			return false
		}
	}
	return true
}

// tableExists reports whether the named table is present in the database.
func (r *Runner) tableExists(ctx context.Context, name string) bool {
	backend := db.DetectBackendFromDB(r.database)
	var rows []db.Row
	var err error
	if backend == db.BackendPostgres {
		rows, err = r.database.Query(ctx,
			`SELECT table_name AS name FROM information_schema.tables WHERE table_name = $1`, name)
	} else {
		rows, err = r.database.Query(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = $1`, name)
	}
	if err != nil {
		return false
	}
	return len(rows) > 0
}

// columnNames returns the column names of the given table.
func (r *Runner) columnNames(ctx context.Context, table string) ([]string, error) {
	backend := db.DetectBackendFromDB(r.database)
	var rows []db.Row
	var err error
	if backend == db.BackendPostgres {
		rows, err = r.database.Query(ctx,
			`SELECT column_name AS name FROM information_schema.columns WHERE table_name = $1`, table)
	} else {
		rows, err = r.database.Query(ctx, "PRAGMA table_info("+table+")")
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if n, ok := row["name"].(string); ok {
			names = append(names, n)
		}
	}
	return names, nil
}

// isDuplicateIndexErr reports whether an error is a benign "index already
// exists" / duplicate constraint error that the repair can ignore.
func isDuplicateIndexErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "relation") && strings.Contains(msg, "exists")
}

// ============================================================================
// Session Pause/Resume (Drift Handling — WI-022)
// ============================================================================

// pauseActiveSessions pauses all active (non-terminal, non-paused) agent sessions
// before migration to prevent data races with schema changes.
// Returns a list of session IDs that were paused.
func (r *Runner) pauseActiveSessions(ctx context.Context) ([]string, error) {
	// Find all active sessions (idle, thinking, planning, tool_exec, executing, waiting_sub)
	rows, err := r.database.Query(ctx,
		`SELECT id FROM sessions WHERE status IN ('idle','thinking','planning','tool_exec','executing','waiting_sub')`)
	if err != nil {
		return nil, fmt.Errorf("migrate: failed to query active sessions: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, ok := row["id"]; ok {
			ids = append(ids, toString(id))
		}
	}

	// Pause them all
	for _, id := range ids {
		_ = r.database.Exec(ctx,
			`UPDATE sessions SET status = 'paused', heartbeat_at = datetime('now') WHERE id = $1`, id)
	}

	return ids, nil
}

// resumePausedSessions resumes the sessions that were paused before migration.
// Sessions are returned to the 'idle' state where the harness loop can reclaim them.
func (r *Runner) resumePausedSessions(ctx context.Context, sessionIDs []string) error {
	for _, id := range sessionIDs {
		_ = r.database.Exec(ctx,
			`UPDATE sessions SET status = 'idle', heartbeat_at = datetime('now') WHERE id = $1 AND status = 'paused'`, id)
	}
	return nil
}

// ============================================================================
// Drift Check
// ============================================================================

// CheckDrift returns true if schema drift is detected and details about what drifted.
// Pausing agents on drift is handled at the harness/caller level.
func (r *Runner) CheckDrift(ctx context.Context) (bool, string, error) {
	state, err := r.GetState(ctx)
	if err != nil {
		return false, "", err
	}
	return state.DriftDetected, state.DriftDetails, nil
}

// ============================================================================
// Helpers
// ============================================================================

// backendMigrationExists checks whether any embedded SQL migration file
// starts with the given version number. This is used by drift detection:
// an applied version that has no matching loaded migration (because
// LoadMigrations filtered it for the current backend) should NOT be
// treated as drift — it was applied on a different backend.
func backendMigrationExists(version int) bool {
	prefix := fmt.Sprintf("%03d_", version)
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			return true
		}
	}
	return false
}

// migrationChecksum returns the SHA-256 content hash of a migration's SQL.
// This is the current checksum scheme: two migrations with the same length
// but different content produce different checksums, so recorded-but-
// different-content drift (BUG-012) is detectable.
func migrationChecksum(sql string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(sql)))
}

// legacyChecksum returns the pre-BUG-012 length-based hex checksum that
// older installs recorded in schema_versions. GetState accepts it as an
// alternative to migrationChecksum so existing installs don't read as
// false drift after the content-hash scheme is introduced.
func legacyChecksum(sql string) string {
	return fmt.Sprintf("%x", len(sql))
}

// splitStatements splits a SQL string on semicolons for SQLite execution.
// SQLite drivers cannot execute multiple statements in a single Exec call.
// Each statement boundary is a line ending with ';'.
func splitStatements(sql string) []string {
	lines := strings.Split(sql, "\n")
	var current strings.Builder
	var statements []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty and comment-only lines
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)

		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" && stmt != ";" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

// triggerIsSQLiteStyle reports whether a CREATE TRIGGER statement is
// SQLite-native (body uses BEGIN...END) as opposed to PostgreSQL-style
// (EXECUTE FUNCTION/PROCEDURE or dollar-quoted bodies). lines must start at
// the statement's first line. The scan stops at the statement's terminating
// semicolon (paren depth 0); if neither marker is found the trigger is
// treated as PostgreSQL-style, preserving the previous skip behavior.
func triggerIsSQLiteStyle(lines []string) bool {
	depth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.Contains(upper, "EXECUTE FUNCTION") || strings.Contains(upper, "EXECUTE PROCEDURE") {
			return false
		}
		for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
			if strings.Contains(trimmed, t) {
				return false
			}
		}
		if hasBeginKeyword(upper) {
			return true
		}
		for _, ch := range trimmed {
			if ch == '(' {
				depth++
			}
			if ch == ')' {
				depth--
			}
		}
		if strings.HasSuffix(trimmed, ";") && depth <= 0 {
			return false // statement ended with no BEGIN keyword → PG-style
		}
	}
	return false
}

// hasBeginKeyword reports whether an uppercased SQL line contains the SQLite
// BEGIN keyword as a standalone word (not inside a longer identifier).
func hasBeginKeyword(upper string) bool {
	return upper == "BEGIN" ||
		strings.HasPrefix(upper, "BEGIN ") ||
		strings.HasPrefix(upper, "BEGIN;") ||
		strings.HasSuffix(upper, " BEGIN") ||
		strings.Contains(upper, " BEGIN ") ||
		strings.Contains(upper, " BEGIN;")
}

// filterForSQLite transforms PostgreSQL migration SQL for SQLite compatibility.
//
// Two phases:
//  1. Strip: remove PG-only constructs (extensions, functions, triggers, policies,
//     role grants, pg_cron, views, ALTER TABLE ADD CONSTRAINT).
//  2. Translate: rewrite types (UUID→TEXT, TIMESTAMPTZ→TEXT, JSONB→TEXT, etc.),
//     functions (gen_random_uuid(), now()), and remove type casts.
func filterForSQLite(rawSQL string) string {
	lines := strings.Split(rawSQL, "\n")
	out := make([]string, 0, len(lines))

	skipMode := 0                           // 0=none, 1=function, 2=trigger, 3=policy, 4=cron, 5=rls, 6=extension, 7=doblock, 8=alterFk, 9=view, 10=index, 11=alterTable, 12=createIndex, 13=comment
	skipUntilMarker := ""                   // closing $$ tag
	skipDepth := 0                          // paren depth tracker
	skipHasUsing := false                   // tracks whether CREATE INDEX block contains USING (PG-specific)
	createIndexBuf := make([]string, 0, 20) // buffer for multi-line CREATE INDEX statements
	alterTableBuf := make([]string, 0, 10)  // buffer for multi-line ALTER TABLE statements
	alterTableHasPG := false                // true if buffered ALTER TABLE has PG-only keywords

	const (
		mNone        = 0
		mFunction    = 1
		mTrigger     = 2
		mPolicy      = 3
		mCron        = 4
		mRLS         = 5
		mExtension   = 6
		mDOBlock     = 7
		mAlterFk     = 8
		mView        = 9
		mIndex       = 10
		mAlterTable  = 11 // multi-line ALTER TABLE: buffer, decide at semicolon
		mCreateIndex = 12
		mComment     = 13
	)

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)

		if skipMode != mNone {
			switch skipMode {
			case mFunction, mDOBlock:
				if skipUntilMarker != "" {
					if strings.Contains(trimmed, skipUntilMarker) && strings.Contains(trimmed, ";") {
						skipMode = mNone
						skipUntilMarker = ""
					}
				} else {
					for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
						if strings.Contains(trimmed, t) {
							skipUntilMarker = t
							break
						}
					}
				}
			case mTrigger:
				if skipUntilMarker != "" {
					if strings.Contains(trimmed, skipUntilMarker) && strings.Contains(trimmed, ";") {
						skipMode = mNone
						skipUntilMarker = ""
					}
				} else {
					for _, ch := range trimmed {
						if ch == '(' {
							skipDepth++
						}
						if ch == ')' {
							skipDepth--
						}
					}
					if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 {
						skipMode = mNone
						skipDepth = 0
					}
					for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
						if strings.Contains(trimmed, t) {
							skipUntilMarker = t
							break
						}
					}
				}
			case mPolicy, mIndex:
				for _, ch := range trimmed {
					if ch == '(' {
						skipDepth++
					}
					if ch == ')' {
						skipDepth--
					}
				}
				if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 {
					skipMode = mNone
					skipDepth = 0
				}
			case mCron:
				if skipUntilMarker != "" {
					if strings.Contains(trimmed, skipUntilMarker) &&
						(strings.Contains(trimmed, ");") || strings.Contains(trimmed, ";")) {
						skipMode = mNone
						skipUntilMarker = ""
					}
				} else {
					for _, ch := range trimmed {
						if ch == '(' {
							skipDepth++
						}
						if ch == ')' {
							skipDepth--
						}
					}
					if strings.Contains(trimmed, ");") && skipDepth <= 1 {
						skipMode = mNone
						skipDepth = 0
					}
					for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
						if strings.Contains(trimmed, t) {
							skipUntilMarker = t
							break
						}
					}
				}
			case mRLS, mExtension, mView, mComment:
			case mAlterFk:
				for _, ch := range trimmed {
					if ch == '(' {
						skipDepth++
					}
					if ch == ')' {
						skipDepth--
					}
				}
				if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 {
					skipMode = mNone
					skipDepth = 0
				}
			case mAlterTable:
				// Buffer multi-line ALTER TABLE; check for PG-only keywords.
				// SQLite supports ADD COLUMN, DROP COLUMN, RENAME TO, RENAME COLUMN.
				// Strip PG-only: ADD CONSTRAINT, ENABLE/DISABLE RLS, OWNER TO, SET SCHEMA, etc.
				alterTableBuf = append(alterTableBuf, line)
				upperTrimmed := strings.ToUpper(trimmed)
				if strings.Contains(upperTrimmed, "ADD CONSTRAINT") ||
					strings.Contains(upperTrimmed, "ENABLE ROW LEVEL SECURITY") ||
					strings.Contains(upperTrimmed, "DISABLE ROW LEVEL SECURITY") ||
					strings.Contains(upperTrimmed, "FORCE ROW LEVEL SECURITY") ||
					strings.Contains(upperTrimmed, " NO FORCE ROW LEVEL SECURITY") ||
					strings.Contains(upperTrimmed, "BYPASSRLS") ||
					strings.Contains(upperTrimmed, "OWNER TO") ||
					strings.Contains(upperTrimmed, "SET SCHEMA") ||
					strings.Contains(upperTrimmed, "SET TABLESPACE") ||
					strings.Contains(upperTrimmed, "ALTER COLUMN") ||
					strings.Contains(upperTrimmed, "VALIDATE CONSTRAINT") ||
					strings.Contains(upperTrimmed, "ENABLE TRIGGER") ||
					strings.Contains(upperTrimmed, "DISABLE TRIGGER") ||
					strings.Contains(upperTrimmed, "ENABLE REPLICA") ||
					strings.Contains(upperTrimmed, "ENABLE ALWAYS") ||
					strings.Contains(upperTrimmed, "CLUSTER ON") ||
					strings.Contains(upperTrimmed, "SET WITHOUT CLUSTER") ||
					strings.Contains(upperTrimmed, "INHERIT") ||
					strings.Contains(upperTrimmed, "NOT OF") {
					alterTableHasPG = true
				}
				if strings.HasSuffix(trimmed, ";") {
					skipMode = mNone
					if !alterTableHasPG {
						// Valid SQLite ALTER TABLE — emit buffered lines
						out = append(out, alterTableBuf...)
					}
					alterTableBuf = alterTableBuf[:0]
					alterTableHasPG = false
				}
			case mCreateIndex:
				// Buffer lines; check for USING; flush or discard at semicolon
				createIndexBuf = append(createIndexBuf, line)
				if strings.Contains(strings.ToUpper(trimmed), "USING") {
					skipHasUsing = true
				}
				if strings.HasSuffix(trimmed, ";") {
					skipMode = mNone
					if !skipHasUsing {
						// Valid SQLite index — emit buffered lines
						out = append(out, createIndexBuf...)
					}
					// Reset buffer and flag
					createIndexBuf = createIndexBuf[:0]
					skipHasUsing = false
				}
			}
			continue
		}

		// --- Not in skip mode: detect if this line starts a skip block ---

		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			out = append(out, line)
			continue
		}

		// Strip PG-only blocks:
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "CREATE EXTENSION"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mExtension
			continue
		case strings.HasPrefix(upper, "CREATE ") && strings.Contains(upper, "FUNCTION"):
			skipMode = mFunction
			skipUntilMarker = ""
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) {
					skipUntilMarker = t
					break
				}
			}
			continue
		case strings.HasPrefix(upper, "DO ") && strings.Contains(upper, "$$"):
			skipMode = mDOBlock
			skipUntilMarker = ""
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) {
					skipUntilMarker = t
					break
				}
			}
			continue
		case strings.HasPrefix(upper, "CREATE TRIGGER"):
			// SQLite-native triggers use BEGIN...END (keep these).
			// PostgreSQL triggers use EXECUTE FUNCTION / $$ bodies (skip these).
			// The CREATE TRIGGER header may span multiple lines, so " BEGIN "
			// may not appear on the first line — scan ahead to the end of the
			// statement before deciding (DOGFOOD-001).
			if triggerIsSQLiteStyle(lines[lineIdx:]) {
				// SQLite-style — keep the line as-is (don't skip)
				// The BEGIN...END body has its own semicolons, but that's fine
				// because splitStatements handles it on the full migration content later.
				// We just need to NOT skip it here.
			} else {
				skipMode = mTrigger
				skipDepth = 0
				skipUntilMarker = ""
				for _, ch := range trimmed {
					if ch == '(' {
						skipDepth++
					}
					if ch == ')' {
						skipDepth--
					}
				}
				if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 {
					continue
				}
				continue
			}
		case strings.HasPrefix(upper, "DROP TRIGGER"):
			continue
		case strings.HasPrefix(upper, "CREATE POLICY"):
			skipMode = mPolicy
			skipDepth = 0
			for _, ch := range trimmed {
				if ch == '(' {
					skipDepth++
				}
				if ch == ')' {
					skipDepth--
				}
			}
			if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 {
				continue
			}
			continue
		case strings.HasPrefix(upper, "DROP POLICY"):
			continue
		case strings.HasPrefix(upper, "CREATE OR REPLACE VIEW") || strings.HasPrefix(upper, "CREATE VIEW"):
			skipMode = mView
			continue
		case strings.HasPrefix(upper, "REVOKE ") || strings.HasPrefix(upper, "GRANT "):
			continue
		case strings.HasPrefix(upper, "COMMENT ON"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mComment
			continue
		case strings.HasPrefix(upper, "ALTER ROLE"):
			continue
		case strings.HasPrefix(upper, "SELECT CRON."):
			skipMode = mCron
			skipDepth = 0
			skipUntilMarker = ""
			for _, ch := range trimmed {
				if ch == '(' {
					skipDepth++
				}
				if ch == ')' {
					skipDepth--
				}
			}
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) {
					skipUntilMarker = t
					break
				}
			}
			continue
		case strings.HasPrefix(upper, "ALTER TABLE") &&
			strings.Contains(upper, "ROW LEVEL SECURITY"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mRLS
			continue
		case strings.HasPrefix(upper, "ALTER TABLE") &&
			strings.Contains(upper, "ADD CONSTRAINT") &&
			strings.Contains(upper, "FOREIGN KEY"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mAlterFk
			continue
		// Multi-line ALTER TABLE statements (ADD CONSTRAINT, ENABLE RLS, etc.)
		// Buffer and decide at semicolon whether to keep (SQLite-compatible) or strip (PG-only).
		case strings.HasPrefix(upper, "ALTER TABLE"):
			if !strings.HasSuffix(trimmed, ";") {
				alterTableBuf = append(alterTableBuf[:0], line) // start buffer with first line
				alterTableHasPG = false
				skipMode = mAlterTable
				continue
			}
			// Single-line ALTER TABLE: strip if it's PG-only
			if strings.Contains(upper, "ADD CONSTRAINT") ||
				strings.Contains(upper, "ENABLE ROW LEVEL SECURITY") ||
				strings.Contains(upper, "BYPASSRLS") {
				continue
			}
			out = append(out, line)
			continue
		// Strip ADD CONSTRAINT line that follows ALTER TABLE (orphaned without ALTER TABLE prefix)
		case strings.HasPrefix(upper, "ADD CONSTRAINT") &&
			strings.Contains(upper, "FOREIGN KEY"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mAlterFk
			continue
		// Strip lines that are part of multi-line ALTER TABLE but don't start with ALTER
		case strings.HasPrefix(upper, "FOREIGN KEY"):
			// These are orphan lines from ALTER TABLE ADD CONSTRAINT; strip them
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			skipMode = mAlterFk
			continue
		// Strip CREATE INDEX ... USING (PG-specific index types: ivfflat, etc.)
		// Single-line: CREATE INDEX ... USING ... ;  (all on one line)
		case strings.HasPrefix(upper, "CREATE INDEX") &&
			strings.Contains(upper, "USING"):
			if strings.HasSuffix(trimmed, ";") {
				continue
			}
			// Multi-line index with USING — track paren depth
			for _, ch := range trimmed {
				if ch == '(' {
					skipDepth++
				}
				if ch == ')' {
					skipDepth--
				}
			}
			skipMode = mIndex
			continue
		// Multi-line CREATE INDEX (may contain USING on subsequent lines)
		case strings.HasPrefix(upper, "CREATE INDEX"):
			if !strings.HasSuffix(trimmed, ";") {
				skipHasUsing = strings.Contains(upper, "USING")
				createIndexBuf = append(createIndexBuf[:0], line)
				skipMode = mCreateIndex
				continue
			}
			// Single-line CREATE INDEX without USING — keep it
			out = append(out, line)
			continue
		}

		out = append(out, line)
	}

	result := strings.Join(out, "\n")

	// Strip goose-format Down migration sections.
	// The custom runner doesn't use goose — it executes all SQL in the file.
	// Goose's -- +goose Down marks the rollback section, which contains
	// Postgres-specific syntax (DROP COLUMN IF EXISTS, etc.) that SQLite rejects.
	// Strip everything from "-- +goose Down" to end of file (or "-- +goose Up" restart).
	if idx := strings.Index(result, "\n-- +goose Down"); idx >= 0 {
		result = result[:idx]
	} else if idx := strings.Index(result, "-- +goose Down"); idx >= 0 {
		result = result[:idx]
	}

	// ===================================================================
	// Phase 2: Translate PG types/functions to SQLite equivalents
	// ===================================================================

	// Types (order matters — replace longer patterns first)
	result = strings.ReplaceAll(result, "TIMESTAMPTZ", "TEXT")
	result = strings.ReplaceAll(result, "BIGSERIAL", "INTEGER")
	result = strings.ReplaceAll(result, " SERIAL ", " INTEGER ")
	result = strings.ReplaceAll(result, " UUID ", " TEXT ")
	result = strings.ReplaceAll(result, "JSONB", "TEXT")
	result = strings.ReplaceAll(result, "jsonb", "TEXT")
	result = strings.ReplaceAll(result, "BIGINT[]", "TEXT")
	result = strings.ReplaceAll(result, "BIGINT[]", "TEXT")
	result = strings.ReplaceAll(result, "UUID[]", "TEXT")
	result = strings.ReplaceAll(result, "INT[]", "TEXT")

	// Functions
	// Order matters — replace INTERVAL expressions before generic now() replacement.
	// "now() + INTERVAL '1 hour'" → "datetime('now', '+1 hour')"
	result = strings.ReplaceAll(result, "now() + INTERVAL '1 hour'", "datetime('now', '+1 hour')")
	result = strings.ReplaceAll(result, "gen_random_uuid()",
		"(lower(hex(randomblob(4)))||'-'||lower(hex(randomblob(2)))||'-4'||substr(lower(hex(randomblob(2))),2)||'-'||substr('89ab',abs(random())%4+1,1)||substr(lower(hex(randomblob(2))),2)||'-'||lower(hex(randomblob(6))))")
	result = strings.ReplaceAll(result, "now()", "datetime('now')")
	result = strings.ReplaceAll(result, "vector(1536)", "BLOB")

	// SQLite doesn't support IF NOT EXISTS on ALTER TABLE ADD COLUMN
	// (only on CREATE TABLE). Strip it so the migration proceeds.
	result = strings.ReplaceAll(result, "ADD COLUMN IF NOT EXISTS", "ADD COLUMN")
	// wrapped in parentheses. Postgres doesn't, so after we replace now()
	// with datetime('now'), we must wrap bare datetime('now') uses.
	// Pattern: "DEFAULT datetime('now')" → "DEFAULT (datetime('now'))"
	result = strings.ReplaceAll(result, "DEFAULT datetime('now')", "DEFAULT (datetime('now'))")

	// Type casts
	result = strings.ReplaceAll(result, "::UUID", "")
	result = strings.ReplaceAll(result, "::uuid", "")
	result = strings.ReplaceAll(result, "::BIGINT", "")
	result = strings.ReplaceAll(result, "::TEXT", "")
	result = strings.ReplaceAll(result, "::text", "")
	result = strings.ReplaceAll(result, "::INT", "")

	// Remove BEGIN/COMMIT wrapping (SQLite does auto-commit per statement)
	result = strings.ReplaceAll(result, "BEGIN;\n", "")
	result = strings.ReplaceAll(result, "\nCOMMIT;", "")

	// Collapse blank lines
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(result)
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case fmt.Stringer:
		return val.String()
	default:
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}
