# SPEC-014: Human-in-the-Loop (HITL) Interrupt State

**Status:** Draft
**Depends On:** SPEC-011 (Canonical Definitions), SPEC-003 (Database Schema), SPEC-005 (Security)
**Created:** 2026-04-08

---

## 1. Overview

Not all agent actions should execute autonomously. Some require human oversight — approving tool usage, reviewing critical decisions, or intervening when the agent is confused. This spec defines how humans inject themselves into the agent loop, pausing execution and requiring explicit approval before proceeding.

The HITL system is database-native: interrupt states are columns on `sessions`, approval requests are rows in `approval_requests`, and the resume mechanism is a status change on a row.

**Source:** Gemini Chat Turn 38 (approval process in pipeline), Turn 22 (IDE dashboard alerts)

---

## 2. Interrupt Sources

### 2.1 Automatic Interrupts (Agent-Triggered)

The agent itself can request human approval before proceeding:

```json
{
  "internal_monologue": "I need to delete 5000 rows from order_tracking. This is destructive.",
  "memory_state_changes": [],
  "system_actions": [
    "SELECT request_approval('delete_bulk_orders', 'Delete 5000 rows from order_tracking where status = cancelled', 'high')"
  ]
}
```

### 2.2 Tool-Required Interrupts

Tools can be marked as `requires_approval = true` in `tools_registry`. When an agent requests such a tool, the harness automatically pauses and creates an approval request instead of executing:

```sql
-- Tools that always require human approval
UPDATE tools_registry SET requires_approval = true WHERE name = 'delete_database';
UPDATE tools_registry SET requires_approval = true WHERE name = 'send_email';
UPDATE tools_registry SET requires_approval = true WHERE name = 'charge_credit_card';
```

### 2.3 System Interrupts (Circuit Breaker)

When circuit breakers trip (SPEC-006 §), the session status changes to `failed` — but with HITL, the system can instead pause for human intervention:

```sql
-- After max_consecutive_errors, pause instead of fail
UPDATE sessions SET status = 'paused'
WHERE id = :session_id
  AND (SELECT consecutive_errors FROM agent_circuit_breakers
       WHERE session_id = :session_id AND breaker_type = 'consecutive_errors')
     >= 3;
```

### 2.4 External Interrupts (Human-Initiated)

A human can pause any running session at any time via the CLI (`conscience session pause <id>`) or the REST API:

```sql
UPDATE sessions SET status = 'paused' WHERE id = :session_id;
```

The harness checks session status before each iteration. If `paused`, it stops the loop.

---

## 3. Schema

### 3.1 approval_requests

```sql
CREATE TABLE approval_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration       BIGINT NOT NULL,
    request_type    TEXT NOT NULL CHECK (request_type IN (
                        'tool_execution', 'destructive_action', 'budget_override',
                        'schema_change', 'sub_agent_spawn', 'custom'
                     )),
    description     TEXT NOT NULL,
    risk_level      TEXT NOT NULL DEFAULT 'medium'
                    CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    context         JSONB NOT NULL DEFAULT '{}',
    target_tool     TEXT,
    target_sql      TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'modified')),
    reviewer_id     TEXT,
    review_notes    TEXT,
    modified_sql    TEXT,          -- If reviewer modifies the SQL before approving
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at     TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ
);

CREATE INDEX idx_approvals_pending ON approval_requests(session_id, status)
    WHERE status = 'pending';
```

### 3.2 hitl_configuration

Per-session and global HITL settings:

```sql
CREATE TABLE hitl_configuration (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope           TEXT NOT NULL CHECK (scope IN ('global', 'session')),
    session_id      UUID REFERENCES sessions(id),  -- NULL for global scope
    auto_pause_on_error_threshold INT NOT NULL DEFAULT 3,
    require_approval_for_destructive BOOLEAN NOT NULL DEFAULT true,
    require_approval_for_schema_changes BOOLEAN NOT NULL DEFAULT true,
    require_approval_for_external_tools BOOLEAN NOT NULL DEFAULT false,
    approval_timeout_minutes INT NOT NULL DEFAULT 60,
    notify_on_pause BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## 4. Flow

### 4.1 Tool-Requiring Approval

```
Agent iteration:
  → LLM requests tool: send_email(to=client@corp.com, subject="Important")
  → Harness checks tools_registry.requires_approval for 'send_email'
  → requires_approval = true → PAUSE

  → Harness creates approval_request:
    INSERT INTO approval_requests (
      session_id, request_type, description, risk_level,
      target_tool, context, status
    ) VALUES (
      :session_id, 'tool_execution',
      'Send email to client@corp.com with subject "Important"',
      'high', 'send_email',
      '{"to": "client@corp.com", "subject": "Important", "body": "..."}',
      'pending'
    )

  → Harness sets session status to 'paused'
  → Harness sends notification (webhook, Slack, email, dashboard)

Human review:
  → Alt-Mode dashboard shows pending approval (via CLI `conscience approve list` or future web UI)
  → Human reviews context, risk level, target
  → Human chooses:
     a) APPROVE → UPDATE approval_requests SET status = 'approved', reviewer_id = 'human-1'
     b) REJECT → UPDATE approval_requests SET status = 'rejected', review_notes = 'Wrong recipient'
     c) MODIFY → UPDATE approval_requests SET status = 'modified', modified_sql = '<adjusted params>'

  → APPROVAL trigger fires:
    → If approved: session status → 'idle', tool executes
    → If rejected: session status → 'idle', error injected into context
    → If modified: session status → 'idle', modified version executes instead

Agent resumes:
  → Context includes: "Your approval request was approved/rejected/modified"
  → Agent continues or adjusts based on outcome
```

### 4.2 Agent-Requested Approval

```
Agent iteration:
  → LLM outputs: SELECT request_approval('custom', 'I want to drop the temp_cache table', 'low')
  → Stored procedure creates approval_request row
  → Session status → 'paused'

Human review (same flow as above)
```

### 4.3 Circuit Breaker Interrupt

```
Agent iteration:
  → 3rd consecutive error
  → Circuit breaker check in harness
  → Instead of status → 'failed', check hitl_configuration
  → If auto_pause_on_error_threshold <= 3: status → 'paused'
  → approval_request created with type = 'budget_override', description = "3 consecutive errors"

Human review:
  → View error context and agent's monologue
  → Options: resume (let agent try again), modify context, or fail session
```

---

## 5. Stored Procedures

### 5.1 request_approval()

```sql
CREATE OR REPLACE FUNCTION request_approval(
    p_request_type TEXT,
    p_description TEXT,
    p_risk_level TEXT DEFAULT 'medium',
    p_target_tool TEXT DEFAULT NULL,
    p_target_sql TEXT DEFAULT NULL,
    p_context JSONB DEFAULT '{}'
)
RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_request_id UUID;
    v_session_id UUID := current_setting('conscience.session_id')::UUID;
    v_iteration BIGINT;
    v_timeout_minutes INT;
BEGIN
    SELECT iteration INTO v_iteration FROM sessions WHERE id = v_session_id;

    SELECT COALESCE(
        (SELECT approval_timeout_minutes FROM hitl_configuration
         WHERE scope = 'session' AND session_id = v_session_id),
        (SELECT approval_timeout_minutes FROM hitl_configuration
         WHERE scope = 'global' LIMIT 1),
        60
    ) INTO v_timeout_minutes;

    INSERT INTO approval_requests (
        session_id, iteration, request_type, description, risk_level,
        target_tool, target_sql, context, status,
        expires_at
    ) VALUES (
        v_session_id, v_iteration, p_request_type, p_description, p_risk_level,
        p_target_tool, p_target_sql, p_context, 'pending',
        now() + (v_timeout_minutes || ' minutes')::INTERVAL
    )
    RETURNING id INTO v_request_id;

    -- Pause the session
    UPDATE sessions SET status = 'paused' WHERE id = v_session_id;

    RETURN v_request_id;
END;
$$;
```

### 5.2 review_approval()

```sql
CREATE OR REPLACE FUNCTION review_approval(
    p_request_id UUID,
    p_decision TEXT,
    p_reviewer_id TEXT,
    p_review_notes TEXT DEFAULT NULL,
    p_modified_sql TEXT DEFAULT NULL
)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_session_id UUID;
BEGIN
    -- Validate decision
    IF p_decision NOT IN ('approved', 'rejected', 'modified') THEN
        RAISE EXCEPTION 'Invalid decision: %. Must be approved, rejected, or modified.', p_decision;
    END IF;

    -- Get session
    SELECT session_id INTO v_session_id
    FROM approval_requests
    WHERE id = p_request_id AND status = 'pending';

    IF v_session_id IS NULL THEN
        RAISE EXCEPTION 'Approval request not found or already resolved';
    END IF;

    -- Update approval request
    UPDATE approval_requests
    SET status = p_decision,
        reviewer_id = p_reviewer_id,
        review_notes = p_review_notes,
        modified_sql = p_modified_sql,
        reviewed_at = now()
    WHERE id = p_request_id;

    -- Wake the session
    UPDATE sessions
    SET status = 'idle', heartbeat_at = now()
    WHERE id = v_session_id AND status = 'paused';

    RETURN p_decision;
END;
$$;
```

### 5.3 Expiry Cleanup

```sql
-- pg_cron job to expire old approval requests
SELECT cron.schedule(
    'expire-approvals',
    '*/5 * * * *',
    $$
    UPDATE approval_requests
    SET status = 'expired'
    WHERE status = 'pending'
      AND expires_at < now();

    -- Fail sessions that are paused with expired approvals
    UPDATE sessions
    SET status = 'failed'
    WHERE id IN (
        SELECT session_id FROM approval_requests
        WHERE status = 'expired'
          AND session_id IN (SELECT id FROM sessions WHERE status = 'paused')
    );
    $$
);
```

---

## 6. Notification Channels

When an approval request is created, the system notifies the human reviewer.

### 6.1 Notification Methods

| Channel | Postgres Backend | SQLite Backend |
|---|---|---|
| Dashboard (real-time) | Postgres LISTEN/NOTIFY → SSE | Go channels → SSE |
| Email | Go net/smtp or Mailgun SDK | Same — shared code |
| Slack | Go HTTP client → Slack Webhook | Same — shared code |
| Webhook | Go HTTP client | Same — shared code |

### 6.2 Notification Schema

```sql
CREATE TABLE notification_log (
    id              BIGSERIAL PRIMARY KEY,
    approval_id     UUID NOT NULL REFERENCES approval_requests(id),
    channel         TEXT NOT NULL CHECK (channel IN ('dashboard', 'email', 'slack', 'webhook')),
    recipient       TEXT NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered       BOOLEAN NOT NULL DEFAULT false
);
```

---

## 7. PocketBase Parity

| Feature | Supabase | PocketBase |
|---|---|---|
| Approval request creation | Stored procedure | Go function with same logic |
| Session pause | `UPDATE sessions SET status = 'paused'` | Same SQL on SQLite |
| Approval review | Stored procedure + trigger | Go hook |
| Notification | Go goroutine with HTTP client | Same — shared code |
| Dashboard real-time | Postgres LISTEN/NOTIFY → SSE | Go channels → SSE |
| Expiry cleanup | pg_cron (if available) or Go cron | Go cron |

---

## 8. Security

### 8.1 Reviewer Authorization

Only users with `alt_mode_role` can approve requests. The `review_approval()` function should verify reviewer identity:

```sql
-- In review_approval(), add:
IF current_setting('conscience.user_id', true) IS NULL THEN
    RAISE EXCEPTION 'Reviewer identity required';
END IF;
```

### 8.2 Approval Audit Trail

Every approval decision is logged with reviewer identity, timestamp, and notes. This provides compliance traceability.

### 8.3 No Auto-Approval

The system MUST NOT auto-approve requests, regardless of risk level. Every `pending` approval requires explicit human action. The `expires_at` column causes expiration, not auto-approval.

---

## 9. Open Questions

1. **Delegation**: Can a reviewer delegate approval to another reviewer? If so, how is the chain tracked?
2. **Batch approvals**: Should related approval requests be batchable (approve/reject all at once)?
3. **Conditional auto-approval**: For low-risk, repetitive actions, could auto-approval be enabled after N manual approvals of the same type?
4. **Mobile notifications**: How to push approval requests to mobile devices (Push notification integration)?