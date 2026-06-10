---
description: Get best-practices guidance for this repo and store it in Memory Bank.
agent: dispatch-axiom
---

Generate best-practices guidance and persist it durably.

Inputs
- `$ARGUMENTS` optional: what area to focus on (testing, security, perf, docs, UI, etc.)

Skills (load on demand):
- `best-practices-axiom` — Always load. Patterns, testing bar, anti-patterns, and checklists for the stack.
- `enterprise-testing-standard` — Load when $ARGUMENTS includes "testing" or "qa".
- `security-review-axiom` — Load when $ARGUMENTS includes "security".

Do
1) Call `@best-practices-axiom` with repo context and `$ARGUMENTS`.
2) Store the output under `.memory-bank/topics/` or a relevant project folder.
3) Update `.memory-bank/_index.md` so it's discoverable.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-best-practices</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include `<files_changed>` (semicolon-separated) when notes are written
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating what area was covered and where the notes were stored.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/topics/<area>-best-practices.md` or similar
- `evidence.notes_path`: full path to the primary best-practices notes file
- `evidence.area_covered`: which area was analyzed (testing, security, perf, etc.)
- `related_commands`: suggested follow-up commands
  - "To apply best practices to a specific work item, run: `/axiom-step --work-item <id>`"
  - "To run an adversarial review using these best practices, run: `/axiom-adversary --target current`"

### Cross-References
- "Best practices notes are stored in: `.memory-bank/topics/`"
- "For testing-specific guidance, see: `.opencode/skills/enterprise-testing-standard/SKILL.md`"
- "For security-specific guidance, see: `.opencode/skills/security-review-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
