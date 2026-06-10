---
description: Post-install convergence point — validate scaffold, config, and integrations. Idempotent.
agent: dispatch-axiom
---

Validate and complete a Axiom installation. This is the convergence point: regardless of which install method was used (scaffold copy, submodule, template repo, or workspace setup), running `/axiom-setup` ensures the repo is fully configured and agents can operate.

Command lifecycle boundary (setup vs init vs bootstrap):
- `/axiom-init` — Creates the initial `.axiom/` and `.opencode/` directories from scratch. Use on a repo that has NEVER had Axiom. Runs once.
- `/axiom-bootstrap` — Creates TODO, implementation plans, and work-item artifacts for a repo that already has Axiom scaffold. Use after init or after a fresh clone of a Axiom-enabled repo.
- `/axiom-setup` (THIS COMMAND) — Post-install convergence. Validates everything is correct, fills gaps, checks integrations. Safe to re-run anytime. Use after init, after bootstrap, after pulling updates, or whenever you want to verify the installation is healthy.

Order: `init` → `bootstrap` → `setup` (but setup is idempotent and can be run at any point).

Goal
- Detect repo type and maturity level
- Validate all scaffold files are present (fill gaps if any are missing)
- Validate `.axiom/axiom.config.yaml` fields
- Run health checks on available backends (Jira MCP, GitHub MCP, etc.)
- Report status and suggest next steps
- Be **idempotent** — safe to re-run on an already-configured repo (only fills gaps, never overwrites)

Inputs
- Repo: $REPO (optional; default current repo)
- Minimal: $MINIMAL (optional; if "true", only validate `.opencode/` and `opencode.jsonc`)

Skills (load on demand):
- `axiom-repository-scaffold` — For the scaffold file manifest and validation rules.
- `axiom-onboarding` — If the repo needs full onboarding after setup.
- `axiom-copilot` — If the user is new and needs guidance on what to do next.

Spec contract: `specs/70-OpenCode-Plugin.md` REQ-PLG-011, REQ-PLG-012, REQ-PLG-017

Do

**If $MINIMAL is "true"**: Execute only steps 1 (detect) and 4 (validate opencode.jsonc). Skip steps 2, 3, 5, 6, 7 — only scaffold `.opencode/` directory and `opencode.jsonc` per REQ-PLG-017. Report minimal status and exit.

**Otherwise (full setup)**: Execute all steps below.

1) Detect repo type:
   - Language/framework (check for `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, etc.)
   - Monorepo vs single repo (check for `workspace.yaml`, multiple `go.mod`, Nx/Turborepo config)
   - Maturity level: fresh (no `.axiom/`), partial (some scaffold), complete (all scaffold present)
   - Report: "Detected: [language] [framework] [repo-type] — maturity: [level]"

2) Validate scaffold files (27 core + 15 memory bank skeleton per `axiom-repository-scaffold` skill):
   - Load the `axiom-repository-scaffold` skill to get the full file manifest.
   - Check each file in the manifest exists.
   - For missing files: create them from embedded templates (never overwrite existing files).
   - For existing files that are empty or corrupt (0 bytes): warn but do not overwrite — report as "needs attention".
   - Ensure `.axiom/sessions/` is in `.gitignore` (REQ-PLG-106a).
   - Report: "[N] scaffold files present, [M] created, [K] already existed, [J] need attention"

3) Validate `.axiom/axiom.config.yaml`:
   - `repo_id` is set and NOT `REPO_ID_PLACEHOLDER` — if placeholder, generate a UUID and set it.
   - `jira.project_key` is set and not null — if null, report as ⚠️ needs_attention with message: "Set `jira.project_key` in `.axiom/axiom.config.yaml` to enable Jira integration (e.g., `DEX`, `PROJ`)." Do NOT attempt interactive prompting — agents cannot prompt users mid-command.
   - `confidence` weights sum to 100 — if not, warn and suggest correction.
   - `version` field is `1` — warn if missing.
   - Report each field status: ✅ valid / ⚠️ needs attention / ❌ missing

4) Validate `opencode.jsonc`:
   - File exists and is valid JSONC
   - Has at least one agent configured
   - MCP server entries are present (warn if missing, don't fail)
   - **Jira runner projects**: If `jira.project_key` is set in `.axiom/axiom.config.yaml`, check whether `.opencode/prompts/jira-runner.md` exists and is listed in `opencode.jsonc` `prompts` array. If missing, report ⚠️ with message: "Add `.opencode/prompts/jira-runner.md` to your `opencode.jsonc` prompts array to enable KISS mode by default in Jira runner containers. See `.memory-bank/docs/kiss-mode.md`."
   - Report status

5) Run health checks on available backends (best-effort, don't fail if unavailable):
   - Jira MCP: attempt `GET /myself` or equivalent — report connected/unavailable
   - GitHub MCP: attempt to list repos — report connected/unavailable
   - Notion MCP: attempt to search — report connected/unavailable
   - Report: "Backends: Jira ✅ | GitHub ✅ | Notion ❌ (not configured)"

6) Check memory bank health:
   - `.memory-bank/_index.md` exists and is not empty
   - `.memory-bank/_prompt.md` exists
   - `.memory-bank/work-items/_current.md` exists
   - If any missing, create from templates (per scaffold skill)

6b) Check worktree directory health:
   - `.worktrees/` directory exists — if missing, create it
   - `.worktrees/_prompt.md` exists — if missing, create from template (worktree creation/merge/cleanup rules)
   - `.worktrees/_index.md` exists — if missing, create from template (empty active worktree inventory)
   - Report: "Worktrees: ✅ directory + governance files present" or "⚠️ created missing files"

7) Report status and suggest next steps:
   - Summary table: what was validated, what was created, what needs attention
   - If repo is fresh: suggest `/axiom-bootstrap` to create TODO and plans
   - If repo has specs but no plans: suggest `/axiom-meta-plan`
   - If repo is fully set up: report "Ready to run. Try `/axiom-step` or `morty run`."
   - If Jira is connected: suggest `/axiom-sync-jira` to sync work items

Output
- A structured status report with per-check results
- Any files created or modified (with paths)
- Suggested next commands based on repo state

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating the repo maturity level and what was created/repaired.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
- `evidence.scaffold_files_created`: count of new scaffold files created
- `evidence.scaffold_files_ok`: count of scaffold files already present
- `evidence.scaffold_files_attention`: count of files needing manual attention
- `evidence.backends_available`: list of available backends (jira|github|notion)
- `evidence.repo_maturity`: fresh|partial|complete
- `related_commands`: suggested follow-up commands based on repo state
  - "To bootstrap TODO and plans, run: `/axiom-bootstrap`"
  - "To start onboarding, run: `/axiom-onboarding`"
  - "To sync Jira work items, run: `/axiom-sync-jira`"

### Cross-References
- "Scaffold file manifest is in: `.opencode/skills/axiom-repository-scaffold/SKILL.md`"
- "Setup sequence: `/axiom-init` → `/axiom-bootstrap` → `/axiom-setup` → `/axiom-onboarding`"
- "Config file is at: `.axiom/axiom.config.yaml`"

axiom:trace work_item=DEX-292 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-011,REQ-PLG-012,REQ-PLG-017 jira_ref=DEX-292 work_item=command-quality-01
