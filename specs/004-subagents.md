# SPEC-004: Subagent Spawning, Process Control & Communication

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence. Session status values use lowercase canonical states (idle, waiting_sub, thinking, etc.) not ALL-CAPS.

## Overview

Agents in the Consensus framework can spawn sub-agents to parallelize work, delegate tasks, and scope memory. Sub-agents are first-class citizens of the task system with enforced isolation and controlled communication channels.

## Spawning

An agent spawns a sub-agent by inserting a row into the `tasks` table. The parent-child relationship is tracked via `sessions.parent_id` (SPEC-003 §2.1):

```sql
INSERT INTO tasks (session_id, title, description, status)
VALUES ('parent-session-uuid', 'Research all REST endpoints...', '...', 'pending');
```

The harness picks up the new task, assigns it a fresh `session_id`, and begins execution. The sub-agent starts with a clean slate — no inherited runtime state, no open transactions.

## Memory Forking

Sub-agents receive compressed memory pointers from their parent, not full context. This is implemented as an instant context clone via SQL:

```sql
INSERT INTO memory_events (type, content, summary_text, session_id, iteration_created)
SELECT 'inherited_pointer', me.content, me.summary_text, 'child-session-uuid', me.iteration_created
FROM memory_events me
JOIN display_modes dm ON dm.memory_id = me.id AND dm.mode = 'compressed'
WHERE me.session_id = 'parent-session-uuid';
```

Key properties of memory forking:

- Only memory events with `display_modes.mode = 'compressed'` are inherited, not raw event data
- The child gets references it can decompress on demand, avoiding a full context dump
- Forking is a single SQL statement — atomic and instant
- The child's memory is fully isolated after forking; parent writes do not propagate

## Event-Driven Wakeups

Parent agents do not poll for sub-agent completion. Instead, a Postgres trigger wakes the parent automatically:

```sql
CREATE OR REPLACE FUNCTION wake_parent_on_completion()
RETURNS TRIGGER AS $$
BEGIN
  IF NEW.status = 'completed' AND NEW.parent_id IS NOT NULL THEN
    UPDATE sessions
    SET status = 'idle'
    WHERE id = NEW.parent_id
      AND status = 'waiting_sub';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER session_completion_wake_parent
  AFTER UPDATE ON sessions
  FOR EACH ROW
  EXECUTE FUNCTION wake_parent_on_completion();
```

Flow:
1. Parent spawns sub-agent and sets its own status to `waiting_sub`
2. Sub-agent executes its task
3. Sub-agent completes → trigger fires → parent status set to `idle`
4. Harness picks parent back up on next heartbeat

## Pause and Resume

Parents explicitly control their own suspension:

```sql
-- Parent pauses itself after delegating work
UPDATE tasks SET status = 'waiting_sub' WHERE session_id = 'parent-session-uuid';
```

The parent resumes only when a sub-agent in its lineage completes work or an external event triggers a status change back to `idle`. There is no polling loop — all wakeups are event-driven through Postgres triggers.

## RBAC Scope Model

Agents operate at three scope levels with decreasing privilege:

| Scope | Inheritance | Persistence | Example |
|---|---|---|---|
| Global Agent | System-wide | Permanent | System orchestrator, billing agent |
| Project Agent | Single Atlas project | Permanent | Code reviewer, documentation maintainer |
| Sub-Agent | Ephemeral | Task duration only | Research task, file search, test runner |

Scope enforcement:

- Global agents can read across projects but cannot write to project-scoped resources without escalation
- Project agents are confined to their Atlas project boundary
- Sub-agents inherit their parent's project scope but have no cross-project visibility

## Row-Level Security Isolation

Sub-agents cannot access other agents' memories or tools, enforced at the database level:

```sql
CREATE POLICY isolate_memories ON memory_events
  FOR SELECT
  USING (
    session_id = current_setting('consensus.session_id')::UUID
  );
```

```sql
CREATE POLICY isolate_tasks ON tasks
  FOR SELECT
  USING (
    session_id = current_setting('consensus.session_id')::UUID
  );
```

Additional isolation rules:

- Sub-agents cannot modify other agents' tools or memories
- A sub-agent can only see its own `tasks` row and its parent's task row (for result injection)
- Memory writes are scoped to the agent's own `session_id` — no cross-session writes
- Tool ownership is enforced: agents cannot update tool definitions they do not own

## Parent-Child Communication

Communication flows through structured channels:

| Channel | Direction | Mechanism |
|---|---|---|
| Task instruction | Parent → Child | `instruction` column in `tasks` row |
| Task result | Child → Parent | `result` column updated on completion, trigger wakes parent |
| Memory inheritance | Parent → Child | Forked compressed pointers at spawn time |
| Status signals | Child → Parent | Status change trigger (completed → wake parent) |

There is no shared memory space after forking. If a parent needs to send additional data to a running sub-agent, it must write to the `agent_messages` table (defined in SPEC-011 §12.1) scoped to the child's session:

```sql
INSERT INTO agent_messages (target_session_id, sender_session_id, payload)
VALUES ('child-session-uuid', 'parent-session-uuid', '{"type": "context_update", "body": "..."}');
```

The child's harness reads this table at each iteration and injects messages into the context.

## Error Propagation

If a sub-agent fails:
1. Its task status is set to `failed`
2. The `wake_parent_on_completion` trigger fires regardless of success/failure
3. Parent receives the error message via the task's `result` column
4. Parent decides: retry with modified instruction, spawn replacement, or escalate

## Lifecycle Summary

```
parent (idle)
  │
  ├─ INSERT INTO tasks (session_id, title, description, status)  ← spawn
  ├─ UPDATE tasks SET status = 'waiting_sub' WHERE self           ← pause
  │
  └─ [sub-agent executes]
       │
       ├─ Memory fork at spawn
       ├─ Isolated execution under RLS
       ├─ On completion: trigger wakes parent
       └─ Parent resumes (idle) with sub-agent result
```

## Sub-agent Depth Limit

Maximum sub-agent nesting depth is 5 (default, configurable via `system_settings`). The `spawn_subagent()` stored procedure checks current depth before allowing creation. This prevents circular or unbounded spawning.