# SPEC-007: LLM JSON Output Schema

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

## Overview

The Consensus framework uses a hybrid approach for LLM output: a structured JSON wrapper containing raw SQL statements. This combines reliable parsing (JSON) with expressive power (SQL) without inventing a custom DSL.

## Why Not a Custom DSL

A custom domain-specific language would require:
- A custom parser with error recovery
- LLM fine-tuning on the new syntax
- Ongoing maintenance of the parser as requirements evolve
- Double the surface area for bugs (parser + execution)

SQL is already a well-specified language that LLMs are extensively trained on. The JSON wrapper provides structure; SQL provides expressiveness.

## Why Not Raw SQL Without JSON

Raw SQL in LLM output requires regex-based extraction:
- Distinguishing agent reasoning from executable statements is fragile
- Multi-statement transactions need delimiter parsing
- Monologue/audit data has no natural home
- Adding new instruction types requires regex changes

JSON gives us reliable structural parsing for free.

## Output Schema

```json
{
  "internal_monologue": "I need to compress memory 104 and queue a web scrape",
  "memory_state_changes": [
    "UPDATE active_context SET display_mode = 'compressed' WHERE event_id = 104",
    "INSERT INTO memory_pages (name, target_ids) VALUES ('api_research', ARRAY[105, 106])"
  ],
  "system_actions": [
    "CREATE TABLE user_auth_schema (id UUID, data JSONB)"
  ]
}
```

### Field Definitions

| Field | Type | Required | Purpose |
|---|---|---|---|
| `internal_monologue` | `string` | Yes | Agent's reasoning for this iteration. Captured for audit ledger without stuffing reasoning into SQL comments. |
| `memory_state_changes` | `string[]` | No | SQL statements that modify the agent's own memory space. Executed within the iteration transaction. |
| `system_actions` | `string[]` | No | SQL statements that interact with the system (create tables, call stored procedures, request tools). Executed within the iteration transaction. |

### Extended Fields (optional, used as needed)

```json
{
  "internal_monologue": "...",
  "memory_state_changes": ["..."],
  "system_actions": ["..."],
  "tool_requests": [
    {
      "tool_name": "web_scraper",
      "parameters": { "url": "https://api.example.com/docs" },
      "justification": "Need to fetch API schema for documentation task"
    }
  ],
  "sub_agent_spawns": [
    {
      "instruction": "Research authentication patterns in the codebase",
      "model_preference": "gpt-4o-mini"
    }
  ],
  "task_update": {
    "status": "COMPLETED",
    "result_summary": "Documentation generated for 23 endpoints"
  }
}
```

## Parsing Flow

```
LLM Response
    │
    ▼
JSON Parse (guaranteed by Structured Outputs)
    │
    ├─ Extract internal_monologue → audit_logs table
    │
    ├─ Extract memory_state_changes → SQL execution queue
    │
    ├─ Extract system_actions → SQL execution queue
    │
    ├─ Extract tool_requests → tool execution with quarantine
    │
    ├─ Extract sub_agent_spawns → tasks table inserts
    │
    └─ Extract task_update → stored procedure calls
```

### Harness parsing logic

```typescript
interface AgentResponse {
  internal_monologue: string;
  memory_state_changes?: string[];
  system_actions?: string[];
  tool_requests?: ToolRequest[];
  sub_agent_spawns?: SubAgentSpawn[];
  task_update?: TaskUpdate;
}

function parseResponse(raw: string): AgentResponse {
  const parsed: AgentResponse = JSON.parse(raw);

  if (!parsed.internal_monologue) {
    throw new Error("Missing required field: internal_monologue");
  }

  return parsed;
}
```

No regex. No delimiter parsing. Just `JSON.parse()`.

## Structured Outputs Guarantee

Both OpenAI and Anthropic support Structured Outputs (JSON Schema enforcement):

```typescript
const response = await openai.chat.completions.create({
  model: "gpt-4o",
  response_format: {
    type: "json_schema",
    json_schema: {
      name: "agent_response",
      strict: true,
      schema: {
        type: "object",
        properties: {
          internal_monologue: { type: "string" },
          memory_state_changes: { type: "array", items: { type: "string" } },
          system_actions: { type: "array", items: { type: "string" } }
        },
        required: ["internal_monologue"],
        additionalProperties: false
      }
    }
  }
});
```

This guarantees the payload shape at the API level. The harness never has to handle malformed JSON.

## Audit Ledger

`internal_monologue` is captured separately from SQL execution:

```sql
INSERT INTO audit_logs (session_id, iteration, monologue, sql_executed, result)
VALUES (
  'session-uuid',
  14,
  'I need to compress memory 104 and queue a web scrape',
  ARRAY[
    'UPDATE display_modes SET mode = ''compressed'' WHERE memory_id = 104',
    'INSERT INTO memory_pages (name, target_ids) VALUES (''api_research'', ARRAY[105, 106])'
  ],
  'committed'
);
```

This separation provides:

- **Auditability**: Every decision has a human-readable rationale
- **Debugging**: Developers can trace why an agent took an action
- **Compliance**: Monologue is not mixed into SQL comments where it could affect execution
- **Training data**: Monologue-action pairs form a dataset for fine-tuning

## SQL Inside JSON — Safety

SQL strings inside JSON are parameterless. All values are embedded directly by the LLM. This is safe because:

1. The LLM is generating SQL against its own schema — it knows the table structures
2. RLS prevents cross-session effects (SPEC-005)
3. Transactions prevent partial state (SPEC-006)
4. Stored procedures wrap destructive operations (SPEC-006)
5. SQL injection from external data is blocked by the cognitive firewall (SPEC-005)

The alternative — parameterized queries with separate parameters — would require a more complex JSON structure and a custom query builder. The simplicity of raw SQL inside JSON outweighs the theoretical risk, given the multiple safety layers.

## SQL Injection Mitigation

SQL from LLM output is classified before execution (SPEC-011 §8):

1. **Statement Classifier**: Each SQL statement is classified as `DML_READ`, `DML_WRITE`, `DDL_CREATE`, `DDL_ALTER`, `DML_DELETE`, or `DANGEROUS`
2. **Execution Policy**: `DANGEROUS` statements (DROP, TRUNCATE, GRANT, REVOKE) are never executed. `DML_DELETE` is restricted to stored procedures only. `DML_WRITE` must target allowed tables (core + dynamic agent tables via whitelist).
3. **Table Whitelist**: The harness maintains a whitelist of core tables and dynamically discovers agent-created tables. DML_WRITE statements targeting tables not in the whitelist are blocked.

This replaces the assumption that raw SQL in JSON is safe purely because of RLS. RLS limits blast radius but does not prevent all damage (e.g., an agent can still delete its own data).

## Extensibility

New instruction types are added as new top-level keys:

```json
{
  "internal_monologue": "...",
  "memory_state_changes": [],
  "system_actions": [],
  "future_instruction_type": { "...": "..." }
}
```

The harness ignores unknown keys by default, enabling backward-compatible schema evolution. Adding a new instruction type requires:
1. Update the JSON schema in the LLM prompt
2. Add a handler in the harness
3. Add any new tables or stored procedures

No parser changes needed.