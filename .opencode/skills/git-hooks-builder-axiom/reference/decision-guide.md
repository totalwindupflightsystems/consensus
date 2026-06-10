Decision guide

Step 1: Pick an installation approach
- If the repo already uses a hook manager/config, prefer extending it.
- Else prefer a tracked hooks directory (e.g., .githooks/) + repo-local setup instructions using core.hooksPath.
  - Optionally use a repo-committed .gitconfig include approach so setup is one command.

Step 2: Build a checks inventory from repo evidence
- Prefer commands that already exist:
  - package scripts, Makefile/justfile, CI workflows, documented dev commands
- Prefer tools already configured (eslint/prettier/ruff/etc.) over adding new dependencies.

Step 3: Map checks to hook events (typical defaults)
pre-commit (fast, staged-file oriented)
- formatting / lint on staged files
- trivial safety checks: trailing whitespace, large files, merge conflict markers, secrets scan (if already present)

commit-msg
- commit message policy (conventional commits, ticket prefix)
- optionally auto-normalize minor formatting

pre-push (heavier)
- unit tests, typecheck, build sanity check (but avoid full CI unless user wants it)

prepare-commit-msg (special)
- templating and auto-population only; use cautiously for enforcement because it cannot be skipped via --no-verify.

Step 4: Strictness selection
- Default to warn-only when:
  - tooling isn’t reliably present
  - runtime is slow or non-deterministic
  - the repo doesn’t already enforce it elsewhere
- Default to blocking when:
  - failure is very likely a real issue (e.g., syntax error, failing unit tests) AND runtime fits the budget
  - the repo already treats it as required in CI and developers benefit from earlier feedback

Step 5: User overrides
- Implement user-requested hooks even if non-default, but add mitigations:
  - timeouts
  - warn-only mode
  - optional “full” vs “quick” paths
  - clear bypass instructions where applicable
