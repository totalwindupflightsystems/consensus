# Multi-Repo Workspace Install

This document covers setting up Axiom across multiple repositories in a single workspace. For single-repo install, see [SKILL.md](SKILL.md).

**Spec reference**: `specs/40-Multi-Repo-Workspace.md`

## When to Use Multi-Repo Mode

Use workspace mode when:
- You work across 2+ repos that form one logical system
- You want agents to see specs and memory from all repos in one session
- You want cross-repo traceability (trace markers with `repo=<slug>`)

Do NOT use workspace mode for:
- Monorepos (single git repo with multiple packages — just use single-repo install)
- Repos that are completely independent

## Quick Start

```bash
# From the Axiom repo, set up workspace at the parent directory:
python3 .axiom/scaffold/workspace-setup.py --target /path/to/workspace --memory-bank-full

# Or with auto-install of Axiom into repos that don't have it:
python3 .axiom/scaffold/workspace-setup.py --target /path/to/workspace --install-missing --memory-bank-full

# Dry run first to see what would happen:
python3 .axiom/scaffold/workspace-setup.py --target /path/to/workspace --dry-run
```

## What workspace-setup.py Does

1. **Discovers** git repos one level below the target directory
2. **Excludes** the Axiom source repo automatically (detects `.axiom/scaffold/install.py`)
3. **Generates** `workspace.yaml` manifest with auto-derived slugs
4. **Creates** aggregation symlinks: `specs/<slug>/` -> `<repo>/specs/`, `.memory-bank/<slug>/` -> `<repo>/.memory-bank/`
5. **Creates** workspace-level files: `AGENTS.md`, `opencode.jsonc`, `specs/README.md`, `.memory-bank/_index.md`, `.memory-bank/_prompt.md`
6. **Copies** `.opencode/` infrastructure (agents, commands, skills, prompts) to workspace root
7. **Optionally installs** Axiom into member repos that don't have it (with `--install-missing`)

The copied `.opencode/` infrastructure must include `.opencode/skills/axiom-skill-map/` and `.opencode/prompts/skill-map-client.md`, and the workspace `opencode.jsonc` must include `.opencode/prompts/skill-map-client.md` in `instructions`. Workspace agents use that prompt to know they can load the skill map whenever capability routing is unclear.

## Supported Layouts

### Flat Layout (repos are direct children)

```
/code/                          # workspace root
  Axiom/                      # Axiom source (auto-excluded)
  frontend/                     # member repo
  backend/                      # member repo
  api-gateway/                  # member repo
```

### Nested Layout (repos under repos/ subdirectory)

```
/code/                          # workspace root
  repos/
    frontend/                   # member repo
    backend/                    # member repo
  workspace.yaml
```

The script auto-detects which layout is in use.

## Result: Workspace Directory Structure

After setup, the workspace root looks like:

```
workspace-root/
  workspace.yaml                # manifest (repo list + slugs)
  opencode.jsonc                # single OpenCode config
  AGENTS.md                     # workspace-level agent rules
  .opencode/                    # agents/commands/skills/prompts
  specs/                        # aggregated specs (symlinks)
    README.md                   # workspace-level spec inventory
    frontend/ -> frontend/specs/
    backend/ -> backend/specs/
  .memory-bank/                 # aggregated memory (symlinks)
    _index.md                   # workspace-level memory index
    _prompt.md                  # workspace-level memory rules (WHERE to write)
    frontend/ -> frontend/.memory-bank/
    backend/ -> backend/.memory-bank/
  frontend/                     # member repo (unchanged)
    specs/
    .memory-bank/
    .axiom/
    ...
  backend/                      # member repo (unchanged)
    specs/
    .memory-bank/
    .axiom/
    ...
```

## CLI Options

| Flag | Default | Description |
|---|---|---|
| `--target` | (required) | Path to workspace root directory |
| `--source` | auto-detected | Path to Axiom source repo |
| `--name` | directory name | Workspace name for manifest |
| `--install-missing` | false | Install Axiom into repos that don't have it |
| `--memory-bank-full` | false | Use full memory bank skeleton when installing |
| `--force` | false | Overwrite existing workspace-level files |
| `--dry-run` | false | Print actions without writing |
| `--verify` | false | Verify existing workspace (read-only) |
| `--exclude` | none | Regex pattern for repo names to exclude (repeatable) |
| `--verbose` | false | Show detailed file operations |

## Verification

```bash
# Verify an existing workspace:
python3 .axiom/scaffold/workspace-setup.py --target /path/to/workspace --verify
```

This checks:
- `workspace.yaml` exists and is valid
- All listed repos exist and are git repos
- Symlinks point to correct targets
- Workspace-level files exist
- `.opencode/` is present

## Integration with Operating Modes

### Local CLI Mode
- `cd` to workspace root and run `opencode`
- Agents detect workspace mode via `workspace.yaml`
- Use Ralph loop with `repo=<slug>` to target specific repos

### Local Automated / Full Automated (Runtime)
- Runtime detects `workspace.yaml` during startup
- Install-refresh preflight validates workspace integrity
- Per-repo work items are dispatched to the correct member repo
- Cross-repo work items use a single `work_item_id` with per-repo plans

### Jira/GitHub Service
- Jira tickets can reference workspace-level work items
- GitHub PRs are created per-repo (each repo has its own git history)
- Cross-repo trace markers link related changes

## Relationship to Single-Repo Install

- `workspace-setup.py` calls `install.py` for each member repo that needs Axiom (when `--install-missing`)
- `install.py` detects multi-repo context and suggests `workspace-setup.py` when appropriate
- Both scripts are idempotent and safe to re-run
- The workspace root is NOT a git repo; member repos are

## Troubleshooting

| Issue | Fix |
|---|---|
| Broken symlink after repo move | Re-run `workspace-setup.py` (idempotent) |
| Repo missing from workspace | Add to `workspace.yaml` and re-run setup |
| Agent writes to wrong location | Check `.memory-bank/_prompt.md` guardrail; agent must write to `<repo-path>/` not `specs/<slug>/` |
| Windows symlink failure | Script auto-detects and falls back to path references |
| Slug collision | Script auto-appends `-2`, `-3` etc. Edit `workspace.yaml` to customize |
| Axiom source included | Script auto-excludes repos containing `.axiom/scaffold/install.py` |
