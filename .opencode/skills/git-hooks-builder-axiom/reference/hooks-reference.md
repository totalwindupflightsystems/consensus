Hooks basics
- Hooks live in $GIT_DIR/hooks by default, or a directory set via core.hooksPath.
- Hooks that aren’t executable are ignored.
- Git generally runs hooks from the repo working tree root, but hooks triggered during push run in $GIT_DIR.

High-value client-side hooks
pre-commit
- Runs before the commit message is obtained.
- Can be bypassed with --no-verify.
- Non-zero exit aborts the commit.
Typical use: fast staged-file checks (format/lint, basic static checks).

commit-msg
- Receives the commit message file path.
- Can be bypassed with --no-verify.
- Non-zero exit aborts the commit.
Typical use: enforce conventional commits / ticket prefixes / message lint.

prepare-commit-msg
- Runs after default message is prepared and before editor.
- Not suppressed by --no-verify.
- Non-zero exit aborts commit.
Typical use: templating or auto-populating message; not a replacement for pre-commit.

pre-push
- Runs before pushing; receives remote name/location as args.
- Receives refs-to-push on stdin.
- Non-zero exit aborts push.
Typical use: heavier checks (unit tests, typecheck) that should run less often.

Useful “non-blocking / maintenance” hooks
post-checkout, post-merge, post-commit
- Usually for notifications or local maintenance. Some cannot affect outcome.
