---
description: One-command kickoff for a new request (specs + work item + roadmap + loop).
agent: dispatch-axiom
---

Kick off a new request with minimal typing.

Inputs
- `$WORK_ITEM_ID` optional.
- `$ARGUMENTS` required: the request.

Skills (load on demand):
- `axiom-copilot` — If the user seems new or unsure, load this skill to walk them through what kickoff does and why each step matters.
- `spec-kickoff-axiom` — For iterative spec refinement from minimal input.
- `axiom-capability-surface` — If the request is vague, load this to understand what Axiom can do.

Do (fail-closed)
1) Run `/axiom-spec-request $ARGUMENTS`.
2) Run `/axiom-work-item` for `$WORK_ITEM_ID` and `$ARGUMENTS`.
3) Run `/axiom-roadmap-refresh`.
4) Run `/axiom-loop`.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-kickoff</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include `<commands_run>` (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the work item ID created and whether the loop is ready.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: union of all files created/modified across all sub-steps (full paths, semicolon-separated)
  - Typically: spec files, `.memory-bank/work-items/<id>/meta-planning.md`, `plan.md`, `plan.yaml`, `.memory-bank/TODO.md`, `PROMPT.md`
- `evidence.work_item_id`: the stable work item ID created
- `evidence.spec_paths`: list of spec files created/updated
- `evidence.plan_path`: full path to `plan.yaml`
- `evidence.loop_ready`: true|false — whether the Ralph loop scaffold was created
- `related_commands`: suggested follow-up commands
  - "To execute the first step, run: `/axiom-step --work-item <id>`"
  - "To run all steps in sequence, run: `/axiom-step-loop --work-item <id>`"
  - "To view the plan, read: `.memory-bank/work-items/<id>/plan.md`"

### Cross-References
- "Sub-commands: `/axiom-spec-request`, `/axiom-work-item`, `/axiom-roadmap-refresh`, `/axiom-loop`"
- "Work item artifacts are at: `.memory-bank/work-items/<id>/`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
