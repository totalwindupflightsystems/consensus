---
name: axiom-work-item-branch-management
description: Portable work item branch management contract — automatic branch creation, deterministic naming, merge-from-primary, conflict strategies, security controls, concurrency locking, and cross-surface configuration.
version: "1.0"
synopsis: |
  Defines the contract for automatic git branch creation, checkout, and primary-branch merging when
  Axiom begins work on a work item. Covers deterministic branch naming (`axiom/<work-item-id>`),
  branch name sanitization as a security control, merge-from-primary with conflict strategies
  (abort/theirs), rebase option, primary branch detection, config/env/CLI/API precedence, container
  bootstrap integration, file-based advisory locking, stale branch detection, rate limiting,
  protected branch awareness, credential protection, and structured event emission.
when-to-use: |
  Load this skill when implementing branch management logic, configuring branching behavior,
  debugging merge conflicts, designing branch security controls, handling dirty working trees,
  or integrating branch setup into the run lifecycle or container bootstrap.
tags:
  vertical: [coding, planning]
  category: development
  core: false
---

# Work Item Branch Management (Portable)

This skill defines the contract for automatic git branch management in Axiom.

Source spec: `specs/63-Work-Item-Branch-Management.md`

---

## Branch Naming Contract

### Deterministic Derivation

```
axiom/<sanitized-work-item-id>
```

Examples:
- `ABC-123` → `axiom/ABC-123`
- `auth-login-rate-limit` → `axiom/auth-login-rate-limit`

Rules:
- `axiom/` prefix is mandatory and not configurable (provides clear namespace).
- Given the same `work_item_id`, the same branch name MUST always be produced.
- Optional description suffix: `axiom/<id>/<sanitized-description>` (max 60 chars).

### Branch Name Sanitization (Security Control)

Sanitization steps (in order):

| Step | Rule | Example |
|------|------|---------|
| 1 | Replace whitespace with `-` | `"my ticket"` → `"my-ticket"` |
| 2 | Remove chars not in `[a-zA-Z0-9._/-]` | `"ABC#123"` → `"ABC123"` |
| 3 | Collapse consecutive `-` or `.` | `"ABC--123"` → `"ABC-123"` |
| 4 | Remove leading/trailing `-` or `.` | `"-ABC-"` → `"ABC"` |
| 5 | Remove `..` sequences | `"ABC..123"` → `"ABC.123"` |
| 6 | Remove trailing `.lock` | `"ABC.lock"` → `"ABC"` |
| 7 | Truncate to 236 chars (after prefix) | Platform limit: 244 total |
| 8 | If empty after sanitization, use `"unknown"` | `"###"` → `"unknown"` |

**Security purpose**: Prevents path traversal, git ref injection, command injection, and DoS via long names.

**Hard rule**: All git operations MUST use array-based subprocess invocation, never shell string interpolation.

---

## Merge-from-Primary Contract

### Primary Branch Detection (priority order)

1. **Config override**: `branching.primary_branch` in `.axiom/axiom.config.yaml`
2. **Git remote HEAD**: `git symbolic-ref refs/remotes/origin/HEAD`
3. **Fallback heuristic**: Check `main` → `master` → `develop` (first that exists)
4. **Fatal fallback**: Fail with `primary_branch_not_detected`

### Sync Strategy

| Strategy | Behavior | Default |
|----------|----------|---------|
| `merge` | `git merge origin/<primary> --no-edit` — preserves merge commits | **Yes** |
| `rebase` | `git rebase origin/<primary>` — linear history, rewrites commits | No |
| `none` | Skip sync entirely | No |

Rules:
- `sync_strategy` takes precedence over `merge_from_primary` boolean.
- If both set, `sync_strategy` wins with deprecation WARNING.
- `rebase` + `theirs` conflict strategy is an **invalid combination** — fail startup with clear error.

### Conflict Strategy

| Strategy | Behavior | Default |
|----------|----------|---------|
| `abort` | Abort merge, restore pre-merge state. Then apply `fail_on_conflict`. | **Yes** |
| `theirs` | Resolve all conflicts favoring primary. Log WARNING per file. | No |

- `fail_on_conflict: true` (default): aborted merge fails the run.
- `fail_on_conflict: false`: proceed on stale base with WARNING.

### Fetch Failure

If fetch fails (network/auth error): log WARNING and proceed without merging. Not a fatal error.

---

## Security Controls

### Credential Protection

- `GIT_TERMINAL_PROMPT=0` for all git subprocess calls.
- Sanitize git error messages: strip URLs containing `@` (credential-in-URL pattern).
- Regex: replace `https?://[^@]+@[^\s]+` with `https://[REDACTED]@<host>/...`.

### Rate Limiting

- Default: max 10 branch creations per minute per repository.
- Configurable: `branching.rate_limit_per_minute` (1-100).
- Exceeded: fail with `branch_rate_limited` error + `retry_after_seconds` hint.

### Protected Branch Awareness

- Default protected patterns: `main`, `master`, `develop`, `release/*`, `hotfix/*`.
- Configurable: `branching.protected_patterns` (glob patterns).
- If derived branch matches a protected pattern: fail with `branch_protected`.
- Pattern matching: `fnmatch`-style, case-sensitive.

---

## Concurrency and Locking

### File-Based Advisory Lock

- Lock file: `.git/axiom-branch.lock`
- Timeout: 30 seconds (configurable: `branching.lock_timeout_seconds`, 5-300).
- Lock contains: PID of holder + run ID (for diagnostics).
- MUST be released after branch setup (success or failure) — use context manager / try-finally.
- Stale lock detection: if PID no longer running, overwrite with WARNING.

---

## Stale Branch Detection

- A branch is "stale" if last commit > `branching.stale_branch_days` (default: 30, range: 7-365).
- Runs during branch setup, after checkout, before study phase.
- Emits `branch_stale_detected` WARNING (max 10 branches per event).
- Advisory only — does NOT block branch setup.
- Queries local refs only (no network calls).

---

## Configuration

```yaml
branching:
  enabled: true                    # Master switch
  merge_from_primary: true         # Merge from primary after checkout
  primary_branch: null             # Auto-detect if null
  conflict_strategy: "abort"       # "abort" or "theirs"
  fail_on_conflict: true           # Fail run on aborted merge conflict
  description: null                # Optional branch description suffix
  prefix: "axiom"                # Branch namespace prefix
  sync_strategy: "merge"           # "merge", "rebase", or "none"
  rate_limit_per_minute: 10        # Max branch creations per minute
  protected_patterns:              # Glob patterns for protected branches
    - "main"
    - "master"
    - "develop"
    - "release/*"
    - "hotfix/*"
  lock_timeout_seconds: 30         # Advisory lock timeout
  stale_branch_days: 30            # Days before stale detection
```

### Precedence (highest wins)

1. CLI flag
2. Environment variable (`AXIOM_BRANCH_*`)
3. Config file (`.axiom/axiom.config.yaml`)
4. Hardcoded default

### Environment Variables

| Variable | Overrides |
|----------|-----------|
| `AXIOM_BRANCH_ENABLED` | `branching.enabled` |
| `AXIOM_BRANCH_MERGE_PRIMARY` | `branching.merge_from_primary` |
| `AXIOM_BRANCH_PRIMARY` | `branching.primary_branch` |
| `AXIOM_BRANCH_CONFLICT_STRATEGY` | `branching.conflict_strategy` |
| `AXIOM_BRANCH_FAIL_ON_CONFLICT` | `branching.fail_on_conflict` |
| `AXIOM_BRANCH_PREFIX` | `branching.prefix` |
| `AXIOM_BRANCH_SYNC_STRATEGY` | `branching.sync_strategy` |

Boolean env vars: `"true"`, `"1"`, `"yes"` (case-insensitive) = true; all others = false.

---

## CLI Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--branch` | Enabled | Explicitly enable branching |
| `--no-branch` | — | Disable branching (work on current branch) |
| `--merge-primary` | Enabled | Enable merge-from-primary |
| `--no-merge-primary` | — | Disable merge-from-primary |
| `--branch-name` | Derived | Override branch name (bypasses derivation, still sanitized) |
| `--primary-branch` | Auto-detected | Override primary branch |
| `--branch-description` | None | Description suffix |

Rules:
- `--branch` and `--no-branch` are mutually exclusive.
- `--branch-name` bypasses `axiom/` prefix (caller provides full name).
- When `--no-branch` is set, all other branch flags are ignored with WARNING.

---

## Full Branch Setup Algorithm

1. **Resolve config** (CLI > env > config > defaults)
2. **Check enabled** — if disabled, skip all branch operations
3. **Derive branch name** — sanitize work item ID, apply prefix
4. **Handle dirty working tree** — stash if uncommitted changes
5. **Create or checkout branch** — check local, then remote, then create new from primary HEAD
6. **Merge from primary** — fetch, merge/rebase per strategy, handle conflicts
7. **Restore stashed changes** — stash pop; fail if conflicts

### Lifecycle Position

Branch setup occurs AFTER intake normalization and BEFORE the study phase.

---

## Container Bootstrap Integration

Branch setup integrates into container bootstrap between Steps 7 and 8 (after OpenCode health check, before orchestrator start):

- If `AXIOM_BRANCH_ENABLED=true`: derive branch, checkout, merge from primary.
- If resuming from PVC: checkout existing work branch.
- Branch setup failures in container mode are logged but do NOT prevent orchestrator start.

---

## Dirty Working Tree Handling

- **In-process mode**: stash → checkout → merge → stash pop. If stash pop conflicts, fail.
- **Container mode**: working tree SHOULD be clean (fresh clone). If dirty (PVC resume), same stash strategy.

---

## Structured Events

| Event | Level | When |
|-------|-------|------|
| `branch_setup_started` | INFO | Branch setup begins |
| `branch_created` | INFO | New branch created |
| `branch_checked_out` | INFO | Existing branch checked out |
| `branch_merge_started` | INFO | Merge from primary begins |
| `branch_merge_completed` | INFO | Merge succeeded |
| `branch_merge_conflict` | WARN | Merge produced conflicts |
| `branch_merge_conflict_skipped` | WARN | Conflict occurred but run proceeds |
| `branch_merge_fetch_failed` | WARN | Fetch of primary failed |
| `branch_setup_completed` | INFO | Branch setup done |
| `branch_setup_failed` | ERROR | Branch setup failed |
| `branch_stale_detected` | WARN | Stale branches found |
| `branch_rate_limited` | WARN | Rate limit exceeded |
| `branch_lock_acquired` | DEBUG | Lock acquired |
| `branch_lock_timeout` | ERROR | Lock acquisition timed out |

All events include standard correlation fields: `run_id`, `work_item_id`, `repo`.

---

## Error Taxonomy

| Error | Retryable | Run Impact |
|-------|-----------|------------|
| `branch_creation_failed` | No | Run fails |
| `branch_checkout_failed` | No | Run fails |
| `branch_merge_conflict` | No (strategy-dependent) | Depends on `fail_on_conflict` |
| `branch_fetch_failed` | No (degraded mode) | Run proceeds with WARNING |
| `primary_branch_not_detected` | No | Run fails (if merge enabled) |
| `branch_name_invalid` | No | Run fails |
| `branch_rate_limited` | Yes (after cooldown) | Run fails with retry hint |
| `branch_protected` | No | Run fails |
| `branch_lock_timeout` | Yes | Run fails with retry hint |
| `branch_rebase_conflict` | No (strategy-dependent) | Depends on `fail_on_conflict` |

---

## Open Decisions

- **OD-WBM-001**: Branch cleanup after PR merge — stale detection in scope; auto-deletion deferred.
- **OD-WBM-002**: Branch protection rules on `axiom/*` branches — deferred.
- **OD-WBM-003**: Shallow clone depth for merge — use `--unshallow` on failure.
- **OD-WBM-004**: Rebase on shared branches — document risk; restrict to single-run branches.

---

## Install-Time Configuration

<!-- axiom:trace work_item=branch-management-02 spec=specs/67-Go-Agent-Orchestration-Engine.md jira_ref=DEX-62 plan=phase-4/task-4-1/step-4-1-1 -->

During `axiom init`, the installer SHOULD ask the user which branching mode to use.

> **Note**: The interactive `axiom init` prompt for branching mode is not yet
> implemented. To enable auto-branch mode, manually add the following to your
> `.axiom/axiom.config.yaml` after running `axiom init`:
>
> ```yaml
> git:
>   branching:
>     mode: "auto-branch"       # or "off" (default) or "worktree"
>     on_complete: "draft-pr"   # or "pr", "merge", "none"
>     primary_branch: "main"
>     auto_cleanup: true
> ```
>
> Alternatively, use env vars (no config file edit needed):
> ```bash
> export AXIOM_GIT_BRANCHING_MODE=auto-branch
> export AXIOM_GIT_BRANCHING_ON_COMPLETE=draft-pr
> ```

### Prompt Sequence

**Question 1 (required):**
```
Which branching mode do you want? (off/auto-branch/worktree) [default: off]:
```

Write the answer to `.axiom/axiom.config.yaml` under `git.branching.mode`.

**Question 2 (only if user chose `auto-branch`):**
```
Auto-branch prefix? [default: axiom/]:
```

**Question 3 (only if user chose `auto-branch`):**
```
On-complete action? (draft-pr/pr/merge/none) [default: draft-pr]:
```

### YAML to Write

**Mode `off` (default — no additional questions):**
```yaml
git:
  branching:
    mode: "off"
```

**Mode `auto-branch`:**
```yaml
git:
  branching:
    mode: "auto-branch"
    auto_branch_prefix: "axiom/"   # or user-provided value
    on_complete: "draft-pr"          # or user-provided value
    primary_branch: "main"
    auto_cleanup: true
```

**Mode `worktree`:**
```yaml
git:
  branching:
    mode: "worktree"
```

### Behavior Notes

- `mode: "off"` is the safe default — no branch management, no side effects.
- `mode: "auto-branch"` opts in to automatic branch creation and on-complete actions.
- `mode: "worktree"` signals that the repo uses git worktrees (the Axiom default for its own development); branch setup is skipped because worktrees manage their own branching.
- The `git.branching` section is optional in the config file; omitting it is equivalent to `mode: "off"`.
- All fields have safe defaults — partial config is valid.
