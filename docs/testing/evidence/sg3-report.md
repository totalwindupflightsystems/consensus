# SG-3 report — the manifest's checks are now decidable and effect-shaped

**Subgoal.** *Replace existence checks and undecidable criteria with effects.* (Merged tree §4, SG-3;
from Quorum #1 claims C1 and C4, both contradicted 5/5.)

## done-when → measured

| requirement | result |
|---|---|
| criteria carry a number or operator | **MET** — **0 undecidable** remain under the strict rule |
| the ~19 existence-only rows are turned into outcomes | **MET** — all 19 carry an effect criterion *and* a VOID condition, plus an `sg3_disposition` |
| the `forsifier` typo repaired and both keys audited | **MET** — 1 row → 0; its value folded into `falsifier` |
| a re-run of the shape census shows the risk set | **MET** — `verification_shape` is now a permanent, machine-readable field on all 284 rows |

Artifact: this file + the `sg3_repair` block inside the manifest.

## What changed

- **144 criteria rewritten** (originals preserved verbatim in `pass_criteria_original`, so nothing is
  lost and the transformation is auditable).
- **19 existence-shaped rows** rewritten to the *effect* they were pretending to measure, each with
  its VOID condition. The three that mattered most:
  - `C-036` (T6, the incident row): `"Both fields present and runner self-test passed"` →
    **"VOID unless the committed results artifact reports tests_executed > 0 for all 4 suites AND 0 rows
    classified SETUP_FAILURE; the pin/hash lines are a PREREQUISITE, never the pass condition."**
    The check that closed `C-GAP-032-ALPHA` on zero executed tests can no longer pass.
  - `C-039` (T7): `"Nonempty implementation-backed wiring census"` →
    **"count of crier-origin messages that became agent turns >= 1, else VOID; a wiring grep alone is not a pass."**
  - `C-066` (T1), `C-191` (T0), `C-015` (T2): listener/pin/prose checks → counted, VOID-able effects.
- **New fields on every row:** `verification_shape` (behaviour 63 / mixed 9 / existence 19 / other 193),
  `pass_criteria_decidable`, and on existence rows `sg3_disposition`.

## A defect this subgoal caught — in my own first repair

My v1 judged decidability on `expected` **+** `pass_criteria` together. That let `C-015`
("All DB/API assertions true") and `C-036` be declared *decidable* on the strength of a number that
lived in a **different field**, so they were never rewritten — a check that passes while nothing about
it changed, which is the exact defect C1/C4 name. Reverted and re-run with the strict rule:
**a criterion is decidable only if the criterion itself carries a digit or a comparison phrase.**
That moved the count from 52 to 144 rewritten and took the residue to 0.

## Honest limitations of this repair

1. **Decidability is guaranteed; specificity is not.** Rows whose command shape the deriver did not
   recognise (193 rows are classified `other`) received a decidable **floor** — `exit code == 0 AND the
   artifact is non-empty` — rather than a bespoke number. The original text is preserved beside it. The
   tier re-measurements (SG-8) are where per-row specificity gets tightened against real observations.
2. **The command strings were not rewritten**, only the criteria. A criterion that now demands an
   observed count is not the same as a command that *produces* it; producing it is SG-4…SG-7's work.
   The 19 existence rows still have grep-shaped commands until their tier subgoal lands.
3. **`other` (193) is a bag, not a clean bill.** It contains genuine assertions (`go build`, `git
   rev-parse`) *and* unclassified weak checks. The census makes the bag visible; it does not empty it.

## What did not move

The product. No tier has run. T6 still has zero executed upstream assertions, T7 wiring is still
absent, T4 still has never been graded non-vacuously. This pass made the plan's own promises
machine-checkable — it did not cash any of them.
