---
name: axiom-autotune
description: >
  Autonomous configuration optimization via benchmark-driven parameter sweeps. Load when optimizing Axiom settings, running parameter sweeps, managing autotune profiles, or diagnosing suboptimal configuration.
version: "1.0"
tags:
  vertical: ['coding', 'operations']
  category: operations
  core: false
---
# axiom-autotune — Autonomous Configuration Optimization via Benchmark-Driven Parameter Sweeps

Autonomous configuration optimization via benchmark-driven parameter sweeps. Load this skill when optimizing Axiom settings for a specific model and workload, running parameter sweeps, managing autotune profiles, or diagnosing suboptimal configuration.

**Two operating modes:**
- **CLI Mode** — agent runs `axiom autotune` commands directly for quick operations (list, diff, apply, stale check)
- **Supervised Mode** — agent acts as Rick, supervising a Morty-driven autotune sweep over hours/days, reporting progress, handling failures, feeding new parameter combinations

## When to Load

- User asks to optimize Axiom settings for a model or workload
- User wants to run a parameter sweep or benchmark
- User asks about autotune profiles, baselines, or staleness
- User wants to compare current config against an optimized profile
- Agent needs to supervise a long-running autotune sweep
- Agent needs to understand the autotune system to integrate with it
- Performance issues suggest suboptimal configuration

---

## Mode 1: CLI Mode (Direct Agent Control)

Use CLI mode for quick, synchronous operations. The agent runs commands directly and gets immediate results. No Morty process needed.

### Commands

```bash
# List stored profiles — see what's already been optimized
axiom autotune list

# Show diff between a profile and current config
axiom autotune diff --profile claude-sonnet-4-concurrent-users

# Apply a profile to axiom.config.yaml (review diff first!)
axiom autotune apply --profile claude-sonnet-4-concurrent-users

# Check all profiles for staleness (model version changed? search space changed?)
axiom autotune stale

# Establish a baseline with current config (required before a sweep)
axiom autotune baseline --model claude-sonnet-4

# Start a sweep (short-running — for small search spaces only)
axiom autotune run --model claude-sonnet-4 --workload concurrent-users
```

### Slash Command

```
/axiom-autotune list
/axiom-autotune diff --profile claude-sonnet-4-concurrent-users
/axiom-autotune apply --profile claude-sonnet-4-concurrent-users
/axiom-autotune stale
```

### When to Use CLI Mode

- **Listing/diffing/applying profiles** — always CLI mode, instant results
- **Staleness checks** — always CLI mode
- **Baselines** — CLI mode (runs one benchmark, ~5 minutes)
- **Small sweeps** (< 20 combinations) — CLI mode is fine
- **Large sweeps** (20+ combinations, hours of benchmarks) — use Supervised Mode instead

---

## Mode 2: Supervised Mode (Rick Watches Morty Sweep)

Use supervised mode for long-running parameter sweeps. The agent acts as Rick — supervising a Morty-driven sweep, reporting progress, handling failures, and escalating when needed.

**This mode uses the Rick + Morty supervision pattern from `rick-and-morty-axiom` skill.**

### How It Works

```
User ←→ Agent (Rick role)
              ↕
           rick.sh (10s watchdog loop)
              ↕
           Morty (morty run — autotune-sweep template)
              ↕
         OpenCode sessions (running benchmarks)
              ↕
         Target repo (config being optimized)
```

The autotune sweep runs as a **Morty Loop template** with 5 stages:
1. `baseline` — measure current config performance
2. `generate-combos` — create parameter combinations from search space
3. `benchmark-loop` — execute each combination with real workloads
4. `collect-results` — aggregate metrics across all combinations
5. `select-profile` — pick the best combination per objective function

Morty handles the execution. Rick (the agent) handles the supervision.

### Starting a Supervised Sweep

```bash
# 1. Verify prerequisites
morty validate autotune-sweep.json
curl -sf http://127.0.0.1:4096/global/health && echo "OpenCode OK"

# 2. Start Morty with the autotune template
cd <repo-root>
nohup morty run autotune-sweep.json \
  --work-item-hint autotune-01 \
  > .morty/autotune.log 2>&1 &

# 3. Verify Morty started
sleep 3 && morty status

# 4. Start Rick watchdog
while true; do
    ./morty/scripts/rick.sh 300 60 autotune-sweep.json
    exit_code=$?
    case $exit_code in
        0) # Watch window complete — report status
           echo "[rick] Sweep in progress. $(axiom autotune status)"
           continue ;;
        1) # Morty dead — restart
           echo "[rick] Morty died during sweep. Restarting..."
           nohup morty run autotune-sweep.json --work-item-hint autotune-01 > .morty/autotune.log 2>&1 &
           sleep 3
           continue ;;
        2) # OpenCode down — wait and restart
           echo "[rick] OpenCode down. Waiting..."
           until curl -sf http://127.0.0.1:4096/global/health >/dev/null 2>&1; do sleep 30; done
           nohup morty run autotune-sweep.json --work-item-hint autotune-01 > .morty/autotune.log 2>&1 &
           sleep 3
           continue ;;
        3) # New commit — report progress
           echo "[rick] Sweep progress: new benchmark result committed."
           continue ;;
        *) echo "[rick] Unknown exit: $exit_code"; break ;;
    esac
done
```

### Rick's Autotune-Specific Responsibilities

| Responsibility | What Rick Does |
|---|---|
| **Progress reporting** | "Sweep is 47% complete. 23 of 48 combinations benchmarked. Best so far: latency=1.2s, cost=$0.03/request." |
| **Checkpoint monitoring** | Check `.morty/autotune-checkpoint.json` for sweep state. If Morty restarts, it resumes from last checkpoint. |
| **Stale detection** | If the model version changes mid-sweep, alert the user — results may be invalid. |
| **Cost tracking** | Report cumulative sweep cost: "This sweep has cost $4.50 so far across 23 benchmark runs." |
| **Result interpretation** | When sweep completes, summarize: "Best profile for claude-sonnet-4 + concurrent-users: session.ttl=1800s, retry.max=5, context.window=8192. Saves 23% cost vs. baseline with <2% quality loss." |
| **Escalation** | If sweep is stuck (same combination failing repeatedly), escalate: "Combination #24 has failed 3 times. The parameter set {session.ttl=300, retry.max=1} may be too aggressive. Skip it?" |

### Circuit Breakers (Autotune-Specific)

| Condition | Threshold | Action |
|---|---|---|
| Same combination fails 3x | 3 consecutive failures | Skip combination, log finding, continue sweep |
| Sweep cost exceeds budget | Configurable (default: $50) | Pause sweep, escalate to user |
| Morty restarts > 5x/hour | 5 restarts | Stop sweep, escalate |
| No progress for 30 min | 30 min, no new benchmark result | Check Morty logs, escalate if stuck |
| Benchmark latency > 10x baseline | Single benchmark | Flag as outlier, re-run once, skip if still outlier |

### When Sweep Completes

1. Morty writes the profile to `.axiom/autotune-profiles/<model>-<workload>.yaml`
2. Rick reports the results to the user in plain language
3. Rick shows the diff: `axiom autotune diff --profile <name>`
4. Rick asks: "Apply this profile? (axiom autotune apply --profile <name>)"
5. User decides. Rick applies if approved.

---

## Key Concepts

### Search Space

Defined in `.axiom/autotune-search-space.yaml`. Specifies which parameters to sweep and their valid ranges:

```yaml
version: 1
name: "concurrent-users-sweep"
parameters:
  - path: "session.ttl_seconds"
    range: [300, 3600]
    step: 300
    type: integer
  - path: "session.max_reuse"
    range: [10, 100]
    step: 10
    type: integer
```

### Profiles

Stored in `.axiom/autotune-profiles/`. Each profile is a YAML file with the optimized settings, the objective function used, the benchmark results, and a staleness fingerprint.

```
.axiom/autotune-profiles/
├── _baseline/                          # Baseline measurements
└── claude-sonnet-4-concurrent-users.yaml  # Optimized profile
```

### Objectives

- **balanced** — optimize for a weighted combination of cost, latency, and quality
- **cost** — minimize token spend while maintaining quality threshold
- **latency** — minimize response time while maintaining quality threshold
- **quality** — maximize output quality while staying within cost budget

### Sweep Engine (Morty Integration)

AutoTune runs as a Morty Loop template with stages:
1. `baseline` — measure current config performance
2. `generate-combos` — create parameter combinations from search space
3. `benchmark-loop` — execute each combination with real workloads
4. `collect-results` — aggregate metrics across all combinations
5. `select-profile` — pick the best combination per objective function

Supports checkpoint/resume via Morty's SQLite state store. If a sweep is interrupted, `morty run` resumes from the last checkpoint automatically.

### Staleness Detection

Profiles include a fingerprint of the model version, search space hash, and config schema version. When any of these change, the profile is marked stale. Check with:

```bash
axiom autotune stale
```

### Cost Attribution

Every sweep run gets a `sweep_id` that correlates with the cost tracking system (`specs/47-Cost-Tracking-And-Session-Analytics.md`). You can see exactly how much a sweep cost.

## Workload Archetypes

| Archetype | Description | Key Parameters |
|---|---|---|
| `concurrent-users` | Multiple interactive users sharing sessions | session TTL, max reuse, context window |
| `batch-ci` | Automated CI/CD runs, no human interaction | retry timing, timeout, parallelism |
| `large-session` | Long-running autonomous agent sessions | stale threshold, keepalive, checkpoint interval |
| `mixed` | Combination of interactive and automated | all of the above |

## File Locations

| File | Purpose |
|---|---|
| `specs/93-Axiom-AutoTune.md` | Full specification (703 lines, 78 requirements) |
| `.axiom/src/axiom/cli/autotune.py` | CLI implementation (7 subcommands) |
| `.axiom/src/axiom/autotune/` | Core autotune module |
| `.axiom/autotune-search-space.yaml` | Default search space definition |
| `.axiom/autotune-profiles/` | Stored profiles |
| `.opencode/commands/axiom-autotune.md` | Slash command definition |
| `morty/internal/autotune/` | Go sweep engine (runner, checkpoint, combos, state, template) |
| `morty/scripts/rick.sh` | Rick watchdog script (supervision loop) |
| `.axiom/tests/autotune/` | Test suite (6 test files) |
| `.memory-bank/prds/axiom/axiom-autotune.md` | Source PRD |
| `.memory-bank/work-items/autotune-01/` | Work item |

## Related Skills

- `rick-and-morty-axiom` — the Rick + Morty supervision pattern (required for Supervised Mode)
- `performance-benchmark-axiom` — benchmark execution that AutoTune drives
- `chaos-engineer-axiom` — optional resilience testing during sweeps
- `axiom-cost-tracking` — cost attribution for sweep runs
- `axiom-local-dev-test-harness` — local test environment for sweep development

## Spec Reference

`specs/93-Axiom-AutoTune.md` — 703 lines, 78 requirements (REQ-CAT-001 through REQ-CAT-078)

## Traceability

```
axiom:trace work_item=autotune-01 spec=specs/93-Axiom-AutoTune.md jira_ref=DEX-438
```
