# Consensus HTTP API Reference

The Consensus server exposes a JSON REST API under `/api/v1/`, an SSE event
stream, an OpenAPI specification, and several auxiliary surfaces (MCP, webhooks,
opencode shim). This reference covers every endpoint with request/response
examples. The canonical machine-readable contract is the bundled OpenAPI spec —
see [OpenAPI](#openapi-specification) below.

- Base URL: `http://<host>:8090` (default port, configurable via `CONSENSUS_PORT` / config `server.port`)
- Auth: `Authorization: Bearer <api-key>` header (keys are `cs_ak_...` secrets created via `POST /api/v1/auth/keys`)
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

| Route | Description |
|---|---|
| `GET /openapi.json` | Bundled OpenAPI spec as JSON |
| `GET /openapi.yaml` | Bundled OpenAPI spec as YAML |
| `GET /doc` | Swagger UI explorer (also `/doc/*`) |

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

| Route | Description |
|---|---|
| `POST /api/v1/auth/keys` | Create an API key |
| `GET /api/v1/auth/keys` | List API keys |
| `DELETE /api/v1/auth/keys/{keyID}` | Revoke an API key |

```bash
curl -X POST http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"name":"ci","scope":"readonly"}'
```

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
| `/instance/*`, `/session/*`, `/config/*`, `/agent/*`, `/event`, `/permission/*`, `/project/*`, ... | shim admin key | opencode protocol shim (SPEC-017) — translates the opencode server protocol into native Consensus calls |

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
