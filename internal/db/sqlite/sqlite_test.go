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
