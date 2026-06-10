# hardening-sre-axiom

Reliability and SRE hardening for any codebase. Covers missing timeouts, retries without backoff, missing circuit breakers, rate limiting gaps, resource leaks (goroutines, connections, file descriptors), missing graceful shutdown, and swallowed errors. Produces HARDEN-SRE-* findings with Tier-3+ verifiable acceptance criteria.

## When to Load

- Auditing a service for reliability gaps
- After a production incident caused by missing timeout/retry/circuit breaker
- Before a major traffic event
- When `chaos-engineer-axiom` designs fault injection experiments
- As part of a quarterly hardening battery

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
