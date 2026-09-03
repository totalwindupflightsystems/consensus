# Dogfood Integration Report — 2026-09-03

**Run type:** cron dogfood (coding-hermes-dogfood skill), full real-use run + ephemeral bunker install leg.
**Verdict:** 🟡 PROMISING-BUT-ROUGH (third consecutive run with this label, but the failure surface moved: Aug runs were MCP/docs issues; today the *documented conversational path* is the thing that does not work).
**Time-to-first-success:** ~120 s (build 15 s + init + serve + health 200). A user who then follows the docs to *talk to the agent* never reaches success at all — see DF-CONSENSUS-6.
**Friction count:** 6 (2 blocking, 3 doc-level, 1 infra).

---

## Promise vs reality

| README promise | Verified live? |
|---|---|
| `consensus init` prints one-time admin key, 90-day expiry | ✅ exact match (fresh scratch DB) |
| init → serve → health `status:ok` on :8090 (we used :18201) | ✅ ~2 min including build |
| `POST /sessions` 201 with id/api_key/status | ⚠️ works but 400s on undocumented `agent_name` (still, reconfirms DF-CONSENSUS-2), and requested `model` is ignored → `"model":"default"` (new, DF-CONSENSUS-9) |
| `POST /sessions/{id}/message` "Triggers the harness loop; returns the agent response" | ❌ **returns `message_received`, no agent response is ever produced or observable** (DF-CONSENSUS-6) |
| Append-only memory ledger, ACID | ✅ goal-driven staged SQL INSERT landed as `thinking` event; ledger enforced |
| "Survives kill -9" | ✅ storage layer (session + messages intact, heartbeat auto-resume) / ⚠️ nothing *useful* to recover because in-flight runs emit zero durable output (DF-CONSENSUS-8) |
| Circuit breakers, billing, audit | ❌ empty tables after ~30 real LLM calls on the interactive path (part of DF-CONSENSUS-6) |
| Docker quickstart `docker pull ghcr.io/wojons/consensus:latest` | ❌ denied — image not anonymously pullable; repo also not anonymously cloneable (DF-CONSENSUS-7) |
| README demo (real LLM calls, memory events, crash recovery) | ✅ the demo's own code path works — because it uses `goal` + `{"type":"user_instruction"}`, not the documented message contract |

## The one pattern that works (use this until DF-CONSENSUS-6 is fixed)

Put the whole task in the session **goal**, send a wake message with the demo
payload shape — the agent then plans, stages SQL, executes it ACID-committed,
and writes memory events you can poll:

```bash
# 1. Session with an executable goal
curl -X POST http://127.0.0.1:18201/api/v1/sessions \
  -H "Authorization: Bearer $BOOTSTRAP_KEY" -H 'Content-Type: application/json' \
  -d '{"agent_name":"task-bot","goal":"Insert one memory event of type '"'"'thinking'"'"' with content '"'"'DF-TEST-12345 stored'"'"' for this session, then stop."}'
# → 201 {"id":"<sid>","status":"booting","api_key":"cs_sk_...",...}

# 2. Wake it (NOTE: {"type":"user_instruction"} — not the docs' {"role","content"})
curl -X POST http://127.0.0.1:18201/api/v1/sessions/<sid>/message \
  -H "Authorization: Bearer $BOOTSTRAP_KEY" -H 'Content-Type: application/json' \
  -d '{"type":"user_instruction","content":"Start working on the task now."}'

# 3. Poll memory — verified: staged SQL executed, event landed
curl http://127.0.0.1:18201/api/v1/sessions/<sid>/memory -H "Authorization: Bearer $BOOTSTRAP_KEY"
# → [{"id":8,"type":"thinking","content":"DF-TEST-12345 stored",...}]
```

What **does not** work: `{"role":"user","content":"Say hello and nothing else."}`
(the payload in docs/API.md). Server log across 4 runs / ~30 real DeepSeek
calls shows `llm: calling provider ... messages=2` on *every* turn — session
memory is never included in the prompt, so the model literally cannot see the
message. The model's own memory write during the A/B test says it plainly:
`"No user-supplied text is present in this turn - the transaction window is
empty"`. Staged-command SQL errors are also never fed back, so the model
blind-retries `no such column: id` until max_turns, then "auto-commits"
nothing and the session goes `idle` with zero error surfaced to the caller.

## Crash recovery (killed mid-loop, for real)

`kill -9` during an active planning run → restart on same sqlite DB:
health 200, session row intact, `status:planning` resumed automatically by the
heartbeat (iteration advanced), both user messages preserved in the ledger.
That part is genuinely solid. The caveat: because in-flight runs produce no
durable output (above), what you recover is your *own* input plus session
metadata — `staging_buffer` stranded 74 staged commands across runs while
`iteration_commits`, `audit_logs`, and `agent_billing` stayed at 0 rows.

## Fresh-machine install leg (ephemeral bunker, mandatory)

- bunker-las-03 spawn failed: `port range allocation: no free port ranges
  available (pool exhausted: 10 ranges)` with `bunker list` showing zero
  agents = orphaned in-memory ranges (documented class:
  `qa-foreman-ops/references/spawn-pool-exhaustion-2026-09-03.md`; fix =
  bunkerd restart, skipped — needs interactive approval). Ran on
  **bunker-las-01** instead.
- Documented Docker path: `docker pull ghcr.io/wojons/consensus:latest` →
  `denied`. Same agent pulls `alpine:latest` fine (network/daemon OK).
  Registry manifest endpoint: 401 for anonymous. `git clone` of the GitHub
  repo also requires auth. **Neither documented install path works for a
  fresh user.** Not fixed by widening anything (hard rule) — the fix is
  publishing the image in CI, or documenting the auth requirement.
- Verified fallback (with repo access): tar the tree over ssh, bootstrap Go
  1.26.5 tarball (agent has no Go, no sudo — docs assume toolchains),
  `go build -o bin/consensus ./cmd/consensus/` → **61 s**, then documented
  init/serve: health 200 `status:ok` v0.1.0 sqlite schema 23 → **smoke PASS**.
- Environment landmine recorded: on bunker-managed agents the spawn-time
  rootless dockerd already owns `/run/user/<uid>/dockerd-rootless`; the
  systemd user unit then fails with `failed to lock ... another RootlessKit
  is running with the same state directory` and restart-loops. Use the
  socket `bunker spawn` prints (`/run/bunker/<agent>/docker.sock`).

## Judgment

1. **Does it work?** The infrastructure yes (bootstrap, auth, REST, MCP, SSE,
   ledger ACID, crash recovery, goal-execution with real LLM calls). The
   product's headline interaction — send a message, get a response — no.
2. **Is it useful?** The goal-driven autonomous-execution mode is real and is
   the honest core of the "database is the runtime" claim. If the docs led
   with *that* pattern and fixed the prompt-assembly bug, this would be a
   usable product.
3. **Is it usable?** Bootstrap is excellent; the first conversation is a
   dead end with no error. A user cannot diagnose this from the outside —
   it took server-log forensics.
4. **Is it trustworthy?** ACID holds; nothing corrupted; kill -9 clean.
   Observability promises (billing/audit/breakers) did not fire on the path
   that everyone will try first.

## Tasks filed

| ID | Pri | One-liner |
|---|---|---|
| DF-CONSENSUS-6 | P0 | Conversational path broken: user message never reaches LLM; no reply ever produced/observable; billing/audit/commits empty |
| DF-CONSENSUS-7 | P0 | ghcr image not anonymously pullable; repo not anonymously cloneable — neither README install path works from zero |
| DF-CONSENSUS-8 | P1 | Crash recovery real but recovers almost nothing (zero durable output from in-flight runs; staging_buffer residue) |
| DF-CONSENSUS-9 | P2 | Install docs assume toolchains + bunkerd double-dockerd lock; model field ignored; duplicate user_message race |

Prior pending dogfood tasks re-verified today: DF-CONSENSUS-2 (400 on
documented create-session payload) still reproduces; DF-CONSENSUS-3 (async
message contract, no observable response) is *worse* than filed — there is no
response at all (folded into DF-CONSENSUS-6).
