# Dogfood Log

| Date | Verdict | Promise | Top findings | Time-to-first-success |
|---|---|---|---|---|
| 2026-08-04 | 🟡 PROMISING-BUT-ROUGH | Database-native agent runtime: append-only memory, crash recovery, semantic retrieval, circuit breakers, REST+CLI+MCP+Go client. | 1. P0 Append-only triggers silently missing on fresh installs (migration 017 stripped by filterForSQLite) — UPDATE memory_events succeeded. 2. P1 `session pause`/`resume` CLI broken (status vs action verb mismatch). 3. P1 Circuit breaker never trips — sessions fail instead of pausing, agent_circuit_breakers empty. | ~2 min (init → serve → create session) |

Run details (2026-08-04):
- Real use: scratch instance (SQLite, port 18123, no LLM key); REST workflow
  (sessions/memory/context/tasks/billing/metrics/config), kill -9 crash
  recovery verified, external Go consumer via pkg/client (16/16 calls OK),
  CLI sweep, MCP stdio, OpenAPI + chronicle.
- Tasks written: DOGFOOD-001 (P0), DOGFOOD-002/003 (P1), DOGFOOD-004..007
  (P2) — pending on the board.
- Artifacts: docs/dogfood/2026-08-04-integration.md,
  docs/dogfood/diagnostics.md, skills/consensus-usage/SKILL.md.
- Foreman: not woken (CooldownS=7200 < 14400; scheduler registration healthy,
  NamespaceID=coding-hermes, DecayRate=1, Enabled=true).
