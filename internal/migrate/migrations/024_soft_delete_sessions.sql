-- 024_soft_delete_sessions.sql — soft-delete tombstone for sessions
-- (DF-CONSENSUS-28: DELETE /api/v1/sessions/{id} previously overloaded
-- status='failed' as a tombstone, so "deleted" sessions stayed visible in
-- list/get and kept accepting messages).
--
-- Soft delete is expressed as a NULLable deleted_at timestamp (SPEC-003 §2.1,
-- SPEC-015 §3.1). It deliberately does NOT add a status vocabulary member:
-- status is a CHECK-constrained lifecycle FSM (SPEC-011 §1) and 'deleted' is
-- an API-visibility state, not a lifecycle state. A plain nullable-column
-- ALTER works on both backends without a SQLite table rebuild.

ALTER TABLE sessions ADD COLUMN deleted_at TIMESTAMPTZ;
