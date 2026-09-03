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
version: 2.4.0
category: software-development
---

# Consensus — Usage Skill

Consensus is a database-native agent runtime ("the database IS the agent"):
agent context is a live SQL view, memory is a ledger in SQLite/Postgres,
sessions survive `kill -9`, and everything is manageable over REST + CLI +
MCP. Module: `github.com/wojons/consensus` (branch `master`, Go 1.26).

> **2026-09-03 status:** the goal-driven execution pattern below is verified
> working with real LLM calls. The documented conversational message contract
> (`{"role":"user",...}`) is verified BROKEN (DF-CONSENSUS-6) — use the
> goal-driven pattern until it's fixed.

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
// go.mod:  require github.com/wojons/consensus v0.0.0
//          replace github.com/wojons/consensus => /path/to/consensus
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
  - 6 tools (`tools/list`): create_session, send_message,
    get_session_status, list_memory, review_approval, query_tool
  - 2 resources (`resources/list`): sessions, tools_registry
  - 1 resource template (`resources/templates/list`): session_context
  - 1 prompt (`prompts/list`): agent_status
- **stdio works for tools/list+**: `printf '<jsonrpc>' | consensus mcp-stdio
  --server http://… --api-key …`. Since DOGFOOD-106 → `706358c`, the
  `--api-key` flag (flag > env > config) is injected into `initialize`'s
  `_meta.authorization`, so initialize authenticates instead of returning
  "Authentication required".

## Current landmines (from the 2026-09-03 run — DO NOT assume fixed)

0. **The documented conversational path is DEAD (DF-CONSENSUS-6, verified
   2026-09-03 with real LLM calls).** `POST /sessions/{id}/message` with
   docs/API.md's `{"role":"user","content":...}` payload never reaches the
   LLM: the planning prompt stays `messages=2` every turn, the model itself
   reports "No user-supplied text is present in this turn", no assistant
   reply is ever produced anywhere, tokens stay 0, and the session ends
   `idle` with no error surfaced. **Do not use this path.** Use the
   goal-driven pattern above (task in `goal` + `{"type":"user_instruction"}`
   wake message) — that one verifiably stages and executes SQL and lands
   memory events. Until DF-CONSENSUS-6 is fixed, treat any docs example
   using `{"role":...}` as broken.
1. **Neither README quickstart works from a clean machine** (DF-CONSENSUS-7,
   verified on an ephemeral bunker agent 2026-09-03): `docker pull
   ghcr.io/wojons/consensus:latest` → `denied` (image not anonymously
   pullable; the same agent pulls alpine fine), and the GitHub repo is not
   anonymously cloneable. With repo access the verified fallback is:
   tar the tree over, bootstrap Go 1.26.5 (agent has no Go, no sudo —
   set GOPATH away from the extracted GOROOT), `go build` ≈ 61 s,
   init/serve/health-200 PASS.
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

## Crash recovery (verified three runs)

`kill -9` the server → DB intact → restart with the same config → sessions
and memory events all there. This claim is real (re-verified 2026-09-03:
heartbeat auto-resumed the in-flight session). Caveat (DF-CONSENSUS-8): what
you recover is your own input plus session state — an in-flight conversational
run emits zero durable output, so there is no agent work to recover unless the
goal-driven path had already committed it. `staging_buffer` keeps stranded
rows from abnormal exits; check it to see what a run *tried* to do.
