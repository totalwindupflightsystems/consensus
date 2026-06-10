# WI-002: Migrate lib/pq → pgx/v5 — Verification

## Phase 1: Dependency + Connection Pool Migration ✅

### Phase 1 Task 1: Add pgx/v5, remove lib/pq
- Added `github.com/jackc/pgx/v5` and `github.com/jackc/pgx/v5/pgxpool` to `go.mod`
- Removed `github.com/lib/pq` from `go.mod`
- `go mod tidy` ✅
- **Evidence**: `grep pgx go.mod` → present; `grep lib/pq go.mod` → absent

### Phase 1 Task 2: Rewrite postgres.go with pgxpool
- Replaced `database/sql` + `lib/pq` with `pgx/v5/pgxpool`
- `DB` struct: `conn *sql.DB` → `pool *pgxpool.Pool`
- `Open()`: uses `pgxpool.NewWithConfig()` instead of `sql.Open()`
- `Tx` struct: `tx *sql.Tx` → `tx pgx.Tx`
- `BeginTx()`: uses `pool.Begin()` instead of `conn.BeginTx()`
- `Exec/Query/QueryRow`: uses pgx pool methods
- Updated scan helpers for `pgx.Rows` (uses `rows.Values()` instead of `rows.Scan()`)
- `Commit/Rollback`: uses `context.Background()` (pgx requires context, db.Tx interface doesn't pass it)
- `go build ./...` ✅
- `go test ./...` ✅

## Phase 2: FOR UPDATE SKIP LOCKED ✅

### Phase 2 Task 1: Add backend-aware locking
- Updated `ClaimNextReadyTask` in `executor.go`
- Added `db` import for backend type detection
- Postgres: appends `FOR UPDATE SKIP LOCKED` inside subquery
- SQLite: keeps existing behavior (no SKIP LOCKED needed)
- `go build ./...` ✅
- `go test ./internal/harness/...` ✅ (all 100+ tests pass)

## Phase 3: Postgres LISTEN/NOTIFY ✅

### Phase 3 Task 1: Notification listener
- Created `internal/db/postgres/notify.go`
- `NotificationListener` acquires dedicated connection from pool
- Supports multiple channels, handler callback, graceful shutdown
- Safe quoting of channel identifiers
- `go build ./...` ✅

## Summary
- **3 tasks completed**
- **lib/pq fully removed from go.mod**
- **pgx/v5 with connection pool in place**
- **FOR UPDATE SKIP LOCKED on Postgres**
- **LISTEN/NOTIFY listener ready for EventBus wiring**
- All 22+ package tests pass

axiom:trace work_item=WI-002-migrate-pgx spec=specs/009-deployment.md,specs/015-api-and-mcp.md,specs/022-library-research.md plan=phases-1-2-3 evidence=verification.md
