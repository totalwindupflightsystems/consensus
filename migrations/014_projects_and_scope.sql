-- ============================================================================
-- Conscience — 014_projects_and_scope.sql
-- ============================================================================
-- RBAC Scope Model: projects table + project_id on sessions/tasks (SPEC-004 §RBAC).
-- SSE Event Stream: SQL triggers for session/approval notifications (SPEC-015 §4).
--
-- Part A (CS-GAP-009): Scope hierarchy — Global (NULL) > Project > Sub-Agent.
-- Part B (CS-GAP-013): LISTEN/NOTIFY triggers for real-time events.
--
-- axiom:trace work_item=WI-008-WI-009
--   spec=specs/004-subagents.md,specs/005-security.md,specs/015-api-and-mcp.md
--   plan=phase-1/task-1-1
-- ============================================================================

BEGIN;

-- ============================================================================
-- PART A — RBAC Scope Model (CS-GAP-009)
-- ============================================================================

-- 1. projects — Named project boundaries for agent scoping.
-- SPEC-004 §RBAC: Global > Project > Sub-Agent hierarchy.
CREATE TABLE IF NOT EXISTS projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Add project_id to sessions.
-- NULL project_id = Global scope (agent can access across projects).
-- Non-NULL = Project scope (agent confined to that project).
-- Note: SQLite does NOT support IF NOT EXISTS in ALTER TABLE ADD COLUMN.
-- This is a new migration so the column won't exist yet.
ALTER TABLE sessions ADD COLUMN project_id UUID REFERENCES projects(id);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id)
    WHERE project_id IS NOT NULL;

-- 3. Add project_id to tasks for scope propagation.
-- Sub-agents inherit their parent session's project_id.
ALTER TABLE tasks ADD COLUMN project_id UUID REFERENCES projects(id);

CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id)
    WHERE project_id IS NOT NULL;

-- ============================================================================
-- PART B — SSE Event Stream Triggers (CS-GAP-013)
-- ============================================================================
-- Postgres LISTEN/NOTIFY triggers for real-time event streaming.
-- These feed the NotificationListener which bridges to the Go EventBus.
-- SPEC-015 §4.1.

-- 4. Notify on session status changes.
CREATE OR REPLACE FUNCTION notify_session_change()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify(
        'session_events',
        json_build_object(
            'session_id', NEW.id,
            'status', NEW.status,
            'iteration', NEW.iteration,
            'project_id', NEW.project_id,
            'timestamp', now()
        )::text
    );
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS session_status_notify ON sessions;
CREATE TRIGGER session_status_notify
    AFTER UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION notify_session_change();

-- Also NOTIFY on INSERT for newly created sessions.
DROP TRIGGER IF EXISTS session_create_notify ON sessions;
CREATE TRIGGER session_create_notify
    AFTER INSERT ON sessions
    FOR EACH ROW EXECUTE FUNCTION notify_session_change();

-- 5. Notify on approval requests (SPEC-015 §4.1, SPEC-014).
CREATE OR REPLACE FUNCTION notify_approval_request()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify(
        'approval_events',
        json_build_object(
            'approval_id', NEW.id,
            'session_id', NEW.session_id,
            'request_type', NEW.request_type,
            'risk_level', NEW.risk_level,
            'description', NEW.description,
            'status', NEW.status,
            'timestamp', now()
        )::text
    );
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS approval_request_notify ON approval_requests;
CREATE TRIGGER approval_request_notify
    AFTER INSERT ON approval_requests
    FOR EACH ROW EXECUTE FUNCTION notify_approval_request();

-- Also notify on status changes (approved/rejected/expired).
DROP TRIGGER IF EXISTS approval_status_notify ON approval_requests;
CREATE TRIGGER approval_status_notify
    AFTER UPDATE ON approval_requests
    FOR EACH ROW EXECUTE FUNCTION notify_approval_request();

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary:
--   1 new table: projects
--   2 new columns: sessions.project_id, tasks.project_id
--   2 new indexes: idx_sessions_project, idx_tasks_project
--   2 new functions: notify_session_change, notify_approval_request
--   4 new triggers: session_status_notify, session_create_notify,
--                   approval_request_notify, approval_status_notify
--
-- axiom:trace work_item=WI-008-WI-009
--   spec=specs/004-subagents.md,specs/005-security.md,specs/015-api-and-mcp.md
--   plan=phase-1/task-1-1
-- ============================================================================
