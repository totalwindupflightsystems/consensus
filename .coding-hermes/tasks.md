# Consensus — Model-Router Task Matrix

> **Core purpose:** Multi-backend (SQLite+Postgres) agentic memory runtime with Chronicle UI, OpenCode shim, and models.dev auto-sync.
> **Language:** Go | **CI:** GitHub Actions (green) | **Production:** Docker/ghcr.io

## Active

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| DEPS-002 | Bump 3 outdated direct Go deps (chi v5.2.5→5.3.1, pgx, sqlite) + 17 indirect | Low | 3 | — | +testing, +code-generation, ++terminal | **Step 3.7 Flash** | Mechanical dep bump + validation. Budget-optimized at $0.09/1M. | DeepSeek V4 Flash |
| TEST-001 | Tests for internal/modelsync (257 lines, 0 tests) | Medium | 4 | — | +++testing, ++code-generation, +database | **DeepSeek V4 Pro** | DB queries + provider mapping — needs debugging for test correctness. | GLM-5.2 |
| PERF-001 | Go benchmarks for hot paths (planning, compression, retrieval) | Low | 2 | — | ++testing, +performance, +code-generation | **Step 3.7 Flash** | Mechanical benchmark boilerplate. Budget-friendly. | DeepSeek V4 Flash |
| GAP-001 | Missing budget_limit_cents migration | Critical | 2 | — | +database, ++code-generation, +testing | **DeepSeek V4 Pro** | DB migration — data integrity risk. Must be correct. | GLM-5.2 |
| GAP-002 | TestAPIProxy_UpstreamError returns 404 instead of 502 | Medium | 3 | — | ++debugging, +testing, +code-generation | **DeepSeek V4 Pro** | HTTP error mapping debugging. Needs investigation. | GLM-5.2 |
| GAP-003 | TestAPIProxy_UpstreamError port conflict (Dagger Engine) | Medium | 2 | — | ++debugging, +testing, +infra | **DeepSeek V4 Flash** | Port conflict — simple fix, test-only. | DeepSeek V4 Pro |
| DEPS | Bump Go toolchain to 1.26.5 (stdlib vulns) | Medium | 1 | — | +code-generation, ++terminal | **DeepSeek V4 Flash** | Mechanical toolchain update. | Step 3.7 Flash |
| INFRA | Prepaid buckets health probe | Low | 1 | — | +infra, +api-use | **DeepSeek V4 Flash** | Simple health check. | Hy3 |
| BOOTSTRAP | Create .coding-hermes/tasks.md | Trivial | 1 | — | +documentation, ++file-editing | **Hy3** | File creation only. Cheapest. | DeepSeek V4 Flash |

## Completed

| ID | Task | Pri | Cpx | Commit | Model |
|----|------|-----|-----|--------|-------|
| PHASE1.1 | Chronicle design system CSS + Go package + route | High | 4 | — | DeepSeek V4 Pro |
| PHASE1.2 | Layout shell: app chrome, command palette (⌘K), responsive | High | 5 | — | DeepSeek V4 Pro |
| PHASE1.3 | Component library: Button → Skeleton (9 components) + Table/CodeBlock | High | 4 | — | DeepSeek V4 Pro |
| PHASE1.4 | Investigation Workbench: THINK/SAYS panes, Input, Evidence, Discovery, WebSocket | High | 6 | — | DeepSeek V4 Pro |
| PHASE1.5 | Overview Dashboard (6 KPI cards) + Status Dashboard (health.css) | High | 5 | — | DeepSeek V4 Pro |
| PHASE1.6 | Timeline Explorer (1,596 lines CSS) + Entity Graph (D3.js force-directed) | High | 6 | — | GPT-5.6 Sol |
| PHASE1.7 | Wire Chronicle UI to Consensus API + E2E real LLM test | Critical | 5 | PHASE1.1-1.6 | 0727588 | DeepSeek V4 Pro |
| PHASE2 | OpenCode shim: 26 endpoints verified, smoke test, OpenAPI docs | High | 5 | — | DeepSeek V4 Pro |
| PHASE2.Models | models.dev integration (syncer.go, CLI, auto-sync, auto-register) | High | 5 | 3135ffa | DeepSeek V4 Pro |
| PHASE3 | Docker image, quickstart, compose-prod, Postgres RLS fix (2 bugs) | High | 5 | — | GPT-5.6 Sol |
| PHASE3.SDK | Go SDK/client library (749 lines, 22 structs, 24 endpoints) | High | 4 | 876da30 | DeepSeek V4 Pro |
| PHASE4 | Hardened testing: provider failure, concurrency, budget, memory poisoning, migration under load, 100+ iteration | High | 6 | — | DeepSeek V4 Pro |
| PHASE5.AC-040 | Circuit breaker verification + wiring | Medium | 4 | — | DeepSeek V4 Pro |
| PHASE5.AC-053/4/5 | Undefined AC placeholders — CLOSED (never existed) | Low | 1 | — | DeepSeek V4 Flash |
| PHASE5.AC-056 | Postgres verification: RLS wiring gaps identified | Medium | 3 | — | DeepSeek V4 Pro |
| PHASE6 | Expand UI spec: §§8-24 (Search, Reports, Settings, Task Queue → Testing) — 2,561 lines | Medium | 3 | — | GPT-5.6 Terra |
| GitReins | Guards, 20 tasks, Tier 2 evaluator, 6 real bugs fixed | High | 4 | — | DeepSeek V4 Pro |
| CI/CD | 8 jobs green, 5-platform cross-compile, SQLite+Postgres dual-backend | High | 4 | — | Step 3.7 Flash |
| E2E | Real LLM tests, live demo, semantic retrieval, consensus-watchdog cron | High | 5 | — | DeepSeek V4 Pro |
| Admin | Admin UI (/ui/ route), DESIGN.md, diagrams.md, chronicle.html GH Pages | Medium | 3 | — | DeepSeek V4 Pro |

## Assumptions

- Dual Go backend (SQLite + Postgres) with `go build ./... && go test ./... -short && go vet ./...`
- GitReins Tier 1 guard (secrets, build, lint, tests) active
- Consensus serve + OpenCode shim both verified
- Phase 1-6 feature work is complete; active tasks are maintenance/gap closure

## Routing Notes

- Phases 1-6 are historical completions — documented for audit trail, not routable
- Active tasks primarily maintenance: Go deps, test coverage, gap closure
- Step 3.7 Flash for mechanical tasks ($0.09/1M) — deps, perf benchmarks
- DeepSeek V4 Pro for debugging (GAP-001/002) and testing (TEST-001)
- GPT-5.6 Sol for complex architecture (Entity Graph, Postgres RLS debugging)
- GPT-5.6 Terra for spec/documentation heavy lifting (Phase 6 UI spec)

## Execution Order

1. GAP-001 (migration — blocks everything if schema wrong)
2. DEPS-002, DEPS (toolchain — unblock other work)
3. GAP-003 (port conflict — simple, unblock test suite)
4. GAP-002 (error mapping — debugging)
5. TEST-001 (modelsync tests)
6. PERF-001 (benchmarks — low pri)
7. INFRA, BOOTSTRAP (trivial)

## Escalation Conditions

- DB migration fails tests → escalate to GPT-5.6 Sol (data integrity)
- modelsync tests expose provider mapping bugs → escalate to GLM-5.2
- Concurrency/race conditions found → escalate to GPT-5.6 Sol

---

## [ ] NEVER-DONE — Run coding-hermes-never-done 11-point audit

Load coding-hermes-never-done skill. Run ALL 11 checks: spec alignment, doc coverage, test gaps, package upgrades, pitfall hunt, performance audit, endpoint verification, CI/CD health, DuckBrain sync, code quality, middle-out wiring. Create a task for EVERY gap found. This task is never complete — the audit always finds something.

> **Last audit:** 2026-07-21T05:38Z — PRODUCTIVE TICK #4 (GAP-003 found+fixed). 30 specs, 28/28 pkgs pass, 14 benchmarks, 72 routes, CI 5/5 green. Idle counter reset to 0. Cooldown: 900s.
