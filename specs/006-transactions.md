# SPEC-006: Atomic Cognition — Transactions, Rollback & Error Handling

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

## Overview

Every iteration of the agent loop is wrapped in a SQL transaction. This guarantees atomic cognition: either all state changes from an iteration succeed, or none do. There is no partial state. The agent never operates on corrupted or half-applied state.

## The Transaction Loop

```
BEGIN
  → Read active_context_view (materialized snapshot)
  → Format context for LLM
  → Call LLM → receive JSON response
  → Parse JSON, extract SQL statements
  → Execute SQL statements sequentially
  → COMMIT or ROLLBACK
```

If any SQL statement fails, the entire transaction is rolled back. The agent receives the exact Postgres error in its next context iteration.

## Atomic Iteration

```typescript
async function executeIteration(sessionId: string): Promise<void> {
  const tx = await db.beginTransaction();

  try {
    const context = await tx.selectFrom('active_context_view')
      .where('session_id', '=', sessionId)
      .execute();

    const llmResponse = await callLLM(formatContext(context));
    const sqlStatements = parseJsonResponse(llmResponse);

    for (const sql of sqlStatements) {
      await tx.execute(sql);
    }

    await tx.commit();
    await saveIterationSnapshot(sessionId, llmResponse);

  } catch (error) {
    await tx.rollback();
    await injectErrorIntoNextContext(sessionId, error);
  }
}
```

### Key properties

- **Atomicity**: All SQL in one iteration commits together or rolls back together
- **Isolation**: Each iteration sees a consistent snapshot via `active_context_view`
- **Durability**: Committed state persists in Postgres WAL
- **Error transparency**: The agent sees the exact Postgres error message, enabling self-correction

## Error Handling Examples

### Hallucinated column

```
Iteration 1:
  Agent outputs: SELECT title, nonexistent_col FROM memory_events
  Postgres error: column "nonexistent_col" does not exist
  → ROLLBACK
  → Agent receives error in next context

Iteration 2:
  Agent corrects: SELECT title, content FROM memory_events
  → COMMIT
```

### Constraint violation

```
Iteration 1:
  Agent outputs: INSERT INTO tasks (session_id, status) VALUES ('sid', 'INVALID_STATUS')
  Postgres error: invalid input value for enum task_status: "INVALID_STATUS"
  → ROLLBACK

Iteration 2:
  Agent corrects: INSERT INTO tasks (session_id, status) VALUES ('sid', 'pending')
  → COMMIT
```

### RLS policy violation (logical error)

Syntactically valid SQL that violates logical constraints is caught by RLS:

```
Iteration 1:
  Agent outputs: DELETE FROM tool_requests  -- no WHERE clause
  Postgres executes: DELETE FROM tool_requests WHERE session_id = current_setting('consensus.session_id')::UUID
  → Only the agent's own rows are deleted (RLS scoping)
  → COMMIT, but impact is limited to agent's own data
```

Raw destructive DML without proper scoping is either:
- Made safe by RLS (DELETE only affects agent's own rows)
- Blocked by stored procedure interface (see below)

## Stored Procedures for Destructive Operations

For operations where raw DML is dangerous even with RLS, the framework exposes stored procedures:

```sql
CREATE OR REPLACE FUNCTION complete_task(task_id UUID)
RETURNS void AS $$
BEGIN
  UPDATE tasks
  SET status = 'published', completed_at = now()
  WHERE id = task_id
    AND session_id = current_setting('consensus.session_id')::UUID;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
```

The agent calls:

```sql
SELECT complete_task(task_id := 'task-uuid');
```

Instead of:

```sql
UPDATE tasks SET status = 'published' WHERE id = 'task-uuid';
```

Benefits:
- The function enforces scoping internally
- The function can add validation logic (e.g., prevent completing an already-completed task)
- The agent cannot bypass the function's constraints via raw SQL
- `SECURITY DEFINER` allows controlled privilege elevation

Operations that require stored procedures:

| Operation | Procedure | Why |
|---|---|---|
| Complete a task | `complete_task()` | Enforces state transitions |
| Cancel a task | `cancel_task()` | Enforces state transitions, triggers cleanup |
| Create sub-agent | `spawn_subagent()` | Validates parent exists, sets parent to SLEEPING |
| Archive memory | `archive_memory()` | Enforces soft delete, records reason |
| Approve tool use | `approve_tool()` | Validates tool exists, records consent |

## Circuit Breakers

Unbounded agent loops are dangerous. The framework enforces hard limits:

### max_consecutive_errors (default: 3)

If an agent produces 3 consecutive errors, it is paused and an alert is sent:

```sql
CREATE TABLE agent_circuit_breakers (
  session_id UUID REFERENCES sessions(id),
  breaker_type TEXT NOT NULL,       -- 'consecutive_errors', 'iterations', 'budget'
  threshold INT NOT NULL,
  current_count INT NOT NULL DEFAULT 0,
  tripped_at TIMESTAMPTZ,
  PRIMARY KEY (session_id, breaker_type)
);
```

### max_iterations_per_task (default: 50)

No task can run more than 50 iterations. If the limit is hit, the task is marked `FAILED` with reason `max_iterations_exceeded`.

### Budget limits

Per-agent, per-task, and per-hour budget caps:

```sql
CREATE TABLE agent_budget_limits (
  agent_id UUID REFERENCES sessions(id),
  scope TEXT NOT NULL,              -- 'per_task', 'per_hour', 'per_day'
  max_tokens INT NOT NULL,
  max_cost_cents INT NOT NULL,
  PRIMARY KEY (agent_id, scope)
);
```

Before each LLM call, the harness checks cumulative spend against limits.

## Agent Billing

Every API response is recorded for cost tracking and auditing:

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

This data feeds into:
- Budget circuit breakers (real-time spend tracking)
- Cost dashboards (historical analysis)
- Model routing decisions (cheapest capable model selection)

## Audit Logs

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

## Error Classification

| Error Type | Example | Handling |
|---|---|---|
| Syntax error | Misspelled SQL keyword | ROLLBACK → agent self-corrects from error message |
| Semantic error | Wrong column name | ROLLBACK → agent self-corrects from error message |
| Constraint violation | Invalid enum value | ROLLBACK → agent self-corrects from error message |
| RLS violation | Cross-session access attempt | ROLLS silently via RLS — agent sees empty result set |
| Logical destructiveness | Unscoped DELETE | Made safe by RLS or blocked by stored procedure |
| LLM hallucination | Fabricated table name | ROLLBACK → Postgres error provides ground truth |
| Budget exceeded | Token limit hit | Task paused, alert sent to admin |

## Iteration Snapshot

> Iteration snapshots are now stored in `iteration_commits.llm_response` and `iteration_commits.sql_executed` columns (SPEC-011 §11). The separate `iteration_snapshots` table is eliminated.

## Two-Phase Execution

> Tool execution now happens OUTSIDE the main cognition transaction (SPEC-011 §7). The cognition transaction handles memory state changes only. Tool requests are written as pending rows and executed asynchronously.