# SG-1 report — the guards are real, and they can fail

**Subgoal.** *Make the guards executable, and prove they can fail.* (Merged tree §4, SG-1; from
Quorum #1 claim C2, contradicted by 5/5 seats.)

## done-when → measured

| requirement | result |
|---|---|
| C-122/C-123/C-187/C-230 exist as real scripts with exit contracts | **MET** — `defect_gate.sh`, `guard_void.sh`, `zero_target.sh`, `rederive.sh` under `scripts/battery/` (the directory 25 manifest references previously pointed at, which did not exist) |
| a tier with zero observables reports VOID | **MET** — `guard_void` → rc 2, voice `guard_void: VOID` |
| a zero-audit step exits 2 | **MET** — `zero_target 0` → rc 2, voice `zero_target: VOID` |
| a red-proof artifact is captured | **MET** — `docs/testing/evidence/sg1-guard-redproof.txt`, **11 cases, 11 ok, 0 broken** |

Command: `bash scripts/battery/redproof.sh` → `RESULT: PASS — every guard failed with its own
VOID/RED verdict on bad input, and passed on good input.` (exit 0)

## Two defects the red-proof caught — in the guards themselves

1. **`rederive.sh` did not run at all.** A `printf` header carried unquoted parentheses
   (`SHA256(12)`), which is a bash syntax error. My first syntax sweep said "clean" because the
   `&&` chain short-circuited into an `||` fallback echo — the sweep itself was a false green.
   Fixed, and the sweep now loops over every script with an explicit failure echo.
2. **A crash impersonated a correct VOID.** Because the broken script exited 2 and 2 is also the
   *correct* VOID code, the empty-run case "passed" for the wrong reason. This is the
   right-answer-for-the-wrong-reason class, one level down from the incident this whole round is
   about. Closed by requiring each guard to emit **its own verdict voice** (`rederive: VOID`) in
   addition to the exit code — and by keeping the positive controls, which is what exposed it.

The lesson is now enforced in code, not written in a comment: **exit code alone is not a verdict.**

## What this does and does not settle

- **Settles:** the plan's six named false-green guards were 5 parts prose and 2 parts
  non-existent directory. Four of them are now executable and red-proven, and the tier→counter
  mapping that C-123 asserted against nothing now exists (`tier_counters.tsv`, 16 tiers).
- **Does not settle:** nothing about the *product*. No tier has run. `T6` still has zero executed
  upstream assertions; `T7` wiring is still absent; `T4` has still never been graded non-vacuously
  over a real conversation. Those are SG-4…SG-6.

## Next

**SG-2** — make the manifest executable (define `RUN_ROOT`/`EV`/`BASE`/`SRV`/`TARGET_SHA`, ship the
missing tool prerequisites, dry-run with zero undefined variables). Until that lands, every tier
command remains silently skippable.
