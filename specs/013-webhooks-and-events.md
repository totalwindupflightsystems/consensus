# SPEC-013: Webhook & External Event Ingestion

**Status:** Draft
**Depends On:** SPEC-011 (Canonical Definitions), SPEC-003 (Database Schema)
**Created:** 2026-04-08

---

## 1. Overview

Conscience is not just request-response. External systems push events into the database via webhooks, and those events wake agents, trigger workflows, and create tasks. This spec defines how events enter the system, how they're validated, and how they route to the correct agent.

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
    signatureValid := verifyHMAC(body, r.Header.Get("X-Signature-256"), registration.Secret)

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

        -- Wake the target session if it's idle or waiting
        IF v_session_id IS NOT NULL THEN
            UPDATE sessions
            SET status = 'idle',
                heartbeat_at = now()
            WHERE id = v_session_id
              AND status IN ('waiting_sub', 'paused');
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

All webhook endpoints verify signatures using HMAC-SHA256:

```typescript
function verifyHMAC(body: string, signature: string | null, secret: string): boolean {
    if (!signature) return false;
    const expected = crypto.subtle.sign('HMAC',secret, new TextEncoder().encode(body));
    return timingSafeEqual(signature, `sha256=${expected}`);
}
```

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
2. **Event ordering**: Should events be processed strictly in order per source, or can they be processed in parallel?
3. **Webhook secret rotation**: How often should HMAC secrets be rotated? Can this be done without downtime?
4. **Event archival**: How long should completed events remain in `external_events` before archival or deletion?