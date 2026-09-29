# Consensus HTTP API Reference

The Consensus server exposes a JSON REST API under `/api/v1/`, an SSE event
stream, an OpenAPI specification, and several auxiliary surfaces (MCP, webhooks,
opencode shim). This reference covers every endpoint with request/response
examples. The canonical machine-readable contract is the bundled OpenAPI spec —
see [OpenAPI](#openapi-specification) below.

- Base URL: `http://<host>:8090` (default port, configurable via `CONSENSUS_PORT` / config `server.port`; default bind `127.0.0.1` via config `server.hostname` — the opencode shim surface, including the auth-free public `/instance/*` endpoints, must stay on a loopback bind, see SPEC-017 §3.10)
- Auth: set the `Authorization` header to `Bearer $CONSENSUS_API_KEY` (keys are `cs_ak_...` secrets; the first one — the bootstrap admin key — is printed once at server startup, see [API Key Management](#api-key-management)). The webhook endpoint is the exception: it uses a per-registration HMAC signature and no Bearer key.
- Errors: JSON envelope `{"error":{"code":"...","message":"...","details":"..."}}` with the appropriate HTTP status
- Auth failures return `401` with code `UNAUTHORIZED`; missing/invalid UUID path params return `400` with code `INVALID_UUID`

---

## Health (no auth)

### `GET /api/v1/health`

Liveness/readiness probe. No authentication required. Reports version, uptime,
DB backend and diagnostics.

```bash
curl http://localhost:8090/api/v1/health
```

```json
{
  "status": "ok",
  "version": "0.1.0",
  "uptime_seconds": 7,
  "api_latency_ms": 0,
  "db_latency_ms": 0.128,
  "llm_latency_ms": 0,
  "error_rate_pct": 0,
  "db_backend": "sqlite",
  "db_path": "/home/consensus/data/consensus.db",
  "db_size_mb": 0.45,
  "db_tables": 37,
  "db_migrations": 22,
  "schema_version": 23,
  "active_connections": {
    "websocket": 0,
    "db_pool_active": 0,
    "db_pool_max": 0,
    "llm_active": 0,
    "api_requests_last_min": 0
  },
  "system_log": []
}
```

`db_backend` is `"sqlite"` or `"postgres"` depending on the configured adapter.

---

## Event Stream

### `GET /api/v1/events`

Server-Sent Events (SSE) stream. Requires a valid API key (DF-CONSENSUS-30):

| Client | Result |
|---|---|
| No / invalid / expired key | `401 UNAUTHENTICATED` |
| `admin` or `readonly` key | `200` — any `session_id`, or the global stream without one |
| `session` key bound to session X, `session_id=X` | `200` — the stream for X |
| `session` key bound to X, foreign/absent `session_id` | `403 FORBIDDEN` |

401/403 are the standard JSON error envelope, written before any
`text/event-stream` header. Browsers' `EventSource` cannot send
authorization headers — the Chronicle dashboard falls back to memory
polling automatically.

```bash
curl -N -H "Authorization: Bearer <key>" \
  "http://localhost:8090/api/v1/events?session_id=<session-uuid>"
```

Emits `event:` frames as sessions progress (message created, tool executed,
iteration completed, approval requested, ...).

---

## OpenAPI Specification

The machine-readable contract is embedded in the binary and served at these
routes — no working-directory dependency, and the Docker image serves them
too:

| Route | Description |
|---|---|
| `GET /openapi.json` | Bundled OpenAPI spec as JSON |
| `GET /openapi.yaml` | Bundled OpenAPI spec as YAML |
| `GET /doc/api` | Swagger UI explorer for the REST API (servers URL derived from the request Host) |

The served contract is always the copy embedded in the binary. After running
`make bundle-spec`, rebuild Consensus to publish the updated contract.

> `GET /doc` serves the machine-readable OpenAPI document (JSON by default;
> `Accept: application/yaml` returns YAML) for the opencode-compatible surface.
> The interactive REST API explorer remains at `/doc/api`. The document is public.

```bash
curl http://localhost:8090/openapi.json | jq '.paths | keys'
```

**Authoritative path count: 61** (36 opencode shim + 24 native `/api/v1` +
1 HMAC-authenticated webhook path), including the four `/instance` routes
(`/instance`, `/instance/path`, `/instance/vcs`, `/instance/vcs/diff`) from
SPEC-017 §3.10. The embedded `specs/openapi/bundled.yaml` (served at
`/openapi.json`) is the source of truth — when routes are added or removed,
update this count in the same change.

---

## Sessions

All routes below require an `Authorization` header with the value
`Bearer $CONSENSUS_API_KEY`.

### `POST /api/v1/sessions` — create a session

```bash
curl -X POST http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"agent_name":"demo","goal":"Summarize the ledger."}'
```

`agent_name` and `goal` are required. The optional request fields are
`model_id`, `context_budget`, `hitl_config`, and `project_id`. Returns `201`
with the created session (id, status, model, created_at).

> **Model selection:** per-session model selection is not yet honored by the
> runtime. The JSON field `model` is not part of this request schema and is
> silently ignored; although `model_id` can be stored on the session, the
> harness client still executes with the model selected by server configuration
> (`llm.default_model`) and the model registry. Configure the effective model
> there instead of expecting a create-session field to switch it.

### `GET /api/v1/sessions` — list sessions

```bash
curl http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

Returns a JSON array of sessions. An empty array is valid.

### `GET /api/v1/sessions/{id}` — get one session

```bash
curl http://localhost:8090/api/v1/sessions/<session-uuid> \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

### `PATCH /api/v1/sessions/{id}` — update a session

Update status (`pause`, `resume`, `cancel`) or settings:

```bash
curl -X PATCH http://localhost:8090/api/v1/sessions/<session-uuid> \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"status":"pause"}'
```

### `DELETE /api/v1/sessions/{id}` — delete a session

```bash
curl -X DELETE http://localhost:8090/api/v1/sessions/<session-uuid> \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

### `POST /api/v1/sessions/{id}/message` — send a message

```bash
curl -X POST http://localhost:8090/api/v1/sessions/<session-uuid>/message \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"role":"user","content":"Summarize the ledger."}'
```

Queues the user turn and returns `200` with `{"status":"message_received",...}`.
Processing is asynchronous. Poll `GET /api/v1/sessions/{id}` until the session
returns to `idle`; the durable assistant reply is then available in
`last_message`, and token totals are exposed as `tokens_used_in` and
`tokens_used_out`. The same reply is also listed as a `text_block` by
`GET /api/v1/sessions/{id}/memory`.

---

## Session Sub-resources

| Route | Description |
|---|---|
| `GET /api/v1/sessions/{id}/memory` | List memory events for the session |
| `GET /api/v1/sessions/{id}/memory/{memoryID}` | Fetch one memory event |
| `GET /api/v1/sessions/{id}/context` | Active context view (live SQL view) |
| `GET /api/v1/sessions/{id}/iterations` | Iteration history |
| `GET /api/v1/sessions/{id}/tasks` | Tasks spawned by the session |
| `POST /api/v1/sessions/{id}/tasks` | Create a task |
| `GET /api/v1/sessions/{id}/approvals` | Pending/reviewed approvals |
| `GET /api/v1/sessions/{id}/billing` | Token/cost ledger for the session |

```bash
curl http://localhost:8090/api/v1/sessions/<session-uuid>/billing \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

---

## Tasks

| Route | Description |
|---|---|
| `PATCH /api/v1/tasks/{taskID}` | Update task fields (status, priority, ...) |
| `POST /api/v1/tasks/{taskID}/claim` | Claim a task for execution |

```bash
curl -X POST http://localhost:8090/api/v1/tasks/<task-id>/claim \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

---

## Tools & Skills

| Route | Description |
|---|---|
| `GET /api/v1/tools` | List available agent tools |
| `GET /api/v1/skills` | List installed skills |
| `GET /api/v1/skills/{skillName}` | Fetch one skill's definition |
| `POST /api/v1/tools/{toolName}/execute` | Execute a tool |

```bash
curl http://localhost:8090/api/v1/tools \
  -H "Authorization: Bearer cs_ak_your_secret_key"

curl -X POST http://localhost:8090/api/v1/tools/query/execute \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"query":"SELECT * FROM sessions LIMIT 5"}'
```

---

## Approvals

| Route | Description |
|---|---|
| `GET /api/v1/approvals` | List approvals |
| `GET /api/v1/approvals/{approvalID}` | Get one approval |
| `POST /api/v1/approvals/{approvalID}/review` | Approve/reject (`{"decision":"approve"}` or `{"decision":"reject"}`) |

---

## Config & Metrics

| Route | Description |
|---|---|
| `GET /api/v1/config` | Server configuration (redacted secrets) |
| `GET /api/v1/metrics` | Operational metrics (also accessible with readonly scope) |

```bash
curl http://localhost:8090/api/v1/config \
  -H "Authorization: Bearer cs_ak_your_secret_key"
```

---

## API Key Management

### First key: the bootstrap admin key

A fresh instance has no credentials, so `POST /api/v1/auth/keys` rejects the
request until one exists — the way in is the bootstrap key. On first startup
against an empty database, the server prints the first admin key exactly once
(in the terminal output of `consensus init` / `consensus serve`; with Docker,
`docker logs`):

```
consensus: first_admin_key created=true key=cs_ak_<64 hex chars> key_prefix=<8 chars> id=<uuid> created_at=<RFC 3339> expires_at=<RFC 3339>
consensus: this key expires at <RFC 3339> (… from now)
consensus: save this key now; it is stored hashed and will not be printed again
```

Capture it now: the raw secret is stored only as a hash and is never printed
again (later startups print `created=false` with just the `key_prefix`). The
bootstrap key has `admin` scope, so it works as a Bearer credential on every
admin endpoint — including `POST /api/v1/auth/keys`, which is how you mint
durable keys for day-to-day use.

Bootstrap keys expire after **90 days** by default
(`CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS=2160`); set the env var to change the
TTL, or `0` for no expiry.

### Key management endpoints

| Route | Description |
|---|---|
| `POST /api/v1/auth/keys` | Create an API key |
| `GET /api/v1/auth/keys` | List API keys |
| `DELETE /api/v1/auth/keys/{keyID}` | Revoke an API key |

Worked sequence — bootstrap key → durable key → new key as Bearer:

```bash
# 1. Mint a durable key with the bootstrap admin key (the response includes
#    the new key's secret in the `api_key` field — shown once)
curl -X POST http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $BOOTSTRAP_KEY" \
  -H "Content-Type: application/json" \
  -d '{"scope":"readonly"}'

# 2. Authenticate with the new key
curl http://localhost:8090/api/v1/metrics \
  -H "Authorization: Bearer $NEW_KEY"
```

`POST /api/v1/auth/keys` requires `admin` scope; valid scopes are `admin`,
`session`, `readonly`, `webhook`. The endpoint accepts optional `expires_in`
(seconds from now) and, for `session` scope, `session_id`.

---

## Quarantine (Cognitive Firewall)

| Route | Description |
|---|---|
| `GET /api/v1/quarantine` | List quarantined content |
| `POST /api/v1/quarantine/{qID}/approve` | Approve quarantined content |
| `POST /api/v1/quarantine/{qID}/reject` | Reject quarantined content |

---

## Webhooks

### `POST /webhooks/{source}` — ingest an external event

This route does **not** use an API key. The handler authenticates the exact
request body with the secret stored in a webhook registration
(`internal/webhook/webhook.go:282-294`).

#### 1. Register the source

There is currently no HTTP API or CLI command for webhook registration. Create
the row directly in the configured database. The production schema requires
`id`, `name`, `source`, `url_path`, `secret`, `event_types`, optional session and
workflow targets, `enabled`, and `created_at`
(`migrations/007_webhook_tables.sql:10-22`). For the default SQLite database:

```bash
DB_PATH="$HOME/.consensus/consensus.db"
WEBHOOK_SECRET='replace-with-your-webhook-secret'

sqlite3 "$DB_PATH" <<SQL
INSERT INTO webhook_registrations (
  id, name, source, url_path, secret, event_types,
  target_session_id, target_workflow_id, enabled, created_at
) VALUES (
  'wh_docs_demo', 'docs_demo', 'docs-demo', '/webhooks/docs-demo',
  '$WEBHOOK_SECRET', '{"ping"}',
  NULL, NULL, 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
);
SQL
```

Use the database selected by `database.url` / `CONSENSUS_DB_URL` if it differs
from the default. For Postgres, execute the same column list with your SQL client
and use an RFC 3339 timestamp for `created_at`. Keep the secret out of shell
history and source control in production.

The URL segment is looked up against the registration's `name`, `url_path`, or
`source`; the example deliberately sets `source` to the URL segment
`docs-demo` (`internal/webhook/webhook.go:139-149,586-606`).

#### 2. Sign and deliver the exact bytes

The handler checks `X-Hub-Signature-256`, then `X-Signature-256`, then
`X-Signature`; the first non-empty value wins. The value is the lowercase hex
HMAC-SHA256 of the **raw body bytes**, keyed by the registration secret. A
`sha256=` prefix is accepted but optional
(`internal/webhook/webhook.go:282-294,626-634`). This runnable example signs the
same shell variable that `curl --data-binary` sends:

```bash
BODY='{"event":"ping","message":"hello"}'
SIGNATURE=$(printf '%s' "$BODY" \
  | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" -hex \
  | cut -d ' ' -f 2)

curl -i -X POST http://localhost:8090/webhooks/docs-demo \
  -H 'Content-Type: application/json' \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  -H 'X-Event-Type: ping' \
  -H 'X-Delivery-ID: docs-demo-001' \
  --data-binary "$BODY"
```

A new delivery returns:

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{"status":"accepted"}
```

`X-Event-Type` falls back to `X-GitHub-Event`, then `unknown`.
`X-Delivery-ID` falls back to `X-GitHub-Delivery`; a repeated non-empty delivery
ID returns `200` with `status: "duplicate"`
(`internal/webhook/webhook.go:636-647,701-713`).

#### 3. Understand validation and routing

A missing or mismatched signature is **not rejected at the HTTP boundary**. The
handler still stores the delivery, marks `signature_valid=false`, gives the
event `quarantined` status, and returns the same `202 {"status":"accepted"}` as
a new valid delivery. In the normal server wiring it also submits the body to
the Cognitive Firewall quarantine inserter. This behavior follows
`internal/webhook/webhook.go:634,649-697,712-713,739-744`.

Registration fields currently behave as follows:

| Registration field | Current runtime behavior |
|---|---|
| `event_types` | Stored as text, but not loaded by `rowToRegistration` or consulted by the HTTP handler. It does **not** filter deliveries today; the request headers determine `event_type` (`internal/webhook/webhook.go:773-799,636-643`). |
| `target_session_id` | Copied onto the new `external_events.session_id` row (`internal/webhook/webhook.go:649-662`). It does not by itself wake the session. |
| `target_workflow_id` | Copied onto `external_events.workflow_id` (`internal/webhook/webhook.go:649-662`). No workflow executor is invoked by this handler. |
| `enabled` | `0` rejects the delivery with `403`; `1` permits ingestion (`internal/webhook/webhook.go:609-611`). |

After valid ingestion the event starts as `pending`. Every five seconds the
routing loop reads pending events and tests enabled `routing_rules` in ascending
priority. A rule can match substrings of `source`, `event_type`, and payload; a
match rewrites the event's session/workflow targets and marks it `routed`. A
matched session target is changed from `waiting_sub` or `paused` to `idle`; a
workflow target is recorded on the event, but this loop does not start a
workflow (`cmd/consensus/main.go:237-239`,
`internal/webhook/webhook.go:417-475,485-529`). Webhook events use the stored
source value `webhook`, so routing rules should match `source_pattern='webhook'`
and the delivered event type rather than the registration's source name.

To verify the example delivery in SQLite:

```bash
sqlite3 "$DB_PATH" \
  "SELECT source_id, event_type, signature_valid, status, session_id, workflow_id FROM external_events WHERE source_id = 'docs-demo-001';"
```

With the valid signature and no matching routing rule, the expected fields are
`docs-demo-001|ping|1|pending||`.

---

## Auxiliary Surfaces

| Route | Auth | Description |
|---|---|---|
| `/mcp/*` | — | MCP server (SSE + message endpoints) |
| `/webhooks/{source}` | HMAC-SHA256 signature | Webhook ingestion (SPEC-013); see [Webhooks](#webhooks) |
| `/ui/` | — | Web admin console (proxies API via its own `/api/` path) |
| `/chronicle/` | — | Chronicle investigation workbench |
| `/instance`, `/instance/path`, `/instance/vcs`, `/instance/vcs/diff` | — (public) | opencode protocol shim (SPEC-017 §3.10) — singleton instance list, workspace `PathInfo`, live git branch info, and per-file diff stats; other upstream `/instance/*` sub-paths return 501, unknown sub-paths 404 |
| `/session/*`, `/config/*`, `/agent/*`, `/event`, `/permission/*`, `/project/*`, `/doc`, ... | shim admin key | opencode protocol shim (SPEC-017) — translates the opencode server protocol into native Consensus calls; `/doc` serves the machine-readable shim contract (the REST API explorer lives at `/doc/api`) |

---

## Error Format

All errors use the standard envelope:

```json
{
  "error": {
    "code": "INVALID_UUID",
    "message": "session ID must be a valid UUID: abc",
    "details": "session ID must be a valid UUID: abc"
  }
}
```

Common codes: `UNAUTHORIZED` (401), `INVALID_UUID` (400), `NOT_FOUND` (404),
`CONFLICT` (409), `RATE_LIMITED` (429).
