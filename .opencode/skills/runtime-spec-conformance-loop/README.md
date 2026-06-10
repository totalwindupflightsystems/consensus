# runtime-spec-conformance-loop

Real runtime conformance loop for any surface (CLI, HTTP API, SSE, background jobs, worker daemons, adapters). Execute actual behavior first, compare against specs, and for every mismatch: add a failing regression test, apply the smallest fix, and rerun runtime plus tests until the full target spectrum is green.

## When to Load

- You are validating real behavior against specs (not just unit-level correctness)
- You need a repeatable fix loop for runtime bugs
- You want to cover a complete surface area ("entire spectrum") without green theater
- You are working across mixed surfaces, not only CLI

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
