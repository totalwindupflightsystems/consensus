-- ============================================================================
-- Consensus — 032_seed_builtin_tools.sql
-- ============================================================================
-- DF-CONSENSUS-62: the HITL approval gate was unreachable from any user-facing
-- surface on a fresh install.
--
-- Evidence (dogfood 2026-10-20, docs/dogfood/2026-10-20-approvals-pty-gate.md):
--   * `select count(*) from tools_registry` = 0 after `consensus init`
--   * `grep -rn 'INSERT INTO tools_registry' migrations/ internal/` found no seed
--   * `consensus tool` was inspect-only (list/show) — no register verb
--   * internal/api/server.go registered GET /api/v1/tools and
--     POST /api/v1/tools/{toolName}/execute, but no create/update route
--
-- Net: the gate fires on tools_registry rows with requires_approval=true
-- (SPEC-014 §2.2, internal/tools/approval.go), but the only way to create such
-- a row was a hand-written INSERT. This migration ships the built-in rows a
-- fresh install can actually dispatch, so the documented approval flow is
-- reachable end to end. The registration surfaces (POST /api/v1/tools and
-- `consensus tool register`) cover everything beyond this seed.
--
-- Seeded rows (deliberately small and honest — no invented capabilities):
--
--   list_sessions       go_native  Read-only. Implemented by the REST execute
--                                  path (internal/api/tools.go
--                                  executeGoNativeTool) and by the async
--                                  executor's go_native handler.
--   demo_approval_gate  go_native  requires_approval=true. Executing it parks
--                                  the tool request at 'awaiting_approval' for
--                                  human review before it runs. This is the
--                                  on-ramp to the approval flow: a fresh
--                                  install can walk
--                                  request → awaiting_approval → approve →
--                                  execute without inventing a destructive
--                                  tool just to exercise the gate.
--
-- Idempotency: ON CONFLICT (name) DO NOTHING (same pattern as
-- 029_seed_default_chat_model.sql). Re-running never overwrites an operator's
-- own registration, and an existing install that added these rows by hand is
-- left untouched.
--
-- No schema change: tools_registry is defined in 001_initial_schema.sql
-- (Postgres) and 009_sqlite_task_tool_tables.sql (SQLite). Only data lands here.
--
-- axiom:trace row=DF-CONSENSUS-62
--   spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md
--   impl=internal/migrate/migrations/032_seed_builtin_tools.sql
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — BUILT-IN TOOLS A FRESH INSTALL CAN DISPATCH
-- ============================================================================
-- id / created_at / updated_at / rate_limit_per_min are left to their column
-- defaults: the id default is dialect-specific (PG gen_random_uuid, SQLite
-- randomblob) and both are exercised by INSERTs that omit the column.

INSERT INTO tools_registry
    (name, description, hemisphere, parameter_schema, handler_type, handler_ref,
     status, enabled, requires_approval)
VALUES
    ('list_sessions',
     'List the most recent agent sessions (id, agent_name, status, iteration).',
     'internal',
     '{"type":"object","properties":{},"additionalProperties":false}',
     'go_native',
     'list_sessions',
     'active',
     true,
     false)
ON CONFLICT (name) DO NOTHING;

INSERT INTO tools_registry
    (name, description, hemisphere, parameter_schema, handler_type, handler_ref,
     status, enabled, requires_approval)
VALUES
    ('demo_approval_gate',
     'Example approval-gated tool: every execution request parks at awaiting_approval for human review before it runs (no-op acknowledgement on approval).',
     'internal',
     '{"type":"object","properties":{"note":{"type":"string"}},"additionalProperties":true}',
     'go_native',
     'demo_approval_gate',
     'active',
     true,
     true)
ON CONFLICT (name) DO NOTHING;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary:
--   - 2 tools_registry rows: list_sessions (approval-free) and
--     demo_approval_gate (requires_approval=true) — the approval gate is now
--     reachable from a fresh install without direct DB writes.
-- ============================================================================
