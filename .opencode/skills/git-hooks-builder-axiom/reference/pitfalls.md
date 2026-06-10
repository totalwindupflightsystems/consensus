Common pitfalls and mitigations

1) “We put hooks in .git/hooks and nobody else gets them”
- .git/hooks isn’t committed by default. Prefer a tracked hooks directory + core.hooksPath, or an existing hooks manager approach.
- If using core.hooksPath, document the one-time setup step and keep changes repo-local.

2) “Hooks don’t run at all”
- Hooks must be executable or Git ignores them.
- Confirm Git is actually looking at the intended directory (core.hooksPath vs default).
- If core.hooksPath is used, note it was introduced in Git 2.9 (older Git won’t honor it).

3) “We accidentally made commits un-bypassable”
- pre-commit and commit-msg are bypassable with --no-verify.
- prepare-commit-msg is NOT suppressed by --no-verify; use it mainly for templating, not enforcement.

4) “Pre-commit is so slow people disable it”
- Prefer staged-file scoping and quick checks in pre-commit.
- Move heavy checks to pre-push or CI.
- Consider warn-only behavior (messages + exit 0) when latency is unpredictable.

5) “Pre-push scripts behave weirdly”
- pre-push reads what’s being pushed from stdin in a defined format; if you need branch/sha info, parse stdin.
- Push-related hooks run in $GIT_DIR, not necessarily the working tree.

6) “Hooks break on Windows / different shells”
- Prefer a runtime the repo already depends on (node/python) for portability.
- If using bash, keep it POSIX-ish and avoid relying on nonstandard utilities.

7) “Hooks are noisy”
- Be quiet on success; print actionable steps on failure.
- Prefer auto-fix where safe (formatters), and summarize what changed.

8) “Hooks enforce policies that aren’t aligned with CI”
- Prefer reusing CI commands and repo scripts.
- If local checks differ, explain why (speed vs completeness) and ensure CI remains the source of truth.
