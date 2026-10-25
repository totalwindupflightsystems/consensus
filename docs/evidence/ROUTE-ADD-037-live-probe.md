# ROUTE-ADD-037 — GET /api/session (v2.session.list) live probe

ch:trace row=ROUTE-ADD-037
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.list
  test=internal/shim/opencode/api_session_routes_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-037-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-24T23:03:00-05:00
Base commit: `77f0c5a7f6482298d4347ef2512061708d34bb1c` (`main`, the
implementation commit this probe exercises; this evidence document and the
matching `ch:trace` line are added in the follow-up commit).

**Source item:** the board row cites `SHIM-DRIFT-033`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration —
and in the artifact this worker started from `SHIM-DRIFT-033` was
`POST /experimental/workspace`. The operation, path and declared response set in
the brief all name `GET /api/session` = `v2.session.list`, so that is the row
served here; the id mapping is recorded here and in the served-surface evidence
rather than being silently accepted. The gates (`TestNotImplementedTableMatches
DeclaredSurfaceArtifact`, `declared_vs_served_consistency_test.go`,
`declared_vs_served_artifact_test.go`) key on method + path + operationId, which
is the identity this row moves.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-route037` |
| Binary SHA-256 | `ae6152825eed620699f8200fcaa7c0ea17bf9090747f439e171415542738f904` |
| Listener | `127.0.0.1:18093` |
| Backend | scratch SQLite (`/tmp/route037-scratch-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` (`CONSENSUS_MOCK_LLM=1`) — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with a throwaway probe token |

## Probe transcript

```
## unauth GET /api/session
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401
## auth GET /api/session
{"cursor":{},"data":[]}
HTTP 200
## auth GET /api/session?order=sideways
{"_tag":"InvalidRequestError","field":"order","kind":"Query","message":"query parameter \"order\" must be one of \"asc\" or \"desc\" (the declared enum)"}
HTTP 400
## auth GET /api/session?cursor=abc
{"_tag":"InvalidCursorError","message":"query parameter \"cursor\" is not a cursor issued by this server: the session list surface returns no cursor.previous or cursor.next"}
HTTP 400
## auth GET /api/session?workspace=wrk_a&order=desc&limit=5
{"cursor":{},"data":[]}
HTTP 200
## auth POST /api/session
{"error":"not_implemented","operation":"v2.session.create","detail":"the Consensus shim serves no session-create contract: it does not own the opencode session lifecycle (v2.session.create is its own board row)"}
HTTP 501
## auth PUT /api/session
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405
## control auth GET /api/session/active
404 page not found

HTTP 404
## control unauth GET /api/session
HTTP 401
```

## What each arm proves

- **200** answers the declared `SessionsResponse` exactly: both required members
  (`data`, `cursor`), `data` an empty but NON-NULL array, `cursor` an empty
  object — no SessionV2Info entry is fabricated from a `sessions` row that
  cannot supply eight of its twelve required members (handler doc comment).
- **401** answers the declared `UnauthorizedError` envelope
  (`{_tag, message}`), i.e. the exact-path `v2EnvelopeRoutePaths` entry is in
  effect — not the shim's v1 `UNAUTHENTICATED` body.
- **400 InvalidRequestError** answers a declared query parameter violating the
  pinned schema (`order` outside the `asc|desc` enum) and names the offending
  parameter (`field: order`).
- **400 InvalidCursorError** answers a `cursor` value this surface never issued,
  instead of ignoring it and returning an unfiltered list.
- **501 typed not_implemented naming `v2.session.create`** answers the declared
  sibling method on the same exact path (SHIM-GAP-002: a registered route must
  not pretend a declared operation does not exist). It is recorded in
  `notImplementedRoutes` and in the served surface's `intentional_narrowing`
  bucket with a rationale, and leaves both when ROUTE-ADD-038 serves
  `v2.session.create` for real.
- **405** for PUT, a method the pinned document does not declare on this path.
- **404 controls**: the declared `/api/session/{sessionID}[...]` family is NOT
  served by this exact-path registration (`/api/session/active` keeps the
  net/http default 404 its own contract rows are filed against), so no
  `/api/session/*` catch-all was introduced.

## Reproduce

```bash
cd <repo>
go build -o /tmp/consensus-route037 ./cmd/consensus
mkdir -p /tmp/route037-scratch-live
cat > /tmp/route037-scratch-live/consensus.yaml <<'YAML'
llm:
  provider: mock
  default_model: mock-model
  max_context_tokens: 32000
  max_output_tokens: 4096
YAML
CONSENSUS_MOCK_LLM=1 /tmp/consensus-route037 serve \
  --config /tmp/route037-scratch-live/consensus.yaml \
  --db-url 'sqlite:///tmp/route037-scratch-live/scratch.db' \
  --port 18093 --hostname 127.0.0.1 --log-level error &

# seed a throwaway api key (the shim's auth reads the api_keys table)
TOK='route037-live-probe-token'
PREFIX=$(printf '%s' "$TOK" | cut -c1-8)
HASH=$(printf '%s' "$TOK" | sha256sum | cut -d' ' -f1)
sqlite3 /tmp/route037-scratch-live/scratch.db \
  "INSERT INTO api_keys (key_hash, key_prefix, scope) VALUES ('$HASH', '$PREFIX', 'admin');"

curl -s -w '\nHTTP %{http_code}\n' -H "Authorization: Bearer $TOK" \
  http://127.0.0.1:18093/api/session
```

Note for the next probe: the shim's auth wall accepts `Authorization: Bearer`
(api_keys lookup) or Basic — it does NOT read a bare `x-api-key` header, so an
`x-api-key` request answers 401 even with a valid `--api-key` flag.
