// Package driver: database factory and dispatch.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/001-architecture.md plan=phase-1/task-1-1/step-1-1-2
package driver

import (
	"context"
	"fmt"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/postgres"
	"github.com/wojons/consensus/internal/db/sqlite"
)

// Open creates a database connection based on the configuration URL.
// It detects the backend from the URL scheme and initializes the
// appropriate driver.
func Open(ctx context.Context, cfg db.Config) (db.DB, error) {
	backend, err := db.DetectBackend(cfg.URL)
	if err != nil {
		return nil, err
	}
	switch backend {
	case db.BackendPostgres:
		return openPostgres(ctx, cfg)
	case db.BackendSQLite:
		return openSQLite(ctx, cfg)
	default:
		return nil, fmt.Errorf("driver: unknown backend: %s", backend)
	}
}

func openPostgres(ctx context.Context, cfg db.Config) (db.DB, error) {
	pgDB, err := postgres.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("driver: postgres backend failed: %w", err)
	}
	return pgDB, nil
}

func openSQLite(ctx context.Context, cfg db.Config) (db.DB, error) {
	sqDB, err := sqlite.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("driver: sqlite backend failed: %w", err)
	}
	return sqDB, nil
}
