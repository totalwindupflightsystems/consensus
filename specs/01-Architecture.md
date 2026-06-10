# Architecture — Conscience

## Context
Conscience is a **database-native cognitive architecture** for AI agents. Instead of running agents in a Bash terminal with filesystem state, the database (PostgreSQL or SQLite) IS the runtime environment — the memory ledger, security sandbox, event bus, and cognitive loop.

The LLM is inherently non-deterministic. It must be paired with the most deterministic system in computing: a relational database. Conscience replaces the Bash/file-system execution layer with SQL as the agent's native language for state management, memory, and self-improvement.

The system is a single Go binary that embeds the full harness loop, REST API, MCP server, CLI, and dual database drivers (Postgres + SQLite). There is no external state — the database is the single source of truth.

## Components

### 1. Go Binary (`conscience`)
Single statically-linked binary containing all subsystems:
- **Harness Loop**: Reads `active_context_view`, sends Markdown to LLM, receives JSON payload, executes SQL in transactions, handles rollback/retry
- **REST API**: Session CRUD, approval review, event streaming, health checks
- **MCP Server**: Model Context Protocol for external tool integration (stdio and HTTP transports)
- **CLI**: 16 commands — init, serve, session, migrate, config, approve, memory, tool, status, mcp-stdio, completion
- **DB Driver Interface**: Abstraction over Postgres (pgx/v5) and SQLite (embedded), the LLM never knows which backend is active

### 2. Database Layer (Postgres / SQLite)
Core tables that form the agent's cognitive substrate:
- `memory_events`: Append-only ledger — never updated, never deleted. Complete cognitive history with embeddings.
- `active_context_view`: Dynamic SQL VIEW combining recent events with compressed summaries, providing the LLM's working memory
- `sessions`: Agent lifecycle — status (booting/idle/thinking/executing/completed/failed/paused), parent_id for sub-agents, heartbeat
- `tasks`: Async event queue for deferred work
- `tool_requests` / `tool_results`: External tool dispatch and result storage
- `skills_registry`: Progressive disclosure of agent capabilities
- `iteration_commits`: Time-travel snapshots for rollback
- `agent_billing`: Cost tracking with circuit breakers
- `api_keys`: SHA-256 hashed keys with scoped roles (admin/session/readonly/webhook)

### 3. External Tool Runner
Sandboxed subprocess execution for agent-requested commands:
- 30s timeout per execution, 1MB output cap
- Temp working directory isolation, environment variable whitelist
- Channel semaphore: max 10 concurrent subprocesses
- Rate limiting: SQL-backed per-tool per-minute caps
- HITL approval gating for destructive/schema-change operations

### 4. LLM Provider Layer
Pluggable provider adapters (OpenAI API-compatible):
- Real HTTP clients for OpenAI and Anthropic with Structured Outputs and prompt caching
- Factory pattern with config-driven wiring
- LM Studio, Ollama, and any OpenAI-compatible endpoint supported via `base_url` override
- Embedding support for vector operations (compression, semantic search)

### 5. Compression Worker
Background goroutine that:
- Polls for uncompressed memory events above cosine similarity threshold
- Generates tier-1 summaries via LLM, validates against original embeddings
- Progresses events through tier-1 → tier-2 → tier-3 compression
- Updates `display_modes` to control what the LLM sees in `active_context_view`

### 6. HITL (Human-in-the-Loop) System
- Approval requests for destructive operations and schema changes
- Session pause/resume with `paused` status
- Configurable auto-pause on consecutive error threshold
- Circuit breaker integration — trips on budget overrun, error storms, or approval timeout

### 7. OpenCode Shim
Adapter layer that makes Conscience sessions appear as OpenCode agents:
- Service adapter mapping Conscience session lifecycle to OpenCode agent protocol
- HTTP server shim for OpenCode-compatible endpoints
- Enables Axiom and Hermes to drive Conscience agents as if they were OpenCode instances

## Data
- **Primary store**: PostgreSQL (production) or SQLite (local dev) — database IS the state machine
- **Event sourcing**: Append-only `memory_events` table with sequential IDs, never modified or deleted
- **Snapshots**: `iteration_commits` table provides time-travel rollback points
- **Context windows**: `active_context_view` — a SQL VIEW that dynamically assembles the LLM's working memory from recent events + compressed summaries
- **Config**: `conscience.yaml` (default), overridable via env vars (`CONSCIENCE_*`)

## Interfaces
- **REST API**: `POST /v1/sessions` (create), `GET /v1/sessions/:id` (status), `POST /v1/sessions/:id/messages` (send), `GET /v1/sessions/:id/stream` (SSE), `POST /v1/approvals/:id` (review), `GET /health`
- **MCP**: stdio transport for IDE integration, HTTP transport for remote tools. Tools: create_session, send_message, get_session_status, list_memory, review_approval, query_tool
- **LLM Protocol**: OpenAI-compatible chat completions API — any provider speaking this protocol works
- **Database Driver**: Go `database/sql` interface with pgx/v5 (Postgres) and mattn/go-sqlite3 (SQLite) backends

## Threat Model
- **LLM is untrusted**: Every output treated as potentially malicious. RLS scopes all queries to `session_id`. Hallucinated `DELETE` only affects agent's own data.
- **SQL injection**: Parameterized queries enforced at the harness level. Raw SQL from LLM JSON payloads validated against schema before execution.
- **API key exposure**: Keys stored as SHA-256 hashes, never plaintext after generation. Prefixed `cs_` for identification without revealing full key.
- **Sandbox escape**: External tools run in isolated subprocess with env whitelist, temp dir, 30s timeout. Cannot access database directly.
- **Denial of wallet**: Per-session budget limits, circuit breakers on consecutive errors, rate limiting on tool execution.
- **Cross-session leakage**: RLS at Postgres kernel level, API Rules + Go hook middleware at PocketBase level. Session A cannot read Session B's data.

## Operational Notes
- **Start**: `./conscience serve` starts REST API + harness loop
- **Init**: `./conscience init` creates SQLite database + runs migrations
- **Migrate**: `./conscience migrate up` applies pending migrations (1-15)
- **Build**: `go build -o conscience ./cmd/conscience` produces ~18MB binary
- **Test**: `go test ./...` — 27 packages, all passing
- **Config**: `conscience.yaml` in working directory or `CONSCIENCE_CONFIG` env var
- **Database**: Default `sqlite://dev.db`, switch to Postgres via `database.url: postgres://...`
- **LLM**: Default provider/model in config, overridable via `CONSCIENCE_LLM_PROVIDER`, `CONSCIENCE_LLM_BASE_URL`, etc.
