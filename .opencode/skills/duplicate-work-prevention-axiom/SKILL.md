---
tags:
  vertical: [planning, coding]
  category: planning
  core: false
---

# Duplicate Work Prevention Skill

**Skill ID**: `duplicate-work-prevention-axiom`  
**Version**: 1.0  
**Spec ref**: `specs/02-Workflows.md`, `specs/00-PRD.md`  
**Jira**: DEX-205  
**axiom:trace**: `work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-1/task-97-1-1/step-97-1-1-1`

---

## Purpose

Teach agents to check for existing work before starting new work, preventing
duplicate effort, conflicting implementations, and wasted cycles.

This skill is a **soft SOP** (guidance, not a hard contract). Load it when
starting any new work item, feature, or investigation to ensure you are not
duplicating something already in progress or recently completed.

---

## When to Load This Skill

Load this skill when:
- Starting a new work item or feature
- Receiving a user request that might overlap with existing work
- About to create a new spec, plan, or implementation
- Investigating a bug that might already have a fix in progress

---

## Duplicate Work Prevention Checklist

Before starting any new work, run through this checklist:

### 1. Check the Active Work Queue

```bash
# Check what's currently active
cat .memory-bank/work-items/_current.md

# Check the full TODO backlog
grep -n "^\- \[ \]" .memory-bank/TODO.md | head -30
```

**What to look for**: Is there already an open TODO item or active work item
that covers the same spec requirement or feature area?

### 2. Search the Memory Bank

```bash
# Search for related work items
ls .memory-bank/work-items/

# Search for related topics
grep -r "KEYWORD" .memory-bank/ --include="*.md" -l | head -10
```

**What to look for**: Existing work items, plans, or findings that address
the same problem.

### 3. Check the Spec

```bash
# Check if the spec already has a "realized-by" link
grep -n "realized-by\|DONE\|complete\|implemented" specs/RELEVANT-SPEC.md | head -20
```

**What to look for**: Requirements already marked as implemented or with
realized-by links pointing to existing code.

### 4. Check Git History

```bash
# Search recent commits for related work
git log --oneline --since="30 days ago" | grep -i "KEYWORD"

# Check if a file already exists
ls .axiom/plugin/src/layers/FEATURE/ 2>/dev/null
```

**What to look for**: Recent commits that already implement the feature,
or existing source files that cover the requirement.

### 5. Check the Implementation Plans

```bash
# List all implementation plans
ls .memory-bank/implementation-plans/

# Check if a plan already covers this area
grep -l "KEYWORD" .memory-bank/implementation-plans/*.md
```

**What to look for**: Existing implementation plans that already schedule
the work you are about to start.

---

## Decision Rules

| Finding | Action |
|---------|--------|
| Active work item covers the same spec requirement | Route to that work item; do not create a new one |
| Completed work item already satisfies the requirement | Verify the implementation; do not re-implement |
| Partial implementation exists | Extend the existing work item; do not start fresh |
| No existing work found | Proceed with creating a new work item |
| Conflicting implementations found | Escalate to @devils-advocate-axiom before proceeding |

---

## Anti-Patterns to Avoid

- **Parallel duplicate work**: Starting a new work item while an identical one is already active
- **Re-implementing completed features**: Implementing something that was already shipped
- **Scope overlap without coordination**: Two work items touching the same files/modules without awareness
- **Stale plan resurrection**: Reopening a completed plan step for "transcript repair" when the product behavior is already correct

---

## Quick Verification Commands

```bash
# Is this spec requirement already implemented?
grep -rn "REQ-PLG-XXX\|step-70-4" .memory-bank/TODO.md | grep "\[x\]"

# Is there an existing module for this feature?
find .axiom/plugin/src -name "*.ts" | xargs grep -l "FEATURE_KEYWORD" 2>/dev/null

# What work items exist for this area?
ls .memory-bank/work-items/ | grep -i "KEYWORD"
```

---

## Integration with Axiom Workflow

This skill integrates with the standard Axiom loop:

1. **Before intake**: Run the checklist above
2. **During planning**: Reference existing work items in the new plan
3. **During implementation**: Check for existing modules before creating new ones
4. **During verification**: Confirm the new work does not regress existing behavior

---

## Notes

- This skill is loaded automatically when the `duplicate-work-prevention` prompt
  is injected via `opencode.jsonc` instructions
- The skill is a reminder, not a blocker — use judgment when requirements overlap
- When in doubt, ask the user before proceeding with potentially duplicate work
