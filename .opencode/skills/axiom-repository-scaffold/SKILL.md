---
name: axiom-repository-scaffold
description: Portable repository scaffold and bootstrapping contract for Axiom — installation methods, scaffold file manifest, template contents, validation rules, install-refresh lifecycle, and post-install onboarding.
version: "1.0"
synopsis: |
  Defines how any repository becomes Axiom-ready. Covers three installation methods (scaffold
  script copy, git submodule + symlinks, multi-repo workspace), the full scaffold file manifest
  (27+ files), idempotent install behavior, install-refresh lifecycle per work item, ownership
  manifest enforcement, autonomous runtime onboarding, and post-install guided onboarding.
when-to-use: |
  Load this skill when bootstrapping a new repo for Axiom, running the scaffold installer,
  validating scaffold completeness, implementing install-refresh lifecycle, designing autonomous
  onboarding flows, or troubleshooting missing scaffold files.
tags:
  vertical: [onboarding, coding]
  category: onboarding
  core: false
---

# Repository Scaffold and Bootstrapping (Portable)

This skill defines how any repository becomes Axiom-ready.

Source spec: `specs/16-Repository-Scaffold.md`

---

## Two Contexts

Axiom operates in two contexts:

| Context | Description |
|---|---|
| **Building Axiom** (this repo) | `.opencode/` and `opencode.jsonc` exist to help agents work on Axiom itself |
| **Using Axiom in another repo** | Target repo must be scaffolded with `specs/`, `.memory-bank/`, `.axiom/`, `.opencode/`, `AGENTS.md` |

---

## Installation Method Decision Table

| Condition | Recommended Method | Reference |
|---|---|---|
| New repo, standalone files, no upstream sync | **Scaffold script** (copy) | `specs/16-Repository-Scaffold.md` |
| Want agents/commands to auto-update from upstream | **Git submodule + symlinks** | `specs/29-Operating-Modes.md` |
| Multiple repos under one parent directory | **Multi-repo workspace** | `specs/40-Multi-Repo-Workspace.md` |
| `.opencode/` already exists | **Scaffold with `--scaffold-only`** | This skill |
| Automated runtime, repo not yet Axiom-ready | **Autonomous onboarding** (runtime auto-scaffolds) | `specs/44-Autonomous-Intake-And-Lifecycle.md` |

All methods produce the same per-repo result: `.opencode/`, `.axiom/`, `.memory-bank/`, and `AGENTS.md` ready for agents.

---

## Scaffold Script CLI

```
python .axiom/scaffold/install.py --target /path/to/repo
```

### Options

| Flag | Default | Description |
|---|---|---|
| `--target` | (required) | Path to the target repo root |
| `--repo-id` | derived from git remote or dir name | `org/repo` identifier for config |
| `--scaffold-only` | false | Only install scaffold templates (skip `.opencode/` infrastructure) |
| `--memory-bank-full` | false | Install full `.memory-bank/` skeleton (agents/inbox/projects/topics/best-practices/known-gaps) |
| `--force` | false | Overwrite existing files (default: skip existing) |
| `--dry-run` | false | Print what would be created without writing |
| `--verbose` | false | Show skipped files in output |

### Behavior Rules

| Rule | Detail |
|---|---|
| **Idempotent** | Safe to re-run; existing files are SKIPPED (never overwritten unless `--force`) |
| **No network** | All templates are bundled; no downloads during install |
| **Logging** | Logs `CREATE` for new files, `SKIP` for existing files |
| **Exit code** | 0 on success (even if all skipped); non-zero only on errors |
| **Summary** | Prints `Scaffold complete: N created, M skipped.` |

---

## Scaffold File Manifest (v2)

### Core Files (27)

| File | Purpose |
|---|---|
| `.axiom/axiom.config.yaml` | Per-repo Axiom configuration |
| `.axiom/command-registry.yaml` | Command-to-tag-requirements registry |
| `.axiom/tools/` | Installable debug tooling entrypoints |
| `.axiom/scripts/` | Helper scripts for debug tooling |
| `.memory-bank/_index.md` | Root Memory Bank inventory |
| `.memory-bank/_prompt.md` | Memory Bank global rules for agents |
| `.memory-bank/implementation-plans/_index.md` | Implementation plan inventory |
| `.memory-bank/implementation-plans/_prompt.md` | Implementation plans local rules |
| `.memory-bank/projectBrief.md` | Project scope and goals |
| `.memory-bank/productContext.md` | Problem space, UX goals, domain terminology |
| `.memory-bank/systemPatterns.md` | Architecture, design patterns, data flow |
| `.memory-bank/techContext.md` | Languages, frameworks, build tools, deployment |
| `.memory-bank/decisionLog.md` | Key technical and product decisions |
| `.memory-bank/activeContext.md` | Current focus and priorities |
| `.memory-bank/progress.md` | What works, what's next |
| `.memory-bank/TODO.md` | Working execution backlog |
| `.memory-bank/work-items/_index.md` | Work item inventory |
| `.memory-bank/work-items/_prompt.md` | Work items local rules |
| `.memory-bank/work-items/_current.md` | Default loop pointer file |
| `.memory-bank/work-items/onboarding-01/meta-planning.md` | Seed onboarding meta-plan |
| `.memory-bank/work-items/onboarding-01/plan.md` | Seed onboarding plan |
| `.memory-bank/work-items/onboarding-01/plan.yaml` | Seed onboarding executable plan |
| `.memory-bank/work-items/onboarding-01/verification.md` | Seed onboarding rolling verification |
| `AGENTS.md` | Agent rules pointing to specs/ |
| `specs/README.md` | Minimal spec inventory stub |
| `specs/_index.md` | Specs inventory index |
| `specs/_prompt.md` | Specs writing rules |
| `.worktrees/_prompt.md` | Worktree creation/merge/cleanup rules |
| `.worktrees/_index.md` | Active worktree inventory |
| `specs/00-PRD.md` | Starter PRD template |
| `specs/01-Architecture.md` | Starter architecture template |

### Optional: Full Memory Bank Skeleton (`--memory-bank-full`)

| File/Folder | Purpose |
|---|---|
| `.memory-bank/agents/` + `_index.md` + `_prompt.md` | Agent profiles and reflections |
| `.memory-bank/inbox/` + `_index.md` + `_prompt.md` | Inter-agent messages |
| `.memory-bank/projects/` + `_index.md` + `_prompt.md` | Project-specific durable knowledge |
| `.memory-bank/topics/` + `_index.md` + `_prompt.md` | Cross-project evergreen knowledge |
| `.memory-bank/best-practices/` + `_index.md` + `_prompt.md` | Portable best-practices notes |
| `.memory-bank/findings/` + `_index.md` + `_prompt.md` | Durable findings from adversarial/self-improvement loops |
| `.memory-bank/known-gaps/` + `_index.md` + `_prompt.md` + `TODO.md` | Known gaps scorecards and backlog |

---

## Key Template Contents

### `.axiom/axiom.config.yaml`

```yaml
version: 1
repo_id: "REPO_ID_PLACEHOLDER"  # TODO: replace with org/repo

confidence:
  weights:
    requirements_clarity: 20
    spec_alignment: 20
    test_coverage: 20
    checks_pass_rate: 25
    plan_completion: 10
    ambiguity_remaining: 5
  thresholds:
    low: 40
    high: 70

verification:
  required_checks:
    - pre_commit

limits:
  max_open_prs: 3

persistence:
  checkpoint_on_every_step: true
  stale_lock_timeout_seconds: 300

# --- Git Branching (axiom:trace work_item=branch-management-02 jira_ref=DEX-62 plan=phase-4/task-4-1/step-4-1-2) ---
# mode: "off" is the safe default — no branch management side effects.
# Change to "auto-branch" to enable per-work-item branch creation + on-complete PR/merge.
# Change to "worktree" if the repo uses git worktrees (Axiom default for its own dev).
git:
  branching:
    mode: "off"
```

### `.memory-bank/_prompt.md` (key rules)

- **No secrets** (API keys, tokens, credentials, PII) — redact as `[REDACTED]`
- Prefer crisp, verifiable statements over narratives
- Always link to canonical artifacts (Jira key, PR URL, run id, spec path)
- Update the Memory Bank immediately when state changes
- Start with `_index.md` to discover what exists
- When creating/updating a file, update the relevant `_index.md`
- Separate facts from assumptions; label assumptions

---

## Work Item ID Collision Avoidance

| Source | ID Format | Collision Risk |
|---|---|---|
| Jira-backed | Jira key (e.g., `ABC-123`) | None (globally unique) |
| Non-Jira | `<project-slug>-<sequence>` (e.g., `bootstrapping-01`) | Scan `.memory-bank/work-items/` for existing folders matching slug prefix, use next number |

The scaffold does NOT create work items — those are created at runtime when a ticket is assigned or work request is received.

---

## Scaffold Validation Checklist

After running the scaffold, all of these must be true:

- `.axiom/axiom.config.yaml` exists and is valid YAML with `version: 1`
- `.axiom/command-registry.yaml` exists and is valid YAML with `version: 1`
- `.memory-bank/_index.md` exists and lists all core context files
- `.memory-bank/_prompt.md` exists
- All 7 core context files exist (`projectBrief`, `productContext`, `systemPatterns`, `techContext`, `decisionLog`, `activeContext`, `progress`)
- `.memory-bank/TODO.md` exists
- `.memory-bank/implementation-plans/` has `_index.md` and `_prompt.md`
- `.memory-bank/work-items/` has `_index.md`, `_prompt.md`, `_current.md`
- `onboarding-01` work item skeleton exists (4 files)
- `AGENTS.md` exists
- `specs/README.md`, `specs/_index.md`, `specs/_prompt.md` exist
- `specs/00-PRD.md` and `specs/01-Architecture.md` exist

Expected output: `Validation: 29/29 files present. Scaffold is valid.`

---

## Install-Refresh Lifecycle

At the start of every work item run, runtime MUST execute an install refresh check.

### Refresh Scope

- `.memory-bank/` control files and required indexes
- `.opencode/` agents/commands/prompts/skills availability
- `opencode.jsonc` baseline compatibility
- `.axiom/axiom.config.yaml` and `.axiom/command-registry.yaml` compatibility

### Refresh Rules

| Rule | Detail |
|---|---|
| Idempotent and deterministic | Same inputs produce same outputs |
| Failures block run start | Refresh failures MUST block before planning |
| Machine-readable counts | `created`, `updated`, `skipped`, `conflicted`, `failed` |
| Ownership manifest enforced | `.axiom/install/ownership-manifest.yaml` declares installer-managed paths |
| Non-destructive by contract | Auto-write only allowlisted paths; non-allowlisted paths are validate-only |
| Fail-closed on conflicts | Non-allowlisted incompatible paths return `reason_code` + `operator_action` |

### Refresh Result Schema

```json
{
  "mode": "scaffold|link",
  "phase": "refresh_on_start",
  "counts": {
    "created": 0,
    "updated": 0,
    "skipped": 0,
    "conflicted": 0,
    "failed": 0
  },
  "status": "ok|blocked",
  "reason_code": "",
  "operator_action": ""
}
```

`reason_code` and `operator_action` are REQUIRED when `status=blocked` or `counts.conflicted > 0`.

---

## Autonomous Runtime Onboarding

- Runtime flows MUST support autonomous onboarding for non-Axiom repos
- If required assets are missing at runtime start, runtime MUST run scaffold/link install preflight automatically when writes are allowed
- If writes are forbidden, runtime MUST fail closed with exact install patch instructions
- Applies equally to Jira-triggered runs and runtime-local CLI/API runs

---

## Post-Install Onboarding Flow

After running the installer, guide the user through:

1. Ask about the project to populate `projectBrief.md`
2. Help fill in `productContext.md`, `systemPatterns.md`, `techContext.md`
3. Record initial decisions in `decisionLog.md`
4. Set current focus in `activeContext.md`
5. Create initial work items in `TODO.md`

The `axiom-onboarding` skill provides structured guidance for this process.

---

## Full Install Mode vs Scaffold-Only

| Mode | What it installs |
|---|---|
| **Full install** (default) | Scaffold templates + all `.opencode/` infrastructure (agents, commands, skills, prompts, `opencode.jsonc`) |
| **`--scaffold-only`** | Only scaffold templates; use when `.opencode/` already exists (e.g., from submodule or template repo) |

See `.axiom/scaffold/MANIFEST.md` for the full file list.

---

## Agent Checklists

### Running the Scaffold

1. Verify target repo path exists and is a valid git repository
2. Run `python .axiom/scaffold/install.py --target /path/to/repo` (or `--dry-run` first)
3. Verify output shows `CREATE` for new files and `SKIP` for existing files
4. Verify final summary: `Scaffold complete: N created, M skipped.`
5. Check that all 29 expected files exist (see Validation Checklist)
6. Replace `REPO_ID_PLACEHOLDER` in `.axiom/axiom.config.yaml` with actual `org/repo`
7. Guide user through filling in template placeholders
8. If `--memory-bank-full` was used: verify additional folders were created

### Modifying the Scaffold

1. Update the file template content in the spec (the template IS the contract)
2. Update the File List table if adding/removing scaffold files
3. Update the Scaffold Validation section with the new expected file count
4. Update `.axiom/scaffold/install.py` to match the spec
5. Run the scaffold with `--dry-run` to verify the new file list
6. Run the scaffold for real against a temp directory and verify validation passes

---

## Non-Goals

- The scaffold does NOT ship the Axiom runtime/CLI (that's a separate Python package)
- The scaffold does NOT create work items (those are created at runtime)
- The scaffold does NOT fetch anything from the network
