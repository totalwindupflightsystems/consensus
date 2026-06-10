---
name: axiom-meta-planning-contract
description: Portable meta-planning contract (required sections, light vs standard).
version: "1.2"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# Meta-Planning Contract (Portable)

Meta-planning happens before implementation planning.

## Pre-Execution Gate (MANDATORY)

**Before executing any plan step or writing any `runs/<RUN_ID>/` snapshot, the following files
MUST exist at `.memory-bank/work-items/<WORK_ITEM_ID>/`:**

| File | Status required | Consequence if absent |
|---|---|---|
| `meta-planning.md` | MUST exist | BLOCK execution — write it first |
| `plan.md` | MUST exist | BLOCK execution — write it first |
| `plan.yaml` | MUST exist | BLOCK execution — write it first |

**Checklist (run before first step execution):**
- [ ] `meta-planning.md` written to work-item root
- [ ] `plan.md` written to work-item root
- [ ] `plan.yaml` written to work-item root
- [ ] At least one `axiom:trace work_item=<ID> ...` line present in `meta-planning.md`

If any item is unchecked: **stop, write the missing file(s), then proceed.**

> **Why this gate exists**: In production use (fl97inc/pastebin, 2026-04-14), 5 of 6 work items
> were found to have only `runs/` folders with no plan files. Agents had executed work and written
> run snapshots but skipped the durable planning artifacts. A future agent picking up any of those
> work items had no structured context for intent, phases, or acceptance criteria.
> Source: OpenCode session `ses_272686314ffebpTpLFqXZp1bGE`.

## Outputs (Required)

Create/update **before execution begins**:
- `.memory-bank/work-items/<WORK_ITEM_ID>/meta-planning.md`
- `.memory-bank/work-items/<WORK_ITEM_ID>/plan.md`
- `.memory-bank/work-items/<WORK_ITEM_ID>/plan.yaml`

Create/update **after first run completes**:
- `.memory-bank/work-items/<WORK_ITEM_ID>/verification.md` (rolling)
- `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/verification.md` (immutable snapshot)

## When Meta-Planning Runs

- Default: every work item.
- Exception: mechanical fixes may use a light meta-plan.

## Mechanical Fix Criteria

A change is a mechanical fix only if ALL are true:
- Single file affected
- No behavior change
- No contract/spec update needed
- Deterministic change (no design judgment)

If any criterion is ambiguous: treat as standard (full) meta-plan.

## Light Meta-Plan

Include only:
1. Intent
2. Contract Reconciliation (abbreviated)
3. Verification Design

Add: `<!-- meta-plan-type: light -->` near the top so tooling can detect it.

**The pre-execution gate still applies to light meta-plans.** Even a light meta-plan must be
written to `meta-planning.md` before any execution step runs.

## Standard Meta-Plan Sections (REQUIRED)

If a section is not applicable, write `N/A — <reason>` (do not omit sections).

1) Intent
- What changes and why?
- In-scope / out-of-scope

2) Contract Reconciliation
- **Jira ticket reference** (key + URL if Jira-sourced; "N/A — non-Jira work item" otherwise)
- Acceptance criteria summary (quoted from Jira ticket or intake)
- Contract/specs touched (target repo contract, not Axiom core specs)
- Conflicts discovered and resolution
- **Related Jira tickets** (parent/child, blocking, linked issues) if any

3) Decision Points
- Product/policy decisions needed
- Defaults chosen and what changes with other choices

4) Risks and Blast Radius
- Security-sensitive areas
- Operational risks
- Rollback/containment notes

5) Verification Design
- Evidence that proves done
- Required vs optional checks
- Which verifiers must run

#### 5a) Test Value Assessment (REQUIRED when tests are added/modified)

Per `specs/48-Test-Quality-Gates.md#REQ-TQ-011`, answer these questions:

1. **Spec-to-test mapping**: Which spec requirement does each test verify?
2. **Real code path**: What actual product function does each test call? (Not mocks, not stubs, not raw HTTP)
3. **Deletion test**: What breaks if this test is deleted? If "nothing" → don't write it.
4. **Gap check**: Any spec requirements with zero test coverage?
5. **Anti-pattern check**: Do any tests match the green theater anti-patterns?

The goal: tests that verify what the system actually needs to do, not tests that make coverage go up.

Key anti-patterns to avoid:
- Fake executor bypass (returns "ok" without exercising real path)
- Raw HTTP instead of adapter (tests the mock server, not the code)
- Coverage padding (`*_coverage.py` with no behavioral assertions)
- Source inspection (`inspect.getsource()` instead of calling the function)
- Weak assertions (`score >= 0`, `isinstance()` alone)
- Missing negative tests (happy path only)

6) Ambiguity Assessment
- Assumptions the plan depends on
- Ambiguity rating (Low/Medium/High) + rationale
- Missing inputs + compensations

## Jira Ticket Tracking (REQUIRED when Jira-sourced)

When the `WORK_ITEM_ID` is a Jira key (e.g., `PROJ-123`), the meta-planning process MUST follow these Jira management rules. These ensure the Jira ticket remains the durable notebook and audit log for the work item.

### Before Writing

| Step | Action | Reference |
|---|---|---|
| 1 | Read latest Jira ticket state (description, AC, comments, priority, labels, linked issues) via Atlassian MCP | `specs/05-Jira-Integration.md` |
| 2 | Check intake confidence; follow pushback protocol if below threshold | `specs/05-Jira-Integration.md#intake-confidence-pushback-protocol` |
| 3 | Identify parent/child/blocking Jira relationships | `specs/05-Jira-Integration.md#multi-repo-work-v1` |

If Atlassian MCP is unavailable: proceed with intake data and note "Jira sync deferred — MCP unavailable" in the meta-plan.

### During Writing

| Step | Action | Reference |
|---|---|---|
| 4 | Include `jira_ref=<KEY>` in all `axiom:trace` markers | `specs/21-Traceability-Doctrine.md#external-reference-fields` |
| 5 | Quote Jira ticket AC in Contract Reconciliation section | `specs/20-Meta-Planning.md#2-contract-reconciliation-required` |
| 6 | Include `jira_key` in `plan.yaml` `work_item` section | `specs/03-Plan-Schema.md` |

### After Writing

| Step | Action | Reference |
|---|---|---|
| 7 | Post progress comment to Jira summarizing meta-plan | `specs/05-Jira-Integration.md#comment-format` |
| 8 | Transition ticket to "AI Handoff (Plan)" | `specs/10-Lifecycle-State-Machine.md` |
| 9 | Post questions to Jira if ambiguity detected | `specs/05-Jira-Integration.md#comment-types` |

### Downstream Handoff

The meta-plan and plan MUST carry enough Jira context that downstream agents can:
- Include `jira_ref=` in trace markers without re-reading the ticket
- Know the ticket's acceptance criteria for verification
- Post progress/evidence comments to the correct Jira ticket

This means `plan.yaml` MUST include `jira_key` and `plan.md` MUST reference the Jira ticket in its header.

### Non-Jira Work Items

When the work item is NOT Jira-sourced (e.g., local CLI, manual request):
- Write "N/A — non-Jira work item" in the Jira reference field of Contract Reconciliation
- Omit Jira status transitions and comment posting
- All other meta-planning rules still apply
- If the work item is later linked to a Jira ticket, update the meta-plan and plan with the Jira key

## Bug Fix Mode

Bug Fix Mode activates for work items where: Jira type is Bug/Hotfix, `mode=bugfix` is set, or the title/description contains `[bugfix]` or `[hotfix]`.

**Instead of the full meta-plan**, Bug Fix Mode uses a 3-phase lightweight plan:

| Phase | Name | Required steps |
|---|---|---|
| Phase 0 | Root Cause Confirmation | (1) Staleness/already-resolved check (Gate 1); (2) Reproduce-or-Flag classification (Gate 4); (3) Strategy Falsification — hypothesis + ≥2 alternatives + falsification criteria (Gate 3) |
| Phase 1 | Fix Implementation | (4) Targeted minimal diff — scope fence to bug target files and direct dependencies; (5) No opportunistic refactoring; (6) Trace marker adjacent to changed behavior |
| Phase 2 | Regression Test + PR Scope Check | (7) Regression test proving the bug is fixed; (8) PR scope check — file count, no unrelated diffs, no memory-bank files in app PR (Gate 6) |

**What Bug Fix Mode suppresses** (skip list):
- New ADR creation (unless fix changes an architectural decision)
- New runbook creation (unless fix changes operational behavior)
- New observability spec sections
- Full adversarial battery (@devils-advocate-axiom, @assumption-buster-axiom, @redteam-axiom)

**Gate 3 is NOT suppressed** — Strategy Falsification runs in Phase 0 regardless of Bug Fix Mode.

**Scope-size guard**: If the plan touches >3 files or >100 lines net change, the adversarial battery suppression is automatically lifted.

**Override**: Add `include=<item>` to re-enable any suppressed item. `include=all` re-enables everything.

**Full spec**: `specs/20-Meta-Planning.md#bug-fix-mode`

<!-- axiom:trace work_item=sprint-44-gate-integration-01 spec=specs/20-Meta-Planning.md plan=phase-3/task-3-1/step-3-1-1 evidence=.memory-bank/work-items/sprint-44-gate-integration-01/verification.md#ac-4 -->
