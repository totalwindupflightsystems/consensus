# SPEC-005: Security — RLS, Alt-Mode, Cognitive Firewall & Secrets

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

## Overview

The Consensus framework assumes the LLM is an untrusted runtime. Every output is treated as potentially malformed, malicious, or hallucinated. Security is enforced at the database level through Row-Level Security, soft-delete semantics, content quarantine, and zero-knowledge secret handling.

## Row-Level Security

Every table in the system is scoped to `session_id`. This means an agent that hallucinates a `DELETE` statement can only affect its own data.

### PostgreSQL (Supabase)

```sql
ALTER TABLE memory_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY session_isolation ON memory_events
  FOR ALL
  USING (session_id = current_setting('consensus.session_id')::UUID);
```

Applied consistently across all tables:

- `memory_events` — agents read/write only their own memories
- `display_modes` — compressed display state scoped to owning session
- `tasks` — agents see only their own task rows and their children's rows
- `tool_requests` / `tool_files` — agents access only tools they own or are shared with

### Why RLS, not application logic

Application-level access control requires every code path to correctly enforce rules. One missed check leaks data. RLS enforces isolation at the storage layer — even direct SQL execution cannot bypass it.

### PocketBase (Local Path)

PocketBase does not have database-level RLS. Session isolation is enforced at two layers:

1. **PocketBase API Rules** (declarative, zero Go code): Every collection has filter rules like `session_id = @request.auth.id` that automatically scope all API queries. This provides the same isolation guarantee as simple RLS policies for all API-accessible operations.

2. **Go hook middleware** (for column-level and role-based controls that RLS provides on Postgres): The harness shim injects `WHERE session_id = ?` into queries before execution, and Go hooks enforce the 4-role permission model (SPEC-011 §13).

**Critical:** PocketBase has two hook types. **Request hooks** (`OnRecordBeforeCreateRequest`, `OnRecordBeforeUpdateRequest`, etc.) have access to `e.Auth` (the authenticated user) and MUST be used for all security enforcement. **Model hooks** (`OnRecordCreate`, `OnRecordUpdate`, etc.) fire for all operations including internal programmatic saves but have NO request context — use them only for data integrity checks, never for authorization.

The 4-role model (SPEC-011 §13) maps to PocketBase as follows:

| `agent_role` | API Rules + request hooks with auth context |
| `compression_worker` | Go goroutine with full DB access (runs as admin) |
| `alt_mode_role` | Admin API key with BYPASSRLS equivalent |
| `tool_executor` | Go goroutine with scoped API Rules |

**Important enforcement scope**: PocketBase API Rules (`listRule`, `viewRule`, `createRule`, `updateRule`, `deleteRule`) are evaluated **only for HTTP API requests**. Programmatic Go operations via `app.Save(record)` or `app.DB().NewQuery(...)` bypass API Rules entirely. This is analogous to how PostgreSQL RLS is bypassed by SECURITY DEFINER functions or superuser connections. The security model for PocketBase relies on:
- **API Rules**: Enforce access control for all external-facing traffic (REST API, MCP, webhooks) — covers the same surface as simple RLS policies
- **Go request hooks**: Enforce access control with auth context for API operations that need column-level or role-based controls — covers the same surface as complex RLS policies
- **Go model hooks**: Enforce data integrity constraints for all operations (including internal programmatic ones) — covers the same surface as CHECK constraints and triggers

## Alt-Mode: Safe Soft Delete

Agents must never permanently destroy data. Alt-Mode implements soft delete via a `deleted_at` column and an RLS policy:

```sql
ALTER TABLE memory_events ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE POLICY hide_deleted ON memory_events
  FOR SELECT
  USING (deleted_at IS NULL);
```

When an agent "deletes" a memory:

```sql
UPDATE memory_events SET deleted_at = now() WHERE id = 42;
```

The row disappears from normal queries. No data is destroyed.

### Agent Recovery Mode

A special role exists for recovering alt-deleted data:

```sql
CREATE ROLE agent_recovery_mode;

-- Recovery role bypasses the hide_deleted policy
CREATE POLICY recovery_sees_all ON memory_events
  FOR SELECT
  TO agent_recovery_mode
  USING (true);
```

Use cases:

- Auditing what an agent deleted and why
- Restoring accidentally removed memories
- Forensic analysis of agent behavior

Recovered data is visible only to the recovery role, never to the agent itself.

## Cognitive Firewall

External data (API responses, scraped pages, user uploads) is untrusted. The cognitive firewall quarantines all external input before it enters agent memory.

### Architecture

```
External Data
    │
    ▼
external_quarantine table
    │
    ▼
Fast local model scan
    │
    ├─ Clean → tool_results (available to agent)
    │
    └─ Infected → quarantine_logs + security fault alert to agent
```

### Implementation

```sql
CREATE TABLE external_quarantine (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    source_type     TEXT NOT NULL CHECK (source_type IN ('scrape', 'api_response', 'file_upload', 'user_paste')),
    source_url      TEXT,
    raw_content     TEXT NOT NULL,
    content_hash    TEXT NOT NULL,
    validation_status TEXT NOT NULL DEFAULT 'pending'
                      CHECK (validation_status IN ('pending', 'validated', 'rejected', 'expired')),
    validation_notes TEXT,
    promoted_memory_id BIGINT REFERENCES memory_events(id),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '1 hour'),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quarantine_pending ON external_quarantine(session_id, validation_status)
    WHERE validation_status = 'pending';
```

Scanning process:

 1. All external data lands in `external_quarantine` with `validation_status = 'pending'`
2. A fast local model (e.g., Llama 3 8B) scans for prompt injection patterns
3. Clean data is promoted to `tool_results` and becomes available to the agent
4. Infected data gets `validation_status = 'rejected'` with details in `validation_notes`
5. The agent receives a security fault alert: "External input quarantined — potential prompt injection detected"

The agent never sees raw external data. It only sees the scanner's verdict.

### What the scanner detects

- Prompt injection in API responses (hidden instructions in JSON, HTML, etc.)
- Role-playing jailbreaks embedded in scraped content
- Data exfiltration payloads in user uploads
- Malformed content designed to confuse the JSON parser

## Zero-Knowledge Secrets

Agents never handle real secret values. They reference secrets through alias syntax:

```
{{SECRET.GH_TOKEN}}
{{SECRET.DB_CONNECTION_STRING}}
{{SECRET.API_KEY_STRIPE}}
```

### Lifecycle

1. **Configuration**: Admin registers secrets in a vault (Supabase Vault, env vars, or HSM)
2. **Agent code**: Agent writes `{{SECRET.GH_TOKEN}}` in its SQL or tool calls
3. **Runtime injection**: Harness replaces `{{SECRET.X}}` with real values before execution
4. **Response scrubbing**: Harness strips any real secret values from LLM responses using regex pattern matching against known secret values
5. **Audit**: Secret access is logged to `secret_access_audit` with timestamp, session_id, and alias used

```sql
CREATE TABLE secret_access_audit (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES sessions(id),
  secret_alias TEXT NOT NULL,
  accessed_at TIMESTAMPTZ DEFAULT now()
);
```

The agent knows *that* it used a secret, but never *what* the secret is. If the LLM somehow outputs a real secret value, the harness scrubs it before the response is stored or displayed.

## Contextual Permission Inheritance

An agent impersonates the security clearance of the user who owns the session. RLS policies evaluate queries as if the human ran them directly.

```sql
-- Sessions table: user context is set via consensus.user_id (SPEC-011 §9), not a direct user_id FK
CREATE TABLE sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  -- user context provided via SET LOCAL consensus.user_id; no direct user_id FK
  -- ...
);

-- At harness startup, set the session context
SET LOCAL consensus.session_id = 'session-uuid';
SET LOCAL consensus.user_id = 'user-uuid-here';
```

RLS policies then use `current_setting('consensus.user_id')` to enforce the same access rules that apply to the human:

```sql
CREATE POLICY user_data_scope ON project_resources
  FOR ALL
  USING (
    project_id IN (
      SELECT project_id FROM user_project_access
      WHERE user_id = current_setting('consensus.user_id')::UUID
    )
  );
```

This means:
- An agent running under Alice's session can only see Alice's projects
- An agent running under Bob's session can only see Bob's projects
- The agent has no way to escalate beyond the user's clearance

## Tool Ownership

Agents cannot modify tool definitions owned by other agents:

```sql
CREATE POLICY enforce_ownership ON tool_files
  FOR UPDATE
  USING (session_id = current_setting('consensus.session_id')::UUID);
```

This prevents:
- One agent rewriting another's tool to inject malicious behavior
- Accidental tool corruption from hallucinated UPDATE statements
- Privilege escalation through tool modification

## User ID Injection

The harness automatically appends the `user_id` from the sessions table to tool calls. The agent never sees the user ID and cannot tamper with it.

```
Agent outputs:
  { "system_actions": ["SELECT * FROM projects"] }

Harness rewrites to:
  SELECT * FROM projects WHERE user_id = 'user-uuid-from-session'
```

This is enforced at the harness layer, not at the LLM layer. The agent cannot opt out of user scoping.

## Security Model Summary

| Layer | Mechanism | Protects Against |
|---|---|---|
| Data isolation | RLS on every table | Cross-session data leaks, hallucinated DML |
| Data durability | Alt-Mode soft delete | Permanent data loss |
| Input integrity | Cognitive Firewall + quarantine | Prompt injection from external sources |
| Secret safety | Zero-knowledge aliases + scrubbing | Secret leakage through LLM output |
| Access control | Contextual permission inheritance | Privilege escalation |
| Tool integrity | Ownership RLS policy | Tool tampering |
| User scoping | Harness-level user_id injection | Cross-user data access |