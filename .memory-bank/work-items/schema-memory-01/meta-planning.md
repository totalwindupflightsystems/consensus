---
work_item_id: schema-memory-01
status: not-started
repo: wojons/conscientiousness
created: 2026-05-03
updated: 2026-05-03T00-00Z  # spec-alignment-scan-01 repair run
run_id: 2026-05-03-spec-align-fix-01
specs_cited:
  - specs/002-memory.md
  - specs/003-database.md
  - specs/005-security.md
  - specs/007-json-schema.md
  - specs/011-canonical-definitions.md
---

# Meta-Planning — Schema + Memory Foundation

Mission: turn the Conscience database and memory specs into a runnable schema foundation before any higher-level harness/API work depends on it. This work item owns the durable data model, append-only memory ledger, canonical enums, security constraints, and JSON validation rules.

axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/003-database.md,specs/005-security.md,specs/007-json-schema.md,specs/011-canonical-definitions.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/schema-memory-01/verification.md prompt=.memory-bank/work-items/_prompt.md

## Scope

In scope:
- Consolidated SQL migration draft for all core tables/enums from SPEC-003 §2 and SPEC-011.
- Memory event ledger (`memory_events`) as truly append-only with separate `display_modes` table (SPEC-002 §2, SPEC-011 §3).
- `active_context_view` as dynamic SQL VIEW joining `display_modes` (SPEC-002 §3, SPEC-003 §3.2, SPEC-011 §3.4).
- `memory_pages` with `linked_page_ids` single-level expansion (SPEC-002 §5, SPEC-003 §2.4).
- `compression_queue` and background worker contract (SPEC-002 §8, SPEC-011 §3.6).
- `model_registry` with 3-tier routing matrix (SPEC-002 §9, SPEC-003 §2.14).
- Row-Level Security (RLS) policies on all core tables with the 4-role model from SPEC-011 §13.
- Agent-facing grants (`agent_role`, `compression_worker`, `tool_executor`, `alt_mode_role`) per SPEC-003 §7.5.
- Cognitive firewall tables (`external_quarantine`) (SPEC-005 §Cognitive Firewall, SPEC-003 §2.13).
- Zero-knowledge secret handling (`secret_access_audit`, `vault.secrets`) (SPEC-005 §Zero-Knowledge Secrets, SPEC-003 §7.6).
- SQL statement classification safety policy — multi-statement splitting, classifier, table whitelist, stored-procedure routing (SPEC-011 §8, SPEC-007 §SQL Inside JSON — Safety).
- JSON Schema validation for dynamic entity `data` columns via `pg_jsonschema` / `sqlite-jsonschema` (SPEC-003 §4, SPEC-002 §10.4).
- LLM output JSON schema contract and `audit_logs` table (SPEC-007 §Output Schema, SPEC-011 §12.3).
- Canonical enum values for `sessions.status`, `tasks.status`, `memory_events.type`, `tools_registry.status`/`hemisphere`/`handler_type`, `agent_billing.category`, `compression_queue` states (SPEC-011 §1–§6).
- SQLite/PocketBase parity decisions documented per feature (SPEC-003 §8, SPEC-011 §9.6).

Out of scope:
- Harness loop execution, iteration runner, tool dispatcher.
- REST/MCP/CLI surfaces.
- Deployment automation (Docker, CI/CD).
- Alt-Mode UI/dashboard.
- Live LLM integration or embedding pipeline.

## Acceptance Criteria

### AC-MEM-001 — Core Tables Exist
All 18+ core tables from SPEC-003 §2 and SPEC-011 §12 are defined in a migration file with correct column types, CHECK constraints, FKs, and indexes: `sessions`, `memory_events`, `display_modes`, `iteration_commits`, `memory_pages`, `tasks`, `tool_requests`, `tool_results`, `tools_registry`, `skills_registry`, `agent_billing`, `workflows`, `custom_agent_tools`, `tool_files`, `external_quarantine`, `model_registry`, `compression_queue`, `agent_messages`, `system_settings`, `audit_logs`, `secret_access_audit`.

### AC-MEM-002 — Append-Only memory_events
`memory_events` has `REVOKE UPDATE, DELETE ON memory_events FROM agent_role` enforced. No `display_mode` column exists on `memory_events`. The `compression_worker` role has `GRANT UPDATE (summary_text) ON memory_events TO compression_worker` — and no other UPDATE permission. (SPEC-002 §2, SPEC-011 §3.2–3.3)

### AC-MEM-003 — display_modes Table
A separate `display_modes` table exists with CHECK `mode IN ('full','compressed','hidden')`, PRIMARY KEY on `memory_id`, and full GRANT SELECT/INSERT/UPDATE to `agent_role`. Default (no row) = `'full'`. (SPEC-011 §3.2, SPEC-003 §2.2a)

### AC-MEM-004 — active_context_view
`active_context_view` is defined as a SQL VIEW that JOINs `display_modes` via LEFT JOIN, applies CASE logic for compressed/hidden modes, enforces session isolation via `current_setting('conscience.session_id')`, and uses `DISTINCT ON (id)` for page deduplication. (SPEC-002 §3, SPEC-011 §3.4)

### AC-MEM-005 — memory_pages with Linked Page Support
`memory_pages` table includes `linked_page_ids BIGINT[]` column. Resolution CTE in the view/query expands both `target_ids` and `linked_page_ids` with single-level nesting. (SPEC-002 §5, SPEC-003 §2.4)

### AC-MEM-006 — compression_queue
`compression_queue` table exists with columns for `event_id`, `current_tier`, `next_tier`, `status`, `attempts`, `created_at`. Background worker (go routine or pg_cron) contract is documented even if not yet implemented. (SPEC-002 §8.4, SPEC-011 §3.6)

### AC-MEM-007 — model_registry with 3-Tier Routing
`model_registry` exists with `tier INT CHECK (tier IN (1,2,3))`, `max_context`, `cost_per_m_in`, `cost_per_m_out`, `classifier_tags`, `enabled`. Routing queries (horizontal within tier, vertical escalation) are documented in comments or a helper function. (SPEC-002 §9, SPEC-003 §2.14)

### AC-MEM-008 — RLS Policies on All Core Tables
All core tables have `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` and a session-isolation policy (`FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID)`). The 4-role model is implemented with explicit GRANT/REVOKE statements: `agent_role` (INSERT/SELECT only on memory_events, no vault), `compression_worker` (UPDATE summary_text only), `alt_mode_role` (BYPASSRLS + ALL), `tool_executor` (tool_results + tool_requests status). (SPEC-003 §7.5, SPEC-005 §Row-Level Security, SPEC-011 §13)

### AC-MEM-009 — external_quarantine (Cognitive Firewall)
`external_quarantine` table exists with `CHECK (source_type IN (...))`, `validation_status CHECK`, `expires_at DEFAULT (now() + INTERVAL '1 hour')`, `promoted_memory_id FK → memory_events`. Index on `(session_id, validation_status) WHERE validation_status = 'pending'`. (SPEC-005 §Cognitive Firewall, SPEC-003 §2.13)

### AC-MEM-010 — secret_access_audit + Vault
`secret_access_audit` table exists with `session_id`, `secret_alias`, `accessed_at`. Vault schema is documented in migration comments or an install script. `agent_role` has `REVOKE SELECT ON vault.secrets FROM agent_role`. (SPEC-005 §Zero-Knowledge Secrets, SPEC-003 §7.6)

### AC-MEM-011 — SQL Statement Classification Safety Policy
The migration includes (or adjacent documentation explicitly defines) a SQL classifier with statement types `DML_READ | DML_WRITE | DML_DELETE | DDL_CREATE | DDL_ALTER | DANGEROUS`. Multi-statement splitting on semicolons is described. A core table whitelist is enumerated. Stored-procedure preference is documented. DANGEROUS = never execute. (SPEC-011 §8, SPEC-007 §SQL Inside JSON — Safety)

### AC-MEM-012 — JSON Schema DB Validation
Migration includes `CREATE EXTENSION IF NOT EXISTS pg_jsonschema` and at least one example CHECK constraint using `jsonb_matches_schema(...)` on a dynamic-entity-like table or as a documented pattern. SQLite parity via `sqlite-jsonschema` with identical syntax is documented. (SPEC-003 §4, SPEC-002 §10.4)

### AC-MEM-013 — LLM Output Schema + audit_logs
`audit_logs` table exists with columns `session_id`, `iteration`, `monologue`, `sql_executed TEXT[]`, `result CHECK (result IN ('committed','rolled_back'))`. The JSON output schema contract (`internal_monologue`, `memory_state_changes`, `system_actions`) from SPEC-007 § is documented in migration comments or a companion schema doc. (SPEC-007 §, SPEC-011 §12.3)

### AC-MEM-014 — Canonical Enum/State Values from SPEC-011
All CHECK constraints use canonical lowercase values:
- `sessions.status`: booting, idle, planning, thinking, tool_exec, executing, waiting_sub, paused, completed, failed
- `tasks.status`: pending, claimed, in_progress, reviewed, published, failed, cancelled
- `memory_events.type`: header, text_block, tool_call, tool_result, thinking, system, inherited_pointer, user_message
- `tools_registry.status`: active, testing, deprecated, disabled
- `tools_registry.hemisphere`: internal, external
- `tools_registry.handler_type`: sql_function, http_endpoint, go_native, subprocess
- `agent_billing.category`: cognition, compression, embedding, tool_call
- `display_modes.mode`: full, compressed, hidden
- `external_quarantine.validation_status`: pending, validated, rejected, expired
- `custom_agent_tools.status`: draft, testing, active, deprecated
(SPEC-011 §1–§6, SPEC-003 §2)

### AC-MEM-015 — SQLite/PocketBase Parity Per Feature
A parity decision table is included in migration comments or a separate `.memory-bank/work-items/schema-memory-01/parity.md` covering: vector search (pgvector → sqlite-vec), JSON Schema (pg_jsonschema → sqlite-jsonschema), RLS (native → API Rules + Go hooks), background jobs (pg_cron → Go goroutines + ticker), async HTTP (pg_net → Go net/http), append-only enforcement (REVOKE → Go OnRecordBeforeUpdateRequest hook), secrets (Vault → env vars + encrypted .env), dynamic table creation (SECURITY DEFINER → Go OnRecordBeforeCreateRequest hook), WAL mode requirement, and write-contention backoff strategy. (SPEC-003 §8, SPEC-011 §9.6)

### AC-MEM-016 — Migration Parse/Apply Verified
The migration file parses successfully against at least one backend (Postgres via `psql -f` or SQLite via `sqlite3 < migration.sql`). Verification evidence is recorded. (Self-evident, anchors all other ACs.)

## Assumptions

- [ASSUMPTION] Implementation will start Postgres-first, then add SQLite parity. How to verify: confirm with user or inspect first implementation PR. Impact if wrong: migration layout may need dual-backend restructuring.
- [ASSUMPTION] `pg_jsonschema` and `sqlite-jsonschema` extensions are available/loadable in the target environments. How to verify: extension existence check during migration apply. Impact if wrong: fallback to application-layer JSON validation needed.

## Non-Goals

- Do NOT implement the harness, iteration loop, or tool dispatch.
- Do NOT connect to a live LLM.
- Do NOT produce a running agent — schema-only deliverable.

## Risks

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | SQLite parity may require different constraint enforcement | Medium | Explicit parity table; PocketBase hooks documented as equivalent |
| R2 | pg_jsonschema extension may not be available in all Postgres deployments | Low | Conditional `CREATE EXTENSION IF NOT EXISTS`; app-layer fallback noted |
| R3 | 18+ tables in a single migration may be hard to review | Low | Organize by subsystem (core → memory → security → billing); each section headed with spec refs |
| R4 | `current_setting('conscience.session_id')` depends on harness setting it — not testable in isolation | Low | RLS policies included but marked as requiring harness context for live verification |
