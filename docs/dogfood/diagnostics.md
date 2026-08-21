# Consensus Diagnostics Trail (2026-08-04 dogfood run)

This is the "how it's built, why, what breaks, the right way" record —
explained lessons, not raw logs. It complements
`docs/dogfood/2026-08-04-integration.md` (user-facing) by explaining the
internals a future agent will touch when fixing the DOGFOOD-* tasks.

## 1. How the system is built

**Stack:** Go 1.26 (module `github.com/wojons/consensus`), chi router,
Cobra CLI, `modernc.org/sqlite` (pure-Go, CGO-free) or pgx/Postgres,
DuckDB-backed foreman board under `.coding-hermes/board/`.

**Layers:**
- `cmd/consensus/main.go` — wiring: `init` (bootstrap + admin key),
  `serve` (API + harness + MCP + shims), `mcp-stdio`, `migrate`.
- `internal/api/` — REST surface (`/api/v1/*`), SSE at `/api/v1/events`,
  OpenAPI at `/openapi.json` (note: NOT under `/api/v1/`).
- `internal/harness/` — the agent loop: heartbeat picks up active sessions,
  planning loop calls the LLM, tools execute, iterations commit.
- `internal/migrate/` — embedded SQL migrations (`go:embed migrations/*`),
  auto-applied on startup; `schema_versions` table tracks them.
- `internal/db/` — driver wrapper (SQLite/Postgres) + `filterForSQLite`
  PG→SQLite translation layer.
- `internal/cli/` — management commands that are REST clients (SPEC-016).
- `pkg/client/` — the public typed Go client (the best-integrated surface).
- `internal/compression`, `memory`, `billing`, `hitl`, `tools`, `webhook`,
  `chronicle` — worker subsystems.

**The migration runner (`internal/migrate/migrate.go`):**
1. `LoadMigrations` reads embedded `NNN_name.sql` files, skipping
   `*_postgres_*` on SQLite and `*_sqlite_*` on Postgres.
2. `filterForSQLite` strips PG-only constructs (functions, policies,
   `USING` indexes, goose Down sections, type casts) and translates types.
3. `splitStatements` splits on lines ending in `;`, executes each via
   `database.Exec`, then records the migration in `schema_versions`.
4. `Up` refuses to run when drift is detected; `repairTrustLevel` exists
   as a precedent for silent repair of past migration bugs.

## 2. The append-only bug (DOGFOOD-001) — full root cause

**Symptom:** fresh `init` + `serve`; `schema_versions` says v17 applied;
`sqlite_master` has **zero triggers**; `UPDATE memory_events` succeeds.

**Mechanism:** `filterForSQLite` decides whether a `CREATE TRIGGER` is
SQLite-native by checking `strings.Contains(upper, " BEGIN ")` **on the
first line only**. The first line of migration 017's triggers is:

```sql
CREATE TRIGGER IF NOT EXISTS trg_memory_events_append_only_update
```

No `BEGIN` there → the code enters `mTrigger` skip mode, which **drops every
line** until one ends with `;` (and drops that one too via `continue`). The
entire trigger statement vanishes. `splitStatements` then finds nothing to
execute; `Up` records version 17 as applied. Silent success, no triggers.

**Why the existing guard is wrong:** the keep-check must see the trigger
*body* (`FOR EACH ROW BEGIN ... END;`), which arrives on later lines. A
multi-line trigger can never be classified from its header line.

**Fix directions (for the foreman):**
- Best: in `filterForSQLite`, pass `*_sqlite_*.sql` files through with no
  filtering at all (the naming convention already declares them SQLite-only).
- Or: buffer trigger statements and only skip those whose bodies use
  `EXECUTE FUNCTION` / `$$` (PG style).
- Add a regression test: after `AutoMigrate` on SQLite, assert
  `sqlite_master` contains both triggers and that UPDATE/DELETE raise.
- Consider a `repairTriggers` startup check like `repairTrustLevel` for
  already-initialized DBs (the repo's own `dev.db` is likely affected too —
  check `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'`).

**Related:** migration 018 (Postgres append-only) is skipped on SQLite and
its own comment says Postgres enforcement is "a separate task" — so the
append-only promise is unimplemented on BOTH backends despite
`schema_versions` recording v17. The demo test that shows "UPDATE attempt →
500" runs against a *manually triggered* database (or predates the filter
bug) — it does not reflect a fresh install.

## 3. The CLI pause/resume bug (DOGFOOD-002) — root cause

`internal/cli/session.go`:
```go
client.UpdateSession(args[0], map[string]any{"status": "paused"})  // pause cmd
client.UpdateSession(args[0], map[string]any{"status": "idle"})    // resume cmd
```
`internal/api/service.go` `UpdateSession` switch:
```go
case "pause": ... case "resume": ... case "cancel": ...
```
The CLI sends the *target state*; the server wants the *action verb*. The
server's error `unknown status action: "paused" (use pause, resume, or
cancel)` is actually correct and helpful — the CLI is at fault. Fix in the
CLI (send `pause`/`resume`), and add a CLI-level test that hits a live
server (the existing client tests use httptest, which is why this slipped).

## 4. Circuit breaker not tripping (DOGFOOD-003) — observations

`internal/harness/circuit.go` writes `agent_circuit_breakers` rows, but the
real failure path (`planning: LLM call failed`, see
`internal/harness/planning.go`) produced `status=failed` sessions with an
empty breaker table. Either the counter is only consulted in paths that
didn't run (interactive vs autonomous), or the error return short-circuits
before `circuit.go` is reached. The README/demo promise ("2 consecutive
errors → session pauses") needs an end-to-end test with a failing provider.

## 5. LLM error path (DOGFOOD-004)

`internal/llm` parses the response body as JSON regardless of HTTP status.
DeepSeek's 401 body is not JSON-shaped at the parse point → the misleading
"invalid character 'A' looking for beginning of value". The audit trail
(`audit_logs.error_message`) captures the real error — the API response and
logs should surface it instead of the parse error.

## 6. The right way to develop against this repo

- **Guards:** GitReins (`gitreins guard`) gates commits: secrets + build
  BLOCK, vet WARNS, tests run for changed packages. Docs-only changes pass
  cleanly. All agent commits need the `Co-authored-by` trailer used by the
  foreman (`Co-authored-by: Alexis Okuwa <wojonstech@gmail.com>`).
- **Branch:** `master` (not `main`). Never push; the foreman's convention is
  local commits + unpushed backlog.
- **Board:** `.coding-hermes/board/board.db` (DuckDB) is canonical;
  `tasks.parquet`/`events.parquet` are exports. Task rows: `id`, `title`
  (long-form detail lives in the title), `status` (`pending`/`complete`),
  `priority`, `complexity`, `capability_tags`. Fixtures (`NEVER-DONE`,
  `E2E-001`) are perpetual. Python duckdb module available in system python
  and `~/.hermes/venvs/board`.
- **Testing a change:** `go test ./internal/<pkg>/ -short`; the full demo
  needs `DEEPSEEK_API_KEY` and **skips silently without it** — don't trust a
  green run as "demo verified".

## 7. Errors encountered during this run (mine, explained)

| Error | Explanation |
|---|---|
| `file is not a database` (sqlite3 on board.db) | Board is DuckDB, not SQLite — use duckdb |
| guard `embedded null byte` on `./consensus --help` | The Hermes cron lifecycle guard reads referenced files as scripts; compiled binaries crash it — use `go run` instead |
| `consensus init` prints `Server URL: http://127.0.0.1:8094` | Hardcoded default from repo `consensus.yaml`; `--config` port wins at serve time |
| MCP `missing _meta.authorization` then `Invalid API key` | MCP auth comes from `--api-key`/config only; `_meta` is ignored; messages are misleading (DOGFOOD-007) |

---

# 2026-08-15 re-run addendum (dogfood #2)

## How the system is built (updated)

- **Board is now JSONL, not DuckDB.** `.coding-hermes/board/tasks.jsonl` +
  `events.jsonl` are the canonical, git-tracked board (JSONL-NORM-001,
  `e362ac5`); `board.db` is a local cache the foreman heals with
  `sync_tasks_jsonl_to_db.py`. Write new tasks as JSONL rows in
  `tasks.jsonl` (schema: `id, title, status, priority, complexity,
  depends_on, blocks, primary_model, primary_provider, capability_tags,
  worker_status, created_at, …`). The Aug-4 diagnostics section that calls
  DuckDB canonical is historical — trust the JSONL.
- **MCP auth architecture (and its flaw):** the API key is validated ONLY
  at `initialize` via `params._meta.authorization` (`internal/mcp/auth.go`),
  then scopes ride on the in-memory `mcpSession`. Nothing requires an
  authenticated initialize before tool calls — sessions are minted by the
  SSE stream with no credentials. `checkWriteAccess`/`checkAdminScope` only
  constrain *scoped* keys; empty scope passes. This is DOGFOOD-101.
- **MCP session ID/key generation is deterministic** (`generateUUID` =
  `byte(i*7)`, `generateShortID` = `byte(i*13%256)` in
  `internal/mcp/tools.go`). Every MCP-created session shares one session id
  (2nd create collides — UNIQUE constraint) and one predictable session
  key. This is DOGFOOD-102.
- **OpenAPI spec resolution is CWD-dependent** (`internal/api/openapi.go`
  `resolveSpecPath()`: `specs/openapi/bundled.yaml` etc. relative to CWD;
  no embed). `/doc` is shadowed by the opencode shim's Swagger UI
  (`internal/shim/opencode/server.go` mounts `/doc` with a hardcoded
  `servers: [{url: "http://localhost:8090"}]`). DOGFOOD-103.
- **Keyless smoke exists:** `make smoke` = `go test -run Smoke ./demo/`
  (C-GAP-019, `ac8d36a`) — mock-LLM, scratch sqlite, real server binary,
  45s deadline. `go test -short ./...` is 36/36 green keyless (~142s).

## Errors I hit this run (and the right way)

| Error | Explanation / right way |
|---|---|
| `session not found` on MCP POST | I used INTEGRATION.md's placeholder sessionId instead of the one from my own `/mcp/sse` endpoint event. Use YOUR sessionId (DOGFOOD-105 filed to fix the doc). |
| `UNIQUE constraint failed: sessions.id` on 2nd MCP create_session | Deterministic generateUUID — every MCP session has the same id (DOGFOOD-102). Create sessions via REST until fixed. |
| MCP `initialize` → `Authentication required` with `--api-key` | The stdio CLI never injects the key into `_meta.authorization` (DOGFOOD-106); SSE clients must put the key there themselves. |
| `Authentication required` **not** returned for tools/list | By design today — auth only checked at initialize (DOGFOOD-101). Don't assume an authenticated surface. |
| `bind: address already in use` (H3 example, :8095) | Port taken by another service on this host; doc example hardcodes it (DOGFOOD-107). |
| `/openapi.json` 404 while `/doc` 200 | CWD had no `specs/openapi/`; and `/doc` is the opencode-shim UI, not the REST UI (DOGFOOD-103). |
| `pgrep -f 'consensus serve'` matched my own shell | The command line contains the pattern; match the binary path instead. |
| `test -f docs/dogfood/` said missing in a fresh clone | `test -f` on a directory is false — use `test -d`. The files are tracked. |

## The right way (summary)

1. Scratch instance: `consensus init --config …` + `consensus serve` with an
   explicit `server.port` (8090 is taken by the sidecar on this host) and
   `database.url` to a scratch sqlite file; no LLM key needed for the
   REST/CLI/DB surface.
2. Session creation: REST `POST /api/v1/sessions` (random IDs/keys). MCP
   create_session: once, max — until DOGFOOD-102.
3. Ledger + breaker are now real: `UPDATE memory_events` fails; a dead LLM
   pauses the session after `max_consecutive_errors`.
4. OpenAPI: run from repo root or vendor `specs/openapi/bundled.yaml` —
   until DOGFOOD-103.
5. H3: mount `internal/shim/h3` yourself (still a library, not wired into
   `consensus serve` — INTEGRATION.md §2.3, honest about it).
