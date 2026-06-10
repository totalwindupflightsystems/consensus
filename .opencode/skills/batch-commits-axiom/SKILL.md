---
name: batch-commits-axiom
description: Batch commit workflow for Axiom — group dirty-tree changes into logical, well-messaged commits with optional push.
version: "1.0"
tags:
  vertical: [coding]
  category: development
  core: false
---

# Batch Commits (Portable)

Commit a dirty worktree as one or more logical, well-messaged git commits. Each commit groups related changes, uses a conventional commit message, and preserves traceability.

## When to Use

- After a multi-file editing session that touched several concerns (e.g., agent configs + docs + code).
- When `git status` shows many unstaged/untracked changes that should not be a single monolithic commit.
- When the user says "commit", "batch commit", or "commit and push".

## Workflow

### Step 0 — Pull and Sync First (ALWAYS — before touching anything)

```bash
git pull --rebase
```

**This is mandatory, not optional.** Run it before surveying the worktree, before staging anything, before committing. If you skip this and the remote has diverged, you will hit conflicts during push that are painful to resolve after the fact.

Outcomes:
- **Already up to date** → proceed normally
- **Rebased cleanly** → proceed normally; note any commits that landed from remote
- **Rebase conflict** → STOP. Resolve conflicts first, then restart the workflow from Step 1.
  - For `verification.md` / memory bank files: almost always take `--theirs` (remote is newer)
  - For code files: read both sides, merge carefully, never blindly take one side
  - Use `GIT_EDITOR="true" git rebase --continue` to avoid interactive editor prompts

**If the worktree is dirty when you try to pull:**
```bash
git stash
git pull --rebase
git stash pop
```

### Step 1 — Survey the Worktree

```text
git status --porcelain
git diff --stat
```

Produce a change inventory: list every modified, added, deleted, and untracked file with a one-line description of what changed.

### Step 2 — Group into Logical Commits

Partition the change inventory into **logical commit groups**. Each group should:

- Touch one concern (e.g., "agent config", "spec update", "test fix", "docs").
- Be independently meaningful — reverting one commit should not break another.
- Follow the repo's existing commit message style (check `git log --oneline -10`).

Heuristics for grouping:
- Same directory + same purpose → same commit.
- Config changes across many files for one feature → same commit.
- Unrelated fixes → separate commits.
- Docs that describe the code change → same commit as the code, or immediately after.

If all changes are tightly related (one feature, one concern), a single commit is fine.

### Step 3 — Draft Commit Messages

Use **conventional commits** format matching the repo style:

```text
<type>(<scope>): <short summary>

<optional body — why, not what>
```

Common types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `ci`, `style`, `perf`.

Rules:
- Summary line ≤ 72 characters.
- Focus on **why** rather than **what** (the diff shows what).
- Include trace refs (`work_item=`, `spec=`) in the body when applicable.
- Never include secrets, tokens, or credentials in commit messages.

### Step 4 — Stage and Commit Each Group

For each logical group, in dependency order (base changes first):

```text
git add <files in group>
git commit -m "<message>"
```

Verify after each commit:
- `git status` shows remaining changes are correct.
- No files were accidentally included or excluded.

### Step 5 — Push (if requested)

Only push when the user explicitly asks. Before pushing:

```bash
# Verify you are ahead of remote and not behind
git log --oneline origin/main..HEAD   # what will be pushed
git status                            # must show "nothing to commit"
git push
```

Safety rules:
- NEVER force push to main/master.
- NEVER push if there are merge conflicts.
- **If push is rejected** (remote has diverged since Step 0): run `git pull --rebase` again,
  resolve any conflicts with `GIT_EDITOR="true"`, then push. Do not force push.

## Guardrails

- **Pull first**: Step 0 is mandatory. Never skip it.
- **No secret commits**: Skip files that likely contain secrets (`.env`, `credentials.json`, `*.pem`, `*.key`). Warn the user if they explicitly request committing these.
- **No empty commits**: Do not create commits with no changes.
- **Preserve unrelated changes**: If the worktree has changes the user did NOT make in this session, do NOT include them unless the user explicitly says to.
- **Fail closed on ambiguity**: If you cannot determine which changes belong together, ask the user (up to 7 questions) before committing.
- **Respect hooks**: Do not use `--no-verify` unless the user explicitly requests it. If a pre-commit hook fails, fix the issue and create a NEW commit (do not amend).
- **Use `git mv` for moves**: When moving files or folders, always use `git mv <src> <dst>` instead of filesystem `mv`. This preserves history and avoids the "2,979 deleted + 13,000 created" explosion that happens when git sees a move as delete + untracked.

## Output

After all commits are created, report:

```text
Commits created:
1. <hash> <message>
2. <hash> <message>
...

Files committed: <count>
Files remaining (uncommitted): <count or "none">
Push status: <pushed to origin/main | not pushed (user did not request)>
```

## Edge Cases

### Single logical change
If all changes are one concern, create one commit. Do not artificially split.

### Mixed committed + uncommitted
If some changes are already staged, include them in the appropriate logical group. Use `git diff --cached --stat` to see staged changes.

### Amend scenarios
Only amend if ALL conditions are met:
1. User explicitly requested amend.
2. HEAD commit was created by you in this conversation.
3. Commit has NOT been pushed to remote.
Otherwise, create a new commit.

### Untracked files
Include untracked files in the grouping analysis. Ask the user if any untracked files should be `.gitignore`d rather than committed.

### Moving files or folders (archives, renames, restructures)
Always use `git mv` — never filesystem `mv` or shell `cp` + `rm`:
```bash
# Move a single file
git mv old/path/file.md new/path/file.md

# Move an entire folder
git mv .memory-bank/work-items/old-folder/ .memory-bank/work-items/_archive/old-folder/

# Commit the move
git commit -m "chore(memory-bank): archive completed work items via git mv"
```
Git will record these as renames (similarity index ~100%), keeping full history intact and
avoiding the massive delete + create noise that breaks `git log --follow`.

### Rebase conflicts during push
If push is rejected after committing:
```bash
git pull --rebase
# For memory bank / verification.md conflicts — remote is almost always newer:
git checkout --theirs <conflicted-file>
git add <conflicted-file>
GIT_EDITOR="true" git rebase --continue
# Repeat for each conflict, then:
git push
```
