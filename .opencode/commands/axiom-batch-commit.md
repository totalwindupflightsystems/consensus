---
description: Batch commit dirty-tree changes into logical, well-messaged commits with optional push.
agent: dispatch-axiom
---

Batch commit the current worktree changes into logical git commits.

Inputs
- `$PUSH` optional: `yes` to push after committing (default: no).
- `$SCOPE` optional: limit to specific paths (default: entire worktree).
- `$ALL` optional: additional instructions (e.g., `amend`, `single-commit`, `include-untracked`).

Skills (load on demand):
- `batch-commits-axiom` — Always load. Batch commit workflow, grouping rules, and conventional commit style.
- `git-commit-writing-axiom` — Style guide for commit messages.

Do
1) Load skill `batch-commits-axiom`.
2) Execute its workflow:
   a) **Step 0 first**: run `git pull --rebase` before anything else. If the worktree
      is dirty, stash first (`git stash`), pull, then pop (`git stash pop`). Resolve
      any rebase conflicts before proceeding. This is mandatory — never skip it.
   b) Survey the worktree (`git status --porcelain`, `git diff --stat`).
   c) Group changes into logical commits.
   d) Draft conventional commit messages matching repo style (`git log --oneline -10`).
   e) Stage and commit each group in dependency order.
      - Use `git mv` for any file/folder moves — never filesystem mv.
   f) If `$PUSH` is `yes`, push to remote. If push is rejected, pull --rebase again
      and resolve conflicts with `GIT_EDITOR="true"` before retrying.
3) Fail closed if:
   - Merge conflicts exist after pull that cannot be auto-resolved.
   - Secret files detected without explicit user approval.
   - Worktree is clean after pull (nothing to commit).

Output
- Summary of commits created (hash + message).
- Count of files committed vs remaining.
- Push status.
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-batch-commit</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` commits created and push status

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many commits were created and whether push succeeded.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files committed (full paths, semicolon-separated)
- `evidence.commits_created`: list of commit SHAs and messages created
- `evidence.commits_count`: count of commits created
- `evidence.push_status`: pushed|not_pushed|failed
- `related_commands`: suggested follow-up commands
  - "To verify the committed changes, run: `/axiom-verify --work-item <id>`"
  - "To sync trace markers after committing, run: `/axiom-sync-trace`"

### Cross-References
- "Commit message style is in: `.opencode/skills/git-commit-writing-axiom/SKILL.md`"
- "Batch commit workflow is in: `.opencode/skills/batch-commits-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md
