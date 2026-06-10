---
description: Create/update `.memory-bank/implementation-plans/` aligned to `.memory-bank/TODO.md`.
agent: pm-axiom
---

Maintain the Axiom implementation plans.

Skills (load on demand):
- `axiom-implementation-plans` — Implementation plan schema and alignment rules. Always load.
- `implementation-plan-history` — Current vs historical plans, run snapshots, and comment queue handling.
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `jira-workflow-axiom` — Jira operating model (load when plans reference Jira-sourced work items).

Context files:
- `.opencode/skills/implementation-plan-history/SKILL.md`
- `.opencode/skills/axiom-implementation-plans/SKILL.md` (includes Jira ticket tracking rules)
- `.memory-bank/TODO.md`
- `.memory-bank/implementation-plans/_index.md`

Instructions:
- Follow the prompt in @.opencode/prompts/axiom-implementation-plans.md
- When plans reference Jira-sourced work items, include `jira_ref` in trace markers and note expected Jira status transitions per the implementation plans skill.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many implementation plans were created/updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/implementation-plans/P-XX-*.md` files, `_index.md`
- `evidence.plans_path`: `.memory-bank/implementation-plans/` (full path to directory)
- `evidence.plans_created`: count of new implementation plan files created
- `evidence.plans_updated`: count of existing implementation plan files updated
- `related_commands`: suggested follow-up commands
  - "To execute a step from the updated plans, run: `/axiom-step --work-item <id>`"
  - "To refresh the full roadmap, run: `/axiom-roadmap-refresh`"
  - "To view the plans index, read: `.memory-bank/implementation-plans/_index.md`"

### Cross-References
- "Implementation plan schema is in: `.opencode/skills/axiom-implementation-plans/SKILL.md`"
- "Plans index is at: `.memory-bank/implementation-plans/_index.md`"
- "TODO roadmap is at: `.memory-bank/TODO.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
