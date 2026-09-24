# SPEC-008: The Harness — Execution Loop & Runtime

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

## Overview

The Harness is the universal translator between LLM JSON output and database SQL execution. It is the only component that talks to both the LLM and the database. It is the runtime that drives every agent iteration.

The harness is a **long-running Go process**, not a serverless function. It holds open transactions, manages multi-turn planning sessions (SPEC-020), and persists across many agent iterations. It connects to any supported database backend via a driver interface.

## Core Loop

```
┌─────────────────────────────────────────────┐
│                  HARNESS                     │
│                                             │
│  1. Read active_context_view from DB        │
│  2. Format as Markdown context              │
│  3. Send to LLM with system instructions    │
│  4. Parse JSON response                     │
│  5. Extract SQL statements                  │
│  6. Execute in transaction (BEGIN/COMMIT)   │
│  7. On error: ROLLBACK, inject error        │
│  8. On success: COMMIT, save snapshot       │
│  9. Inject secrets before execution         │
│  10. Scrub secrets from responses           │
│  11. Loop or complete                       │
│                                             │
└─────────────────────────────────────────────┘
```

## Deployment Model

The harness is a **single Go binary** (`consensus`) that contains all runtime components:

```
┌─────────────────────────────────────────┐
│           Go Binary (consensus)         │
│                                         │
│  ┌─────────┐  ┌──────┐  ┌───────────┐  │
│  │ Harness  │  │ REST │  │ MCP Server│  │
│  │ Loop     │  │ API  │  │           │  │
│  └────┬─────┘  └──┬───┘  └─────┬─────┘  │
│       │           │            │         │
│       └───────────┴────────────┘         │
│                   │                      │
│          Database Driver Interface       │
└───────────────────┬─────────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
   ┌────┴─────┐          ┌─────┴──────┐
   │ Postgres │          │   SQLite   │
   │ (pgx)    │          │ (embedded) │
   └──────────┘          └────────────┘
```

The binary connects to whatever database is configured:

| Setup | Database | Connection |
|---|---|---|
| Local development | SQLite (embedded in binary) | `--db sqlite://data.db` |
| Self-hosted Supabase | Local Postgres | `--db postgres://localhost:5432/consensus` |
| Supabase Cloud | Hosted Postgres | `--db postgres://postgres:pass@db.xxx.supabase.co:5432/postgres` |
| Any Postgres | Any provider | `--db postgres://...` |

One binary. One flag. Same runtime regardless of where the database lives.

### Why Go, not TypeScript/Deno

The harness is a **persistent worker process**, not a request-response handler:
- SPEC-020 interactive transactions need 60+ seconds of open transaction time
- The heartbeat loop runs continuously, polling for ready tasks
- Tool execution requires sandboxed subprocess management
- A single binary with no runtime dependencies is the simplest deployment

Edge Functions (Deno) have 25-150 second max execution limits, no persistent connections, and cold starts on every invocation. They are suitable for the REST API layer but not for the harness loop.

## Detailed Execution

```go
func RunAgentIteration(ctx context.Context, sessionID string) (*IterationResult, error) {
    // 1. Read context
    context, err := ReadActiveContext(ctx, sessionID)
    if err != nil {
        return nil, err
    }

    // 2. Format for LLM
    messages := []Message{
        {Role: "system", Content: systemInstructions},
        {Role: "user", Content: FormatContextAsMarkdown(context)},
    }

    // 3. Call LLM
    rawResponse, err := CallLLM(ctx, messages, WithJSONSchema())
    if err != nil {
        return nil, err
    }

    // 4. Parse JSON
    parsed, err := ParseAgentResponse(rawResponse)
    if err != nil {
        return nil, err
    }

    // PHASE 1: Cognition Transaction (fast, sub-second)
    tx, err := db.BeginTx(ctx)
    if err != nil {
        return nil, err
    }

    defer func() {
        if tx.IsActive() {
            tx.Rollback()
        }
    }()

    if err := tx.SetSessionContext(ctx, sessionID); err != nil {
        return nil, err
    }

    // Execute memory_state_changes
    for _, stmt := range SplitStatements(parsed.MemoryStateChanges) {
        classified := ClassifyStatement(stmt)
        EnforceExecutionPolicy(classified, stmt, sessionID, dynamicTables)
        if err := tx.Execute(ctx, Sanitize(stmt)); err != nil {
            tx.Rollback()
            LogError(ctx, sessionID, err)
            return &IterationResult{Status: "error", Error: FormatPGError(err)}, nil
        }
    }

    // Execute system_actions
    for _, stmt := range SplitStatements(parsed.SystemActions) {
        classified := ClassifyStatement(stmt)
        EnforceExecutionPolicy(classified, stmt, sessionID, dynamicTables)
        if err := tx.Execute(ctx, Sanitize(stmt)); err != nil {
            tx.Rollback()
            LogError(ctx, sessionID, err)
            return &IterationResult{Status: "error", Error: FormatPGError(err)}, nil
        }
    }

    // Write tool requests as pending rows
    for _, toolReq := range parsed.ToolRequests {
        if err := tx.Execute(ctx, `
            INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status)
            VALUES ($1, $2, $3, $4, 'pending')
        `, sessionID, iterationID, toolReq.ToolName, toolReq.Parameters); err != nil {
            tx.Rollback()
            return nil, err
        }
    }

    // Write sub-agent spawn requests
    for _, spawn := range parsed.SubAgentSpawns {
        if err := tx.Execute(ctx, `
            INSERT INTO tasks (session_id, title, description, status)
            VALUES ($1, $2, $3, 'pending')
        `, sessionID, spawn.Instruction, spawn.Instruction); err != nil {
            tx.Rollback()
            return nil, err
        }
    }

    // Update session status
    newStatus := "idle"
    if len(parsed.ToolRequests) > 0 {
        newStatus = "tool_exec"
    } else if len(parsed.SubAgentSpawns) > 0 {
        newStatus = "waiting_sub"
    }
    if err := tx.Execute(ctx, `
        UPDATE sessions SET status = $1, heartbeat_at = now() WHERE id = $2
    `, newStatus, sessionID); err != nil {
        tx.Rollback()
        return nil, err
    }

    if err := tx.Commit(); err != nil {
        return nil, err
    }

    // PHASE 2: Tool Execution (async, long-running, no transaction)
    if len(parsed.ToolRequests) > 0 {
        if err := ToolExecutionPhase(ctx, sessionID, iterationID); err != nil {
            return nil, err
        }
    }

    // Save audit log
    scrubbed := ScrubSecrets(rawResponse)
    SaveAuditLog(ctx, sessionID, parsed.InternalMonologue, scrubbed)

    return &IterationResult{Status: "success"}, nil
}

func ToolExecutionPhase(ctx context.Context, sessionID string, iterationID int64) error {
    requests, err := db.QueryContext(ctx, `
        SELECT * FROM tool_requests
        WHERE session_id = $1 AND iteration_id = $2 AND status = 'pending'
    `, sessionID, iterationID)
    if err != nil {
        return err
    }

    for _, req := range requests {
        result, err := ExecuteToolSandboxed(ctx, req)
        if err != nil {
            // Write failure in mini-transaction
            tx, _ := db.BeginTx(ctx)
            tx.SetSessionContext(ctx, sessionID)
            tx.Execute(ctx, `
                UPDATE tool_requests SET status = 'failed', completed_at = now() WHERE id = $1
            `, req.ID)
            tx.Execute(ctx, `
                INSERT INTO tool_results (request_id, session_id, output, is_error, error_code)
                VALUES ($1, $2, $3, true, 'TOOL_ERROR')
            `, req.ID, sessionID, err.Error())
            tx.Commit()
            continue
        }

        tx, _ := db.BeginTx(ctx)
        tx.SetSessionContext(ctx, sessionID)
        tx.Execute(ctx, `
            UPDATE tool_requests SET status = 'completed', completed_at = now() WHERE id = $1
        `, req.ID)
        tx.Execute(ctx, `
            INSERT INTO tool_results (request_id, session_id, output, is_error, token_count)
            VALUES ($1, $2, $3, false, $4)
        `, req.ID, sessionID, result.Output, result.TokenCount)
        tx.Commit()
    }

    // After all tools complete, transition session back to thinking
    db.Execute(ctx, `
        UPDATE sessions SET status = 'thinking', heartbeat_at = now()
        WHERE id = $1 AND status = 'tool_exec'
    `, sessionID)

    return nil
}
```

## Heartbeat

The harness is a persistent process that polls for ready tasks on a fixed interval. It does not rely on serverless invocation.

### Heartbeat Loop

```go
func StartHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            task, err := ClaimNextReadyTask(ctx)
            if err != nil || task == nil {
                continue // no ready tasks
            }
            go RunAgentIteration(ctx, task.SessionID)
        }
    }
}

func ClaimNextReadyTask(ctx context.Context) (*Task, error) {
    // Postgres: SELECT ... FOR UPDATE SKIP LOCKED
    // SQLite: advisory lock or claimed_at column
    var task Task
    err := db.QueryRowContext(ctx, `
        UPDATE tasks
        SET status = 'in_progress'
        WHERE status = 'pending'
          AND id = (
            SELECT id FROM tasks
            WHERE status = 'pending'
            ORDER BY priority DESC, created_at ASC
            LIMIT 1
            FOR UPDATE SKIP LOCKED
          )
        RETURNING id, session_id
    `).Scan(&task.ID, &task.SessionID)

    if err == sql.ErrNoRows {
        return nil, nil
    }
    return &task, err
}
```

### Postgres vs SQLite Task Claiming

| Backend | Claiming Strategy |
|---|---|
| Postgres (any) | `SELECT ... FOR UPDATE SKIP LOCKED` — built-in, safe for concurrent workers |
| SQLite | Single writer means no concurrent claim contention — `UPDATE ... WHERE status = 'pending' LIMIT 1` is sufficient |

When running against Supabase Cloud, the same Go binary can be deployed as multiple instances for horizontal scaling. `FOR UPDATE SKIP LOCKED` ensures each task is claimed by exactly one worker.

## Token Caching

LLM API calls are the primary cost driver. Token caching minimizes redundant processing.

### Prefix Hierarchy

The message structure follows a strict prefix order:

```
[System Instructions]          ← static, always cached after first call
[Event Ledger]                 ← append-only, cached from previous iteration
[Dynamic Context]              ← changes every iteration (current task state, tool results)
```

For Anthropic's prompt caching, cache breakpoints are set on the system instructions and event ledger. Dynamic content is placed at the end so that cached prefixes remain valid.

## Model Cascade

Not every task needs GPT-4. The harness routes to the cheapest capable model:

> Model registry is defined in SPEC-003 §2.14 with canonical schema. The harness reads model_registry to route LLM calls.

### Routing logic

```go
func SelectModel(task *Task, registry []Model) *Model {
    requiredTier := getTierForTaskType(task.Instruction)
    if task.ModelPreference != "" {
        requiredTier = getTierForModel(task.ModelPreference)
    }

    var candidates []Model
    for _, m := range registry {
        if m.CapabilityTier >= requiredTier && m.SupportsStructuredOutputs {
            candidates = append(candidates, m)
        }
    }
    sort.Slice(candidates, func(i, j int) bool {
        return costPerToken(candidates[i]) < costPerToken(candidates[j])
    })

    return &candidates[0]
}
```

Rules:
- Sub-agents default to the cheapest model (tier 5) unless the parent specifies otherwise
- Tasks involving schema creation or complex SQL use higher-tier models
- Budget limits can force downgrades mid-task

## Secrets Injection & Scrubbing

### Injection

Before any SQL is executed, `{{SECRET.X}}` aliases are replaced with real values.

```go
func InjectSecrets(sql string, secrets map[string]string) string {
    for alias, value := range secrets {
        sql = strings.ReplaceAll(sql, fmt.Sprintf("{{SECRET.%s}}", alias), value)
    }
    return sql
}
```

### Scrubbing

After the LLM responds, any real secret values are stripped:

```go
func ScrubSecrets(response string, secrets map[string]string) string {
    for alias, value := range secrets {
        response = strings.ReplaceAll(response, value, fmt.Sprintf("[REDACTED:%s]", alias))
    }
    return response
}
```

Scrubbing uses exact-match replacement against known secret values. It runs on every response before storage or display.

## SQL Execution Model

### Three Execution Tiers

The harness supports three tiers of SQL execution, controlled by the agent's trust level. Lower-trust models are restricted to stored procedures only; higher-trust models gain access to broader SQL execution.

#### Tier 1: Stored Procedure Only

The safest mode. The agent can only call predefined stored procedures. All mutations go through validated function signatures.

```json
{
  "memory_state_changes": [
    "SELECT set_display_mode(104, 'compressed')",
    "SELECT complete_session('session-uuid')"
  ]
}
```

**When to use:** Small/cheap models (tier 4-5), untrusted environments, or when maximum safety is required.

#### Tier 2: Parameterized SQL

The agent writes SQL with `$1, $2...` parameter placeholders. The harness binds values separately, preventing SQL injection from external data.

```json
{
  "memory_state_changes": [
    {"sql": "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ($1, $2, $3, $4)", "params": ["text_block", "User requested dark mode", "session-uuid", 5]}
  ]
}
```

**When to use:** Medium-trust models, operations involving external/user data.

#### Tier 3: Raw SQL (with classifier + whitelist)

The agent writes raw SQL strings. The harness runs them through the statement classifier (SPEC-011 §8) and table whitelist before execution.

```json
{
  "memory_state_changes": [
    "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'User requested dark mode', 'session-uuid', 5)"
  ]
}
```

**When to use:** High-trust models (tier 1-2), operations on the agent's own schema where the LLM knows the table structures.

All three tiers are subject to the statement classifier, table whitelist, and RLS policies. The tiers control the *format* of the SQL, not the safety checks.

### Multi-Statement Splitting

LLM output may contain multiple SQL statements separated by semicolons in a single string. The harness splits on semicolons and classifies each statement independently:

```go
func SplitStatements(statements []string) []string {
    var result []string
    for _, stmt := range statements {
        parts := strings.Split(stmt, ";")
        for _, part := range parts {
            trimmed := strings.TrimSpace(part)
            if trimmed != "" {
                result = append(result, trimmed)
            }
        }
    }
    return result
}
```

### SQL Sanitization

```go
func Sanitize(sql string) string {
    sql = strings.ReplaceAll(sql, "\x00", "")
    sql = strings.TrimPrefix(sql, "\uFEFF")
    sql = strings.TrimSpace(sql)
    return sql
}
```

### Execution Policy Enforcement

```go
func EnforceExecutionPolicy(classified StatementClass, sql string, sessionID string, dynamicTables map[string]bool) error {
    if classified == DANGEROUS {
        return fmt.Errorf("blocked dangerous statement: %s", truncate(sql, 100))
    }
    if classified == DML_WRITE && !IsTableAllowed(sql, dynamicTables) {
        return fmt.Errorf("blocked write to unauthorized table: %s", truncate(sql, 100))
    }
    return nil
}
```

## Context Formatting

The harness formats the database context as Markdown for the LLM:

```go
func FormatContextAsMarkdown(ctx *ActiveContextView) string {
    return fmt.Sprintf(`
# Active Context — Session %s

## Current Task
%s

## Memory (Last %d events)
%s

## Available Tools
%s

## Compressed Memory Pages
%s

## Constraints
- Iteration: %d / %d
- Budget used: %d / %d cents
- Consecutive errors: %d / %d
`,
        ctx.SessionID,
        ctx.TaskInstruction,
        len(ctx.RecentMemories),
        formatMemories(ctx.RecentMemories),
        formatTools(ctx.Tools),
        formatPages(ctx.MemoryPages),
        ctx.Iteration, ctx.MaxIterations,
        ctx.BudgetUsedCents, ctx.BudgetLimitCents,
        ctx.ConsecutiveErrors, ctx.MaxConsecutiveErrors,
    )
}
```

## Error Recovery Flow

```
Iteration N:
  → SQL fails: column "foo" does not exist
  → ROLLBACK
  → Inject error into context

Iteration N+1:
  → Context now includes: "Previous error: column 'foo' does not exist"
  → Agent corrects SQL
  → COMMIT
  → Continue
```

If the agent cannot recover after `max_consecutive_errors` (3 by default):

```
Iteration N+3:
  → Still failing
  → Circuit breaker trips
  → Task status → FAILED
  → Alert sent to admin
```

## Complete Lifecycle

```
Heartbeat triggers
  │
  ▼
Claim next pending task (FOR UPDATE SKIP LOCKED)
  │
  ▼
Set task status → in_progress
  │
  ▼
┌─────────────── Iteration Loop ───────────────┐
│                                               │
│  Read context                                 │
│  Format as Markdown                           │
│  Call LLM (with token caching)                │
│  Parse JSON response                          │
│  Inject secrets into SQL                      │
│  Execute SQL in transaction                   │
│    ├─ Success → COMMIT, save snapshot         │
│    └─ Error → ROLLBACK, inject error          │
│  Scrub secrets from response                  │
│  Save audit log                               │
│  Check circuit breakers                       │
│    ├─ Task complete? → break loop             │
│    ├─ Error limit? → fail task, break loop    │
│    └─ Continue? → next iteration             │
│                                               │
└───────────────────────────────────────────────┘
  │
  ▼
Set task status → published or failed
  │
  ▼
If sub-agent completion, trigger wakes parent
```