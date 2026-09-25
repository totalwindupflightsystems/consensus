// postgres_test.go — Postgres pool-override integration test.
//
// Gated by CONSENSUS_TEST_POSTGRES_URL env var. Skips gracefully when unset.
// Verifies that the database.max_open_conns override reaches the pgx pool
// (pool-fix follow-up: the override must work per-backend — SQLite pins 4
// as its <=0 fallback, Postgres pins 10).
//
// axiom:trace work_item=C-GAP-POOL-FOLLOWUP impl=internal/db/postgres/postgres.go
package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// TestOpenMaxOpenConnsOverrideReachesPool proves cfg.MaxOpenConns reaches
// pgxpool.MaxConns on the live postgres path.
func TestOpenMaxOpenConnsOverrideReachesPool(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}

	ctx := context.Background()

	t.Run("explicit override", func(t *testing.T) {
		d, err := Open(ctx, db.Config{URL: pgURL, MaxOpenConns: 3})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer d.Close()

		if got := d.pool.Stat().MaxConns(); got != 3 {
			t.Errorf("pool MaxConns = %d, want 3 (cfg.MaxOpenConns=3 must reach the pool)", got)
		}
	})

	t.Run("zero falls back to 10", func(t *testing.T) {
		d, err := Open(ctx, db.Config{URL: pgURL, MaxOpenConns: 0})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer d.Close()

		if got := d.pool.Stat().MaxConns(); got != 10 {
			t.Errorf("pool MaxConns = %d, want 10 (MaxOpenConns<=0 falls back to 10)", got)
		}
	})
}
