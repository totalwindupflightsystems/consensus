---
work_item_id: interfaces-api-cli-01
status: not-started
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-03
last_alignment_scan: 2026-05-03
---

# Meta-Planning — API, MCP, CLI, and UI Adapter

Mission: expose Conscience through stable operator and machine interfaces once the schema and harness exist. This track owns REST endpoints, MCP transport, CLI commands, OpenAPI synchronization, opencode adapter behavior, and user flow proof.

axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md,specs/016-cli-interface.md,specs/017-ui-adapter-layer.md,specs/018-openapi-contract.md,specs/019-user-interaction-flows.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/interfaces-api-cli-01/verification.md prompt=.memory-bank/work-items/_prompt.md

## Acceptance Criteria

### Auth & Rate Limiting (SPEC-015 §2, §7)

1. **AC-AUTH-01**: REST API authenticates via `Authorization: Bearer <key>` header against the `api_keys` table.
2. **AC-AUTH-02**: Four key scopes enforced: `admin` (full CRUD), `session` (single-session scope), `readonly` (SELECT only), `webhook` (external_events INSERT only).
3. **AC-AUTH-03**: Only SHA-256 hash of the API key stored in `key_hash`; raw key never persisted.
4. **AC-AUTH-04**: Key prefix (`cs_*`) with first 8 chars stored in `key_prefix` for indexed lookup.
5. **AC-AUTH-05**: Session-scoped keys restrict access to that session only (RLS via `SET LOCAL conscience.session_id`).
6. **AC-AUTH-06**: Expired keys (past `expires_at`) rejected with 401 UNAUTHENTICATED.
7. **AC-AUTH-07**: `POST /api/v1/auth/keys` (create), `GET /api/v1/auth/keys` (list prefix+scope), `DELETE /api/v1/auth/keys/:id` (revoke) all operational.
8. **AC-AUTH-08**: Rate limiting enforced per key type: admin 1000/min, session 100/min, readonly 200/min, webhook 500/min.
9. **AC-AUTH-09**: Rate limit middleware runs before database queries; sliding 60-second window.
10. **AC-AUTH-10**: Exceeded limit returns 429 RATE_LIMITED with standard error envelope.

### REST Endpoint Families (SPEC-015 §3)

11. **AC-REST-01**: Sessions: `POST /api/v1/sessions` (create, returns api_key), `GET` (list with filters), `GET /:id` (detail), `PATCH /:id` (pause/resume/cancel), `DELETE /:id` (soft-delete).
12. **AC-REST-02**: Messages: `POST /api/v1/sessions/:id/message` with `type`: `user_instruction` | `user_correction` | `system_override`.
13. **AC-REST-03**: Memory & Context: `GET /:id/memory` (list), `GET /:id/context` (active context view), `GET /:id/iterations` (iteration commits), `GET /:id/memory/:mid` (single event).
14. **AC-REST-04**: Tasks: `POST /:id/tasks` (create), `GET` (list), `PATCH /api/v1/tasks/:tid` (update status), `POST /api/v1/tasks/:tid/claim` (claim).
15. **AC-REST-05**: Tools & Skills: `GET /api/v1/tools` (list), `POST /api/v1/tools/:name/execute` (execute internal tool), `GET /api/v1/skills` (list), `GET /api/v1/skills/:name` (full instructions).
16. **AC-REST-06**: Approvals: `GET /api/v1/approvals` (list pending), `GET /api/v1/approvals/:id` (detail), `POST /api/v1/approvals/:id/review` (approve/reject/modify), `GET /api/v1/sessions/:id/approvals` (session-scoped).
17. **AC-REST-07**: Billing & Metrics: `GET /api/v1/sessions/:id/billing`, `GET /api/v1/metrics`, `GET /api/v1/sessions/:id/iterations/:iid/audit`.
18. **AC-REST-08**: Configuration: `GET /api/v1/config` (redacted), `PATCH /api/v1/config` (update), `GET /api/v1/config/models` (model registry).
19. **AC-REST-09**: Health: `GET /api/v1/health` (no auth required, returns 200).
20. **AC-REST-10**: Error response format: `{ "error": { "code": "...", "message": "...", "details": {} } }` for all endpoints.
21. **AC-REST-11**: HTTP error codes mapped: 400 INVALID_REQUEST, 401 UNAUTHENTICATED, 403 FORBIDDEN, 404 NOT_FOUND, 409 CONFLICT, 429 RATE_LIMITED, 500 INTERNAL_ERROR.

### MCP Server (SPEC-015 §5)

22. **AC-MCP-01**: MCP server exposes 6 tools: `create_session`, `send_message`, `get_session_status`, `list_memory`, `review_approval`, `query_tool`.
23. **AC-MCP-02**: MCP server exposes 3 resources: `conscience://sessions`, `conscience://sessions/{id}/context`, `conscience://tools`.
24. **AC-MCP-03**: MCP server exposes 1 prompt: `agent_status`.
25. **AC-MCP-04**: MCP auth via Bearer token in initialize handshake `_meta.authorization`; same `api_keys` table and scope enforcement.
26. **AC-MCP-05**: MCP SSE transport at `/mcp/sse`.
27. **AC-MCP-06**: MCP stdio transport for local PocketBase (auth optional, equivalent to peer auth).
28. **AC-MCP-07**: MCP parameter schemas validated against tool definitions.

### SSE & Real-time Streams (SPEC-015 §4)

29. **AC-SSE-01**: SSE endpoint serves `Content-Type: text/event-stream` with keep-alive.
30. **AC-SSE-02**: Session-scoped subscription via `?session_id=` query parameter.
31. **AC-SSE-03**: Postgres backend: LISTEN/NOTIFY → SSE; SQLite backend: Go goroutines + channels → SSE (shared code surface).
32. **AC-SSE-04**: Event types: session status changes, memory event created, tool execution start/end, approval requested.

### CLI Surface (SPEC-016)

33. **AC-CLI-01**: Global flags: `--server` (CONSCIENCE_SERVER), `--api-key` (CONSCIENCE_API_KEY), `--format` (table|json|yaml), `--quiet`, `--config`.
34. **AC-CLI-02**: Output formats: table (default), json, yaml; --quiet suppresses non-essential output.
35. **AC-CLI-03**: Exit codes: 0=success, 1=general error, 2=invalid args, 3=server unreachable, 4=auth failed, 5=not found, 6=conflict, 7=rate limited.
36. **AC-CLI-04**: Command groups operational: `serve`, `init`, `session` (create/list/show/logs/pause/resume/cancel/cost), `approve` (list/show/approve/reject/interactive), `migrate` (run/version/rollback/create), `config` (list/get/set/edit), `status`, `memory` (list/show/iterations/pages), `tool` (list/show), `skill` (list/show).
37. **AC-CLI-05**: Config file resolution: `./conscience.yaml` > `~/.conscience/config.yaml` > `/etc/conscience/config.yaml`.
38. **AC-CLI-06**: Shell completion scripts generated for bash, zsh, fish.
39. **AC-CLI-07**: All CLI commands are thin REST clients; no direct database access.
40. **AC-CLI-08**: Interactive approval mode walks operator through pending approvals.

### OpenAPI Contract (SPEC-018)

41. **AC-OPENAPI-01**: Split spec files: root `openapi.yaml` references paths/ (sessions, messages, memory, tasks, tools, skills, approvals, metrics, shim-openai, shim-anthropic) and components/ (schemas, responses, parameters, security).
42. **AC-OPENAPI-02**: Bundle step produces single `bundled.yaml` via redocly CLI.
43. **AC-OPENAPI-03**: Lint validation passes (`redocly lint bundled.yaml`).
44. **AC-OPENAPI-04**: Contract tests per endpoint: request validation, response shape, status codes, auth rejection.
45. **AC-OPENAPI-05**: Schemathesis property-based testing against `bundled.yaml`.
46. **AC-OPENAPI-06**: Server serves `/doc` (Swagger UI), `/openapi.yaml`, `/openapi.json` at runtime.
47. **AC-OPENAPI-07**: CI workflow: bundle + lint + no-breaking-changes check (oasdiff).
48. **AC-OPENAPI-08**: Type generation: openapi-typescript → TypeScript types, oapi-codegen → Go client.
49. **AC-OPENAPI-09**: Versioning: `info.version` (semver, bumped on API shape change) separate from `engine_version`.

### opencode Shim (SPEC-017)

50. **AC-SHIM-01**: Global endpoints: `GET /global/health` (static), `GET /global/event` (real-time SSE).
51. **AC-SHIM-02**: Session CRUD: `GET/POST /session`, `GET/PATCH/DELETE /session/:id`, `GET /session/:id/children`, `POST /session/:id/abort`.
52. **AC-SHIM-03**: Message translation: opencode `parts` format → native `POST /api/v1/sessions/:id/message`; native response → opencode message with info + parts.
53. **AC-SHIM-04**: Event stream mapping: session_status_change → session.updated, memory_event_created → message.created, tool_execution → tool.started/completed, approval_requested → permission.requested/resolved.
54. **AC-SHIM-05**: File endpoints: `/find`, `/find/file`, `/find/symbol`, `/file/content`, `/file/status` → native tool execution API.
55. **AC-SHIM-06**: Config/Provider/Agent endpoints mapped to native API: `/config` → `GET/PATCH /api/v1/config`, `/provider` → config providers, `/agent` → agent types, `/auth/:id` → auth management.
56. **AC-SHIM-07**: Auth translation: HTTP Basic Auth (`opencode:<password>`) → password validated as admin API key → native API calls with resolved key.
57. **AC-SHIM-08**: HITL translation: approval_requested → opencode permission.requested event; permission response → native `POST /api/v1/approvals/:id/review`.
58. **AC-SHIM-09**: Shim session mapping table (`shim_session_map`) for opencode external_id → Conscience session_id.
59. **AC-SHIM-10**: Excluded endpoints return 501: `prompt_async`, `shell`, `command`, `share`, `summarize`, `init`, `fork`, `revert`, `project`, `vcs`.
60. **AC-SHIM-11**: Connection flow: `conscience serve --adapter opencode` → `opencode attach http://localhost:8090` → TUI connects, authenticates, creates/restores session.
61. **AC-SHIM-12**: `/doc` serves OpenAPI 3.1 spec at shim root; `/mcp` endpoints for MCP config/status.

### User Flow Proof (SPEC-019)

62. **AC-FLOW-01**: Developer first connection: install → init → serve → `opencode attach` → first message works end-to-end.
63. **AC-FLOW-02**: Developer ongoing work: multi-session persistence (Day 1 analysis remembered on Day 2); session pause/resume without data loss.
64. **AC-FLOW-03**: Developer multi-tool: same session accessible via opencode TUI, Claude Code MCP, and CLI; shared memory.
65. **AC-FLOW-04**: Operator deployment flow: init (Supabase or PocketBase) → serve → status → session management.
66. **AC-FLOW-05**: Operator HITL flow: approval notification → review context → approve/reject/modify/cancel → agent resumes.
67. **AC-FLOW-06**: Local onboarding (PocketBase): under 5 minutes from install to first agent interaction.
68. **AC-FLOW-07**: Team onboarding (Supabase): schema install → config → serve → API key distribution per developer.
69. **AC-FLOW-08**: MCP-only onboarding: `claude mcp add` or opencode MCP config → tool access without full TUI replacement.
70. **AC-FLOW-09**: Error recovery — agent stuck: consecutive errors → paused → operator diagnosis via `session logs` → cancel or fix.
71. **AC-FLOW-10**: Error recovery — budget exceeded: auto-pause → `conscience session resume` or config set `budget_limit_cents`.
72. **AC-FLOW-11**: Error recovery — server unreachable: clear error message with guidance to start `conscience serve`.
73. **AC-FLOW-12**: Error recovery — schema migration: `conscience status` shows outdated → `conscience migrate` → sessions resume.

## Scope Fences

**In scope:**
- REST API endpoints for all families in SPEC-015 §3
- Auth middleware (key validation, scope enforcement, key prefix indexing)
- Rate limiting middleware (per-key-type sliding window)
- SSE real-time event stream
- MCP server (tools, resources, prompts, SSE/stdio transports)
- CLI commands (all groups from SPEC-016 §5)
- OpenAPI split-spec repository, bundle step, lint, contract tests, runtime serving
- opencode protocol shim (all endpoints from SPEC-017 §3, including exclusions)
- User flow verification covering all 6 SPEC-019 workflows

**Out of scope / future:**
- Web admin UI (deferred per SPEC-016 §12)
- TUI dashboard (Bubble Tea, deferred)
- pi-agent shim (research pending per SPEC-017 §5.1)
- gRPC/protobuf (no requirement yet per SPEC-018 §11)

## Evidence Strategy
- Contract tests (per SPEC-018 §6) for every REST endpoint
- Schemathesis automated property-based testing
- CLI integration test suite (session lifecycle, approvals, config round-trip)
- MCP compliance test (tool call → response, resource read, auth)
- opencode shim integration test (attach → create session → message → response → event translation)
- User-flow end-to-end walkthrough logs (first connection, multi-tool, HITL, error recovery)
- OpenAPI CI: bundle + lint + no-breaking-changes automated in GitHub Actions

## Testing Strategy
- Unit tests: auth middleware, rate limiter, message format translation, shim event mapping
- Integration tests: REST endpoint families against running server
- Contract tests: OpenAPI request/response validation per endpoint
- Property-based: Schemathesis against bundled.yaml
- CLI tests: output format verification, exit code verification, error conditions
- MCP tests: tool invocation, resource read, prompt execution
- shim tests: end-to-end opencode protocol translation

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| opencode server API version drift | High | Pin to specific opencode version; add version-check in shim |
| Rate limiting performance impact | Medium | In-memory counter with periodic DB sync; measure latency overhead |
| MCP protocol changes | Medium | Target MCP 2024-11-05; add protocol version negotiation |
| File system access mismatch (shim) | High | Document clearly in shim; tools read local filesystem via Conscience tool system |
| PostgREST OpenAPI overlap | Low | Conscience spec is authoritative superset; document relationship |
| CLI binary size (Go embedded) | Low | Acceptable for Go binary; measure and budget |
