# Quorum #1 — merged verdicts, Goal + Falsifier, and the subgoal tree

Round: `quorum-consensus-test-plan-STRESS-v2` · 2026-09-26 · 5 seats / 5 families, 5/5 landed.
Subject: the consensus full test plan v1 (284 tests) — is its step list fit to become
subgoals with verification? Coordinator: this session (the commit layer).

## 1. Kill ledger — dispositions after the coordinator's raw-data re-check

| Claim | Verdict | Converged? | Coordinator re-verified against raw data |
|---|---|---|---|
| C1 effect-not-existence | **CONTRADICTED** | 5/5 | CONFIRMED. C-036 grades string presence in a `summary.md`; C-039 `grep`s for crier wiring; C-066/067 grade a listener/spec inventory; C-076 grades serve-log strings ("anti-mock" by marker); C-191 greps `git log` for a commit. |
| C2 false-green guards are declarations | **CONTRADICTED** | 5/5 | CONFIRMED, and worse than argued: `scripts/battery/` **does not exist** while the manifest references it **25×**; `scripts/battery/rederive.sh` ABSENT. |
| C3 commands not executable as written | **CONTRADICTED** | 5/5 | CONFIRMED. **Zero** variable assignments in plan+manifest while `$RUN_ROOT` ×117, `$EV` ×114, `$SRV` ×24, `$BASE` ×17, `$KEY` ×14, `$ADMIN_KEY` ×6, `$RUN_ID` ×6 are used. `yq` ABSENT, `hey`/`ab` ABSENT on the run host. |
| C4 undecidable pass_criteria | **CONTRADICTED** | 5/5 | SETTLED at the primary source (seats split 89 vs 189): **84** tests carry no digit at all in expected+criteria; **69** carry neither a digit nor a comparison/numeric phrase — that 69 is the strict undecidable set. |
| C5 surface coverage | SPLIT (2 CONTRADICTED, 2 ACCEPTED-with-gaps) | no | AMENDED: the mapping is sound but the shortfall sits exactly on the owner's asks — MCP attach (T5 sweep FAIL 404/400), crier (T7 unrun, wiring absent), baby project (T10 blocked at the first POST by a 400), tool-audit (T3 `tool_requests=0`), T4 measured-vacuous. |
| C6 subgoal convertibility | **ACCEPTED** (4 seats produced sketches; 1 produced them under a CONTRADICTED label on the *direct-conversion* reading) | yes | ACCEPTED with that amendment: the sketches are the deliverable and they converge (below). |
| C7 repeat-offense | SPLIT (2 CONTRADICTED, 2 ACCEPTED-with-recurrences) | no | AMENDED — recurrences are real and named: `Bearer ***` literals in commands, success-marker greps, `\| tee` making the log the evidence, undefined vars as silent-skip. |
| C8 the incident | **ACCEPTED** | 4/4 | CONFIRMED by my own check: `16/18`, `2/2`, `7/7` exist **only** in `.coding-hermes/board/{events,tasks}.jsonl` (+ a `.bak`) — in no evidence artifact. Mechanism agreed: C-036's pass_criteria ("both fields present") is satisfied by an evidence file describing a dead run, and the plan binds "no artifact = FAIL" to the report, not to closure. |

**Merged fix surface (from C8, all seats agreeing):** bind an executor and a closure check to
the artifacts — a tier may not be closed without a committed results file carrying **tests>0**.

## 2. New verified facts established this round (not in the brief)

1. `docs/testing/consensus-test-manifest-v1.json` carries **both** `falsifier` and a **`forsifier`**
   key — a misspelled duplicate. Any tool reading `falsifier` silently misses the rows that use
   the typo. (Coordinator, V3.)
2. `scripts/battery/**` — referenced 25×, exists 0×. Two of the six named false-green guards
   (C-187, C-230) live in that absent directory. (Coordinator, V2; seats 3 and 4 independently.)
3. The strict undecidable set is **69/284**, not 89 or 189. (Coordinator, V5 — resolving the
   seats' disagreement at the source.)
4. The suite counts quoted at closure exist only in board JSONL. (Coordinator V6; seat 2 first.)
5. Seat 5 found the runner itself deletes its own results (`scripts/test-opencode-upstream.sh:327`
   `rm -f "$RESULTS"`), which is part of why no counts were ever committed.

## 3. Stage 2 — GOAL + FALSIFIER

**GOAL.** A committed evidence bundle in which every tier reports a **nonzero observable count**
and each of the seven capabilities the product promises — real LLM answers, real multi-turn agent
loops, real session tracking, real tool calls the agent itself issues, a fake "baby" project driven
end to end, MCP access by an external agent, and a message received through crier — is proven by an
**effect on live data**, with the plan's own false-green guards **proven able to fail** before any
green is trusted.

**FALSIFIER.** Any tier closes with zero executed tests (the C-GAP-032-ALPHA shape), or any named
capability's evidence is an existence check where an effect was claimed. Meeting the falsifier does
not fail the project — it fails the *run*, which re-opens as new subgoals.

## 4. Stage 3 — the subgoal tree (merged from five seats' sketches)

Ordered smallest-decisive-first; the first four are preconditions, so they precede the tiers.

```
SG-1  Make the guards executable, and prove they can fail            [C2 · 5/5 · kill-early]
      done-when: C-122/C-123/C-187/C-230 exist as real scripts that exit nonzero; run against
                 a stub tier with 0 observables they report VOID (C-123) and exit 2 (C-187);
                 both red-proofs captured.
      evidence:  docs/testing/evidence/sg1-guard-redproof.txt
      depends-on: none
      falsifier: a guard returns PASS on the stub -> no green from this plan is trustworthy.

SG-2  Make the manifest executable, then freeze the run                [C3 · 5/5]
      done-when: RUN_ROOT/EV/BASE/SRV/TARGET_SHA and the guard scripts are defined and shipped;
                 `grep -cE '^[A-Z_]+=' ` > 0; every referenced tool resolves on the run host
                 (yq + a load tool added); manifest commands dry-run without an undefined var.
      evidence:  docs/testing/evidence/sg2-env.txt · the env file itself, committed
      depends-on: SG-1
      falsifier: any command still references an unset variable -> it is silently skippable.

SG-3  Replace existence checks and undecidable criteria with effects   [C1+C4 · 5/5]
      done-when: the 69 undecidable criteria carry a number or operator; the ~19 existence-only
                 rows are rewritten as outcomes; the `forsifier` typo key is repaired and both
                 keys audited; a re-run of the shape census shows existence-only = 0.
      evidence:  docs/testing/evidence/sg3-census-after.txt (same script as the Stage-1 census)
      depends-on: SG-2
      falsifier: a rewritten row can still pass while nothing observable changes.

SG-4  T6 — prove the pinned upstream suite EXECUTES tests           [5/5 · the incident tier]
      done-when: the runner stops deleting its results (script:327); a COMMITTED results file
                 lists 4 suites each with tests>0 and classification != SETUP_FAILURE; counts
                 parsed from raw bun output, never hand-typed. Otherwise T6 is honest RED and
                 C-GAP-032-ALPHA stays open — the tier may not be greened by the Go port.
      evidence:  docs/evidence/opencode-upstream-v1.18.29-r2/{summary.md,results.tsv,4 logs}
      depends-on: SG-1 (a red-proof exists), SG-2
      falsifier: all four suites still collect 0 tests from correct package roots -> re-scope
                 the row to "the shim cannot host upstream suites" instead of cycling forever.

SG-5  T7 — crier go/no-go BEFORE any spend                             [4/5]
      done-when: wiring census; if absent (expected today) T7 is marked EXPECTED-FAIL pinned to
                 CR-IN-001 in writing and excluded from the GREEN gate; if present, one delivered
                 nonce appears as a crier-origin user_message with a matching ack.
      evidence:  docs/testing/evidence/sg5-t7-census.txt (+ t7-intake.json if wired)
      depends-on: none
      falsifier: the census is 0 and CR-IN-001 is pending -> running T7 is phantom wiring; delete
                 it from the GREEN gate, never silently skip.

SG-6  T4 — non-vacuous grading over a real conversation                [3/5]
      done-when: one real round trip yields session status idle with tokens>0 AND >=1
                 agent_billing row with cost>0; the sweep driver is fixed to print audited_targets
                 and to FAIL on a non-idle session.
      evidence:  docs/testing/evidence/t4-rerun/t4.json (same schema as tier-sweep-v2.json)
      depends-on: SG-2 (live-key preflight)
      falsifier: the driver again reports rows over zero conversations -> the RUNNER is the
                 defect; file the runner row, do not grade the product.

SG-7  T10 — the fake "baby" project end to end                          [2/5]
      done-when: the exact task-create body is documented and returns 201 (today's 400 is a
                 string-vs-object mismatch — print the body); then a goal session produces a file
                 whose sha256 appears in a tool_results payload and the file is non-empty.
      evidence:  docs/testing/evidence/sg7-t10/{t10-ledger.json,artifact}
      depends-on: SG-2
      falsifier: task-create still 400s -> file the DF row and xfail-pin the API leg; never green.

SG-8..SG-14  Re-measure the remaining tiers on the frozen SHA, each against the 2026-09-24 sweep
             baseline: T0 (build/unit) · T1 (API contract vs SERVED spec — decides whether
             DF-CONSENSUS-18's fix closed the 60-vs-59 drift) · T2 (live LLM + MULTI-TURN) ·
             T3 (tool calls END TO END, agent-issued — the ledger must show the agent initiating,
             not merely a 200 from /execute) · T5 (MCP surface + a REAL client attach, not curl
             alone) · T8 (dead-key breaker + restart) · T9 (bounded load against the load gate).
             Each: done-when = the tier's counters all nonzero; evidence = its artifact path.

SG-15  GATE + the evidence bundle: defect gate, VOID detection, secret hygiene, and a
       single-file HTML report carrying per-claim RECOMPUTE commands that a third party can run.

SG-LAST  RE-EVALUATE  (never consumed by running it)
       done-when: the Stage-1 measurement is re-run — tier sweep + board census + CI — and
                  compared before/after, with every subgoal's claim checked against the artifact
                  that should now carry it, and an explicit "what did not move" section. On any
                  failure the tree re-opens with the finding subgoals added and THIS subgoal
                  re-appended. Then Quorum #2 verifies the result, not the intent.
       evidence:  docs/testing/evidence/sg-last-reevaluation.md
       depends-on: SG-15
```

## 5. Merge dispositions

- **Accepted as-is:** the four convergent contradictions (C1–C4) and C8; all five seats' C6 sketches
  (merged into SG-1…SG-7).
- **Accepted-amended:** C5 and C7 — the seats split; the underlying recurrences are real, the
  "coverage is fine" reading is not.
- **Rejected:** the framing that the plan is a runnable harness. It is a *design document*; the
  verified fact is that **as written it cannot execute**, which is precisely why TEST-CONSENSUS-001
  failed twice and why C-GAP-032-ALPHA could close on zero tests. That rejection is what makes SG-1
  and SG-2 preconditions rather than tier work.
- **Seat 5's C6 note** (the runner deletes its own results file) is adopted as a hard requirement in
  SG-4, not a footnote.
