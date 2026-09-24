-- ============================================================================
-- Conscience — 012_trust_level.sql
-- ============================================================================
-- Add agent_trust_level to sessions for the three-tier SQL execution model
-- (SPEC-008 §5.4, WI-006).
--
-- Default is 'high' for backward compatibility: existing sessions and test
-- sessions retain Tier 3 (raw SQL) access. New low-trust sessions must
-- explicitly set trust_level = 'low' or 'medium'.
--
-- axiom:trace work_item=WI-006 spec=specs/003-database.md,specs/008-harness.md plan=phase-3/task-1
-- ============================================================================

BEGIN;

ALTER TABLE sessions
ADD COLUMN trust_level TEXT NOT NULL DEFAULT 'high'
CHECK (trust_level IN ('low', 'medium', 'high'));

COMMIT;
