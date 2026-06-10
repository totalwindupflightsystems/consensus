---
work_item_id: schema-memory-01
status: in-progress
repo: wojons/conscientiousness
updated: 2026-05-03
run_id: 2026-05-03-spec-align-fix-01
---

# Plan — Schema + Memory Foundation

Build the database first because Conscience treats the database as the runtime. The smallest safe slice is a migration draft that captures canonical tables/enums and can be verified before harness code exists.

axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/003-database.md,specs/005-security.md,specs/007-json-schema.md,specs/011-canonical-definitions.md plan=phase-1 task-1 step-1 evidence=.memory-bank/work-items/schema-memory-01/verification.md

## AC → Verification

| AC | Description | Verification Path | Status |
|---|---|---|---|
| AC-MEM-001 | Core Tables Exist (18+ tables) | Inspect migration; run `psql -f` or `sqlite3 < migration.sql` and query `information_schema.tables` | not-started |
| AC-MEM-002 | Append-Only memory_events (REVOKE UPDATE/DELETE, compression_worker SUMMARY_TEXT only) | Review GRANT/REVOKE statements in migration | not-started |
| AC-MEM-003 | display_modes Table (separate, mutable, CHECK mode IN full/compressed/hidden) | Verify CREATE TABLE + GRANT statements in migration | not-started |
| AC-MEM-004 | active_context_view (JOIN display_modes, CASE logic, session isolation, DISTINCT dedup) | Verify VIEW definition in migration matches SPEC-011 §3.4 | not-started |
| AC-MEM-005 | memory_pages with linked_page_ids (single-level expansion CTE) | Verify table schema and expansion query in migration | not-started |
| AC-MEM-006 | compression_queue Table (event_id, current_tier, next_tier, attempts) | Verify CREATE TABLE in migration | not-started |
| AC-MEM-007 | model_registry with 3-Tier Routing (tier CHECK 1/2/3, routing queries documented) | Verify CREATE TABLE + documented routing queries | not-started |
| AC-MEM-008 | RLS Policies (ENABLE RLS on all core tables + 4-role GRANT/REVOKE model) | Verify RLS + GRANT block in migration against SPEC-011 §13 table | not-started |
| AC-MEM-009 | external_quarantine (Cognitive Firewall — CHECK constraints, expires_at, FK to memory_events) | Verify CREATE TABLE + index in migration | not-started |
| AC-MEM-010 | secret_access_audit + Vault (audit table, agent_role REVOKE on vault) | Verify CREATE TABLE + REVOKE in migration | not-started |
| AC-MEM-011 | SQL Statement Classification Safety Policy (classifier, multi-statement split, core table whitelist, DANGEROUS=blocked) | Verify documentation block in migration comments or companion file | not-started |
| AC-MEM-012 | JSON Schema DB Validation (pg_jsonschema extension + example CHECK, SQLite parity noted) | Verify CREATE EXTENSION + example constraint in migration | not-started |
| AC-MEM-013 | LLM Output Schema + audit_logs (audit_logs table, JSON schema documented) | Verify CREATE TABLE + documented schema contract | not-started |
| AC-MEM-014 | Canonical Enum/State Values from SPEC-011 (all CHECK constraints use lowercase canonical values) | Grep migration for CHECK constraints and cross-reference SPEC-011 canonical lists | not-started |
| AC-MEM-015 | SQLite/PocketBase Parity Per Feature (parity table covering all 9+ feature gaps) | Verify parity documentation in migration comments or parity.md | not-started |
| AC-MEM-016 | Migration Parse/Apply Verified (parses against Postgres or SQLite) | Run `psql -f` or equivalent; capture evidence | not-started |

## Phases

### Phase 1 — Core Tables & Memory Ledger (SPEC-002, SPEC-003)

**Phase 1 Task 1:** Create all core tables from SPEC-003 §2 and SPEC-011 §12.
- `sessions` with canonical status CHECK constraint (SPEC-011 §1).
- `memory_events` — append-only, NO display_mode column, CHECK on type (SPEC-011 §3.5 8 canonical types).
- `display_modes` — separate mutable table (SPEC-011 §3.2).
- `iteration_commits` — with `llm_response JSONB`, `sql_executed TEXT[]`, `rows_affected` (SPEC-011 §11).
- `tasks` with canonical status CHECK constraint (SPEC-011 §2).
- `memory_pages` with `linked_page_ids` (SPEC-002 §5).
- `tool_requests`, `tool_results`, `tools_registry` (canonical from SPEC-011 §4.2.1), `skills_registry` (SPEC-011 §4.2.2).
- `agent_billing` with cache tokens (SPEC-011 §6.2).
- `workflows`, `custom_agent_tools` (SPEC-011 §5.2), `tool_files`.
- `model_registry` with tier routing (SPEC-002 §9, SPEC-003 §2.14).
- `compression_queue` (SPEC-002 §8.4).
- `agent_messages`, `system_settings` (SPEC-011 §12.1-12.2).
- `audit_logs` (SPEC-011 §12.3).
- All relevant indexes and `CREATE UNIQUE INDEX` statements from SPEC-003.

**Phase 1 Task 2:** Define `active_context_view` per SPEC-011 §3.4 and SPEC-002 §3.
- CTE for active pointers from `iteration_commits`.
- LEFT JOIN `display_modes`.
- CASE for compressed/hidden/full rendering.
- Session isolation via `current_setting('conscience.session_id')`.
- DISTINCT ON for page deduplication (SPEC-002 §3.6).
- `string_agg()` Markdown aggregation query (SPEC-002 §7.3).

**Phase 1 Task 3:** Define `memory_pages` resolution CTE — expansion of both `target_ids` and `linked_page_ids` with single-level nesting (SPEC-002 §5.2, SPEC-003 §2.4).

### Phase 2 — Security: RLS, Roles, Quarantine, Secrets (SPEC-005)

**Phase 2 Task 1:** Enable RLS on all core tables and create session-isolation policies.
- `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` for: `memory_events`, `display_modes`, `iteration_commits`, `memory_pages`, `tasks`, `tool_requests`, `tool_results`, `tool_files`, `external_quarantine`, `agent_billing`, `compression_queue`, `agent_messages`, `audit_logs`.
- `CREATE POLICY session_isolate_*` with `FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID)`.

**Phase 2 Task 2:** Implement the 4-role permission model (SPEC-011 §13, SPEC-003 §7.5).
- `agent_role`: SELECT/INSERT on memory_events; SELECT/INSERT/UPDATE on display_modes; SELECT/INSERT on iteration_commits; limited UPDATE on sessions (status, heartbeat); SELECT/INSERT/UPDATE on tasks; SELECT/INSERT on tool_requests; SELECT on active_context_view; REVOKE all on vault.
- `compression_worker`: GRANT UPDATE (summary_text) ON memory_events; SELECT/INSERT/UPDATE on display_modes and compression_queue.
- `alt_mode_role`: GRANT ALL ON ALL TABLES, BYPASSRLS.
- `tool_executor`: GRANT SELECT/INSERT on tool_results; GRANT UPDATE on tool_requests (status only).
- Explicit `REVOKE UPDATE, DELETE ON memory_events FROM agent_role`.

**Phase 2 Task 3:** Cognitive firewall — `external_quarantine` with constraints, indexes, and documented scanning flow (SPEC-005 §Cognitive Firewall).

**Phase 2 Task 4:** Secret handling — `secret_access_audit` table, vault REVOKE statements, `{{SECRET.*}}` alias pattern documented, harness scrubbing note (SPEC-005 §Zero-Knowledge Secrets).

### Phase 3 — JSON Schema & SQL Safety (SPEC-007, SPEC-011)

**Phase 3 Task 1:** JSON Schema DB validation infrastructure.
- `CREATE EXTENSION IF NOT EXISTS pg_jsonschema`.
- Document `jsonb_matches_schema(...)` CHECK constraint pattern with example (SPEC-003 §4.2).
- Document `sqlite-jsonschema` identical-syntax parity, including known limitations (`$ref`, `format`) (SPEC-003 §4.3–4.5).

**Phase 3 Task 2:** LLM output JSON schema contract documented in migration comments.
- Required: `internal_monologue` (string).
- Optional: `memory_state_changes`, `system_actions`, `tool_requests`, `sub_agent_spawns`, `task_update`.
- `audit_logs` table with `monologue TEXT`, `sql_executed TEXT[]`, `result CHECK (committed, rolled_back)` (SPEC-011 §12.3).

**Phase 3 Task 3:** SQL statement classification safety policy (SPEC-011 §8).
- Document multi-statement splitting on semicolons.
- Document classifier: `DML_READ | DML_WRITE | DML_DELETE | DDL_CREATE | DDL_ALTER | DANGEROUS`.
- Document execution policy table (SPEC-011 §8.3 Layer 2).
- Document core table whitelist (SPEC-011 §8.3 Layer 3): `memory_events`, `display_modes`, `iteration_commits`, `memory_pages`, `tasks`, `tool_requests`, `tool_results`, `sessions`, `custom_agent_tools`, `tools_registry`, `skills_registry`, `agent_billing`, `workflows`, `tool_files`, `external_quarantine`, `compression_queue`, `model_registry`.
- Document stored-procedure preference (SPEC-011 §8.3 Layer 4): `set_display_mode()`, `complete_session()`, `create_agent_memory_table()`.

**Phase 3 Task 4:** Dynamic entity generator function (`create_agent_memory_table`) with reserved-name blocklist including ALL 21 reserved table names from SPEC-003 §3.2. Document SQLite/PocketBase Go hook equivalent.

### Phase 4 — Canonical Definitions & Parity (SPEC-011)

**Phase 4 Task 1:** Audit all CHECK constraints for canonical enum values from SPEC-011.
- `sessions.status`: booting, idle, planning, thinking, tool_exec, executing, waiting_sub, paused, completed, failed.
- `tasks.status`: pending, claimed, in_progress, reviewed, published, failed, cancelled.
- `memory_events.type`: header, text_block, tool_call, tool_result, thinking, system, inherited_pointer, user_message.
- `tools_registry.status`: active, testing, deprecated, disabled.
- `tools_registry.hemisphere`: internal, external.
- `tools_registry.handler_type`: sql_function, http_endpoint, go_native, subprocess.
- `agent_billing.category`: cognition, compression, embedding, tool_call.
- `display_modes.mode`: full, compressed, hidden.
- `external_quarantine.validation_status`: pending, validated, rejected, expired.
- `external_quarantine.source_type`: scrape, api_response, file_upload, user_paste.
- `custom_agent_tools.status`: draft, testing, active, deprecated.
- `custom_agent_tools.language`: javascript, typescript, sql, python, go.
- `tool_requests.status`: pending, executing, completed, failed, timeout.

**Phase 4 Task 2:** Produce SQLite/PocketBase parity documentation covering all 11+ feature rows.
- Vector search, JSON Schema, RLS, background jobs, async HTTP, JavaScript runtime, append-only enforcement, triggers, WAL mode, secrets, dynamic table creation (SPEC-003 §8.2).
- PocketBase API Rules as RLS equivalent with auth-context enforcement note (SPEC-005 §PocketBase).
- Exponential backoff + jitter for SQLite write contention (SPEC-003 §8.7).
- 4-role model mapping to PocketBase (SPEC-005 §PocketBase table).
- `SET LOCAL` safety with Supavisor/PgBouncer transaction pooling (SPEC-011 §9.5).
- ORM configuration (`prepare: false` for Drizzle on Supavisor port 6543) (SPEC-011 §9.5).

### Phase 5 — Verification & Evidence

**Phase 5 Task 1:** Apply migration against Postgres (or SQLite if Postgres unavailable) and capture output.
- Command: `psql -h localhost -U postgres -d conscience_test -f migrations/001_initial_schema.sql` (or `sqlite3 :memory: < migrations/001_initial_schema.sql`).
- Verify exit code 0.
- Query `information_schema.tables` to confirm all expected tables exist.

**Phase 5 Task 2:** Populate `verification.md` with evidence from all checks.
- Mark each AC as PASS/FAIL with evidence citation.

**Phase 5 Task 3:** Run spec-verifier agent (`@spec-verifier-axiom`) to confirm contract alignment between migration and all cited specs.

## Trace Markers

| Step | Full Trace |
|---|---|
| Phase 1 Task 1 | `axiom:trace work_item=schema-memory-01 spec=specs/003-database.md,specs/011-canonical-definitions.md plan=phase-1/task-1` |
| Phase 1 Task 2 | `axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/011-canonical-definitions.md plan=phase-1/task-2` |
| Phase 2 Task 1-4 | `axiom:trace work_item=schema-memory-01 spec=specs/005-security.md,specs/003-database.md plan=phase-2` |
| Phase 3 Task 1-2 | `axiom:trace work_item=schema-memory-01 spec=specs/007-json-schema.md,specs/003-database.md plan=phase-3` |
| Phase 3 Task 3 | `axiom:trace work_item=schema-memory-01 spec=specs/011-canonical-definitions.md plan=phase-3/task-3` |
| Phase 4 | `axiom:trace work_item=schema-memory-01 spec=specs/011-canonical-definitions.md,specs/003-database.md plan=phase-4` |
| Phase 5 | `axiom:trace work_item=schema-memory-01 plan=phase-5 evidence=.memory-bank/work-items/schema-memory-01/verification.md` |

## Commit Template

```
feat(schema): add core database schema migration for SPEC-002/003/005/007/011

Covers all 18+ core tables, active_context_view, RLS policies,
4-role permission model, cognitive firewall, secret handling,
JSON Schema validation infrastructure, SQL statement classifier,
canonical enums from SPEC-011, and SQLite/PocketBase parity docs.

axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/003-database.md,specs/005-security.md,specs/007-json-schema.md,specs/011-canonical-definitions.md
```
