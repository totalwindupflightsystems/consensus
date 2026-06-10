---
name: axiom-collaborative-intent-resolution
description: Portable conversational intake protocol for resolving raw user intent into executable plans, questions, inline actions, or blocked status via a structured loop with OpenCode.
version: "1.0"
synopsis: |
  Defines the conversational intake loop that replaces rigid meta-planning for intent-based runs.
  Raw intent text reaches OpenCode via `/axiom-intake` with enriched repo context. OpenCode returns
  structured XML (plan, questions, inline actions, or blocked reason). Axiom drives the loop until
  resolved. Covers context enrichment, XML response types, operating-mode question handling, ephemeral
  plans, graceful degradation (no silent 0-step), and phased rollout gates.
when-to-use: |
  Load this skill when implementing or debugging the collaborative intake loop, building the
  `/axiom-intake` command template, handling intent-based runs, designing question/answer flows
  across operating modes, or creating ephemeral plans from inline actions.
tags:
  vertical: [planning, coding]
  category: planning
  core: false
---

# Collaborative Intent Resolution (Portable)

This skill defines the conversational intake protocol for resolving raw user intent into actionable work.

Source spec: `specs/59-Collaborative-Intent-Resolution.md`

---

## Core Concept

The collaborative intake loop replaces the rigid meta-plan → plan pipeline for intent-based runs:

1. Raw intent text goes to OpenCode via `/axiom-intake` with enriched repo context.
2. OpenCode returns structured XML: a plan, questions, inline actions, or a blocked reason.
3. Axiom drives a conversational loop: relay questions per operating mode, re-send with answers, repeat until resolved.
4. Existing `plan.yaml`-based execution is preserved unchanged for work-item-id runs.

**Key invariant**: Every run produces steps, questions, or an explicit blocked status — never silent 0-step completion.

---

## Intent Intake Protocol

### Routing Decision

```
IF run has --intent AND no existing plan.yaml:
  → Collaborative intake path (/axiom-intake)
ELSE IF run has existing plan.yaml:
  → Existing plan execution path (unchanged)
ELSE IF run has --work-item with existing plan:
  → Existing execution path (unchanged)
```

Rules:
- The orchestrator MUST detect intent-mode runs (intent provided, no existing plan.yaml).
- For intent-mode runs, skip meta-planning and invoke `/axiom-intake` directly.
- Raw intent text MUST be passed to OpenCode unmodified (no slugification, no summarization).
- `work_item_id` MAY be derived from the intent (slugified) or provided explicitly.

---

## Context Enrichment

Before sending intent to OpenCode, Axiom MUST enrich the request with repo context.

### Required Fields

| Field | Type | Source |
|-------|------|--------|
| `repo_name` | string | Directory name of `--repo` path |
| `repo_path` | string | Absolute path to repo |
| `primary_languages` | list[string] | File extension analysis (top 3) |
| `available_commands` | list[string] | `.opencode/commands/*.md` filenames |
| `memory_bank_summary` | object | `.memory-bank/_index.md` parse: work item count, recent activity |
| `specs_summary` | list[string] | `specs/README.md` parse: list of spec titles |
| `git_branch` | string | Current git branch |
| `git_clean` | boolean | Whether worktree is clean |

### Optional Fields

| Field | Type | Source |
|-------|------|--------|
| `existing_work_items` | list[string] | `.memory-bank/work-items/` directory listing |
| `config_summary` | object | `.axiom/axiom.config.yaml` key fields |

**Performance constraint**: Context enrichment MUST complete in <2 seconds. Use cached values; avoid expensive operations (no `git log`, no full file tree traversal).

---

## Conversational Loop Engine

```
FUNCTION run_collaborative_intake(intent, context, mode, session):
  iteration = 0
  max_iterations = config.intake.max_iterations  # default: 10
  timeout_per_iteration = config.intake.timeout_seconds  # default: 120

  WHILE iteration < max_iterations:
    iteration += 1
    payload = build_intake_payload(intent, context, answers=previous_answers)
    response = send_command(session, "/axiom-intake", payload, timeout=timeout_per_iteration)

    IF response is None OR timed out:
      RETURN IntakeResult(type="blocked", reason="OpenCode did not respond")

    parsed = extract_xml_envelope(response)

    IF parsed contains <plan>:
      RETURN IntakeResult(type="plan", plan=parse_plan(parsed))
    ELSE IF parsed contains <inline_actions>:
      RETURN IntakeResult(type="inline_actions", actions=parse_actions(parsed))
    ELSE IF parsed contains <questions>:
      answers = relay_questions(parsed.questions, mode)
      IF answers is None:
        RETURN IntakeResult(type="blocked", reason="questions_pending")
      previous_answers = answers
      CONTINUE
    ELSE IF parsed contains <blocked_reason>:
      RETURN IntakeResult(type="blocked", reason=parsed.blocked_reason)
    ELSE:
      RETURN IntakeResult(type="blocked", reason="unparseable_response")

  RETURN IntakeResult(type="blocked", reason="max_iterations_exceeded")
```

### Invariants

- Loop MUST NOT exceed `max_iterations` (configurable, default 10).
- Each iteration MUST have a timeout (configurable, default 120 seconds).
- Every exit path MUST produce an `IntakeResult` — never `None` or silent completion.
- Loop MUST emit structured log events at each iteration.

### Session Management

- Reuse the same OpenCode session across iterations to preserve conversation context.
- If context window risks overflow, create a fresh session with a summary of prior iterations.
- Session reuse is default; fresh session is a fallback triggered by context overflow errors.

---

## XML Response Types

All responses are inside a `<axiom>` envelope containing an `<intent_resolution>` compound tag.

### 1. Plan Response

OpenCode has enough information to produce a plan:

```xml
<intent_resolution>
  <plan>
    <work_item_id>build-react-project-01</work_item_id>
    <title>Build React Project</title>
    <phases>
      <phase id="phase-1" title="Setup">
        <task id="task-1-1" title="Initialize project">
          <step id="step-1-1-1">
            <objective>Create React app with TypeScript template</objective>
            <actions>npx create-react-app my-app --template typescript</actions>
            <verification>Directory my-app/ exists with package.json</verification>
          </step>
        </task>
      </phase>
    </phases>
  </plan>
</intent_resolution>
```

### 2. Questions Response

OpenCode needs clarification:

```xml
<intent_resolution>
  <questions>
    <question id="q1" required="true">
      <text>What programming language?</text>
      <options>
        <option key="python">Python</option>
        <option key="typescript">TypeScript</option>
      </options>
    </question>
    <question id="q2" required="false">
      <text>Should the project include a web UI?</text>
    </question>
  </questions>
</intent_resolution>
```

### 3. Inline Actions Response

Intent is simple enough for immediate execution without a formal plan:

```xml
<intent_resolution>
  <inline_actions>
    <action id="a1" type="file_create">
      <description>Create README.md</description>
      <target>README.md</target>
    </action>
    <action id="a2" type="command">
      <description>Initialize git repository</description>
      <command>git init</command>
    </action>
  </inline_actions>
</intent_resolution>
```

### 4. Blocked Response

OpenCode cannot resolve the intent:

```xml
<intent_resolution>
  <blocked_reason>
    <reason>vibe-bench.md not found in repository</reason>
    <attempted>Searched repo root, docs/, specs/, .memory-bank/</attempted>
    <suggestions>
      <suggestion>Provide the full path to vibe-bench.md</suggestion>
      <suggestion>Create vibe-bench.md first, then re-run</suggestion>
    </suggestions>
  </blocked_reason>
</intent_resolution>
```

### Parsing Rules

- `<intent_resolution>` MUST contain exactly one of: `<plan>`, `<questions>`, `<inline_actions>`, or `<blocked_reason>`.
- If multiple present, prefer: `<plan>` > `<inline_actions>` > `<questions>` > `<blocked_reason>`.
- If `<intent_resolution>` is missing, treat as unparseable and apply v2 variant retry.
- Questions with `required="true"` MUST be answered before the loop can proceed.
- Unknown sub-tags: log WARN and ignore.

### Status Tag Values

- Plan and inline_actions: `<status>ok</status>`.
- Questions and blocked_reason: `<status>blocked</status>`.
- `/axiom-intake` restricts `<status>` to `ok|blocked` only. `fail` is not valid for this command.

---

## Operating Mode Question Handling

| Mode | Question Behavior | Answer Source |
|------|-------------------|---------------|
| **Local CLI** | Display questions on stdout with numbered options; prompt via stdin | Human operator at terminal |
| **Local Automated** | Return questions in run result with `status=blocked`; wait for answers via `POST /api/v1/runs/{run_id}/answers` | Human operator via API/UI |
| **Full Automated** | Re-send to OpenCode with "Decide autonomously; operator unavailable." If still questions after 2 retries, escalate to `status=blocked` | OpenCode (autonomous) |

### Full-Auto Escalation Sequence

1. First question receipt: re-send with "You must decide autonomously based on available context."
2. Second question receipt: re-send with "Make your best judgment. If you truly cannot proceed, return `<blocked_reason>`."
3. Third question receipt: escalate to `status=blocked` with questions surfaced in diagnostics.

---

## Ephemeral Plans from Inline Actions

When OpenCode returns `<inline_actions>`, Axiom creates an ephemeral in-memory plan:

- Ephemeral plans are executed through the normal step execution path.
- Ephemeral plans are NOT persisted to disk (no `plan.yaml` file created).
- Evidence from ephemeral plan execution MUST still be captured in the run folder.
- Checkpoint/resume is NOT supported for ephemeral plans (they are re-created on retry).

---

## Graceful Degradation

### No Silent 0-Step Completion

Every run MUST produce one of:
- Executed steps (`steps_completed > 0`)
- Pending questions (`status=blocked, blocked_reason=questions_pending`)
- Explicit blocked status with diagnostics

Silent 0-step completion (`status=completed, steps_completed=0`) is a product bug.

### Actionable Diagnostics

When a run is blocked, the `RunResult` MUST include:
- `diagnostics`: what was attempted and why it failed.
- `suggested_actions`: concrete next steps the user can take.

| Failure Type | Diagnostics | Suggested Actions |
|-------------|-------------|-------------------|
| OpenCode unavailable | "OpenCode server not reachable at {url}" | "Start OpenCode: `opencode serve --port 4096`" |
| Intent too vague | "OpenCode returned questions after {n} iterations" | "Provide more detail" |
| Questions pending | "Questions require human input" | "Answer questions and re-run" |
| Plan generation failed | "OpenCode returned blocked_reason: {reason}" | "Create plan.yaml manually or refine intent" |
| Max iterations exceeded | "Conversational loop exceeded {max} iterations" | "Simplify intent or provide explicit plan" |
| XML parse failure | "Could not parse OpenCode response as XML" | "Check OpenCode server logs; retry" |

---

## Configuration

```yaml
intake:
  max_iterations: 10        # Maximum conversational loop iterations
  timeout_seconds: 120      # Per-iteration timeout for OpenCode response
  enrichment_timeout: 2     # Maximum seconds for context enrichment
  auto_answer_retries: 2    # Full-auto mode: retries before escalating to blocked
```

All fields have defaults and are optional.

---

## Lifecycle Integration

| Intake Phase | `lifecycle_stage` | `workflow_state` | `run_status` |
|-------------|-------------------|------------------|--------------|
| Context enrichment | `normalized` | `intake` | `in_progress` |
| First `/axiom-intake` send | `planning` | `study` | `in_progress` |
| Question/answer loop | `planning` | `study` | `in_progress` |
| Plan received | `planning` → `executing` | `plan` → `implement` | `in_progress` |
| Inline actions received | `executing` | `implement` | `in_progress` |
| Blocked | `planning` | `intake` | `blocked` |

---

## Structured Log Events

| Event | When | Key Fields |
|-------|------|------------|
| `intake_started` | Loop begins | `work_item_id`, `intent` (truncated 200 chars), `mode` |
| `intake_iteration` | Each loop iteration | `iteration`, `response_type`, `question_count` |
| `intake_questions_relayed` | Questions sent to user/auto | `question_ids`, `mode`, `relay_method` |
| `intake_answers_received` | Answers received | `question_ids`, `source` (human/auto) |
| `intake_resolved` | Loop completes successfully | `resolution_type` (plan/inline_actions), `iterations_used` |
| `intake_blocked` | Loop ends in blocked | `reason`, `iterations_used`, `diagnostics` |

---

## Phased Rollout

| Phase | Scope | Gate |
|-------|-------|------|
| **P1** | `plan` or `blocked_reason` only — fixes silent 0-step completion | Tier 3 runtime proof; regression proof for existing plan.yaml path |
| **P2** | Add `<questions>` support for CLI mode only | Contract tests for question parsing; CLI interaction test |
| **P3** | Add API question/answer flow (`/answers` endpoint) | OpenAPI updated; concurrent submission tests |
| **P4** | Add `<inline_actions>` with security controls | Security review pass; action allowlist; negative tests |

Each phase has an independent rollback point.

---

## Capability Surface Awareness

The `/axiom-intake` command template MUST reference the `axiom-capability-surface` skill so OpenCode can discover the full menu of tools, workflows, and agents when resolving complex or vague intents.

---

## Backward Compatibility

- Existing `plan.yaml` runs: unchanged.
- `--work-item` with existing plan: unchanged.
- `/axiom-meta-plan`: remains available for work-item-id runs.
- `/axiom-step` execution: unchanged. Plans from intake use the same step loop.

---

## Open Decisions

- **OD-CIR-001**: Repo file tree in context — summary-only for v1; revisit if OpenCode needs more.
- **OD-CIR-002**: Plan + questions in same response — treat as "plan with caveats" for v1.
