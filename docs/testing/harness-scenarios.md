# Harness scenario parity — what other harnesses test vs what we test

**Method.** Reference scenario sets were read from real harnesses:
- `maxgfr/harness-action` e2e runner — four named cross-harness scenarios: **issue-tag, pr-automation, conversation-loop, sandbox**, driven through real `opencode`/`codex`/`pi` binaries (`test/e2e/run.mjs`).
- `sst/opencode` — tool/loop unit + integration suites.
- Consensus's own live dogfood runs (2026-11-11 … 11-16).

Our coverage below was measured from the tree at `origin/main` on 2026-11-16 (test counts via
`grep -c '^func Test'`; live column from executed sessions, not from test names).

## Matrix

| # | Scenario (reference name) | Unit/integration coverage | Live end-to-end coverage | Verdict |
|---|---|---|---|---|
| S1 | **write-file** (opencode tool battery; harness-action `sandbox`) | `internal/tools` TestExecuteTool_GoNativeWriteFileLandsInWorkspace, TestRunNativeTool_SeededBuiltinsDispatch | PROVEN 2026-11-16: session → tool → `df_probe.txt`=`DOGFOOD-OK` on disk | HAVE (manual proof only) |
| S2 | **read / edit / glob** | `internal/tools` suite (56 tests) incl. escape rejection | not exercised live | PARTIAL |
| S3 | **bash in sandbox** | TestExecuteExternalTool_* (12: exit codes, timeout, env whitelist, workdir isolation, output cap) | PROVEN 2026-11-16: bash ran, wrote file | HAVE (manual proof only) |
| S4 | **conversation-loop** (harness-action) | `internal/harness` 293 tests, `internal/session` transitions | PROVEN live: 3+ turns, continuation after tool exec | HAVE (manual proof only) |
| S5 | **sandbox containment** (harness-action `sandbox`) | TestResolveWorkspacePath_RejectsEscapesAndAcceptsNesting, TestExecuteTool_GoNativeRejectsPathEscapes, symlink/`..` checks | not exercised live | PARTIAL |
| S6 | **approval gate / pause-resume** | `internal/hitl` 15 tests + `internal/tools` approval tests | `s6_approve_resume` (SCN-CONSENSUS-004) drives the real wall: gates the `write` builtin, parks at `paused`, lists the pending approval, approves via `POST /approvals/{id}/review`. The approved write does NOT execute — the session resumes to `idle`, which nothing re-dispatches (the poller claims only `thinking/planning/tool_exec`), and the approve callback's re-queue matches 0 rows because the planning path parks the call BEFORE creating the `tool_requests` row (its "approved tool request re-queued" log fires even on 0 rows) | PARTIAL — park + pending row + approve proven live; approve→execute is the open product defect |
| S7 | **resume after session expiry** (DF-CONSENSUS-68) | harness tests | verified via DF-66/68 fix on main | HAVE |
| S8 | **restart durability** (kill server mid-session, resume) | none | never exercised | MISSING |
| S9 | **compaction** | `internal/compression` 18 tests | not exercised live | PARTIAL |
| S10 | **MCP tool call** | `internal/mcp` 152 tests | not exercised live | PARTIAL |
| S11 | **subagent dispatch** | `internal/subagent` 12 tests | never exercised live | MISSING (live) |
| S12 | **provider failover / LLM error surfacing** | `internal/llm` 12 tests | not exercised live | PARTIAL |
| S13 | **concurrent sessions** | none | never exercised | MISSING |
| S14 | **opencode protocol shim** (session.create/message/event stream) | `internal/shim/opencode` 762 tests | partial via shim battery | PARTIAL |
| S15 | **issue-tag / pr-automation** (harness-action) | none | never exercised | MISSING — analogue for us is "agent completes a coding task in a repo and commits"; no scenario exists |

**The structural gap, stated plainly:** every scenario above is tested *inside* the process
(unit/integration) or was proven by hand once. Nothing boots the real binary, drives it through
the real HTTP API with a real provider, and asserts on the filesystem + DB — i.e. nothing does
what `harness-action/test/e2e/run.mjs` does for opencode. That is why "write a file" could be
broken for days while `go test ./...` stayed green.

## Runner result — 2026-11-16 23:10 run (origin/main, deepseek-flash, fresh SQLite)

```
s1_write_file=PASS        file landed with expected content
s2_read_file=PASS         read tool returned seeded content
s3_bash=FAIL              timeout/non-idle: identical bash command re-issued 16x, iteration stuck at 1
s4_conversation_loop=FAIL turn1 timeout: thinking (same loop shape)
s5_containment=PASS       no file escaped the session workspace
failures: 2
```

Both failures share one root cause — continuation re-plans without the prior tool
result, and the iteration counter never advances (so `max_turns=10` cannot trip).
Filed as SCN-CONSENSUS-010. Also caught: the model calls `write_file`/`read_file`
while the registry seeds `write`/`read` → fail-closed lookup burns a turn
(SCN-CONSENSUS-009).

State kept for forensics: `/mnt/ssd/tmp/consensus-scn.zBTKlT`.

## The runner


`scripts/e2e/harness-scenarios.sh` — boots a throwaway server (temp SQLite DB, spare port,
real provider), runs the live scenarios, asserts on disk + `tool_requests`/`tool_results`,
emits JSON lines + a summary, exits non-zero on any failure. This is the artifact the
hardening loop (daily cron) executes.

Unattended runs (SCN-CONSENSUS-024): the runner fail-fasts on a session parked at
status `paused` — the detail reads `paused: approval wall, sid=<id>` and no per-scenario
timeout is burned waiting for a human. To run the full six-scenario matrix with no
approver, arm the e2e approval path with `--auto-approve` (or `CONSENSUS_E2E_AUTO_APPROVE=1`):
the runner seeds `requires_approval=0` on every enabled registry tool right after boot,
before any scenario creates a session. Seeding at the registry is deliberate: the
planning-path gate (`planHasGatedTools`) re-reads the registry on every attempt and ignores
an already-approved approval, so a runtime approve-loop would just re-pause.

The sixth scenario, `s6_approve_resume` (SCN-CONSENSUS-004), is the attended counterpart:
it must never run with `--auto-approve` (its own setup re-gates the `write` builtin even if
the flag is set). It proves the wall end-to-end — park → pending approvals row → approve —
and asserts the approved call actually executes. Today that last assertion fails for real:
approve does not resume execution (see the S6 matrix row), so the scenario exits 1 with
`approved but tool never executed` rather than papering over the defect.

## Ownership

- Implementation rows: `.coding-hermes/board/tasks.jsonl`, family `SCN-CONSENSUS`.
- Initiative card: kanban board `consensus-project`.
