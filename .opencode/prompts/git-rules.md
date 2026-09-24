# Git Rules

Full git rules for Consensus agents (referenced from AGENTS.md "Git and Workspace Hygiene"):

- All commits made by or with an AI agent MUST include a `Co-authored-by` trailer. Read the identity from `.axiom/axiom.config.yaml` under `git.co_author`.
- Always `git pull --rebase` before committing. Stash first if the worktree is dirty.
- Use `git mv` for all file and folder moves — never filesystem `mv` or `cp` + `rm`.
- Stage only the files you changed — never `git add -A` in a shared worktree.
- NEVER revert unrelated changes you did not make unless explicitly requested.
- NEVER run destructive git commands (`reset --hard`, `push --force`) unless explicitly requested.
- Do not amend commits unless explicitly requested.
- Do not push to remote unless the user explicitly asks.
