# runtime-serve-bootstrap-test-01 — Plan

Add automated regression coverage for the full serve bootstrap path so a future refactor cannot silently remove first-admin-key creation from local startup.

## Steps

1. Identify a test seam for serve startup that avoids binding a long-running server when possible.
2. If needed, extract a small helper from `runServer` for migration + first-admin-key bootstrap.
3. Add a test proving fresh DB startup creates one admin key and the key authenticates `/api/v1/sessions`.
4. Run targeted tests and `make test`.
5. Update `verification.md` with real output.

## Verification

- `go test ./internal/bootstrap -v -count=1`
- Targeted package test for the serve startup helper or subprocess path.
- `make test`

## Rollback

Revert the test/helper extraction. The existing runtime bootstrap behavior remains covered manually by `runtime-dev-bootstrap-auth-01`.

axiom:trace work_item=runtime-serve-bootstrap-test-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-serve-bootstrap-test-01/plan.md test=tbd evidence=.memory-bank/work-items/runtime-serve-bootstrap-test-01/verification.md
