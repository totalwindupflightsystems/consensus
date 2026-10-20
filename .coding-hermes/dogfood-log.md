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

2026-09-26 (3rd angle: fix-verification of DF-27/28/29 + CLI surface; runs 1-2 on 09-25 did REST and MCP) | ✅ SHIPPABLE (first time) | All three 09-25 P1 fixes verified FIXED in real use: key mint returns real cs_sk_ (scope field singular), agent_billing records per-iteration rows + session tokens 17.9k/3.2k, DELETE is a correct soft-delete tombstone (200/404/410, row survives per pinned spec). CLI works for inspection; 2 new P2s: DF-CONSENSUS-33 CONSENSUS_API_KEY env dead + auth exit 0, DF-CONSENSUS-34 no CLI message path (CLI-only session wedges in booting). | t2fs ~4 min (init→serve→first reply); friction 2 | cold turn 78.2s (create+2 iterations), warm 19.2s, LLM calls 1.2-3.9s; PERF-11 stalls did NOT reproduce; nothing worth a profile | install_seconds=91 (bunker-mvp substitution: las-03 down 3rd run, DF-CONSENSUS-32 still open; clone 4.3s @ 87340a0, go toolchain auto-fetch, health 200 in 3s; agent destroyed, CLI destroy deadline_exceeded×2 → manual root cleanup) | smoke=ok (health 200, 37 tables, schema v24) | Artifacts: docs/dogfood/2026-09-26-fix-verification-cli.md, diagnostics.md addendum, skills/consensus-usage v2.8.0. Board rows DF-CONSENSUS-33/34/35 — commit a86d20c (3 rows, numstat-verified).
| 2026-09-26b | 🟡 PROMISING-BUT-ROUGH | (2nd run today; ANGLE = real opencode client attach + webhook ingestion — the two never-dogfooded surfaces, both with fresh foreman commits) SPEC-017: `opencode attach` works against the shim; SPEC-013: webhooks ingest, route, wake agents. | 1. DF-CONSENSUS-43 (P1) TUI attach crashes on boot: shim GET /config lacks provider_default the pinned client reads; black screen, zero requests hit the server. 2. DF-CONSENSUS-44 (P1) run --attach fire-and-forget: 0 bytes + exit 0 in 1s while the agent answers invisibly 5s later. 3. DF-CONSENSUS-42 (P1) --config flag silently ignored (port-decoy A/B proof); CONSENSUS_CONFIG env works. | ~7 min (worktree→build→init→serve→first ledger reply); healthy turn 2.8-3.2s | install_seconds=109 (dedi-2 substitution: las-03 down 4th run, las-01 down, las-02 bunkerd crash-loop 'non-loopback plaintext' restart counter 46627; clone 8s @ fa24a3e, Go provisioned, health 200 schema 24; bare /mcp 200 SSE = DF-CONSENSUS-23 independently verified FIXED; destroy deadline_exceeded×3 → TTL backstop + manual cleanup pending) | smoke=ok

Run details (2026-09-26 evening, @ fa24a3e, clean worktree — main tree had 16 uncommitted foreman files):
- Real client: opencode 1.18.29 (the repo's own pinned evidence target) via `run --attach` and TUI `attach` in a PTY; scratch instance :8130 sqlite /tmp/dogfood-run/dogfood.db, real DeepSeek key (probed 200 first).
- Verified working: shim basic-auth (admin key), keyless /global/health + /instance, DF-CONSENSUS-37 fixed-workspace fix (anonymous GET /path echoes worktree), message POST → canned ack 66ms → real agent_response in ledger ~3s; webhook full recipe per docs/API.md (202 valid, 202+quarantined bad-sig, 200 duplicate, routing rule pending→routed <12s); webhook wake flips paused→idle (log-proven) — BUT no iteration dispatched, payload never reaches agent, stranded messages stay stranded (DF-CONSENSUS-46 P2); shim stores user text JSON-quoted vs raw native path (DF-CONSENSUS-45 P2).
- Bonus live verification: circuit breaker persisted 3 rows + paused session after 3 real LLM 401s (my unexpanded ${DEEPSEEK_API_KEY} env — resume via native PATCH status=resume works; verbs pause/resume/cancel).
- Perf: no PERF row — healthy turns 2.8-3.2s post→reply (LLM 1.9-3.9s, accept 66ms), cold first turn ~5s; PERF-CONSENSUS-11 stall class did not reproduce (6-turn sample); nothing slow enough to profile.
- Tasks written: DF-CONSENSUS-42 (P1), -43 (P1), -44 (P1), -45 (P2), -46 (P2) — board JSONL, numstat 5/0 verified.
- Artifacts: docs/dogfood/2026-09-26-opencode-shim-webhooks.md, diagnostics.md addendum, skills/consensus-usage/SKILL.md v2.9.0.
- Foreman: NOT woken (21600 fleet law; injected rows picked up at normal cadence per briefing).


2026-10-02 | 🟡 PROMISING-BUT-ROUGH | (angle = fork endpoint verification post-ROUTE-FIX-011; binary rebuilt Oct 2 00:43) | 1. DF-CONSENSUS-47 (P2) GET /session/{id}/children drops model_id — returns model:"" while GET /session/{id} returns correct value for same row. | t2fs ~2s (server already running from prior tick); fork API ~50ms | friction 1 | install_seconds=0 (binary pre-built) | smoke=ok (fork works, error paths correct: 404 unknown session, 400 bad messageID format, 404 missing message) | Artifacts: board row DF-CONSENSUS-47 | Foreman: NOT woken (existing cadence sufficient for P2)

2026-10-13 | ✅ SOLID (new-surface angle: the two route families merged Oct-13 morning — /mcp auth+connect (ROUTE-ADD-094..098) and /pty/{ptyID} (ROUTE-ADD-105..109) — first dogfood drive of either; prior runs never touched them) | Driven as a real opencode client against a scratch :8271 sqlite instance built from HEAD 3220307 (fresh build, 1.0s). PTY family FULLY conformant live: list [], shells enumerated, create typed 501 with plain-language detail, get/update/remove/connectToken typed PtyNotFoundError 404, connect 403 foreign-workspace vs 404 served-workspace split correct, ill-shaped ID 400, undeclared methods keep pre-change 404. MCP family correct per-operation but ONE real P1: PATCH /config accepts the declared mcp:{name:...} block and writes it under the literal key 'mcp' WITHOUT flattening to the mcp.<name> keys the family resolves — every /mcp/{name}/* op 404s a name the user just registered; the dotted-key form works, proving the resolver, not the client. Plus P2: GET /config returns raw Go map strings ("map[websearch:map[...]]") — jq gets null, no programmatic round-trip. | Perf: every shim response 0.4-1.9ms; nothing slow enough to profile, no PERF row (skill law: a win nobody can feel is not a finding). | install_seconds=125 total (spawn 25.0 + clone 2.5 + cold build 92.7 + init/serve/health 4.4; bunker-mvp substitution — las-02/03/04 all ssh-timeout, 5th consecutive las-03 failure, re-proves DF-CONSENSUS-32/35 premise), smoke=ok (fresh-clone auth enforced, /doc 200, version 0.1.0; destroy orphan-gate refused once — live serve process — killed via serve.pid then destroyed clean, residue recheck at 90s clean) | Board rows: DF-CONSENSUS-53 (P1 config/mcp flatten), DF-CONSENSUS-54 (P2 config map-string serialization), DF-CONSENSUS-55 (P2 install-verified, closed). Foreman: normal cadence.

2026-10-13b (re-proof run; earlier turns same day committed 8e4752f with rows 53/54/55 + full new-surface drive) | RE-PROOF of pending rows on fresh evidence | angle: re-prove DF-CONSENSUS-53/54 premises on the CURRENT :8271 HEAD-3220307 scratch instance (server restarted from the surviving scratch DB after the pre-compaction shutdown; key recovered by re-inserting an admin row - first-admin key raw secret is never re-printable, a small operational footnote for the bootstrap design). | Premises BOTH re-proven: DF-53 - PATCH /config mcp:{dfproof1:...} answers 200 keys:1 but writes the literal key 'mcp' (store shows mcp=map[dfproof1:...]); POST /mcp/dfproof1/auth 404s McpServerNotFoundError; the dotted-key form mcp.dfproof1 resolves instantly (400 McpUnsupportedOAuthError) proving the resolver is fine and the flatten step is the entire gap. DF-54 - GET /config settings.mcp renders verbatim as "map[websearch:map[type:remote url:...]]" raw Go map string. | Also completed this run: the full live /pty battery (13 probes incl. declared 403 foreign-workspace arm on /pty/{id}/connect, 400 Params/Query/Payload/Body arms, 404 typed NotFound) and resolved+unresolved /mcp arms against mcp.websearch (McpUnsupportedOAuthError 400, truthful-false 200 connect/disconnect, DELETE auth 200, whitespace 400, blank selector 400) - all conformant per the pinned 1.18.33 contract; no NEW findings, so no new rows (duplicate rows would be waste). | install_seconds=0 (leg already PASSED this day: DF-CONSENSUS-55, bunker-mvp, 125s - no repeated premise to prove; siblings' asce/9router agents on bunker-mvp are NOT this lane's, left untouched) | smoke=ok (health 200, auth 401/200 enforced) | Foreman: normal cadence; board NOT touched this run.

2026-10-16 (angle = provider contracts + native SSE + crash recovery + new /api families; runs earlier today (sibling, 4c171c7 board-only) did fresh-user keyless/mock-LLM + key precedence) | 🟡 PROMISING-BUT-ROUGH | README promises "the database is the runtime with real-time streams and kill-9 durability": turns, billing, durability, resume, and the new REST families all held; the event-stream promise broke. | 1. DF-CONSENSUS-56 (P1) GET /api/v1/events never publishes terminal transitions — 6 captures (incl. concurrent readers) end at thinking while REST showed idle; pinned internal/harness/executor.go:822 (SQL-only status write, no PublishSessionUpdate; all publishers in internal/api). 2. DF-CONSENSUS-57 (P1) origin/master c9b7747 lags local 4c171c7 by 7+ commits — fresh clones get no thinking-mode/streaming work, schema 28 vs 30. 3. DF-CONSENSUS-58 (P2) restart without key env = silent empty ${DEEPSEEK_API_KEY} → 3 burned 401 iterations → paused session (fail-closed correct, restart trap); fixed pprof :8095 collides for any second instance. Re-proofs on HEAD: DF-53 (mcp flatten) and DF-54 (native JSON config) both FIXED. New /api families (agent/command/health/model/skill/reference/provider/integration/fs/credential) conformant on first real drive; v2 /api/event refuses honestly per declared narrowing. Install leg PASSED on bunker-mvp agent dfsync-df (las-03 down 6th consecutive run): clone 1s/build 86s/init 2s/health+Chronicle 200; env-var path re-proven (DF-10 stays fixed); crash leg: 2× kill -9, session/memory/billing/key intact, resume works. | t2fs ~4 min (init→serve→first reply) | cold turn 4.1s = warm 4.1s, ack 39ms; nothing slow enough to profile, no PERF row | install_seconds=89 (go1.26.5 preinstalled on agent image; destroy clean) | smoke=ok | Artifacts: docs/dogfood/2026-10-16-provider-contracts-sse.md, skills/consensus-usage v2.10.0. Board rows DF-CONSENSUS-56/57/58 — appended pre-census-verified, id-diff checked. Foreman: normal cadence.

2026-10-20 (angle = the two surfaces landed after the 10-16 run: GET /api/pty ROUTE-ADD-028 and GET /api/provider/{providerID} ROUTE-ADD-027, both merged 634517b/a745ffb 2026-10-19; plus partial verification of the DF-CONSENSUS-58 fix dabab7a) | ✅ SHIPPABLE on the driven surfaces, one new P1 filed | /api/pty conformant on EVERY arm (401 unauth / 200 truthful-empty data[] + real LocationInfo / 400 blank, unknown-field, unreadable-dir location selectors with field+kind / POST 501 typed not_implemented naming v2.pty.create / 405 / deep-path default 404 / v1 /pty untouched 200 []). GET /api/provider/{id} correct per-arm and resolves real credential rows. NEW FINDING DF-CONSENSUS-59 (P1): PUT /auth/{providerID} (server.go:2238) validates NOTHING — {"bogus":true} → 200 "saved", persisted verbatim as auth.deepseek.bogus|true; A/B control: junk PUT on zz-fake-provider then flips GET /api/provider/zz-fake-provider from the correct 404 to a 200 PHANTOM provider (never-touched control stays 404), missing-required-key body also accepted; DELETE cleanup works so the fix is PUT-side validation against the declared Auth anyOf. DF-58 WARN leg re-proven (keyless boot logs the No-LLM-key WARN first) + :8095 collision premise re-proven on this host (bind: address already in use vs a foreign process); restart-trap leg left to the row's owner (event 659). | Perf: /api/pty p50 0.53ms max 0.61ms (n=20), provider-get p50 0.64ms, cold-first 0.56ms, init 1.29s; nothing slow enough to profile — no PERF row (skill law). | install_seconds=89 fresh-install OK on bunker-mvp agent 3ecd9aa8 @ exact HEAD facf424 (native suite PASS; ci-act UNVERIFIED = known INT-CI-7 Docker-push Bad-credentials class; chaos-disconnect UNVERIFIED = documented vacuity class; docker cells skipped by policy; agent destroyed by collect) | smoke=ok (validate OK, 590→590 warnings, 0 new errors) | Artifacts: docs/dogfood/2026-10-20-v2-pty-provider-surface.md, skills/consensus-usage v2.11.0, board row DF-CONSENSUS-59 + events 658/659. Foreman: normal cadence; foreman NOT woken by this lane.

2026-10-20 (angle = approval-gate end-to-end (first real drive of APPROV-GATE-001's user surface) + GET /api/pty (new Oct-19 route) + DF-58 re-proof + SSE DF-56 fix verification; local scratch :18777 @ 6736542 + fresh-clone install leg on bunker-mvp) | 🟡 PROMISING-BUT-ROUGH | README promise "the database is the runtime" held again: turns, billing, kill -9 durability (billing byte-identical across restart), documented resume, SSE terminal transitions (DF-56 fix verified live: thinking AND idle both streamed). Approvals loop WORKS end to end when seeded (gate fires within one 500ms poll, review flips approved, request executes) — but the user-facing on-ramps are broken: review body vocabulary contradicts the docs, timestamps serialize as zero-time, and nothing on a fresh install can create the gated tool the gate needs. | 1. DF-CONSENSUS-57-R2 (P2, recurrence) origin/master 92050ad lags main again: GET /api/pty (634517b) unpushed — fresh GitHub clone 404s the route (proved on clean bunker agent dfsync-df1020, install leg PASSED, probe.sh). 2. DF-CONSENSUS-60 (P2) docs say decision=approve/reject; API validates approved/rejected/modified — documented body 400s. 3. DF-CONSENSUS-61 (P2) approvals API returns created_at/reviewed_at 0001-01-01T00:00:00Z + expires_at null while DB rows are correct — expiry (approval_timeout_minutes=60) invisible to clients. 4. DF-CONSENSUS-62 (P3) tools_registry ships EMPTY (0 rows, no seed migration, CLI inspect-only, no REST create); docs' own POST /tools/query/execute example 400s 'session_id is required' — approval gate reachable only via direct SQL (which this run used to prove the loop). Re-proofs on HEAD: DF-58 premise LIVE (keyless restart burned 2× 401 iterations → paused; +pprof :8095 collision on FIRST instance this time — host demo process imhotep-demo pid 7506 holds it; new occupied-port diagnostic message is excellent); DOGFOOD-111 env-override WARN re-proven; DF-54 GET /config now proper JSON (no map-strings); DF-9 dedupe working as designed (5s window, log line 'dropped duplicate user_message'). Run-integrity lesson (logged for future ticks): my keyless restart silently outlived its leg — serve3 'port occupied by a Consensus server' refusal left serve2 serving legs 3/3b, contaminating two conclusions (false 'resume doesn't clear breaker'; false 'wedged booting session') until the A/B was re-run on a verified-keyed server; ALWAYS assert pid+key-probe+bind after every restart inside a leg. | t2fs n/a (scratch reuse); approvals t2fs via SQL seed ~15 min | cold turn 2.09s (create 55ms, ack 35ms), warm 2.06s; approval gate poll <500ms; nothing slow enough to profile, no PERF row | install_seconds=92 (clone 1s @ 92050ad + build 89s + init 1s + health/Chronicle 200; go1.26.5 preinstalled; agent destroyed clean after evidence) | smoke=ok (fresh-clone health 200 version 0.1.0, auth 401/200 split correct on pty+approvals, Chronicle 200 312KB) | Artifacts: docs/dogfood/2026-10-20-approvals-pty-gate.md (this repo), dogfood-log this entry, board rows DF-CONSENSUS-57-R2/60/61/62 (append-verified: 438→442 lines, id diff exact). Foreman: normal cadence (read-only lane did not wake or mutate scheduler state).
