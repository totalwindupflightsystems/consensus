# ROUTE-FIX-039 live probe — GET /session/{sessionID}/todo

ch:trace row=ROUTE-FIX-039
  spec=specs/openapi/upstream/openapi-1.18.33.json#session.todo
  test=internal/shim/opencode/session_todo_test.go (TestSessionTodo*)
  doc=docs/evidence/ROUTE-FIX-039-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-039-live-probe.md
  witness=none:self-verified-in-worktree — scripted curls against a locally built
    binary; every value below is copied verbatim from the probe transcript.
    verdict=/commit= are deliberately absent in-tree: a verdict binds to a commit
    that already exists, so it is recorded by the commit footer / foreman
    verification, never self-referenced by the change it judges.

Date: 2026-10-03 (local, UTC-05) / 2026-10-03T15:48Z
Worktree: `/home/kara/worktrees/consensus-ROUTE-FIX-039` (branch `wt/ROUTE-FIX-039`,
base `435989adc5b33b9f44e1a1e26595971a52cae8ee`); the probe ran with this change
applied to the worktree — the committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database** and
the opencode shim route was driven over HTTP. Every value below is copied from a
live HTTP response or a live `sqlite3` invocation — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/t039-consensus` (sha256 `99cb1789eb66373a3b05bbfbeb49e31cd7711ce775cf813668e981b6f2e13cc8`) |
| build | `go build -o /tmp/t039-consensus ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/worktrees/consensus-ROUTE-FIX-039` @ `wt/ROUTE-FIX-039`, base `435989a` |
| server | `127.0.0.1:18611`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/t039-run/t039.db`, created by the boot's auto-migrate (38 tables) |
| sqlite3 | 3.53.4 (used only to seed and to read back) |
| auth | bootstrap admin key (Bearer); captured from the boot's stdout into a file and used from a curl header — the value is never printed or recorded |

Seeded for the probe (direct `sqlite3`, after the boot's migrations ran):

- `sessions sesprobe1` — status `idle`
- `sessions sesempty` — status `idle` (no task rows)
- `sessions 1a2b3c4d-0000-4000-8000-000000000000` — status `idle` (the UUID shape
  `internal/api/sessions.go newUUID` actually mints)
- 7 `tasks` rows in `sesprobe1`, one per declared mapping arm:

```
seeded: sesprobe1/pending/1 wire the todo route
seeded: sesprobe1/claimed/3 claimed arm
seeded: sesprobe1/in_progress/4 in progress arm
seeded: sesprobe1/reviewed/7 reviewed arm
seeded: sesprobe1/published/8 published arm
seeded: sesprobe1/failed/10 failed arm
seeded: sesprobe1/cancelled/5 cancelled arm
```

## 2. Declared contract, live

`<code> <body>` below is curl's HTTP status and the response body, verbatim.

| arm | request | observed |
|---|---|---|
| known session with tasks (declared 200) | `GET /session/sesprobe1/todo` | `200 [{"content":"wire the todo route","priority":"high","status":"pending"},{"content":"claimed arm","priority":"high","status":"in_progress"},{"content":"in progress arm","priority":"medium","status":"in_progress"},{"content":"reviewed arm","priority":"medium","status":"completed"},{"content":"published arm","priority":"low","status":"completed"},{"content":"failed arm","priority":"low","status":"cancelled"},{"content":"cancelled arm","priority":"medium","status":"cancelled"}]` |
| session with no tasks (declared 200) | `GET /session/sesempty/todo` | `200 []` — an empty **array**, never `null` |
| UUID session id (runtime shape, no tasks) | `GET /session/1a2b3c4d-0000-4000-8000-000000000000/todo` | `200 []` — the declared `^ses` pattern does **not** gate real session ids |
| declared `?directory=` selector | `GET /session/sesprobe1/todo?directory=/tmp` | `200` — the same 7-entry array (the selector is validated, not narrowing) |
| declared `?workspace=` selector | `GET /session/sesprobe1/todo?workspace=default` | `200` — the same 7-entry array |
| unknown session (declared 404) | `GET /session/sesmissing/todo` | `404 {"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| blank `?directory=` (declared 400) | `GET /session/sesprobe1/todo?directory=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"directory\" must not be blank"}}` |
| blank `?workspace=` (declared 400) | `GET /session/sesprobe1/todo?workspace=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"workspace\" must not be blank"}}` |
| whitespace-only `?directory=` (declared 400) | `GET /session/sesprobe1/todo?directory=%20` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"directory\" must not be blank"}}` |
| blank selector on an **unknown** session | `GET /session/sesmissing/todo?directory=` | `400` (the malformed request is refused before the store read — validation order) |

Neighbours and auth (must be unchanged by this route):

| probe | observed |
|---|---|
| `POST /session/sesprobe1/todo` (undeclared method) | `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` (router default, pre-existing — 405 is not in the declared set) |
| `GET /session/sesprobe1` | `200 {"completedAt":null,"createdAt":"2026-10-03T00:00:00Z","goal":"probe","id":"sesprobe1","iteration":0,"model":"m","status":"idle","title":"probe","tokensIn":0,"tokensOut":0}` |
| `GET /session/sesprobe1/diff` (ROUTE-FIX-010 sibling) | `200 [ … SnapshotFileDiff entries … ]` |
| `GET /session/sesprobe1/message` | `200 []` |
| `GET /session/sesprobe1/todo` with **no** Authorization header | `401 {"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` |
| `GET /session/sesprobe1/todo` after every error arm | `200` — the same 7-entry array; `SELECT 'tasks rows total: '||count(*) FROM tasks` = `tasks rows total: 7` (no-effect proof: no arm mutated the ledger) |

## 3. Which declared codes are reachable

`session.todo` declares `200` (Array(Todo)), `400` (BadRequest |
InvalidRequestError) and `404` (NotFoundError).

- **200 is reachable** — a known session answers the declared array, translated
  from its `tasks` rows, in the declared field vocabulary (`content`, `status`,
  `priority`, nothing else); a session with no rows answers `[]`, not `null`.
- **400 is reachable** — a present-but-blank (or whitespace-only) declared query
  selector `directory`/`workspace` (live above). The operation declares no
  requestBody, so the declared query selectors are what keeps this arm
  reachable; validation runs before the session lookup.
- **404 is reachable** — an unknown session id (live above), via the shared
  `p1ResolveSession` read.
- **No 501 is served** — the pre-fix answer to every request was the router
  catch-all's untyped 404; the route no longer answers 501 for any arm (the
  in-repo battery `TestSessionTodoNeverAnswers501` pins this, and no POST arm is
  routed to a stub).

## 4. Why the declared 200 is a translation, not a fabrication

The runtime keeps a genuine per-session action-item ledger — `tasks`
(migrations 001/009): `session_id`, `title`, `description`, `status` ∈
{pending, claimed, in_progress, reviewed, published, failed, cancelled},
`priority` ∈ 1..10 — which is exactly the "tasks and action items" the upstream
operation describes. The live 200 above shows each mapping arm:

| tasks row | declared Todo |
|---|---|
| `pending` / priority 1 | `{"status":"pending","priority":"high"}` |
| `claimed` / priority 3 | `{"status":"in_progress","priority":"high"}` |
| `in_progress` / priority 4 | `{"status":"in_progress","priority":"medium"}` |
| `reviewed` / priority 7 | `{"status":"completed","priority":"medium"}` |
| `published` / priority 8 | `{"status":"completed","priority":"low"}` |
| `failed` / priority 10 | `{"status":"cancelled","priority":"low"}` |
| `cancelled` / priority 5 | `{"status":"cancelled","priority":"medium"}` |

Orders of magnitude aside, nothing here is invented: every field comes from a row
the database actually holds, and a session with no rows gets the truthful `[]`
rather than a synthesized entry.

## 5. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` registers the route
(`outcome: 200`, with the per-arm evidence) and
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is regenerated
by `scripts/compare-opencode-declared-served.py` (the pre-change artifact was
byte-reproducible from the pre-change inputs, so the regenerated diff is exactly
this route's flip):

| measure | before | after |
|---|---|---|
| `covered_operations` | 49 | **50** (`session.todo` enters) |
| `narrowed_error_contract_only` | 9 | **8** (`session.todo` leaves; `SHIM-NARROWED-007` in the pre-change artifact's positional ids, `SHIM-NARROWED-009` in the 2026-09-29 baseline report) |
| `served_route_entries` | 98 | **99** (the GET route) |
| `drift_declared_not_served` | 128 (NOT-SERVED 103, OUTCOME-MISMATCH 25) | 128 (unchanged — no new drift row) |
| `drift_served_not_declared` | 17 (PATH-ABSENT 16, METHOD-ABSENT 1) | 17 (unchanged) |
