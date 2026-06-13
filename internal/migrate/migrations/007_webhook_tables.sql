-- ============================================================================
-- Conscience — 007_webhook_tables.sql
-- ============================================================================
-- Production DDL for webhook ingestion and event routing tables (SPEC-013).
-- Previously these tables were only created in test setup (CREATE TABLE IF NOT EXISTS).
--
-- axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-1 evidence=.memory-bank/work-items/spec-013-hardening-01/verification.md
-- ============================================================================

-- 1. webhook_registrations — Defines accepted webhooks, HMAC secrets, routing targets.
CREATE TABLE IF NOT EXISTS webhook_registrations (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL,
    url_path        TEXT NOT NULL,
    secret          TEXT NOT NULL,
    event_types     TEXT NOT NULL DEFAULT '{}',
    target_session_id   TEXT,
    target_workflow_id  TEXT,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL
);

-- 2. external_events — Universal inbox for all incoming events.
CREATE TABLE IF NOT EXISTS external_events (
    id              SERIAL PRIMARY KEY,
    source          TEXT NOT NULL,
    source_id       TEXT,
    event_type      TEXT NOT NULL,
    payload         TEXT NOT NULL,
    headers         TEXT,
    signature_valid INTEGER NOT NULL DEFAULT 0,
    session_id      TEXT,
    workflow_id     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TEXT
);

-- Idempotency index: prevent duplicate events (SPEC-013 §8.4)
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_source_id ON external_events(source, source_id);

-- Optimisation indexes (SPEC-013 §3.1)
CREATE INDEX IF NOT EXISTS idx_events_pending ON external_events(status)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_events_status ON external_events(status);
CREATE INDEX IF NOT EXISTS idx_events_session ON external_events(session_id);

-- 3. routing_rules — Pattern-matching rules for event routing (SPEC-013 §3.3).
CREATE TABLE IF NOT EXISTS routing_rules (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    source_pattern  TEXT,
    event_type_pattern  TEXT,
    payload_pattern     TEXT,
    target_session_id   TEXT,
    target_workflow_id  TEXT,
    priority        INTEGER NOT NULL DEFAULT 5,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_routing_rules_priority ON routing_rules(priority ASC);
