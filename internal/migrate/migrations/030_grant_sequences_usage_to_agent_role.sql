-- ============================================================================
-- Conscience — 030_grant_sequences_usage_to_agent_role.sql
-- ============================================================================
-- PG-SEQ-42501: the application pool runs EVERY connection under
-- SET ROLE agent_role (internal/db/postgres/postgres.go, AfterConnect hook),
-- but migration 028 granted only TABLE verbs — no sequence was ever granted
-- USAGE, so any pool INSERT that draws from a BIGSERIAL fails with SQLSTATE
-- 42501 ("permission denied for sequence ...").
--
-- Reported failure (PG probe, fresh pgvector/pg16 DB built from main,
-- 2026-10-16):
--   INSERT INTO memory_events ...: permission denied for sequence
--   memory_events_id_seq (SQLSTATE 42501)
--
-- Census context (measured on the fresh DB above): 14 sequences exist in
-- schema public and has_sequence_privilege('agent_role', <seq>, 'USAGE') was
-- false for ALL of them before this migration. The grant below is the same
-- catalog-wide shape 028 uses for tables: grant to ALL sequences in the
-- schema, not an enumerated list, so migrations that add sequences later do
-- not silently reintroduce the defect.
--
-- Scope: USAGE alone satisfies every pool write path — nextval() fires on
-- INSERT even without USAGE (only currval/setval/lastval need SELECT) — but
-- the row's prescribed fix grants SELECT as well, mirroring 028's
-- SELECT-bearing shape, so currval-style read-backs stay covered too. The
-- sequence privileges do not gate the role's own sequences (agent_role owns
-- none today; on a fresh DB every sequence is owned by the migration role),
-- so the catalog-wide grant cannot self-invalidate.
--
-- Idempotency: GRANT is naturally idempotent in PostgreSQL (re-granting an
-- existing privilege is a no-op), and the pg→SQLite filter strips GRANT
-- statements (internal/migrate/migrate.go filterForSQLite), so this migration
-- is a no-op on SQLite and safe to re-run on Postgres.
--
-- axiom:trace work_item=PG-SEQ-42501
--   spec=specs/003-database.md,specs/005-security.md
--   impl=internal/migrate/migrations/030_grant_sequences_usage_to_agent_role.sql
-- ============================================================================

BEGIN;

-- Every BIGSERIAL-backed INSERT through the agent_role pool needs USAGE on
-- the owning sequence (SELECT covers currval/lastval-style reads).
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO agent_role;

COMMIT;
