---
description: Audit and update specs after new code lands. Produces an impact matrix, augments affected specs, and verifies alignment.
agent: tower-axiom
---

Analyze recent code changes and update specs to reflect new behavior. This is the "code landed,
now catch the specs up" command — the inverse of `/axiom-spec-extract` (which extracts specs
from scratch) and the complement to `/axiom-backfill-git` (which backfills memory from git).

Use it when:
- New features or fixes have landed and specs need updating
- After a sprint or batch of commits, to ensure specs stay current
- Before a release, to verify spec coverage of all changes
- As part of the onboarding pipeline (after backfill, before full onboarding)
- When a spec verifier flags drift between code and specs

**Key principle**: Augment mode only — never rewrite or delete existing spec content.
Show a `git diff` of what changed before committing so the human can review.

Inputs
- `$ARGUMENTS` optional: scope and options. Examples:
  - `since=HEAD~10` — analyze commits since this ref (default: `HEAD~20`)
  - `since=2026-01-01` — analyze commits since this date
  - `since=abc1234` — analyze commits since this SHA
  - `scope=src/api/**` — only analyze commits touching these paths
  - `specs=specs/30-External-API.md,specs/32-Security.md` — only refresh these specific specs
  - `dry-run` — show impact matrix and proposed changes without writing files
  - `depth=quick|standard|full` — analysis depth (default: `standard`)
    - `quick`: impact matrix only, no spec augmentation (~2 min)
    - `standard`: impact matrix + augment affected specs (~10 min)
    - `full`: impact matrix + augment + verify alignment + suggest new specs (~20 min)

Skills (load on demand):
- `spec-writing-axiom` — Load for spec augmentation. Governs style, scope fences, requirements clarity.
- `spec-kickoff-axiom` — Load if new specs need to be created (not just augmented).
- `conformance-testing-loop` — Load if verification of spec-to-code alignment is needed.
- `traceability-doctrine` — Load to ensure trace markers are added to augmented specs.

Do
1) Parse `$ARGUMENTS` for `since`, `scope`, `specs`, `dry-run`, and `depth`.
2) **Phase 1 — Impact Matrix**: determine which specs are affected by recent changes.
   a) Run `git log --oneline <since>..HEAD` to get the commit list.
   b) Run `git diff --name-only <since>..HEAD` to get changed files.
   c) Map changed files to specs using these heuristics:
      - `src/api/**` or `*.py` routes → `specs/30-External-API-And-Realtime.md`
      - `src/auth/**` or auth-related → `specs/32-Security-Hardening-Roadmap.md`
      - `src/cli/**` or CLI changes → `specs/29-Operating-Modes.md`
      - `src/db/**` or migrations → `specs/62-Structured-Data-Capture-PostgreSQL.md`
      - `.opencode/agents/**` → `specs/17-Memory-Bank-Maintainer-Agent.md` (if memory-bank agent)
      - `.opencode/commands/**` → `specs/13-Command-Registry.md`
      - `.opencode/skills/**` → relevant skill spec (check skill frontmatter for `primary_spec`)
      - `tests/**` → specs referenced in test file headers
      - Any file with `axiom:trace` markers → extract `spec=` refs from those markers
   d) Emit the impact matrix:
      ```
      IMPACT MATRIX (since HEAD~10):
        10 commits analyzed
        23 files changed
        Specs affected:
          specs/30-External-API-And-Realtime.md  (3 files changed)
          specs/32-Security-Hardening-Roadmap.md (2 files changed)
          specs/13-Command-Registry.md           (1 file changed)
        Specs unaffected: 23 specs
        New behavior detected (no spec coverage): src/new-feature/ (suggest new spec)
      ```
   e) If `depth=quick` or `dry-run`: stop here and emit the impact matrix only.

3) **Phase 2 — Spec Augmentation** (standard and full depth only):
   For each affected spec:
   a) Read the current spec file.
   b) Read the changed code files that map to this spec.
   c) Identify gaps: new behavior not covered by existing REQ-* or AC-* items.
   d) Augment the spec with new requirements, updated acceptance criteria, or clarifications.
      - Add new `REQ-*` items for new behavior.
      - Update existing `REQ-*` items if behavior changed.
      - Add `realized-by:` links pointing to the implementing code.
      - Add `axiom:trace` markers with `work_item=` and `spec=` refs.
      - **Never delete or rewrite existing requirements** — only add or clarify.
   e) Show a `git diff` of proposed changes before writing.
   f) If `dry-run`: show diff but do not write.

4) **Phase 3 — Verification** (full depth only):
   a) For each augmented spec, run a quick conformance check:
      - Does the code implement what the spec now says?
      - Are there any contradictions between old and new spec content?
   b) Flag any contradictions as `WARN` items for human review.
   c) Suggest new specs for behavior that has no spec coverage.

5) Write augmented specs to `specs/` (unless `dry-run`).
6) Update `specs/README.md` if new specs were created.
7) Write a refresh manifest to `.memory-bank/work-items/<work_item>/runs/<run_id>/spec-refresh-manifest.md`.

Stop conditions
- If `since` ref doesn't exist in git history: BLOCKED with suggestion to use a valid ref.
- If no specs are affected: emit "No specs affected" and stop (status: ok).
- If `dry-run`: emit impact matrix and proposed diffs, then stop.
- If a spec file has merge conflicts or is malformed: WARN and skip that spec.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-spec-refresh</command>`
  - `<status>ok|fail|blocked|partial</status>` — `ok` if complete; `partial` if some specs skipped; `fail` if impact matrix failed; `blocked` if prerequisites missing
  - `<summary>` one sentence: commits analyzed, specs affected, specs augmented
  - `<detailed_summary>` impact matrix + augmentation summary + verification results
  - `<evidence>` include:
    - `<files_changed>` paths to all augmented spec files
    - `<commits_analyzed>` count of commits in the `since` range
    - `<specs_affected>` count of specs with changes
    - `<specs_augmented>` count of specs actually updated
    - `<new_requirements_added>` count of new REQ-* items added
    - `<new_specs_suggested>` list of suggested new specs for uncovered behavior
    - `<contradictions_found>` count of contradictions flagged for human review
  - `<diagnostics>` for warnings (contradictions, skipped specs, uncovered behavior)
  - `<review.assumptions>` assumptions made during spec mapping
  - `<review.questions_for_human>` contradictions and new spec suggestions needing human decision
  - `<modify_plan>` false (spec refresh doesn't change execution plans)
  - `<memory_updates>` path to spec-refresh-manifest.md

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many commits were analyzed and how many specs were augmented.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL spec files created/modified (full paths, semicolon-separated)
- `evidence.specs_augmented`: list of spec file paths that were updated
- `evidence.new_specs_suggested`: list of suggested new spec paths for uncovered behavior
- `evidence.commits_analyzed`: count of commits in the `since` range
- `evidence.new_requirements_added`: count of new REQ-* items added
- `evidence.refresh_manifest_path`: full path to the spec-refresh-manifest.md
- `related_commands`: suggested follow-up commands
  - "To verify spec-to-code alignment after refresh, run: `/axiom-verify --work-item <id>`"
  - "To create new specs for uncovered behavior, run: `/axiom-spec-request <behavior-description>`"
  - "To run a full spec extraction, run: `/axiom-spec-extract`"

### Cross-References
- "Spec inventory is at: `specs/README.md`"
- "Refresh manifest is at: `.memory-bank/work-items/<id>/runs/<run_id>/spec-refresh-manifest.md`"
- "Companion command: `/axiom-spec-extract` (full extraction from scratch)"

axiom:trace work_item=DEX-302 spec=specs/29-Operating-Modes.md jira_ref=DEX-302 plan=phase-1/task-1-3/step-1-3-1 doc=.opencode/commands/axiom-spec-refresh.md commit=
