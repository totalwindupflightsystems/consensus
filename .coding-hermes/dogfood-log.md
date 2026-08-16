# Dogfood Log

| Date | Verdict | Promise | Top findings | Time-to-first-success |
|---|---|---|---|---|
| 2026-08-04 | 🟡 PROMISING-BUT-ROUGH | Database-native agent runtime: append-only memory, crash recovery, semantic retrieval, circuit breakers, REST+CLI+MCP+Go client. | 1. P0 Append-only triggers silently missing on fresh installs (migration 017 stripped by filterForSQLite) — UPDATE memory_events succeeded. 2. P1 `session pause`/`resume` CLI broken (status vs action verb mismatch). 3. P1 Circuit breaker never trips — sessions fail instead of pausing, agent_circuit_breakers empty. | ~2 min (init → serve → create session) |

| 2026-08-15 | 🟡 PROMISING-BUT-ROUGH (re-run; core upgraded) | Database-native agent runtime: append-only memory, crash recovery, semantic retrieval, circuit breakers, REST+CLI+MCP+Go client. | Aug-4 P0/P1s all FIXED and re-verified live; NEW: 1. P0 MCP surface unauthenticated (tools work with no key). 2. P0 MCP create_session deterministic IDs+shared session keys (works once, isolation broken). 3. P1 OpenAPI contract CWD-dependent (/openapi.json 404 in Docker; /doc = shim UI). | ~2 min (init → serve → create session) |

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

Run details (2026-08-15):
- Real use: scratch instance (SQLite, port 18123, no LLM key); REST workflow,
  live circuit-breaker trip, append-only trigger enforcement, CLI sweep
  (pause/resume fixed), crash recovery via kill -9, external Go consumer via
  pkg/client (5/5), MCP SSE + stdio (happy path + auth probing), H3 shim
  example verbatim, OpenAPI CWD dependence, keyless smoke + full -short suite
  (30/30, 142s), fresh-clone AGENTS.md paths.
- Tasks written: DOGFOOD-101 (P0), DOGFOOD-102 (P0), DOGFOOD-103/104 (P1),
  DOGFOOD-105/106 (P2), DOGFOOD-107 (P3) — pending on the board (JSONL).
- Artifacts: docs/dogfood/2026-08-15-integration.md, diagnostics.md
  addendum, skills/consensus-usage/SKILL.md v2.0 (stale landmines removed).
- Foreman: not woken (CooldownS=3600 < 14400; enabled, healthy). P0s will be
  picked up on the next tick.
