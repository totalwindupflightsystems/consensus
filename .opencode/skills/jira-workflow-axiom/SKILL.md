---
name: jira-workflow-axiom
description: Parent Jira workflow skill for Axiom. Covers issue taxonomy, create/update/comment/transition rules, evidence mirroring, intake pushback protocol, output confidence, overdue escalation, sync command, and repo targeting precedence. Strongly routes Jira operations through @pm-axiom.
version: "2.0"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Jira Workflow

Use this skill when the work involves Jira as a workflow system, not just Jira writing.

axiom:trace work_item=DEX-74 spec=specs/05-Jira-Integration.md,specs/14-Integrations-Jira-GitHub.md plan= test= doc=.memory-bank/explorations/jira-workflow-skill.md prompt=.opencode/skills/jira-workflow-axiom/SKILL.md evidence= commit= jira_ref=DEX-74

## Primary Rule

Default to `@pm-axiom` for Jira operations.

- If Jira state needs to change, prefer calling `@pm-axiom`.
- If a ticket must be created, refined, transitioned, linked, commented on, or closed, prefer calling `@pm-axiom`.
- Other agents SHOULD provide facts, evidence, and desired outcomes to `@pm-axiom` instead of operating Jira directly.
- This keeps Atlassian MCP concentrated in the planning agent, reduces context-window bloat, and improves process consistency.

## Issue Type Hierarchy (required)

Every ticket MUST be linked to its parent level. Work is only traceable when the full chain exists:

```
Initiative → Epic → Task → Subtask
```

**When creating any ticket, always set the parent:**

| Creating | Parent required | Action if missing |
|---|---|---|
| **Subtask** | Task | Fail closed — always require |
| **Task** | Epic | Search for matching epic; create one if none found |
| **Epic** | Initiative | Warn user; create epic anyway (soft requirement) |
| **Initiative** | None | Top-level — no parent |

**Check `jira.hierarchy.default_epics` in config first** — if a default epic exists for the work area, use it. If not, search the project for a matching epic before creating a new one.

See `jira-field-standard-axiom` skill for full hierarchy rules, JQL patterns, and config reference.

## Child Skills

- `jira-ticket-writing-axiom` for issue summaries and descriptions
- `jira-comment-writing-axiom` for progress, blocker, and evidence comments
- `jira-field-standard-axiom` for the cross-workspace custom field contract and admin rollout checklist

Load this parent skill first, then load a child writing skill when ticket text or comment text must be authored.

## Jira Operating Model

Treat Jira as a workflow and audit system, not just a note field.

- Jira holds the durable statement of why the work exists.
- Repo artifacts hold the implementation truth and evidence detail.
- Jira comments mirror progress, blockers, decisions, and evidence summaries.
- Jira status should reflect real work state, not aspiration.

## Repo Scoping (Labels)

Jira projects often serve multiple repos or teams. Axiom uses labels to scope tickets to the correct repo.

- Read `jira.repo_label` from `.axiom/axiom.config.yaml`.
- When creating tickets: ALWAYS include `repo_label` in the `labels` array (alongside any other labels).
- When querying tickets: ALWAYS include `labels = "{repo_label}"` in JQL when `repo_label` is set.
- When picking up an existing ticket that lacks the label: add it.
- When `repo_label` is null: skip label-based scoping; rely on the `Axiom repo` custom field.

Example -- creating a ticket with the Atlassian MCP:

```json
{
  "labels": ["axiom-backend", "axiom", "automated"]
}
```

Example -- JQL query scoped to this repo:

```
project = DEX AND labels = "axiom-backend" AND assignee = currentUser() AND resolution = Unresolved
```

## Canonical Repo Targeting Precedence (Jira Only)

When a Jira ticket is picked up, the target repo is resolved using this normative precedence table:

| Case | `Axiom repo` field state | `jira.repo_override` | Selected repo | Action |
|---|---|---|---|---|
| C1 | Field definition missing in Jira | Absent | N/A | Fail closed; do not enqueue. |
| C2 | Field definition missing in Jira | Present (`org/repo`) | Override | Continue using override. |
| C3 | Field exists but value is empty/null | Absent | N/A | Log WARN; skip ticket. |
| C4 | Field exists but value is empty/null | Present | Override | Use override (log WARN). |
| C5 | Field exists with invalid format | Absent | N/A | Log WARN; skip ticket. |
| C6 | Field exists with invalid format | Present | Override | Use override (log WARN). |
| C7 | Field exists with valid `org/repo` | Absent | Field value | Use field value. |
| C8 | Field exists with valid value = override | Present | Field value | Equivalent; use field value. |
| C9 | Field exists with valid value != override | Present | Field value | **Field value wins**; log WARN with both values. |

Key rule: `jira.repo_override` MUST NOT supersede a valid `Axiom repo` field value.

## Issue Type Selection

| Type | Use when | Avoid when |
|---|---|---|
| Epic | the work spans multiple tickets, teams, repos, or milestones | the work can realistically be completed as one ticket |
| Task / Story / standard issue | the work is a single deliverable with one owner path and one definition of done | the work is too broad and needs multiple independently tracked deliverables |
| Sub-task | the parent issue already defines the outcome and you need smaller assignable execution units | the child item has its own separate outcome, lifecycle, or evidence trail |

## Split Heuristics

Open or split an epic when one or more are true:

- multiple repos or teams are involved
- the work needs separate PRs or separate approvals
- the work spans multiple milestones or releases
- the ticket description becomes a mini-project plan

Create sub-tasks when one or more are true:

- several contributors need parallel execution under one parent outcome
- the parent issue is still one coherent deliverable
- the subtasks do not need their own standalone stakeholder narrative

Prefer a standard task instead of a sub-task when the item needs:

- its own acceptance criteria and evidence trail
- its own lifecycle beyond the parent
- independent prioritization

## When To Create vs Update

- Create a new issue when the work has a distinct outcome, owner path, or timeline.
- Update an existing issue when you are refining scope, acceptance criteria, or implementation details for the same outcome.
- Create linked follow-up issues instead of inflating one ticket when deferred work becomes real scoped work.

## Due Date Tracking

Due dates are a first-class concern in the Axiom Jira workflow. They enable health tracking, overdue detection, and sync reporting.

### Setting Due Dates

- `@pm-axiom` MUST set a `duedate` when creating any Jira ticket.
- If the requester does not specify a due date, estimate based on scope/complexity and note the assumption.
- For sprint-assigned tickets, default to the sprint end date unless a tighter deadline exists.
- For epics, set the due date to the expected completion of the last child ticket.

### Updating Due Dates

- When scope changes materially, re-evaluate and update the due date.
- Always add a comment when changing a due date: `Due date moved from YYYY-MM-DD to YYYY-MM-DD: <reason>`.
- Valid reasons: scope expansion, blocked dependency, reprioritization, estimate correction.
- Do not silently move due dates without a comment.

### Overdue Detection and Escalation (REQ-JIRA-DUEDATE-003)

- The `/axiom-sync-jira` command MUST detect tickets past their due date.
- Overdue tickets with no activity in >48 hours are escalation candidates.
- Priority-aware thresholds: P1/P2 flagged after 24 hours of no activity; P3+ after 48 hours.
- For overdue tickets, `@pm-axiom` SHOULD either:
  - Update the due date with a reason, or
  - Post a blocker comment explaining the delay, or
  - Transition to blocked if genuinely blocked.

### Due Date in the Atlassian MCP

When creating or updating tickets via the Atlassian MCP:

```json
{
  "fields": {
    "duedate": "2026-04-15"
  }
}
```

When using `createJiraIssue`, include `duedate` in `additional_fields`:

```json
{
  "additional_fields": {
    "duedate": "2026-04-15"
  }
}
```

## Intake Confidence Pushback Protocol (REQ-JIRA-PUSHBACK-001)

When a ticket is assigned to Axiom, the system evaluates ticket readiness using intake confidence scoring. If the ticket lacks sufficient information, Axiom **pushes back** -- posting a comment requesting more details before starting work.

### Pushback Decision Flow

1. **Evaluate** intake confidence (4 signals: ticket completeness, AC quality, scope clarity, context availability).
2. **Score >= proceed_threshold (default 50)**: Proceed normally -- start meta-planning.
3. **Score between pushback_threshold (default 30) and proceed_threshold**: Proceed with warnings; post assumptions to Jira.
4. **Score < pushback_threshold**: Check if pushback is enabled and attempts remain.
   - If yes: post pushback comment, set state `waiting_for_info`, wait for ticket update.
   - If no (disabled or attempts exhausted): proceed with warnings.

### Pushback Configuration

| Setting | Default | Description |
|---|---|---|
| `pushback_enabled` | `true` | Master switch. When `false`, always proceed. |
| `pushback_threshold` | `30` | Score below which pushback triggers. |
| `proceed_threshold` | `50` | Score at/above which Axiom proceeds normally. |
| `max_pushback_attempts` | `2` | Max pushback comments per ticket before proceeding anyway. |
| `pushback_cooldown_hours` | `24` | Min hours between re-evaluations (default/low priority). |
| `pushback_cooldown_urgent_hours` | `4` | Min hours between re-evaluations for P1/P2 tickets. |
| `pushback_jira_transition` | `null` | Optional Jira status to transition to during pushback (e.g., "Needs Info"). |

### Pushback Comment Requirements

1. MUST include intake confidence score and band.
2. MUST include numbered list of specific missing information items.
3. MUST include per-signal breakdown table.
4. MUST include actionable guidance (what to add, with examples).
5. MUST include pushback attempt counter (`attempt/max`).
6. MUST include trace references.
7. MUST NOT include secrets, tokens, or sensitive data.
8. MUST NOT reveal internal repo structure (file paths like `specs/`, `.memory-bank/`) or internal tooling details. Signal notes MUST be limited to ticket-derived observations.
9. MUST respect the 32,000 character Jira comment limit.

### Pushback Rules

- Pushback comments are idempotent: key is `axiom:pushback:{work_item_id}:{attempt}` (independent of `run_id`).
- Cooldown respects priority: urgent tickets get faster turnaround.
- Ticket updates trigger re-evaluation (via webhook or polling).
- Pushback state is persisted in `.memory-bank/work-items/<ID>/intake-confidence.yaml`.
- After `max_pushback_attempts`, Axiom proceeds with warnings (never stuck in pushback loop).

## Output Confidence in Jira Comments

When Axiom completes work and creates a PR, the evidence comment includes an **output confidence** breakdown with a **confidence journey** summary:

```
Confidence Journey:
  Intake:    {intake_score}/100 ({intake_band}) -- ticket quality at assignment
  Execution: {exec_score}/100 ({exec_band}) -- confidence during implementation
  Output:    {output_score}/100 ({output_band}) -- delivery quality assessment
```

This gives reviewers a complete picture: was the ticket well-specified? Did confidence grow during execution? How confident is Axiom in the final delivery?

The evidence comment also includes the output confidence signal breakdown:

| Signal | Score | Detail |
|---|---|---|
| Delivery completeness | {value}/100 | {note} |
| Verification depth | {value}/100 | {note} |
| Risk transparency | {value}/100 | {note} |
| Spec & trace alignment | {value}/100 | {note} |
| Change safety | {value}/100 | {note} |

## Jira <-> Repo Sync Command (REQ-JIRA-SYNC-001)

The `/axiom-sync-jira` command provides bidirectional health checking between Jira tickets and repo work items.

Scope:
1. Reads from both Jira (via Atlassian MCP) and the repo (`.memory-bank/work-items/`, `jira-mapping.md`).
2. Detects: overdue tickets, missing due dates, status drift, stale tickets, field hygiene gaps, unmapped tickets, orphaned work items.
3. **Safe remediations** (missing due dates, status drift where repo is ahead, missing fields) MAY be executed directly.
4. **Ambiguous remediations** (status drift where Jira is ahead, stale tickets) MUST be proposed only, not executed without confirmation.
5. Updates `jira-mapping.md` with newly discovered mappings or status changes.
6. Fails closed if Atlassian MCP is unavailable or `jira.project_key` is not configured.

When `jira.repo_label` is set, the sync command MUST scope its health report to labeled tickets only.

## Legacy and Cross-Board Project Support

### Legacy Projects (`jira.legacy_project_keys`)

When a team migrates Jira projects (e.g., `DEX` → `SWDE`), old tickets still exist and need to be searchable. The config tracks legacy projects as an array:

```yaml
jira:
  legacy_project_keys:
    - key: "DEX"
      migrated_date: "2026-04-16"
      search_hint: 'labels = "DEX" OR text ~ "DEX-"'
```

**Agent rules for legacy projects**:
- NEVER create new tickets in legacy projects
- Search legacy projects when looking for historical context, trace markers, or related work
- When referencing old tickets in trace markers, use the full key (e.g., `DEX-123`)
- Update/close existing legacy tickets when superseded (e.g., mark as "Migrated to SWDE-456")
- Use `search_hint` JQL to find migrated tickets efficiently

### Cross-Board Projects (`jira.cross_board_projects`)

Many repos work across multiple Jira boards. The config tracks which boards this repo interacts with and what agents are allowed to do there:

```yaml
jira:
  cross_board_projects:
    - key: "POP"
      relationship: "Platform/infra — IAM, AWS, CI/CD"
      can_create: true
    - key: "SSE"
      relationship: "Shared services — MCP servers, integrations"
      can_create: false
```

**Agent rules for cross-board projects**:
- Search cross-board projects when investigating related work or dependencies
- Link tickets across boards when work spans projects (use Jira issue links)
- Comment on cross-board tickets when providing context or evidence
- Only create tickets in cross-board projects when `can_create: true` AND the work clearly belongs to that board (not this repo's primary project)
- Always apply `repo_label` when creating tickets in cross-board projects
- When creating in a cross-board project, include a link back to the primary project ticket

### Project Selection Logic

When an agent needs to create a Jira ticket:

1. **Default**: Use `jira.project_key` (primary project)
2. **Cross-board**: If the work clearly belongs to another board AND `can_create: true` for that board, create there instead
3. **Legacy**: NEVER create in legacy projects — always use the primary project
4. **Unknown**: If unsure which board, create in the primary project and let a human move it

## When To Comment

Add a Jira comment when any of these happen:

- work starts and there is meaningful context to share
- a blocker appears or clears
- evidence has been produced and should be mirrored
- a question needs a human answer
- the work is handed off
- the ticket is closed but needs a traceability addendum

Do not comment just to narrate trivial activity.

## Comment Types

| Situation | Preferred comment shape |
|---|---|
| pushback | intake confidence -> what's missing -> signal breakdown -> actionable guidance |
| progress update | status -> what changed -> evidence -> next step |
| blocker | blocked reason -> impact -> unblock needed |
| evidence mirror | result -> commands/tests/docs -> repo paths -> confidence journey |
| decision note | decision -> rationale -> linked artifacts |
| escalation | blocker description -> what was attempted -> what's needed |
| conflict | conflict details -> resolution status -> support window info |
| post-close update | why the update is being added after close -> traceability artifact refs |

Use `jira-comment-writing-axiom` to draft the actual text.

## When To Transition Status

Transition the issue when the workflow state actually changed.

- move to in-progress when execution is truly underway
- move to blocked when work cannot proceed without an external unlock
- move to review when the ticket has reached its next human verification gate
- move to done only when the ticket's definition of done is satisfied

Do not transition status just because a comment was added.

## When To Assign Or Reassign

- assign to the active owner when a person or agent is expected to drive the next step
- reassign when ownership genuinely changes
- avoid leaving issues assigned to someone who is only waiting on another party

If the next step is human clarification, assign or @mention the human owner only if the local Jira process expects it.

## Evidence Mirroring Rules

- Mirror summaries, not full logs.
- Link repo evidence paths, verification notes, PRs, and specs.
- Keep comments short enough to skim in Jira.
- Never paste secrets, tokens, or large raw outputs.
- If the ticket is the durable audit log, make sure Jira has enough context that a human can see status and evidence at a glance.

## Closed Ticket Rule

Closed tickets may still receive comments when the comment adds traceability, evidence, or historical clarification.

Examples:
- post-close repo evidence summary
- final PR or release link
- audit correction that does not reopen scope

Do not reopen solely to add traceability notes unless the actual work state changed.

## Recommended PM Handoff Payload

When another agent needs Jira work done, send `@pm-axiom`:

```json
{
  "request": "Manage Jira updates for this work item",
  "work_item_id": "<id>",
  "mode": "docs_only",
  "context_refs": [
    "<jira key>",
    "<spec refs>",
    "<evidence paths>",
    "<desired transition or comment reason>"
  ],
  "desired_outputs": [
    "jira action plan",
    "issue-type recommendation",
    "comment draft",
    "transition recommendation",
    "due date recommendation"
  ],
  "due_date": "<YYYY-MM-DD or null if unknown>",
  "due_date_rationale": "<why this date, or 'estimate based on scope'>"
}
```

## Strong Recommendations

- Always ask `@pm-axiom` to manage Jira unless a higher-priority prompt explicitly assigns Jira ownership elsewhere.
- Always use the writing child skills for text quality.
- Keep workflows lean; do not invent statuses when a comment would do.
- Keep acceptance criteria testable and current.
- Keep Jira synchronized with real repo evidence.

## Sources

- `specs/05-Jira-Integration.md`
- `specs/14-Integrations-Jira-GitHub.md`
- `specs/11-Confidence-Scoring.md` (intake/execution/output confidence)
- `jira-field-standard-axiom`
- `jira-ticket-writing-axiom`
- `jira-comment-writing-axiom`
- Atlassian guidance on workflows, work types, epics/stories/tasks, and acceptance criteria
