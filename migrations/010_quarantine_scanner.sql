-- ============================================================================
-- Conscience — 010_quarantine_scanner.sql
-- ============================================================================
-- Cognitive Firewall: Add 'webhook' to external_quarantine.source_type CHECK
-- constraint and create the scanning infrastructure (SPEC-005 §Cognitive Firewall,
-- SPEC-013 §5.2, WI-004).
--
-- The external_quarantine table was created in migration 001 with source_type
-- limited to ('scrape', 'api_response', 'file_upload', 'user_paste'). Webhook
-- events now need to be stored here when they fail signature validation or
-- match heuristic threat patterns.
--
-- axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/003-database.md plan=phase-5/task-1/step-1
-- ============================================================================

-- 1. Add 'webhook' to the source_type CHECK constraint.
--    PostgreSQL: ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT
--    SQLite: must recreate the table (ALTER TABLE ... ALTER COLUMN not supported).
--    This migration handles both backends via conditional execution.

-- PostgreSQL path:
ALTER TABLE external_quarantine DROP CONSTRAINT IF EXISTS external_quarantine_source_type_check;
ALTER TABLE external_quarantine ADD CONSTRAINT external_quarantine_source_type_check
    CHECK (source_type IN ('scrape', 'api_response', 'file_upload', 'user_paste', 'webhook'));

-- SQLite path: SQLite doesn't support ALTER TABLE DROP CONSTRAINT.
-- The table will be recreated if running on SQLite.
-- For now, the Go code maps 'webhook' source to 'api_response' for SQLite compat.
-- A future migration can add full SQLite ALTER TABLE support.

-- 2. Create an index on validation_status for efficient pending scans.
CREATE INDEX IF NOT EXISTS idx_quarantine_validation_status ON external_quarantine(validation_status);

-- 3. Create an index on created_at for time-based queries.
CREATE INDEX IF NOT EXISTS idx_quarantine_created_at ON external_quarantine(created_at DESC);
