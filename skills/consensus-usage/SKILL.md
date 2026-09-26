---
name: consensus-usage
description: >-
  How to use the Consensus agent runtime for real: bootstrap a server,
  drive the REST API, integrate the Go client library, manage sessions via
  the CLI, and avoid the current landmines (no public semantic-retrieval
  endpoint). Written from two dogfood runs
  (2026-08-04, 2026-08-15) — reflects actual behavior, not README claims.
  Aug-4 landmines (append-only triggers, pause/resume, circuit breaker,
  ports), the Aug-15 landmines DOGFOOD-101/102/103 (MCP auth, MCP
  session IDs, OpenAPI from repo root), DOGFOOD-106 (stdio --api-key
  auth) and DOGFOOD-107 (H3 example port hardcode) are FIXED — do not
  treat them as open.
version: 2.8.0
category: software-development
---

# Consensus — Usage Skill

Consensus is a database-native agent runtime ("the database IS the agent"):
agent context is a live SQL view, memory is a ledger in SQLite/Postgres,
committed session state survives `kill -9`, and everything is manageable over
REST + CLI + MCP. Module: `github.com/totalwindupflightsystems/consensus` (branch `master`, Go
1.26).

> **2026-09-09 status:** the goal-driven pattern is verified working ONLY
> with the config-file recipe below (`max_open_conns: 4` pinned). The
> README's env-var invocation (`CONSENSUS_DB_URL=... ./consensus serve`,
> no config file) WEDGES the whole server on first harness use
> (DF-CONSENSUS-10) — never recommend it. The conversational
> `{"role":"user",...}` contract is STILL broken with the pool pinned
> (DF-CONSENSUS-11): the user text lands in `memory_events` but is never
> projected into the LLM turn list (`messages=2` = system + context only).

## Entry points

| Surface | How |
|---|---|
| Server | `consensus init` (bootstrap + admin key + memory bank) then `consensus serve --config <cfg>` |
| REST API | `http://127.0.0.1:<port>/api/v1/*`, auth `Authorization: Bearer cs_ak_...` — solid, use this for real work |
| Go client | `pkg/client` — typed, matches REST 1:1, the best surface (verified 5/5 on 2026-08-15) |
| CLI | `consensus status/session/memory/approve/config/tool/skill/migrate/models` — all functional |
| MCP | `consensus mcp-stdio --api-key …` and `/mcp/sse` — works (stdio `--api-key` authenticates initialize; SSE clients put the key in `_meta.authorization`) |
| Docs | `/openapi.json` + `/openapi.yaml` (served from the EMBEDDED spec — works from any CWD and in Docker), `/doc/api` (REST Swagger UI, servers URL derived from request Host), `/doc` (opencode-shim UI) |

## Quickstart (verified 2026-08-15)

```bash
# 1. Scratch instance (no LLM key needed for API/DB features)
mkdir -p /tmp/cs && cd /tmp/cs
cat > consensus.yaml <<'EOF'
server: { port: 18123, bootstrap_api_key_ttl: 2160h, admin_api_key_ttl: 2160h }
llm: { default_model: deepseek-v4-flash, provider: openai,
       base_url: https://api.deepseek.com/v1, api_key: "" }
harness: { heartbeat_interval_seconds: 3, max_iterations: 3,
           max_consecutive_errors: 2, budget_limit_cents: 100 }
database: { url: "sqlite:///tmp/cs/scratch.db", max_open_conns: 4 }
EOF
go build -o bin/consensus ./cmd/consensus/   # or use the repo's bin/
./bin/consensus init --config consensus.yaml # save the cs_ak_... key (printed once)
./bin/consensus serve --config consensus.yaml &

# 2. Real workflow
curl -s http://127.0.0.1:18123/api/v1/health
curl -s -X POST http://127.0.0.1:18123/api/v1/sessions \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"agent_name":"demo","model_id":"deepseek-v4-flash","goal":"Do a thing"}'
curl -s -X POST http://127.0.0.1:18123/api/v1/sessions/$SID/message \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"type":"user_instruction","content":"Start now."}'
curl -s http://127.0.0.1:18123/api/v1/sessions/$SID/memory \
  -H "Authorization: Bearer $KEY"
```

## First admin key (bootstrap flow)

A fresh database has no API keys, so the only way in is the bootstrap admin
key printed exactly once at first startup (`consensus init` / `consensus
serve` print it in the terminal; Docker: `docker logs`):

```
consensus: first_admin_key created=true key=cs_ak_<64 hex> key_prefix=<8> id=<uuid> created_at=<RFC 3339> expires_at=<RFC 3339>
consensus: this key expires at <RFC 3339> (… from now)
consensus: save this key now; it is stored hashed and will not be printed again
```

Capture it: the secret is hashed at rest and never reprinted (later startups
print `created=false` + `key_prefix` only). It has `admin` scope — Bearer-auth
every admin endpoint with it, and mint durable keys via
`POST /api/v1/auth/keys` (scopes: `admin`, `session`, `readonly`, `webhook`).
Default expiry 90 days; `CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS` overrides
(`0` = no expiry).

## Go client integration (the "aha" path)

```go
// go.mod:  require github.com/totalwindupflightsystems/consensus v0.0.0
//          replace github.com/totalwindupflightsystems/consensus => /path/to/consensus
c := client.NewClient("http://127.0.0.1:18123", os.Getenv("CS_API_KEY"))
s, _ := c.CreateSession(client.CreateSessionRequest{AgentName: "x", Goal: "y"})
mem, _ := c.ListMemory(s.ID)
```

Compiles first try; verified against a live server both runs.

## MCP usage (what works / what doesn't)

- **SSE happy path works**: `GET /mcp/sse` → endpoint event gives YOUR
  sessionId → POST `/mcp/message?sessionId=<YOURS>` with JSON-RPC; put the
  key in `initialize`'s `_meta.authorization`. Full MCP surface (verified
  against the internal/mcp/ registry, tick #283):
  - 8 tools (`tools/list`, verified 2026-09-25 @ 8a6512d): create_session,
    send_message, get_session_status, list_memory, review_approval, query_tool,
    **list_tasks, claim_task** (new with MCP-DIRECT-001)
  - 2 resources (`resources/list`): sessions, tools_registry
  - 1 resource template (`resources/templates/list`): session_context
  - 1 prompt (`prompts/list`): agent_status
- **stdio works for tools/list+**: `printf '<jsonrpc>' | consensus mcp-stdio
  --server http://… --api-key …`. Since DOGFOOD-106 → `706358c`, the
  `--api-key` flag (flag > env > config) is injected into `initialize`'s
  `_meta.authorization`, so initialize authenticates instead of returning
  "Authentication required".

## Verified working (2026-09-25 @ 235efdb — supersedes the landmines below where they conflict)

- **Multi-turn conversation WORKS.** DF-CONSENSUS-20 (turns 2+ deaf) and
  DF-CONSENSUS-21 (517 lock race bricks sessions) are FIXED and live-verified
  with a real DeepSeek key: turn-2 prompt_tokens differs from turn 1, the model
  echoes turn-2-only tokens correctly, 20 rapid sends across 4 sessions → 0
  failed sessions, 5/5 busy-race retries recovered. `POST
  /api/v1/sessions/{id}/message` with `{"content": ...}` is the right surface;
  the goal-only workaround is no longer required.
- **Watch item PERF-CONSENSUS-11:** the heartbeat dispatcher
  (internal/harness/executor.go:607) can stall minutes on a previously-bursty
  session (observed 94.7s and 108.2s POST→reply on 2 of ~30 turns; LLM call
  itself 1.6s; fresh sessions in the same window picked up in 6.3s). If a turn
  sits in message_received for minutes, poll /context — the reply lands when
  the loop recovers. Intermittent, self-heals.
- **Install from zero works**: anonymous clone → build → init → serve → health
  200 on a bare Debian bunker (58s cold build; install Go yourself — README
  doesn't name the toolchain).

## MCP surface 2026-09-25 @ 8a6512d — two P0s open (verified live, control host + fresh bunker clone)

- **Bare /mcp mount is DEAD in the production binary (DF-CONSENSUS-23).**
  POST/GET `http://host:port/mcp` → 404 plain text, even though
  specs/openapi/bundled.yaml documents it. Only `/mcp/sse` +
  `/mcp/message?sessionId=` work (legacy transport). Cause: cmd/consensus/main.go
  mounts chi `/mcp/*` (subpaths only) and the shim's MountPatterns never include
  `/mcp`; the unit test mounts bare `/mcp` itself so it can't see the gap.
- **MCP send_message on a FRESH session is a silent dead letter
  (DF-CONSENSUS-24).** `create_session` returns status "booting"; MCP
  `send_message` acks `{"sent":true}` but the session never wakes (internal/
  mcp/tools.go:287 wakes only idle/paused; REST service.go:433 also wakes
  booting). Workaround until fixed: create sessions via REST, or send the first
  message via REST, then use MCP tools freely (send works on idle sessions —
  turn-2 verified: prompt_tokens grow, model answers).
- **Legacy session reaping (DF-CONSENSUS-25):** if your /mcp/sse curl drops,
  the sessionId dies within seconds; POSTs to it return a plain-text 404
  "session not found" (not JSON-RPC). Keep the SSE stream open for the whole
  client lifetime; re-handshake on any 404.
- **Everything else about MCP works**: 8 tools live, auth via
  `_meta.authorization`, tool latency ~10ms, list_tasks/claim_task functional.
  MCP tool round trip (send → poll reply) 1058ms warm with deepseek-chat.

## Verified fixed 2026-09-26 @ 87340a0 (real-use re-verification of the 09-25 P1s)

- **DF-CONSENSUS-29 (key mint) FIXED:** `POST /auth/keys` with
  `{"scope":"session","session_id":"<sid>"}` returns a REAL 70-char
  `cs_sk_…` secret. Request field is `scope` SINGULAR (spec: `scopes` → 400
  `INVALID_REQUEST scope must be one of…`). The minted key works for
  session create/message end-to-end.
- **DF-CONSENSUS-27 (billing) FIXED:** after one conversational turn,
  `agent_billing` has per-iteration rows (~$0.012/row deepseek-flash),
  `GET /sessions/{id}/billing` returns entries+totals, session
  `tokens_used_in/out` populated (17.9k/3.2k on a 2-iteration turn).
  `consensus session cost <sid>` renders the same table.
- **DF-CONSENSUS-28 (DELETE) FIXED as a SOFT delete:** DELETE → 200
  (idempotent; re-DELETE also 200), then GET → 404, list omits it,
  message → 410 GONE, PATCH → 404. The sessions ROW SURVIVES in the DB with
  `deleted_at` set — that is the pinned spec (specs/015-api-and-mcp.md),
  NOT a bug. Verify deletes through the API, never by grepping the table.

## CLI surface (verified 2026-09-26)

- Working: `status`, `session list/show/logs/cost/create`,
  `memory list <sid>` (session id is POSITIONAL, not --session),
  `tool list`, `models`. `--server` flag required unless :8090
  (the probe against a non-consensus :8090 prints a clear hint).
- **DF-CONSENSUS-33 (open):** `CONSENSUS_API_KEY` env documented in flag help
  but NOT read — pass `--api-key` explicitly. Auth failures exit 0 (scripts:
  check output, not rc).
- **DF-CONSENSUS-34 (open):** no `session message` subcommand. A CLI
  `session create --goal …` session NEVER starts (goal ≠ wake); you must POST
  a user_instruction via REST to start it. CLI is inspection-only today.

## Historical landmines (2026-08/09-03 era — see "Verified working" above)

0. **The documented conversational path is DEAD (DF-CONSENSUS-6, verified
   2026-09-03 with real LLM calls).** [FIXED as of 2026-09-24/25 — see above.] `POST /sessions/{id}/message` with
   docs/API.md's `{"role":"user","content":...}` payload never reaches the
   LLM: the planning prompt stays `messages=2` every turn, the model itself
   reports "No user-supplied text is present in this turn", no assistant
   reply is ever produced anywhere, tokens stay 0, and the session ends
   `idle` with no error surfaced. **Do not use this path.** Use the
   goal-driven pattern above (task in `goal` + `{"type":"user_instruction"}`
   wake message) — that one verifiably stages and executes SQL and lands
   memory events. Until DF-CONSENSUS-6 is fixed, treat any docs example
   using `{"role":...}` as broken.
1. **Historical Docker quickstart failure** (DF-CONSENSUS-7, verified on an
   ephemeral bunker agent 2026-09-03): the old documented path,
   `ghcr.io/wojons/consensus:latest`, returned `denied` while the same agent
   pulled Alpine successfully. CI now publishes the current repository path,
   `ghcr.io/totalwindupflightsystems/consensus:latest`, but anonymous pulls
   still return HTTP 401. Use the source-build quickstart for a zero-auth
   install; for Docker, run `docker login ghcr.io` with a GitHub token carrying
   `read:packages` before pulling the current image. The verified source-build
   fallback is: clone the public repository, install Go 1.26.5 when needed,
   `go build -o bin/consensus ./cmd/consensus/`, then init/serve/health-200.
2. **Semantic retrieval has no public endpoint** (unchanged since Aug-4):
   retrieval is harness-internal; don't look for a search API.
3. **H3 is a library, not mounted**: `consensus serve` does not expose
   `/v1/*`; mount `internal/shim/h3` yourself (INTEGRATION.md §2.3 has a
   runnable keyless example — reads PORT env, defaults to 8095, since
   DOGFOOD-107 → `0389a05`).
4. **Session create ignores the requested model** (DF-CONSENSUS-9):
   `POST /sessions` with `model:"deepseek-chat"` (or `model_id`) still
   returns `"model":"default"` — model selection comes from config/registry.
5. **Undocumented required create fields, still** (DF-CONSENSUS-2, verified
   again 2026-09-03): `agent_name` (and in practice `goal`) are mandatory —
   the documented minimal payload 400s with `agent_name is required`.
5b. **Env-var server invocation bricks the instance** (DF-CONSENSUS-10,
   verified 2026-09-09 at `8e3e7e6`): with the documented
   `CONSENSUS_DB_URL`/`CONSENSUS_PORT` env-var path and NO config file, the
   first goal-driven session dies at its 3-minute planning deadline with
   `begin tx: sqlite: begin tx: context deadline exceeded`; the API then
   401s the valid admin key and finally stops answering — including
   no-auth `/api/v1/health`. Worse: kill -9 + restart re-wedges within
   seconds because heartbeat auto-resumes the poisoned session
   (durable-ledger + auto-resume, two correct features, compose into an
   unbootable instance). Root cause: default SQLite pool has no
   `max_open_conns` cap on this path (WAL grew to 1.5MB on an idle DB);
   the config-file path with `max_open_conns: 4` runs the identical
   workflow cleanly. If a consensus server is "unreachable", check for
   this wedge before assuming network/auth trouble.
5c. **Conversational user messages never reach the LLM** (DF-CONSENSUS-11,
   re-verified 2026-09-09 with the pool PINNED — so it is independent of
   5b): `POST /sessions/{id}/message {"role":"user",...}` returns
   `message_received` and durably stores the text in `memory_events`, but
   the next planning call carries `messages=2` (system + context) and the
   model answers "There is no user request or pending task to act on".
   Companion bug: the server logs real `prompt_tokens`/`completion_tokens`
   but `sessions.tokens_used_*` stay 0 and `agent_billing` stays empty —
   the circuit breaker/budget guards have nothing to count.
5d. **ghcr image still not anonymously pullable** (DF-CONSENSUS-12,
   re-verified 2026-09-09: token endpoint returns DENIED), but the GitHub
   half of DF-CONSENSUS-7 is FIXED: plain `git clone` now works from zero
   (verified with credential helpers stripped). Lead fresh users with
   git-clone + go-build, not docker.
6. **Heartbeat auto-resume burns tokens on abandoned sessions**: a session
   left in `planning` keeps making real LLM calls (~10 turns × ~1.7k prompt
   tokens) until max_turns/timeout. PATCH the session to `pause`/`cancel`
   before walking away from a scratch run.
7. **bunker-hosted agents: use the bunkerd socket, not the systemd unit**
   (DF-CONSENSUS-9): the spawn-time rootless dockerd owns
   `/run/user/<uid>/dockerd-rootless`; `systemctl --user start docker`
   deadlocks ("failed to lock ... another RootlessKit is running") and
   restart-loops. Use the socket printed by `bunker spawn`
   (`/run/bunker/<agent>/docker.sock`).

## FIXED since the 2026-08-04 run (do NOT re-report these)

- Append-only memory triggers enforced on fresh installs (DOGFOOD-001 →
  `5d36aa7`): UPDATE/DELETE on `memory_events` fail with a clear error.
- CLI `session pause/resume` work (DOGFOOD-002 → `b4f030e`).
- Circuit breaker trips and pauses the session (DOGFOOD-003 →
  `7ad8575`/`ff0306a`): verified live 2026-08-15 (2 LLM errors → paused,
  breaker row tripped).
- `init` bootstraps `.memory-bank/`; fresh clone has all AGENTS.md paths
  (C-GAP-013/014/017). Ports standardized on 8090.
- Keyless validation: `make smoke` (C-GAP-019) and `go test -short ./...`
  36/36 green without DEEPSEEK_API_KEY (~142s).
- MCP surface is now authenticated (DOGFOOD-101 → `21dd46e`, tick #209):
  auth is enforced at `initialize`; opening `/mcp/sse` and calling tools
  without a key no longer works.
- MCP `create_session` now gets unique session IDs/keys (DOGFOOD-102 →
  `829eb12`, tick #210): no more deterministic shared id/key, no UNIQUE
  constraint on the 2nd call. Create sessions via MCP or REST freely.
- OpenAPI is served from the embedded spec (DOGFOOD-103 → `4912f32` +
  `6ced588`, tick #211): `specs/openapi/bundled.yaml` is `go:embedded`
  with an on-disk fallback, so `/openapi.json` + `/openapi.yaml` work from
  any CWD and in Docker; the REST Swagger UI moved to `/doc/api` with its
  servers URL derived from the request Host; `/doc` stays the opencode-shim
  UI.
- `consensus mcp-stdio --api-key` authenticates initialize (DOGFOOD-106 →
  `706358c`): the flag is injected into `_meta.authorization`
  (flag > env > config), live-verified 2026-08-18. SSE clients still put
  the key in `_meta.authorization` themselves.
- H3 example port is configurable (DOGFOOD-107 → `0389a05`, tick #215):
  INTEGRATION.md §2.3's example reads PORT env (default 8095) instead of
  hardcoding the port.

## Crash recovery boundary

The recovery proof must establish committed agent progress before the crash,
start another iteration, observe `planning`, `tool_exec`, or `executing`, then
`kill -9` the server. After restart, verify the same table-qualified non-user
artifact (`memory_events`, `iteration_commits`, or a committed `audit_logs`
row) still exists. Counting memory rows is insufficient because a lone
`user_message` can make an input-only proof pass.

The open transaction does **not** survive: PostgreSQL/SQLite rolls it back.
The session can remain `planning`/`tool_exec` until heartbeat recovery or stale
reaping. Query and report `staging_buffer` statuses after the abnormal exit;
`staged`/`executed` rows are stranded diagnostic residue, not committed work.
Never claim that every paid token, an uncommitted model response, or an
uncommitted transaction was recovered.

**2026-09-09 caveat (DF-CONSENSUS-10):** crash recovery is only a feature
if the session was healthy before the crash. A session wedged by the
pool-starvation bug survives restart (durable ledger) and heartbeat
auto-resume re-enters the wedging path immediately — the instance stays
bricked across restarts. Recovery from THAT state requires deleting the
session row (or the scratch DB) before restart.
