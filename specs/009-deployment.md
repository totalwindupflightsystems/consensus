# SPEC-009: Deployment

> **Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

## Overview

Conscience is a **single Go binary** that contains the harness, REST API, and MCP server. It connects to any supported database backend via a driver interface. There is one deployment model — the binary — with two database options:

1. **Postgres** — Supabase Cloud, self-hosted Supabase, or any Postgres provider
2. **SQLite** — embedded in the binary, zero external dependencies

The user experience is identical regardless of database backend. The LLM doesn't know which database it's running on. It sees the same schema, reads the same views, outputs the same JSON.

---

## The Binary

```bash
# Build
go build -o conscience ./cmd/conscience

# Run with SQLite (local dev, zero dependencies)
./conscience serve --db sqlite://conscience.db

# Run with local Postgres
./conscience serve --db postgres://localhost:5432/conscience

# Run with self-hosted Supabase
./conscience serve --db postgres://localhost:5432/postgres

# Run with Supabase Cloud
./conscience serve --db postgres://postgres:pass@db.xxx.supabase.co:5432/postgres

# Run with any Postgres provider (Neon, Railway, RDS, etc.)
./conscience serve --db postgres://user:pass@your-postgres-host:5432/conscience
```

One binary. One flag. Same runtime.

### What's Inside the Binary

| Component | Description |
|---|---|
| Harness loop | Long-running goroutine that polls for tasks and runs agent iterations |
| REST API | HTTP server (SPEC-015) serving `/api/v1/*` endpoints |
| MCP server | Model Context Protocol server (stdio + SSE) |
| Heartbeat | Cron-based task claiming at configurable interval |
| CLI | Management commands (SPEC-016): `init`, `migrate`, `session`, `approve`, `config` |
| Database drivers | `pgx` for Postgres, `modernc.org/sqlite` for SQLite |
| Schema migrations | Embedded SQL migration files, auto-applied on startup |

### Configuration

```yaml
# conscience.yaml (or flags, or env vars)
db: postgres://localhost:5432/conscience
listen: ":8090"
heartbeat_interval: 5s
llm:
  default_model: gpt-4o
  api_keys:
    openai: sk-...
    anthropic: sk-ant-...
harness:
  planning:
    max_turns: 10
    transaction_timeout_ms: 60000
    auto_commit_on_max: true
```

---

## Database Backends

### Postgres (any provider)

The full Conscience feature set is available on Postgres:

| Feature | Mechanism |
|---|---|
| Row-Level Security | Native RLS policies |
| Session isolation | `SET LOCAL conscience.session_id` (auto-resets at COMMIT/ROLLBACK) |
| Triggers | Native `AFTER INSERT/UPDATE` triggers |
| Stored procedures | `plpgsql` functions with `SECURITY DEFINER` |
| Scheduling | `pg_cron` (if available) or Go-cron fallback |
| LISTEN/NOTIFY | Real-time event streaming |
| Vector search | `pgvector` extension |
| Task claiming | `SELECT ... FOR UPDATE SKIP LOCKED` |

**Supabase Cloud specifics:**
- Use connection string from Project Settings → Database → Connection string
- Supavisor transaction mode (port 6543) is safe for `SET LOCAL`
- Supabase Vault can be used for secret storage (optional — the binary also supports env vars and config file)
- RLS, pg_cron, pg_net all available as hosted features

**Self-hosted Supabase specifics:**
- Identical feature set to Cloud
- The Go binary connects to the Postgres instance like any other client
- No Edge Functions needed — the binary IS the compute layer

**Any Postgres (Neon, Railway, RDS, Crunchy, etc.):**
- The binary only needs a connection string
- Requires: `pgvector` extension for embedding search
- Optional: `pg_cron` for in-database scheduling (the binary has its own Go-cron fallback)

### SQLite (embedded)

SQLite runs inside the binary process. No external database to install or manage.

| Feature | Mechanism |
|---|---|
| Row-Level Security | Go-layer enforcement (API Rules + WHERE injection) |
| Session isolation | Go context passing + WHERE clause injection |
| Triggers | Go hooks (`OnRecordAfterCreate`, etc.) |
| Stored procedures | Go-implemented handler functions |
| Scheduling | Go-cron (built into binary) |
| Real-time events | Server-Sent Events (Go goroutines) |
| Vector search | `sqlite-vec` extension |
| Task claiming | Single-writer serialization (no concurrent contention) |

### Parity Matrix

| Postgres Feature | SQLite Equivalent | Implementation Gap |
|---|---|---|
| RLS policies | Go-layer API Rules + WHERE injection | Medium — Go shim enforces isolation |
| `SET LOCAL` / `current_setting()` | Go context passing | Medium — application-layer equivalent |
| `plpgsql` stored procedures | Go handler functions | Low — same logic, different language |
| `pgvector` | `sqlite-vec` | Low — both support KNN search |
| `pg_cron` | Go-cron | None — Go-cron built into binary |
| `pg_net` | Go `net/http` | None — native HTTP in Go |
| `FOR UPDATE SKIP LOCKED` | Single-writer serialization | None — SQLite has no concurrent writers |
| `SECURITY DEFINER` | Go hooks with elevated access | None — Go layer already has full DB access |
| LISTEN/NOTIFY | SSE via Go goroutines | Low — different mechanism, same UX |

---

## Deployment Topologies

### 1. Local Development (simplest)

```bash
./conscience serve --db sqlite://dev.db
# Everything in one process. Zero dependencies.
```

### 2. Local Postgres

```bash
# Start Postgres (Docker, Homebrew, etc.)
docker run -d -p 5432:5432 -e POSTGRES_DB=conscience postgres:16

# Run binary
./conscience serve --db postgres://localhost:5432/conscience
```

### 3. Supabase Cloud

```bash
# Just point at the hosted Postgres
./conscience serve --db postgres://postgres:pass@db.xxx.supabase.co:5432/postgres
# Binary runs on your machine, a VM, fly.io, Railway, anywhere.
```

### 4. Self-Hosted Supabase

```bash
# Supabase Docker stack running locally
docker compose up -d  # starts Postgres, Auth, Realtime, etc.

# Point binary at the Postgres instance
./conscience serve --db postgres://supabase_admin:pass@localhost:5432/postgres
# Optionally use Supabase Auth, Realtime, Vault alongside the binary
```

### 5. Production (any Postgres + any VM)

```bash
# Deploy binary to fly.io
fly deploy --app conscience-prod

# Or Railway
railway up

# Or bare metal / Kubernetes
kubectl apply -f conscience-deployment.yaml

# Database can be anywhere the binary can reach via network
```

### 6. Horizontal Scaling

Multiple binary instances can share one Postgres backend:

```
┌──────────┐  ┌──────────┐  ┌──────────┐
│ conscience│  │ conscience│  │ conscience│
│ worker 1  │  │ worker 2  │  │ worker 3  │
└─────┬─────┘  └─────┬─────┘  └─────┬─────┘
      │              │              │
      └──────────────┴──────────────┘
                     │
              ┌──────┴──────┐
              │  Postgres   │
              │  (shared)   │
              └─────────────┘
```

`FOR UPDATE SKIP LOCKED` ensures each task is claimed by exactly one worker. No coordination needed between binary instances. This works with any Postgres provider — Supabase Cloud, Neon, RDS, etc.

SQLite mode does not support horizontal scaling (single-writer). For single-machine workloads, SQLite is sufficient.

---

## Schema Migrations

### Strategy

Migration SQL files are embedded in the binary and auto-applied on startup:

```go
//go:embed migrations/*.sql
var migrationFS embed.FS

func AutoMigrate(db *sql.DB) error {
    // Read current version from schema_versions table
    // Apply pending migrations in order
    // Record each migration in schema_versions
}
```

### Migration Files

```
migrations/
  001_initial_schema.sql
  002_session_indexes.sql
  003_staging_buffer.sql
  ...
```

Each migration is idempotent where possible (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`).

### Manual Migration

```bash
# Check current schema version
./conscience migrate status

# Apply pending migrations
./conscience migrate up

# Rollback last migration
./conscience migrate down
```

### Drift Handling

If schema version drift is detected:

1. All agents pause (sessions set to `paused`)
2. Migration script runs
3. All agents resume

No agent runs against an incompatible schema. Ever.

---

## Install Experience Summary

| Step | With SQLite | With Any Postgres |
|---|---|---|
| 1 | Download binary | Download binary |
| 2 | `./conscience serve` | `./conscience serve --db postgres://...` |
| 3 | Enter API key | Enter API key |
| 4 | System runs | System runs |

No Docker required. No Node.js. No Python. No runtime dependencies. A single statically-linked Go binary.

---

## Why Not Deno Edge Functions

Previous spec versions described the Supabase deployment path using Deno Edge Functions. This has been replaced with the Go binary model for three reasons:

1. **The harness is a long-running loop**, not a request-response handler. Edge Functions have 25-150 second execution limits. SPEC-020 interactive transactions need 60+ seconds of open transaction time.

2. **One language, one binary.** Go compiles to a single static binary with no runtime dependencies. Deno requires a runtime, a deploy step, and a separate codebase from the PocketBase Go shim.

3. **Same code, all backends.** The Go binary talks to Postgres via `pgx` and SQLite via `modernc.org/sqlite` behind a shared interface. No Kysely/Drizzle ORM needed — direct SQL with Go's `database/sql` is simpler and more portable.

Edge Functions may still have a role for future webhook handlers or lightweight API proxies, but the core runtime is the Go binary.

---

*SPEC-009 — Deployment — Conscience Framework*
