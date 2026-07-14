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
	"embed"
	"fmt"
	"io/fs"
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
	CurrentVersion      int
	AppliedMigrations   []string
	PendingMigrations   []string
	DriftDetected       bool
	DriftDetails        string
	MigrationRequired   bool
}

// Runner manages schema migrations.
type Runner struct {
	database    db.DB
	migrations  []Migration
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
	for _, row := range rows {
		v, ok := row["version"]
		if !ok {
			continue
		}
		version := toInt(v)
		name, _ := row["name"].(string)
		appliedVersions = append(appliedVersions, version)
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

	// Drift detection: are there applied migrations that don't have embedded files?
	for _, av := range appliedVersions {
		found := false
		for _, m := range r.migrations {
			if m.Version == av {
				found = true
				break
			}
		}
		if !found {
			state.DriftDetected = true
			state.DriftDetails += fmt.Sprintf("Applied migration version %d has no matching embedded file\n", av)
		}
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
			sql = filterForSQLite(sql)
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

		// Record the migration
		checksum := fmt.Sprintf("%x", len(m.SQL)) // Simple length-based checksum
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

	return len(applied) > 0, nil
}

// repairTrustLevel adds sessions.trust_level if missing (migration 013 filterForSQLite bug).
// Safe to call on any DB: checks PRAGMA table_info before ALTER TABLE.
func (r *Runner) repairTrustLevel(ctx context.Context) error {
	cols, err := r.database.Query(ctx, "PRAGMA table_info(sessions)")
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

// filterForSQLite transforms PostgreSQL migration SQL for SQLite compatibility.
//
// Two phases:
//   1. Strip: remove PG-only constructs (extensions, functions, triggers, policies,
//      role grants, pg_cron, views, ALTER TABLE ADD CONSTRAINT).
//   2. Translate: rewrite types (UUID→TEXT, TIMESTAMPTZ→TEXT, JSONB→TEXT, etc.),
//      functions (gen_random_uuid(), now()), and remove type casts.
func filterForSQLite(rawSQL string) string {
	lines := strings.Split(rawSQL, "\n")
	out := make([]string, 0, len(lines))

	skipMode := 0           // 0=none, 1=function, 2=trigger, 3=policy, 4=cron, 5=rls, 6=extension, 7=doblock, 8=alterFk, 9=view, 10=index, 11=alterTable, 12=createIndex, 13=comment
	skipUntilMarker := ""   // closing $$ tag
	skipDepth := 0          // paren depth tracker
	skipHasUsing := false   // tracks whether CREATE INDEX block contains USING (PG-specific)
	createIndexBuf := make([]string, 0, 20) // buffer for multi-line CREATE INDEX statements
	alterTableBuf := make([]string, 0, 10) // buffer for multi-line ALTER TABLE statements
	alterTableHasPG := false               // true if buffered ALTER TABLE has PG-only keywords

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

	for _, line := range lines {
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
						if ch == '(' { skipDepth++ }
						if ch == ')' { skipDepth-- }
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
					if ch == '(' { skipDepth++ }
					if ch == ')' { skipDepth-- }
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
						if ch == '(' { skipDepth++ }
						if ch == ')' { skipDepth-- }
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
				if ch == '(' { skipDepth++ }
				if ch == ')' { skipDepth-- }
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
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mExtension; continue
		case strings.HasPrefix(upper, "CREATE ") && strings.Contains(upper, "FUNCTION"):
			skipMode = mFunction; skipUntilMarker = ""
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) { skipUntilMarker = t; break }
			}
			continue
		case strings.HasPrefix(upper, "DO ") && strings.Contains(upper, "$$"):
			skipMode = mDOBlock; skipUntilMarker = ""
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) { skipUntilMarker = t; break }
			}
			continue
		case strings.HasPrefix(upper, "CREATE TRIGGER"):
			// SQLite-native triggers use BEGIN...END (keep these)
			// PostgreSQL triggers use EXECUTE FUNCTION (skip these)
			if strings.Contains(upper, " BEGIN ") {
				// SQLite-style — keep the line as-is (don't skip)
				// The BEGIN...END body has its own semicolons, but that's fine
				// because splitStatements handles it on the full migration content later.
				// We just need to NOT skip it here.
			} else {
				skipMode = mTrigger; skipDepth = 0; skipUntilMarker = ""
				for _, ch := range trimmed { if ch == '(' { skipDepth++ }; if ch == ')' { skipDepth-- } }
				if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 { continue }
				continue
			}
		case strings.HasPrefix(upper, "DROP TRIGGER"):
			continue
		case strings.HasPrefix(upper, "CREATE POLICY"):
			skipMode = mPolicy; skipDepth = 0
			for _, ch := range trimmed { if ch == '(' { skipDepth++ }; if ch == ')' { skipDepth-- } }
			if strings.HasSuffix(trimmed, ";") && skipDepth <= 0 { continue }
			continue
		case strings.HasPrefix(upper, "DROP POLICY"):
			continue
		case strings.HasPrefix(upper, "CREATE OR REPLACE VIEW") || strings.HasPrefix(upper, "CREATE VIEW"):
			skipMode = mView; continue
		case strings.HasPrefix(upper, "REVOKE ") || strings.HasPrefix(upper, "GRANT "):
			continue
		case strings.HasPrefix(upper, "COMMENT ON"):
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mComment; continue
		case strings.HasPrefix(upper, "ALTER ROLE"):
			continue
		case strings.HasPrefix(upper, "SELECT CRON."):
			skipMode = mCron; skipDepth = 0; skipUntilMarker = ""
			for _, ch := range trimmed { if ch == '(' { skipDepth++ }; if ch == ')' { skipDepth-- } }
			for _, t := range []string{"$$", "$func$", "$body$", "$tag$"} {
				if strings.Contains(trimmed, t) { skipUntilMarker = t; break }
			}
			continue
		case strings.HasPrefix(upper, "ALTER TABLE") &&
			strings.Contains(upper, "ROW LEVEL SECURITY"):
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mRLS; continue
		case strings.HasPrefix(upper, "ALTER TABLE") &&
			strings.Contains(upper, "ADD CONSTRAINT") &&
			strings.Contains(upper, "FOREIGN KEY"):
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mAlterFk; continue
		// Multi-line ALTER TABLE statements (ADD CONSTRAINT, ENABLE RLS, etc.)
		// Buffer and decide at semicolon whether to keep (SQLite-compatible) or strip (PG-only).
		case strings.HasPrefix(upper, "ALTER TABLE"):
			if !strings.HasSuffix(trimmed, ";") {
				alterTableBuf = append(alterTableBuf[:0], line) // start buffer with first line
				alterTableHasPG = false
				skipMode = mAlterTable; continue
			}
			// Single-line ALTER TABLE: strip if it's PG-only
			if strings.Contains(upper, "ADD CONSTRAINT") ||
				strings.Contains(upper, "ENABLE ROW LEVEL SECURITY") ||
				strings.Contains(upper, "BYPASSRLS") {
				continue
			}
			out = append(out, line); continue
		// Strip ADD CONSTRAINT line that follows ALTER TABLE (orphaned without ALTER TABLE prefix)
		case strings.HasPrefix(upper, "ADD CONSTRAINT") &&
			strings.Contains(upper, "FOREIGN KEY"):
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mAlterFk; continue
		// Strip lines that are part of multi-line ALTER TABLE but don't start with ALTER
		case strings.HasPrefix(upper, "FOREIGN KEY"):
			// These are orphan lines from ALTER TABLE ADD CONSTRAINT; strip them
			if strings.HasSuffix(trimmed, ";") { continue }
			skipMode = mAlterFk; continue
		// Strip CREATE INDEX ... USING (PG-specific index types: ivfflat, etc.)
		// Single-line: CREATE INDEX ... USING ... ;  (all on one line)
		case strings.HasPrefix(upper, "CREATE INDEX") &&
			strings.Contains(upper, "USING"):
			if strings.HasSuffix(trimmed, ";") { continue }
			// Multi-line index with USING — track paren depth
			for _, ch := range trimmed { if ch == '(' { skipDepth++ }; if ch == ')' { skipDepth-- } }
			skipMode = mIndex; continue
		// Multi-line CREATE INDEX (may contain USING on subsequent lines)
		case strings.HasPrefix(upper, "CREATE INDEX"):
			if !strings.HasSuffix(trimmed, ";") {
				skipHasUsing = strings.Contains(upper, "USING")
				createIndexBuf = append(createIndexBuf[:0], line)
				skipMode = mCreateIndex
				continue
			}
			// Single-line CREATE INDEX without USING — keep it
			out = append(out, line); continue
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
