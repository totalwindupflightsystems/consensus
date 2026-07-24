# Consensus — Model Router Task Matrix

> **Core purpose:** Database-native agent harness with Go-based planning loop, H3 protocol integration, and multi-model LLM routing.

## Active — Critical

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| SQLITE-DEADLINE | Fix SQLite `context deadline exceeded` in static binary. The `consensus-static` binary fails to start the planning loop — every session creation times out after 3 min with `sqlite: begin tx: context deadline exceeded`. Go context passed to SQLite transaction has a deadline shorter than what SQLite WAL mode requires. Fix: increase Go context deadline in Consensus's planning module, or switch to PostgreSQL backend, or use Docker-built binary. | CRITICAL | 4 | — | sqlite,database,blocking | Kimi K3 | Bug fix: database connectivity, Go context deadline issue | DeepSeek V4 Pro |
| BUNKER-KEY | Fix DEEPSEEK_API_KEY corruption on bunker. Initial grep/cut/tr pipeline introduced trailing `}` character. Key corrected via Python script but running Consensus process may still have old key cached in database. Fix: delete dev.db, verify .env clean, restart Consensus, verify log shows `llm: calling provider` with no auth error. | CRITICAL | 2 | — | api-key,bunker,blocking | DeepSeek V4 Flash | Simple/env fix: key rotation and verification | Kimi K3 |
| VET-CI-FAILURE | CI `go vet` step failing on latest commit (140f15f). `go vet ./...` passes locally (Go 1.26.5) — may be CI toolchain mismatch. Investigate CI log for specific vet warnings. Workaround: align local+CI Go versions, or check CI workflow for vet-specific flags. | HIGH | 2 | — | ci,vet,toolchain | DeepSeek V4 Flash | CI debugging: vet mismatch | Kimi K3 |
| DEPLOY-05 | Full E2E smoke test (H3 → Consensus → LLM → response). Verify complete round-trip: message enters H3 shim, traverses h3-consensus-adapter, reaches Consensus REST API, agent loop processes via LLM, response returns. Currently blocked by SQLite deadline + API key issues above. | CRITICAL | 3 | SQLITE-DEADLINE,BUNKER-KEY | e2e,integration,smoke-test | GPT-5.6 Luna | E2E testing: full integration verification across H3→Consensus→LLM | Step 3.7 Flash |

## Active — Infrastructure

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| DEPLOY-01 | ✅ Bunker agent with Hermes v0.19.0 | DONE | 2 | — | deploy,bunker | — | ✅ Done | — |
| DEPLOY-02 | ✅ Consensus binary deployed to bunker | DONE | 2 | — | deploy,bunker | — | ✅ Done | — |
| DEPLOY-03 | ✅ H3 adapter bridging H3 ↔ Consensus | DONE | 3 | — | deploy,adapter | — | ✅ Done | — |
| DEPLOY-04 | ✅ SSH tunnel host → bunker | DONE | 1 | — | deploy,network | — | ✅ Done | — |
| DEPLOY-06 | Fix DEEPSEEK_API_KEY corruption on bunker (trailing `}` from shell pipeline) — see BUNKER-KEY above | CRITICAL | 2 | — | api-key,bunker | DeepSeek V4 Flash | Simple/env fix | Kimi K3 |

## H3 Integration Status

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| H3-PROTOCOL | ✅ H3 protocol specs (27 docs) | DONE | — | — | spec | GPT-5.6 Terra | Spec/doc writing | — |
| H3-ADAPTER | ✅ H3 adapter (sdk-go/cmd/h3-consensus-adapter) — 43/43 compliance | DONE | 4 | — | adapter,go | DeepSeek V4 Pro | Architecture: adapter implementation | — |
| H3-TEST-BATTERY | ✅ H3 test battery | DONE | 3 | — | testing | Step 3.7 Flash | Testing: test battery execution | — |
| H3-SESSION | ✅ Session creation via adapter | DONE | 2 | — | integration | — | ✅ Done | — |
| H3-MESSAGE | ✅ Message delivery → Consensus agent | DONE | 2 | — | integration | — | ✅ Done | — |
| H3-LOOP | ✅ Consensus picks up agent loop | DONE | 2 | — | integration | — | ✅ Done | — |
| H3-LLM | ❌ LLM API call — auth on bunker, SQLite on local | BLOCKED | 2 | BUNKER-KEY,SQLITE-DEADLINE | llm,api | Kimi K3 | Bug fix: API auth + database | — |
| H3-ROUNDTRIP | ❌ Full round-trip response — blocked by above | BLOCKED | 2 | H3-LLM | e2e,integration | GPT-5.6 Luna | E2E testing: blocked pending fixes | — |

## Consensus Adapter Fixes (2026-07-24)

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| FIX-01 | ✅ `test_5_10_session_not_found` — No `GET /v1/sessions/{id}` route; 405 returned. Added route: returns 404 for unknown, 200+status for known. | DONE | 2 | — | adapter,fix | DeepSeek V4 Flash | Simple route addition | — |
| FIX-02 | ✅ `test_2_4_process_text_finished_false` — Adapter always returned `finished: true` when Consensus idle. Added `streamingDetected()`: content hints like "do not finish" → `finished: false`. | DONE | 2 | — | adapter,fix | MiniMax M3 | Bug fix: streaming detection | — |

## NEVER-DONE — 11-point audit

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| NEVER-DONE | 11-point audit: spec alignment, doc coverage, test gaps, package upgrades, pitfall hunt, performance audit, endpoint verification, CI/CD health, DuckBrain sync, code quality, middle-out wiring. Run every 3-4 ticks. | Low | 3 | — | audit,quality | DeepSeek V4 Pro | Architecture-level project audit across all subsystems | GLM-5.2 |

- [ ] **E2E-001 — E2E Testing Tick (self-improving loop)** | Recurring every 5-10 ticks | — | — | Luna (browser/screenshots) or Step 3.7 Flash (CLI/API) | foreman-direct | — | —
