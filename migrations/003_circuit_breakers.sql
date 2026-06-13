-- Migration 003: agent_circuit_breakers table (SPEC-006 §Circuit Breakers)
-- axiom:trace work_item=spec-006-hardening-01 spec=specs/006-transactions.md plan=phase-1/task-2

-- +goose Up
CREATE TABLE IF NOT EXISTS agent_circuit_breakers (
    session_id   UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    breaker_type TEXT NOT NULL CHECK (breaker_type IN ('consecutive_errors', 'iterations', 'budget')),
    threshold    INTEGER NOT NULL DEFAULT 5,
    current_count INTEGER NOT NULL DEFAULT 0,
    tripped_at   TIMESTAMP,
    reset_at     TIMESTAMP,
    PRIMARY KEY (session_id, breaker_type)
);

-- +goose Down
DROP TABLE IF EXISTS agent_circuit_breakers;
