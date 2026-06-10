---
description: Refresh TODO and implementation plans from current specs/memory.
agent: dispatch-axiom
---

Refresh roadmap artifacts.

Skills (load on demand):
- `axiom-todo` — TODO.md format, spec-aligned roadmap rules, and checkbox conventions.
- `axiom-implementation-plans` — Implementation plan schema and alignment rules.
- `axiom-xml-protocol` — XML envelope format and required tag set.

Do
1) Run `/axiom-todo` to update `.memory-bank/TODO.md`.
2) Run `/axiom-implementation-plans` to update `.memory-bank/implementation-plans/`.
3) Run `/axiom-sync-indexes` to keep `_index.md` inventories aligned.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-roadmap-refresh</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include `<files_changed>` (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many TODO items and implementation plans were updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/TODO.md`, `.memory-bank/implementation-plans/*.md`, updated `_index.md` files
- `evidence.todo_path`: `.memory-bank/TODO.md` (full path)
- `evidence.plans_path`: `.memory-bank/implementation-plans/` (full path to directory)
- `evidence.plans_updated`: count of implementation plan files updated
- `related_commands`: suggested follow-up commands
  - "To execute the next step from the refreshed TODO, run: `/axiom-step --work-item <id>`"
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To view the current roadmap, read: `.memory-bank/TODO.md`"

### Cross-References
- "TODO format is defined in: `.opencode/skills/axiom-todo/SKILL.md`"
- "Implementation plan schema is in: `.memory-bank/implementation-plans/_prompt.md`"
- "To create a new work item plan, run: `/axiom-meta-plan --work-item <id>`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
