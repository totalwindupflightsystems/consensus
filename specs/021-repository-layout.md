# SPEC-021: Repository Layout & Go Project Structure

**Status:** Draft
**Depends On:** All previous specs (000-020)
**Created:** 2026-04-12

---

## 1. Overview

This spec defines how the Consensus repository is organized, which Go packages exist, what lives where, and how spec sections map to implementation packages. The goal: a developer can read this file and know exactly where to find or add code.

---

## 2. Top-Level Layout

```
conscientiousness/
├── cmd/
│   └── consensus/           # Main binary entry point
│       └── main.go
├── internal/                 # Private application packages
│   ├── harness/              # Agent iteration loop (SPEC-008, SPEC-020)
│   ├── api/                  # REST API handlers (SPEC-015)
│   ├── mcp/                  # MCP server (SPEC-015 §5)
│   ├── db/                   # Database driver interface + implementations
│   │   ├── db.go             # Interface definition
│   │   ├── postgres/         # pgx implementation
│   │   └── sqlite/           # modernc.org/sqlite implementation
│   ├── tools/                # Tool registry + execution sandbox (SPEC-010)
│   ├── secrets/              # Secret injection & scrubbing (SPEC-005)
│   ├── security/             # RLS context, statement classifier, execution policy (SPEC-005, SPEC-011 §8)
│   ├── memory/               # Context formatting, memory page management (SPEC-002)
│   ├── session/              # Session lifecycle, status transitions (SPEC-011 §1)
│   ├── subagent/             # Sub-agent spawning, memory forking (SPEC-004)
│   ├── hitl/                 # Human-in-the-loop approvals (SPEC-014)
│   ├── webhook/              # Webhook ingestion + event routing (SPEC-013)
│   ├── billing/              # Cost tracking, budget circuit breakers (SPEC-006)
│   ├── migrate/              # Schema migration runner
│   ├── shim/                 # opencode server protocol shim (SPEC-017)
│   │   └── opencode/         # opencode-specific translation
│   ├── llm/                  # LLM client abstraction (OpenAI, Anthropic)
│   ├── config/               # Configuration loading (YAML, env, flags)
│   └── cli/                  # CLI command definitions (SPEC-016)
├── migrations/               # SQL migration files
│   ├── 001_initial_schema.sql
│   ├── 002_shim_session_map.sql
│   ├── 003_circuit_breakers.sql
│   ├── 004_staging_buffer.sql
│   ├── 005_agent_budget_limits.sql
│   ├── 006_dynamic_entity_generator.sql
│   ├── 007_webhook_tables.sql
│   ├── 008_hitl_tables.sql
│   ├── 009_json_schema_support.sql
│   ├── 010_quarantine_scanner.sql
│   ├── 011_tool_sandbox.sql
│   ├── 012_trust_level.sql
│   ├── 013_active_context_view_enhanced.sql
│   ├── 014_projects_and_scope.sql
│   └── 015_embedding_model.sql
├── specs/                    # Design specifications (this directory)
│   ├── 000-north-star.md
│   └── ...
├── memory-bank/              # Agent context files
├── go.mod
├── go.sum
├── Makefile                  # Build, test, lint targets
├── Dockerfile                # Multi-stage build for container deployment
├── consensus.yaml           # Default configuration file
└── README.md
```

---

## 3. Package Details

### 3.1 `cmd/consensus/` — Binary Entry Point

Minimal. Parses config, initializes logger, starts server.

| File | Purpose |
|---|---|
| `main.go` | Entry point — config, DB open, migrate, serve |

### 3.2 `internal/db/` — Database Driver Interface

The critical abstraction. Everything else depends on this interface, never on a specific driver.

```go
package db

type DB interface {
    BeginTx(ctx context.Context) (Tx, error)
    Exec(ctx context.Context, query string, args ...any) error
    Query(ctx context.Context, query string, args ...any) ([]Row, error)
    QueryRow(ctx context.Context, query string, args ...any) (Row, error)
    Backend() Backend
    Close() error
}

type Tx interface {
    Exec(ctx context.Context, query string, args ...any) error
    Query(ctx context.Context, query string, args ...any) ([]Row, error)
    QueryRow(ctx context.Context, query string, args ...any) (Row, error)
    SetSessionContext(ctx context.Context, sessionID string) error
    Commit() error
    Rollback() error
    IsActive() bool
}
```

Postgres implementation wraps `pgx/v5` with `SET LOCAL consensus.session_id` on `SetSessionContext`.
SQLite implementation wraps `modernc.org/sqlite` with Go-layer session injection on `SetSessionContext`.

| File | Purpose |
|---|---|
| `db.go` | Interface definition (`DB`, `Tx`, `Row`, `Config`, `Backend`) |
| `driver/driver.go` | `Open()` — URL-based factory that wires Postgres or SQLite |
| `postgres/postgres.go` | PostgreSQL implementation via pgx/v5 |
| `postgres/notify.go` | PostgreSQL LISTEN/NOTIFY support |
| `sqlite/sqlite.go` | SQLite implementation via modernc.org/sqlite |
| `dynamic/dynamic.go` | Dynamic table generation and management |
| `jsonschema/validator.go` | JSON Schema validation for memory_events content |

### 3.3 `internal/harness/` — Agent Iteration Loop

The core runtime. Implements SPEC-008 (one-shot) and SPEC-020 (interactive transaction).

| File | Purpose |
|---|---|
| `harness.go` | Harness types, `New()`, `readModelPricing()`, `calculateCostUSD()` |
| `executor.go` | `RunAgentIteration()`, LLM call dispatch, transaction execution |
| `context.go` | Context formatting, `FormatContextAsMarkdown()` |
| `prompt.go` | System prompt template assembly |
| `parser.go` | JSON response parsing from LLM |
| `planning.go` | Interactive multi-turn planning (SPEC-020) |
| `circuit.go` | Circuit breaker logic for error rate limiting |
| `audit.go` | Audit log persistence after each iteration |
| `tool_executor.go` | Async tool execution loop, polling `tool_requests` |

### 3.4 `internal/api/` — REST API

HTTP handlers for SPEC-015 endpoints.

| File | Purpose |
|---|---|
| `server.go` | Router setup, middleware (auth, rate limiting) |
| `service.go` | Shared service layer — sessions, messages, config, tools, metrics |
| `types.go` | Shared type definitions (SessionResponse, etc.) |
| `helpers.go` | Shared helpers (newUUID, sha256, toInt, toString, etc.) |
| `sessions.go` | `/api/v1/sessions/*` handlers |
| `memory.go` | `/api/v1/sessions/:id/memory/*` handlers |
| `tasks.go` | `/api/v1/sessions/:id/tasks/*` handlers |
| `tools.go` | `/api/v1/tools/*`, `/api/v1/skills/*` handlers |
| `approvals.go` | `/api/v1/approvals/*` handlers (HITL) |
| `billing.go` | `/api/v1/sessions/:id/billing`, `/api/v1/metrics` |
| `events.go` | SSE handler for real-time event stream |
| `openapi.go` | OpenAPI spec endpoints |
| `quarantine.go` | Quarantine review endpoints |
| `doc.go` | Package documentation |

### 3.5 `internal/mcp/` — MCP Server

Model Context Protocol server from SPEC-015 §5.

| File | Purpose |
|---|---|
| `server.go` | MCP protocol handler, JSON-RPC dispatch, SSE transport |
| `server_test.go` | MCP server tests |
| `tools.go` | `tools/list` and `tools/call` — create_session, send_message, get_session_status, list_memory, review_approval, query_tool |
| `resources.go` | Resource handlers (`consensus://sessions/*`, `consensus://tools`), prompts handlers |
| `auth.go` | MCP authentication (API key validation), auth helpers, shared type converters |
| `auth_test.go` | MCP auth tests |
| `stdio.go` | MCP stdio transport (JSON-RPC 2.0 over stdin/stdout) |
| `stdio_test.go` | MCP stdio transport tests |
| `doc.go` | Package documentation |

### 3.6 `internal/tools/` — Tool Execution

Implements SPEC-010 tool registry, execution sandbox, and JIT tool loading.

| File | Purpose |
|---|---|
| `tools.go` | Static tool definitions and registration for harness |
| `execute.go` | Tool execution dispatch (SQL function, HTTP endpoint, native Go) |
| `sandbox.go` | Subprocess isolation, timeout enforcement |
| `approval.go` | Tool approval gating for HITL-sensitive operations |
| `rate_limiter.go` | Per-tool rate limiting to prevent abuse |

### 3.7 `internal/security/` — SQL Safety

Implements SPEC-011 §8 statement classifier and execution policy.

| File | Purpose |
|---|---|
| `classifier.go` | SQL statement classification (DML_READ, DML_WRITE, DDL, DANGEROUS) |
| `executor.go` | SQL execution policy, table whitelist enforcement |
| `triggers.go` | Security trigger detection and logging |

### 3.8 `internal/secrets/` — Secret Management

Implements SPEC-005 zero-knowledge secrets.

| File | Purpose |
|---|---|
| `secrets.go` | `{{SECRET.X}}` alias replacement, secret injection, LLM response scrubbing |

### 3.9 `internal/memory/` — Memory Management

Implements SPEC-002 context window and memory pages.

| File | Purpose |
|---|---|
| `memory.go` | Read `active_context_view`, format for LLM, memory management |

### 3.10 `internal/session/` — Session Lifecycle

Implements SPEC-011 §1 session status state machine.

| File | Purpose |
|---|---|
| `session.go` | Create, transition, validate status changes |
| `complete.go` | Session completion logic, result summarization |

### 3.11 `internal/subagent/` — Sub-Agent Management

Implements SPEC-004 spawning, memory forking, parent wake.

| File | Purpose |
|---|---|
| `subagent.go` | Sub-agent creation, memory forking, parent wake on completion |

### 3.12 `internal/hitl/` — Human-in-the-Loop

Implements SPEC-014 approval requests and notifications.

| File | Purpose |
|---|---|
| `hitl.go` | Create approval request, process decision, risk assessment, notification dispatch |

### 3.13 `internal/webhook/` — Webhook Ingestion

Implements SPEC-013 event routing.

| File | Purpose |
|---|---|
| `webhook.go` | HTTP handler for `/webhooks/:source`, HMAC signature verification, event routing |

### 3.14 `internal/billing/` — Cost Tracking

Implements SPEC-006 budget limits and circuit breakers.

| File | Purpose |
|---|---|
| `tracker.go` | Per-iteration cost recording, budget limit checking |

### 3.15 `internal/llm/` — LLM Client

Abstraction over OpenAI and Anthropic APIs. Uses raw HTTP for both providers — no SDK dependency required.

| File | Purpose |
|---|---|
| `client.go` | Interface definition, factory (ProviderOpenAI/Anthropic/Mock/OpenRouter), config validation |
| `openai_client.go` | OpenAI / OpenRouter HTTP implementation with response_format (json_object, json_schema strict mode) |
| `anthropic_client.go` | Anthropic Messages API HTTP implementation with prompt caching (cache_control ephemeral) |
| `embedding.go` | Embedding API client for memory compression pipeline |
| `embedding_test.go` | Embedding client tests |
| `client_test.go` | Factory, config, and mock client tests |
| `doc.go` | Package documentation |

### 3.16 `internal/shim/opencode/` — opencode Protocol Shim

Implements SPEC-017. Translates opencode server protocol requests to native Consensus API calls.

| File | Purpose |
|---|---|
| `server.go` | opencode-compatible HTTP routes, all endpoint handlers |
| `service_adapter.go` | Adapter bridging `api.Service` → `opencode.Service` interface |
| `doc.go` | Package documentation |

### 3.17 `internal/cli/` — CLI Commands

Implements SPEC-016 using Cobra.

| File | Purpose |
|---|---|
| `root.go` | Root command, global flags |
| `serve.go` | `consensus serve` |
| `init.go` | `consensus init` |
| `session.go` | `consensus session` (list, create, cancel) |
| `approve.go` | `consensus approve` |
| `migrate.go` | `consensus migrate` (up, down, status) |
| `config.go` | `consensus config` (get, set) |
| `status.go` | `consensus status` |
| `memory.go` | `consensus memory` (list, search) |
| `tool.go` | `consensus tool` (list, execute) |
| `client.go` | HTTP client for Consensus API |
| `completion.go` | Shell completion generation |
| `formatter.go` | Output formatting (table, JSON, text) |

### 3.18 `internal/config/` — Configuration

| File | Purpose |
|---|---|
| `config.go` | Configuration struct definition, load from YAML/env/flags |

### 3.19 `internal/migrate/` — Schema Migrations

Migration runner with embedded SQL files. Maintains dual copies: `migrations/` (source of truth) and `internal/migrate/migrations/` (embedded via `//go:embed`). Both must be kept in sync.

| File | Purpose |
|---|---|
| `migrate.go` | Migration runner (embedded SQL files with `//go:embed`) |
| `migrate_test.go` | Migration runner tests |
| `doc.go` | Package documentation |
| `migrations/` | Embedded SQL migration files (duplicate of `migrations/` root) |

### 3.20 `internal/compression/` — Memory Compression Pipeline

Implements WI-012 vector-validated memory compression with tier escalation.

| File | Purpose |
|---|---|
| `compression.go` | Compression logic, cosine similarity, tier escalation |
| `compression_test.go` | Compression unit tests |
| `worker.go` | Background compression worker with configurable poll interval and batch size |
| `worker_test.go` | Worker tests |

### 3.21 `internal/bootstrap/` — Initialization

| File | Purpose |
|---|---|
| `admin_key.go` | Admin API key generation and bootstrapping |
| `admin_key_test.go` | Admin key tests |

### 3.23 `internal/quarantine/` — External Data Scanning

Implements SPEC-005 external data quarantine pipeline.

| File | Purpose |
|---|---|
| `adapter.go` | Quarantine adapter interface for different data sources |
| `scanner.go` | Quarantine scanning pipeline |
| `scanner_test.go` | Scanner tests |
| `service.go` | Quarantine service orchestration |
| `service_test.go` | Service tests |
| `doc.go` | Package documentation |

### 3.24 `internal/web/` — Web Server

Thin HTTP server wrapper around the internal API and MCP handlers.

| File | Purpose |
|---|---|
| `server.go` | HTTP server setup, graceful shutdown |
| `server_test.go` | Server tests |
| `doc.go` | Package documentation |

---

## 4. Dependency Graph

```
cmd/consensus
  ├── internal/cli
  ├── internal/config
  ├── internal/db
  │   ├── internal/db/postgres
  │   └── internal/db/sqlite
  ├── internal/harness
  │   ├── internal/db
  │   ├── internal/llm
  │   ├── internal/memory
  │   ├── internal/security
  │   ├── internal/secrets
  │   ├── internal/session
  │   ├── internal/tools
  │   ├── internal/billing
  │   └── internal/subagent
  ├── internal/api
  │   ├── internal/db
  │   ├── internal/session
  │   ├── internal/hitl
  │   ├── internal/billing
  │   └── internal/secrets
  ├── internal/mcp
  │   ├── internal/db
  │   ├── internal/session
  │   ├── internal/hitl
  │   └── internal/tools
  ├── internal/shim/opencode
  │   ├── internal/api
  │   └── internal/db
  ├── internal/webhook
  │   └── internal/db
  └── internal/migrate
      └── internal/db
  ├── internal/web
  │   ├── internal/api
  │   └── internal/mcp
  ├── internal/quarantine
  │   └── internal/db
  └── internal/bootstrap
      └── internal/db
```

Key rule: **dependencies flow inward.** `internal/harness` depends on `internal/db`, but `internal/db` depends on nothing else. `internal/api` depends on `internal/db` but NOT on `internal/harness`. No circular dependencies.

---

## 5. External Dependencies

| Dependency | Purpose | Required |
|---|---|---|
| `github.com/jackc/pgx/v5` | Postgres driver + connection pooling | Yes |
| `modernc.org/sqlite` | Pure-Go SQLite driver (no CGO) | Yes |
| `github.com/go-chi/chi/v5` | HTTP router (zero dependencies) | Yes |
| `github.com/spf13/cobra` | CLI framework | Yes |
| `github.com/google/uuid` | UUID generation | Yes |
| `github.com/santhosh-tekuri/jsonschema/v5` | JSON Schema validation | Yes |
| `gopkg.in/yaml.v3` | YAML config parsing | Yes |

**Note:** LLM providers use raw HTTP (no SDK). MCP server is a custom implementation (no SDK). Migration runner is custom (no goose/golang-migrate). This keeps the dependency tree minimal and eliminates SDK boilerplate.

No CGO. No Node.js. No Python. Build with `CGO_ENABLED=0 go build`.

---

## 6. Build Targets

```makefile
# Build binary
build:
	CGO_ENABLED=0 go build -o bin/consensus ./cmd/consensus

# Run with SQLite (zero dependencies)
dev:
	go run ./cmd/consensus serve --db sqlite://dev.db

# Run with local Postgres
dev-pg:
	go run ./cmd/consensus serve --db postgres://localhost:5432/consensus

# Run tests
test:
	go test ./internal/... -v

# Run linter
lint:
	golangci-lint run ./...

# Docker build
docker:
	docker build -t consensus .
```

---

## 7. Dockerfile

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /consensus ./cmd/consensus

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /consensus /usr/local/bin/consensus
COPY consensus.yaml /etc/consensus/consensus.yaml
EXPOSE 8090
ENTRYPOINT ["consensus"]
CMD ["serve"]
```

---

## 8. Spec-to-Package Mapping

| Spec | Primary Package(s) | Secondary Package(s) |
|---|---|---|
| SPEC-000 North Star | (philosophy, no code) | — |
| SPEC-001 Architecture | (principles, no code) | — |
| SPEC-002 Memory | `internal/memory` | `internal/harness/context.go` |
| SPEC-003 Database | `internal/db`, `migrations/` | `internal/migrate` |
| SPEC-004 Subagents | `internal/subagent` | `internal/memory` |
| SPEC-005 Security | `internal/security`, `internal/secrets` | `internal/db` (RLS), `internal/quarantine` |
| SPEC-006 Transactions | `internal/harness`, `internal/billing` | `internal/db` |
| SPEC-007 JSON Schema | `internal/db/jsonschema` | `internal/db` (CHECK constraints in migrations) |
| SPEC-008 Harness | `internal/harness` | `internal/llm`, `internal/tools` |
| SPEC-009 Deployment | `Dockerfile`, `Makefile`, `consensus.yaml` | `cmd/consensus` |
| SPEC-010 Tools | `internal/tools` | `internal/security/classifier.go` |
| SPEC-011 Canonical Definitions | (authority spec, cross-cutting) | All packages |
| SPEC-012 System Prompt | `internal/harness/prompt.go` | `internal/memory` |
| SPEC-013 Webhooks | `internal/webhook` | `internal/api` |
| SPEC-014 HITL | `internal/hitl` | `internal/api/approvals.go` |
| SPEC-015 API & MCP | `internal/api`, `internal/mcp` | `internal/cli`, `internal/web` |
| SPEC-016 CLI | `internal/cli` | `cmd/consensus` |
| SPEC-017 UI Adapter | `internal/shim/opencode` | `internal/api` |
| SPEC-018 OpenAPI | `internal/api/openapi.go` (auto-generated) | — |
| SPEC-019 User Flows | (UX spec, no direct code) | All user-facing packages |
| SPEC-020 Interactive Transactions | `internal/harness/planning.go` | `internal/session` |
| SPEC-021 Repository Layout | (this file) | — |

---

## 9. Implementation Order

Phased approach — each phase produces a testable artifact:

### Phase 1: Foundation (can run `go build`)
1. `go.mod` + dependencies
2. `internal/config` — configuration loading
3. `internal/db` — interface + Postgres + SQLite implementations
4. `migrations/001_initial_schema.sql` — all tables from SPEC-003
5. `internal/migrate` — migration runner
6. `cmd/consensus/main.go` — entry point that connects, migrates, starts

### Phase 2: Harness Loop (can run an agent iteration)
7. `internal/llm` — OpenAI + Anthropic clients
8. `internal/security` — statement classifier, execution policy
9. `internal/secrets` — injection + scrubbing
10. `internal/session` — status state machine
11. `internal/harness` — one-shot iteration loop (SPEC-008), executor, context, parser
12. `internal/harness/planning.go` — multi-turn planning (SPEC-020)

### Phase 3: API Surface (can create sessions via HTTP)
13. `internal/api` — REST endpoints (SPEC-015)
14. `internal/billing` — cost tracking
15. `internal/cli` — management commands (SPEC-016)

### Phase 4: Integration (can use from external tools)
16. `internal/mcp` — MCP server (SPEC-015 §5)
17. `internal/webhook` — webhook ingestion (SPEC-013)
18. `internal/hitl` — approval flow (SPEC-014)

### Phase 5: Shims (can use existing tools' UIs)
19. `internal/shim/opencode` — opencode server protocol (SPEC-017)

---

*SPEC-021 — Repository Layout — Consensus Framework*
