---
name: axiom-lifecycle-state-machine
description: Portable lifecycle state machine for Axiom work items — states, transitions, guards, PR lifecycle, conflict remediation, dependency coordination, approval audit trail, and permission auto-approval.
version: "1.0"
synopsis: |
  Defines the 8 core lifecycle states (intake → study → plan → implement → verify → review → done → blocked),
  valid transitions with triggers/actions, PR state machine, conflict remediation policy, dependent work item
  coordination, approval audit trail, and agent permission auto-approval. Portable — no repo specs dependency.
when-to-use: |
  Load this skill when orchestrating work item lifecycle transitions, implementing state machine logic,
  building PR management, handling merge conflicts, coordinating dependent work items, tracking approval
  audit trails, or implementing permission auto-approval gates.
tags:
  vertical: [planning, coding]
  category: planning
  core: false
---

# Axiom Lifecycle State Machine (Portable)

This skill defines the work item and PR lifecycle state machines for Axiom.

Source spec: `specs/10-Lifecycle-State-Machine.md`

---

## Actors

- **Human**: creates/edits ticket, answers questions, reviews/merges PR.
- **Automated systems**: create tickets (Sentry/logs/scanners), update ticket context.
- **Axiom**: executes the workflow, posts comments, creates/updates PRs.
- **Axiom subagents**: invoked by Axiom for specialized work.

---

## Routing / Runnable Gate

A ticket becomes runnable when ALL of:

1. Ticket is assigned to the Axiom Jira service account.
2. Ticket has a target repo (via `Axiom repo` custom field or `jira.repo_override` config).
3. The repo's `max_open_prs` limit has not been reached.

If condition 2 fails: log warning and skip.

---

## Jira Status Mapping

| Jira Status | Internal State | `execution.status` | Trigger |
|---|---|---|---|
| Backlog / To Do | `intake` | `pending` | Ticket assigned to Axiom |
| AI Handoff (Study) | `study` | `pending` | Axiom picks up ticket |
| AI Handoff (Plan) | `plan` | `in_progress` | Meta-planning starts |
| AI Handoff (Implement) | `implement` | `in_progress` | First implementation step |
| AI Handoff (Verify) | `verify` | `in_progress` | Verification chain starts |
| Human Review | `review` | `completed` | Confidence ≥ threshold, all gates pass, PR created |
| Done | `done` | `completed` | PR merged (human action) |
| Blocked | `blocked` | `blocked` | Escalation triggered |

Rules:
- Status discovery at startup via `GET /rest/api/3/project/{projectKey}/statuses`.
- Case-insensitive matching; substring match on "AI Handoff" prefix as fallback.
- Cache status list per run (no mid-run re-query).
- If expected statuses missing: log warning, use default mapping.

---

## Internal State Transitions

```
[*] → intake → study → plan → implement → verify → review → done → [*]
                                    ↕           ↕        ↕
                                 blocked     blocked   blocked
```

### Transition Table

| From | To | Trigger | Actions |
|---|---|---|---|
| `intake` | `study` | Axiom picks up ticket | Jira → "AI Handoff (Study)"; create work-item folder |
| `intake` | `blocked` | Missing repo target or config | Post Jira comment; do not start |
| `study` | `plan` | Context gathered | Jira → "AI Handoff (Plan)"; compute initial confidence |
| `study` | `blocked` | Critical ambiguity, confidence LOW | Post clarifying questions to Jira |
| `plan` | `implement` | Plan created | Jira → "AI Handoff (Implement)"; create branch |
| `plan` | `blocked` | Plan creation failed after retries | Post Jira comment |
| `implement` | `verify` | All steps complete | Jira → "AI Handoff (Verify)" |
| `implement` | `blocked` | Step failed after retries exhausted | Post failure details to Jira |
| `implement` | `implement` | Verifier injects new work | Append injected steps; resume |
| `verify` | `review` | Confidence ≥ threshold AND all gates pass | Jira → "Human Review"; PR `draft: false`; post evidence |
| `verify` | `implement` | Verification failure injects work | Append steps; Jira → "AI Handoff (Implement)" |
| `verify` | `blocked` | Verification failed after retries | Post Jira comment |
| `review` | `done` | PR merged (human) | Jira → "Done"; finalize evidence bundle |
| `review` | `implement` | Human requests changes | Jira → "AI Handoff (Implement)"; parse review; inject steps |
| `review` | `blocked` | Human blocks ticket | Jira → "Blocked" |
| `blocked` | `study` | Blocker resolved, re-study | Jira → "AI Handoff (Study)" |
| `blocked` | `implement` | Blocker resolved, resume from cursor | Jira → "AI Handoff (Implement)" |

---

## Default Ticket Workflow (Phases)

### 1. Intake
Validate runnable gate. Create work-item folder in Memory Bank.

### 2. Study
Read ticket AC, description, comments. Read specs/ and Memory Bank. Identify ambiguities. Compute initial confidence. If LOW → blocked with questions.

### 3. Plan
Meta-planning: reconcile AC with specs. Produce plan (phases/tasks/steps) with verification chain. Persist to Memory Bank.

### 4. Implement
Execute plan steps via `/commands`. Each step produces XML output. Update specs when behavior changes. Checkpoint after each step.

### 5. Verify
Run verification chain (step/task/phase verifiers). Run required checks. Verifiers may inject new work → back to implement. Compute confidence with all signals.

### 6. Review
Create/update PR. Mark ready-for-review when evidence complete and confidence HIGH. Post evidence to Jira and PR. Wait for human action.

### 7. Done
PR merged. Jira → "Done". Finalize evidence bundle. Update Memory Bank. Optionally capture workspace snapshot.

### 8. Reopen / Follow-Up
If snapshot enabled and ticket reopened: restore workspace from snapshot. Otherwise: fresh workspace.

---

## PR Lifecycle

### PR States

| PR State | Description | Jira State |
|---|---|---|
| `created` | Draft PR opened | `plan` or `implement` |
| `updating` | Commits being added | `implement` or `verify` |
| `conflict_remediation` | Merge conflicts; auto-fixing | `implement` |
| `ready_for_review` | Evidence complete; non-draft | `review` |
| `merged` | Human merged | `done` |
| `abandoned` | Closed without merge | `blocked` or re-queued |

### Key PR Transitions

- `updating` → `conflict_remediation`: merge conflict detected (webhook or polling).
- `conflict_remediation` → `updating`: conflicts resolved.
- `conflict_remediation` → `abandoned`: support window expired or remediation failed.
- `updating` → `ready_for_review`: evidence bundle pass + confidence ≥ threshold + all checks pass.
- `ready_for_review` → `updating`: human requests changes.
- `ready_for_review` → `merged`: human merges.

---

## Conflict Remediation Policy

### Triggers
- **Webhook**: GitHub `push` to PR target branch, or `check_suite` with `mergeable_state: "dirty"`.
- **Polling**: periodic check of all open Axiom PRs for `mergeable_state`.

### Support Window
- **Sprint-based** (when sprint data available): current sprint + `support_window_sprints - 1` previous. Default: 2 sprints.
- **Calendar-based** (no sprint data): `support_window_sprints × 14` days from PR creation. Default: 28 days.

### Resolution Process
1. Detect conflict.
2. Check support window — if expired, post Jira comment and stop.
3. Fetch latest effective PR target branch (do NOT hardcode `main` or `origin`).
4. Attempt merge/rebase.
5. If success: push resolved commit; resume.
6. If fail: retry up to `max_retry_attempts` with implementation agent.
7. If all fail: post Jira comment; mark `blocked`.

---

## Dependent Work Item Coordination

### Dependency Declaration

```yaml
dependencies:
  - upstream_work_item_id: "ABC-100"
    condition: "pr_merged"   # v1: only pr_merged supported
```

### Waiting State
- Sub-state of `blocked` with `blocker_type: "dependency_waiting"`.
- No implementation steps execute while waiting.
- Planning (study, plan) MAY proceed.
- Polling checks upstream PR status periodically.

### Unblock Rules
- Upstream PR `merged: true` confirmed via fresh GitHub API call (not cached/webhook-only).
- ALL declared dependencies must be satisfied.
- Transition to `implement` (resume from cursor) or `plan` (if planning incomplete).

### Key Acceptance Criteria
- **AC-DEP-1**: Unsatisfied dependency → `blocked` with `blocker_type: "dependency_waiting"`. No implementation.
- **AC-DEP-2**: Unblock only after fresh API confirms `merged: true`.
- **AC-DEP-3**: Emit `state_transition` with `trigger: "dependency_satisfied"`.
- **AC-DEP-4**: Upstream closed without merge → remain blocked; emit `dependency_upstream_closed` at WARN.

---

## Approval Audit Trail

Every approval decision that gates lifecycle progression is recorded as an `ApprovalRecord`.

### Approval Types

| Type | Gate | Approver |
|---|---|---|
| `plan_approval` | plan → implement | Human or automated (confidence gate) |
| `verification_approval` | verify → review | Automated (checks pass + confidence ≥ threshold) |
| `pr_review_approval` | ready_for_review → merged | Human (GitHub PR review APPROVED) |
| `security_review_approval` | Security gate | Human or automated |
| `change_request` | ready_for_review → updating | Human (CHANGES_REQUESTED) |
| `escalation_approval` | blocked → implement/study | Human (Jira comment) |

### Evidence Linkage Rules
1. `artifact_approved` must resolve to a real artifact.
2. `artifact_hash` (SHA-256) binds approval to specific version — stale if artifact changes.
3. `evidence_refs` must include at least one independently verifiable path.
4. Records are immutable; corrections via new records with rationale.
5. Stored in immutable run snapshots: `.memory-bank/work-items/<ID>/runs/<RUN_ID>/approvals.yaml`.

### Key Acceptance Criteria
- **AC-AUDIT-1**: Every gate transition must have a corresponding `ApprovalRecord` with valid `artifact_hash`.
- **AC-AUDIT-2**: Stale approval detection fires on hash mismatch; blocks until re-approved.
- **AC-AUDIT-3**: Completed work items must have ≥1 human approval (PR merge) + ≥1 automated (verification gate).

---

## Agent Permission Auto-Approval

### Permission Types

| Type | Auto-Approval Eligible |
|---|---|
| `file_write` | Yes (with conditions) |
| `spec_update` | Yes (with conditions) |
| `config_update` | Yes (with conditions) |
| `dependency_add` | Yes (with conditions) |
| `destructive_command` | **Never** |
| `secret_access` | **Never** |
| `production_deploy` | **Never** |
| `branch_force_push` | **Never** |
| `merge_to_protected` | **Never** |

### Auto-Approval Conditions (ALL must be true)
1. Permission type is eligible (not in "Never" list).
2. `auto_approval.enabled: true` in config.
3. `plan_step_ref` references a valid, uncompleted plan step.
4. `target_resource` within plan step's declared scope.
5. No sensitive-area escalation rules triggered.
6. Confidence ≥ `auto_approval.min_confidence` (default: 40 / MEDIUM).
7. Budget not exhausted (`max_per_run`, default: 50).

### Timeout and Escalation
- "Never" types: immediate escalation, no timeout.
- Condition-failure: escalate to human via Jira comment; block if required.
- Timeout (default 60 min): either `block` (default) or `skip` per config.
- Denial: step marked blocked; check for alternative path in plan.

---

## Lifecycle Event Logging

All state transitions emit structured log events. Key events:

| Event | When |
|---|---|
| `ticket_intake` | Ticket enters queue |
| `state_transition` | Any internal state change |
| `jira_status_transition` | Jira status changed |
| `pr_created` / `pr_ready_for_review` / `pr_merged` / `pr_abandoned` | PR lifecycle |
| `conflict_detected` / `conflict_resolved` / `conflict_failed` | Conflict remediation |
| `escalation` | Ticket blocked |
| `dependency_waiting` / `dependency_satisfied` / `dependency_upstream_closed` | Dependency coordination |
| `approval_recorded` / `approval_stale` / `approval_missing` | Approval audit |
| `permission_requested` / `permission_auto_approved` / `permission_escalated` | Permission auto-approval |

---

## Benchmark Work Item Types

| Type | Description |
|---|---|
| `benchmark_bootstrap` | Initial spec extraction + clean-room build |
| `benchmark_sync` | Upstream change processing |
| `benchmark_custom` | Jira-driven custom work on sink repo |

---

## Configuration References

- Conflict remediation: `conflicts.auto_fix.*` in `.axiom/axiom.config.yaml`
- Approval audit: `approval_audit.*` in `.axiom/axiom.config.yaml`
- Auto-approval: `auto_approval.*` in `.axiom/axiom.config.yaml`
- Sprint detection: `jira.use_native_sprint_field` (default: true)

---

## Open Decisions

- Custom substates per repo (v1: via custom verification checks, not custom Jira statuses).
- Parallel task execution (v1: sequential within a phase).
- Auto-close stale PRs (v1: Jira comment only, no auto-close).
