# runtime-serve-bootstrap-test-01 — Meta Planning

## Summary

The first-admin-key bootstrap fix is proven by manual live probes, but the full `consensus serve` wiring path is not protected by an automated regression test. This work item adds automated coverage for the startup sequence: config/database open, migrations, first admin key bootstrap, server handler, and authenticated protected endpoint probe.

## Scope

In scope:
- Add an automated test or script covering the full serve/bootstrap/auth path.
- Keep secrets redacted in test logs.
- Update verification evidence.

Out of scope:
- Redesigning key bootstrap policy.
- Postgres end-to-end coverage; that is `postgres-bootstrap-verification-01`.

## Acceptance Criteria

1. A regression test fails if `runServer` or equivalent serve startup no longer creates the first admin key.
2. The generated admin key authenticates a protected endpoint in the test.
3. The test does not persist or print the raw key in repo artifacts.
4. `go test ./internal/bootstrap -v -count=1` and relevant package tests pass.

axiom:trace work_item=runtime-serve-bootstrap-test-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-serve-bootstrap-test-01/meta-planning.md evidence=.memory-bank/work-items/runtime-serve-bootstrap-test-01/verification.md
