---
name: implementation-plan-history
description: Portable rules for current vs historical plans, run snapshots, and comment queue handling.
version: "1.0"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Implementation Plan History (Portable)

This skill defines how to store current and historical implementation plans and how to handle mid-flight comments safely.

## Storage Model

Work item folder:
```
.memory-bank/work-items/<WORK_ITEM_ID>/
```

Run folder:
```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/
```

## Current vs Historical

- Current plan (mutable):
  - `.memory-bank/work-items/<WORK_ITEM_ID>/plan.md`
  - `.memory-bank/work-items/<WORK_ITEM_ID>/plan.yaml`

- Historical plan snapshots (immutable per run):
  - `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/plan.md`
  - `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/plan.yaml`

Rule: once a run completes, the run snapshot files MUST NOT be edited. Corrections happen in a new run.

## Comment Queue (Do Not Mutate Plan Mid-Flight)

Problem:
- New comments can arrive on Jira/PR during a run.
- The active plan MUST NOT change mid-flight.
- Comments MUST NOT be lost.

Solution:
- Queue comments in `checkpoint.yaml` under `queued_comments`.

Schema:
```yaml
queued_comments:
  - id: "comment-123"
    author: "user@example.com"
    timestamp: "2026-02-06T10:00:00Z"
    body: "Please handle edge case X"
    source: "jira"           # jira | github
    processed: false
```

Processing trigger (safe checkpoints):
- Process queued comments at the end of each phase (or other explicit safe checkpoint).

Processing rules:
- If comment requires plan changes beyond current run scope:
  - finish current phase safely
  - start a new run with an updated plan
- Mark processed comments `processed: true` in `checkpoint.yaml`.

## Specs vs Plans Boundary

Definitions:
- Specs define WHAT (durable contract).
- Plans define HOW (execution strategy for a single work item).

Rules:
- Plans do not override specs.
- Plans do not create durable rules.
- If a plan includes a spec-worthy decision, inject a spec update step (specwriter) and link it.

## Plan Trace Expectations

Each step in `plan.yaml` SHOULD include:
- `spec_ref`: which contract requirement it implements
- `verification`: how the step is verified

When the work item is Jira-sourced:
- The `work_item.jira_key` field in `plan.yaml` MUST be set (not null).
- All `axiom:trace` markers in plan artifacts MUST include `jira_ref=<KEY>`.
- The comment queue (`queued_comments`) tracks Jira comments alongside GitHub comments — the `source` field distinguishes them.

References:
- `specs/05-Jira-Integration.md` (ticket-as-work-unit, comment format)
- `specs/21-Traceability-Doctrine.md#external-reference-fields` (jira_ref in trace markers)
- `.opencode/skills/jira-workflow-axiom/SKILL.md` (Jira operating model)
