---
description: Initialize Axiom in a blank repo or multi-repo workspace (create baseline structure).
agent: dispatch-axiom
---

Initialize a repository (or multi-repo workspace) so Axiom can operate.

If Axiom is not installed in this repo yet (no `.opencode/`), stop and instruct the user to install Axiom first.

Skills (load on demand):
- `axiom-copilot` — If the user is new, load this to explain what init does and walk them through the full setup sequence (init → bootstrap → specs → plan → execute).
- `axiom-install` — If Axiom isn't installed yet, load this for installation guidance.
- `multi-repo-coordinator-axiom` — If this is a multi-repo workspace.
- `axiom-repository-scaffold` — For the scaffold file manifest and template contents.

Do
1) Detect workspace context:
   - Check if `workspace.yaml` exists in the current directory or an ancestor. If yes, this is a multi-repo workspace — follow workspace rules from `specs/40-Multi-Repo-Workspace.md`.
   - Check if the current directory contains 2+ git repos as direct children (but no `workspace.yaml`). If yes, suggest running workspace setup first:
     ```
     python3 <AXIOM_REPO>/.axiom/scaffold/workspace-setup.py --target <this-dir> --memory-bank-full --install-missing
     ```
   - Otherwise, proceed with single-repo init.

2) Create the baseline repo structure (only if missing):
   - `specs/README.md`
   - `AGENTS.md`
   - `.axiom/axiom.config.yaml`
   - `.axiom/command-registry.yaml`
   - `.memory-bank/_index.md`
   - `.memory-bank/_prompt.md`
   - `.memory-bank/TODO.md`
   - `.memory-bank/projectBrief.md`
   - `.memory-bank/productContext.md`
   - `.memory-bank/systemPatterns.md`
   - `.memory-bank/techContext.md`
   - `.memory-bank/decisionLog.md`
   - `.memory-bank/activeContext.md`
   - `.memory-bank/progress.md`
   - `.memory-bank/work-items/_index.md`
   - `.memory-bank/work-items/_prompt.md`
   - `.memory-bank/implementation-plans/_index.md`
   - `.memory-bank/implementation-plans/_prompt.md`

   Keep the content minimal and portable. Never overwrite existing files unless the user explicitly asks.

3) Then run `/axiom-bootstrap`.

Output
- A checklist of files created/skipped.
- If workspace context detected: workspace status and member repo list.
- The next command to run (`/axiom-bootstrap`).

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many files were created and what the next step is.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
- `evidence.files_created`: count of new files created
- `evidence.files_skipped`: count of files that already existed (not overwritten)
- `evidence.workspace_mode`: true|false — whether workspace mode was detected
- `related_commands`: suggested follow-up commands
  - "To complete setup, run: `/axiom-bootstrap`"
  - "To validate the installation, run: `/axiom-setup`"
  - "To start onboarding, run: `/axiom-onboarding`"

### Cross-References
- "Scaffold file manifest is in: `.opencode/skills/axiom-repository-scaffold/SKILL.md`"
- "Setup sequence: `/axiom-init` → `/axiom-bootstrap` → `/axiom-setup` → `/axiom-onboarding`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
