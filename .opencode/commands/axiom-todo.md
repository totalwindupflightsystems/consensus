---
description: Create/update `.memory-bank/TODO.md` (project roadmap) aligned to specs.
agent: pm-axiom
---

Maintain the Axiom project TODO.

Context files:
- `specs/README.md`
- `.memory-bank/_index.md`
- `.memory-bank/TODO.md`

Skills (load on demand):
- `axiom-copilot` — If the user is new, load this to explain what TODO.md is and how it fits into the Axiom lifecycle.
- `axiom-todo` — Detailed TODO maintenance rules.
- `todo-archive-scripts` — For archiving completed TODO blocks.

Instructions:
- Follow the prompt in @.opencode/prompts/axiom-todo.md

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many TODO items were added/updated/archived.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: `.memory-bank/TODO.md` (full path) if modified
- `evidence.todo_path`: `.memory-bank/TODO.md` (full path)
- `evidence.items_added`: count of new TODO items added
- `evidence.items_updated`: count of existing TODO items updated
- `evidence.items_archived`: count of TODO items archived
- `related_commands`: suggested follow-up commands
  - "To refresh implementation plans from the updated TODO, run: `/axiom-roadmap-refresh`"
  - "To execute the highest-priority TODO item, run: `/axiom-step --work-item <id>`"

### Cross-References
- "TODO format is defined in: `.opencode/skills/axiom-todo/SKILL.md`"
- "TODO archive operations: `axiom todo archive --work-item <id> --phase <phase>`"
- "Implementation plans are at: `.memory-bank/implementation-plans/`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
