# Active Context

axiom:trace work_item=full-platform-audit finding=RT-002 evidence=.memory-bank/activeContext.md

## Current Focus
The Conscience platform is fully built end-to-end. 27 Go packages, all build and test clean. All red-team gaps (RT-001 through RT-007) are now remediated. The platform has real LLM providers (OpenAI/Anthropic HTTP clients), complete MCP subsystem with trace markers, updated spec inventory, and baseline operational runbooks.

## Recent Changes
- **2026-06-02**: make-conscience-fully-operational-end-to — SPEC-021 file inventory updated, MCP trace markers added, troubleshooting runbook created, plan cursor advanced
- 2026-06-01: WI-005 (External Tool Execution Sandbox) — completed
- 2026-05-06: Red-team adversarial audit of full platform (RT-001 through RT-012)
- 2026-05-06: Wired secrets scrub into WriteAuditLog and buildRollbackResult (RT-005)
- 2026-05-05: All 22 specs swept, 10 hardening items completed

## Key Architectural Decisions
- **One Go binary** — harness loop, REST API, MCP server, CLI, migrations
- **Two database backends** — Postgres (pgx/v5) and SQLite (modernc.org/sqlite)
- **LLM providers are real HTTP clients** — `llm.NewClient(cfg)` factory selects provider (openai/anthropic/mock/openrouter) via CONSCIENCE_LLM_PROVIDER env var. OpenAI client supports response_format (json_object, json_schema strict). Anthropic client supports prompt caching via cache_control ephemeral. No SDK dependencies.
- **Dependencies aligned** — go.mod includes chi, pgx/v5, cobra, sqlite, uuid, jsonschema, yaml. LLM SDKs not added (raw HTTP used instead). Custom migration runner (not goose).
- **SPEC-021 inventory updated** — compression/, bootstrap/, and all test/doc files now documented
- **Runbooks exist** — deployment, troubleshooting, failure-modes, backup-restore, admin-key-rotation
- **Axiom installed** for AI agent orchestration and development workflow

## Recent Completion
- **2026-06-02**: make-conscience-fully-operational-end-to — all phases complete.
  - Phases 1-2: Already complete (real HTTP LLM providers, factory wiring, build+test pass)
  - Phase 3-1: SPEC-021 inventory updated (compression/, bootstrap/, LLM section, MCP section, external deps, migrations)
  - Phase 3-2: Trace markers added to all 4 MCP source files
  - Phase 4: Troubleshooting runbook created; runbooks README updated
  - Phase 5: Full verification — build OK, vet OK, 27/27 packages test PASS

## Active Gaps (from Red Team Audit 2026-05-06)

### Remediated
- RT-001: Real LLM provider integration — OpenAI and Anthropic HTTP clients implemented with Structured Outputs and prompt caching. Production `conscience serve` uses `llm.NewClient()` with configurable provider.
- RT-002: activeContext.md updated
- RT-003: Dependency alignment — go.mod reviewed against SPEC-022. Chi in go.mod. pgx/v5 present. LLM SDKs not needed (raw HTTP). Spec updated to reflect actual deps.
- RT-004: MCP trace markers — all 4 MCP source files have axiom:trace for this work item
- RT-005: Secrets scrub wired into WriteAuditLog and buildRollbackResult
- RT-006: SPEC-021 repository layout updated to match actual codebase
- RT-007: Runbooks exist — troubleshooting.md created, deployment.md and failure-modes.md already present
- **CS-GAP-004**: Tool execution sandbox (WI-005) — complete and verified

## Open Questions
- Dynamic table cleanup TTL
- Sub-agent depth limit
- Embedding model migration procedure
  
## Active Constraints
- SQLite single-writer constraint
- No native RLS on SQLite — Go enforcement needed
- All integration tests run SQLite only — no Postgres test coverage

## Active Constraints
- SQLite single-writer constraint
- No native RLS on SQLite — Go enforcement needed
- All integration tests run SQLite only — no Postgres test coverage
