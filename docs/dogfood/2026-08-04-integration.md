# Consensus Dogfood — Integration Report (2026-08-04)

**Verdict: 🟡 PROMISING-BUT-ROUGH**
**Promise:** "Your database is the runtime" — a database-native agent harness
(append-only memory, semantic retrieval, crash recovery, circuit breakers,
session isolation) with REST API, CLI, MCP, and a Go client library.
**Reality:** The core runtime is real and works (crash recovery verified
end-to-end; API/CLI/client all functional). But the #1 headline feature —
append-only memory enforced by DB triggers — is **silently absent on fresh
installs** (DOGFOOD-001), two documented CLI commands are broken
(DOGFOOD-002), and the circuit breaker doesn't trip (DOGFOOD-003).

This report is written for a future user or agent asking: *"does this thing
work, and how do I use it for real?"* It is the record of an actual use run,
not a test-suite summary.

---

## 1. What the run did (real use, not tests)

Scratch instance in `/tmp/dogfood-consensus/run` (fresh SQLite DB, port
18123, **no LLM API key** — deliberately, to test what a user gets before
they add a key):

1. `consensus init` — bootstrap: migrations applied, admin API key issued. ✅
2. `consensus serve` — server up in ~2s (harness heartbeat, compression
   worker, HITL manager, opencode shim, SQLite event poller all start). ✅
3. REST workflow: health → create session → send message → session state →
   memory list → context → iterations → tasks CRUD → approvals → billing →
   metrics → config. **All worked.** ✅
4. Crash recovery: created session 2 with a memory event, `kill -9` the
   server binary, restarted on the same DB → **session and memory event
   intact**. The core "data survives kill -9" claim is TRUE. ✅
5. Library integration: wrote a real external Go consumer
   (`/tmp/dogfood-consensus/consumer`) importing
   `github.com/wojons/consensus/pkg/client` via a `replace` directive —
   **compiled and ran first try**, 16/16 client call groups executed against
   the live server. ✅ (Details below.)
6. CLI sweep: status, session show/list/cancel/cost, memory list, approve
   list, config list, migrate version — work. `session pause`/`resume` —
   **broken**. ❌
7. MCP stdio: JSON-RPC `initialize` works (tools/resources/prompts
   capabilities) with `--api-key`. ✅
8. OpenAPI contract at `/openapi.json` + Swagger UI at `/doc`, Chronicle
   dashboard at `/chronicle/`. ✅

Time-to-first-success: **~2 minutes** from zero to a created session.

## 2. The working integration recipe (the right way)

### A. Run a scratch instance

```bash
mkdir -p /tmp/cs-demo && cd /tmp/cs-demo
cat > consensus.yaml <<'EOF'
server: { port: 18123, bootstrap_api_key_ttl: 2160h, admin_api_key_ttl: 2160h }
llm:
  default_model: deepseek-v4-flash
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: ""            # set DEEPSEEK_API_KEY or leave empty to test non-LLM paths
harness:
  heartbeat_interval_seconds: 3
  max_iterations: 3
  max_consecutive_errors: 2
  budget_limit_cents: 100
database: { url: "sqlite:///tmp/cs-demo/scratch.db", max_open_conns: 4 }
EOF
go run github.com/wojons/consensus/cmd/consensus init \
  --config consensus.yaml --db-url "sqlite:///tmp/cs-demo/scratch.db"
# → prints admin key cs_ak_...  (SAVE IT — printed once, stored hashed)
go run github.com/wojons/consensus/cmd/consensus serve --config consensus.yaml
curl http://127.0.0.1:18123/api/v1/health
```

### B. Drive it from a Go consumer (library pattern)

```go
// go.mod: require github.com/wojons/consensus v0.0.0
//         replace github.com/wojons/consensus => /path/to/consensus
c := client.NewClient("http://127.0.0.1:18123", os.Getenv("CS_API_KEY"))
h, _ := c.Health()
s, _ := c.CreateSession(client.CreateSessionRequest{
    AgentName: "consumer", Goal: "do a thing", ModelID: "deepseek-v4-flash"})
c.SendMessage(s.ID, client.SendMessageRequest{Content: "start", Type: "user_instruction"})
mem, _ := c.ListMemory(s.ID)   // []MemoryEventResponse
```

The typed client is genuinely pleasant: no hand-rolled HTTP, consistent
errors, matches the REST API 1:1. This was the "aha" — the library is the
best part of the project.

### C. The API at a glance

| Endpoint | Purpose | Works? |
|---|---|---|
| `GET /api/v1/health` | rich health (db, schema v, latency) | ✅ |
| `POST/GET /api/v1/sessions` | create/list | ✅ |
| `GET/PATCH/DELETE /api/v1/sessions/{id}` | get / pause-resume-cancel / delete | ⚠️ PATCH via CLI broken (DOGFOOD-002) |
| `POST /api/v1/sessions/{id}/message` | wake session | ✅ |
| `GET /api/v1/sessions/{id}/memory` | memory ledger | ✅ |
| `GET /api/v1/sessions/{id}/context` | active context view | ✅ |
| `GET /api/v1/sessions/{id}/iterations` | iteration commits | ✅ (empty until LLM success) |
| `POST/GET /api/v1/sessions/{id}/tasks` | task sub-list | ✅ |
| `GET /api/v1/sessions/{id}/approvals` | HITL queue | ✅ |
| `GET /api/v1/sessions/{id}/billing` | cost | ✅ |
| `GET /api/v1/tools`, `/skills` | registries | ⚠️ empty on fresh install (DOGFOOD-005) |
| `GET /api/v1/approvals` + review | HITL | ✅ |
| `GET /api/v1/config`, `/metrics` | ops | ✅ |
| `POST /api/v1/auth/keys` | key management | ✅ |
| `GET /openapi.json`, `/doc`, `/chronicle/` | contract + dashboards | ✅ |
| MCP stdio (`consensus mcp-stdio`) | MCP tools/resources/prompts | ✅ (auth via `--api-key`) |

Auth: `Authorization: Bearer cs_ak_...` (admin key from `init`).

## 3. Errors hit and their fixes (as a user would see them)

| # | What happened | Why | Workaround / fix |
|---|---|---|---|
| 1 | `UPDATE memory_events` succeeded — append-only violated | Migration 017 triggers silently dropped by `filterForSQLite` (DOGFOOD-001) | P0 task on board; until fixed, don't rely on the ledger being immutable |
| 2 | `session pause` → `unknown status action: "paused"` | CLI sends target state, server wants verb (DOGFOOD-002) | Use the REST API directly: `PATCH /api/v1/sessions/{id}` `{"status":"pause"}` |
| 3 | `llm: parse response (status 401): invalid character 'A'...` | Client JSON-parses the upstream error body (DOGFOOD-004) | Set a valid `DEEPSEEK_API_KEY`; the message is a bug, not your fault |
| 4 | Session flips to `failed` with 0 tokens, no reason in API | No error surfaced on the session object (DOGFOOD-004) | Check `audit_logs` table (it records `error_message`) |
| 5 | `tool list` → `(no results)` | Registry empty on fresh install (DOGFOOD-005) | Nothing to do; tools are only populated via registration |
| 6 | MCP: `missing _meta.authorization` / `Invalid API key` | Auth is read from `--api-key`/config, not `_meta` (DOGFOOD-007) | Pass `--api-key cs_ak_...` to `mcp-stdio` |
| 7 | `session logs` → `(no results)` | `iteration_commits` empty for failed runs (DOGFOOD-007) | Use `GET .../memory` to see what happened |

## 4. What a new user would need that isn't documented

- **The append-only trigger claim is false until DOGFOOD-001 is fixed** —
  the most important thing to know. Do not build compliance/audit workflows
  on it yet.
- How to register tools / install skills (the registry is exposed but empty
  and undocumented).
- How to do semantic retrieval as a user — there is **no** search endpoint;
  retrieval is internal to the harness and needs an embedding API.
- MCP auth mechanics (flag/config, not `_meta`).
- Which port is canonical (README 8090 vs repo config 8094).

## 5. What I'd tell the maintainer to fix FIRST (1 hour)

1. **DOGFOOD-001 (P0):** make `filterForSQLite` pass SQLite trigger files
   through untouched, add a post-init assertion that the triggers exist.
   This is a ~10-line fix that restores the project's headline promise.
2. **DOGFOOD-002 (P1):** map CLI status verbs to server action verbs.
3. **DOGFOOD-003 (P1):** wire harness errors into `circuit.go` so the
   advertised breaker actually trips.

Everything else is polish. The core is genuinely good.
