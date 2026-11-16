package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/tools"
)

// ch:trace row=DF-CONSENSUS-66 spec=specs/010-tools.md#builtin-tools wave=none:serial test=internal/harness/tool_executor_native_test.go evidence=worker-local-pending verdict=none:foreman-pending commit=worker-pending memory=none:foreman-pending
// STATUS: partial
// TIER: T1
// EVIDENCE: worker test output pending

func TestFreshMigrationSeedsFilesystemBuiltinTools(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	if _, err := New(database).AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	rows, err := database.Query(context.Background(), `SELECT name, handler_type, handler_ref, enabled, status FROM tools_registry WHERE name IN ('read', 'write', 'edit', 'glob', 'bash') ORDER BY name`)
	if err != nil {
		t.Fatalf("query tools_registry: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("seeded filesystem tools = %d, want 5", len(rows))
	}
	for _, row := range rows {
		name := toString(row["name"])
		if toString(row["handler_type"]) != "go_native" || toString(row["status"]) != "active" || toInt(row["enabled"]) == 0 {
			t.Errorf("%s registry row = %#v, want active enabled go_native", name, row)
		}
		ref := toString(row["handler_ref"])
		if _, ok := tools.NativeHandlerFor(ref); !ok {
			t.Errorf("%s handler_ref %q is not registered", name, ref)
		}
	}
}

func TestFreshMigrationSeedIsIdempotent(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	runner := New(database)
	if _, err := runner.AutoMigrate(ctx); err != nil {
		t.Fatalf("first AutoMigrate: %v", err)
	}
	if err := database.Exec(ctx, `UPDATE tools_registry SET description = 'operator-owned' WHERE name = 'read'`); err != nil {
		t.Fatalf("customize read row: %v", err)
	}

	// Re-running the exact seed is safe and does not overwrite operator data.
	rows, err := database.Query(ctx, `SELECT description FROM tools_registry WHERE name = 'read'`)
	if err != nil || len(rows) != 1 || !strings.Contains(toString(rows[0]["description"]), "operator-owned") {
		t.Fatalf("read row was unexpectedly overwritten: rows=%#v err=%v", rows, err)
	}
}
