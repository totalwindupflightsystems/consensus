# bootstrap-admin-key-policy-01 — Verification

Status: pass

## Summary

Bootstrap admin key expiry/rotation policy implemented and verified. The implementation covers:

| Requirement | Status | Evidence |
|---|---|---|
| REQ-BOOTSTRAP-TTL-001: Configurable TTL via `CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS` (default 2160h/90d) | ✅ PASS | `GetBootstrapKeyTTL()` reads env var; `EnsureFirstAdminKey()` sets `expires_at` |
| REQ-BOOTSTRAP-TTL-002: Bootstrap-time visibility of expiry | ✅ PASS | `FormatResult()` includes `expires_at` in machine-parseable output and human-readable line |
| REQ-BOOTSTRAP-TTL-003: No change to auth middleware | ✅ PASS | Auth middleware already checks `expires_at` — verified by `TestAuthMiddleware_ExpiredKey_Returns401` |
| ADR-023: Decision documented | ✅ PASS | `specs/023-adr-bootstrap-key-expiry.md` documents 90-day default, rationale, alternatives |
| Backward compat (TTL=0 = no expiry) | ✅ PASS | `TestEnsureFirstAdminKey_WithZeroTTL_NoExpiry` verifies `expires_at IS NULL` |

## Verification Commands

All commands run 2026-05-28.

### `go test ./internal/bootstrap/ -v -count=1` — PASS (16/16 tests)

TTL-specific tests:
- `TestEnsureFirstAdminKey_WithTTL_SetsExpiresAt` — PASS
- `TestEnsureFirstAdminKey_WithZeroTTL_NoExpiry` — PASS
- `TestEnsureFirstAdminKey_DefaultTTL_Is90Days` — PASS
- `TestEnsureFirstAdminKey_ExpiredKeyRejected` — PASS
- `TestFormatResult_IncludesExpiry` — PASS
- `TestFormatResult_NoExpiry_ShowsDoesNotExpire` — PASS
- `TestGetBootstrapKeyTTL_EnvVarParsing` — PASS
- All other bootstrap tests — PASS

### `go test ./internal/api/ -v -count=1` — PASS (all tests)

Auth expiry enforcement verified:
- `TestAuthMiddleware_ExpiredKey_Returns401` — PASS (expired keys correctly rejected)

### `go build ./... && go test ./... -count=1` — PASS (build + all 24 packages)

## Acceptance Criteria Coverage

| AC | Status | How Verified |
|---|---|---|
| Bootstrap key has configurable TTL (default 90d) | ✅ PASS | `GetBootstrapKeyTTL()` returns `DefaultBootstrapKeyTTLHours * time.Hour` (2160h) |
| TTL=0 disables expiry (backward compat) | ✅ PASS | `EnsureFirstAdminKey` with TTL=0 sets `expires_at IS NULL` |
| Expiry shown in bootstrap output | ✅ PASS | `FormatResult()` emits `expires_at=` line and human-readable duration |
| Auth middleware enforces expiry unchanged | ✅ PASS | `TestAuthMiddleware_ExpiredKey_Returns401` passes |
| ADR documents decision | ✅ PASS | `specs/023-adr-bootstrap-key-expiry.md` exists with acceptance date 2026-05-28 |

## Artifact Trace Map

```
specs/015-api-and-mcp.md#req-bootstrap-ttl-001 ← → internal/bootstrap/admin_key.go
specs/015-api-and-mcp.md#req-bootstrap-ttl-002 ← → internal/bootstrap/admin_key.go (FormatResult)
specs/015-api-and-mcp.md#req-bootstrap-ttl-003 ← → internal/api/server.go (auth middleware, unchanged)
specs/023-adr-bootstrap-key-expiry.md            ← ADR documenting the decision
cmd/consensus/main.go                            ← wired with GetBootstrapKeyTTL()
internal/bootstrap/admin_key_test.go              ← TTL tests (7 new tests)
```

axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md,specs/005-security.md evidence=.memory-bank/work-items/bootstrap-admin-key-policy-01/verification.md
