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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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
		defer func() { _ = d.Close() }()

		if got := d.pool.Stat().MaxConns(); got != 3 {
			t.Errorf("pool MaxConns = %d, want 3 (cfg.MaxOpenConns=3 must reach the pool)", got)
		}
	})

	t.Run("zero falls back to 10", func(t *testing.T) {
		d, err := Open(ctx, db.Config{URL: pgURL, MaxOpenConns: 0})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = d.Close() }()

		if got := d.pool.Stat().MaxConns(); got != 10 {
			t.Errorf("pool MaxConns = %d, want 10 (MaxOpenConns<=0 falls back to 10)", got)
		}
	})
}

// ============================================================================
// REVIEW-CONSENSUS-3 Finding 3 — UUID columns must normalize to canonical text
// ============================================================================
//
// pgx decodes a `uuid` column as [16]byte (pgtype.UUIDCodec.DecodeValue returns
// uuid.Bytes). Before the fix normalizeValue passed that through untouched, so
// every UUID read through the Postgres driver reached the API layer as a byte
// array: `toString` rendered it as a decimal byte list, the session-scoped auth
// check ("session key can only message its own session") compared it against the
// URL path UUID and never matched, and task/memory-event ids were unusable.
//
// The unit test below is backend-agnostic and always runs; the integration test
// is gated by CONSENSUS_TEST_POSTGRES_URL and proves the scan path (scanRow →
// normalizeValue) against a real uuid column.

func TestNormalizeValueUUIDToCanonicalString(t *testing.T) {
	const canonical = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	raw, err := uuid.Parse(canonical)
	if err != nil {
		t.Fatalf("parse fixture uuid: %v", err)
	}
	var rawBytes [16]byte = raw

	tests := []struct {
		name       string
		in         any
		want       any
		wantString bool // the value must arrive as a plain string (not a byte array)
	}{
		{"nil", nil, nil, false},
		{"string passthrough", "already-a-string", "already-a-string", true},
		{"[]byte to string", []byte("bytes-to-string"), "bytes-to-string", true},
		{"[16]byte uuid to canonical string", rawBytes, canonical, true},
		{"google uuid.UUID to canonical string", raw, canonical, true},
		{"pgtype.UUID valid", pgtype.UUID{Bytes: rawBytes, Valid: true}, canonical, true},
		{"pgtype.UUID invalid", pgtype.UUID{Valid: false}, nil, false},
		{"int64 passthrough", int64(42), int64(42), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeValue(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeValue(%T %v) = %T %v, want %T %v",
					tc.in, tc.in, got, got, tc.want, tc.want)
			}
			// A byte-array leak is the regression: a uuid must arrive as a
			// plain string, not [16]byte (which the API renders as a byte list).
			if tc.wantString {
				if _, isString := got.(string); !isString {
					t.Errorf("normalizeValue(%T) returned %T, want string", tc.in, got)
				}
			}
		})
	}

	// Negative control: the raw [16]byte does not read as the canonical text —
	// this is exactly what the pre-fix code handed to the API layer, where
	// toString falls through to fmt.Sprintf("%v", v).
	if rendered := fmt.Sprintf("%v", rawBytes); rendered == canonical {
		t.Fatalf("premise broken: %q already reads as the canonical uuid, so this test could not catch the defect", rendered)
	} else if !strings.Contains(rendered, " ") {
		t.Fatalf("premise broken: %v does not render as a byte list", rawBytes)
	}
}

// TestPostgresUUIDNormalization is the live half: a real uuid column read
// through the driver's scan path must arrive as the canonical string. Gated by
// CONSENSUS_TEST_POSTGRES_URL (skips without a server, so `-short` stays green).
func TestPostgresUUIDNormalization(t *testing.T) {
	pgURL := os.Getenv("CONSENSUS_TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("CONSENSUS_TEST_POSTGRES_URL not set; skipping Postgres UUID normalization test")
	}

	ctx := context.Background()
	d, err := Open(ctx, db.Config{URL: pgURL})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = d.Close() }()

	// Run as the connection owner: the probe table is created by this test and
	// the main pool runs under SET ROLE agent_role with no grant on it. The
	// scan path under test (scanRow → normalizeValue) is identical for both
	// pools — AdminDB() reuses the same *DB scan helpers.
	admin := d.AdminDB()

	const table = "uuid_norm_probe"
	if err := admin.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (id UUID PRIMARY KEY, label TEXT)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	defer func() { _ = admin.Exec(ctx, `DROP TABLE IF EXISTS `+table) }()
	if err := admin.Exec(ctx, `DELETE FROM `+table); err != nil {
		t.Fatalf("reset probe table: %v", err)
	}

	const want = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	if err := admin.Exec(ctx, `INSERT INTO `+table+` (id, label) VALUES ($1, 'probe')`, want); err != nil {
		t.Fatalf("insert probe row (arg encode): %v", err)
	}

	// (1) The round trip: the argument string must come back verbatim.
	rows, err := admin.Query(ctx, `SELECT id, label FROM `+table+` WHERE id = $1`, want)
	if err != nil {
		t.Fatalf("query probe row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("probe rows = %d, want 1", len(rows))
	}
	got, ok := rows[0]["id"].(string)
	if !ok {
		t.Fatalf("normalizeValue left the uuid as %T (%v) — the API layer cannot compare it",
			rows[0]["id"], rows[0]["id"])
	}
	if got != want {
		t.Fatalf("uuid round trip = %q, want %q", got, want)
	}
	if label, _ := rows[0]["label"].(string); label != "probe" {
		t.Errorf("non-uuid column mangled: label=%q", label)
	}

	// (2) A uuid compared in SQL to a driver-decoded value (the actual shape of
	// the session-key check): the second read must resolve the same row.
	rows, err = admin.Query(ctx, `SELECT id FROM `+table+` WHERE id = $1::uuid AND id::text = $2`, want, want)
	if err != nil {
		t.Fatalf("query probe row (text/cast form): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("cast-form rows = %d, want 1", len(rows))
	}
	if got, _ := rows[0]["id"].(string); got != want {
		t.Errorf("cast-form uuid = %q, want %q", got, want)
	}

	// (3) NULL uuid must stay nil, not become a zero-value string.
	if err := admin.Exec(ctx, `ALTER TABLE `+table+` ADD COLUMN IF NOT EXISTS nullable_id UUID`); err != nil {
		t.Fatalf("add nullable column: %v", err)
	}
	rows, err = admin.Query(ctx, `SELECT nullable_id FROM `+table+` WHERE id = $1`, want)
	if err != nil {
		t.Fatalf("query nullable uuid: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("nullable rows = %d, want 1", len(rows))
	}
	if rows[0]["nullable_id"] != nil {
		t.Errorf("NULL uuid normalized to %T %v, want nil", rows[0]["nullable_id"], rows[0]["nullable_id"])
	}
}
