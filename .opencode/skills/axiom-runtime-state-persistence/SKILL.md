---
name: axiom-runtime-state-persistence
description: Portable runtime state persistence model — run state machine, checkpoint semantics, plan cursor, crash recovery, delegation stack, side-effect idempotency, file locking, atomic writes, and watchdog timeouts.
version: "1.0"
synopsis: |
  Defines how the Axiom repo runner persists run state, checkpoints, and plan cursor position to
  support crash recovery, resume, and auditability. Covers the run state model (pending → in_progress →
  blocked → completed → failed → cancelled), checkpoint schema and timing, plan cursor advancement,
  resume-after-crash algorithm, delegation stack persistence, step side-effect idempotency strategies
  (git, Jira, file writes, PRs), file-based locking with TOCTOU prevention, atomic file writes,
  materializer state persistence, pending/stuck run timeouts, and concurrency safety.
when-to-use: |
  Load this skill when implementing checkpoint logic, designing crash recovery, building the plan
  cursor, handling delegation stack persistence, implementing file-based locks, designing idempotent
  side effects, or configuring run timeout/watchdog behavior.
tags:
  vertical: [coding, ops]
  category: development
  core: false
---

# Runtime State Persistence Model (Portable)

This skill defines how the Axiom repo runner persists run state for crash recovery and auditability.

Source spec: `specs/24-Runtime-State-Persistence.md`

---

## Run State Model

| Status | Meaning |
|--------|---------|
| `pending` | Run created but execution has not started |
| `in_progress` | Runner is actively executing plan steps |
| `blocked` | Execution paused; waiting for human input or external event |
| `failed` | Run terminated due to unrecoverable error or escalation |
| `completed` | All plan steps and verification gates passed |
| `cancelled` | Run cancelled via external request (API/UI/CLI) |

### State Transitions

```
pending → in_progress        (runner starts executing)
pending → cancelled          (external cancel before execution)
in_progress → blocked        (human input needed, or escalation)
in_progress → failed         (unrecoverable error, retries exhausted)
in_progress → completed      (all steps + verifications pass)
in_progress → cancelled      (external cancel)
blocked → in_progress        (blocker resolved, human input received)
blocked → failed             (stuck-time exceeded, or human cancels)
blocked → cancelled          (external cancel)
```

Rules:
- Only the repo runner may transition states (no external mutation in v1).
- Every transition is logged as a structured event.
- `completed`, `failed`, and `cancelled` are terminal states. A new run is created for re-execution.

---

## Checkpoint Semantics

### When Checkpoints Occur

1. **After each step completes** (success or failure) — before advancing cursor.
2. **After each verification gate completes** — before deciding inject or proceed.
3. **On state transitions** — whenever `execution.status` changes.
4. **Before escalation** — capture full context before posting to Jira.

### Checkpoint Schema (v1)

```yaml
version: 1
work_item_id: "ABC-123"
run_id: "2026-02-05T15-22-09Z-abc"
timestamp: "2026-02-05T15:25:00Z"
execution:
  status: "in_progress"
  cursor:
    phase_id: "phase-1"
    task_id: "task-1"
    step_id: "step-3"
  last_completed_step:
    phase_id: "phase-1"
    task_id: "task-1"
    step_id: "step-2"
    outcome: "ok"           # ok | fail | blocked
    verification_passed: true
  attempts:
    current_task_attempts: 1
    total_steps_executed: 5
    total_steps_failed: 0
  delegation_stack:
    - target: "@security-review-axiom"
      delegated_from_step: "step-3"
      return_to: "step-4"
      delegated_at: "2026-02-05T15:24:00Z"
      status: "in_progress"  # pending | in_progress | completed | failed
recovery:
  resumable: true
  resume_from:
    phase_id: "phase-1"
    task_id: "task-1"
    step_id: "step-3"
  reason: null
```

### Checkpoint File Locations

```
.memory-bank/work-items/<WORK_ITEM_ID>/
  plan.yaml              # current plan with execution.status + cursor
  plan.md                # human-readable plan (updated with outcomes)
  checkpoint.yaml        # lightweight checkpoint metadata
```

---

## Plan Cursor

### Cursor Fields

```yaml
execution:
  cursor:
    phase_id: "phase-1"
    task_id: "task-1"
    step_id: "step-1"
```

### Advancement Rules

- Cursor advances only after a step completes AND its required verifications pass.
- Verification failure with injected steps → cursor moves to first injected step.
- Step failure with retries available → cursor stays on same step.
- Step failure with retries exhausted → cursor stays; run transitions to `failed` or `blocked`.

### Persistence Locations

1. `plan.yaml` (`execution.cursor`) — always reflects next step to execute.
2. `checkpoint.yaml` (`execution.cursor` + `last_completed_step`) — includes last completed step for recovery.

---

## Resume-After-Crash

### Decision Algorithm

```
1. Read checkpoint.yaml for the work item.
2. If no checkpoint → start a new run.
3. If checkpoint unreadable → corruption handling (see below).
4. If checkpoint valid:
   a. If status is completed/failed/cancelled → start a new run.
   b. If status is in_progress/blocked:
      - If recovery.resumable is true → resume from recovery.resume_from.
      - If recovery.resumable is false → mark failed, start new run.
5. On resume:
   a. Validate cursor against plan.yaml.
   b. Set status to in_progress, continue from cursor.
```

### Checkpoint Corruption Handling

If `checkpoint.yaml` is unreadable (YAML parse error, missing required fields):

1. Log WARNING with corruption details.
2. Treat as absent — start from beginning of current phase (not entire plan).
3. Determine "current phase" from `plan.yaml` (first phase with incomplete steps).
4. Write fresh `checkpoint.yaml` with recovered cursor.

### Plan vs Checkpoint Disagreement

If checkpoint references a `step_id` that doesn't exist in `plan.yaml`:

1. Log WARNING with mismatched step_id.
2. Reset cursor to first incomplete step in plan.
3. Write updated `checkpoint.yaml`.

---

## Delegation Stack

### Frame Schema

| Field | Type | Description |
|-------|------|-------------|
| `target` | string | Agent handle delegated to |
| `delegated_from_step` | string | Step ID that triggered delegation |
| `return_to` | string | Step ID to resume after completion |
| `delegated_at` | ISO 8601 | Timestamp of delegation |
| `status` | enum | `pending`, `in_progress`, `completed`, `failed` |

### Persistence Rules

- Frame MUST be written to `checkpoint.yaml` BEFORE delegate agent is invoked.
- Status: `pending` → `in_progress` → `completed` or `failed`.
- On completion: pop frame, advance to `return_to`.
- On failure: pop frame, mark delegating step as failed.
- Maximum stack depth: 5 (per XML protocol spec).

### Crash Recovery with Delegation Stack

| Top Frame Status | Recovery Action |
|------------------|-----------------|
| Empty stack | Normal cursor-based recovery |
| `pending` | Invoke delegate (never started) |
| `in_progress` | Re-invoke delegate (was mid-execution) |
| `completed` | Pop frame, advance to `return_to` |
| `failed` | Pop frame, mark delegating step failed |

---

## Step Side-Effect Idempotency

When a step is re-executed after crash, use these strategies:

| Side Effect | Strategy | Check |
|-------------|----------|-------|
| Git branch creation | Check if branch exists before creating | `GET /repos/.../git/ref/heads/{branch}` |
| Git commit | Check if HEAD contains expected changes | Compare file contents or commit message marker |
| Jira comment | Search for `axiom:run_id={run_id}` marker | Update existing if found; create if not |
| File writes | Overwrite (inherently idempotent) | N/A |
| PR creation | Check for existing open PR on branch | `GET /repos/.../pulls?state=open&head=...` |
| PR comment | Search for `axiom:run_id` + `section` marker | Update existing if found |

---

## Run Artifact Persistence

### Current State (Mutable)

```
.memory-bank/work-items/<WORK_ITEM_ID>/
  plan.yaml, plan.md, meta-planning.md, checkpoint.yaml, verification.md, pr.md
```

### Per-Run Snapshots (Immutable After Completion)

```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/
  inputs.md, plan.yaml, plan.md, checkpoint.yaml, verification.md, outputs.md, events.jsonl
```

- `inputs.md` and initial plan written at run start.
- Final checkpoint, verification, outputs written at run end.
- Snapshots are immutable after run ends.

---

## Lock and Concurrency Model

### v1: Single Runner Per Work Item

- One runner instance per work item at a time.
- No concurrent step execution within a run.
- No concurrent runs for the same work item.

### File-Based Lock

- Location: `.memory-bank/work-items/<WORK_ITEM_ID>/run.lock`
- Contents: `run_id`, `started_at`, `pid`, `hostname`
- Acquisition: MUST use `os.open()` with `O_CREAT | O_EXCL` (atomic creation, TOCTOU prevention).
- Stale detection: if PID is dead, remove lock and retry.
- Release: MUST be in `finally` block — guaranteed even on unhandled exceptions.

### PID Validation

- PID values ≤ 0 MUST be treated as invalid/stale (prevents `os.kill(0, 0)` false positive).
- `stale_lock_timeout_seconds` applied during acquisition.

### Lock File Format Validation

- MUST be valid YAML with required fields (`run_id`, `started_at`, `pid`, `hostname`).
- Non-YAML, empty, or missing-field lock files treated as corrupt and removed with WARNING.

---

## Atomic File Writes

All state file writes MUST be atomic:

1. Write to temporary file in same directory (unique name: `.<target>.<pid>.<timestamp>.tmp`).
2. Rename to target path (atomic on POSIX).
3. If rename fails, clean up temp file and propagate error.

---

## Materializer State Persistence

For API server restart resilience:

- File: `.axiom/state/materializer-state.json`
- Persist: best-effort snapshot on event ingestion, every 5 seconds, and on graceful shutdown.
- Durability: temp-file + flush + fsync + atomic rename.
- Restore: best-effort load during startup before accepting API traffic.
- Corruption: log warning, start with empty state (fail-open for availability).

---

## Watchdog Timeouts

### Pending Run Timeout

- Default: 300 seconds (5 minutes).
- Config: `persistence.pending_timeout_seconds`.
- Behavior: transition to `failed` with `reason: "pending_timeout"`.

### Stuck Run Detection

- Default threshold: 1800 seconds (30 minutes).
- Config: `persistence.stuck_run_timeout_seconds`.
- At 1x threshold: emit `run_stuck_detected` WARNING.
- At 2x threshold: transition to `blocked` with `reason: "stuck_timeout"`.

### Watchdog Serialization

- Watchdogs MUST re-read current status before applying transitions.
- State transitions MUST use compare-and-swap semantics.
- Two watchdogs MUST NOT transition the same run to different terminal states.

---

## Configuration

```yaml
persistence:
  checkpoint_on_every_step: true       # Per-step checkpoints (safer but slower)
  snapshot_events_jsonl: false          # Per-run event log in snapshot
  stale_lock_timeout_seconds: 300      # Stale lock detection threshold
  pending_timeout_seconds: 300         # Pending run timeout
  stuck_run_timeout_seconds: 1800      # Stuck run detection threshold
```

---

## Plan Injection Interaction

When a verification gate injects new steps:

1. Update `plan.yaml` with injected steps.
2. Set cursor to first injected step.
3. Write checkpoint with updated plan and cursor.
4. Resume execution from injected step.

If crash occurs between injection and checkpoint: re-run verification on resume (may re-inject — acceptable; verification gates catch issues).

---

## Resolved Decisions

| Decision | Resolution |
|----------|------------|
| Step-level rollback | v1: no rollback. Re-execution relies on idempotency checks. |
| Checkpoint backend | v1: file-based YAML on disk. Future: database backend. |
| Run retention | v1: keep all runs. Future: configurable retention policy. |
| Distributed locking | v1: file-based (single host). Future: Redis/etcd. |
| Partial step completion | v1: re-execute entire step on crash. Future: finer-grained checkpointing. |
