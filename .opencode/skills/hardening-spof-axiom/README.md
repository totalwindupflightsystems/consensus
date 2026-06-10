# hardening-spof-axiom

Single points of failure (SPOF) detection, blast radius analysis, and remediation patterns for any codebase. Covers missing timeouts, no circuit breakers, no replicas, no fallback paths, and shared resources that stop unrelated features. Produces HARDEN-SPOF-* findings with Tier-3+ verifiable acceptance criteria.

## When to Load

- Auditing a service for resilience gaps
- After a production outage caused by a dependency failure
- Before a major traffic event (launch, sale, migration)
- As part of a quarterly hardening battery
- When `chaos-engineer-axiom` identifies a missing fallback

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
