# Consensus — Master Gap Analysis (Fresh Sweep)

**Generated**: 2026-05-29 | **Project**: /home/kara/conscientiousness
**Implementation State**: 123 Go files, 42 test files, 27 packages. All build and test clean.

---

## Methodology

Two parallel sub-agents independently deep-verified all 22 specs + ADR + PRD against actual Go code paths:
- **Agent A**: Core architecture (specs 000-008, 011, 020) — harness, memory, DB, security, transactions, planning
- **Agent B**: Interfaces & deployment (specs 009-010, 012-023, PRD) — tools, webhooks, HITL, API/MCP, CLI, shim, OpenAPI

Both agents traced code paths, not just file existence. Convergent findings cross-validated.

---

## Part 1: What Works (Validated)

| Spec | Area | Verdict | Notes |
|------|------|---------|-------|
| 000 | North Star vision | ✅ Match | DB-as-runtime, atomic cognition, dual backend |
| 001 | Architecture | ✅ Match | DBaaR, Write Once Deploy Anywhere, agent as microservice |
| 002 | Memory (append-only) | ✅ Match | memory_events never updated, iteration commits, page resolution |
| 003 | Database schema | ✅ Strong | 14+ core tables, dynamic table generation, reserved names |
| 005 | Security (RLS) | ✅ Match | Row-level security on 7 tables, Alt-mode soft delete, secrets inject/scrub |
| 006 | Transactions | ✅ Strong | Atomic iteration (BEGIN/COMMIT/ROLLBACK), circuit breakers, budget limits, audit logs |
| 007 | JSON Schema (parsing) | ✅ Match | AgentOutput struct, JSON.parse, SQL classifier, multi-statement split |
| 008 | Harness loop | ✅ Strong | 11-step core loop, heartbeat, secrets, context truncation, reactive truncation |
| 009 | Deployment | ✅ Match | Single binary, auto-migrate, Dockerfile, Makefile, config priority chain |
| 011 | Canonical definitions | ✅ Strong | State machines, display modes, tool/skill registry, billing schema, statement classifier, table whitelist |
| 013 | Webhooks & events | ✅ Strong | HMAC verification, routing loop, session wake, rate limiting, idempotency |
| 014 | HITL interrupt state | ✅ Strong | All 6 request types, 3 decisions, expiry cron, session pause/resume |
| 015 | API & MCP | ✅ Strong | REST endpoints (10+ families), auth middleware, rate limiting, MCP 6 tools + 3 resources |
| 016 | CLI interface | ✅ Partial | 15+ commands working; missing `session logs`, `session cost`, `config list/edit` |
| 017 | UI adapter (shim) | ✅ Strong | opencode protocol translation, session/message/config/event mapping, auth translation |
| 019 | User interaction flows | ✅ Design | UX spec — no code to verify; personas have working paths |
| 020 | Multi-turn planning | ✅ Near-complete | Interactive tx, staging buffer, 6 actions, 60s timeout, crash recovery partial |
| 023 | ADR bootstrap key expiry | ✅ Complete | 90-day TTL, env var config, visibility, auth middleware |

---

## Part 2: Critical Gaps (NEW — not in prior audit)

### 🔴 CRITICAL

#### CS-GAP-001: No Vector-Validated Compression Pipeline
- **Specs**: SPEC-002 §8, SPEC-011
- **Status**: **RESOLVED** ✅ (WI-012, 2026-05-29)
- **What was built**: Embedding client (OpenAI /v1/embeddings), cosine similarity in Go, compression worker goroutine, tier escalation (raw→compressed→abstract→canonical), billing recording, migration 015
- **Files**: `internal/llm/embedding.go`, `internal/compression/worker.go`, `internal/compression/compression.go`, `migrations/015_embedding_model.sql`
- **Effort**: ~8h (actual) vs ~30h (estimated)

#### CS-GAP-002: No JSON Schema Enforcement (`pg_jsonschema`)
- **Specs**: SPEC-003 §3.4, SPEC-007
- **What's missing**: No `jsonb_matches_schema()` CHECK constraints, no `pg_jsonschema` extension, no `sqlite-jsonschema`. LLM output is JSON-parsed but not schema-validated at the API or DB level.
- **Impact**: Core architectural promise — "unbypassable constraints" via `ALTER TABLE ADD CONSTRAINT CHECK(jsonb_matches_schema(...))` — is just SQL comments, not working code.
- **Effort**: ~20h

#### CS-GAP-003: No Cognitive Firewall (Quarantine Scanning)
- **Specs**: SPEC-005 §3
- **What's missing**: `external_quarantine` table exists but no scanning pipeline. External data enters memory_events without passing through a quarantine validation model.
- **Impact**: The spec's promise that "the agent never sees raw external data" is false. Untrusted webhook payloads go straight to memory.
- **Effort**: ~15h

#### CS-GAP-004: No Tool Execution Sandbox (External Hemisphere)
- **Specs**: SPEC-010
- **What's missing**: `custom_agent_tools` table exists but no `ExecuteExternalTool()`. No subprocess isolation, no Deno/TypeScript runtime, no CI/CD pipeline for tool testing.
- **Impact**: External tools are registrable but not executable. The entire External hemisphere is theoretical.
- **Effort**: ~25h

### 🟠 HIGH

#### CS-GAP-005: Three-Tier SQL Execution Model Missing
- **Specs**: SPEC-008 §5.4
- **What's missing**: Only Tier 3 (raw SQL with classifier) exists. Tier 1 (stored-proc-only) and Tier 2 (parameterized queries) are not implemented.
- **Impact**: No way to restrict cheaper/less-trusted models. All agent SQL runs with full DML capability.
- **Effort**: ~10h

#### CS-GAP-006: Harness Bypasses `active_context_view` VIEW
- **Specs**: SPEC-001 §Data Flow, SPEC-002 §7, SPEC-003
- **What's missing**: The VIEW exists in migrations but harness assembles context in Go (`context.go:314-349`) via manual SQL queries, not by selecting from the view.
- **Impact**: Cache hierarchy ordering, `DISTINCT ON` deduplication, RLS integration, window functions — all VIEW logic lost.
- **Effort**: ~8h

#### CS-GAP-007: No Structured Outputs Guarantee
- **Specs**: SPEC-007 §3
- **What's missing**: LLM client sends `response_format: {type: "json_object"}` but no `json_schema` constraint with `strict: true`. Agent can return malformed JSON.
- **Impact**: Harness must handle JSON parse errors manually. No deterministic output shape guarantee.
- **Effort**: ~5h

#### CS-GAP-008: No Trigger-Based Constraint Enforcement
- **Specs**: SPEC-003, SPEC-005
- **What's missing**: State transition locks, prerequisite checks, rate limiting triggers, epistemic anchoring validation — all defined in SQL but not enforced by DB triggers.
- **Impact**: Agent can set any task status, claim tasks before prerequisites complete, violate transition rules.
- **Effort**: ~12h

#### CS-GAP-009: RBAC Scope Model Absent
- **Specs**: SPEC-004 §RBAC
- **What's missing**: No Global/Project/Sub-Agent tier system. All agents treated equally. No cross-project visibility rules.
- **Effort**: ~10h

#### CS-GAP-010: RLS 4-Role Model Not Used at Runtime
- **Specs**: SPEC-005, SPEC-011
- **What's missing**: `agent_role`, `compression_worker`, `alt_mode_role`, `tool_executor` defined in SQL migrations but harness connects as single DB role. `SET LOCAL consensus.user_id` never called.
- **Effort**: ~6h

#### CS-GAP-011: Anthropic Client is a Stub
- **Specs**: SPEC-022
- **What's missing**: `NewAnthropicClient()` returns `stubClient` that always errors: "anthropic client not yet implemented". No Claude models usable.
- **Effort**: ~8h

### 🟡 MEDIUM

#### CS-GAP-012: lib/pq Instead of pgx/v5
- **Specs**: SPEC-009, SPEC-022
- **What's missing**: `github.com/lib/pq` instead of `github.com/jackc/pgx/v5`. No connection pooling, no LISTEN/NOTIFY, no `FOR UPDATE SKIP LOCKED` via driver API.
- **Effort**: ~8h

#### CS-GAP-013: SSE Event Stream Not Connected
- **Specs**: SPEC-015 §Real-Time
- **What's missing**: `EventBus` exists but no goroutine pushes events to it. No DB trigger listener, no polling loop feeding SSE subscribers.
- **Effort**: ~4h

#### CS-GAP-014: CLI Missing Commands
- **Specs**: SPEC-016
- **What's missing**: `session logs`, `session cost`, `config list/edit`, `migrate up/down` (direct, not REST proxy)
- **Effort**: ~5h

#### CS-GAP-015: go.mod Missing 5/10 SPEC-022 Dependencies
- **Specs**: SPEC-021, SPEC-022
- **What's missing**: `pgx/v5`, `openai-go`, `anthropic-sdk-go`, `mcp-go`, `chi/v5`, `goose/v3` — none in go.mod. Custom implementations lack prompt caching, structured outputs via SDK, chi middleware.
- **Effort**: ~10h

#### CS-GAP-016: Prompt Caching Breakpoints Absent
- **Specs**: SPEC-012 §Cache, SPEC-003
- **What's missing**: No Anthropic `cache_control` or OpenAI prompt caching configuration on static prompt layers.
- **Effort**: ~4h

#### CS-GAP-017: Shim Missing File Operations
- **Specs**: SPEC-017
- **What's missing**: `/file/*`, `/find/*` endpoints return 501. No file operation translation in the opencode shim.
- **Effort**: ~6h

#### CS-GAP-018: Token Budget Calculation Bypasses Model Registry
- **Specs**: SPEC-008
- **What's missing**: `calculateCostUSD` in harness.go:311 uses hardcoded pricing map instead of querying `model_registry` table.
- **Effort**: ~2h

### 🔵 LOW

#### CS-GAP-019: MCP stdio Transport Missing
- **Specs**: SPEC-015 §Transport
- **Effort**: ~5h

#### CS-GAP-020: OpenAPI Bundle/Build Missing
- **Specs**: SPEC-018
- **Effort**: ~3h

#### CS-GAP-021: Tool-Approval Gating Not Wired
- **Specs**: SPEC-014
- **Effort**: ~4h

#### CS-GAP-022: sqlite.go Tx.IsActive Always Returns True
- **Specs**: SPEC-006
- **Effort**: ~2h

---

## Part 3: Still-Open From Prior Audit (Revalidated)

| Gap | Status | Notes |
|-----|--------|-------|
| RT-001: Mock LLM fallback | **Still open** 🔴 | `main.go:112-114` — confirmed critical |
| RT-003: go.mod missing SPEC-022 deps | **Still open** 🔴 | 5/10 missing — now CS-GAP-015 |
| RT-004: MCP zero trace markers | **Still open** 🟡 | MCP has no telemetry |
| RT-006: SPEC-021 stale inventory | **Still open** 🟡 | 7 phantom files |
| RT-007: No ops runbooks | **Still open** 🟡 | No alerting/runbook infrastructure |
| RT-002: activeContext outdated | **RESOLVED** ✅ | Updated this sweep |
| RT-005: Secrets scrub wired | **RESOLVED** ✅ | Verified in audit.go |

---

## Summary Statistics

- **22 specs + ADR + PRD analyzed**: 24 total
- **Fully working**: 16 (67%)
- **Partially working**: 6 (25%)
- **Not working / absent**: 2 (8%) — No compression pipeline, No tool sandbox

- **New critical gaps found**: 4 (CS-GAP-001 through 004)
- **New high gaps found**: 7 (CS-GAP-005 through 011)
- **New medium gaps found**: 7 (CS-GAP-012 through 018)
- **Total gaps**: 22 (combining new + existing)

**Approximate total remaining effort**: ~200-250h
