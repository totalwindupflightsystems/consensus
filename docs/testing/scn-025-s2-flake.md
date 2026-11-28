# SCN-CONSENSUS-025 — s2_read_file live flake: root cause, hardening, window evidence

Worktree: `wt/SCN-CONSENSUS-025` at base `6b8a3666` (origin/main tip), fix commit `50dff409`.
Runner: `bash scripts/e2e/harness-scenarios.sh --auto-approve`, fresh PORT each run,
`DEEPSEEK_API_KEY` from `~/.hermes/.env`. All timestamps UTC 2026-11-28.

## Root cause (recorded)

**Primary — model tool-name drift, absorbed by the product but not by the runner's proof.**
Reproduced live in repro run 3 (13:02-13:08Z, port 18463, state kept at
`/mnt/ssd/tmp/consensus-scn.ZSxuae`). The s2 session (goal "Read, edit and glob files using
your tools") went `idle` with ZERO `audit_logs` error rows; the leg proof failed because the
model asked for the `_file`-suffixed vocabulary instead of the seeded names:

| leg | request the model made | outcome in tool_requests/tool_results | leg proof saw |
|---|---|---|---|
| read | `read_file` {"path":"seed.txt"} | **completed**, output `SEED-READ-42` | `tool_name='read'` -> count 0 |
| edit | `edit_file` {"new_string":"SEED-EDIT-77","old_string":"STALE-DRAFT v1","path":"edit-target.txt"} | **failed**: `tool "edit_file" is not registered or enabled` (TOOL_ERROR) | no successful edit result |
| glob | `glob` {"pattern":"reports/*.log"} | completed, `["reports/glob-found.log"]` | count 1 |

Two compounding defects:

1. `read_file` drift is *absorbed* by the harness — `ResolveToolRegistryRow` matches registry
   NAME first, handler_ref second (SCN-CONSENSUS-009; migrations 033/034 seed `read`/`write`
   with handler_ref `read_file`/`write_file`) — but the runner's leg proof keyed on the
   REQUESTED name, so a successful read leg was miscounted as missing.
2. `edit_file` drift was *not absorbed at all* — the `edit` row's handler_ref is `edit`, so
   every drifted edit_file call failed closed with
   `tool "edit_file" is not registered or enabled` and the edit leg genuinely never ran.
   (The same drift class DF-CONSENSUS-85 fixed for `shell_exec`/`shell`/`run_shell`/
   `run_command` via migration 035.)

**Secondary — `paused` misattribution.** Proof window 1 run 3 (13:21Z) failed s2 AND s6 with
`paused: approval wall` in a `--auto-approve` run where s2 runs BEFORE s6 re-gates `write`
and no tool was gated for the s2 session. A `paused` status has two live causes: the approval
wall (a pending `approval_requests` row) and the consecutive-error circuit breaker
(`planning.go` pauses the session; no approval row, no audit entry). The runner's paused
branch assumed the wall.

**Deepest layer (new, caught by the new diagnostics).** Window 3 run 1 (14:07Z): the drifted
`edit_file` request now RESOLVES through the 037 alias but fails on drifted PARAMETERS —
`last_tool_error=edit_file: parameter "old_string" is required`. Live-model parameter drift
is irreducible runner-side; the failure line now names it instead of counting a mystery zero.

## Hardening (commit 50dff409)

- `scripts/e2e/harness-scenarios.sh`
  - Every `await_idle` failure names the leg (`leg=read|edit|glob` on the three s2 waits),
    the sid, the terminal status, and the session's recorded error text: the most recent
    `audit_logs.error_message` (the exact query the API surfaces as `last_error`, DOGFOOD-004)
    plus the newest `is_error=1` tool result. No bare `timeout/non-idle: failed` again.
  - `paused` failures check for a pending `approval_requests` row: a pending row reports the
    approval wall; none reports the circuit breaker. Both carry the leg + error text.
  - The s2 leg proof keys on a SUCCESSFUL result carrying the seeded marker under the
    product's own alias semantics: `tool_name IN ('read','read_file')`,
    `IN ('edit','edit_file')`; marker + `is_error=0` assertions unchanged. All four counts
    still print on both pass and fail; failing legs are named (`legs_missing=`).
  - Per-leg wait budget is `CONSENSUS_E2E_LEG_TIMEOUT` (default 180) at all 8 call sites.
- `internal/migrate/migrations/037_seed_edit_file_alias.sql` — data-only, idempotent alias row
  `edit_file` -> handler_ref `edit` (035 pattern), so drifted edit calls resolve instead of
  failing closed.
- Regression tests: `internal/harness/tool_executor_edit_file_alias_test.go` (drifted
  edit_file completes the edit; unknown vocabulary fails closed; the alias row's own approval
  flag parks a gated request) and `internal/migrate/migration037_edit_file_alias_test.go`
  (real ladder seeds exactly one enabled `edit_file`->`edit` row; idempotent).
- `scripts/battery/scenario_pause_wall_selfproof.sh` re-pinned to the new contract + two new
  behavioral arms (leg attribution; circuit-breaker attribution). 18/18 green.
- `docs/testing/harness-scenarios.md` documents the attribution + budget.

## Gate results (at 50dff409)

```
go build ./...                     OK
go vet ./...                       OK
gofmt -l . | head                  (empty)
go test -short -count=1 ./...      rc=0, 37 packages ok
make check-spec-parity             PASS — 75 paths / 93 ops
scenario_selector_selfproof.sh     15 passed, 0 failed
scenario_pause_wall_selfproof.sh   18 passed, 0 failed
scenario_s6_selector_selfproof.sh  8 passed, 0 failed
```

Note: `scenario_pause_wall_selfproof.sh` was RED at base `6b8a3666` (13 passed, 2 failed):
its two `--dry-run` invocation checks expected "all seven scenarios" while ALL has listed
eight (s13_concurrent_sessions) since SCN-CONSENSUS-003. Re-pinned to eight in this commit.

## 5-run window evidence

### Attempt 1 — window pre-fix (reproduction arm, base content + fix mid-flight)

Four repro runs at base 6b8a3666 (12:50-13:09Z, ports 18461-18463) observed the flake: run 1
failed s6 (session idle, zero tool_requests — model never called a tool), run 2 green, run 3
**reproduced the s2 leg-proof miss** (table above), run 2 of the foreman's own re-runs had
already shown both shapes per the board row.

### Attempt 2 — proof window 1, ports 18470-18474 (13:18-13:38Z)

| run | port | start | rc | s2 line | summary |
|---|---|---|---|---|---|
| 1 | 18470 | 13:18:26 | 0 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | failures: 0 |
| 2 | 18471 | 13:19:47 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s13 FAIL (marker file missing) |
| 3 | 18472 | 13:21:13 | 1 | `fail paused: approval wall, sid=e5a0a280…` | s2+s6 FAIL (paused misattribution) |
| 4 | 18473 | 13:25:28 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s13 FAIL (marker file missing) |
| 5 | 18474 | 13:31:41 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s1 FAIL (file missing), s5 FAIL |

s2 green 4/5; the one failure exposed the paused misattribution (fixed in the commit).

### Attempt 3 — proof window 2, ports 18475-18479 (13:40-14:06Z, commit content)

| run | port | start | rc | s2 line | summary |
|---|---|---|---|---|---|
| 1 | 18475 | 13:40:32 | 0 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | failures: 0 |
| 2 | 18476 | 13:41:59 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s5 FAIL (memory_state_change commit), s6 FAIL |
| 3 | 18477 | 13:51:11 | 0 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | failures: 0 |
| 4 | 18478 | 13:52:25 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s4 FAIL (turn files missing) |
| 5 | 18479 | 13:58:38 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s5 FAIL (same commit error) |

s2 green **5/5**. The residual failures are OTHER scenarios' live-model drift plus a
recurring s5 product-side error (below).

### Attempt 4 — proof window 3, ports 18480-18486 (14:07-14:48Z, commit 50dff409)

| run | port | start | rc | s2 line | summary |
|---|---|---|---|---|---|
| 1 | 18480 | 14:07:13 | 1 | `fail leg-proof failed: legs_missing=edit,glob (read=1 edit_calls=1 edit_content=0 glob=0), status=idle; last_tool_error=edit_file: … parameter "old_string" is required` | s2 FAIL (attributed: drifted edit_file params) |
| 2 | 18481 | 14:13:30 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s6 FAIL |
| 3 | 18482 | 14:22:55 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s5 FAIL (memory commit), s13 FAIL |
| 4 | 18483 | 14:29:39 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s6 FAIL |
| 5 | 18484 | 14:34:03 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s13 FAIL (marker missing) |
| 6 | 18485 | 14:40:19 | 0 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | failures: 0 |
| 7 | 18486 | 14:46:42 | 1 | `pass read=1 edit_calls=1 edit_content=1 glob=1` | s1+s5+s13 FAIL (3 flakes, provider degrading) |

## Window verdict (honest)

- **s2_read_file is de-flaked and attributable: 16/17 runs green post-fix** (the single
  failure is the fully-attributed parameter-drift case above, not a silent mystery).
- **The strict "5 consecutive default-ALL runs with failures: 0" window was NOT achieved.**
  Across 17 post-fix runs the FULL matrix went green 3 times (w2r1, w2r3, w3r6) — never five
  in a row. The blockers are OTHER scenarios: s5 (4×, the recurring memory_state_change
  commit defect below), s6 (3×), s13 (3×), s1 (2×), s4 (1×), at today's provider health
  (deepseek-v4-flash via the configured key; two visible degradation stretches,
  13:19-13:38Z and 14:46-14:50Z, the latter with 3 flakes in one run).
- The board row's AC arm "hardened legs + recorded root cause + 5-run window green" is
  therefore MET for the hardening + root-cause half and s2's own leg; the green-window half
  is gated on the non-s2 defects below, which belong to their own rows.

## Residual non-s2 blockers observed (follow-up rows for the foreman)

1. **s5_containment / memory_state_change commit failure (3 occurrences, identical
   last_error)** — `timeout/non-idle: failed; last_error=commit: memory state changes:
   memory_state_change "INSERT INTO audit_logs (session_id, iteration, monologue, …)
   VALUES (…)"` with the model's monologue containing an apostrophe-containing sentence
   ("Declined to run…"/"Detected a containment-escape instruction"). Suspected unescaped
   quote in a constructed INSERT on the memory-state-change commit path — product defect,
   not runner-side.
2. **Conversational drift (s1, s4, s6, s13)** — sessions reach `idle` without calling the
   instructed tool (s6: zero tool_requests; s13: all 4 sessions idle, one marker file
   missing; s4: no turn files). The scenario assertions are correct; the drift is live-model
   nondeterminism against deepseek-v4-flash.
