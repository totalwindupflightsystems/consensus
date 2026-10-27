# ROUTE-ADD-040 + ROUTE-ADD-049 — live probe (merged by INT-BOARD-1)

ch:trace row=ROUTE-ADD-040,ROUTE-ADD-049
  spec=specs/openapi/upstream/openapi-1.18.33.surface.json#v2.session.get,v2.session.switchModel
  test=internal/shim/opencode/api_session_get_test.go,internal/shim/opencode/api_session_model_switch_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-040-live-probe.md
  witness=live:127.0.0.1:18040
  verdict=none:foreman-will-run-after-merge

Date: 2026-10-27, INT-BOARD-1 board-integrity reconciliation (worktree
`consensus-INT-BOARD-1`, branch `wt/INT-BOARD-1` on `71dc39a` = origin/main at
dispatch). Both routes were merged from their unmerged worker branches
(`wt/route-add-040` tip `7fabf15`, `wt/ROUTE-ADD-049` `3e0b67f`) as part of
INT-BOARD-1 and probed together on one scratch instance.

**Operation identity note:** the board rows cite `SHIM-DRIFT-036` (040) and
`SHIM-DRIFT-010` (049). The committed artifact's drift ids are POSITIONAL —
the comparator renumbers them on every regeneration — so this probe, the
served-surface rows and the acceptance jq checks are keyed on **operationId**
(`v2.session.get`, `v2.session.switchModel`) + method + path, never on a drift
id.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-intboard1` (built from the merged tree) |
| Binary SHA-256 | `7247497035290a317d47a4f6d9c2df1cab3fe5012f9150da92f79bf892c6a70d` (post-fix run; pre-fix run `1e9da9bd767d1e204e5a1f45064b5ed1763224fa56df8ae67721b122f7f235bb`) |
| Listener | `127.0.0.1:18040` |
| Backend | scratch SQLite (`/tmp/intboard1-scratch-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` + `CONSENSUS_MOCK_LLM=1` — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with a throwaway probe token |
| Session fixture | one real `sessions` row (`9b1c4d2e-…`, agent_name `build`, model_id `mock-model`, status `idle`) |

## What the first probe run found (the probe earned its keep — again)

The first run, against the branch code merged verbatim, answered the declared
**404 arm of GET /api/session/{sessionID} with a 400**:

```
## auth GET unknown session      (declared 404)
{"_tag":"InvalidRequestError","field":"sessions","kind":"Shim","message":"failed to read session: sqlite: no rows in result"}
HTTP 400
```

Root cause (the exact defect class ROUTE-ADD-041's live probe found in its
sibling two weeks earlier): `internal/db` drivers report a missing row as an
ERROR VALUE (`sqlite: no rows in result`), not as `(nil, nil)`, so the branch
handler's `err != nil -> 400` branch swallowed the "no such session" answer and
the declared NotFoundError 404 was unreachable on a live server. The branch's
unit test could not see it — `mockDB` returns `(nil, nil)` for the no-row case,
and the branch test fed the no-rows error string as a stand-in for an
"unreadable store", codifying the bug. Fixed with the shared `isNoRowsResult`
seam (`api_session_agent.go`): a no-rows reply is the declared 404, every
OTHER read failure keeps the declared 400, and the test now pins BOTH arms
with distinct error fixtures (`sqlite: disk I/O error` -> 400, `sqlite: no
rows in result` -> 404).

## Probe transcript (post-fix build)

```
=== ROUTE-ADD-040: GET /api/session/{sessionID} ===
## auth GET known session        (declared 200)
{"data":{"agent_name":"build","goal":"","id":"9b1c4d2e-0000-4000-8000-abcdefabcdef","status":"idle"}}
HTTP 200

## auth GET unknown session      (declared 404)
{"data":{"message":"Session not found"},"name":"NotFoundError"}
HTTP 404

## unauth GET                    (declared 401)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth GET blank segment %20    (declared 400)
{"_tag":"InvalidRequestError","field":"sessionID","kind":"Session","message":"sessionID path parameter is required"}
HTTP 400

## auth POST                     (undeclared method, declared 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
HTTP 405

=== ROUTE-ADD-049: POST /api/session/{sessionID}/model ===
## auth POST known session       (declared 204)
HTTP 204                (empty body, no Content-Type)

## auth POST unknown session     (declared 404)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-1111-4222-8333-444444444444\" in this runtime: nothing to switch","sessionID":"00000000-1111-4222-8333-444444444444"}
HTTP 404

## unauth POST                   (declared 401)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth GET /model               (undeclared method, declared 405)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
HTTP 405

=== controls ===
## auth GET /api/session/active  (sibling stays served, declared 200)
{"data":{}}
HTTP 200

## auth GET /api/session/a/b     (deeper path keeps default 404)
404 page not found
HTTP 404

## auth POST /api/session/extra/deeper  (deeper path keeps default 404)
404 page not found
HTTP 404
```

## Declared-vs-served leg (the rows' acceptance probes)

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-int-board-1   -> exit 0

jq '.drift_declared_not_served[]? | select(.method=="GET" and .path=="/api/session/{sessionID}")'   -> 0 lines
jq '.drift_declared_not_served[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/model")' -> 0 lines
jq '.covered[]? | select(.method=="GET" and .path=="/api/session/{sessionID}")'
  -> exactly one entry, served_outcome "200"
jq '.covered[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/model")'
  -> exactly one entry, served_outcome "204"

counts (main -> merged): served_route_entries 169 -> 171,
  covered_operations 121 -> 123, drift_declared_not_served 41 -> 39
  (NOT-SERVED 37 -> 35); narrowed_unexplained stays 0;
  drift_served_not_declared stays 0
```

## Merge-completion notes (what the merge itself fixed)

Both branches predated the ROUTE-ADD-041 work that landed on main first, so the
merge is a union plus three completions, each pinned by a test:

1. **040 404 arm** — the `isNoRowsResult` split described above
   (`TestApiSessionGetServesDeclared404/no-rows-reply…`, and the live probe).
2. **049 401 arm** — the branch registered a literal
   `"/api/session/{sessionID}/model"` entry in `v2EnvelopeRoutePaths`, but that
   map is keyed by CONCRETE request paths, so the parameterized key could never
   match and the declared 401 arm was unserved by construction (the same
   pattern-literal defect 041's probe documented). The merged tree serves it
   through the `isApiSessionModelPath` two-segment rule
   (`TestApiSessionModelSwitchDeclared401` pins it on the live middleware).
3. **049 served-surface row** — the branch commit closed the row WITHOUT
   adding the `/api/session/{sessionID}/model` row to
   `consensus-shim-served-surface.yaml` (the row's own acceptance probe could
   not have passed on the branch: the comparator classifies from that file).
   The merged row is added and the artifact regenerated.
   The 049 session-existence check also moved to the shared
   `isNoRowsResult` seam (`TestApiSessionModelSwitchUnreadableStoreIsDeclared400`
   vs `…UnknownSessionIsDeclared404`).

## Arm map (each declared response, observed)

ROUTE-ADD-040 (v2.session.get, declared 200/400/401/404):
- **200** — `{data: {id, agent_name, goal, status}}` from the real sessions
  row (runtime-native members only; no invented SessionV2Info members).
- **404** — `{name: "NotFoundError", data: {message: "Session not found"}}`
  (the upstream sdk-error-shape NamedError body).
- **401** — the v2 `UnauthorizedError` envelope via the one-segment
  `/api/session/` rule.
- **400** — blank `%20` segment; any genuinely unreadable store.
- **405** — every undeclared method (GET is the only declared method).

ROUTE-ADD-049 (v2.session.switchModel, declared 204/400/401/404):
- **204** — the truthful empty no-op for a held session (no model-switch
  pipeline exists; the declared success carries no body).
- **404** — typed `{_tag: SessionNotFoundError, sessionID, message}`.
- **401** — the v2 envelope via the `isApiSessionModelPath` rule.
- **400** — blank `%20` segment; any genuinely unreadable store.
- **405** — every undeclared method (POST is the only declared method).

Controls: no `/api/session/*` catch-all — deeper paths keep net/http's default
404, `/api/session/active` keeps its own 200, and the sibling routes do not
reclassify each other.

## Scope honesty

This is a probe of a scratch instance on a mock LLM with an empty (but real)
sessions store: it proves the routing, MountPatterns wiring, auth envelopes,
store-read arms and the declared success arms are exercised by a running
server. It does NOT exercise a populated Consensus deployment, an authenticated
multi-key scope, or any LLM-backed session lifecycle — none of which either
operation's declared contract depends on. The fixture session row exists only
so the 200/204 arms (not 404) are reachable.
