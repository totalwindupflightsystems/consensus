// Package sqlite tests: parent-directory creation (C-GAP-026).
package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// TestOpenCreatesParentDirectory verifies that Open creates missing parent
// directories for the database file (SQLite itself does not). C-GAP-026:
// the default URL is ~/.consensus/consensus.db, and ~/.consensus does not
// exist on a fresh machine — the first serve would otherwise fail.
func TestOpenCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "sub", "consensus.db")

	conn, err := Open(context.Background(), db.Config{URL: "sqlite://" + dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if fi, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	} else if fi.Size() == 0 {
		t.Error("expected non-empty database file after open")
	}

	parent := filepath.Join(dir, "nested", "sub")
	fi, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("parent path is not a directory: %s", parent)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected parent directory mode 0700, got %o", perm)
	}
}

// TestOpenExistingParentDirectory verifies MkdirAll is a no-op for existing
// directories and does not clobber their permissions.
func TestOpenExistingParentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "existing"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	dbPath := filepath.Join(dir, "existing", "consensus.db")

	conn, err := Open(context.Background(), db.Config{URL: "sqlite://" + dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	fi, err := os.Stat(filepath.Join(dir, "existing"))
	if err != nil {
		t.Fatalf("parent dir: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o755 {
		t.Errorf("expected existing parent perms preserved (0755), got %o", perm)
	}
}

// TestOpenMemorySkippedParentCreation verifies :memory: opens cleanly
// without touching the filesystem.
func TestOpenMemorySkippedParentCreation(t *testing.T) {
	conn, err := Open(context.Background(), db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("Open :memory:: %v", err)
	}
	defer conn.Close()
}

// TestOpenMaxOpenConnsOverrideReachesDriver proves the max_open_conns
// override actually reaches the driver (pool-fix follow-up: the shipped
// consensus.yaml pins database.max_open_conns: 5 and the no-config
// default is 8 — both must be honored per-backend, including SQLite).
// SQLite is in-package, so the wrapped *sql.DB is reachable via d.conn.
func TestOpenMaxOpenConnsOverrideReachesDriver(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit override", func(t *testing.T) {
		d, err := Open(ctx, db.Config{URL: "sqlite://:memory:", MaxOpenConns: 3})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer d.Close()

		if got := d.conn.Stats().MaxOpenConnections; got != 3 {
			t.Errorf("MaxOpenConnections = %d, want 3 (cfg.MaxOpenConns=3 must reach the driver)", got)
		}
	})

	t.Run("zero falls back to 4", func(t *testing.T) {
		d, err := Open(ctx, db.Config{URL: "sqlite://:memory:", MaxOpenConns: 0})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer d.Close()

		if got := d.conn.Stats().MaxOpenConnections; got != 4 {
			t.Errorf("MaxOpenConnections = %d, want 4 (MaxOpenConns<=0 falls back to 4)", got)
		}
	})
}
