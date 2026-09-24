package driver

import (
	"context"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// axiom:trace work_item=runtime-harness-01 spec=specs/001-architecture.md plan=phase-1/task-1-1/step-1-1-2 test=internal/db/driver/driver_test.go

func TestOpenSQLiteInMemory(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("Open(sqlite://:memory:) unexpected error: %v", err)
	}
	if database == nil {
		t.Fatal("expected non-nil db")
	}
	database.Close()
}

func TestOpenSQLiteFileTemp(t *testing.T) {
	ctx := context.Background()

	// Use a unique temp file to avoid cross-test contamination
	f, err := os.CreateTemp("", "consensus-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	path := f.Name()
	defer os.Remove(path)

	database, err := Open(ctx, db.Config{URL: "sqlite://" + path})
	if err != nil {
		t.Fatalf("Open(sqlite file) unexpected error: %v", err)
	}
	defer database.Close()

	// Create schema
	database.Exec(ctx, "CREATE TABLE IF NOT EXISTS _test (id INTEGER PRIMARY KEY, name TEXT)")

	// Begin transaction
	tx, err := database.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := tx.Exec(ctx, "INSERT INTO _test VALUES (1, 'tx_test')"); err != nil {
		t.Fatalf("tx.Exec: %v", err)
	}

	// Query within tx
	rows, err := tx.Query(ctx, "SELECT id, name FROM _test WHERE id = 1")
	if err != nil {
		t.Fatalf("tx.Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row in tx, got %d", len(rows))
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify committed outside tx
	rows, err = database.Query(ctx, "SELECT id, name FROM _test")
	if err != nil {
		t.Fatalf("query after commit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after commit, got %d", len(rows))
	}
}

func TestTransactionRollback(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Create schema
	database.Exec(ctx, "CREATE TABLE IF NOT EXISTS _test (id INTEGER PRIMARY KEY, name TEXT)")
	database.Exec(ctx, "INSERT INTO _test VALUES (1, 'before_rollback')")

	tx, err := database.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	tx.Exec(ctx, "INSERT INTO _test VALUES (2, 'should_not_exist')")

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Verify only original row exists
	rows, _ := database.Query(ctx, "SELECT id, name FROM _test")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after rollback, got %d", len(rows))
	}
	if rows[0]["name"] != "before_rollback" {
		t.Errorf("expected 'before_rollback', got %v", rows[0]["name"])
	}
}
