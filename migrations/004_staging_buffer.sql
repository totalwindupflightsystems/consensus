-- ============================================================================
-- Conscience — 004_staging_buffer.sql
-- ============================================================================
-- SPEC-020 interactive transaction staging buffer.
-- This table is the agent's transaction scratchpad — every staged command,
-- its execution state, and results are persisted here so that crash recovery
-- and multi-turn planning work reliably.
--
-- axiom:trace work_item=spec-020-hardening-01
--   spec=specs/020-multi-turn-planning.md
--   plan=phase-1/task-1/step-1
--   evidence=.memory-bank/work-items/spec-020-hardening-01/verification.md
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS staging_buffer (
    id              SERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration       BIGINT NOT NULL,
    turn            INT NOT NULL DEFAULT 1,
    seq             INT NOT NULL DEFAULT 0,
    cmd_type        TEXT NOT NULL DEFAULT 'sql'
                    CHECK (cmd_type IN (
                        'sql', 'file_write', 'file_edit', 'file_delete',
                        'memory_write', 'tool_call_ref'
                    )),
    payload         JSONB NOT NULL DEFAULT '{}',
    description     TEXT NOT NULL DEFAULT '',
    executed        BOOLEAN NOT NULL DEFAULT false,
    result          JSONB,
    status          TEXT NOT NULL DEFAULT 'staged'
                    CHECK (status IN (
                        'staged', 'executed', 'committed',
                        'rolled_back', 'failed'
                    )),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_staging_session ON staging_buffer(session_id, iteration);
CREATE INDEX IF NOT EXISTS idx_staging_status ON staging_buffer(session_id)
    WHERE status IN ('staged', 'executed');

-- Agent role access: full CRUD within session scope (RLS enforces session isolation)
GRANT SELECT, INSERT, UPDATE ON staging_buffer TO agent_role;

-- Row-Level Security: agents can only see their own staging buffer
ALTER TABLE staging_buffer ENABLE ROW LEVEL SECURITY;

CREATE POLICY staging_session_isolate ON staging_buffer
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

COMMIT;
