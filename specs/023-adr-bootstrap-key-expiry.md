# ADR-023: Bootstrap Admin Key Expiry — 90-Day Default TTL

**Status:** Accepted
**Date:** 2026-05-28
**Supersedes:** None

axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/023-adr-bootstrap-key-expiry.md

## Context

The bootstrap admin key is created on first run and printed to stdout. This key grants full admin access to the API. Because it is emitted to the terminal, it can leak into:

- Terminal scrollback buffers
- CI/CD job logs
- Shell history files (`~/.bash_history`, etc.)
- Screenshots or screen recordings
- Tmux/screen session logs

Prior to this decision, the bootstrap key was created with `expires_at = NULL` (never expires). This meant a leaked key remained valid indefinitely, creating an unbounded security exposure window.

The key is SHA-256 hashed in the database and cannot be recovered after creation, but the raw key is visible at bootstrap time and cannot be revoked retroactively if the operator does not act.

## Decision

Set the default bootstrap admin key TTL to **90 days** (2160 hours), configurable via `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS`. Setting the variable to `0` disables expiry (backward compatible).

## Rationale

### Why 90 days, not permanent

A permanent key means any leak at bootstrap time creates an immortal credential. 90 days bounds the exposure: even if the key leaks, it stops working after 90 days. This is long enough that operators are not pressured to rotate during initial setup and evaluation, but short enough to meaningfully limit damage.

### Why 90 days, not 30 days

30 days creates operational friction. Teams evaluating Conscience may not get around to creating replacement keys within a month, especially during proof-of-concept or trial deployments. A 30-day TTL would cause the bootstrap key to expire before the operator has finished onboarding, resulting in lockout and a bad first-run experience.

### Why 90 days, not 365 days

365 days is too close to "permanent" to be a meaningful security improvement. A key leaked into CI logs remains valid for a full year. 90 days is a compromise between security and operational convenience that aligns with common industry practices for credential rotation (e.g., AWS IAM access key age alerts at 90 days, many corporate policies require 90-day rotation).

### Why configurable (not hardcoded)

Different deployment topologies have different threat models. A single-developer local instance may prefer no expiry. A shared CI/CD deployment may want 7 days. Making the TTL configurable via an environment variable lets operators choose their own balance without code changes.

### Why `0` means no expiry (backward compat)

Existing deployments were created with permanent keys. Changing the default to "key expires" without an escape hatch would break those deployments. Setting `0` to mean "no expiry" means operators who want the old behavior can set `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS=0` and be unaffected.

## Consequences

- **New deployments** get a 90-day expiry on the bootstrap key by default. Operators see the expiry in bootstrap output and are nudged to create their own admin keys.
- **Existing deployments** are unaffected. Their bootstrap keys were created with `expires_at = NULL` and will continue to work. If they re-bootstrap (new database), the new key will have a TTL.
- **Auth middleware** requires no changes. The existing `(expires_at IS NULL OR expires_at > datetime('now'))` check already handles both cases.
- **Documentation burden**: bootstrap instructions should recommend creating a replacement key and optionally revoking the bootstrap key immediately.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Permanent key (status quo) | Unbounded exposure from stdout leak |
| 30-day TTL | Too aggressive for evaluation periods; causes lockout during onboarding |
| 365-day TTL | Security benefit marginal over permanent; leaked key valid for a year |
| Force immediate revocation | Requires interactive input at bootstrap time, breaks automation and headless deploys |
| Dedicated "bootstrap" key type | Adds complexity to key scope table and middleware; admin scope with TTL is sufficient |
| Auto-revocation after first replacement key is created | Surprising behavior; operator may intend to keep the bootstrap key as a fallback |

## Follow-up

- Monitor whether operators find 90 days too short or too long in practice.
- Consider adding a log warning at auth time when a bootstrap key is used past 30 days (advisory, not blocking).
- Consider adding a startup warning when `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS=0` is set, noting the security implications.
