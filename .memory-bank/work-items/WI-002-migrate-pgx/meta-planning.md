# WI-002: Migrate lib/pq → pgx/v5 — Meta-Planning

**Operator Brief**: The PostgreSQL backend uses `database/sql` with the `lib/pq` driver. This lacks connection pooling, `LISTEN/NOTIFY` support, and `FOR UPDATE SKIP LOCKED` via the driver API. We need to migrate to `pgx/v5` which is the modern Go PostgreSQL driver with native connection pooling, pub/sub support, and better performance.

**Specs**: 009 (Deployment), 015 (API & MCP), 022 (Library Research)

**Gaps**: CS-GAP-012, CS-GAP-013, CS-GAP-015

**Verification Bar**: Standard (production-blocker fix)

---

## Scope Fences

**In scope**:
- Replace `github.com/lib/pq` with `github.com/jackc/pgx/v5` in `go.mod`
- Rewrite `internal/db/postgres/postgres.go` to use `pgx/v5` connection pool
- Implement `FOR UPDATE SKIP LOCKED` in `ClaimNextReadyTask` for Postgres
- Wire Postgres `LISTEN/NOTIFY` to feed the `EventBus` for real-time SSE events
- Update SPEC-022 to reflect pgx/v5 decision

**Out of scope**:
- SQLite backend changes (SQLite stays on `modernc.org/sqlite`)
- Removing `lib/pq` from vendor (no vendor dir)
- Full SSE streaming integration (just the pgx LISTEN/NOTIFY → EventBus bridge)
- `goose/v3` migration runner migration

---

## Acceptance Criteria

| # | Criterion | Verification Path |
|---|-----------|-------------------|
| AC1 | `go.mod` uses `github.com/jackc/pgx/v5` instead of `github.com/lib/pq` | `grep pgx go.mod` |
| AC2 | `postgres.go` uses `pgx/v5/pgxpool` connection pool | `go build ./...` |
| AC3 | `ClaimNextReadyTask` uses `FOR UPDATE SKIP LOCKED` syntax for Postgres | Inspect `executor.go` |
| AC4 | Postgres LISTEN/NOTIFY is wired into EventBus | Inspect `postgres.go` or new file for notification listener |
| AC5 | SPEC-022 updated to reflect pgx choice | Inspect `specs/022-library-research.md` |
| AC6 | All existing tests pass (those using SQLite) | `go test ./...` |

---

## Decision Points

| Decision | Options | Chosen | Rationale |
|----------|---------|--------|-----------|
| pgx pool vs simple connection | a) `pgxpool`, b) `pgx` direct | **pgxpool** | Connection pooling is the main driver for migration |
| DB interface changes | a) keep `database/sql` interface, b) expose pgx types | **Keep DB interface** | Interface stays the same; only backend impl changes |
| LISTEN/NOTIFY integration | a) in postgres.go, b) separate file | **Separate file** | Separation of concerns; notification listener is orthogonal to DB ops |

---

## Assumptions

| # | Statement | How to Verify | Impact if Wrong |
|---|-----------|---------------|-----------------|
| A1 | pgx/v5 can replace lib/pq without changing `db.DB` interface | `database/sql`-style Query/Exec patterns are supported by pgx | May need minor interface changes |
| A2 | `pgxpool` provides `Exec`/`Query` that match current usage | Review pgx docs | May need adapter layer |

---

## Open Questions

None — well-defined migration with clear outcomes.

---

axiom:trace work_item=WI-002-migrate-pgx spec=specs/009-deployment.md,specs/015-api-and-mcp.md,specs/022-library-research.md plan=meta-planning.md
