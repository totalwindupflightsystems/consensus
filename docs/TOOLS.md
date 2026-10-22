# Tools & Skills

Consensus exposes tools and skills to agents through the registry tables
(`tools_registry`, `skills_registry`) and the API/CLI surfaces:

- `GET /api/v1/tools` · `POST /api/v1/tools` · `POST /api/v1/tools/{name}/execute`
- `consensus tool list` · `consensus tool show <name>` · `consensus tool register <name>`

## On a fresh install

`consensus init` seeds two built-in tools (migration
`032_seed_builtin_tools.sql`):

| Tool | `handler_type` | `requires_approval` | What it does |
|---|---|---|---|
| `list_sessions` | `go_native` | no | Lists the most recent sessions |
| `demo_approval_gate` | `go_native` | **yes** | Parks each execution request for human review — an end-to-end example of the approval gate |

`consensus tool list` shows them immediately after `init`.

## Registering a tool

Use the CLI (it calls `POST /api/v1/tools`, so a server must be running):

```bash
consensus tool register fetch_url \
  --description "Fetch a URL and return its body" \
  --hemisphere external \
  --handler-type http_endpoint \
  --handler-ref "https://fetcher.internal/fetch" \
  --parameter-schema '{"type":"object","properties":{"url":{"type":"string"}}}'
```

Add `--requires-approval` to put every execution behind human review, or POST
the equivalent JSON with an **admin** API key:

```bash
curl -X POST http://localhost:8090/api/v1/tools \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "fetch_url",
        "description": "Fetch a URL and return its body",
        "hemisphere": "external",
        "handler_type": "http_endpoint",
        "handler_ref": "https://fetcher.internal/fetch",
        "requires_approval": true
      }'
```

Required: `name`, `description`, `handler_ref`. Everything else falls back to
the registry defaults:

| Column | Allowed values | Default |
|---|---|---|
| `hemisphere` | `internal`, `external` | `internal` |
| `handler_type` | `sql_function`, `http_endpoint`, `go_native`, `subprocess` | `sql_function` |
| `status` | `active`, `testing`, `deprecated`, `disabled` | `active` |
| `enabled` | boolean | `true` |
| `requires_approval` | boolean | `false` |
| `parameter_schema` | JSON object (JSON Schema) | `{}` |
| `rate_limit_per_min` | integer | unlimited |

A taken name returns `409 CONFLICT`; a non-admin key returns `403 FORBIDDEN`.
The columns and CHECK constraints are defined in
`internal/migrate/migrations/001_initial_schema.sql` (Postgres) and
`009_sqlite_task_tool_tables.sql` (SQLite).

## Executing a tool

```bash
curl -X POST http://localhost:8090/api/v1/tools/list_sessions/execute \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<session-uuid>","parameters":{}}'
```

`session_id` is required. `sql_function` and `go_native` tools return their
result inline; `http_endpoint` and `subprocess` tools enqueue a `tool_requests`
row that the harness executor claims asynchronously.

## Approval-gated tools

When a tool's row has `requires_approval=true`, the executor does not run it:
the request parks at `awaiting_approval` and an `approval_requests` row is
created for a human. Approve it and the request is re-queued and executed;
reject it and the request fails with a recorded refusal.

```bash
# 1. Ask for execution
curl -X POST http://localhost:8090/api/v1/tools/demo_approval_gate/execute \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<session-uuid>"}'

# 2. Find the parked request + its approval id
curl "http://localhost:8090/api/v1/tool-requests?status=awaiting_approval" \
  -H "Authorization: Bearer cs_ak_your_secret_key"

# 3. Approve (admin key)
curl -X POST http://localhost:8090/api/v1/approvals/<approval-id>/review \
  -H "Authorization: Bearer cs_ak_your_secret_key" \
  -H "Content-Type: application/json" \
  -d '{"decision":"approved","notes":"reviewed"}'
```

See `docs/API.md` (Tools & Skills) for the full request/response shapes.

## Installing a skill

Skills are rows in `skills_registry` (there is no REST/CLI write surface yet):

```sql
INSERT INTO skills_registry (name, instructions, metadata, linked_tool_ids)
VALUES ('my-skill', 'Step-by-step instructions for the agent...', '{}', NULL);
```

The `instructions` text is injected into the agent context when the skill is
used; `linked_tool_ids` binds prerequisite tools.

## Programmatic registration

Agent sessions can also author tools at runtime through `custom_agent_tools`
(`internal/tools/`), which is a separate, per-session namespace from
`tools_registry`. Tools registered there are not dispatchable until they are
promoted into `tools_registry`.
