# SPEC-013: Webhook & External Event Ingestion

**Status:** Draft
**Depends On:** SPEC-011 (Canonical Definitions), SPEC-003 (Database Schema)
**Created:** 2026-04-08

---

## 1. Overview

Consensus is not just request-response. External systems push events into the database via webhooks, and those events wake agents, trigger workflows, and create tasks. This spec defines how events enter the system, how they're validated, and how they route to the correct agent.

The core principle: **The database is the event bus.** No Kafka, no Redis, no RabbitMQ. Webhooks write rows to tables; triggers route them to agents.

**Source:** Gemini Chat Turn 38 (email → database → agent flow), Turn 34 (webhook as first-class pattern)

---

## 2. Event Ingestion Architecture

```
External System (GitHub, Stripe, Email, etc.)
    │
    ▼
Webhook Endpoint (Go HTTP handler in binary)
    │
    ▼ Validate signature, parse payload
    │
    ▼
external_events table (inbox)
    │
    ▼ AFTER INSERT trigger
    │
    ├── Route to existing session (wake agent)
    ├── Route to workflow (start automation)
    ├── Route to tasks (create new task)
    └── Route to external_quarantine (validate before processing)
```

---

## 3. Schema

### 3.1 external_events

The universal inbox for all incoming events:

```sql
CREATE TABLE external_events (
    id              BIGSERIAL PRIMARY KEY,
    source          TEXT NOT NULL CHECK (source IN (
                        'webhook', 'email', 'cron', 'manual', 'api'
                     )),
    source_id       TEXT,           -- External ID (e.g., GitHub delivery ID, email message ID)
    event_type      TEXT NOT NULL,  -- e.g., 'push', 'payment.received', 'new_email'
    payload         JSONB NOT NULL, -- Raw event data
    headers         JSONB,          -- HTTP headers (for debugging webhooks)
    signature_valid BOOLEAN NOT NULL DEFAULT false,
    session_id      UUID REFERENCES sessions(id),  -- NULL until routed
    workflow_id     UUID REFERENCES workflows(id),  -- NULL if no workflow match
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'routed', 'processing', 'completed', 'failed', 'quarantined')),
    last_error      TEXT,           -- Final delivery-failure error (NULL unless status = 'failed', §5.3)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_events_pending ON external_events(status)
    WHERE status = 'pending';
CREATE INDEX idx_events_source_type ON external_events(source, event_type);
CREATE INDEX idx_events_session ON external_events(session_id);
```

### 3.2 webhook_registrations

Defines which webhooks the system accepts and how to validate them:

```sql
CREATE TABLE webhook_registrations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    source          TEXT NOT NULL,           -- 'github', 'stripe', 'custom'
    url_path        TEXT NOT NULL,           -- e.g., '/webhooks/github'
    secret          TEXT NOT NULL,           -- HMAC secret (stored in vault on Supabase)
    event_types     TEXT[] NOT NULL DEFAULT '{}', -- Empty = accept all
    target_session_id UUID REFERENCES sessions(id),  -- Route to specific agent
    target_workflow_id UUID REFERENCES workflows(id), -- Route to specific workflow
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.3 routing_rules

Pattern-matching rules that route events to agents or workflows:

```sql
CREATE TABLE routing_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    source_pattern  TEXT,           -- Match source (e.g., 'github')
    event_type_pattern TEXT,       -- Match event_type (e.g., 'push')
    payload_pattern JSONB,         -- JSONB path expression for deep matching
    target_session_id UUID REFERENCES sessions(id),
    target_workflow_id UUID REFERENCES workflows(id),
    priority        INT NOT NULL DEFAULT 5,  -- Lower = higher priority
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## 4. Webhook Endpoint Implementation

### 4.1 Webhook Handler

```go
func HandleWebhook(w http.ResponseWriter, r *http.Request) {
    source := chi.URLParam(r, "source")
    registration := getWebhookRegistration(source)

    if registration == nil {
        http.Error(w, "unknown webhook source", http.StatusNotFound)
        return
    }

    body, _ := io.ReadAll(r.Body)
    signature := r.Header.Get("X-Hub-Signature-256")
    if signature == "" {
        signature = r.Header.Get("X-Signature-256")
    }
    if signature == "" {
        signature = r.Header.Get("X-Signature")
    }
    signatureValid := verifyHMAC(body, signature, registration.Secret)

    eventType := extractEventType(source, body, r.Header)
    payload, _ := json.Marshal(parsePayload(body))

    // Insert into external_events
    db.Exec(r.Context(), `
        INSERT INTO external_events (source, source_id, event_type, payload, headers,
                                     signature_valid, session_id, workflow_id, status)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, "webhook",
        r.Header.Get("X-Delivery-ID"),
        eventType,
        string(payload),
        string(headersJSON(r.Header)),
        signatureValid,
        registration.TargetSessionID,
        registration.TargetWorkflowID,
        map[bool]string{true: "pending", false: "quarantined"}[signatureValid],
    )

    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
```

The production server mounts the handler at the source-bearing chi route
`/webhooks/*`. A path such as `/webhooks/github` reaches the handler with
`github` as its source. The bare `/webhooks/` path still reaches the handler
with an empty source and returns `400`.

---

## 5. Event Routing (Trigger-Based)

### 5.1 Automatic Route Matching

When an `external_event` is inserted, a trigger attempts to match it to a routing rule:

```sql
CREATE OR REPLACE FUNCTION route_external_event()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_session_id UUID;
    v_workflow_id UUID;
    v_target_found BOOLEAN := false;
BEGIN
    -- If already routed by webhook registration, skip
    IF NEW.session_id IS NOT NULL OR NEW.workflow_id is NOT NULL THEN
        UPDATE external_events SET status = 'routed' WHERE id = NEW.id;
        RETURN NEW;
    END IF;

    -- Try routing rules (highest priority first)
    SELECT target_session_id, target_workflow_id
    INTO v_session_id, v_workflow_id
    FROM routing_rules
    WHERE enabled = true
      AND (source_pattern IS NULL OR NEW.source ~ source_pattern)
      AND (event_type_pattern IS NULL OR NEW.event_type ~ event_type_pattern)
    ORDER BY priority ASC
    LIMIT 1;

    IF v_session_id IS NOT NULL OR v_workflow_id IS NOT NULL THEN
        UPDATE external_events
        SET session_id = v_session_id,
            workflow_id = v_workflow_id,
            status = 'routed'
        WHERE id = NEW.id;

        -- Wake the target session and hand it the payload (DF-CONSENSUS-46:
        -- a bare status flip left the session stranded — the harness
        -- heartbeat loop dispatches only 'thinking'/'planning'/'tool_exec'
        -- sessions, and the agent can only act on an event it can see).
        -- The payload is ALWAYS recorded as a 'user_message' memory event
        -- (same shape the message API delivers; the next iteration reads it
        -- even if one is already in flight). Sessions that are not actively
        -- iterating ('idle', 'booting', 'waiting_sub', 'paused') are flipped
        -- to 'thinking' with an iteration bump so the heartbeat loop claims
        -- them; 'failed'/'completed' sessions get the payload recorded but
        -- are not autonomously resurrected by an external trigger.
        IF v_session_id IS NOT NULL THEN
            INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
            VALUES ('user_message',
                    '[webhook ' || NEW.event_type || '] ' || NEW.payload::text,
                    v_session_id,
                    (SELECT iteration + 1 FROM sessions WHERE id = v_session_id),
                    now());

            UPDATE sessions
            SET status = 'thinking',
                heartbeat_at = now(),
                iteration = iteration + 1
            WHERE id = v_session_id
              AND status IN ('idle', 'booting', 'waiting_sub', 'paused');
        END IF;
    ELSE
        -- No route found — leave as pending for manual review
        UPDATE external_events SET status = 'pending' WHERE id = NEW.id;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER external_event_router
    AFTER INSERT ON external_events
    FOR EACH ROW EXECUTE FUNCTION route_external_event();
```

### 5.2 Quarantine for Suspicious Events

Events with `signature_valid = false` or matching known attack patterns are quarantined:

```sql
-- Cognitive firewall check (reuses external_quarantine infrastructure)
INSERT INTO external_quarantine (session_id, source_type, raw_content, content_hash, validation_status)
SELECT NEW.session_id, 'webhook', NEW.payload::text, md5(NEW.payload::text), 'pending'
FROM external_events
WHERE id = NEW.id AND NEW.signature_valid = false;
```

Quarantined events require Alt-Mode approval before processing.

### 5.3 Routed-Payload Delivery Retry (REVIEW-CONSENSUS-5)

The session-target delivery in §5.1 (the `user_message` memory_events INSERT)
is retried with bounded exponential backoff: up to 3 attempts, sleeping 200ms
before the second attempt and 400ms before the third (doubling, capped at
800ms). The bounds are constants in `internal/webhook`, not configuration —
no config surface exists for them by design.

Attempts stop on the first success. A shutdown guard abandons remaining
attempts when the routing loop's context is already cancelled, so shutdown
never waits out the backoff. Each retry logs an attempt-scoped warning; a
delivery that succeeds after a retry logs an info line.

When every attempt fails, the outcome is persisted on the event row itself
(SPEC-013 §3.1 lifecycle value `'failed'`) instead of being dropped with only
a log line:

```sql
UPDATE external_events
SET status = 'failed', last_error = :final_error, processed_at = :failure_time
WHERE id = :event_id;
```

- `status = 'failed'` is the terminal delivery-failure state. The routing
  target (`session_id`) is preserved for diagnosis and manual redelivery.
- `last_error` (added by migration 026) carries the final attempt's error
  text; it is NULL on every non-failed row.
- A transient failure (a later attempt succeeds) leaves the event `'routed'`
  with no `last_error`.

Failed deliveries are reviewable with:

```sql
SELECT id, source, event_type, session_id, last_error, processed_at
FROM external_events
WHERE status = 'failed' AND session_id IS NOT NULL
ORDER BY processed_at DESC;
```

Migration 026 adds the `last_error` column and the partial index
`idx_events_failed_delivery` backing that review query. No backfill:
pre-026 rows carry no verifiable delivery-outcome signal (memory_events rows
have no event_id link), so historical rows are left exactly as they are and
the lifecycle starts at deploy time.

---

## 6. Example Workflows

### 6.1 GitHub Push → Documentation Agent

```sql
-- Register webhook
INSERT INTO webhook_registrations (name, source, url_path, secret, event_types, target_session_id)
VALUES ('github_push', 'github', '/webhooks/github', 'whsec_...', 
        ARRAY['push'], 'doc-agent-session-uuid');

-- When a push event arrives, it routes to the doc agent
-- The agent's system prompt includes: "You receive GitHub push events. Update documentation accordingly."
```

### 6.2 Email → Task Agent

```sql
-- Register email webhook
INSERT INTO webhook_registrations (name, source, url_path, secret, event_types, target_workflow_id)
VALUES ('inbound_email', 'email', '/webhooks/email', 'whsec_...',
        ARRAY['new_email'], (SELECT id FROM workflows WHERE name = 'email_triage'));

-- The email_triage workflow:
-- 1. Parse email content
-- 2. Classify (spam, question, action item)
-- 3. Route to appropriate agent or create task
```

### 6.3 Scheduled Cron → Recurring Agent

```sql
-- pg_cron triggers a synthetic event every morning
SELECT cron.schedule(
    'morning-report',
    '0 8 * * *',
    $$
    INSERT INTO external_events (source, event_type, payload, status)
    VALUES ('cron', 'daily_report', '{"type": "morning_report"}', 'routed');
    $$
);

-- Routing rule sends it to the report agent
INSERT INTO routing_rules (name, source_pattern, event_type_pattern, target_session_id)
VALUES ('daily_report', 'cron', 'daily_report', 'report-agent-session-uuid');
```

---

## 7. PocketBase Parity

| Feature | Postgres Backend | SQLite Backend |
|---|---|---|
| Webhook endpoint | Go HTTP handler (shared code) | Same |
| Event storage | `external_events` table | Same table in SQLite |
| Trigger routing | PostgreSQL trigger | Go database hook |
| Cron events | pg_cron (if available) or Go cron | Go `time.Ticker` |
| HMAC verification | Go `crypto/hmac` | Same |
| Quarantine | `external_quarantine` table + trigger | Same table + Go hook |
| Session wake | Trigger → `UPDATE sessions` | Go hook → `UPDATE sessions` |

---

## 8. Security Considerations

### 8.1 HMAC Verification

All webhook endpoints verify signatures using HMAC-SHA256 over the exact raw
request-body bytes. The handler accepts these header names in precedence order:
`X-Hub-Signature-256`, `X-Signature-256`, then `X-Signature`. The header value
is the lowercase hex digest; an optional `sha256=` prefix is stripped before a
constant-time comparison (`internal/webhook/webhook.go:282-294,626-634`).

```go
func verifyHMAC(body []byte, signature, secret string) bool {
    if signature == "" || secret == "" {
        return false
    }
    signature = strings.TrimPrefix(signature, "sha256=")
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := hex.EncodeToString(mac.Sum(nil))
    return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}
```

A missing or mismatched signature is stored as a quarantined event and still
receives `202 Accepted`; see `docs/API.md#webhooks` for the registration and
delivery procedure.

### 8.2 Rate Limiting

Webhook endpoints are rate-limited per source IP:

```sql
-- Rate limit check in the webhook handler
SELECT COUNT(*) FROM external_events
WHERE source = 'webhook'
  AND headers->>'x-forwarded-for' = :client_ip
  AND created_at > now() - INTERVAL '1 minute'
HAVING COUNT(*) < 60;  -- 60 requests per minute per IP
```

### 8.3 Payload Size Limits

- Maximum webhook payload: 1 MB
- Maximum headers: 64 KB
- Oversized payloads are rejected with 413 status

### 8.4 Idempotency

The `source_id` column enables deduplication:

```sql
-- Prevent duplicate processing
CREATE UNIQUE INDEX idx_events_source_id ON external_events(source, source_id)
    WHERE source_id IS NOT NULL;
```

Webhook handlers should use `ON CONFLICT DO NOTHING` for idempotent inserts:

```sql
INSERT INTO external_events (...)
VALUES (...)
ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL DO NOTHING;
```

---

## 9. Open Questions

1. **Retry semantics**: When an event fails processing, should we retry with exponential backoff? Or leave it in 'failed' status for manual review?
   - RESOLVED (REVIEW-CONSENSUS-5, §5.3): the routed-payload session delivery
     retries with bounded exponential backoff (3 attempts, 200ms doubling,
     capped); a terminal failure persists `status = 'failed'` + `last_error` on
     the event row for manual review. Processing-stage retries (rule execution,
     agent handling) remain out of scope.
2. **Event ordering**: Should events be processed strictly in order per source, or can they be processed in parallel?
3. **Webhook secret rotation**: How often should HMAC secrets be rotated? Can this be done without downtime?
4. **Event archival**: How long should completed events remain in `external_events` before archival or deletion?