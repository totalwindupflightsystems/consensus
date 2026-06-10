# Finding: SPEC-002 Memory Specification Gaps

**Type:** spec-conformance
**Severity:** medium
**Date:** 2026-05-05
**Source:** Idle sweep-006 against `specs/002-memory.md`
**Status:** remediated — `spec-002-hardening-01` complete (5/5 ACs PASS)

## Summary

Sweep-006 audited `specs/002-memory.md` (Cognitive Memory Engine) against the Go codebase. Found 8 gaps across 5 severity levels. 5 portable gaps were remediated; 3 PostgreSQL-dependent features deferred.

## Gaps Found

| Gap | Spec § | Severity | Fix |
|---|---|---|---|
| No memory page resolution in active context view | §5.4 | MEDIUM | `resolvePageMemoryIDs()` in harness context reader |
| No deduplication when pages share events | §3.6 | LOW | `annotatePageEvents()` two-pass dedup |
| Hardcoded model pricing, not routed through model_registry | §9.4 | MEDIUM | `readModelPricing()` with DB query + fallback |
| No reactive context truncation on LLM 400 errors | §11 | HIGH | `handleLLMError()` truncate+retry cycle |
| No explicit page-fault handler | §4.3 | LOW | `LoadMemoryEvent()` convenience function |

## Deferred (PostgreSQL-dependent)

- §8 compression loop (requires pgvector, pg_cron, embedding pipeline)
- §3.5 tool call collapse (requires FILTER window functions)
- §7.3 string_agg Markdown aggregation (Go formatting is equivalent)

## Files Changed
- `internal/harness/context.go` — page resolution + dedup
- `internal/harness/harness.go` — model routing + ContextRetries field
- `internal/harness/executor.go` — reactive truncation + page-fault handler

## Verification
`go build ./...` succeeds, `go test ./...` passes (22 packages, 0 failures).
All existing tests continue to pass — no regressions.

## Pattern
This is the second idle sweep that found gaps (sweep-003 against SPEC-006 was first). The pattern suggests:
- The codebase implements spec-required behavior through practical paths (API endpoints, harness loops) rather than literal spec translations
- View-based features (active_context_view) are partially emulated in Go for SQLite portability
- PostgreSQL-only features are correctly deferred

axiom:trace work_item=spec-002-hardening-01 spec=specs/002-memory.md sweep=sweep-006 finding=spec-memory-gaps
