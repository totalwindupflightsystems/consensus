---
name: consensus-usage
description: >-
  How to use the Consensus agent runtime for real: bootstrap a server,
  drive the REST API, integrate the Go client library, manage sessions via
  the CLI, and avoid the known landmines (append-only triggers missing on
  fresh installs, broken pause/resume CLI verbs, empty tools registry).
  Written from a 2026-08-04 dogfood run — reflects actual behavior, not
  README claims.
version: 1.0.0
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
| Server | `consensus init` (bootstrap + admin key) then `consensus serve --config <cfg>` |
| REST API | `http://127.0.0.1:<port>/api/v1/*`, auth `Authorization: Bearer cs_ak_...` |
| Go client | `pkg/client` — typed, matches REST 1:1, the best surface |
| CLI | `consensus status/session/memory/approve/config/tool/skill/migrate/models` |
| MCP | `consensus mcp-stdio --api-key cs_ak_...` (JSON-RPC over stdio) |
| Docs | `/openapi.json` + `/doc` (Swagger UI), `/chronicle/` dashboard |

## Quickstart (verified 2026-08-04)

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
go run github.com/wojons/consensus/cmd/consensus init --config consensus.yaml \
  --db-url "sqlite:///tmp/cs/scratch.db"     # save the cs_ak_... key (printed once)
go run github.com/wojons/consensus/cmd/consensus serve --config consensus.yaml &

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
Compiles first try; all 16 client methods verified against a live server.

## Known landmines (from the 2026-08-04 dogfood run)

1. **Append-only memory is NOT enforced on fresh installs** (DOGFOOD-001,
   P0). `UPDATE memory_events` succeeds; migration 017's triggers are
   silently stripped by `filterForSQLite`. Don't build audit/compliance
   workflows on ledger immutability until fixed.
2. **`consensus session pause|resume` are broken** (DOGFOOD-002, P1) —
   `unknown status action: "paused"`. Workaround: `PATCH
   /api/v1/sessions/{id}` with `{"status":"pause"}` / `{"status":"resume"}`.
   `session cancel` works.
3. **No API key → cryptic failure**: sessions go `failed` with 0 tokens and
   the log shows `llm: parse response (status 401): invalid character 'A'...`
   (DOGFOOD-004). The real reason is in `audit_logs.error_message`.
4. **Tools/skills registries are empty** on a fresh install (DOGFOOD-005);
   `tool list`/`skill list` print `(no results)`.
5. **No user-facing semantic search endpoint** (DOGFOOD-006); retrieval is
   harness-internal and needs an embedding API.
6. **MCP auth** is via `--api-key`/config only — ignore the `_meta`
   authorization error messages (DOGFOOD-007).
7. **Ports**: README says 8090, repo `consensus.yaml`/`init` output say 8094
   — pass `--port`/config explicitly.
8. **Demo test skips silently** without `DEEPSEEK_API_KEY` — a green
   `go test ./demo/` proves nothing.

## Crash recovery (verified)

`kill -9` the server → DB intact on disk → restart with the same config →
sessions and memory events are all there. This claim is real.

## Repo conventions for agents

- GitReins guards every commit (`gitreins guard`); docs-only changes pass.
- Branch `master`; never push (foreman convention).
- Board: `.coding-hermes/board/board.db` (DuckDB) + parquet exports; task
  rows have `id/title/status/priority/complexity/capability_tags`.
- Agent commits carry `Co-authored-by: Alexis Okuwa <wojonstech@gmail.com>`.
- More detail: `docs/dogfood/2026-08-04-integration.md` (user guide) and
  `docs/dogfood/diagnostics.md` (internals + root causes).
