# ROUTE-ADD-061 — POST /api/session/{sessionID}/wait (v2.session.wait) live probe

ch:trace row=ROUTE-ADD-061
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.wait
  test=internal/shim/opencode/api_session_wait_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-061-live-probe.md
  witness=live:127.0.0.1:18096,127.0.0.1:18097,127.0.0.1:18098
  verdict=pending:foreman-tier2

Date: 2026-11-03 (~04:41–04:44Z). Change tree at `5ae5140` (= origin/main at
dispatch, plus this uncommitted change).

**Source item:** the board row cites `SHIM-DRIFT-057`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration
(this regeneration moved 18 remaining drift rows down by one) — so this probe,
the served-surface row and the tests are keyed on **operationId `v2.session.wait`**
+ method + path, never on a drift id. In the pre-change artifact the operation
was row `SHIM-DRIFT-001`.

## Run identity

| Field | Value |
|---|---|
| Pre-change (RED) binary | `/tmp/consensus-ra061-red`, SHA-256 `21a1923bb39f3da16e1c228b322e9db8253f81a374873bd845cdbecac01d1a81` (built from a detached worktree of `5ae5140`, i.e. the route UNREGISTERED) |
| Post-change binary | `/tmp/consensus-ra061-live`, SHA-256 `7d9491ef0ecc131eb4678c9fec82d569186fbc6d9afa8b39e711c900f054a39a` (this change tree) |
| Listeners | `127.0.0.1:18096` (RED) / `127.0.0.1:18097` (post-change) / `127.0.0.1:18098` (wait-budget arm, `CONSENSUS_MESSAGE_RESPONSE_TIMEOUT=3s`) |
| Backend | scratch SQLite (`/tmp/ra061-red/scratch.db`, `/tmp/ra061-live/scratch.db`, `/tmp/ra061-budget/scratch.db`), migrations applied by the server on startup |
| LLM | `provider: mock` + `CONSENSUS_MOCK_LLM=1` — no provider credentials involved |
| Auth | a `api_keys` row seeded per scratch DB (`key_prefix ra061-li`, admin scope, sha256 of the probe token); the token never entered the repository |
| Session fixtures | `9b1c4d2e-…abcde1` status `idle`, `9b1c4d2e-…abcde2` status `thinking` (live instance), `9b1c4d2e-…abcde9` status `thinking`→`waiting_sub` (budget instance) |

## RED proof (pre-change build, port 18096)

```
## RED: auth POST /api/session/{id}/wait
404 page not found

HTTP 404

## control: auth GET /api/session/active
{"data":{}}
HTTP 200

## control: auth POST /api/session/{id}/compact (pre-existing sibling)
HTTP 204
```

The 404 is net/http's default page, i.e. no route existed — indistinguishable
from "no such opencode operation", exactly the drift artifact's NOT-SERVED class.
Both controls prove the shim was mounted and the auth wall armed, so the 404 is
the missing route and not a dead instance.

## Post-change probe transcript (port 18097)

```
## unauth POST /api/session/{id}/wait            (declared 401 UnauthorizedError)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth POST /api/session/{id}/wait  IDLE        (declared 204, empty body)

HTTP 204

## auth POST /api/session/{id}/wait  UNKNOWN     (declared 404 SessionNotFoundError)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-fffffffffffe\" in this runtime: nothing to wait for","sessionID":"00000000-0000-4000-8000-fffffffffffe"}

HTTP 404

## auth POST /api/session/%20/wait               (declared 400 kind Path field sessionID)
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Path","message":"sessionID path parameter must not be blank"}
HTTP 400

## auth GET /api/session/{id}/wait               (undeclared method -> 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
HTTP 405

## auth POST with an undeclared JSON body        (ignored -> 204; the document declares no requestBody)
HTTP 204

## auth POST /api/session/{id}/wait?x=1          (undeclared query ignored -> 204)
HTTP 204

## auth POST /api/session/{id}/wait  BUSY        (status=thinking -> the wait blocks)
-- flipping 9b1c4d2e-0000-4000-8000-abcdefabcde2 status thinking -> idle at +3s
   status line: HTTP 204
   body bytes: 0
   elapsed: 3.04s (waited for the transition, did NOT answer 204 up front)

## auth POST busy session DELETED mid-wait       (declared 404, re-resolved every poll)
-- deleting 9b1c4d2e-0000-4000-8000-abcdefabcde2 at +3s
   status line: HTTP 404
   body: {"_tag":"SessionNotFoundError","message":"no session \"9b1c4d2e-0000-4000-8000-abcdefabcde2\" in this runtime: nothing to wait for","sessionID":"9b1c4d2e-0000-4000-8000-abcdefabcde2"}

## control: auth POST /api/session/{id}/compact  (ROUTE-ADD-042 sibling unchanged)
HTTP 204

## control: auth GET /api/session/{id}           (ROUTE-ADD-040 sibling unchanged)
{"data":{"agent_name":"build","goal":"probe fixture","id":"9b1c4d2e-0000-4000-8000-abcdefabcde1","status":"idle"}}
HTTP 200

## control: auth POST /api/session/{id}/wait/x   (deeper path stays unregistered, no catch-all)
404 page not found

HTTP 404
```

The BUSY row is the load-bearing one: the declared success was NOT answered up
front for a session whose agent loop was in `thinking`. The request stayed open
3.04s and answered the empty 204 only after the row left the working set — the
serve log carries the matching line:

```
level=INFO msg="opencode-shim: session wait is blocking on a running agent loop" \
  session_id=9b1c4d2e-0000-4000-8000-abcdefabcde2 status=thinking budget=1m30s
```

The two 204s with a body and with a query string are the "cannot refuse input the
document does not declare" arm: the pinned operation declares no requestBody and
no query parameter, so those inputs are ignored rather than answered with an
invented 400.

## Declared 503 arm (a) — unreadable sessions store (port 18097)

A store-read failure is injected by renaming the `sessions` table, which makes
the handler's read fail with a non-"no rows" error, then restored:

```
## healthy store: IDLE session (declared 204)
HTTP 204

-- injecting: ALTER TABLE sessions RENAME TO sessions_unreadable_probe
## unreadable store (sessions table gone): declared 503 ServiceUnavailableError
{"_tag":"ServiceUnavailableError","message":"failed to resolve session: sqlite: query row: SQL logic error: no such table: sessions (1)","service":"sessions"}
HTTP 503

-- restoring: ALTER TABLE sessions_unreadable_probe RENAME TO sessions
## restored store: IDLE session again (declared 204)
HTTP 204
```

This is why the route reports a store failure as `503` and not as the `400` its
`switchAgent` / `switchModel` / `history` siblings answer: `v2.session.wait`
DECLARES a 5xx arm, and absence is never claimed from a store that could not be
read. The restore proves the 503 came from the injected failure.

## Declared 503 arm (b) — the wait expired before the loop went idle (port 18098)

A third instance ran with `CONSENSUS_MESSAGE_RESPONSE_TIMEOUT=3s` (the BUG-013
deployment knob this route reuses as its wait budget) against a session parked in
`waiting_sub` — a working status the harness heartbeat does not settle:

```
{"_tag":"ServiceUnavailableError","message":"session \"9b1c4d2e-0000-4000-8000-abcdefabcde9\" agent loop is still \"waiting_sub\" after 3s: no idle state was observed, try again","service":"sessions"}
HTTP 503 elapsed=3.001717s
```

Match the budget to the reserved status: the same session answered `204` in
0.5 ms while the harness was still settling it (see scope honesty below), and the
`503` only after the budget expired with the loop genuinely still working. The
message names the status last read, so a client can tell a long turn from a dead
one instead of receiving an invented `204`.

## Harness observation (why the wait is not another no-op 204)

On a live instance the harness heartbeat claims working sessions and drives them
(`harness: found active session` → `planning: starting interactive session` →
`planning: responding to user` in the serve log). One probe session seeded with
`status='thinking'` was claimed and settled to `idle` (iteration 1) by the
running mock-LLM turn before the first poll, and the route answered `204`
immediately — a TRUTHFUL answer, because the loop really had gone idle. The
`waiting_sub` fixture above is the arm that proves the blocking path, precisely
because nothing in the runtime moves that status on its own.

## Declared-vs-served leg

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-061   -> exit 0

jq -c '.drift_declared_not_served[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/wait")'
  -> (no output)
jq -c '.covered[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/wait")'
  -> {"path":"/api/session/{sessionID}/wait","method":"POST","operationId":"v2.session.wait",
      "declared_responses":"204,400,401,404,503","served_outcome":"204",...}

counts: served_route_entries 191 -> 192, covered_operations 141 -> 142,
        drift_declared_not_served 19 -> 18 (NOT-SERVED 15 -> 14),
        narrowed_error_contract_only 23 (unchanged), narrowed_unexplained 0,
        intentional_narrowing 3, semantic_different 2, drift_served_not_declared 0
```

`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is the
comparator's own output copied verbatim (never hand-edited); a second run at this
tree is byte-identical (`cmp` clean), and the artifact half of this flip is
pinned by `TestApiSessionWaitDeclaredVsServedFlip`.

The regeneration also rewrote the committed `appendix-*.md` / `family-summary.md`
copies, which had lagged the committed JSON: nine operations the JSON already
listed as covered (`v2.session.permission.get`, `v2.session.permission.reply`,
`v2.session.prompt`, `v2.session.question.list`, `v2.session.question.reject`,
`v2.session.question.reply`, `v2.session.revert.commit`, `v2.session.revert.stage`,
`experimental.workspace.list`) and one narrowing (`experimental.workspace.create`,
`SHIM-NARROWED-108`) were missing from the appendices. The JSON diff for this
change is exactly the one row flip plus counts and the documented positional
drift-id renumbering.

## Scope honesty

This is a probe of scratch instances on a mock LLM with small but REAL sqlite
stores (real migration-001 schema). It proves routing (including the chi
`MountPatterns` mount the server actually uses), the auth envelopes, all five
declared response codes (204/400/401/404/503) plus the 405 method guard, the real
wait behaviour against a real `sessions.status` column, and that the sibling
session routes are unchanged. It does NOT exercise a Postgres-backed deployment, a
long LLM turn (the mock provider answers in milliseconds), or a multi-session
fleet under concurrency. The `thinking` fixture's fast settlement is the mock
provider's speed, not a claim about production turn length.
