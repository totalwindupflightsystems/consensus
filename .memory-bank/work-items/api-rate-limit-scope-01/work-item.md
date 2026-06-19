---
work_item_id: api-rate-limit-scope-01
title: API Rate Limiting Scope-Aware
status: DONE
mode: patch_fix
verification_bar: standard
created: 2026-05-28
completed: 2026-05-28
---

# Work Item: api-rate-limit-scope-01

## Objective

Make API rate limiting scope-aware so that admin, session, readonly, and webhook API keys each get their configured rate limit instead of all sharing the admin default.

## Summary

The `checkRateLimit` function in `internal/api/server.go` had a bug: it hardcoded the admin rate limit (1000 req/min) for every API key, regardless of the key's scope. The scope was extracted from the database but only AFTER the rate limit check, so it was never consulted.

### Changes

1. **`internal/api/server.go`** — Core fix:
   - Added `AdminRate`, `SessionRate`, `ReadonlyRate`, `WebhookRate` fields to `ServerConfig`
   - Added `apiRates` map to `Server` struct (initialized from config with defaults fallback)
   - Added `resolveRates()` helper to build the active rate map from config with default fallbacks
   - Added `scopeRateLimit(scope)` method to look up limit by scope (unknown scopes default to "session" = 100)
   - Modified `checkRateLimit(ctx, prefix, scope)` to accept scope and look up the correct limit
   - Moved scope extraction before the rate limit check in `authMiddleware`

2. **`cmd/consensus/main.go`** — Config wiring:
   - Pass `cfg.APIRate` fields to `api.ServerConfig` when creating the API server

3. **`internal/api/server_test.go`** — Tests:
   - 10 scope-specific rate limiting tests covering:
     - Admin scope at boundary (999/1000 passes, 1000/1000 blocks)
     - Session scope at boundary (99/100 passes, 100/100 blocks)
     - Readonly scope at boundary (199/200 passes, 200/200 blocks)
     - Webhook scope at boundary (499/500 passes, 500/500 blocks)
     - Unknown scope fallback to session default
     - Config override (SessionRate: 50)
     - Zero config override keeps defaults
     - Window reset after expiry

## Default Limits (SPEC-015 §7.1)

| Scope    | Default (req/min) |
|----------|-------------------|
| admin    | 1000              |
| session  | 100               |
| readonly | 200               |
| webhook  | 500               |

Config overrides via `consensus.yaml` (`api_rate.*`) or CLI flags.

## Verification

- All 27 packages build and test clean (zero regressions)
- 10 scope-specific rate limit tests PASS
- Window reset behavior verified

## Trace

```
axiom:trace work_item=api-rate-limit-scope-01
  spec=specs/015-api-and-mcp.md
  impl=internal/api/server.go,cmd/consensus/main.go
  test=internal/api/server_test.go
```
