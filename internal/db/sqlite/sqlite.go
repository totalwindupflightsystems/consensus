// Package sqlite implements the db.DB interface for SQLite via modernc.org/sqlite.
//
// SQLite is the local-first backend for single-user and air-gapped deployments.
// Uses database/sql with modernc.org/sqlite driver (pure Go, no CGO).
//
// axiom:trace work_item=runtime-harness-01 spec=specs/003-database.md,specs/021-repository-layout.md plan=phase-1/task-1-1/step-1-1-2
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/wojons/consensus/internal/db"
)

// DB is a SQLite-backed implementation of db.DB.
type DB struct {
	conn *sql.DB
}

// Open creates a new SQLite connection.
// The connection URL format is "sqlite://path/to/file.db" or "sqlite://:memory:".
func Open(ctx context.Context, cfg db.Config) (*DB, error) {
	path := strings.TrimPrefix(cfg.URL, "sqlite://")
	if path == "" {
		return nil, fmt.Errorf("sqlite: empty path in URL %q", cfg.URL)
	}

	// Add query parameters for WAL mode, busy timeout, and foreign keys
	dsn := path
	if !strings.Contains(dsn, "?") {
		dsn += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	} else {
		if !strings.Contains(dsn, "_pragma=journal_mode") {
			dsn += "&_pragma=journal_mode(WAL)"
		}
		if !strings.Contains(dsn, "_pragma=busy_timeout") {
			dsn += "&_pragma=busy_timeout(5000)"
		}
		if !strings.Contains(dsn, "_pragma=foreign_keys") {
			dsn += "&_pragma=foreign_keys(on)"
		}
	}

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open failed: %w", err)
	}

	// Verify connection
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sqlite: ping failed: %w", err)
	}

	// Explicitly enable WAL mode and busy timeout (driver may not honor DSN pragmas)
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON"); err != nil {
		slog.Warn("sqlite: explicit pragmas failed", "error", err)
	}

	// WAL mode allows concurrent readers alongside one writer.
	// Set higher to let heartbeat + planning + event polling coexist.
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 4
	}
	conn.SetMaxOpenConns(maxOpen)

	return &DB{conn: conn}, nil
}

// Backend returns the database backend type.
func (d *DB) Backend() db.Backend {
	return db.BackendSQLite
}

// BeginTx starts a SQLite transaction.
func (d *DB) BeginTx(ctx context.Context) (db.Tx, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin tx: %w", err)
	}
	return &Tx{tx: tx, active: true}, nil
}

// Exec executes a query without returning rows.
func (d *DB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite: exec: %w", err)
	}
	return nil
}

// Query executes a query that returns rows.
func (d *DB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// QueryRow executes a query that returns at most one row.
func (d *DB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query row: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("sqlite: no rows in result")
	}

	row, err := scanSingleRow(rows)
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		return nil, fmt.Errorf("sqlite: multiple rows returned")
	}

	return row, rows.Err()
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// ============================================================================
// Transaction
// ============================================================================

// Tx wraps a database/sql transaction.
type Tx struct {
	tx        *sql.Tx
	sessionID string
	active    bool
}

// Exec executes a query within the transaction.
func (tx *Tx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := tx.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite tx: exec: %w", err)
	}
	return nil
}

// Query executes a query that returns rows within the transaction.
func (tx *Tx) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	rows, err := tx.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite tx: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// QueryRow executes a query that returns at most one row within the transaction.
func (tx *Tx) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := tx.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite tx: query row: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("sqlite tx: no rows in result")
	}

	row, err := scanSingleRow(rows)
	if err != nil {
		return nil, err
	}

	if rows.Next() {
		return nil, fmt.Errorf("sqlite tx: multiple rows returned")
	}

	return row, rows.Err()
}

// SetSessionContext stores the session ID for Go-layer RLS enforcement.
// SQLite doesn't have native RLS, so session-filtering happens in Go hooks.
func (tx *Tx) SetSessionContext(ctx context.Context, sessionID string) error {
	tx.sessionID = sessionID
	return nil
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	tx.active = false
	return tx.tx.Commit()
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error {
	tx.active = false
	return tx.tx.Rollback()
}

// IsActive returns true if the transaction has not been committed or rolled back.
func (tx *Tx) IsActive() bool {
	return tx.active
}

// ============================================================================
// Row scanning helpers
// ============================================================================

func scanRows(rows *sql.Rows) ([]db.Row, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("scan: columns: %w", err)
	}

	var results []db.Row
	for rows.Next() {
		row, err := scanRow(rows, columns)
		if err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func scanSingleRow(rows *sql.Rows) (db.Row, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("scan: columns: %w", err)
	}
	return scanRow(rows, columns)
}

func scanRow(rows *sql.Rows, columns []string) (db.Row, error) {
	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
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
	case int64:
		return val
	case float64:
		return val
	case bool:
		return val
	case nil:
		return nil
	default:
		return val
	}
}
