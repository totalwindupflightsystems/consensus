# git-hooks-builder

An Agent Skill that scans a Git repository and recommends, implements, and verifies useful Git hooks (pre-commit, commit-msg, pre-push, etc.). It picks the right hooks mechanism for the repo — whether that's an existing manager like Husky or Lefthook, a tracked hooks directory with `core.hooksPath`, or plain hook scripts — and fits new hooks into the project's existing conventions.

OpenCode loads skills on-demand via the built-in `skill` tool, discovering them from project/global locations including `.opencode/skills/<name>/SKILL.md` and Claude-compatible `.claude/skills/<name>/SKILL.md`.

## Install

Pick ONE location:

**Project-local (recommended):**
- `.opencode/skills/git-hooks-builder-axiom/SKILL.md`
- or `.claude/skills/git-hooks-builder-axiom/SKILL.md` (Claude-compatible; also discovered by OpenCode)

**Global:**
- `~/.config/opencode/skills/git-hooks-builder-axiom/SKILL.md`
- or `~/.claude/skills/git-hooks-builder-axiom/SKILL.md`

Important: the directory name must match the `name:` in `SKILL.md` and follow the lowercase-hyphen format.

## Use

### Claude Code
Invoke it directly as a slash command:
- `/git-hooks-builder scan` — scan the repo and get hook recommendations
- `/git-hooks-builder add pre-commit lint` — add a pre-commit hook that runs the repo's linter
- `/git-hooks-builder add commit-msg conventional` — enforce conventional commit messages

### OpenCode
Ask the agent to load/run the skill. OpenCode exposes available skills to the agent and loads them via the `skill` tool.

Example prompts:
- "Scan this repo and recommend git hooks"
- "Add a pre-commit hook that runs lint and typecheck on staged files"
- "Set up commit message enforcement using conventional commits"

## What it does

The skill follows a 4-step workflow:

### 1. Reconnaissance
Scans the repo to understand:
- **Languages and build systems** — detected from `package.json`, `pyproject.toml`, `go.mod`, `Cargo.toml`, Makefile, etc.
- **Existing hooks** — checks for hook managers (Husky, Lefthook, pre-commit framework), tracked hooks directories, and existing hook scripts in `.git/hooks/`
- **Available commands** — identifies lint, format, test, typecheck, and build commands from package scripts, Makefiles, CI configs
- **Constraints** — speed budget, OS/cross-platform needs, warn vs block preference

### 2. Hook plan
Proposes a structured plan before writing anything:
- Which hook events to implement (pre-commit, commit-msg, pre-push)
- What each hook runs, scoped to staged files when possible
- Whether each hook is warn-only or blocking
- Which install approach to use (existing manager vs `core.hooksPath` vs direct scripts)
- Tradeoffs and alternatives for risky or slow choices

### 3. Implementation
Writes the hook scripts and configuration:
- Hook scripts placed where Git will actually run them
- Helper scripts and config as needed
- Lightweight docs: how to install, how to bypass, how to troubleshoot

### 4. Verification
Runs a small verification routine:
- Simulates or directly runs each hook and confirms exit codes/messages
- Runs the repo's relevant lint/test commands for touched areas

## Output format

The skill produces structured XML-tagged output:

| Tag | Contents |
|---|---|
| `<hook_plan>` | Proposed hooks, events, commands, warn/block policy, install approach |
| `<changes_made>` | Files created or modified |
| `<install_instructions>` | How to activate the hooks (team-shareable) |
| `<verification>` | Commands run and their results |
| `<troubleshooting>` | Common issues and fixes |

## Design principles

- **Extend, don't replace** — prefers the repo's existing hook manager and conventions over introducing new tooling.
- **Warn first** — defaults to warn-only when uncertainty is high; the user opts into blocking behavior.
- **Fast pre-commit, heavy pre-push** — keeps pre-commit hooks fast (staged-file-scoped lint/format); moves heavier checks to pre-push or CI.
- **Team-shareable** — prefers tracked hooks directories or manager configs that the whole team gets automatically.
- **Deterministic** — no network calls or flaky environment dependencies in hooks unless explicitly requested.
- **No surprises** — won't change global git config, won't make commits fail for long-running checks without consent, won't duplicate what CI already enforces.

## Supported hook mechanisms

The skill auto-detects and works with:

| Mechanism | Detection | Notes |
|---|---|---|
| **Husky** | `.husky/` dir or `husky` in `package.json` | JS/TS ecosystem; uses `npx husky` |
| **Lefthook** | `lefthook.yml` or `lefthook` in deps | Multi-language; parallel execution |
| **pre-commit (Python)** | `.pre-commit-config.yaml` | Python ecosystem; uses `pre-commit run` |
| **Tracked hooks dir** | `.githooks/` or similar + `core.hooksPath` | Language-agnostic; no extra tooling |
| **Direct scripts** | `.git/hooks/*` | Simplest; not team-shareable without setup |

If no mechanism exists, the skill recommends one based on the repo's tech stack and team size.

## Common hook recipes

### Pre-commit: lint staged files
Runs the repo's linter only on staged files. Fast, low-friction, catches issues before they enter history.

### Pre-commit: format check
Checks (or auto-fixes) formatting on staged files. Pairs well with lint.

### Commit-msg: conventional commits
Validates commit messages against the [Conventional Commits](https://www.conventionalcommits.org/) spec (`feat:`, `fix:`, `docs:`, etc.).

### Pre-push: full test suite
Runs the full test suite before pushing. Heavier, but catches integration issues before they hit CI.

### Pre-push: type check
Runs typecheck (TypeScript `tsc --noEmit`, Python `mypy`, etc.) before pushing.

## Bypassing hooks

All generated hooks respect the standard Git bypass:
```bash
git commit --no-verify   # skip pre-commit and commit-msg hooks
git push --no-verify     # skip pre-push hooks
```

The skill documents this in the generated install instructions and encourages teams to use it sparingly.

## Troubleshooting

- **Hook doesn't run**: confirm the hook file is executable (`chmod +x`) and in the right location (check `git config core.hooksPath` if using a tracked hooks dir).
- **Hook runs but wrong version**: if using a hook manager, ensure `npm install` / `pip install` / `lefthook install` has been run after pulling.
- **Hook is too slow**: move heavy checks from pre-commit to pre-push. Scope lint/format to staged files only (`git diff --cached --name-only`).
- **Hook breaks on CI**: CI environments typically don't run Git hooks. If yours does, add a `CI` environment variable check to skip interactive hooks.
- **Team doesn't have hooks**: add a setup step to your onboarding docs or a `make setup` / `npm run prepare` target that installs hooks.

## References

The `SKILL.md` references optional bundled reference files:
- `reference/hooks-reference.md` — detailed Git hooks API reference
- `reference/decision-guide.md` — flowchart for choosing a hooks mechanism
- `reference/pitfalls.md` — common mistakes and how to avoid them

These are not yet created. The skill works without them; they serve as optional deep-dive context for the agent.
