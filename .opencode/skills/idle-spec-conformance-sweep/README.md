# idle-spec-conformance-sweep

Portable idle-time spec conformance sweep policy for any spec-driven project. When no unblocked work items remain (all TODO items are complete, credential-gated, or explicitly deferred), the loop does NOT stop. Instead it picks a random spec, audits the codebase for alignment, and either confirms conformance or creates remediation work items for discovered gaps. This keeps agents productively verifying and hardening the system during otherwise idle cycles.

## When to Load

- All TODO checkboxes are checked, credential-gated, or explicitly deferred
- No active steering packet or work item is unblocked
- The loop would otherwise declare `BLOCKED` or `stop`
- You want continuous spec-to-system alignment verification during quiet periods
- You are scaffolding a new Ralph loop and want to include idle-time productivity

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
