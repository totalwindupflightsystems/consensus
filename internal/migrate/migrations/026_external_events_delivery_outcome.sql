-- ============================================================================
-- Conscience — 026_external_events_delivery_outcome.sql
-- ============================================================================
-- Observability for routed-payload webhook delivery (REVIEW-CONSENSUS-5).
--
-- Delivery of a routed payload to its target session (the 'user_message'
-- memory_events INSERT in the webhook routing loop, internal/webhook/webhook.go)
-- is now retried with bounded exponential backoff (up to 3 attempts, 200ms →
-- 400ms → 800ms). If every attempt fails, the delivery outcome is persisted on
-- the event row itself so the failure is observable and diagnosable:
--
--   status       = 'failed'  (SPEC-013 §3.1 lifecycle value; delivery failure
--                             previously left the row 'routed' with the payload
--                             silently dropped)
--   last_error   = <final attempt error>
--   processed_at = <final attempt time>
--
-- The column is NULL except on a terminal delivery failure, so existing
-- queries and consumers are unaffected.
--
-- No backfill: pre-migration rows carry no delivery-outcome signal we can
-- verify (memory_events rows have no event_id link, and correlating by
-- created_at across mixed TEXT/TIMESTAMPTZ formats is not trustworthy), so
-- historical rows are left exactly as they are and the new lifecycle starts
-- at deploy time.
--
-- axiom:trace work_item=REVIEW-CONSENSUS-5 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1
-- ============================================================================

ALTER TABLE external_events ADD COLUMN last_error TEXT;

-- ============================================================================
-- Composite index: the routing loop scans pending events only through the
-- existing partial index; this index just makes the failure-review query
-- ("which deliveries failed, newest first") cheap:
--
--   SELECT id, source, event_type, last_error, processed_at
--   FROM external_events
--   WHERE status = 'failed' AND session_id IS NOT NULL
--   ORDER BY processed_at DESC;
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_events_failed_delivery
    ON external_events(processed_at DESC)
    WHERE status = 'failed' AND session_id IS NOT NULL;
