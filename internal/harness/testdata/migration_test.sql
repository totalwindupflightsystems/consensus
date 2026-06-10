-- SQLite-compatible test migration for harness integration tests.
-- Creates only the tables queried by the harness core loop.
-- 
-- axiom:trace work_item=runtime-harness-01 spec=specs/003-database.md plan=phase-6/task-6-1/step-6-1-1

BEGIN;

-- sessions — root identity table
CREATE TABLE sessions (
    id                TEXT PRIMARY KEY,
    parent_id         TEXT REFERENCES sessions(id),
    agent_name        TEXT NOT NULL DEFAULT 'test_agent',
    model_id          TEXT NOT NULL DEFAULT 'test-model',
    status            TEXT NOT NULL DEFAULT 'booting'
                      CHECK (status IN ('booting','idle','planning','thinking','tool_exec','executing','waiting_sub','paused','completed','failed')),
    trust_level       TEXT NOT NULL DEFAULT 'high'
                      CHECK (trust_level IN ('low', 'medium', 'high')),
    goal              TEXT NOT NULL DEFAULT '',
    context_budget    INTEGER NOT NULL DEFAULT 128000,
    tokens_used_in    INTEGER NOT NULL DEFAULT 0,
    tokens_used_out   INTEGER NOT NULL DEFAULT 0,
    iteration         INTEGER NOT NULL DEFAULT 0,
    project_id        TEXT,
    heartbeat_at      TEXT NOT NULL DEFAULT (datetime('now')),
    planning_max_turns INTEGER NOT NULL DEFAULT 10,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at      TEXT
);

-- model_registry — must exist before sessions FK 
CREATE TABLE model_registry (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id          TEXT NOT NULL UNIQUE,
    tier              INTEGER NOT NULL DEFAULT 1 CHECK (tier IN (1,2,3)),
    max_context       INTEGER NOT NULL DEFAULT 128000,
    cost_per_m_in     REAL NOT NULL DEFAULT 0,
    cost_per_m_out    REAL NOT NULL DEFAULT 0,
    enabled           INTEGER NOT NULL DEFAULT 1
);

-- memory_events — append-only ledger
CREATE TABLE memory_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    type              TEXT NOT NULL
                      CHECK (type IN ('header','text_block','tool_call','tool_result','thinking','system','inherited_pointer','user_message')),
    content           TEXT NOT NULL DEFAULT '',
    summary_text      TEXT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration_created INTEGER NOT NULL DEFAULT 0,
    linked_memory_pages TEXT,  -- JSON array as text
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- display_modes — display state separate from memory_events
CREATE TABLE display_modes (
    memory_id         INTEGER NOT NULL REFERENCES memory_events(id) ON DELETE CASCADE,
    mode              TEXT NOT NULL DEFAULT 'full'
                      CHECK (mode IN ('full','compressed','hidden')),
    set_at            TEXT NOT NULL DEFAULT (datetime('now')),
    set_by_iteration  INTEGER NOT NULL DEFAULT 0,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    PRIMARY KEY (memory_id)
);

-- iteration_commits — snapshots per iteration
CREATE TABLE iteration_commits (
    iteration_id      INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    active_pointers   TEXT NOT NULL DEFAULT '[]', -- JSON array
    display_rules     TEXT NOT NULL DEFAULT '{}', -- JSON object
    llm_response      TEXT,  -- JSON
    sql_executed      TEXT,  -- JSON array
    rows_affected     INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- audit_logs — per-iteration audit trail
CREATE TABLE audit_logs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration         INTEGER NOT NULL,
    monologue         TEXT,
    sql_executed      TEXT, -- JSON array
    result            TEXT NOT NULL CHECK (result IN ('committed','rolled_back')),
    error_message     TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- tools_registry — tool catalog (harness reads this)
CREATE TABLE tools_registry (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    description       TEXT NOT NULL DEFAULT '',
    hemisphere        TEXT NOT NULL DEFAULT 'internal' CHECK (hemisphere IN ('internal','external')),
    parameter_schema  TEXT NOT NULL DEFAULT '{}',
    handler_type      TEXT NOT NULL DEFAULT 'sql_function' 
                      CHECK (handler_type IN ('sql_function','http_endpoint','go_native','subprocess')),
    handler_ref       TEXT NOT NULL DEFAULT '',
    owner_session_id  TEXT REFERENCES sessions(id),
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','testing','deprecated','disabled')),
    enabled           INTEGER NOT NULL DEFAULT 1,
    requires_approval INTEGER NOT NULL DEFAULT 0,
    rate_limit_per_min INTEGER,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- skills_registry — skill catalog (progressive disclosure)
CREATE TABLE skills_registry (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    metadata          TEXT NOT NULL DEFAULT '{}',
    instructions      TEXT NOT NULL DEFAULT '',
    linked_tool_ids   TEXT NOT NULL DEFAULT '[]',
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- tool_requests — outbound tool invocations
CREATE TABLE tool_requests (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration_id      INTEGER NOT NULL DEFAULT 0,
    tool_name         TEXT NOT NULL DEFAULT '',
    parameters        TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','awaiting_approval','executing','completed','failed','timeout')),
    timeout_ms        INTEGER NOT NULL DEFAULT 30000,
    approval_request_id TEXT REFERENCES approval_requests(id),
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    executed_at       TEXT,
    completed_at      TEXT
);

-- tool_results — responses from tool execution
CREATE TABLE tool_results (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id        INTEGER NOT NULL REFERENCES tool_requests(id),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    output            TEXT NOT NULL DEFAULT '',
    is_error          INTEGER NOT NULL DEFAULT 0,
    error_code        TEXT,
    exit_code         INTEGER,
    duration_ms       INTEGER,
    token_count       INTEGER,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_tool_req_pending ON tool_requests(session_id, status);
CREATE INDEX idx_tool_result_request ON tool_results(request_id);

-- tasks — unified work queue
CREATE TABLE tasks (
    id                TEXT PRIMARY KEY,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    parent_task_id    TEXT REFERENCES tasks(id),
    title             TEXT NOT NULL DEFAULT '',
    description       TEXT,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','claimed','in_progress','reviewed','published','failed','cancelled')),
    priority          INTEGER NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    locked_by_agent   TEXT REFERENCES sessions(id),
    prerequisite_ids  TEXT NOT NULL DEFAULT '[]', -- JSON array
    result_memory_id  INTEGER REFERENCES memory_events(id),
    project_id        TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    claimed_at        TEXT,
    completed_at      TEXT
);

-- agent_messages — inter-session communication
CREATE TABLE agent_messages (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    target_session_id TEXT NOT NULL REFERENCES sessions(id),
    sender_session_id TEXT NOT NULL REFERENCES sessions(id),
    payload           TEXT NOT NULL DEFAULT '{}',
    read              INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- compression_queue — background compression
CREATE TABLE compression_queue (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id          INTEGER NOT NULL REFERENCES memory_events(id),
    current_tier      INTEGER NOT NULL DEFAULT 1,
    next_tier         INTEGER NOT NULL DEFAULT 2,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','processing','completed','failed')),
    attempts          INTEGER NOT NULL DEFAULT 0,
    max_attempts      INTEGER NOT NULL DEFAULT 3,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at      TEXT
);

-- memory_pages — named ID groups
CREATE TABLE memory_pages (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL,
    target_ids        TEXT NOT NULL DEFAULT '[]', -- JSON array
    linked_page_ids   TEXT NOT NULL DEFAULT '[]',
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(name, session_id)
);

-- staging_buffer — multi-turn planning scratchpad
CREATE TABLE staging_buffer (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    turn              INTEGER NOT NULL DEFAULT 1,
    seq               INTEGER NOT NULL DEFAULT 1,
    sql_statement     TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'staged'
                      CHECK (status IN ('staged','executed','committed','rolled_back','failed')),
    result            TEXT,
    error             TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- api_keys — API key authentication
CREATE TABLE api_keys (
    id                TEXT PRIMARY KEY,
    key_hash          TEXT NOT NULL UNIQUE,
    key_prefix        TEXT NOT NULL DEFAULT '',
    scope             TEXT NOT NULL DEFAULT 'admin' CHECK (scope IN ('admin','session','readonly','webhook')),
    session_id        TEXT REFERENCES sessions(id),
    expires_at        TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

-- approval_requests — HITL approval requests (SPEC-014 §3.1)
CREATE TABLE approval_requests (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(id),
    iteration       INTEGER NOT NULL,
    request_type    TEXT NOT NULL CHECK (request_type IN (
                        'tool_execution', 'destructive_action', 'budget_override',
                        'schema_change', 'sub_agent_spawn', 'custom'
                     )),
    description     TEXT NOT NULL,
    risk_level      TEXT NOT NULL DEFAULT 'medium'
                    CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    context         TEXT NOT NULL DEFAULT '{}',
    target_tool     TEXT,
    target_sql      TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'modified')),
    reviewer_id     TEXT,
    review_notes    TEXT,
    modified_sql    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    reviewed_at     TEXT,
    expires_at      TEXT
);

CREATE INDEX idx_approvals_pending ON approval_requests(session_id, status)
    WHERE status = 'pending';
CREATE INDEX idx_approvals_status ON approval_requests(status);

-- hitl_configuration — per-session and global HITL settings (SPEC-014 §3.2)
CREATE TABLE hitl_configuration (
    id              TEXT PRIMARY KEY,
    scope           TEXT NOT NULL CHECK (scope IN ('global', 'session')),
    session_id      TEXT REFERENCES sessions(id),
    auto_pause_on_error_threshold INTEGER NOT NULL DEFAULT 3,
    require_approval_for_destructive INTEGER NOT NULL DEFAULT 1,
    require_approval_for_schema_changes INTEGER NOT NULL DEFAULT 1,
    require_approval_for_external_tools INTEGER NOT NULL DEFAULT 0,
    approval_timeout_minutes INTEGER NOT NULL DEFAULT 60,
    notify_on_pause INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- notification_log — HITL notification audit trail (SPEC-014 §6.2)
CREATE TABLE notification_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    approval_id     TEXT NOT NULL REFERENCES approval_requests(id),
    channel         TEXT NOT NULL DEFAULT 'dashboard'
                    CHECK (channel IN ('dashboard', 'email', 'slack', 'webhook')),
    recipient       TEXT NOT NULL DEFAULT 'admin',
    sent_at         TEXT NOT NULL DEFAULT (datetime('now')),
    delivered       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_notif_log_approval ON notification_log(approval_id);

-- api_rate_limits — per-key sliding window rate counters
CREATE TABLE api_rate_limits (
    key_prefix        TEXT PRIMARY KEY,
    requests_count    INTEGER NOT NULL DEFAULT 0,
    window_start      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- system_settings — key-value system configuration (SPEC-011 §12.2)
CREATE TABLE system_settings (
    key               TEXT PRIMARY KEY,
    value             TEXT NOT NULL DEFAULT '',
    description       TEXT,
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- agent_billing — cost tracking per session per iteration (SPEC-011 §6.2)
CREATE TABLE agent_billing (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration         INTEGER NOT NULL DEFAULT 0,
    model_id          TEXT NOT NULL DEFAULT '',
    category          TEXT NOT NULL DEFAULT 'cognition' 
                      CHECK (category IN ('cognition','compression','embedding','tool_call')),
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd          REAL NOT NULL DEFAULT 0,
    recorded_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_test_sessions_status ON sessions(status);
CREATE INDEX idx_test_memory_session ON memory_events(session_id, iteration_created);
CREATE INDEX idx_test_iteration_session ON iteration_commits(session_id, iteration_id DESC);
CREATE INDEX idx_test_audit_session ON audit_logs(session_id, iteration);
CREATE INDEX idx_test_tasks_session_status ON tasks(session_id, status);
CREATE INDEX idx_test_messages_target ON agent_messages(target_session_id, read);
CREATE INDEX idx_test_compression_pending ON compression_queue(status) WHERE status = 'pending';
CREATE INDEX idx_test_tasks_locked ON tasks(locked_by_agent) WHERE locked_by_agent IS NOT NULL;

-- agent_circuit_breakers — circuit breaker persistence (SPEC-006 §Circuit Breakers)
CREATE TABLE agent_circuit_breakers (
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    breaker_type  TEXT NOT NULL CHECK (breaker_type IN ('consecutive_errors', 'iterations', 'budget')),
    threshold     INTEGER NOT NULL DEFAULT 5,
    current_count INTEGER NOT NULL DEFAULT 0,
    tripped_at    TEXT,
    reset_at      TEXT,
    PRIMARY KEY (session_id, breaker_type)
);

COMMIT;
