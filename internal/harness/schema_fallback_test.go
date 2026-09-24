// Package harness: regression tests for the coreTableColumns fallback schema
// (HARNESS-SCHEMA-001).
//
// The fallback column lists in prompt.go are what the LLM sees when DB schema
// discovery fails. If a fallback lists columns that don't exist in the real
// schema, the LLM generates INSERTs against them, the DB rejects the statement
// ("table tasks has no column named assigned_to"), and the planning loop dies.
// These tests parse the migration files and verify the fallbacks stay in sync
// with the actual schema.
package harness

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot returns the repository root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // internal/harness/schema_fallback_test.go -> repo root
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// parseCreateTableColumns extracts column names from every CREATE TABLE block
// in the given SQL text. Column names are the first token of each non-empty
// line inside the block that isn't a constraint keyword, a quoted value, or a
// closing paren (handles multi-line CHECK constraints).
func parseCreateTableColumns(t *testing.T, sql string) map[string][]string {
	t.Helper()
	tables := map[string][]string{}
	lines := strings.Split(sql, "\n")

	var curTable string
	var inBlock bool
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !inBlock {
			if strings.HasPrefix(trimmed, "CREATE TABLE") {
				// "CREATE TABLE [IF NOT EXISTS] name ("
				fields := strings.Fields(trimmed)
				idx := 2 // CREATE TABLE <name>
				if len(fields) > idx && fields[idx] == "IF" {
					idx = 5 // CREATE TABLE IF NOT EXISTS <name>
				}
				if len(fields) <= idx {
					continue
				}
				curTable = fields[idx]
				inBlock = true
			}
			continue
		}
		// Inside a CREATE TABLE block.
		if strings.HasPrefix(trimmed, ");") || trimmed == ")" {
			inBlock = false
			continue
		}
		first := strings.Fields(trimmed)[0]
		// Skip constraint clauses, continuation lines of CHECK lists, and
		// closing parens of nested blocks.
		switch first {
		case "CHECK", "CONSTRAINT", "PRIMARY", "UNIQUE", "FOREIGN", "REFERENCES", ")", "(":
			continue
		}
		if strings.HasPrefix(first, "'") || strings.HasPrefix(first, ")") {
			continue
		}
		tables[curTable] = append(tables[curTable], first)
	}
	return tables
}

// readMigrations loads all migration SQL files from the repo.
func readMigrations(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "migrate", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

// TestCoreTableColumnsTasksMatchMigrationSchema is the HARNESS-SCHEMA-001
// regression test: the "tasks" fallback must list only real columns from the
// migration schema (009_sqlite_task_tool_tables.sql is the SQLite variant used
// in tests; 001_initial_schema.sql is the Postgres source of truth).
func TestCoreTableColumnsTasksMatchMigrationSchema(t *testing.T) {
	migs := readMigrations(t)

	sqliteSQL, ok := migs["009_sqlite_task_tool_tables.sql"]
	if !ok {
		t.Fatal("missing migration: 009_sqlite_task_tool_tables.sql")
	}
	pgSQL, ok := migs["001_initial_schema.sql"]
	if !ok {
		t.Fatal("missing migration: 001_initial_schema.sql")
	}

	sqliteCols := parseCreateTableColumns(t, sqliteSQL)["tasks"]
	pgCols := parseCreateTableColumns(t, pgSQL)["tasks"]
	if len(sqliteCols) == 0 {
		t.Fatal("no tasks columns parsed from 009_sqlite_task_tool_tables.sql")
	}
	if len(pgCols) == 0 {
		t.Fatal("no tasks columns parsed from 001_initial_schema.sql")
	}

	// Both variants must declare the same column names.
	if strings.Join(sqliteCols, ",") != strings.Join(pgCols, ",") {
		t.Errorf("tasks schema divergence:\n  sqlite: %v\n  pg:     %v", sqliteCols, pgCols)
	}

	fallback := coreTableColumns["tasks"]
	fallbackCols := strings.Split(fallback, ",")
	for i := range fallbackCols {
		fallbackCols[i] = strings.TrimSpace(fallbackCols[i])
	}

	// Every fallback column must exist in the real schema.
	realSet := map[string]bool{}
	for _, c := range sqliteCols {
		realSet[c] = true
	}
	for _, c := range fallbackCols {
		if !realSet[c] {
			t.Errorf("coreTableColumns[\"tasks\"] lists column %q that does not exist in migration 009 tasks schema", c)
		}
	}

	// Required real columns must be present in the fallback.
	required := []string{
		"title", "priority", "locked_by_agent", "prerequisite_ids",
		"result_memory_id", "claimed_at", "completed_at",
	}
	for _, c := range required {
		if !realSet[c] {
			t.Errorf("migration 009 tasks schema missing expected column %q — test fixture stale?", c)
		}
		if !containsStr(fallbackCols, c) {
			t.Errorf("coreTableColumns[\"tasks\"] missing real column %q", c)
		}
	}

	// The bug columns must be ABSENT from the fallback.
	for _, banned := range []string{"assigned_to", "result"} {
		if containsStr(fallbackCols, banned) {
			t.Errorf("coreTableColumns[\"tasks\"] still lists nonexistent column %q — HARNESS-SCHEMA-001 NOT fixed", banned)
		}
	}

	t.Logf("tasks fallback (%d cols): %s", len(fallbackCols), fallback)
}

// TestCoreTableColumnsAllMatchMigrationSchemas guards every fallback entry
// against the parsed migration schemas so fabricated columns can't regress in
// any core table (the same bug class as HARNESS-SCHEMA-001).
func TestCoreTableColumnsAllMatchMigrationSchemas(t *testing.T) {
	migs := readMigrations(t)
	schemas := map[string][]string{}
	for _, sql := range migs {
		for table, cols := range parseCreateTableColumns(t, sql) {
			schemas[table] = cols
		}
	}

	for table, fallback := range coreTableColumns {
		realCols, ok := schemas[table]
		if !ok {
			// Table may be defined dynamically; skip tables with no migration.
			t.Logf("coreTableColumns[%q]: no CREATE TABLE found in migrations — skipped", table)
			continue
		}
		realSet := map[string]bool{}
		for _, c := range realCols {
			realSet[c] = true
		}
		for _, c := range strings.Split(fallback, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if !realSet[c] {
				t.Errorf("coreTableColumns[%q] lists column %q that does not exist in the migration schema (real: %v)", table, c, realCols)
			}
		}
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
