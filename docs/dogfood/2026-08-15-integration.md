# Consensus Dogfood — Integration Report (2026-08-15, re-run)

**Verdict: 🟡 PROMISING-BUT-ROUGH (upgraded from the 2026-08-04 run — the
P0/P1 blockers of that run are all FIXED; two new P0s found on the MCP
surface)**

**Promise (unchanged):** "Your database is the runtime" — a database-native
agent harness (append-only memory, crash recovery, semantic retrieval,
circuit breakers, session isolation) with REST API, CLI, MCP, and a Go
client library.

**Reality:** The Aug-4 findings are genuinely fixed — append-only triggers
enforced, CLI pause/resume works, the circuit breaker trips, keyless smoke
exists, init bootstraps the memory bank, and `go test -short ./...` is 30/30
green in 142s with no API key. Every headline flow I re-ran worked. BUT the
re-run's deepest probes (MCP surface, OpenAPI contract) exposed new
problems: **the MCP endpoints are effectively unauthenticated** (P0) and
**MCP-created sessions use deterministic IDs and shared predictable session
keys** (P0). The REST/CLI/DB core is solid; the MCP bridge is not.

This is the record of an actual use run (2026-08-15), not a test summary.

---

## Status of the 2026-08-04 findings (all verified fixed this run)

| Aug-4 finding | Fix commit | Re-verified 2026-08-15 |
|---|---|---|
| DOGFOOD-001 — append-only triggers missing on fresh installs (P0) | `5d36aa7` | ✅ `UPDATE memory_events` → `Error: stepping, memory_events is append-only: UPDATE is not permitted (19)`; DELETE likewise; both triggers present in `sqlite_master` |
| DOGFOOD-002 — `session pause`/`resume` CLI broken (P1) | `b4f030e` | ✅ `session resume` works (paused→idle); `session pause` on an already-paused session returns actionable `CONFLICT: cannot pause session in status "paused"` |
| DOGFOOD-003 — circuit breaker never trips (P1) | `7ad8575` + `ff0306a` (C-GAP-011) | ✅ Live: no LLM key → 2 consecutive LLM errors → session auto-`paused`; `agent_circuit_breakers` row `(consecutive_errors, threshold=2, current_count=2, tripped_at set)` |
| DOGFOOD-004..007 (LLM error messages, last_error, empty-registry hints, MCP auth guidance) | `03339a5`/`cee65af` (tick #95) | ✅/partial — error surfaces are actionable; see NEW findings for MCP auth reality |

Also re-verified working: `consensus init` prints **"Memory bank: ready"**
(C-GAP-013/014), fresh clone contains every path AGENTS.md mandates
(C-GAP-017), Dockerfile binds 0.0.0.0 (C-GAP-016), `make smoke` passes
keyless in <60s (C-GAP-019), H3 shim has 18 tests (C-GAP-020),
INTEGRATION.md exists (C-GAP-021), README links dogfood reports (C-GAP-022).

---

## 1. What this run did (real use, not tests)

Scratch instance in `/tmp/dogfood-consensus/run` (fresh SQLite DB, port
18123, **no LLM API key** — same discipline as Aug-4), binary built from
HEAD `2164f38`:

1. `consensus init` → migrations, admin key, memory-bank bootstrap. ✅
2. `consensus serve` → health payload matches README byte-for-byte shape
   (37 tables, 22 migrations, schema_version 23). ✅
3. REST workflow: create session → send message → harness picked it up
   (booting → thinking, iteration 1) → memory events recorded. ✅
4. **Circuit breaker (live):** no LLM key → 2 consecutive errors → session
   `paused`, breaker row tripped. The "safety limit" promise now behaves. ✅
5. **Append-only ledger (live):** direct UPDATE and DELETE on
   `memory_events` rejected by DB triggers with clear messages. ✅
6. CLI sweep: `status`, `session list/show/pause/resume`, `memory list`,
   `config list`, `models list`, `approve list` — all functional, clean
   tables/JSON, sensible exit codes. ✅ (Aug-4's broken verbs are fixed.)
7. **Crash recovery:** `kill -9` the server mid-session → restart → session
   state, heartbeat and memory intact. Still true. ✅
8. **Go client library:** external consumer in `/tmp/dogfood-consensus/consumer`
   (go.mod `replace` → local repo), used ONLY `pkg/client` — compiled and
   ran first try: health/create/send/memory/get, 5/5. The library remains
   the best surface. ✅
9. **MCP SSE (documented happy path):** open `/mcp/sse` → endpoint event →
   initialize with `_meta.authorization` → tools/list (6 tools) →
   create_session → send_message. Works exactly as INTEGRATION.md shows. ✅
10. **MCP stdio:** JSON-RPC over stdin; tools/list works. ⚠️ (initialize
    auth broken — see DOGFOOD-106)
11. **H3 shim:** ran the INTEGRATION.md §2.3 example verbatim (in a scratch
    git worktree) — /v1/health advertises capabilities, /v1/process →
    `text`, /v1/result → `tool_call` → `end(task_complete)`, malformed body
    → 400 `INVALID_REQUEST` envelope. Exactly as documented. ✅
12. **OpenAPI:** `/openapi.json` + `/openapi.yaml` 200 when the server's
    CWD is the repo root; **404 from any other CWD** (and by construction
    in Docker, which ships no `specs/`). `/doc` is served by the opencode
    shim (not the REST API) with a hardcoded `servers: [{"url":
    "http://localhost:8090"}]`. ❌ (DOGFOOD-103)
13. **Test suite:** `env -u DEEPSEEK_API_KEY go test -short ./...` — 30/30
    packages green, ~142s wall. The Aug-4 "short suite hangs" is gone. ✅

Time-to-first-success: **~2 minutes** (init → serve → create session).

## 2. NEW findings (this run)

### DOGFOOD-101 (P0, security) — MCP surface is effectively unauthenticated

Auth is validated only inside `initialize` (`internal/mcp/auth.go`
`validateAuth` reads `_meta.authorization`). The SSE endpoint hands a
sessionId to **any** caller, and `tools/list` + `tools/call`
(`create_session`, `send_message`, `list_memory`, `get_session_status`,
`query_tool`) run without a key and without a prior initialize.
`checkWriteAccess`/`checkAdminScope` only restrict *scoped* keys — an
unauthenticated session has empty scope and passes.

Live proof: on a fresh SSE stream with **no key and no initialize**,
`tools/call list_memory` returned session `a04a8445-…`'s memory content;
`tools/call create_session` was attempted (blocked only by DOGFOOD-102's ID
collision). With the README-default Docker `-p 8090:8090`, this is a remote
agent-memory read/write hole.

### DOGFOOD-102 (P0, correctness + security) — deterministic MCP session IDs and keys

`internal/mcp/tools.go`:

```go
func generateUUID() string {           // bytes i*7 — always the SAME id
func generateShortID(n int) string {   // bytes i*13%256 — always the SAME key
```

- 2nd `tools/call create_session` → `UNIQUE constraint failed: sessions.id`
  (hit live). The tool works **exactly once per server lifetime**.
- Every MCP-created session gets the **same** session API key. Live proof:
  the key the tool issued, `cs_sk_000d1a2734414e5b6875828f9ca9b6c3d0ddeaf7
  04111e2b3845525f6c798693`, equals the deterministic constant — anyone can
  derive it. Session isolation (README headline) is defeated on the MCP
  surface: one known string opens every MCP session's memory. (REST-created
  sessions use proper random keys — the defect is MCP-only.)

### DOGFOOD-103 (P1) — OpenAPI contract only exists when CWD = repo root

See §1.12. README says the contract is available "on a running server";
reality: `/openapi.json` 404s for the Docker image and any non-repo-root
CWD, and `/doc` shows the opencode-shim spec, not the REST API, with a
hardcoded port.

### DOGFOOD-104 (P1) — the usage skill was stale

`skills/consensus-usage/SKILL.md` still told agents that append-only memory
is not enforced, pause/resume are broken, and the breaker never trips — all
fixed weeks ago. This run rewrites it (see artifacts).

### DOGFOOD-105/106/107 (P2/P3) — doc-level friction

- INTEGRATION.md §1.2 uses a placeholder sessionId that cannot work.
- `consensus mcp-stdio --api-key …` does not forward the key into
  initialize → "Authentication required" even with the flag (part of the
  DOGFOOD-101 family, but a distinct documented-flow break).
- H3 example hardcodes :8095 (collided with a live service on this host).

## 3. The working recipe (updated — the right way)

Same as Aug-4 §2, plus:

- **MCP clients must** send the key inside `initialize`'s
  `_meta.authorization` (SSE) — but see DOGFOOD-101: today it's optional.
  Do NOT rely on it for security until DOGFOOD-101 lands.
- **Do not use `tools/call create_session` more than once** until
  DOGFOOD-102 lands (2nd call collides). Use the REST API
  `POST /api/v1/sessions` for session creation (random IDs/keys).
- **OpenAPI:** run the server from the repo root, or generate and place
  `specs/openapi/bundled.yaml` next to the binary, until DOGFOOD-103 lands.
- H3 example: change the port if 8095 is taken.

## 4. What a new user gets in 2 minutes

```bash
consensus init --db-url sqlite:///tmp/cs/scratch.db   # save the cs_ak_… key
consensus serve --config <your.yaml>                  # port 18123
curl localhost:18123/api/v1/health                    # {"status":"ok",…}
# then: REST sessions/memory workflow, Go client, CLI — all solid.
```

Everything in the REST/CLI/DB core that the README promises works and
survives kill -9. The MCP bridge needs the two P0s fixed before it should
be exposed anywhere.
