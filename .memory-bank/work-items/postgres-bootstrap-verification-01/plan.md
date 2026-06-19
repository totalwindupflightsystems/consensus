# postgres-bootstrap-verification-01 — Plan

Prove first-admin-key bootstrap works on Postgres, not just SQLite. Prefer an opt-in integration test gated by `CONSENSUS_TEST_POSTGRES_URL` so normal local tests remain fast.

## Steps

1. Check existing test harness conventions for Postgres env vars.
2. Add a gated Postgres integration test or document why it cannot be added.
3. Verify key creation, hashed storage, existing-key no-reprint, and authenticated protected endpoint.
4. Update evidence.

## Verification

- `CONSENSUS_TEST_POSTGRES_URL=... go test ./internal/bootstrap -run Postgres -v -count=1` when available.
- `go test ./internal/bootstrap -v -count=1`.
- `make test`.

axiom:trace work_item=postgres-bootstrap-verification-01 spec=specs/015-api-and-mcp.md,specs/016-cli-interface.md plan=.memory-bank/work-items/postgres-bootstrap-verification-01/plan.md evidence=.memory-bank/work-items/postgres-bootstrap-verification-01/verification.md
