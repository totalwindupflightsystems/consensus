---
description: Produce meta-planning + implementation plan for a work item.
agent: dispatch-axiom
---

You are planning work for Axiom.

Inputs:
- Work item id: $WORK_ITEM_ID
  - If this is a Jira-backed run, use the Jira key (e.g., `ABC-123`).
  - Otherwise, use a stable identifier (e.g., `bootstrapping-01`).
- Repo: $REPO

Skills (load on demand):
- `axiom-meta-planning-contract` — Detailed meta-planning rules and required sections.
- `axiom-plan-schema` — Plan YAML structure reference.
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `jira-workflow-axiom` — Jira operating model (load when `$WORK_ITEM_ID` is a Jira key).

Do:
1) Read `specs/` and `.memory-bank/_index.md`.
2) Create or update work-item files under `.memory-bank/work-items/$WORK_ITEM_ID/`.
3) Produce meta-planning notes and an implementation plan (Markdown + YAML) per:
   - `.memory-bank/_prompt.md` (memory rules)
   - `.opencode/skills/axiom-meta-planning-contract/SKILL.md`
   - `.opencode/skills/axiom-plan-schema/SKILL.md`
4) Ensure verification gates are explicit.

Output:
- Emit the required XML envelope per `.opencode/skills/axiom-xml-protocol/SKILL.md`.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the work item ID and how many phases/tasks/steps were planned.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/work-items/$WORK_ITEM_ID/meta-planning.md`, `plan.md`, `plan.yaml`
- `evidence.work_item_id`: the stable work item ID used
- `evidence.plan_path`: full path to `plan.yaml`
- `evidence.meta_planning_path`: full path to `meta-planning.md`
- `evidence.phases_count`: number of phases in the plan
- `evidence.steps_count`: total number of steps across all phases
- `related_commands`: suggested follow-up commands
  - "To execute the first step, run: `/axiom-step --work-item <id>`"
  - "To run all steps in sequence, run: `/axiom-step-loop --work-item <id>`"
  - "To refresh the roadmap after planning, run: `/axiom-roadmap-refresh`"

### Cross-References
- "Plan schema is defined in: `.opencode/skills/axiom-plan-schema/SKILL.md`"
- "Meta-planning contract is in: `.opencode/skills/axiom-meta-planning-contract/SKILL.md`"
- "Work item artifacts are at: `.memory-bank/work-items/$WORK_ITEM_ID/`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
