---
description: One-command bootstrap: scaffold checks, TODO/plans, and Ralph loop. Workspace-aware.
agent: dispatch-axiom
---

Bootstrap this repo (or multi-repo workspace) for Axiom execution.

Goal
- Make the repo "ready to run" Axiom with:
  - `.memory-bank/` present and navigable
  - `.memory-bank/TODO.md` created/updated from existing specs/docs
  - `.memory-bank/implementation-plans/` created/updated
  - a runnable Ralph loop scaffold present (`PROMPT.md` + runner script)

Inputs
- Repo: $REPO (optional; default current repo)
- Goal: $GOAL (optional; what the user wants to build)
- Work item id: $WORK_ITEM_ID (optional; default `onboarding-01`)

Skills (load on demand):
- `axiom-copilot` — If the user is new, load this to explain what bootstrap does and what comes next.
- `axiom-onboarding` — For the full onboarding workflow (bootstrap is one step within it).
- `ralph-wiggum-loop` — For understanding the Ralph loop artifacts that bootstrap creates.

Do
0) Detect workspace context:
   - If `workspace.yaml` exists in the current directory or an ancestor, this is a multi-repo workspace.
   - Read `workspace.yaml` to discover member repos and their slugs.
   - In workspace mode, bootstrap targets a specific member repo. If $REPO is not set, ask the user which repo to bootstrap (or bootstrap all).
   - If the current directory contains 2+ git repos but no `workspace.yaml`, suggest running workspace setup first:
     ```
     python3 <AXIOM_REPO>/.axiom/scaffold/workspace-setup.py --target <this-dir> --memory-bank-full --install-missing
     ```
   - In workspace mode, all file paths below are relative to the target member repo root (e.g., `repos/<slug>/` or `<slug>/`), NOT the workspace root.

1) Validate scaffold exists (fail closed if missing):
   - `.axiom/` exists
   - `.opencode/` exists
   - `.memory-bank/_index.md` and `.memory-bank/_prompt.md` exist
   - `specs/README.md` exists (or create minimal stub)

2) Ensure Memory Bank subfolders exist (create if missing):
   - `.memory-bank/work-items/_index.md`
   - `.memory-bank/work-items/_prompt.md`
   - `.memory-bank/implementation-plans/_index.md`
   - `.memory-bank/implementation-plans/_prompt.md`

3) Create/update the onboarding work item plan:
   - If $WORK_ITEM_ID is empty, set it to `onboarding-01`.
   - Run `/axiom-meta-plan` with:
     - `WORK_ITEM_ID=$WORK_ITEM_ID`
     - `GOAL=$GOAL` (or default: "Bootstrap this repo for Axiom; derive TODO and implementation plans from existing specs/docs; prepare a runnable Ralph loop.")

4) Create a default Ralph work item file if missing:
   - `.memory-bank/work-items/_current.md`
   - It should point at `.memory-bank/work-items/$WORK_ITEM_ID/` and name the next smallest step.

5) Generate/update roadmap artifacts:
   - Run `/axiom-todo` to (re)build `.memory-bank/TODO.md` from the repo's specs/docs.
   - Run `/axiom-implementation-plans` to (re)build `.memory-bank/implementation-plans/` aligned to TODO.

6) Verify spec alignment (light gate):
   - Run `/axiom-verify` with `WORK_ITEM_ID=$WORK_ITEM_ID`.

7) Generate the Ralph loop scaffold:
   - In workspace mode, pass `repo=<slug>` to target the correct member repo.
   - Run `/ralph-wiggum-loop advanced harness=opencode runner=bash meta_layer=1 work_item=.memory-bank/work-items/_current.md`

Output
- A short checklist of what is now ready and what is still missing.
- If workspace mode: which member repo was bootstrapped and workspace status.
- Paths to:
  - `.memory-bank/TODO.md`
  - `.memory-bank/implementation-plans/_index.md`
  - `.memory-bank/work-items/_current.md`
  - `PROMPT.md` and `ralph-loop.sh` (or `*.ralphgen.*` variants)

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating whether the repo is bootstrapped and ready for loop execution.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/TODO.md`, `.memory-bank/implementation-plans/*.md`, `.memory-bank/work-items/<id>/*.md`, `.memory-bank/work-items/_current.md`, `PROMPT.md`, `ralph-loop.sh`
- `evidence.work_item_id`: the onboarding work item ID
- `evidence.todo_path`: `.memory-bank/TODO.md` (full path)
- `evidence.plans_path`: `.memory-bank/implementation-plans/` (full path)
- `evidence.current_path`: `.memory-bank/work-items/_current.md` (full path)
- `evidence.loop_ready`: true|false — whether the Ralph loop scaffold was created
- `related_commands`: suggested follow-up commands
  - "To start the Ralph loop, run: `bash ralph-loop.sh`"
  - "To validate the installation, run: `/axiom-setup`"
  - "To run a full onboarding pipeline, run: `/axiom-onboard-full`"

### Cross-References
- "Onboarding workflow is in: `.opencode/skills/axiom-onboarding/SKILL.md`"
- "Setup sequence: `/axiom-init` → `/axiom-bootstrap` → `/axiom-setup` → `/axiom-onboarding`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
