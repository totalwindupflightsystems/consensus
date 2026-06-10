# Conscience Deployment

Deploy the Conscience agent runtime on any database backend, from local development to production scaling.

axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-2/step-1-2-1 doc=deploy/README.md

## Quick Start

```bash
# Build the binary
go build -o conscience ./cmd/conscience

# Run with SQLite (zero dependencies)
./conscience serve --db sqlite://conscience.db
```

## Topology Overview

| # | Topology | Database | When to use |
|---|---|---|---|
| 1 | [Local SQLite](#1-local-sqlite) | SQLite (embedded) | Single developer, air-gapped, quick demo |
| 2 | [Local Postgres](#2-local-postgres) | Postgres (Docker/Homebrew) | Full-featured local dev |
| 3 | [Supabase Cloud](#3-supabase-cloud) | Postgres (hosted) | Team deployments, managed infrastructure |
| 4 | [Self-hosted Supabase](#4-self-hosted-supabase) | Postgres (local Docker) | Full Supabase stack on-prem |
| 5 | [Any Postgres + VM](#5-any-postgres--vm) | Any Postgres provider | Production, multi-cloud |
| 6 | [Horizontal Scaling](#6-horizontal-scaling) | Postgres (shared) | High throughput, multiple workers |

---

## 1. Local SQLite

Simplest deployment. Everything runs in one process.

```bash
# Download or build the binary
go build -o conscience ./cmd/conscience

# Initialize (creates schema, admin key, config)
./conscience init --db sqlite://conscience.db

# Start the server
./conscience serve --db sqlite://conscience.db
```

The database file (`conscience.db`) is created if it doesn't exist. No external dependencies. WAL mode is enabled automatically for concurrent reads.

**Limitations:**
- No horizontal scaling (single-writer SQLite)
- No built-in vector search (optional — can be added with sqlite-vec extension)
- Access Rules use Go-layer enforcement, not database RLS

---

## 2. Local Postgres

Full feature set with local Postgres.

```bash
# Start Postgres (any method)
docker run -d --name postgres-conscience \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=conscience \
  -p 5432:5432 postgres:16

# Initialize schema
./conscience init --db postgres://postgres:password@localhost:5432/conscience

# Start
./conscience serve --db postgres://postgres:password@localhost:5432/conscience
```

**Features available:**
- Full RLS for session isolation
- pgvector for semantic search
- FOR UPDATE SKIP LOCKED for task claiming
- Postgres triggers for event routing (optional pg_cron for scheduling)

---

## 3. Supabase Cloud

Point the binary at a hosted Supabase Postgres instance.

```bash
# Get connection string from Supabase Dashboard → Project Settings → Database
# Use port 6543 (Supavisor transaction mode) or 5432 (direct)

./conscience serve \
  --db "postgres://postgres:[PASSWORD]@db.[PROJECT].supabase.co:6543/postgres"
```

**What Supabase provides:**
- Hosted Postgres with pgvector
- RLS policies (managed via migration files)
- Optional: Supabase Auth for user management
- Optional: Supabase Vault for secret storage
- Optional: Realtime for WebSocket subscriptions

**What the binary handles:**
- Harness loop, heartbeat, task claiming
- REST API, MCP server, protocol shims
- Migrations (auto-applied on startup)

---

## 4. Self-hosted Supabase

Run the full Supabase Docker stack alongside the binary.

```bash
# Clone and start Supabase
git clone https://github.com/supabase/supabase
cd supabase/docker
docker compose up -d

# Point binary at the local Postgres
./conscience serve --db postgres://supabase_admin:your-password@localhost:5432/postgres
```

This gives you the full Supabase features (Auth, Storage, Realtime, Edge Functions) locally, with Conscience running as a Go binary beside them.

---

## 5. Any Postgres + VM

Deploy the binary to any VM that can reach your Postgres instance.

### fly.io

```bash
# Create fly app
fly launch --name conscience

# Set database URL as secret
fly secrets set CONSCIENCE_DB_URL="postgres://..."

# Deploy
fly deploy
```

### Railway

```bash
railway up
# Set CONSCIENCE_DB_URL environment variable in Railway dashboard
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: conscience
spec:
  replicas: 3
  selector:
    matchLabels:
      app: conscience
  template:
    metadata:
      labels:
        app: conscience
    spec:
      containers:
      - name: conscience
        image: conscience:latest
        env:
        - name: CONSCIENCE_DB_URL
          valueFrom:
            secretKeyRef:
              name: conscience-secrets
              key: db-url
        - name: CONSCIENCE_LISTEN
          value: ":8090"
```

---

## 6. Horizontal Scaling

Run multiple binary instances sharing a single Postgres database. No coordination needed between instances — `FOR UPDATE SKIP LOCKED` ensures exclusive task claiming.

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

```bash
# Start three workers
./conscience serve --db postgres://... --listen :8090 &
./conscience serve --db postgres://... --listen :8091 &
./conscience serve --db postgres://... --listen :8092 &
```

All workers pull from the same task queue. No leader election, no coordination protocol. The database handles mutual exclusion.

**Requirements:**
- Postgres backend (SQLite can't do this)
- pgvector extension for semantic search
- Optional: pg_cron for maintenance jobs (if not, Go-cron handles it)

---

## Configuration

All deployment options use the same `conscience.yaml` config file or environment variables. See `internal/config/config.go` for the full schema.

### Environment Variables

| Variable | Config Key | Default |
|---|---|---|
| `CONSCIENCE_DB_URL` | `database.url` | — |
| `CONSCIENCE_LISTEN` | `server.port` | `8090` |
| `CONSCIENCE_HOSTNAME` | `server.hostname` | `127.0.0.1` |
| `CONSCIENCE_LLM_API_KEY` | `llm.api_key` | — |
| `CONSCIENCE_LLM_PROVIDER` | `llm.provider` | `openai` |
| `CONSCIENCE_LOG_LEVEL` | `logging.level` | `info` |

---

## Schema Migrations

Migrations are embedded in the binary and auto-applied on startup. The `schema_versions` table tracks which migrations have been applied.

```bash
# Check current version
./conscience migrate version

# Apply pending
./conscience migrate up

# Rollback last
./conscience migrate down

# Create a new migration stub
./conscience migrate create "add_new_table"
```

**Drift protection:** If the database has migrations applied that don't match embedded files, all agents pause (session status → `paused`) until the drift is resolved. This ensures no agent runs against an incompatible schema.

---

## Scripts

| Script | Purpose |
|---|---|
| `deploy/local-sqlite.sh` | Start with SQLite backend |
| `deploy/local-postgres.sh` | Start with local Postgres |

See also: `Makefile` for build targets.
