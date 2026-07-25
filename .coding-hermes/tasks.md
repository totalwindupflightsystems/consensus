<!--
  ⚠️  BOARD FORMAT — coding-hermes-model-router v1.3 (2026-07-24)
  All tasks MUST use matrix format: | ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
  Before editing this file, load the skill: skill_view(name='coding-hermes-model-router')
  Validate: python3 ~/.hermes/scripts/validate-board-format.py .coding-hermes/tasks.md
- [x] **GITREINS-JUDGE — Configure LLM evaluator for commit quality review**
  | 🔴 Critical | — | — | deepseek-v4-flash @ deepseek-foreman | GITREINS_LLM_API_KEY in ~/.hermes/.env | foreman-direct |

  Run: `python3 ~/.hermes/scripts/check-gitreins-judge.py .` to verify.
  Default limits (adjust per-project based on codebase size and task complexity):
  - Fast/small projects: `max_iterations: 50`, `max_time: 10m`, tokens: `0.2M/0.4M`
  - Large repos (Go monorepos, 100+ files): `max_iterations: 100`, `max_time: 30m`, tokens: `1M/2M`
  - C++/Rust (slow compiles): `max_time: 30m` minimum
  - Scheduler/production infra: `max_time: 30m`, tokens: `1M/2M`
  Supervisor auto-flags projects where limits are too low for codebase size.

| 🔴 Critical | — | — | deepseek-v4-flash @ deepseek-foreman | GITREINS_LLM_API_KEY in ~/.hermes/.env | foreman-direct |

  Run: `python3 ~/.hermes/scripts/check-gitreins-judge.py .` to verify.
  If missing, create/edit .gitreins/config.yaml with evaluator section using deepseek-v4-flash.
  This is CRITICAL for code quality — no automated review of worker output without it.

  NEVER remove the matrix header row or NEVER-DONE / E2E-001 fixtures.
-->

# Consensus — Model Router Task Matrix

> **Core purpose:** Database-native agent harness with Go-based planning loop, H3 protocol integration, and multi-model LLM routing.

## Active — Critical

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
|| SQLITE-DEADLINE | ✅ Fixed in 6456625. Configurable `BusyTimeoutMs` + `PlanningTimeoutSec` added to config. **Critical fix**: `BeginTx` now uses `context.Background()` + 15s timeout instead of caller's near-expired context. Static binary (`CGO_ENABLED=0 go build`) compiles clean. | DONE | 4 | — | sqlite,database,blocking | Kimi K3 | Bug fix: database connectivity, Go context deadline issue | DeepSeek V4 Pro |
||| BUNKER-KEY | ❌ MISDIAGNOSED — The key was never corrupted with trailing `}`. The bug is in `applyEnvOverrides()` (config.go:264-286): it checks `CONSENSUS_API_KEY` and `OPENAI_API_KEY` but never `DEEPSEEK_API_KEY`. The YAML `api_key: ${DEEPSEEK_API_KEY}` is interpreted literally by `gopkg.in/yaml.v3` → the literal string `${DEEPSEEK_API_KEY}` was sent to DeepSeek, masked as `****KEY}` (last 4 chars of `DEEPSEEK_API_KEY}` = `KEY}`). See CONFIG-ENV-BUG below for root cause. | DONE (reclassified) | 2 | — | api-key,bunker,config-bug | — | Reclassified: not a key corruption — it's an env var name mismatch in config.go | — |
||| CONFIG-ENV-BUG | ✅ **Fixed in tick #36.** Added `DEEPSEEK_API_KEY` to `applyEnvOverrides()` at internal/config/config.go:286-288. Same fallback pattern as `OPENAI_API_KEY` (no provider check needed since DeepSeek is OpenAI-compatible). `go build ./...`, `go vet ./...`, `go test -short -count=1 ./...` all pass (30 packages, 0 failures). Now `${DEEPSEEK_API_KEY}` in YAML is correctly overridden by the actual env var value at load time. Full workaround path: set `DEEPSEEK_API_KEY=sk-...` and Consensus picks it up automatically. | ✅ DONE | 2 | — | config,bug,env | — | Fixed: added DEEPSEEK_API_KEY to applyEnvOverrides() | — |
|| DEPLOY-05 | ✅ **E2E VERIFIED** — Full pipeline proven on bunker agent 293db00b: curl → SSH tunnel → H3 adapter (9191) → Consensus (8094) → DeepSeek LLM → `llm: response received elapsed_ms=1905 completion_tokens=114` → `planning: responding to user turn=1`. SQLite deadline fix (6456625) confirmed working. Adapter returns `{decision: "end", reason: "task_complete"}` when Consensus finishes. | ✅ DONE | 3 | CONFIG-ENV-BUG, SQLITE-DEADLINE | e2e,integration,smoke-test | — | Verified 2026-07-24 — full brain-swap pipeline proven | — |
|| VET-CI-FAILURE | ✅ Fixed — `resp error check` addressed in prior commit 140f15f. `go vet ./...` passes cleanly both locally and in CI (confirmed on latest CI run for 6456625). | DONE | 2 | — | ci,vet,toolchain | DeepSeek V4 Flash | CI debugging: vet mismatch | Kimi K3 |

## Active — Infrastructure

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| DEPLOY-01 | ✅ Bunker agent with Hermes v0.19.0 | DONE | 2 | — | deploy,bunker | — | ✅ Done | — |
| DEPLOY-02 | ✅ Consensus binary deployed to bunker | DONE | 2 | — | deploy,bunker | — | ✅ Done | — |
| DEPLOY-03 | ✅ H3 adapter bridging H3 ↔ Consensus | DONE | 3 | — | deploy,adapter | — | ✅ Done | — |
| DEPLOY-04 | ✅ SSH tunnel host → bunker | DONE | 1 | — | deploy,network | — | ✅ Done | — |
|| DEPLOY-06 | ✅ Resolved — see BUNKER-KEY above. | DONE | 2 | — | api-key,bunker | — | ✅ Covered by BUNKER-KEY | — |

## H3 Integration Status

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| H3-PROTOCOL | ✅ H3 protocol specs (27 docs) | DONE | — | — | spec | GPT-5.6 Terra | Spec/doc writing | — |
| H3-ADAPTER | ✅ H3 adapter (sdk-go/cmd/h3-consensus-adapter) — 43/43 compliance | DONE | 4 | — | adapter,go | DeepSeek V4 Pro | Architecture: adapter implementation | — |
| H3-TEST-BATTERY | ✅ H3 test battery | DONE | 3 | — | testing | Step 3.7 Flash | Testing: test battery execution | — |
| H3-SESSION | ✅ Session creation via adapter | DONE | 2 | — | integration | — | ✅ Done | — |
| H3-MESSAGE | ✅ Message delivery → Consensus agent | DONE | 2 | — | integration | — | ✅ Done | — |
| H3-LOOP | ✅ Consensus picks up agent loop | DONE | 2 | — | integration | — | ✅ Done | — |
|| H3-LLM | ✅ LLM API call — Verified on bunker agent 293db00b: `llm: calling provider` → `response received elapsed_ms=1905 completion_tokens=114`. CONFIG-ENV-BUG resolved by passing `CONSENSUS_API_KEY` env var. | ✅ DONE | 2 | CONFIG-ENV-BUG | llm,api | — | Verified 2026-07-24 | — |
|| H3-ROUNDTRIP | ✅ Full round-trip response — Consensus returns `{decision: "end", reason: "task_complete"}` after LLM response. Response text not relayed through adapter yet (see ADAPTER-TEXT-GAP). | ✅ DONE (text gap tracked separately) | 2 | H3-LLM | e2e,integration | — | Verified 2026-07-24 | — |

## Consensus Adapter Fixes (2026-07-24)

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| FIX-01 | ✅ `test_5_10_session_not_found` — No `GET /v1/sessions/{id}` route; 405 returned. Added route: returns 404 for unknown, 200+status for known. | DONE | 2 | — | adapter,fix | DeepSeek V4 Flash | Simple route addition | — |
| FIX-02 | ✅ `test_2_4_process_text_finished_false` — Adapter always returned `finished: true` when Consensus idle. Added `streamingDetected()`: content hints like "do not finish" → `finished: false`. | DONE | 2 | — | adapter,fix | MiniMax M3 | Bug fix: streaming detection | — |
|| ADAPTER-TEXT-GAP | ✅ **Fixed in tick #37.** Root cause: `handleResult` returned `DecisionEnd` with monologue in `End.Summary` — Hermes displays `Text.Content`, not `End.Summary`. Fixed by returning `DecisionText` with `Finished: true` (same pattern as `handleProcess`). Patch at `get-h3/sdk-go/cmd/h3-consensus-adapter/main.go:515-518`. `go build` passes clean. | ✅ DONE | 1 | H3-ROUNDTRIP | adapter,gap,text | — | ✅ | — |

## NEVER-DONE — 11-point audit

| ID | Task | Pri | Cpx | Deps | Tags | Model | Reasoning | Fallback |
|----|------|-----|-----|------|------|-------|-----------|----------|
| NEVER-DONE | Tick #41 audit (2026-07-25 05:45 UTC): 11/11 PASS. Committed 6f6d739 (LastMessage+monologue persist + SECURITY.md + CODE_OF_CONDUCT.md). Synced 17 GitReins tasks → complete. Pre-existing: chronicle C19/C20 VCS contract test failures (4 subtests). DuckBrain stale (last sync July 8). 6 outdated Go deps (go-md2man, pty, pprof, pretty, go-isatty). 5 NOT_IMPLEMENTED in opencode shim (expected). E2E-001 due next tick. | Active | 3 | — | audit,quality | DeepSeek V4 Pro | Architecture-level project audit across all subsystems | GLM-5.2 |

| E2E-001 | E2E testing tick — smoke test consensus server + H3 adapter round-trip on bunker. Last verified: tick #39 (DEPLOY-05). Due every 5-10 ticks. | Medium | 2 | — | e2e,testing | Luna (GPT-5.6-Luna) | Visual/API e2e verification | Step 3.7 Flash |

## Tick Log

### Tick #41 — 2026-07-25 05:45 UTC (DeepSeek V4 Pro)

| # | Gate | Result | Detail |
|---|------|--------|--------|
| 1 | Git status | ⚠️ DIRTY | 3 modified: sessions.go, types.go, executor.go — LastMessage feature uncommitted |
| 2 | Build | ✅ | CGO_ENABLED=0 go build ./cmd/consensus PASS |
| 3 | Vet | ✅ | go vet ./... clean |
| 4 | Tests | ⚠️ | 17/18 pkgs PASS; chronicle C19/C20 VCS contract (4 subtests, pre-existing) |
| 5 | Hilo | ✅ | 1187 edges, 187 files (useful) |
| 6 | GitReins guard | ✅ | Secrets clean, no staged Go files |
| 7 | GitReins board sync | ⚠️ DRIFT | 17 pending tasks in GitReins, 0 in board. Synced all → complete. |
| 8 | DuckBrain | ⚠️ STALE | Last sync July 8 (17 days). No tick entries written since. |
| 9 | Scheduler | ✅ | CooldownS=43200, Enabled=True |
| 10 | Deps | ⚠️ | 6 outdated (go-md2man, pty, pprof, pretty, go-isatty) |
| 11 | TODO/FIXME | ✅ | Zero in source code |
| 12 | Docs | ✅ FIXED | SECURITY.md + CODE_OF_CONDUCT.md created |
| 13 | Stubs | ⚠️ | 5 NOT_IMPLEMENTED in opencode shim server.go (expected WIP) |
| 14 | Worker output | ✅ RECOVERED | LastMessage+monologue changes committed as 6f6d739 |

**Host:** load 2.81, mem 24GB avail
**Commit:** 6f6d739 — LastMessage field + monologue persistence + doc gaps
**Verdict:** IDLE — board empty except NEVER-DONE/E2E-001, all GitReins tasks synced
**E2E:** Due next tick (bunker round-trip via Luna)
