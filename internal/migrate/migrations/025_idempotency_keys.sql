-- 025_idempotency_keys.sql — durable POST /message replay protection
--
-- A key is reserved before the memory event is inserted and completed with the
-- resulting event id in the same transaction. The nullable response id exists
-- only while that transaction is in flight; rollback removes the reservation.

CREATE TABLE idempotency_keys (
    key                 TEXT NOT NULL,
    session_id          UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    response_message_id BIGINT REFERENCES memory_events(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_idempotency_keys_session_key
    ON idempotency_keys(session_id, key);

GRANT SELECT, INSERT, UPDATE ON idempotency_keys TO agent_role;
