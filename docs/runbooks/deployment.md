# Deployment Runbook

**Purpose**: Deploy the Consensus binary and apply database migrations.
**Severity**: Normal (planned maintenance)
**Estimated Time**: 10-15 minutes

---

## Prerequisites

- Go 1.25+ toolchain (or pre-built binary)
- Database connection (PostgreSQL 16+ or SQLite 3.x)
- Network access to the database endpoint
- `CONSENSUS_API_KEY` (admin scope) for health checks

---

## Step 1: Build the Binary

```bash
# Build with embedded migrations
make build

# Verify binary
./bin/consensus version
# Expected output: Consensus v0.1.0 (or current version)
```

### Build Options

```bash
# Cross-compile for different platform
GOOS=linux GOARCH=amd64 make build

# Build without CGO (default for production Docker images)
CGO_ENABLED=0 go build -o bin/consensus ./cmd/consensus

# Build with specific Go version
go version  # must be 1.25+
```

---

## Step 2: Database Migration

Consensus automatically applies schema migrations on startup (auto-migrate mode). You can also run migrations manually:

### Option A: Auto-Migrate (Recommended)

Start the server — migrations run automatically:

```bash
./bin/consensus serve --db-url postgres://user:pass@host:5432/consensus
```

The server logs confirm:
```
consensus: schema migrations applied
consensus: starting  addr=0.0.0.0:8090
```

### Option B: Manual Migration

Run migrations independently before starting the server:

```bash
# Check current migration status
./bin/consensus migrate status --db-url postgres://user:pass@host:5432/consensus

# Apply all pending migrations
./bin/consensus migrate up --db-url postgres://user:pass@host:5432/consensus

# Rollback last migration (emergency only)
./bin/consensus migrate down --db-url postgres://user:pass@host:5432/consensus
```

### Migration Safety

- All migrations are wrapped in transactions
- Idempotent — safe to re-run
- Version-tracked in `schema_migrations` table
- Down migrations are destructive — test in staging first

---

## Step 3: Health Check

After startup, verify the server is healthy:

```bash
# Health endpoint (no auth required)
curl http://localhost:8090/api/v1/health

# Expected response: {"status":"ok","version":"0.1.0","uptime_seconds":...}

# Check with admin key
curl -H "Authorization: Bearer $CONSENSUS_API_KEY" http://localhost:8090/api/v1/sessions

# Expected response: {"sessions":[],"total":0}  (or session list)
```

### Health Check Indicators

| Check | Command | Expected |
|-------|---------|----------|
| Server running | `curl localhost:8090/api/v1/health` | HTTP 200, `status: ok` |
| DB connected | Health check includes DB | `database: connected` |
| Schema current | `./bin/consensus migrate status` | `Schema version: N` (latest) |
| Bootstrap key | Server stdout at startup | `Admin key: cs_ak_...` |
| LLM configured | Server logs | `consensus: starting` |

---

## Step 4: Verify Core Functionality

```bash
# 1. Create a test session
SESSION=$(curl -s -X POST http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"agent_name":"test-verify","goal":"smoke test"}')
echo "$SESSION" | head -c 200

# 2. List sessions
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY"

# 3. Check metrics
curl -s http://localhost:8090/api/v1/metrics \
  -H "Authorization: Bearer $CONSENSUS_API_KEY"
```

---

## Rollback

```bash
# 1. Stop the server
kill <PID>  # or systemctl stop consensus

# 2. Rollback the binary
cp bin/consensus.prev bin/consensus  # if you kept a backup

# 3. Rollback database (if schema changed)
./bin/consensus migrate down --db-url postgres://user:pass@host:5432/consensus

# 4. Restart
./bin/consensus serve --db-url postgres://user:pass@host:5432/consensus
```

---

## Verification

- [ ] Binary compiles: `make build`
- [ ] Migrations apply: `./bin/consensus migrate status`
- [ ] Server starts: `curl localhost:8090/api/v1/health` → 200
- [ ] Auth works: `curl` with API key → sessions list
- [ ] Bootstrap key printed on first startup

> **Trace**: `axiom:trace work_item=WI-019 spec=specs/009-deployment.md,specs/016-cli-interface.md doc=docs/runbooks/deployment.md`
