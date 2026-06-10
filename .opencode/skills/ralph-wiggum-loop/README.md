# ralph-wiggum-loop

This is an Agent Skill that generates a repo-aware "Ralph loop" scaffold: one or more `PROMPT*.md` files plus a runnable loop script (`ralph-loop.*`) that repeatedly runs your harness (OpenCode or Claude Code) until the work is complete.

It has two modes:
- **simple**: generates a single `PROMPT.md` that contains the full loop contract
- **advanced**: generates a small prompt bundle (`PROMPT-core.md` + optional `PROMPT-*.md`) and an assembled `PROMPT.md` that integrates them

OpenCode loads skills on-demand via the built-in `skill` tool, discovering them from project/global locations including `.opencode/skills/<name>/SKILL.md` and Claude-compatible `.claude/skills/<name>/SKILL.md`.

## Install

Pick ONE location:

**Project-local (recommended):**
- `.opencode/skills/ralph-wiggum-loop/SKILL.md`
- or `.claude/skills/ralph-wiggum-loop/SKILL.md` (Claude-compatible; also discovered by OpenCode)

**Global:**
- `~/.config/opencode/skills/ralph-wiggum-loop/SKILL.md`
- or `~/.claude/skills/ralph-wiggum-loop/SKILL.md`

Important: the directory name must match the `name:` in `SKILL.md` and follow the lowercase-hyphen format.

## Use

### Claude Code
Invoke it directly as a slash command:
- `/ralph-wiggum-loop simple`
- `/ralph-wiggum-loop advanced harness=auto runner=bash enforce_commit=1`

Controls:
- Builder model: add `model=<MODEL>` (e.g., `model=openai/gpt-5.2`).

### OpenCode
Use it by asking the agent to load/run the skill (OpenCode exposes available skills to the agent and loads them via the `skill` tool).

If you're scripting, you can run OpenCode non-interactively and attach files with `--file` (the loop runner this skill generates uses that pattern).

Controls:
- Builder agent/model: add `agent=<AGENT>` and `model=<MODEL>`.
- Meta verifier agent/model (when `meta_layer=1`): add `verify_agent=<AGENT>` and `verify_model=<MODEL>`.

## What gets generated

At minimum:
- `PROMPT.md`
- `ralph-loop.sh` (default runner)
- `docs/ralph-loop.md`
- a work item file (auto-chosen; typically `.memory-bank/work-items/_current.md` if present, otherwise `.ralph/work-items/_current.md`)

In **advanced** mode you may also get:
- `PROMPT-core.md`
- `PROMPT-verify.md` (if the repo has real test/lint/typecheck gates)
- `PROMPT-mcp.md` (if MCP is configured)
- `PROMPT-agents.md` (if subagents exist)
- `PROMPT-INDEX.md` (a quick "which prompt does what" map)

The prompt content is generated from what the skill discovers in your repo: docs/specs, package/build systems, CI config, and harness capabilities (agents/subagents/skills/MCP).

## How the generated loop runs

The generated runner:
1. reads the work item file
2. runs one "fresh" harness iteration using `PROMPT.md` (+ work item attached)
3. extracts the iteration status (`PASS|FAIL|BLOCKED`) from the output
4. logs output per iteration
5. repeats until max iterations or a terminal stop condition

When `meta_layer=1`, the generated meta runner (`ralph-meta-loop.sh`) also:
- generates a steering packet after each verifier cycle
- attaches that steering packet to the next builder iteration

### Steering packet

The steering packet is a small text file (written under the loop log dir) that makes verifier steering explicit and machine-parseable.

Minimum fields:
- `STATUS`, `DECISION`, `STEP_AUDITED`
- `NEXT_BUILDER_STEP` (single bullet)
- `NEXT_BUILDER_PROMPT` (single bullet)
- `EVIDENCE` (single bullet)
- `SOURCE_VERIFY_LOG`
- `CYCLE_NUMBER`, `BUILDER_DURATION`, `VERIFIER_DURATION`, `CYCLE_DURATION`, `LOOP_ELAPSED`

OpenCode's `opencode run` supports attaching files via `--file/-f`, which is how the loop provides the prompt + work item each iteration.

### Iteration output contract

Each iteration is expected to end with a structured block:

```
ITERATION_RESULT
STATUS: PASS|FAIL|BLOCKED
WORK_ITEM: <path>
DID: <1-5 bullets>
CHANGED: <files or "none">
VERIFY: <commands + pass/fail>
EVIDENCE: <links/snippets>
NEXT: <next smallest step>
BLOCKERS: <if any>
COMMIT_SHA: <sha|NONE>
```

### Commit gate enforcement

When `ENFORCE_COMMIT=1`, the runner checks whether HEAD advanced after each iteration. If the iteration reported `PASS` (or `BLOCKED` due to missing commit evidence) but no commit was created, the runner exits with code 3 so the caller can run a commit-only follow-up and resume. This prevents false blockers caused by process gating rather than implementation failure.

The generated prompt also requires commit-quality gating when running in a git repo:
- prefer small, focused commits (one concern per commit),
- split unrelated changes into separate commits,
- in multi-repo workspaces, prefer one focused commit per touched repo unless a cross-repo atomic contract change is required.

This keeps merge conflict handling, rollback, and cherry-picking safer.

### Timing and duration output

Both `ralph-loop.sh` and `ralph-meta-loop.sh` log timing information at three levels:

1. **Loop level**: start date/time printed at launch, end date/time and total elapsed time printed at exit.
2. **Per-iteration / per-cycle**: each iteration logs its start time, the agent harness start/end with duration, and the iteration total duration plus cumulative loop elapsed time.
3. **Agent run level**: each `opencode run` or `claude -p` invocation is bracketed with `[agent-run] Starting...` and `[agent-run] Harness finished... (agent ran for Xm Ys)` lines.

For the meta-loop, builder and verifier durations are also recorded in the steering packet (`BUILDER_DURATION`, `VERIFIER_DURATION`, `CYCLE_DURATION`, `LOOP_ELAPSED`).

Example output:
```
========================================
Ralph Loop starting
  STARTED_AT: 2026-02-15 14:30:00 UTC
  REPO_ROOT:  /home/user/my-repo
  ...
========================================

========================================
  Iteration 1 of 50
  Started:  2026-02-15 14:30:02 UTC
========================================
[agent-run] Starting harness (opencode) at 2026-02-15 14:30:02 UTC...
...
[agent-run] Harness finished at 2026-02-15 14:32:45 UTC (agent ran for 2m 43s)
Iteration 1 reported PASS. (iteration: 2m 45s, loop total: 2m 47s)
```

## Default paths

- Work item path preference:
  1. `.memory-bank/work-items/_current.md`
  2. `.ralph/work-items/_current.md`
- Log directory preference:
  1. `.memory-bank/work-items/loop-runs/`
  2. `.ralph/loop-runs/`

## Agent/model controls

The generated runner scripts allow pinning both the builder and verifier (meta layer) execution.

- Builder:
  - env: `AGENT`, `MODEL`
  - args: `agent=...`, `model=...`
- Meta verifier (when `meta_layer=1`):
  - env: `VERIFY_AGENT`, `VERIFY_MODEL`
  - args: `verify_agent=...`, `verify_model=...`

## OpenCode server attach (planned, currently disabled)

We want loop runners to optionally run through a long-lived `opencode serve` process so engineers can attach and inspect live sessions.

However, as of opencode v1.1.59, `opencode run --attach <url> --agent <name>` fails with: `No context found for instance`.

Tracking:
- https://github.com/anomalyco/opencode/issues/6489
- https://github.com/anomalyco/opencode/pull/8154

Until this is fixed in a released OpenCode version, the generated runners use direct `opencode run` (no `--attach`).

## Notes on agents and MCP

If OpenCode agents/subagents exist, the generated prompts will include a delegation section. OpenCode distinguishes primary agents and subagents, and subagents can be invoked by @-mention.

If MCP servers exist, the generated prompts will include an MCP usage section. MCP adds tools but also adds context, so enabling too many servers can bloat context.

## Troubleshooting

- **Skill doesn't show up**: confirm `SKILL.md` is in one of OpenCode's discovery paths and that `name` matches the folder name.
- **Mixed OpenCode + Claude Code frontmatter**: OpenCode only recognizes a small set of frontmatter fields and ignores unknown ones (so Claude-only fields are safe to keep).
- **Runner exits with code 3**: commit gate was triggered but no commit was created. Run a commit-only harness iteration, then resume the loop.
- **Runner exits with code 2**: max iterations reached without a terminal stop condition. Check the work item and logs for progress.
