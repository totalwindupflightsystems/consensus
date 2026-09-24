# Security Policy

## Reporting a Vulnerability

Consensus is a database-native AI agent harness. If you discover a security vulnerability, please report it responsibly.

**Do NOT open a public issue.** Instead, email the maintainer directly:

- Email: wojons@wojonstech.com

Please include:
- A detailed description of the vulnerability
- Steps to reproduce
- Affected versions
- Any potential mitigations you've identified

## Response Timeline

- **Initial response:** Within 72 hours
- **Status update:** Within 5 business days
- **Resolution target:** Within 30 days (depending on severity)

## Security Design

Consensus implements defense-in-depth security measures:

- **Row-Level Security (RLS):** Session-level data isolation
- **Append-Only Memory:** Immutable event log with REVOKE UPDATE/DELETE
- **Secret Scrubbing:** API keys and credentials are never logged or stored in plaintext
- **SQL Injection Prevention:** Parameterized queries throughout (no string concatenation)
- **Quarantine:** External inputs are validated and sandboxed before execution
- **Budget Enforcement:** Agent budget limits prevent runaway resource consumption

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| master  | ✅ Active development |

## Security Headers

The built-in web server applies:
- `Content-Security-Policy`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`

## Dependencies

Dependencies are pinned and audited. Supply chain security is enforced via GitReins pre-commit guards (secrets scanning, static analysis).
