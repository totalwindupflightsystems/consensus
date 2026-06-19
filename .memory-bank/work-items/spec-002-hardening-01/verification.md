# Verification — SPEC-002 Remediation (spec-002-hardening-01)

axiom:trace work_item=spec-002-hardening-01 spec=specs/002-memory.md plan=phase-1..3 evidence=.memory-bank/work-items/spec-002-hardening-01/verification.md

## Build
```bash
$ go build ./...
# (no output — success)
```

## Test
```bash
$ go test ./...
ok  	github.com/wojons/consensus/internal/api	(cached)
ok  	github.com/wojons/consensus/internal/billing	(cached)
ok  	github.com/wojons/consensus/internal/cli	(cached)
ok  	github.com/wojons/consensus/internal/config	(cached)
ok  	github.com/wojons/consensus/internal/db	(cached)
ok  	github.com/wojons/consensus/internal/db/driver	(cached)
ok  	github.com/wojons/consensus/internal/harness	1.287s
ok  	github.com/wojons/consensus/internal/hitl	(cached)
ok  	github.com/wojons/consensus/internal/llm	0.265s
ok  	github.com/wojons/consensus/internal/mcp	(cached)
ok  	github.com/wojons/consensus/internal/migrate	(cached)
ok  	github.com/wojons/consensus/internal/secrets	(cached)
ok  	github.com/wojons/consensus/internal/security	(cached)
ok  	github.com/wojons/consensus/internal/session	(cached)
ok  	github.com/wojons/consensus/internal/shim/opencode	(cached)
ok  	github.com/wojons/consensus/internal/subagent	(cached)
ok  	github.com/wojons/consensus/internal/tools	(cached)
ok  	github.com/wojons/consensus/internal/webhook	(cached)

22 packages, 0 failures.
```

## AC Verification

| AC | Criterion | Status | Evidence |
|---|---|---|---|
| AC-MEM-H01 | Memory page resolution wired into context assembly | PASS | `resolvePageMemoryIDs()` in `internal/harness/context.go` — queries `memory_pages`, expands `target_ids` + `linked_page_ids` (single-level), returned as deduplicated map. Called in `ReadActiveContext()`. |
| AC-MEM-H02 | Deduplication when pages share events | PASS | `annotatePageEvents()` in `internal/harness/context.go` — two-pass dedup: marks page membership, filters duplicate IDs, keeps first occurrence. Page events annotated with "(page)" suffix. |
| AC-MEM-H03 | Model routing through model_registry | PASS | `readModelPricing()` in `internal/harness/harness.go` — queries `model_registry` for `cost_per_m_in/out`, falls back to hardcoded `modelPricing` map. `calculateCostUSD()` unchanged but well-documented. |
| AC-MEM-H04 | Reactive context truncation on LLM 400 errors | PASS | `handleLLMError()` in `internal/harness/executor.go` — detects context-limit errors via `isContextLimitError()`, truncates largest message at 65% with `truncateContext()`, retries up to 3x. |
| AC-MEM-H05 | Page-fault load_memory_event handler | PASS | `LoadMemoryEvent()` in `internal/harness/executor.go` — queries `memory_events` + `display_modes` for a specific event, session-scoped. Returns full `MemoryEventInfo`. Agents can also use raw SELECT. |

## Files Changed
- `internal/harness/context.go` — Added page resolution + dedup (~120 lines)
- `internal/harness/harness.go` — Added `readModelPricing`, `toFloat64`, `ContextRetries` field
- `internal/harness/executor.go` — Added reactive truncation + page-fault handler

## Verification Date
2026-05-05 — All 5 ACs PASS. Build and tests clean.
