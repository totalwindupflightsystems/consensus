---
description: Run onboarding flow after Axiom install (TODO, plans, work-item, loop prompts).
agent: dispatch-axiom
---

Onboard this repository for loop-driven execution after Axiom installation.

Inputs
- `$ARGUMENTS` optional key/value overrides passed to the skill (`goal=...`, `work_item_id=...`, `loop_mode=...`, `meta_layer=...`).

Skills (load on demand):
- `axiom-onboarding` — Always load. Full onboarding workflow, artifact checklist, and readiness gates.
- `axiom-xml-protocol` — XML envelope format and required tag set.

Do
1) Load skill `axiom-onboarding`.
2) Execute its workflow with safe defaults.
3) Fail closed if required onboarding artifacts are missing.

Output
- Short readiness report with:
  - status (`ok|fail|blocked`)
  - work item id
  - ready artifacts
  - missing artifacts
  - next command to start the loop

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating whether the repo is ready for loop execution and what's missing.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/TODO.md`, `.memory-bank/implementation-plans/*.md`, `.memory-bank/work-items/<id>/*.md`, `PROMPT.md`
- `evidence.work_item_id`: the onboarding work item ID
- `evidence.ready_artifacts`: list of artifact paths that are ready
- `evidence.missing_artifacts`: list of artifact paths that are still missing
- `evidence.loop_ready`: true|false — whether the Ralph loop scaffold is ready
- `related_commands`: suggested follow-up commands
  - "To start the Ralph loop, run: `bash ralph-loop.sh`"
  - "To execute the first step, run: `/axiom-step --work-item <id>`"
  - "To run a full onboarding pipeline, run: `/axiom-onboard-full`"

### Cross-References
- "Onboarding workflow is in: `.opencode/skills/axiom-onboarding/SKILL.md`"
- "Active work item is at: `.memory-bank/work-items/_current.md`"
- "For a fresh repo, run: `/axiom-init` then `/axiom-bootstrap` then `/axiom-onboarding`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
