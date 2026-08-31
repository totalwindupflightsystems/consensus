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
   💥 Server killed (simulating crash)
   ✓ Database intact on disk
   ✓ Server restarted
   ✓ Session data intact — crash recovery works
```

Real LLM calls. Real agent output. Survives kill -9. Try that with LangChain.

Run it yourself:
```bash
DEEPSEEK_API_KEY=$DEEPSEEK_API_KEY go test -v -run TestDemo -timeout 300s ./demo/
```

---

## What Your Current Framework Can't Do

| Your Framework | What Happens | Consensus |
|---------------|-------------|------------|
| Agent computes a value, server restarts | Gone. Start over. Burn more tokens. | SQLite WAL. Heartbeat auto-resumes. Proven. |
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
| All of them | $50 debugging session, 40 turns deep, terminal crashes. Gone. | SQLite WAL. Heartbeat resumes active sessions on restart. Every token you paid for is preserved. |

---

## The Test Suite Doesn't Lie

```
36/36 packages green — zero failures
```

Not mocked. Not simulated. The E2E tests launch a real server, make real
DeepSeek API calls, kill the server mid-session, and verify the agent
resumes cleanly on restart. Vector-validated compression with cosine
similarity ≥0.85 acceptance threshold.

---

## 5-Minute Setup

### Option 1: Docker (recommended)

Pull the image and run. That's it.

```bash
# Pull the image
docker pull ghcr.io/wojons/consensus:latest

# Run with SQLite (zero config — data persists in a volume)
docker run -d \
  --name consensus \
  -p 8090:8090 \
  -v consensus-data:/home/consensus/data \
  -e DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" \
  ghcr.io/wojons/consensus:latest

# Verify it's alive
curl http://localhost:8090/api/v1/health
# → {"status":"ok","version":"0.1.0","uptime_seconds":8,"api_latency_ms":0,"db_latency_ms":0.1,
#    "llm_latency_ms":0,"error_rate_pct":0,"db_backend":"sqlite","db_path":"/home/consensus/data/consensus.db",
#    "db_size_mb":0.00390625,"db_tables":37,"db_migrations":22,"schema_version":23,
#    "active_connections":{"websocket":0,"db_pool_active":0,"db_pool_max":0,"llm_active":0,"api_requests_last_min":0},
#    "system_log":[]}
# Key fields: status (ok|degraded|unhealthy), db_backend (sqlite|postgres), schema_version.

# Check the Chronicle dashboard
open http://localhost:8090/chronicle/
```

**Production (PostgreSQL + pgvector):**

```bash
docker run -d \
  --name consensus \
  -p 8090:8090 \
  -e CONSENSUS_DB_URL="postgres://user:pass@host:5432/consensus?sslmode=require" \
  -e DEEPSEEK_API_KEY="$DEEPSEEK_API_KEY" \
  -e CONSENSUS_API_KEY="cs_ak_your_secret_key" \
  ghcr.io/wojons/consensus:latest
```

### Option 2: Go binary (local development)

```bash
go build -o bin/consensus ./cmd/consensus/
./bin/consensus init
./bin/consensus serve
```

> **⚠ Always build before you run.** Never execute a stale binary: the
> repo-root `./consensus` file is a gitignored build artifact from an old
> checkout and goes out of date with every pull, and `bin/consensus`
> must be rebuilt after `git pull` too. Always `go build -o bin/consensus
> ./cmd/consensus/` (or `make build`; `make fresh` also removes the stray
> root binary). Confirm what you're about to run with
> `bin/consensus --version`.

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
| `CONSENSUS_LLM_BASE_URL` | `https://api.deepseek.com/v1` | Override LLM API endpoint |
| `CONSENSUS_API_KEY` | — | Protect the API with an auth key |
| `CONSENSUS_DB_URL` | `sqlite://$HOME/.consensus/consensus.db` | PostgreSQL or SQLite DSN |
| `CONSENSUS_PORT` | `8090` | Server listen port (if occupied by a stale sidecar, see [Port 8090 already in use?](#port-8090-already-in-use-stale-sidecar-shadowing)) |
| `CONSENSUS_AUTO_SYNC` | — | Auto-refresh model registry interval (e.g. `24h`)

**Docker Compose** (docker-compose.prod.yml — full stack, Consensus + PostgreSQL):

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: consensus
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-consensus}
      POSTGRES_DB: consensus
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U consensus -d consensus"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

  consensus:
    image: ghcr.io/wojons/consensus:latest
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
    environment:
      - CONSENSUS_API_KEY=${CONSENSUS_API_KEY}
      - DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY}
    volumes:
      - consensus-data:/home/consensus/data
    healthcheck:
      test: ["CMD-SHELL", "curl -sf http://localhost:8090/api/v1/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s

volumes:
  pgdata:
  consensus-data:
```

> The dev `docker-compose.yml` is PostgreSQL-only — it exists to run the local
> integration tests (Makefile `test-pg*` targets, `internal/migrate/postgres_full_test.go`),
> not the Consensus server. For a full Consensus + PostgreSQL stack use
> `docker compose -f docker-compose.prod.yml up -d`.

---

## Documentation

- **[HTTP API Reference](docs/API.md)** — every REST endpoint with request/response examples, auth requirements, and error codes
- **[Integration Guide](docs/INTEGRATION.md)** — connect external systems: MCP clients (SSE + stdio) and the H3 brain-swap adapter, with worked examples
- **[Quickstart (cross-platform)](docs/quickstart-cross-platform.md)** — Docker, macOS, Linux, WSL2
- **[OpenAPI spec](specs/018-openapi-contract.md)** — machine-readable contract served at `/openapi.json` and `/openapi.yaml` (embedded in the binary — available from any working directory and in the Docker image), with the REST API Swagger UI at `/doc/api` on a running server (`/doc` is the opencode shim's own Swagger UI)
- **[Dogfood reports](docs/dogfood/)** — real-use integration reports (findings + per-item resolution status)

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
