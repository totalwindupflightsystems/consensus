-- ============================================================================
-- Conscience — 016_sqlite_missing_tables.sql
-- ============================================================================
-- SQLite-native versions of tables from 001_initial_schema.sql that were
-- silently dropped by the filterForSQLite mAlterFk bug (no exit condition).
-- The bug caused all tables after ALTER TABLE ADD CONSTRAINT FOREIGN KEY
-- (line 222 of 001) to be skipped.
--
-- Uses IF NOT EXISTS so it's safe even after the filter bug is fixed
-- and 001_initial_schema successfully creates these tables.
-- ============================================================================

-- Tool results — responses from external tool runner
CREATE TABLE IF NOT EXISTS tool_results (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id        INTEGER NOT NULL REFERENCES tool_requests(id),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    output            TEXT NOT NULL,
    is_error          INTEGER NOT NULL DEFAULT 0,
    error_code        TEXT,
    token_count       INTEGER,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tool_result_request ON tool_results(request_id);

-- Skills registry — knowledge bundles for progressive disclosure
CREATE TABLE IF NOT EXISTS skills_registry (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name              TEXT NOT NULL UNIQUE,
    metadata          TEXT NOT NULL DEFAULT '{}',
    instructions      TEXT NOT NULL DEFAULT '',
    linked_tool_ids   TEXT NOT NULL DEFAULT '[]',
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Agent billing — cost tracking per session per iteration
CREATE TABLE IF NOT EXISTS agent_billing (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration         INTEGER NOT NULL,
    model_id          TEXT NOT NULL,
    category          TEXT NOT NULL CHECK (category IN (
                          'cognition', 'compression', 'embedding', 'tool_call'
                      )),
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd          REAL NOT NULL DEFAULT 0,
    recorded_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_billing_session ON agent_billing(session_id);
CREATE INDEX IF NOT EXISTS idx_billing_recorded ON agent_billing(recorded_at);

-- Workflows — reusable multi-step agent configurations
CREATE TABLE IF NOT EXISTS workflows (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT,
    steps             TEXT NOT NULL DEFAULT '[]',
    trigger_event     TEXT,
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Custom agent tools — agent-authored tools stored in DB
CREATE TABLE IF NOT EXISTS custom_agent_tools (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    creator_session_id  TEXT NOT NULL REFERENCES sessions(id),
    name                TEXT NOT NULL UNIQUE,
    language            TEXT NOT NULL CHECK (language IN (
                            'javascript', 'typescript', 'sql', 'python', 'go'
                        )),
    source_code         TEXT NOT NULL DEFAULT '',
    parameter_schema    TEXT NOT NULL DEFAULT '{}',
    approved            INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'testing', 'active', 'deprecated')),
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Tool files — file-like objects stored in DB
CREATE TABLE IF NOT EXISTS tool_files (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    name              TEXT NOT NULL,
    mime_type         TEXT NOT NULL DEFAULT 'application/octet-stream',
    content_b64       TEXT NOT NULL DEFAULT '',
    size_bytes        INTEGER NOT NULL DEFAULT 0,
    memory_event_id   INTEGER REFERENCES memory_events(id),
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tool_files_session ON tool_files(session_id);

-- External quarantine — staging area for untrusted external data
CREATE TABLE IF NOT EXISTS external_quarantine (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    source_type       TEXT NOT NULL CHECK (source_type IN (
                          'scrape', 'api_response', 'file_upload', 'user_paste'
                      )),
    source_url        TEXT,
    raw_content       TEXT NOT NULL DEFAULT '',
    content_hash      TEXT NOT NULL DEFAULT '',
    validation_status TEXT NOT NULL DEFAULT 'pending'
                      CHECK (validation_status IN (
                          'pending', 'validated', 'rejected', 'expired'
                      )),
    validation_notes  TEXT,
    promoted_memory_id INTEGER REFERENCES memory_events(id),
    expires_at        TEXT NOT NULL DEFAULT (datetime('now', '+1 hour')),
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_quarantine_pending ON external_quarantine(session_id, validation_status);

-- Secret access audit — audit log for every secret resolution
CREATE TABLE IF NOT EXISTS secret_access_audit (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    secret_alias      TEXT NOT NULL DEFAULT '',
    accessed_at       TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_secret_audit_session ON secret_access_audit(session_id);
