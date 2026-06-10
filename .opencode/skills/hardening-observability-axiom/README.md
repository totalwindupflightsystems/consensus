# hardening-observability-axiom

Observability hardening for any codebase. Covers structured logging gaps, missing correlation IDs, SLI/SLO definition, OpenTelemetry instrumentation, Prometheus metric naming and cardinality, PII in logs, and the "3am debugging" checklist. Produces HARDEN-OBS-* findings with Tier-3+ verifiable acceptance criteria.

## When to Load

- Quarterly observability audit
- Before a major launch (can you debug it in production?)
- After a production incident where debugging was hard
- When setting up a new service
- As part of a hardening battery

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
