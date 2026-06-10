---
description: Sync Jira ticket state with repo work items (status, due dates, evidence, health).
agent: pm-axiom
---

Bidirectional sync between Jira tickets and repo work items so the system stays healthy and dates stay honest.

Skills (load on demand):
- `jira-workflow-axiom` — Jira operating model, issue taxonomy, sync rules, and evidence mirroring. Always load.
- `jira-field-standard-axiom` — Standard Jira field contract for Axiom workspaces.
- `axiom-xml-protocol` — XML envelope format and required tag set.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many tickets were synced and how many issues were found.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/jira-mapping.md` if updated
- `evidence.tickets_synced`: count of Jira tickets processed
- `evidence.remediations_applied`: list of actions taken (status transitions, field updates, etc.)
- `evidence.remediations_proposed`: list of actions needing human confirmation
- `evidence.health_issues_found`: count of health issues detected (overdue, drift, stale, etc.)
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To view the Jira mapping, read: `.memory-bank/jira-mapping.md`"

### Cross-References
- "Jira workflow skill is in: `.opencode/skills/jira-workflow-axiom/SKILL.md`"
- "Jira mapping is at: `.memory-bank/jira-mapping.md`"
- "Spec: `specs/05-Jira-Integration.md`"

axiom:trace work_item=command-quality-01 spec=specs/05-Jira-Integration.md,specs/14-Integrations-Jira-GitHub.md plan= test= doc= prompt=.opencode/skills/jira-workflow-axiom/SKILL.md evidence= commit=

Inputs
- Optional: `$ARGUMENTS` = scope filter (Jira project key, specific ticket keys, work item ids, or `--overdue-only`).
- Requires: Atlassian MCP available (fail closed if not).

Prerequisites
- Load `jira-workflow-axiom` skill (parent) and `jira-field-standard-axiom` (field contract).
- Read `.axiom/axiom.config.yaml` for `jira.project_key` and `jira.repo_label`.
- Read `.memory-bank/jira-mapping.md` for the canonical Jira ↔ work item mapping.

Do

## Phase 1: Gather State (read-only)

1) **Jira snapshot**: Query Jira for all tickets in the project that are scoped to this repo:
   - If `jira.repo_label` is set, use JQL: `project = {project_key} AND labels = "{repo_label}" AND (assignee = currentUser() OR key in ({mapped_keys})) AND resolution = Unresolved ORDER BY duedate ASC`
   - If `jira.repo_label` is null, fall back to: `project = {project_key} AND (assignee = currentUser() OR key in ({mapped_keys})) AND resolution = Unresolved ORDER BY duedate ASC`
   - Capture: key, summary, status, priority, duedate, assignee, labels, updated timestamp.

2) **Repo snapshot**: Enumerate `.memory-bank/work-items/` and cross-reference with `jira-mapping.md`:
   - For each work item: check if plan.yaml/plan.md exists, check verification.md, check latest run status.
   - Derive repo-side status: `done | in_progress | blocked | not_started | deferred`.

3) **Build diff table**: For each mapped ticket, compare:
   - Jira status vs repo status (detect drift).
   - Jira due date vs today (detect overdue).
   - Jira due date presence (detect missing due dates).
   - Last Jira comment timestamp vs last repo activity (detect stale tickets).
   - Axiom custom fields populated vs empty (detect field hygiene gaps).

## Phase 2: Health Report

4) **Produce health report** with these sections:

   a. **Overdue tickets**: Jira tickets past their due date. Include days overdue, current status, last activity.
   b. **Missing due dates**: Tickets with no due date set. Flag as hygiene issue.
   c. **Status drift**: Tickets where Jira status disagrees with repo status (e.g., Jira says "In Progress" but repo work item is done or blocked).
   d. **Stale tickets**: Tickets with no Jira comment or repo activity in >7 days while status is not Done/Deferred.
   e. **Field hygiene**: Tickets missing required Axiom custom fields (repo, work item id, spec refs, verification bar, evidence ref).
   f. **Label hygiene**: When `jira.repo_label` is set, tickets in jira-mapping.md that are missing the repo label.
   g. **Unmapped tickets**: Jira tickets assigned to Axiom (and labeled for this repo) that have no entry in jira-mapping.md.
   h. **Orphaned work items**: Repo work items in jira-mapping.md whose Jira ticket no longer exists or is resolved.

## Phase 3: Remediation (with confirmation)

5) **For each issue found**, propose a remediation action:

   | Issue | Proposed action |
   |---|---|
   | Missing due date | Set due date based on scope estimate; add comment with rationale |
   | Status drift (repo ahead) | Transition Jira ticket to match repo status; add evidence comment |
   | Status drift (Jira ahead) | Flag for human review (Jira may have been manually updated) |
   | Stale ticket | Post a status check comment asking for update |
   | Missing custom fields | Populate from repo work item data where available |
   | Missing repo label | Add `repo_label` to ticket labels |
   | Unmapped ticket | Add entry to jira-mapping.md |
   | Orphaned work item | Mark as resolved in jira-mapping.md |
   | Overdue, no activity | Post overdue escalation comment |

6) **Execute remediations**:
   - For safe actions (missing due dates, status drift where repo is ahead, missing fields, missing repo labels): execute directly.
   - For ambiguous actions (status drift where Jira is ahead, stale tickets): propose only, do not execute without confirmation.
   - For each Jira update, post a comment explaining the sync action.

## Phase 4: Update Mapping

7) **Update `.memory-bank/jira-mapping.md`**:
   - Add any unmapped tickets.
   - Update status column to match current state.
   - Update summary statistics.
   - Update "Last updated" timestamp.

Fail closed
- If Atlassian MCP is not available, emit `blocked` status with instructions to configure MCP.
- If `.axiom/axiom.config.yaml` has no `jira.project_key`, emit `blocked`.
- If jira-mapping.md is missing, emit `blocked` with instructions to create it.
- Do not fabricate Jira data. If a query fails, report the failure.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-jira</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence health summary
  - `<evidence><health_report>` the full health report
  - `<evidence><remediations_applied>` list of actions taken
  - `<evidence><remediations_proposed>` list of actions needing confirmation
  - `<diagnostics>` warnings, errors, or MCP issues

Output (human-readable)
- Also produce a scannable Markdown summary:

```markdown
## Jira Sync Health Report — {date}

### Summary
- Repo label: {repo_label or "not configured"}
- Tickets synced: N
- Healthy: N
- Overdue: N
- Missing due dates: N
- Missing repo labels: N
- Status drift: N
- Stale: N

### Overdue Tickets
| Ticket | Summary | Due | Days Overdue | Status | Last Activity |
|--------|---------|-----|-------------|--------|---------------|

### Actions Taken
- ...

### Actions Needing Confirmation
- ...
```
