---
work_item_id: runtime-harness-01
status: not-started
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-03
amended_by: spec-alignment-scan-01 — expanded ACs to cover all cited spec detail (SPEC-006, SPEC-008, SPEC-010, SPEC-012, SPEC-020)
---

# Meta-Planning — Runtime Harness

**Mission:** implement the Consensus cognition runtime after the schema and package skeleton exist. This work item owns the harness iteration loop, dynamic system prompt assembly, tool execution boundaries, and multi-turn interactive transaction planning behavior. The first slice must prove one minimal iteration can read context, accept structured output, commit database changes, and enqueue tool work.

axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md,specs/008-harness.md,specs/010-tools.md,specs/012-system-prompt-and-discovery.md,specs/020-multi-turn-planning.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/runtime-harness-01/verification.md prompt=.memory-bank/work-items/_prompt.md

## In Scope

- Harness loop: read active context, format for LLM, call LLM with structured JSON schema, parse response, execute SQL in transaction, commit or rollback per SPEC-006 / SPEC-008.
- Dynamic system prompt assembly from session, schema (core + dynamic tables), tools registry, skills registry, and constraints per SPEC-012.
- Three-tier SQL execution: stored-procedure-only, parameterized, raw-with-classifier per SPEC-008 §SQL Execution Model.
- Statement classification and execution policy enforcement (classifier, sanitization, table whitelist, DANGEROUS/DML_WRITE blocking) per SPEC-006, SPEC-008.
- Interactive multi-turn transaction staging: `stage_and_execute` / `stage_only` / `tool_call` / `commit` / `rollback` / `respond` actions per SPEC-020 §3.
- Staging buffer persistence and lifecycle: `staged` → `executed` → `committed` | `rolled_back` | `failed` per SPEC-020 §4.
- Turn-level context formatting showing transaction state per SPEC-020 §6.
- Transaction timeout (60s default) and crash recovery via pg_cron reaping + orphan cleanup per SPEC-020 §14.
- Rollback-retry with max cap (3 retries) per SPEC-020 §14.3.
- Tool (external) execution: registry lookup, sandboxed subprocess execution, `tool_results` writes, secret scrubbing per SPEC-010, SPEC-008.
- Heartbeat loop polling for ready tasks, `ClaimNextReadyTask` with FOR UPDATE SKIP LOCKED per SPEC-008 §Heartbeat.
- Secrets injection (`{{SECRET.X}}` → real value) before SQL execution and scrubbing from LLM responses before storage per SPEC-008 §Secrets.
- Audit logging per iteration per SPEC-006 §Audit Logs.
- Iteration snapshot save per SPEC-006 §Iteration Snapshot.

## Out of Scope (this work item)

- REST API, MCP server surface (→ `interfaces-api-cli-01`).
- CLI management commands (→ `interfaces-api-cli-01`).
- Deployment topology, Docker, Kubernetes (→ `deployment-ops-01`).
- Model cascade / model routing implementation (uses SPEC-003 `model_registry` but full cascade deferred).
- Agent billing / budget circuit breakers (deferred to later slice).
- Sub-agent spawn lifecycle beyond tool request analogy (covered in `subagent-orchestration-01`).
- pg_cron setup (assumed available; scheduling SQL is in spec).

## Acceptance Criteria

### Harness Core Loop (SPEC-006, SPEC-008)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-001 | Harness loop reads `active_context_view`, formats context as Markdown, sends to LLM with structured JSON schema, parses the response, and executes SQL in a transaction (BEGIN → SQL → COMMIT or ROLLBACK). | SPEC-008 §Core Loop, SPEC-006 §2-3 |
| AC-002 | SQL execution applies classifier (ClassifyStatement), policy enforcement (EnforceExecutionPolicy), sanitization (Sanitize), and table whitelist (IsTableAllowed). DANGEROUS statements are blocked; DML_WRITE to unauthorized tables is blocked. | SPEC-006 §2.4, SPEC-008 §SQL Execution Model |
| AC-003 | Multi-statement LLM output is split on semicolons, each statement independently classified, sanitized, and policy-checked. | SPEC-008 §Multi-Statement Splitting |
| AC-004 | Secrets are injected before SQL execution (`{{SECRET.X}}` replacement) and scrubbed from LLM responses before audit/storage. | SPEC-008 §Secrets Injection & Scrubbing |
| AC-005 | `memory_events` UPDATE/DELETE is revoked from agent_role. Only `compression_worker` role may update `summary_text`. | SPEC-006 §2.4, SPEC-002 §2.4 |
| AC-006 | Audit log rows are written per iteration with monologue, SQL executed, result (committed/rolled_back), and error_message. | SPEC-006 §Audit Logs |
| AC-007 | Iteration snapshot (llm_response + sql_executed) is saved to `iteration_commits` on success. | SPEC-006 §Iteration Snapshot |

### System Prompt & Schema Discovery (SPEC-012)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-008 | Dynamic system prompt is assembled per-iteration from: session row (agent_name, goal, budget, iteration), core tables list, dynamic tables list, tools_registry (enabled, filtered by ownership), skills_registry (enabled, metadata only), and database constraints. | SPEC-012 §2-3, §7 |
| AC-009 | Schema discovery queries `information_schema.tables` for core tables (fixed whitelist per SPEC-012 §2.2) and dynamic tables (all public tables NOT in the core whitelist, scoped to session). | SPEC-012 §2.2, §4.3 |
| AC-010 | JSON Schema for LLM output format is enforced at the API level (Structured Outputs). The prompt describes the JSON structure; API-level schema gates correctness. | SPEC-012 §4.4 |
| AC-011 | Prompt caching is applied with breakpoints: Layer 1 (identity+rules, always cached), Layer 2 (schema+tools, cached after first call), Layer 3 (dynamic context, rarely cached). | SPEC-012 §5 |
| AC-012 | Skills progressive disclosure: agent sees only `name` + `metadata` in system prompt; `load_skill(name)` returns full instructions on demand. | SPEC-012 §4.2, SPEC-010 §Progressive Disclosure |
| AC-013 | Sub-agent prompt assembly filters tools to internal-hemisphere + approved external only; sub-agent inherits core schema knowledge but not parent's memory_events or tool approvals. | SPEC-012 §6 |

### Interactive Multi-Turn Planning (SPEC-020)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-014 | Harness supports interactive transaction: BEGIN stays open across N turns; each turn the agent issues an action (`stage_and_execute`, `stage_only`, `tool_call`, `commit`, `rollback`, `respond`). | SPEC-020 §3, §5 |
| AC-015 | Staging buffer table persists each staged command with lifecycle states: `staged` → `executed` → `committed` | `rolled_back` | `failed`. Buffer is scoped to a single iteration (session_id + iteration). | SPEC-020 §4 |
| AC-016 | On `stage_and_execute`: staged commands execute inside the open transaction and results are written back to the staging buffer so the agent sees them next turn. | SPEC-020 §5, §7 |
| AC-017 | On `stage_only`: commands are written to staging buffer but not executed; agent continues planning. | SPEC-020 §3 action table |
| AC-018 | On `commit`: final `memory_state_changes` are applied; all staging buffer entries → `committed`; session status → `idle`; iteration incremented; transaction COMMIT. | SPEC-020 §5 commit handler |
| AC-019 | On `rollback`: transaction ROLLBACK; staging buffer entries → `rolled_back`; agent may re-open transaction for retry or `end_iteration` to stop. | SPEC-020 §8 |
| AC-020 | Rollback-retry is capped at `maxRollbackRetries` (default 3). After exceeding the cap, the iteration ends with an error. | SPEC-020 §14.3 |
| AC-021 | Turn context formatting includes the full transaction state: what commands were staged, what executed, what results were returned, what remains pending. | SPEC-020 §6 |
| AC-022 | Transaction timeout (default 60s) enforced: if timeout fires, transaction is rolled back, staging buffer → `failed`, session → `failed`. | SPEC-020 §14.1 |
| AC-023 | Crash recovery: uncommitted transaction is rolled back by DB (MVCC/WAL); session in `planning`/`tool_exec` with stale `heartbeat_at` (5+ min) is marked `failed` by pg_cron; orphaned staging buffer entries are cleaned up to `failed`. | SPEC-020 §14.2 |
| AC-024 | Max turns (default 10) enforced: if reached with executed commands, auto-commit; if reached with no executed work, rollback with error. | SPEC-020 §5 end-of-loop, §11 |
| AC-025 | `tool_call_ref` entries in staging buffer are references only — actual tool execution happens outside the transaction (consistent with SPEC-011 §7), with session status → `tool_exec` during execution and → `planning` on resume. | SPEC-020 §9.2 |

### Tool Execution Boundary (SPEC-010, SPEC-008)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-026 | Tool requests are resolved against `tools_registry` by name; handler_type + handler_ref determine execution path (sql_function, http_endpoint, go_native, subprocess). | SPEC-010 §7 |
| AC-027 | Tool resolution order: internal hemisphere first → skill-linked tools → JIT registry (`custom_agent_tools`) → runtime built-ins. | SPEC-010 §Tool Resolution Order |
| AC-028 | External tools execute in a sandboxed subprocess (Deno or native Go) with per-tool timeout (default 30s). | SPEC-010 §3, §8 |
| AC-029 | Tool results are written to `tool_results` table: success rows (is_error=false, token_count), failure rows (is_error=true, error_code). Each result is written in its own mini-transaction. | SPEC-008 §ToolExecutionPhase |
| AC-030 | `tool_requests` status transitions: pending → completed | failed with `completed_at` timestamp. After all tools complete, session transitions from `tool_exec` back to `thinking`. | SPEC-008 §ToolExecutionPhase |
| AC-031 | Tool ownership RLS enforced: Agent A cannot modify Agent B's tool. Modification requests go through `task_queue` as internal tickets. | SPEC-010 §Row-Level Security |

### Heartbeat & Task Claiming (SPEC-008)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-032 | Heartbeat loop polls every 5 seconds; claims next ready task with atomic claim-and-update. Postgres uses FOR UPDATE SKIP LOCKED; SQLite uses advisory lock or claimed_at. | SPEC-008 §Heartbeat |
| AC-033 | `ClaimNextReadyTask` sets task status to `in_progress` atomically and returns the claimed task's id + session_id. If no pending tasks, returns nil. | SPEC-008 §Heartbeat |

### Display & Memory Pages (SPEC-002)

| ID | Criterion | Spec Source |
|---|---|---|
| AC-034 | `display_modes` table replaces mutable display state on `memory_events`. The `active_context_view` JOINs `display_modes` to resolve `full` / `compressed` / `hidden` rendering per memory event. | SPEC-002 §3.2, SPEC-002 §2.3 Note, SPEC-011 §3 |
| AC-035 | `memory_pages` table resolves named page references: `target_ids` (→ memory_events.id) and `linked_page_ids` (→ other memory_pages.id, single-level expansion) are expanded into active pointers. Deduplication via DISTINCT ON id ensures overlapping pages don't duplicate tokens. | SPEC-002 §5 |
| AC-036 | `active_context_view` uses session-level RLS scoping (`current_setting('consensus.session_id')`) to ensure each session sees only its own events. Hidden events (WHERE dm.mode = 'hidden') are excluded. | SPEC-002 §3.2 |

### Compression & Model Registry

| ID | Criterion | Spec Source |
|---|---|---|
| AC-037 | `compression_queue` table is present; entries track pending compression jobs with status lifecycle. | SPEC-002 §7 |
| AC-038 | `model_registry` table is readable by the harness for LLM routing decisions. | SPEC-003 §2.14, SPEC-008 §Model Cascade |

## Decision Points

- [DP-1] First slice: one-shot (single-turn) path only, or also multi-turn? **Decision:** implement both — the single-turn path is the fast path; multi-turn is needed for SPEC-020 coverage but can be a later phase. The plan.yaml reflects this: Phase 1 = one-shot, Phase 2 = multi-turn expansion.
- [DP-2] Full pg_cron setup or schema-only? **Decision:** schema-only for SQL statements (`SELECT cron.schedule(...)`) — the actual pg_cron extension installation is out of scope.
- [DP-3] SQLite parity for every Postgres feature? **Decision:** implement the driver interface; SQLite-specific implementations (Go-layer RLS, WHERE injection) are scoped in but only to the extent the parity matrix in SPEC-002 §Parity Matrix demands.

## Assumptions

| # | Assumption | How to Verify | Impact if Wrong |
|---|-----------|---------------|-----------------|
| A-1 | `schema-memory-01` has created all tables cited in SPEC-006, SPEC-008, SPEC-012, SPEC-020 before this work item starts. | Check DB schema before first iteration. | Harness loop cannot run — blocked until schema work completes. |
| A-2 | `repo-bootstrap-01` has established the Go module, driver interface, and package skeleton. | `go build ./...` succeeds before starting. | Cannot import harness packages — blocked. |
| A-3 | LLM backend is mockable in tests (no real API keys needed in CI). | Run integration tests with mock LLM. | Tests cannot run without real API keys — slower feedback. |
| A-4 | pg_cron extension is available in target Postgres environments. | `SELECT * FROM cron.job` returns rows. | Crash recovery SQL statements fail — must fall back to Go-level timer-based reaping. |

## Open Questions

1. **Partial commit** (SPEC-020 §15 Q5): should the agent be able to commit a subset of staged commands, or always all-or-nothing? SPEC-020 leaves this open. Default assumption: all-or-nothing per iteration; partial commit deferred to a future spec amendment.
2. **Turn-level billing** (SPEC-020 §15 Q6): each LLM call is a separate billing event? Assumption: yes, each turn is a billing row; the iteration aggregates. Needs confirmation from SPEC-011.
3. **Prompt size vs. detail** (SPEC-012 §8 Q1): should dynamic table JSONB schemas be in the system prompt or queried on demand? Assumption: on-demand for now; prompt includes table names + comments only.
4. **Constraint discovery frequency** (SPEC-012 §8 Q4): query constraints every iteration or cache for the session? Assumption: cache for the session (Layer 2); re-query only on ALTER TABLE events.
