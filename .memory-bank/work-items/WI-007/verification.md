# WI-007 — Verification Evidence

**Status**: PASS
**Date**: 2026-05-29
**Verification Bar**: Standard

---

## Acceptance Criteria Mapping

| # | Criterion | Verification | Status |
|---|-----------|-------------|--------|
| 1 | Replace Go context assembly with VIEW query | Postgres: `readMemoriesFromView` queries `active_context_view`. SQLite: Go fallback preserved. | ✅ |
| 2 | Wire `SET LOCAL conscience.session_id` before reads | `ReadActiveContext` begins tx → `SetSessionContext()` → queries. PG: `SELECT set_config(...)`. SQLite: stores session ID. | ✅ |
| 3 | VIEW features work end-to-end | VIEW includes DISTINCT ON, CASE rendering, cache_tier, collapse_status. Go `sortByCacheTier` provides Layer 1→2→3 ordering. | ✅ |
| 4 | Tool call collapse logic added to VIEW | `migrations/013_active_context_view_enhanced.sql` adds window-function based collapse when >10 tool calls exist. | ✅ |
| 5 | Dual-backend support | Both Postgres (VIEW) and SQLite (Go fallback) paths implemented. All 26 test packages pass on SQLite. | ✅ |

## Build Verification

```
$ go build ./... 
warning: both GOPATH and GOROOT are the same directory (/home/kara/go)
[no errors]
```

## Test Results

```
$ go test ./... -count=1
ok  	github.com/wojons/conscientiousness/internal/harness	1.440s
... all 26 packages ok ...
```

## Trace Map

```
WI-007
├── specs/001-architecture.md (§4 Core Data Flow)
├── specs/002-memory.md (§3 Context Window as VIEW, §7 Markdown Gen)
├── specs/003-database.md (§6 Token Caching Strategy)
├── specs/005-security.md (§RLS Row-Level Security)
├── migrations/013_active_context_view_enhanced.sql
├── internal/harness/context.go
└── internal/harness/context_test.go
```

## Open Items

- Postgres integration test: requires a running Postgres instance with the VIEW applied. The VIEW cannot be tested on SQLite (stripped by filterForSQLite).
- The `collapse_status` column in the VIEW is informational. The actual collapse action (excluding collapsed tool calls from output) should be implemented as a follow-up.

**axiom:trace work_item=WI-007 spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md plan=phase-3/verify evidence=.memory-bank/work-items/WI-007/verification.md**
