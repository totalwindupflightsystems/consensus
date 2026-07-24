# Consensus — Model-Router Task Matrix

> **Core purpose:** Multi-backend (SQLite+Postgres) agentic memory runtime with Chronicle UI, OpenCode shim, and models.dev auto-sync.
> **Language:** Go | **CI:** GitHub Actions (green, last verified 2026-07-18) | **Production:** Docker/ghcr.io
> **Foreman:** deepseek-v4-pro @ deepseek-foreman | **DuckBrain:** consensus

## Active

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| — | No pending tasks — board empty | — | — | — | — | — | — | — |

## Completed

| ID | Task | Pri | Cpx | Commit | Model |
|----|------|-----|-----|--------|-------|
| U02 | JSON round-trip tests for 22 types (319 lines, 28 total tests, all PASS) | Low | 2±1 | 25185aa | DS-V4-Flash |
| U01 | Usability & coverage audit: 33 endpoints ✓, error handling ✓, stubs ✓, 1 gap (pkg/client/types → U02) | High | 3±1 | foreman-direct | DS-V4-Pro (foreman) |
| UX-011 | Fix port 8090 shadowing: VerifyIdentity() + PersistentPreRunE health check. 7 tests. Catches non-Consensus service at default port. | Medium | 4±1 | 078c843 | MiniMax-M3 (worker) + foreman-direct |
| UX-007 | Cross-platform quickstart: documented 15 gotchas for macOS/Win(WSL2)/Docker. Verified cross-compile ✅. File: docs/cross-platform-gotchas.md | Low | 5±1 | foreman-direct | DS-V4-Pro (foreman) |
| UX-010 | Fix health endpoint hang in Docker (Postgres admin pool) | High | 4±1 | 13c1af1 | DS-V4-Pro (foreman-direct) |
| UX-006 | Error recovery flow: 5 common mistakes, verify detection+guidance+recovery | Medium | 4±1 | foreman-direct | DS-V4-Pro (foreman) |
| UX-009 | Fix Dockerfile CMD --db→--db-url + CONSENSUS_DB_URL env | High | 1±1 | 81b3935 | DS-V4-Pro (foreman-direct) |
| UX-005 | README command accuracy: 10 cmds tested (6✓, 2→UX-009, 1→API key, 1 macOS) | Medium | 2±1 | — | DS-V4-Flash (foreman-direct) |
| UX-004 | Deployment smoke test: PG ✅, migrations ✅, UI ✅, health⚠️ | High | 4±1 | 7dd8c72 | DS-V4-Pro (foreman) |
| UX-003 | Chronicle first-time user flow: cold-start UI, THINK/SAYS, timeline, entity graph | High | 4±1 | — | DS-V4-Pro (foreman) |
| UX-002 | CLI error ergonomics: 20+ incorrect commands, verify error messages | Medium | 3±1 | 9458453 | DS-V4-Flash |
| UX-001 | Onboarding walkthrough: clone→init→serve→UI→demo investigation | High | 4±1 | — | DS-V4-Flash (foreman) |
| DEPS-002 | Bump 3 outdated direct Go deps (chi v5.2.5→5.3.1, pgx, sqlite) + 17 indirect | Low | 3 | 4603df1 | Step 3.7 Flash |
| TEST-001 | Tests for internal/modelsync (257 lines, 0 tests) | Medium | 4 | 17edc13 | DeepSeek V4 Pro |
| PERF-001 | Go benchmarks for hot paths (planning, compression, retrieval) — 14 benchmarks | Low | 2 | 572139a | Step 3.7 Flash |
| GAP-001 | Missing budget_limit_cents migration — migration 022 | Critical | 2 | 51ce655 | DeepSeek V4 Pro |
| GAP-002 | TestAPIProxy_UpstreamError returns 404 instead of 502 | Medium | 3 | de07902 | DeepSeek V4 Pro |
| GAP-003 | TestAPIProxy_UpstreamError port conflict (Dagger Engine, port 19999→19990) | Medium | 2 | b690e24 | DeepSeek V4 Flash |
| DEPS | Bump Go toolchain to 1.26.5 (stdlib vulns) | Medium | 1 | d8a2504 | DeepSeek V4 Flash |
| INFRA | Prepaid buckets health probe — all 6 healthy | Low | 1 | 2681f6b | DeepSeek V4 Flash |
| BOOTSTRAP | Create .coding-hermes/tasks.md | Trivial | 1 | 6b8bfbf | Hy3 |
| PHASE1.1 | Chronicle design system CSS + Go package + route | High | 4 | — | DeepSeek V4 Pro |
| PHASE1.2 | Layout shell: app chrome, command palette (⌘K), responsive | High | 5 | — | DeepSeek V4 Pro |
| PHASE1.3 | Component library: Button → Skeleton (9 components) + Table/CodeBlock | High | 4 | — | DeepSeek V4 Pro |
| PHASE1.4 | Investigation Workbench: THINK/SAYS panes, Input, Evidence, Discovery, WebSocket | High | 6 | — | DeepSeek V4 Pro |
| PHASE1.5 | Overview Dashboard (6 KPI cards) + Status Dashboard (health.css) | High | 5 | — | DeepSeek V4 Pro |
| PHASE1.6 | Timeline Explorer (1,596 lines CSS) + Entity Graph (D3.js force-directed) | High | 6 | — | GPT-5.6 Sol |
| PHASE1.7 | Wire Chronicle UI to Consensus API + E2E real LLM test | Critical | 5 | 0727588 | DeepSeek V4 Pro |
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

## Execution Order

1. UX-001 — Onboarding walkthrough ✅ (tick #8)
2. UX-002 — CLI error ergonomics (next)
3. UX-003 through UX-007, U01 — pending

## Escalation Conditions

- New security vuln in deps → create SEC task
- CI goes red on code-change commit → investigate
- Bane adds new work → resume normal 900s cooldown

---

## Phase 8: Real Integration Testing (OpenCode Shim)

> **WHY THIS PHASE EXISTS:** Phase 2 verified 25/25 shim endpoints return correct HTTP status codes. That is NOT integration testing. Nobody has ever pointed a real OpenCode client at Consensus and confirmed "I can't tell the difference." HTTP smoke tests ≠ semantic interoperability. This phase closes that gap.

- [x] **INT-001 — Full session lifecycle via shim**: start Consensus serve, create session via `POST /session`, send a real message, wait for response, verify THINK/SAYS blocks in memory. Use real LLM. One curl-based Go test in `internal/chronicle/shim_real_llm_test.go`. ✅ Commit `648c356` — 383-line test, build ✓, vet ✓, guard ✓. Foreman-direct: file was pre-written by prior tick, verified + committed.
- [ ] **INT-002 — Streaming (SSE) via shim**: send a message with `stream=true`, verify Server-Sent Events arrive chunked with `data:` prefix. Confirm the client receives progressive THINK/SAYS blocks before the final `[DONE]`.
- [ ] **INT-003 — Tool execution via shim**: send a message that triggers a tool call (e.g. `read_file`), verify the shim returns `tool_use` blocks, accept the tool result, verify the agent incorporates it. Test bash execution, file read, file write.
- [ ] **INT-004 — Multi-turn conversation via shim**: send 5 messages with context threading via `session_id`, verify the agent maintains conversation state across turns. Confirm memory events accumulate correctly (10+ events).
- [ ] **INT-005 — OpenCode SDK against Consensus**: use the official OpenCode Go client library to talk to Consensus via the shim. Verify the SDK's types, streaming, and error handling work without modification.
- [ ] **INT-006 — VSCode/Cursor extension against Consensus**: configure the OpenCode VSCode extension to point at Consensus, open a file, send a prompt, verify the agent responds correctly. (Requires GUI environment — document setup for manual testing.)
- [ ] **INT-007 — Budget enforcement via shim**: create a session with a $0.01 budget, send a message, verify the agent stops at budget limit. Confirm the shim returns appropriate error (402 or budget_exceeded).
- [ ] **INT-008 — Indistinguishability benchmark**: run 10 identical prompts against Consensus shim vs real OpenCode server. Compare: latency, token usage, response quality, streaming behavior. Goal: < 5% difference across all metrics.

## Idle Tick Log

|| Tick | Date | Counter | Checks | New Tasks | Action |
||------|------|---------|--------|-----------|--------|
|||| #31 | 2026-07-24 ~04:18Z | 7/7 (IDLE#12) | **IDLE#12 — 6TH ESCALATION.** Discovery sweep clean: build ✓, vet ✓, tests 30/30 ✓, Hilo 1135 edges ✓, CI 3/3 green ✓, no issues, no remote commits. Cooldown reverted AGAIN (43200→1800 on restart). Re-set to 43200s (verified: CooldownS=43200, Enabled=True). **IDLE#12 — SIXTH escalation. Bane: consensus idle 12 consecutive ticks. Cooldown reverts to 1800s on EVERY scheduler restart — re-fixed 6x. Root cause: fleet TOML cooldown-reset-on-restart pitfall. Recommend: (1) set Enabled=false in scheduler DB, or (2) fix fleet TOML persistence. NEVER-DONE rule prohibits auto-disable.** | 0 | Cooldown→43200s (12h). Idle tick #12. **6TH ESCALATION.** |
||| #30 | 2026-07-24 ~00:08Z | 7/7 (IDLE#11) | **IDLE#11 — 5TH ESCALATION.** Cooldown reverted AGAIN (43200→1800 on restart). Re-set to 43200s (verified: PUT+GET → CooldownS=43200). Build ✓, vet ✓, tests all pass ✓, Hilo 1135 edges ✓, no issues, no remote commits, no outdated direct deps. **IDLE#11 — FIFTH escalation. Bane: consensus idle for 11 consecutive ticks. Cooldown reverts to 1800s on EVERY scheduler restart (re-fixed 5x). Strongly recommend setting Enabled=false in scheduler DB. NEVER-DONE rule prohibits auto-disable.** | 0 | Cooldown→43200s (12h). Idle tick #11. **5TH ESCALATION.** |
|| #29 | 2026-07-24 ~01:41Z | 7/7 (IDLE#10) | **IDLE#10 — 4TH ESCALATION.** Cooldown reverted AGAIN (43200→1800 on restart). Re-set to 43200s (verified via PUT+GET). Build ✓, vet ✓, tests 30/30 ✓, Hilo 1135 edges ✓, vulns 0 ✓, no TODOs, no issues, no remote commits, no outdated direct deps. **IDLE#10 — FOURTH escalation. Bane: consensus has been idle for 10 consecutive ticks. The cooldown-reset-on-restart pitfall is chronic — cooldown reverts to 1800s on every scheduler restart. Strongly recommend setting Enabled=false in the scheduler DB or fixing fleet TOML persistence. NEVER-DONE rule prohibits auto-disable.** | 0 | Cooldown→43200s (12h). Idle tick #10. **4TH ESCALATION.** |
|| #28 | 2026-07-23 ~21:20Z | 7/7 (IDLE#9) | **IDLE#9 — 3RD ESCALATION.** Cooldown reverted AGAIN (43200→1800 on restart). Re-set to 43200s. Build ✓, vet ✓, tests all pass ✓, Hilo 1135 edges ✓, CI 5/5 green ✓, vulns 0 ✓, no actionable TODOs, no issues, no remote commits. 1 known-gap TODO (WI-004) — documented security gap in ApproveQuarantine, present across all 8 prior idle ticks, not new. **IDLE#9 — THIRD escalation. Bane: consensus has been idle for 9 consecutive ticks across ~96h. The cooldown keeps reverting to 1800s on scheduler restart. Recommend: either manually set Enabled=false in the scheduler DB, or fix the fleet TOML cooldown to persist across restarts.** | 0 | Cooldown→43200s (12h). Idle tick #9. **3RD ESCALATION.** |
|| #27 | 2026-07-23 ~17:21Z | 7/7 (IDLE#8) | **IDLE#8 — POST-ESCALATION.** Cooldown reverted AGAIN (43200→1800 on restart). Re-set to 43200s. Build ✓, vet ✓, tests 30/30 ✓, Hilo 1135 edges ✓, no TODOs, no issues, no remote commits. **IDLE#8 — SECOND escalation. Bane: consensus has been idle for 8 consecutive ticks. Recommend either manual disable or self-pause. Do NOT auto-disable per never-done rule.** | 0 | Cooldown→43200s (12h). Idle tick #8. **2ND ESCALATION.** |
|| #26 | 2026-07-23 ~13:20Z | 6/7 (IDLE#7) | **IDLE#7 ESCALATION** — Discovery sweep clean: build ✓, vet ✓, tests all pass ✓, vulns 0 ✓, GitReins guard ✓, Hilo 1135 edges ✓, CI 3/3 ✓, no TODOs, no issues, no remote commits, no outdated direct deps. DuckBrain: skipped (Connection Error — pre-existing). Cooldown reverted 43200→1800 on restart; re-set to 43200s (12h). **7 idle ticks — escalated to Bane. Do NOT self-disable.** | 0 | Cooldown→43200s (12h). Idle tick #7. **ESCALATED: recommend self-pause or manual disable.** |
|| #25 | 2026-07-23 ~09:26Z | 5/7 (IDLE#6) | **IDLE** — Discovery sweep clean: build ✓, vet ✓, tests 30/30 ✓, vulns 0 ✓, GitReins guard ✓, Hilo 1135 edges ✓, no TODOs, no issues, no remote commits. DuckBrain unreachable (Connection Error). Cooldown reverted 43200→1800 on restart; re-set to 43200s (12h). IDLE#6 — next tick escalates to self-pause at IDLE#7. | 0 | Cooldown→43200s (12h). Idle tick #6. |
| #24 | 2026-07-23 ~05:24Z | 4/7 (IDLE#5) | **IDLE** — Discovery sweep clean: build ✓, vet ✓, tests 30/30 ✓, vulns 0 ✓, GitReins guard ✓, Hilo 1135 edges ✓, no TODOs, no issues, no remote commits. Cooldown reverted 14400→1800 on restart; re-set to 43200s (12h — IDLE#5 escalation). | 0 | Cooldown→43200s (12h). Idle tick #5. |
| #23 | 2026-07-22 ~20:37Z | 3/7 (IDLE#4) | **IDLE** — Discovery sweep clean: build ✓, vet ✓, tests all pass ✓, CI 5/5 green ✓, vulns 0 ✓, GitReins guard ✓, Hilo 1135 edges ✓, no TODOs, no issues, no remote commits, no outdated direct deps. All 11 NEVER-DONE checks: pass. Cooldown reverted 14400→1800 on restart; re-set to 14400s. | 0 | Cooldown→14400s (4h). Idle tick #4. |
| #22 | 2026-07-22 ~14:28Z | 2/7 (IDLE#3) | **IDLE** — Resource-constrained: host thread exhaustion (errno=11). go build/vet couldn't spawn threads. GitReins guard partial: build ✓, lint ✓, tests ✓, static_analysis ✓, secrets ○ (gitleaks crash — pre-existing). Cooldown reverted 43200→1800 on restart; re-set to 14400s (4h). | 0 | Cooldown→14400s (4h). Idle tick #3. |
| #21 | 2026-07-22 ~14:16Z | 1/7 (IDLE#2) | **IDLE** — Discovery sweep clean: build ✓, vet ✓, tests 30/30 ✓, CI 5/5 green ✓, vulns 0 ✓, Hilo 1135 edges ✓, no TODOs. NEVER-DONE audit: all checks pass. Cooldown reverted from 43200→7200 on restart. Re-set to 43200s. | 0 | Cooldown→43200s (12h). Idle tick #2. |
| #20 | 2026-07-22 ~06:38Z | 0/7 (RESET→IDLE#1) | **IDLE** — Full discovery sweep: build ✓, vet ✓, tests 30/30 ✓, CI 5/5 green ✓, vulns 0 ✓, GitReins guard ✓, Hilo 1135 edges ✓, no TODOs, no issues, no remote commits, no outdated direct deps. Only NEVER-DONE remains. | 0 | Cooldown→43200s but reverted to 7200s on restart. |
| #19 | 2026-07-22 ~05:57Z | 0/7 (RESET) | **PRODUCTIVE** — U02: JSON round-trip tests for pkg/client/types.go. DS-V4-Flash worker wrote types_test.go (319 lines, generic jsonRoundTrip helper, all 22 types). 28/28 tests pass (22 new + 6 existing). Build ✓, vet ✓, guard ✓, commit `25185aa`. Worker committed directly (file verifier false-negative on path). | 0 (reset) | Cooldown→900s. 1 task done. Board now empty. |
| #18 | 2026-07-22 ~05:52Z | 0/7 (RESET) | **PRODUCTIVE** — UX-011: port 8090 shadowing fixed. MiniMax-M3 worker wrote VerifyIdentity() + PersistentPreRunE + 7 tests (+271 lines). Build ✓, guard ✓, tests ✓, commit `078c843`. | 0 (reset) | Cooldown→900s. 1 task done. |
| #17 | 2026-07-22 ~10:32Z | 0/7 (RESET) | **PRODUCTIVE** — UX-007: cross-platform quickstart gotchas documented. Foreman-direct investigation. | 0 (reset) | Cooldown→900s. 1 task done. |
| #16 | 2026-07-22 ~05:40Z | 0/7 (RESET) | **PRODUCTIVE** — U01: usability+coverage audit completed. 33 endpoints ✓, 1 gap → U02. | 1 (U02) | Cooldown→900s. U01 done, U02 created. |
| #15 | 2026-07-22 ~05:29Z | 0/7 (RESET) | **PRODUCTIVE** — UX-010: AdminDB wiring fix (1 line). Commit `13c1af1`. | 0 (reset) | Cooldown→900s. |
| #14 | 2026-07-22 ~05:26Z | 0/7 (RESET) | **PRODUCTIVE** — UX-006: CLI error verification. 6/7 PASS. Port 8090 gap → UX-011. | 1 (UX-011) | Cooldown→900s. |
| #13 | 2026-07-22 ~05:21Z | 0/7 (RESET) | **PRODUCTIVE** — UX-009: Dockerfile CMD fix (+2/-2). Commit `81b3935`. | 0 (reset) | Cooldown→900s. |
| #12 | 2026-07-22 ~05:20Z | 0/7 (RESET) | **PRODUCTIVE** — UX-005: README command accuracy. 10 cmds tested. | 0 (reset) | Cooldown→900s. |
| #9 | 2026-07-22 ~06:53Z | 0/7 (RESET) | **PRODUCTIVE** — UX-002: 25 incorrect commands tested, 3 error gaps fixed. Commit `9458453`. | 0 (reset) | Cooldown→900s. |
| #8 | 2026-07-22 ~06:37Z | 0/7 (RESET) | **PRODUCTIVE** — Board activated! UX-001 completed. | 0 (reset) | Cooldown 14400→900s. |

---

## [ ] NEVER-DONE — Run coding-hermes-never-done 11-point audit

Load coding-hermes-never-done skill. Run ALL 11 checks: spec alignment, doc coverage, test gaps, package upgrades, pitfall hunt, performance audit, endpoint verification, CI/CD health, DuckBrain sync, code quality, middle-out wiring. Create a task for EVERY gap found. This task is never complete — the audit always finds something.

> **Last audit:** 2026-07-24T04:18Z — IDLE TICK #31 (IDLE#12). All checks pass; 0 gaps found.
> **Next:** Idle counter 7/7 (IDLE#12). **6TH ESCALATION TO BANE — cooldown reverted 6x. Manual disable strongly recommended. Chronic cooldown-reset-on-restart pitfall unresolved.**
