---
name: pattern-delegate-and-continue
description: >-
  Fire off a background task to another agent, continue your own work
  immediately, and check the result later when you need it. Uses
  conductor_spawn + conductor_status/result. The simplest async pattern.
  Runtime-tested 2026-05-18.
version: "1.0"
tags:
  vertical: [delegation, async, workflow]
  category: pattern
  core: false
spec: specs/121-Pattern-Generator.md
trigger_conditions:
  - "You have work that can run in the background while you do something else"
  - "You want to delegate a self-contained task and not wait for it"
  - "You're doing multi-step work and one step is independent"
  - "You want to 'fire and forget' a task but still get the result eventually"
tools_required:
  - conductor_spawn
  - conductor_status (check progress)
  - conductor_result (fetch result when needed)
estimated_steps: 3
estimated_duration: "instant spawn + background execution + instant result fetch"
lifecycle:
  state: active
  created: "2026-05-18"
  last_validated: "2026-05-18"
  validation_count: 1
---

# Pattern: Delegate and Continue

Spawn a single background agent for a self-contained task. Don't wait — continue
your own work. Check the result later when you need it. The async version of
asking a coworker to handle something while you keep going.

**Spec**: `specs/121-Pattern-Generator.md`
**Observed from**: real executions on 2026-05-18 (math-test agent: spawned → continued → checked → got "56")

<!-- axiom:trace work_item=pattern-design-01 spec=specs/121-Pattern-Generator.md -->

---

## Prerequisites

| Requirement | How to Verify | Expected | If Missing |
|-------------|---------------|----------|------------|
| Conductor plugin loaded | Call `conductor_status` | returns dashboard | Plugin not loaded |

---

## Tool Chain

| Step | Purpose | Tool | Key Input | Key Output | On Failure | Criticality |
|------|---------|------|-----------|------------|------------|-------------|
| 1 | Delegate | `conductor_spawn` | `{name, task, timeout}` | `{agent_id, status: "running"}` | Check session_id is `ses_*` | Required |
| 2 | Continue | (your own tools) | — | your work output | — | Your work |
| 3 | Check result | `conductor_result` | `{id: agent_id}` | `{status, result_summary}` | Wait or re-check later | When needed |

---

## Pseudocode

```text
PATTERN delegate_and_continue(task_name, task_prompt, my_own_work_fn):

  // Step 1: Delegate (instant — returns immediately)
  agent = CALL conductor_spawn(
    name: task_name,
    task: task_prompt + "\nCall conductor_done with your result as the summary. " +
          "Your spawn_secret is in the conductor_envelope.",
    timeout: "5m"
  )
  agent_id = agent.agent_id
  // Don't wait — the agent is working in the background now.

  // Step 2: Do your own work (unblocked)
  my_result = my_own_work_fn()

  // Step 3: Check the delegated result (when you need it)
  result = CALL conductor_result(id: agent_id)
  IF result.status == "done":
    // Great — use result.result_summary
    RETURN { my_result, delegated_result: result.result_summary }
  ELSE IF result.status == "running":
    // Still working — either wait or come back later
    // CALL conductor_wait(id: agent_id, timeout: "2m") if blocking is OK
    RETURN { my_result, delegated_status: "still_running" }
  ELSE:
    RETURN { my_result, delegated_status: result.status, error: result.error }
```

---

## On-Track / Off-Track Signals

| Signal | Type | After Step | Indicator | Response |
|--------|------|-----------|-----------|----------|
| SIG-01 | on_track | 1 | `conductor_spawn` returns `ses_*` session_id | Agent is running |
| SIG-02 | off_track | 1 | Returns `cnd_*` synthetic ID | SDK client issue |
| SIG-03 | on_track | 3 | `conductor_result` returns `status: "done"` | Use result_summary |
| SIG-04 | on_track | 3 | `conductor_result` returns `status: "running"` | Agent still working — check later |
| SIG-05 | off_track | 3 | `conductor_result` returns `status: "cancelled"` | Agent timed out — task too big or prompt unclear |

---

## Key Design Decision: When to check the result

```
Check IMMEDIATELY after your work:
  → When you need the delegated result to proceed
  → Use conductor_result, then conductor_wait if still running

Check LATER (at end of session):
  → When the delegated work is nice-to-have, not blocking
  → Check in conductor_status dashboard before closing

Don't check at all (fire and forget):
  → When the delegated task is self-sufficient (e.g., "update this doc")
  → The agent calls conductor_done and the result sits in the DB
  → Another session can read it later via conductor_result
```

---

## Example Execution Trace

```
─── Pattern Instance: delegate-and-continue ───

[Step 1] conductor_spawn
→ conductor_spawn({ name: "math-test", task: "What is 7×8? Reply with just the number..." })
← { agent_id: "bg_b6ujeyy9", session_id: "ses_1c277...", status: "running" }
✓ SIG-01: real session, agent is running

[Step 2] Continue own work (stash operations, tree commits, whatever)
  ... 45 seconds pass while you do other things ...

[Step 3] conductor_result (or just check status)
→ conductor_status()
← "bg_b6ujeyy9 (math-test): DONE — 56 [cost: $0.000]"
✓ SIG-03: done with result "56"

RESULT: PATTERN_COMPLETE
  delegated_result: "56"
  my_own_work: completed in parallel
```

---

## Comparison with Other Patterns

| Pattern | When to use |
|---------|------------|
| **delegate-and-continue** | ONE background task, check result later |
| **coordinate-swarm** | N parallel tasks, collect ALL results at once |
| **handoff-context** | Pass context to a future agent (sequential, not parallel) |
| **graph harness** | Multi-step DAG with dependencies |

---

## When NOT to Use This Pattern

- **You need the result before doing anything else**: just call the tool yourself
- **The task has dependencies on your current work**: use graph harness instead
- **The delegated task needs interactive back-and-forth**: conductor agents are one-shot
