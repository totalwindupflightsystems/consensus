# runtime-dev-bootstrap-auth-01 — Meta Planning

## Summary

[FACT] `make dev` starts the Consensus server successfully on `127.0.0.1:8090`, and `/api/v1/health` returns healthy. Protected API endpoints return `401` because `dev.db` has no admin API key. `consensus init` says the server will print an admin key on first start, but the implementation only prints an informative message and does not create a key.

This work item closes the fresh-developer bootstrap gap: a new dev instance must provide a deterministic way to obtain the first admin API key or otherwise authenticate local testing.

## Scope

In scope:
- Make fresh local bootstrap produce a usable admin API key or equivalent deterministic local-dev auth path.
- Align implementation with `specs/016-cli-interface.md` and `specs/015-api-and-mcp.md`.
- Add regression tests proving a fresh SQLite dev instance can authenticate and hit protected endpoints.

Out of scope:
- Changing the full security model.
- Adding external secret stores.
- Changing production auth semantics beyond the bootstrap path.

## Acceptance Criteria

1. `consensus init --db-url sqlite://dev.db` or first `consensus serve` creates a first admin API key when no admin key exists.
2. The generated secret is printed exactly once or the operator is given a safe deterministic retrieval path.
3. `make dev` fresh-start documentation/logging is truthful and actionable.
4. A runtime probe can use the admin key to access `/api/v1/sessions` without `401`.
5. Tests cover the no-admin-key bootstrap case and avoid leaking full secrets in logs.

## Evidence Already Captured

- `make dev` started server and `/api/v1/health` returned `{"status":"healthy","version":"0.1.0"}`.
- `GET /api/v1/sessions`, `/api/v1/tools`, and `/api/v1/openapi.json` returned `401` without auth.
- `sqlite3 dev.db "SELECT ... FROM api_keys"` returned no rows.
- `go run ./cmd/consensus init --db-url sqlite://dev.db --format json` printed an informative message only; no key was produced.

## References

- `specs/016-cli-interface.md` — init command promises database tables, defaults, and first admin API key.
- `specs/015-api-and-mcp.md` — REST/MCP authentication via API keys.
- `internal/cli/init.go` — init command stub.
- `cmd/consensus/main.go` — `runInit()` and `runServer()` startup path.

axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/meta-planning.md evidence=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/verification.md
