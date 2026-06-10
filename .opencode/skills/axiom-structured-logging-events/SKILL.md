---
name: axiom-structured-logging-events
description: Portable rules for emitting Axiom structured log events (schema, correlation, redaction, omit-not-null).
version: "2.0"
license: MIT
compatibility: opencode
metadata:
  workflow: logging
  outputs: "none (behavioral guidance only)"
tags:
  vertical: [coding, ops, sre]
  category: development
  core: false
---

# Axiom Structured Logging Events (Portable)

Use this skill when adding or reviewing runtime logging. All rules, event catalogs, and schemas are inlined — no spec file required.

axiom:trace work_item=doctrine spec=specs/25-Structured-Logging-Events.md plan= test= doc=.opencode/skills/axiom-structured-logging-events/SKILL.md evidence= commit=

---

## Format

- All log events are emitted as **JSON Lines** (one JSON object per line, newline-delimited).
- Encoding: UTF-8.
- Timestamp format: ISO 8601 with timezone (`YYYY-MM-DDTHH:MM:SS.sssZ`).
- Output destination (v1): stdout/stderr.

---

## Required Correlation Fields

Every log event MUST include these fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `timestamp` | string (ISO 8601) | Always | Event time |
| `level` | string | Always | `DEBUG`, `INFO`, `WARN`, or `ERROR` |
| `event_type` | string | Always | Structured event name (see catalog below) |
| `component` | string | Always | `repo_runner`, `control_plane`, or subcomponent |
| `run_id` | string | When in a run | Ties all events in a run together |
| `work_item_id` | string | When in a run | Jira key or stable slug |
| `repo` | string | When in a run | `org/repo` identifier |
| `phase_id` | string | When executing a step | Current phase |
| `task_id` | string | When executing a step | Current task |
| `step_id` | string | When executing a step | Current step |
| `correlation_id` | string | When making HTTP calls | `X-Axiom-Correlation-ID` |
| `controller_id` | string | When in controller mode | Controller instance identifier |

### Omit, Never Null

Fields that are not applicable MUST be **omitted entirely**. Do NOT set to `null` or `""`. If a field is not applicable in context, omit it.

### Baseline Field Validation

`level`, `event_type`, and `component` MUST be validated at emission time:
- `level` MUST be one of: `DEBUG`, `INFO`, `WARN`, `ERROR`. `None`, empty string, and non-string values MUST be rejected with `ValueError`.
- `event_type` MUST be a non-empty string. `None` and `""` MUST be rejected.
- `component` MUST be a non-empty string. `None` and `""` MUST be rejected.

**Verification**: `emit_event(level=None)` raises `ValueError`. `emit_event(event_type="")` raises `ValueError`.

---

## Log Levels

| Level | When to Use |
|---|---|
| `DEBUG` | Internal state details; not emitted in production by default |
| `INFO` | Normal operational events: run started, step completed, PR created. Primary audit trail. |
| `WARN` | Recoverable issues: optional XML tags missing, retry triggered, stale lock detected |
| `ERROR` | Failures requiring attention: step failed, escalation triggered, unrecoverable error |

Rules:
- `INFO` is the default production log level.
- `DEBUG` is enabled via env var or config flag only.
- `WARN` and `ERROR` events SHOULD include a `reason` field with actionable context.
- Do NOT log expected control-flow as `ERROR` (e.g., a verification failure that triggers injection is `INFO`).

---

## Event Type Catalog

### Run Lifecycle

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `run_started` | INFO | Run begins | `run_id`, `work_item_id`, `repo`, `plan_step_count` |
| `run_resumed` | INFO | Run resumes after crash | `run_id`, `resume_from_step`, `reason` |
| `run_completed` | INFO | All steps and verifications pass | `run_id`, `total_steps`, `total_duration_ms`, `final_confidence` |
| `run_failed` | ERROR | Run terminates with failure | `run_id`, `reason`, `reason_code`, `failed_at_stage`, `last_step`, `total_attempts` |
| `run_blocked` | WARN | Run paused for human input | `run_id`, `reason`, `reason_code`, `operator_action`, `questions` |
| `state_transition` | INFO | `execution.status` changes | `run_id`, `from_status`, `to_status`, `reason` |

### Step Execution

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `step_started` | INFO | Step execution begins | `phase_id`, `task_id`, `step_id`, `command` |
| `step_completed` | INFO | Step execution succeeds | `phase_id`, `task_id`, `step_id`, `command`, `duration_ms`, `confidence`, `status` |
| `step_failed` | ERROR | Step execution fails (after retries) | `phase_id`, `task_id`, `step_id`, `command`, `error_class`, `reason`, `attempts` |
| `step_retried` | WARN | Step is being retried | `phase_id`, `task_id`, `step_id`, `command`, `attempt`, `retry_reason` |
| `step_skipped` | INFO | Step skipped on resume (already completed) | `phase_id`, `task_id`, `step_id`, `reason` |

### Verification

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `verification_started` | INFO | Verification gate begins | `phase_id`, `task_id`, `step_id`, `verifier_type`, `verifier_command` |
| `verification_passed` | INFO | Verification gate passes | `phase_id`, `task_id`, `step_id`, `verifier_type`, `score` |
| `verification_failed` | INFO | Verification gate fails (may trigger injection) | `phase_id`, `task_id`, `step_id`, `verifier_type`, `reason`, `inject_requested` |
| `plan_injection` | INFO | Verifier requests new steps | `phase_id`, `task_id`, `injected_steps`, `reason` |

Note: `verification_failed` is `INFO` not `ERROR` — it is expected control flow. It becomes `ERROR` only if it leads to run failure.

### OpenCode HTTP

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `opencode_request_sent` | DEBUG | HTTP request sent to OpenCode | `correlation_id`, `command`, `endpoint` |
| `opencode_response_received` | DEBUG | HTTP response received | `correlation_id`, `http_status`, `duration_ms`, `token_usage` |
| `opencode_request_failed` | WARN | HTTP request failed (will retry) | `correlation_id`, `error_class`, `http_status`, `retry_attempt` |
| `opencode_health_check` | DEBUG | Health check poll | `status`, `attempt` |
| `opencode_health_ok` | INFO | Health check passes (once at startup) | `version`, `startup_duration_ms` |
| `opencode_health_failed` | ERROR | Health check fails after timeout | `reason`, `attempts`, `timeout_ms` |

### XML Parsing

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `xml_parse_ok` | DEBUG | XML envelope parsed successfully | `command`, `tags_found` |
| `xml_tags_missing` | WARN | Required XML tags missing (will retry) | `command`, `missing_tags`, `attempt` |
| `xml_tags_invalid` | WARN | XML tags present but fail validation | `command`, `invalid_tags`, `validation_errors` |
| `xml_v2_retry` | INFO | Retrying with v2 variant for tag regeneration | `command`, `missing_tags`, `attempt` |

### V2 Variant Recovery

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `v2_variant_started` | INFO | V2 variant invocation begins | `command`, `missing_tags`, `attempt`, `v2_type` |
| `v2_variant_completed` | INFO | V2 variant recovers tags | `command`, `recovered_tags`, `attempt`, `v2_type`, `duration_ms` |
| `v2_variant_failed` | WARN | Recovery attempt fails | `command`, `missing_tags`, `attempt`, `v2_type`, `reason` |
| `direct_model_call_started` | INFO | Direct model call (last resort) begins | `command`, `missing_tags`, `attempt`, `model_hint` |
| `direct_model_call_completed` | INFO | Direct model call recovers tags | `command`, `recovered_tags`, `attempt`, `duration_ms` |
| `direct_model_call_failed` | WARN | Direct model call fails | `command`, `missing_tags`, `attempt`, `reason` |
| `xml_v2_success` | INFO | All remaining tags recovered | `command`, `recovered_tags`, `attempt`, `v2_type` |
| `xml_v2_partial` | WARN | Some but not all tags recovered | `command`, `recovered_tags`, `still_missing_tags`, `attempt`, `v2_type` |
| `xml_recovery_exhausted` | ERROR | All recovery attempts failed; escalating | `command`, `missing_tags`, `total_attempts`, `escalation_reason` |
| `xml_merge_completed` | DEBUG | Tags merged from recovery into original | `command`, `merged_tags`, `source` |

`v2_type` values: `variant` (attempt 2) or `direct` (attempt 3).

### Escalation

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `escalation_triggered` | ERROR | Escalation to human via Jira | `reason`, `attempts_exhausted`, `stuck_duration_ms` |
| `jira_comment_posted` | INFO | Escalation comment posted to Jira | `jira_key`, `comment_type` |

### Progress Events

Emitted at lifecycle boundaries to provide visibility into what the system is doing, including orchestrator-heavy phases where no plan steps are executing.

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `stage_changed` | INFO | Run transitions between progress stages | `run_id`, `work_item_id`, `stage`, `previous_stage`, `current_activity` |
| `activity_updated` | INFO | Current activity changes within a stage | `run_id`, `work_item_id`, `stage`, `current_activity` |

Valid `stage` values: `intake`, `planning`, `implementing`, `verifying`, `reviewing`, `finalizing`, `blocked`, `completed`, `failed`, `cancelled`.

`current_activity`: human-readable description, max 200 characters.

**Minimum emission boundaries**:

| Lifecycle Boundary | Event | `stage` | Example `current_activity` |
|---|---|---|---|
| Run picked up from queue | `stage_changed` | `intake` | `"Starting run"` |
| Meta-planning starts | `stage_changed` | `planning` | `"Starting meta-planning"` |
| First step starts | `stage_changed` | `implementing` | `"Executing step: {title}"` |
| Each step starts | `activity_updated` | `implementing` | `"Executing step: {title} ({path})"` |
| Verification chain starts | `stage_changed` | `verifying` | `"Running verification chain"` |
| PR created / review requested | `stage_changed` | `reviewing` | `"Awaiting human review on PR #{number}"` |
| Run blocked | `stage_changed` | `blocked` | `"Blocked: {reason}"` |
| Run completed | `stage_changed` | `completed` | *(omitted — terminal)* |

Rules:
- `stage_changed` MUST be emitted at each lifecycle boundary, even if no plan steps exist yet.
- `activity_updated` SHOULD NOT be emitted more frequently than once per 5 seconds.

### Liveness Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `run_stalled` | WARN | Run has not made forward progress for longer than stall threshold | `run_id`, `work_item_id`, `stage`, `last_progress_at`, `stall_threshold_seconds`, `stalled_duration_seconds` |

Rules:
- `run_stalled` MUST NOT be emitted for runs that are `is_waiting=true` (blocked/reviewing).
- `run_stalled` MUST be emitted at most once per stall episode.
- Default stall threshold: 600 seconds.

### Telemetry / Session Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `session_created` | INFO | Orchestrator creates an OpenCode session | `run_id`, `work_item_id`, `opencode_session_id` |
| `session_started` | INFO | Session created with step-linkage context | `run_id`, `work_item_id`, `opencode_session_id`, `phase_id`, `task_id`, `step_id`, `purpose` |
| `session_completed` | INFO | Session reaches idle/completed | `run_id`, `work_item_id`, `opencode_session_id`, `tokens_total`, `tokens_input`, `tokens_output`, `cost_usd`, `message_count`, `duration_ms` |
| `session_failed` | WARN | Session encounters unrecoverable error | `run_id`, `work_item_id`, `opencode_session_id`, `error`, `error_code` |
| `telemetry_snapshot` | INFO | Periodic telemetry poll completes | `run_id`, `work_item_id`, `opencode_session_id`, `opencode_session_status`, `tokens_total`, `tokens_input`, `tokens_output`, `cost_usd`, `message_count` |

`opencode_session_status` values: `busy`, `idle`, `retry`, `unknown`.

`telemetry_snapshot` rules:
- MUST NOT be emitted more frequently than the configured poll interval (default 5 seconds).
- All numeric fields MUST be `null` (not `0`) when data is unavailable.

### Integration Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `pr_created` | INFO | GitHub PR created | `pr_url`, `pr_number`, `branch` |
| `pr_updated` | INFO | GitHub PR updated | `pr_url`, `pr_number`, `commits_added` |
| `pr_ready_for_review` | INFO | PR marked ready for review | `pr_url`, `pr_number`, `confidence`, `evidence_bundle_path` |
| `pr_conflict_detected` | WARN | PR has merge conflicts | `pr_url`, `pr_number` |
| `pr_conflict_resolved` | INFO | Conflict remediation succeeded | `pr_url`, `pr_number` |
| `jira_transition` | INFO | Jira ticket status changed | `jira_key`, `from_status`, `to_status` |

### Checkpoint Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `checkpoint_written` | DEBUG | Checkpoint persisted to disk | `checkpoint_path`, `cursor_position` |
| `checkpoint_loaded` | INFO | Checkpoint loaded on resume | `checkpoint_path`, `cursor_position`, `run_id` |
| `lock_acquired` | DEBUG | Run lock acquired | `lock_path`, `run_id` |
| `lock_released` | DEBUG | Run lock released | `lock_path`, `run_id` |
| `lock_stale_detected` | WARN | Stale lock found and removed | `lock_path`, `stale_run_id`, `stale_pid` |

### MCP Server Events

| Event Type | Level | When | Key Fields |
|---|---|---|---|
| `mcp_server_started` | INFO | MCP server startup completes | `transport`, `api_base_url` |
| `mcp_server_stopped` | INFO | MCP server shutdown | `reason` |
| `mcp_tool_invoked` | INFO | MCP tool call received and completed | `tool_name`, `duration_ms`, `success`, `http_status` |
| `mcp_auth_failed` | WARN | Auth passthrough rejected by API server | `tool_name`, `http_status` |
| `mcp_api_connection_error` | ERROR | Cannot reach Axiom API server | `api_base_url`, `reason` |

---

## Event Payload Schema

Every event is a JSON object. Example:

```json
{
  "timestamp": "2026-02-05T15:25:03.456Z",
  "level": "INFO",
  "event_type": "step_completed",
  "component": "repo_runner.engine",
  "run_id": "2026-02-05T15-22-09Z-abc",
  "work_item_id": "ABC-123",
  "repo": "org/repo",
  "phase_id": "phase-1",
  "task_id": "task-1",
  "step_id": "step-2",
  "command": "/update-specs",
  "duration_ms": 45200,
  "confidence": 85,
  "status": "ok",
  "message": "Updated specs/03-Plan-Schema.md with new field definitions."
}
```

### Optional Fields (Any Event)

| Field | Type | Notes |
|---|---|---|
| `message` | string | Human-readable description |
| `reason` | string | Why this event occurred (especially for failures/warnings) |
| `duration_ms` | integer | Duration of the operation in milliseconds |
| `error_class` | string | Error classification (TIMEOUT, VALIDATION_ERROR, SERVER_ERROR, etc.) |
| `token_usage` | object | `{prompt_tokens, completion_tokens, total_tokens}` |
| `metadata` | object | Arbitrary key-value pairs for event-specific data |

---

## Sensitive Data Rules (Non-Negotiable)

- **Never log secrets, tokens, API keys, or passwords.** Redact as `[REDACTED]`.
- **Never log PII** unless explicitly required for audit and approved by governance.
- **Never log full request/response bodies** from external APIs at INFO level. Use DEBUG with redaction.
- **Never log full OpenCode response text** at INFO level. Log only structured metadata.
- **Redact environment variables** containing secrets when logging configuration at startup.

### Redaction Utility (`redact_sensitive`)

The structured logger MUST provide `redact_sensitive(data: dict) -> dict`:

- Returns a **new dict** — MUST NOT mutate the input.
- Scans all field names (case-insensitive) for: `token`, `secret`, `password`, `key`, `credential`, `auth`, `authorization`, `apikey`. Matching values are replaced with `[REDACTED]`.
- Scans all string values for patterns: GitHub PATs (`ghp_…`), Slack tokens (`xoxb-…`), OpenAI keys (`sk-…`), Bearer tokens, HTTP Basic auth, DB connection strings, URL query parameter secrets.
- Recursively processes nested dicts and lists.
- MUST handle non-string dict keys (int, None, tuple) without crashing — skip key-name matching, still recurse into values.
- `extra_patterns` containing `None` values MUST be silently skipped.
- `extra_patterns` containing invalid regex MUST be skipped with a WARN log, not crash.
- Redaction string is always exactly `[REDACTED]` — no variants.
- MUST NOT produce false positives on: `author`, `tokenizer`, `token_count`, `authentication_method`, `key_name`, `password_policy`.

**Verification**:
- `redact_sensitive({42: "value"})` does not crash.
- `redact_sensitive({"k": "v"}, extra_patterns=[None])` does not crash.
- `redact_sensitive({"author": "alice"})` returns `{"author": "alice"}` (not redacted).

---

## Observability Correlation

```
Jira ticket (work_item_id)
  → Run (run_id)
    → Step (phase_id/task_id/step_id)
      → OpenCode request (correlation_id)
        → XML envelope (run block fields)
    → Verification (verifier_type + step context)
    → PR (pr_url, pr_number)
  → Memory Bank artifacts (.memory-bank/work-items/<ID>/runs/<RUN_ID>/)
```

Queries that must be possible:
- All events for work item ABC-123: `work_item_id=ABC-123`
- All events for run X: `run_id=X`
- All failures for step-3 of task-1: `step_id=step-3 AND task_id=task-1 AND level=ERROR`
- All escalations in last 24h: `event_type=escalation_triggered`
- Token usage for a run: `event_type=opencode_response_received`, aggregate `token_usage`

---

## Configuration

```yaml
logging:
  level: "INFO"                    # default log level; override with AXIOM_LOG_LEVEL
  format: "jsonl"                  # v1: only jsonl supported
  redact_patterns:                 # additional regex patterns beyond defaults
    - "ghp_[A-Za-z0-9_]+"
    - "xoxb-[A-Za-z0-9-]+"
```

Environment variable overrides:
- `AXIOM_LOG_LEVEL`: overrides `logging.level`.
- `AXIOM_LOG_DEBUG_HTTP`: when `true`, enables DEBUG-level logging for OpenCode HTTP events.

---

## Per-Run Event Log

When `persistence.snapshot_events_jsonl: true` in `.axiom/axiom.config.yaml`:
- All events for a run are written to `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/events.jsonl`.
- This file is immutable after the run ends.
- Provides a self-contained audit trail without requiring access to a central log aggregator.

---

## Error Classification Requirements

### REQ-LOG-ERR-001: HTTP Error Classification

`classify_http_error()` MUST correctly classify HTTP status codes:

| Status | Classification | Retryable |
|---|---|---|
| 408 | `TIMEOUT` | Yes |
| 504 | `TIMEOUT` | Yes |
| 429 | `RATE_LIMITED` | Yes (with backoff) |
| 410 | `NOT_FOUND` | No |

The function MUST handle `response_body=None` without crashing (treat as empty string).

### REQ-LOG-ERR-002: Redaction Utility Robustness

See Redaction Utility section above.

### REQ-LOG-ERR-003: Exception Type Consistency

All Axiom-specific errors MUST inherit from `CodeOpsError`. Functions MUST raise:
- `ValueError` for invalid arguments (not `AttributeError`).
- `TypeError` for wrong argument types (not `AttributeError`).
- `AttributeError` MUST NOT be raised as a result of `None` being passed where a string is expected.

---

## Review Checklist

1. Does the event include baseline fields (`level`, `event_type`, `component`) and validate them?
2. Are optional fields omitted (not null)?
3. Are correlation fields included when in run/step/request context?
4. Are secrets and external bodies excluded or redacted?
5. Are `event_type` strings stable and grep-friendly?
6. Is `verification_failed` logged at `INFO` (not `ERROR`)?
7. Are `run_stalled` events emitted at most once per stall episode?
8. Are `telemetry_snapshot` events rate-limited to the configured poll interval?
