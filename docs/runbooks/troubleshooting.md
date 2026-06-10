# Troubleshooting Runbook

**Purpose**: General diagnostics for Conscience operational issues — log locations, health checks, startup problems, and common configuration errors.
**Severity**: Varies
**Estimated Time**: 5-30 minutes depending on symptom

---

## axiom:trace work_item=make-conscience-fully-operational-end-to spec=specs/009-deployment.md doc=docs/runbooks/troubleshooting.md

---

## Quick Diagnostic Flow

```
Is the server running?
├── No  → Check startup logs (see §1)
│         → Check binary exists and is executable
│         → Check database connectivity
├── Yes → Health check passes?
│         ├── No  → Run /api/v1/health (see §2)
│         │         → Check database connection
│         │         → Check migration state
│         ├── Yes → Agent loop running?
│         │         ├── No  → Check heartbeat logs
│         │         │         → Verify LLM config
│         │         ├── Yes → Session stuck?
│         │                   → Check agent iteration logs
│         │                   → Check LLM API errors
```

---

## §1. Log Locations

Conscience writes all operational logs to **stderr** (not stdout). The bootstrap admin key is the only output on stdout.

| Environment | Log Destination | How to View |
|---|---|---|
| Local dev | stderr (terminal) | Scroll back in terminal or redirect: `conscience serve 2> conscience.log` |
| Docker | Container stderr | `docker logs <container>` |
| systemd | Journal | `journalctl -u conscience -f` |
| Kubernetes | Container stderr | `kubectl logs <pod> -c conscience` |

### Log Levels

Set via `CONSCIENCE_LOG_LEVEL` env var or `--log-level` flag:

| Level | When to Use |
|---|---|
| `info` (default) | Normal operations |
| `debug` | Diagnosing agent iteration issues |
| `warn` | Expected but notable conditions |
| `error` | Recoverable failures |

```bash
# Enable debug logging
CONSCIENCE_LOG_LEVEL=debug ./bin/conscience serve --db sqlite://conscience.db
```

### Key Log Messages to Watch

| Message | Meaning | Action |
|---|---|---|
| `conscience: schema migrations applied` | Migrations ran on startup | Normal — first start or schema change |
| `conscience: starting` | Server is listening | Normal |
| `conscience: llm client init failed` | LLM provider config error | Check §3 |
| `conscience: agent iteration N/M failed` | Agent call failed | Check LLM or DB |
| `conscience: heartbeat claimed task` | Agent iteration started | Normal |
| `conscience: HITL manager started` | Approval system ready | Normal |

---

## §2. Health Check Endpoints

### Basic Health

```bash
# No auth required
curl http://localhost:8090/api/v1/health
```

**Expected response:**
```json
{
  "status": "ok",
  "version": "0.1.0",
  "database": "connected",
  "schema_version": 15,
  "uptime_seconds": 3600
}
```

**Troubleshooting health check failures:**

| Symptom | Likely Cause | Fix |
|---|---|---|
| HTTP 200 with `status: "degraded"` | DB slow or migration pending | Check DB load, run migrate status |
| HTTP 503 | DB not connected | Check DB URL, credentials |
| Connection refused | Server not running | Start the binary |
| Timeout | Firewall or wrong port | Check listen address in config |

### Auth Check

```bash
curl -H "Authorization: Bearer $CONSCIENCE_API_KEY" \
  http://localhost:8090/api/v1/sessions
```

| Response | Meaning |
|---|---|
| `{"sessions":[...]}` | Auth working, server operational |
| HTTP 401 | API key invalid or expired — run `conscience init` to bootstrap new key |
| HTTP 403 | API key lacks required scope |

---

## §3. Startup Failures

### Symptom: Binary exits immediately with error

```bash
./bin/conscience serve --db-url postgres://localhost:5432/conscience
# Output: config: ...
# or: db: ...
# or: migrate: ...
```

**Diagnostic steps:**

1. **Check config loading** — the first error is usually config-related:
   ```bash
   # Test config loading explicitly
   CONSCIENCE_LOG_LEVEL=debug ./bin/conscience config get all
   ```

2. **Check database connectivity** — the second error is usually DB:
   ```bash
   # Test DB connection independently
   psql "$DATABASE_URL" -c "SELECT 1"
   # or for SQLite:
   sqlite3 /path/to/conscience.db "SELECT 1;"
   ```

3. **Check migration state** — if DB connects but schema is wrong:
   ```bash
   ./bin/conscience migrate status --db-url "$DATABASE_URL"
   ```

### Symptom: LLM initialization fails

```
conscience: llm client init failed: llm: mock provider requires CONSCIENCE_MOCK_LLM=1 env var
```

**Cause:** Provider is set to `mock` without the safety env var.

**Fix:** Set `CONSCIENCE_LLM_PROVIDER=openai` (or anthropic) and provide the API key:

```bash
export CONSCIENCE_LLM_PROVIDER=openai
export OPENAI_API_KEY=sk-...
./bin/conscience serve --db-url postgres://localhost:5432/conscience
```

Or if you intentionally want mock mode:
```bash
export CONSCIENCE_MOCK_LLM=1
./bin/conscience serve
```

### Symptom: "no admin key in database"

This happens on first startup when no bootstrap key is found. Conscience auto-generates one:

```
Admin key: cs_ak_xxxxxxxxxxxxxxxx
```

**If you miss the startup output:**
```bash
# Generate a new bootstrap key
./bin/conscience init --db-url "$DATABASE_URL"
```

---

## §4. Runtime Diagnostics

### Slow Agent Iterations

```bash
# Check for long-running sessions
curl -H "Authorization: Bearer $CONSCIENCE_API_KEY" \
  http://localhost:8090/api/v1/sessions
# Look for sessions with status "thinking" or "boot"

# Check LLM latency from logs
grep "llm: response received" conscience.log | tail -5
# Look for elapsed_ms > 30000 (30s)
```

### Database Connection Issues

```bash
# Check active connections (Postgres)
SELECT * FROM pg_stat_activity WHERE application_name = 'conscience';

# Check SQLite busy state
# Look for "database is locked" errors in logs

# Connection pool exhaustion symptoms:
# - Health check returns degraded
# - Agent iterations fail with "timeout: acquiring connection"
# Fix: increase pool size or reduce concurrent agents
```

### Memory / Disk Pressure

```bash
# Check binary size
ls -lh bin/conscience

# Check SQLite database size
ls -lh conscience.db

# Check Postgres database size
SELECT pg_size_pretty(pg_database_size('conscience'));

# Monitor log growth
du -sh /var/log/conscience/
```

---

## §5. Common Configuration Errors

| Error | Cause | Fix |
|---|---|---|
| `config: unknown field "..."` | YAML typo in conscience.yaml | Check config file syntax |
| `db: unsupported driver` | DB URL scheme not recognized | Must start with `postgres://` or `sqlite://` |
| `migrate: schema version N > max available` | Binary older than DB schema | Update binary to match migration set |
| `migrate: migration N not found in binary` | DB schema newer than binary | Rollback DB or update binary |
| `llm: unknown provider` | CONSCIENCE_LLM_PROVIDER typo | Must be `openai`, `anthropic`, `openrouter`, or `mock` |
| `llm: api error: 401` | Invalid or missing API key | Check OPENAI_API_KEY or ANTHROPIC_API_KEY env var |
| `llm: api error: 429` | Rate limited | Wait and retry; reduce concurrent sessions |

---

## §6. Diagnostic Commands Quick Reference

```bash
# Health check
curl http://localhost:8090/api/v1/health

# Migration status
./bin/conscience migrate status --db-url "$DATABASE_URL"

# View active sessions
./bin/conscience session list

# Check server version
./bin/conscience version

# Configuration dump
./bin/conscience config get all

# Stress test connectivity
for i in 1 2 3 4 5; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8090/api/v1/health
done
```

---

## §7. Escalation Path

| Symptom | Escalate To | Contact |
|---|---|---|
| P1: Server down | On-call SRE | [team-channel] |
| P2: LLM errors persisting > 15min | LLM provider support | OpenAI/Anthropic dashboard |
| P3: Agent loop pauses | Developer on-duty | [team-channel] |
| P3: Slow performance | Performance team | File GitHub issue |

---

> **See also:**
> - [Deployment Runbook](deployment.md) — build, deploy, verify
> - [Failure Modes](failure-modes.md) — LLM errors, DB loss, disk full
> - [Backup & Restore](backup-restore.md) — data recovery procedures
> - [Admin Key Rotation](admin-key-rotation.md) — key lifecycle
