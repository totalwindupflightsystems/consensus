# WI-007 — Wire active_context_view as Actual Context Source

**Gap**: CS-GAP-006 (HIGH) — Harness bypasses `active_context_view` VIEW
**Specs**: 001 (§4 Data Flow), 002 (§3, §7), 003 (§6), 005 (§RLS)
**Status**: Planning

---

## Meta-Plan

### Why This Matters

The `active_context_view` already exists in migration 001 and includes:
- `DISTINCT ON` deduplication (SPEC-002 §3.6)
- `CASE` rendering for display_modes (compressed/hidden/full) (SPEC-002 §3.2)
- `current_setting('consensus.session_id')` for RLS isolation (SPEC-005)

But the harness **never queries it**. Instead, `readMemoryEvents()` in `context.go:436` manually joins `memory_events` + `display_modes` in Go. The VIEW adds:
- Tool call collapse via window functions (SPEC-002 §3.5) — not yet in the VIEW
- Cache tier ordering (SPEC-003 §6.2) — not yet in the VIEW
- `DISTINCT ON` dedup (SPEC-002 §3.6) — already in VIEW
- RLS integration via `SET LOCAL` — already in VIEW

### Dual-Backend Strategy

| Backend | Context Source | RLS Method |
|---------|---------------|------------|
| **Postgres** | `active_context_view` (migration 013 enhanced) | `SET LOCAL consensus.session_id` via `SetSessionContext()` |
| **SQLite** | Go-level query (existing `readMemoryEvents`) | Go-layer session_id WHERE clause |

The migration 013 enhances the VIEW for Postgres and includes SQLite-strippable operators. SQLite views are entirely stripped by `filterForSQLite()` in `migrate.go` because SQLite doesn't support `current_setting`, RLS, or PG-specific functions.

### Cache Tier Ordering (SPEC-003 §6.2)

The Markdown output must order content by cache tier for maximum prompt caching:
- **Layer 1** (cache_tier=1): Static system content — headers, system messages
- **Layer 2** (cache_tier=2): Immutable ledger — older memory events (>5 iterations ago)
- **Layer 3** (cache_tier=3): Dynamic content — recent events, tool results

This ordering is applied in Go after reading from either the VIEW or the raw table.

### Type-to-Markdown Mapping (SPEC-002 §7.2)

| `type` value | Current format | Target format |
|---|---|---|
| `header` | `[header \| iter N] content` | `## content` |
| `text_block` | `[text_block \| iter N] content` | `content` (paragraph block) |
| `tool_call` | `[tool_call \| iter N] content` | `**Tool Call:** content` |
| `tool_result` | `[tool_result \| iter N] content` | `> **Result:** content` |
| `thinking` | `[thinking \| iter N] content` | `<!-- thinking: content -->` |
| `system` | `[system \| iter N] content` | `***content***` |

---

## Phases

### Phase 1: Migration 013 — Enhanced active_context_view
- Drop old view, recreate with:
  - Tool call collapse window function (SPEC-002 §3.5)
  - Cache tier column (SPEC-003 §6.2.3)
  - Page resolution UNION (SPEC-002 §5.2)
  - `DISTINCT ON` dedup (SPEC-002 §3.6)

### Phase 2: Refactor harness/context.go
- Wire `SetSessionContext` in `ReadActiveContext` via transaction
- Backend-aware `readMemoryEvents`: VIEW on Postgres, Go fallback on SQLite
- Cache tier ordering in Go
- Type-to-Markdown formatting per SPEC-002 §7.2

### Phase 3: Test
- Existing tests must pass on SQLite (backward compatible)
- Verify VIEW features in Postgres (manual or via integration test if PG available)
- `go test ./...` must pass

### Phase 4: Commit
- Conventional commit with Co-authored-by trailer

---

**axiom:trace work_item=WI-007 spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md plan=meta-planning**
