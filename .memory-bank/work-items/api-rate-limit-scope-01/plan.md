# api-rate-limit-scope-01 — Plan

Make API rate limiting scope-aware. The likely fix is to pass authenticated scope into `checkRateLimit` and select from `defaultRateLimits` by scope.

## Steps

1. Inspect `internal/api/server.go` auth middleware and rate limit tests.
2. Add tests proving each key scope uses its configured limit.
3. Update implementation to use scope-aware limits.
4. Run API tests and full suite.

## Verification

- `go test ./internal/api -run RateLimit -v -count=1`
- `make test`

## Rollback

Revert the `checkRateLimit` signature and tests. Existing permissive behavior returns.

axiom:trace work_item=api-rate-limit-scope-01 spec=specs/015-api-and-mcp.md plan=.memory-bank/work-items/api-rate-limit-scope-01/plan.md test=internal/api/server_test.go evidence=.memory-bank/work-items/api-rate-limit-scope-01/verification.md
