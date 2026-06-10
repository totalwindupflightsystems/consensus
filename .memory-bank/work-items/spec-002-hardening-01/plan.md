---
work_item_id: spec-002-hardening-01
status: active
repo: wojons/conscientiousness
created: 2026-05-05
reason: Idle sweep-006 against specs/002-memory.md found 8 gaps (2 HIGH, 4 MEDIUM, 2 LOW)
---

# Plan — SPEC-002 Remediation

Fix portable memory specification gaps found in idle sweep-006. PostgreSQL-only features
(compression loop §8, tool call collapse §3.5, string_agg §7.3) are deferred —
they require pgvector/pg_cron which is not the current deployment target.

This work item focuses on fixes that work on BOTH SQLite and Postgres backends.

axiom:trace work_item=spec-002-hardening-01 spec=specs/002-memory.md plan=phase-1/task-1 evidence=.memory-bank/work-items/spec-002-hardening-01/verification.md

## AC → Verification Mapping

| AC | Gap | Fix | Verification Path | Status |
|---|---|---|---|---|
| AC-MEM-H01 | §5.4 Page resolution not in active view | Wire `memory_page` resolution into context assembly so pages automatically expand | Go test: create page, verify context includes page events | PENDING |
| AC-MEM-H02 | §3.6 No deduplication | Add `DISTINCT` or Go-layer dedup when multiple pages share events | Go test: two pages, one shared event → context shows it once | PENDING |
| AC-MEM-H03 | §9.4 Hardcoded model pricing | Read from `model_registry` table instead of hardcoded map; fall back to map | Go test: query model_registry, verify harness uses DB data | PENDING |
| AC-MEM-H04 | §11 Reactive truncation not implemented | Catch 400 context-limit from LLM API, truncate last large event, inject instructions | Go test: mock LLM returns 400, verify harness truncates and retries | PENDING |
| AC-MEM-H05 | §4.3 No explicit page-fault handler | Add `load_memory_event(id)` harness helper exposed as internal tool | Go test: call load_memory_event, verify result injected into context | PENDING |

## Phases

### Phase 1: Context Assembly Fixes (AC-MEM-H01, AC-MEM-H02)
- Wire `memory_pages` resolution into `api/memory.go` active context handler
- Add deduplication when pages overlap
- Files: `internal/api/memory.go`, `internal/api/memory_test.go`

### Phase 2: Model Routing (AC-MEM-H03)
- Replace hardcoded `modelPricing` map with `model_registry` table queries
- Add fallback pricing for unknown models
- Files: `internal/harness/harness.go`, `internal/harness/harness_test.go`

### Phase 3: Reactive Truncation (AC-MEM-H04)
- Add error catcher in LLM client for 400/context-exceeded
- Truncate the largest event, inject instructions
- Retry with truncated context
- Files: `internal/llm/client.go`, `internal/llm/client_test.go`, or `internal/harness/harness.go`

### Phase 4: Page-Fault Handler (AC-MEM-H05)
- Add `load_memory_event` tool/handler to harness
- Agent can issue SQL via MemoryStateChanges to load full content
- Files: `internal/harness/context.go` (or new), `internal/harness/context_test.go`

## Commit Message Template

```
fix(memory): remediate spec-002 portable gaps from sweep-006

- Wire memory_pages resolution into context assembly (AC-MEM-H01)
- Add deduplication for overlapping page events (AC-MEM-H02)
- Route model selection through model_registry table (AC-MEM-H03)
- Add reactive context truncation on 400 limit errors (AC-MEM-H04)
- Add load_memory_event page-fault handler (AC-MEM-H05)

axiom:trace work_item=spec-002-hardening-01 spec=specs/002-memory.md plan=phase-1..4

Co-authored-by: Axiom Agent <svc_axiom@dexdat.ai>
```

## Deferred (PostgreSQL-dependent, not implementable on SQLite)

- §8 compression loop → requires pgvector, pg_cron, embedding pipeline
- §3.5 tool call collapse → uses FILTER window functions
- §7.3 string_agg() Markdown aggregation → Go formatting is equivalent
- §9.5 user-configurable routing rules → follow-up feature
