-- ============================================================================
-- Migration 023: zero-UUID tenant_id column defaults (BUG-008)
-- ============================================================================
-- POST /api/v1/sessions/{id}/message returned HTTP 500 INTERNAL_ERROR
-- ("failed to store message", SQLSTATE 42501) because the message handler
-- INSERTs into memory_events without tenant_id, memory_events.tenant_id is
-- UUID NOT NULL with no column default, and FORCE ROW LEVEL SECURITY
-- (dexdat-core migrations/consensus/002_rls.sql) rejects the resulting NULL
-- via the tenant_isolation_memory_events WITH CHECK
-- (tenant_id = current_setting('app.current_tenant_id')::uuid).
--
-- The established multi-tenant pattern in this codebase is the zero-UUID
-- column default (sessions.tenant_id). This migration extends that pattern
-- to EVERY table that carries a tenant_id column without a default, so
-- inserts that omit tenant_id land in the nil tenant instead of failing RLS.
-- The table set is discovered dynamically from information_schema so the bug
-- class is covered by definition (on the stock stack this was 33 tables:
-- every consensus table except sessions) and the migration cannot drift from
-- the live schema.
--
-- Idempotent: SET DEFAULT on an already-defaulted column is a no-op, and a
-- re-run discovers zero tables.
--
-- Safe no-op on schemas without tenant_id columns: sidecar-only Postgres /
-- SQLite deployments (specs/009-deployment.md) use the sidecar's own initial
-- schema, which does not carry tenant_id columns and has no RLS policies —
-- the dynamic discovery simply finds nothing.
--
-- SQLite note: this file is a no-op there — the sidecar SQLite schema has no
-- tenant_id columns, and filterForSQLite strips DO $$ blocks.
-- ============================================================================

DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT table_name
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND column_name = 'tenant_id'
          AND column_default IS NULL
        ORDER BY table_name
    LOOP
        EXECUTE format(
            'ALTER TABLE %I ALTER COLUMN tenant_id SET DEFAULT ''00000000-0000-0000-0000-000000000000''::uuid',
            t
        );
        RAISE NOTICE 'Migration 023: set zero-UUID tenant_id default on %', t;
    END LOOP;
END $$;
