# batch-commits-axiom

Portable skill for batching dirty-worktree changes into logical, well-messaged git commits.

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions — workflow, grouping heuristics, message format, guardrails |

## Usage

Load this skill when:
- The user asks to "commit", "batch commit", or "commit and push" after a multi-file editing session.
- A command needs to commit changes as part of its workflow.

## Command

`/axiom-batch-commit` — loads this skill and executes the batch commit workflow.
