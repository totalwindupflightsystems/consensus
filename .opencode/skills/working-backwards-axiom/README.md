# working-backwards-axiom

Plan from the end-user experience backward to implementation details. Every plan step includes not just "what to build" but "how to verify it's connected end-to-end." Prevents the common AI failure mode where components are built in isolation and never wired together. Companion to runtime-completeness-gate-axiom: this skill PREVENTS wiring gaps at planning time; the gate DETECTS them after implementation.

## When to Load

Load when:
- Creating a new work item's `meta-planning.md` or `plan.md`
- Planning any feature that has a user-visible surface (CLI, API, UI, worker output)
- Planning any feature that crosses subsystem boundaries
- Planning any feature with multiple write paths or read paths

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
