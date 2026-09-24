-- ============================================================================
-- Conscience — 013_active_context_view_enhanced.sql
-- ============================================================================
-- Enhanced active_context_view with tool call collapse, cache tier ordering,
-- page resolution, and DISTINCT ON deduplication.
--
-- axiom:trace work_item=WI-007
--   spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md
--   plan=phase-1/migration
-- ============================================================================

BEGIN;

-- ============================================================================
-- Drop old view to recreate with enhancements
-- ============================================================================
DROP VIEW IF EXISTS active_context_view;

-- ============================================================================
-- Enhanced active_context_view
--
-- Features added beyond the original migration 001 definition:
--   1. Page resolution: UNIONs active_ids (from iteration_commits) with
--      expanded memory_pages (direct target_ids + linked_page_ids).
--   2. DISTINCT ON deduplication: when multiple sources reference the same
--      memory event ID, it appears only once (SPEC-002 §3.6).
--   3. collapse_status: window-function-based tool call collapse when
--      more than 10 tool calls exist in the session (SPEC-002 §3.5).
--   4. cache_tier: numerical ordering tier for prompt caching optimization
--      (SPEC-003 §6.2): Layer 1=static, Layer 2=ledger, Layer 3=dynamic.
--
-- The VIEW is session-isolated via current_setting('conscience.session_id').
-- SQLite does not support this VIEW (stripped by filterForSQLite) — the
-- Go harness provides equivalent logic for the SQLite backend.
-- ============================================================================
CREATE OR REPLACE VIEW active_context_view AS
-- Resolve active pointers from the latest iteration commit
WITH active_ids AS (
    SELECT unnest(active_pointers) AS ptr_id
    FROM iteration_commits
    WHERE session_id = current_setting('conscience.session_id')::UUID
    ORDER BY iteration_id DESC
    LIMIT 1
),
-- Expand memory pages: direct target IDs (SPEC-002 §5.2)
page_direct_ids AS (
    SELECT unnest(mp.target_ids) AS ptr_id
    FROM memory_pages mp
    WHERE mp.session_id = current_setting('conscience.session_id')::UUID
),
-- Expand memory pages: linked page target IDs (single-level nesting)
page_linked_ids AS (
    SELECT unnest(mp2.target_ids) AS ptr_id
    FROM memory_pages mp1
    JOIN memory_pages mp2 ON mp2.id = ANY(mp1.linked_page_ids)
    WHERE mp1.session_id = current_setting('conscience.session_id')::UUID
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
    WHERE id = current_setting('conscience.session_id')::UUID
),
-- Tool call counts for collapse logic (SPEC-002 §3.5)
tool_call_stats AS (
    SELECT
        COUNT(*) AS total_calls,
        COUNT(*) FILTER (WHERE type = 'tool_call') > 10 AS should_collapse
    FROM memory_events
    WHERE session_id = current_setting('conscience.session_id')::UUID
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
      AND session_id = current_setting('conscience.session_id')::UUID
)
SELECT DISTINCT ON (me.id)
    me.id,
    me.session_id,
    me.iteration_created,
    me.type,
    me.content                                 AS raw_content,
    me.summary_text,
    COALESCE(dm.mode, 'full')                  AS display_mode,
    -- Rendered text: compressed→summary_text, hidden→NULL, full→content
    CASE
        WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
            THEN me.summary_text
        WHEN COALESCE(dm.mode, 'full') = 'hidden'
            THEN NULL
        ELSE me.content
    END                                        AS rendered_text,
    -- Tool call collapse status (SPEC-002 §3.5):
    -- If >10 tool calls in session, only show last 2; mark rest 'collapsed'
    CASE
        WHEN me.type = 'tool_call'
             AND (SELECT should_collapse FROM tool_call_stats)
             AND COALESCE((SELECT rn FROM tool_call_ranking WHERE id = me.id), 0) > 2
            THEN 'collapsed'
        ELSE 'full'
    END                                        AS collapse_status,
    -- Cache tier for prompt caching optimization (SPEC-003 §6.2):
    --   1 = static system (headers, system messages) — ALWAYS cached
    --   2 = immutable ledger (events >5 iterations old) — HEAVILY cached
    --   3 = dynamic scratchpad (recent events, tool results) — rarely cached
    CASE
        WHEN me.type IN ('system', 'header')
            THEN 1
        WHEN me.iteration_created <= (SELECT current_iter FROM session_iter) - 5
            THEN 2
        ELSE 3
    END                                        AS cache_tier,
    -- Pointer source annotation: which resolution path included this event
    CASE
        WHEN ai.ptr_id IS NOT NULL
            THEN 'page'
        ELSE 'direct'
    END                                        AS source_type
FROM memory_events me
LEFT JOIN display_modes dm ON dm.memory_id = me.id
LEFT JOIN all_ids ai ON me.id = ai.ptr_id
WHERE COALESCE(dm.mode, 'full') != 'hidden'
    AND me.session_id = current_setting('conscience.session_id')::UUID
ORDER BY me.id, me.iteration_created;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Changes from original active_context_view:
--   - Added page resolution (all_ids CTE: active_ids + page_direct_ids + page_linked_ids)
--   - Added collapse_status column for tool call window-function rules
--   - Added cache_tier column for prompt caching optimization
--   - Added source_type column annotation
--   - Added raw_content column for full content access when needed
--   - ID columns now include session_id and summary_text for richer querying
--
-- axiom:trace work_item=WI-007
--   spec=specs/001-architecture.md,specs/002-memory.md,specs/003-database.md,specs/005-security.md
--   plan=phase-1/migration
-- ============================================================================
