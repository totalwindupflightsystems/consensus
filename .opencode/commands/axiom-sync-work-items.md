---
description: Audit and sync work item folder hygiene (plans, evidence, runs).
agent: tower-axiom
---

Sync `.memory-bank/work-items/` hygiene so work items are executable and evidence is findable.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `axiom-implementation-plans` — Work item plan schema and required files. Load when creating or repairing work item artifacts.
- `evidence-bundle-schema` — Evidence bundle schema (verification.md + outputs.md). Load when creating or repairing run evidence.

Do
1) Enumerate `.memory-bank/work-items/<WORK_ITEM_ID>/` folders.
2) For each work item, ensure the expected files exist:
   - `meta-planning.md`, `plan.md`, `plan.yaml`, `verification.md`
3) Ensure `runs/` exists.
4) Ensure `.memory-bank/work-items/_index.md` lists all work items.
5) For any missing artifacts, either create minimal stubs (portable) or mark as `blocked` with injected steps.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-work-items</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><files_changed>` list of ALL files created/modified (full paths, semicolon-separated)
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many work items were audited and how many had missing artifacts.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
- `evidence.work_items_audited`: count of work item folders examined
- `evidence.work_items_repaired`: count of work items where missing artifacts were created
- `evidence.work_items_blocked`: list of work item IDs that could not be auto-repaired (need manual intervention)
- `related_commands`: suggested follow-up commands
  - "To sync all indexes after repair, run: `/axiom-sync-indexes`"
  - "To run the full sync suite, run: `/axiom-sync-all`"

### Cross-References
- "Work item plan schema is in: `.memory-bank/work-items/_prompt.md`"
- "Evidence bundle schema is in: `.opencode/skills/evidence-bundle-schema/SKILL.md`"
- "To create a new work item plan, run: `/axiom-meta-plan --work-item <id>`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
