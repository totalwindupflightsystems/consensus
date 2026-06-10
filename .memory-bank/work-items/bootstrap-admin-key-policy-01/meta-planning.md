# bootstrap-admin-key-policy-01 — Meta Planning

## Summary

Bootstrap admin keys currently have `expires_at = NULL`, which is allowed by the schema but means a leaked first admin key is valid until revoked. This work item performs a small security policy decision and implements either expiry or explicit rotation guidance.

## Acceptance Criteria

1. A decision is recorded: permanent bootstrap key, default expiry, or configurable expiry.
2. If expiry is selected, implementation and tests enforce it.
3. If no expiry is selected, docs/verification explain manual revocation and risk.
4. Existing auth behavior remains compatible.

axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md,specs/005-security.md plan=.memory-bank/work-items/bootstrap-admin-key-policy-01/meta-planning.md evidence=.memory-bank/work-items/bootstrap-admin-key-policy-01/verification.md
