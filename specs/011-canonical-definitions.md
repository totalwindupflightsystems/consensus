# SPEC-011: Canonical Definitions — Cross-Spec Reconciliation

**Status:** Draft
**Purpose:** Authoritative definitions for all terms, state machines, schemas, and namespaces that were inconsistent across SPEC-001 through SPEC-010
**Supersedes:** Contradictory definitions in earlier specs — where this spec differs, THIS SPEC WINS
**Created:** 2026-04-08

---

## 1. Session Status State Machine

### 1.1 Canonical States

The `sessions.status` column uses these values:

```sql
CHECK (status IN (
    'booting',       -- Session created, context not yet loaded
    'idle',          -- Waiting for work (no active iteration)
    'planning',      -- Interactive transaction open — staging, executing, inspecting (SPEC-020)
    'thinking',      -- Harness sent context to LLM, awaiting response
    'tool_exec',     -- LLM requested tools, harness executing externally
    'executing',     -- Agent called commit — applying final staged changes (SPEC-020)
    'waiting_sub',   -- Sub-agent(s) spawned, parent paused
    'paused',        -- Human-in-the-loop pause (SPEC-014 HITL)
    'completed',     -- Goal achieved, session ended
    'failed'         -- Unrecoverable error or circuit breaker tripped
))
```

### 1.2 Valid Transitions

```
booting ──► idle
idle ──► thinking            (one-shot model, SPEC-008)
idle ──► planning            (interactive transaction model, SPEC-020)
thinking ──► idle            (no state changes needed)
thinking ──► tool_exec       (LLM requested external tool)
thinking ──► waiting_sub     (LLM spawned sub-agent)
thinking ──► completed       (LLM signals task done)
thinking ──► failed          (circuit breaker, max errors)
planning ──► tool_exec       (external tool call during planning)
planning ──► executing       (agent called commit, applying final changes)
planning ──► idle            (rollback + end, or respond without changes)
planning ──► failed          (unrecoverable error during planning)
tool_exec ──► thinking       (tool result injected, continue one-shot loop)
tool_exec ──► planning       (tool result injected, continue planning loop)
tool_exec ──► failed         (tool timeout, unrecoverable)
executing ──► idle           (commit succeeded)
executing ──► failed         (commit failed, error)
waiting_sub ──► idle         (all sub-agents completed, trigger wakes parent)
waiting_sub ──► failed       (sub-agent failed, parent fails too)
idle ──► paused              (HITL interrupt)
paused ──► idle              (HITL resume)
```

### 1.3 Why These States (Not READY/SLEEPING/RUNNING)

SPEC-004 and SPEC-008 used ALL-CAPS names (READY, SLEEPING, RUNNING). The canonical set uses lowercase because:
- PostgreSQL `CHECK` constraints and `current_setting` values are case-sensitive text
- SPEC-003 already defined these exact states with lowercase values
- The semantics map cleanly: `READY` → `idle`, `SLEEPING` → `waiting_sub`, `RUNNING` → `thinking`+`tool_exec`
- The split of `RUNNING` into `thinking` and `tool_exec` is critical: tool execution happens OUTSIDE the main transaction (§7)

### 1.4 Migration from Spec-004/008 Terminology

| Old Term (SPEC-004/008) | Canonical Term | Table |
|---|---|---|
| READY | idle | sessions.status |
| RUNNING | thinking or tool_exec | sessions.status |
| SLEEPING | waiting_sub | sessions.status |
| COMPLETED | completed | sessions.status |
| FAILED | failed | sessions.status |

---

## 2. Task Status State Machine

### 2.1 Canonical States

```sql
CHECK (status IN (
    'pending',       -- Created, not yet claimed
    'claimed',       -- Agent locked the task
    'in_progress',   -- Agent actively working
    'reviewed',      -- Output reviewed (required before published)
    'published',     -- Output finalized, available to consumers
    'failed',        -- Unrecoverable error
    'cancelled'      -- Deliberately cancelled
))
```

### 2.2 Valid Transitions

```
pending ──► claimed          (agent locks task via locked_by_agent)
claimed ──► in_progress      (agent begins work)
in_progress ──► reviewed     (self-review or sub-agent review)
in_progress ──► failed       (error or circuit breaker)
reviewed ──► published       (final approval, irreversible)
reviewed ──► failed          (review rejected)
published ──► (terminal)     (cannot revert — §5.1 trigger enforces)
pending ──► cancelled        (cancelled before starting)
claimed ──► cancelled        (cancelled after claiming)
```

### 2.3 Why Not READY/RUNNING/COMPLETED

Tasks are NOT sessions. Tasks have a publish workflow with gatekeeping. The `sessions` state machine tracks the runtime loop; the `tasks` state machine tracks the work lifecycle. They are separate concerns.

---

## 3. Display Mode Resolution (Append-Only Fix)

### 3.1 The Contradiction

SPEC-002 §2.1 states `memory_events` is "never updated and never deleted." SPEC-002 §8.3 and §3.4 show `UPDATE memory_events SET display_mode = 'compressed'` — a direct mutation of the append-only ledger. SPEC-003 §2.2 also defines `display_mode` as a column on `memory_events`.

### 3.2 Canonical Resolution: Separate `display_modes` Table

The `display_mode` for each memory event is stored in a **separate table** that IS mutable, while `memory_events` remains truly append-only (INSERT/SELECT only for agent_role).

```sql
CREATE TABLE display_modes (
    memory_id       BIGINT NOT NULL REFERENCES memory_events(id) ON DELETE CASCADE,
    mode            TEXT NOT NULL DEFAULT 'full'
                    CHECK (mode IN ('full', 'compressed', 'hidden')),
    set_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    set_by_iteration BIGINT NOT NULL,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    PRIMARY KEY (memory_id)
);
```

### 3.3 Updated Append-Only Enforcement

```sql
-- memory_events: true append-only. No UPDATE, no DELETE for agent_role.
REVOKE UPDATE, DELETE ON memory_events FROM agent_role;

-- display_modes: fully mutable. Agent can change display state independently.
GRANT SELECT, INSERT, UPDATE ON display_modes TO agent_role;
```

### 3.4 Updated Active Context View

The view JOINs `display_modes` instead of reading from `memory_events.display_mode`:

```sql
CREATE OR REPLACE VIEW active_context_view AS
WITH active_ids AS (
    SELECT unnest(active_pointers) AS ptr_id
    FROM iteration_commits
    WHERE session_id = current_setting('conscience.session_id')::UUID
    ORDER BY iteration_id DESC
    LIMIT 1
)
SELECT
    me.id,
    me.iteration_created,
    me.type,
    COALESCE(dm.mode, 'full') AS display_mode,
    CASE
        WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
            THEN me.summary_text
        WHEN COALESCE(dm.mode, 'full') = 'hidden'
            THEN NULL
        ELSE me.content
    END AS rendered_text
FROM memory_events me
JOIN active_ids ai ON me.id = ai.ptr_id
LEFT JOIN display_modes dm ON dm.memory_id = me.id
WHERE COALESCE(dm.mode, 'full') != 'hidden'
    AND me.session_id = current_setting('conscience.session_id')::UUID
ORDER BY me.iteration_created, me.id;
```

## 3.5 Canonical memory_events.type Values

The `memory_events.type` CHECK constraint uses these values:

```sql
CHECK (type IN (
    'header',             -- Session start / context header
    'text_block',         -- Agent text output
    'tool_call',          -- Outbound tool invocation
    'tool_result',        -- Tool execution result
    'thinking',           -- Agent internal monologue / chain-of-thought
    'system',             -- System-injected message (error, notification, config change)
    'inherited_pointer',  -- Forked compressed pointer from parent (SPEC-004 §Memory Forking)
    'user_message'        -- Human input via API/message endpoint (SPEC-015 §3.1)
))
```

### 3.5.1 Why These Types

| Type | Source | Purpose |
|---|---|---|
| `header` | SPEC-002 §2 | Session metadata header |
| `text_block` | SPEC-002 §2 | Standard agent text output |
| `tool_call` | SPEC-002 §2, SPEC-008, SPEC-020 | Tool invocation record |
| `tool_result` | SPEC-002 §2, SPEC-008 | Tool execution response |
| `thinking` | SPEC-002 §2 | Agent reasoning trace |
| `system` | SPEC-002 §2 | System messages |
| `inherited_pointer` | SPEC-004 §Memory Forking | Forked compressed pointers from parent to child |
| `user_message` | SPEC-015 §3.1 (POST /sessions/:id/message) | Human-to-agent messages |

All specs referencing `memory_events.type` must use these canonical values. The CHECK constraint in SPEC-003 §2.2 and SPEC-002 §2.2 must include all 8 types.

---

### 3.6 Compression Still Writes to memory_events.summary_text

The compression loop (SPEC-002 §8) writes `summary_text` to `memory_events`. This IS an UPDATE — but it is performed by a **system role** (compression worker), NOT the `agent_role`. The agent never updates `memory_events`. The compression worker has elevated privileges for `summary_text` only:

```sql
-- Compression worker can update summary_text only
CREATE ROLE compression_worker;
GRANT UPDATE (summary_text) ON memory_events TO compression_worker;
-- Still cannot update content, display_mode, or any other column
```

### 3.7 Migration: Remove display_mode from memory_events

The `display_mode` column is removed from `memory_events`:

```sql
ALTER TABLE memory_events DROP COLUMN display_mode;
```

Instead, all display state lives in `display_modes`. Default is `'full'` (no row = full display).

---

## 4. Unified Tool & Skill Registry

### 4.1 The Contradiction

Three different schemas across specs:
- SPEC-003 §2.8 `skills_registry`: id, name, description, hemisphere, parameter_schema, handler_type, handler_ref, enabled, requires_approval, rate_limit_per_min
- SPEC-010 `skills_registry`: id, skill_name, metadata, instructions, linked_tools
- SPEC-010 `tools_registry`: id, name, owner_agent_id, status (ACTIVE/TESTING/DEPRECATED/DISABLED), hemisphere

### 4.2 Canonical Resolution: Two Tables, Clear Separation

**Tools** are executable capabilities (SQL functions, HTTP endpoints, Go handlers, subprocesses). **Skills** are knowledge bundles (instructions, workflows, progressive disclosure). They are different things and live in different tables.

#### 4.2.1 tools_registry (Canonical)

```sql
CREATE TABLE tools_registry (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    description       TEXT NOT NULL,
    hemisphere        TEXT NOT NULL CHECK (hemisphere IN ('internal', 'external')),
    parameter_schema  JSONB NOT NULL DEFAULT '{}',
    handler_type      TEXT NOT NULL CHECK (handler_type IN (
                        'sql_function', 'http_endpoint', 'go_native', 'subprocess'
                     )),
    handler_ref       TEXT NOT NULL,
    owner_session_id  UUID REFERENCES sessions(id),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'testing', 'deprecated', 'disabled')),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    rate_limit_per_min INT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

This merges SPEC-003's operational fields (handler_type, handler_ref, rate_limit_per_min, requires_approval) with SPEC-010's governance fields (owner_session_id, status).

#### 4.2.2 skills_registry (Canonical)

```sql
CREATE TABLE skills_registry (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    metadata          JSONB NOT NULL,
    instructions      TEXT NOT NULL,
    linked_tool_ids   UUID[] NOT NULL DEFAULT '{}',
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

This merges SPEC-003's `name`/`description` into `metadata` (which already includes description) and keeps SPEC-010's `instructions` + `linked_tools` pattern. `linked_tool_ids` references `tools_registry.id` (validated at application layer since UUID[] can't FK).

#### 4.2.3 Why Two Tables

- `tools_registry` answers: "What can I EXECUTE?" (query by handler_type, hemisphere)
- `skills_registry` answers: "What do I KNOW?" (progressive disclosure — metadata cheap, instructions expensive)
- A skill references tools via `linked_tool_ids`. A tool has no skill references.
- The old `skills_registry` in SPEC-003 was really a tools registry misnamed. Renamed to match function.

---

## 5. Unified custom_agent_tools Schema

### 5.1 The Contradiction

- SPEC-003 §2.11: id, session_id, name, language, source_code, parameter_schema, approved, UNIQUE(name, session_id)
- SPEC-010: id, tool_name, ts_code, created_by, unique on tool_name globally

### 5.2 Canonical Schema

```sql
CREATE TABLE custom_agent_tools (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_session_id UUID NOT NULL REFERENCES sessions(id),
    name              TEXT NOT NULL,
    language          TEXT NOT NULL CHECK (language IN ('javascript', 'typescript', 'sql', 'python', 'go')),
    source_code       TEXT NOT NULL,
    parameter_schema  JSONB NOT NULL DEFAULT '{}',
    approved          BOOLEAN NOT NULL DEFAULT false,
    status            TEXT NOT NULL DEFAULT 'draft'
                      CHECK (status IN ('draft', 'testing', 'active', 'deprecated')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(name)      -- Globally unique tool name (prevents namespace collision)
);
```

### 5.3 Design Decisions

- **UNIQUE(name)** globally, not per-session. Tool names are identifiers that other agents reference. Two agents creating `zillow_scraper` is a collision, not independent evolution.
- **creator_session_id** (not `created_by` / generic `agents` FK) because sessions are the identity unit in this system (SPEC-003 §2.1).
- **language** supports all runtime targets, not just TypeScript. SQL tools are internal hemisphere; TS/JS/Python/Go are external.
- **status** field replaces the separate testing/active flow from SPEC-010 CI/CD — this is the governance column.
- **approved** is separate from status: `approved = true` is an Alt-Mode admin gate. `status = 'active'` means tests passed. Both must be true for the tool to be available.

---

## 6. Unified agent_billing Schema

### 6.1 The Contradiction

- SPEC-003 §2.9: id, session_id, model_id, tokens_in, tokens_out, cost_usd, category, recorded_at
- SPEC-006 §: id, session_id, iteration, model, prompt_tokens, completion_tokens, total_tokens, estimated_cost_cents, created_at

### 6.2 Canonical Schema

```sql
CREATE TABLE agent_billing (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration         BIGINT NOT NULL,
    model_id          TEXT NOT NULL,
    category          TEXT NOT NULL CHECK (category IN (
                        'cognition', 'compression', 'embedding', 'tool_call'
                     )),
    prompt_tokens     BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    cost_usd          NUMERIC(12,6) NOT NULL DEFAULT 0,
    recorded_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.3 Design Decisions

- Uses BIGSERIAL (from SPEC-003) not UUID — billing rows are append-only ledger entries, monotonic IDs are appropriate.
- **iteration** (from SPEC-006) is required — you need to know which iteration generated the cost.
- **model_id** (from SPEC-003, TEXT) not `model` — references model_registry's model_id column consistently.
- **category** (from SPEC-003) — critical for understanding cost breakdown (main loop vs compression vs embeddings).
- **cache_read_tokens / cache_write_tokens** — NEW. Both Anthropic and OpenAI return cache-specific usage metadata. This replaces the vague "total_tokens" and enables cache hit rate analytics.
- **cost_usd** NUMERIC(12,6) (from SPEC-003) not `estimated_cost_cents` INT — USD with 6 decimal places supports micro-cent granularity for cheap models.
- No `total_tokens` column — it's derivable as `prompt_tokens + completion_tokens`.

---

## 7. Tool Execution Flow (Outside Main Transaction)

### 7.1 The Contradiction

SPEC-008 § shows tool execution INSIDE the SQL transaction:
```typescript
for (const toolReq of parsed.tool_requests ?? []) {
    await executeToolRequest(tx, toolReq, sessionId);
}
```

But the Gemini conversation (Turn 13) explicitly warns against this: tool execution can take seconds/minutes and must NOT block the transaction. If a tool times out while inside `BEGIN...COMMIT`, the transaction holds locks and can deadlock.

### 7.2 Canonical Flow: Two-Phase Execution

The iteration is split into TWO transactions:

**Phase 1: Cognition Transaction (fast, sub-second)**

```typescript
async function cognitionPhase(sessionId: string, llmResponse: AgentResponse) {
    const tx = await db.beginTransaction();
    try {
        // Memory state changes (display modes, memory pages, etc.)
        for (const sql of llmResponse.memory_state_changes ?? []) {
            await tx.execute(parameterize(sql));
        }
        // System actions (create tables, call stored procedures)
        for (const sql of llmResponse.system_actions ?? []) {
            await tx.execute(parameterize(sql));
        }
        // Write tool requests to tool_requests table (pending status)
        for (const toolReq of llmResponse.tool_requests ?? []) {
            await tx.execute(sql`
                INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
                VALUES (${sessionId}, ${iterationId}, ${toolReq.tool_name}, ${JSON.stringify(toolReq.parameters)}, 'pending')
            `);
        }
        // Write sub-agent spawn requests to tasks table
        for (const spawn of llmResponse.sub_agent_spawns ?? []) {
            await tx.execute(sql`
                INSERT INTO tasks (session_id, title, description, status)
                VALUES (${sessionId}, ${spawn.instruction}, ${spawn.instruction}, 'pending')
            `);
        }
        // Update session status
        const hasTools = (llmResponse.tool_requests?.length ?? 0) > 0;
        const hasSubs = (llmResponse.sub_agent_spawns?.length ?? 0) > 0;
        const newStatus = hasTools ? 'tool_exec' : hasSubs ? 'waiting_sub' : 'idle';
        await tx.execute(sql`
            UPDATE sessions SET status = ${newStatus}, heartbeat_at = now() WHERE id = ${sessionId}
        `);
        await tx.commit();
    } catch (error) {
        await tx.rollback();
        throw error;
    }
}
```

**Phase 2: Tool Execution (async, long-running, no transaction)**

```typescript
async function toolExecutionPhase(sessionId: string, iterationId: bigint) {
    // Read pending tool requests (committed in Phase 1)
    const requests = await db.selectFrom('tool_requests')
        .where('session_id', '=', sessionId)
        .where('iteration_id', '=', iterationId)
        .where('status', '=', 'pending')
        .selectAll()
        .execute();

    for (const req of requests) {
        try {
            const result = await executeToolSandboxed(req);

            // Write result in its OWN mini-transaction
            const tx = await db.beginTransaction();
            await tx.execute(sql`
                UPDATE tool_requests SET status = 'completed', completed_at = now()
                WHERE id = ${req.id}
            `);
            await tx.execute(sql`
                INSERT INTO tool_results (request_id, session_id, output, is_error, token_count)
                VALUES (${req.id}, ${sessionId}, ${result.output}, false, ${result.tokenCount})
            `);
            await tx.commit();
        } catch (error) {
            const tx = await db.beginTransaction();
            await tx.execute(sql`
                UPDATE tool_requests SET status = 'failed', completed_at = now()
                WHERE id = ${req.id}
            `);
            await tx.execute(sql`
                INSERT INTO tool_results (request_id, session_id, output, is_error, error_code)
                VALUES (${req.id}, ${sessionId}, ${error.message}, true, 'TOOL_ERROR')
            `);
            await tx.commit();
        }
    }

    // After all tools complete, transition session back to thinking
    await db.execute(sql`
        UPDATE sessions SET status = 'thinking', heartbeat_at = now()
        WHERE id = ${sessionId} AND status = 'tool_exec'
    `);
}
```

### 7.3 Why Two-Phase

1. **No long-running locks**: The cognition transaction completes in milliseconds
2. **Tool failures are isolated**: One tool failure doesn't roll back memory state changes
3. **Tool results are individual transactions**: Each tool result is committed independently
4. **Session status is accurate**: `tool_exec` persists while tools run, `thinking` resumes after
5. **Timeout safety**: If a tool hangs and the process crashes, the session status is `tool_exec`, and pg_cron reaps it as stale

### 7.4 Sub-Agent Spawning

Sub-agents are spawned in Phase 1 (session status → `waiting_sub`). The parent does NOT execute a Phase 2. The parent waits for the trigger to wake it when all sub-agents complete.

---

## 8. SQL Injection Mitigation

### 8.1 The Risk

SPEC-008 § shows `tx.execute(injectSecrets(sql, sessionId))` where `sql` is raw text from LLM output embedded directly in `execute()`. If the LLM generates:

```sql
DELETE FROM memory_events; -- oops
```

It executes directly. RLS limits blast radius, but doesn't prevent all damage (agent can still delete its own data).

### 8.2 Multi-Statement Splitting (NEW)

LLM output may contain multiple SQL statements separated by semicolons in a single string. This creates an injection risk: a string starting with `SELECT` could contain a `DELETE` after the semicolon, bypassing the classifier.

The harness **splits all SQL strings on semicolons before classification**. Each resulting statement is classified independently:

```typescript
function splitStatements(statements: string[]): string[] {
    const result: string[] = [];
    for (const stmt of statements) {
        const parts = stmt.split(';').map(s => s.trim()).filter(s => s.length > 0);
        for (const part of parts) {
            result.push(part);
        }
    }
    return result;
}
```

Example: `"SELECT * FROM mem; DELETE FROM tasks"` is split into `["SELECT * FROM mem", "DELETE FROM tasks"]`, each classified independently as `DML_READ` and `DML_DELETE`.

This applies to all three execution tiers (SPEC-008 §SQL Execution Model).

### 8.3 Canonical Mitigation: Statement Classifier + Whitelist + Stored Procs

**Layer 1: Statement Classification** (applied after multi-statement splitting per §8.2)

The harness classifies each SQL statement from LLM output before execution:

```typescript
type StatementClass = 'DML_WRITE' | 'DML_READ' | 'DDL_CREATE' | 'DDL_ALTER' | 'DML_DELETE' | 'DANGEROUS';

function classifyStatement(sql: string): StatementClass {
    const normalized = sql.trim().toUpperCase();

    // Block: DROP, TRUNCATE, GRANT, REVOKE, CREATE ROLE, ALTER ROLE
    if (/^(DROP|TRUNCATE|GRANT|REVOKE|CREATE\s+ROLE|ALTER\s+ROLE)/.test(normalized)) {
        return 'DANGEROUS';
    }
    if (/^DELETE\s+FROM/i.test(normalized)) {
        return 'DML_DELETE';
    }
    if (/^(INSERT|UPDATE)\s+/i.test(normalized)) {
        return 'DML_WRITE';
    }
    if (/^SELECT/i.test(normalized)) {
        return 'DML_READ';
    }
    if (/^CREATE\s+TABLE/i.test(normalized)) {
        return 'DDL_CREATE';
    }
    if (/^ALTER\s+TABLE/i.test(normalized)) {
        return 'DDL_ALTER';
    }
    return 'DANGEROUS'; // Unknown = blocked
}
```

**Layer 2: Execution Policy per Statement Class**

| Class | Allowed? | Constraints |
|---|---|---|
| `DML_READ` | Yes | No restrictions |
| `DML_WRITE` | Yes | Must target allowed tables only (core + dynamic agent tables) |
| `DML_DELETE` | Restricted | Soft-delete only via stored procedures (SPEC-003 §9.1) |
| `DDL_CREATE` | Yes | Must go through `create_agent_memory_table()` function |
| `DDL_ALTER` | Restricted | Only ADD CONSTRAINT (JSON Schema locks). No DROP COLUMN, no DROP TABLE |
| `DANGEROUS` | Blocked | Never executed. Error injected into context |

**Layer 3: Allowed Table Whitelist for DML_WRITE**

```typescript
const CORE_TABLES = new Set([
    'memory_events', 'display_modes', 'iteration_commits', 'memory_pages',
    'tasks', 'tool_requests', 'tool_results', 'sessions',
    'custom_agent_tools', 'tools_registry', 'skills_registry',
    'agent_billing', 'workflows', 'tool_files', 'external_quarantine',
    'compression_queue', 'model_registry'
]);

function isTableAllowed(sql: string, dynamicTables: Set<string>): boolean {
    const tables = extractTableNames(sql); // Regex extraction of FROM/INTO/UPDATE targets
    return tables.every(t => CORE_TABLES.has(t) || dynamicTables.has(t));
}
```

Dynamic tables are discovered by querying `information_schema.tables` for non-core tables at session start.

**Layer 4: Stored Procedure Preference**

When the target table is `memory_events`, `tasks`, or `sessions`, the harness PREFERENTIALLY routes to stored procedures. This is the **Tier 1** execution mode from SPEC-008 §SQL Execution Model. Lower-trust models are restricted to Tier 1 (stored procedures only); higher-trust models can use Tier 2 (parameterized) or Tier 3 (raw SQL with classifier + whitelist).

```sql
-- Instead of: UPDATE memory_events SET display_mode = 'compressed' WHERE id = 104
-- Agent should call: SELECT set_display_mode(104, 'compressed')
-- Instead of: UPDATE sessions SET status = 'completed' WHERE id = $1
-- Agent should call: SELECT complete_session($1)
```

The system prompt instructs the agent to use stored procedures. The classifier warns (but doesn't block) raw DML against core tables.

---

## 9. Unified current_setting Namespace

### 9.1 The Contradiction

Three different namespace prefixes across specs:
- `conscience.session_id` (SPEC-003 §3.2, §6.3, §7.5)
- `app.current_session_id` (SPEC-004 §, SPEC-005 §)
- `app.current_user_id` (SPEC-005 §)

### 9.2 Canonical Namespace: `conscience.*`

All session-scoped settings use the `conscience` prefix:

```sql
-- Set by harness at session start, per-request
SET LOCAL conscience.session_id = 'uuid-here';
SET LOCAL conscience.user_id = 'uuid-here';

-- System-wide settings (set at install)
SET conscience.llm_endpoint = 'https://api.openai.com/v1/chat/completions';
SET conscience.api_key_ref = 'llm_api_key';  -- Vault reference, not the key itself
```

### 9.3 RLS Policies Updated

```sql
-- All RLS policies use conscience.session_id
CREATE POLICY session_isolate_memory ON memory_events
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_tasks ON tasks
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

-- User-scoped policies use conscience.user_id
CREATE POLICY user_data_scope ON project_resources
    FOR ALL USING (
        project_id IN (
            SELECT project_id FROM user_project_access
            WHERE user_id = current_setting('conscience.user_id')::UUID
        )
    );
```

### 9.4 Why `conscience.*` Not `app.*`

- `app.*` is a generic prefix that could collide with other Postgres extensions or application settings
- `conscience.*` is namespaced to this framework specifically
- Makes debugging easier: `SELECT * FROM pg_settings WHERE name LIKE 'conscience.%'` shows all framework state

### 9.5 Security: SET LOCAL per Transaction

The `conscience.session_id` MUST be set as `SET LOCAL` inside each transaction, never as a session-level setting. This prevents session cross-contamination when connection pooling reuses connections:

```typescript
async function setCognitionContext(sessionId: string, userId: string) {
    // SET LOCAL resets at transaction end — no leakage to next transaction
    await tx.execute(sql`SET LOCAL conscience.session_id = ${sessionId}`);
    await tx.execute(sql`SET LOCAL conscience.user_id = ${userId}`);
}
```

**Connection pooling safety:** `SET LOCAL` is safe with both Supabase Supavisor (transaction mode) and PgBouncer (transaction mode). In both cases, `SET LOCAL` resets automatically at `COMMIT` or `ROLLBACK` — there is no race condition or leakage risk. `SET` (without `LOCAL`) would be dangerous with pooling because it persists across transactions; `SET LOCAL` is specifically designed for this pattern. Statement-level pooling (e.g., PgBouncer `pool_mode=statement`) is NOT supported for Conscience — transaction pooling is required.

**ORM configuration:** When connecting to Supabase via Supavisor transaction mode (port 6543), prepared statements are NOT supported. Drizzle must be configured with `prepare: false` on the Postgres.js client. Kysely uses the simple query protocol by default and does not require configuration changes. Direct connections (port 5432) and session mode support prepared statements normally.

### 9.6 PocketBase: Application-Layer Session Context

SQLite does not support `SET LOCAL` or `current_setting()`. PocketBase achieves equivalent session isolation through two layers:

1. **PocketBase API Rules** (declarative): Collection rules like `session_id = @request.auth.id` provide basic scoping for all API routes with zero Go code.
2. **Go harness middleware** (for operations requiring RLS-equivalent controls): The harness injects `WHERE session_id = ?` into queries, equivalent to the PostgreSQL RLS policies but enforced at the application layer.

The harness shim (SPEC-009 §Conscience Shim Layer) abstracts this difference behind a common interface:

```typescript
// PostgreSQL implementation
async function setSessionContext(tx, sessionId: string) {
    await tx.execute(sql`SET LOCAL conscience.session_id = ${sessionId}`);
}

// PocketBase implementation — no DB-level setting needed
// Session ID is passed as a parameter to every query via the shim
async function setSessionContext(sessionId: string) {
    // Store in Go request context: e.Set("sessionId", sessionId)
    // WHERE clause injection happens in the shim layer
}
```

---

## 10. Vector Embedding Parity

### 10.1 The Problem

SPEC-002 §8 computes cosine similarity between `embedding_raw` and `embedding_summary` to validate compression quality. If these embeddings come from different models (Tier 1 vs Tier 2), the vectors occupy different vector spaces — cosine similarity is meaningless.

### 10.2 Canonical Rule: One Embedding Model

All embeddings across ALL tiers use the **same embedding model**. The model is stored in a system setting:

```sql
-- Set at install time, cannot be changed while sessions are active
INSERT INTO system_settings (key, value) VALUES ('embedding_model', 'text-embedding-3-small');
```

The compression loop may use different LLM models (Tier 1/2/3) for **generating summaries**, but the embedding step always uses the configured embedding model:

```
1. Tier 1 LLM generates summary          ← uses Tier 1 LLM
2. Embed original content                 ← uses embedding model (same for all)
3. Embed summary                          ← uses embedding model (same for all)
4. Cosine similarity between embeddings   ← VALID (same vector space)
```

This guarantees that cosine similarity comparison is mathematically valid regardless of which LLM tier generated the summary.

---

## 11. Iteration Snapshots Table

### 11.1 The Contradiction

SPEC-006 § defines `iteration_snapshots` but SPEC-003 §2.3 defines `iteration_commits`. These serve overlapping purposes.

### 11.2 Canonical Resolution: Merge into iteration_commits

`iteration_commits` is the existing canonical table. `iteration_snapshots` from SPEC-006 adds `llm_response` and `sql_executed` — valuable for replay. Add those columns:

```sql
CREATE TABLE iteration_commits (
    iteration_id      BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    active_pointers   BIGINT[] NOT NULL,
    display_rules     JSONB NOT NULL DEFAULT '{}',
    llm_response      JSONB,
    sql_executed      TEXT[],
    rows_affected     INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_session_iteration ON iteration_commits(session_id, iteration_id);
```

The `iteration_snapshots` table from SPEC-006 is eliminated. Its data lives in `iteration_commits.llm_response` and `iteration_commits.sql_executed`.

---

## 12. Additional Unified Tables

### 12.1 agent_messages (From SPEC-004)

SPEC-004 § references `agent_messages` for parent→child communication but never defines the schema:

```sql
CREATE TABLE agent_messages (
    id                BIGSERIAL PRIMARY KEY,
    target_session_id UUID NOT NULL REFERENCES sessions(id),
    sender_session_id UUID NOT NULL REFERENCES sessions(id),
    payload           JSONB NOT NULL,
    read              BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_target ON agent_messages(target_session_id, read);
```

The child's harness reads unread messages at each iteration and injects them into context.

### 12.2 system_settings

Referenced in §10.2 but not defined in any spec:

```sql
CREATE TABLE system_settings (
    key               TEXT PRIMARY KEY,
    value             TEXT NOT NULL,
    description       TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 12.3 audit_logs (From SPEC-007)

SPEC-007 § references `audit_logs` but SPEC-006 § calls it `iteration_snapshots`. Canonical: use `iteration_commits.llm_response` for the LLM output, and a separate `audit_logs` for structured access:

```sql
CREATE TABLE audit_logs (
    id                BIGSERIAL PRIMARY KEY,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration         BIGINT NOT NULL,
    monologue         TEXT,
    sql_executed      TEXT[],
    result            TEXT NOT NULL CHECK (result IN ('committed', 'rolled_back')),
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_session ON audit_logs(session_id, iteration);
```

---

## 13. Canonical RLS Role Model

### 13.1 Roles

| Role | Purpose | RLS | Can UPDATE memory_events? |
|---|---|---|---|
| `agent_role` | Normal agent execution | Subject to RLS | No (INSERT/SELECT only) |
| `compression_worker` | Background compression | Subject to RLS | Yes (summary_text column only) |
| `alt_mode_role` | Admin operations | Bypasses RLS (BYPASSRLS) | Yes (full access) |
| `tool_executor` | Writes tool_results | Subject to RLS | No (writes tool_results only) |

### 13.2 Grant Summary

```sql
-- agent_role: core agent operations
GRANT SELECT, INSERT ON memory_events TO agent_role;
GRANT SELECT, INSERT, UPDATE ON display_modes TO agent_role;
GRANT SELECT, INSERT ON iteration_commits TO agent_role;
GRANT SELECT, INSERT, UPDATE ON sessions TO agent_role;  -- status, heartbeat only
GRANT SELECT, INSERT, UPDATE ON tasks TO agent_role;
GRANT SELECT, INSERT ON tool_requests TO agent_role;
GRANT SELECT, INSERT ON agent_billing TO agent_role;
GRANT SELECT ON active_context_view TO agent_role;
GRANT SELECT, INSERT ON agent_messages TO agent_role;
-- No UPDATE/DELETE on memory_events. No access to vault.

-- compression_worker: background summary generation
GRANT UPDATE (summary_text) ON memory_events TO compression_worker;
GRANT SELECT, INSERT, UPDATE ON display_modes TO compression_worker;
GRANT SELECT, INSERT, UPDATE ON compression_queue TO compression_worker;

-- alt_mode_role: admin bypass
GRANT ALL ON ALL TABLES IN SCHEMA public TO alt_mode_role;
ALTER ROLE alt_mode_role BYPASSRLS;

-- tool_executor: external tool result writer
GRANT SELECT, INSERT ON tool_results TO tool_executor;
GRANT UPDATE ON tool_requests TO tool_executor;  -- status changes only
```

---

## 14. Cross-Spec Impact Map

This table shows which earlier spec sections are superseded by SPEC-011:

| SPEC-011 Section | Supersedes | What Changes |
|---|---|---|
| §1 Session State Machine | SPEC-003 §2.1, SPEC-004 (READY/SLEEPING), SPEC-008 (READY/RUNNING), SPEC-020 §9 | Lowercase states, expanded transitions, planning/executing for interactive transactions |
| §2 Task State Machine | SPEC-003 §2.5, SPEC-004, SPEC-008 | Unified task lifecycle |
| §3 Display Mode | SPEC-002 §2.2 (display_mode column), §3.4, §8.3 | New display_modes table, removed column |
| §4 Tool/Skill Registry | SPEC-003 §2.8, SPEC-010 (both registries) | Two-table split, merged fields |
| §5 Custom Agent Tools | SPEC-003 §2.11, SPEC-010 | Global unique names, language support |
| §6 Billing | SPEC-003 §2.9, SPEC-006 | Cache tokens, NUMERIC cost, category |
| §7 Tool Execution | SPEC-008 (in-transaction tools), SPEC-006 § | Two-phase execution |
| §8 SQL Injection | SPEC-007 (raw SQL safety), SPEC-008 | Classifier + whitelist + stored procs, multi-statement splitting, three-tier execution model |
| §9 Namespace | SPEC-003, SPEC-004, SPEC-005 (mixed prefixes) | All `conscience.*`, SET LOCAL safe with transaction pooling, PocketBase uses app-layer context |
| §10 Embedding Parity | SPEC-002 §8, §9 | One embedding model for all tiers |
| §11 Iteration Commits | SPEC-003 §2.3, SPEC-006 (iteration_snapshots) | Merged into iteration_commits |
| §12 New Tables | SPEC-004, SPEC-007 (undefined references) | agent_messages, system_settings, audit_logs |
| §13 RLS Roles | SPEC-003 §7.5, SPEC-005 (mixed roles) | Four-role model with explicit grants |

---

## 15. Open Questions

1. **Dynamic table TTL**: When a session ends, should its dynamic agent tables be dropped, or left for audit? (SPEC-003 §11 Q1)
2. **Sub-agent depth limit**: Maximum nesting depth for sub-agent spawning to prevent circular/unbounded spawning
3. **Embedding model migration**: Procedure for changing the embedding model after data exists (re-embed everything?)

### 16. Resolved Questions

4. **~~SET LOCAL in SQLite~~** — RESOLVED: PocketBase does not support `SET LOCAL` / `current_setting()`. Session context is passed through Go middleware (`WHERE session_id = ?`) and PocketBase API Rules (`session_id = @request.auth.id`). The harness shim (SPEC-009) abstracts this behind a common interface. `SET LOCAL` is safe with connection pooling (Supavisor/PgBouncer transaction mode) because it resets at `COMMIT`/`ROLLBACK`. Statement-level pooling is NOT supported. See SPEC-009 §Session Context Isolation and §9.6.

5. **~~PocketBase full rewrite concern~~** — RESOLVED: PocketBase parity is an incremental adaptation, not a full rewrite. ~30% of the database layer needs architecturally different code (RLS → API Rules + Go hooks, `current_setting()` → Go context, PL/pgSQL → Go). ~50% is "same logic, different language" (triggers → Go hooks, pg_cron → app.Cron, pg_net → Go HTTP). ~20% is truly portable (table schemas, CHECK constraints, JSON Schema via sqlite-jsonschema). See SPEC-009 §Conscience Shim Layer and §SQLite Parity.

6. **~~SQL injection from multi-statement strings~~** — RESOLVED: The harness now splits all SQL strings on semicolons before classification (SPEC-011 §8.2, SPEC-008 §SQL Execution Model). Each statement is classified independently. A string like `"SELECT * FROM mem; DELETE FROM tasks"` is split into two statements, blocked at the classifier level.

7. **~~Parameterize/sanitize function contract~~** — RESOLVED: The `parameterize()` function from SPEC-008 is replaced by a three-tier execution model (SPEC-008 §SQL Execution Model): Tier 1 = stored procedures only, Tier 2 = parameterized SQL with `$1/$2` bind variables, Tier 3 = raw SQL with classifier + whitelist. The `sanitize()` function provides defense-in-depth (null byte removal, whitespace normalization) and is documented in SPEC-008.