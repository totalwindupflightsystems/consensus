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
    $ref: "./paths/tools.yaml#/tools"          # GET list + POST register (operationId createTool)
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

### 12.3 The recorded error-contract narrowings, with code evidence

The comparator's detection (not a hand list) surfaces the rows whose served
answer is a declared ERROR code while the declared response set also carries a
success code. At the pinned refresh (upstream v1.18.33) there are **14**. Each
one is recorded here with:

- the **rationale** — why the declared success is unreachable for ANY input;
- the **code evidence** (`file:line symbol`) — the handler that answers the row
  and the exact statement that refuses the declared success, so the claim can be
  checked instead of believed;
- the **widening requirement** — what would have to be BUILT for the declared
  success to be answerable truthfully.

SHIM-GAP-004 recorded the reasons in prose; this row (SHIM-GAP-005 — the second
board row under that id; the earlier one declared the 17 shim extensions) re-derived
every row against the code at `main`, filed the evidence above, and pinned the
whole ledger with `internal/shim/opencode/shim_gap_005_test.go`, which fails when
a detected narrowing is missing from this section, when a citation no longer
resolves to a real file and symbol, or when a narrowed operation answers its
declared success at runtime.

**Widening was probed for all 14 before recording** (that is the order this
section requires): for every row the declared success is an object, a boolean or
a credential whose truth depends on state the runtime does not own — a PTY
registry, an MCP OAuth client, a question producer, an addressable part store, a
session-publishing pipeline. None of the 14 is widenable by a contract detail;
each names the engine it needs. Where the runtime DOES own the state, the row was
widened instead of recorded — see §12.2 (`permission.reply`, `tui.selectSession`,
`permission.respond`, `session.deleteMessage`, `session.todo`, `session.unrevert`,
`project.update`).

#### SHIM-NARROWED-003 — question.reject (`POST /question/{requestID}/reject`)
- declared: 200 boolean ("Question rejected successfully"), 400, 404
- served: 404 typed `QuestionNotFoundError` for every well-formed requestID; 400 for an ill-shaped id (declared pattern `^que`)
- rationale: no question producer exists in this runtime — nothing writes a question row, so no `^que` id can name one and the declared boolean 200 could only ever assert that a question was rejected when none exists.
- evidence: `internal/shim/opencode/server.go:2994 handleQuestionByID`; refusal `internal/shim/opencode/server.go:3024 writeUpstreamRequestNotFound`
- widening requires: a question producer (upstream `QuestionRequest`: `^que` id, sessionID, questions[]) — the runtime's human-input surface is `approval_requests` (SPEC-014), a design change with its own row.

#### SHIM-NARROWED-004 — question.reply (`POST /question/{requestID}/reply`)
- declared: 200 boolean ("Question answered successfully"), 400, 404
- served: 404 typed `QuestionNotFoundError`; 400 for an ill-shaped id AND for a body violating the declared `{answers*: QuestionAnswer[]}` schema (the 400 arm widened by SHIM-GAP-004)
- rationale: same missing question producer as SHIM-NARROWED-003; `GET /question` answers the declared empty list rather than inventing entries, so the declared boolean 200 has nothing to report.
- evidence: `internal/shim/opencode/server.go:2994 handleQuestionByID`; refusal `internal/shim/opencode/server.go:3024 writeUpstreamRequestNotFound`
- widening requires: the same question producer.

#### SHIM-NARROWED-006 — part.delete (`DELETE /session/{sessionID}/message/{messageID}/part/{partID}`)
- declared: 200 boolean ("Successfully deleted part"), 400, 404
- served: 404 `NotFoundError` for every well-formed partID; 400 for a blank/ill-shaped messageID (`^msg`) or partID (`^prt`)
- rationale: the runtime keeps no part store and no part id is addressable — parts are SYNTHESIZED id-less on read — so no `^prt` value names a part that could be deleted, and `memory_events` is append-only (SPEC-002 §2.1; trigger migrations 017/018).
- evidence: `internal/shim/opencode/session_p1_routes.go:793 sessionPartDelete`; refusal `internal/shim/opencode/session_p1_routes.go:835 writeOpencodeError`; the id-less synthesis `internal/shim/opencode/server.go:1901` (in `getMessageByID`, declared `internal/shim/opencode/server.go:1864`)
- widening requires: an addressable part store — stable `prt` ids over the synthesized parts plus the part routes that list/get them.

#### SHIM-NARROWED-007 — part.update (`PATCH /session/{sessionID}/message/{messageID}/part/{partID}`)
- declared: 200 Part ("Successfully updated part"), 400, 404
- served: 404 `NotFoundError` for every well-formed partID; 400 for ill-shaped path parameters and for a body that is not a Part object
- rationale: same missing part store, and unlike a boolean the declared 200 is a Part OBJECT — echoing the synthesized part unchanged would assert an update that was never applied.
- evidence: `internal/shim/opencode/session_p1_routes.go:691 sessionPartUpdate`; refusal `internal/shim/opencode/session_p1_routes.go:751 writeOpencodeError`
- widening requires: the addressable part store AND a part-editing engine.

#### SHIM-NARROWED-012 — pty.get (`GET /pty/{ptyID}`)
- declared: 200 Pty, 400, 404
- served: 404 typed `PtyNotFoundError` for every well-formed ptyID; 400 for an ill-shaped id or a blank/non-directory selector
- rationale: the runtime keeps no PTY registry and spawns no pseudo-terminal (`pty.list` answers the truthful empty array, `pty.create` is the declared intentional narrowing), so no ptyID names a terminal and the declared `Pty` object cannot be produced without asserting one that does not exist.
- evidence: `internal/shim/opencode/pty_routes.go:214 ptyGet`; refusal `internal/shim/opencode/pty_routes.go:224 writeOpencodeNotFoundError`
- widening requires: a PTY registry + runner in the runtime (create/get/update/remove over real sessions).

#### SHIM-NARROWED-013 — pty.update (`PUT /pty/{ptyID}`)
- declared: 200 Pty, 400, 404
- served: 404 typed `PtyNotFoundError`; 400 for the same inputs plus a body violating the declared `{title?, size?{rows*,cols*}}` schema
- rationale: no PTY exists whose title/size could be applied, so the declared 200 Pty would report an update that did not happen.
- evidence: `internal/shim/opencode/pty_routes.go:245 ptyUpdate`; refusal `internal/shim/opencode/pty_routes.go:258 writeOpencodeNotFoundError`
- widening requires: the PTY registry + runner.

#### SHIM-NARROWED-014 — pty.remove (`DELETE /pty/{ptyID}`)
- declared: 200 boolean ("Session removed"), 400, 404
- served: 404 typed `PtyNotFoundError`; 400 for an ill-shaped id or a blank/non-directory selector
- rationale: the declared boolean is only meaningful for a terminal that exists; the runtime can create none (`pty.create` is the intentional narrowing), so the boolean could only report a removal that never happened.
- evidence: `internal/shim/opencode/pty_routes.go:330 ptyRemove`; refusal `internal/shim/opencode/pty_routes.go:340 writeOpencodeNotFoundError`
- widening requires: the PTY registry + runner.

#### SHIM-NARROWED-015 — pty.connect (`GET /pty/{ptyID}/connect`)
- declared: 200 boolean ("Connected session"), 403, 404
- served: 404 `NotFoundError` (NamedError shape) for a well-formed ptyID; the declared 403 `{_tag:"Forbidden"}` for a selector outside the single served workspace (this operation declares no 400, so a refusal is reported with the declared 403)
- rationale: the shim opens no PTY WebSocket and connects no terminal — there is nothing to connect to and nothing to report as connected.
- evidence: `internal/shim/opencode/pty_routes.go:356 handlePtyConnect`; refusal `internal/shim/opencode/pty_routes.go:366 writeOpencodeNamedError`
- widening requires: the PTY registry plus a WebSocket/attach surface.

#### SHIM-NARROWED-016 — pty.connectToken (`POST /pty/{ptyID}/connect-token`)
- declared: 200 `PtyTicketConnectToken` `{ticket, expires_in}`, 400, 403, 404
- served: 404 typed `PtyNotFoundError`; 400 for input violating the declared patterns/selectors; the declared 403 `PtyForbiddenError` for an existing foreign workspace
- rationale: the declared 200 is a short-lived single-use CREDENTIAL; the shim mints none (no ticket store, no terminal to attach to), and a minted ticket would be a fabricated credential.
- evidence: `internal/shim/opencode/pty_routes.go:393 handlePtyConnectToken`; refusal `internal/shim/opencode/pty_routes.go:419 writeOpencodeNotFoundError`
- widening requires: a ticket mint/store bound to a real PTY attach surface.

#### SHIM-NARROWED-017 — mcp.auth.start (`POST /mcp/{name}/auth`)
- declared: 200 `{authorizationUrl*, oauthState*}`, 400, 404
- served: 400 `McpUnsupportedOAuthError` for a server the settings store resolves; 400 `InvalidRequestError` for malformed input or an unreadable store; typed 404 `McpServerNotFoundError` for an unknown name
- rationale: the declared 200 requires BOTH strings and the runtime implements no MCP OAuth flow — a minted URL/state pair would be a lie the caller hands to a browser.
- evidence: `internal/shim/opencode/server.go:5063 handleMCPAuthStart`; refusal `internal/shim/opencode/server.go:5081 writeMCPUnsupportedOAuthError`
- widening requires: an MCP OAuth client (dynamic client registration + authorization-URL construction).

#### SHIM-NARROWED-018 — mcp.auth.authenticate (`POST /mcp/{name}/auth/authenticate`)
- declared: 200 `MCPStatus`, 400, 404
- served: same arms as SHIM-NARROWED-017 (400 `McpUnsupportedOAuthError` for a resolved server)
- rationale: the declared 200 is LIVE connection state (connected / needs_auth / …); the shim runs no MCP client and tracks no connection, so every `MCPStatus` value would be invented.
- evidence: `internal/shim/opencode/server.go:5105 handleMCPAuthAuthenticate`; refusal `internal/shim/opencode/server.go:5123 writeMCPUnsupportedOAuthError`
- widening requires: an MCP client that tracks per-server connection status (plus the OAuth flow).

#### SHIM-NARROWED-019 — mcp.auth.callback (`POST /mcp/{name}/auth/callback`)
- declared: 200 `MCPStatus`, 400, 404
- served: 400 `InvalidRequestError` naming the real reason for a well-formed `{code*}` body; 400 for a body violating the declared schema; typed 404 for an unknown name
- rationale: the exchange needs the OAuth flow the matching start/authenticate call would have parked, and this runtime parks none — so there is nothing to exchange `code` against, and no connection status to report.
- evidence: `internal/shim/opencode/server.go:5154 handleMCPAuthCallback`; refusal `internal/shim/opencode/server.go:5176 writeOpencodeInvalidRequestError`
- widening requires: a parked OAuth flow (state + PKCE) and a token exchange.

#### SHIM-NARROWED-020 — session.share (`POST /session/{sessionID}/share`)
- declared: 200 Session carrying `share {url}`, 400, 404, 500
- served: 400 `INVALID_REQUEST` naming the limitation for a known session; 404 for an unknown session
- rationale: the declared 200 asserts the session IS shared with a URL; Consensus publishes nothing (export is native, SPEC-015) and a fabricated URL is forbidden, so no input can truthfully reach the declared success.
- evidence: `internal/shim/opencode/session_p1_routes.go:226 sessionShare`; refusal `internal/shim/opencode/session_p1_routes.go:230 writeOpencodeError`
- widening requires: a session-publishing pipeline with real share ids and URLs.

#### SHIM-NARROWED-021 — session.unshare (`DELETE /session/{sessionID}/share`)
- declared: 200 Session, 400, 404, 500
- served: 404 `NotFoundError` naming the reason for a known session; 404 for an unknown session
- rationale: with no share concept there is no active share to remove, and the declared 200 would assert that a share was removed that never existed. (`session.unrevert` can answer its declared 200 as the session *as it stands* because that operation's post-condition is a state the runtime already holds; here the post-condition is the ABSENCE of a share the runtime never models — and the sibling `session.share` always answers 400, so a 200 "successfully unshared" would report a transition the shim cannot perform in either direction.)
- evidence: `internal/shim/opencode/session_p1_routes.go:195 sessionUnshare`; refusal `internal/shim/opencode/session_p1_routes.go:201 writeOpencodeError`
- widening requires: the same session-publishing pipeline as SHIM-NARROWED-020.

#### SHIM-NARROWED-022 — v2.credential.update (`PATCH /api/credential/{credentialID}`)
- declared: 204 <No Content>, 400, 401
- served: 400 `InvalidRequestError` for every well-formed request (naming the credentialID and the missing store); 400 for a blank credentialID, a body violating the declared `{label*: string}` additionalProperties:false schema, or a malformed deepObject location selector; 401 via the /api/* auth wall's v2 envelope
- rationale: the runtime keeps NO credential store — no credentials table exists in the migration set (001..026), `api_keys` (the auth material the shim does hold) is never exposed through this declared path, and the provider OAuth surface (provider.auth, provider.{providerID}/oauth/callback) stores nothing a caller could relabel — so the declared 204 would echo a label update that never happened. This is the ROUTE-FIX-037 part.update precedent applied to the v2 credential family (ROUTE-ADD-008).
- evidence: `internal/shim/opencode/api_credential_update.go:77 credentialUpdate`; refusal `internal/shim/opencode/api_credential_update.go:131 writeV2InvalidRequest` (the well-formed arm — every malformed arm is also a writeV2InvalidRequest call inside credentialUpdate); dispatcher `internal/shim/opencode/api_credential.go handleApiCredential`
- widening requires: a credential store (a credentials table the OAuth surfaces actually write, with labels the declared shape can express).

#### SHIM-NARROWED-100 — v2.event.subscribe (`GET /api/event`)
- declared: 200 "Event stream" (SSE of `V2Event`), 400, 401
- served: 400 `InvalidRequestError` naming the reason, for every well-formed
  subscribe (the operation declares no parameters and no requestBody, so there
  is no narrower input arm); 401 `UnauthorizedError` unauthenticated (the v2
  envelope via the exact-path `v2EnvelopeRoutePaths` entry); 405 for undeclared
  methods
- rationale: the declared 200 is an SSE stream of `V2Event` — a discriminated
  union whose members carry a required id matching `^evt_`, a required
  `data.info: Message` (for the message.* members) and a required `durable`
  aggregate envelope (`{aggregateID, seq, version}`). The runtime's only real
  event stream is the one backing the v1 `/event` route (`handleGlobalEvent`),
  and it emits v1-shaped events (`message.created`, `tool.started`,
  `permission.requested`, ...) with none of those members; `internal/db` keeps
  no v2 session-event store that could back the declared frames truthfully.
  Re-emitting the v1 frames under the v2 route would present an undeclared
  payload shape as the declared contract, so the shim never fabricates the 200.
  (ROUTE-FIX-037 precedent; ROUTE-ADD-009.)
- evidence: `internal/shim/opencode/api_event_subscribe.go:41 handleApiEventSubscribe`; refusal `internal/shim/opencode/api_event_subscribe.go:47 writeV2InvalidRequest`
- widening requires: a real v2 session-event store (a design change with its
  own row, not a contract detail).

#### SHIM-NARROWED-101 — v2.pty.get (`GET /api/pty/{ptyID}`)
- declared: 200 Pty, 400, 401, 404
- served: 404 typed `PtyNotFoundError` for every well-formed ptyID; 400 for an ill-shaped id (declared pattern `^pty`) or a blank/non-directory selector; 401 via the /api/* auth wall's v2 envelope
- rationale: the same missing PTY registry as the v1 pty.get (SHIM-NARROWED-012) — the /api/pty/{ptyID} family routes to the SAME handlers (`handleApiPtyByID` dispatches to `ptyGet`/`ptyUpdate`/`ptyRemove`), so no `^pty` id names a terminal and the declared 200 Pty cannot be produced without asserting one that does not exist (ROUTE-ADD-031).
- evidence: `internal/shim/opencode/api_pty_by_id.go:63 handleApiPtyByID`; refusal `internal/shim/opencode/pty_routes.go:224 writeOpencodeNotFoundError`
- widening requires: the PTY registry + runner (the SHIM-NARROWED-012 engine).

#### SHIM-NARROWED-102 — v2.pty.update (`PUT /api/pty/{ptyID}`)
- declared: 200 Pty, 400, 401, 404
- served: 404 typed `PtyNotFoundError`; 400 for the same inputs plus a body violating the declared `{title?, size?{rows*,cols*}}` schema; 401 via the /api/* auth wall's v2 envelope
- rationale: no PTY exists whose title/size could be applied (ROUTE-ADD-032) — the v1 pty.update rationale (SHIM-NARROWED-013) applies verbatim through the shared dispatcher.
- evidence: `internal/shim/opencode/api_pty_by_id.go:65 handleApiPtyByID`; refusal `internal/shim/opencode/pty_routes.go:258 writeOpencodeNotFoundError`
- widening requires: the PTY registry + runner.

#### SHIM-NARROWED-103 — v2.pty.remove (`DELETE /api/pty/{ptyID}`)
- declared: 204 <No Content>, 400, 401, 404
- served: 404 typed `PtyNotFoundError` for every well-formed ptyID; 400 for an ill-shaped id or a blank/non-directory selector; 401 via the /api/* auth wall's v2 envelope
- rationale: the declared 204 would report the removal of a terminal that never existed (ROUTE-ADD-030) — the v1 pty.remove rationale (SHIM-NARROWED-014) applies verbatim through the shared dispatcher.
- evidence: `internal/shim/opencode/api_pty_by_id.go:67 handleApiPtyByID`; refusal `internal/shim/opencode/pty_routes.go:340 writeOpencodeNotFoundError`
- widening requires: the PTY registry + runner.

#### SHIM-NARROWED-106 — v2.pty.connectToken (`POST /api/pty/{ptyID}/connect-token`)
- declared: 200 `PtyTicketConnectToken`, 400, 401, 403, 404
- served: 404 typed `PtyNotFoundError` for every well-formed ptyID; 400 `BadRequestError` for an ill-shaped `^pty` id or malformed workspace selector; 403 `PtyForbiddenError` for a foreign workspace selector; 401 via the /api/* auth wall's v2 `UnauthorizedError` envelope
- rationale: the runtime keeps no PTY registry and mints no one-time connect ticket, so the declared 200 cannot be produced truthfully. ROUTE-ADD-034 registers the v2 path and delegates to the existing v1 handler, preserving the v1 validation order and typed error envelopes without fabricating a ticket.
- evidence: `internal/shim/opencode/api_pty_connect_token.go:34 handleApiPtyConnectToken`; delegated validation `internal/shim/opencode/pty_routes.go:393 handlePtyConnectToken`; 401 via `internal/shim/opencode/server.go:866 isV2EnvelopeRoutePath`
- widening requires: a real PTY registry and one-time ticket minting/lookup mechanism.

#### SHIM-NARROWED-105 — v2.pty.connect (`GET /api/pty/{ptyID}/connect`)
- declared: 200 boolean ("Connected session"), 400, 401, 403, 404
- served: 404 typed `NotFoundError` (the `NamedError` shape `{name, data:{message}}`) for every well-formed request; the declared 403 `{_tag:"Forbidden"}` for a blank or foreign directory/workspace selector; 401 via the /api/* auth wall's v2 `UnauthorizedError` envelope; the pre-change net/http 404 for an undeclared method (405 is in no declared set). The declared 400 arm is unreachable for the same reason the success is: the handler validates no declared input of its own, so an ill-shaped ptyID is answered the declared 404 — the v1 pty.connect behaviour (SHIM-NARROWED-015) reached through the delegation ROUTE-ADD-033 registered.
- rationale: the shim keeps no PTY registry and opens no PTY WebSocket (the v1 pty.connect truth, SHIM-NARROWED-015), so there is no terminal to attach to and the declared boolean 200 could only assert a connection that does not exist. ROUTE-ADD-033 registered the v2 route — which is why this operation left the comparator's NOT-SERVED class and is recorded here instead; its `connect-token` sibling stays NOT-SERVED until ROUTE-ADD-034 registers that shape.
- evidence: `internal/shim/opencode/api_pty_connect.go:12 handleApiPtyConnect`; refusal `internal/shim/opencode/api_pty_connect.go:22 writeOpencodeNamedError`; selector arm `internal/shim/opencode/pty_routes.go:372 ptyConnectForbidden`; 401 via the shape match `internal/shim/opencode/server.go:860 isV2EnvelopeRoutePath`
- widening requires: the PTY registry plus a WebSocket/attach surface (the SHIM-NARROWED-015 engine).

#### SHIM-NARROWED-107 — v2.session.message (`GET /api/session/{sessionID}/message/{messageID}`)
- declared: 200 `{data: SessionMessage}`, 400, 401, 404
- served: 404 typed `SessionNotFoundError` naming sessionID for an unknown session (resolved FIRST — the declared 404 anyOf is ambiguous between unknown session and unknown message); 404 typed `MessageNotFoundError` {_tag, sessionID, messageID, message} for every well-formed messageID, INCLUDING one the session holds; 400 `InvalidRequestError` for a blank sessionID or blank messageID segment (kind Path); 401 via the /api/* auth wall's v2 `UnauthorizedError` envelope; 405 for undeclared methods
- rationale: the declared 200 carries a `SessionMessage` — a discriminated union whose every member requires an id matching `^msg_` (SessionMessageUser requires time.created/text/type=user; SessionMessageAssistant requires agent/model/content[]; closed by additionalProperties:false). The runtime's message rows are memory_events rows keyed by the native autoincrement integer — the same identifiers the v1 message routes serve — and minting a `msg_` id for this one response would fabricate an identifier the sibling v2 message-list never emits and the v1 routes never resolve; unlike the sibling LIST operations (v2.session.history / v2.session.messages, which answer their declared 200 envelopes with runtime-native members) there is no runtime-native shape that can stand in inside the declared envelope either, because the declared `data` member IS the SessionMessage object itself. The declared 200 is therefore unreachable for every real row (the ROUTE-ADD-009 v2.event.subscribe precedent), and asserting a delivered SessionMessage for a held row would be the same fabrication as asserting a delivered pty. Widening requires the runtime to key messages by `^msg_` identifiers — a design change with its own row, not a contract detail.
- evidence: `internal/shim/opencode/api_session_message_get.go:84 handleApiSessionMessageGet`; session-first resolution `internal/shim/opencode/api_session_message_get.go:129` via `isNoRowsResult`; message resolution `sessionHasMessage` (`internal/shim/opencode/server.go:7776`); refusal `internal/shim/opencode/api_session_message_get.go:152 writeV2MessageNotFoundError`; 401 via the shape match `internal/shim/opencode/server.go isV2EnvelopeRoutePath` → `isApiSessionMessageGetPath`
- widening requires: ^msg_-keyed message identities (a message-id minting engine) so a GET can truthfully return the declared SessionMessage shape.

`counts` at this revision: `narrowed_error_contract_only: 22`, **`narrowed_unexplained: 0`**,
`intentional_narrowing: 2` (`pty.create` and its declared v2 sibling `v2.pty.create`,
SPEC-017 §3.9), `semantic_different: 2`
(the `/mcp` protocol delegation, SPEC-024 §A).

**On the "6 narrowed operations" figure.** A stale snapshot of the comparator's
counts is pasted into
`docs/evidence/opencode-upstream-v1.18.33/surface-diff-report.md` §10 (the
ROUTE-ADD-104 follow-up), where `narrowed_error_contract_only` reads `6`. That is
a pre-SHIM-GAP-004 count: the detection rule was loosened in SHIM-GAP-004 (it now
derives the error-contract class from the declared response set and the served
outcome instead of a hand list), which surfaced the remaining rows, and the
artifact has read `14` since. The artifact is the authority; that snapshot is
superseded and is annotated as such in place. A task brief that cites "6
remaining narrowed operations" is citing the superseded snapshot, not a live
count — re-derive with §12.4 before acting on it.

### 12.4 Reproducing and checking this section

```bash
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/opencode-declared-vs-served
# byte-identical (the script's own output order is the artifact's order):
diff /tmp/opencode-declared-vs-served/declared-vs-served.json \
     specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json   # no output = reproducible
# and the same comparison canonically, if either side has been re-formatted:
diff <(jq -S . /tmp/opencode-declared-vs-served/declared-vs-served.json) \
     <(jq -S . specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json)
```
Both sides of the second diff must be canonicalised: the committed artifact is
the script's own insertion-ordered output, so a one-sided `jq -S .` reports the
key ordering alone as a difference and the check can never pass. The first
(byte) diff is the strong one — it is what SHIM-GAP-005 verified at this
revision (empty output).
The comparator exits non-zero (naming the row) when a detected error-contract
narrowing has no rationale, and also when a row claims the marker while the
runtime answers the declared success — so this section cannot drift away from the
code silently.

The §12.3 ledger itself is checked from the Go side — census against the
artifact, every `file:line symbol` citation resolved, and every narrowed
operation driven live to prove it never answers its declared success:

```bash
go test ./internal/shim/opencode -run 'TestShimGap005' -count=1 -v
```
