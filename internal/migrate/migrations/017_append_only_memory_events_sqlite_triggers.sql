-- 017_append_only_memory_events_sqlite_triggers.sql
-- Enforce append-only invariant on memory_events via SQLite triggers.
-- Canonical from SPEC-002 §2.1, §2.4.
-- Single-line bodies to avoid semicolon splitting in migration runner.
-- Skipped on Postgres — Postgres append-only enforcement is a separate task.

CREATE TRIGGER IF NOT EXISTS trg_memory_events_append_only_update
BEFORE UPDATE ON memory_events
FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'memory_events is append-only: UPDATE is not permitted'); END;

CREATE TRIGGER IF NOT EXISTS trg_memory_events_append_only_delete
BEFORE DELETE ON memory_events
FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'memory_events is append-only: DELETE is not permitted'); END;
