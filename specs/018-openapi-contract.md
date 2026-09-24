# SPEC-018: OpenAPI Contract

**Status:** Draft
**Depends On:** SPEC-015 (API & External Interface Layer), SPEC-017 (UI Adapter Layer)
**Created:** 2026-04-12

---

## 1. Overview

Consensus maintains a machine-readable OpenAPI 3.1 specification as the single source of truth for its REST API. The OpenAPI contract is used for:

1. **Auto-generated client SDKs** — TypeScript, Go, Python, etc.
2. **Contract testing** — validate server behavior against the spec
3. **Interactive API documentation** — Swagger UI / Redoc
4. **Tool integration** — let external tools discover the API programmatically
5. **CLI generation** — drive SPEC-016 CLI commands from the spec

---

## 2. Specification Structure

```
specs/
  openapi/
    openapi.yaml              # Root spec (references components)
    paths/
      sessions.yaml           # /api/v1/sessions endpoints
      messages.yaml           # /api/v1/sessions/:id/message
      memory.yaml             # /api/v1/sessions/:id/memory
      tasks.yaml              # /api/v1/sessions/:id/tasks
      tools.yaml              # /api/v1/tools endpoints
      skills.yaml             # /api/v1/skills endpoints
      approvals.yaml          # /api/v1/approvals endpoints
      metrics.yaml            # /api/v1/metrics endpoints
      shim-openai.yaml        # /v1/chat/completions, /v1/models
      shim-anthropic.yaml     # /v1/messages
    components/
      schemas.yaml            # Shared data models
      responses.yaml          # Shared response shapes
      parameters.yaml         # Shared path/query params
      security.yaml           # Auth schemes
    bundled.yaml              # Single-file build output (generated)
```

### Why Split?

The root `openapi.yaml` uses `$ref` to pull in path and component files. This keeps the spec maintainable — each domain (sessions, approvals, shims) is its own file. A build step bundles everything into a single `bundled.yaml` for distribution.

---

## 3. Root Spec

```yaml
# openapi.yaml
openapi: "3.1.0"
info:
  title: Consensus Agent Runtime
  description: |
    Database-native cognitive architecture for AI agents.
    All state lives in the database. The API writes to the same tables the agent reads from.
  version: "0.1.0"
  contact:
    name: Consensus
    url: https://github.com/consensus/consensus
  license:
    name: MIT

servers:
  - url: http://localhost:8090
    description: Local development (SQLite or local Postgres)
  - url: https://{host}
    description: Remote deployment (any Postgres provider)
    variables:
      host:
        default: localhost:8090

security:
  - BearerAuth: []

tags:
  - name: Sessions
    description: Agent session lifecycle
  - name: Memory
    description: Agent memory and context inspection
  - name: Tasks
    description: Task queue management
  - name: Tools
    description: Tool and skill registry
  - name: Approvals
    description: Human-in-the-loop approvals
  - name: Metrics
    description: System metrics and billing
  - name: OpenAI Shim
    description: OpenAI Chat Completions compatible endpoints
  - name: Anthropic Shim
    description: Anthropic Messages API compatible endpoints

paths:
  # Native REST API
  /api/v1/sessions:
    $ref: "./paths/sessions.yaml#/sessions"
  /api/v1/sessions/{sessionId}:
    $ref: "./paths/sessions.yaml#/session"
  /api/v1/sessions/{sessionId}/message:
    $ref: "./paths/messages.yaml"
  /api/v1/sessions/{sessionId}/memory:
    $ref: "./paths/memory.yaml#/memory"
  /api/v1/sessions/{sessionId}/memory/{memoryId}:
    $ref: "./paths/memory.yaml#/memoryItem"
  /api/v1/sessions/{sessionId}/context:
    $ref: "./paths/memory.yaml#/context"
  /api/v1/sessions/{sessionId}/iterations:
    $ref: "./paths/memory.yaml#/iterations"
  /api/v1/sessions/{sessionId}/tasks:
    $ref: "./paths/tasks.yaml#/tasks"
  /api/v1/tasks/{taskId}:
    $ref: "./paths/tasks.yaml#/task"
  /api/v1/tasks/{taskId}/claim:
    $ref: "./paths/tasks.yaml#/claim"
  /api/v1/tools:
    $ref: "./paths/tools.yaml#/tools"
  /api/v1/tools/{toolName}/execute:
    $ref: "./paths/tools.yaml#/execute"
  /api/v1/skills:
    $ref: "./paths/skills.yaml#/skills"
  /api/v1/skills/{skillName}:
    $ref: "./paths/skills.yaml#/skill"
  /api/v1/approvals:
    $ref: "./paths/approvals.yaml#/approvals"
  /api/v1/approvals/{approvalId}:
    $ref: "./paths/approvals.yaml#/approval"
  /api/v1/approvals/{approvalId}/review:
    $ref: "./paths/approvals.yaml#/review"
  /api/v1/sessions/{sessionId}/approvals:
    $ref: "./paths/approvals.yaml#/sessionApprovals"
  /api/v1/metrics:
    $ref: "./paths/metrics.yaml#/metrics"
  /api/v1/sessions/{sessionId}/billing:
    $ref: "./paths/metrics.yaml#/billing"
  /api/v1/sessions/{sessionId}/iterations/{iterationId}/audit:
    $ref: "./paths/metrics.yaml#/audit"

  # OpenAI Shim
  /v1/chat/completions:
    $ref: "./paths/shim-openai.yaml#/chatCompletions"
  /v1/models:
    $ref: "./paths/shim-openai.yaml#/models"

  # Anthropic Shim
  /v1/messages:
    $ref: "./paths/shim-anthropic.yaml#/messages"
  /v1/messages/count_tokens:
    $ref: "./paths/shim-anthropic.yaml#/countTokens"

components:
  $ref: "./components/schemas.yaml"
```

---

## 4. Component Schemas

### 4.1 Core Data Models

```yaml
# components/schemas.yaml (excerpt)

Session:
  type: object
  required: [id, status, agent_name, goal, iteration, created_at]
  properties:
    id:
      type: string
      format: uuid
    status:
      type: string
      enum: [booting, idle, thinking, tool_exec, waiting_sub, paused, completed, failed]
    agent_name:
      type: string
    goal:
      type: string
    model_id:
      type: string
    iteration:
      type: integer
    context_budget:
      type: integer
    budget_used_cents:
      type: integer
    budget_limit_cents:
      type: integer
    max_iterations:
      type: integer
    consecutive_errors:
      type: integer
    heartbeat_at:
      type: string
      format: date-time
    created_at:
      type: string
      format: date-time
    completed_at:
      type: string
      format: date-time

CreateSessionRequest:
  type: object
  required: [agent_name, goal]
  properties:
    agent_name:
      type: string
    goal:
      type: string
    model_id:
      type: string
      default: gpt-4o
    context_budget:
      type: integer
      default: 128000
    hitl_config:
      type: object
      properties:
        require_approval_for_destructive:
          type: boolean
          default: true

Message:
  type: object
  required: [content, type]
  properties:
    content:
      type: string
    type:
      type: string
      enum: [user_instruction, user_correction, system_override]

MemoryEvent:
  type: object
  properties:
    id:
      type: string
      format: uuid
    session_id:
      type: string
      format: uuid
    type:
      type: string
      enum: [text_block, tool_result, agent_response, error, system, summary]
    content:
      type: string
    iteration_created:
      type: integer
    sequence_number:
      type: integer
    created_at:
      type: string
      format: date-time

ApprovalRequest:
  type: object
  properties:
    id:
      type: string
      format: uuid
    session_id:
      type: string
      format: uuid
    iteration:
      type: integer
    request_type:
      type: string
      enum: [tool_execution, destructive_action, budget_override, schema_change, sub_agent_spawn, custom]
    description:
      type: string
    risk_level:
      type: string
      enum: [low, medium, high, critical]
    status:
      type: string
      enum: [pending, approved, rejected, expired, modified]
    target_tool:
      type: string
    target_sql:
      type: string
    context:
      type: object
    reviewer_id:
      type: string
    review_notes:
      type: string
    created_at:
      type: string
      format: date-time
    reviewed_at:
      type: string
      format: date-time

ApprovalReview:
  type: object
  required: [decision]
  properties:
    decision:
      type: string
      enum: [approved, rejected, modified]
    notes:
      type: string
    modified_sql:
      type: string

Error:
  type: object
  required: [error]
  properties:
    error:
      type: object
      required: [code, message]
      properties:
        code:
          type: string
        message:
          type: string
        details:
          type: object
```

### 4.2 Shared Parameters

```yaml
# components/parameters.yaml

SessionId:
  name: sessionId
  in: path
  required: true
  schema:
    type: string
    format: uuid

ApprovalId:
  name: approvalId
  in: path
  required: true
  schema:
    type: string
    format: uuid

Limit:
  name: limit
  in: query
  schema:
    type: integer
    default: 50

StatusFilter:
  name: status
  in: query
  schema:
    type: string
```

### 4.3 Security Schemes

```yaml
# components/security.yaml

BearerAuth:
  type: http
  scheme: bearer
  description: API key prefixed with cs_ (e.g., cs_sk_a1b2c3d4...)
```

---

## 5. Build Process

### 5.1 Bundle

The split spec is bundled into a single file for distribution:

```bash
# Using redocly CLI
npx @redocly/cli bundle specs/openapi/openapi.yaml --output specs/openapi/bundled.yaml

# Validate
npx @redocly/cli lint specs/openapi/bundled.yaml
```

### 5.2 Generate SDKs

```bash
# TypeScript client
npx openapi-typescript specs/openapi/bundled.yaml -o packages/client/src/types.ts
npx openapi-fetch --input specs/openapi/bundled.yaml --output packages/client/src/

# Go client
oapi-codegen --package consensus specs/openapi/bundled.yaml > client/client.go

# Python client
datamodel-codegen --input specs/openapi/bundled.yaml --output client/python/
```

### 5.3 Generate Docs

```bash
# Swagger UI (served at /doc/api when server is running — /doc is the opencode shim's own UI)
npx swagger-ui-watcher specs/openapi/bundled.yaml

# Redoc static HTML
npx redocly build-docs specs/openapi/bundled.yaml --output docs/api.html
```

---

## 6. Contract Testing

### 6.1 Strategy

Every API endpoint has corresponding contract tests that validate:
1. **Request validation** — malformed requests are rejected
2. **Response shape** — responses match the schema
3. **Status codes** — correct codes for success and error cases
4. **Auth** — unauthenticated/forbidden requests are rejected

### 6.2 Test Structure

```typescript
// tests/api/sessions.test.ts
describe('POST /api/v1/sessions', () => {
    it('creates a session with valid request', async () => {
        const res = await client.POST('/api/v1/sessions', {
            body: { agent_name: 'test', goal: 'do stuff' }
        });
        assert.match(res.data.id, /^[\da-f-]{36}$/);
        assert.equal(res.data.status, 'booting');
        assert.ok(res.data.api_key);
    });

    it('rejects missing required fields', async () => {
        const res = await client.POST('/api/v1/sessions', { body: {} });
        assert.equal(res.status, 400);
        assert.equal(res.error.code, 'INVALID_REQUEST');
    });

    it('rejects without auth', async () => {
        const res = await unauthenticatedClient.POST('/api/v1/sessions', { body: { agent_name: 'test', goal: 'x' } });
        assert.equal(res.status, 401);
    });
});
```

### 6.3 Schemathesis (Property-Based)

```bash
# Auto-generate test cases from the spec
schemathesis run specs/openapi/bundled.yaml \
    --base-url http://localhost:8090 \
    --header "Authorization: Bearer cs_ak_..." \
    --checks all
```

---

## 7. Versioning

### 7.1 Spec Versioning

The OpenAPI spec version is separate from the Consensus runtime version:
- **Spec version** (`info.version`): follows semver, incremented when API shape changes
- **Runtime version** (`engine_version` in sessions table): tracks schema version

### 7.2 Breaking Changes

A breaking change is any change that:
- Removes an endpoint
- Removes a field from a response
- Changes a field type
- Adds a required field to a request
- Changes a status code

Breaking changes require a new major version and a new API prefix (`/api/v2/`).

### 7.3 Non-Breaking Changes

These are allowed within a version:
- Adding new endpoints
- Adding new optional fields
- Adding new enum values (with server-side defaults)
- Adding new query parameters (with defaults)

---

## 8. CI Integration

```yaml
# .github/workflows/api-spec.yaml
name: API Spec Validation
on: [push, pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Bundle spec
        run: npx @redocly/cli bundle specs/openapi/openapi.yaml --output specs/openapi/bundled.yaml
      - name: Lint spec
        run: npx @redocly/cli lint specs/openapi/bundled.yaml
      - name: Check no breaking changes
        run: npx oasdiff breaking specs/openapi/bundled.yaml --base main
      - name: Generate types
        run: npx openapi-typescript specs/openapi/bundled.yaml --check
```

---

## 9. Serving the Spec

The Consensus server serves the OpenAPI spec at runtime:

| Path | Content |
|---|---|
| `GET /doc/api` | Swagger UI for the REST API (interactive docs; servers URL derived from the request Host) |
| `GET /doc` | Swagger UI for the opencode shim surface (SPEC-017) — NOT the REST API docs |
| `GET /openapi.yaml` | Raw YAML spec |
| `GET /openapi.json` | JSON spec |

The spec is embedded into the Go binary (specs/embed.go), so it is served
regardless of the process working directory and in the Docker image (which
does not copy specs/). In development the server prefers
`specs/openapi/bundled.yaml` relative to the CWD when it exists, so
re-bundling picks up live edits without a rebuild. Served by the Go binary's
HTTP handler regardless of database backend.

---

## 10. Backend Parity

| Feature | Postgres Backend | SQLite Backend |
|---|---|---|
| Spec serving | Go handler at `/doc/api`, `/openapi.yaml` (embedded spec) | Same — shared code |
| Contract tests | Same test suite | Same test suite |
| SDK generation | Same process | Same process |
| PostgREST overlap | PostgREST auto-generates OpenAPI — Consensus spec overrides/supplements | Manual spec maintained |

**Note on PostgREST:** Supabase's PostgREST already generates an OpenAPI spec from database schema. The Consensus spec is a superset — it covers the same CRUD operations (with the same shapes) plus custom endpoints (message sending, tool execution, approvals, shims) that PostgREST cannot generate. The Consensus spec is authoritative.

---

## 11. Open Questions

1. **PostgREST integration**: Should the Consensus spec reference PostgREST's auto-generated endpoints (for standard CRUD) or fully replace them?
2. **Spec-first vs code-first**: Should the OpenAPI spec be hand-maintained (spec-first) or auto-generated from code annotations (code-first)? This spec assumes spec-first.
3. **gRPC/protobuf**: Will there ever be a need for a binary protocol, or is HTTP+JSON+SSE sufficient?
