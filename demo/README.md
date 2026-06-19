# Conscience Demo

Run the live demo:

```bash
go test -v -run TestDemo_FullAgentHarness -timeout 300s ./demo/
```

Requires `DEEPSEEK_API_KEY` environment variable (or set in conscience.yaml).

## What It Demonstrates

1. **Append-Only Memory** — UPDATE/DELETE attempted on `memory_events` → rejected by DB trigger
2. **Semantic Retrieval** — 8 events across 3 topic clusters → security search returns security events first
3. **Crash Recovery** — Server kill -9 mid-session → database intact → restart resumes session
4. **Circuit Breaker** — Consecutive error threshold configured → breaker state visible in `agent_circuit_breakers`

## Expected Output

```
╔══════════════════════════════════════════════════════════════╗
║         CONSCIENCE — Database-as-Agent-Runtime Demo         ║
╚══════════════════════════════════════════════════════════════╝

✓ Database initialized
✓ Server started
✓ Server healthy

━━━ DEMO 1: Append-Only Memory Ledger ━━━
   UPDATE attempt → 500 append-only: UPDATE is not permitted
   DELETE attempt → 500 append-only: DELETE is not permitted
   INSERT allowed → 200
   ✓ Memory is append-only — UPDATE/DELETE blocked at DB level

━━━ DEMO 2: Semantic Retrieval ━━━
   Inserted 8 events across 3 topic clusters
   Security search results:
     → Security audit reveals XSS vulnerability in admin dashboard
     → API key rotation requires 90-day expiration
     → Row-level security prevents cross-session data leaks
   ✓ Semantic retrieval finds relevant events by topic

━━━ DEMO 3: Crash Recovery ━━━
   💥 Server killed (simulating crash)
   ✓ Database file intact after crash
   ✓ Server restarted
   ✓ Session survived crash — heartbeat resuming

━━━ DEMO 4: Circuit Breaker ━━━
   Circuit breaker state: {"rows":[{"breaker_type":"consecutive_errors",...}]}
   ✓ Circuit breaker tracking active

╔══════════════════════════════════════════════════════════════╗
║                      DEMO COMPLETE                          ║
╚══════════════════════════════════════════════════════════════╝
```
