# runtime-dev-bootstrap-auth-01 — Plan

Fresh local startup currently proves the server runs but blocks testing protected endpoints because no admin API key exists. Fix the bootstrap path so a fresh developer can initialize, authenticate, and continue runtime testing.

## AC to Verification

| AC | Verification |
|---|---|
| First admin key created on bootstrap | Fresh SQLite DB + `conscience init` creates one `api_keys` row with `scope='admin'`; fresh `conscience serve` also creates and prints a one-time key |
| Secret handling safe | Logs/tests assert full key is not persisted or reprinted after creation |
| Protected endpoint accessible | Start server, use generated/admin key, `GET /api/v1/sessions` is not `401` |
| Docs/logs truthful | CLI output matches implemented behavior |

## Steps

1. **Decide bootstrap contract**
   - Choose whether `init` creates the key, `serve` creates it, or both share one helper.
   - Preferred: `init` creates the key; `serve` also creates the key on a fresh DB so `make dev` is self-bootstrapping.

2. **Implement bootstrap helper**
   - Add code that detects zero admin keys and inserts one hashed admin key.
   - Print the secret only at creation time.

3. **Add tests**
   - Unit/integration test for fresh SQLite bootstrap.
   - Runtime-ish test for authenticated protected endpoint.

4. **Run verification**
   - `make test`
   - Fresh `make dev` probe with key redacted in evidence.

## Rollback

Revert bootstrap helper and tests. Existing auth behavior returns to requiring pre-seeded keys.

axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/plan.md test=make-test,runtime-health-auth-probe evidence=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/verification.md
