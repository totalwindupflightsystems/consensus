-- Migration 021: Grant agent roles to the application user
-- =========================================================
-- The application now uses SET ROLE agent_role on every connection
-- (AfterConnect hook in internal/db/postgres/postgres.go).
-- For this to work, the database user (whoever the app connects as)
-- must be a member of the agent_role, compression_worker,
-- alt_mode_role, and tool_executor roles.
--
-- This migration is idempotent — GRANT on an already-granted
-- membership is a no-op in PostgreSQL.
--
-- axiom:trace work_item=phase3-fix-set-role spec=specs/003-database.md

GRANT agent_role TO CURRENT_USER;
GRANT compression_worker TO CURRENT_USER;
GRANT alt_mode_role TO CURRENT_USER;
GRANT tool_executor TO CURRENT_USER;
