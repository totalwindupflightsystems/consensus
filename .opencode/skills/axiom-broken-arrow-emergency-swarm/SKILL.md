---
name: axiom-broken-arrow-emergency-swarm
description: Portable emergency concurrent agent swarm protocol for Axiom — time-boxed, inbox-driven multi-agent diagnosis with polling loops, shared findings, and incident commander synthesis.
version: "1.0"
synopsis: |
  Defines the /broken-arrow command: a time-boxed concurrent multi-agent emergency swarm that
  investigates a problem from all angles simultaneously. Agents communicate via inbox, post findings
  to a shared board, and an Incident Commander synthesizes a diagnosis report. Covers orchestrator
  script behavior, agent polling loop, communication protocol, default roster, configuration,
  safety constraints, and fail-safe recovery.
when-to-use: |
  Load this skill when implementing or invoking the /broken-arrow emergency swarm, designing
  concurrent agent investigation workflows, building inbox-driven agent communication, or
  implementing time-boxed multi-agent diagnosis for unknown failures.
tags:
  vertical: [ops, sre]
  category: operations
  core: false
---

# Broken Arrow: Emergency Concurrent Agent Swarm (Portable)

This skill defines the `/broken-arrow` emergency swarm protocol for Axiom.

Source spec: `specs/46-Broken-Arrow-Emergency-Swarm.md`

---

## Concept

When something breaks and the root cause is unknown, Broken Arrow launches a **swarm of specialized agents simultaneously**, each investigating a different angle. They communicate through the inbox system, share findings in real time, and converge on a diagnosis within a hard time limit.

| Normal Axiom Workflow | Broken Arrow |
|---|---|
| Sequential: plan -> step -> verify | Concurrent: all agents launch at once |
| One agent per step | Multiple agents per problem |
| Orchestrated by Tower | Self-coordinating via inbox |
| No time limit | Hard time limit (default 5 min) |
| Produces code changes | Produces diagnosis + recommended fixes |
| Driven by a plan | Driven by a problem statement |

**Key principle**: Broken Arrow does NOT apply fixes. It diagnoses. Fixing is a separate step.

---

## Architecture

### High-Level Flow

1. User invokes `/broken-arrow "problem statement"`
2. Orchestrator script creates incident ID (`BA-<timestamp>`)
3. Writes problem statement to shared inbox
4. Launches N agent sessions in parallel
5. Each agent runs a polling loop: check inbox -> investigate -> post findings -> repeat
6. At MAX_TIME: orchestrator sends STOP signal
7. Incident Commander (`sitrep-axiom`) synthesizes all findings into a report
8. Report is output to user

### Components

| Component | Responsibility |
|---|---|
| **Orchestrator Script** (`scripts/broken_arrow.sh`) | Parse args, create incident, launch agents, monitor, stop, collect report |
| **OpenCode Command** (`.opencode/commands/broken-arrow.md`) | Invocable from TUI, CLI, API, UI |
| **Agent Polling Loop** | Each agent's investigation cycle (inbox-driven) |
| **Shared Incident Inbox** | `.memory-bank/inbox/broken-arrow-<ID>/` |
| **Broken Arrow Report** | Final synthesis by `sitrep-axiom` |

### Shared Incident Inbox Structure

```
.memory-bank/inbox/broken-arrow-<ID>/
  problem.md              # Problem statement (written by orchestrator)
  _index.md               # Incident index
  findings/               # Agent findings (append-only)
    dev-axiom-001.md
    qa-axiom-001.md
    sre-ops-axiom-001.md
  stop.md                 # STOP signal (written by orchestrator at MAX_TIME)
  report.md               # Final report (written by sitrep-axiom)
```

---

## Default Agent Roster

### Core Swarm (always launched)

| Agent | Investigation Angle |
|---|---|
| `sitrep-axiom` | **Incident Commander** -- coordinates, synthesizes, writes final report |
| `dev-axiom` | Code path analysis -- traces crash/bug through source code |
| `qa-axiom` | Test suite analysis -- what's failing, passing, not covered |
| `trace-auditor-axiom` | Trace/plan drift -- did something change that shouldn't have? |

### Extended Swarm (keyword-triggered)

| Agent | Trigger Keywords | Investigation Angle |
|---|---|---|
| `sre-ops-axiom` | crash, startup, server, health, port | Runtime state, logs, process health |
| `security-review-axiom` | auth, permission, secret, token, access | Security/config issues |
| `redteam-axiom` | wrong, unexpected, behavior | Adversarial: "what if the spec was wrong?" |
| `assumption-buster-axiom` | always, should, supposed to | Hidden prerequisites, undocumented deps |
| `devils-advocate-axiom` | design, architecture, pattern | "What if the approach itself is wrong?" |
| `db-architect-axiom` | database, migration, query, schema | DB-layer issues |
| `performance-axiom` | slow, timeout, memory, cpu | Performance degradation |
| `cloud-engineer-axiom` | deploy, container, k8s, env | Infrastructure/environment issues |
| `dependency-bot-axiom` | import, module, package, version | Dependency/version conflicts |

### User Override

Users can override the roster: `axiom broken-arrow "problem" --agents dev-axiom,qa-axiom,sre-ops-axiom`

---

## Agent Polling Loop

Each agent runs this loop (expressed as prompt instructions):

```
BROKEN ARROW POLLING LOOP
Incident: <incident_id>
Max time: <max_seconds> seconds
Poll interval: <poll_interval> seconds

LOOP:
  1. CHECK STOP SIGNAL
     Read .memory-bank/inbox/broken-arrow-<ID>/stop.md
     If exists: finish current finding, write final summary, EXIT

  2. CHECK INBOX
     Read all files in .memory-bank/inbox/<my-name>/ with status: new
     For each: mark read, investigate requests, incorporate replies

  3. INVESTIGATE
     Run assigned investigation angle (bash, read files, check logs, run tests)
     MAY spawn sub-agents (via Task tool) for deeper investigation

  4. POST FINDINGS
     Write to .memory-bank/inbox/broken-arrow-<ID>/findings/<my-name>-<seq>.md
     Include: finding_type, confidence, evidence, hypothesis

  5. CHECK TIME
     If elapsed >= max_seconds: finish current finding, EXIT

GOTO LOOP
```

### Sleep Implementation

Agents cannot literally `sleep()`. The polling interval is implemented as:
1. Agent runs investigation work
2. Checks elapsed time via `date +%s`
3. If under poll_interval since last check: do lightweight work (re-read inbox, write notes)

### Sub-Agent Spawning

Agents MAY spawn sub-agents when:
- A specialist not in the current swarm is needed
- A deep-dive would take too long in the main loop
- Parallel investigation of sub-hypotheses is beneficial

Sub-agents inherit the incident ID, write to the same shared inbox, and are subject to MAX_TIME.

---

## Communication Protocol

### Inbox Message Schema (Broken Arrow variant)

```yaml
---
mb:
  type: message
  title: "<subject>"
  created: YYYY-MM-DDTHH:MM:SSZ
  links:
    up: "../_index.md"
from: <agent-name>
to: <agent-name> | broken-arrow-<ID>
status: new | read | acted | blocked
incident_id: broken-arrow-<ID>
finding_type: observation | hypothesis | request | reply | stop | report
confidence: 0-100
---
```

### Finding Types

| Type | Meaning | Written by |
|---|---|---|
| `observation` | Raw data point (log line, test output, file content) | Any agent |
| `hypothesis` | Proposed root cause with supporting evidence | Any agent |
| `request` | Request for another agent to investigate something | Any agent |
| `reply` | Response to a request | Any agent |
| `stop` | Signals all agents to finish and stop | Orchestrator |
| `report` | The final Broken Arrow Report | `sitrep-axiom` |

### Agent-to-Agent Direct Messaging

```
Agent A wants Agent B to check something:
  -> writes to .memory-bank/inbox/<agent-B>/<incident-id>-<timestamp>.md
  -> with finding_type: request

Agent B reads inbox, sees request:
  -> investigates
  -> writes reply to .memory-bank/inbox/<agent-A>/<incident-id>-<timestamp>-reply.md
  -> with finding_type: reply, in_reply_to: <original message path>
```

---

## Broken Arrow Report

Produced by `sitrep-axiom` as Incident Commander. Required sections:

1. **Incident ID** and timestamp
2. **Problem statement** (as given)
3. **Agent participation** -- which agents ran, what they investigated
4. **Key findings** -- top findings from all agents, ranked by confidence
5. **Root cause hypotheses** -- ranked list with supporting evidence
6. **Recommended fix steps** -- actionable next steps
7. **Evidence bundle** -- links to all finding files
8. **Confidence score** -- overall diagnosis confidence (0-100)

---

## Orchestrator Script Behavior

### Phases

1. **INIT**: Generate incident ID, create inbox structure, write problem.md, select agents, verify OpenCode server
2. **LAUNCH** (parallel): Create OpenCode sessions for each agent, send agent prompts, record session mappings
3. **MONITOR** (polling): Check session liveness, check for stop acknowledgments, check for report.md, break early if diagnosis found
4. **STOP**: Write stop.md, wait 30s for agents to finish, abort remaining sessions
5. **SYNTHESIS**: If no report yet, send synthesis command to sitrep-axiom, wait 60s
6. **OUTPUT**: Print report, write to memory bank, exit 0 (report) or 1 (timeout)

### CLI Arguments

```
broken_arrow.sh \
  --problem "description"
  --repo /path/to/repo
  --max-time 300          # seconds (default: 300)
  --poll-interval 15      # seconds (default: 15)
  --agents auto           # auto|all|<comma-separated>
  --max-agents 8          # max concurrent (default: 8)
  --opencode-url http://...
  --incident-id BA-xxx    # override (optional)
  --output-format text|json
```

---

## Configuration

### `.axiom/axiom.config.yaml`

```yaml
broken_arrow:
  max_time_seconds: 300
  poll_interval_seconds: 15
  max_agents: 8
  default_agents: auto       # auto | all | [list]
  auto_trigger_on_failures: 0  # 0 = disabled; N = trigger after N consecutive failures
  report_format: text
  keep_sessions_after: false
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `AXIOM_BA_MAX_TIME` | `300` | Max time in seconds |
| `AXIOM_BA_POLL_INTERVAL` | `15` | Poll interval in seconds |
| `AXIOM_BA_MAX_AGENTS` | `8` | Max concurrent agents |

---

## Safety Constraints

| Constraint | Rule |
|---|---|
| Read/write mode | Read-heavy, write-light by default |
| File reads | Agents MAY read any file in the repo |
| Diagnostic commands | Agents MAY run tests, linters, health checks |
| Remote push | Agents MUST NOT push to remote branches |
| Spec/memory modification | Agents MUST NOT modify `specs/` or `.memory-bank/` outside incident inbox |
| Fix implementation | NOT part of Broken Arrow (diagnosis only) |

### Resource Limits

| Limit | Default |
|---|---|
| Max concurrent agents | 8 (configurable) |
| Max time | 300s (hard ceiling: 600s) |
| Max sub-agents per agent | 3 |
| Max total sessions | 20 |

### Fail-Safe Behavior

| Failure | Recovery |
|---|---|
| Orchestrator crashes | Cleanup command: `axiom broken-arrow --cleanup <ID>` |
| Agent session errors | Log and continue with remaining agents |
| `sitrep-axiom` fails to report | Orchestrator assembles raw findings dump |
| MAX_TIME reached | Force synthesis with whatever findings exist |

---

## Invocation Surfaces

| Interface | How to invoke |
|---|---|
| OpenCode TUI | `/broken-arrow "problem"` |
| OpenCode CLI | `opencode run -p "/broken-arrow problem"` |
| Axiom CLI | `axiom broken-arrow "problem"` |
| Axiom API | `POST /api/v1/broken-arrow` with `{"problem": "..."}` |
| Axiom UI | Emergency Swarm panel |
| Runtime (auto) | Triggered on N consecutive failures (configurable) |
| Jira | Comment `@axiom broken-arrow: <problem>` |

---

## Memory Bank Integration

Each incident creates a work item:

```
.memory-bank/work-items/broken-arrow-<ID>/
  problem.md
  report.md
  agents.md
  runs/<RUN_ID>/
    verification.md
    findings/
```

---

## Structured Logging Events

| Event | Level | When |
|---|---|---|
| `broken_arrow_launched` | INFO | Swarm launched |
| `broken_arrow_agent_started` | INFO | Each agent session started |
| `broken_arrow_finding_posted` | DEBUG | Each finding written |
| `broken_arrow_agent_stopped` | INFO | Each agent session stopped |
| `broken_arrow_report_ready` | INFO | Final report produced |
| `broken_arrow_timeout` | WARN | Max time reached without report |
| `broken_arrow_agent_error` | ERROR | Agent session errored |

---

## Non-Goals

- Does NOT automatically apply fixes (diagnosis only)
- Does NOT replace normal Axiom workflow for planned work
- Does NOT guarantee a root cause (best-effort diagnosis)
- Does NOT support distributed repos (single repo per incident in v1)
