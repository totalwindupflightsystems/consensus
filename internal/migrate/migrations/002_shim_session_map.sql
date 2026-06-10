-- ============================================================================
-- Conscience — 002_shim_session_map.sql
-- ============================================================================
-- Maps external protocol session IDs to Conscience session IDs.
-- Used by protocol shims (opencode, pi-agent) to track which external session
-- corresponds to which Conscience session.
--
-- axiom:trace work_item=interfaces-api-cli-01
--   spec=specs/017-ui-adapter-layer.md,specs/003-database.md
--   plan=phase-6/task-6-1/step-6-1-1
-- ============================================================================

BEGIN;

-- 1. shim_session_map — Maps external shim session IDs to Conscience sessions.
-- SPEC-017 §8.
CREATE TABLE IF NOT EXISTS shim_session_map (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shim_type       TEXT NOT NULL CHECK (shim_type IN ('opencode', 'pi-agent', 'mcp')),
    external_id     TEXT NOT NULL,       -- The shim's external session ID
    session_id      UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure one external_id per shim_type (opencode sends the same session_id for reconnects).
CREATE UNIQUE INDEX IF NOT EXISTS idx_shim_session_map
    ON shim_session_map(shim_type, external_id);

-- Lookup by Conscience session_id for reverse mapping.
CREATE INDEX IF NOT EXISTS idx_shim_session_map_session
    ON shim_session_map(session_id);

-- 2. api_keys — API key management table (SPEC-015 §2.2, SPEC-017 §8).
-- These keys authenticate external callers. The shim uses this table to resolve
-- Basic Auth passwords into session-scoped or admin API keys.
CREATE TABLE IF NOT EXISTS api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash        TEXT NOT NULL UNIQUE, -- SHA-256 of the actual key
    key_prefix      TEXT NOT NULL,        -- First 8 chars for index-assisted lookup
    scope           TEXT NOT NULL CHECK (scope IN ('admin', 'session', 'readonly', 'webhook')),
    session_id      UUID REFERENCES sessions(id) ON DELETE CASCADE,  -- NULL for admin/readonly/webhook
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);
CREATE INDEX IF NOT EXISTS idx_api_keys_session ON api_keys(session_id);

-- 3. api_rate_limits — Per-key rate limiting counters (SPEC-015 §7.1).
CREATE TABLE IF NOT EXISTS api_rate_limits (
    key_prefix       TEXT PRIMARY KEY,
    requests_count   INT NOT NULL DEFAULT 0,
    window_start     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary of objects created:
--   3 tables: shim_session_map, api_keys, api_rate_limits
--   4 indexes
--
-- axiom:trace work_item=interfaces-api-cli-01
--   spec=specs/017-ui-adapter-layer.md,specs/015-api-and-mcp.md
--   plan=phase-6/task-6-1/step-6-1-1
-- ============================================================================
