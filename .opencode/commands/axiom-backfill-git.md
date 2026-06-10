---
description: Backfill memory bank context from git commit history for repos onboarding into Axiom.
agent: dispatch-axiom
---

Read the repository's git history oldest-to-newest and generate memory bank files that give
agents historical context about a project that didn't start with Axiom.

This is the "memory transplant" companion to `/axiom-spec-extract` (which backfills specs
from code). Together they give a complete onboarding picture: specs from code + memory from git.

Inputs
- `$ARGUMENTS` optional: scope and options. Examples:
  - `project=myapp` — project name for memory bank paths (default: derived from repo name)
  - `since=2024-01-01` — only analyze commits after this date
  - `scope=src/**` — only analyze commits touching these paths
  - `depth=full|standard|quick` — analysis depth (default: `standard`)
    - `quick`: tags + dependency changes + first/last commits only (~2 min)
    - `standard`: all high-signal commits + era detection (~10 min)
    - `full`: every commit analyzed, richest narrative (~30 min for large repos)
  - `no-contributors` — skip contributor map generation (privacy: avoids exposing emails)
  - `dry-run` — show what would be generated without writing files
  - `batch_size=N` — number of commits per batch (default: 15; use 5 for very large commits, 50 for small repos)
  - `batch_mode=true` — force batch processing even for small repos (default: auto-enabled for 50+ commits)
  - `incremental` — only process commits since the last backfill cursor (see DEX-301; requires prior backfill run)

Skills (load on demand):
- `git-history-backfill-axiom` — Always load. Core backfill method, signal detection, batch processing, and output format.
- `axiom-onboarding` — Load if the user wants to continue into full onboarding after backfill.
- `decision-archaeology-axiom` — Load if a specific decision needs deeper investigation than git alone provides.

Do
1) Load skill `.opencode/skills/git-history-backfill-axiom/SKILL.md`.
2) Verify the repo has git history (`git log --oneline -1`). If no commits, emit BLOCKED.
3) Ensure `.memory-bank/` exists. If not, create the minimal scaffold (or suggest running `/axiom-init` first).
4) Parse `$ARGUMENTS` for project name, date range, scope, depth, batch_size, batch_mode, and incremental.
5) Determine batch mode:
   - If `batch_mode=true` OR commit count ≥ 50: use batch processing protocol from the skill.
   - Otherwise: process all commits in a single pass.

   **If `incremental` argument is set:**
   a) Find the most recent backfill work item by scanning `.memory-bank/work-items/` for folders
      containing a `plan.yaml` with `cursor.last_processed_sha` set.
      - Look for work items with IDs matching: `backfill-*`, `git-backfill-*`, or the project name.
      - If multiple found, use the most recently modified one.
   b) Read `cursor.last_processed_sha` from that plan.yaml.
   c) Count new commits: `git rev-list <last_sha>..HEAD --count`
   d) If count == 0: emit "Already up to date — last processed commit: <sha> (<date>)" and stop (status: ok).
   e) If count > 0:
      - Emit: "Found <N> new commits since last backfill (SHA <short_sha>). Creating catchup work item..."
      - Create a new work item ID: `<project>-backfill-catchup-<YYYYMMDD>`
      - Create `.memory-bank/work-items/<catchup_id>/plan.yaml` with the cursor set to last_sha
      - Process only commits from last_sha..HEAD using batch mode (batch_size from args or default 15)
   f) If no prior cursor found: warn "No prior backfill cursor found. Falling back to full backfill." and proceed with full backfill.
6) If batch mode is active, execute the **Batch Processing Protocol** from the skill:
   a) **Triage Phase**: scan the batch with `git show --stat`, classify each commit as WRITE/SKIP/AMBIGUOUS.
      Emit the triage table before writing any files.
   b) **Process WRITE commits**: full signal detection + narrative construction.
   c) **Record SKIP commits**: lightweight YAML entry only (no full memory bank note).
   d) **Process AMBIGUOUS commits**: reduced depth; promote to WRITE if touching high-signal paths.
   e) **Write consolidated batch evidence**: one `verification.md` + `skipped.yaml` + `triage-table.md` per batch.
   f) **Advance cursor**: update `plan.yaml` with `last_processed_sha` after each batch.
7) If single-pass mode, execute the standard backfill method:
   a) **Phase 1 — Git Log Extraction**: read commit history oldest-to-newest using `git log --reverse`.
      For large repos (1000+ commits) at `standard` depth, sample strategically:
      all tags, all dependency manifest changes, all migration files, first/last 50 per year,
      and any commit with 10+ files changed.
   b) **Phase 2 — Signal Detection**: identify high-signal commits (architecture shifts,
      dependency changes, migrations, CI changes, releases, contributor changes, reverts, hotfixes).
   c) **Phase 3 — Narrative Construction**: group commits into eras, build chronological narrative,
      infer decisions from significant changes.
   d) **Phase 4 — Memory Bank File Generation**: produce timeline, decision log seed, tech context
      seed, contributor map, milestone markers, and backfill manifest.
8) Write generated files to `.memory-bank/projects/<project>/` and augment root-level files
   (`decisionLog.md`, `techContext.md`). Never overwrite existing content — append with
   `## Backfilled from Git History` sections.
9) Update `.memory-bank/projects/<project>/_index.md` and `.memory-bank/_index.md` if needed.
10) Produce the backfill manifest with confidence levels and known gaps.

Stop conditions
- If the repo has no git history: BLOCKED.
- If `.memory-bank/` doesn't exist and can't be created: BLOCKED with suggestion to run `/axiom-init`.
- If `dry-run` is specified: show what would be generated (including triage table if batch mode) and stop.
- If `incremental` is set but no prior cursor exists: warn and fall back to full backfill.
- If commit messages are uniformly poor (>80% are single-word): still proceed but set overall
  confidence to "low" and note the limitation prominently.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-backfill-git</command>`
  - `<status>ok|fail|blocked|partial</status>` — `ok` if complete; `partial` if batch mode and more batches remain; `fail` if git history is unreadable; `blocked` if prerequisites missing
  - `<summary>` one sentence: commits analyzed, eras detected, files generated (or batch progress if partial)
  - `<detailed_summary>` backfill report: eras, decisions inferred, confidence assessment, gaps
  - `<evidence>` include:
    - `<files_changed>` paths to all generated/updated memory bank files
    - `<commits_analyzed>` count of commits processed
    - `<commits_skipped>` count of SKIP-classified commits (batch mode only)
    - `<date_range>` oldest to newest commit date
    - `<eras_detected>` count of eras identified
    - `<decisions_inferred>` count of decisions added to decision log
    - `<contributors_found>` count of unique contributors
    - `<overall_confidence>` low|medium|high
   - `<batch_cursor>` last processed SHA (batch mode only; enables incremental catchup)
     - `<batch_progress>` "3 of 7 batches complete" (batch mode only)
     - `<incremental_mode>` true|false — whether incremental/catchup mode was used
     - `<new_commits_found>` count of new commits since last backfill (incremental mode only)
## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many commits were processed and how many eras were detected.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/projects/<project>/timeline.md`, `decision-log-seed.md`, `tech-context-seed.md`, `contributor-map.md`, `backfill-manifest.md`
- `evidence.memory_bank_files`: list of ALL memory bank files created/updated
- `evidence.commits_analyzed`: count of commits processed
- `evidence.eras_detected`: count of eras identified
- `evidence.decisions_inferred`: count of decisions added to decision log
- `evidence.batch_cursor`: last processed SHA (batch mode only; enables incremental catchup)
- `related_commands`: suggested follow-up commands
  - "To continue with incremental catchup, run: `/axiom-backfill-git incremental`"
  - "To extract specs from the codebase, run: `/axiom-spec-extract`"
  - "To run the full onboarding pipeline, run: `/axiom-onboard-full`"

### Cross-References
- "Backfill methodology is in: `.opencode/skills/git-history-backfill-axiom/SKILL.md`"
- "Memory bank files are at: `.memory-bank/projects/<project>/`"
- "Companion command: `/axiom-spec-extract` (backfills specs from code)"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
