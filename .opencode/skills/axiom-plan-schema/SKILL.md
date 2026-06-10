---
name: axiom-plan-schema
description: Portable plan.yaml schema summary for Axiom planning/execution (no repo specs dependency).
version: "1.1"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Axiom Plan Schema (Portable)

This skill defines the v1 shape of `.memory-bank/work-items/<WORK_ITEM_ID>/plan.yaml`.

## Key Concepts

- Phase: major chunk of work
- Task: logical unit of work (may contain `subtasks`)
- Subtask: nested task shape (max depth: task -> subtask)
- Step: atomic scheduling unit (executed one at a time)
- Verification: checks run after steps/tasks/phases

## Jira Linking in plan.yaml (REQUIRED when Jira-sourced)

The `work_item.jira_key` field connects the executable plan to its Jira ticket. This field
is the single source of truth that downstream agents use to include `jira_ref=` in trace
markers and to post progress/evidence comments to the correct Jira ticket.

Rules:
- When the work item originates from a Jira ticket, `jira_key` MUST be set to the Jira key (e.g., `PROJ-123`).
- When the work item is NOT Jira-sourced, `jira_key` MUST be `null` (not omitted).
- If a non-Jira work item is later linked to a Jira ticket, update `jira_key` in the plan.
- The `jira_key` value MUST match the `jira_ref=` value used in `axiom:trace` markers.
- Downstream agents (dev, QA, docs, ops) read `jira_key` from the plan to know where to post evidence — do not force them to re-read the ticket.

References:
- `specs/05-Jira-Integration.md` (ticket-as-work-unit)
- `specs/21-Traceability-Doctrine.md#external-reference-fields` (jira_ref in trace markers)
- `.opencode/skills/jira-workflow-axiom/SKILL.md` (Jira operating model)

## Plan YAML Shape (v1)

```yaml
version: 1
work_item:
  id: "work-item-id"
  jira_key: null              # REQUIRED field — set to Jira key (e.g., "PROJ-123") when Jira-sourced; null otherwise
  repo: "org/repo"
  pr_branch: "axiom/work-item-id"
meta_planning:
  notes_md_path: ".memory-bank/work-items/work-item-id/meta-planning.md"
plan:
  phases:
    - id: "phase-1"
      title: "Phase title"
      tasks:
        - id: "task-1"
          title: "Task title"
          subtasks: []
          steps:
            - id: "step-1"
              title: "Step title"
              command: "/axiom-step"
              spec_ref: "specs/<repo-spec>.md#anchor" # or "N/A — <reason>"
              inputs: {}
              verification:
                - type: "qa_agent"
                  command: "/axiom-verify"
                  required: true
                  timeout_seconds: 300
                  inputs: {}
          verification:
            - type: "aggregate"
              command: "/axiom-verify"
              required: true
              timeout_seconds: 600
execution:
  status: "pending" # pending|in_progress|blocked|failed|completed
  cursor:
    phase_id: "phase-1"
    task_id: "task-1"
    step_id: "step-1"
```

## Step Schema Rules

- `id`, `title`, `command` are REQUIRED.
- `spec_ref` is RECOMMENDED and should link to the target repo's own contract/specs.
- `inputs` is OPTIONAL; use for expected files, parameters, etc.
- `test_value` is RECOMMENDED for steps that add/modify tests (see below).

## Test Value Field (RECOMMENDED for test steps)

Per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, steps that add or modify tests SHOULD include
a `test_value` field that documents what real value the test produces:

```yaml
steps:
  - id: "step-1"
    title: "Add crash recovery integration test"
    command: "/axiom-step"
    spec_ref: "specs/24-Runtime-State-Persistence.md#checkpoint-resume"
    test_value:
      verifies: "System resumes from checkpoint after mid-run crash"
      real_code_path: "orchestrator.run() → checkpoint.write() → checkpoint.read()"
      regression_caught: "Crash during run loses all progress"
      anti_patterns_checked: true
```

This field helps the verifier and adversarial agents assess whether the test produces
real value or is green theater. Plans without `test_value` on test steps will receive
a lower confidence score during gap analysis.

## Verification Object

```yaml
- type: "qa_agent"          # qa_agent|spec_agent|security_agent|aggregate|command
  command: "/axiom-verify" # or a shell command when type=command
  required: true
  timeout_seconds: 300
  inputs: {}
```

Rules:
- Multiple verifications allowed; run in order.
- Any `required: true` failure blocks progress.
