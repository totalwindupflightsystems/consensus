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
      shim.yaml               # Opencode protocol shim (SPEC-017)
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
    description: OpenAI Chat Completions compatible endpoints — PLANNED / NOT IMPLEMENTED (no paths file exists yet)
  - name: Anthropic Shim
    description: Anthropic Messages API compatible endpoints — PLANNED / NOT IMPLEMENTED (no paths file exists yet)

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
  /api/v1/tool-requests:
    $ref: "./paths/tools.yaml#/toolRequests"
  /api/v1/tool-requests/{requestId}:
    $ref: "./paths/tools.yaml#/toolRequest"
  /api/v1/sessions/{sessionId}/tool-calls:
    $ref: "./paths/tools.yaml#/sessionToolCalls"
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

  # ── Opencode Protocol Shim (SPEC-017) ──
  # (root openapi.yaml references ./paths/shim.yaml for /global/*, /session/*,
  # /config, /find, /file/*, /permission*, /tui/*, /lsp, /mcp, /event,
  # /project*, /vcs*, /instance* — see shim.yaml for the full surface)
  #
  # SHIM-GAP-006 — declared semantic divergence on /mcp: the upstream opencode
  # document (specs/openapi/upstream/openapi-1.18.33.json) declares mcp.status
  # (GET /mcp) and mcp.add (POST /mcp) as MCP *management* operations. Consensus
  # keeps its MCP *protocol* delegation on both methods (streamable-HTTP
  # transport, MCP-DIRECT-001) and declares the divergence instead of adopting
  # upstream's semantics. The served document paths/shim.yaml (mcp: block)
  # describes the real streamable-HTTP behavior — responses 200/202/400/401/405/
  # 501 — so a client generated from OUR bundle can predict the endpoint. A
  # client generated from the UPSTREAM document cannot call mcp.status/mcp.add
  # against this runtime; the reason and the full decision record live in
  # specs/015-api-and-mcp.md §5.4.1 and
  # docs/reports/shim-source-002-baseline-diff-2026-09-29.md (SHIM-GAP-006
  # addendum). scripts/compare-opencode-declared-served.py classifies these two
  # rows as SEMANTIC-DIFFERENT (SHIM-SEMANTIC-001/002).

  # OpenAI/Anthropic compat endpoints (/v1/chat/completions, /v1/models,
  # /v1/messages, /v1/messages/count_tokens) — PLANNED / NOT IMPLEMENTED.
  # These are NOT in the served spec and NOT registered in the shim router.
  # When implemented, they belong in new paths files (e.g. shim-openai.yaml,
  # shim-anthropic.yaml, which do not exist yet).

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
    budget_limit_cents:
      type: integer
      default: 0
      description: Per-session cost cap in cents, enforced by the harness before each LLM call. 0 or absent = no limit.
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
# Swagger UI (served at /doc/api when server is running)
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
| `GET /doc` | Machine-readable OpenAPI document for the opencode-compatible surface (JSON by default; YAML when explicitly requested) |
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

---

## 12. Declared vs Served: Narrowing Decisions

An operation the shim *answers* can still be one a client cannot *use*. SPEC-024
§A.3 (SHIM-SOURCE-001) requires the declared surface and the served surface to be
compared mechanically after every refresh (declared == served, per path); this
section is the in-repo record of every place the served surface is deliberately
NARROWER than the declared one, so the narrowing is visible to a client reading
the contract instead of only to someone reading `server.go`.

### 12.1 The two narrowing classes

| Class | Served outcome | Served-surface marker | Comparator class |
|---|---|---|---|
| Intentional narrowing | typed `501` `not_implemented` (the operation is not translated at all) | `x-shim-intentional-narrowing: true` + `narrowing-rationale` | `intentional_narrowing` |
| **Error-contract narrowing** | a declared ERROR code (the operation is answered, but its declared success code is unreachable — e.g. `404` from `POST /permission/{requestID}/reply` while upstream declares `200,400,404`) | `x-shim-error-contract-narrowing: true` + `narrowing-rationale` | `narrowed_error_contract_only` |

Widening to the declared contract is always preferred; a narrowing is the honest
fallback when the runtime state the declared success describes does not exist.
**Neither class may be silent.** The comparator
(`scripts/compare-opencode-declared-served.py`) DETECTS the error-contract class
from the declared response set and the served outcome — never from a hand list,
so a newly-narrowed route cannot hide inside `covered` merely because the error it
answers is in the declared set — and **aborts** when a detected narrowing carries
no rationale. (An operation whose declared set contains NO success code is not an
error-contract narrowing at all — answering a declared error code is then the
whole declared contract.)

`counts.narrowed_unexplained` in
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is the number of
narrowed rows WITHOUT a recorded rationale. It is `0` by construction: a
regeneration that would produce a non-zero value exits non-zero instead of writing
the artifact. The count is also asserted by
`internal/shim/opencode/shim_gap_004_test.go::TestDeclaredVsServedAccountsForEveryNarrowing`,
together with the rule that every one of the 11 findings below is either fully
covered or carries a non-empty rationale.

### 12.2 The 11 findings (SHIM-GAP-004) and their disposition

At the pinned refresh (upstream v1.18.33, `openapi-1.18.33.surface.json`) the
2026-09-29 triage report (`docs/reports/shim-source-002-baseline-diff-2026-09-29.md`)
carried `SHIM-NARROWED-001…011` — operations the shim answered with only the error
half of the contract. Each one is now either WIDENED to every declared response
code or RECORDED here with the reason. Six were widened by the earlier ROUTE-FIX
wave this row inherits; SHIM-GAP-004 widened the two that were still fixable with
the state the runtime already owns and recorded the rest.

| # | operationId | path + method | declared | disposition | reason / evidence |
|---|---|---|---|---|---|
| 001 | `permission.reply` | `POST /permission/{requestID}/reply` | 200,400,404 | **WIDENED** | The runtime already owned the state and the write path: the declared body `{reply*: once\|always\|reject, message?}` (`additionalProperties: false`) is validated (400 Body/Payload), an unknown or no-longer-pending id answers the declared typed `PermissionNotFoundError` (404), and a pending row is resolved through the REAL `approval_requests` write path — the same mapping `permission.respond`/`resolvePermission` use — answering the declared boolean `200 true` plus the `approval_resolved` SSE event. `permissionReply`, `internal/shim/opencode/server.go`. |
| 002 | `project.update` | `PATCH /project/{projectID}` | 200,400,404 | **WIDENED** (ROUTE-FIX-034) | Resolvable singleton-workspace ids persist declared `name`/`icon`/`commands` metadata and return the derived Project; malformed bodies/selectors answer 400, unknown ids the typed `ProjectNotFoundError` 404. |
| 003 | `question.reject` | `POST /question/{requestID}/reject` | 200,400,404 | **RECORDED** | No question producer exists (see 004). The declared 400 for an ill-shaped `requestID` (pattern `^que`) and the declared 404 `QuestionNotFoundError` are served; the operation declares no requestBody, so no body is read or required. |
| 004 | `question.reply` | `POST /question/{requestID}/reply` | 200,400,404 | **RECORDED** (400 arm widened) | No question producer exists in this runtime: upstream's `QuestionRequest` (required id `^que`, `sessionID`, `questions[]` of `QuestionInfo`) has no counterpart — the human-input surface is `approval_requests` (SPEC-014), `GET /question` answers the declared EMPTY list rather than inventing entries, and nothing in the shim or the native API writes a question row. No `^que` id can exist, so the declared 200 boolean would assert that a question was answered when none exists. The declared 400 (ill-shaped id, and now the declared `{answers*: QuestionAnswer[]}` requestBody) and the 404 are served. |
| 005 | `session.deleteMessage` | `DELETE /session/{sessionID}/message/{messageID}` | 200,400,404,409 | **WIDENED** (ROUTE-FIX-035) | All four declared codes reachable: 400 (blank/ill-shaped `messageID`), 404 (unknown session or message), 409 `SessionBusyError` (mid-turn session), and the declared boolean 200 — truthfully `false` for a known message, because `memory_events` is append-only (SPEC-002 §2.1; triggers in migrations 017/018) so no delete engine exists. |
| 006 | `part.delete` | `DELETE /session/{sessionID}/message/{messageID}/part/{partID}` | 200,400,404 | **RECORDED** | No part store: parts are SYNTHESIZED on read by `getMessageByID` from `memory_events.content` as one id-less text part, so no part carries the declared `^prt` id and the declared 200 boolean would report deleting something that cannot be addressed. ROUTE-FIX-036 gave the operation its own handler so the declared 400 (blank/ill-shaped `messageID ^msg` / `partID ^prt`) and the declared 404 are served. |
| 007 | `part.update` | `PATCH /session/{sessionID}/message/{messageID}/part/{partID}` | 200,400,404 | **RECORDED** | Same missing part store. Unlike `session.deleteMessage` (whose declared 200 is a boolean that can truthfully report `false`) this operation declares a Part OBJECT the runtime cannot produce, so the declared 404 `NotFoundError` is the answer for every well-formed `partID` and the declared 400 arms are served. Previously scored `covered` because the comparison only asked whether the served status was IN the declared set — that is the invisibility this section removes. |
| 008 | `permission.respond` | `POST /session/{sessionID}/permissions/{permissionID}` | 200,400,404 | **WIDENED** (ROUTE-FIX-038) | Declared body validated, unknown/foreign/non-pending permission answers the declared 404, a resolved permission the declared boolean `true` plus the SSE event. |
| 009 | `session.todo` | `GET /session/{sessionID}/todo` | 200,400,404 | **WIDENED** (ROUTE-FIX-039) | Unknown session 404; known session answers the declared `Todo[]` as the truthful EMPTY list (no per-session todo store exists). The declared 400 has no client-controllable input: the only parameter is the path `sessionID`, and the optional `directory`/`workspace` selectors are accepted and ignored under the singleton-workspace convention. |
| 010 | `session.unrevert` | `POST /session/{sessionID}/unrevert` | 200,400,404,409 | **WIDENED** (ROUTE-FIX-040) | Unknown session 404, mid-turn session the declared 409 `SessionBusyError`, idle session the declared 200 with the Session AS IT STANDS (no revert engine exists; nothing is claimed to have been restored). |
| 011 | `tui.selectSession` | `POST /tui/select-session` | 200,400,404 | **WIDENED** | The bare `/tui/` catch-all answered the declared 404 for every request. The declared body `{sessionID*}` (`additionalProperties: false`) is now validated strictly (400 Body/Payload), an unknown session answers the declared 404 `NotFoundError`, and a session that EXISTS answers the declared boolean 200 `false` — the shim attaches no opencode TUI process, so nothing was navigated (the `tui.showToast`/`tui.executeCommand` truthful-false precedent). `tuiSelectSession`, `internal/shim/opencode/server.go`. |

Two further notes on 009/010 and the widened rows: their declared 400 arms are
served where a client can actually violate the schema; where the declared set
lists codes the operation's declared inputs cannot reach, that is stated above
rather than papered over with a fabricated arm.

### 12.3 Every recorded error-contract narrowing at this refresh

The comparator's detection (not a hand list) surfaced rows that were previously
scored `covered` while the declared success was unreachable. All of them are now
recorded with a reason on their served-surface row and carried in
`narrowed_error_contract_only` of the artifact:

| id | operationId | path + method | declared | why the declared success is unreachable |
|---|---|---|---|---|
| SHIM-NARROWED-003 | `question.reject` | `POST /question/{requestID}/reject` | 200,400,404 | no question producer (see §12.2 row 003) |
| SHIM-NARROWED-004 | `question.reply` | `POST /question/{requestID}/reply` | 200,400,404 | no question producer (see §12.2 row 004) |
| SHIM-NARROWED-006 | `part.delete` | `DELETE /session/{sessionID}/message/{messageID}/part/{partID}` | 200,400,404 | no part store, parts synthesized id-less, append-only ledger (row 006) |
| SHIM-NARROWED-007 | `part.update` | `PATCH /session/{sessionID}/message/{messageID}/part/{partID}` | 200,400,404 | same, and the declared 200 is an object the runtime cannot produce (row 007) |
| SHIM-NARROWED-012 | `pty.get` | `GET /pty/{ptyID}` | 200,400,404 | no PTY registry: `pty.list` is truthfully empty and `pty.create` is the declared intentional narrowing, so no `^pty` id can exist |
| SHIM-NARROWED-013 | `pty.update` | `PUT /pty/{ptyID}` | 200,400,404 | same: there is no PTY whose title/size could be applied |
| SHIM-NARROWED-014 | `pty.remove` | `DELETE /pty/{ptyID}` | 200,400,404 | same: the declared 200 boolean would report removing a terminal that never existed |
| SHIM-NARROWED-015 | `pty.connect` | `GET /pty/{ptyID}/connect` | 200,403,404 | the shim opens no PTY WebSocket and connects no session; the declared 403 `{_tag: Forbidden}` is served for a selector outside the single served workspace (the operation declares no 400) |
| SHIM-NARROWED-016 | `pty.connectToken` | `POST /pty/{ptyID}/connect-token` | 200,400,403,404 | the declared 200 is a short-lived single-use credential the shim never mints |
| SHIM-NARROWED-017 | `mcp.auth.start` | `POST /mcp/{name}/auth` | 200,400,404 | no MCP OAuth flow: neither `authorizationUrl` nor `oauthState` can be minted truthfully |
| SHIM-NARROWED-018 | `mcp.auth.authenticate` | `POST /mcp/{name}/auth/authenticate` | 200,400,404 | the declared 200 `MCPStatus` is live connection state; the shim runs no MCP client and tracks no connection |
| SHIM-NARROWED-019 | `mcp.auth.callback` | `POST /mcp/{name}/auth/callback` | 200,400,404 | the shim parks no OAuth flow, so there is nothing to exchange `code` against |
| SHIM-NARROWED-020 | `session.share` | `POST /session/{sessionID}/share` | 200,400,404,500 | Consensus publishes no sessions (export is native, SPEC-015), so no share URL can be produced |
| SHIM-NARROWED-021 | `session.unshare` | `DELETE /session/{sessionID}/share` | 200,400,404,500 | the runtime keeps no share concept, so no session has an active share to remove |

`counts`: `narrowed_error_contract_only: 14`, **`narrowed_unexplained: 0`**,
`intentional_narrowing: 1` (`pty.create`, SPEC-017 §3.9), `semantic_different: 2`
(the `/mcp` protocol delegation, SPEC-024 §A).

Making parts addressable (`part.delete`/`part.update`) or building a PTY/MCP-OAuth
runner would convert those rows to full coverage; each is a design change with its
own row, not a contract detail. Until then this table is the contract.

### 12.4 Reproducing and checking this section

```bash
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/opencode-declared-vs-served
diff <(jq -S . /tmp/opencode-declared-vs-served/declared-vs-served.json) \
     specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json   # no output = reproducible
```
The comparator exits non-zero (naming the row) when a detected error-contract
narrowing has no rationale, and also when a row claims the marker while the
runtime answers the declared success — so this section cannot drift away from the
code silently.
