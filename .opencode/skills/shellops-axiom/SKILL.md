---
name: shellops-axiom
description: >-
  ShellOps is the Axiom sensory and action layer for shell environments. Covers
  terminal management, log intelligence, action classification, operating modes,
  and the sensory model (ears/triage/instinct). Load this skill when building or
  integrating with ShellOps, interpreting shell session data, classifying shell
  actions, or configuring the ShellOps daemon.
version: "2.0"
tags:
  vertical: [operations, coding, observability]
  category: shell-operations
  core: false
spec: specs/114-ShellOps-PRD.md
---

# ShellOps

ShellOps is the Axiom layer that understands, monitors, and acts on shell environments. It provides structured visibility into what's happening in terminals, classifies actions by safety level, and surfaces actionable intelligence to agents and operators.

**Specs**: `specs/114-ShellOps-PRD.md` through `specs/119-ShellOps-Operating-Modes.md`
**Binary**: `shellops/cmd/shellops/` (Go, pure-Go SQLite via modernc.org/sqlite — no CGO)
**Plugin**: `.opencode/plugins/shellops.ts` (26 MCP tools)
**Tests**: `.opencode/plugins/shellops.test.ts` (28 live-daemon integration tests)
**Config**: `.shellops/config.yaml`
**Runtime DB**: `.shellops/shellops.db` (SQLite WAL, auto-created on startup, gitignored)

<!-- axiom:trace work_item=shellops-01 spec=specs/114-ShellOps-PRD.md -->

---

## When to Load This Skill

Load when an agent needs to:
- Start the ShellOps daemon and verify it's healthy
- Monitor shell sessions for events (command execution, output, errors)
- Classify shell actions by safety level (SAFE/CAUTIOUS/DANGEROUS/FORBIDDEN)
- Extract intelligence from log files or terminal output
- Configure or troubleshoot the ShellOps daemon
- Run ops investigations using Graph Harness templates
- Integrate ShellOps events into Axiom workflows

---

## Quick Start — Building and Running the Daemon

### Step 1: Find Go

Go is NOT in the default PATH on Axiom workspaces. Find it:

```bash
# Check known locations (try in order):
GO=$(find /home/coder -path "*/toolchain@v0.0.1-go1.25.0.linux-amd64/bin/go" 2>/dev/null | head -1)
# OR: check _tmp/go-install
GO="${GO:-/home/coder/code/Axiom/_tmp/go-install/go/bin/go}"
# OR: GOPATH variant
GO="${GO:-/home/coder/code/Axiom/_tmp/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin/go}"
echo "Using Go: $GO"
$GO version
```

### Step 2: Build the Binary

```bash
cd /home/coder/code/Axiom/shellops
export GOPATH=/home/coder/code/Axiom/_tmp/gopath
$GO build -o /home/coder/code/Axiom/_tmp/shellops-bin ./cmd/shellops/
echo "Built: _tmp/shellops-bin"
```

### Step 3: Start the Daemon

The daemon MUST be started in a way that survives shell timeouts. Use parenthesized subshell:

```bash
cd /home/coder/code/Axiom
(_tmp/shellops-bin start --port 9876 --root . >> _tmp/shellops-daemon.log 2>&1 &)
sleep 2
curl -s http://127.0.0.1:9876/health
```

**CLI syntax** (v2 uses subcommands):
- `shellops-bin start --port 9876 --root .` — Start the daemon
- `shellops-bin stop --root .` — Stop the running daemon
- `shellops-bin version` — Print version
- `shellops-bin help` — Show usage

**Important**: Do NOT use `nohup ... &` or `setsid ... &` in bash tool calls — these get killed when the shell session times out (120s default). The parenthesized subshell `(cmd &)` is the only reliable pattern.

### Step 4: Verify

```bash
# Health (must return status:ok)
curl -s http://127.0.0.1:9876/health

# Classify a command
curl -s -X POST http://127.0.0.1:9876/api/v1/classify \
  -H 'Content-Type: application/json' \
  -d '{"command":"kubectl get pods"}'

# Check status
curl -s http://127.0.0.1:9876/api/v1/status
```

### Stopping the Daemon

```bash
# Graceful stop via CLI
_tmp/shellops-bin stop --root .

# OR kill by PID
pkill -f "shellops-bin.*9876"
```

---

## Architecture

```
Terminal / Shell Session
        ↓
   Ears (event listeners)    ← specs/116, 119
        ↓
   Log Intelligence          ← specs/117
        ↓
   Action Classification     ← specs/118 (SAFE/CAUTIOUS/DANGEROUS/FORBIDDEN)
        ↓
   Triage / Instinct         ← specs/119, 120
        ↓
   ShellOps Daemon (:9876)   ← specs/115
        ↓
   Plugin (shellops.ts)      ← OpenCode plugin surface (26 tools)
```

---

## Action Classification (spec 118) — The Safety Boundary

Every command is classified before execution:

| Level | Meaning | Agent behavior |
|-------|---------|----------------|
| **SAFE** | Read-only or benign | Auto-execute |
| **CAUTIOUS** | Side effects, reversible | Auto-execute + verify |
| **DANGEROUS** | Destructive or high-risk | Block, require human approval |
| **FORBIDDEN** | Never allowed via automation | Hard block, cannot override |

Classification is config-driven per environment (see `.shellops/config.yaml`).

---

## Configuration (`.shellops/config.yaml`)

```yaml
action_classification:
  environment: production   # production (CAUTIOUS default), staging/dev (SAFE default)
  rules:
    - pattern: "DROP DATABASE"
      level: FORBIDDEN
      reason: "Database deletion is never allowed via automation"
    - pattern: "kubectl delete"
      level: DANGEROUS
      environments: [production, staging]
    - pattern: "kubectl get"
      level: SAFE

resilience:
  spawn_budget:
    max_agents_per_investigation: 5
    max_total_agents: 20
    max_agent_cost_usd: 10.00
```

---

## Plugin Surface — 26 MCP Tools

The OpenCode plugin (`.opencode/plugins/shellops.ts`) exposes all functionality as MCP tools:

### Execution & Classification
| Tool | Description |
|------|-------------|
| `shellops_exec` | Execute a classified shell command |
| `shellops_classify` | Classify a command's safety level |
| `shellops_status` | Daemon health and stats |
| `shellops_health` | Quick health check |

### Terminal Management
| Tool | Description |
|------|-------------|
| `shellops_terminal_create` | Create a persistent terminal session |
| `shellops_terminal_run` | Run a command in a terminal |
| `shellops_terminal_capture` | Capture terminal output |
| `shellops_terminal_list` | List active terminals |
| `shellops_terminal_destroy` | Kill a terminal session |

### Log Intelligence & Watches
| Tool | Description |
|------|-------------|
| `shellops_watch_start` | Watch a file for pattern matches |
| `shellops_watch_query` | Query watch match results |
| `shellops_watch_list` | List active watches |
| `shellops_watch_stop` | Stop a file watch |
| `shellops_logs_query` | Query structured log entries |
| `shellops_logs_similar` | Find similar log patterns |

### Events & Listeners
| Tool | Description |
|------|-------------|
| `shellops_events_query` | Query captured events |
| `shellops_events_listen` | Start an event listener |
| `shellops_events_stop` | Stop an event listener |

### Service Profiles & Ops
| Tool | Description |
|------|-------------|
| `shellops_profile_load` | Load a service profile |
| `shellops_profile_query` | Query service operational knowledge |
| `shellops_broadcast` | Send alerts to Slack/PagerDuty |
| `shellops_investigate` | Run a structured investigation |
| `shellops_triage` | Triage incident signals → severity + template |

### Nohup/Background Process Tracking (spec 116 §4)
| Tool | Description |
|------|-------------|
| `shellops_nohup_list` | List tracked detached processes |
| `shellops_nohup_check` | Check status of a tracked process |
| `shellops_nohup_output` | Read tail of nohup output file |

### Plugin Hooks

- `tool.execute.before` → Action classification enforcement
- `system.transform` → Ops context injection (service profile, watches, events)
- `on_session_idle` → Watch results surfacing (fires when session becomes idle)

---

## SQLite Schema (6 tables)

ShellOps persists data in `.shellops/shellops.db` (auto-created, WAL mode, gitignored):

| Table | Purpose |
|-------|---------|
| `action_log` | All classified shell actions with timestamps |
| `log_entries` | Extracted log events from watched files |
| `watch_matches` | Pattern match results from file watches |
| `file_watches` | Active file watch registrations |
| `events` | Captured events from listeners |
| `tracked_processes` | Nohup/background process tracking (PID, command, status, output file) |

---

## Integration with Other Plugins

ShellOps is designed to work with the full Axiom plugin suite:

| Plugin | Integration |
|--------|-------------|
| **Graph Harness** | Triage recommends graph templates; ops investigations run as DAGs |
| **Context Stash** | Investigation findings stored in shared stash |
| **Tree Memory** | Durable investigation state with branch-per-agent |
| **Conductor** | Background agent spawning for parallel investigations |
| **Code Intelligence** | Blast radius analysis for proposed fixes |

### Graph Harness Templates (ready to use)

Pre-built ops templates in `.graph-harness/templates/`:
- `ops-incident-investigation.yaml` — triage → investigate → classify → remediate → evidence
- `ops-credential-rotation.yaml` — sequential with approval gates for DANGEROUS steps
- `ops-deployment-verification.yaml` — pre/post deploy checks
- `ops-capacity-investigation.yaml` — resource analysis
- `ops-runbook-execution.yaml` — single-agent runbook following

---

## Key Source Files

| Path | Purpose |
|------|---------|
| `shellops/cmd/shellops/` | CLI entrypoint (start/stop/version subcommands) |
| `shellops/internal/daemon/daemon.go` | Main daemon (~1100 lines) with all route registrations |
| `shellops/internal/db/db.go` | SQLite schema creation and connection |
| `shellops/internal/db/nohup.go` | Nohup CRUD operations |
| `shellops/internal/logwatch/logwatch.go` | Log file watching (SetDB/LoadFromDB/SetRedactor) |
| `shellops/internal/classifier/classifier.go` | Action classification + command substitution detection |
| `shellops/internal/config/config.go` | Config with RootDir and resilience markers |
| `shellops/internal/resilience/` | Circuit breaker for daemon stability |
| `shellops/internal/sensory/` | Ears, triage, profiles, broadcaster, proprioception |
| `.opencode/plugins/shellops.ts` | Plugin (26 tools, hooks) |
| `.opencode/plugins/shellops.test.ts` | 28 live-daemon integration tests |
| `.shellops/config.yaml` | Runtime configuration |

---

## Daemon API Reference

Base URL: `http://127.0.0.1:9876` (env: `SHELLOPS_PORT`, default: 9876)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Quick health + uptime |
| GET | `/api/v1/status` | Full status (terminals, watches, env, panic mode) |
| POST | `/api/v1/classify` | Classify a command → SAFE/CAUTIOUS/DANGEROUS/FORBIDDEN |
| POST | `/api/v1/terminal/create` | Create terminal session |
| POST | `/api/v1/terminal/send` | Run command in terminal |
| POST | `/api/v1/terminal/read` | Capture terminal output |
| GET | `/api/v1/terminal/list` | List terminals |
| POST | `/api/v1/terminal/kill` | Destroy terminal |
| POST | `/api/v1/watch/start` | Start file watch |
| GET | `/api/v1/watch/query` | Query watch results |
| GET | `/api/v1/watch/list` | List watches |
| POST | `/api/v1/watch/stop` | Stop watch |
| GET | `/api/v1/logs/query` | Query log entries |
| GET | `/api/v1/logs/similar` | Similar log patterns |
| GET | `/api/v1/events/query` | Query events |
| POST | `/api/v1/events/listen` | Start listener |
| POST | `/api/v1/events/stop` | Stop listener |
| GET | `/api/v1/profiles/query` | Query service profiles |
| POST | `/api/v1/broadcast` | Send alert |
| POST | `/api/v1/investigate` | Run investigation |
| POST | `/api/v1/triage` | Triage signals → severity |
| GET | `/api/v1/nohup/list` | List tracked processes |
| GET | `/api/v1/nohup/check?id=N` | Check process status |
| GET | `/api/v1/nohup/output?id=N` | Tail process output |

---

## Troubleshooting

### Daemon won't start
- Check if port 9876 is already in use: `lsof -i :9876`
- Check logs: `cat _tmp/shellops-daemon.log`
- Verify binary was rebuilt after code changes

### Daemon dies after 2 minutes
- This means it was started inside a bash tool call that timed out
- Fix: use `(cmd &)` subshell pattern, NOT `nohup` or `setsid`

### All endpoints return 404
- The running binary is stale (compiled before route changes)
- Fix: rebuild (`go build -o _tmp/shellops-bin ./cmd/shellops/`) and restart

### `go build` can't find Go
- Go is NOT in default PATH on Axiom workspaces
- Find it: `find /home/coder -path "*/bin/go" -name "go" 2>/dev/null`
- Common path: `_tmp/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin/go`

---

## Running Tests

```bash
# Go unit tests (all 16 packages)
GO=/path/to/go
export GOPATH=/home/coder/code/Axiom/_tmp/gopath
cd shellops && $GO test ./... -count=1 -timeout=120s

# TypeScript plugin build verification
cd .opencode && bun build plugins/shellops.ts --outfile /dev/null

# Live daemon integration tests (requires daemon running on :9876)
cd .opencode && bun test plugins/shellops.test.ts
```

---

## Known Limitations (v0.1)

- Linux eBPF deep inspection: Phase 2
- Distributed session correlation: Phase 3
- Real-time OS-level action blocking (seccomp/AppArmor): Planned for FORBIDDEN enforcement
- Windows PTY support: Not planned
- Broadcaster: runs in simulated mode unless `SHELLOPS_SLACK_WEBHOOK` is set

axiom:trace work_item=shellops-01 spec=specs/114-ShellOps-PRD.md
