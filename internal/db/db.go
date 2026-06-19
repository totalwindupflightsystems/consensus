// Package db defines the database driver interface for Consensus.
//
// All other packages depend on this interface — never on a specific driver.
// This is the single architectural seam that enables dual-backend support:
// PostgreSQL (via pgx/v5) and SQLite (via modernc.org/sqlite).
//
// To open a database, use db/driver.Open() which wires both backends.
//
// axiom:trace work_item=repo-bootstrap-01 spec=specs/001-architecture.md,specs/021-repository-layout.md plan=phase-1/task-1/step-3
package db

import (
	"context"
	"fmt"
)

// DB is the top-level database connection interface.
// It provides transaction creation, direct execution, and lifecycle management.
type DB interface {
	// BeginTx starts a new transaction with the default isolation level.
	BeginTx(ctx context.Context) (Tx, error)

	// Exec executes a query without returning rows.
	Exec(ctx context.Context, query string, args ...any) error

	// Query executes a query that returns rows.
	Query(ctx context.Context, query string, args ...any) ([]Row, error)

	// QueryRow executes a query that returns at most one row.
	QueryRow(ctx context.Context, query string, args ...any) (Row, error)

	// Backend returns which database engine is in use (postgres or sqlite).
	Backend() Backend

	// Close closes the database connection.
	Close() error
}

// Tx represents a database transaction.
// Transactions carry session context for Row-Level Security enforcement.
type Tx interface {
	// Exec executes a query within the transaction.
	Exec(ctx context.Context, query string, args ...any) error

	// Query executes a query that returns rows within the transaction.
	Query(ctx context.Context, query string, args ...any) ([]Row, error)

	// QueryRow executes a query that returns at most one row within the transaction.
	QueryRow(ctx context.Context, query string, args ...any) (Row, error)

	// SetSessionContext sets the session identity for RLS isolation.
	// On Postgres this issues SET LOCAL consensus.session_id.
	// On SQLite this stores the session ID for Go-layer enforcement.
	SetSessionContext(ctx context.Context, sessionID string) error

	// Commit commits the transaction.
	Commit() error

	// Rollback aborts the transaction.
	Rollback() error

	// IsActive returns true if the transaction has not been committed or rolled back.
	IsActive() bool
}

// Row is a single row result from a query.
type Row map[string]any

// Config holds database connection configuration.
type Config struct {
	// URL is the database connection string.
	// Postgres: postgres://user:pass@host:port/dbname
	// SQLite:   sqlite://path/to/db/file
	URL string `yaml:"url" json:"url"`

	// MaxOpenConns is the maximum number of open connections (Postgres only).
	MaxOpenConns int `yaml:"max_open_conns" json:"max_open_conns"`

	// MaxIdleConns is the maximum number of idle connections (Postgres only).
	MaxIdleConns int `yaml:"max_idle_conns" json:"max_idle_conns"`
}

// Backend identifies which database backend is in use.
type Backend string

const (
	BackendPostgres Backend = "postgres"
	BackendSQLite   Backend = "sqlite"
)

// DetectBackend returns the database backend from a connection URL.
func DetectBackend(url string) (Backend, error) {
	if len(url) < 9 {
		return "", fmt.Errorf("db: invalid database URL: %s", url)
	}
	switch url[:9] {
	case "postgres:":
		return BackendPostgres, nil
	case "postgresq":
		return BackendPostgres, nil
	case "sqlite://":
		return BackendSQLite, nil
	default:
		return "", fmt.Errorf("db: unsupported database URL scheme in %q (expected postgres:// or sqlite://)", url)
	}
}

// DetectBackendFromDB returns the backend type from a DB instance.
// This is preferred over DetectBackend() when you already have a connection.
func DetectBackendFromDB(database DB) Backend {
	if database == nil {
		return BackendSQLite // safest default
	}
	return database.Backend()
}
