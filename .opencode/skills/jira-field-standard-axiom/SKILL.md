---
name: jira-field-standard-axiom
description: Standard Jira field contract for Axiom workspaces. Defines the minimum cross-workspace field set, naming, types, and admin rollout guidance for agent-manageable Jira execution.
version: "1.0"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Jira Field Standard

Use this skill when a Jira project needs to support Axiom with consistent cross-workspace metadata.

axiom:trace work_item=DEX-74 spec=specs/05-Jira-Integration.md,specs/14-Integrations-Jira-GitHub.md,specs/06-Project-Configuration.md plan= test= doc=.memory-bank/explorations/jira-field-standard.md prompt=.opencode/skills/jira-field-standard-axiom/SKILL.md evidence= commit= jira_ref=DEX-74

## Purpose

Axiom works best when Jira projects expose a small, predictable metadata surface.

- Repos, specs, evidence, and runtime state stay authoritative in the repo.
- Jira stores enough structured metadata to route, mirror, and audit work consistently.
- The field set should be portable across projects like `DEX`, `SSE`, and future workspaces.

## Primary Rule

Prefer a lean field set.

- Put stable identifiers and routing data in custom fields.
- Put volatile execution detail in comments, linked repo paths, and evidence bundles.
- Do not create custom fields for data that changes every run.

## Repo Scoping Within Shared Jira Projects

Jira projects typically serve a team or group of efforts, not a single repo. When multiple repos share a Jira project, use **Jira labels** to scope tickets to the correct repo.

### How It Works

- Each repo declares a `jira.repo_label` in `.axiom/axiom.config.yaml` (e.g., `axiom-backend`).
- When creating tickets, Axiom auto-applies this label.
- When querying tickets, JQL includes `labels = "{repo_label}"` to filter.
- The `/axiom-sync-jira` command uses this label to scope its health report.

### Label Naming Convention

- Use lowercase with hyphens: `axiom-backend`, `axiom-frontend`, `my-service`.
- No spaces (Jira restriction).
- Prefix with `axiom-` for discoverability when the Jira project has non-Axiom labels too.

### Relationship to `Axiom repo` Custom Field

Labels and the custom field are complementary:

| Mechanism | Purpose |
|---|---|
| `jira.repo_label` (label) | Quick JQL filtering; no admin setup; works out of the box |
| `Axiom repo` (custom field) | Authoritative routing; validated `org/repo` format; used by intake pipeline |

Use both for defense-in-depth. Use labels alone when custom field admin is not available.

## Required Axiom Fields

These fields are the recommended minimum cross-workspace standard.

| Field name | Suggested type | Required? | Purpose |
|---|---|---:|---|
| `Axiom Repo` | single-line text | yes | Canonical repo or workspace slug for routing work |
| `Axiom Work Item ID` | single-line text | yes | Stable Axiom work item id, e.g. `opencode-plugin-01` |
| `Axiom Spec Refs` | paragraph text | yes | Spec paths and anchors governing the ticket |
| `Axiom Verification Bar` | single select | yes | `standard`, `high`, or `mission_critical` |
| `Axiom Evidence Ref` | URL or single-line text | yes | Primary verification/evidence path or URL |

## Required Native Jira Fields (Date Tracking)

These are native Jira fields (not custom) that Axiom requires for scheduling and health tracking.

| Field name | Jira field key | Required? | Purpose |
|---|---|---:|---|
| `Due date` | `duedate` | yes | Target completion date. MUST be set when creating tickets. Used for health tracking, overdue detection, and sync reporting. |
| `Start date` | `startDate` (or custom) | recommended | When work is expected to begin. Used for timeline estimation and sprint planning context. |
| `Priority` | `priority` | yes | Jira native priority. Used for pushback cooldown (P1/P2 get shorter cooldowns) and work ordering. |

### Due Date Rules

1. **On ticket creation**: `@pm-axiom` MUST set a `duedate` when creating or refining a Jira ticket. If the user/operator does not specify a due date, the agent SHOULD estimate one based on scope and complexity, and note the estimate as an assumption in the ticket description.
2. **On ticket update**: When scope changes materially (new acceptance criteria, blocked dependencies cleared, scope expansion), `@pm-axiom` SHOULD re-evaluate and update the due date with a comment explaining the change.
3. **Overdue detection**: The `/axiom-sync-jira` command checks for tickets past their due date and flags them in the sync report. Overdue tickets with no recent activity are escalation candidates.
4. **Due date in comments**: Progress comments SHOULD include days remaining or days overdue when the due date is set. Use format: `Due: YYYY-MM-DD (N days remaining)` or `Due: YYYY-MM-DD (N days overdue)`.
5. **Sprint alignment**: When a ticket has both a due date and a sprint, the due date SHOULD fall within or before the sprint end date. Misalignment is flagged as a warning in sync reports.

## Recommended Optional Fields

Add these when the Jira admin burden is acceptable.

| Field name | Suggested type | Purpose |
|---|---|---|
| `Axiom Desired Mode` | single select | `bugfix`, `implement_feature`, `refactor`, `audit`, `release`, `docs_only`, `incident` |
| `Axiom Blocked Reason` | paragraph text | Short machine-friendly blocker explanation |
| `Axiom External Refs` | paragraph text | PR URLs, Notion refs, incident docs, external trackers |
| `Axiom Target Env` | single select | `local`, `dev`, `stage`, `prod` |
| `Axiom Risk Tolerance` | single select | `low`, `standard`, `high` |

## Fields To Avoid

Do not create custom fields for:

- commit hashes
- every run id
- raw test output
- full plans
- every PR comment
- per-agent scratch notes
- due date overrides (use the native Jira `duedate` field, not a custom field)

Those belong in repo evidence, Jira comments, or linked artifacts.

## Naming Guidance

- Prefix Axiom-owned custom fields with `Axiom ` for discoverability.
- Use title case in Jira display names.
- Keep names stable across workspaces.
- Prefer text/select fields over complex custom object types unless a strong reporting need exists.

## Suggested Select Options

### `Axiom Verification Bar`

- `standard`
- `high`
- `mission_critical`

### `Axiom Desired Mode`

- `bugfix`
- `implement_feature`
- `refactor`
- `hardening`
- `audit`
- `release`
- `docs_only`
- `incident`

### `Axiom Target Env`

- `local`
- `dev`
- `stage`
- `prod`

### `Axiom Risk Tolerance`

- `low`
- `standard`
- `high`

## DEX Baseline Snapshot

DEX currently exposes standard Jira project fields such as `Summary`, `Description`, `Labels`, `Assignee`, `Due date`, `Parent`, `Team`, `Start date`, `Flagged`, and a few Atlassian-native development fields.

DEX does **not** currently expose the Axiom-specific custom field set above as visible create/edit metadata.

## Issue Type Hierarchy

Axiom uses a 4-level hierarchy to track work from strategic intent down to individual tasks. Every ticket MUST be linked to its parent level so work is traceable from Initiative → Epic → Task → Subtask.

```
Initiative
    └── Epic
            └── Task
                    └── Subtask
```

| Level | Jira Type | What it represents | Who creates it |
|---|---|---|---|
| **Initiative** | Initiative (classic) | Strategic capability or product area. Spans multiple epics, multiple sprints. | Owner / PM |
| **Epic** | Epic | A major feature or work stream. Contains multiple tasks. Maps to a Axiom worktree batch or spec area. | `@pm-axiom` |
| **Task** | Task | A single deliverable. Maps to one Axiom work item / worktree. | `@pm-axiom` / agents |
| **Subtask** | Subtask | A specific step within a task. Maps to a phase or step in a Axiom implementation plan. | Agents |

### Hierarchy Rules for Agents

**When creating a Task:**
- MUST set `parent` to an Epic
- If no Epic exists for the work area, create one first or ask the user
- Epic should map to a spec area (e.g., "Release Engineering", "Morty Orchestration", "Wave Portal")

**When creating a Subtask:**
- MUST set `parent` to a Task
- Use for breaking down complex tasks into verifiable steps
- Each subtask should have a clear, testable acceptance criterion

**When creating an Epic:**
- MUST set `parent` to an Initiative (in classic Jira with Initiative hierarchy)
- If no Initiative exists, create one or ask the user which Initiative this belongs to
- Epic name should match the spec area or worktree theme

**When creating an Initiative:**
- Top-level — no parent required
- Represents a strategic product area (e.g., "AI Agent Platform", "Developer Experience", "Observability")
- Created by owner/PM, not by agents autonomously

### JQL for Hierarchy Navigation

```jql
# All tasks under a specific epic
project = SWDE AND issuetype = Task AND "Epic Link" = SWDE-42

# All open tasks without an epic (orphaned — fix these)
project = SWDE AND issuetype = Task AND "Epic Link" is EMPTY AND status != Done

# All epics under an initiative
project = SWDE AND issuetype = Epic AND "Parent" = SWDE-10

# Full hierarchy for a work area
project = SWDE AND (issuetype = Initiative OR issuetype = Epic OR issuetype = Task) AND labels = "axiom-repo"
```

### Hierarchy Config in `.axiom/axiom.config.yaml`

```yaml
jira:
  hierarchy:
    initiative_type: "Initiative"    # Jira issue type name for top-level strategic items
    epic_type: "Epic"                # Jira issue type name for epics
    task_type: "Task"                # Jira issue type name for tasks
    subtask_type: "Subtask"          # Jira issue type name for subtasks
    
    # Default parent epics for common work areas.
    # Agents use these when creating tasks without an explicit parent.
    # Format: "work_area_keyword": "PROJ-NNN"
    default_epics:
      release: null          # Set to epic key when release engineering epic exists
      morty: null            # Set to epic key when Morty epic exists
      wave: null             # Set to epic key when Wave epic exists
      analyze: null          # Set to epic key when Analyze epic exists
      security: null         # Set to epic key when security epic exists
      infra: null            # Set to epic key when infra epic exists
    
    # Require parent on creation (fail closed if no parent found)
    require_epic_for_tasks: true
    require_task_for_subtasks: true
    require_initiative_for_epics: false  # Softer — warn but don't block
```

### Agent Behavior When Parent Is Missing

| Situation | Agent Action |
|---|---|
| Creating a Task with no matching Epic | Search for existing epics in the project. If found, link to best match. If not found, create an Epic first, then the Task. |
| Creating a Subtask with no parent Task | Always require a parent Task — fail closed. |
| Creating an Epic with no Initiative | Warn the user. Create the Epic anyway (soft requirement). |
| `require_epic_for_tasks: false` | Create Task without Epic but add label `no-epic` for later cleanup. |

## Admin Rollout Playbook

When standing up a new Jira project for Axiom:

1. Create the required Axiom custom fields with the names and types above.
2. Add them to the create, edit, and view screens for the issue types Axiom will touch.
3. At minimum, expose them on `Task`, `Epic`, and `Subtask`.
4. Ensure the project workflow keeps status values simple and maps blocked work clearly.
5. Mirror the Jira project key into repo config at `.axiom/axiom.config.yaml` under `jira.project_key`.
6. Confirm the PM/admin agent knows which project owns the field rollout.

## Agent Operating Model

- `@pm-axiom` should be the default owner of Jira metadata hygiene.
- `@tower-axiom` may inspect field availability and route follow-up work.
- Builder and verifier agents should supply facts, then let PM mirror them into Jira fields/comments.

## Automation Reality Check

This skill defines the contract and the rollout checklist.

- If Jira custom-field admin APIs or MCP tools are available, an admin-capable agent may implement this skill directly.
- If those admin APIs are not exposed, the agent should still use this skill to generate the exact field request for a human Jira admin.
- Fail closed: do not pretend fields were created unless Jira metadata confirms they exist.

## Recommended Admin Request Template

Use this when a workspace is missing the Axiom field set:

```text
Please add the standard Axiom Jira custom fields to project <PROJECT_KEY>:

Required:
- Axiom Repo (single-line text)
- Axiom Work Item ID (single-line text)
- Axiom Spec Refs (paragraph text)
- Axiom Verification Bar (single select: standard, high, mission_critical)
- Axiom Evidence Ref (URL or single-line text)

Optional:
- Axiom Desired Mode (single select)
- Axiom Blocked Reason (paragraph text)
- Axiom External Refs (paragraph text)
- Axiom Target Env (single select)
- Axiom Risk Tolerance (single select)

Please add them to the create, edit, and view screens for Task, Epic, and Subtask.

Also ensure the following native Jira fields are visible on create/edit screens:
- Due date
- Start date (if available in your Jira plan)
- Priority
```

## Related Skills

- `jira-workflow-axiom`
- `jira-ticket-writing-axiom`
- `jira-comment-writing-axiom`

## Sources

- `specs/05-Jira-Integration.md`
- `specs/14-Integrations-Jira-GitHub.md`
- `specs/06-Project-Configuration.md`
