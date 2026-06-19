# WI-007 — Execution Plan

## Summary
Wire `active_context_view` as the actual context source for the harness, replacing Go-level SQL assembly. Add tool call collapse logic to the VIEW. Wire `SET LOCAL consensus.session_id` for RLS enforcement.

## Phases

### Phase 1: Migration 013 ✅
- File: `migrations/013_active_context_view_enhanced.sql`
- Enhanced VIEW with:
  - Page resolution (iter_commits + memory_pages union)
  - DISTINCT ON deduplication
  - Display mode CASE rendering
  - Tool call collapse window functions
  - Cache tier column (Layer 1/2/3)

### Phase 2: Refactor context.go ✅
- `ReadActiveContext` now uses transaction + SetSessionContext for RLS
- Backend-aware: VIEW on Postgres, Go fallback on SQLite
- New methods: `readMemoriesFromView`, `readSessionTx`, `readMemoryEventsTx`, `resolvePageMemoryIDsTx`, `readToolsTx`
- Cache tier sorting via `sortByCacheTier`
- `MemoryEventInfo` now has CacheTier and CollapseStatus fields

### Phase 3: Verification ✅
- `go test ./...` — all 26 packages pass
- `go build ./...` — clean build
- Migration SQL is valid Postgres

## Backend Behavior

| Operation | Postgres | SQLite |
|-----------|----------|--------|
| RLS context | `SET LOCAL consensus.session_id` via `SetSessionContext()` | Store sessionID in Tx struct |
| Memory events | Query `active_context_view` (enhanced VIEW) | Manual `SELECT ... LEFT JOIN display_modes` |
| Page resolution | VIEW CTE (all_ids union) | Go `resolvePageMemoryIDsTx` |
| Dedup | `DISTINCT ON` in VIEW | Go `annotatePageEvents` |
| Tool call collapse | Window function in VIEW | Not supported (N/A) |
| Cache tier | `cache_tier` column from VIEW | Go `sortByCacheTier` (tier=0, no-op) |

## Trace Map

```
Work Item WI-007
  → Specs: 001 (§4), 002 (§3,§7), 003 (§6), 005 (§RLS)
  → Migration: 013_active_context_view_enhanced.sql
  → Implementation: internal/harness/context.go
  → Tests: internal/harness/context_test.go (+ full suite)
  → Evidence: .memory-bank/work-items/WI-007/verification.md
```

**axiom:trace work_item=WI-007 spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md plan=phase-2/task-1 impl=internal/harness/context.go**
