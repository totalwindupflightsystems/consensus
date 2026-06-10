---
name: ralph-wiggum-loop
description: Generate repo-aware Ralph loop prompts (PROMPT.md + optional PROMPT-*.md) and a runnable loop script (ralph-loop.*) for OpenCode or Claude Code, portable across projects.
license: MIT
compatibility: agentskills
metadata:
  workflow: ralph-loop
  outputs: "PROMPT.md,PROMPT-*.md,ralph-loop.*,PROMPT-VERIFY.md,ralph-meta-loop.sh,.opencode/agents/ralph-wiggum-verify.md"
# Claude Code extensions (safe to keep; OpenCode ignores unknown fields):
disable-model-invocation: true
allowed-tools: Read, Grep, Glob, Write, Edit, Bash(git *), Bash(opencode *), Bash(claude *), Bash(sh *), Bash(bash *), Bash(python *), Bash(node *), Bash(npm *), Bash(pnpm *), Bash(yarn *), Bash(bun *), Bash(make *), Bash(just *), Bash(go *), Bash(cargo *), Bash(dotnet *), Bash(mvn *), Bash(gradle *)
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

## What I do

I scaffold a “Ralph loop” for *this repo* by generating:

1) **Prompting**
- **Simple mode:** one `PROMPT.md` that fully specifies the loop behavior.
- **Advanced mode:** a modular prompt set (`PROMPT-core.md` + `PROMPT-<type>.md`), plus an assembled `PROMPT.md` that integrates the modules.

2) **Runner (“Ralph loop interface”)**
- A runnable loop script (default `ralph-loop.sh`) that repeatedly calls your harness (OpenCode or Claude Code) with:
  - `PROMPT.md` (and optionally other prompt modules)
  - a **work item** file (the current objective)
  - logging + optional git gating

3) **Light docs**
- `docs/ralph-loop.md` with “how to run” + repo-specific tips.

This skill is about being *repo-aware*: it reads goals, conventions, available agents/skills/MCP, and builds a loop that fits the project instead of forcing one framework.

## When to use me

Use me when you want a reliable multi-iteration coding harness that:
- starts “fresh” each iteration (stateless by default),
- does one small verified step per run,
- produces consistent structured iteration outputs,
- optionally spawns nested “sub loops” for subtasks.

## How to invoke

You can pass arguments; I parse them from `$ARGUMENTS`.

Recommended invocations:

- `/ralph-wiggum-loop simple`
- `/ralph-wiggum-loop advanced`
- `/ralph-wiggum-loop simple harness=opencode runner=bash`
- `/ralph-wiggum-loop advanced harness=auto runner=bash out=. work_item=.memory-bank/work-items/_current.md`
- `/ralph-wiggum-loop advanced meta_layer=1 harness=opencode runner=bash`

### Argument grammar

- First positional arg can be `simple` or `advanced`.
- Additional args may be `key=value` pairs:

Keys:
- `harness`: `auto` (default) | `opencode` | `claude`
- `runner`: `auto` (default) | `bash` | `python` | `node`
- `out`: output directory for generated files (default: repo root)
- `prompt_name`: base prompt name (default: `PROMPT.md`)
- `work_item`: path to work item file (default: auto-detected)
- `log_dir`: path to log directory (default: auto-detected)
- `repo`: target member repo slug (workspace topology only; default: inferred or first repo in `workspace.yaml`)
- `enforce_commit`: `0|1` (default: auto; usually 1 if repo is “loop-driven”, else 0)
- `agent`: builder agent name (OpenCode only; default: empty)
- `model`: builder model name (OpenCode/Claude; default: empty)
- `thinking`: `0|1` (OpenCode only; default: 0). When `1`, pass `--thinking` to `opencode run`.
- `max_iters`: integer default 50
- `sleep_seconds`: integer default 2
- `meta_layer`: `0|1` (default: 0). When `1`, generate verifier-captain artifacts and a builder+verifier meta loop runner.
- `meta_verify_agent`: verifier agent name (default: `ralph-wiggum-verify`)
- `meta_verify_model`: verifier model (default: same as builder model unless explicitly set)
- `verify_agent`: alias for `meta_verify_agent`
- `verify_model`: alias for `meta_verify_model`
  (Reserved for future) OpenCode server attach: we want runners to support `opencode serve` + `opencode run --attach`,
  but agent selection is currently broken in attach mode (opencode v1.1.59; see README). Do not implement it yet.

If something is ambiguous, make a safe default (don’t ask a bunch of questions).

## Guardrails

- This skill is a **scaffolder**. Do not implement product features.
- Prefer **additive changes** (new files) over editing existing files.
- If `PROMPT.md` or `ralph-loop.sh` already exists, do **not overwrite**:
  - write `PROMPT.ralphgen.md` and/or `ralph-loop.ralphgen.sh`,
  - and clearly tell the user what to do next.
- Never run destructive commands. Only read files, inspect configs, and create the generated artifacts.

## Step 1 — Repo discovery (read-only)

From the repo root:
1) Identify top-level conventions and rules:
- Prefer `AGENTS.md` if present; otherwise look for `CLAUDE.md`.
- Read `README.md`, `CONTRIBUTING.md`, and any `docs/` or `specs/` that define goals and constraints.
- Detect existing “work item” system (examples: `.memory-bank/`, `work-items/`, `tasks/`, `tickets/`).

Workspace topology (multi-repo) detection:
- If a valid `workspace.yaml` exists in the current directory or an ancestor directory, treat this as a multi-repo workspace (see `specs/40-Multi-Repo-Workspace.md`).
- In a workspace, you MUST pick a target member repo for generation. Prefer:
  - If `repo=<slug>` (new arg; see below) is provided, use that.
  - Else, if you are currently inside `repos/<slug>/...`, infer `<slug>` from the path.
  - Else, default to the first repo in `workspace.yaml` and clearly tell the user how to override.
- All generated artifacts (PROMPT.md, runner scripts, docs) MUST be written into the target member repo, not the workspace root.

2) Detect project tech and verification entry points:
- Identify languages/frameworks from standard files:
  - JS/TS: `package.json`
  - Python: `pyproject.toml`, `requirements.txt`
  - Go: `go.mod`
  - Rust: `Cargo.toml`
  - JVM: `pom.xml`, `build.gradle`
  - .NET: `.sln`, `.csproj`
- Identify how to run:
  - tests, lint, typecheck, build, format
  - from `package.json` scripts, Makefile/Justfile, CI workflows, docs

3) Detect existing agent ecosystem:
- OpenCode: `.opencode/` config, agents, skills, MCP
- Claude Code: `.claude/agents/` and `.claude/skills/`

Produce a short internal “Repo Capability Summary” to drive generation (don’t write it to disk unless useful).

## Step 2 — Harness discovery (tools/agents/skills/MCP)

Decide `harness=auto` like this:
- If `opencode` is available (`command -v opencode`), prefer OpenCode.
- Else if `claude` is available (`command -v claude`), use Claude Code print mode.
- Else: generate scripts but mark harness commands as TODO.

If using OpenCode:
- Capture: `opencode --version`
- Capture: `opencode agent list` (if available) and note default agent options
- Capture: `opencode mcp list`
- Note CLI semantics you’ll rely on:
  - `opencode run` supports `--agent` and `--file` attachments. (Used by the runner script.)
  - `opencode run` supports `--model` to pin a model (builder and verifier).
  - `opencode run` supports `--attach <url>` to run via a headless `opencode serve` process.
  - `opencode serve` starts a headless server; use `--hostname` + `--port` (port 0 = random).

If using Claude Code CLI:
- Capture: `claude -v`
- Note that print mode uses `claude -p` and you can append system prompt files with `--append-system-prompt-file`.
- Note `--output-format` can be used for scripting.
- Detect custom subagents from `.claude/agents/` if present.

## Step 3 — Decide prompt architecture (simple vs advanced)

### Simple mode (default)
Generate:
- `PROMPT.md` (single file)
- `ralph-loop.sh` (or chosen runner language)
- `docs/ralph-loop.md`
- a work item file if missing

### Advanced mode
Generate a modular prompt set, but keep it minimal and relevant:
- Always:
  - `PROMPT-core.md` (the loop contract)
  - `PROMPT.md` (assembled / integrated prompt)
  - `PROMPT-INDEX.md` (what each prompt is for + recommended usage)
- Conditionally (only if relevant):
  - `PROMPT-agents.md` (if subagents exist)
  - `PROMPT-mcp.md` (if MCP servers exist)
  - `PROMPT-verify.md` (if repo has strong test/lint gates)
  - `PROMPT-specs.md` (if repo is spec-driven)
  - `PROMPT-release.md` (if repo has release workflow that matters)

### Meta layer (optional overlay)
When `meta_layer=1`, keep normal simple/advanced generation and additionally create a verifier-captain overlay:
- `PROMPT-VERIFY.md` (strict verifier output contract)
- `.opencode/agents/ralph-wiggum-verify.md` (primary verifier-captain agent)
- `ralph-meta-loop.sh` (orchestrates builder iteration -> verifier audit -> next builder iteration)

Meta layer rules:
- Verifier must audit one latest builder iteration at a time.
- Verifier must fail closed on missing evidence.
- Verifier output must include: `STATUS`, `STEP_AUDITED`, `DECISION`, `NEXT_BUILDER_STEP`, `NEXT_BUILDER_PROMPT`, `EVIDENCE`.
- Meta runner must support `--single` for one builder+verifier cycle.

Integration rule:
- `PROMPT.md` must be runnable *alone* (it should include or summarize the critical parts of the modular prompts).
- The modular prompts exist so humans can edit them independently.

## Step 4 — Generate the files

### 4A) Work item path selection
Pick `WORK_ITEM_PATH`:
- If operating in a multi-repo workspace, `WORK_ITEM_PATH` MUST be inside the target member repo:
  - Prefer: `repos/<slug>/.memory-bank/work-items/_current.md`
  - Fallback: `repos/<slug>/.ralph/work-items/_current.md`
- Else (single-repo):
  - If `.memory-bank/work-items/_current.md` exists, use it.
  - Else create `.ralph/work-items/_current.md`.

If creating a new file, seed it with:
- a short explanation
- a placeholder task
- and a rule: keep the objective and next step current each iteration.

### 4B) Logging directory selection
Pick `LOG_DIR`:
- If operating in a multi-repo workspace, `LOG_DIR` MUST be inside the target member repo:
  - Prefer: `repos/<slug>/.memory-bank/work-items/loop-runs/`
  - Fallback: `repos/<slug>/.ralph/loop-runs/`
- Else (single-repo):
  - If `.memory-bank/work-items/` exists: `.memory-bank/work-items/loop-runs/`
  - Else: `.ralph/loop-runs/`

### 4C) PROMPT.md generation rules (repo-aware)
When writing prompt(s), incorporate repo-specific findings:
- Startup reads: list the *actual* files that matter in this repo.
- Verification: include the *actual* commands the repo expects.
- Conventions: quote/reflect key rules from AGENTS.md / CLAUDE.md (briefly).
- Tools/agents/MCP/skills: include what is actually available.

If operating in a multi-repo workspace:
- Include `workspace.yaml` in Required Startup Reads.
- Include the workspace-level `.memory-bank/_prompt.md` in Required Startup Reads (guardrail: where-to-write rules).
- List member repos (slugs + paths) so the builder agent can locate cross-repo specs and memory.
- Instruct the builder agent to always write artifacts to the correct member repo root, never to the workspace-level aggregated symlink paths.

Avoid generic fluff. The prompt should feel like it was written for this repo.

---

# File templates (fill with repo-specific values)

## Template: PROMPT.md (Simple Mode)

Write `PROMPT.md` with this structure (adapt headings if the repo already has a prompt style):

1) Purpose & scope
- One-iteration-per-run policy
- Fresh-context assumption (you must re-read required startup reads each run)
- “Smallest safe step” philosophy

2) Repo context
- PROJECT: <repo name / short description>
- GOALS: <derived from README/specs>
- NON-GOALS / constraints: <derived>

3) Required Startup Reads (every loop)
- Bullet list of files and why
- Include the current work item file path

4) Environment capabilities
- HARNESS: opencode | claude
- SUBAGENTS: <names + one-line use>
- SKILLS: <names + one-line use>
- MCP: <server names + what they provide>

5) Canonical Dev Cycle (the loop)
- Understand → Plan → Implement → Verify → Record evidence → Update work item
- Verification must be real (tests/lint/build as applicable)

6) Git Commit Gate Policy (when inside a git repo or workspace member repo)
- Treat each iteration as one commit-sized unit of work when files change.
- Prefer small, focused commits that cover one concern so merge conflict resolution, rollback, and cherry-pick are safer.
- Do not batch unrelated edits into one commit; split by concern.
- In multi-repo workspaces, prefer one focused commit per touched repo unless a cross-repo atomic contract change is required.
- If commit creation is blocked by policy or permissions, set `COMMIT_SHA: NONE`, explain why, and return `STATUS: BLOCKED` with a commit-ready next step.

7) Nested Ralph Loops (optional, but recommended)
- When a task is too large, create a child work item file and run the loop against it.
- Parent loop should treat child loop completion as an input.

8) Idle-Time Spec Conformance Sweep (MUST for spec-driven repos)
- When no unchecked TODO items, unblocked work items, or active steering packets remain, do NOT stop.
- Instead, run a random spec conformance sweep per the `idle-spec-conformance-sweep` skill.
- Loop persistence rule: do not declare terminal blocked/stop while non-credential alignment work remains.
- Load `.opencode/skills/idle-spec-conformance-sweep/SKILL.md` for the full sweep algorithm.
- Include the sweep policy verbatim in the generated PROMPT.md so the loop agent knows to enter idle sweep mode.

9) Iteration Output Contract (MUST)
Require the agent to end every run with:

ITERATION_RESULT
STATUS: PASS|FAIL|BLOCKED
WORK_ITEM: <path>
DID: <1-5 bullets of what changed / what was learned>
CHANGED: <files changed or “none”>
VERIFY: <commands run + pass/fail; or “not-run” with reason>
EVIDENCE: <links to logs, test output snippets, grep results, etc.>
NEXT: <the next smallest verifiable step>
BLOCKERS: <if any>
COMMIT_SHA: <sha|NONE>

10) Fail-closed conditions
- If unclear about goals/specs: stop and output FAIL with NEXT being “read X / clarify Y”
- If tests fail: do not proceed to more changes
- If tool permissions prevent verification: output FAIL and say what permission is needed
- If commit gate applies and a focused commit is required but not possible, return BLOCKED with an exact split-and-commit plan.

## Template: PROMPT-core.md (Advanced)

`PROMPT-core.md` contains sections 1, 3, 5, 6, 8, 9, 10 above.
Keep it stable and repo-specific.

## Template: PROMPT-verify.md (Advanced, if relevant)

Include:
- The repo's expected verification commands, ordered by cheap→expensive
- A strict policy: "No PASS without running the appropriate gate (unless explicitly impossible)"
- Guidance for interpreting failures

### Bug-Fix Gate Compliance

For bug-fix work items (mode=bugfix, Jira type Bug/Hotfix, or [bugfix] label), Ralph MUST verify that the following gate outputs exist in `verification.md` before declaring the build PASS:

| Gate | Required output | How to check |
|---|---|---|
| Gate 1 (Staleness) | `## Staleness Decision` section (WARN or HARD BLOCK overridden) | grep for "## Staleness Decision" in verification.md |
| Gate 3 (Strategy Falsification) | `## Strategy Falsification` section with all 5 elements | grep for "## Strategy Falsification" and "Falsification criteria" in verification.md |
| Gate 4 (Reproduce-or-Flag) | `reproduction_status` field in evidence bundle | grep for "reproduction_status" in verification.md |
| Gate 5 (Live/Dead Path) | `## Live/Dead Path Check` section (if code was modified) | grep for "## Live/Dead" in verification.md |

**If any required gate output is missing**: return FAIL with `steering=steer` and inject a step:
"Gate [N] output missing from verification.md. Run the gate before proceeding: [gate spec reference]."

**Mechanical fix exception**: If the work item is a mechanical fix (single-line typo, config value, import correction), Gate 3 may be abbreviated to a one-line note. Gate 5 may use the quick-check path. Gates 1, 4, 6, 7 still apply.

**Gate reference**: `specs/20-Meta-Planning.md#gate-order`

## Template: PROMPT-agents.md (Advanced, if relevant)

Include:
- List subagents and what they’re for
- Delegation pattern:
  - “Use Explore agent for investigation”
  - “Use Review agent for code review”
  - “Main agent remains responsible for final edits and verification”
- If no subagents exist, don’t create this file.

## Template: PROMPT-mcp.md (Advanced, if relevant)

Include:
- Which MCP servers exist
- What kinds of tasks they’re best for (docs search, github operations, db introspection, etc.)
- “Prefer MCP for external systems; keep repo changes gated by local verification”

## Template: PROMPT.md (Advanced, assembled)

`PROMPT.md` should integrate:
- PROMPT-core (verbatim)
- plus short integrated sections from PROMPT-verify / PROMPT-agents / PROMPT-mcp as applicable

Also add a short “Prompt Map” at the top:
- which modular files exist
- when a human should edit which file

## Runner: ralph-loop.sh (default)

Generate a bash runner unless the user explicitly requests another language.

This script should:
- auto-detect harness (opencode preferred)
- execute one harness call per iteration with explicit status gating (`PASS|FAIL|BLOCKED`)
- call the harness once per iteration
- log output per iteration
- optionally enforce commit gating

Use this starting template and adapt it:

```bash
#!/usr/bin/env bash
set -euo pipefail

# -------- config (overridable by env or args) --------
MODE="${MODE:-simple}"
HARNESS="${HARNESS:-auto}"          # auto|opencode|claude
RUNNER="${RUNNER:-bash}"            # bash|python|node (informational)
OUT_DIR="${OUT_DIR:-.}"

PROMPT_FILE_DEFAULT="${PROMPT_FILE:-PROMPT.md}"
WORK_ITEM_PATH="${WORK_ITEM_PATH:-}"
LOG_DIR="${LOG_DIR:-}"

MAX_ITERS="${MAX_ITERS:-50}"
SLEEP_SECONDS="${SLEEP_SECONDS:-2}"

ENFORCE_COMMIT="${ENFORCE_COMMIT:-0}"   # 0|1
AGENT="${AGENT:-}"                      # OpenCode agent (optional)
MODEL="${MODEL:-}"                      # OpenCode/Claude model (optional)
THINKING="${THINKING:-0}"               # OpenCode: 0|1

# -------- arg parsing (simple key=value) --------
for arg in "$@"; do
  case "$arg" in
    simple|advanced) MODE="$arg" ;;
    harness=*) HARNESS="${arg#harness=}" ;;
    runner=*) RUNNER="${arg#runner=}" ;;
    out=*) OUT_DIR="${arg#out=}" ;;
    prompt=*|prompt_file=*) PROMPT_FILE_DEFAULT="${arg#*=}" ;;
    work_item=*) WORK_ITEM_PATH="${arg#work_item=}" ;;
    log_dir=*) LOG_DIR="${arg#log_dir=}" ;;
    enforce_commit=*) ENFORCE_COMMIT="${arg#enforce_commit=}" ;;
    max_iters=*) MAX_ITERS="${arg#max_iters=}" ;;
    sleep_seconds=*) SLEEP_SECONDS="${arg#sleep_seconds=}" ;;
    agent=*) AGENT="${arg#agent=}" ;;
    model=*) MODEL="${arg#model=}" ;;
  esac
done

# -------- go to repo root --------
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

# -------- defaults for work item + logs --------
if [[ -z "$WORK_ITEM_PATH" ]]; then
  if [[ -f ".memory-bank/work-items/_current.md" ]]; then
    WORK_ITEM_PATH=".memory-bank/work-items/_current.md"
  else
    WORK_ITEM_PATH=".ralph/work-items/_current.md"
  fi
fi

if [[ -z "$LOG_DIR" ]]; then
  if [[ -d ".memory-bank/work-items" ]]; then
    LOG_DIR=".memory-bank/work-items/loop-runs"
  else
    LOG_DIR=".ralph/loop-runs"
  fi
fi

mkdir -p "$(dirname "$WORK_ITEM_PATH")" "$LOG_DIR"

if [[ ! -f "$WORK_ITEM_PATH" ]]; then
  cat > "$WORK_ITEM_PATH" <<'EOF'
# Work Item (Ralph Loop)
Write the task for the loop here.

Rules:
- This file is the single source of truth for the current objective.
- Keep this file updated with current objective, constraints, and next smallest step.
EOF
fi

PROMPT_FILE="$PROMPT_FILE_DEFAULT"
if [[ ! -f "$PROMPT_FILE" ]]; then
  echo "ERROR: Prompt file not found: $PROMPT_FILE"
  echo "Run the generator skill to create it, or set PROMPT_FILE=/path/to/prompt."
  exit 1
fi

# -------- harness auto-detect --------
if [[ "$HARNESS" == "auto" ]]; then
  if command -v opencode >/dev/null 2>&1; then
    HARNESS="opencode"
  elif command -v claude >/dev/null 2>&1; then
    HARNESS="claude"
  else
    HARNESS="unknown"
  fi
fi

if [[ "$HARNESS" == "unknown" ]]; then
  echo "ERROR: Could not find opencode or claude on PATH."
  echo "Install one, or set HARNESS explicitly and edit the runner accordingly."
  exit 1
fi


# -------- helpers --------
timestamp() { date +"%Y-%m-%d_%H-%M-%S"; }
epoch_seconds() { date +%s; }

format_duration() {
  local total_seconds="$1"
  local hours=$((total_seconds / 3600))
  local minutes=$(( (total_seconds % 3600) / 60 ))
  local seconds=$((total_seconds % 60))
  if [[ $hours -gt 0 ]]; then
    printf "%dh %dm %ds" "$hours" "$minutes" "$seconds"
  elif [[ $minutes -gt 0 ]]; then
    printf "%dm %ds" "$minutes" "$seconds"
  else
    printf "%ds" "$seconds"
  fi
}

run_once_opencode() {
  local msg="$1"
  local cmd=(opencode run "$msg" --file "$PROMPT_FILE" --file "$WORK_ITEM_PATH" --format default)
  [[ -n "$AGENT" ]] && cmd+=(--agent "$AGENT")
  [[ -n "$MODEL" ]] && cmd+=(--model "$MODEL")
  [[ "$THINKING" == "1" ]] && cmd+=(--thinking)
  "${cmd[@]}"
}

run_once_claude() {
  local msg="$1"
  local cmd=(claude -p "$msg" --append-system-prompt-file "$PROMPT_FILE" --output-format text)
  [[ -n "$MODEL" ]] && cmd+=(--model "$MODEL")
  # Optional: restrict tools for safety (edit as needed)
  # cmd+=(--tools "default")
  "${cmd[@]}"
}

# -------- loop --------
LOOP_START_EPOCH="$(epoch_seconds)"
LOOP_START_TS="$(timestamp)"

echo "========================================"
echo "Ralph Loop starting"
echo "  STARTED_AT: $(date '+%Y-%m-%d %H:%M:%S %Z')"
echo "  REPO_ROOT:  $REPO_ROOT"
echo "  HARNESS:    $HARNESS"
echo "  MODE:       $MODE"
echo "  PROMPT:     $PROMPT_FILE"
echo "  WORK_ITEM:  $WORK_ITEM_PATH"
echo "  LOG_DIR:    $LOG_DIR"
echo "  MAX_ITERS:  $MAX_ITERS"
echo "  ENFORCE_COMMIT: $ENFORCE_COMMIT"
echo "========================================"

for i in $(seq 1 "$MAX_ITERS"); do
  before_sha="$(git rev-parse HEAD 2>/dev/null || echo "no-git")"
  ts="$(timestamp)"
  iter_start_epoch="$(epoch_seconds)"
  log="$LOG_DIR/loop-${ts}-iter-${i}.log"

  echo "" | tee -a "$log"
  echo "========================================"  | tee -a "$log"
  echo "  Iteration $i of $MAX_ITERS"              | tee -a "$log"
  echo "  Started:  $(date '+%Y-%m-%d %H:%M:%S %Z')" | tee -a "$log"
  echo "========================================"  | tee -a "$log"
  echo "Work item:" | tee -a "$log"
  sed -n '1,120p' "$WORK_ITEM_PATH" | tee -a "$log"
  echo "" | tee -a "$log"

  msg=$(
    cat <<EOF
You are running a Ralph Loop iteration.

Inputs:
- PROMPT file is available (use it as the operating contract).
- Work item file: $WORK_ITEM_PATH (contents included above in logs; open it directly if needed).

Task:
- Execute EXACTLY ONE small, verifiable iteration toward completing the work item.
- Update repo state as needed.
- Run the appropriate verification commands for this repo.
- End with the required ITERATION_RESULT block from the prompt.

If the task is ambiguous, fail-closed with STATUS: FAIL and a precise NEXT step.
EOF
  )

  echo "[agent-run] Starting harness ($HARNESS) at $(date '+%Y-%m-%d %H:%M:%S %Z')..." | tee -a "$log"
  agent_start_epoch="$(epoch_seconds)"

  if [[ "$HARNESS" == "opencode" ]]; then
    run_once_opencode "$msg" 2>&1 | tee -a "$log"
  else
    run_once_claude "$msg" 2>&1 | tee -a "$log"
  fi

  agent_end_epoch="$(epoch_seconds)"
  agent_duration=$((agent_end_epoch - agent_start_epoch))
  echo "[agent-run] Harness finished at $(date '+%Y-%m-%d %H:%M:%S %Z') (agent ran for $(format_duration $agent_duration))" | tee -a "$log"

  after_sha="$(git rev-parse HEAD 2>/dev/null || echo "no-git")"
  status="$(grep -Eo 'STATUS: (PASS|FAIL|BLOCKED)' "$log" | tail -n1 | awk '{print $2}')"

  if [[ "$ENFORCE_COMMIT" == "1" && "$before_sha" == "$after_sha" ]]; then
    if [[ "$status" == "PASS" ]] || grep -Eqi 'commit_sha:[[:space:]]*`?NONE`?|missing commit|without new commit|no commit was created' "$log"; then
      echo "Commit gate triggered without new commit." | tee -a "$log"
      echo "Portable recovery: run one commit-only harness iteration, then continue loop on success." | tee -a "$log"
      exit 3
    fi
  fi

  iter_end_epoch="$(epoch_seconds)"
  iter_duration=$((iter_end_epoch - iter_start_epoch))
  loop_elapsed=$((iter_end_epoch - LOOP_START_EPOCH))

  if [[ "$status" == "PASS" ]]; then
    echo "Iteration $i reported PASS. (iteration: $(format_duration $iter_duration), loop total: $(format_duration $loop_elapsed))" | tee -a "$log"
  elif [[ "$status" == "BLOCKED" ]]; then
    echo "Iteration $i reported BLOCKED. (iteration: $(format_duration $iter_duration), loop total: $(format_duration $loop_elapsed))" | tee -a "$log"
  else
    echo "Iteration $i finished with status: ${status:-UNKNOWN}. (iteration: $(format_duration $iter_duration), loop total: $(format_duration $loop_elapsed))" | tee -a "$log"
  fi

  sleep "$SLEEP_SECONDS"
done

LOOP_END_EPOCH="$(epoch_seconds)"
LOOP_TOTAL=$((LOOP_END_EPOCH - LOOP_START_EPOCH))
echo "========================================"
echo "Ralph Loop finished"
echo "  ENDED_AT:    $(date '+%Y-%m-%d %H:%M:%S %Z')"
echo "  STARTED_AT:  $LOOP_START_TS"
echo "  TOTAL_TIME:  $(format_duration $LOOP_TOTAL)"
echo "  ITERATIONS:  $MAX_ITERS"
echo "  EXIT_REASON: max_iters_reached"
echo "========================================"
echo "Reached MAX_ITERS=$MAX_ITERS without terminal stop condition. Exiting."
exit 2
````

After writing, ensure:

* `chmod +x ralph-loop.sh`
* script uses safe defaults and does not overwrite existing files

## Runner: ralph-meta-loop.sh (when meta_layer=1)

Generate `ralph-meta-loop.sh` that:
- runs one builder iteration using `PROMPT.md`
- runs one verifier iteration using `PROMPT-VERIFY.md`
- parses verifier `STATUS` and `DECISION`
- builds a **steering packet** from verifier output and feeds it into the next builder cycle
- supports `--single` (one meta cycle only)
- writes separate builder and verifier logs under loop log dir

Timing instrumentation (MUST):
- At meta-loop start: log start date/time (`STARTED_AT: <datetime>`) and record epoch for total duration.
- Per meta-cycle: log cycle start date/time, then after builder run log `[builder] finished (duration: Xm Ys)`, after verifier run log `[verifier] finished (duration: Xm Ys)`, and at cycle end log total cycle duration and cumulative loop elapsed time.
- At meta-loop end (max iters or terminal stop): log end date/time, total elapsed time, and total cycles completed.
- Use the same `epoch_seconds()` and `format_duration()` helpers as `ralph-loop.sh`.

Controls (MUST be supported):
- Builder selection:
  - env: `AGENT`, `MODEL`
  - args: `agent=...`, `model=...`
- Verifier selection:
  - env: `VERIFY_AGENT`, `VERIFY_MODEL`
  - args: `verify_agent=...`, `verify_model=...` (aliases for `meta_verify_agent`/`meta_verify_model`)

When harness=opencode, the meta runner MUST pass these through:
- Builder: `opencode run ... --agent "$AGENT" --model "$MODEL"`
- Verifier: `opencode run ... --agent "$VERIFY_AGENT" --model "$VERIFY_MODEL"`

When harness=opencode, do NOT use `opencode serve` / `--attach` yet.

Reason:
- As of opencode v1.1.59, `opencode run --attach <url> --agent <name>` fails with: `No context found for instance`.
- Tracking: https://github.com/anomalyco/opencode/issues/6489 (fix PR: https://github.com/anomalyco/opencode/pull/8154)

Once the fix is released, we can re-enable an attach-based meta runner to allow live inspection via `opencode attach`.

### Steering packet (MUST)

The meta runner MUST generate a small, machine-parseable steering packet file after each verifier cycle.

Purpose:
- Give the next builder iteration a single authoritative “what to do next” input.
- Keep steering portable across harnesses and models.

Packet path:
- Write under the loop log dir (e.g., `.memory-bank/work-items/loop-runs/`).
- Suggested filename: `${ts}_meta-cycle-${i}_steering.txt`

Packet format (exact keys; YAML-like but not YAML):
```
STATUS: PASS|FAIL|BLOCKED
DECISION: continue|steer|stop
STEP_AUDITED: <id or description>
NEXT_BUILDER_STEP:
- <single bullet: next smallest step id/intent>
NEXT_BUILDER_PROMPT:
- <single bullet: which prompt file to use next>
EVIDENCE:
- <single bullet: evidence path to consult>
SOURCE_VERIFY_LOG: <path>
CYCLE_NUMBER: <int>
BUILDER_DURATION: <human-readable, e.g. "2m 34s">
VERIFIER_DURATION: <human-readable, e.g. "1m 12s">
CYCLE_DURATION: <human-readable, e.g. "3m 46s">
LOOP_ELAPSED: <human-readable, e.g. "15m 22s">
```

How the builder consumes it:
- The builder invocation MUST attach the packet (`--file <packet>` for OpenCode).
- The builder message MUST say: "Apply verifier steering from the attached steering packet. Treat `NEXT_BUILDER_STEP` and `NEXT_BUILDER_PROMPT` as authoritative."

Fail-closed rules:
- If the verifier output is missing required fields, the meta runner MUST stop.
- If the steering packet cannot be written, the meta runner MUST stop.

Safety constraints:
- stop on verifier `BLOCKED` or `DECISION: stop`
- do not auto-commit or push in meta runner
- avoid destructive git commands

If `ralph-meta-loop.sh` already exists, write `ralph-meta-loop.ralphgen.sh` instead.

## Template: PROMPT-VERIFY.md (when meta_layer=1)

Include:
- required verifier reads (`PROMPT.md`, TODO, work-item plan/plan.yaml/verification, newest loop logs)
- gate policy (`PASS|FAIL|BLOCKED`) and fail-closed conditions
- strict machine-parseable output contract
- steering payload requirements for next builder iteration

## Template: .opencode/agents/ralph-wiggum-verify.md (when meta_layer=1)

Create a repo-aware verifier-captain agent definition modeled after existing Axiom agent style:
- frontmatter with `mode: primary`, low temperature, and model default
- sections: Title, Context, Role, Objective, Inputs, Outputs, Gate Policy, Workflow, Fail-Closed Rules, Trace marker
- explicit prohibition on evidence invention and destructive git operations
- scoped to iteration audit (not whole-repo completion claims)

If the agent file already exists, write `.opencode/agents/ralph-wiggum-verify.ralphgen.md`.

## docs/ralph-loop.md

Write a short guide:

* what a Ralph loop is in this repo
* where the prompt and work item live
* how to run:

  * `./ralph-loop.sh`
  * override variables (builder): `HARNESS=opencode`, `AGENT=tower-axiom`, `MODEL=openai/gpt-5.2`
  * override variables (meta verifier, when meta_layer=1): `VERIFY_AGENT=ralph-wiggum-verify`, `VERIFY_MODEL=openai/gpt-5.2`
  * paths and gating: `WORK_ITEM_PATH=...`, `PROMPT_FILE=...`, `ENFORCE_COMMIT=1`
* how to do nested loops (child work items)

## Final output to the user (in chat)

After generating files, print:

* A list of files created (with paths)
* Which harness was targeted
* The recommended next command to run the loop
* In advanced mode: a 1–2 line “which prompt to use” recommendation (and if there are multiple viable PROMPT variants, clearly tell the user to pick one)
