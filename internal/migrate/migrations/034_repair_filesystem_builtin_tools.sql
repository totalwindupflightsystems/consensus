-- ============================================================================
-- Consensus — 034_repair_filesystem_builtin_tools.sql
-- ============================================================================
-- DF-CONSENSUS-79: migration 033 was shipped insert-only (ON CONFLICT DO
-- NOTHING), so installs that already carried read/write/edit/glob/bash rows
-- from 032 kept their stale dispatch fields (e.g. handler_ref
-- 'legacy_stub', status 'disabled', enabled false). Databases that already
-- recorded 033 can never be repaired by re-editing 033: the runner skips
-- applied versions, and editing their content raises checksum drift that
-- blocks every later migration.
--
-- This migration performs the repair as a forward data-only step, so both
-- pre-033 and already-at-033 installs converge on the real native handlers
-- declared in internal/tools/native.go.
--
-- Scope: only the reserved builtin names below, and only their dispatch
-- fields (handler_type, handler_ref, status, enabled). Operator-owned
-- metadata (description, parameter schema, hemisphere, ownership, rate
-- limits) is preserved, and unrelated tool names are never touched. Rows
-- that are already correct are rewritten to the same values, so re-running
-- is a no-op.
-- ============================================================================

BEGIN;

UPDATE tools_registry SET handler_type = 'go_native', handler_ref = 'read_file', status = 'active', enabled = true, updated_at = CURRENT_TIMESTAMP WHERE name = 'read';
UPDATE tools_registry SET handler_type = 'go_native', handler_ref = 'write_file', status = 'active', enabled = true, updated_at = CURRENT_TIMESTAMP WHERE name = 'write';
UPDATE tools_registry SET handler_type = 'go_native', handler_ref = 'edit', status = 'active', enabled = true, updated_at = CURRENT_TIMESTAMP WHERE name = 'edit';
UPDATE tools_registry SET handler_type = 'go_native', handler_ref = 'glob', status = 'active', enabled = true, updated_at = CURRENT_TIMESTAMP WHERE name = 'glob';
UPDATE tools_registry SET handler_type = 'go_native', handler_ref = 'bash', status = 'active', enabled = true, updated_at = CURRENT_TIMESTAMP WHERE name = 'bash';

COMMIT;
