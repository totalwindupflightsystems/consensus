# ROUTE-ADD-045/047/048 — GET /api/session/{sessionID}/message-read family (history, messages, message) live probe

ch:trace row=ROUTE-ADD-045
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.history
  test=internal/shim/opencode/api_session_history_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-045-live-probe.md
  witness=live:127.0.0.1:18746
  verdict=pending:foreman-tier2

Date: 2026-10-28 (~09:05Z), wave `consensus-foreman-2026-10-28-07-41-25`
(rows ROUTE-ADD-045, ROUTE-ADD-047, ROUTE-ADD-048; gitreins task `route-add-045`).
Worktree `wt/ROUTE-ADD-045` on `1106e4c` (= origin/main at dispatch).

**Source items:** the three sibling message-read routes of the
`/api/session/{sessionID}` family. The committed artifact's drift ids are
POSITIONAL — the comparator renumbers them on every regeneration — so this
probe and the served-surface rows are keyed on **operationId**:
`v2.session.history`, `v2.session.messages`, `v2.session.message`.

## Run identity

| Field | Value |
|---|---|
| Pre-change (RED) binary | `/tmp/consensus-red-ra045`, SHA-256 `c75ac49b…` (built from the dispatched base 1106e4c + untracked handler skeleton, i.e. the routes UNREGISTERED) |
| Post-change binary | `/tmp/consensus-ra045-live`, SHA-256 `0f47e8385e941983…` (full change tree) |
| Listener | `127.0.0.1:18745` (red run) / `127.0.0.1:18746` (post-fix run) |
| Backend | scratch SQLite (`/tmp/ra045-red/scratch.db`, `/tmp/ra045-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` + `CONSENSUS_MOCK_LLM=1` — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with a throwaway probe token, sent as `Authorization: Bearer ***` (redacted after first use; the server's own auto-minted first-admin key on each fresh scratch DB was printed to the serve log and is likewise `[REDACTED]` here — it never left this host) |
| Session fixture | one real `sessions` row (`9b1c4d2e-…`, agent_name `build`, model_id `mock-model`, status `idle`) plus three `memory_events` rows (id 1 user_message full; id 2 text_block compressed with `summary_text='compact answer'`; id 3 text_block hidden) and the two matching `display_modes` rows |

## RED proof (pre-change build, port 18745)

All three paths answered net/http's default 404 — the drift artifact's
NOT-SERVED class, indistinguishable from "no such opencode operation":

```
== RED: auth'd GET /api/session/{id}/history
404 page not found
HTTP 404
== RED: auth'd GET /api/session/{id}/message
404 page not found
HTTP 404
== RED: auth'd GET /api/session/{id}/message/msg-1
404 page not found
HTTP 404
== RED: unauth GET /api/session/{id}/history      (unmounted from the chi mount -> no route at all, not even the auth wall's 401)
404 page not found
HTTP 404
== control: unauth GET /api/session/active        (auth wall still armed, v2 envelope intact)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401
```

## Post-change probe transcript (port 18746; full session fixture seeded)

```
## unauth GET /api/session/{id}/history          (declared 401, v2 envelope)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth GET /api/session/{id}/history  KNOWN     (declared 200; hidden omitted, compressed compact)
{"data":[{"content":"hello from the probe","created_at":"2026-10-28 09:04:39","iteration_created":0,"memory_event_id":1,"type":"user_message"},{"created_at":"2026-10-28 09:04:39","display_mode":"compressed","iteration_created":0,"memory_event_id":2,"summary_text":"compact answer","type":"text_block"}],"hasMore":false}
HTTP 200

## auth GET /api/session/{id}/history  UNKNOWN   (declared 404 SessionNotFoundError)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-ffffffffffff\" in this runtime: no history to report","sessionID":"00000000-0000-4000-8000-ffffffffffff"}
HTTP 404

## auth GET history blank session segment        (declared 400 kind Path)
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Path","message":"sessionID path parameter must not be blank"}
HTTP 400

## auth GET history bad limit                    (declared 400 kind Query field limit)
{"_tag":"InvalidRequestError","field":"limit","kind":"Query","message":"limit must be a positive integer"}
HTTP 400

## auth GET history bad after                    (declared 400 kind Query field after)
{"_tag":"InvalidRequestError","field":"after","kind":"Query","message":"after must be a non-negative memory_events id (the runtime resume token)"}
HTTP 400

## auth POST /api/session/{id}/history           (undeclared method, 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405

## auth GET /api/session/{id}/message  KNOWN     (declared 200 {data, cursor}; hidden row omitted, compact view for the compressed row)
{"cursor":{},"data":[{"content":"hello from the probe","created_at":"2026-10-28 09:04:39","iteration_created":0,"memory_message_id":1,"type":"user_message"},{"created_at":"2026-10-28 09:04:39","display_mode":"compressed","iteration_created":0,"memory_message_id":2,"summary_text":"compact answer","type":"text_block"}]}
HTTP 200

## auth GET /api/session/{id}/message page 1     (declared 200, cursor.next present)
{"cursor":{"next":"1"},"data":[{"content":"hello from the probe","created_at":"2026-10-28 09:04:39","iteration_created":0,"memory_message_id":1,"type":"user_message"}]}
HTTP 200

## auth GET /api/session/{id}/message page 2     (declared 200 via cursor token; cursor.previous points back)
{"cursor":{"previous":"1"},"data":[{"created_at":"2026-10-28 09:04:59","display_mode":"compressed","iteration_created":0,"memory_message_id":2,"summary_text":"compact answer","type":"text_block"}]}
HTTP 200

## auth GET message cursor+order combined        (declared 400 — the pinned document forbids the combination)
{"_tag":"InvalidRequestError","field":"cursor","kind":"Query","message":"cursor must not be combined with order (the pinned document forbids it)"}
HTTP 400

## auth GET message foreign cursor               (declared 400 kind Cursor — store-decided)
{"_tag":"InvalidRequestError","field":"cursor","kind":"Cursor","message":"cursor names no memory event of this session"}
HTTP 400

## auth GET /api/session/{id}/message  UNKNOWN   (declared 404 SessionNotFoundError)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-ffffffffffff\" in this runtime: no messages to list","sessionID":"00000000-0000-4000-8000-ffffffffffff"}
HTTP 404

## auth GET message bad order                    (declared 400 kind Query field order)
{"_tag":"InvalidRequestError","field":"order","kind":"Query","message":"order must be one of the declared values: asc, desc"}
HTTP 400

## auth GET /api/session/{id}/message/msg-1      (SHIM-NARROWED-107: the session HOLDS msg-1; declared 404 MessageNotFoundError, messageID named, never a fabricated SessionMessage)
{"_tag":"MessageNotFoundError","message":"the session holds this message row, but the declared SessionMessage body (a ^msg_-keyed union) is not producible truthfully from this runtime's integer-keyed ledger","messageID":"msg-1","sessionID":"9b1c4d2e-0000-4000-8000-abcdefabcdef"}
HTTP 404

## auth GET /api/session/{id}/message/msg-999    (declared 404 MessageNotFoundError, unknown id)
{"_tag":"MessageNotFoundError","message":"no message \"msg-999\" in session \"9b1c4d2e-0000-4000-8000-abcdefabcdef\" (or held, but the declared SessionMessage body is not producible truthfully)","messageID":"msg-999","sessionID":"9b1c4d2e-0000-4000-8000-abcdefabcdef"}
HTTP 404

## auth GET message/{id} on UNKNOWN session      (declared 404 SessionNotFoundError — session resolved FIRST, no messageID member)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-ffffffffffff\" in this runtime: no message to resolve","sessionID":"00000000-0000-4000-8000-ffffffffffff"}
HTTP 404

## auth GET message/{id} blank messageID         (declared 400 kind Path field messageID)
{"_tag":"InvalidRequestError","field":"messageID","kind":"Path","message":"messageID path parameter must not be blank"}
HTTP 400

## auth DELETE /api/session/{id}/message/msg-1   (undeclared method, 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405

## control: auth GET /api/session/{id} sibling   (stays as ROUTE-ADD-040 landed it)
{"data":{"agent_name":"build","goal":"","id":"9b1c4d2e-0000-4000-8000-abcdefabcdef","status":"idle"}}
HTTP 200

## control: auth GET /api/session/{id}/agent     (ROUTE-ADD-041 sibling untouched — GET keeps its 405; POST still answers 204)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405

## control: unauth GET /api/session/{id}/compact (undeclared deeper path keeps the default 404 — no catch-all added)
404 page not found
HTTP 404
```

## Arms not reachable live (stated, not hidden)

- **500 UnknownError (v2.session.messages only)** — a store-read failure cannot
  be injected into a healthy scratch instance without sabotaging the server, so
  this arm is proven by the unit test
  `TestApiSessionMessagesStoreFailureAnswersDeclared500` (mockDB
  `queryRowErr`/`queryErr` = a non-"no rows" error → 500 `{"_tag":"UnknownError"}`),
  not by curl. It is the family's only declared 5xx arm, which is exactly why
  the sibling routes keep the declared 400 for store failures instead.
- The fixture work itself exercised the real migration-001 store rules: the
  first probe attempt's `UPDATE memory_events SET summary_text=…` was ABORTED by
  the append-only trigger (migrations 017/018) and the `display_modes` insert
  failed on the NOT NULL `set_by_iteration` — so the fixture appends
  `summary_text` WITH the row and inserts display rows with `set_by_iteration=0`
  (both visible in the probe script).

## Declared-vs-served leg

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/ra045-parity-out-2810   -> exit 0

counts (regenerated): served_route_entries 171 -> 174, covered_operations 123 -> 125,
        narrowed_error_contract_only 21 -> 22 (SHIM-NARROWED-107 = v2.session.message),
        drift_declared_not_served 39 -> 36 (NOT-SERVED 35 -> 32)
regenerated artifact == committed comparator output (copied, never hand-edited)
```

Drift-id renumbering (verified by diffing the old vs new artifacts by
operationId+method+path): 33 remaining drift rows shifted down by the removals —
`SHIM-DRIFT-005→004` through `SHIM-DRIFT-039→036`; three rows LEFT the class
(`SHIM-DRIFT-004/006/007` = the three operations this wave serves). Never cite a
drift id; cite the operationId.

## Scope honesty

This is a probe of a scratch instance on a mock LLM with a small but REAL
sqlite store (real migration-001 schema, real append-only triggers): it proves
the routing, auth envelopes, store-read arms, the display-state replay and the
declared error arms are exercised by a running server. It does NOT exercise a
populated multi-session deployment, postgres backend, or any LLM-backed session
lifecycle — none of which these operations' declared contracts depend on.
