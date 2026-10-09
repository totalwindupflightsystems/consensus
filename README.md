# Consensus

![CONSENSUS](docs/social-preview.png)

**Stop building agents on sand. Your database is the runtime.**

---

## Your Agent Framework Is Lying To You

88% of agentic AI projects never reach production. Not because of bad prompts.
Not because of model limitations. Because of **state**.

The orchestration frameworks — LangChain, CrewAI, AutoGPT, Bee Agent — share
the same architecture: a Python script holding your agent's thoughts in a
`dict`, a JSON blob, or an in-memory vector store. Then it crashes. Everything
your agent learned is gone. The API credits are burned. The context window
overflowed three iterations ago. And you can't trace *why* it made that
decision because the reasoning was evicted from the buffer.

**That's not an agent. That's a slot machine with a nice README.**

The coding agents — Claude Code, OpenCode, Cursor, Copilot, pi-agent, Hermes,
OpenClaw — are worse. Every session starts from zero. No memory across
invocations. No audit trail of what was tried and failed. The agent fixes a
bug on Tuesday, reintroduces it on Thursday because it has no idea it already
solved this problem. You paste in the same 400 lines of context every time.
You repeat yourself more than the model does.

**That's not autonomous. That's a fancy autocomplete with a CLI.**

---

## The Database *Is* The Agent

Consensus flips the architecture. The database is not a sidecar bolted on
after the fact to save artifacts. The database **is** the execution engine.

- Agent context is a live SQL VIEW, not a Python dict
- Agent memory is an append-only ledger enforced by database triggers — not a
  JSON file that gets overwritten on the next save
- Every state change is ACID-committed or fully rolled back — no "the agent
  thinks it saved but the filesystem disagrees"
- Session isolation is enforced at the DB layer — Agent A physically cannot
  read Agent B's data

This isn't theoretical. It's running. Right now. With real DeepSeek API calls.

### Proof — Live Demo Output

```
╔══════════════════════════════════════════════════════════════╗
║     CONSCIENCE — Real LLM-Powered Agent Harness Demo        ║
╚══════════════════════════════════════════════════════════════╝

━━━ DEMO 2: Multi-Topic Sessions ━━━
   ┌─ Demo 2a (Security) ─────────────────────────────
   │ Status: idle | Iterations: 2
   │ Memory events: 4
  💬 Cross-site request forgery (CSRF): an attacker tricks a user
      into performing unwanted actions on a trusted site
  💬 Cross-site scripting (XSS): an attacker injects malicious
      scripts into web pages viewed by other users
  💬 SQL injection: an attacker inserts malicious SQL code into
      input fields to manipulate database queries
   └─────────────────────────────────────────

━━━ DEMO 3: Crash Recovery ━━━
   ✓ Committed agent artifact recorded before crash
   ✓ Observed in-flight status "planning"
   💥 Server killed while work was in flight
   ✓ Database intact on disk
   Recovery diagnostics: staging_buffer={...} stranded=N stranded_entries=[...]
   ✓ Server restarted
   ✓ Same committed agent artifact remains after restart
```

Real LLM calls. Real committed agent output survives `kill -9`. The open
transaction does not: it rolls back atomically, while any durable
`staging_buffer` residue is reported as recovery diagnostics.

Run it yourself:
```bash
DEEPSEEK_API_KEY=$DEEPSEEK_API_KEY go test -v -run TestDemo -timeout 300s ./demo/
```

---

## What Your Current Framework Can't Do

| Your Framework | What Happens | Consensus |
|---------------|-------------|------------|
| Agent commits a value, server restarts | Gone. Start over. Burn more tokens. | SQLite WAL preserves the committed artifact; heartbeat can recover active session state. Proven. |
| Agent makes a bad decision | Good luck finding *why*. The reasoning was evicted 5 turns ago. | Append-only `memory_events` ledger. Full audit trail. Every thought, every tool call, permanently recorded. |
| Context window hits 128K tokens | Hope your manual truncation didn't cut anything important. | Vector-validated compression. Every summary must pass cosine similarity ≥0.85 against the original. Fail → escalate. No guesswork. |
| "I know the agent mentioned the API key issue somewhere" | `grep` through log files. Good luck. | Semantic retrieval (harness-internal). Embed your query, get ranked results by cosine similarity inside memory compression. No user-facing search endpoint — retrieval powers the compression worker, not a public API. |
| Two agents running concurrently | Shared dicts. Race conditions. Agent B overwrites Agent A's state. | Session-scoped memory. DB-level isolation. Physically impossible to cross-contaminate. |
| API rate limit triggers a retry loop | Agent retries 400 times before you notice. $200 gone. | `agent_circuit_breakers`. 2 consecutive errors → session pauses. Configurable per session. (LLM error paths in planning and task-claim iterations) |
| Agent "saves" its work | Did it actually save? Did the file write complete? Who knows? | ACID transactions. Commit fully or rollback entirely. No partial state. Ever. |

### Your Coding Agent Has Amnesia

| Tool | What It Can't Do | Consensus |
|------|-----------------|------------|
| Claude Code, OpenCode, Cursor, Copilot | Session starts from zero. Every. Single. Time. | Persistent sessions with append-only memory. Agent remembers what it did yesterday. |
| pi-agent, Hermes, OpenClaw | "I already fixed this bug. Why am I fixing it again?" | Full audit trail. Every decision, every tool call, every fix — permanently recorded and queryable. |
| All of them | You paste context. It responds. You paste more context. It responds again. | The agent owns its context. It queries its own memory. It decides what's relevant. |
| All of them | No shared state between parallel agents. Each lives in its own silo. | Multi-session with DB-level isolation. Agents can spawn sub-agents that share a memory ledger. |
| All of them | $50 debugging session, 40 turns deep, terminal crashes. Gone. | SQLite WAL preserves committed session, ledger, and audit rows. The open transaction rolls back; uncommitted model output and paid tokens are not claimed as recovered. |

---

## The Test Suite Doesn't Lie

```bash
go test -short -count=1 ./...
```

The default short suite is keyless. The key-gated crash E2E launches a real
server, makes real DeepSeek API calls, first records committed agent output,
then starts another iteration, observes an in-flight status, kills the server,
and verifies the same committed artifact after restart. It also reports
staged/executed `staging_buffer` residue. The open transaction is expected to
roll back; the test does not claim that
uncommitted work or every paid token survives. Vector-validated compression
uses a cosine similarity ≥0.85 acceptance threshold.

---

## 5-Minute Setup

### Option 1: Build from the public repository (recommended)

The repository is anonymously cloneable, so the source build is the working
fresh-user path:

**Requirements:** building requires the Go toolchain — go.mod targets
`go 1.26.0` (toolchain `go1.26.5`); install Go from https://go.dev/dl/ if
`go version` fails. Node.js and Bun are not required to build or run Consensus;
both are required only for the optional `make test-opencode-upstream` suite. Its
runner does not enforce minimum versions (last verified with Node 22 and Bun
1.4.2).

```bash
git clone https://github.com/totalwindupflightsystems/consensus.git
cd consensus
export DEEPSEEK_API_KEY="sk-..."
go build -o bin/consensus ./cmd/consensus/

# The shipped config explicitly pins database.max_open_conns: 5.
./bin/consensus init --config consensus.yaml
./bin/consensus serve --config consensus.yaml
```

Save the one-time admin key printed by `init`. On first boot, `serve` also
prints the bootstrap admin key once on stdout (stored hashed, never printed
again) — save it when you see it. In a second terminal, verify the
server and open Chronicle:

```bash
curl http://localhost:8090/api/v1/health
# → {"status":"ok",...}
open http://localhost:8090/chronicle/    # macOS
# xdg-open http://localhost:8090/chronicle/  # Linux desktop
```

The config-file path is recommended because its database pool is explicit and
reviewable, but a config file is no longer required for DeepSeek. With only
`DEEPSEEK_API_KEY` set, Consensus selects `https://api.deepseek.com/v1` and the
DeepSeek-compatible `deepseek-v4-flash` model instead of the OpenAI defaults.
Set `CONSENSUS_LLM_BASE_URL`, `CONSENSUS_LLM_MODEL`, or
`CONSENSUS_LLM_PROVIDER` to override those choices. When no
`database.max_open_conns` is supplied, Consensus uses the fixed default pool of
8 connections. The `database.max_open_conns` override applies to both backends:
SQLite passes it to the driver as the connection cap (falling back to 4
connections when the key is unset or 0), and PostgreSQL maps it to the pgx
connection pool's `MaxConns` (fallback 10). The 8-connection default above is
injected by the config loader before the backend ever sees the value. For
example:

```bash
CONSENSUS_DB_URL="sqlite:///tmp/consensus.db" ./bin/consensus init
CONSENSUS_DB_URL="sqlite:///tmp/consensus.db" CONSENSUS_PORT=8124 \
  CONSENSUS_LLM_BASE_URL="https://api.deepseek.com/v1" \
  ./bin/consensus serve
```

> **⚠ Always build before you run.** Never execute a stale binary: the
> repo-root `./consensus` file is a gitignored build artifact from an old
> checkout and goes out of date with every pull, and `bin/consensus`
> must be rebuilt after `git pull` too. Always `go build -o bin/consensus
> ./cmd/consensus/` (or `make build`; `make fresh` also removes the stray
> root binary). Confirm what you're about to run with
> `bin/consensus --version`.

### Option 2: Docker (registry access currently required)

CI publishes `ghcr.io/totalwindupflightsystems/consensus:latest` — the image
path follows this repository
(`github.com/totalwindupflightsystems/consensus`). Anonymous GHCR pulls of it
currently return HTTP 401; package visibility is not something this
repository's documentation can change, so prefer the source build above. If
your GitHub account already has package access, log in first (username: your
GitHub username; password: a personal access token with the `read:packages`
scope):

```bash
docker login ghcr.io
```

Then pull and run:

```bash
docker pull ghcr.io/totalwindupflightsystems/consensus:latest

docker run -d \
  --name consensus \
  -p 8090:8090 \
  -v consensus-data:/home/consensus/data \
  -e DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" \
  ghcr.io/totalwindupflightsystems/consensus:latest

curl http://localhost:8090/api/v1/health
```

**Production (PostgreSQL + pgvector; same GHCR access requirement):**

```bash
docker run -d \
  --name consensus \
  -p 8090:8090 \
  -e CONSENSUS_DB_URL="postgres://user:***@host:5432/consensus?sslmode=require" \
  -e DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" \
  -e CONSENSUS_API_KEY="cs_ak_your_secret_key" \
  ghcr.io/totalwindupflightsystems/consensus:latest
```

#### Port 8090 already in use? (stale sidecar shadowing)

If `serve` dies with an actionable diagnostic naming the occupant class
(instead of a bare `bind: address already in use`), the listen port is
shadowed by a stale `consensus-sidecar` or other leftover process. The
tell-tale symptom: `curl http://localhost:8090/api/v1/health` returns
`404 page not found` instead of `{"status":"ok",...}` — the stale sidecar
owns the port and answers every path with 404.

Identify the occupant read-only (host state — never kill it automatically):

```bash
ss -tlnp | grep :8090
```

Start on a free port instead — either form works:

```bash
CONSENSUS_PORT=8095 ./bin/consensus serve
# or
./bin/consensus serve --port 8095
```

Then verify: `curl http://localhost:8095/api/v1/health` → `{"status":"ok",...}`.

Three commands. You have a running agent harness with:
- Append-only memory ledger
- Vector-validated compression (harness-internal)
- ACID transactions
- Crash recovery
- Circuit breakers
- Session isolation

### First login / API key

On first startup against a fresh database, Consensus prints its first admin
key exactly once (in the terminal output of `consensus init` / `consensus
serve`; with Docker, in `docker logs`):

```
consensus: first_admin_key created=true key=cs_ak_<64 hex chars> key_prefix=<8 chars> id=<uuid> created_at=<RFC 3339> expires_at=<RFC 3339>
consensus: this key expires at <RFC 3339> (… from now)
consensus: save this key now; it is stored hashed and will not be printed again
```

Capture it immediately — the secret is stored only as a hash and never printed
again (restarts print `created=false` with just the `key_prefix`). It has
`admin` scope, so it authenticates every admin endpoint. Use it as Bearer to
mint durable keys of any scope:

```bash
curl -X POST http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $BOOTSTRAP_KEY" \
  -H "Content-Type: application/json" \
  -d '{"scope":"readonly"}'
```

The response includes the new key's secret (`api_key`). Key management
(`POST`/`GET /api/v1/auth/keys`, `DELETE /api/v1/auth/keys/{keyID}`) is
documented in [docs/API.md](docs/API.md). Bootstrap keys expire after **90
days** by default; override with `CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS` (`0` =
no expiry).

### Configuration

| Env Var | Default | What it does |
|---------|---------|-------------|
| `DEEPSEEK_API_KEY` | (required) | DeepSeek API key for LLM calls |
| `OPENROUTER_API_KEY` | — | Alternative: use OpenRouter instead of DeepSeek direct |
| `OPENAI_API_KEY` | — | OpenAI API key when the `openai` provider is selected |
| `ANTHROPIC_API_KEY` | — | Anthropic API key when the `anthropic` provider is selected |
| `CONSENSUS_LLM_BASE_URL` | `https://api.deepseek.com/v1` | Override LLM API endpoint |
| `OPENROUTER_BASE_URL` | — | Fallback LLM API endpoint override for OpenRouter |
| `CONSENSUS_LLM_PROVIDER` | `openai` | Override the LLM provider |
| `CONSENSUS_LLM_MODEL` | provider-specific | Override the default LLM model |
| `CONSENSUS_API_KEY` | — | Protect the API with an auth key |
| `CONSENSUS_DB_URL` | `sqlite://$HOME/.consensus/consensus.db` | PostgreSQL or SQLite DSN |
| `CONSENSUS_CONFIG` | auto-discovered | Explicit configuration file path |
| `CONSENSUS_HOSTNAME` | `127.0.0.1` | Server bind address |
| `CONSENSUS_PORT` | `8090` | Server listen port (if occupied by a stale sidecar, see [Port 8090 already in use?](#port-8090-already-in-use-stale-sidecar-shadowing)) |
| `CONSENSUS_SERVER` | `http://localhost:8090` | Server base URL used by CLI client commands |
| `CONSENSUS_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, or `error`) |
| `CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS` | `90d` | Bootstrap admin-key lifetime in hours (`0` disables expiry) |
| `CONSENSUS_CRIER_URL` | `http://localhost:8767` | Crier relay base URL |
| `CONSENSUS_CRIER_AGENT` | — | Crier agent identity and inbox |
| `CONSENSUS_AUTO_SYNC` | — | Auto-refresh model registry interval (e.g. `24h`) |
| `CONSENSUS_MOCK_LLM` | disabled | Enable the mock LLM provider when set to `1` |

**Docker Compose** (`docker-compose.prod.yml` — full stack, Consensus +
PostgreSQL) also requires authenticated access to the currently private GHCR
image:

```yaml
# Consensus — Production Docker Compose
# ====================================
# Consensus + PostgreSQL 16 + pgvector, no dev tools.
#
# Start:   docker compose -f docker-compose.prod.yml up -d --wait
# Stop:    docker compose -f docker-compose.prod.yml down
# Logs:    docker compose -f docker-compose.prod.yml logs -f
#
# Prerequisites:
#   - .env file with DEEPSEEK_API_KEY, CONSENSUS_API_KEY
#   - Docker Engine 24+ with Compose v2
#
# axiom:trace work_item=prod-docker-compose spec=specs/003-database.md

services:
  # ── PostgreSQL 16 with pgvector ──────────────────────────────────
  postgres:
    image: pgvector/pgvector:pg16
    container_name: consensus-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: consensus
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-consensus}
      POSTGRES_DB: consensus
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U consensus -d consensus"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
    ports:
      - "127.0.0.1:5432:5432"   # local-only; don't expose to network
    networks:
      - consensus-net

  # ── Consensus Runtime ────────────────────────────────────────────
  # Image published by .github/workflows/ci.yml from this repo. Anonymous
  # GHCR pulls currently return 401: run `docker login ghcr.io` first with
  # a personal access token that has read:packages.
  consensus:
    image: ghcr.io/totalwindupflightsystems/consensus:latest
    container_name: consensus-runtime
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "8090:8090"
    command:
      - serve
      - --db-url
      - postgres://consensus:${POSTGRES_PASSWORD:-consensus}@postgres:5432/consensus?sslmode=disable
      - --hostname
      - "0.0.0.0"
      - --port
      - "8090"
      - --log-level
      - ${CONSENSUS_LOG_LEVEL:-info}
    environment:
      - CONSENSUS_API_KEY=${CONSENSUS_API_KEY}
      - DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY}
      - CONSENSUS_LLM_BASE_URL=${CONSENSUS_LLM_BASE_URL:-}
      - CONSENSUS_AUTO_SYNC=${CONSENSUS_AUTO_SYNC:-24h}
    volumes:
      - consensus-data:/home/consensus/data
    healthcheck:
      test: ["CMD-SHELL", "curl -sf http://localhost:8090/api/v1/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s
    networks:
      - consensus-net

volumes:
  pgdata:
    name: consensus-pgdata
  consensus-data:
    name: consensus-data

networks:
  consensus-net:
    name: consensus-net
    driver: bridge
```

> The dev `docker-compose.yml` is PostgreSQL-only — it exists to run the local
> integration tests (Makefile `test-pg*` targets, `internal/migrate/postgres_full_test.go`),
> not the Consensus server. For a full Consensus + PostgreSQL stack use
> `docker compose -f docker-compose.prod.yml up -d`.

---

## Documentation

- **[HTTP API Reference](docs/API.md)** — every REST endpoint with request/response examples, auth requirements, and error codes
- **[Integration Guide](docs/INTEGRATION.md)** — connect external systems: MCP clients (SSE + stdio) and the H3 brain-swap adapter, with worked examples
- **[CLI Exit Codes](docs/CLI-EXIT-CODES.md)** — per-verb exit-code table (0/1/2 + 3–7), the scriptable-CLI rationale, and `set -e`-safe usage examples
- **[Testing and QA](docs/testing/README.md)** — canonical test plan, machine manifest, quorum verdicts, evidence layout, and executable battery guards

  MCP clients attach to a running `consensus serve` on three surfaces, all JSON-RPC 2.0 (the server listens on `127.0.0.1:8090` by default; worked examples in the guide's §1):

  - **Streamable HTTP** — `POST /mcp` with a JSON-RPC request body answers a JSON-RPC response (`application/json`); the `initialize` response carries your session id in the `Mcp-Session-Id` header (send it back on every follow-up call). `GET /mcp` serves the SSE stream for server-initiated messages.
  - **SSE over HTTP** — `GET /mcp/sse` opens the event stream; its first `endpoint` event carries the POST target (`data: /mcp/message?sessionId=<id>`). POST each JSON-RPC message there with that `sessionId` — a missing one is a 400, a stale one is 410 Gone.
  - **stdio** — `consensus mcp-stdio` speaks JSON-RPC on stdin/stdout (authenticate with `--api-key cs_ak_...` or `CONSENSUS_API_KEY`).

  The SSE handshake, end to end:

  ```bash
  curl -N http://127.0.0.1:8090/mcp/sse
  # → event: endpoint
  #   data: /mcp/message?sessionId=<YOUR_SESSION_ID>
  ```

- **[Quickstart (cross-platform)](docs/quickstart-cross-platform.md)** — Docker, macOS, Linux, WSL2
- **[OpenAPI spec](specs/018-openapi-contract.md)** — the REST contract is served at `/openapi.json` and `/openapi.yaml`, with its Swagger UI at `/doc/api`; the opencode-compatible shim contract is served separately at `/doc` (all are embedded in the binary and available from any working directory and in the Docker image)
- **[Muster client generation](docs/MUSTER.md)** — generate a CLI, MCP server, and Go library from the served spec with [muster](https://github.com/wojons/muster): `openapi-cli generate http://localhost:8090/openapi.json` (no hand-written glue; the muster-facing contract is pinned by tests)
- **[Dogfood reports](docs/dogfood/)** — real-use integration reports (findings + per-item resolution status)
- **[Debugging guide](docs/DEBUGGING.md)** — the pprof debug listener: capturing goroutine/heap/CPU profiles from a running server, plus a wedge playbook

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       Consensus                              │
│                                                               │
│  ┌──────────┐   ┌──────────┐   ┌───────────────────────┐    │
│  │ REST API │   │ Harness  │   │ Compression Worker    │    │
│  │ (chi)    │   │ (core    │   │ (vector-validated     │    │
│  │          │   │  loop)   │   │  summarization)       │    │
│  └────┬─────┘   └────┬─────┘   └───────────┬───────────┘    │
│       │               │                     │                │
│       └───────────────┼─────────────────────┘                │
│                       │                                      │
│              ┌────────┴────────┐                             │
│              │  SQLite / PG    │                             │
│              │                 │                             │
│              │  sessions       │  ← agent identity           │
│              │  memory_events  │  ← append-only ledger       │
│              │  event_embeddings ← semantic retrieval        │
│              │  staging_buffer │  ← staged SQL execution     │
│              │  circuit_breakers ← safety limits             │
│              │  compression_queue ← summarization            │
│              │  audit_logs     │  ← full traceability        │
│              └─────────────────┘                             │
└──────────────────────────────────────────────────────────────┘
```

## License

MIT

---

**[Run the demo →](demo/)** &nbsp;|&nbsp; **[Specifications →](specs/)** &nbsp;|&nbsp; **[Deployment →](deploy/)** &nbsp;|&nbsp; **[Cross-Platform Guide →](docs/quickstart-cross-platform.md)**
