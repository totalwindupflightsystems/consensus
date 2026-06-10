# SPEC-020: Interactive Transaction Staging

**Status:** Draft
**Depends On:** SPEC-008 (Harness), SPEC-006 (Transactions), SPEC-010 (Tools), SPEC-003 (Database)
**Amends:** SPEC-008 (Harness core loop), SPEC-006 (Transaction model)
**Created:** 2026-04-12

---

## 1. Overview

The current harness fires one LLM call per iteration and commits all SQL atomically. This works for simple tasks but fails for complex ones where the agent needs to work like a human engineer in an open `psql` session:

```
BEGIN;
  → UPDATE users SET status = 'active' WHERE id = 123;
  → -- ok, 1 row updated
  
  → SELECT * FROM users WHERE id = 123;
  → -- looks good, status is now 'active'
  
  → INSERT INTO audit_log (action, target) VALUES ('status_change', 'user:123');
  → -- inserted
  
  → -- hmm, I should also update the related orders
  → SELECT count(*) FROM orders WHERE user_id = 123 AND status = 'pending';
  → -- 5 pending orders, I should flag them
  
  → UPDATE orders SET flagged = true WHERE user_id = 123 AND status = 'pending';
  → -- 5 rows updated
  
  → SELECT count(*) FROM orders WHERE flagged = true AND user_id = 123;
  → -- confirmed 5
  
COMMIT;
```

A human doesn't write all that SQL blind and hope it works. They run commands one at a time, inspect results, and decide what to do next. **SPEC-020 gives the agent the same workflow.**

The agent opens a transaction, stages commands, executes them individually, sees the output, decides what to stage next, and commits when satisfied. Everything stays uncommitted until the agent explicitly commits — or rolls back if something looks wrong.

**Core principle:** The agent works like an engineer in an open transaction window. Stage, execute, inspect, decide, repeat, commit.

---

## 2. Model

```
┌─────────────────────────────────────────────────────────────┐
│                     ITERATION N                              │
│                                                             │
│  ┌──────────── INTERACTIVE TRANSACTION ──────────────┐      │
│  │                                                    │      │
│  │  BEGIN TRANSACTION                                 │      │
│  │                                                    │      │
│  │  Turn 1: Agent reads context                       │      │
│  │    → stage: SELECT * FROM auth_table               │      │
│  │    → execute → see results                         │      │
│  │    → "ok, I see 3 rows with broken tokens"         │      │
│  │                                                    │      │
│  │  Turn 2: Agent plans fixes                         │      │
│  │    → stage: UPDATE auth_table SET token=NULL ...   │      │
│  │    → execute → "3 rows updated"                    │      │
│  │                                                    │      │
│  │  Turn 3: Agent verifies                            │      │
│  │    → stage: SELECT count(*) FROM auth_table WHERE  │      │
│  │    → execute → "0 rows with broken tokens, good"   │      │
│  │                                                    │      │
│  │  Turn 4: Agent adds related fix                    │      │
│  │    → tool_call: read_file("src/auth.ts")           │      │
│  │    → see file contents                             │      │
│  │    → stage: file_edit("src/auth.ts", ...)          │      │
│  │    → execute → file updated                        │      │
│  │                                                    │      │
│  │  Turn 5: Agent commits                             │      │
│  │    → "everything looks correct"                    │      │
│  │    → COMMIT                                       │      │
│  │                                                    │      │
│  └────────────────────────────────────────────────────┘      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### How It Differs From One-Shot

| Aspect | One-Shot (Before) | Interactive Transaction (After) |
|---|---|---|
| LLM calls per iteration | 1 | N turns |
| Transaction lifecycle | Open → run all → close | Open → stage/execute per turn → close when ready |
| Agent sees results | Only after commit (next iteration) | Immediately after each staged command |
| Tool calls | After commit, separate phase | Inline during transaction |
| Rollback | Whole iteration fails | Agent can rollback and try again within same iteration |
| Feels like | Batch SQL script | Interactive psql session |

---

## 3. LLM Response Schema

Each turn the agent responds with structured JSON indicating what it wants to do:

```json
{
  "internal_monologue": "I see 3 rows with broken tokens. I'll null them out and verify.",
  "action": "stage_and_execute",
  "staged_commands": [
    {
      "type": "sql",
      "payload": "UPDATE auth_tokens SET token = NULL WHERE expires_at < now()",
      "description": "Clear expired tokens"
    }
  ],
  "tool_requests": [],
  "message_to_user": null
}
```

### Action Types

| Action | Meaning | What Happens |
|---|---|---|
| `stage_and_execute` | Run these commands now, give me results | Commands execute inside open transaction, results fed back |
| `stage_only` | Queue these but don't run yet | Commands go to staging buffer, agent continues planning |
| `tool_call` | I need to run an external tool | Tool executes outside transaction, result fed back |
| `commit` | Everything looks good, commit | Transaction commits, iteration ends |
| `rollback` | Something's wrong, start over | Transaction rolls back, agent can retry or give up |
| `respond` | I have something to say to the user | Ends planning, sends message, returns to idle |

### Response Fields

```json
{
  "internal_monologue": "string (required)",
  "action": "stage_and_execute | stage_only | tool_call | commit | rollback | respond",
  "staged_commands": [
    {
      "type": "sql | file_write | file_edit | file_delete | memory_write",
      "payload": "string or object",
      "description": "human-readable description of what this does"
    }
  ],
  "tool_requests": [
    { "tool_name": "...", "parameters": {} }
  ],
  "memory_state_changes": ["SQL strings applied at commit time"],
  "message_to_user": "string or null"
}
```

---

## 4. Staging Buffer

The staging buffer is the agent's transaction scratchpad. It holds every command the agent has staged — executed or pending — so the agent can see what's in its transaction.

```sql
CREATE TABLE staging_buffer (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration       BIGINT NOT NULL,
    turn            INT NOT NULL DEFAULT 1,
    sequence        INT NOT NULL DEFAULT 0,
    type            TEXT NOT NULL CHECK (type IN (
                        'sql', 'file_write', 'file_edit', 'file_delete',
                        'memory_write', 'tool_call_ref'
                    )),
    payload         JSONB NOT NULL DEFAULT '{}',
    description     TEXT NOT NULL DEFAULT '',
    executed        BOOLEAN NOT NULL DEFAULT false,
    result          JSONB,
    status          TEXT NOT NULL DEFAULT 'staged'
                    CHECK (status IN (
                        'staged', 'executed', 'committed',
                        'rolled_back', 'failed'
                    )),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_at     TIMESTAMPTZ
);

CREATE INDEX idx_staging_session ON staging_buffer(session_id, iteration);
```

### Fields

| Field | Purpose |
|---|---|
| `turn` | Which planning turn created this |
| `sequence` | Order within the turn |
| `type` | What kind of command |
| `payload` | The actual SQL, file content, or tool params |
| `description` | Human-readable explanation (the agent writes this) |
| `executed` | Whether this command has been run inside the transaction |
| `result` | Output from execution (row count, query results, error) |
| `status` | Lifecycle state |

---

## 5. Harness Core Loop (Updated)

```typescript
async function runAgentIteration(sessionId: string): Promise<IterationResult> {
    const maxTurns = getMaxPlanningTurns(sessionId); // default: 10
    let turn = 0;
    let action = 'stage_and_execute';
    let messageToUser = null;

    // Open the transaction — stays open across all turns
    const tx = await db.beginTransaction();
    await tx.execute(sql`SET LOCAL conscience.session_id = ${sessionId}`);

    // Set session status
    await db.execute(sql`UPDATE sessions SET status = 'planning' WHERE id = ${sessionId}`);

    try {
        while (turn < maxTurns) {
            turn++;

            // ── 1. Read context ──
            const context = await readActiveContext(sessionId);
            const stagedSoFar = await readStagingBuffer(sessionId);

            // ── 2. Format context including transaction state ──
            const messages = buildTurnMessages(context, stagedSoFar, turn);

            // ── 3. Call LLM ──
            const rawResponse = await callLLM(messages, { response_format: jsonSchema });
            const parsed = JSON.parse(scrubSecrets(rawResponse));

            action = parsed.action;
            messageToUser = parsed.message_to_user ?? null;

            // ── 4. Handle each action type ──

            if (action === 'stage_and_execute' || action === 'stage_only') {
                // Write staged commands to buffer
                for (let i = 0; i < (parsed.staged_commands?.length ?? 0); i++) {
                    const cmd = parsed.staged_commands[i];
                    const entry = await writeStagedCommand(sessionId, turn, i, cmd);

                    if (action === 'stage_and_execute') {
                        // Execute inside the open transaction
                        const result = await executeStagedCommand(tx, entry);
                        // Write result back to buffer so agent sees it next turn
                        await updateStagedResult(entry.id, result);
                    }
                }
                continue; // next turn
            }

            if (action === 'tool_call') {
                // Execute tools outside the transaction (they're external)
                // but feed results back into context for the next turn
                for (const toolReq of parsed.tool_requests ?? []) {
                    const result = await executeToolSandboxed(toolReq);
                    await writeToolResultToContext(sessionId, result);
                }
                continue; // next turn
            }

            if (action === 'commit') {
                // Apply any final memory_state_changes
                for (const sqlStmt of splitStatements(parsed.memory_state_changes ?? [])) {
                    const classified = classifyStatement(sqlStmt);
                    enforceExecutionPolicy(classified, sqlStmt, sessionId, dynamicTables);
                    await tx.execute(sanitize(sqlStmt));
                }

                // Mark all staged commands as committed
                await tx.execute(sql`
                    UPDATE staging_buffer SET status = 'committed'
                    WHERE session_id = ${sessionId} AND status IN ('staged', 'executed')
                `);

                // Update session
                await tx.execute(sql`
                    UPDATE sessions SET
                        status = 'idle',
                        iteration = iteration + 1,
                        heartbeat_at = now()
                    WHERE id = ${sessionId}
                `);

                await tx.commit();
                return { status: 'success', messageToUser, turnsUsed: turn };
            }

            if (action === 'rollback') {
                await tx.rollback();

                // Mark all staged commands as rolled back
                await db.execute(sql`
                    UPDATE staging_buffer SET status = 'rolled_back'
                    WHERE session_id = ${sessionId} AND status IN ('staged', 'executed')
                `);

                // Agent can continue planning with a fresh transaction
                // or end the iteration
                if (parsed.end_iteration) {
                    return { status: 'rolled_back', messageToUser, turnsUsed: turn };
                }

                // Re-open transaction for retry
                tx = await db.beginTransaction();
                await tx.execute(sql`SET LOCAL conscience.session_id = ${sessionId}`);
                continue;
            }

            if (action === 'respond') {
                // No staged changes, just reply to user
                await tx.rollback();
                return { status: 'success', messageToUser, turnsUsed: turn };
            }
        }

        // Max turns reached — auto-commit if we have staged work
        const hasExecuted = await hasExecutedCommands(sessionId);
        if (hasExecuted) {
            await tx.execute(sql`
                UPDATE staging_buffer SET status = 'committed'
                WHERE session_id = ${sessionId} AND status IN ('staged', 'executed')
            `);
            await tx.execute(sql`
                UPDATE sessions SET status = 'idle', iteration = iteration + 1
                WHERE id = ${sessionId}
            `);
            await tx.commit();
            return { status: 'success', turnsUsed: turn, autoCommitted: true };
        }

        await tx.rollback();
        return { status: 'error', error: 'Agent exceeded planning turns without producing output' };

    } catch (error) {
        await tx.rollback();
        await db.execute(sql`
            UPDATE staging_buffer SET status = 'failed'
            WHERE session_id = ${sessionId} AND status IN ('staged', 'executed')
        `);
        await logError(sessionId, error);
        return { status: 'error', error: formatPostgresError(error) };
    }
}
```

---

## 6. Context Formatting — Transaction State

The agent sees its full transaction state each turn — what it has staged, what executed, and what the results were:

```typescript
function buildTurnMessages(
    context: ActiveContextView,
    stagedSoFar: StagedCommand[],
    turn: number
): Message[] {
    // Build transaction state section
    const transactionState = stagedSoFar.length > 0
        ? stagedSoFar.map((cmd, i) => {
            const icon = cmd.status === 'executed' ? '✓' : cmd.status === 'failed' ? '✗' : '…';
            const result = cmd.result
                ? `\n    Result: ${JSON.stringify(cmd.result).substring(0, 200)}`
                : '';
            return `${icon} [Turn ${cmd.turn}] ${cmd.description}${result}`;
          }).join('\n')
        : 'Empty — no commands staged yet.';

    return [
        { role: 'system', content: systemInstructions },
        { role: 'user',   content: formatContextAsMarkdown(context) },
        {
            role: 'user',
            content: `
# Transaction Window — Turn ${turn}

You have an open transaction. You can stage and execute commands, see results, and decide what to do next.

## Transaction Contents
${transactionState}

## Available Actions
- **stage_and_execute**: Stage commands and execute them immediately. You'll see results next turn.
- **stage_only**: Stage commands without executing (queue for later).
- **tool_call**: Run an external tool (file read, web fetch, etc.). Results appear next turn.
- **commit**: Commit the transaction. All staged+executed commands become permanent.
- **rollback**: Roll back the transaction. Everything undone. You can retry or stop.
- **respond**: Reply to the user without committing any changes.

What do you want to do?
            `.trim()
        }
    ];
}
```

### Example: What the Agent Sees

```
# Transaction Window — Turn 3

## Transaction Contents
✓ [Turn 1] SELECT count of expired auth tokens
    Result: {"count": 3}
✓ [Turn 2] Clear expired tokens (UPDATE auth_tokens SET token = NULL WHERE expires_at < now())
    Result: {"rows_affected": 3}
… [Turn 3] Verify no expired tokens remain (SELECT count(*) FROM auth_tokens WHERE token IS NOT NULL AND expires_at < now())
    Result: pending execution

## Available Actions
...
```

The agent can see: I checked the count (3), I cleared them (3 rows), I staged a verification query. Next turn I'll see the verification result and decide whether to commit.

---

## 7. Executing Staged Commands

```typescript
async function executeStagedCommand(tx: Transaction, entry: StagedCommand): Promise<any> {
    switch (entry.type) {
        case 'sql': {
            const classified = classifyStatement(entry.payload);
            enforceExecutionPolicy(classified, entry.payload, entry.session_id, dynamicTables);
            const result = await tx.execute(sanitize(entry.payload));
            // For SELECTs: return rows. For DML: return row count.
            return {
                type: 'sql',
                rows_affected: result.count ?? 0,
                rows: result.rows?.slice(0, 50), // cap at 50 rows for context
                columns: result.columns,
            };
        }

        case 'file_edit': {
            const current = await readFileFromDB(tx, entry.session_id, entry.payload.path);
            const updated = applySearchReplace(current, entry.payload);
            await writeFileToDB(tx, entry.session_id, entry.payload.path, updated);
            return { type: 'file_edit', path: entry.payload.path, status: 'updated' };
        }

        case 'file_write': {
            await writeFileToDB(tx, entry.session_id, entry.payload.path, entry.payload.content);
            return { type: 'file_write', path: entry.payload.path, status: 'written' };
        }

        case 'memory_write': {
            // Memory writes are deferred to commit time by default
            // but the agent can execute them to verify structure
            return { type: 'memory_write', status: 'will_apply_at_commit' };
        }
    }
}
```

---

## 8. Rollback and Retry

The agent can rollback its transaction and start fresh within the same iteration:

```
Turn 1: stage_and_execute → UPDATE users SET role = 'admin' WHERE id = 123
Turn 2: see result → "1 row updated"
Turn 3: stage_and_execute → SELECT * FROM users WHERE id = 123
Turn 4: see result → "wait, role should be 'editor' not 'admin', that's wrong"
Turn 5: rollback → transaction rolls back, UPDATE is undone
Turn 6: stage_and_execute → UPDATE users SET role = 'editor' WHERE id = 123
Turn 7: see result → "1 row updated"
Turn 8: stage_and_execute → SELECT * FROM users WHERE id = 123
Turn 9: see result → "role is 'editor', correct"
Turn 10: commit → done
```

The agent can also rollback and give up:

```
Turn 1-5: trying various approaches, all failing with constraint violations
Turn 6: rollback + end_iteration → "I can't resolve the constraint issues. Asking user for guidance."
```

---

## 9. Session Status Updates

### 9.1 Reconciliation with SPEC-011 §1

SPEC-011 §1 defines the canonical session status state machine. This section amends it with two new statuses for the interactive transaction model. The updated canonical states (SPEC-011 §1.1) are:

| Status | When | Added By |
|---|---|---|
| `planning` | Agent is in interactive transaction — staging, executing, inspecting | SPEC-020 (this spec) |
| `executing` | Agent called commit — applying final changes | SPEC-020 (this spec) |

### 9.2 Tool Execution Reconciliation with SPEC-011 §7

SPEC-011 §7 defines a two-phase execution model where tool execution happens **outside** the main transaction. In the interactive transaction model, this principle is preserved:

- **`tool_call_ref`** entries in the staging buffer are references to tool calls, NOT inline execution within the transaction.
- When the agent issues `action: tool_call`, the harness **suspends the open transaction** (session status → `tool_exec`), executes the tool outside the transaction, writes the result to `tool_results`, then **resumes the transaction** (session status → `planning`) and feeds the result back.
- The staging buffer entry with `type = 'tool_call_ref'` records that a tool was called during planning, but the actual execution and result storage happen outside the transaction boundary, consistent with SPEC-011 §7.2.
- If the process crashes during tool execution, the transaction is lost (uncommitted), and the `tool_exec` status triggers pg_cron reaping — same crash safety as the one-shot model.

```sql
status TEXT NOT NULL DEFAULT 'booting'
    CHECK (status IN (
        'booting', 'idle', 'planning', 'thinking',
        'tool_exec', 'executing', 'waiting_sub',
        'completed', 'failed', 'paused'
    ))
```

| Status | When |
|---|---|
| `planning` | Agent is in interactive transaction — staging, executing, inspecting |
| `tool_exec` | Agent is running an external tool during planning (transaction suspended) |
| `executing` | Agent called commit — applying final changes |

---

## 10. Backward Compatibility

One-shot still works. If the agent responds on turn 1 with:

```json
{
  "action": "stage_and_execute",
  "staged_commands": [
    { "type": "sql", "payload": "INSERT INTO memory_events ...", "description": "..." }
  ],
  "message_to_user": "Done."
}
```

Then turn 2:
```json
{ "action": "commit" }
```

Two turns, same result as the old one-shot model. Minimal overhead. The agent only uses extra turns when it needs them.

For even simpler cases, the agent can commit in a single turn:

```json
{
  "action": "commit",
  "staged_commands": [...],
  "memory_state_changes": [...]
}
```

This stages and commits in one go — functionally identical to the old model.

---

## 11. Configuration

```yaml
harness:
  planning:
    max_turns: 10               # Max turns per interactive transaction
    auto_commit_on_max: true    # Auto-commit if max_turns reached with staged work
    max_staged_commands: 50     # Max commands in staging buffer per iteration
    result_row_limit: 50        # Max rows returned from SELECT results
```

Per-session override:

```sql
ALTER TABLE sessions ADD COLUMN planning_max_turns INT NOT NULL DEFAULT 10;
```

---

## 12. Why This Matters for Model Quality

Different models have different strengths:

| Model Type | One-Shot | Interactive Transaction |
|---|---|---|
| GPT-4o / Claude Opus | Good at one-shot complex planning | Excellent — can verify and self-correct |
| GPT-4o-mini / Claude Haiku | Struggles with complex one-shot | Much better — can iterate and build up |
| Open-source (Llama, Mistral) | Often fails on complex SQL | Can work — simple steps, verify each one |

The interactive transaction model lets cheaper models punch above their weight. A model that can't write a complex 5-statement transaction in one shot can absolutely write 5 simple statements one at a time, checking results between each.

This directly enables the model cascade (SPEC-008 §Model Cascade) to use cheaper models for more tasks — they don't need to be perfect in one shot, they just need to be correct per-turn.

---

## 13. Amended Specs

### SPEC-008 (Harness)

The `runAgentIteration` function is replaced by the interactive transaction model in §5. The original Phase 1/Phase 2 model becomes the single-turn fast path.

### SPEC-006 (Transactions)

Transaction guarantees unchanged. New behavior: transactions stay open across multiple turns within an iteration, instead of open-close per iteration. Timeout and crash recovery added (§14).

### SPEC-003 (sessions table)

`status` CHECK constraint gains `'planning'` and `'executing'`. Status index updated to include new statuses. `staging_buffer` added to reserved table names.

### SPEC-011 (Canonical Definitions)

§1 session status state machine gains `'planning'` and `'executing'` with valid transitions. §7 tool execution model preserved — `tool_call_ref` entries in staging buffer are references only, actual execution happens outside transaction boundary (§9.2).

---

## 14. Transaction Timeout & Crash Recovery

### 14.1 Transaction Timeout

Interactive transactions must not stay open indefinitely. A configurable timeout prevents long-running transactions from holding locks:

```yaml
harness:
  planning:
    transaction_timeout_ms: 60000    # Default: 60 seconds
```

If the timeout is exceeded, the harness rolls back the transaction and marks the session as `failed`:

```typescript
const timeoutHandle = setTimeout(async () => {
    if (tx.isActive) {
        await tx.rollback();
        await db.execute(sql`
            UPDATE staging_buffer SET status = 'failed'
            WHERE session_id = ${sessionId} AND status IN ('staged', 'executed')
        `);
        await db.execute(sql`
            UPDATE sessions SET status = 'failed' WHERE id = ${sessionId}
        `);
    }
}, transactionTimeoutMs);
```

### 14.2 Crash Recovery

If the process crashes while an interactive transaction is open:

1. The uncommitted transaction is automatically rolled back by PostgreSQL/SQLite (MVCC/WAL guarantees)
2. The session remains in `planning` or `tool_exec` status
3. `pg_cron` reaps stale sessions (SPEC-003 §7.1) — sessions with `heartbeat_at` older than 5 minutes in `planning`/`tool_exec` are marked `failed`
4. The staging buffer entries with status `staged` or `executed` are orphaned — a cleanup job marks them as `failed`

```sql
-- Add planning/tool_exec to stale session reaping
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

-- Cleanup orphaned staging buffer entries
SELECT cron.schedule(
    'cleanup-orphaned-staging',
    '*/5 * * * *',
    $$
    UPDATE staging_buffer
    SET status = 'failed'
    WHERE status IN ('staged', 'executed')
      AND session_id IN (
        SELECT id FROM sessions WHERE status = 'failed'
      );
    $$
);
```

### 14.3 Infinite Rollback-Retry Prevention

The agent can rollback and retry within a single iteration, but this is capped:

```typescript
const maxRollbackRetries = 3;
let rollbackCount = 0;

if (action === 'rollback') {
    rollbackCount++;
    if (rollbackCount >= maxRollbackRetries && !parsed.end_iteration) {
        // Force end the iteration after too many rollbacks
        return { status: 'error', error: 'Max rollback retries exceeded' };
    }
    // ... re-open transaction
}
```

---

## 15. Open Questions

1. **Staging buffer cleanup**: When do old staging_buffer entries get cleaned up? (Resolved: pg_cron cleanup job marks orphaned entries as `failed`, §14.2)
3. **Cross-iteration persistence**: Staged commands do NOT survive across iterations. The buffer is scoped to a single iteration. On commit or rollback, all entries are finalized. (Resolved: `staging_buffer` rows are per-iteration lifecycle.)
4. **Concurrent access**: Yes, MVCC handles this on Postgres — readers see the pre-transaction snapshot. SQLite WAL mode allows concurrent reads during writes. (Resolved: No additional mechanism needed.)
5. **Partial commit**: Should the agent be able to commit a subset of staged commands? Or is it always all-or-nothing?
6. **Turn-level billing**: Each LLM call is a separate billing event? Or one event per iteration?
