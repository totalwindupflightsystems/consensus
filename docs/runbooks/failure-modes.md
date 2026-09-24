# Failure Modes Runbook

**Purpose**: Diagnose and recover from common Consensus failure modes.
**Severity**: Varies (P1-P3 depending on impact)

---

## Failure Mode 1: LLM API Errors

### Symptom

```
Harness log: ERROR llm call failed: LLM API error: 429 Too Many Requests
Agent session status: stuck in 'thinking' or 'boot' indefinitely
Agent response: "error: LLM_API_ERROR"
```

Server logs show:
```
consensus: llm client init failed: ...
consensus: agent iteration 3/50 failed: LLM API error: ...
```

### Causes

| Cause | Indicator | Recovery |
|-------|-----------|----------|
| Rate limited | HTTP 429 | Wait 60s, retry. Reduce agent count. |
| API key expired | HTTP 401 | Rotate API key in config. |
| Model unavailable | HTTP 404 | Check model ID in config. Fall back to gpt-4o. |
| Network timeout | `context deadline exceeded` | Check network. Increase timeout. |
| Provider outage | HTTP 5xx | Check provider status page. Fail over. |

### Recovery Steps

```bash
# 1. Check which sessions are stuck
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" | \
  jq '.sessions[] | select(.status == "thinking" or .status == "booting") | {id, status, iteration}'

# 2. Cancel stuck sessions
SESSION_ID="<stuck-session-id>"
curl -X POST "http://localhost:8090/api/v1/sessions/$SESSION_ID/message" \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"content":"Cancel this session and report current state","type":"user_instruction"}'

# 3. Verify LLM provider health
curl -s http://localhost:8090/api/v1/health | jq '.llm'

# 4. If provider is down, switch to alternate provider in config.yaml
# consensus.yaml:
# llm:
#   provider: openai     # or anthropic
#   base_url: https://api.openai.com/v1
#   model: gpt-4o-mini  # fallback model
```

### Prevention

- Configure budget limits per session to prevent runaway costs
- Set `max_iterations` per session (default: 50)
- Use the circuit breaker (`max_consecutive_errors`) to auto-pause failing sessions

---

## Failure Mode 2: Database Connection Loss

### Symptom

```
Server log: ERROR db: connection refused
Server log: ERROR db: no such host
Server log: ERROR db: SSL connection failed
Agent iteration fails: "query: connection pool exhausted"
```

### Causes

| Cause | Indicator | Recovery |
|-------|-----------|----------|
| DB server down | `connection refused` | Restart DB, verify connectivity |
| Network partition | `i/o timeout` | Check network, DNS, firewall |
| Connection pool exhausted | `pool exhausted` | Increase pool size, reduce agent count |
| SSL/TLS mismatch | `SSL error` | Update SSL certs, check pg_hba.conf |
| Credential rotated | `password authentication failed` | Update `CONSENSUS_DB_URL` |

### Recovery Steps

```bash
# 1. Check database connectivity from the server host
psql "$CONSENSUS_DB_URL" -c "SELECT 1;"

# 2. Check connection pool (if Postgres)
psql "$CONSENSUS_DB_URL" -c "SELECT count(*) FROM pg_stat_activity WHERE datname = 'consensus';"

# 3. Restart the Consensus server (attempts to reconnect)
docker restart consensus
# or: systemctl restart consensus

# 4. If pool exhaustion: reduce concurrent agents
# Set CONSENSUS_MAX_WORKERS=2 in environment before restart

# 5. Verify reconnection
curl -s http://localhost:8090/api/v1/health | jq '.database'
# Expected: {"status": "connected", ...}
```

### Prevention

- Use connection pooling (PgBouncer for Postgres)
- Set `max_connections` in postgresql.conf
- Monitor `pg_stat_activity` for runaway agent connections
- SQLite users: avoid concurrent writes from multiple processes

---

## Failure Mode 3: Disk Full

### Symptom

```
Server log: ERROR sqlite: database or disk is full
Server log: ERROR pq: could not extend file: No space left on device
Agent write operations fail persistently
OS metrics show disk usage at 100%
```

### Causes

| Cause | Indicator | Recovery |
|-------|-----------|----------|
| Uncompressed memory events | Large `memory_events` table | Run compression, archive old sessions |
| Audit log growth | Large `audit_logs` table | Set retention policy, truncate old logs |
| Session data | Many active sessions | Complete or cancel stale sessions |
| DB log file growth (Postgres WAL) | WAL directory filling disk | Configure WAL archiving, increase wal_keep_size |

### Recovery Steps

```bash
# 1. Check disk usage
df -h /var/lib/postgresql/
du -sh /var/lib/postgresql/16/main/

# 2. Identify largest tables (Postgres)
psql "$CONSENSUS_DB_URL" -c "
SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) AS size
FROM pg_catalog.pg_statio_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 10;
"

# 3. Archive and remove old audit logs
# Set retention to 90 days in config:
# retention:
#   audit_logs_days: 90

# 4. Complete stale sessions via API
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" | \
  jq '.sessions[] | select(.status == "idle" or .status == "booting") | .id' | \
  while read id; do
    id=$(echo $id | tr -d '"')
    curl -X POST "http://localhost:8090/api/v1/sessions/$id/message" \
      -H "Authorization: Bearer $CONSENSUS_API_KEY" \
      -H "Content-Type: application/json" \
      -d '{"content":"Session timed out due to resource constraints.","type":"system_override"}'
  done

# 5. VACUUM (Postgres) or PRAGMA optimize (SQLite)
psql "$CONSENSUS_DB_URL" -c "VACUUM;"
# SQLite: sqlite3 dev.db "PRAGMA optimize;"
```

### Prevention

- Monitor disk usage with alerts at 80%, 90%, 95%
- Configure `memory_compression` to auto-compress old events
- Set audit_log retention via system_settings table
- Use `max_iterations` to prevent runaway sessions
- Set budget limits per session

---

## Failure Mode 4: Server Crash / OOM Kill

### Symptom

```
Server process disappears from process list
System logs: "Out of memory: killed process consensus"
System logs: "consensus.service: Main process exited, code=killed, status=9/SIGKILL"
Monitoring dashboard shows memory spike then drop
```

### Causes

- Memory leak in Go runtime (long-running sessions accumulating context)
- Too many concurrent sessions exceeding available RAM
- LLM response processing allocating large memory blocks
- Insufficient resource limits (Docker memory limit too low)

### Recovery Steps

```bash
# 1. Restart the server
docker start consensus
# or: systemctl start consensus

# 2. Check for corrupted database (SQLite)
sqlite3 dev.db "PRAGMA integrity_check;"

# 3. Reduce concurrent agent count
export CONSENSUS_MAX_WORKERS=2
./bin/consensus serve

# 4. Add systemd resource limits
# /etc/systemd/system/consensus.service:
# [Service]
# MemoryMax=2G
# MemoryHigh=1.5G
```

### Prevention

- Set `MemoryMax` and `MemoryHigh` in systemd unit
- Configure Docker memory limits (`--memory=2g`)
- Monitor Go runtime metrics via `/api/v1/metrics`
- Add swap for protection (not a solution for production)

---

## Failure Mode 5: Stuck Agent Session

### Symptom

```
Session status stays in 'thinking' for >5 minutes
Agent iterations stop incrementing
Server logs show no activity for the session
```

### Recovery Steps

```bash
# 1. Identify stuck sessions
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" | \
  jq '.sessions[] | select(.status == "thinking") | {id, iteration, heartbeat_at}'

# 2. Force-complete the session
curl -X POST "http://localhost:8090/api/v1/sessions/$SESSION_ID/message" \
  -H "Authorization: Bearer $CONSENSUS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"content":"","type":"system_override"}'

# 3. If that fails, cancel via database (emergency)
# WARNING: Direct DB write — use only when API unavailable
psql "$CONSENSUS_DB_URL" -c "
UPDATE sessions SET status = 'failed', completed_at = NOW()
WHERE id = '$SESSION_ID' AND status = 'thinking';
"
```

---

## Failure Mode 6: MCP Client Connection Failure

### Symptom

```
MCP client (Claude Desktop) shows "Failed to connect to server"
No SSE events received on MCP session
```

### Recovery Steps

```bash
# 1. Verify MCP endpoint is running
curl -s http://localhost:8090/mcp/sse
# Expected: SSE stream with endpoint event

# 2. Check auth
# The API key is configured on the CLIENT side (--api-key flag, CONSENSUS_API_KEY
# env, or consensus.yaml server.api_key). The MCP client forwards it to the
# server in initialize._meta.authorization — you do NOT set _meta yourself.
# A 401 "Invalid API key" means the configured key is missing/expired.

# 3. Test with raw JSON-RPC
echo '{"jsonrpc":"2.0","id":1,"method":"ping"}' | \
  ./bin/consensus mcp-stdio --db-url sqlite://dev.db

# Expected stdout:
# {"jsonrpc":"2.0","id":1,"result":{}}
```

---

## Summary: Quick Reference

| Symptom | Most Likely Cause | First Action |
|---------|------------------|--------------|
| LLM calls failing 429 | Rate limited | Wait 60s, reduce agents |
| Session stuck in 'thinking' | LLM API error | Check LLM provider |
| Writes failing "disk full" | Disk space exhausted | Archive old sessions |
| Server OOM kill | Too many concurrent agents | Reduce workers, add memory |
| DB connection refused | DB server down | Restart DB, check network |
| Health check returns 5xx | Server startup incomplete | Check server logs |
| MCP client can't connect | Wrong transport or auth | Check client config |

---

> **Trace**: `axiom:trace work_item=WI-019 spec=specs/009-deployment.md,specs/008-harness.md doc=docs/runbooks/failure-modes.md`
