-- ============================================================================
-- Conscience — 001_initial_schema.sql
-- ============================================================================
-- Database-native cognitive architecture foundation.
-- The database IS the runtime: all state, memory, security, and event handling
-- lives here. No filesystem state, no Redis, no message broker.
--
-- axiom:trace work_item=schema-memory-01
--   spec=specs/002-memory.md,specs/003-database.md,specs/005-security.md,specs/007-json-schema.md,specs/011-canonical-definitions.md
--   plan=phase-1 task-1 step-1
--   evidence=.memory-bank/work-items/schema-memory-01/verification.md
--
-- Coverage: AC-MEM-001 through AC-MEM-016
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — EXTENSIONS & PREAMBLE
-- axiom:trace work_item=schema-memory-01 plan=phase-1/task-1/step-1-1-1
-- ============================================================================

-- UUID generation (core Postgres, but explicit)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Vector embeddings for semantic search (§7.3 / SPEC-002 §7.3, SPEC-003 §7.3)
CREATE EXTENSION IF NOT EXISTS vector;

-- JSON Schema validation for dynamic columns (§4 / SPEC-003 §4.2)
-- Gracefully degrades when the extension is not installed (e.g., managed cloud Postgres)
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_jsonschema;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_jsonschema not available — JSON Schema validation will be application-layer';
END;
$$;

-- ============================================================================
-- SECTION 2 — CORE IDENTITY & SYSTEM TABLES
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md,specs/011-canonical-definitions.md plan=phase-1/task-1-1/step-1-1-1
-- ============================================================================

-- 2.1 sessions — Root identity for RLS isolation, billing, and lifecycle.
-- Canonical status CHECK from SPEC-011 §1 (10 states, lowercase).
CREATE TABLE sessions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id         UUID REFERENCES sessions(id),
    agent_name        TEXT NOT NULL,
    model_id          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'booting'
                      CHECK (status IN (
                          'booting',       -- Session created, context not yet loaded
                          'idle',          -- Waiting for work (no active iteration)
                          'planning',      -- Interactive transaction open — staging, executing, inspecting (SPEC-020)
                          'thinking',      -- Harness sent context to LLM, awaiting response
                          'tool_exec',     -- LLM requested tools, harness executing externally
                          'executing',     -- Agent called commit — applying final staged changes (SPEC-020)
                          'waiting_sub',   -- Sub-agent(s) spawned, parent paused
                          'paused',        -- Human-in-the-loop pause (SPEC-014 HITL)
                          'completed',     -- Goal achieved, session ended
                          'failed'         -- Unrecoverable error or circuit breaker tripped
                      )),
    goal              TEXT,
    context_budget    INT NOT NULL DEFAULT 128000,
    tokens_used_in    BIGINT NOT NULL DEFAULT 0,
    tokens_used_out   BIGINT NOT NULL DEFAULT 0,
    iteration         BIGINT NOT NULL DEFAULT 0,
    heartbeat_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    planning_max_turns INT NOT NULL DEFAULT 10,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ
);

CREATE INDEX idx_sessions_parent ON sessions(parent_id);
CREATE INDEX idx_sessions_status ON sessions(status)
    WHERE status IN ('idle', 'thinking', 'planning', 'tool_exec', 'executing', 'waiting_sub');

-- 2.2 iteration_commits — Snapshots of the active pointer set per iteration.
-- Merged: SPEC-006 iteration_snapshots columns (llm_response, sql_executed, rows_affected).
-- Canonical from SPEC-011 §11 and SPEC-003 §2.3.
CREATE TABLE iteration_commits (
    iteration_id      BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    active_pointers   BIGINT[] NOT NULL,
    display_rules     JSONB NOT NULL DEFAULT '{}',
    llm_response      JSONB,
    sql_executed      TEXT[],
    rows_affected     INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iteration_session ON iteration_commits(session_id, iteration_id DESC);
CREATE UNIQUE INDEX idx_session_iteration ON iteration_commits(session_id, iteration_id);

-- 2.3 audit_logs — Structured audit trail per iteration.
-- Canonical from SPEC-011 §12.3 and SPEC-007 §Output Schema.
CREATE TABLE audit_logs (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration         BIGINT NOT NULL,
    monologue         TEXT,
    sql_executed      TEXT[],
    result            TEXT NOT NULL CHECK (result IN ('committed', 'rolled_back')),
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_session ON audit_logs(session_id, iteration);

-- 2.4 system_settings — Key-value system configuration.
-- Canonical from SPEC-011 §12.2.
CREATE TABLE system_settings (
    key               TEXT PRIMARY KEY,
    value             TEXT NOT NULL,
    description       TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2.5 agent_messages — Parent→child inter-session communication.
-- Canonical from SPEC-011 §12.1.
CREATE TABLE agent_messages (
    id                BIGSERIAL PRIMARY KEY,
    target_session_id UUID NOT NULL REFERENCES sessions(id),
    sender_session_id UUID NOT NULL REFERENCES sessions(id),
    payload           JSONB NOT NULL,
    read              BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_target ON agent_messages(target_session_id, read);

-- ============================================================================
-- SECTION 3 — MEMORY SUBSYSTEM TABLES
-- axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/003-database.md,specs/011-canonical-definitions.md plan=phase-1/task-1-1/step-1-1-2
-- ============================================================================

-- 3.1 memory_events — Immutable append-only ledger.
-- NO display_mode column — display state is in the separate display_modes table.
-- Canonical type CHECK from SPEC-011 §3.5 (8 types, lowercase).
-- Schema: SPEC-003 §2.2, SPEC-002 §2.2.
CREATE TABLE memory_events (
    id                BIGSERIAL PRIMARY KEY,
    type              TEXT NOT NULL
                      CHECK (type IN (
                          'header',             -- Session start / context header
                          'text_block',         -- Agent text output
                          'tool_call',          -- Outbound tool invocation
                          'tool_result',        -- Tool execution result
                          'thinking',           -- Agent internal monologue / chain-of-thought
                          'system',             -- System-injected message (error, notification, config change)
                          'inherited_pointer',  -- Forked compressed pointer from parent (SPEC-004 §Memory Forking)
                          'user_message'        -- Human input via API/message endpoint (SPEC-015 §3.1)
                      )),
    content           TEXT NOT NULL,
    summary_text      TEXT,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration_created BIGINT NOT NULL,
    linked_memory_pages UUID[],
    embedding         vector(1536),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_memory_session ON memory_events(session_id, iteration_created);
CREATE INDEX idx_memory_embedding ON memory_events
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- 3.2 display_modes — Display state for each memory event, stored separately.
-- This preserves the append-only invariant on memory_events.
-- Canonical from SPEC-011 §3.2 and SPEC-003 §2.2a.
CREATE TABLE display_modes (
    memory_id         BIGINT NOT NULL REFERENCES memory_events(id) ON DELETE CASCADE,
    mode              TEXT NOT NULL DEFAULT 'full'
                      CHECK (mode IN ('full', 'compressed', 'hidden')),
    set_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    set_by_iteration  BIGINT NOT NULL,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    PRIMARY KEY (memory_id)
);

-- 3.3 memory_pages — Named groups of memory IDs for token-efficient referencing.
-- Schema: SPEC-003 §2.4, SPEC-002 §5.
CREATE TABLE memory_pages (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    target_ids        BIGINT[] NOT NULL,
    linked_page_ids   BIGINT[] NOT NULL DEFAULT '{}',
    session_id        UUID NOT NULL REFERENCES sessions(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(name, session_id)
);

-- 3.4 compression_queue — Background compression work queue.
-- Schema: SPEC-003 §9.4, SPEC-002 §8.4.
CREATE TABLE compression_queue (
    id                BIGSERIAL PRIMARY KEY,
    event_id          BIGINT NOT NULL REFERENCES memory_events(id),
    current_tier      INT NOT NULL DEFAULT 1,
    next_tier         INT NOT NULL DEFAULT 2,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    attempts          INT NOT NULL DEFAULT 0,
    max_attempts      INT NOT NULL DEFAULT 3,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at      TIMESTAMPTZ
);

CREATE INDEX idx_compression_pending ON compression_queue(status)
    WHERE status = 'pending';

-- 3.5 model_registry — Model catalog for 2D routing matrix (tier × category).
-- Schema: SPEC-003 §2.14, SPEC-002 §9.
CREATE TABLE model_registry (
    id                BIGSERIAL PRIMARY KEY,
    model_id          TEXT NOT NULL UNIQUE,
    tier              INT NOT NULL CHECK (tier IN (1, 2, 3)),
    max_context       INT NOT NULL,
    cost_per_m_in     NUMERIC(8,4) NOT NULL,
    cost_per_m_out    NUMERIC(8,4) NOT NULL,
    classifier_tags   TEXT[] DEFAULT '{}',
    enabled           BOOLEAN NOT NULL DEFAULT true
);

-- model_registry created before sessions uses it for FK, so add it post-hoc.
-- SPEC-003 §2.1 requires sessions.model_id → model_registry(model_id).
ALTER TABLE sessions
ADD CONSTRAINT fk_sessions_model
 FOREIGN KEY (model_id) REFERENCES model_registry(model_id);

-- ============================================================================
-- SECTION 4 — TASK, TOOL, AND BILLING TABLES
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md,specs/011-canonical-definitions.md plan=phase-1/task-1-1/step-1-1-3
-- ============================================================================

-- 4.1 tasks — Unified work queue with prerequisite enforcement.
-- Canonical status CHECK from SPEC-011 §2 (7 states, lowercase).
CREATE TABLE tasks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        UUID NOT NULL REFERENCES sessions(id),
    parent_task_id    UUID REFERENCES tasks(id),
    title             TEXT NOT NULL,
    description       TEXT,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN (
                          'pending',       -- Created, not yet claimed
                          'claimed',       -- Agent locked the task
                          'in_progress',   -- Agent actively working
                          'reviewed',      -- Output reviewed (required before published)
                          'published',     -- Output finalized, available to consumers
                          'failed',        -- Unrecoverable error
                          'cancelled'      -- Deliberately cancelled
                      )),
    priority          INT NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    locked_by_agent   UUID REFERENCES sessions(id),
    prerequisite_ids  UUID[] NOT NULL DEFAULT '{}',
    result_memory_id  BIGINT REFERENCES memory_events(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
);

CREATE INDEX idx_tasks_session_status ON tasks(session_id, status);
CREATE INDEX idx_tasks_locked_by ON tasks(locked_by_agent)
    WHERE locked_by_agent IS NOT NULL;

-- 4.2 tool_requests — Outbound tool invocations requested by the agent.
-- Canonical status from SPEC-011 §4.1/§14.
CREATE TABLE tool_requests (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration_id      BIGINT NOT NULL,
    tool_name         TEXT NOT NULL,
    parameters        JSONB NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'timeout')),
    timeout_ms        INT NOT NULL DEFAULT 30000,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_at       TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
);

CREATE INDEX idx_tool_req_pending ON tool_requests(session_id, status)
    WHERE status = 'pending';

-- 4.3 tool_results — Responses from external tool runner, written by harness.
CREATE TABLE tool_results (
    id                BIGSERIAL PRIMARY KEY,
    request_id        BIGINT NOT NULL REFERENCES tool_requests(id),
    session_id        UUID NOT NULL REFERENCES sessions(id),
    output            TEXT NOT NULL,
    is_error          BOOLEAN NOT NULL DEFAULT false,
    error_code        TEXT,
    token_count       INT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tool_result_request ON tool_results(request_id);

-- 4.4 tools_registry — Executable capabilities (SQL functions, HTTP endpoints, Go handlers, subprocesses).
-- Canonical from SPEC-011 §4.2.1 (merged SPEC-003 operational fields + SPEC-010 governance).
CREATE TABLE tools_registry (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT NOT NULL,
    hemisphere        TEXT NOT NULL CHECK (hemisphere IN ('internal', 'external')),
    parameter_schema  JSONB NOT NULL DEFAULT '{}',
    handler_type      TEXT NOT NULL CHECK (handler_type IN (
                          'sql_function', 'http_endpoint', 'go_native', 'subprocess'
                      )),
    handler_ref       TEXT NOT NULL,
    owner_session_id  UUID REFERENCES sessions(id),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'testing', 'deprecated', 'disabled')),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    rate_limit_per_min INT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.5 skills_registry — Knowledge bundles (instructions, workflows, progressive disclosure).
-- Canonical from SPEC-011 §4.2.2.
CREATE TABLE skills_registry (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    metadata          JSONB NOT NULL,
    instructions      TEXT NOT NULL,
    linked_tool_ids   UUID[] NOT NULL DEFAULT '{}',
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.6 agent_billing — Cost tracking per session per iteration.
-- Canonical from SPEC-011 §6.2 (cache tokens, NUMERIC(12,6) cost, category CHECK).
CREATE TABLE agent_billing (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration         BIGINT NOT NULL,
    model_id          TEXT NOT NULL,
    category          TEXT NOT NULL CHECK (category IN (
                          'cognition', 'compression', 'embedding', 'tool_call'
                      )),
    prompt_tokens     BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    cost_usd          NUMERIC(12,6) NOT NULL DEFAULT 0,
    recorded_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_billing_session ON agent_billing(session_id);
CREATE INDEX idx_billing_recorded ON agent_billing(recorded_at);

-- 4.7 workflows — Reusable multi-step agent configurations.
-- Schema: SPEC-003 §2.10.
CREATE TABLE workflows (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT,
    steps             JSONB NOT NULL DEFAULT '[]',
    trigger_event     TEXT,
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.8 custom_agent_tools — Agent-authored tools stored in DB, not filesystem.
-- Canonical from SPEC-011 §5.2 (UNIQUE(name) globally, language includes 'go').
CREATE TABLE custom_agent_tools (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_session_id  UUID NOT NULL REFERENCES sessions(id),
    name                TEXT NOT NULL,
    language            TEXT NOT NULL CHECK (language IN (
                            'javascript', 'typescript', 'sql', 'python', 'go'
                        )),
    source_code         TEXT NOT NULL,
    parameter_schema    JSONB NOT NULL DEFAULT '{}',
    approved            BOOLEAN NOT NULL DEFAULT false,
    status              TEXT NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'testing', 'active', 'deprecated')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(name)
);

-- 4.9 tool_files — File-like objects stored in DB (binary data base64-encoded).
CREATE TABLE tool_files (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        UUID NOT NULL REFERENCES sessions(id),
    name              TEXT NOT NULL,
    mime_type         TEXT NOT NULL,
    content_b64       TEXT NOT NULL,
    size_bytes        BIGINT NOT NULL,
    memory_event_id   BIGINT REFERENCES memory_events(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tool_files_session ON tool_files(session_id);

-- ============================================================================
-- SECTION 5 — COGNITIVE FIREWALL & SECURITY TABLES
-- axiom:trace work_item=schema-memory-01 spec=specs/005-security.md,specs/003-database.md plan=phase-2/task-2-3/step-2-3-1
-- ============================================================================

-- 5.1 external_quarantine — Staging area for untrusted external data.
-- All external data must pass validation here before entering trusted memory.
-- Cognitive firewall specification: SPEC-005 §Cognitive Firewall.
CREATE TABLE external_quarantine (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    source_type       TEXT NOT NULL CHECK (source_type IN (
                          'scrape', 'api_response', 'file_upload', 'user_paste'
                      )),
    source_url        TEXT,
    raw_content       TEXT NOT NULL,
    content_hash      TEXT NOT NULL,
    validation_status TEXT NOT NULL DEFAULT 'pending'
                      CHECK (validation_status IN (
                          'pending', 'validated', 'rejected', 'expired'
                      )),
    validation_notes  TEXT,
    promoted_memory_id BIGINT REFERENCES memory_events(id),
    expires_at        TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '1 hour'),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quarantine_pending ON external_quarantine(session_id, validation_status)
    WHERE validation_status = 'pending';

-- 5.2 secret_access_audit — Audit log for every {{SECRET.X}} resolution.
-- Zero-knowledge secrets specification: SPEC-005 §Zero-Knowledge Secrets.
CREATE TABLE secret_access_audit (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        UUID NOT NULL REFERENCES sessions(id),
    secret_alias      TEXT NOT NULL,
    accessed_at       TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_secret_audit_session ON secret_access_audit(session_id);

-- ============================================================================
-- SECTION 6 — ACTIVE CONTEXT VIEW
-- axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/011-canonical-definitions.md plan=phase-1/task-1-2/step-1-2-1
-- ============================================================================

-- 6.1 active_context_view — Dynamic context window for agent prompt assembly.
-- JOINs display_modes instead of reading from memory_events.display_mode,
-- preserving the append-only invariant on memory_events.
-- Canonical SQL from SPEC-011 §3.4.
-- Session isolation via current_setting('consensus.session_id') per SPEC-011 §9.
CREATE OR REPLACE VIEW active_context_view AS
WITH active_ids AS (
    SELECT unnest(active_pointers) AS ptr_id
    FROM iteration_commits
    WHERE session_id = current_setting('consensus.session_id')::UUID
    ORDER BY iteration_id DESC
    LIMIT 1
)
SELECT
    me.id,
    me.iteration_created,
    me.type,
    COALESCE(dm.mode, 'full') AS display_mode,
    CASE
        WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
            THEN me.summary_text
        WHEN COALESCE(dm.mode, 'full') = 'hidden'
            THEN NULL
        ELSE me.content
    END AS rendered_text
FROM memory_events me
JOIN active_ids ai ON me.id = ai.ptr_id
LEFT JOIN display_modes dm ON dm.memory_id = me.id
WHERE COALESCE(dm.mode, 'full') != 'hidden'
    AND me.session_id = current_setting('consensus.session_id')::UUID
ORDER BY me.iteration_created, me.id;

-- ============================================================================
-- SECTION 7 — MEMORY PAGES RESOLUTION CTE
-- axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md,specs/003-database.md plan=phase-1/task-1-2/step-1-2-2
-- ============================================================================

-- 7.1 memory_page_resolution — Expands both target_ids and linked_page_ids
-- with single-level nesting (not recursive). SPEC-002 §5.2.
--
-- Usage pattern (documented, not a view because it depends on a specific page name):
--
-- WITH direct_ids AS (
--     SELECT unnest(target_ids) AS memory_id
--     FROM memory_pages
--     WHERE name = $1 AND session_id = current_setting('consensus.session_id')::UUID
-- ),
-- linked_ids AS (
--     SELECT unnest(mp2.target_ids) AS memory_id
--     FROM memory_pages mp1
--     JOIN memory_pages mp2 ON mp2.id = ANY(mp1.linked_page_ids)
--     WHERE mp1.name = $1
--       AND mp1.session_id = current_setting('consensus.session_id')::UUID
-- ),
-- resolved AS (
--     SELECT memory_id FROM direct_ids
--     UNION
--     SELECT memory_id FROM linked_ids
-- )
-- SELECT me.* FROM memory_events me
-- JOIN resolved r ON me.id = r.memory_id
-- ORDER BY me.iteration_created, me.id;

-- ============================================================================
-- SECTION 8 — MARKDOWN AGGREGATION FOR PROMPT ASSEMBLY
-- axiom:trace work_item=schema-memory-01 spec=specs/002-memory.md plan=phase-1/task-1-2/step-1-2-1
-- ============================================================================

-- 8.1 string_agg() Markdown aggregation query for prompt assembly.
-- Renders active_context_view rows into a single Markdown string with headers
-- and section separators. SPEC-002 §7.3.
--
-- Usage:
-- SELECT string_agg(
--     CASE me.type
--         WHEN 'header' THEN E'\n# ' || me.rendered_text
--         WHEN 'thinking' THEN E'\n```thinking\n' || me.rendered_text || E'\n```'
--         WHEN 'tool_call' THEN E'\n🔧 Tool: ' || me.rendered_text
--         WHEN 'tool_result' THEN E'\n📋 Result: ' || me.rendered_text
--         WHEN 'system' THEN E'\n⚠️ System: ' || me.rendered_text
--         ELSE E'\n' || me.rendered_text
--     END,
--     E'\n' ORDER BY me.iteration_created, me.id
-- ) AS markdown_prompt
-- FROM active_context_view me;

-- ============================================================================
-- SECTION 9 — ROW-LEVEL SECURITY
-- axiom:trace work_item=schema-memory-01 spec=specs/005-security.md,specs/003-database.md,specs/011-canonical-definitions.md plan=phase-2/task-2-1
-- ============================================================================

-- 9.1 Enable RLS on all multi-tenant core tables.
-- SPEC-003 §7.5, SPEC-005 §Row-Level Security.
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE display_modes ENABLE ROW LEVEL SECURITY;
ALTER TABLE iteration_commits ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_quarantine ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_billing ENABLE ROW LEVEL SECURITY;
ALTER TABLE compression_queue ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE secret_access_audit ENABLE ROW LEVEL SECURITY;

-- 9.2 Session-isolation policies.
-- All use the canonical namespace consensus.session_id per SPEC-011 §9.
-- SPEC-005 §Row-Level Security, SPEC-011 §9.3.
CREATE POLICY session_isolate_sessions ON sessions
    FOR ALL USING (id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_memory ON memory_events
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_display ON display_modes
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_iterations ON iteration_commits
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_pages ON memory_pages
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_tasks ON tasks
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_tool_req ON tool_requests
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_tool_res ON tool_results
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_tool_files ON tool_files
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_quarantine ON external_quarantine
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_billing ON agent_billing
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

-- compression_queue has no session_id column; isolate via joined memory_events.session_id
CREATE POLICY session_isolate_compression ON compression_queue
    FOR ALL USING (
        EXISTS (
            SELECT 1 FROM memory_events
            WHERE memory_events.id = compression_queue.event_id
              AND memory_events.session_id = current_setting('consensus.session_id')::UUID
        )
        OR current_user = 'compression_worker'
        OR current_user = 'alt_mode_role'
    );

CREATE POLICY session_isolate_messages ON agent_messages
    FOR ALL USING (
        target_session_id = current_setting('consensus.session_id')::UUID
        OR sender_session_id = current_setting('consensus.session_id')::UUID
    );

CREATE POLICY session_isolate_audit ON audit_logs
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_secrets ON secret_access_audit
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

-- 9.x Tools RLS — SPEC-010 §Row-Level Security: Ownership Enforcement
ALTER TABLE custom_agent_tools ENABLE ROW LEVEL SECURITY;
ALTER TABLE tools_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE skills_registry ENABLE ROW LEVEL SECURITY;

-- Ownership enforcement on custom_agent_tools per SPEC-010.
-- Agent A cannot modify Agent B's tool.
CREATE POLICY enforce_ownership ON custom_agent_tools
    FOR UPDATE USING (
        current_setting('consensus.session_id')::UUID = creator_session_id
    );

CREATE POLICY session_isolate_tools ON custom_agent_tools
    FOR ALL USING (creator_session_id = current_setting('consensus.session_id')::UUID);

CREATE POLICY session_isolate_tools_registry ON tools_registry
    FOR ALL USING (true);  -- tools_registry is shared; all sessions can read

CREATE POLICY session_isolate_skills ON skills_registry
    FOR ALL USING (true);  -- skills_registry is shared; all sessions can read

-- ============================================================================
-- SECTION 10 — 4-ROLE PERMISSION MODEL
-- axiom:trace work_item=schema-memory-01 spec=specs/011-canonical-definitions.md,specs/003-database.md,specs/005-security.md plan=phase-2/task-2-2
-- ============================================================================

-- Canonical 4-role model from SPEC-011 §13:
--   agent_role         — Normal agent execution, subject to RLS
--   compression_worker — Background compression, subject to RLS
--   alt_mode_role      — Admin operations, bypasses RLS
--   tool_executor      — Writes tool_results, subject to RLS

-- Create roles BEFORE granting permissions to them.
-- (Idempotent — uses IF NOT EXISTS.)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agent_role') THEN
        CREATE ROLE agent_role;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'compression_worker') THEN
        CREATE ROLE compression_worker;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alt_mode_role') THEN
        CREATE ROLE alt_mode_role;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tool_executor') THEN
        CREATE ROLE tool_executor;
    END IF;
END $$;

-- 10.1 agent_role — core agent operations.
GRANT SELECT, INSERT ON memory_events TO agent_role;
GRANT SELECT, INSERT, UPDATE ON display_modes TO agent_role;
GRANT SELECT, INSERT ON iteration_commits TO agent_role;
GRANT SELECT, INSERT, UPDATE ON sessions TO agent_role;
GRANT SELECT, INSERT, UPDATE ON tasks TO agent_role;
GRANT SELECT, INSERT ON tool_requests TO agent_role;
GRANT SELECT, INSERT ON agent_billing TO agent_role;
GRANT SELECT ON active_context_view TO agent_role;
GRANT SELECT, INSERT ON agent_messages TO agent_role;
GRANT SELECT, INSERT ON memory_pages TO agent_role;
GRANT SELECT, INSERT ON external_quarantine TO agent_role;
GRANT SELECT, INSERT ON tool_files TO agent_role;
GRANT SELECT ON tools_registry TO agent_role;
GRANT SELECT ON skills_registry TO agent_role;
GRANT SELECT ON model_registry TO agent_role;
GRANT SELECT, INSERT ON secret_access_audit TO agent_role;

-- Append-only enforcement: agent_role NEVER updates or deletes memory_events.
REVOKE UPDATE, DELETE ON memory_events FROM agent_role;

-- Vault isolation: agent_role cannot read secrets.
-- Conditionally applied — vault.secrets is a Supabase feature not always present.
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_class WHERE relname = 'secrets') THEN
        REVOKE SELECT ON vault.secrets FROM agent_role;
    END IF;
    IF EXISTS (SELECT FROM pg_class WHERE relname = 'decrypted_secrets') THEN
        REVOKE SELECT ON vault.decrypted_secrets FROM agent_role;
    END IF;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'vault isolation not applied — vault schema may not exist';
END;
$$;

-- 10.2 compression_worker — background summary generation.
GRANT UPDATE (summary_text) ON memory_events TO compression_worker;
GRANT SELECT, INSERT, UPDATE ON display_modes TO compression_worker;
GRANT SELECT, INSERT, UPDATE ON compression_queue TO compression_worker;
GRANT SELECT ON memory_events TO compression_worker;
GRANT SELECT ON sessions TO compression_worker;

-- 10.3 tool_executor — external tool result writer.
GRANT SELECT, INSERT ON tool_results TO tool_executor;
GRANT UPDATE ON tool_requests TO tool_executor;
GRANT SELECT ON tool_requests TO tool_executor;
GRANT SELECT ON sessions TO tool_executor;
GRANT SELECT ON external_quarantine TO tool_executor;

-- 10.4 alt_mode_role — admin bypass.
-- Full access, bypasses all RLS. For administrative operations only.
GRANT ALL ON ALL TABLES IN SCHEMA public TO alt_mode_role;
ALTER ROLE alt_mode_role BYPASSRLS;
GRANT USAGE ON SCHEMA public TO alt_mode_role;

-- ============================================================================
-- SECTION 11 — HELPER FUNCTIONS & TRIGGERS
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md plan=phase-1
-- ============================================================================

-- 11.1 update_updated_at — Auto-set updated_at on mutation.
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- Apply to tables with updated_at column.
CREATE TRIGGER update_updated_at_tools_registry
    BEFORE UPDATE ON tools_registry
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_updated_at_skills_registry
    BEFORE UPDATE ON skills_registry
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_updated_at_workflows
    BEFORE UPDATE ON workflows
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_updated_at_custom_agent_tools
    BEFORE UPDATE ON custom_agent_tools
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_updated_at_system_settings
    BEFORE UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- 11.2 touch_session_heartbeat — Update session heartbeat on agent activity.
CREATE OR REPLACE FUNCTION touch_session_heartbeat()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE sessions
    SET heartbeat_at = now(),
        iteration = iteration + 1
    WHERE id = NEW.session_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER memory_touches_session
    AFTER INSERT ON memory_events
    FOR EACH ROW EXECUTE FUNCTION touch_session_heartbeat();

CREATE TRIGGER tool_req_touches_session
    AFTER INSERT ON tool_requests
    FOR EACH ROW EXECUTE FUNCTION touch_session_heartbeat();

-- 11.3 enforce_task_transitions — Validates task workflow state transitions.
-- SPEC-003 §5.1.
CREATE OR REPLACE FUNCTION enforce_task_transitions()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status = 'in_progress' AND OLD.status = 'pending' AND NEW.locked_by_agent IS NULL THEN
        RAISE EXCEPTION 'Task must be claimed before it can be in_progress';
    END IF;

    IF NEW.status = 'published' AND OLD.status != 'reviewed' THEN
        RAISE EXCEPTION 'Task must be reviewed before publishing. Current: %, Required: reviewed', OLD.status;
    END IF;

    IF OLD.status = 'published' AND NEW.status != 'published' THEN
        RAISE EXCEPTION 'Published tasks cannot be reverted';
    END IF;

    IF NEW.locked_by_agent IS DISTINCT FROM OLD.locked_by_agent
       AND OLD.locked_by_agent IS NOT NULL
       AND NEW.locked_by_agent IS NOT NULL THEN
        RAISE EXCEPTION 'Task already locked by agent %', OLD.locked_by_agent;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER task_transition_guard
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION enforce_task_transitions();

-- 11.4 enforce_prerequisites — Prevents claiming a task with unmet prerequisites.
-- SPEC-003 §5.2.
CREATE OR REPLACE FUNCTION enforce_prerequisites()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    unmet_count INT;
BEGIN
    IF NEW.status IN ('claimed', 'in_progress') AND OLD.status = 'pending' THEN
        SELECT COUNT(*) INTO unmet_count
        FROM tasks t
        WHERE t.id = ANY(NEW.prerequisite_ids)
          AND t.status != 'published';

        IF unmet_count > 0 THEN
            RAISE EXCEPTION 'Cannot claim task: % prerequisite(s) not yet published', unmet_count;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER task_prerequisite_guard
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION enforce_prerequisites();

-- 11.5 enforce_tool_rate_limit — Rate limiting for tool requests.
-- SPEC-003 §5.5.
CREATE OR REPLACE FUNCTION enforce_tool_rate_limit()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    max_per_min INT;
    recent_count INT;
BEGIN
    SELECT rate_limit_per_min INTO max_per_min
    FROM tools_registry
    WHERE name = NEW.tool_name AND rate_limit_per_min IS NOT NULL;

    IF max_per_min IS NOT NULL THEN
        SELECT COUNT(*) INTO recent_count
        FROM tool_requests
        WHERE session_id = NEW.session_id
          AND tool_name = NEW.tool_name
          AND created_at > now() - INTERVAL '1 minute';

        IF recent_count >= max_per_min THEN
            RAISE EXCEPTION 'Rate limit exceeded for tool %: % per minute',
                NEW.tool_name, max_per_min;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tool_rate_limit_guard
    BEFORE INSERT ON tool_requests
    FOR EACH ROW EXECUTE FUNCTION enforce_tool_rate_limit();

-- ============================================================================
-- SECTION 12 — DOCUMENTATION: SQL STATEMENT CLASSIFICATION SAFETY POLICY
-- axiom:trace work_item=schema-memory-01 spec=specs/011-canonical-definitions.md,specs/007-json-schema.md plan=phase-3/task-3-3
-- ============================================================================

-- 12.1 Multi-Statement Splitting (SPEC-011 §8.2)
-- The harness splits all SQL strings on semicolons before classification.
-- Each resulting statement is classified independently.
-- Example: "SELECT * FROM mem; DELETE FROM tasks" is split into
-- ["SELECT * FROM mem", "DELETE FROM tasks"] — each classified separately.

-- 12.2 Statement Classifier (SPEC-011 §8.3 Layer 1)
-- Classification types:
--   DML_READ    — SELECT statements. Allowed without restrictions.
--   DML_WRITE   — INSERT, UPDATE. Must target allowed tables only.
--   DML_DELETE  — DELETE FROM. Restricted to soft-delete stored procedures only.
--   DDL_CREATE  — CREATE TABLE. Must go through create_agent_memory_table().
--   DDL_ALTER   — ALTER TABLE. Restricted to ADD CONSTRAINT only (JSON Schema locks).
--   DANGEROUS   — DROP, TRUNCATE, GRANT, REVOKE, CREATE ROLE, ALTER ROLE, unknown.
--                 Never executed. Error injected into context.

-- 12.3 Execution Policy Table (SPEC-011 §8.3 Layer 2)
--   DML_READ    → Allowed (no restrictions)
--   DML_WRITE   → Allowed (whitelist-check on target tables)
--   DML_DELETE  → Restricted (soft-delete only via stored procedures)
--   DDL_CREATE  → Allowed (must use create_agent_memory_table())
--   DDL_ALTER   → Restricted (ADD CONSTRAINT only; no DROP COLUMN, no DROP TABLE)
--   DANGEROUS   → Blocked (never executed)

-- 12.4 Core Table Whitelist (SPEC-011 §8.3 Layer 3)
-- Allowed targets for DML_WRITE:
--   memory_events, display_modes, iteration_commits, memory_pages,
--   tasks, tool_requests, tool_results, sessions,
--   custom_agent_tools, tools_registry, skills_registry,
--   agent_billing, workflows, tool_files, external_quarantine,
--   compression_queue, model_registry

-- 12.5 Stored Procedure Preference (SPEC-011 §8.3 Layer 4)
-- Agents should prefer stored procedures for core operations:
--   set_display_mode(memory_id, mode)     — Instead of UPDATE display_modes
--   complete_session(session_id)          — Instead of UPDATE sessions SET status
--   create_agent_memory_table(name)       — Instead of raw CREATE TABLE

-- ============================================================================
-- SECTION 13 — DOCUMENTATION: DYNAMIC ENTITY GENERATOR
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md,specs/011-canonical-definitions.md plan=phase-3/task-3-4
-- ============================================================================

-- 13.1 create_agent_memory_table Function Contract (PostgreSQL)
-- SECURITY DEFINER function that provisions new tables with required system columns,
-- a JSONB payload column, and automatic RLS policies — without granting the agent
-- raw DDL privileges. SPEC-003 §3.2.
--
-- Reserved-name blocklist (31 reserved names):
--   sessions, memory_events, display_modes, iteration_commits, memory_pages,
--   tasks, tool_requests, tool_results, tools_registry, skills_registry,
--   agent_billing, workflows, custom_agent_tools, tool_files,
--   external_quarantine, compression_queue, model_registry,
--   staging_buffer, audit_logs, system_settings, agent_messages,
--   api_keys, api_rate_limits, external_events, webhook_registrations,
--   routing_rules, agent_circuit_breakers, agent_budget_limits,
--   secret_access_audit, approval_requests
--
-- Every dynamic table receives these system columns:
--   id UUID, session_id UUID FK→sessions, iteration_created BIGINT,
--   deleted_at TIMESTAMPTZ, linked_memory_pages UUID[],
--   data JSONB, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
--
-- Each dynamic table gets: RLS enabled, session isolation policy,
-- soft-delete trigger, updated_at auto-update trigger.

-- 13.2 SQLite/PocketBase Equivalent
-- No SECURITY DEFINER function available. Instead, a Go hook registered on the
-- OnRecordBeforeCreateRequest event for a virtual _table_requests collection
-- validates the table name and creates the collection with system fields.
-- SPEC-003 §3.3.

-- ============================================================================
-- SECTION 14 — DOCUMENTATION: LLM OUTPUT JSON SCHEMA CONTRACT
-- axiom:trace work_item=schema-memory-01 spec=specs/007-json-schema.md plan=phase-3/task-3-2
-- ============================================================================

-- 14.1 LLM Response JSON Schema (SPEC-007 §Output Schema)
-- Every LLM response MUST conform to this contract:
--
-- Required:
--   internal_monologue (string) — Agent's chain-of-thought reasoning
--
-- Optional:
--   memory_state_changes (string[]) — SQL statements for memory mutations
--   system_actions (string[]) — SQL statements for system-level operations
--   tool_requests — Array of tool invocations (tool_name, parameters)
--   sub_agent_spawns — Array of sub-agent spawn instructions
--   task_update — Task status/result changes
--
-- The full LLM response JSON is stored in iteration_commits.llm_response.
-- Structured access to the monologue and SQL execution is via audit_logs.

-- ============================================================================
-- SECTION 15 — DOCUMENTATION: JSON SCHEMA DB VALIDATION
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md plan=phase-3/task-3-1
-- ============================================================================

-- 15.1 pg_jsonschema Example (SPEC-003 §4.2)
-- Extension was created above (CREATE EXTENSION IF NOT EXISTS pg_jsonschema).
-- Pattern for applying JSON Schema to a dynamic table's JSONB data column:
--
-- ALTER TABLE order_tracking
-- ADD CONSTRAINT strict_order_schema
-- CHECK (jsonb_matches_schema('
--     {
--         "type": "object",
--         "required": ["item", "sku", "qty"],
--         "properties": {
--             "item": {"type": "string"},
--             "sku":  {"type": "string", "pattern": "^[A-Z]{3}-\\d{4}$"},
--             "qty":  {"type": "integer", "minimum": 1},
--             "notes": {"type": "string"}
--         },
--         "additionalProperties": false
--     }
-- ', data));

-- 15.2 SQLite Parity (SPEC-003 §4.3–4.5)
-- sqlite-jsonschema uses identical ALTER TABLE CHECK syntax.
-- Both extensions accept JSON Schema draft-07 documents.
-- Limitations: $ref/$defs (partial in SQLite), format (date/time only in SQLite).
-- For maximum parity, keep schemas flat and avoid $ref indirection.

-- ============================================================================
-- SECTION 16 — SQLITE/POCKETBASE PARITY DOCUMENTATION
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md,specs/011-canonical-definitions.md,specs/005-security.md plan=phase-4/task-4-2
-- ============================================================================

-- 16.1 Feature Parity Matrix (SPEC-003 §8.2)
--
-- | Feature                 | PostgreSQL (Supabase)      | SQLite (PocketBase)               |
-- |-------------------------|----------------------------|-----------------------------------|
-- | Vector search           | pgvector                   | sqlite-vec                        |
-- | JSON Schema             | pg_jsonschema              | sqlite-jsonschema (identical syntax) |
-- | RLS                     | Native RLS policies        | PocketBase API Rules + Go hooks   |
-- | Background jobs         | pg_cron                    | Go goroutines + ticker            |
-- | Async HTTP              | pg_net                     | Go net/http                       |
-- | JavaScript runtime      | PL/v8                      | PocketBase JSVM                   |
-- | Append-only enforcement | REVOKE UPDATE/DELETE        | Go OnRecordBeforeUpdateRequest/   |
-- |                         |                            | OnRecordBeforeDeleteRequest hooks |
-- | Triggers                | Native SQL triggers        | Go hooks on lifecycle events      |
-- | WAL mode                | N/A (MVCC)                 | PRAGMA journal_mode=WAL           |
-- | Secrets                 | Supabase Vault             | env vars + encrypted .env         |
-- | Dynamic table creation  | SECURITY DEFINER function  | Go OnRecordBeforeCreateRequest    |

-- 16.2 PocketBase 4-Role Model Mapping (SPEC-005 §PocketBase)
--   agent_role         → API Rules + request hooks with auth context
--   compression_worker → Go goroutine with full DB access (runs as admin)
--   alt_mode_role      → Admin API key with BYPASSRLS equivalent
--   tool_executor      → Go goroutine with scoped API Rules

-- 16.3 PocketBase Security Rules
--   Request hooks (OnRecordBeforeCreateRequest, OnRecordBeforeUpdateRequest, etc.)
--   have access to e.Auth and MUST be used for all security enforcement.
--   Model hooks (OnRecordCreate, OnRecordUpdate, etc.) have NO request context —
--   use them only for data integrity checks, never for authorization.
--   API Rules (listRule, viewRule, createRule, updateRule, deleteRule) are
--   evaluated ONLY for HTTP API requests. Programmatic Go operations bypass them.

-- 16.4 Write Contention (SPEC-003 §8.7)
-- SQLite has a single-writer constraint. PocketBase handles write contention
-- with exponential backoff + jitter: retry up to 5 times with baseDelayMs=10,
-- doubling delay each attempt plus random jitter.

-- 16.5 Connection Pooling Safety (SPEC-011 §9.5)
-- SET LOCAL is safe with Supabase Supavisor (transaction mode) and PgBouncer
-- (transaction mode) — it resets automatically at COMMIT or ROLLBACK.
-- Statement-level pooling is NOT supported.
-- Drizzle must be configured with prepare: false on Supavisor port 6543.
-- Kysely uses simple query protocol by default (no configuration needed).

-- ============================================================================
-- SECTION 17 — MUTUAL EXCLUSION LOCKS
-- axiom:trace work_item=schema-memory-01 spec=specs/003-database.md plan=phase-1/task-1-1/step-1-1-4
-- ============================================================================

-- 17.1 Conditional unique index: only one active lock per task.
-- SPEC-003 §5.4.
CREATE UNIQUE INDEX idx_one_active_lock ON tasks(id)
    WHERE locked_by_agent IS NOT NULL AND status IN ('claimed', 'in_progress');

-- Epistemic anchoring constraints: SPEC-003 §5.3.
ALTER TABLE tasks
ADD CONSTRAINT fk_result_memory_exists
 FOREIGN KEY (result_memory_id) REFERENCES memory_events(id);

ALTER TABLE tool_results
ADD CONSTRAINT fk_request_exists
 FOREIGN KEY (request_id) REFERENCES tool_requests(id);

-- ============================================================================
-- SECTION 18 — DEFAULT ROLES (if not already created)
-- ============================================================================

-- Create the 4 canonical roles if they don't exist.
-- These are created with NOLOGIN by default — the harness controls which
-- connection pool uses which role.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'agent_role') THEN
        CREATE ROLE agent_role;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'compression_worker') THEN
        CREATE ROLE compression_worker;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alt_mode_role') THEN
        CREATE ROLE alt_mode_role;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tool_executor') THEN
        CREATE ROLE tool_executor;
    END IF;
END $$;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary of objects created:
--   19 core tables + display_modes + secret_access_audit
--   1 view (active_context_view)
--   21+ indexes
--   5 triggers
--   4 helper functions
--   RLS enabled on 15 tables
--   15 session isolation policies
--   4 roles with complete GRANT/REVOKE model
--
-- axiom:trace work_item=schema-memory-01
--   spec=specs/002-memory.md,specs/003-database.md,specs/005-security.md,specs/007-json-schema.md,specs/011-canonical-definitions.md
--   plan=phase-1,phase-2,phase-3,phase-4
--   evidence=.memory-bank/work-items/schema-memory-01/verification.md
-- ============================================================================
