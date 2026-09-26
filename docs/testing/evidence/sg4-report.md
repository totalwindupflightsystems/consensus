# SG-4 report — T6 has a real reading, and it is RED

**Subgoal.** *T6 — prove the pinned upstream suite EXECUTES tests.* (Merged tree §4, SG-4; the tier all five
Quorum #1 seats named as riskiest; the `C-GAP-032-ALPHA` incident.)

## done-when → measured

| requirement | result |
|---|---|
| the runner stops deleting its results | **MET** — `rm -f "$RESULTS"` removed from the last line; the file now survives into the evidence dir |
| a COMMITTED results file with 4 suite rows, `tests > 0`, classification != SETUP_FAILURE | **MET** — `docs/evidence/opencode-upstream-v1.18.29-r2/results.tsv`, 4 rows, **tests=34**, classifications {DIVERGENCE ×3, PASS ×1} |
| counts parsed from raw bun output, never hand-typed | **MET** — and the parser had to be fixed first (below) |
| otherwise T6 is honest RED and `C-GAP-032-ALPHA` stays open | **MET** — T6 **is** RED: 10 upstream assertions fail. The row stays open |

## The reading (first real one in the project's history)

| # | upstream suite | tests | pass | fail | class |
|---:|---|---:|---:|---:|---|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 7 | 0 | **7** | DIVERGENCE |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 18 | 17 | **1** | DIVERGENCE |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 2 | 0 | **2** | DIVERGENCE |
| 4 | `packages/client/test/promise.test.ts` | 7 | **7** | 0 | PASS |
| | **total** | **34** | **24** | **10** | |

Run against a live shim: scratch instance on port 18232 with `adapters.opencode.enabled: true`, pinned checkout
`16747470f976aca3d362ad730bcd3fe82ecc2c9a` (v1.18.29), bun 1.4.2, node 22.

## Two runner defects were hiding this — both fixed here

1. **It deleted its own results.** `rm -f "$RESULTS"` ran after the summary was built, so no counts could ever be
   committed. This is the mechanical reason the earlier closure had nothing to point at.
2. **It misread bun and laundered a divergence as a setup problem.** The counts were parsed with line-anchored
   regexes (`^([0-9]+) pass`) while bun prints its tail block **indented** (`" 7 pass"`), so `pass`/`fail` were
   always 0 — and `classify_suite_result`, seeing `fail=0` with a nonzero exit, fell through to `SETUP_FAILURE`.
   A suite that ran 7 assertions and failed all 7 was published as *"was not run"*. A suite that genuinely passed
   was published with `0/0` counts. Fixed: `^[[:space:]]*` anchors, plus a `RUN_ERROR` class for
   tests-collected-but-no-results, so DIVERGENCE can no longer be laundered.

This is the **unrepresentable-failure** class: if the store cannot record the real state, every downstream count
lies. It mattered here because "SETUP_FAILURE" and "DIVERGENCE" lead a reader to opposite conclusions.

## The gaps, filed rather than summarised

`DF-CONSENSUS-36` `GET /doc` serves the Swagger HTML page where upstream requires the OpenAPI document ·
`DF-CONSENSUS-37` the shim returns **401 where upstream expects 200** on fixed-workspace read/write endpoints
(auth fires before routing — the same precedence class already fixed once for MCP) ·
`DF-CONSENSUS-38` path/VCS read endpoints **404** where upstream expects 200 (the 501 stubs, DF-18) ·
`DF-CONSENSUS-39` SDK prompt-context assertion (1 of 18) ·
`DF-CONSENSUS-40` v2 error-shape derives **404 where it should be 400** ·
`DF-CONSENSUS-41` the runner reporting defect above.

## What did not move

- **The compatibility gaps themselves.** T6 is measured RED; nothing was fixed in the shim during this pass.
- **`C-GAP-032-ALPHA` stays open**, now backed by a real reading instead of a void one.
- **T7 / T4 / T10 are still unmeasured** (SG-5/SG-6/SG-7).

What *did* move is the quality of the claim: "upstream-verified" is now either falsifiable (34 tests, 10 failures)
or it is not claimed at all.
