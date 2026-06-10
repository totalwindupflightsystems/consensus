# Implementation Plans Folder Prompt

This folder contains project-level implementation plans aligned to `.memory-bank/TODO.md`.

If this repository also has `specs/` with real contracts, cite and follow those specs.
If `specs/` is missing or only contains stubs, treat the referenced `.opencode/skills/*/SKILL.md` documents as the portable contracts.

Rules
- Keep plans high-level and stable; link each plan to the corresponding TODO section.
- Every plan task should name:
  - verification command(s)
  - evidence path(s) (under `.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/`)
  - rollback note

Minimum contents (portable)
- Summary and scope fences (in/out)
- Explicit TODO checkbox mapping (1:1)
- Dependencies
- Verification section (exact commands/procedures)
- Evidence paths
- Traceability section linking TODO, specs (if present), and work items

Authoritative references (portable)
- `.opencode/skills/axiom-implementation-plans/SKILL.md`
- `.opencode/prompts/axiom-implementation-plans.md`
- `.opencode/skills/baby-steps-methodology/SKILL.md`
- `.opencode/skills/traceability-doctrine/SKILL.md`

Fail-closed rules
- Never claim a task is done without verification commands having been run and recorded as evidence in a work item run snapshot.
- If a verification command cannot be run, mark the task `blocked` and write exact “how to verify” steps.
