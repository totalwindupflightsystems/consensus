# conformance-testing-loop

Behavior-first conformance loop for any software testing surface (CLI, API, SDK/library, workers, jobs, UI flows, integration paths). Execute real behavior, compare against spec/contract, and for every mismatch: add a failing regression test, apply the smallest fix, and rerun until full scope is green.

## When to Load

- You are validating behavior against specs/contracts across any surface
- You need a repeatable test -> fix -> retest loop for runtime or integration bugs
- You want full-scope conformance ("entire spectrum") without green theater
- You are working on code paths where unit-only proof is insufficient

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
