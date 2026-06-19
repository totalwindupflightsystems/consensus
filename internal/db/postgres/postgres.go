// Package postgres implements the db.DB interface for PostgreSQL via pgx/v5.
//
// PostgreSQL is the production backend for multi-tenant and horizontally-scaled
// deployments. Uses pgx/v5 with connection pooling (pgxpool) for production-grade
// database access with native LISTEN/NOTIFY support.
//
// axiom:trace work_item=WI-002-migrate-pgx spec=specs/009-deployment.md,specs/015-api-and-mcp.md,specs/022-library-research.md plan=phase-1/task-2
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wojons/consensus/internal/db"
)

// DB is a PostgreSQL-backed implementation of db.DB.
type DB struct {
	pool *pgxpool.Pool
}

// Open creates a new PostgreSQL connection pool.
// The connection URL format is "postgres://user:pass@host:port/dbname?sslmode=disable"
// or "postgresql://..." (the standard format).
func Open(ctx context.Context, cfg db.Config) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	// Configure pool
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	poolConfig.MaxConns = int32(maxOpen)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping failed: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Backend returns the database backend type.
func (d *DB) Backend() db.Backend {
	return db.BackendPostgres
}

// BeginTx starts a PostgreSQL transaction.
func (d *DB) BeginTx(ctx context.Context) (db.Tx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin tx: %w", err)
	}
	return &Tx{tx: tx, active: true}, nil
}

// Exec executes a query without returning rows.
func (d *DB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("postgres: exec: %w", err)
	}
	return nil
}

// Query executes a query that returns rows.
func (d *DB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// QueryRow executes a query that returns at most one row.
func (d *DB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query row: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("postgres: no rows in result")
	}

	row, err := scanSingleRow(rows)
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		return nil, fmt.Errorf("postgres: multiple rows returned")
	}

	return row, rows.Err()
}

// Close closes the connection pool.
func (d *DB) Close() error {
	d.pool.Close()
	return nil
}

// Pool returns the underlying pgx pool for advanced operations
// like LISTEN/NOTIFY that require a dedicated connection.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// ============================================================================
// Transaction
// ============================================================================

// Tx wraps a pgx transaction.
type Tx struct {
	tx     pgx.Tx
	active bool
}

// Exec executes a query within the transaction.
func (tx *Tx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := tx.tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("postgres tx: exec: %w", err)
	}
	return nil
}

// Query executes a query that returns rows within the transaction.
func (tx *Tx) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	rows, err := tx.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres tx: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// QueryRow executes a query that returns at most one row within the transaction.
func (tx *Tx) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := tx.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres tx: query row: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("postgres tx: no rows in result")
	}

	row, err := scanSingleRow(rows)
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		return nil, fmt.Errorf("postgres tx: multiple rows returned")
	}

	return row, rows.Err()
}

// SetSessionContext sets the session identity for RLS isolation.
// On PostgreSQL this issues SET LOCAL consensus.session_id, which
// is automatically reset when the transaction ends.
func (tx *Tx) SetSessionContext(ctx context.Context, sessionID string) error {
	_, err := tx.tx.Exec(ctx,
		"SELECT set_config('consensus.session_id', $1, true)",
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("postgres: set session context: %w", err)
	}
	return nil
}

// Commit commits the transaction.
// Uses context.Background() because the db.Tx interface does not pass context
// through Commit/Rollback. The transaction context is set at BeginTx time.
func (tx *Tx) Commit() error {
	tx.active = false
	return tx.tx.Commit(context.Background())
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error {
	tx.active = false
	return tx.tx.Rollback(context.Background())
}

// IsActive returns true if the transaction has not been committed or rolled back.
func (tx *Tx) IsActive() bool {
	return tx.active
}

// ============================================================================
// Row scanning helpers
// ============================================================================

func scanRows(rows pgx.Rows) ([]db.Row, error) {
	fields := rows.FieldDescriptions()
	fieldNames := make([]string, len(fields))
	for i, f := range fields {
		fieldNames[i] = string(f.Name)
	}

	var results []db.Row
	for rows.Next() {
		row, err := scanRow(rows, fieldNames)
		if err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func scanSingleRow(rows pgx.Rows) (db.Row, error) {
	fields := rows.FieldDescriptions()
	fieldNames := make([]string, len(fields))
	for i, f := range fields {
		fieldNames[i] = string(f.Name)
	}
	return scanRow(rows, fieldNames)
}

func scanRow(rows pgx.Rows, columns []string) (db.Row, error) {
	values, err := rows.Values()
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	row := make(db.Row, len(columns))
	for i, col := range columns {
		row[col] = normalizeValue(values[i])
	}
	return row, nil
}

// normalizeValue converts driver-specific types to standard Go types.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	default:
		return val
	}
}
