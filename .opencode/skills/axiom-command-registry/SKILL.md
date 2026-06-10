---
name: axiom-command-registry
description: Portable command registry contract for Axiom — YAML schema, command definitions, tag prompts, validation rules, v2 variant config, event handlers, and the agent checklist for adding new commands.
version: "1.0"
synopsis: |
  Defines the `.axiom/command-registry.yaml` schema: defaults (required/optional tags, v2 variant config),
  per-command definitions (required tags, tag prompts, validation), event handler routing, and the agent
  checklist for registering new commands. Covers all 10 built-in commands and their XML tag contracts.
when-to-use: |
  Load this skill when adding a new /command, modifying command tag requirements, writing tag prompts,
  configuring v2 variant retry behavior, adding event handlers, or validating command registry entries.
tags:
  vertical: [coding, planning]
  category: tooling
  core: false
---

# Axiom Command Registry (Portable)

This skill defines the command registry contract for Axiom.

Source spec: `specs/13-Command-Registry.md`

---

## Overview

| Aspect | Detail |
|---|---|
| **Location** | `.axiom/command-registry.yaml` (repo-local, versioned) |
| **Purpose** | Maps each `/command` to required/optional XML tags, validation rules, micro-prompts, and v2 variant config |
| **Fail-closed rule** | Missing required tag = fail; triggers v2 variant retry |
| **Scope** | Only `/command` outputs consumed by automation; free-form subagent reports are NOT validated |
| **Loading** | Loaded once at run startup; malformed entries cause startup error |

---

## Registry Shape (v1)

```yaml
version: 1
defaults:
  required_tags: [...]
  optional_tags: [...]
  v2_variant: { enabled, prompt_template, max_response_tokens, timeout_seconds }
commands:
  /command-name:
    description: "..."
    agent: "agent-handle"
    required_tags: [...]
    optional_tags: [...]
    tag_prompts: { tag: "micro-prompt" }
    v2_variant: { enabled, max_response_tokens }
    validation: { tag: { type, min, max, values, pattern } }
validation:
  # Global validation rules
event_handlers:
  event_name:
    handler: "@agent-handle"
    command: "/command-name"
    description: "..."
```

---

## Default Required Tags (all commands inherit)

```yaml
defaults:
  required_tags:
    - run.run_id
    - run.work_item_id
    - run.intake_source      # jira_event | jira_message | local_cli | local_api | local_file
    - run.repo
    - command
    - status
    - confidence
    - summary
    - detailed_summary
    - related_specs
    - memory_updates
    - modify_plan
    - retry
  optional_tags:
    - run.jira_key            # Set only for Jira-triggered runs
    - evidence.files_changed
    - evidence.commands_ran
    - evidence.tests_ran
    - evidence.checks.passed
    - evidence.checks.failed
    - diagnostics.errors
    - diagnostics.warnings
    - diagnostics.blockers
    - review.risk
    - review.assumptions
    - review.questions_for_human
    - outputs.jira_comment
    - outputs.pr_comment
```

---

## Command Summary Table

| Command | Agent | Required Tags | Purpose |
|---|---|---|---|
| `/axiom-plan` | `tower-axiom` | `modify_plan`, `summary`, `detailed_summary`, `related_specs`, `memory_updates` | Produce implementation plan |
| `/axiom-step` | `tower-axiom` | `status`, `confidence`, `summary`, `detailed_summary`, `evidence.*`, `modify_plan`, `retry` | Execute one plan step |
| `/axiom-verify` | `spec-verifier-axiom` | `status`, `confidence`, `summary`, `modify_plan` | Verify changes against specs |
| `/axiom-meta-plan` | `pm-axiom` | `status`, `confidence`, `summary`, `detailed_summary`, `related_specs`, `memory_updates`, `modify_plan` | Meta-planning |
| `/axiom-todo` | `pm-axiom` | `status`, `summary`, `memory_updates` | Update project TODO |
| `/axiom-implementation-plans` | `pm-axiom` | `status`, `summary`, `memory_updates` | Update implementation plans |
| `/memory-bank-update` | `memory-bank-axiom` | `status`, `summary`, `memory_updates` | Update Memory Bank |
| `/axiom-spec-extract` | `tower-axiom` | `status`, `confidence`, `summary`, `detailed_summary`, `related_specs`, `memory_updates`, `evidence.files_changed` | Reverse-engineer specs |
| `/axiom-intake` | `tower-axiom` | `status`, `confidence`, `summary`, `detailed_summary`, `intent_resolution` | Resolve raw user intent |
| `/axiom-delegate` | _(dynamic)_ | `status`, `confidence`, `summary`, `evidence` | Generic delegation |

---

## Tag Prompts

Micro-prompts injected into the agent's system prompt before command execution. They tell the model what each tag should contain.

### How Tag Prompts Are Used
1. Runner reads command's `tag_prompts` from registry.
2. Assembles "output format" instruction block.
3. Appends to agent's system prompt (or user message prefix).
4. Agent produces correctly formatted XML tags.

### Standard Tag Prompts

| Tag | Prompt |
|---|---|
| `summary` | State what you did in one sentence. Be specific about the outcome. |
| `detailed_summary` | 2-5 sentences: what changed, what was verified, what risks remain. |
| `confidence` | Integer 0-100. Base on evidence: tests passing, spec alignment, remaining ambiguity. |
| `related_specs` | Semicolon-separated list of specs/ files referenced or updated. |
| `memory_updates` | Semicolon-separated list of .memory-bank/ files created or updated. |
| `evidence.checks.passed` | Semicolon-separated names of checks that passed. |
| `evidence.checks.failed` | Semicolon-separated names of checks that failed. Empty if all passed. |
| `modify_plan` | true if you need to inject new steps; false otherwise. |
| `retry` | true if you need another attempt at this step; false otherwise. |

Rules:
- Per-command `tag_prompts` override defaults for the same tag key.
- Tag prompts should be concise (1-2 sentences) and actionable.

---

## V2 Variant Configuration

Configures the v2 variant retry mechanism for missing/invalid XML tags.

```yaml
v2_variant:
  enabled: true
  prompt_template: |
    The previous execution of {command} produced a response but the following
    required XML tags were missing: {missing_tags}.
    ...
    Produce ONLY the missing XML tags listed above with their content.
  max_response_tokens: 2000
  timeout_seconds: 60
```

### Inheritance Rules
- No `v2_variant` block → inherits from `defaults.v2_variant`.
- `v2_variant.enabled: false` → skip v2 retry; go directly to direct model call.
- Per-command fields override defaults field-by-field (not wholesale).

---

## Validation Rules

Applied after XML parsing, before downstream processing.

```yaml
validation:
  confidence:
    type: int
    min: 0
    max: 100
  status:
    type: enum
    values: ["ok", "fail", "blocked"]
  review.risk:
    type: enum
    values: ["low", "medium", "high"]
  modify_plan:
    type: boolean
  retry:
    type: boolean
  evidence.checks.passed:
    type: string
    pattern: "^([a-z_]+;)*[a-z_]*$"
  evidence.checks.failed:
    type: string
    pattern: "^([a-z_]+;)*[a-z_]*$"
```

### Validation Field Types

| Field | Type | Description |
|---|---|---|
| `type` | string | `int`, `float`, `string`, `boolean`, `enum` |
| `min` / `max` | number | For `int` and `float` |
| `values` | array | Allowed values for `enum` |
| `pattern` | string | Regex (full-match) for `string` |

Rules:
- Required tag present but fails validation → triggers v2 variant (same as missing).
- Optional tag fails validation → warning logged, processing continues.

---

## Event Handlers

Maps event names (from `<events>` XML tags) to agent/command pairs.

```yaml
event_handlers:
  spec_changed:
    handler: "@prompt-mirror-axiom"
    command: "/axiom-prompt-mirror-check"
  api_surface_changed:
    handler: "@prompt-mirror-axiom"
    command: "/axiom-prompt-mirror-check"
  security_relevant_change:
    handler: "@security-review-axiom"
    command: "/axiom-security-review"
```

Rules:
- `handler` must be a valid agent handle; invalid → WARN at load time.
- `command` must start with `/` and resolve to installed command; invalid → WARN at load time.
- Unknown event names → INFO log, ignored (not an error).
- Handlers invoked asynchronously — do not block current step.
- Handler failure → WARN log; does NOT fail originating step.
- `event_handlers` section is optional; if absent, all events logged as INFO.

---

## Agent Checklist: Adding a New Command

When adding a new `/command`:

- [ ] Add command definition under `commands:` in `.axiom/command-registry.yaml`
- [ ] Define `required_tags` (minimum: `status`, `summary`)
- [ ] Define `optional_tags` for supplementary data
- [ ] Write `tag_prompts` for each required tag (1-2 sentence micro-prompts)
- [ ] Set `v2_variant.enabled: true` (unless recovery is inappropriate)
- [ ] Add command-specific `validation` rules
- [ ] Create fixture files in `.axiom/tests/fixtures/commands/<command>/` (at least `ok.txt`)
- [ ] Add test scenarios: valid response, missing required tag, invalid tag value
- [ ] Update `specs/13-Command-Registry.md` with the command definition
- [ ] Update `specs/README.md` and `specs/_index.md` if new spec introduced

## Agent Checklist: Adding a New Event Handler

- [ ] Add handler entry under `event_handlers:` in `.axiom/command-registry.yaml`
- [ ] Verify `handler` is a valid agent from `specs/22-Agent-Roster-And-Interactions.md`
- [ ] Verify `command` starts with `/` and resolves to an installed command
- [ ] Add unit tests: handler loads, event dispatches correctly, unknown events INFO-logged

---

## Implementation Location

| Component | Path |
|---|---|
| Registry loader | `.axiom/src/axiom/shared/registry/load.py` |
| Registry schema | `.axiom/src/axiom/shared/registry/schema.py` |
| Registry file | `.axiom/command-registry.yaml` |
| Test fixtures | `.axiom/tests/fixtures/commands/` |
