-- ============================================================================
-- Conscience — 031_sqlite_tool_requests_approval_status.sql
-- ============================================================================
-- APPROV-GATE-001 (§4.1 CRITICAL): rebuild tool_requests + tool_results on
-- SQLite so the status CHECK allows 'awaiting_approval'.
--
-- Migration 012 deliberately skipped the constraint rewrite on SQLite ("Go
-- runtime enforces status values") — but the runtime writes exactly the value
-- the CHECK forbids:
--   handleApprovalGated → UPDATE tool_requests SET status='awaiting_approval'
-- fails with `CHECK constraint failed: status IN ('pending','executing',
-- 'completed','failed','timeout') (275)`, the row stays 'pending', and the
-- same poll Loop claims it and executes: the HITL gate is a no-op on the
-- default backend (evidence: docs/testing/evidence/review-consensus-8
-- findings.md §4.1, raw/07-awaiting-approval-check-constraint-probe.txt).
--
-- SQLite has no ALTER TABLE ... DROP CONSTRAINT, so the standard table-rebuild
-- procedure is used: create *_new, copy rows, drop original, rename.
-- Because foreign_keys is ON at runtime (internal/db/sqlite driver pragma)
-- and tool_results references tool_requests(id), the child table must be
-- rebuilt in the same pass — the raw DROP TABLE tool_requests fails with
-- `FOREIGN KEY constraint failed` whenever tool_results holds rows.
-- The six statuses are what the Go code writes across the executor paths
-- (tool_executor.go:350 'awaiting_approval', :222 'executing', :253 'failed',
-- :261 'completed') plus the pre-existing 'pending' and 'timeout'.
--
-- IMPORTANT (holding the shape across the SQLite splitStatements pass):
--   * `-- +goose Up` (added by the harness at migrate.go) means this whole
--     file is Up-only, but it must ALSO stay goose-parseable for the custom
--     runner. It is — the runner reads plain SQL, splitting comments.
--   * `-- +goose Down` mirrors the rebuild only for documentation; the legacy
--     runner's migrationUpSQL() discards it before execution.
--
-- axiom:trace row=APPROV-GATE-001 spec=docs/testing/evidence/review-consensus-8/findings.md#section-4.1-4.3 impl=internal/migrate/migrations/031_sqlite_tool_requests_approval_status.sql
-- ============================================================================

-- Atomicity note: BEGIN/COMMIT are split into standalone statements by the
-- runner (splitStatements) and execute through the pool. At migration time
-- (startup, sessions paused, one idle pooled connection) every statement
-- lands on the SAME connection, so the whole rebuild commits or rolls back
-- as a unit. If a future caller ever runs migrations across DIFFERENT pool
-- connections, BEGIN/COMMIT degrade to no-ops and the swap becomes
-- best-effort — the destructive window (DROP→RENAME) is then only the two
-- RENAME statements, which only fail under lock contention (SQLITE_BUSY).
BEGIN;

-- 1. New tool_requests with the untruncated status CHECK.
CREATE TABLE tool_requests_new (
    id                INTEGER PRIMARY KEY,               -- preserve old values (incl. AUTOINCREMENT semantics)
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration_id      INTEGER NOT NULL,
    tool_name         TEXT NOT NULL,
    parameters        TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','awaiting_approval','executing','completed','failed','timeout')),
    timeout_ms        INT NOT NULL DEFAULT 30000,
    approval_request_id TEXT,                            -- linked by handleApprovalGated (§4.4)
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    executed_at       TEXT,
    completed_at      TEXT
);

-- Port existing rows 1:1. NULL approval_request_id stays NULL (§4.4 links are
-- established going forward; pre-gate rows had none).
INSERT INTO tool_requests_new (id, session_id, iteration_id, tool_name, parameters, status, timeout_ms, approval_request_id, created_at, executed_at, completed_at)
    SELECT id, session_id, iteration_id, tool_name, parameters, status, timeout_ms, approval_request_id, created_at, executed_at, completed_at
    FROM tool_requests;

-- 2. Rebuild the child (tool_results) around the parent swap so the
--    tool_requests DROP succeeds under PRAGMA foreign_keys=ON.
--    Column note: the SELECT below references exit_code/duration_ms (012) and
--    token_count (011). Both are column-ADD migrations that always run BEFORE
--    this one (the Up() runner applies the ladder in version order on every
--    supported path), so the source table carries all four optional columns
--    wherever the runner reaches this migration.
CREATE TABLE tool_results_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id        INTEGER NOT NULL REFERENCES tool_requests_new(id),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    output            TEXT NOT NULL,
    is_error          INTEGER NOT NULL DEFAULT 0,
    error_code        TEXT,
    exit_code         INTEGER,                          -- migration 012
    duration_ms       BIGINT,                           -- migration 012
    token_count       INTEGER,                          -- migration 011
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO tool_results_new (id, request_id, session_id, output, is_error, error_code, exit_code, duration_ms, token_count, created_at)
    SELECT id, request_id, session_id, output, is_error, error_code, exit_code, duration_ms, token_count, created_at
    FROM tool_results;

-- 3. Reader-mirror swap. The *_new copies are one-statement writes, so a
--    concurrent reader never observes a half-populated mirror: both renames
--    are reader-stable (SQLite resolves them against the reader's snapshot).
DROP TABLE tool_results;
DROP TABLE tool_requests;
ALTER TABLE tool_requests_new RENAME TO tool_requests;
ALTER TABLE tool_results_new RENAME TO tool_results;

-- 4. Recreate the indexes the swap dropped (names live in the schema, not the
--    table body; partial WHERE clauses preserved from 001/011/012).
CREATE INDEX idx_tool_req_pending ON tool_requests(session_id, status)
    WHERE status = 'pending';
CREATE INDEX idx_tool_result_request ON tool_results(request_id);
CREATE INDEX idx_tool_requests_approval ON tool_requests(approval_request_id)
    WHERE approval_request_id IS NOT NULL;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================