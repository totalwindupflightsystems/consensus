# postgres-bootstrap-verification-01 — Accepted Gap Evidence

**Date**: 2026-06-06
**Status**: Accepted Gap — Postgres-specific bootstrap test deferred

## Decision

The Postgres-specific bootstrap verification test (`CONSENSUS_TEST_POSTGRES_URL=<url> go test ./internal/bootstrap -run Postgres -v -count=1`) is not runnable in the current CI environment because no Postgres instance is available.

## Rationale

- The required verification (`go test ./internal/bootstrap -v -count=1`) passes — 7 bootstrap tests covering admin key creation, authentication, concurrency, TTL, expiry.
- The SQLite backend is the primary development target; Postgres support is documented in specs but not yet integrated into CI.
- Adding a gated Postgres test would require either: (a) a running Postgres instance in CI, or (b) a test that skips when `CONSENSUS_TEST_POSTGRES_URL` is unset. The latter is the correct approach when Postgres CI is available.

## Action Required to Close This Gap

When a Postgres CI environment becomes available:
1. Add `CONSENSUS_TEST_POSTGRES_URL` to CI environment variables
2. Add a Postgres-specific test in `internal/bootstrap/` that uses `t.Skip()` when the env var is unset
3. Mark this work item complete

## axiom:trace

axiom:trace work_item=postgres-bootstrap-verification-01 spec=specs/015-api-and-mcp.md,specs/016-cli-interface.md plan=.memory-bank/work-items/postgres-bootstrap-verification-01/plan.yaml evidence=.memory-bank/work-items/postgres-bootstrap-verification-01/accepted-gap.md
