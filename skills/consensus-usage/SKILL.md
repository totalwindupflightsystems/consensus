---
name: consensus-usage
description: >-
  How to use the Consensus agent runtime for real: bootstrap a server,
  drive the REST API, integrate the Go client library, manage sessions via
  the CLI, and avoid the current landmines (mcp-stdio --api-key does not
  authenticate initialize, no public semantic-retrieval endpoint, H3
  example hardcodes port 8095). Written from two dogfood runs
  (2026-08-04, 2026-08-15) — reflects actual behavior, not README claims.
  Aug-4 landmines (append-only triggers, pause/resume, circuit breaker,
  ports) and the Aug-15 landmines DOGFOOD-101/102/103 (MCP auth, MCP
  session IDs, OpenAPI from repo root) are FIXED — do not treat them as
  open.
version: 2.1.0
category: software-development
---

# Consensus — Usage Skill

Consensus is a database-native agent runtime ("the database IS the agent"):
agent context is a live SQL view, memory is a ledger in SQLite/Postgres,
sessions survive `kill -9`, and everything is manageable over REST + CLI +
MCP. Module: `github.com/wojons/consensus` (branch `master`, Go 1.26).

## Entry points

| Surface | How |
|---|---|
| Server | `consensus init` (bootstrap + admin key + memory bank) then `consensus serve --config <cfg>` |
| REST API | `http://127.0.0.1:<port>/api/v1/*`, auth `Authorization: Bearer cs_ak_...` — solid, use this for real work |
| Go client | `pkg/client` — typed, matches REST 1:1, the best surface (verified 5/5 on 2026-08-15) |
| CLI | `consensus status/session/memory/approve/config/tool/skill/migrate/models` — all functional |
| MCP | `consensus mcp-stdio --api-key …` and `/mcp/sse` — works, but see LANDMINE 1 (auth) |
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
  key in `initialize`'s `_meta.authorization`. 6 tools: create_session,
  send_message, get_session_status, list_memory, review_approval,
  query_tool.
- **stdio works for tools/list+**: `printf '<jsonrpc>' | consensus mcp-stdio
  --server http://… --api-key …`.

## Current landmines (from the 2026-08-15 re-run — DO NOT assume fixed)

1. **`consensus mcp-stdio --api-key` does not authenticate initialize**
   (DOGFOOD-106): initialize still says "Authentication required". SSE
   clients must put the key in `_meta.authorization` themselves.
2. **Semantic retrieval has no public endpoint** (unchanged since Aug-4):
   retrieval is harness-internal; don't look for a search API.
3. **H3 is a library, not mounted** (DOGFOOD-107): `consensus serve` does
   not expose `/v1/*`; mount `internal/shim/h3` yourself (INTEGRATION.md
   §2.3 has a runnable keyless example; change the port if 8095 is taken).

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
  30/30 green without DEEPSEEK_API_KEY (~142s).
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

## Crash recovery (verified both runs)

`kill -9` the server → DB intact → restart with the same config → sessions
and memory events all there. This claim is real.
