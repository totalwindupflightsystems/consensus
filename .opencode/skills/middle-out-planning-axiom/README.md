# middle-out-planning-axiom

Middle-Out Implementation Planning for AI agents. Start from the critical integration boundary — the place where the most risk and uncertainty lives — prove it works first, then expand outward in both directions. Prevents the two classic AI failure modes: top-down isolation (components built separately, never wired) and bottom-up avoidance (easy parts built first, hard integration deferred until context runs out).

## When to Load

Load when:
- Planning a feature that crosses system boundaries (API ↔ DB, frontend ↔ backend, agent ↔ runtime)
- Starting a new work item where the integration point is unclear or risky
- A previous implementation had wiring gaps caught by the runtime-completeness-gate
- The plan has multiple phases and you're deciding where to start

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
