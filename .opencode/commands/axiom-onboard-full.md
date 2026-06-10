---
description: End-to-end onboarding pipeline — chains init, spec-extract, git-backfill, incremental catchup, and spec-refresh into one command.
agent: tower-axiom
---

Run the complete Axiom onboarding pipeline for a repo that has code and git history but no Axiom setup.

Each phase is idempotent — if a phase was already completed, it is skipped. The pipeline can resume from any phase.

Use it when:
- Onboarding a new repo into Axiom for the first time
- Resuming a partial onboarding that was interrupted
- Running a full refresh after a long period of inactivity

Inputs
- `$ARGUMENTS` optional:
  - `project=myapp` — project name (default: derived from repo name)
  - `from=<phase>` — resume from a specific phase: `init|spec-extract|backfill|incremental|spec-refresh` (default: auto-detect from state)
  - `skip=<phase>` — skip a specific phase (comma-separated, e.g., `skip=incremental,spec-refresh`)
  - `batch_size=N` — batch size for git backfill (default: 15)
  - `depth=quick|standard|full` — analysis depth for spec-extract and backfill (default: standard)
  - `dry-run` — show what would be run without executing

Skills (load on demand):
- `axiom-onboarding` — Load for onboarding context and conventions.
- `git-history-backfill-axiom` — Load for backfill phase.

Do
1) Parse `$ARGUMENTS` for project, from, skip, batch_size, depth, dry-run.
2) Detect current onboarding state:
   - Phase `init` is done if `.opencode/` and `.memory-bank/` exist.
   - Phase `spec-extract` is done if `specs/` has at least one non-README file.
   - Phase `backfill` is done if `.memory-bank/projects/<project>/backfill-manifest.md` exists.
   - Phase `incremental` is done if backfill cursor is up to date with HEAD.
   - Phase `spec-refresh` is done if a spec-refresh manifest exists newer than the last commit.
3) Build the phase queue: ordered list of phases not yet done (or specified via `from=`).
4) If `dry-run`: emit the phase queue and stop.
5) Execute each phase in order:

   **Phase 1: Init** (skip if already done)
   - Run `/axiom-init` (or verify scaffold is present)
   - Emit: "Phase 1/5: Init ✓ (scaffold already present)" or "Phase 1/5: Init — running..."

   **Phase 2: Spec Extract** (skip if already done)
   - Run `/axiom-spec-extract depth=<depth>`
   - Emit: "Phase 2/5: Spec extract — analyzing codebase..."
   - On completion: "Phase 2/5: Spec extract ✓ (<N> specs extracted)"

   **Phase 3: Git Backfill** (skip if already done)
   - Run `/axiom-backfill-git batch_mode=true batch_size=<batch_size> depth=<depth>`
   - Emit: "Phase 3/5: Git backfill — processing history in batches of <batch_size>..."
   - On completion: "Phase 3/5: Git backfill ✓ (<N> commits processed, <M> eras detected)"

   **Phase 4: Incremental Catchup** (skip if cursor is up to date)
   - Run `/axiom-backfill-git incremental`
   - Emit: "Phase 4/5: Incremental catchup — checking for new commits..."
   - On completion: "Phase 4/5: Incremental catchup ✓ (<N> new commits processed)" or "Phase 4/5: Incremental catchup ✓ (already up to date)"

   **Phase 5: Spec Refresh** (skip if already done)
   - Run `/axiom-spec-refresh since=<backfill_start_sha> depth=<depth>`
   - Emit: "Phase 5/5: Spec refresh — updating specs..."
   - On completion: "Phase 5/5: Spec refresh ✓ (<N> specs updated)"

6) Emit the final onboarding report:
   ```
   ONBOARDING COMPLETE
   ==================
   Project: <project>
   Phases completed: 5/5
   Specs: <N> specs in specs/
   Git history: <N> commits processed, <M> eras detected
   Memory bank: <N> files created/updated
   Gaps found: <N> (see spec-refresh output for details)
   
   Next steps:
   1. Review extracted specs in specs/ — validate INFERRED items
   2. Run /axiom-onboarding to set up TODO, plans, and loops
   3. Start working — agents now have full context
   ```

Stop conditions
- If any phase fails: stop, report which phase failed and why, suggest how to resume.
- If `dry-run`: emit phase queue and stop.
- If a phase is skipped via `skip=`: note it in the report.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-onboard-full</command>`
  - `<status>ok|fail|blocked|partial</status>`
  - `<summary>` one sentence: phases completed, specs extracted, commits processed
  - `<detailed_summary>` full onboarding report
  - `<evidence>` include:
    - `<phases_completed>` count
    - `<phases_skipped>` count and which ones
    - `<phases_failed>` count and which ones
    - `<specs_extracted>` count
    - `<commits_processed>` count
    - `<memory_bank_files>` count
  - `<diagnostics>` for warnings (phases skipped, partial completion)
  - `<review.questions_for_human>` INFERRED spec items needing validation
  - `<modify_plan>` false
  - `<memory_updates>` path to onboarding report in memory bank

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many phases completed and the overall onboarding status.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: union of ALL files created/modified across all phases (full paths, semicolon-separated)
  - Typically: spec files, `.memory-bank/projects/<project>/*.md`, `specs/*.md`
- `evidence.phases_completed`: count of phases that completed successfully
- `evidence.phases_skipped`: count of phases that were skipped (already done)
- `evidence.specs_extracted`: count of spec files created/updated
- `evidence.commits_processed`: count of git commits analyzed
- `evidence.memory_bank_files`: count of memory bank files created/updated
- `related_commands`: suggested follow-up commands
  - "To review extracted specs, read: `specs/README.md`"
  - "To run full onboarding (TODO, plans, loop), run: `/axiom-onboarding`"
  - "To validate the installation, run: `/axiom-setup`"

### Cross-References
- "Onboarding pipeline phases: init → spec-extract → backfill → incremental → spec-refresh"
- "Spec inventory is at: `specs/README.md`"
- "Memory bank is at: `.memory-bank/`"

axiom:trace work_item=DEX-299 spec=specs/29-Operating-Modes.md jira_ref=DEX-305 plan=phase-3/task-3-1/step-3_1_1 doc=.opencode/commands/axiom-onboard-full.md commit= work_item=command-quality-01