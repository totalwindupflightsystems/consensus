-- 018_postgres_memory_events_append_only.sql
-- Postgres append-only enforcement on memory_events via triggers.
-- SQLite equivalent: migration 017 (SQLite trigger syntax).
-- This migration runs ONLY on Postgres backends.

-- Trigger function: raises an exception for any UPDATE or DELETE on memory_events.
CREATE OR REPLACE FUNCTION enforce_memory_events_append_only()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'memory_events is append-only: % is not permitted', TG_OP;
END;
$$;

-- Block UPDATEs on memory_events.
DROP TRIGGER IF EXISTS trg_memory_events_append_only_update ON memory_events;
CREATE TRIGGER trg_memory_events_append_only_update
    BEFORE UPDATE ON memory_events
    FOR EACH ROW
    EXECUTE FUNCTION enforce_memory_events_append_only();

-- Block DELETEs on memory_events.
DROP TRIGGER IF EXISTS trg_memory_events_append_only_delete ON memory_events;
CREATE TRIGGER trg_memory_events_append_only_delete
    BEFORE DELETE ON memory_events
    FOR EACH ROW
    EXECUTE FUNCTION enforce_memory_events_append_only();

-- Goose Down: remove triggers and function.
-- +goose Down
DROP TRIGGER IF EXISTS trg_memory_events_append_only_update ON memory_events;
DROP TRIGGER IF EXISTS trg_memory_events_append_only_delete ON memory_events;
DROP FUNCTION IF EXISTS enforce_memory_events_append_only();
