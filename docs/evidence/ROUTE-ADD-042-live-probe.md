# ROUTE-ADD-042 — POST /api/session/{sessionID}/compact (v2.session.compact) live probe

ch:trace row=ROUTE-ADD-042
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.compact
  test=internal/shim/opencode/api_session_compact_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-042-live-probe.md
  witness=live:127.0.0.1:18747
  verdict=pending:foreman-tier2

Date: 2026-10-28 (~10:10Z). Worktree `wt/ROUTE-ADD-042` on `599aa7a`
(= origin/main at dispatch).

**Source item:** the board row cites `SHIM-DRIFT-038`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration
(this regeneration moved 34 remaining drift rows down by one) — so this probe
and the served-surface row are keyed on **operationId `v2.session.compact`** +
method + path, never on a drift id. In the pre-change artifact the operation was
row `SHIM-DRIFT-001`; neither id is stable.

## Run identity

| Field | Value |
|---|---|
| Pre-change (RED) binary | `/tmp/consensus-ra042-red`, SHA-256 `ec3fb643f10580a7f7e09dd3e4ad1cbf9b92b704d43e014082dac701a298243f` (built from the canonical checkout at origin/main `599aa7a`, i.e. the route UNREGISTERED) |
| Post-change binary | `/tmp/consensus-ra042-live`, SHA-256 `63d82c12c8a19a334e149a7c0c3890f69a8f70f36d49d48aa685ad594a6e8d63` (this worktree's full change tree) |
| Listener | `127.0.0.1:18744` (RED) / `127.0.0.1:18747` (post-change) / `127.0.0.1:18748` (503-arm run) |
| Backend | scratch SQLite (`/tmp/ra042-red/scratch.db`, `/tmp/ra042-live/scratch.db`, `/tmp/ra042-503/scratch.db`), migrations applied by the server on startup |
| LLM | `provider: mock` + `CONSENSUS_MOCK_LLM=1` — no provider credentials involved |
| Auth | the server's own auto-minted first-admin key on each fresh scratch DB, read from the serve log and sent as `Authorization: Bearer *** (redacted here; it never left this host and never entered the repo) |
| Session fixture | one real `sessions` row (`9b1c4d2e-0000-4000-8000-abcdefabcdef`, agent_name `build`, model_id `mock-model`, status `idle`) |

## RED proof (pre-change build, port 18744)

```
## RED: auth POST /api/session/{id}/compact   (net/http default 404 — the NOT-SERVED class)
404 page not found
HTTP 404

## RED control: auth GET /api/session/active   (auth wall + shim armed)
{"data":{}}
HTTP 200
```

The 404 is net/http's default page, i.e. no route existed — indistinguishable
from "no such opencode operation", exactly the drift artifact's NOT-SERVED
class. The control proves the shim was mounted and the auth wall armed, so the
404 is the missing route and not a dead instance.

## Post-change probe transcript (port 18747)

```
## unauth POST /api/session/{id}/compact            (declared 401 UnauthorizedError)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth POST /api/session/{id}/compact  KNOWN       (declared 204, empty body)
HTTP 204

## auth POST /api/session/{id}/compact  UNKNOWN     (declared 404 SessionNotFoundError)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-0000-4000-8000-ffffffffffff\" in this runtime: nothing to compact","sessionID":"00000000-0000-4000-8000-ffffffffffff"}
HTTP 404

## auth POST /api/session/%20/compact               (declared 400 kind Path field sessionID)
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Path","message":"sessionID path parameter must not be blank"}
HTTP 400

## auth GET /api/session/{id}/compact               (undeclared method -> 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
HTTP 405

## auth POST with an undeclared JSON body           (ignored -> 204; the document declares no requestBody)
HTTP 204

## auth POST /api/session/{id}/compact?x=1          (undeclared query ignored -> 204)
HTTP 204

## control: auth GET /api/session/{id}              (ROUTE-ADD-040 sibling unchanged)
{"data":{"agent_name":"build","goal":"probe fixture","id":"9b1c4d2e-0000-4000-8000-abcdefabcdef","status":"idle"}}
HTTP 200

## control: auth GET /api/session/active            (ROUTE-ADD-039 sibling unchanged)
{"data":{}}
HTTP 200

## control: auth POST /api/session/{id}/model       (ROUTE-ADD-049 sibling unchanged, 204 no-op)
HTTP 204

## control: auth POST /api/session/{id}/agent       (ROUTE-ADD-041 sibling unchanged; body required -> 400)
{"_tag":"InvalidRequestError","field":"body","kind":"Body","message":"request body must be a JSON object containing only agent: EOF"}
HTTP 400

## control: auth GET /api/session/{id}/compact/x    (deeper path stays unregistered, no catch-all)
404 page not found
HTTP 404
```

The two 204s with a body and with a query string are the "cannot refuse input
the document does not declare" arm: the pinned operation declares no
requestBody and no query parameter, so those inputs are ignored rather than
answered with an invented 400.

## Declared 503 arm — injected live (port 18748)

A store-read failure is injected on the scratch instance by renaming the
`sessions` table, which makes the handler's read fail with a non-"no rows"
error, then restored:

```
## healthy store: KNOWN session (declared 204)
HTTP 204

-- injecting: ALTER TABLE sessions RENAME TO sessions_unreadable_probe
## unreadable store (sessions table gone): declared 503 ServiceUnavailableError
{"_tag":"ServiceUnavailableError","message":"failed to resolve session: sqlite: query row: SQL logic error: no such table: sessions (1)","service":"sessions"}
HTTP 503

-- restoring: ALTER TABLE sessions_unreadable_probe RENAME TO sessions
## restored store: KNOWN session again (declared 204)
HTTP 204
```

This is the arm that distinguishes this operation from its siblings: because
`v2.session.compact` DECLARES a 5xx, an unreadable store is reported as the
declared 503 `ServiceUnavailableError` with `service: "sessions"`, not as the
declared 400 that `switchAgent` / `switchModel` / `history` answer (they declare
no 5xx). The restore proves the 503 came from the injected store failure and the
handler returns to the 204 on a healthy store.

## Declared-vs-served leg

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-042   -> exit 0

jq -c '.drift_declared_not_served[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/compact")'
  -> (no output)
jq -c '.covered[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/compact")'
  -> {"path":"/api/session/{sessionID}/compact","method":"POST","operationId":"v2.session.compact","served_outcome":"204"}

counts: served_route_entries 174 -> 175, covered_operations 125 -> 126,
        drift_declared_not_served 36 -> 35 (NOT-SERVED 32 -> 31),
        narrowed_error_contract_only 22 (unchanged), intentional_narrowing 3,
        semantic_different 2, drift_served_not_declared 0
```

`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` is the
comparator's own output copied verbatim (never hand-edited); a fresh run at this
tree is byte-identical to the committed artifact, and the artifact half of this
flip is pinned by
`TestApiSessionCompactDeclaredVsServedFlip`.

## Scope honesty

This is a probe of a scratch instance on a mock LLM with a small but REAL sqlite
store (real migration-001 schema). It proves routing (including the chi
`MountPatterns` mount the server actually uses), the auth envelopes, all five
declared response codes (204/400/401/404/503) plus the 405 method guard, and
that the sibling session routes are unchanged. It does NOT exercise a
Postgres-backed deployment, a populated multi-session store, or any LLM-backed
compaction — the declared 204 is answered as a truthful no-op precisely because
this adapter holds no conversation-compaction pipeline (handler doc comment:
`internal/compression` is a background, queue-driven memory-tier pipeline the
shim Server has no reference to; wiring an on-demand compaction is a design
change with its own row).
