# ROUTE-ADD-041 — POST /api/session/{sessionID}/agent (v2.session.switchAgent) live probe

ch:trace row=ROUTE-ADD-041
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.switchAgent
  test=internal/shim/opencode/api_session_agent_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-041-live-probe.md
  witness=live:127.0.0.1:18041
  verdict=pending:foreman-tier2

Date: 2026-10-27 (~12:00Z), tick `consensus-foreman-2026-10-27-09-39-34`.
Base commit: worktree `wt/ROUTE-ADD-041` on `b8f3d61` (= origin/main at dispatch).
Implementation: foreman-direct after the dispatched worker (glm-5.3-flash @ zai-glm)
produced zero file writes in 29 minutes and was stopped by pid.

**Source item:** the board row cites `SHIM-DRIFT-037`. The committed artifact's
drift ids are POSITIONAL — the comparator renumbers them on every regeneration —
so this probe and the served-surface row are keyed on **operationId
`v2.session.switchAgent`** + method + path, never on a drift id.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-route041` |
| Binary SHA-256 | `1d3b56d10dd809afa05727953164c750853978019283076d61a6ff2bed39865c` |
| Listener | `127.0.0.1:18041` |
| Backend | scratch SQLite (`/tmp/route041-scratch-live/scratch.db`), migrations applied by `serve` |
| LLM | `provider: mock` + `CONSENSUS_MOCK_LLM=1` — no provider credentials involved |
| Auth | one `api_keys` row (scope `admin`) seeded with a throwaway probe token — the shim's auth wall reads the `api_keys` table and accepts `Authorization: Bearer`, not a bare `x-api-key` |
| Session fixture | one real `sessions` row (`9b1c4d2e-…`, agent_name `build`, model_id `mock-model`, status `idle`) |

## What the first probe run found (the probe earned its keep)

The first run of this same script, against the first build of this change, answered
**`404 page not found` for every method on the new path** — including the
authenticated POST — while the control `GET /api/session/active` answered 200.
Root cause: the route was registered on the shim's own mux but **omitted from
`MountPatterns`** (server.go), the exact-pattern list the production chi mount
walks; a shim route absent from that list is unreachable on a running server even
though the mux (and every unit test) serves it. Fixed by adding the exact
two-segment entry `"/api/session/{sessionID}/agent"` — never an `/api/session/*`
subtree mount.

The same run also exposed a second, subtler defect in the handler: the `internal/db`
drivers report a missing row as an **error** (`"sqlite: no rows in result"`), not as
`(nil, nil)`, so a bare `err != nil` sent every unknown session down the declared
400 arm and left the declared 404 unreachable on a live server (the unit tests
could not see it because `mockDB` returns `(nil, nil)`). Fixed by separating the two
store answers explicitly (`isNoRowsResult`), and pinned by
`TestApiSessionAgentStoreAnswerArms` so the regression cannot come back.

## Probe transcript (post-fix build)

```
## unauth POST /api/session/{id}/agent            (declared 401)
{"_tag":"UnauthorizedError","message":"invalid credentials"}
HTTP 401

## auth POST /api/session/{id}/agent  KNOWN session   (declared 204)
HTTP 204                     <- empty body, no JSON

## auth POST /api/session/{id}/agent  UNKNOWN session (declared 404)
{"_tag":"SessionNotFoundError","message":"no session \"00000000-…\" in this runtime: nothing to switch","sessionID":"00000000-…"}
HTTP 404

## auth POST /api/session/{id}/agent  body {}          (declared 400)
{"_tag":"InvalidRequestError","field":"agent","kind":"Body","message":"required field \"agent\" is missing"}
HTTP 400

## auth POST /api/session/{id}/agent  body {"agent":"build","model":"x"}   (declared 400)
{"_tag":"InvalidRequestError","field":"model","kind":"Body","message":"unknown field \"model\": the declared body schema allows only agent"}
HTTP 400

## auth POST /api/session/{id}/agent  ABSENT body      (declared 400)
{"_tag":"InvalidRequestError","field":"body","kind":"Body","message":"request body must be a JSON object containing only agent: EOF"}
HTTP 400

## auth GET /api/session/{id}/agent  (undeclared method)
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
HTTP 405

## control: auth GET /api/session/{id} sibling stays unregistered
404 page not found
HTTP 404

## control: auth GET /api/session/active still serves
{"data":{}}
HTTP 200
```

## Declared-vs-served leg (the row's acceptance probe)

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/parity-041     -> exit 0

jq '.drift_declared_not_served[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/agent")' \
  /tmp/parity-041/declared-vs-served.json                                       -> 0 lines (nothing)
jq '.covered[]? | select(.method=="POST" and .path=="/api/session/{sessionID}/agent")' \
  /tmp/parity-041/declared-vs-served.json                                       -> 1 entry, served_outcome "204"

counts: served_route_entries 168 -> 169, covered_operations 120 -> 121,
        drift_declared_not_served 42 -> 41 (NOT-SERVED 38 -> 37)
regenerated artifact == comparator output (jq -S diff empty)
```

## Arm map (each declared response, observed)

- **401** — the v2 `UnauthorizedError` envelope, reached through the
  `isV2EnvelopeRoutePath` two-segment shape (not the shim's own v1 body).
- **204** — the declared success for a session this instance holds and a
  well-formed request. Truthful empty no-op: this adapter holds no opencode
  agent-selection registry, and the declared success carries no body, so no agent
  change, identifier or state is claimed.
- **404** — the typed `SessionNotFoundError {_tag, sessionID, message}` read from
  the runtime's real `sessions` store.
- **400** — every declared input violation: blank sessionID (kind Path), and the
  absent / unparseable / non-object / null / multi-value body, an undeclared field,
  and a missing, non-string or blank `agent` (kind Body).
- **405** — the sibling `METHOD_NOT_ALLOWED` envelope for undeclared methods
  (POST is the only declared method on this path).
- **Controls** — no `/api/session/*` catch-all: the sibling
  `GET /api/session/{sessionID}` keeps its net/http default 404, and the
  ROUTE-ADD-039 `GET /api/session/active` still answers its declared 200.

## Reproduce

```bash
cd /home/kara/worktrees/consensus-ROUTE-ADD-041
go build -o /tmp/consensus-route041 ./cmd/consensus
mkdir -p /tmp/route041-scratch-live
cat > /tmp/route041-scratch-live/consensus.yaml <<'YAML'
llm:
  provider: mock
  default_model: mock-model
  max_context_tokens: 32000
  max_output_tokens: 4096
YAML
CONSENSUS_MOCK_LLM=1 /tmp/consensus-route041 serve \
  --config /tmp/route041-scratch-live/consensus.yaml \
  --db-url 'sqlite:///tmp/route041-scratch-live/scratch.db' \
  --port 18041 --hostname 127.0.0.1 --log-level error &

TOK='route041-live-probe-token'
PREFIX=$(printf '%s' "$TOK" | cut -c1-8)
HASH=$(printf '%s' "$TOK" | sha256sum | cut -d' ' -f1)
sqlite3 /tmp/route041-scratch-live/scratch.db \
  "INSERT INTO api_keys (key_hash, key_prefix, scope) VALUES ('$HASH', '$PREFIX', 'admin');"
sqlite3 /tmp/route041-scratch-live/scratch.db \
  "INSERT INTO sessions (id, agent_name, model_id, status) VALUES ('9b1c4d2e-0000-4000-8000-abcdefabcdef','build','mock-model','idle');"

curl -s -w '\nHTTP %{http_code}\n' -X POST -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' -d '{"agent":"build"}' \
  http://127.0.0.1:18041/api/session/9b1c4d2e-0000-4000-8000-abcdefabcdef/agent
```

## Scope honesty

This is a probe of a scratch instance on a mock LLM with an empty (but real)
sessions store: it proves the routing, auth envelope, store-read arms and the
declared 204 are exercised by a running server. It does NOT exercise a populated
Consensus deployment, an authenticated multi-key scope, or any LLM-backed session
lifecycle — none of which this operation's declared contract depends on. The
fixture session row exists only so the 204 arm (not 404) is reachable.
