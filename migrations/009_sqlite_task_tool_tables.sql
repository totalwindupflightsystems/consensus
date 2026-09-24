-- ============================================================================
-- Conscience — 009_sqlite_task_tool_tables.sql
-- ============================================================================
-- SQLite-native versions of task, tool, and registry tables that the
-- filterForSQLite function misses (UUID type spacing, foreign key ordering).
-- Uses IF NOT EXISTS so it's safe even if 001_initial_schema partially
-- created these tables.
-- ============================================================================

-- Tasks table — SQLite-native syntax
CREATE TABLE IF NOT EXISTS tasks (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    parent_task_id    TEXT REFERENCES tasks(id),
    title             TEXT NOT NULL,
    description       TEXT,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN (
                          'pending', 'claimed', 'in_progress',
                          'reviewed', 'published', 'failed', 'cancelled'
                      )),
    priority          INT NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    locked_by_agent   TEXT REFERENCES sessions(id),
    prerequisite_ids  TEXT NOT NULL DEFAULT '[]',
    result_memory_id  INTEGER REFERENCES memory_events(id),
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    claimed_at        TEXT,
    completed_at      TEXT
);

-- Tool requests — SQLite-native syntax
CREATE TABLE IF NOT EXISTS tool_requests (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL REFERENCES sessions(id),
    iteration_id      INTEGER NOT NULL,
    tool_name         TEXT NOT NULL,
    parameters        TEXT NOT NULL DEFAULT '{}',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'timeout')),
    timeout_ms        INT NOT NULL DEFAULT 30000,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    executed_at       TEXT,
    completed_at      TEXT
);

-- Tools registry — must match the exact columns used by harness/context.go:readTools
-- which queries: name, description, hemisphere FROM tools_registry
-- WHERE enabled = true AND status = 'active'
CREATE TABLE IF NOT EXISTS tools_registry (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT NOT NULL,
    hemisphere        TEXT NOT NULL DEFAULT 'internal'
                      CHECK (hemisphere IN ('internal', 'external')),
    parameter_schema  TEXT NOT NULL DEFAULT '{}',
    handler_type      TEXT NOT NULL DEFAULT 'sql_function'
                      CHECK (handler_type IN ('sql_function', 'http_endpoint', 'go_native', 'subprocess')),
    handler_ref       TEXT NOT NULL DEFAULT '',
    owner_session_id  TEXT REFERENCES sessions(id),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'testing', 'deprecated', 'disabled')),
    enabled           INTEGER NOT NULL DEFAULT 1,
    requires_approval INTEGER NOT NULL DEFAULT 0,
    rate_limit_per_min INTEGER,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);
