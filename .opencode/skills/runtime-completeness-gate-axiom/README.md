# runtime-completeness-gate-axiom

Prevent "looks built" from being confused with "actually works". Forces the last-mile runtime wiring, operator-path proof, and closure checks that catch nil executors, unregistered routes, stubbed adapters, demo-only completion claims, cross-path wiring gaps where subsystems work independently but fail when combined, and verification theater where tests prove component shape but not system behavior.

## When to Load

Load this skill when:
- a work item is nearing completion
- a system has many green tests but no trusted end-to-end proof
- a CLI/server/app has multiple partially integrated subsystems
- the repo has evidence of "candidate technical proof" without accepted runtime proof

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
