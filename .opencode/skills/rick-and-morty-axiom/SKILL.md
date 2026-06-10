---
name: rick-and-morty-axiom
description: >
  Supervising agent pattern for Morty build loops. The AI agent is Rick — it launches OpenCode, starts Morty, runs rick.sh in a loop, reports to the user, and handles mid-loop instructions. Covers hot-reload (§18), stage group retry (§19), API control plane (§20), model fallback, session limits, fleet routing, and operational reliability.
version: "1.2"
tags:
  vertical: ['operations', 'coding']
  category: operations
  core: false
---
# rick-and-morty-axiom — You Are Rick: Run the Loop, Own the System

## Companion Reference

For operational details (exit code namespaces, admin API contract, backoff math, lock files, config schema, fleet management): see **[REFERENCE.md](./REFERENCE.md)** in this skill directory. **Always load REFERENCE.md** when:
- Troubleshooting why rick.sh returned a specific exit code
- Understanding Morty's lock file format or admin API responses
- Configuring backoff/circuit-breaker thresholds
- Managing fleet deployments (multiple Mortys)
- Debugging steering parser behavior or transition routing

---

## What This Is

**You (the AI agent) are Rick.** You are not a passive observer. You are the orchestrator running a live autonomous build system. You launch processes, run scripts, read their output, make decisions, report to the user, and loop again. The user talks to you between cycles when they want status, rule changes, or new work fed in.

The system has three layers and **you manage all of them**:

```
YOU (Rick — the AI agent in this conversation)
 │
 ├─ 1. OpenCode server    ← you start this as a background process
 │     (http://127.0.0.1:4096)
 │
 ├─ 2. Morty binary        ← you start this as a background process
 │     (morty run <config>)    it connects to OpenCode to do AI work
 │
 └─ 3. rick.sh             ← you run this in a loop, read its output,
       (watchdog script)       interpret exit codes, and act on them
```

**The user does not run these.** You do. In this conversation. Right now.

---

## ⚠️ AI Agent Warning: NEVER run rick-loop.sh from a Tool Call

`rick-loop.sh` loops internally and **never returns control to you**. If you run it as a blocking bash tool call, you become unresponsive to user messages for hours — a confirmed 16-hour incident (session ses_232de6e97ffe 04:21–19:54).

**The rule:**
- ✅ `rick.sh` — run this in YOUR loop (it blocks for ≤5 min then returns). You read the result, check for user messages, act, then run it again.
- ❌ `rick-loop.sh` — **never call this from a tool call**. It is for unattended overnight runs where there is NO AI agent supervising.

The user expects to be able to talk to you between cycles. If you wrap the loop inside a script, they cannot reach you.

---

## OPENCODE_URL Default Mismatch — Set This Before Running rick.sh

`rick.sh` defaults to `OPENCODE_URL=http://127.0.0.1:4096`. But always verify by checking the `url` field in your morty config file:

```bash
# Check what URL your morty config expects
grep '"url"' .morty/your-config.json

# Always set explicitly before running rick.sh
export OPENCODE_URL=http://127.0.0.1:4096   # or whatever port your OpenCode uses
bash morty/scripts/rick.sh 30 30 .morty/your-config.json http://127.0.0.1:9091
```

If OpenCode and Morty are on different ports (common when running multiple workspaces), `rick.sh`'s health check will report "OpenCode DOWN" on every invocation. The fix is always setting `OPENCODE_URL` explicitly.

---

## Signal Isolation — `setsid` vs `disown` (Platform-Dependent)

## Signal Isolation — `setsid` vs `disown` (Platform-Dependent)

The requirement is that OpenCode and Morty are removed from your shell's process group so they survive terminal close and rick.sh watchdog timeouts.

| Platform | Command | Why |
|---|---|---|
| **Linux** | `setsid nohup morty run ... > log 2>&1 &` | `setsid` creates new session+process group; `nohup` prevents SIGHUP |
| **macOS** | `nohup morty run ... > log 2>&1 & disown` | macOS doesn't ship `setsid`; `disown` removes the job from the shell's job table |
| **Either** | `nohup morty run ... > log 2>&1 & disown` | `disown` works on both Linux and macOS bash — use this for portability |

**The failure mode without isolation**: rick.sh runs `bash rick.sh 300 ...`. The watchdog window expires or you Ctrl+C it. The shell sends SIGINT to the **entire foreground process group**. If OpenCode or Morty is in the same process group, it receives SIGINT and exits. Now you've lost a mid-cycle run.

**When NOT to isolate**: Don't isolate rick.sh itself — Rick needs to read rick.sh's stdout and control it directly. Only long-lived background services (OpenCode, Morty) need isolation.

---

## The Loop (This Is Your Main Job)

Your entire job is this loop. Everything else is setup or error handling.

```
┌─────────────────────────────────────────────────────┐
│  1. Run rick.sh (blocks for up to 300s)             │
│  2. Read its exit code                              │
│  3. Act on the exit code (see table below)          │
│  4. Report to the user (plain language, not logs)   │
│  5. Check if the user said anything new              │
│  6. Go to 1                                         │
└─────────────────────────────────────────────────────┘
```

**You never stop looping** unless:
- The user tells you to stop
- A circuit breaker fires (5+ restarts in an hour, 2+ hours stuck)
- Morty finishes all its work (max_cycles reached, verifier says stop)

Between steps 4 and 6, the user may give you new instructions:
- "Give me a status update after every cycle"
- "Add this new task to the TODO"
- "Change the morty config to use a different agent"
- "Stop after this cycle"

You incorporate those instructions and keep looping.

---

## Step 0: Check If Morty Is Already Running (Always Do This First)

Before starting anything, check if a previous session left Morty running. **If you skip this, you may start a second Morty and cause a port 9091 collision.**

```bash
# Check if Morty's admin API is responding
curl -sf http://127.0.0.1:9091/api/status 2>/dev/null | python3 -m json.tool
```

**If it responds**: Morty is already running. Skip setup. Go straight to "Step 4: Enter the Rick Loop."

**If connection refused**: Morty is not running. Check for stale state:
```bash
# Check for stale lock files
ls -la .morty/*.lock 2>/dev/null

# If a lock file exists, check if the PID is alive
cat .morty/*.lock 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'PID: {d.get(\"pid\",\"none\")}')" 2>/dev/null
```

If a stale lock file exists with a dead PID, remove it: `rm .morty/*.lock`

**If OpenCode is already running** (check: `curl -sf http://127.0.0.1:4096/global/health`), skip Step 1 too.

Report to the user what you found: "Found Morty running at cycle X, stage Y. Resuming." or "No Morty running. Starting fresh."

---

## Step-by-Step: First Time Setup

### Step 1: Start OpenCode (background process)

OpenCode is the AI backend that Morty talks to. It must be running before Morty starts.

```bash
# Start OpenCode in the target repo directory
# Linux: use setsid nohup | macOS: use nohup + disown | Either: nohup + disown works everywhere
cd /path/to/target-repo
nohup opencode --serve > /tmp/opencode.log 2>&1 & disown
```

> **Why `setsid`**: Without `setsid`, OpenCode stays in the same process group as your shell. When rick.sh is killed (Ctrl+C, timeout, or error), signals propagate to the entire process group — killing OpenCode too. `setsid` creates a new session so OpenCode is fully isolated from Rick's signal propagation. `nohup` alone only prevents SIGHUP (terminal close), not SIGINT/SIGTERM from parent process group signals.

Wait for it to be healthy:
```bash
# Poll until healthy (timeout after 30s)
for i in $(seq 1 30); do
  curl -sf http://127.0.0.1:4096/global/health >/dev/null 2>&1 && echo "OpenCode is up" && break
  sleep 1
done
```

**If OpenCode is already running** (user started it, or it's running from a previous session), skip this step. Just verify:
```bash
curl -sf http://127.0.0.1:4096/global/health && echo "OpenCode OK"
```

### Step 2: Build the Morty Flow

Morty needs a config file (JSON) that defines the build loop stages, and prompt files that tell each agent what to do.

**Key design rule: one work item = one stage pair.** Don't put multiple work items in a single build stage and hope the agent works through them sequentially. Instead, create a `build-<item>` → `verify-<item>` pair for each work item. This gives you:
- Clear observability (admin API shows which item Morty is on)
- Proper routing (verify PASS → next item, FAIL → retry same item, BLOCKED → skip)
- Focused prompts (each stage override points to one specific item)

**Config file** (e.g., `.morty/my-project.json`):
```json
{
  "name": "my-project",
  "description": "Build loop for <what the user wants>",
  "version": "1.0.0",
  "agents": {
    "builder": {
      "opencode_agent": "dev-axiom",
      "prompt_file": "PROMPT.md",
      "role": "Implementation agent"
    },
    "verifier": {
      "opencode_agent": "qa-axiom",
      "prompt_file": "PROMPT-VERIFY.md",
      "role": "Verification agent"
    }
  },
  "stages": [
    {
      "name": "build",
      "agent": "builder",
      "run": "once_per_cycle",
      "transitions": {
        "complete": "verify",
        "default": "verify",
        "failed": "verify"
      }
    },
    {
      "name": "verify",
      "agent": "verifier",
      "run": "once",
      "steering": {
        "output_contract": ["STATUS", "DECISION"],
        "on_pass_continue": "build",
        "on_fail_steer": "build",
        "on_blocked_stop": "done"
      },
      "transitions": {
        "continue": "build",
        "steer": "build",
        "stop": "done",
        "pass": "build",
        "fail": "build",
        "blocked": "done",
        "complete": "build",
        "default": "build",
        "failed": "build"
      }
    }
  ],
  "server": {
    "url": "http://127.0.0.1:4096",
    "health_timeout": "30s",
    "session_timeout": "10m"
  },
  "settings": {
    "max_cycles": 50,
    "sleep_between_cycles": "2s",
    "log_dir": ".morty/logs",
    "log_level": "info"
  },
  "done_when": {
    "verifier_says_stop": true,
    "max_cycles_reached": true
  }
}
```

**Multi-work-item config** — when you have multiple work items, give each its own build→verify stage pair. The `prompt_override` keeps each stage focused on ONE item:

```json
{
  "name": "batch-work",
  "agents": {
    "builder": {"opencode_agent": "dev-axiom", "prompt_file": "PROMPT.md", "role": "builder"},
    "verifier": {"opencode_agent": "ralph-wiggum-verify", "prompt_file": "PROMPT-VERIFY.md", "role": "verifier"}
  },
  "stages": [
    {"name": "build-item-a", "agent": "builder", "prompt_override": "Work on item #1 only: fix-the-bug", "transitions": {"default": "verify-item-a"}},
    {"name": "verify-item-a", "agent": "verifier", "prompt_override": "Verify item #1: fix-the-bug", "steering": {"output_contract": ["STATUS", "DECISION"]}, "transitions": {"pass": "build-item-b", "fail": "build-item-a", "blocked": "build-item-b", "default": "build-item-b"}},

    {"name": "build-item-b", "agent": "builder", "prompt_override": "Work on item #2 only: add-feature", "transitions": {"default": "verify-item-b"}},
    {"name": "verify-item-b", "agent": "verifier", "prompt_override": "Verify item #2: add-feature", "steering": {"output_contract": ["STATUS", "DECISION"]}, "transitions": {"pass": "done", "fail": "build-item-b", "blocked": "done", "default": "done"}}
  ],
  "settings": {"max_cycles": 50, "sleep_between_cycles": "5s"},
  "done_when": {"verifier_says_stop": true, "max_cycles_reached": true}
}
```

The `prompt_override` is short — it just says which item to focus on. The full details live in PROMPT.md (which the builder reads every cycle). The override narrows the scope so the agent doesn't try to work on everything at once.

**Transition routing:**
- `pass` → next item's build stage (item done, move forward)
- `fail` / `steer` → same item's build stage (retry with verifier feedback)
- `blocked` → next item's build stage (skip, can't proceed without human input)
- Last item's `pass` → `"done"` (all items complete)

**Prompt files** — Morty reads these from disk every cycle via the `prompt_file` field (hot-reloadable — you can edit them mid-loop and Morty picks up changes next cycle).

Write `PROMPT.md` (builder prompt) and `PROMPT-VERIFY.md` (verifier prompt) in the target repo root. These are the instructions Morty sends to OpenCode agents each cycle.

**What PROMPT.md must contain** (Morty reads this verbatim and sends it to the builder agent):
- The specific goal for this loop run (not generic — what are we building?)
- The verification commands to run after each change (e.g., `go test ./...`, `npm test`)
- The repo conventions (read from AGENTS.md if it exists, or summarize the key rules)
- The output contract if the verifier needs structured output (e.g., `ITERATION_RESULT` block)

**What PROMPT-VERIFY.md must contain** (sent to the verifier agent):
- What "done" looks like — the acceptance criteria
- The test/lint/build commands to run for verification
- The steering output contract — **this is critical for Morty to route correctly**. The verifier MUST output these markers on their own lines at the end of its response:
  ```
  STATUS: PASS    (or FAIL or BLOCKED)
  DECISION: continue    (or steer or stop)
  ```
  Without these markers, Morty's steering parser falls through to `"complete"` and the loop may stop unexpectedly. See [REFERENCE.md](./REFERENCE.md) § Steering and Transitions for the full parser details and anti-patterns.

**Tip**: If the target repo already has a `PROMPT.md` from a previous ralph-wiggum loop, reuse it. If not, read the repo's `AGENTS.md`, `README.md`, and the current work item to write a good prompt. Don't write a generic "implement the next task" — be specific about what this loop run should accomplish.

You can also use `prompt_inline` in the config JSON instead of a file, or add extra context via `context_files` (e.g., `"context_files": ["specs/README.md", ".memory-bank/TODO.md"]`). Morty reads all of these and concatenates them into the final prompt.

**⚠️ Prompt file path resolution**: Morty resolves `prompt_file` relative to the **config file's directory**, not the repo root. If your config is at `.morty/batch.json` and you set `"prompt_file": "PROMPT.md"`, Morty looks for `.morty/PROMPT.md` — NOT `./PROMPT.md` at the repo root.

**Fix**: Either place prompt files next to the config (inside `.morty/`), or create symlinks:
```bash
ln -sf ../PROMPT.md .morty/PROMPT.md
ln -sf ../PROMPT-VERIFY.md .morty/PROMPT-VERIFY.md
```

This ensures Morty finds the prompts regardless of where you write them. See [REFERENCE.md](./REFERENCE.md) for the full path resolution logic.

### Step 3: Start Morty (background process)

```bash
cd /path/to/target-repo
mkdir -p .morty
nohup morty run \
  --server-url http://127.0.0.1:4096 \
  --admin \
  --admin-port 9091 \
  .morty/my-project.json \
  > .morty/morty.log 2>&1 & disown

echo "Morty started (PID $!)"
sleep 3

# Verify it's running
curl -sf http://127.0.0.1:9091/api/status | python3 -m json.tool
```

The `--admin` flag is **required** — rick.sh uses the admin API to check Morty's state.

> **Why `setsid` on Morty too**: Same reason as OpenCode — Morty must survive rick.sh restarts. When rick.sh is killed and Rick re-runs it, the old process group signals must not reach Morty. `setsid` + `nohup` together give full isolation: `setsid` for process group isolation (prevents SIGINT/SIGTERM propagation), `nohup` for SIGHUP immunity (survives terminal close).

### Step 4: Enter the Rick Loop

**This is where you live.** You run rick.sh, read the output, act, report, and loop.

```bash
# Run one watchdog window (blocks for up to 30s — use 300s only for unattended overnight runs)
# AUTO_START=true means: if Morty is dead, start it with proper isolation automatically.
# The agent never needs to remember setsid/disown/nohup — rick.sh handles it.
cd /path/to/target-repo
AUTO_START=true bash morty/scripts/rick.sh 30 30 .morty/my-project.json http://127.0.0.1:9091
```

> **`AUTO_START=true` is the key** — with this set, rick.sh handles the full lifecycle:
> 1. If Morty is running → monitors it (existing behavior)
> 2. If Morty is dead → starts it with proper signal isolation, then monitors it
> 3. If OpenCode is down → exits 2 (caller must fix OpenCode first)
>
> The agent only needs this ONE command. No separate "start Morty" step.
> rick.sh auto-detects the platform (Linux→setsid, macOS→disown) via `ISOLATION=auto`.
>
> **Override the isolation mode** if auto-detect isn't right:
> `ISOLATION=disown AUTO_START=true bash morty/scripts/rick.sh 30 30 ...`

> **Watch window (first argument) — 30s for interactive, 300s for overnight**:
> With a 300s window, the user cannot communicate with you for up to 5 minutes between cycles.
> For interactive sessions where the user may give instructions, use 30s.
> For unattended overnight runs, use 300s to reduce polling overhead.

Read the exit code. Act on it:

| Exit Code | Meaning | What You Do |
|-----------|---------|-------------|
| **0** | Watch window elapsed cleanly | Report status to user → run rick.sh again |
| **1** | Morty is dead | Report to user → restart Morty → run rick.sh again |
| **2** | OpenCode is down | Report to user → wait for OpenCode → restart Morty → run rick.sh again |
| **3** | New commit detected | Report the commit to user → run rick.sh again |
| **4** | Morty in terminal error | Report to user → kill Morty → clean lock files → restart → run rick.sh again |

> **Why did rick.sh exit 1 when Morty's process is still alive?** The admin API was unreachable — rick.sh treats this as "monitoring degraded" and fail-closes. See [REFERENCE.md](./REFERENCE.md) § Admin API Reachability Contract.

**After every rick.sh return**, before looping:
1. Read rick.sh's stdout — it contains `[rick]` prefixed status lines
2. Summarize to the user in plain language (not raw logs)
3. Check if the user has new instructions
4. Run rick.sh again

---

## How You Report to the User

**NOT this** (log dump):
```
2026-04-14T09:15:32Z [INFO] engine: cycle 47 stage=verify transition=build
```

**THIS** (human-readable):
```
Morty is running. Cycle 47 of 100. Currently verifying.
Last commit: "feat(cli): implement status command" (3 min ago).
No issues. Looping again.
```

When something goes wrong:
```
Morty died after 45 seconds. This is restart #2 of 5.
Restarting now. I'll report back after the next watchdog window.
```

When escalating:
```
⚠️ Morty has restarted 5 times in the last hour. Something is wrong.

Last error: OpenCode returned 503 during session creation.
Last successful commit: "fix(api): add validation" (47 min ago).

Options:
  A) I restart Morty one more time and watch closely
  B) I stop Morty and you investigate the OpenCode logs
  C) I skip this phase and move to the next one

What would you like me to do?
```

---

## Bug-Fix Gate Awareness

When Morty is running a bug-fix loop, the pm-axiom and dev-axiom agents automatically run the 7-gate bug-fix sequence. Rick does NOT need to manually invoke these gates — they are built into the agent prompts. However, Rick SHOULD watch for these signals in Morty's output and act on them:

### HARD BLOCK exits
If Morty's output or exit state contains a HARD BLOCK from Gate 1 (staleness check) or Gate 3 (strategy falsification):
- **Do NOT auto-restart Morty.** A HARD BLOCK means a human decision is required.
- Report to the user: "Morty hit a HARD BLOCK on [Gate 1/Gate 3]. [Reason]. Human approval needed before continuing."
- Wait for the user's instruction before resuming.
- Gate 1 HARD BLOCK: ticket may already be resolved. Ask the user to verify.
- Gate 3 HARD BLOCK: no valid alternatives documented for the fix approach. Ask the user to provide alternatives or approve proceeding.

### SPECULATIVE LABEL
If Morty's PR title contains `[SPECULATIVE]` or verification.md contains `reproduction: unconfirmed`:
- Report to the user: "Morty created a speculative fix — the bug was not reproduced before the fix was written. The PR is marked [SPECULATIVE] and has a −10 confidence penalty."
- This is NOT a blocker — the loop can continue. But the user should know before merging.

### Gate 3 inline fallback
If Morty's output contains "Strategy Falsification (inline)" rather than a subagent dispatch:
- Note to the user: "Gate 3 ran in lightweight inline mode (cost budget was hit). The strategy challenge was lighter than usual."
- This is NOT a blocker.

### Gate override (user instruction)
If the user says "skip the staleness check", "proceed anyway", or "ignore the block":
- Add `override=staleness-check` to the work item with the user's instruction as the justification.
- The user's instruction IS the human approval — record it in verification.md under `## Staleness Decision`.
- Resume Morty with the override in place.

### Gate reference
Full gate sequence: Gate 1 (Staleness) → Gate 2 (Bug Fix Mode) → Gate 3 (Strategy Falsification) → Gate 4 (Reproduce-or-Flag) → Gate 5 (Live/Dead Path) → Gate 6 (PR Scope) → Gate 7 (Post-PR Review Bot)
See `specs/20-Meta-Planning.md#gate-order` for semantics and conflict resolution.

---

## Handling User Instructions Mid-Loop

The user can talk to you at any time. Common requests:

| User Says | What You Do |
|-----------|-------------|
| "Status?" | Query `curl -sf http://127.0.0.1:9091/api/status` and report cycle/stage/state |
| "Give me status after every cycle" | Add a status report step after every rick.sh exit code 0 or 3 |
| "Add this task to the TODO" | Edit `.memory-bank/TODO.md`, report what you added, keep looping |
| "Change the agent to X" | Edit the morty config JSON, restart Morty, keep looping |
| "Stop" | Stop Morty gracefully (see below), report final status, stop looping |
| "Run 10 more cycles then stop" | Edit `max_cycles` in config, restart Morty, keep looping |
| "What did Morty build?" | Read recent git log, summarize commits since loop started |

**You never stop the loop to handle a user request** (unless they say "stop"). You handle it between rick.sh runs.

---

## Stopping Morty Gracefully

When the user says "stop" or you need to shut down Morty, **do not use `pkill`**. Use the admin API:

```bash
# 1. Send graceful stop signal
curl -sf -X POST http://127.0.0.1:9091/api/stop

# 2. Wait for Morty to finish its current cycle and exit (up to 60s)
for i in $(seq 1 60); do
  curl -sf http://127.0.0.1:9091/api/status >/dev/null 2>&1 || { echo "Morty stopped cleanly"; break; }
  sleep 1
done

# 3. Only if still alive after 60s — force kill as last resort
pkill -f "morty run" 2>/dev/null || true
```

This lets Morty finish its current cycle, flush the SQLite state store, and clean up the lock file. Using `pkill` directly risks corrupting the state database and leaving orphaned OpenCode sessions.

---

## Using rick-loop.sh Instead of Manual Looping

For long unattended runs, you can use `rick-loop.sh` which handles the exit-code dispatch, Morty restarts, exponential backoff, and circuit breakers automatically:

```bash
bash morty/scripts/rick-loop.sh \
  --morty-config .morty/my-project.json \
  --opencode-url http://127.0.0.1:4096 \
  --admin-url http://127.0.0.1:9091 \
  --isolation auto
```

### Process Isolation Modes (`--isolation`)

| Mode | Behavior | Platform |
|---|---|---|
| `auto` (default) | Detects platform: uses `setsid` on Linux, `disown` on macOS | Both |
| `setsid` | Creates new session + process group; strongest isolation | Linux only |
| `disown` | Removes from shell job table; works everywhere bash exists | Both |
| `nohup` | Prevents SIGHUP only; does NOT prevent SIGINT/SIGTERM propagation | Both |
| `none` | No isolation; child dies when parent exits (for debugging only) | Both |

**When to use what:**
- `auto` — default, covers most cases
- `setsid` — when you need guaranteed SIGINT isolation (Linux CI pipelines)
- `disown` — macOS, or when setsid isn't available
- `nohup` — legacy compatibility; minimal isolation
- `none` — debugging only; never use in production

You can also set this via environment variable: `ISOLATION=disown rick-loop.sh ...`

This is useful when you want to fire-and-forget, but **you lose the ability to report to the user between cycles**. For interactive supervision (the normal case), run rick.sh yourself in a loop as described above.

---

## Circuit Breakers (When to Stop and Escalate)

You MUST stop looping and ask the user when:

| Condition | Threshold |
|-----------|-----------|
| Morty restart count | > 5 in 1 hour |
| Phase stuck | > 2 hours with no commits |
| OpenCode down | > 10 minutes |
| BLOCKED output in Morty logs | Any occurrence |
| Error rate | > 50% of cycles failing |
| Estimated cost | > $10 total (ask user to continue) |
| Wall time | > 4 hours of continuous running (check in with user) |

---

## Prerequisites

Before starting, verify these are available:

| Dependency | Check Command |
|---|---|
| `bash` 4+ | `bash --version` |
| `python3` | `python3 --version` |
| `curl` | `curl --version` |
| `git` | `git --version` |
| `morty` binary | `morty version` |
| OpenCode | `opencode --version` or check if already running |

Scripts live at:
- `morty/scripts/rick.sh` — inner watchdog (you call this in your loop)
- `morty/scripts/rick-loop.sh` — outer supervisor (alternative to manual looping)

---

## Fleet Monitoring (Optional)

If multiple Mortys are running, use `morty-monitor` for a TUI dashboard:

```bash
morty-monitor              # live dashboard
morty-monitor --once       # one-shot snapshot
```

Build it: `cd Axiom/morty && make install-morty-monitor`

See `.opencode/skills/morty-monitor-build-axiom/SKILL.md` for details.

---

## What You Do NOT Do

- You do **not** write code (Morty does that via OpenCode agents)
- You do **not** make architectural decisions without the user
- You do **not** run forever without reporting (every 5 min minimum)
- You do **not** restart Morty more than 5 times/hour without escalating
- You do **not** modify specs or plans without user approval

---

## Quick Reference: The Whole Flow

```
1. Start OpenCode (if not running)     ← background process
2. Write morty config + prompt files   ← .morty/<name>.json, PROMPT.md, PROMPT-VERIFY.md
3. Start Morty                         ← background process with --admin flag
4. LOOP:                               ← this is where you live
   a. Run rick.sh                      ← blocks up to 300s
   b. Read exit code + stdout
   c. Act (restart / wait / report)
   d. Report to user in plain language
   e. Handle any user instructions
   f. Go to 4a
5. Stop when user says or circuit breaker fires
```

**You are the loop. The loop is you. Keep it running.**

---

## Troubleshooting

Common issues and their fixes. See [REFERENCE.md](./REFERENCE.md) for full operational details.

### Morty cycles complete in <1 second with 0 tokens

**Symptom**: Admin API shows cycles completing in milliseconds, sessions have `cost: 0`, no executor log lines appear.

**Cause**: Prompt assembly failure — Morty can't find PROMPT.md. Check the log for `prompt assembly FAILED`.

**Fix**: Morty resolves `prompt_file` relative to the config file's directory. If config is at `.morty/config.json`, prompt files must be in `.morty/` too. Use symlinks:
```bash
ln -sf ../PROMPT.md .morty/PROMPT.md
```

### rick.sh reports "Morty is DEAD" but Morty is actually running

**Symptom**: `curl http://127.0.0.1:9091/api/status` works, but rick.sh still says dead.

**Cause**: Lock file detection. rick.sh checks `.morty/<config-name>.lock` — if the lock file is missing or has a stale PID, rick.sh treats Morty as dead.

**Fix**: Clean stale locks: `rm -f .morty/*.lock` then let rick.sh restart Morty via AUTO_START.

### "another morty instance is running" error

**Symptom**: Morty refuses to start, reports another instance with a specific PID.

**Cause**: Stale lock file from a previous killed process + the legacy double-lock path `.morty/.morty/`.

**Fix**:
```bash
rm -f .morty/*.lock
rm -rf .morty/.morty/
```

### OpenCode `serve` command not recognized

**Symptom**: Running `opencode --serve` shows help text instead of starting server.

**Cause**: Correct syntax is `opencode serve` (subcommand, not flag).

**Fix**: `opencode serve --port 4096`

---

## New Morty Features (May 2026)

These features were added in the last sprint and are available in the current Morty binary:

### Config Hot-Reload (§18)
Change the morty config file and signal the running engine to pick it up without restarting:
```bash
# Via API (preferred for remote callers)
curl -X POST http://127.0.0.1:9091/api/reload

# Via signal (local operator with shell access)
kill -HUP $(pgrep morty)
```
The new config applies at the next cycle boundary. `config_version` increments in `/api/status`. SSE emits `config.reloaded` or `config.reload_failed`. Invalid configs are rejected — engine keeps running on old config.

### Stage Group Retry (§19)
Group stages into named retry units for plan→execute→verify loops. **Note: Morty configs are JSON. The YAML below is illustrative only — use JSON in actual config files.**
```jsonc
// morty-config.json
{
  "groups": [
    {
      "id": "work_loop",
      "stages": ["plan", "execute", "verify"],
      "retry_from": "plan",
      "max_retries": 3,
      "on_exhausted": "done"
    }
  ],
  "stages": [
    { "name": "plan",    "group": "work_loop", "agent": "planner", "run": "once",
      "transitions": { "default": "execute" } },
    { "name": "execute", "group": "work_loop", "agent": "builder", "run": "once",
      "transitions": { "default": "verify" } },
    { "name": "verify",  "group": "work_loop", "agent": "qa",      "run": "once",
      "transitions": { "pass": "done", "fail": "group_retry" } }
  ]
}
```
When verify fails, execution jumps back to `plan`. After 3 failures, follows `on_exhausted`. See `specs/67-Go-Agent-Orchestration-Engine.md#§19` for the authoritative schema, validation rules, and REQ-MORT-140–149.

### Operational Reliability
- `--max-sessions N` — caps OpenCode session accumulation; oldest idle pruned automatically
- **Model fallback** — after 3 consecutive errors, auto-switches to `models.fallback`; restores primary after 5 successes
- `GET /api/health` — component health (OpenCode, sessions, locks, error_rate)
- `GET /api/sessions` — per-session status with model, tokens, duration, result
- Per-session logs at `.morty/logs/<session-id>.log` + `morty logs --last`
- Lock files always at `$(CWD)/.morty/locks/` (no more hardcoded paths)

### API Control Plane (§20) — In Progress
Full programmatic management without filesystem access:
- `POST /api/dispatch` — inject commands mid-run (allowlisted, timeout-enforced)
- `POST /api/work-items/enqueue` — queue work items (SQLite-persisted)
- `PATCH /api/agents/:name` — runtime model/command override
- `POST /api/workflows/:id/activate` — database-backed workflow storage
- `GET /api/fleet` — local fleet routing across worktree instances

### Server-Side Run TTL
Stale runs auto-expire via materializer sweep:
- `POST /api/v1/admin/runs/reap-stale?max_age_seconds=3600` — admin endpoint for ops recovery
- CLI pre-cancels stale runs before creating new ones (prevents 409 concurrency errors)

---

## Traceability

```
axiom:trace work_item=morty-upgrade-01 spec=specs/67-Go-Agent-Orchestration-Engine.md jira_ref=DEX-455
```
