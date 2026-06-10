---
name: axiom-onboarding
description: Onboard a newly installed Axiom repo or multi-repo workspace into runnable state by preparing TODO, implementation plans, onboarding work-item artifacts, Ralph loops, and prompt bundle. Detects workspace.yaml for multi-repo mode.
license: MIT
compatibility: opencode
metadata:
  workflow: onboarding
  outputs: ".memory-bank/TODO.md,.memory-bank/implementation-plans/,work-items,_current.md,PROMPT.md,PROMPT-VERIFY.md,ralph-loop.sh"
tags:
  vertical: [onboarding]
  category: onboarding
  core: false
---

## What I do

I turn a freshly installed (or partially configured) Axiom repository — or multi-repo workspace — into a loop-ready state.

I focus on these onboarding outcomes:
- `.memory-bank/TODO.md` exists and is aligned to `specs/`
- `.memory-bank/implementation-plans/` exists and matches TODO
- `.memory-bank/work-items/<WORK_ITEM_ID>/` has meta-planning + plan artifacts
- `.memory-bank/work-items/_current.md` points to the next smallest step
- Ralph loop artifacts exist (`PROMPT.md`, `PROMPT-VERIFY.md`, `ralph-loop.sh`, optional `ralph-meta-loop.sh`)

## When to use me

Use this when:
- Axiom was just installed and the team does not know the next steps
- The repo has specs and memory-bank skeletons, but no actionable loop setup
- You want a deterministic onboarding path that fails closed when prerequisites are missing
- A multi-repo workspace was just created with `workspace-setup.py` and needs per-repo bootstrapping

Do not use me for feature implementation; I only prepare execution scaffolding.

## Inputs

Parse `$ARGUMENTS` as optional `key=value` pairs:
- `goal=<text>` (default: derive from `specs/README.md` + `specs/00-PRD.md`)
- `work_item_id=<id>` (default: `onboarding-01`)
- `loop_mode=simple|advanced` (default: `advanced`)
- `meta_layer=0|1` (default: `1`)

If parsing fails, continue with defaults.

## Guardrails

- Fail closed on missing mandatory scaffold; do not claim success without file evidence.
- Never invent test/verification outputs.
- Prefer existing repo commands (`/axiom-*`) and existing skills over bespoke logic.
- Keep changes additive; do not overwrite user-authored files unless explicitly requested.
- If `PROMPT.md` or `ralph-loop.sh` already exists, accept `*.ralphgen.*` variants.

## Authoring principles I follow

1) Progressive disclosure: keep top-level instructions short, defer detail to existing commands/skills.
2) Clear trigger language: state exactly when this skill should be selected.
3) Deterministic checklist: every onboarding result maps to concrete file checks.
4) Reuse existing workflows: orchestrate `/axiom-init`, `/axiom-bootstrap`, `/axiom-todo`, `/axiom-implementation-plans`, `/axiom-loop`, `/axiom-prompt-update`.

## Workflow

### Step 1 - Preflight

Read:
- `AGENTS.md`
- `specs/README.md`
- `specs/00-PRD.md`
- `.memory-bank/_index.md`
- `.memory-bank/_prompt.md`

Then check workspace context:
- If `workspace.yaml` exists in the current directory or an ancestor, this is a multi-repo workspace.
  - Read `workspace.yaml` to discover member repos.
  - Also read `.memory-bank/_prompt.md` at the workspace root (write-location guardrails).
  - All subsequent steps target a specific member repo. If not specified, ask the user or default to the first repo.
  - All file paths are relative to the member repo root, NOT the workspace root.
- If the current directory contains 2+ git repos but no `workspace.yaml`, suggest running workspace setup:
  ```
  python3 <AXIOM_REPO>/.axiom/scaffold/workspace-setup.py --target <dir> --memory-bank-full --install-missing
  ```

Then check whether Axiom is installed:
- `.opencode/` exists (at workspace root in workspace mode, or at repo root in single-repo mode)
- `.axiom/` exists (at the target member repo root)

If not installed, stop and instruct the user to run the install flow first.

### Step 2 - Baseline bootstrap

If the repository is blank or missing core memory-bank files, run:
- `/axiom-init`

Then run:
- `/axiom-bootstrap`

Use:
- `WORK_ITEM_ID=<work_item_id>`
- `GOAL=<goal>` (or derived default)

### Step 3 - Roadmap refresh

Ensure roadmap artifacts are current:
- `/axiom-todo`
- `/axiom-implementation-plans`

If both commands already run as part of the bootstrap flow, only re-run when files are missing or stale.

### Step 4 - Work-item readiness

Ensure these exist for `<work_item_id>`:
- `.memory-bank/work-items/<work_item_id>/meta-planning.md`
- `.memory-bank/work-items/<work_item_id>/plan.md`
- `.memory-bank/work-items/<work_item_id>/plan.yaml`

If missing, run:
- `/axiom-work-item` with `WORK_ITEM_ID=<work_item_id>` and a concise objective.

Then ensure:
- `.memory-bank/work-items/_current.md`

`_current.md` should reference the active work-item folder and identify the next smallest step.

### Step 5 - Loop and prompt scaffold

Create or refresh Ralph loop artifacts:
- `/axiom-loop` with defaults:
  - `<loop_mode>` (default `advanced`)
  - `meta_layer=<meta_layer>` (default `1`)
  - `harness=opencode`
  - `runner=bash`
  - `work_item=.memory-bank/work-items/_current.md`

If prompt files need refresh after repo-specific changes, run:
- `/axiom-prompt-update`

### Step 6 - Lightweight verification gate

Verify onboarding outputs by checking for these paths:
- `.memory-bank/TODO.md`
- `.memory-bank/implementation-plans/_index.md`
- `.memory-bank/work-items/_current.md`
- `.memory-bank/work-items/<work_item_id>/plan.yaml`
- `.memory-bank/topics/axiom-glossary.md` (shared vocabulary — copy from scaffold if missing)
- `_tmp/` directory exists and is gitignored (workspace scratch space)
- `PROMPT.md` (or `PROMPT.ralphgen.md`)
- `ralph-loop.sh` (or `ralph-loop.ralphgen.sh`)
- `PROMPT-VERIFY.md` when `meta_layer=1`

Also verify the **test quality gate bootstrap**:
- `.axiom/scripts/check_test_quality.py` exists (copy from Axiom repo if missing).
- `axiom.config.yaml` contains `verification.test_quality` and `verification.runtime` keys.
  If missing, add the defaults from `specs/48-Test-Quality-Gates.md#configuration-schema`.
- Run the quality gate as a baseline check:
  ```bash
  python3 .axiom/scripts/check_test_quality.py \
    --test-dir .axiom/tests \
    --config .axiom/axiom.config.yaml \
    --json > .memory-bank/work-items/<work_item_id>/test-quality-baseline.json
  ```
  Record the baseline score. If hard failures exist, add them as the first work items
  in the TODO under a "Test Quality Cleanup" section.

If any required artifact is missing, return `blocked` with exact missing paths and next command.

## Output contract

End with a concise onboarding report:
- `STATUS: ok|fail|blocked`
- `WORK_ITEM_ID: <id>`
- `READY:` checklist of satisfied outcomes
- `MISSING:` checklist (or `none`)
- `NEXT COMMAND:` one command to start execution loop

Recommended next command:

`./ralph-loop.sh`

If `meta_layer=1`, also provide:

`./ralph-meta-loop.sh --single`
