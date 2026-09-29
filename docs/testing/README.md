# Testing and QA

This directory is the entry point for Consensus's executable QA doctrine. It indexes the test design, machine-readable cases, quorum review, and evidence without duplicating the full plan.

## Core documents

- [Full test plan v1](consensus-full-test-plan-v1.md) — the canonical 284-test plan. Its primary execution tiers run from T0 (build/unit) through T10 (project integration), followed by defect-gate and setup/support tiers; each tier defines proof obligations and points to per-test detail in the manifest.
- [Machine-readable manifest](consensus-test-manifest-v1.json) — commands, expected results, artifacts, pass criteria, falsifiers, defect gates, and proposer attribution for all 284 canonical tests.
- [Merged quorum verdicts](quorum-q1-merged-verdicts.md) — the five-seat stress review, reconciled findings, goal/falsifier, and the ordered subgoal tree used to harden the plan.

## Evidence layout

- [`evidence/`](evidence/) contains committed subgoal reports and machine artifacts, including the SG-1 guard red-proof, SG-2 environment evidence, SG-3/SG-4 reports, and the recorded tier sweep.
- [`quorum-q1-2026-09-26/`](quorum-q1-2026-09-26/) contains the quorum brief and raw seat logs behind the merged verdicts.
- Some plan rows name run-specific evidence outside this directory. Follow the path recorded by that row rather than treating this index as a claim that every tier has run.

## Run the battery guards

Run commands from the repository root. Start with the self-contained red-proof:

```bash
bash scripts/battery/redproof.sh
```

It builds deliberately good and bad fixtures, checks the guards' verdict text and exit codes, and needs no project server or API key. A successful run ends with `11 ok, 0 broken` and exits 0.

The shared verdict convention is **0 = pass/green**, **1 = RED/failure**, and **2 = VOID/cannot establish a result**. Each script header is authoritative; helpers that cannot produce a meaningful RED use only 0 and 2.

| Script | Purpose and invocation | Exit contract |
|---|---|---|
| [`redproof.sh`](../../scripts/battery/redproof.sh) | `bash scripts/battery/redproof.sh` — prove the guards reject bad fixtures and accept controls | 0 all checks correct; 1 one or more guards broken |
| [`guard_void.sh`](../../scripts/battery/guard_void.sh) | `bash scripts/battery/guard_void.sh <results.json> [tier_counters.tsv]` — reject tiers with zero observables | 0 observed; 1 malformed/unevaluable input; 2 VOID |
| [`zero_target.sh`](../../scripts/battery/zero_target.sh) | `bash scripts/battery/zero_target.sh <audited_targets> [label]` — reject a zero-target audit | 0 one or more targets; 2 VOID |
| [`defect_gate.sh`](../../scripts/battery/defect_gate.sh) | `bash scripts/battery/defect_gate.sh <board.jsonl> <ids-file>` — evaluate run-blocking defects with last-wins board semantics | 0 GREEN; 1 RED/open defect; 2 VOID |
| [`rederive.sh`](../../scripts/battery/rederive.sh) | `bash scripts/battery/rederive.sh <run-dir>` — recompute counts and hashes from evidence | 0 artifacts re-derived; 2 VOID |
| [`run-env.sh`](../../scripts/battery/run-env.sh) | `. scripts/battery/run-env.sh` — define the manifest's run environment; use `require_run_state VAR...` before stateful tiers | helper returns 0 ready; 2 VOID |
| [`require_tools.sh`](../../scripts/battery/require_tools.sh) | `bash scripts/battery/require_tools.sh` — verify required host tools | 0 ready; 2 VOID |
| [`dryrun_manifest.sh`](../../scripts/battery/dryrun_manifest.sh) | `bash scripts/battery/dryrun_manifest.sh [manifest.json]` — verify every manifest variable is declared | 0 resolved; 2 VOID |

Do not interpret a VOID as a skipped pass: it means the run lacks enough evidence to be called green.
