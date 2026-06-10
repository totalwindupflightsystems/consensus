# hardening-security-axiom

Security audit skill for any codebase. Covers injection vulnerabilities (SQL, command, template), authentication/authorization gaps, hardcoded secrets, input validation, CVE-vulnerable dependencies, and data handling (PII in logs, missing encryption). Complements security-review-axiom (STRIDE/threat model) — this skill focuses on code-level patterns. All findings require requires_human_review: true.

## When to Load

- Quarterly security audit of a codebase
- Before a major release or launch
- After adding new user-facing endpoints
- When a dependency CVE is reported
- When `security-review-axiom` flags a code-level concern

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
