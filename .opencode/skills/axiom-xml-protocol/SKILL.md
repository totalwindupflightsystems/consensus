---
name: axiom-xml-protocol
description: Portable XML envelope rules for Axiom /commands (no repo specs dependency).
version: "2.0"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: protocol
  core: true
---

# Axiom XML Protocol (Portable)

Use this skill whenever a response must be machine-consumed by a Axiom `/command`.

## Core Rules

- Put the XML envelope in the FINAL assistant message.
- Emit exactly one envelope of the expected type.
- Required/optional tags are defined by the repo-local command registry: `.axiom/command-registry.yaml`.
- Do not invent tool outputs. If you did not run it, mark it as not run.

## Envelope Types

- Step/plan commands: `<axiom>...</axiom>`
- Verify commands: `<codeops_verify>...</codeops_verify>`

## Full `<axiom>` Envelope

```xml
<axiom>
  <run>
    <run_id>2026-02-03T12:34:56Z-xyz</run_id>
    <jira_key>ABC-123</jira_key>
    <repo>org/repo</repo>
    <phase_id>phase-1</phase_id>
    <task_id>task-2</task_id>
    <step_id>step-7</step_id>
  </run>
  <command>/axiom-step</command>
  <status>ok</status> <!-- ok|fail|blocked -->
  <confidence>72</confidence> <!-- 0-100 integer; see specs/11 -->
  <summary>One sentence describing the outcome.</summary>
  <detailed_summary>More detail for logs/Jira/PR comments.</detailed_summary>
  <related_specs>specs/00-PRD.md; specs/03-Plan-Schema.md</related_specs>
  <memory_updates>.memory-bank/work-items/ABC-123/verification.md</memory_updates>

  <evidence>
    <files_changed>path1; path2</files_changed>
    <commands_ran>cmd1; cmd2</commands_ran>
    <tests_ran>test_cmd1; test_cmd2</tests_ran>
    <checks>
      <passed>check_a; check_b</passed>
      <failed></failed>
    </checks>
  </evidence>

  <diagnostics>
    <errors></errors>
    <warnings></warnings>
    <blockers></blockers>
  </diagnostics>

  <review>
    <risk>low</risk> <!-- low|medium|high -->
    <assumptions></assumptions>
    <questions_for_human></questions_for_human>
  </review>

  <modify_plan>false</modify_plan>
  <retry>false</retry>

  <inject>
    <!-- Only present when modify_plan=true; see Injection Semantics below -->
  </inject>

  <outputs>
    <jira_comment></jira_comment>
    <pr_comment></pr_comment>
  </outputs>
</axiom>
```

Notes:
- `<run>` fields (`run_id`, `jira_key`, `repo`, `phase_id`, `task_id`, `step_id`) allow correlation across logs, Jira, PR, and Memory Bank (trace link L10).
- `checks` names should be stable identifiers used by the repo (e.g., `pre_commit`, `unit_tests`).
- If no checks were run, keep `<passed>` and `<failed>` empty.
- `<confidence>` is 0-100 integer. Full signal breakdown goes in the evidence bundle, not the envelope.
- `<review>` fields (`risk`, `assumptions`, `questions_for_human`) reduce reviewer load and help escalation.
- `<outputs>` contains optional pre-formatted text for Jira/PR comments.

## `<codeops_verify>` Envelope

```xml
<codeops_verify>
  <command>/axiom-verify</command>
  <pass>true</pass>
  <score>0.86</score>
  <notes>Spec aligns; tests passed.</notes>
</codeops_verify>
```

| Tag | Type | Required | Description |
|---|---|---|---|
| `<command>` | string | Yes | The verify command that was run |
| `<pass>` | boolean | Yes | `true` if verification passed, `false` otherwise |
| `<score>` | float (0.00-1.00) | Yes | Verifier's confidence score |
| `<notes>` | string | Yes | Brief explanation of the verification result |

## Tag Semantics

### `<modify_plan>` and `<inject>`

When `<modify_plan>` is `true`, the agent requests the runner to inject new steps into the plan. The `<inject>` block MUST be present.

```xml
<inject>
  <step>
    <id>step-injected-1</id>
    <title>Add missing unit test for edge case</title>
    <command>/axiom-step</command>
    <inputs>
      <expected_files>tests/auth/test_edge_case.py</expected_files>
    </inputs>
    <spec_ref>specs/auth.md#req-auth-005</spec_ref>
  </step>
</inject>
```

Rules:
- Each `<step>` MUST have `<id>` and `<title>`.
- `<command>` defaults to `/axiom-step` if omitted.
- `<inputs>` is optional; passed to the command when the step executes.
- `<spec_ref>` is optional but SHOULD be included for traceability.
- If `<modify_plan>` is `true` but `<inject>` is missing/malformed, the runner treats it as `modify_plan=false`.

### `<retry>` Semantics

When `<retry>` is `true`, the agent self-assesses that the step should be re-run. This counts against the step-level retry budget. If the budget is exhausted, the runner ignores `<retry>=true` and escalates.

`<retry>` and `<modify_plan>` are not mutually exclusive. If both are `true`, the runner applies the injection first, then re-runs the current step.

## Post-Step Routing Tags

After a step completes, agents may emit optional XML tags that trigger dynamic routing. These tags enable delegation to other agents or fire events for asynchronous side effects.

### `<delegate>` Tag (REQ-XML-DELEGATE-001)

Requests the runner to pause the current plan cursor and invoke another agent before advancing.

```xml
<delegate>
  <target>@security-review-axiom</target>
  <reason>Auth changes detected; need security review before proceeding</reason>
  <context>
    <files_changed>src/auth/middleware.py, src/auth/tokens.py</files_changed>
    <spec_refs>specs/32-Security-Hardening-Roadmap.md</spec_refs>
  </context>
  <return_to>step-4</return_to>
</delegate>
```

| Tag | Required | Description |
|---|---|---|
| `<target>` | Yes | Valid agent handle from the agent roster. Invalid targets fail the step. |
| `<reason>` | Yes | Human-readable explanation of why delegation is needed. |
| `<context>` | No | Structured context to pass to the delegated agent. Free-form child elements. |
| `<return_to>` | No | Step ID to resume after delegate completes. If omitted, resume at the next step. |

Rules:
- Runner MUST pause the plan cursor and invoke the delegated agent via `/axiom-delegate`.
- Delegates can delegate further (call stack model). Max delegation depth: 5 (configurable).
- Exceeding max depth fails the step with `delegation_depth_exceeded`.
- Delegation is recorded in `checkpoint.yaml` for crash recovery.
- On delegation failure, the delegating step is marked failed and follows normal escalation.

### `<events>` Tag (REQ-XML-EVENTS-001)

Fires named events for asynchronous side effects after step completion.

```xml
<events>
  <event>
    <name>spec_changed</name>
    <payload>
      <spec_path>specs/32-Security-Hardening-Roadmap.md</spec_path>
      <change_type>requirement_added</change_type>
    </payload>
  </event>
  <event>
    <name>api_surface_changed</name>
    <payload>
      <endpoint>/api/v1/auth/refresh</endpoint>
      <change_type>added</change_type>
    </payload>
  </event>
</events>
```

| Tag | Required | Description |
|---|---|---|
| `<event>` | Yes (at least one) | Container for a single event. |
| `<name>` | Yes | Event name string. Matched against handlers in `command-registry.yaml`. |
| `<payload>` | No | Structured payload to pass to the event handler. Free-form child elements. |

Rules:
- Events are fire-and-forget -- they do NOT block step completion or plan cursor advancement.
- Each event is dispatched independently; failure of one handler does not prevent dispatch of others.
- Unhandled events (no matching handler) are logged as INFO and ignored.
- Event payloads are passed through to handlers as-is (no transformation).

## XML Extraction (How the Runner Finds Your Envelope)

1. Strip markdown code fences from the agent's raw text.
2. Search for the expected envelope type (`<axiom>` for step commands, `<codeops_verify>` for verify commands).
3. If multiple matches, take the LAST one (defensive against agent self-correction).
4. If the expected type is not found, try the other type as fallback (with WARN log).
5. If neither found, trigger v2 variant recovery (see `specs/28-V2-Variant-Mechanism.md`).
6. Parse the extracted XML into a flat dict of `tag -> content` (one level of nesting for `<run>`, `<evidence>`, etc.).
7. Validate required tags per the command registry. Missing required tags trigger v2 variant recovery.

## Quick Reference: Tag Inventory

| Tag | Required | Notes |
|---|---|---|
| `<run>` (with children) | Yes | Correlation block: `run_id`, `jira_key`, `repo`, `phase_id`, `task_id`, `step_id` |
| `<command>` | Yes | The command being executed |
| `<status>` | Yes | `ok`, `fail`, or `blocked` |
| `<confidence>` | Yes | Integer 0-100 |
| `<summary>` | Yes | One sentence |
| `<detailed_summary>` | Optional | Multi-sentence for logs/Jira/PR |
| `<related_specs>` | Optional | Semicolon-separated spec paths |
| `<memory_updates>` | Optional | Paths to memory bank artifacts written |
| `<evidence>` | Yes | Contains `files_changed`, `commands_ran`, `tests_ran`, `checks` |
| `<diagnostics>` | Optional | Contains `errors`, `warnings`, `blockers` |
| `<review>` | Optional | Contains `risk`, `assumptions`, `questions_for_human` |
| `<modify_plan>` | Yes | Boolean (`true`/`false`) |
| `<retry>` | Yes | Boolean (`true`/`false`) |
| `<inject>` | Conditional | Required when `modify_plan=true` |
| `<outputs>` | Optional | Contains `jira_comment`, `pr_comment` |
| `<delegate>` | Optional | Post-step routing: delegation to another agent |
| `<events>` | Optional | Post-step routing: async event dispatch |
