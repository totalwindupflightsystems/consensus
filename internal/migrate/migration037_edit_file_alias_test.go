// Package migrate: SCN-CONSENSUS-025 — migration 037 seeds the `edit_file`
// alias registry row (handler_ref 'edit') so a model's drifted edit_file
// request resolves instead of failing closed with
// `tool "edit_file" is not registered or enabled` (the same drift class
// DF-CONSENSUS-85 fixed for shell names via 035). These tests run the REAL
// ladder (Bootstrap + all migrations) on a fresh SQLite DB.
package migrate

import (
	"context"
	"testing"
)

// TestSqliteMigration037SeedsEditFileAlias asserts the alias row exists after
// the full ladder, points at the native edit handler, and is enabled.
func TestSqliteMigration037SeedsEditFileAlias(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	rows, err := database.Query(ctx,
		`SELECT name, handler_ref, handler_type, enabled, requires_approval
		 FROM tools_registry WHERE name = 'edit_file'`)
	if err != nil {
		t.Fatalf("query edit_file row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("edit_file registry rows = %d, want exactly 1", len(rows))
	}
	if got := toString(rows[0]["handler_ref"]); got != "edit" {
		t.Errorf("handler_ref = %q, want edit", got)
	}
	if got := toString(rows[0]["handler_type"]); got != "go_native" {
		t.Errorf("handler_type = %q, want go_native", got)
	}
	if got := toInt(rows[0]["enabled"]); got != 1 {
		t.Errorf("enabled = %d, want 1", got)
	}
	if got := toInt(rows[0]["requires_approval"]); got != 0 {
		t.Errorf("requires_approval = %d, want 0 (mirrors the ungated edit builtin)", got)
	}

	// The public edit row must be untouched by the alias seed.
	pubRows, err := database.Query(ctx,
		`SELECT handler_ref, enabled FROM tools_registry WHERE name = 'edit'`)
	if err != nil {
		t.Fatalf("query edit row: %v", err)
	}
	if len(pubRows) != 1 {
		t.Fatalf("edit registry rows = %d, want exactly 1", len(pubRows))
	}
	if got := toString(pubRows[0]["handler_ref"]); got != "edit" {
		t.Errorf("edit handler_ref = %q, want edit (037 must not modify the public row)", got)
	}
}

// TestSqliteMigration037Idempotent re-runs the ladder and asserts no duplicate
// alias row appears (ON CONFLICT (name) DO NOTHING).
func TestSqliteMigration037Idempotent(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	runner := New(database)
	for i := 0; i < 2; i++ {
		if _, err := runner.AutoMigrate(ctx); err != nil {
			t.Fatalf("AutoMigrate pass %d failed: %v", i+1, err)
		}
	}
	rows, err := database.Query(ctx, `SELECT name FROM tools_registry WHERE name = 'edit_file'`)
	if err != nil {
		t.Fatalf("query edit_file row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("edit_file registry rows after re-migration = %d, want exactly 1", len(rows))
	}
}
