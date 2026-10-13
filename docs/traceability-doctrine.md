# Traceability Doctrine

This document defines the repository-local `ch:trace` contract for Consensus contributors and auditors. It applies to claims recorded in `.coding-hermes/board/tasks.jsonl`, including implementation, documentation, audit, and verification work.

## Contract

A trace marker is a compact link from a board claim to the contract, verification, observation, judgement, and Git tree that support it. The required fields are:

```text
ch:trace row=<board-id> spec=<contract-locator> test=<test-locator> evidence=<artifact-locator> witness=<observation-or-none:reason> verdict=<verdict-id-or-none:reason> commit=<sha-or-none:reason>
```

The fields are space-delimited `key=value` tokens. Values must not contain unescaped spaces. Use repository-relative paths where the artifact is committed, and add an anchor or test name when one is available. A field that is genuinely unavailable must still be present as `none:<reason>`; omission is not a statement.

The marker fields mean:

| Field | Required meaning |
|---|---|
| `row` | The exact board row ID that owns the claim. |
| `spec` | The contract or requirement locator, normally `path#anchor`. For documentation-only work, the target document and section may be the contract. |
| `test` | The executable check, named test, or an explicit non-code check such as `none:documentation-audit`. |
| `evidence` | The artifact or command output that records what was measured. This must be direct evidence, not another board row. |
| `witness` | An independently observable surface and locator, or `none:<reason>` when no witness was available. |
| `verdict` | The final judgement identifier, or an explicit pre-judgement value such as `none:not-judged` while the row remains open. |
| `commit` | The exact commit containing the claimed state, or an explicit pre-commit value while the row remains open. |

Consensus already uses this grammar. `DOC-14` has all seven required fields in `.coding-hermes/board/tasks.jsonl:398`; it uses `test=none:documentation-audit`, the documentation audit as `evidence`, and explicit `none:` reasons before judgement and commit. `DOC-17` uses the same complete pre-work shape at `.coding-hermes/board/tasks.jsonl:401`. `ROUTE-FIX-032` demonstrates a code-oriented marker with a spec anchor and named Go test at `.coding-hermes/board/tasks.jsonl:253`.

Existing markers sometimes include established optional fields such as `wave`, `doc`, `prompt`, or `memory`. They may add context, but they do not replace any of the seven required fields above. Do not introduce a new field merely to avoid filling a required one.

The source audit for this doctrine is `/home/kara/.hermes/stand-in/docs/consensus/consensus-doc-audit-2026-10-11.md`: its Findings section records that the repository-local doctrine was absent (lines 245-252), and its marker scan records the observed marker shapes (lines 259-284).

## Evidence tiers

Use the lowest tier that accurately describes what was demonstrated. Never describe a higher tier because a lower-tier check passed.

| Tier | Meaning | Consensus example |
|---|---|---|
| T0 — exists | A file, symbol, row, or artifact exists. No wiring or behavior has been demonstrated. | The documentation audit's `test -e` checks established that this file and `TRACEABILITY.md` were absent; that was an existence check, not behavioral proof (`consensus-doc-audit-2026-10-11.md`, Finding `DOC-17`). |
| T1 — imports | The thing is referenced, imported, registered, or otherwise wired into a consumer. It has not yet been exercised. | The audit's source inventory derived API registrations from `internal/api/server.go` and `internal/api/openapi.go` (audit lines 11-54). That proves registration, not endpoint behavior. |
| T2 — unit-tested | A focused test exercised the relevant unit or contract arm. | `ROUTE-FIX-032` names `internal/shim/opencode/server_test.go::TestVCSRawDiffServesDeclaredContract` in its trace marker (`tasks.jsonl:253`). The test locator is stronger than source presence, but by itself is not a live invocation. |
| T3 — invoked | The contributor ran the relevant command, probe, audit, or user-facing path and retained its result. | The documentation audit labels itself `TIER: T3` after executing its source inventory, documentation diff, link sweep, board screen, and marker scan (audit lines 286-288). |

A claim may be marked done only when its trace records T3 invocation evidence or explicitly records why an independent witness does not exist with `witness=none:<reason>`. The `none:` form makes the boundary visible; it does not transform T0-T2 evidence into T3 and does not waive the other close-out requirements.

The audit itself shows the intended distinction: it reached T3 for the documentation checks while explicitly holding back live endpoint claims because no Consensus server was started (audit lines 254-257). Its `DOC-14` through `DOC-19` markers therefore use `witness=none:live-server-not-started` or another specific absence reason rather than implying a live witness (audit lines 279-284).

## Witness rules

A witness records what can be observed independently of the prose that makes the claim. It must be specific enough for an auditor to locate or repeat the observation.

1. Prefer a concrete surface and locator, such as a live endpoint, CI run, committed artifact, or retained command output.
2. Do not use the claim itself as its witness. A board row pointing to another board row is lineage, not observation.
3. When no independent witness was available, write `witness=none:<reason>`. Silence is invalid, and `witness=none` without a reason is incomplete.
4. State the boundary precisely. `none:live-server-not-started` is useful; `none:unavailable` is not.
5. A witness supplements evidence; it does not replace `evidence`, `test`, `verdict`, or `commit`.

The repository contains both witness shapes. `QA-CONSENSUS-26` records a live bunker witness together with a named test and retained QA evidence at `.coding-hermes/board/tasks.jsonl:389`. `DOC-14` records `witness=none:live-server-not-started` because its audit did not cross the live server boundary (`tasks.jsonl:398`). The audit's Unverified section confirms that limitation rather than upgrading source inspection into endpoint proof (`consensus-doc-audit-2026-10-11.md`, lines 254-257).

## Close-out requirements

Before changing a row to a terminal state, the contributor or auditor must verify all of the following:

- The marker contains `row`, `spec`, `test`, `evidence`, `witness`, `verdict`, and `commit` exactly once with non-empty values.
- `row` matches the board row being closed.
- `spec`, `test`, and `evidence` resolve to the actual contract, check, and artifact. Evidence must be direct; another row, a planned command, or a success sentence is not evidence.
- The evidence reaches T3, or the absence of an independent witness is stated as `witness=none:<reason>` without concealing the evidence tier.
- `verdict` names the final judgement. A close-out marker must not retain `verdict=none:not-judged`.
- `commit` names the exact Git tree judged and closed. A close-out marker must not retain `commit=none:not-committed`.
- The witness is reproducible or its absence reason is still true at close-out.
- The row is not closed only because another row, report, or verdict says it is complete. Re-run or inspect the underlying check.
- A recursive marker scan and a five-row spot check confirm the required fields are present before the close-out is recorded.

The last rule is already explicit in the board: `DOC-19` requires independent re-evaluation of `DOC-16`, `DOC-17`, and `DOC-18` and says not to close by reference alone (`.coding-hermes/board/tasks.jsonl:403`). The audit reached the same conclusion by reporting 22 current-surface markers, only 7 with complete fields and 15 incomplete at the time of that scan (`consensus-doc-audit-2026-10-11.md`, lines 259-278). A marker's presence is therefore not a completion verdict.

Use this close-out scan from the repository root:

```bash
grep -rn "ch:trace" .coding-hermes/
```

For each sampled current claim, verify the seven required tokens in the marker itself rather than inferring them from neighboring JSON fields. `DOC-14`, `DOC-15`, `DOC-17`, `DOC-18`, and `DOC-19` are five complete current examples in `.coding-hermes/board/tasks.jsonl:398-403`; each carries all seven required fields, including explicit `none:` values for unfinished judgement or commit state.

## Current baseline and audit handling

This doctrine is prospective and does not silently rewrite board history. On the DOC-17 worktree, the required recursive grep returned 57 matching lines. A machine check of open or pending rows containing `ch:trace` found 10 such rows: the five documentation-audit claims named above contained all seven required fields, while five older open claims (`QA-CONSENSUS-25`, `QA-CONSENSUS-28`, `README-4`, `README-5`, and `README-6`) lacked `spec`, `test`, `verdict`, and `commit` tokens.

Those five gaps remain visible because DOC-17 is forbidden from editing `.coding-hermes/` state. Auditors must not interpret this doctrine's existence as a retroactive repair. The earlier documentation audit likewise reported incomplete markers instead of normalizing them away (`consensus-doc-audit-2026-10-11.md`, lines 259-278). Board owners should bring an open claim up to this contract when they next amend or close it.
