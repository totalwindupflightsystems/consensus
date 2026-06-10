# api-rate-limit-scope-01 — Meta Planning

## Summary

Red-team review found that API rate limiting appears to use the admin limit for every key scope. SPEC-015 defines different limits by scope, so session, readonly, and webhook keys may currently be too permissive.

## Acceptance Criteria

1. Rate limiting uses the authenticated key scope, not a hardcoded admin limit.
2. Tests prove admin/session/readonly/webhook limits are selected correctly.
3. Existing auth middleware behavior remains compatible.
4. `make test` passes.

axiom:trace work_item=api-rate-limit-scope-01 spec=specs/015-api-and-mcp.md plan=.memory-bank/work-items/api-rate-limit-scope-01/meta-planning.md evidence=.memory-bank/work-items/api-rate-limit-scope-01/verification.md
