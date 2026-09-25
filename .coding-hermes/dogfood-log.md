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
2026-09-01 | PROMISING-BUT-ROUGH | 45s t2fs | friction 9 | 5 findings
2026-09-03 | PROMISING-BUT-ROUGH | goal-exec works, chat path dead | friction 6 | 4 findings | t2fs ~120s | install_seconds=61 (tar+Go bootstrap fallback; documented paths FAIL: ghcr denied + repo private) | bunker=las-01 agent=af56fcf2 (las-03 pool exhausted) | smoke=ok

Run details (2026-09-03):
- Real use: scratch sqlite :18201 + real DeepSeek key. Fresh init flow exact
  per docs; REST sweep (sessions/memory/context/iterations/health/openapi);
  A/B isolated the message-path defect; goal-driven execution verified with
  real staged SQL → memory event; kill -9 crash recovery re-verified live
  (heartbeat auto-resume); MCP/SSE/Chronicle/doc-api probes.
- Headline finding DF-CONSENSUS-6 (P0): user message never reaches the LLM
  (messages=2 every turn, ~30 calls), no reply ever produced/observable,
  tokens/billing/audit/commits all 0 after 4 runs.
- DF-CONSENSUS-7 (P0): README docker quickstart image not anonymously
  pullable (ghcr denied) + repo not anonymously cloneable; verified fallback
  install 61s build + smoke PASS on ephemeral bunker agent (destroyed OK).
- Tasks written: DF-CONSENSUS-6 (P0), DF-CONSENSUS-7 (P0), DF-CONSENSUS-8
  (P1), DF-CONSENSUS-9 (P2) — pending on the board (JSONL).
- Artifacts: docs/dogfood/2026-09-03-integration.md,
  docs/dogfood/diagnostics-2026-09-03.md, skills/consensus-usage/SKILL.md
  v2.4.0 (landmines 0-7, goal-driven pattern marked verified).
- Foreman: woken (CooldownS 43200 → 900) — 4 new board tasks incl. 2×P0.


2026-09-04 | PROMISING-BUT-ROUGH | 58s t2fs | friction 7 | 5 findings

| 2026-09-09 | 🔴 DOES-NOT-DELIVER (documented path) / 🟡 PROMISING-BUT-ROUGH (config-file path) | Regression re-test at 8e3e7e6: all 9 prior DF tasks still pending. NEW P0 DF-CONSENSUS-10: README env-var invocation wedges the whole server (begin tx deadline → 401-to-valid-key → health freeze; survives restart via heartbeat poison-pill); A/B pins default SQLite pool (no max_open_conns) as root cause. DF-CONSENSUS-11: conversational path still deaf to user turns even with pool pinned (messages=2; model says "no user request"); tokens_used stay 0 during real calls (breaker/budget dead). DF-CONSENSUS-12: ghcr still DENIED, but anonymous git clone now WORKS (half of DF-CONSENSUS-7 fixed). | see board | t2fs: never on documented path; ~4 min on config-file path |

Run details (2026-09-09):
- Real use: fresh build, 4 isolated legs (env-var ×2, config-file ×2),
  real DEEPSEEK_API_KEY, kill -9 + restart crash-recovery A/B, SSE watch,
  DB-level durable-state forensics. All instances destroyed after use.
- Bunker install leg: SKIPPED-install-bunker — ssh connect timeout to
  bunker-las-03 (100.69.3.13). Installability evidence from this run:
  anonymous git clone verified OK from the control host with credential
  helpers stripped; ghcr pull verified DENIED; source build 2s on Go 1.26.5.
- Tasks written: DF-CONSENSUS-10 (P0), DF-CONSENSUS-11 (P0),
  DF-CONSENSUS-12 (P1) — pending on the board (JSONL).
- Artifacts: docs/dogfood/2026-09-09-integration.md,
  docs/dogfood/diagnostics-2026-09-09.md, skills/consensus-usage/SKILL.md
  v2.5.0 (landmines 5b/5c/5d, crash-recovery poison-pill caveat).
- Verdict note: the split verdict is deliberate. The path the README
  documents cannot deliver (bricks the server); the undocumentable
  config-file path delivers the goal-driven pattern with real LLM calls.
  Third consecutive run finding the conversational contract broken — the
  project's headline promise ("send a message, get the response") remains
  undeliverable as of 2026-09-09.
- Foreman: woken (CooldownS 21600 → 900) — 3 new board rows incl. 2×P0.

2026-09-24 | 🟡 PROMISING-BUT-ROUGH | t2fs: turn-1 ~15s; turn-2+ never | friction 4 | 2 new findings (P0+P1)

Run details (2026-09-24, regression re-test @ 576dc07):
- Angle: re-test the three foreman fixes from the 09-09 verdict (pool wedge,
  conversational deafness, docs drift) + run the bunker install leg that was
  SKIPPED on 09-09. Prior runs swept the same CLI/env/config surfaces, so the
  angle was the fixes themselves, not a new surface.
- Verified fixed: documented env-var install no longer wedges (health 200
  <5s); turn-1 round trip real (1695in/139out tokens, reply observable in
  /context and /memory); README quickstart matches reality.
- Bunker install leg PASSED (first time): agent 89e7da8c on bunker-las-03,
  anonymous clone 8.1s, cold build 62s, init→serve→health 200, agent destroyed.
- DF-CONSENSUS-20 (P1): user message content never enters the LLM prompt on
  turns 2+ — prompt_tokens byte-identical across turns (1668/1668, 1695/1695)
  while memory_events stores the user_message rows; model answers "no question
  yet". Goal text is the only input that always reaches the model.
- DF-CONSENSUS-21 (P0): "consume user messages: sqlite tx: exec: database is
  locked (517)" during planning permanently bricks sessions (status=failed,
  last_error=null, retry dies the same way); billed LLM calls wasted; hit 4/4
  turns under a 0.5s-polling client late in the run, incl. a fresh session's
  first turn.
- Perf: headline round trip unmeasurable this run — the 517 race killed every
  timed turn; no PERF row filed (can't benchmark a failing path; gated by
  DF-CONSENSUS-21). Reference: LLM call 1.3–3.3s, local API <50ms, build
  9.5s/62s, clone 2.4s/8.1s.
- Tasks written: DF-CONSENSUS-20 (P1), DF-CONSENSUS-21 (P0) — board JSONL,
  commit 3f653b3 (surgical, 2 rows, numstat-verified).
- Artifacts: docs/dogfood/2026-09-24-integration.md (this run), prior
  diagnostics unchanged.
- Foreman: NOT woken — scheduler API :9090 timed out at report time
  (pre-existing scheduler slowness). Foreman is active regardless (ticks and
  merges landed 2026-09-24 02:17–04:29); DF-CONSENSUS-20/21 are on the board.

2026-09-25 | SHIPPABLE (conversational promise) | Multi-turn messaging: send message → agent response, every turn, real LLM | 1. PERF-CONSENSUS-11 (P1 heartbeat dispatch stall: 2 turns 94.7s/108.2s vs 1.6s LLM; executor.go:607) 2. DF-CONSENSUS-22 (P2 shipped config boots with compression WARN) 3. both 09-24 defects live-verified FIXED (20-send burst: 0 failed sessions, 5/5 busy-race retries; turn-2 content reaches LLM) | t2fs ~4s (init+serve+health); turn-1 reply ~6s | install_seconds=58 | bunker=las-03 agent=fa023371 | smoke=ok

Run details (2026-09-25, angle = live P0 verification + multi-turn, @ 235efdb):
- Fresh scratch :8126 (env-var path + consensus.yaml, real DeepSeek key).
- DF-CONSENSUS-21 pass criterion executed literally: 20 rapid sends across 4
  sessions (5 parallel each) → 20/20 HTTP 200 (0.03-0.44s), 4/4 idle,
  last_error=null, 5/5 busy-race retries in log, 0 failed sessions.
- DF-CONSENSUS-20 retest: turn-2 prompt_tokens 1683 ≠ turn-1 1679; model
  quoted turn-2-only tokens; replies exact on every probe.
- PERF: healthy turn 2-6s; anomalous 94.7s + 108.2s on 2 turns (heartbeat
  pickup stall, log-proven gaps 6m25s/4m, fresh session 6.3s in same window);
  no profile (stall cleared, pprof not wired). Reference: LLM 1.4-2.9s,
  POST accept 0.03s, build 15.1s warm / 58s cold, clone 4.8s.
- Artifacts: docs/dogfood/2026-09-25-integration.md, diagnostics.md addendum,
  skills/consensus-usage/SKILL.md v2.6.0, board rows PERF-CONSENSUS-11 +
  DF-CONSENSUS-22 (commit 1f99d93, numstat 2 rows verified).
- Foreman: see tick report.

2026-09-25 (afternoon, MCP-surface angle) | 🟡 PROMISING-BUT-ROUGH | MCP attach + full tool workflow over MCP | 1. DF-CONSENSUS-23 (P0 bare /mcp 404s in production; spec documents it; unit test mounts a shape prod doesn't) 2. DF-CONSENSUS-24 (P0 MCP send_message dead-letters on booting sessions — tools.go:287 vs service.go:433 wake drift) 3. DF-CONSENSUS-25 (P1 stale legacy session → plain-text 404, not JSON-RPC) | t2fs ~4s boot; MCP handshake + tools/list ~2s; but MCP-only first turn never completes (DF-24) | friction 3 | install_seconds=50 (clone 5s + go extract 4s + build 41s; agent ca6b95c0, bunker-las-03, destroyed) | smoke=ok (health 200; bare /mcp 404 reproduced on the fresh clone too)

Run details (2026-09-25 PM, angle = MCP surface, first run since MCP-DIRECT-001 @ df33c39):
- Control-host scratch :8127 (config-file path, deepseek-chat, key probed first:
  1 of 11 candidates live per ENV-CONSENSUS-1 procedure).
- Real MCP client workflow: SSE handshake → initialize → tools/list (8 tools incl.
  new list_tasks/claim_task) → create_session → send_message → status → list_memory.
- A/B cross-path: MCP-created session stuck booting 5+ min after MCP send; woke
  instantly on one REST message; MCP send on idle sessions works (turn-2 answered,
  TURN2 in ledger). Root cause pinned tools.go:287 vs service.go:433.
- Bunker install leg PASSED: anonymous clone of totalwindupflightsystems/consensus
  (wojons/consensus is 404 anon — stale URL in some docs), clone 5s, Go 1.26.6
  provisioned, build 41s, init→serve→health 200, bare /mcp 404 reproduced on the
  fresh clone before destroy.
- Perf (PERF-CONSENSUS-12): MCP warm round trip 1058ms (REST control 1075ms);
  cold 41.9s = DF-24 dead-letter wait, not compute. Nothing slow enough to profile.
- Tasks written: DF-CONSENSUS-23 (P0), -24 (P0), -25 (P1), -26 (P2 docs),
  PERF-CONSENSUS-12 — commit f596c04 (5 rows, numstat-verified).
- Artifacts: docs/dogfood/2026-09-25-mcp-surface.md, skills/consensus-usage/SKILL.md
  v2.7.0 (MCP-surface section).
- Foreman: see tick report.
| 2026-09-25b | 🟡 PROMISING-BUT-ROUGH | (2nd run today; ANGLE = REST API + crash-recovery + isolation claims; morning run did MCP) DB-native runtime: async REST turns, billing, key scoping, session lifecycle, SIGKILL recovery. | 1. P1 agent_billing never records + tokens stay 0 through real LLM turns (log shows 1638/158). 2. P1 DELETE /sessions/{id} returns 200 but row survives, stays listed, still takes messages. 3. P1 key-mint response api_key = "cs_sk_...NNN" placeholder — scoped keys dead on arrival. | ~4 min |
