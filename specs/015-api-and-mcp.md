# SPEC-015: API & External Interface Layer

**Status:** Draft
**Depends On:** SPEC-011 (Canonical Definitions), SPEC-009 (Deployment), SPEC-013 (Webhooks)
**Created:** 2026-04-08

---

## 1. Overview

Conscience needs external interfaces for humans to create sessions, monitor agents, review approvals, and integrate with other systems. This spec defines the REST API, real-time event streams, and MCP (Model Context Protocol) integration that allow external systems to interact with the agent runtime.

All external interfaces share a common principle: **they write to the same database the agent reads from.** There is no separate API state — the database is the single source of truth.

---

## 2. Authentication & Authorization

### 2.1 API Key Types

| Key Type | Scope | Role |
|---|---|---|
| `admin_key` | Full CRUD on all tables | `alt_mode_role` |
| `session_key` | CRUD within one session | `agent_role` |
| `readonly_key` | SELECT on all tables | Read-only monitoring |
| `webhook_key` | INSERT into external_events only | Webhook ingest |

### 2.2 Session Key Generation

When a session is created, a unique API key is generated:

```sql
CREATE TABLE api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash        TEXT NOT NULL UNIQUE, -- SHA-256 of the actual key
    key_prefix      TEXT NOT NULL,        -- First 8 chars for identification
    scope           TEXT NOT NULL CHECK (scope IN ('admin', 'session', 'readonly', 'webhook')),
    session_id      UUID REFERENCES sessions(id), -- NULL for admin/readonly/webhook
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);
```

The actual key is never stored — only the SHA-256 hash. Keys are prefixed with `cs_` (e.g., `cs_sk_a1b2c3d4e5f6...`).

### 2.3 Request Authentication

```typescript
// Middleware for all API routes
async function authenticateRequest(req: Request): Promise<AuthResult> {
    const key = req.headers.get('Authorization')?.replace('Bearer ', '');
    if (!key) return { authenticated: false, error: 'Missing API key' };

    const prefix = key.slice(0, 8); // cs_sk_a1
    const hash = sha256(key);

    const apiKey = await db.selectFrom('api_keys')
        .where('key_prefix', '=', prefix)
        .where('key_hash', '=', hash)
        .where('expires_at', '>', new Date())
        .selectAll()
        .executeSingle();

    if (!apiKey) return { authenticated: false, error: 'Invalid or expired API key' };

    // Set session context for RLS
    if (apiKey.session_id) {
        await db.execute(sql`SET LOCAL conscience.session_id = ${apiKey.session_id}`);
    }

    return {
        authenticated: true,
        scope: apiKey.scope,
        sessionId: apiKey.session_id
    };
}
```

### 2.4 Bootstrap Admin Key Expiry Policy

When the system bootstraps its first admin key (via `EnsureFirstAdminKey`), the key is created with a configurable time-to-live. This limits the exposure window for a secret that is printed to stdout and may leak into terminal scrollback, CI logs, or shell history.

**REQ-BOOTSTRAP-TTL-001: Configurable TTL**

The bootstrap admin key is inserted with `expires_at` set to `created_at + TTL`. The TTL is controlled by:

| Environment Variable | Default | Meaning |
|---|---|---|
| `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS` | `2160` (90 days) | Hours until the bootstrap key expires |

When `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS` is set to `0`, `expires_at` is set to `NULL`, meaning the key never expires. This preserves backward compatibility with deployments that rely on permanent bootstrap keys.

**REQ-BOOTSTRAP-TTL-002: Bootstrap-time visibility**

When a new bootstrap key is created, the stdout output (see SPEC-016 §3) MUST include the expiry timestamp so the operator knows when the key will stop working. If TTL is `0`, the output MUST state that the key does not expire.

**REQ-BOOTSTRAP-TTL-003: No change to auth middleware**

The existing auth middleware check — `(expires_at IS NULL OR expires_at > datetime('now'))` — enforces expiry for ALL keys uniformly. No middleware changes are required. Expired bootstrap keys are rejected identically to expired session or readonly keys.

**Operator guidance:**

1. After bootstrap, use the admin key to call `POST /api/v1/auth/keys` and create a replacement admin key with your preferred lifetime.
2. Optionally revoke the bootstrap key via `DELETE /api/v1/auth/keys/:id` immediately after creating a replacement.
3. The bootstrap key is a migration tool, not a long-lived credential.

**Realized by:**

- `internal/bootstrap/admin_key.go` — `EnsureFirstAdminKey` sets `expires_at` based on `CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS`
- `internal/api/server.go` — auth middleware enforces `expires_at` check (unchanged)
- `cmd/conscience/main.go` — calls `EnsureFirstAdminKey` during bootstrap

axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 plan= impl=internal/bootstrap/admin_key.go test=internal/bootstrap/admin_key_test.go

---

## 3. REST API Endpoints

### 3.1 Sessions

| Method | Path | Description | Scope |
|---|---|---|---|
| `POST` | `/api/v1/sessions` | Create a new agent session | admin |
| `GET` | `/api/v1/sessions` | List sessions (with filters) | admin, readonly |
| `GET` | `/api/v1/sessions/:id` | Get session details | admin, readonly, session |
| `PATCH` | `/api/v1/sessions/:id` | Update session (pause, resume, cancel) | admin, session |
| `DELETE` | `/api/v1/sessions/:id` | Soft-delete session (admin only) | admin |
| `POST` | `/api/v1/sessions/:id/message` | Send a message to the agent | session |

#### Create Session

```json
// POST /api/v1/sessions
{
    "agent_name": "research_agent",
    "goal": "Analyze Q4 revenue data and generate a summary report",
    "model_id": "gpt-4o",
    "context_budget": 128000,
    "hitl_config": {
        "require_approval_for_destructive": true
    }
}

// Response
{
    "id": "uuid-here",
    "status": "booting",
    "api_key": "cs_sk_a1b2c3d4...",
    "created_at": "2026-04-08T10:00:00Z"
}
```

#### Send Message

```json
// POST /api/v1/sessions/:id/message
{
    "content": "Focus on the international markets data",
    "type": "user_instruction"
}

// The message is inserted into memory_events and agent_messages
// If the session is 'idle', it transitions to 'thinking'
// If 'paused', it queues for the next iteration
```

### 3.2 Memory & Context

| Method | Path | Description | Scope |
|---|---|---|---|
| `GET` | `/api/v1/sessions/:id/memory` | List memory events for session | session, admin, readonly |
| `GET` | `/api/v1/sessions/:id/context` | Get active context (rendered) | session, admin |
| `GET` | `/api/v1/sessions/:id/iterations` | List iteration commits | session, admin, readonly |
| `GET` | `/api/v1/sessions/:id/memory/:mid` | Get single memory event | session, admin |

### 3.3 Tasks

| Method | Path | Description | Scope |
|---|---|---|---|
| `POST` | `/api/v1/sessions/:id/tasks` | Create a task | session, admin |
| `GET` | `/api/v1/sessions/:id/tasks` | List tasks | session, admin, readonly |
| `PATCH` | `/api/v1/tasks/:tid` | Update task status | session, admin |
| `POST` | `/api/v1/tasks/:tid/claim` | Claim a task | session |

### 3.4 Tools & Skills

| Method | Path | Description | Scope |
|---|---|---|---|
| `GET` | `/api/v1/tools` | List available tools | session, admin, readonly |
| `GET` | `/api/v1/skills` | List skill metadata | session, admin, readonly |
| `GET` | `/api/v1/skills/:name` | Get full skill instructions | session, admin |
| `POST` | `/api/v1/tools/:name/execute` | Execute an internal tool | session |

### 3.5 Approvals (HITL)

| Method | Path | Description | Scope |
|---|---|---|---|
| `GET` | `/api/v1/approvals` | List pending approvals | admin |
| `GET` | `/api/v1/approvals/:id` | Get approval details | admin |
| `POST` | `/api/v1/approvals/:id/review` | Approve/reject/modify | admin |
| `GET` | `/api/v1/sessions/:id/approvals` | List approvals for session | session, admin |

### 3.6 Billing & Metrics

| Method | Path | Description | Scope |
|---|---|---|---|
| `GET` | `/api/v1/sessions/:id/billing` | Get cost breakdown | admin, readonly |
| `GET` | `/api/v1/metrics` | System-wide metrics | admin, readonly |
| `GET` | `/api/v1/sessions/:id/iterations/:iid/audit` | Get iteration audit log | admin, readonly |

### 3.7 Configuration & System

| Method | Path | Description | Scope |
|---|---|---|---|
| `GET` | `/api/v1/config` | Get system configuration (redacted) | admin |
| `PATCH` | `/api/v1/config` | Update system settings | admin |
| `GET` | `/api/v1/config/models` | List model registry entries | admin, readonly |
| `GET` | `/api/v1/health` | Health check (no auth required) | public |

### 3.8 Authentication Management

| Method | Path | Description | Scope |
|---|---|---|---|
| `POST` | `/api/v1/auth/keys` | Create a new API key | admin |
| `GET` | `/api/v1/auth/keys` | List API keys (prefix + scope only) | admin |
| `DELETE` | `/api/v1/auth/keys/:id` | Revoke an API key | admin |

---

## 4. Real-Time Event Streams

### 4.1 Supabase: Postgres LISTEN/NOTIFY

```sql
-- Notify on session status changes
CREATE OR REPLACE FUNCTION notify_session_change()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify(
        'session_events',
        json_build_object(
            'session_id', NEW.id,
            'status', NEW.status,
            'iteration', NEW.iteration,
            'timestamp', now()
        )::text
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER session_status_notify
    AFTER UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION notify_session_change();

-- Notify on approval requests
CREATE OR REPLACE FUNCTION notify_approval_request()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify(
        'approval_events',
        json_build_object(
            'approval_id', NEW.id,
            'session_id', NEW.session_id,
            'request_type', NEW.request_type,
            'risk_level', NEW.risk_level,
            'description', NEW.description,
            'timestamp', now()
        )::text
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER approval_request_notify
    AFTER INSERT ON approval_requests
    FOR EACH ROW EXECUTE FUNCTION notify_approval_request();
```

Supabase Realtime provides WebSocket subscriptions for frontend dashboards.

### 4.2 Server-Sent Events

The Go binary serves SSE for all real-time updates, regardless of database backend:

```go
func HandleSSE(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    sub := eventBus.Subscribe(r.URL.Query().Get("session_id"))
    defer sub.Close()

    for {
        select {
        case event := <-sub.Ch():
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

On Postgres, the event bus is powered by LISTEN/NOTIFY. On SQLite, it uses Go channels populated by database change hooks.

---

## 5. MCP Integration

### 5.1 What is MCP?

The Model Context Protocol (MCP) is an open standard for connecting AI models to external data sources and tools. Conscience exposes its agent runtime as an MCP server, allowing any MCP-compatible client (Claude Desktop, IDE plugins, etc.) to create and interact with agents.

### 5.2 MCP Server Definition

```json
{
    "name": "conscience",
    "version": "0.1.0",
    "description": "Database-native cognitive architecture for AI agents",
    "tools": [
        {
            "name": "create_session",
            "description": "Create a new agent session",
            "parameters": {
                "type": "object",
                "properties": {
                    "agent_name": { "type": "string", "description": "Name for the agent" },
                    "goal": { "type": "string", "description": "The task for the agent to accomplish" },
                    "model_id": { "type": "string", "description": "LLM model to use" }
                },
                "required": ["agent_name", "goal"]
            }
        },
        {
            "name": "send_message",
            "description": "Send a message to a running agent",
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": { "type": "string" },
                    "message": { "type": "string" }
                },
                "required": ["session_id", "message"]
            }
        },
        {
            "name": "get_session_status",
            "description": "Get the current status of an agent session",
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": { "type": "string" }
                },
                "required": ["session_id"]
            }
        },
        {
            "name": "list_memory",
            "description": "List memory events for a session",
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": { "type": "string" },
                    "limit": { "type": "integer", "description": "Max events to return" }
                },
                "required": ["session_id"]
            }
        },
        {
            "name": "review_approval",
            "description": "Approve or reject a pending HITL approval request",
            "parameters": {
                "type": "object",
                "properties": {
                    "approval_id": { "type": "string" },
                    "decision": { "type": "string", "enum": ["approved", "rejected", "modified"] },
                    "notes": { "type": "string" },
                    "modified_sql": { "type": "string" }
                },
                "required": ["approval_id", "decision"]
            }
        },
        {
            "name": "query_tool",
            "description": "Execute an internal (SQL) tool registered in tools_registry",
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": { "type": "string", "description": "Session to execute within" },
                    "tool_name": { "type": "string", "description": "Name from tools_registry" },
                    "parameters": {
                        "type": "object",
                        "description": "Tool-specific parameters matching the tool's parameter_schema",
                        "additionalProperties": true
                    }
                },
                "required": ["session_id", "tool_name"]
            }
        }
    ],
    "resources": [
        {
            "name": "sessions",
            "description": "Active agent sessions",
            "uri": "conscience://sessions"
        },
        {
            "name": "session_context",
            "description": "Active context view for a session",
            "uri": "conscience://sessions/{session_id}/context"
        },
        {
            "name": "tools_registry",
            "description": "Available tools and skills",
            "uri": "conscience://tools"
        }
    ],
    "prompts": [
        {
            "name": "agent_status",
            "description": "Get a summary of what an agent is currently doing",
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": { "type": "string" }
                },
                "required": ["session_id"]
            }
        }
    ]
}
```

### 5.3 MCP Authentication

MCP clients authenticate using the same API key mechanism as the REST API (§2). The key is passed as a Bearer token in the MCP initialization handshake:

```json
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": { "name": "claude-desktop", "version": "1.0" },
    "_meta": {
      "authorization": "Bearer cs_sk_a1b2c3d4..."
    }
  }
}
```

The MCP server validates the key against `api_keys` table (§2.2) and enforces the same scope restrictions. A `session`-scoped key restricts MCP operations to that session. An `admin`-scoped key allows full access.

For stdio transport (PocketBase local), authentication is optional — the MCP server trusts the local process. This is equivalent to PostgreSQL peer authentication for local sockets.

### 5.4 MCP Transport

| Transport | Implementation |
|---|---|
| SSE (Server-Sent Events) | Go HTTP handler at `/mcp/sse` |
| stdio | Go MCP server, for local CLI integration |

### 5.4 MCP Resource Templates

Resources allow MCP clients to read agent state without needing specific tools:

```typescript
// Resource handler
async function readResource(uri: string): Promise<string> {
    if (uri.startsWith('conscience://sessions/') && uri.endsWith('/context')) {
        const sessionId = uri.split('/')[3];
        const context = await db.selectFrom('active_context_view')
            .where('session_id', '=', sessionId)
            .selectAll()
            .execute();
        return formatAsMarkdown(context);
    }
    if (uri === 'conscience://sessions') {
        const sessions = await db.selectFrom('sessions')
            .where('status', 'in', ['idle', 'thinking', 'tool_exec', 'waiting_sub', 'paused'])
            .select(['id', 'agent_name', 'status', 'goal', 'iteration'])
            .execute();
        return JSON.stringify(sessions, null, 2);
    }
    if (uri === 'conscience://tools') {
        const tools = await db.selectFrom('tools_registry')
            .where('enabled', '=', true)
            .select(['name', 'description', 'hemisphere', 'handler_type'])
            .execute();
        return JSON.stringify(tools, null, 2);
    }
    throw new Error(`Unknown resource: ${uri}`);
}
```

---

## 6. Error Responses

All API errors follow a consistent format:

```json
{
    "error": {
        "code": "SESSION_NOT_FOUND",
        "message": "Session with id 'abc-123' does not exist or you do not have access",
        "details": {}
    }
}
```

| HTTP Code | Code | Description |
|---|---|---|
| 400 | `INVALID_REQUEST` | Malformed JSON, missing required field |
| 401 | `UNAUTHENTICATED` | Missing or invalid API key |
| 403 | `FORBIDDEN` | API key scope doesn't permit this action |
| 404 | `NOT_FOUND` | Resource doesn't exist |
| 409 | `CONFLICT` | Session in wrong state for this action |
| 429 | `RATE_LIMITED` | Too many requests |
| 500 | `INTERNAL_ERROR` | Unexpected server error |

### Session State Conflicts

```json
// PATCH /api/v1/sessions/:id — when session is paused and action requires idle
{
    "error": {
        "code": "SESSION_PAUSED",
        "message": "Session is paused awaiting human approval. Use /approvals to review.",
        "details": {
            "session_status": "paused",
            "pending_approvals": ["uuid-approval-1"]
        }
    }
}
```

---

## 7. Rate Limiting

### 7.1 Per-Key Limits

```sql
CREATE TABLE api_rate_limits (
    key_prefix       TEXT PRIMARY KEY,
    requests_count   INT NOT NULL DEFAULT 0,
    window_start     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Default limits:
- `admin_key`: 1000 req/min
- `session_key`: 100 req/min
- `readonly_key`: 200 req/min
- `webhook_key`: 500 req/min

### 7.2 Implementation

Rate limiting is enforced at the middleware layer before database queries:

```typescript
async function rateLimitCheck(keyPrefix: string, limit: number): Promise<boolean> {
    const result = await db.selectFrom('api_rate_limits')
        .where('key_prefix', '=', keyPrefix)
        .selectAll()
        .executeSingle();

    if (!result || result.window_start < new Date(Date.now() - 60000)) {
        // Reset window
        await db.insertInto('api_rate_limits')
            .values({ key_prefix: keyPrefix, requests_count: 1, window_start: new Date() })
            .onConflict((oc) => oc.column('key_prefix').doUpdate())
            .execute();
        return true;
    }

    if (result.requests_count >= limit) return false;

    await db.updateTable('api_rate_limits')
        .set({ requests_count: result.requests_count + 1 })
        .where('key_prefix', '=', keyPrefix)
        .execute();

    return true;
}
```

---

## 8. PocketBase Parity

| Feature | Postgres Backend | SQLite Backend |
|---|---|---|
| REST API | Go HTTP router (net/http or chi) | Same — shared code |
| Auth | API key table + middleware | Same — shared code |
| Real-time | Postgres LISTEN/NOTIFY → SSE | Go goroutines → SSE |
| MCP Server | SSE over HTTP | Same — shared code |
| Rate limiting | Go middleware | Same — shared code |
| API Keys | `api_keys` table + middleware | Same — shared code |

All interface code lives in the Go binary. The database backend only affects data storage and enforcement mechanisms (RLS vs Go hooks), not the API surface.

---

## 9. Open Questions

1. **Pagination**: What pagination strategy for list endpoints? Cursor-based (stable for real-time data) or offset-based (simpler)?
2. **Batch operations**: Should there be batch endpoints (e.g., create multiple tasks in one request)?
3. **API versioning**: URL-based (`/api/v1/`) vs header-based (`Accept: application/vnd.conscience.v1+json`)?
4. **WebSocket vs SSE**: Should the real-time stream use WebSocket (bidirectional) or SSE (simpler, unidirectional)?
5. **CORS**: What origins should be allowed for browser-based dashboards?