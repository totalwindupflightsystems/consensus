-- ============================================================================
-- Conscience — 008_hitl_tables.sql
-- ============================================================================
-- Human-in-the-Loop (HITL) interrupt state tables (SPEC-014).
-- Remediation: gap found in idle spec sweep-017 — these tables only existed
-- as test-only DDL in hitl_test.go. This migration installs them as proper
-- production schema.
--
-- axiom:trace work_item=spec-014-hardening-01
--   spec=specs/014-hitl-interrupt-state.md
--   plan=phase-1/task-1-1/step-1-1-1
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. approval_requests — Pending human approval decisions
-- ============================================================================
-- Six request types per SPEC-014 §2:
--   tool_execution, destructive_action, budget_override,
--   schema_change, sub_agent_spawn, custom
CREATE TABLE IF NOT EXISTS approval_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration       BIGINT NOT NULL,
    request_type    TEXT NOT NULL CHECK (request_type IN (
                        'tool_execution', 'destructive_action', 'budget_override',
                        'schema_change', 'sub_agent_spawn', 'custom'
                    )),
    description     TEXT NOT NULL,
    risk_level      TEXT NOT NULL DEFAULT 'medium'
                    CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    context         JSONB NOT NULL DEFAULT '{}',
    target_tool     TEXT,
    target_sql      TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'modified')),
    reviewer_id     TEXT,
    review_notes    TEXT,
    modified_sql    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at     TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_approvals_pending ON approval_requests(session_id, status)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_approvals_session ON approval_requests(session_id, created_at DESC);

-- ============================================================================
-- 2. hitl_configuration — Per-session and global HITL settings
-- ============================================================================
-- Scope precedence: session overrides global (SPEC-014 §3.2).
-- When scope='session', session_id is required.
CREATE TABLE IF NOT EXISTS hitl_configuration (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope           TEXT NOT NULL CHECK (scope IN ('global', 'session')),
    session_id      UUID REFERENCES sessions(id),
    auto_pause_on_error_threshold INT NOT NULL DEFAULT 3,
    require_approval_for_destructive BOOLEAN NOT NULL DEFAULT true,
    require_approval_for_schema_changes BOOLEAN NOT NULL DEFAULT true,
    require_approval_for_external_tools BOOLEAN NOT NULL DEFAULT false,
    approval_timeout_minutes INT NOT NULL DEFAULT 60,
    notify_on_pause BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_hitl_config_scope ON hitl_configuration(scope);
CREATE INDEX IF NOT EXISTS idx_hitl_config_session ON hitl_configuration(session_id)
    WHERE scope = 'session';

-- ============================================================================
-- 3. notification_log — Record of all HITL notifications
-- ============================================================================
CREATE TABLE IF NOT EXISTS notification_log (
    id              BIGSERIAL PRIMARY KEY,
    approval_id     UUID NOT NULL REFERENCES approval_requests(id),
    channel         TEXT NOT NULL CHECK (channel IN ('dashboard', 'email', 'slack', 'webhook')),
    recipient       TEXT NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered       BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_notif_log_approval ON notification_log(approval_id);

-- ============================================================================
-- 4. Insert default global HITL configuration
-- ============================================================================
-- Only insert if no global config exists yet (idempotent).
INSERT INTO hitl_configuration (scope, auto_pause_on_error_threshold, require_approval_for_destructive, require_approval_for_schema_changes, require_approval_for_external_tools, approval_timeout_minutes, notify_on_pause)
SELECT 'global', 3, true, true, false, 60, true
WHERE NOT EXISTS (
    SELECT 1 FROM hitl_configuration WHERE scope = 'global' LIMIT 1
);

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary of objects created:
--   3 tables: approval_requests, hitl_configuration, notification_log
--   5 indexes
--   1 default global config row
--
-- axiom:trace work_item=spec-014-hardening-01
--   spec=specs/014-hitl-interrupt-state.md
--   plan=phase-1/task-1-1/step-1-1-1
-- ============================================================================
