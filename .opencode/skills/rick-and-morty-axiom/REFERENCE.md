# Rick & Morty Operational Reference

> **Start with SKILL.md** — it tells you what to do. Come here when you need to understand *why* something happened or *how* the internals work.

---

## rick.sh Internals

Location: `morty/scripts/rick.sh`

```bash
# Usage: ./rick.sh [watch_seconds] [report_interval_seconds] [morty_config] [admin_url]
# Defaults: 300s watch, 60s report interval, morty.json, http://127.0.0.1:9091
```

### Exit Code Namespace

rick.sh and the morty binary use **separate** exit code namespaces:

| Code | rick.sh meaning | morty binary meaning |
|------|----------------|---------------------|
| 0 | Watch window elapsed cleanly | Success (all cycles done) |
| 1 | Morty dead / monitoring degraded | Error |
| 2 | OpenCode down | Max cycles reached |
| 3 | New commit detected | User stopped |
| 4 | Terminal Error state (process alive, engine errored) | Blocked |

rick-loop.sh handles **rick.sh** exit codes, not morty binary exit codes directly.

### Inner Loop Design

- **10-second check interval** — worst-case detection latency is 18s (10s sleep + 8s OpenCode health timeout)
- **Exits immediately on state change** — doesn't wait for the full watch window
- **Caller owns actions** — rick.sh only observes; you (Rick) decide what to do
- **Single admin API call per iteration** — fetched once, reused for both status display and terminal-state check (eliminates TOCTOU race)

### Admin API Reachability Contract

When Morty is running, the admin API (`/api/status`) MUST be reachable. If curl returns empty:

- rick.sh treats this as **"monitoring degraded"** and exits 1
- This is **fail-closed**: a false restart is safer than silently masking a terminal error
- Requires Morty started with `--admin` flag

### Terminal Error Detection

rick.sh checks `terminal_state` in the `/api/status` JSON response:
- If `terminal_state == "error"` → exit 4 (process alive but engine cannot self-recover)
- If `terminal_state` is empty/null → continue (Morty is healthy)
- If admin API unreachable → exit 1 (monitoring degraded)

### Process Detection

1. **Primary**: lock file (`.morty/<config-name>.lock`) + PID check via `kill -0`
2. **Fallback**: `pgrep -f "morty run"` (handles startup race / missing lock)

### OpenCode Health Check

`is_opencode_healthy()` makes a **single attempt** with a `--max-time 15` curl timeout (15 seconds). There is no retry loop inside `is_opencode_healthy()` itself.

The outer rick.sh watchdog loop provides implicit retry — if OpenCode is down, rick.sh exits 2, rick-loop.sh waits `MORTY_OC_DOWN_WAIT` (default 30s) and re-runs rick.sh.

> **Note**: REFERENCE.md previously documented "4 retries with 5s backoff, 60s timeout per try" — this described a planned improvement that was implemented differently. The current implementation uses a single 15s attempt. OpenCode's blocked-event-loop behavior (HTTP endpoints blocking for 2+ minutes during agent dispatch) is tolerated by the outer restart loop, not by per-probe retries.

---

## rick-loop.sh Internals

Location: `morty/scripts/rick-loop.sh`

The outer supervisor that automates the exit-code dispatch loop. Use this for unattended runs; use manual looping (per SKILL.md) for interactive supervision.

### All Flags

```bash
rick-loop.sh [options]

--morty-bin PATH        # morty binary (default: morty on PATH)
--morty-config PATH     # config file (default: morty.json)
--morty-log PATH        # log file (default: .morty/morty.log)
--rick PATH             # rick.sh location (default: same dir as this script)
--opencode-url URL      # OpenCode URL (default: http://127.0.0.1:4096)
--admin-url URL         # Morty admin URL (default: http://127.0.0.1:9091)
--admin-port PORT       # Admin port for restart (derived from --admin-url)
--watch-seconds N       # Watch window per rick.sh call (default: 300)
--report-interval N     # Status report interval (default: 60)
--restart-wait N        # Seconds after restart before resuming (default: 10)
--oc-down-wait N        # Seconds to wait when OpenCode is down (default: 30)
--log-level LEVEL       # Morty log level on restart (default: info)
--server-url URL        # OpenCode URL passed to morty run (default: same as --opencode-url)
--max-loops N           # Max iterations, 0=unlimited (default: 0)
--max-error-restarts N  # Max consecutive error restarts (default: 5)
```

All flags have environment variable equivalents (e.g., `MORTY_BIN`, `RICK_WATCH_SECONDS`).

### Restart Backoff (Exponential)

Consecutive error restarts (exit 1 or 4) use exponential backoff:

```
delay = min(N² × 10s, 300s)

N=1 → 10s
N=2 → 40s
N=3 → 90s
N=4 → 160s
N=5 → 250s (then FATAL if max-error-restarts=5)
```

Counter resets on clean tick (exit 0) or new commit (exit 3).

### Terminal Error Restart (Exit 4)

When Morty is in terminal Error state (process alive but engine stuck):
1. Apply exponential backoff
2. `pkill -f "morty run"` to kill the stale process
3. Remove stale lock file (`.morty/<config-name>.lock`)
4. Restart Morty fresh

---

## Steering and Transitions (How Morty Decides What to Do Next)

This is the most important section for writing configs that don't get stuck. Morty's engine runs one stage per cycle, then uses the agent's output to decide which stage to run next.

### The Transition Flow

```
Agent runs → produces output text → steering parser extracts STATUS/DECISION
  → determineTransitionKey() maps STATUS to a transition key
  → evaluateTransition() looks up the key in the stage's transitions map
  → next stage is determined (or "done" if terminal)
```

### Transition Keys

The engine produces these transition keys from the agent's output:

| Agent Output | Transition Key | What It Means |
|---|---|---|
| `STATUS: PASS` | `"pass"` (or `steering.on_pass_continue` if set) | Work is good, continue |
| `STATUS: FAIL` | `"fail"` (or `steering.on_fail_steer` if set) | Work needs fixing, steer back |
| `STATUS: BLOCKED` | `"blocked"` (or `steering.on_blocked_stop` if set) | Can't proceed, stop |
| `DECISION: stop` | `"stop"` | **Highest priority** — always terminal, checked before parsing |
| No steering block | `"complete"` | Non-steering stage finished normally |
| Parse error | `"complete"` | Steering keys not found — falls through to complete |
| Stage error | `"failed"` | Stage execution threw an error |

### Default Transitions (CRITICAL — prevents stuck loops)

When `evaluateTransition()` looks up a transition key in the stage's `transitions` map:

1. **Exact match found** → use that target stage
2. **No exact match, but `"default"` exists** → use the default target
3. **No match and no default** → **treat as done** (engine stops)

**This is the most common cause of Morty "eating work and not producing output"**: if the agent outputs `STATUS: PASS` but the transitions map only has `"continue"` (not `"pass"`), and there's no `"default"`, the engine treats it as done and stops.

### Recommended Transition Maps

**Builder stage** (non-steering, always transitions to verifier):
```json
{
  "name": "build",
  "agent": "builder",
  "run": "once_per_cycle",
  "transitions": {
    "complete": "verify",
    "default": "verify",
    "failed": "verify"
  }
}
```

The `"default"` catches any unexpected transition key. The `"failed"` sends errors to the verifier instead of stopping.

**Verifier stage** (steering, routes based on STATUS):
```json
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
```

**Why so many keys?** Because the steering config (`on_pass_continue`, `on_fail_steer`, `on_blocked_stop`) maps STATUS values to *custom* transition keys. If `on_pass_continue` is `"build"`, then `STATUS: PASS` produces transition key `"build"` — but if `on_pass_continue` is empty, it produces `"pass"`. The transitions map must handle BOTH the custom keys AND the default keys. The `"default": "build"` catches anything unexpected.

**Anti-pattern — transitions that cause silent stops:**
```json
{
  "transitions": {
    "continue": "build",
    "steer": "build",
    "stop": "done"
  }
}
```
If the agent outputs `STATUS: PASS` and `on_pass_continue` is empty, the transition key is `"pass"` — not in the map, no default → **engine stops silently**. The agent did good work but Morty stopped because the transition map was incomplete.

### The 3-Tier Steering Parser

Morty extracts `STATUS` and `DECISION` from the agent's output using a 3-tier parser:

**Tier 1 — Structured markers** (preferred, most reliable):
```
STATUS: PASS
DECISION: continue
```
Simple `KEY: VALUE` on its own line. Case-insensitive key. This is what your PROMPT-VERIFY.md should tell the agent to output.

**Tier 2 — Fenced YAML/JSON blocks**:
````
```yaml
STATUS: PASS
DECISION: continue
```
````
Or:
````
```json
{"STATUS": "PASS", "DECISION": "continue"}
```
````
Parsed from fenced code blocks. Used when the agent wraps its output in markdown.

**Tier 3 — Heuristic keyword scanning**:
The parser scans for keywords like "pass", "fail", "blocked" in the output text. This is the least reliable tier — it can match false positives from prose like "the test passed" → `STATUS: PASS`.

**Best practice**: Tell your verifier agent to output Tier 1 markers explicitly:
```
Your PROMPT-VERIFY.md should end with:

  At the end of your response, output these markers on their own lines:
  STATUS: PASS or STATUS: FAIL or STATUS: BLOCKED
  DECISION: continue or DECISION: steer or DECISION: stop
```

### What Happens When Steering Fails

If the parser can't extract all required keys from `output_contract`:

1. `determineTransitionKey()` returns `"complete"` (fallback)
2. `evaluateTransition()` looks up `"complete"` in the transitions map
3. If `"complete"` exists → uses that target
4. If `"complete"` doesn't exist but `"default"` does → uses default
5. If neither exists → **engine stops** (treats as done)

**This is why `"default"` in your transitions map is critical** — it catches all fallthrough cases and prevents silent stops.

### DECISION: stop — The Nuclear Option

`DECISION: stop` is checked BEFORE the full parser runs. It is the highest-priority signal. If the agent outputs `DECISION: stop` anywhere in its response, the engine produces transition key `"stop"` regardless of STATUS or any other keys.

Use this when the verifier determines the work item is complete or fundamentally blocked with no recovery path.

---

## Morty Config Reference

Config files live in `.morty/<name>.json`. Key sections:

### agents

```json
"agents": {
  "builder": {
    "opencode_agent": "dev-axiom",    // OpenCode agent handle
    "prompt_file": "PROMPT.md",          // prompt sent each cycle
    "role": "Implementation agent",
    "context_files": ["specs/README.md"] // optional extra context
  }
}
```

### stages

Each stage runs an agent and defines transitions:

```json
{
  "name": "verify",
  "agent": "verifier",
  "run": "once",                         // or "once_per_cycle"
  "steering": {
    "output_contract": ["STATUS", "DECISION"],
    "on_pass_continue": "build",
    "on_fail_steer": "build",
    "on_blocked_stop": "done"
  },
  "transitions": {
    "continue": "build",
    "steer": "build",
    "stop": "done"
  }
}
```

### settings

```json
"settings": {
  "max_cycles": 50,
  "sleep_between_cycles": "2s",
  "log_dir": ".morty/logs",
  "log_level": "info"
}
```

### done_when

```json
"done_when": {
  "verifier_says_stop": true,
  "max_cycles_reached": true
}
```

---

## Morty Admin API

When started with `--admin --admin-port 9091`, Morty exposes:

| Endpoint | Method | Returns |
|----------|--------|---------|
| `/api/status` | GET | JSON: state, cycle, current_stage, terminal_state, exit_code, work_item_hint, config_name |
| `/api/graph/mermaid` | GET | text/plain: Mermaid flowchart of stage graph |

### Status Response Fields

```json
{
  "state": "running",           // running, done, error, blocked
  "cycle": 12,
  "current_stage": "verify",
  "terminal_state": "",         // "" = healthy, "error" = terminal
  "exit_code": 0,
  "work_item_hint": "my-task",
  "config_name": "my-project"
}
```

---

## Lock Files

Morty writes `.morty/<config-name>.lock` on startup:

```json
{
  "pid": 12345,
  "config": "my-project",
  "started_at": "2026-04-25T10:00:00Z"
}
```

- rick.sh reads the PID and checks `kill -0 <pid>` to verify the process is alive
- Stale lock files (PID dead) are cleaned up on next Morty start
- On terminal error restart, rick-loop.sh explicitly removes the lock before restarting

---

## Fleet Management

### morty-monitor (TUI Dashboard)

For watching multiple Mortys. See `.opencode/skills/morty-monitor-build-axiom/SKILL.md` for build instructions.

```bash
morty-monitor              # live dashboard, auto-discovers all running Mortys
morty-monitor --once       # one-shot snapshot
morty-monitor --refresh 5  # custom refresh interval
```

**Fleet discovery** (no hardcoded list):
1. If `rick-all.sh` exists with a `FLEET=( "name:port" … )` array, parse it
2. Otherwise scan port range 9090–9200 on 127.0.0.1, probe `/api/status`
3. Fail loudly with a hint if neither finds anything

**Keyboard shortcuts**: `q` quit, `r` refresh, `+/-` interval, `p` pause, `↑↓` navigate, `Enter` detail, `l` logs, `e` errors only

### rick-all.sh (Fleet Launcher)

For starting multiple Mortys at once. Define a `FLEET` array:

```bash
FLEET=(
  "frontend:9091"
  "backend:9092"
  "infra:9093"
)
```

---

## Integration with Axiom

### Jira Updates

When a phase completes:
```bash
curl -sf http://127.0.0.1:9091/api/status | python3 -c "
import json,sys
s = json.load(sys.stdin)
print(f'Morty: {s[\"cycle\"]} cycles, state={s[\"state\"]}')
"
```

### Memory Bank Updates

After each watch window, update:
- `.memory-bank/work-items/<id>/runs/<run-id>/` — run evidence
- `.memory-bank/findings/` — any new anti-patterns discovered

### Feeding New Work

When Morty finishes a phase (TODO phase marked complete):
1. Read `.memory-bank/TODO.md` for the next incomplete phase
2. Update the TODO with the next batch of tasks
3. Optionally update the morty config if the next phase needs different agents
4. Report to user: "Phase X complete. Starting Phase Y with N tasks."

### Gate System Exit Signals

When Morty is running a bug-fix work item, pm-axiom and dev-axiom enforce a 7-gate sequence automatically. Rick watches for these signals in Morty's output:

| Signal | Where It Appears | Rick's Action |
|--------|-----------------|---------------|
| `HARD BLOCK` (Gate 1 or Gate 3) | Morty output / exit state | Do NOT auto-restart. Report to user and wait for human approval. |
| `[SPECULATIVE]` in PR title | PR title / verification.md `reproduction: unconfirmed` | Report to user. Not a blocker — loop continues, but user should know before merging. |
| `Strategy Falsification (inline)` | Morty output | Note to user: Gate 3 ran lightweight (cost budget hit). Not a blocker. |
| User says "skip staleness" / "proceed anyway" | User instruction | Record `override=staleness-check` in work item + verification.md `## Staleness Decision`. Resume Morty. |

Full gate sequence: Gate 1 (Staleness) → Gate 2 (Bug Fix Mode) → Gate 3 (Strategy Falsification) → Gate 4 (Reproduce-or-Flag) → Gate 5 (Live/Dead Path) → Gate 6 (PR Scope) → Gate 7 (Post-PR Review Bot)

See `specs/20-Meta-Planning.md#gate-order` for full semantics. See SKILL.md §Bug-Fix Gate Awareness for Rick's handling instructions.

---

## Traceability

```
axiom:trace work_item=morty-upgrade-01 spec=specs/67-Go-Agent-Orchestration-Engine.md jira_ref=DEX-455
```

Source findings:
- `2026-04-13-rick-and-morty-agent-guided-loop-skill-candidate.md`
- `2026-04-12-morty-sleep-monitor-report-pattern.md`
- `2026-04-12-morty-no-auto-restart-daemon-mode.md`
- `2026-04-13-morty-monolithic-sleep-replaced-with-tight-loop.md`
