-- ============================================================================
-- Conscience — 011_tool_sandbox.sql
-- ============================================================================
-- Schema additions for WI-005: External Tool Execution Sandbox.
--
-- Adds columns to tool_results for tracking subprocess exit codes and duration,
-- adds an approval request linkage column to tool_requests for HITL gating,
-- and adds an 'awaiting_approval' status to tool_requests.
--
-- axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=phase-1/task-1
-- ============================================================================

-- 1. Add exit_code and duration_ms to tool_results.
--    exit_code: subprocess exit code (NULL for non-subprocess handlers, 0 = success, non-zero = failure)
--    duration_ms: wall clock execution time in milliseconds
ALTER TABLE tool_results ADD COLUMN IF NOT EXISTS exit_code INT;
ALTER TABLE tool_results ADD COLUMN IF NOT EXISTS duration_ms BIGINT;

-- 2. Add approval_request_id to tool_requests for HITL gating.
--    When a tool requires approval, the tool_request is linked to the
--    approval_request so the executor can check status.
ALTER TABLE tool_requests ADD COLUMN IF NOT EXISTS approval_request_id UUID REFERENCES approval_requests(id);

-- 3. Add 'awaiting_approval' to the tool_requests status CHECK constraint.
--    PostgreSQL path: drop and recreate the constraint.
ALTER TABLE tool_requests DROP CONSTRAINT IF EXISTS tool_requests_status_check;
ALTER TABLE tool_requests ADD CONSTRAINT tool_requests_status_check
    CHECK (status IN ('pending', 'awaiting_approval', 'executing', 'completed', 'failed', 'timeout'));

-- 4. Create an index on approval_request_id for efficient lookups.
CREATE INDEX IF NOT EXISTS idx_tool_requests_approval ON tool_requests(approval_request_id)
    WHERE approval_request_id IS NOT NULL;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary of changes:
--   2 new columns on tool_results: exit_code, duration_ms
--   1 new column on tool_requests: approval_request_id (FK → approval_requests)
--   1 new status in tool_requests CHECK: 'awaiting_approval'
--   1 new index: idx_tool_requests_approval
--
-- axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/014-hitl-interrupt-state.md plan=phase-1/task-1
-- ============================================================================
