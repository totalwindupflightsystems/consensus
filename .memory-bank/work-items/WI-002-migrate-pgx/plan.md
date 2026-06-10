# WI-002: Migrate lib/pq → pgx/v5 — Plan

**Mission**: Replace the legacy `lib/pq` PostgreSQL driver with modern `pgx/v5` to gain connection pooling, `LISTEN/NOTIFY` for real-time events, and `FOR UPDATE SKIP LOCKED` for concurrent task claiming.

## Acceptance Criteria → Verification

| AC | How to Verify |
|----|---------------|
| AC1: pgx/v5 in go.mod | `grep pgx go.mod` |
| AC2: postgres.go uses pgxpool | `go build ./...` |
| AC3: FOR UPDATE SKIP LOCKED | Inspect executor.go ClaimNextReadyTask |
| AC4: LISTEN/NOTIFY wired to EventBus | Inspect postgres notification listener |
| AC5: SPEC-022 updated | Inspect specs/022-library-research.md |
| AC6: All tests pass | `go test ./...` |

## Phases

### Phase 1: Dependency + Connection Pool
1. Add `pgx/v5` + `pgxpool` to `go.mod`, remove `lib/pq`
2. Rewrite `postgres.go` — use `pgxpool` instead of `database/sql`
3. Update scanning helpers for pgx row types

### Phase 2: FOR UPDATE SKIP LOCKED
1. Add backend detection in `ClaimNextReadyTask`
2. Use `FOR UPDATE SKIP LOCKED` for Postgres, keep existing for SQLite

### Phase 3: LISTEN/NOTIFY → EventBus Bridge
1. Create notification listener that subscribes to Postgres channels
2. Wire into the harness bridge to push to EventBus

### Phase 4: SPEC-022 Update + Verify
1. Update the library research doc
2. `go build ./... && go test ./...`

## Rollback
- `git checkout -- go.mod go.sum` to revert dependency
- Phase 2 (SKIP LOCKED) is only for Postgres; SQLite unaffected
- Phase 3 (LISTEN/NOTIFY) is additive; no-op if not running Postgres

axiom:trace work_item=WI-002-migrate-pgx spec=specs/009-deployment.md,specs/015-api-and-mcp.md,specs/022-library-research.md plan=plan.md
