---
name: axiom-retry-escalation
description: Portable retry and escalation rules for Axiom when structured outputs are missing or verification fails repeatedly.
version: "2.0"
license: MIT
compatibility: opencode
metadata:
  workflow: resilience
  outputs: "none (behavioral guidance only)"
tags:
  vertical: [coding, ops]
  category: methodology
  core: false
---

# Axiom Retry and Escalation (Portable)

Use this skill when designing step retries, tag recovery behavior, and escalation-to-human flows. All rules are inlined — no spec file required.

axiom:trace work_item=doctrine spec=specs/12-Retry-And-Escalation.md plan= test= doc=.opencode/skills/axiom-retry-escalation/SKILL.md evidence= commit=

---

## Tag Recovery (V2 Variant Mechanism)

When required XML tags are missing, the runner invokes a 3-attempt tag recovery budget **per step execution attempt**:

| Attempt | Type | Description |
|---|---|---|
| 1 | Original command | Normal execution via `/command` |
| 2 | V2 variant | Tag-focused re-invocation with original response as context |
| 3 | Direct model call | Minimal tag extraction prompt, last automated attempt |

If all 3 attempts fail: step is marked `blocked` and escalated. No further automated tag recovery.

**Tag recovery vs step retry**: The 3-attempt tag recovery budget operates *within* a single step execution. It is separate from the step-level retry budget. If tag recovery fails and the step is `blocked`, that counts as one failed step toward the task-level escalation threshold.

---

## Step-Level Retry Budget

Each step gets up to **3 execution attempts**:

| Attempt | Type | Trigger |
|---|---|---|
| 1 | Original execution | Normal step execution |
| 2 | Retry (agent-requested) | Agent sets `<retry>true</retry>` in XML envelope |
| 3 | Final retry | Runner retries after verification failure (if retries remain) |

Rules:
- Each retry re-executes the same step (same command, same inputs) as a fresh attempt.
- Each retry gets its own 3-attempt tag recovery budget.
- After 3 step execution attempts, the step is marked as failed. No further retries.
- Configurable via `retry.max_retry_attempts` in `.axiom/axiom.config.yaml` (default: 3).

---

## Timeout Scoping

| Parameter | Scope | Default | Description |
|---|---|---|---|
| `max_minutes_stuck` | Per step | 30 | Max wall-clock time for a single step (including tag recovery). Exceeded → step marked `blocked`. |
| `max_failed_steps_per_task` | Per task | 3 | Max failed steps in a task before escalation is triggered. |

---

## Verification Failure Handling

When a verification gate fails:

1. Verifier SHOULD request injected work (new steps) via `<inject>` block in the XML envelope.
2. Runner applies injected work by updating `plan.yaml` per injection rules.
3. Runner advances cursor to first injected step and resumes.
4. After injected steps complete, runner re-runs the verification that originally failed.

If the verifier does NOT inject work (just reports failure): step is marked failed and counts toward the task-level escalation threshold.

---

## Escalation Conditions

Escalation is triggered when **any one** of the following is true (OR-combined):

| Condition | Description |
|---|---|
| Stuck time exceeded | Step running longer than `max_minutes_stuck` (default: 30 min) |
| Failed steps threshold | Failed steps in current task reaches `max_failed_steps_per_task` (default: 3) |
| Tag recovery exhausted | All 3 tag recovery attempts failed for a step |
| Required structured outputs unavailable | Cannot obtain required XML tags after all recovery attempts |
| Human-only decision required | Agent/verifier explicitly flags a decision requiring human input |
| Resource exhaustion | OOM kill, disk full, or PID limit exceeded after `max_resource_retries` exhausted |

---

## Escalation Output (Required Content)

When escalation is triggered, post a Jira comment containing:
- What was attempted (step ID, command, number of attempts)
- Current blockers (missing tags, verification failures, timeout)
- Proposed options/assumptions (from the agent's `<review>` block)
- What input is needed from a human
- Link to the evidence bundle path in the Memory Bank

---

## Escalation Failure Handling

| Scenario | Action |
|---|---|
| Jira API returns error | Log `escalation_failed`. Retry up to 2× with exponential backoff (5s, 15s). |
| All Jira retries fail | Log `escalation_jira_unreachable`. Mark step `blocked` in checkpoint. Write escalation content to `.memory-bank/work-items/<ID>/escalation-pending.md`. |
| Step is `required: true` | Halt the run. Set `execution.status` to `blocked`. |
| Step is `required: false` | Log the failure. Skip the step. Continue. Record in evidence bundle. |

Rules:
- Escalation failure MUST NOT cause the runner to crash or lose state. Checkpoint is always written before escalation is attempted.

---

## Configuration

```yaml
retry:
  max_retry_attempts: 3             # includes original + retries; min 1, max 10
  max_minutes_stuck: 30             # wall-clock time per step; triggers escalation if exceeded
  max_failed_steps_per_task: 3      # failed steps in a task before escalation
  v2_variant:
    enabled: true
    timeout_seconds: 60
    max_response_tokens: 2000
  direct_model_call:
    enabled: true
    timeout_seconds: 30
    max_response_tokens: 1500
    model: null                     # null = same model; or a model ID string
  max_tag_recovery_attempts: 3
```

**Configuration precedence** (highest → lowest):
1. Per-command `v2_variant` settings in `command-registry.yaml`
2. `defaults.v2_variant` in `command-registry.yaml`
3. `retry.*` in `axiom.config.yaml`
4. Hardcoded fallbacks in runner code

---

## Retry/Recovery Interaction Diagram

```
Step execution attempt 1:
  ├── Command execution → agent response
  ├── Tag validation → missing tags?
  │   ├── Tag recovery attempt 1 (v2 variant)
  │   ├── Tag recovery attempt 2 (direct model call)
  │   └── If all tag recovery fails → step marked blocked → counts as failed step
  ├── If tags present → verification gates
  │   ├── Verification passes → step complete, advance cursor
  │   └── Verification fails → inject work OR mark step failed
  └── If agent requests retry (<retry>true</retry>) → go to attempt 2

Step execution attempt 2 (retry):
  ├── Same command, same inputs, fresh execution
  ├── Gets its own 3-attempt tag recovery budget
  └── ... (same flow as attempt 1)

Step execution attempt 3 (final retry):
  └── ... (same flow; if this fails, step is permanently failed)

Task-level tracking:
  ├── Count failed steps across all steps in the task
  ├── If failed_steps >= max_failed_steps_per_task → escalate
  └── If step is blocked and required → halt run
```

---

## Structured Logging Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `step_retry` | INFO | Step is being retried | `step_id`, `attempt`, `reason` |
| `step_timeout` | WARN | Step exceeded `max_minutes_stuck` | `step_id`, `elapsed_minutes`, `max_minutes` |
| `step_failed` | WARN | Step failed after all retries | `step_id`, `total_attempts`, `reason` |
| `escalation_triggered` | WARN | Escalation condition met | `step_id`, `condition`, `task_failed_steps` |
| `escalation_posted` | INFO | Jira comment posted successfully | `step_id`, `jira_key`, `comment_id` |
| `escalation_failed` | ERROR | Jira post failed | `step_id`, `jira_key`, `error`, `retry_count` |
| `escalation_jira_unreachable` | ERROR | All Jira retries exhausted | `step_id`, `jira_key`, `fallback_path` |

---

## HTTP Error Classification (REQ-RETRY-HTTP408)

The `classify_http_error()` function MUST classify HTTP status codes correctly:

| Status | Classification | Retryable |
|---|---|---|
| 408 | `TIMEOUT` | Yes |
| 504 | `TIMEOUT` | Yes |
| 429 | `RATE_LIMITED` | Yes (with backoff) |
| 410 | `NOT_FOUND` | No (permanent) |
| 4xx (other) | `VALIDATION_ERROR` | No |
| 5xx (other) | `SERVER_ERROR` | Yes |

The function MUST validate that `status_code` is an integer. Non-integer inputs (float, string, None) MUST raise `TypeError` or `ValueError`.

**Verification**:
- `classify_http_error(408)` → `TIMEOUT, retryable=True`
- `classify_http_error(404.5)` → raises `TypeError`
- `classify_http_error("404")` → raises `TypeError`
