---
description: Build/refresh a Ralph loop scaffold for this repo.
agent: dispatch-axiom
---

Build or refresh a Ralph loop scaffold.

Inputs
- `$ARGUMENTS` optional: additional `ralph-wiggum-loop` args (e.g., `advanced meta_layer=1 harness=opencode runner=bash`)

Skills (load on demand):
- `axiom-copilot` — If the user is new to Ralph loops, load this to explain what the loop does, how to run it, and how to monitor/steer it.
- `ralph-wiggum-loop` — Always loaded (see step 2). Full loop generation rules.
- `axiom-operating-modes` — For understanding CLI vs automated vs full-auto execution.

Do
1) Read `AGENTS.md`, `.memory-bank/_index.md`, and `.memory-bank/work-items/_current.md` if present.
2) Load skill `ralph-wiggum-loop`.
3) Run it with safe defaults:
   - `advanced`
   - `meta_layer=1`
   - `harness=opencode`
   - `runner=bash`
   - `work_item=.memory-bank/work-items/_current.md`
   - plus any `$ARGUMENTS` overrides
4) Do not overwrite existing `PROMPT.md` or `ralph-loop.sh`; use `*.ralphgen.*` variants if needed.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-loop</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include `<files_changed>` (semicolon-separated) when files are written
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which loop scaffold files were created/updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `PROMPT.md` or `PROMPT.ralphgen.md`, `ralph-loop.sh` or `ralph-loop.ralphgen.sh`
- `evidence.prompt_path`: full path to the primary prompt file
- `evidence.runner_path`: full path to the runner script (ralph-loop.sh or variant)
- `related_commands`: suggested follow-up commands
  - "To start the Ralph loop, run: `bash ralph-loop.sh` or `bash ralph-loop.ralphgen.sh`"
  - "To update the prompt bundle after changes, run: `/axiom-prompt-update`"
  - "To view the current work item, read: `.memory-bank/work-items/_current.md`"

### Cross-References
- "Ralph loop rules are in: `.opencode/skills/ralph-wiggum-loop/SKILL.md`"
- "Operating modes are in: `.opencode/skills/axiom-operating-modes/SKILL.md`"
- "Active work item is at: `.memory-bank/work-items/_current.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
