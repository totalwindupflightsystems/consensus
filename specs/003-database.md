# SPEC-003: Database Schema & Parity Layer

**Status:** Draft  
**Source:** Gemini Chat Turns 4-5, 9-11, 19-20, 26-28, 30-31, 34-38  
**Depends On:** SPEC-001-ARCHITECTURE, SPEC-002-MEMORY  
**Last Updated:** 2026-04-08  
**Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

---

## 1. Overview

This spec defines the complete database schema for Conscience, covering all core tables, the dynamic entity generator, JSON Schema validation, SQL-enforced constraint types, token caching strategy, and PostgreSQL/SQLite parity requirements.

The database is not a persistence layer — it IS the runtime (SPEC-001 §2.1). Every table defined here serves a dual purpose: data storage and runtime enforcement of agent behavior.

**Source:** Turns 19-20 (unified deployment), Turns 26-28 (schema design, JSONB parity)

---

## 2. Core Tables

### 2.1 sessions

Every agent instance is a row in `sessions`. This is the root identity for RLS isolation, billing, and lifecycle management.

```sql
CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id       UUID REFERENCES sessions(id),
    agent_name      TEXT NOT NULL,
    model_id        TEXT NOT NULL REFERENCES model_registry(model_id),
    status          TEXT NOT NULL DEFAULT 'booting'
                    CHECK (status IN ('booting', 'idle', 'planning', 'thinking', 'tool_exec', 'executing', 'waiting_sub', 'completed', 'failed', 'paused')),
    goal            TEXT,
    context_budget  INT NOT NULL DEFAULT 128000,
    tokens_used_in  BIGINT NOT NULL DEFAULT 0,
    tokens_used_out BIGINT NOT NULL DEFAULT 0,
    iteration       BIGINT NOT NULL DEFAULT 0,
    heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    planning_max_turns INT NOT NULL DEFAULT 10,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_sessions_parent ON sessions(parent_id);
CREATE INDEX idx_sessions_status ON sessions(status) WHERE status IN ('idle', 'thinking', 'planning', 'tool_exec', 'executing', 'waiting_sub');
```

| Column | Purpose |
|---|---|
| `parent_id` | NULL for root agents. Set when this is a sub-agent fork. Enables lineage traversal. |
| `agent_name` | Semantic identity — e.g., `scraper_alpha`, `reviewer_beta`. Not unique; paired with `id`. |
| `status` | Finite state machine. Transition constraints enforced by trigger (§5.1). |
| `goal` | The task description injected into the system prompt. |
| `context_budget` | Max token limit for this agent's LLM context window. |
| `tokens_used_in/out` | Accumulated billing counters. Updated per iteration. |
| `heartbeat_at` | Last known-alive timestamp. `pg_cron` reaps stale sessions (§7.1). |

---

### 2.2 memory_events

The immutable append-only ledger. Full semantics defined in SPEC-002 §2.

```sql
CREATE TABLE memory_events (
    id                BIGSERIAL PRIMARY KEY,
    type              TEXT NOT NULL
                      CHECK (type IN ('header', 'text_block', 'tool_call', 'tool_result', 'thinking', 'system', 'inherited_pointer', 'user_message')),
    content           TEXT NOT NULL,
    summary_text      TEXT,
    session_id        UUID NOT NULL REFERENCES sessions(id),
    iteration_created BIGINT NOT NULL,
    linked_memory_pages UUID[],
    embedding         vector(1536),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_memory_session ON memory_events(session_id, iteration_created);
CREATE INDEX idx_memory_embedding ON memory_events
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Append-only enforcement: no role may UPDATE or DELETE this table
-- except compression_worker, which may UPDATE summary_text only
REVOKE UPDATE, DELETE ON memory_events FROM agent_role;
```

> **Note:** `linked_memory_pages` column added here to cross-reference dynamic entity rows back to their originating memory context. See §10.

> **Note:** `display_mode` has been moved to a separate table (SPEC-011 §3) to preserve true append-only semantics. The `REVOKE UPDATE/DELETE` on `memory_events` now covers ALL columns — no role may UPDATE this table except `compression_worker` for `summary_text` only.

### 2.2a display_modes

Display state for each memory event, stored separately so `memory_events` remains truly append-only. Canonical definition: SPEC-011 §3.2.

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

-- display_modes is fully mutable; agent_role can change display state independently
GRANT SELECT, INSERT, UPDATE ON display_modes TO agent_role;
```

No row in `display_modes` means `'full'` (default). Compression workers also write here when setting a memory event to compressed.

---

### 2.3 iteration_commits

Snapshots of the active pointer set per iteration. Full semantics in SPEC-002 §6.

```sql
CREATE TABLE iteration_commits (
    iteration_id    BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    active_pointers BIGINT[] NOT NULL,
    display_rules   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_iteration_session ON iteration_commits(session_id, iteration_id DESC);
```

---

### 2.4 memory_pages

Named groups of memory IDs for token-efficient referencing. Full semantics in SPEC-002 §5.

```sql
CREATE TABLE memory_pages (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    target_ids      BIGINT[] NOT NULL,
    linked_page_ids BIGINT[] NOT NULL DEFAULT '{}',
    session_id      UUID NOT NULL REFERENCES sessions(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(name, session_id)
);
```

---

### 2.5 tasks

The unified work queue. Tasks can be claimed by agents, carry prerequisites, and enforce state transitions.

```sql
CREATE TABLE tasks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id        UUID NOT NULL REFERENCES sessions(id),
    parent_task_id    UUID REFERENCES tasks(id),
    title             TEXT NOT NULL,
    description       TEXT,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'claimed', 'in_progress', 'reviewed', 'published', 'failed', 'cancelled')),
    priority          INT NOT NULL DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    locked_by_agent   UUID REFERENCES sessions(id),
    prerequisite_ids  UUID[] NOT NULL DEFAULT '{}',
    result_memory_id  BIGINT REFERENCES memory_events(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
);

CREATE INDEX idx_tasks_session_status ON tasks(session_id, status);
CREATE INDEX idx_tasks_locked_by ON tasks(locked_by_agent) WHERE locked_by_agent IS NOT NULL;

-- State transition lock: cannot skip REVIEWED → go straight to PUBLISHED
-- See §5.1 for trigger implementation
```

| Column | Purpose |
|---|---|
| `parent_task_id` | Decomposition tree. A parent task spawns sub-tasks. |
| `locked_by_agent` | Mutual exclusion lock. Only the claiming agent can mutate the task. |
| `prerequisite_ids` | Foreign-key-like dependency chain. All prerequisites must be `published` before this task can be `claimed`. |
| `result_memory_id` | Pointer to the memory_event containing this task's output. Epistemic anchoring (§5.3). |

---

### 2.6 tool_requests

Outbound tool invocations requested by the agent. Written during the agent's SQL transaction; picked up by the harness for external execution.

```sql
CREATE TABLE tool_requests (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration_id    BIGINT NOT NULL,
    tool_name       TEXT NOT NULL,
    parameters      JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'executing', 'completed', 'failed', 'timeout')),
    timeout_ms      INT NOT NULL DEFAULT 30000,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_at     TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_tool_req_pending ON tool_requests(session_id, status)
    WHERE status = 'pending';
```

---

### 2.7 tool_results

Responses from the external tool runner, written back by the harness after execution completes.

```sql
CREATE TABLE tool_results (
    id              BIGSERIAL PRIMARY KEY,
    request_id      BIGINT NOT NULL REFERENCES tool_requests(id),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    output          TEXT NOT NULL,
    is_error        BOOLEAN NOT NULL DEFAULT false,
    error_code      TEXT,
    token_count     INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tool_result_request ON tool_results(request_id);
```

---

### 2.8 tools_registry

Executable capabilities (SQL functions, HTTP endpoints, Go handlers, subprocesses). Agents discover available tools by querying this table — not by reading filesystem directories (SPEC-001 §2.1). Canonical definition: SPEC-011 §4.2.1.

The old `skills_registry` conflated executable tools with knowledge bundles. These are now separate: `tools_registry` answers "What can I EXECUTE?" and `skills_registry` (§2.8a) answers "What do I KNOW?".

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

| Column | Purpose |
|---|---|
| `hemisphere` | Internal tools run as SQL functions inside the DB. External tools run in the sandboxed runtime (SPEC-001 §5). |
| `handler_type` | How to dispatch the tool call. `sql_function` uses `SELECT * FROM {handler_ref}(...)`. `http_endpoint` calls an external HTTP endpoint. `go_native` calls a Go handler function in the binary. `subprocess` spawns a sandboxed subprocess. |
| `handler_ref` | The callable identifier — function name, endpoint, or hook name. |
| `owner_session_id` | Which session created this tool (NULL for system-provided tools). Governance traceability. |
| `status` | Lifecycle governance: `active`, `testing`, `deprecated`, `disabled`. Replaces the binary `enabled` for tool lifecycle. |
| `requires_approval` | If true, the harness must pause and await human approval before dispatching. |
| `rate_limit_per_min` | Optional rate limiter. Enforced by trigger (§5.5). |

### 2.8a skills_registry

Knowledge bundles (instructions, workflows, progressive disclosure). A skill references tools via `linked_tool_ids`. Canonical definition: SPEC-011 §4.2.2.

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

| Column | Purpose |
|---|---|
| `metadata` | JSONB including description, tags, version — cheap to read for progressive disclosure. |
| `instructions` | The full skill prompt — expensive to load, only sent when the skill is activated. |
| `linked_tool_ids` | References `tools_registry.id` (validated at application layer since UUID[] can't FK). |

---

### 2.9 agent_billing

Cost tracking per session per iteration, accrued from LLM API calls and tool usage. Canonical definition: SPEC-011 §6.2.

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

CREATE INDEX idx_billing_session ON agent_billing(session_id);
CREATE INDEX idx_billing_recorded ON agent_billing(recorded_at);
```

| Column | Purpose |
|---|---|
| `iteration` | Which iteration generated this cost — required for per-iteration cost breakdown. |
| `model_id` | References `model_registry.model_id` consistently. TEXT, not `model`. |
| `category` | Critical for understanding cost breakdown: main loop vs compression vs embeddings. |
| `cache_read_tokens` / `cache_write_tokens` | Cache-specific usage metadata returned by Anthropic and OpenAI. Enables cache hit rate analytics. |
| `cost_usd` | NUMERIC(12,6) supports micro-cent granularity for cheap models. `total_tokens` is derivable as `prompt_tokens + completion_tokens`. |

---

### 2.10 workflows

Reusable multi-step agent configurations. A workflow defines the sequence of tool calls, sub-agent spawns, and state transitions.

```sql
CREATE TABLE workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    description     TEXT,
    steps           JSONB NOT NULL DEFAULT '[]',
    trigger_event   TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

The `steps` JSONB array contains ordered objects:

```json
[
  {"type": "tool_call", "tool": "scrape", "params": {"url": "$INPUT_URL"}},
  {"type": "spawn_agent", "agent_name": "summarizer", "goal": "Summarize the scraped content"},
  {"type": "wait_for_subagents"},
  {"type": "tool_call", "tool": "write_file", "params": {"path": "/output/summary.md"}}
]
```

---

### 2.11 custom_agent_tools

Agent-authored tools. When an agent writes code (e.g., a Node script for a specific API integration), the code is stored here — not on the filesystem. Canonical definition: SPEC-011 §5.2.

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

Agents write the code. Alt-Mode administrators approve it. Unapproved tools cannot execute — the harness checks `approved = true` before dispatching. Additionally, `status` must be `'active'` for the tool to be available: both `approved = true` AND `status = 'active'` are required.

Key changes from original schema (SPEC-011 §5.3): `creator_session_id` (not `session_id`) because sessions are the identity unit. `UNIQUE(name)` globally, not per-session — tool names are identifiers that other agents reference, so two agents creating `zillow_scraper` is a collision, not independent evolution. `language` includes `'go'` for Go hook tools. `status` column added for governance (draft → testing → active → deprecated).

---

### 2.12 tool_files

File-like objects stored in the database rather than the filesystem. For binary data (images, PDFs) that tools produce or consume.

```sql
CREATE TABLE tool_files (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    name            TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    content_b64     TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    memory_event_id BIGINT REFERENCES memory_events(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tool_files_session ON tool_files(session_id);
```

---

### 2.13 external_quarantine

Staging area for data received from external sources (scraped pages, API responses) before it enters the trusted memory ledger. All external data must pass validation here before being promoted.

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

**Source:** Turn 34 (external tool results must be validated before entering trusted memory), Turn 11 (circuit breakers for untrusted data)

---

### 2.14 model_registry

Model catalog for the 2D routing matrix. Full semantics in SPEC-002 §9.

```sql
CREATE TABLE model_registry (
    id              BIGSERIAL PRIMARY KEY,
    model_id        TEXT NOT NULL UNIQUE,
    tier            INT NOT NULL CHECK (tier IN (1, 2, 3)),
    max_context     INT NOT NULL,
    cost_per_m_in   NUMERIC(8,4) NOT NULL,
    cost_per_m_out  NUMERIC(8,4) NOT NULL,
    classifier_tags TEXT[] DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true
);
```

---

## 3. Dynamic Entity Generator

### 3.1 Principle

Agents need to create domain-specific data structures at runtime (e.g., `order_tracking`, `bug_reports`). The framework provides a SECURITY DEFINER function that provisions new tables with required system columns, a JSONB payload column, and automatic RLS policies — without granting the agent raw DDL privileges.

**Source:** Turn 27 (dynamic entities), Turn 26 (JSONB design), Turn 34 (schema-gated triggers)

### 3.2 Generator Function (PostgreSQL)

```sql
CREATE OR REPLACE FUNCTION create_agent_memory_table(table_name TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    fq_name TEXT := lower(regexp_replace(table_name, '[^a-z0-9_]', '_', 'g'));
BEGIN
    -- Prevent creation of core tables
    IF fq_name IN ('sessions', 'memory_events', 'iteration_commits', 'memory_pages',
                   'display_modes', 'tasks', 'tool_requests', 'tool_results',
                   'tools_registry', 'skills_registry',
                   'agent_billing', 'workflows', 'custom_agent_tools', 'tool_files',
                   'external_quarantine', 'model_registry', 'compression_queue',
                   'staging_buffer', 'audit_logs', 'system_settings',
                   'agent_messages', 'api_keys', 'api_rate_limits',
                   'external_events', 'webhook_registrations', 'routing_rules',
                   'agent_circuit_breakers', 'agent_budget_limits',
                   'secret_access_audit', 'approval_requests') THEN
        RAISE EXCEPTION 'Cannot create table with reserved name: %', fq_name;
    END IF;

    -- Prevent SQL injection: validate table name
    IF fq_name !~ '^[a-z_][a-z0-9_]{0,62}$' THEN
        RAISE EXCEPTION 'Invalid table name: %. Must be lowercase alphanumeric + underscore, 1-63 chars, start with letter or underscore.', fq_name;
    END IF;

    -- Check if table already exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = fq_name) THEN
        RETURN 'Table already exists: ' || fq_name;
    END IF;

    EXECUTE format('
        CREATE TABLE %I (
            id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            session_id          UUID NOT NULL REFERENCES sessions(id),
            iteration_created   BIGINT,
            deleted_at          TIMESTAMPTZ,
            linked_memory_pages UUID[],
            data                JSONB NOT NULL DEFAULT ''{}'',
            created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
        );
    ', fq_name);

    -- Row-Level Security
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', fq_name);
    EXECUTE format('
        CREATE POLICY isolate_session_%s ON %I
            FOR ALL USING (session_id = current_setting(''conscience.session_id'')::UUID)
    ', fq_name, fq_name, fq_name);

    -- Soft-delete intercept
    EXECUTE format('
        CREATE TRIGGER soft_delete_%s
            BEFORE DELETE ON %I
            FOR EACH ROW EXECUTE FUNCTION soft_delete_intercept()
    ', fq_name, fq_name);

    -- Auto-update updated_at
    EXECUTE format('
        CREATE TRIGGER update_updated_at_%s
            BEFORE UPDATE ON %I
            FOR EACH ROW EXECUTE FUNCTION update_updated_at()
    ', fq_name, fq_name);

    RETURN 'Created table: ' || fq_name;
END;
$$;
```

### 3.3 Generator Function (SQLite / PocketBase)

PocketBase does not support SECURITY DEFINER functions. The equivalent is a Go hook registered on the `OnRecordBeforeCreateRequest` event for a virtual `_table_requests` collection:

```go
func handleCreateTable(app *pocketbase.PocketBase, e *core.RecordCreateEvent) {
    tableName := e.Record.GetString("table_name")
    // Validate name (same regex as Postgres)
    if !regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`).MatchString(tableName) {
        e.Record.Set("status", "rejected")
        return
    }
    // Create collection with system fields
    collection := core.NewBaseCollection(tableName)
    collection.Fields.Add(&fields.TextField{Name: "session_id", Required: true})
    collection.Fields.Add(&fields.NumberField{Name: "iteration_created"})
    collection.Fields.Add(&fields.DateField{Name: "deleted_at"})
    collection.Fields.Add(&fields.JSONField{Name: "linked_memory_pages"})
    collection.Fields.Add(&fields.JSONField{Name: "data"})
    collection.Fields.Add(&fields.DateField{Name: "created_at"})
    collection.Fields.Add(&fields.DateField{Name: "updated_at"})
    app.Dao().SaveCollection(collection)
}
```

### 3.4 System Column Guarantees

Every dynamically generated table includes these columns. The agent cannot omit or rename them:

| Column | Type | Purpose |
|---|---|---|
| `id` | UUID | Primary key, unique reference |
| `session_id` | UUID | RLS isolation scope, foreign key to `sessions` |
| `iteration_created` | BIGINT | Ledger audit trail — which iteration created this row |
| `deleted_at` | TIMESTAMPTZ | Soft delete — intercepted by trigger, never hard-deleted |
| `linked_memory_pages` | UUID[] | Cross-reference to Memory Pages for context anchoring |
| `data` | JSONB | Agent-defined payload — the flexible NoSQL core |
| `created_at` | TIMESTAMPTZ | Timestamp of creation |
| `updated_at` | TIMESTAMPTZ | Timestamp of last mutation (auto-updated by trigger) |

---

## 4. JSON Schema Validation

### 4.1 Principle

Alt-Mode agents (administrative, higher-trust) can lock down a dynamic table's JSONB column with a strict JSON Schema. If a working agent submits malformed data, the transaction is rejected by the database kernel — not by application code.

**Source:** Turn 28 (JSON Schema parity in SQLite), Turn 34 (schema-gated triggers)

### 4.2 PostgreSQL: pg_jsonschema

```sql
-- Enable extension
CREATE EXTENSION IF NOT EXISTS pg_jsonschema;

-- Apply schema constraint to a dynamic table
ALTER TABLE order_tracking
ADD CONSTRAINT strict_order_schema
CHECK (jsonb_matches_schema('
    {
        "type": "object",
        "required": ["item", "sku", "qty"],
        "properties": {
            "item": {"type": "string"},
            "sku":  {"type": "string", "pattern": "^[A-Z]{3}-\\d{4}$"},
            "qty":  {"type": "integer", "minimum": 1},
            "notes": {"type": "string"}
        },
        "additionalProperties": false
    }
', data));
```

### 4.3 SQLite: sqlite-jsonschema

```sql
-- Load extension
SELECT load_extension('./sqlite-jsonschema');

-- Identical ALTER TABLE syntax
ALTER TABLE order_tracking
ADD CONSTRAINT strict_order_schema
CHECK (jsonschema_matches('
    {
        "type": "object",
        "required": ["item", "sku", "qty"],
        "properties": {
            "item": {"type": "string"},
            "sku":  {"type": "string", "pattern": "^[A-Z]{3}-\\d{4}$"},
            "qty":  {"type": "integer", "minimum": 1},
            "notes": {"type": "string"}
        },
        "additionalProperties": false
    }
', data));
```

### 4.4 Why Identical Syntax Matters

Both extensions accept the same JSON Schema draft-07 (or later) documents and use the same `ALTER TABLE ... ADD CONSTRAINT CHECK (...)` pattern. This means:

1. Schema documents are portable — export from Postgres, import into SQLite
2. Migration scripts work on both targets without conditional logic
3. The LLM writes one constraint; it runs everywhere

**Source:** Turn 28 (parity requirement for JSON Schema validation)

### 4.5 Parity Limitations

| Feature | PostgreSQL | SQLite |
|---|---|---|
| `jsonb_matches_schema` | Native (pg_jsonschema) | Native (sqlite-jsonschema) |
| `additionalProperties` | Supported | Supported |
| `$ref` / `$defs` | Supported | Partial (simple refs only) |
| `format` validation | Supported | Limited (date/time only) |
| Custom meta-schemas | Supported | Not supported |

For maximum parity, keep schemas flat and avoid `$ref` indirection or `format` keywords beyond `date-time`.

---

## 5. SQL Constraint Types for Unbypassable Rules

### 5.1 State Transition Locks

Enforce valid workflow steps. An agent cannot skip states.

```sql
CREATE OR REPLACE FUNCTION enforce_task_transitions()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    -- Cannot go from pending directly to in_progress without claiming
    IF NEW.status = 'in_progress' AND OLD.status = 'pending' AND NEW.locked_by_agent IS NULL THEN
        RAISE EXCEPTION 'Task must be claimed before it can be in_progress';
    END IF;

    -- Cannot skip REVIEWED → go straight to PUBLISHED
    IF NEW.status = 'published' AND OLD.status != 'reviewed' THEN
        RAISE EXCEPTION 'Task must be reviewed before publishing. Current: %, Required: reviewed', OLD.status;
    END IF;

    -- Cannot go backwards from published
    IF OLD.status = 'published' AND NEW.status != 'published' THEN
        RAISE EXCEPTION 'Published tasks cannot be reverted';
    END IF;

    -- Cannot claim a task that is already claimed by another agent
    IF NEW.locked_by_agent IS DISTINCT FROM OLD.locked_by_agent
       AND OLD.locked_by_agent IS NOT NULL
       AND NEW.locked_by_agent IS NOT NULL THEN
        RAISE EXCEPTION 'Task already locked by agent %', OLD.locked_by_agent;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER task_transition_guard
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION enforce_task_transitions();
```

**Source:** Turn 35 (state transition locks), Turn 36 (unbypassable constraints)

### 5.2 Prerequisite Dependencies

Tasks declare their dependencies. A trigger prevents claiming a task whose prerequisites are incomplete.

```sql
CREATE OR REPLACE FUNCTION enforce_prerequisites()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    unmet_count INT;
BEGIN
    IF NEW.status IN ('claimed', 'in_progress') AND OLD.status = 'pending' THEN
        SELECT COUNT(*) INTO unmet_count
        FROM tasks t
        WHERE t.id = ANY(NEW.prerequisite_ids)
          AND t.status != 'published';

        IF unmet_count > 0 THEN
            RAISE EXCEPTION 'Cannot claim task: % prerequisite(s) not yet published', unmet_count;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER task_prerequisite_guard
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION enforce_prerequisites();
```

**Source:** Turn 36 (dependency enforcement)

### 5.3 Epistemic Anchoring (Anti-Hallucination Citations)

Memory events and tool results that make claims about other data must cite their source. A foreign key to `memory_events` ensures the cited memory actually exists.

```sql
-- Tasks must anchor their results to real memory
ALTER TABLE tasks
ADD CONSTRAINT fk_result_memory_exists
 FOREIGN KEY (result_memory_id) REFERENCES memory_events(id);

-- Dynamic entity rows can link to their originating memory
-- (linked_memory_pages column, validated in application layer since UUID[] can't FK)

-- Tool results must reference real requests
ALTER TABLE tool_results
ADD CONSTRAINT fk_request_exists
 FOREIGN KEY (request_id) REFERENCES tool_requests(id);
```

For dynamic tables where the JSONB payload itself contains memory references:

```sql
-- pg_jsonschema can validate citation structure inside JSONB
ALTER TABLE research_notes
ADD CONSTRAINT citation_format
CHECK (jsonb_matches_schema('
    {
        "type": "object",
        "required": ["claim", "cited_memory_ids"],
        "properties": {
            "claim": {"type": "string"},
            "cited_memory_ids": {
                "type": "array",
                "items": {"type": "integer"},
                "minItems": 1
            }
        }
    }
', data));
```

Then a trigger cross-references those IDs:

```sql
CREATE OR REPLACE FUNCTION validate_citations_exist()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    cited_id BIGINT;
    missing_ids BIGINT[];
BEGIN
    IF NEW.data ? 'cited_memory_ids' THEN
        FOR cited_id IN SELECT jsonb_array_elements_text(NEW.data->'cited_memory_ids')::BIGINT
        LOOP
            IF NOT EXISTS (SELECT 1 FROM memory_events WHERE id = cited_id) THEN
                missing_ids := array_append(missing_ids, cited_id);
            END IF;
        END LOOP;

        IF array_length(missing_ids, 1) > 0 THEN
            RAISE EXCEPTION 'Cited memory IDs do not exist: %', missing_ids;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
```

**Source:** Turn 36 (epistemic anchoring), Turn 34 (schema-gated triggers)

### 5.4 Mutual Exclusion

Prevent concurrent agents from modifying the same resource.

```sql
-- Conditional unique index: only one active lock per task
CREATE UNIQUE INDEX idx_one_active_lock ON tasks(id)
    WHERE locked_by_agent IS NOT NULL AND status IN ('claimed', 'in_progress');

-- For dynamic entities with a locked_by column added via migration:
-- CREATE UNIQUE INDEX idx_{table}_one_lock ON {table}(id)
--     WHERE locked_by_agent IS NOT NULL;
```

Optimistic concurrency on sessions:

```sql
-- Prevent two iterations from writing simultaneously
CREATE UNIQUE INDEX idx_session_iteration ON iteration_commits(session_id, iteration_id);
```

**Source:** Turn 36 (mutual exclusion locks)

### 5.5 Rate Limiting

Triggers counting recent events before allowing INSERT.

```sql
CREATE OR REPLACE FUNCTION enforce_tool_rate_limit()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    max_per_min INT;
    recent_count INT;
BEGIN
    SELECT rate_limit_per_min INTO max_per_min
    FROM tools_registry
    WHERE name = NEW.tool_name AND rate_limit_per_min IS NOT NULL;

    IF max_per_min IS NOT NULL THEN
        SELECT COUNT(*) INTO recent_count
        FROM tool_requests
        WHERE session_id = NEW.session_id
          AND tool_name = NEW.tool_name
          AND created_at > now() - INTERVAL '1 minute';

        IF recent_count >= max_per_min THEN
            RAISE EXCEPTION 'Rate limit exceeded for tool %: % per minute',
                NEW.tool_name, max_per_min;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tool_rate_limit_guard
    BEFORE INSERT ON tool_requests
    FOR EACH ROW EXECUTE FUNCTION enforce_tool_rate_limit();
```

**Source:** Turn 36 (rate limiting via triggers)

---

## 6. Token Caching Strategy

### 6.1 Principle

Modern LLM APIs (Anthropic, OpenAI) support prompt caching — reusing previously processed tokens when the prompt prefix is unchanged. The `active_context_view` is designed to maximize cache hits by maintaining a strict immutability hierarchy from top to bottom.

**Source:** Turn 30 (token economics), Turn 9 (view structure for prompt assembly)

### 6.2 Cache Hierarchy in the VIEW

The `active_context_view` must be ordered so that the most stable content sits at the top (always cached) and the most volatile content sits at the bottom (rarely cached).

```
┌──────────────────────────────────────────────────┐
│  LAYER 1: STATIC SYSTEM INSTRUCTIONS             │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • Agent role definition                          │
│  • Available tools (tools_registry query)        │
│  • Session constraints and context budget         │
│  • Immutable. Changes across sessions, never      │
│    within a session. ALWAYS cached.               │
│                                                   │
│  LAYER 2: IMMUTABLE EVENT LEDGER                 │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • memory_events rendered via display_modes       │
│  • Append-only — new rows added at the end,       │
│    existing rows never modified                   │
│  • display_modes changes (compressed/hidden) only │
│    affect rendering, not content mutation           │
│    affect rendering, not content mutation         │
│  • HEAVILY CACHED — grows but never shifts        │
│                                                   │
│  LAYER 3: DYNAMIC SCRATCHPAD & TOOL RESULTS       │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • Current task state, iteration counter          │
│  • Most recent tool_result rows                   │
│  • Compression summaries (volatile — may change)  │
│  • Placed at END so mutations don't invalidate    │
│    cached prefix above                            │
│  • RARELY CACHED — changes every iteration       │
│                                                   │
└──────────────────────────────────────────────────┘
```

### 6.3 View Implementation for Cache Ordering

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

> **Note:** This view JOINs `display_modes` instead of reading a `display_mode` column from `memory_events`, preserving the append-only invariant. The `cache_tier` logic from an earlier draft has been removed — the base view matches the canonical definition in SPEC-011 §3.4. Cache tier ordering can be added back as a separate materialized view if needed.

### 6.4 Cache Invalidation Analysis

| Mutation Type | Which Layer | Cache Impact |
|---|---|---|
| New memory event appended | Layer 2 (end) | None — prefix unchanged, new tokens appended |
| `display_modes` row updated to 'compressed' | Layer 2 | Partial — content swapped to summary, but position same |
| `display_modes` row updated to 'hidden' | Layer 2 | Partial — row removed from view, but preceding prefix intact |
| New tool request/result | Layer 3 | Expected — always volatile |
| Active pointers change (prune) | Layer 2 | Partial — rows removed from end of tier |
| System instruction change | Layer 1 | Full — entire cache invalidated (rare, only at session start) |

**Source:** Turn 30 (token caching economics), Turn 9 (view-driven prompt assembly)

---

## 7. PostgreSQL-Specific Features

### 7.1 pg_cron: Maintenance Jobs (Postgres)

When running against Postgres with `pg_cron` available, scheduled maintenance jobs run inside the database:

```sql
-- Enable extension
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Reap sessions with no heartbeat for 5 minutes
SELECT cron.schedule(
    'reap-stale-sessions',
    '* * * * *',
    $$
    UPDATE sessions
    SET status = 'failed'
    WHERE status IN ('idle', 'thinking', 'planning', 'tool_exec', 'executing', 'waiting_sub')
      AND heartbeat_at < now() - INTERVAL '5 minutes';
    $$
);

-- Clean up expired quarantine entries
SELECT cron.schedule(
    'purge-expired-quarantine',
    '*/10 * * * *',
    $$
    DELETE FROM external_quarantine
    WHERE validation_status = 'pending'
      AND expires_at < now();
    $$
);
```

When `pg_cron` is not available (or on SQLite), the Go binary runs equivalent jobs as Go cron goroutines.

**Source:** Turn 12 (heartbeat mechanism), Turn 34 (quarantine expiration)

### 7.2 pg_net: Async HTTP to LLM API

```sql
-- Enable extension
CREATE EXTENSION IF NOT EXISTS pg_net;

-- Example: fire-and-forget LLM compression request from a trigger
CREATE OR REPLACE FUNCTION queue_compression_request()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_endpoint TEXT;
    v_payload JSONB;
BEGIN
    IF NEW.summary_text IS NULL AND NEW.embedding IS NOT NULL THEN
        v_endpoint := current_setting('conscience.llm_endpoint');

        v_payload := jsonb_build_object(
            'model', (SELECT model_id FROM model_registry WHERE tier = 1 AND enabled = true ORDER BY cost_per_m_out ASC LIMIT 1),
            'prompt', NEW.content,
            'max_tokens', 500
        );

        PERFORM net.http_post(
            url := v_endpoint,
            headers := '{"Content-Type": "application/json", "Authorization": "Bearer ' || current_setting('conscience.api_key') || '"}'::jsonb,
            body := v_payload
        );
    END IF;
    RETURN NEW;
END;
$$;
```

**Source:** Turn 10 (background processing), Turn 30 (async compression)

### 7.3 pgvector: Semantic Search

```sql
-- Enable extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Already defined on memory_events.embedding (§2.2)
-- IVFFlat index for approximate nearest neighbor search
CREATE INDEX idx_memory_embedding ON memory_events
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Semantic recall query (used in page fault, §4.3 of SPEC-002)
SELECT me.id, me.content, 1 - (me.embedding <=> $1::vector) AS similarity
FROM memory_events me
LEFT JOIN display_modes dm ON dm.memory_id = me.id
WHERE me.session_id = $2
  AND COALESCE(dm.mode, 'full') != 'hidden'
ORDER BY me.embedding <=> $1::vector
LIMIT 10;
```

**Source:** SPEC-002 §8 (vector-validated compression), §7.3 (semantic recall)

### 7.4 PL/v8: In-Database JavaScript

```sql
-- Enable extension
CREATE EXTENSION IF NOT EXISTS plv8;

-- Example: in-DB JSON transformation without round-tripping to external runtime
CREATE OR REPLACE FUNCTION transform_tool_output(input JSONB)
RETURNS JSONB
LANGUAGE plv8
AS $$
    const data = JSON.parse(input);
    // Agent-defined transformation logic stored in DB
    const result = data.items.map(item => ({
        id: item.id,
        summary: item.title + ': ' + item.description.substring(0, 100)
    }));
    return JSON.stringify({ results: result, count: result.length });
$$;
```

Use cases: lightweight data reshaping, field extraction, and format conversion that doesn't warrant a full external tool call.

**Source:** Turn 26 (in-DB computation), Turn 19 (reduce round-trips)

### 7.5 Row-Level Security

```sql
-- Enable RLS on all core tables
ALTER TABLE memory_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE display_modes ENABLE ROW LEVEL SECURITY;
ALTER TABLE iteration_commits ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_results ENABLE ROW LEVEL SECURITY;

-- Session-scoped isolation policy (applied to each table)
CREATE POLICY session_isolate_memory ON memory_events
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_display ON display_modes
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_iterations ON iteration_commits
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_tasks ON tasks
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_tool_req ON tool_requests
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);

CREATE POLICY session_isolate_tool_res ON tool_results
    FOR ALL USING (session_id = current_setting('conscience.session_id')::UUID);
```

#### Canonical RLS Role Model (SPEC-011 §13)

| Role | Purpose | RLS | Can UPDATE memory_events? |
|---|---|---|---|
| `agent_role` | Normal agent execution | Subject to RLS | No (INSERT/SELECT only) |
| `compression_worker` | Background compression | Subject to RLS | Yes (summary_text column only) |
| `alt_mode_role` | Admin operations | Bypasses RLS (BYPASSRLS) | Yes (full access) |
| `tool_executor` | Writes tool_results | Subject to RLS | No (writes tool_results only) |

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

**Source:** SPEC-001 §2.1 (RLS as security sandbox), Turn 34 (Alt-Mode bypass)

### 7.6 Vault: Secrets

```sql
-- Enable Supabase Vault for storing API keys, not in env vars or code
INSERT INTO vault.secrets (name, secret)
VALUES ('llm_api_key', 'sk-...');

-- Reading secrets in SECURITY DEFINER functions only
CREATE OR REPLACE FUNCTION get_llm_api_key()
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_key TEXT;
BEGIN
    SELECT decrypted_secret INTO v_key
    FROM vault.decrypted_secrets
    WHERE name = 'llm_api_key';
    RETURN v_key;
END;
$$;

-- Agent role cannot read vault directly
REVOKE SELECT ON vault.secrets FROM agent_role;
REVOKE SELECT ON vault.decrypted_secrets FROM agent_role;
```

**Source:** Turn 34 (secrets management), Turn 36 (least-privilege access)

---

## 8. SQLite / PocketBase Parity

### 8.1 Principle

One architecture, two deployment targets (SPEC-001 §2.3). The SQLite/PocketBase target must provide feature parity with PostgreSQL/Supabase for all critical behaviors.

**Source:** Turns 19-20 (write once, deploy anywhere)

### 8.2 Feature Parity Matrix

| Feature | PostgreSQL (Supabase) | SQLite (PocketBase) | Parity Strategy |
|---|---|---|---|
| Vector search | pgvector | sqlite-vec | Same cosine similarity API, different indexes |
| JSON Schema | pg_jsonschema | sqlite-jsonschema | Identical `ALTER TABLE ... CHECK` syntax |
| Row-Level Security | RLS policies | PocketBase API Rules | Application-layer enforcement in Go hooks |
| Background jobs | pg_cron | Go goroutines + ticker | PocketBase `OnServe` hook with scheduler |
| Async HTTP | pg_net | Go `net/http` | PocketBase middleware |
| JavaScript runtime | PL/v8 | PocketBase JSVM | Same purpose, different sandbox |
| Append-only enforcement | REVOKE UPDATE/DELETE | Go `OnRecordBeforeUpdateRequest` hook | Intercept and reject |
| Triggers | Native SQL triggers | Go hooks on lifecycle events | Same semantics, different mechanism |
| WAL mode | N/A (MVCC) | `PRAGMA journal_mode=WAL` | Required for concurrent writes |
| Secrets | Supabase Vault | Environment variables + encrypted `.env` | Both restrict agent access |
| Dynamic table creation | SECURITY DEFINER function | Go `OnRecordBeforeCreateRequest` hook | Same validation, different executor |

### 8.3 sqlite-vec: Vector Search

```sql
-- Load extension
SELECT load_extension('./sqlite-vec');

-- Create virtual table for embeddings
CREATE VIRTUAL TABLE memory_embeddings_vec USING vec0(
    id INTEGER PRIMARY KEY,
    embedding float[1536]
);

-- Insert embedding
INSERT INTO memory_embeddings_vec (id, embedding)
VALUES (1, '[0.1, 0.2, ...]');

-- Cosine similarity search
SELECT id, distance
FROM memory_embeddings_vec
WHERE embedding MATCH '[0.3, 0.1, ...]'
ORDER BY distance
LIMIT 10;
```

### 8.4 sqlite-jsonschema: JSON Validation

Identical syntax to PostgreSQL (§4.3). Both use ALTER TABLE CHECK constraints with JSON Schema documents.

### 8.5 WAL Mode for Concurrent Writes

```sql
-- Enable WAL mode on database open
PRAGMA journal_mode=WAL;
PRAGMA wal_autocheckpoint=1000;

-- Read concurrency: multiple readers don't block writers
-- Write serialization: only one writer at a time (handled by PocketBase)
```

### 8.6 PocketBase API Rules as Application-Layer RLS

PocketBase doesn't have database-level RLS. Instead, API Rules on collections enforce access control:

```javascript
// pb_hooks/rls_rules.js
// Session-isolated read: only return records belonging to the requesting session
onRecordBeforeRequest((e) => {
    const sessionId = e.requestInfo.auth?.record?.id;
    if (e.record.get('session_id') !== sessionId) {
        throw new BadRequestError('Session isolation violation');
    }
}, 'memory_events');

// Append-only enforcement on memory_events
onRecordBeforeUpdate((e) => {
    throw new BadRequestError('memory_events is append-only');
}, 'memory_events');

onRecordBeforeDelete((e) => {
    throw new BadRequestError('memory_events is append-only');
}, 'memory_events');
```

**Source:** Turns 19-20 (PocketBase parity), Turn 28 (API Rules as RLS equivalent)

### 8.7 Exponential Backoff + Jitter for Write Contention

SQLite has a single-writer constraint. PocketBase must handle write contention gracefully:

```javascript
// pb_hooks/write_contention.js
onRecordBeforeRequest(async (e) => {
    const maxRetries = 5;
    const baseDelayMs = 10;

    for (let attempt = 0; attempt < maxRetries; attempt++) {
        try {
            await e.next();
            return;
        } catch (err) {
            if (err.message?.includes('database is locked')) {
                const delay = baseDelayMs * Math.pow(2, attempt)
                    + Math.random() * baseDelayMs; // jitter
                await new Promise(r => setTimeout(r, delay));
                continue;
            }
            throw err;
        }
    }
    throw new Error('Max retries exceeded for write contention');
}, '*');
```

**Source:** Turn 19 (concurrent write handling), Turn 28 (PocketBase as production target)

---

## 9. Helper Functions & Triggers (Shared)

### 9.1 Soft Delete Intercept

```sql
CREATE OR REPLACE FUNCTION soft_delete_intercept()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    -- Instead of deleting, set deleted_at timestamp
    UPDATE pg_temp.soft_delete_target
    SET deleted_at = now()
    WHERE id = OLD.id;
    RETURN NULL; -- Cancel the DELETE
END;
$$;
```

> **Implementation note:** The generic trigger uses `TG_TABLE_NAME` in production to dynamically reference the calling table. The simplified version above illustrates the pattern. Dynamic DDL via `EXECUTE format('UPDATE %I SET deleted_at = now() WHERE id = $1.id', TG_TABLE_NAME) USING OLD;` handles all caller tables.

### 9.2 Update Timestamp Auto-Set

```sql
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;
```

### 9.3 Session Heartbeat Updater

```sql
CREATE OR REPLACE FUNCTION touch_session_heartbeat()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE sessions
    SET heartbeat_at = now(),
        iteration = iteration + 1
    WHERE id = NEW.session_id;
    RETURN NEW;
END;
$$;

-- Apply to tables that indicate agent activity
CREATE TRIGGER memory_touches_session
    AFTER INSERT ON memory_events
    FOR EACH ROW EXECUTE FUNCTION touch_session_heartbeat();

CREATE TRIGGER tool_req_touches_session
    AFTER INSERT ON tool_requests
    FOR EACH ROW EXECUTE FUNCTION touch_session_heartbeat();
```

### 9.4 Compression Queue

```sql
CREATE TABLE compression_queue (
    id              BIGSERIAL PRIMARY KEY,
    event_id        BIGINT NOT NULL REFERENCES memory_events(id),
    current_tier    INT NOT NULL DEFAULT 1,
    next_tier       INT NOT NULL DEFAULT 2,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts   INT NOT NULL DEFAULT 3,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_compression_pending ON compression_queue(status)
    WHERE status = 'pending';
```

> **Note:** Compression workers (SPEC-011 §13) execute in the background with elevated permissions to update `memory_events.summary_text` only. They also write to `display_modes` when setting a memory event to compressed.

---

## 10. Tables Defined in Other Specs

These tables are not repeated here to avoid duplication. Each is fully defined in its owning spec. All are included in the reserved names list in §3.2.

### 10.1 From SPEC-006 (Transactions)

| Table | Purpose |
|---|---|
| `agent_circuit_breakers` | Tracks consecutive errors, iteration limits, budget thresholds per session |
| `agent_budget_limits` | Per-agent budget caps (tokens, cost) by scope (per_task, per_hour, per_day) |
| `audit_logs` | Structured audit trail per iteration (monologue, SQL executed, commit/rollback result) |

### 10.2 From SPEC-011 §12 (Canonical Definitions)

| Table | Purpose |
|---|---|
| `agent_messages` | Parent→child inter-session communication channel |
| `system_settings` | Key-value system configuration (embedding_model, etc.) |

### 10.3 From SPEC-005 (Security)

| Table | Purpose |
|---|---|
| `secret_access_audit` | Audit log for every `{{SECRET.X}}` resolution |

### 10.4 From SPEC-013 (Webhooks)

| Table | Purpose |
|---|---|
| `external_events` | Universal inbox for all incoming events (webhook, email, cron, manual, API) |
| `webhook_registrations` | Defines accepted webhooks, HMAC secrets, event type filters, routing targets |
| `routing_rules` | Pattern-matching rules that route events to agents or workflows |

### 10.5 From SPEC-014 (HITL)

| Table | Purpose |
|---|---|
| `approval_requests` | Pending human approval requests with risk level, SQL preview, and decision fields |
| `hitl_configuration` | Per-session HITL settings (require_approval_for_destructive, etc.) |
| `notification_log` | Record of all HITL notifications sent (channel, recipient, status) |

### 10.6 From SPEC-015 (API & MCP)

| Table | Purpose |
|---|---|
| `api_keys` | API key hashes, scopes, session bindings, expiration |
| `api_rate_limits` | Per-key request counting with sliding window |

### 10.7 From SPEC-017 (UI Adapter Layer)

| Table | Purpose |
|---|---|
| `shim_session_map` | Maps external protocol sessions (opencode, MCP) to Conscience session IDs |

### 10.8 From SPEC-020 (Interactive Transaction Staging)

| Table | Purpose |
|---|---|
| `staging_buffer` | Agent's transaction scratchpad — staged commands, execution results, lifecycle status |

---

## 11. Cross-Reference Map

| Spec Section | Source Turns | Related SPEC-001 Sections | Related SPEC-002 Sections | Related SPEC-011 Sections |
|---|---|---|---|---|
| §2 Core Tables | 4-5, 9-11, 26 | §3 System Components | §2-6 | §3 (display_modes), §4 (tools/skills), §5 (custom_agent_tools), §6 (agent_billing) |
| §3 Dynamic Entity Generator | 27, 26, 34 | §2.1 DBaaR | §10 | — |
| §4 JSON Schema Validation | 28, 34 | §2.3 Write Once | §10.4 | — |
| §5 Constraint Types | 35, 36, 34 | §2.2 Atomic Cognition | — | — |
| §6 Token Caching | 30, 9 | — | §3 (View ordering) | §3.4 (canonical active_context_view) |
| §7 Postgres Features | 10, 12, 26, 34 | §2.1 DBaaR, §5 Hemispheres | §8 (pgvector) | §9 (namespace), §13 (role model) |
| §8 SQLite Parity | 19, 20, 28 | §2.3 Write Once | — | §9.5 (SET LOCAL in SQLite) |
| §9 Helper Functions | 9, 10 | §4 Core Data Flow | §6-7 | §13 (compression_worker role) |

---

## 12. Open Questions

1. **Dynamic table cleanup**: Should dynamically created tables have a TTL? When a session ends, should its dynamic tables be dropped, or just soft-deleted via `deleted_at` on all rows? — *Remains open (SPEC-011 §15 Q1).*
2. **Compression queue concurrency**: With multiple harness workers processing the queue, `SELECT ... FOR UPDATE SKIP LOCKED` prevents double-processing on Postgres. SQLite requires application-layer locking (Go mutex).
3. **PL/v8 alternatives**: PL/v8 requires a C extension and may not be available on all Supabase plans or self-hosted Postgres. The Go binary handles all procedural logic natively — PL/v8 is not needed.
4. **API Rules vs true RLS**: PocketBase API Rules are application-layer, not kernel-layer. A bug in the Go hook layer could expose data. Should we add a secondary checks in the JSVM layer?
5. **sqlite-vec index rebuilding**: sqlite-vec does not support incremental index updates. When embeddings change, the entire virtual table must be rebuilt. Strategy needed for production-scale reindexing.
6. **Token cache metrics**: How to measure cache hit rates in production? Both Anthropic and OpenAI return cache-related usage metadata. Should this be stored in `agent_billing`? — *Now possible with `cache_read_tokens` and `cache_write_tokens` columns in the canonical `agent_billing` schema (SPEC-011 §6.2).*