# Consensus HTTP API Reference

The Consensus server exposes a JSON REST API under `/api/v1/`, an SSE event
stream, an OpenAPI specification, and several auxiliary surfaces (MCP, webhooks,
opencode shim). This reference covers every endpoint with request/response
examples. The canonical machine-readable contract is the bundled OpenAPI spec —
see [OpenAPI](#openapi-specification) below.

- Base URL: `http://<host>:8090` (default port, configurable via `CONSENSUS_PORT` / config `server.port`)
- Auth: `Authorization: Bearer <api-key>` header (keys are `cs_ak_...` secrets; the first one — the bootstrap admin key — is printed once at server startup, see [API Key Management](#api-key-management))
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

## Event Stream (no auth)

### `GET /api/v1/events`

Server-Sent Events (SSE) stream. No authentication — session isolation is
enforced via the `session_id` query parameter.

```bash
curl -N "http://localhost:8090/api/v1/events?session_id=<session-uuid>"
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

In development the server prefers `specs/openapi/bundled.yaml` relative to
the process working directory when it exists, so re-running
`make bundle-spec` picks up live edits without a rebuild.

> `GET /doc` is **not** the REST API explorer — it serves the opencode shim's
> own Swagger UI (SPEC-017 surface: `/session`, `/config`, `/event`, ...).
> Use `/doc/api` for the REST API.

```bash
curl http://localhost:8090/openapi.json | jq '.paths | keys'
```

---

## Sessions

All routes below require `Authorization: Bearer <api-key>`.

### `POST /api/v1/sessions` — create a session

```bash
curl -X POST http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-chat","system_prompt":"You are a helpful agent."}'
```

Returns `201` with the created session (id, status, model, created_at).

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

Triggers the harness loop; returns the agent response.

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

## Auxiliary Surfaces

| Route | Auth | Description |
|---|---|---|
| `/mcp/*` | — | MCP server (SSE + message endpoints) |
| `/webhooks/` | HMAC signature | Webhook ingestion (SPEC-013) |
| `/ui/` | — | Web admin console (proxies API via its own `/api/` path) |
| `/chronicle/` | — | Chronicle investigation workbench |
| `/instance/*`, `/session/*`, `/config/*`, `/agent/*`, `/event`, `/permission/*`, `/project/*`, `/doc`, ... | shim admin key | opencode protocol shim (SPEC-017) — translates the opencode server protocol into native Consensus calls; `/doc` serves the shim's own Swagger UI (the REST API explorer lives at `/doc/api`) |

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
