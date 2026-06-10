# bootstrap-admin-key-policy-01 — Plan

Decide whether first admin API keys should expire. The simplest safe path is to document the current permanent-key behavior and add rotation/revocation guidance unless product/security chooses default expiry.

## Steps

1. Review `specs/015-api-and-mcp.md` and `specs/005-security.md` for key lifetime requirements.
2. Choose policy: permanent, default expiry, or config-driven expiry.
3. Implement code/tests/docs matching the decision.
4. Verify create/list/delete key behavior still works.

## Verification

- Auth key tests in `internal/api`.
- Bootstrap tests in `internal/bootstrap`.
- `make test`.

axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md,specs/005-security.md plan=.memory-bank/work-items/bootstrap-admin-key-policy-01/plan.md evidence=.memory-bank/work-items/bootstrap-admin-key-policy-01/verification.md
