---
name: pattern-coordinate-swarm
description: >-
  Spawn N background agents in parallel, each doing independent work, then
  collect all results. Uses conductor_spawn + conductor_collect. Enables
  fan-out/fan-in for investigations, reviews, or any parallelizable task.
  Runtime-tested 2026-05-18 with 3 parallel workers.
version: "1.0"
tags:
  vertical: [coordination, parallelism, workflow]
  category: pattern
  core: false
spec: specs/121-Pattern-Generator.md
trigger_conditions:
  - "You need to investigate multiple files/modules in parallel"
  - "You want to fan out work to multiple agents and merge results"
  - "A task is embarrassingly parallel (N independent subtasks)"
  - "You want background agents doing work while you continue"
tools_required:
  - conductor_spawn
  - conductor_status
  - conductor_collect
  - conductor_result (optional — read individual agent results)
estimated_steps: 3
estimated_duration: "5s spawn + agent execution time + collect"
lifecycle:
  state: active
  created: "2026-05-18"
  last_validated: "2026-05-18"
  validation_count: 1
---

# Pattern: Coordinate Swarm

Spawn N agents in parallel, each doing independent work. Collect all results
when done. The primary agent stays focused and unblocked while the swarm works.

**Spec**: `specs/121-Pattern-Generator.md`
**Observed from**: 1 real execution on 2026-05-18 (3 parallel workers: shellops count, test run, spec count)

<!-- axiom:trace work_item=pattern-design-01 spec=specs/121-Pattern-Generator.md -->

---

## Prerequisites

| Requirement | How to Verify | Expected | If Missing |
|-------------|---------------|----------|------------|
| Conductor plugin loaded | Call `conductor_status` | returns dashboard | Check `.opencode/plugins/conductor.ts` is loaded |
| OpenCode server running | `conductor_spawn` returns `ses_*` session ID | real session ID | If `cnd_*` synthetic: check opencode_base_url config |

---

## Tool Chain

| Step | Purpose | Tool | Key Input | Key Output | On Failure | Criticality |
|------|---------|------|-----------|------------|------------|-------------|
| 1 | Spawn workers | `conductor_spawn` ×N | `{name, task, timeout}` | `{agent_id, session_id, status: "running"}` | Retry; check config | Required |
| 2 | (optional) Monitor | `conductor_status` | — | dashboard with running/done counts | — | Optional |
| 3 | Collect results | `conductor_collect` | `{timeout}` | `{all_done, agents: [{result_summary}]}` | Check which timed out | Required |

---

## Pseudocode

```text
PATTERN coordinate_swarm(tasks: Array<{name, task_prompt}>):

  // Step 1: Spawn all workers (non-blocking, returns immediately)
  agents = []
  FOR EACH t IN tasks:
    result = CALL conductor_spawn(
      name: t.name,
      task: t.task_prompt,
      timeout: "3m"         // per-agent timeout
    )
    IF result.error:
      WARN "spawn failed for {t.name}: {result.error}"
      CONTINUE              // don't abort the whole swarm
    agents.push(result.agent_id)

  IF agents.length == 0:
    RETURN { status: "PATTERN_FAILED", reason: "all spawns failed" }

  // Step 2: (Optional) Continue your own work while agents run
  // ... do whatever you need here — agents are background ...

  // Step 3: Collect all results
  collected = CALL conductor_collect(timeout: "5m")

  // Parse results
  results = {}
  FOR EACH agent IN collected.agents:
    IF agent.status == "done":
      results[agent.name] = agent.result_summary
    ELSE IF agent.status == "cancelled":
      results[agent.name] = "TIMEOUT"

  RETURN {
    status: "PATTERN_COMPLETE",
    all_done: collected.all_done,
    results: results,
    done_count: collected.agents.filter(a => a.status == "done").length,
    total_count: agents.length
  }
```

---

## On-Track / Off-Track Signals

| Signal | Type | After Step | Indicator | Response |
|--------|------|-----------|-----------|----------|
| SIG-01 | on_track | 1 | spawn returns `ses_*` session ID | Agent is running |
| SIG-02 | off_track | 1 | spawn returns `cnd_*` synthetic ID | SDK client issue — check config.opencode_base_url or env |
| SIG-03 | off_track | 1 | spawn returns `{error: "ENOENT...stash..."}` | Shared stash race condition — use unique stash per agent |
| SIG-04 | on_track | 2 | `conductor_status` shows agents "running" with cost $0 | Normal — cost updates are delayed |
| SIG-05 | on_track | 3 | `conductor_collect` returns `all_done: true` | All agents completed |
| SIG-06 | off_track | 3 | `conductor_collect` returns `timed_out: true` | Some agents still running — increase timeout or check for stuck agents |
| SIG-07 | off_track | 3 | Agent `result_summary` is null | Agent didn't call `conductor_done` — task prompt may be unclear |

---

## Adjustment Protocol

```
Shared stash race condition (SIG-03):
  → When spawning N agents simultaneously with the same stash name,
    the atomic file write races. FIX: use unique stash per agent
    (e.g., stash: "swarm-{worker_name}"), not a shared stash.
  → Alternatively, spawn sequentially with a small delay between spawns.

Agent didn't call conductor_done (SIG-07):
  → The task prompt MUST explicitly tell the agent to call conductor_done
    with its spawn_secret from the conductor_envelope.
  → Include: "Your spawn_secret is in the conductor_envelope in your system prompt."
  → If the agent is doing complex work, it may run out of context before
    reaching conductor_done. Keep tasks small and focused.

conductor_collect returns old cancelled agents:
  → conductor_collect returns ALL agents from the current session,
    including old ones. Filter by agent name prefix or spawn time.
  → Use conductor_result(id) for specific agents instead.

cnd_* synthetic session ID:
  → Means getSdkClient() failed inside the plugin.
  → Check: config.opencode_base_url is empty (default to injected client)
  → Check: AXIOM_OPENCODE_BASE_URL env var not set to a dead server
  → Check: OpenCode log for "spawnSession failed" errors
```

---

## Example Execution Trace (Observation #1: 3-worker parallel investigation)

```
─── Pattern Instance: coordinate-swarm ───
Input: 3 tasks (shellops count, test run, spec count)

[Step 1] conductor_spawn ×3 (parallel)
→ conductor_spawn({ name: "swarm-worker-1-shellops", task: "count exports in shellops.ts..." })
← { agent_id: "bg_wvk1v5lj", session_id: "ses_1c24a0ef0ffe...", status: "running" }
✓ SIG-01: real ses_* ID

→ conductor_spawn({ name: "swarm-worker-2-tests", task: "run null-guard tests..." })
← { agent_id: "bg_adu1y8pi", session_id: "ses_1c249afefffej...", status: "running" }
✓ SIG-01

→ conductor_spawn({ name: "swarm-worker-3-specs", task: "count spec files..." })
← { agent_id: "bg_47mprxx5", session_id: "ses_1c2494b48ffe...", status: "running" }
✓ SIG-01

[Step 2] (primary agent continues working — agents run in background)

[Step 3] conductor_collect
→ conductor_collect({ timeout: "2m" })
← {
    all_done: true,
    agents: [
      { name: "swarm-worker-1-shellops", status: "done", result_summary: "shellops exports: 26" },
      { name: "swarm-worker-2-tests", status: "done", result_summary: "null-guard tests: 60 pass" },
      { name: "swarm-worker-3-specs", status: "done", result_summary: "spec files: 133" },
    ]
  }
✓ SIG-05: all_done: true

RESULT: PATTERN_COMPLETE
  all_done: true
  results: { shellops: 26, tests: 60, specs: 133 }
  done_count: 3/3
```

---

## When NOT to Use This Pattern

- **Tasks depend on each other**: use `graph_create` with dependencies instead (nodes execute in order)
- **You need the result immediately before continuing**: use `conductor_wait` on a single agent
- **Simple sequential work**: just do it yourself — spawning 1 agent for 1 task adds overhead
- **You need real-time streaming output**: agents work in the background; you only see the final result
