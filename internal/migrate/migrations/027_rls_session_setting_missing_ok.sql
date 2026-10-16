-- ============================================================================
-- Conscience — 027_rls_session_setting_missing_ok.sql
-- ============================================================================
-- PG-RLS-42704: every RLS policy and session-scoped view that reads
-- current_setting('consensus.session_id')::UUID raises SQLSTATE 42704
-- ("unrecognized configuration parameter") on any connection where the GUC
-- is not set. The harness only sets it with SET LOCAL inside transactions
-- (internal/harness/context.go, internal/db/db.go), but the pool also runs
-- NON-transactional queries — the task-claim UPDATE (internal/harness/
-- executor.go) and the tool_executor approval-gate JOIN over
-- tools_registry/tool_requests — on connections where the GUC was never
-- set, so the policy predicate errors on every poll cycle instead of
-- filtering rows.
--
-- Fix (board row PG-RLS-42704, option A): recreate each affected policy
-- with current_setting('consensus.session_id', true) (missing_ok=true) and
-- an explicit NULL-safe cast:
--
--     NULLIF(current_setting('consensus.session_id', true), '')::UUID
--
-- When the GUC is unset (or set to ''), the predicate evaluates to NULL →
-- no rows are visible (fail closed) instead of erroring. Behaviour is
-- UNCHANGED for connections that set the GUC.
--
-- active_context_view is recreated for the same reason: selecting from it
-- on a GUC-less connection errored identically.
--
-- Migrations already applied are immutable, so the historical files
-- (001_initial_schema.sql, 004_staging_buffer.sql, 014_active_context_
-- view_enhanced.sql) are left untouched; this migration covers the live
-- policies. New dynamic entities created by 006 already use the
-- missing_ok form.
--
-- axiom:trace work_item=PG-RLS-42704
--   spec=specs/005-security.md,specs/003-database.md
--   impl=internal/migrate/migrations/027_rls_session_setting_missing_ok.sql
-- ============================================================================

BEGIN;

-- Helper predicate fragment (inlined per policy below; Postgres policies
-- cannot reference a function indirection without a perf cost, so each
-- policy spells the NULL-safe predicate explicitly).

-- ---------------------------------------------------------------------------
-- 9.2 core-table session-isolation policies (001 §9.2)
-- ---------------------------------------------------------------------------
DROP POLICY IF EXISTS session_isolate_sessions ON sessions;
CREATE POLICY session_isolate_sessions ON sessions
    FOR ALL USING (id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_memory ON memory_events;
CREATE POLICY session_isolate_memory ON memory_events
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_display ON display_modes;
CREATE POLICY session_isolate_display ON display_modes
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_iterations ON iteration_commits;
CREATE POLICY session_isolate_iterations ON iteration_commits
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_pages ON memory_pages;
CREATE POLICY session_isolate_pages ON memory_pages
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_tasks ON tasks;
CREATE POLICY session_isolate_tasks ON tasks
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_tool_req ON tool_requests;
CREATE POLICY session_isolate_tool_req ON tool_requests
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_tool_res ON tool_results;
CREATE POLICY session_isolate_tool_res ON tool_results
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_tool_files ON tool_files;
CREATE POLICY session_isolate_tool_files ON tool_files
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_quarantine ON external_quarantine;
CREATE POLICY session_isolate_quarantine ON external_quarantine
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_billing ON agent_billing;
CREATE POLICY session_isolate_billing ON agent_billing
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

-- compression_queue has no session_id column; isolate via joined
-- memory_events.session_id (001 §9.2).
DROP POLICY IF EXISTS session_isolate_compression ON compression_queue;
CREATE POLICY session_isolate_compression ON compression_queue
    FOR ALL USING (
        EXISTS (
            SELECT 1 FROM memory_events
            WHERE memory_events.id = compression_queue.event_id
              AND memory_events.session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
        )
        OR current_user = 'compression_worker'
        OR current_user = 'alt_mode_role'
    );

DROP POLICY IF EXISTS session_isolate_messages ON agent_messages;
CREATE POLICY session_isolate_messages ON agent_messages
    FOR ALL USING (
        target_session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
        OR sender_session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
    );

DROP POLICY IF EXISTS session_isolate_audit ON audit_logs;
CREATE POLICY session_isolate_audit ON audit_logs
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

DROP POLICY IF EXISTS session_isolate_secrets ON secret_access_audit;
CREATE POLICY session_isolate_secrets ON secret_access_audit
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

-- ---------------------------------------------------------------------------
-- 9.x tools ownership policies (001 §9.x)
-- ---------------------------------------------------------------------------
DROP POLICY IF EXISTS enforce_ownership ON custom_agent_tools;
CREATE POLICY enforce_ownership ON custom_agent_tools
    FOR UPDATE USING (
        NULLIF(current_setting('consensus.session_id', true), '')::UUID = creator_session_id
    );

DROP POLICY IF EXISTS session_isolate_tools ON custom_agent_tools;
CREATE POLICY session_isolate_tools ON custom_agent_tools
    FOR ALL USING (creator_session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

-- ---------------------------------------------------------------------------
-- 004 staging buffer isolation
-- ---------------------------------------------------------------------------
DROP POLICY IF EXISTS staging_session_isolate ON staging_buffer;
CREATE POLICY staging_session_isolate ON staging_buffer
    FOR ALL USING (session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);

-- ---------------------------------------------------------------------------
-- active_context_view (001 §6, replaced by 014) — same NULL-safe GUC read.
-- ---------------------------------------------------------------------------
DROP VIEW IF EXISTS active_context_view;
CREATE OR REPLACE VIEW active_context_view AS
-- Resolve active pointers from the latest iteration commit
WITH active_ids AS (
    SELECT unnest(active_pointers) AS ptr_id
    FROM iteration_commits
    WHERE session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
    ORDER BY iteration_id DESC
    LIMIT 1
),
-- Expand memory pages: direct target IDs (SPEC-002 §5.2)
page_direct_ids AS (
    SELECT unnest(mp.target_ids) AS ptr_id
    FROM memory_pages mp
    WHERE mp.session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
),
-- Expand memory pages: linked page target IDs (single-level nesting)
page_linked_ids AS (
    SELECT unnest(mp2.target_ids) AS ptr_id
    FROM memory_pages mp1
    JOIN memory_pages mp2 ON mp2.id = ANY(mp1.linked_page_ids)
    WHERE mp1.session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
),
-- Union all pointer sources with dedup
all_ids AS (
    SELECT ptr_id FROM active_ids
    UNION
    SELECT ptr_id FROM page_direct_ids
    UNION
    SELECT ptr_id FROM page_linked_ids
),
-- Current session iteration for cache tier calculation
session_iter AS (
    SELECT COALESCE(MAX(iteration), 0) AS current_iter
    FROM sessions
    WHERE id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
),
-- Tool call counts for collapse logic (SPEC-002 §3.5)
tool_call_stats AS (
    SELECT
        COUNT(*) AS total_calls,
        COUNT(*) FILTER (WHERE type = 'tool_call') > 10 AS should_collapse
    FROM memory_events
    WHERE session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
),
-- Rank recent tool calls to keep the last 2 non-collapsed
tool_call_ranking AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            ORDER BY iteration_created DESC, id DESC
        ) AS rn
    FROM memory_events
    WHERE type = 'tool_call'
      AND session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
)
SELECT DISTINCT ON (me.id)
    me.id,
    me.session_id,
    me.iteration_created,
    me.type,
    me.content                                 AS raw_content,
    me.summary_text,
    COALESCE(dm.mode, 'full')                  AS display_mode,
    CASE
        WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
            THEN me.summary_text
        WHEN COALESCE(dm.mode, 'full') = 'hidden'
            THEN NULL
        ELSE me.content
    END                                        AS rendered_text,
    CASE
        WHEN me.type = 'tool_call'
             AND (SELECT should_collapse FROM tool_call_stats)
             AND COALESCE((SELECT rn FROM tool_call_ranking WHERE id = me.id), 0) > 2
            THEN 'collapsed'
        ELSE 'full'
    END                                        AS collapse_status,
    CASE
        WHEN me.type IN ('system', 'header')
            THEN 1
        WHEN me.iteration_created <= (SELECT current_iter FROM session_iter) - 5
            THEN 2
        ELSE 3
    END                                        AS cache_tier,
    CASE
        WHEN ai.ptr_id IS NOT NULL
            THEN 'page'
        ELSE 'direct'
    END                                        AS source_type
FROM memory_events me
LEFT JOIN display_modes dm ON dm.memory_id = me.id
LEFT JOIN all_ids ai ON me.id = ai.ptr_id
WHERE COALESCE(dm.mode, 'full') != 'hidden'
    AND me.session_id = NULLIF(current_setting('consensus.session_id', true), '')::UUID
ORDER BY me.id, me.iteration_created;

COMMIT;
