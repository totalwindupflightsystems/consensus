# ROUTE-ADD-043 — GET /api/session/{sessionID}/context live probe

ch:trace row=ROUTE-ADD-043 spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.context test=internal/shim/opencode/api_session_context_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml wave=.coding-hermes/waves/consensus-foreman-2026-10-30-00-02-15.json evidence=docs/evidence/ROUTE-ADD-043-live-probe.md witness=127.0.0.1:18743 verdict=none:foreman-judges commit=none:pending

Date: 2026-10-30. Scratch instance, SQLite backend, mock LLM, no provider credentials.

## Run identity

- Binary: `/tmp/consensus-043-bin`, built with `go build -o /tmp/consensus-043-bin ./cmd/consensus`.
- Listener: `127.0.0.1:18743`.
- Database: `/tmp/route043-live/scratch.db`, migrations applied by the server.
- LLM: `provider: mock` and `CONSENSUS_MOCK_LLM=1`.
- Auth: the server's auto-minted first-admin key was used locally and redacted from this transcript.
- Session fixture: one real session row, ID `9b1c4d2e-0000-4000-8000-abcdefabcdef`, with visible and compressed context rows; no credentials are stored in this file.

## Probe transcript

```
$ curl -s -w '\nHTTP %{http_code}\n' http://127.0.0.1:18743/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/context
{"data":[{"content":"hello from the probe","created_at":"2026-10-30 00:12:03","iteration_created":0,"memory_event_id":1,"type":"user_message"},{"created_at":"2026-10-30 00:12:03","display_mode":"compressed","iteration_created":0,"memory_event_id":2,"summary_text":"compact answer","type":"text_block"}]}
HTTP 200

$ curl -s -w '\nHTTP %{http_code}\n' http://127.0.0.1:18743/api/session/00000000-0000-4000-8000-ffffffffffff/context
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-ffffffffffff\" in this runtime: no context to return","sessionID":"00000000-0000-4000-8000-ffffffffffff"}
HTTP 404

$ curl -s -w '\nHTTP %{http_code}\n' http://127.0.0.1:18743/api/session/%20/context
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Path","message":"sessionID path parameter must not be blank"}
HTTP 400

$ curl -s -w '\nHTTP %{http_code}\n' http://127.0.0.1:18743/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/context -X POST
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405
```

The authenticated 200/400/404 outcomes above are from the scratch instance. The
unauthenticated 401 arm was also checked against the scratch listener and returned
the v2 UnauthorizedError envelope. The store-failure 500 arm was exercised by the
focused shim test (`TestApiSessionContextStoreFailureAnswersDeclared500`); the 500
is not safely injectable into a healthy running instance without sabotaging its database.

## Declared-vs-served regeneration

```
$ python3 scripts/compare-opencode-declared-served.py specs/openapi/upstream/openapi-1.18.33.surface.json specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-043
exit 0

$ jq -c '.drift_declared_not_served[]? | select(.method=="GET" and .path=="/api/session/{sessionID}/context")' /tmp/parity-043/declared-vs-served.json
# no output

$ jq -c '.covered[]? | select(.method=="GET" and .path=="/api/session/{sessionID}/context")' /tmp/parity-043/declared-vs-served.json
{"path":"/api/session/{sessionID}/context","method":"GET","operationId":"v2.session.context","served_outcome":"200"}
```

The regenerated artifact reported `served_route_entries=176`,
`covered_operations=127`, and `drift_declared_not_served=34`. Drift IDs are
positional and are not used as the route identity.
