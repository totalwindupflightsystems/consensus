# SPEC-012: System Prompt & Schema Discovery Protocol

**Status:** Draft
**Depends On:** SPEC-011 (Canonical Definitions), SPEC-003 (Database Schema)
**Created:** 2026-04-08

---

## 1. Overview

Every agent iteration begins with a system prompt that tells the LLM what it is, what tools it has, what schema it operates on, and what constraints it must follow. This spec defines how that system prompt is assembled, how the agent discovers available schema and tools at runtime, and how progressive disclosure keeps token costs manageable.

The system prompt is not hardcoded. It is assembled dynamically from database rows — the same database the agent reads and writes. This means:

- Adding a tool = `INSERT INTO tools_registry`
- Adding a skill = `INSERT INTO skills_registry`
- Discovering available capabilities = `SELECT FROM tools_registry / skills_registry`
- The prompt changes when the database changes — no redeployment needed

**Source:** Gemini Chat Turns 16-17 (system prompt design), Turn 38 (schema discovery)

---

## 2. System Prompt Architecture

### 2.1 Layer Structure

The system prompt follows the same caching hierarchy as the active context view (SPEC-002 §6, SPEC-003 §6.2):

```
┌──────────────────────────────────────────────────┐
│  LAYER 1: IDENTITY & RULES                        │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • Agent name, role, and constraints               │
│  • Unbypassable rules (from constraints table)     │
│  • Session context (goal, budget, iteration limit) │
│  • IMMUTABLE across iterations. ALWAYS CACHED.     │
│                                                    │
│  LAYER 2: SCHEMA & TOOLS                           │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • Available tables (core + dynamic)               │
│  • Tool registry (name, description, parameters)   │
│  • Skill metadata (name, short description)        │
│  • Changes rarely (only when tools/skills update). │
│  • CACHED after first call.                        │
│                                                    │
│  LAYER 3: ACTIVE CONTEXT VIEW                      │
│  ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄  │
│  • Current memory events (from active_context_view)│
│  • Tool results from previous iteration             │
│  • Dynamic scratchpad content                       │
│  • Changes EVERY iteration. RARELY CACHED.         │
│                                                    │
└──────────────────────────────────────────────────┘
```

### 2.2 Assembly Query

```sql
WITH session_info AS (
    SELECT id, agent_name, goal, context_budget, iteration, status
    FROM sessions
    WHERE id = current_setting('conscience.session_id')::UUID
),
available_tools AS (
    SELECT name, description, hemisphere, handler_type, parameter_schema
    FROM tools_registry
    WHERE enabled = true
      AND (owner_session_id IS NULL OR owner_session_id = current_setting('conscience.session_id')::UUID)
    ORDER BY hemisphere, name
),
available_skills AS (
    SELECT name, metadata
    FROM skills_registry
    WHERE enabled = true
    ORDER BY name
),
core_tables AS (
    SELECT table_name
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_type = 'BASE TABLE'
      AND table_name IN (
          'sessions', 'memory_events', 'display_modes', 'iteration_commits',
          'memory_pages', 'tasks', 'tool_requests', 'tool_results',
          'tools_registry', 'skills_registry', 'custom_agent_tools',
          'agent_billing', 'workflows', 'tool_files', 'external_quarantine',
          'model_registry', 'compression_queue', 'display_modes',
          'agent_messages', 'system_settings', 'audit_logs'
      )
),
dynamic_tables AS (
    SELECT table_name
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_type = 'BASE TABLE'
      AND table_name NOT IN (
          SELECT table_name FROM core_tables
      )
)
SELECT
    si.agent_name,
    si.goal,
    si.context_budget,
    si.iteration,
    si.status,
    (SELECT json_agg(core_tables) FROM core_tables) AS core_schema,
    (SELECT json_agg(dynamic_tables) FROM dynamic_tables) AS dynamic_schema,
    (SELECT json_agg(row_to_json(t)) FROM available_tools t) AS tools,
    (SELECT json_agg(row_to_json(s)) FROM available_skills s) AS skills
FROM session_info si;
```

---

## 3. Layer 1: Identity & Rules

### 3.1 Template

```markdown
# Identity

You are **{agent_name}**, an AI agent operating within the Conscience framework.
You manage your own cognition through SQL statements. You do not run shell commands.
You read from and write to a relational database. Every thought and action is a SQL transaction.

## Goal

{goal}

## Constraints

- You are on iteration {iteration} of a maximum {max_iterations}.
- Your total budget is {context_budget} tokens.
- You cannot UPDATE or DELETE rows in `memory_events`. It is append-only.
- You cannot DROP tables, TRUNCATE, GRANT, or REVOKE.
- You must use stored procedures for destructive operations (complete_session, set_display_mode, soft_delete).
- All your queries are scoped to your session via RLS. You cannot access other sessions' data.

## Output Format

Respond with a JSON object:
{
  "internal_monologue": "Your reasoning for this iteration",
  "memory_state_changes": ["SQL statements to modify your memory/context"],
  "system_actions": ["SQL statements for system operations"],
  "tool_requests": [{"tool_name": "...", "parameters": {...}, "justification": "..."}],
  "sub_agent_spawns": [{"instruction": "...", "model_preference": "..."}],
  "task_update": {"status": "...", "result_summary": "..."}
}

Only include fields you need. `internal_monologue` is always required.
```

### 3.2 Constraint Source

Unbypassable rules from database constraints are included in the system prompt so the LLM knows what will be rejected:

```sql
SELECT conname, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = 'memory_events'::regclass
   OR conrelid = 'tasks'::regclass
   OR conrelid = 'sessions'::regclass;
```

This produces human-readable descriptions like:
- `CHECK (status IN ('booting', 'idle', 'thinking', ...))` → "Session status must be one of: booting, idle, thinking, ..."
- `CHECK (hemisphere IN ('internal', 'external'))` → "Tool hemisphere must be 'internal' or 'external'"

---

## 4. Layer 2: Schema & Tools

### 4.1 Progressive Disclosure

The system prompt includes ONLY metadata for tools and skills — not full instructions:

```markdown
## Available Tools

| Tool | Hemisphere | Handler | Description |
|------|-----------|---------|-------------|
| web_scrape | external | http_endpoint | Scrape a URL and return raw HTML |
| vector_search | internal | sql_function | Semantic search across memory events |
| create_table | internal | sql_function | Provision a new dynamic entity table |

## Available Skills (metadata only — use `load_skill` to fetch instructions)

| Skill | Description |
|-------|------------|
| excel_generator | Generate Excel spreadsheets from query results |
| api_documenter | Document REST API endpoints from code |
```

### 4.2 Skill Loading on Demand

When the agent determines it needs a skill:

```sql
SELECT load_skill('excel_generator');
-- Returns the full instruction text, injected into the next iteration's context
```

This keeps the system prompt small (~100 tokens per skill for metadata) and only expands context when needed.

### 4.3 Dynamic Schema Discovery

Dynamic tables created by the agent (`create_agent_memory_table()`) appear in the schema section:

```markdown
## Your Dynamic Tables

| Table | Purpose | Created |
|-------|---------|---------|
| order_tracking | You created this 3 iterations ago | 2026-04-08 14:32 |
```

The agent discovers its own tables by querying:

```sql
SELECT table_name, obj_description((quote_ident(table_name))::regclass, 'pg_class') as comment
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name NOT IN ({CORE_TABLES})
  AND table_name IN (
      SELECT table_name FROM session_dynamic_tables
      WHERE session_id = current_setting('conscience.session_id')::UUID
  );
```

### 4.4 JSON Schema for Output Format

The LLM output format is enforced via Structured Outputs (SPEC-007). The system prompt describes the JSON structure, but the actual enforcement happens at the API level:

```json
{
  "type": "object",
  "properties": {
    "internal_monologue": { "type": "string" },
    "memory_state_changes": { "type": "array", "items": { "type": "string" } },
    "system_actions": { "type": "array", "items": { "type": "string" } },
    "tool_requests": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "tool_name": { "type": "string" },
          "parameters": { "type": "object" },
          "justification": { "type": "string" }
        },
        "required": ["tool_name", "parameters"]
      }
    },
    "sub_agent_spawns": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "instruction": { "type": "string" },
          "model_preference": { "type": "string" }
        },
        "required": ["instruction"]
      }
    },
    "task_update": {
      "type": "object",
      "properties": {
        "status": { "type": "string", "enum": ["completed", "failed"] },
        "result_summary": { "type": "string" }
      }
    }
  },
  "required": ["internal_monologue"]
}
```

---

## 5. Prompt Caching Strategy

### 5.1 Cache Breakpoints

```typescript
const messages = [
    {
        role: "system",
        content: layer1_identity + layer2_schema_tools,
        cache_control: { type: "ephemeral" }  // Cache breakpoint 1
    },
    {
        role: "user",
        content: layer2_context_view,           // Cache breakpoint 2
        cache_control: { type: "ephemeral" }
    },
    {
        role: "assistant",
        content: previous_llm_response || "",   // No cache — changes every time
    },
    {
        role: "user",
        content: current_dynamic_context         // No cache
    }
];
```

### 5.2 Invalidation Rules

| Change | Layer | Cache Impact |
|---|---|---|
| New tool/skill registered | Layer 2 | Partial — regenerate tools section |
| Dynamic table created | Layer 2 | Partial — add to dynamic tables |
| New memory event | Layer 3 | Expected — always volatile |
| Display mode change | Layer 3 | Expected — always volatile |
| Constraint added (ALTER TABLE) | Layer 2 | Partial — add to constraints |
| Session goal change | Layer 1 | Full — regenerate identity |

### 5.3 Cache Cost Tracking

All cache hits and misses are recorded in `agent_billing`:

```sql
INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd)
VALUES (...);
```

This enables analysis of cache efficiency over time (SPEC-011 §6).

---

## 6. Sub-Agent Prompt Variation

When a sub-agent is spawned, its system prompt is assembled from the same query but with different parameters:

```sql
-- Sub-agent gets filtered tools (only internal hemisphere + approved external)
SELECT name, description, hemisphere, handler_type, parameter_schema
FROM tools_registry
WHERE enabled = true
  AND (hemisphere = 'internal' OR requires_approval = false)
  AND (owner_session_id IS NULL OR owner_session_id = :child_session_id)
```

Sub-agents inherit:
- Core schema knowledge (same tables)
- Filtered tool access (fewer external tools by default)
- Their own session goal (from spawn instruction)
- Compressed memory pointers (not full context — SPEC-004 §)

Sub-agents do NOT inherit:
- Parent's full memory_events (only compressed pointers)
- Parent's dynamic tables (they can discover and create their own)
- Parent's tool approvals (must earn their own)

---

## 7. Harness Assembly Code

```typescript
async function assembleSystemPrompt(sessionId: string): Promise<string> {
    const config = await db.selectFrom('sessions')
        .where('id', '=', sessionId)
        .select(['agent_name', 'goal', 'context_budget', 'iteration', 'status'])
        .executeSingle();

    const tools = await db.selectFrom('tools_registry')
        .where('enabled', '=', true)
        .select(['name', 'description', 'hemisphere', 'handler_type', 'parameter_schema'])
        .execute();

    const skills = await db.selectFrom('skills_registry')
        .where('enabled', '=', true)
        .select(['name', 'metadata'])
        .execute();

    const coreTables = await getCoreTables();
    const dynamicTables = await getDynamicTables(sessionId);
    const constraints = await getSessionConstraints(sessionId);

    return formatSystemPrompt({
        agentName: config.agent_name,
        goal: config.goal,
        contextBudget: config.context_budget,
        iteration: config.iteration,
        maxIterations: MAX_ITERATIONS,
        tools,
        skills,
        coreTables,
        dynamicTables,
        constraints
    });
}
```

---

## 8. Open Questions

1. **Prompt size vs. detail tradeoff**: Should dynamic tables include their JSONB schema in the system prompt, or should the agent query it on demand? Including it costs tokens; not including it risks hallucinated columns.
2. **Skill metadata granularity**: How many tokens should skill metadata consume? 50? 100? Need benchmarking with real skill descriptions.
3. **Multi-model prompt format**: Anthropic uses `cache_control` in content blocks; OpenAI uses `type: "ephemeral"`. The harness must abstract this.
4. **Constraint discovery frequency**: Should constraints be queried every iteration (accurate but slow) or cached for the session (fast but stale if ALTER TABLE runs mid-session)?