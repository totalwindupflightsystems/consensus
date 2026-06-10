---
name: git-commit-writing-axiom
description: >
  Git commit messages (conventional-commit structure, concise body, trace footers)
  plus operational git hygiene rules (co-author trailer, pull-before-commit, git mv,
  safety rules, rebase conflict resolution). Load before any git operation.
version: "1.1"
tags:
  vertical: [coding]
  category: development
  core: true
---

# Git Commit Writing + Git Hygiene

Load this skill before committing, branching, or any git operation. Covers commit message format, co-author trailers, and operational git rules.

axiom:trace work_item=DEX-73 spec=specs/06-Project-Configuration.md

---

## 1. Operational Git Rules (apply to every session)

### 1.1 Co-Author Trailer (required)

All commits made by or with an AI agent MUST include a `Co-authored-by` trailer.

**Interactive mode** (human drives, AI assists — the default):
```
Co-authored-by: Axiom Agent <svc_axiom@dexdat.ai>
```
Human is the primary author. Axiom is the co-author.

**Pipeline mode** (Coder workspace, background autonomous runs):
```
Co-authored-by: <ticket reporter name> <reporter@email>
```
Axiom Agent is the primary author. The human reviewer/reporter is the co-author.

The co-author identity is configured in `.axiom/axiom.config.yaml` under `git.co_author` (interactive) and `git.pipeline_co_author` (pipeline). **Read it from config — do not hardcode.**

### 1.2 Pull Before Committing — Always

```bash
git pull --rebase
```

Run this before staging or committing anything. If the worktree is dirty, stash first:

```bash
git stash && git pull --rebase && git stash pop
```

Skipping this causes painful conflict resolution after the fact. It takes 2 seconds. Always do it.

### 1.3 Use `git mv` for File and Folder Moves

```bash
git mv old/path/file.md new/path/file.md
```

Never use filesystem `mv` or `cp` + `rm` for tracked files. Git sees those as delete + untracked, producing massive noisy diffs and losing history. `git mv` records the operation as a rename with full history intact.

### 1.4 Safety Rules

- NEVER revert unrelated changes you did not make unless explicitly requested.
- NEVER run destructive git commands (`reset --hard`, `push --force`) unless explicitly requested.
- Do not amend commits unless explicitly requested.
- Do not push to remote unless the user explicitly asks.
- If changes are in files you must touch, read carefully and integrate rather than discarding.
- You may be in a dirty worktree. That's normal. Don't clean up someone else's work.

### 1.5 Resolving Rebase Conflicts

```bash
# If push is rejected after committing:
git pull --rebase
# For memory bank / verification.md conflicts — remote is almost always newer:
git checkout --theirs <conflicted-file>
git add <conflicted-file>
GIT_EDITOR="true" git rebase --continue   # avoids interactive editor
git push
```

---

## 2. Commit Message Format

```text
<type>[optional scope]: <description>

<optional body: why, context, or follow-up>

<optional footers>
```

### 2.1 Type Guidance

- `feat`: new user-visible capability
- `fix`: bug fix or broken behavior repair
- `docs`: documentation-only change
- `refactor`: internal restructuring without behavior change
- `test`: test-only improvement
- `chore` / `build` / `ci`: maintenance or tooling work

### 2.2 Writing Rules

- Keep the subject concise and imperative.
- Use the body for why, not for repeating the diff.
- Add `BREAKING CHANGE:` when the change alters compatibility.
- Add trace footers when the repo expects them.

### 2.3 Full Example

```text
feat(profiles): add skill mode switching engine

Profile engine with skill filtering and tool disabling.
Supports coding, personal-context, and custom profiles.

Trace: spec=specs/83-Skill-Mode-Switching.md jira_ref=DEX-433
Co-authored-by: Axiom Agent <svc_axiom@dexdat.ai>
```

---

## 3. Good Body Prompts

- Why now?
- What user or system behavior changed?
- What follow-up remains?

## 4. Avoid

- subjects that start with past tense
- body text that just repeats filenames
- multi-purpose commits disguised as one logical change
