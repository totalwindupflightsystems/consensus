# hardening-quality-axiom

Test coverage gap detection for any codebase. Focuses on missing tests that would actually catch bugs — not just line coverage. Covers critical paths with no tests, error paths with no tests, edge cases, integration boundaries, assertionless tests, tautology tests, and flaky tests. Complements test-quality-gates-axiom. Produces HARDEN-QUAL-* findings with Tier-3+ verifiable acceptance criteria.

## When to Load

- Quarterly quality audit of a codebase
- After a production bug that tests should have caught
- Before a major release
- When `test-quality-gates-axiom` flags quality issues
- As part of a hardening battery

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
