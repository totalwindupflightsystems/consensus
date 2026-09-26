# scripts/battery — the false-green guards (SG-1)

These four guards were **prose** in the test plan (Quorum #1, claim C2, contradicted 5/5) and
two of them pointed at a directory that did not exist while the manifest referenced it 25×.
They are now executable, each with a real exit contract, and each red-proven.

## Exit contract (shared)

| code | meaning |
|---:|---|
| `0` | the check passed, **on at least one real observation** |
| `1` | RED — a real failure / malformed input |
| `2` | VOID — nothing was observed, so nothing may be called green |

`VOID` is the load-bearing one. A tier that measured nothing is not green; a battery cell that
audited zero targets is not green; a board whose defect list cannot be read is not green. This is
the class that let `C-GAP-032-ALPHA` be closed with zero upstream tests executed.

## The guards

- **`guard_void.sh <results.json> [tier_counters.tsv]`** — C-123. A tier with zero observables is
  VOID. Reads `tier_counters.tsv`, the tier→counter mapping the plan asserted against *nothing*
  (the manifest had no counter field at all).
- **`zero_target.sh <audited_targets> [label]`** — C-187. Exits 2 when a step audited zero targets,
  and also when the count is missing or non-numeric.
- **`defect_gate.sh <board.jsonl> <ids-file> `** — C-122. Re-reads the board **last-wins** for the
  run-blocking defect list and emits GREEN / RED / VOID *with* an exit code. An id absent from the
  board is VOID, never GREEN — you cannot judge what you cannot find.
- **`rederive.sh <run-dir>`** — C-230. Third-party re-derivation: recomputes the headline numbers
  from the artifacts and pins each to a sha256. An empty run dir is VOID.
- **`redproof.sh`** — the deliverable of SG-1. Runs every guard against deliberately bad input and
  a positive control, asserting **both** the exit code **and** the guard's own verdict voice.

## Usage

```bash
bash scripts/battery/redproof.sh                     # prove the guards can fail
bash scripts/battery/guard_void.sh  <results.json>   # VOID any tier with 0 observables
bash scripts/battery/zero_target.sh <n> "<label>"    # VOID a zero-audit step
bash scripts/battery/defect_gate.sh .coding-hermes/board/tasks.jsonl ids.txt
bash scripts/battery/rederive.sh    <run-dir>
```

Requires `jq` on the run host (declared, not assumed — see PLAN-Q1-002).
