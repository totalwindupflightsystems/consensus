## Git Rules

Before any git operation (commit, branch, merge, push), load the `git-commit-writing-axiom` skill. It contains:

- **Co-author trailer** — required on every commit. Read identity from `.axiom/axiom.config.yaml` under `git.co_author`.
- **Pull before commit** — always `git pull --rebase` before staging.
- **Use `git mv`** — never filesystem `mv` for tracked files.
- **Safety rules** — no force push, no hard reset, no amend unless asked.
- **Commit format** — conventional commits with trace footers.
