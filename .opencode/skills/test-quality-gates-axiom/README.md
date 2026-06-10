# test-quality-gates-axiom

Portable test quality gate workflow for Axiom. Enforces that high coverage is backed by high-signal tests — no assertionless tests, no tautologies, Tier-3+ runtime evidence required. Includes Test Value Doctrine to distinguish value-producing tests from green theater. Load this skill when writing tests, reviewing test quality, or verifying a work item step is "done". Applies to every repo managed by Axiom.

## When to Load

Load this skill when:
- Writing new tests for any work item step
- **Planning tests during meta-planning or implementation planning** (NEW — REQ-TQ-011)
- Reviewing whether a step is "done" (verifier gate)
- Running `/axiom-verify` on any code change

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
