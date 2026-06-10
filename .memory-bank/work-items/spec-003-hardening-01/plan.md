---
work_item_id: spec-003-hardening-01
status: completed
repo: wojons/conscientiousness
created: 2026-05-05
spec_refs:
  - specs/003-database.md
  - specs/006-transactions.md
  - specs/011-canonical-definitions.md
source_sweep: sweep-007
---

# Plan — SPEC-003 Database Hardening (Gaps from Sweep-007)

Remediate the 3 executable gaps found during the idle conformance sweep of
`specs/003-database.md`.

axiom:trace work_item=spec-003-hardening-01 spec=specs/003-database.md,specs/006-transactions.md,specs/011-canonical-definitions.md plan=sweep-007-remediation

## Gap 1 (HIGH): agent_budget_limits migration

No migration file exists for the `agent_budget_limits` table defined in SPEC-006 §Budget Limits.

**Fix:** Create `005_agent_budget_limits.sql` with the canonical schema from SPEC-006.

## Gap 2 (HIGH): create_agent_memory_table() — not executable

The `create_agent_memory_table()` function is only documented in the migration
file's comment section; there is no runtime implementation. Agents cannot
dynamically create tables.

**Fix:** Create `internal/db/dynamic/` package with:
- `CreateTable(ctx, db, tableName, sessionID)` — Go-level equivalent
- Name validation (lowercase alphanumeric + underscore, 1-63 chars)
- Reserved name blocklist (31 reserved names from SPEC-003 §3.2)
- Creates table with system columns: id, session_id, iteration_created,
  deleted_at, linked_memory_pages, data (JSONB), created_at, updated_at
- Session isolation enforcement (Postgres: RLS policies; SQLite: Go hooks)
- Soft-delete trigger registration
- updated_at auto-update trigger

## Gap 3 (HIGH): soft_delete_intercept() — not executable

The `soft_delete_intercept()` trigger function is only documented. DELETE
operations are not intercepted and converted to soft deletes at runtime.

**Fix:** Implement `soft_delete_intercept()` as a Go-level stored procedure
that converts `DELETE FROM` on dynamic tables to `UPDATE ... SET deleted_at = now()`.
This runs through the normal db.DB interface and works on both backends.

The LLM emits DELETE statements normally; the harness replaces them with
soft-delete UPDATEs if the target is a dynamic table with a `deleted_at` column.

## Implementation Phases

### Phase 1: Migration 005 — agent_budget_limits
- Create `migrations/005_agent_budget_limits.sql`
- Sync to `internal/migrate/migrations/005_agent_budget_limits.sql`

### Phase 2: Migration 006 — dynamic entity generator (Postgres)
- Create `migrations/006_dynamic_entity_generator.sql` with the full
  `create_agent_memory_table()` PL/pgSQL function and `soft_delete_intercept()` trigger

### Phase 3: Go package — internal/db/dynamic
- Create package with `CreateTable()`, `SoftDeleteIntercept()`, `IsReservedName()`
- Test with SQLite (in-memory)

### Phase 4: Integration
- Wire into circuit breaker / staging buffer tests
- Verify build + test pass
