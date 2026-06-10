---
description: Refresh PROMPT.md and PROMPT-VERIFY.md (Ralph loop prompt bundle).
agent: dispatch-axiom
---

Refresh `PROMPT.md` and related prompt bundle artifacts.

Inputs
- `$ARGUMENTS` optional: guidance about what changed (stack changes, new commands, new invariants).

Skills (load on demand):
- `ralph-wiggum-loop` — Always load (see step 1). Full loop generation rules.
- `axiom-xml-protocol` — XML envelope format and required tag set.

Do
1) Load skill `ralph-wiggum-loop`.
2) Run it in `advanced` mode to regenerate prompt bundle:
   - if existing prompts exist, do not overwrite; write `PROMPT.ralphgen.md` (and related) instead.
3) If `$ARGUMENTS` is non-empty, incorporate it as "what changed" context.
4) Ensure docs mention the current attach-mode status (planned/disabled) and steering packet rules.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-prompt-update</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence>` include `<files_changed>` (semicolon-separated) when files are written
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which prompt files were created/updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `PROMPT.md` or `PROMPT.ralphgen.md`, `PROMPT-VERIFY.md` or `PROMPT-VERIFY.ralphgen.md`
- `evidence.prompt_path`: full path to the primary prompt file (PROMPT.md or PROMPT.ralphgen.md)
- `evidence.verify_prompt_path`: full path to the verify prompt file (if created)
- `related_commands`: suggested follow-up commands
  - "To run the Ralph loop with the updated prompt, use: `axiom run --work-item <id> --repo .`"
  - "To view the updated prompt, read: `PROMPT.md` or `PROMPT.ralphgen.md`"

### Cross-References
- "Ralph loop prompt rules are in: `.opencode/skills/ralph-wiggum-loop/SKILL.md`"
- "Prompt bundle files: `PROMPT.md`, `PROMPT-VERIFY.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
