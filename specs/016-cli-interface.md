# SPEC-016: CLI Interface

**Status:** Draft
**Depends On:** SPEC-015 (API & External Interface Layer), SPEC-009 (Deployment)
**Created:** 2026-04-12

---

## 1. Overview

The `consensus` CLI is a management tool for the Consensus agent runtime. It is NOT a TUI or a chat interface — users interact with agents through their preferred AI tools (opencode, aider, continue.dev, etc.) via the API shims defined in SPEC-017. The CLI handles operational tasks: starting the server, managing sessions, reviewing approvals, running migrations, and inspecting system state.

The CLI is a thin client that talks to the Consensus server via the REST API defined in SPEC-015. It does not connect to the database directly.

---

## 2. Design Principles

1. **Management, not interaction** — The CLI operates the runtime. Chat happens through AI tools.
2. **Server-dependent** — Almost all commands require a running Consensus server. The CLI is a REST client, not a direct DB client.
3. **Scriptable** — All output available as JSON (`--format json`). Designed for pipes, cron jobs, and automation.
4. **Zero config by default** — Connects to `http://localhost:8090` unless told otherwise. Auth via `CONSENSUS_API_KEY` env var.

---

## 3. Installation

### Supabase Path

```bash
npm install -g consensus-cli
# or
brew install consensus/tap/consensus
```

### Binary Path

The CLI is embedded in the Go binary. No separate installation.

```bash
consensus serve                    # starts server (harness + API + MCP)
consensus --help                   # management commands
```

---

## 4. Global Flags

| Flag | Environment Variable | Default | Description |
|---|---|---|---|
| `--server` | `CONSENSUS_SERVER` | `http://localhost:8090` | Server base URL |
| `--api-key` | `CONSENSUS_API_KEY` | — | API key for authentication |
| `--format` | — | `table` | Output format: `table`, `json`, `yaml` |
| `--quiet` | — | `false` | Suppress non-essential output |
| `--config` | — | `./consensus.yaml` or `~/.consensus/config.yaml` | Config file path |

---

## 5. Commands

### 5.1 `consensus serve`

Start the Consensus server (REST API, protocol adapters, MCP server, harness heartbeat).

```bash
consensus serve [flags]

Flags:
  --port          Port to listen on (default: 8090)
  --hostname      Bind address (default: 127.0.0.1)
  --mcp           Enable MCP server endpoint (default: true)
  --adapter       Enable TUI protocol shims: opencode, pi-agent (default: opencode)
  --db-url        Database connection URL (PocketBase: ignored, uses embedded SQLite)
  --migrations    Path to migration files (default: ./migrations)
  --log-level     Log level: debug, info, warn, error (default: info)
```

Starts the Go binary with harness loop, REST API, and MCP server. With SQLite, the database is embedded. With Postgres, connects to the specified database.

### 5.2 `consensus init`

Bootstrap a new Consensus instance. Creates tables, default configuration, and an admin API key.

```bash
consensus init [flags]

Flags:
  --db-url        Database connection URL (required for Postgres)
  --supabase      Initialize for Supabase deployment
  --pocketbase    Initialize for PocketBase deployment (default: auto-detected)
  --llm-key       LLM API key (stored in vault or env)
  --llm-provider  Default LLM provider: openai, anthropic (default: openai)

Output:
  Admin API key:  cs_ak_a1b2c3d4...
  Server URL:     http://localhost:8090
  Config saved:   ./consensus.yaml
```

This runs the consolidated SQL schema, creates the `hitl_configuration` global defaults, and generates the first admin API key.

### 5.3 `consensus session`

Manage agent sessions.

```bash
# Create a new session
consensus session create --goal "Analyze Q4 revenue data" [--agent-name research] [--model gpt-4o]

# List sessions
consensus session list [--status idle,thinking,paused] [--limit 20]

# Show session details
consensus session show <session-id>

# Tail session events (live stream)
consensus session logs <session-id> [--follow] [--iterations 10]

# Pause a running session
consensus session pause <session-id>

# Resume a paused session
consensus session resume <session-id>

# Cancel a session
consensus session cancel <session-id>

# Get session cost breakdown
consensus session cost <session-id>
```

**Output formats:**

```
# consensus session list --format table
ID         AGENT       STATUS    ITER  COST      AGE
abc-123    researcher  thinking  14    $0.42     5m
def-456    coder       idle       3    $0.08     2m
ghi-789    analyst     paused    22    $1.10     12m

# consensus session list --format json
[
  {"id": "abc-123", "agent_name": "researcher", "status": "thinking", "iteration": 14, "cost_cents": 42, "age": "5m"},
  ...
]
```

### 5.4 `consensus approve`

Human-in-the-loop approval management.

```bash
# List pending approvals
consensus approve list [--session <session-id>] [--risk-level high,critical]

# Show approval details
consensus approve show <approval-id>

# Approve a request
consensus approve <approval-id> [--notes "Looks good"]

# Reject a request
consensus reject <approval-id> --reason "Wrong target table"

# Approve with modification
consensus approve <approval-id> --modified-sql "DELETE FROM temp_cache WHERE created_at < now() - interval '7 days'"
```

**Interactive mode** (when run without flags):

```
$ consensus approve
Pending approvals (3):

  [1] HIGH    Tool: delete_database (session: abc-123)
      "Drop table temp_cache"
      Age: 2m

  [2] MEDIUM  Tool: send_email (session: def-456)
      "Send report to client@corp.com"
      Age: 5m

  [3] LOW     Agent-requested (session: ghi-789)
      "I want to create a new memory page for caching"
      Age: 1m

Approve which? [1-3/a/r/q]: 1
Approved: delete_database for session abc-123
```

### 5.5 `consensus migrate`

Run database migrations.

```bash
# Run all pending migrations
consensus migrate

# Check current schema version
consensus migrate version

# Rollback last migration
consensus migrate rollback

# Create a new migration file
consensus migrate create "add_new_memory_table"
```

### 5.6 `consensus config`

Manage configuration.

```bash
# Get all configuration
consensus config list

# Get a specific value
consensus config get llm.default_model

# Set a value
consensus config set llm.default_model gpt-4o
consensus config set hitl.auto_pause_on_error_threshold 5
consensus config set shim.openai.enabled true

# Edit configuration file in $EDITOR
consensus config edit
```

### 5.7 `consensus status`

System health and metrics.

```bash
# System overview
consensus status

Output:
  Server:          running (uptime: 2h 15m)
  Database:        connected (PostgreSQL 16 / SQLite 3.45)
  Active sessions: 3
  Pending tasks:   1
  Pending approvals: 2
  Harness:         running (heartbeat: 5s)
  Schema version:  0.3.0
  LLM provider:    openai (gpt-4o)
```

### 5.8 `consensus memory`

Inspect agent memory (read-only).

```bash
# List memory events for a session
consensus memory list <session-id> [--type text_block,tool_result] [--limit 50]

# Show a specific memory event
consensus memory show <memory-id>

# Show iteration history
consensus memory iterations <session-id> [--diff]

# Show compressed memory pages
consensus memory pages <session-id>
```

### 5.9 `consensus tool`

Inspect available tools and skills.

```bash
# List registered tools
consensus tool list

# Show tool details
consensus tool show <tool-name>

# List skills
consensus skill list

# Show skill details
consensus skill show <skill-name>
```

---

## 6. Configuration File

### Location Priority

1. `./consensus.yaml` (project-level)
2. `~/.consensus/config.yaml` (user-level)
3. `/etc/consensus/config.yaml` (system-level, Linux only)

### Schema

```yaml
server:
  url: http://localhost:8090
  api_key: cs_ak_...                    # or use CONSENSUS_API_KEY env var

llm:
  default_model: gpt-4o
  provider: openai                       # openai | anthropic
  api_key: sk-...                        # or use OPENAI_API_KEY / ANTHROPIC_API_KEY env var
  max_context_tokens: 128000
  max_output_tokens: 16384

adapters:
  opencode:
    enabled: true                         # implement opencode server protocol for `opencode attach`
  pi-agent:
    enabled: false                        # enable when pi-agent protocol is researched

hitl:
  auto_pause_on_error_threshold: 3
  require_approval_for_destructive: true
  require_approval_for_schema_changes: true
  approval_timeout_minutes: 60

harness:
  heartbeat_interval_seconds: 5
  max_iterations: 100
  max_consecutive_errors: 3
  budget_limit_cents: 1000

database:
  url: postgresql://...                  # Supabase path
  # or embedded for PocketBase path
  migrations_path: ./migrations

logging:
  level: info                            # debug | info | warn | error
  format: text                           # text | json
```

---

## 7. Shell Completion

```bash
# Generate completion script
consensus completion bash > /etc/bash_completion.d/consensus
consensus completion zsh > "${fpath[1]}/_consensus"
consensus completion fish > ~/.config/fish/completions/consensus.fish
```

---

## 8. Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Server unreachable |
| 4 | Authentication failed |
| 5 | Not found |
| 6 | Conflict (wrong state) |
| 7 | Rate limited |

---

## 9. Implementation

### Supabase Path

TypeScript CLI distributed via npm. Uses the REST API (SPEC-015) for all operations.

```
packages/cli/
  src/
    commands/        # One file per command
    client.ts        # REST API client
    config.ts        # Config file management
    output.ts        # Table/JSON/YAML formatting
  package.json
```

Dependencies: `commander`, `ora`, `chalk`, `yaml`

### PocketBase Path

CLI is embedded in the Go binary. Commands are registered as Cobra commands.

```go
// cmd/consensus/main.go
func main() {
    app := pocketbase.New()

    // Built-in PocketBase commands (serve, migrate, etc.)
    // + Consensus commands (session, approve, etc.)

    app.Start()
}
```

---

## 10. PocketBase Parity

| Feature | Supabase | PocketBase |
|---|---|---|
| CLI binary | npm package (`consensus-cli`) | Embedded in Go binary |
| `serve` | Starts Go binary with harness + API + MCP | Starts Go binary with embedded SQLite |
| `init` | Runs SQL script via Supabase client | Runs migrations on embedded SQLite |
| `session *` | REST API calls | REST API calls (same endpoints) |
| `approve *` | REST API calls | REST API calls (same endpoints) |
| `migrate` | Runs SQL migration files | Runs Go migration functions |
| `config` | YAML file | YAML file |

---

## 11. Management Surface

The CLI is the primary management interface. Every management task can be done from the terminal. This table is the canonical list of what needs managing and how the CLI handles it:

### 11.1 Day-to-Day Operations

| Task | CLI Command | What It Shows |
|---|---|---|
| See what's happening | `consensus status` | Active sessions, pending approvals, costs, health |
| List sessions | `consensus session list` | All sessions with status, iteration count, cost, age |
| Check a specific session | `consensus session show <id>` | Full session details, current iteration, memory summary |
| Tail agent activity | `consensus session logs <id> --follow` | Live stream of agent thoughts, tool calls, memory writes |
| Inspect memory | `consensus memory list <id>` | Ordered memory events for a session |
| View iteration history | `consensus memory iterations <id>` | All iteration commits with timestamps |
| See compressed pages | `consensus memory pages <id>` | Memory compression state |
| Check cost | `consensus session cost <id>` | Token usage, cost breakdown per iteration |
| Kill a stuck agent | `consensus session cancel <id>` | Immediately fails the session |
| Pause/resume | `consensus session pause <id>` / `resume <id>` | Pause and resume without data loss |

### 11.2 Approvals (HITL)

| Task | CLI Command | What It Shows |
|---|---|---|
| See pending approvals | `consensus approve list` | All pending with risk level, description, age |
| Filter by risk | `consensus approve list --risk-level high,critical` | Only high/critical |
| See approval details | `consensus approve show <id>` | Full context: agent monologue, target SQL, previous actions |
| Approve | `consensus approve <id>` | Resumes agent |
| Reject | `consensus reject <id> --reason "..."` | Injects rejection into context |
| Modify and approve | `consensus approve <id> --modified-sql "..."` | Runs modified version |
| Interactive mode | `consensus approve` | Walk through pending approvals one by one |

### 11.3 System Administration

| Task | CLI Command | What It Shows |
|---|---|---|
| First-time setup | `consensus init` | Database schema, admin key, config |
| Start server | `consensus serve` | API, shim, MCP, harness heartbeat |
| Check schema version | `consensus migrate version` | Current vs required version |
| Run migrations | `consensus migrate` | Applies pending migrations |
| Rollback migration | `consensus migrate rollback` | Undo last migration |
| Get config | `consensus config list` | All current settings |
| Change config | `consensus config set <key> <value>` | Updates running config |
| Edit config in editor | `consensus config edit` | Opens $EDITOR |
| List tools | `consensus tool list` | All registered tools with status |
| List skills | `consensus skill list` | Skill metadata |
| Check API key scopes | `consensus config list` (shows auth section) | Keys and their scopes |

### 11.4 Observability

| Task | CLI Command | What It Shows |
|---|---|---|
| System overview | `consensus status` | Health, active sessions, pending tasks, costs, uptime |
| Per-session audit | `consensus session logs <id> --iterations 5` | Last N iterations with SQL executed |
| Iteration diff | `consensus memory iterations <id> --diff` | What changed between iterations |
| System metrics | `consensus status --verbose` | Total sessions, total cost, avg cost/session, error rate |
| Billing breakdown | `consensus session cost <id>` | Per-iteration token counts and costs |

---

## 12. Future: Web Admin UI (Not Specced Yet)

The CLI is the only management interface for the initial release. A web admin UI is planned as a future phase. When the time comes, the web UI will be built on top of the same native REST API (SPEC-015) — no special endpoints or logic needed.

### What the Web UI Would Provide (Future)

| View | What It Shows | Maps To |
|---|---|---|
| Dashboard | Active sessions, pending approvals, system health | `GET /api/v1/metrics` |
| Session list | Filterable table of all sessions | `GET /api/v1/sessions` |
| Session detail | Live iteration log, memory browser, cost chart | `GET /api/v1/sessions/:id`, SSE event stream |
| Approval queue | Pending approvals with approve/reject buttons | `GET /api/v1/approvals`, `POST .../review` |
| Memory browser | Searchable memory events with timeline | `GET /api/v1/sessions/:id/memory` |
| Cost dashboard | Charts of spend over time, per-session, per-model | `GET /api/v1/sessions/:id/billing` |
| Audit log | Immutable iteration history with SQL diff | `GET /api/v1/sessions/:id/iterations/:iid/audit` |
| Config editor | Web form for settings | `GET/PATCH /api/v1/config` |
| User/key management | API key CRUD, scope assignment | `api_keys` table management |

### Why It's Not Specced Yet

1. The native API (SPEC-015) already exposes everything a web UI would need
2. The CLI proves the API surface is complete — if the CLI can do it, a web UI can too
3. PocketBase already ships with a built-in admin UI for raw collection/record management
4. Designing a web UI before the system exists is premature — build the runtime first, then design the dashboard around real usage patterns

### Potential Approaches (Future Decision)

- **PocketBase Admin UI extension** — PocketBase has a built-in admin panel. Add custom pages/views to it for Consensus-specific views (approvals, memory, sessions). Zero frontend framework to maintain.
- **opencode web UI** — opencode has a web mode. If we shim it (SPEC-017), we get a web UI for session/chat. Doesn't cover admin/ops views though.
- **Custom SPA** — React/Vue/Svelte dashboard. Full control but full maintenance burden.
- **Terminal dashboard** — Something like `consensus dashboard` using Bubble Tea / charmbracelet. Stays in the terminal but adds real-time panels.
