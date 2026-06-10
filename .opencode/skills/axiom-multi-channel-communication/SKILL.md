---
name: axiom-multi-channel-communication
description: Portable multi-channel communication protocol for Axiom — channel router, MCP client integration, SES/SNS fallback, wait-for-reply mechanism, message routing, and channel contracts for Slack, GitHub, Jira, and Email.
version: "1.0"
synopsis: |
  Defines how Axiom communicates outbound through Slack, Email (SES), GitHub, and Jira using MCP
  as primary transport and SES/SNS as reliability fallback. Covers unified message types, canonical
  message envelope, per-channel contracts, MCP client protocol, dual-path delivery, wait-for-reply
  with configurable timeouts, message routing rules, configuration schema, security constraints,
  error handling, and structured logging/metrics.
when-to-use: |
  Load this skill when implementing or configuring multi-channel communication, building channel
  adapters (Slack/GitHub/Jira/Email), implementing MCP client integration for outbound messaging,
  designing wait-for-reply workflows, configuring SES/SNS fallback delivery, or routing messages
  across channels based on type and urgency.
tags:
  vertical: [ops, planning]
  category: operations
  core: false
---

# Axiom Multi-Channel Communication (Portable)

This skill defines the multi-channel communication protocol for Axiom.

Source spec: `specs/60-Multi-Channel-Communication.md`

---

## Architecture Overview

Axiom uses a **Channel Router** that dispatches structured messages to one or more channels based on configuration, message type, and context.

- **Primary path**: MCP tool calls to channel MCP servers (Slack, GitHub, Jira)
- **Fallback path**: SES email or SNS notification as reliability backstop
- **Wait-for-reply**: Pause execution, send question, wait for human response, resume

**Key distinction**: This is Axiom as MCP *client* (outbound). `specs/52-MCP-Server-Integration.md` is Axiom as MCP *server* (inbound).

---

## Unified Message Types

All channels support the same message types. Channel-specific formatting is handled by adapters.

| Message Type | Purpose | Urgency | Fallback Required |
|---|---|---|---|
| `status_update` | Run progress, phase transitions | Low | Configurable (default: no) |
| `question` | Clarifying questions requiring human response | Medium | Yes |
| `escalation` | Blocked run, failed verification, human action needed | High | Yes |
| `evidence_summary` | Verification results, confidence scores, PR readiness | Medium | Configurable (default: yes) |
| `pushback` | Intake confidence pushback | Medium | Yes |
| `broken_arrow` | Emergency swarm activation | Critical | Yes (always) |

---

## Canonical Message Envelope

```yaml
message:
  id: "msg_<ulid>"
  type: "question"                     # one of the message types
  work_item_id: "ABC-123"
  run_id: "run-2026-03-09T10-00-00Z_01"
  repo: "org/repo"
  subject: "Clarification needed: rate limit thresholds"
  body: |
    Axiom needs clarification...
  metadata:
    confidence_score: 45
    confidence_band: "MEDIUM"
    phase_id: "phase-1"
    task_id: "task-1-2"
    step_id: "step-1-2-3"
  channels: [slack, jira]              # target channels
  fallback:
    enabled: true
    type: "ses"                        # "ses" or "sns"
  reply_expected: true
  reply_timeout_seconds: 86400
  trace: "axiom:trace work_item=ABC-123 spec=specs/60-Multi-Channel-Communication.md"
```

---

## Channel Contracts

### Slack

| Operation | MCP Tool | Description |
|---|---|---|
| Send message | `slack_send_message` | Post to configured channel |
| Reply in thread | `slack_reply_to_thread` | Follow-up in existing thread |
| Read replies | `slack_get_thread_replies` | Poll for new replies |
| Add reaction | `slack_add_reaction` | Emoji reaction |
| Update message | `slack_update_message` | Edit previously sent message |

Idempotency: `metadata` field with `codeops_message_id` for deduplication.

### GitHub

| Operation | MCP Tool | Description |
|---|---|---|
| Post comment | `github_create_issue_comment` | Comment on issue/PR |
| Add reaction | `github_add_reaction` | Emoji reaction |
| Create issue | `github_create_issue` | New issue for escalations |
| Create PR | `github_create_pull_request` | New pull request |
| Read issue/PR | `github_get_issue` / `github_get_pull_request` | Read content |
| List comments | `github_list_issue_comments` | Reply detection |

Transport toggle: `channels.github.transport: mcp` (opt-in) or `http` (default, existing adapter).

### Jira

| Operation | MCP Tool | Description |
|---|---|---|
| Create issue | `jira_create_issue` | Issue/epic/sub-task |
| Add comment | `jira_add_comment` | ADF-formatted comment |
| Transition | `jira_transition_issue` | Change status |
| Update fields | `jira_update_issue` | Modify fields |
| Read issue | `jira_get_issue` | Read content |
| List comments | `jira_list_comments` | Reply detection |

Transport toggle: `channels.jira.transport: mcp` or `http` (default).

### Email (SES) -- Deferred

Send-only in v1. Contract defined for future implementation. Supports SES `SendEmail` and `SendTemplatedEmail` APIs.

---

## MCP Client Integration

### Protocol Requirements

- MCP protocol version `2024-11-05`
- `initialize` handshake before any tool calls
- `tools/list` discovery after init; validate required tools present
- Missing required tools -> channel marked `degraded`, fallback activated
- Both `stdio` and `sse` transports supported

### Tool Discovery Validation

| Channel | Required Tools | Optional Tools |
|---|---|---|
| Slack | `slack_send_message`, `slack_get_thread_replies` | `slack_reply_to_thread`, `slack_add_reaction`, `slack_update_message` |
| GitHub | `github_create_issue_comment`, `github_list_issue_comments` | `github_add_reaction`, `github_create_issue`, etc. |
| Jira | `jira_add_comment`, `jira_list_comments` | `jira_create_issue`, `jira_transition_issue`, etc. |

### Security

- MCP commands validated against binary allowlist (controller-level, not per-repo)
- Packages MUST use pinned versions; `npx -y` with unpinned versions PROHIBITED in production
- MCP server processes started with EMPTY environment + only declared env vars + system vars (`PATH`, `HOME`, `LANG`)
- Tool name mappings configurable via `tool_mappings` in config

---

## SES/SNS Reliability Fallback

### Dual-Path Delivery

- `question`, `escalation`, `pushback`, `broken_arrow`: ALWAYS sent through both primary AND fallback
- `status_update`, `evidence_summary`: primary only; fallback configurable
- Primary failure: fallback attempted regardless of message type

### Fallback Configuration

```yaml
channels:
  fallback:
    enabled: true
    type: ses                          # "ses" or "sns"
    ses:
      sender: "axiom@example.com"
      region: "us-east-1"
      recipients:
        default: ["team@example.com"]
        escalation: ["oncall@example.com"]
        broken_arrow: ["engineering-all@example.com"]
    sns:
      topic_arn: "arn:aws:sns:us-east-1:123456789:axiom-notifications"
```

### SNS Message Attributes

| Attribute | Type | Value |
|---|---|---|
| `message_type` | String | One of the unified message types |
| `work_item_id` | String | Work item identifier |
| `repo` | String | `org/repo` |
| `urgency` | String | `low`, `medium`, `high`, `critical` |

Credentials: AWS credential chain (env vars, instance profile, task role). NEVER in config files.

---

## Wait-for-Reply Mechanism

### Flow

1. Runtime sends message with `reply_expected=true`
2. Channel Router delivers message and registers pending reply with timeout
3. Reply Listener polls/receives webhooks for replies
4. On reply: validate source, sanitize content, correlate to pending request, resume execution
5. On timeout: send escalation, resume with timeout signal

### Reply Correlation

| Channel | Mechanism |
|---|---|
| Slack | Thread timestamp (`thread_ts`) |
| GitHub | Issue/PR number + `<!-- axiom:reply_to=msg_... -->` marker |
| Jira | Issue key + `axiom:reply_to=msg_...` marker |
| Email | `In-Reply-To` / `References` headers |

### Timeout Configuration

```yaml
channels:
  wait_for_reply:
    default_timeout_seconds: 86400     # 24 hours
    poll_interval_seconds: 60
    timeouts:
      slack: { question: 14400, escalation: 3600 }
      jira: { question: 86400, escalation: 14400 }
      github: { question: 86400, escalation: 14400 }
    on_timeout: "escalate"             # "escalate" | "proceed_with_assumptions" | "block"
```

### Security

- Pending reply state includes HMAC integrity signature (or stored on controller, outside agent write scope)
- Reply source validation MANDATORY: authorized responders list per work item
- Replies from unauthorized users: logged and ignored
- Reply content sanitized per input sanitization spec before injection

---

## Message Routing

### Routing Precedence

1. **Explicit channel override** in message
2. **Message type routing table** from config
3. **Work item source channel** (reply where work came from)
4. **Default channel** from config

### Routing Table

```yaml
channels:
  routing:
    default: jira
    by_type:
      status_update: { channels: [jira], fallback: false }
      question: { channels: [slack, jira], fallback: true }
      escalation: { channels: [slack, jira, github], fallback: true }
      evidence_summary: { channels: [jira, github], fallback: true }
      pushback: { channels: [jira], fallback: true }
      broken_arrow: { channels: [slack, jira], fallback: true }
```

### Multi-Channel Deduplication

- Each channel delivery tracked independently (shared message ID, unique delivery ID)
- First reply from any channel satisfies the pending reply

---

## Error Handling

### Error Taxonomy

| Error Class | Retryable | Action |
|---|---|---|
| `AUTH_FAILED` | No | Mark degraded; activate fallback |
| `RATE_LIMITED` | Yes | Backoff per provider rules |
| `CHANNEL_UNAVAILABLE` | Yes | Restart MCP server; retry; fallback after max |
| `DELIVERY_FAILED` | Depends | Retry if transient; fallback if permanent |
| `TIMEOUT` | Yes | Retry with backoff; fallback after max |
| `INVALID_MESSAGE` | No | Log ERROR; do not retry |

### Retry Policy

```yaml
channel_retry:
  max_attempts: 3          # 1 initial + 2 retries
  base_delay_seconds: 2.0
  max_delay_seconds: 30.0
  jitter: true
  backoff_factor: 2
```

### MCP Server Recovery

1. Detect failure within 30s (process monitoring or heartbeat)
2. Restart MCP server (max 3 attempts with exponential backoff)
3. If restart fails: mark `degraded`, activate fallback
4. On recovery: re-validate and mark `healthy`

### Exhausted Delivery

If both primary and fallback fail:
1. Emit `ERROR` event `channel_delivery_exhausted`
2. Persist to `.axiom/state/undelivered-messages.json` for manual retry
3. For `escalation`/`broken_arrow`: attempt any remaining enabled channel

---

## Structured Log Events

| Event | Level | Key Fields |
|---|---|---|
| `channel_message_sent` | INFO | channel, message_type, message_id, duration_ms |
| `channel_message_failed` | ERROR | channel, error_class, reason |
| `channel_fallback_triggered` | WARN | channel, fallback_type, reason |
| `channel_fallback_sent` | INFO | fallback_type, duration_ms |
| `channel_fallback_failed` | ERROR | fallback_type, error_class |
| `channel_reply_received` | INFO | channel, message_id, reply_source |
| `channel_reply_timeout` | WARN | channel, timeout_seconds |
| `channel_mcp_connected` | INFO | channel, transport, tools_available |
| `channel_mcp_disconnected` | WARN | channel, reason |
| `channel_mcp_tool_invoked` | DEBUG | channel, tool_name, duration_ms |
| `channel_degraded` | WARN | channel, missing_tools |
| `channel_recovered` | INFO | channel |

---

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `codeops_channel_messages_sent_total` | Counter | channel, message_type, status |
| `codeops_channel_message_duration_seconds` | Histogram | channel, message_type |
| `codeops_channel_fallback_total` | Counter | fallback_type, status |
| `codeops_channel_replies_total` | Counter | channel, message_type |
| `codeops_channel_reply_latency_seconds` | Histogram | channel, message_type |
| `codeops_channel_timeouts_total` | Counter | channel, message_type |
| `codeops_channel_mcp_health` | Gauge | channel |
| `codeops_channel_degraded` | Gauge | channel |

---

## Configuration Validation Rules

At startup, validate:
1. If `channels.enabled`, at least one channel must be enabled
2. MCP transport requires non-empty `mcp.command`
3. Fallback enabled requires valid SES sender or SNS topic ARN
4. `routing.default` must reference an enabled channel
5. Routes referencing disabled channels: ignored with WARN
6. Timeout: > 0 and <= 604800 (7 days)
7. Poll interval: >= 10 and <= 3600
8. MCP `env` values reference env var names (resolved at runtime, never raw secrets)
