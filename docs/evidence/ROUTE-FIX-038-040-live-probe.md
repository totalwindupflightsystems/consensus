# ROUTE-FIX-038/039/040 live probe — POST /session/{sessionID}/permissions/{permissionID}, GET /session/{sessionID}/todo, POST /session/{sessionID}/unrevert

ch:trace row=ROUTE-FIX-038
  rows=ROUTE-FIX-038,ROUTE-FIX-039,ROUTE-FIX-040
  spec=specs/openapi/upstream/openapi-1.18.33.json#permission.respond
  spec=specs/openapi/upstream/openapi-1.18.33.json#session.todo
  spec=specs/openapi/upstream/openapi-1.18.33.json#session.unrevert
  wave=consensus-foreman-2026-10-03-07-49-18.json#task-1
  test=internal/shim/opencode/session_p1_routes_test.go (TestSessionPermissionRespond*, TestSessionTodo*, TestSessionUnrevert*)
  doc=docs/evidence/ROUTE-FIX-038-040-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-038-040-live-probe.md
  witness=none:unattended-worker-session — scripted curls against a locally built
    binary; every value below is copied verbatim from the probe transcript.

Date: 2026-10-03 (local, UTC-05)
Worktree: /home/kara/worktrees/consensus-ROUTE-FIX-038-040 (branch
`wt/ROUTE-FIX-038-040`, base 8891c1b); the probe ran with this change applied
to the worktree — the committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim routes were driven over HTTP. Every value below is
copied from a live HTTP response or a live `sqlite3` invocation — none is
hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary (pre-fix RED) | `/tmp/rf038040/consensus` (sha256 `072a96c242af1b32a3ccd30e5f9598b9d41d2b88215040178e5d40bb5198a9ab`), built from HEAD 8891c1b |
| binary (post-fix) | `/tmp/rf038040/consensus-fixed` (sha256 `50aa045586f6367c1f25e6db09a958a8168db661b42f632c922a1802444153ac`), built from the worktree diff |
| server | `127.0.0.1:18513`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/rf038040/probe.db?_journal_mode=WAL` (config `/tmp/rf038040/consensus.yaml`, `CONSENSUS_CONFIG`), created by `consensus init` (exit 0, `Migrations: applied`) |
| auth | bootstrap admin key (`cs_ak_…`, printed once by init); captured to a file and used from a curl header — the value is never printed in this artifact beyond the masked prefix |

Seeded for the probe (direct `sqlite3`, after the boot's migrations ran):

- `sessions sesprobe1` — status `idle`
- `sessions sesbusy1` — status `thinking` (the runtime's real mid-turn state;
  re-set to `thinking` before each 409 arm after the serve loop had flipped it
  via its own circuit-breaker path)
- `approval_requests per-11111111-…` — `status pending`, `session_id sesprobe1`
- `approval_requests per-22222222-…` — `status pending`, `session_id sesprobe1`

## 2. Pre-fix RED (binary at HEAD, before the change)

Every arm below answered the router catch-all before the fix:

| request | observed (pre-fix) |
|---|---|
| `GET /session/smissing/todo` | `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |
| `POST /session/sesprobe1/unrevert` | `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |
| `POST /session/sesprobe1/permissions/per-11111111-…` + `{"response":"once"}` | `404 {"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |

The declared success codes (200) were unreachable from outside the code — the
defect the board rows describe.

## 3. Declared contract, live (post-fix binary)

### 3a. session.todo — GET /session/{sessionID}/todo (declared 200 Todo[], 400, 404)

| arm | request | observed |
|---|---|---|
| **success (declared 200 array)** | `GET /session/sesprobe1/todo` | `200` body `[]` |
| busy session (todo declares no 409) | `GET /session/sesbusy1/todo` | `200` body `[]` |
| unknown session (declared 404) | `GET /session/smissing/todo` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| undeclared method (router default kept) | `POST /session/sesprobe1/todo` | `404` `{"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |

### 3b. session.unrevert — POST /session/{sessionID}/unrevert (declared 200 Session, 400, 404, 409)

The operation declares NO requestBody, so no body is sent on any arm.

| arm | request | observed |
|---|---|---|
| **success (declared 200 Session)** | `POST /session/sesprobe1/unrevert` | `200` `{"completedAt":null,"createdAt":"2026-10-03T08:00:00Z","goal":"g","id":"sesprobe1","iteration":0,"model":"m","status":"idle","title":"worker","tokensIn":0,"tokensOut":0}` — the row AS IT STANDS (no restore engine exists; nothing was ever reverted) |
| **mid-turn (declared 409)** | `POST /session/sesbusy1/unrevert` | `409` `{"_tag":"SessionBusyError","message":"session is mid-turn; the operation requires an idle session","sessionID":"sesbusy1"}` |
| unknown session (declared 404) | `POST /session/smissing/unrevert` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| undeclared method (router default kept) | `GET /session/sesprobe1/unrevert` | `404` `{"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |

The declared 400 arm: the operation declares no requestBody and its only
path parameter arrives from the URL, so there is no client-supplied value the
handler can find ill-formed — the sibling sessionUnshare (ROUTE-FIX-015, same
declared vocabulary minus 409) has the identical shape, and the test suite
covers the arm at the handler level the same way.

### 3c. permission.respond — POST /session/{sessionID}/permissions/{permissionID} (declared 200 boolean, 400, 404)

| arm | request | observed |
|---|---|---|
| **success (declared 200 boolean)** | `POST /session/sesprobe1/permissions/per-11111111-…` + `{"response":"reject"}` | `200` body `true` |
| row after the 200 | `sqlite3 … WHERE id='per-11111111-…'` | `rejected\|denied by permission.respond\|opencode-shim\|1` — the REAL approval_requests columns (status/review_notes/reviewer_id/reviewed_at), no decision_reason column touched |
| **success (declared 200 boolean, `once`)** | `POST /session/sesprobe1/permissions/per-22222222-…` + `{"response":"once"}` | `200` body `true`; row after: `approved\|granted once` |
| already resolved (declared 404) | second `POST …/per-11111111-…` + `{"response":"once"}` | `404` `{"error":{"code":"NOT_FOUND","message":"permission is not pending"}}` |
| permission of another session (declared 404) | `POST /session/sesbusy1/permissions/per-22222222-…` + `{"response":"once"}` | `404` `{"error":{"code":"NOT_FOUND","message":"permission not found in session"}}` (row untouched) |
| unknown session (declared 404) | `POST /session/smissing/permissions/per-11111111-…` | `404` `{"error":{"code":"NOT_FOUND","message":"session not found"}}` |
| unknown permission (declared 404) | `POST /session/sesprobe1/permissions/per-99999999-…` | `404` `{"error":{"code":"NOT_FOUND","message":"permission not found"}}` |
| value outside the declared enum (declared 400) | `{"response":"sometimes"}` | `400` `{"error":{"code":"INVALID_REQUEST","message":"required field \"response\" must be one of \"once\", \"always\", \"reject\""}}` |
| missing required response (declared 400) | `{}` | `400` (same message) |
| malformed body (declared 400) | `{"response":` | `400` `{"error":{"code":"INVALID_REQUEST","message":"malformed request body"}}` |
| absent body (declared 400) | no body at all | `400` `{"error":{"code":"INVALID_REQUEST","message":"request body required: the declared schema carries required fields"}}` |
| ill-shaped permissionID, declared pattern ^per (declared 400) | `POST …/permissions/not-a-per` | `400` `{"error":{"code":"INVALID_REQUEST","message":"permissionID must match the declared pattern ^per"}}` |
| undeclared method (router default kept) | `GET /session/sesprobe1/permissions/per-11111111-…` | `404` `{"error":{"code":"NOT_FOUND","message":"endpoint not found"}}` |

## 4. Which declared codes are reachable

- `session.todo`: **200** (empty truthful list, live), **404** (unknown
  session, live). The declared 400 has no reachable client input: the
  operation's only parameter is the path sessionID (the optional
  directory/workspace query parameters are accepted and ignored — the shim
  treats the server as a singleton instance), so there is no body or enum to
  get wrong; the sibling sessionUnshare precedent (ROUTE-FIX-015) has the
  same declared-400 shape.
- `session.unrevert`: **200** (Session as it stands, live), **404** (unknown
  session, live), **409** (mid-turn SessionBusyError with the typed `_tag`
  body, live). Same 400 note as todo (no requestBody declared).
- `permission.respond`: **200** (truthful `true`, live, with the row
  resolution verified in the real columns), **400** (malformed/missing/enum
  body + ill-shaped permissionID, live), **404** (unknown session, unknown
  permission, foreign-session permission, non-pending permission, live).

Every declared response code for each operation is reachable on the live
surface; the `endpoint not found` router-default answers remain exactly as
they were for UNDECLARED methods (no stub-list entry was removed).

## 5. Neighbours unchanged

- `GET /session/status` still answers its own aggregate; the sibling P1
  routes (revert/share/shell/summarize/init/fork) were not modified.
- The 501 stub list (`prompt_async, shell, command, share, summarize, init,
  fork, revert`) is intact — undeclared methods on those sub-paths keep the
  pre-existing 501; undeclared methods on the three new sub-paths keep the
  router default 404 (live above).

## 6. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` registers the
three routes and `scripts/compare-opencode-declared-served.py` was re-run to
regenerate `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`:

| measure | before | after |
|---|---|---|
| `covered_operations` | 49 | **52** (permission.respond, session.todo, session.unrevert enter) |
| `narrowed_error_contract_only` | 9 | **6** (the three session-scoped rows above leave; `permission.reply`, `project.update`, `question.reject`, `question.reply`, `part.delete`, `tui.selectSession` stay narrowed) |
| `served_route_entries` | 98 | **101** |
| `drift_declared_not_served` | 128 (NOT-SERVED 103, OUTCOME-MISMATCH 25) | 128 (unchanged — no new drift row) |
| `drift_served_not_declared` | 17 (PATH-ABSENT 16, METHOD-ABSENT 1) | 17 (unchanged) |

`SHIM-NARROWED-NNN` ids are positional (re-assigned by enumeration order on
every regeneration), so the flip is: the three rows that carried
`id: SHIM-NARROWED-008` (permission.respond), `-009` (session.todo) and
`-010` (session.unrevert) in the pre-change artifact are now in `covered`;
the narrowed list simply shrinks to six and the remaining ids shift down.
