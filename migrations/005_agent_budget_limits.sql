-- ============================================================================
-- Conscience — 005_agent_budget_limits.sql
-- ============================================================================
-- Budget limits per agent. Enforces caps on tokens and cost at per-task,
-- per-hour, and per-day scopes. The harness checks cumulative spend against
-- these limits before each LLM call (SPEC-006 §Budget limits).
--
-- axiom:trace work_item=spec-003-hardening-01
--   spec=specs/003-database.md,specs/006-transactions.md,specs/011-canonical-definitions.md
--   plan=sweep-007-remediation
--   source_sweep=sweep-007
--
-- Gap: agent_budget_limits table was defined in SPEC-006 but had no migration file.
-- Fix: Create canonical schema with scope-based constraints.
-- ============================================================================

BEGIN;

-- 1. agent_budget_limits — Per-agent budget caps
-- Canonical schema from SPEC-006 §Budget Limits.
CREATE TABLE agent_budget_limits (
    agent_id        UUID NOT NULL REFERENCES sessions(id),
    scope           TEXT NOT NULL CHECK (scope IN ('per_task', 'per_hour', 'per_day')),
    max_tokens      INT NOT NULL,
    max_cost_cents  INT NOT NULL,
    PRIMARY KEY (agent_id, scope)
);

-- 2. Index for fast budget lookups by session
CREATE INDEX idx_budget_limits_agent ON agent_budget_limits(agent_id);

COMMIT;

-- ============================================================================
-- END OF MIGRATION 005
-- Summary of objects created:
--   1 table (agent_budget_limits)
--   1 index
-- ============================================================================
