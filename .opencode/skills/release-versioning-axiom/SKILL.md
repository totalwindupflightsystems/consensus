---
name: release-versioning-axiom
description: "Version numbering, changelog generation, and release cutting for Axiom. Covers semver+git-hash versioning, branch vs main behavior, pre-commit hooks, changelog from conventional commits, and where version numbers live in the codebase."
version: "1.0"
---

# Release Versioning — Axiom

How Axiom versions itself, generates changelogs, and cuts releases.

## Version Format

```
<major>.<minor>.<patch>+<short-git-hash>
```

Examples:
- `0.2.0+a3f7b2c` — release 0.2.0, commit a3f7b2c
- `0.2.1+f8e3d1a` — patch release, commit f8e3d1a
- `0.0.0-dev+9bd4447` — feature branch build, no real version

**Semver** tells users what changed. **Git hash** tells developers exactly which commit. Together they're unambiguous.

## Branch Behavior

| Branch | Version format | When it bumps | Who sees it |
|---|---|---|---|
| `main` (or repo-configured primary) | `<major>.<minor>.<patch>+<hash>` | On merge (see bump rules below) | Users, changelog, release notes |
| Feature branches (`axiom/wt-*`) | `<parent-version>+<hash>` | Never bumps — inherits the version that existed when the worktree was created, referenced by its own hash | Developers only — throwaway |
| Release tags | `v<major>.<minor>.<patch>` | When a release is cut | GitHub releases, install scripts |

Feature branches inherit the parent version from when they branched off. They don't get their own version number — they're work in progress. The hash identifies the exact commit. The version only bumps when it hits main.

## Single Source of Truth

The version lives in ONE place: `VERSION` at the repo root.

```
0.2.0
```

That's it — just the semver part. The git hash is appended at build/runtime by reading `git rev-parse --short HEAD`.

Everything else reads from `VERSION`:
- `pyproject.toml` — reads `VERSION` file at build time (or uses a build plugin)
- `package.json` — updated by the version bump hook
- `cli/main.py` — reads `VERSION` file at runtime
- `app.py` — reads `VERSION` file at runtime
- Docker tags — `axiom:<version>+<hash>`

**Rule**: Never hardcode version strings in source files. Always read from `VERSION` or derive from it.

## Bump Rules (when merging to main)

### Default behavior

| Merge source | Default bump | Rationale |
|---|---|---|
| **Worktree merge** (`axiom/wt-*` → main) | **minor** | Worktrees represent planned work — features, capabilities, improvements. Default to minor. |
| **Direct commit on main** | **patch** | Direct commits on main are typically small fixes, typos, config tweaks. Default to patch (bug fix). |
| **Branch with `bug`, `fix`, `hotfix` in name** | **patch** | Branch name signals intent — it's a fix. |
| **Branch with `feature`, `feat` in name** | **minor** | Branch name signals intent — it's a feature. |
| **Any merge with `BREAKING CHANGE` in commit body/footer** | **major** | Breaking changes always bump major, regardless of other signals. |

### Override hierarchy (highest wins)

1. **Human or agent explicit override** — `--version 1.0.0` or `version: 1.0.0` in merge metadata. Always wins.
2. **`BREAKING CHANGE` keyword** in any commit body/footer → major
3. **Branch name keywords** (`bug`/`fix`/`hotfix` → patch, `feature`/`feat` → minor)
4. **Conventional commit type scanning** — scan all commits in the merge, highest type wins (`feat` → minor, `fix` → patch)
5. **Default by merge source** — worktree → minor, direct commit → patch

### Conventional commit type mapping (used as signal #4)

| Commit type | Bump level |
|---|---|
| `feat` | minor |
| `fix` | patch |
| `chore`, `docs`, `refactor`, `test`, `ci`, `build` | patch |
| Multiple types in one merge | highest wins (`feat` + `fix` → minor) |

### Configuration

All defaults are configurable in `.axiom/axiom.config.yaml`:

```yaml
release:
  primary_branch: main                    # which branch gets real versions
  worktree_default_bump: minor            # default bump for worktree merges
  direct_commit_default_bump: patch       # default bump for direct commits
  branch_name_keywords:
    patch: [bug, fix, hotfix, patch]
    minor: [feature, feat, enhancement]
    major: [breaking, major]
  allow_agent_override: true              # agents can override version
  allow_human_override: true              # humans can override version (always true)
```

**Pre-merge hook** (runs before merge commit on main):
1. Scan all commits being merged for conventional commit types
2. Determine bump level (major > minor > patch)
3. Read current `VERSION`, apply bump, write new `VERSION`
4. Update `package.json` version field
5. Amend the merge commit to include the version bump

**On feature branches**: no bump. Version is always `0.0.0-dev+<hash>`.

## Co-Author Trailer

All commits made by or with AI agents MUST include:

```
Co-authored-by: Axiom Agent <svc_axiom@dexdat.ai>
```

This is configured in `.axiom/axiom.config.yaml` under `git.co_author` and appended automatically by the pre-commit hook or the agent's commit workflow. See `git-commit-writing-axiom` skill for full rules.

## Changelog

`CHANGELOG.md` at the repo root. Updated on every merge to main.

### Format

```markdown
# Changelog

## [0.2.0] — 2026-04-13

### Added
- Multi-language code analysis with adapter pattern (analyze-01, DEX-386)
- Skill mode switching and tool disabling by profile (profiles-01)

### Fixed
- Broadcast buffer bounds in WebSocket layer (hardening-01)

### Changed
- Command dispatch wired to OpenCode runtime (command-dispatch-01)

## [0.1.0] — 2026-04-01
...
```

### Entry Template

When writing a changelog entry manually, use this template:

```
- <user-facing description of what changed> (<work-item-id>[, <JIRA-KEY>])
```

Examples:
- `- Add `axiom version` CLI subcommand that prints `X.Y.Z+<hash>` (release-engineering-01, DEX-437)`
- `- Fix hardcoded version strings — all source files now read from VERSION file (release-engineering-01)`
- `- Emergency concurrent agent swarm via /broken-arrow command (broken-arrow-01, DEX-435)`

**Entry quality rules:**
- Write for the user, not the developer: "Add X" not "feat(scope): add X"
- Include the work item ID so entries are traceable
- Include the Jira key when the work item has one
- One entry per merge (not per commit) — curated, not a git log dump
- Use past tense for Fixed ("Fix X"), present tense for Added ("Add X")

### Automated Generation

Use `scripts/changelog_gen.py` to generate a section from git log:

```bash
# Preview what would be generated since a tag
python3 scripts/changelog_gen.py --since v0.1.0 --dry-run

# Generate and prepend to CHANGELOG.md
python3 scripts/changelog_gen.py --since v0.1.0 --version 0.2.0 --prepend CHANGELOG.md

# Generate since a date
python3 scripts/changelog_gen.py --since 2026-04-01 --dry-run
```

The generator groups commits by conventional commit type (feat → Added, fix → Fixed, chore/ci/docs → Changed). The output is a starting point — curate it before committing.

### Rules

- **Group by**: Added, Changed, Fixed, Removed, Security, Deprecated
- **One entry per merge** (not per commit) — curated, not a git log dump
- **Link to work item** when available (e.g., `analyze-01`, `DEX-386`)
- **User-facing language** — describe what changed for the user, not internal refactoring details
- **Bug fixes are in the changelog** but release notes emphasize features (see below)

### What goes in the changelog vs release notes

| Changelog | Release notes |
|---|---|
| Every merge to main | Only on tagged releases |
| Developer-facing, technical | User-facing, plain language |
| Includes all fix/chore/refactor | Emphasizes features and important fixes |
| Chronological, grouped by type | Impact-first, grouped by theme |

## Release Notes (for tagged releases)

When you cut a release (tag `v<major>.<minor>.<patch>`), generate release notes from the changelog entries since the last tag. Release notes go to:
- GitHub release description
- Jira release (if configured)
- `.memory-bank/releases/v<version>.md` (durable record)

Release notes emphasize **features and important fixes**. Minor bug fixes and internal chores are summarized as "Various bug fixes and improvements" unless they're user-impacting.

## Where to Update on Version Bump

When the version bumps, these files MUST be updated:

| File | What changes | How |
|---|---|---|
| `VERSION` | The semver number | Written by bump hook |
| `CHANGELOG.md` | New version section added | Written by bump hook from commit messages |
| `.axiom/plugin/package.json` | `"version"` field | Written by bump hook |
| `pyproject.toml` | `version` field | Written by bump hook (or reads VERSION at build) |

These files SHOULD read `VERSION` at runtime instead of hardcoding:
| File | Current hardcoded value | Fix |
|---|---|---|
| `.axiom/src/axiom/cli/main.py` | `version="axiom 0.1.0"` | Read `VERSION` file |
| `.axiom/src/axiom/repo_runner/api/app.py` | `_APP_VERSION = "0.1.0"` | Read `VERSION` file |
| `.axiom/src/axiom/control_plane/api/app.py` | `version="0.1.0"` | Read `VERSION` file |

## Git Hooks

### Pre-commit (feature branches)
- Conventional commit format validation (already have `git-commit-writing-axiom` skill)
- No version bump — branches are `0.0.0-dev+<hash>`

### Pre-merge to main (or post-merge hook)
1. Collect all commit messages from the merge
2. Determine bump level from conventional commit types
3. Bump `VERSION`
4. Generate changelog entry
5. Update `package.json`
6. Include in merge commit

### Post-tag
- Generate release notes from changelog
- Create GitHub release (if `gh` available)
- Post to Jira (if configured)

## Agent Responsibilities

| Agent | Role |
|---|---|
| `@release-manager-axiom` | Cuts releases, generates release notes, manages tags |
| `@dev-axiom` | Writes conventional commits (uses `git-commit-writing-axiom` skill) |
| `@ci-cd-axiom` | Ensures CI runs version validation, changelog check |
| `@tower-axiom` | On merge, triggers version bump + changelog update |

## Implementation Plan

This skill describes the target state. Implementation order:

1. **Create `VERSION` file** — set to current version (`0.2.0` after current merges)
2. **Create `CHANGELOG.md`** — backfill from recent merge history
3. **Add `_version.py`** — reads `VERSION` file, appends git hash at runtime
4. **Update hardcoded versions** — cli/main.py, app.py, control_plane/app.py → read from `_version.py`
5. **Add pre-merge hook** — conventional commit scan → bump → changelog → package.json
6. **Add `axiom version` CLI command** — prints `<semver>+<hash>`
7. **Add CI check** — verify VERSION matches package.json, verify CHANGELOG has entry for current version

## Cross-References

- `git-commit-writing-axiom` — conventional commit format
- `changelog-release-notes-writing-axiom` — changelog and release notes style
- `enterprise-release-quality` — release quality gates
- `git-hooks-builder-axiom` — hook implementation patterns
- `@release-manager-axiom` — agent that executes releases
- `specs/70-OpenCode-Plugin.md` — plugin versioning and publishing
