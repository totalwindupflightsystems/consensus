# hardening-database-axiom

Database and data layer hardening for any codebase. Covers N+1 queries, connection pool exhaustion, transactions spanning HTTP calls, table-locking migrations, dual-write consistency, and resilience under DB failure. Migration findings always require requires_human_review: true. Produces HARDEN-DB-* findings with Tier-3+ verifiable acceptance criteria.

## When to Load

- Auditing a service with database interactions
- Before a major schema migration
- After a production DB performance incident
- When query latency is unexpectedly high
- When connection pool exhaustion is observed

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
