# T1 evidence — full OpenAPI sweep against a locally built consensus binary

Task: `TEST-CONSENSUS-001` (tier T1). Generated 2026-09-30T08:16:06.204620Z.

> Previous attempt ran on the bunker agent `consensus-dev`, which has since expired. This run is LOCAL: the binary
> was built and served on this host and the sweep was driven against it over HTTP.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/consensus-test-bin` (sha256 `7d3250f76bd5c345282c5b5ad14b97bc1ac13541f9a9cebcf46150f824c6baf5`) |
| **binary version string** | **`consensus version 0.1.0`** |
| **server port** | **8099** (`127.0.0.1:8099`, pid 420370) |
| server started | Wed Sep 30 03:11:27 2026 |
| db | `sqlite:///tmp/t1-sweep/scratch/t1.db?_journal_mode=WAL` (backend `sqlite`) |
| repo / branch | `/home/kara/consensus` @ `master` |
| HEAD under test | `9424db6aae714e014ccde272284370277647778a` |
| go toolchain | `go version go1.26.5 linux/amd64` |
| served `/openapi.json` | sha256 `448367020bb439f1ac6e39f5496f952bf2ccdaf1d33363bc4b369495282d751b` |
| served `/openapi.yaml` | sha256 `56d1f5d6a7a6b6609adb3cf403a347a470031a6b2c03dd6c798d52695e853b3d` |
| `specs/openapi/bundled.yaml` | sha256 `56d1f5d6a7a6b6609adb3cf403a347a470031a6b2c03dd6c798d52695e853b3d` |
| served yaml == bundled yaml | **yes, byte-identical** |

The server was started with a **dummy** `DEEPSEEK_API_KEY` so that `POST /api/v1/sessions` passes its
`MISSING_LLM_CONFIG` gate (internal/api/sessions.go:40) and the session-scoped paths could be exercised with a real
session id. No live provider traffic was generated; T1 does not assert on LLM behaviour (that is T2).

## 2. Census

| measure | value |
|---|---|
| paths in the test plan's assertion (`60`) | 60 |
| **paths served / declared** | **66** |
| **operations (path x method) declared** | **78** |
| paths in `specs/openapi/bundled.yaml` | 66 |
| served path set == bundled path set | yes |

Every declared operation was probed, twice: once with an admin bearer token and a minimal request, once with no
credential at all. The unauthenticated pass exists because a status can only be called a match if it is the status
the documented request would produce.

## 3. Result summary

| measure | count |
|---|---|
| operations probed | 78 |
| status matches (minimal request) | 77 |
| **status mismatches (minimal request)** | **1** |
| **further status mismatches reachable only with a well-formed body** | **3** (T1-D1, T1-D2, T1-D5) |
| **documented status proven unreachable (T1-D4)** | **1** |
| route-not-mounted (chi plain-text 404) | 0 |
| undocumented 5xx in the minimal-request sweep | 0 |
| operations returning 401 unauthenticated but not declaring 401 | 7 |
| error-envelope failures on 4xx/5xx | 1 |
| confirmed defects | 5 |
| declared-schema (format) findings | 3 |
| plan-drift findings | 1 |
| documented skips | 6 |

The two status rows are different measurements and both matter. The minimal request (`{}` / no body) reaches
validation gates first: it produced **1** mismatch out of 78 operations (T1-D3). Three more operations answer a
status the contract forbids only once the request is well formed — the `{}` body is refused earlier and hides the
defect. A status-matched minimal sweep is therefore not evidence that the contract holds; it is evidence that the
REJECTION path holds. One further defect (T1-D4) answers a status that IS documented while the documented success
status is unreachable.

Observed status histogram across all 78 operations: `200`: 48, `400`: 17, `404`: 9, `409`: 1, `501`: 3.

## 4. Defects

Each defect below is a status the served contract does not allow, reproduced against the live binary. Response
bodies are quoted verbatim from the run record.

### T1-D1 — POST /api/v1/auth/keys returns 200 where the contract declares 201

- severity: **high**
- class: status-code contract violation
- request: `POST /api/v1/auth/keys`
- documented statuses: `[201, 400, 401, 403, 500]`
- **observed status: `200`**
- spec reference: paths./api/v1/auth/keys.post.responses.201 -> components.schemas.CreateAPIKeyResponse ('API key created.')
- prior-candidate recheck: CONFIRMED (was listed as an unverified claim from the expired bunker run)

```json
{"api_key":"cs_sk_3814514e959ee4715ac1e43586a86658f552dfda7420f914800736c8be781ab6","created_at":"2026-09-30T08:14:27Z","id":"85071824-c38b-41c4-a881-9ee47c4039e6","key_prefix":"cs_sk_38","scope":"readonly"}
```

### T1-D2 — POST /api/v1/sessions/{sessionId}/tasks returns 200 where the contract declares 201

- severity: **high**
- class: status-code contract violation
- request: `POST /api/v1/sessions/68ed0699-367d-4206-9268-25ebab753dcb/tasks`
- documented statuses: `[201, 400, 401, 403, 500]`
- **observed status: `200`**
- prior-candidate recheck: NEW (same class as T1-D1; not in the prior candidate list)

```json
{"id":"a1fd8b68-59c6-4081-8f17-c45c134f46e2","session_id":"68ed0699-367d-4206-9268-25ebab753dcb","title":"t1-d2-task","description":"201 contract check","status":"pending","priority":5,"prerequisite_ids":[],"created_at":"2026-09-30T08:14:27Z"}
```

control — `POST /api/v1/sessions` → **201**

> POST /api/v1/sessions DOES return the documented 201, so the handler family is inconsistent: the create-session handler sets 201, the create-task handler does not.

### T1-D3 — GET /project/{projectId} returns 404 where the contract declares only 501

- severity: **medium**
- class: status-code contract violation + error-vocabulary divergence
- request: `GET /project/t1-sweep-project`
- documented statuses: `[501]`
- **observed status: `404`**
- prior-candidate recheck: CONFIRMED (was listed as an unverified claim from the expired bunker run)

```json
{"_tag":"ProjectNotFoundError","message":"Project not found: t1-sweep-project","projectID":"t1-sweep-project"}

```

control — `GET /project` → **501**

> the sibling root path /project returns the documented 501 NOT_IMPLEMENTED envelope; the parameterised path answers a 404 with a different (opencode-shaped) error body.

### T1-D4 — GET /permission/{permissionId} answers 404 for an EXISTING approval row: the documented 200 is unreachable

- severity: **high**
- class: status-code contract violation, silently masked as not-found
- request: `GET /permission/t1-perm-0002   (approval_requests row t1-perm-0002 injected into the scratch DB, status=pending)`
- documented statuses: `[200, 401, 404]`
- **observed status: `404`**
- root cause: internal/shim/opencode/server.go:2235 getPermission SELECTs ar.sql_preview, ar.decision_reason and ar.resolved_at; the shipped SQLite schema approval_requests (migrations/001_initial_schema.sql) has target_sql, review_notes and reviewed_at instead. The query error is swallowed into a 404, so a schema bug is reported as 'not found'.
- prior-candidate recheck: NEW (derived from the same root cause as T1-D5)

```json
{"error":{"code":"NOT_FOUND","message":"permission not found"}}
```

control — `GET /api/v1/approvals/t1-perm-0002` → **200**

> the same row read through the consensus route returns the documented 200, so the row is readable and the shim handler is the failing surface.

### T1-D5 — POST /permission/{permissionId}/resolve returns 500 on a well-formed body

- severity: **critical**
- class: undocumented 5xx from a schema/code drift
- request: `POST /permission/t1-perm-0002/resolve`
- documented statuses: `[200, 400, 401, 404]`
- **observed status: `500`**
- root cause: internal/shim/opencode/server.go:2278 UPDATE approval_requests SET status=$1, decision_reason=$2, resolved_at=$3, resolved_by='opencode-shim' — those three columns do not exist in the shipped approval_requests schema (review_notes / reviewed_at / reviewer_id are the real columns). SQLite raises 'no such column: decision_reason', surfaced verbatim as a 500.
- methodology: The bulk sweep's minimal `{}` body is refused by the decision vocabulary check (400) BEFORE the UPDATE runs, which masks the defect. It only reproduces with a well-formed {'decision': 'approved'|'rejected'|'modified'} body.
- prior-candidate recheck: CONFIRMED (was listed as an unverified claim of 500 vs {200,400,401,404})

```json
{"error":{"code":"INTERNAL_ERROR","message":"failed to resolve permission: sqlite: exec: SQL logic error: no such column: decision_reason (1)"}}
```

control — `POST /api/v1/approvals/t1-perm-0002/review` → **200**

> the consensus route POST /api/v1/approvals/{id}/review resolves the same row with the documented 200.

## 5. Plan drift: the 60-path census is stale

- plan asserts **60 paths** (`C-006, C-067, C-131, C-132, C-192, C-240, C-242`); the served contract declares **66 paths / 78 operations**.
- `GET /openapi.yaml` is byte-identical to `specs/openapi/bundled.yaml` (**yes**; sha256 `56d1f5d6a7a6b6609adb3cf403a347a470031a6b2c03dd6c798d52695e853b3d` both
  sides) and the path sets are identical, so this is plan staleness rather than implementation drift.
  Those cells must be run with a measured census (66 paths / 78 operations) or they fail as written.

## 6. Documentation gaps — routes that enforce auth but do not declare 401

The prior attempt reported a "missing-401-docs class on stub routes". It reproduces, and it is larger than one route:

| operation | documented statuses | unauthenticated response |
|---|---|---|
| `GET /api/v1/skills` | `[200, 500]` | 401 |
| `GET /api/v1/skills/{skillName}` | `[200, 404, 500]` | 401 |
| `GET /api/v1/tools` | `[200, 500]` | 401 |
| `GET /find/symbol` | `[501]` | 401 |
| `GET /project/{projectId}` | `[501]` | 401 |
| `GET /tui/{action}` | `[404, 501]` | 401 |
| `GET /vcs/{vcsId}` | `[501]` | 401 |

For all 7, a request with no credential returns 401 UNAUTHENTICATED while the operation's `responses` object does
not list 401. A generated client that trusts the spec cannot predict the rejection.

## 7. Declared-schema (format) findings — status matched, body violates the schema

### T1-F1 — heartbeat_at is not RFC 3339 on GET /api/v1/sessions/{sessionId}

- severity: **medium**
- declared:

  > components.schemas.Session.heartbeat_at = {type: string, format: date-time}
- get_observed_value:

  > 2026-09-30 08:14:27
- get_value_is_rfc3339:

  > False
- patch_observed_value:

  > 2026-09-30T08:14:27Z
- patch_value_is_rfc3339:

  > True
- note:

  > the SAME field on the SAME resource is serialised two different ways depending on the handler: space-separated on GET, RFC 3339 on PATCH. `date-time` requires RFC 3339.
- requests:

  > ['GET /api/v1/sessions/68ed0699-367d-4206-9268-25ebab753dcb', 'PATCH /api/v1/sessions/68ed0699-367d-4206-9268-25ebab753dcb']

### T1-F2 — GET /api/v1/approvals/{approvalId} returns created_at as the zero time

- severity: **medium**
- declared:

  > components.schemas.Approval.created_at = {type: string, format: date-time}
- response_body:

  > {"id":"t1-perm-0002","session_id":"68ed0699-367d-4206-9268-25ebab753dcb","iteration":2,"request_type":"destructive_action","description":"t1 sweep pending approval","risk_level":"high","status":"pending","created_at":"0001-01-01T00:00:00Z"}
- db_value:

  > 2026-09-30 08:14:17  (SELECT created_at FROM approval_requests WHERE id='t1-perm-0002')
- note:

  > the stored value is present and non-null; the read path drops it to 0001-01-01T00:00:00Z.

### T1-F3 — POST /api/v1/approvals/{approvalId}/review writes a non-parseable reviewed_at (Go time.String with the monotonic reading)

- severity: **medium**
- response_body:

  > {"id":"t1-perm-0002","session_id":"68ed0699-367d-4206-9268-25ebab753dcb","iteration":2,"request_type":"destructive_action","description":"t1 sweep pending approval","risk_level":"high","status":"approved","reviewer_id":"admin","created_at":"0001-01-01T00:00:00Z","reviewed_at":"0001-01-01T00:00:00Z"}
- db_value:

  > 2026-09-30 03:14:27.257223769 -0500 -05 m=+179.071315254  (SELECT reviewed_at FROM approval_requests WHERE id='t1-perm-0002')
- note:

  > reviewed_at is written with Go's time.Time.String(), including the 'm=+…' monotonic suffix. It is not RFC 3339 and is not parseable by the declared date-time format.

## 8. Observations (not contract violations)

### T1-O1 — 8 operations answer 2xx without any credential

- health/doc/global/health/mcp declare `security: []` and are intentionally public. The four /instance operations inherit the document-level BearerAuth[] requirement yet serve 2xx with no credential — a posture question for security review, not a status-code mismatch.

### T1-O2 — POST /mcp rejects Bearer auth and demands an MCP-specific credential channel

- the spec declares document-level `BearerAuth` (so the operation lists 401 among its documented statuses), but a Bearer header is refused: the JSON-RPC error answers -32000 'API key missing from the MCP initialize request — provide it via --api-key or CONSENSUS_API_KEY (the MCP client forwards it in _meta.authorization)'. A generic OpenAPI client (e.g. muster, which drives services from their own spec) cannot authenticate against /mcp using the declared scheme.
- evidence: `{"request": "POST /mcp", "documented_statuses": [200, 202, 400, 401, 405, 501], "observed_status": 401, "response_body": "{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"code\":-32000,\"message\":\"Authentication required\",\"data\":\"API key missing from the MCP initialize request \u2014 provide it via --api-key or CONSENSUS_API_KEY (the MCP client forwards it in _meta.authorization)\"}}\n"}`

### T1-O3 — POST /permission//resolve (empty path parameter) answers 307, not 4xx

- chi's redirect for a trailing/empty segment. Harmless, recorded for completeness: the sweep records it as INFO because the concrete URL is not a declared operation.
- evidence: `{"request": "POST /permission//resolve", "observed_status": 307}`

## 9. Skips (each null carries a reason)

| operation | reason | status recorded |
|---|---|---|
| `GET /api/v1/skills/{skillName} (200 branch)` | skills_registry is empty on a fresh instance (GET /api/v1/skills -> []) so no SkillName exists to address; only the 404 branch was exercised. | 404 |
| `POST /api/v1/tools/{toolName}/execute (200 branch)` | tools_registry is empty on a fresh instance (GET /api/v1/tools -> []) so no ToolName exists to execute; only the 400 (unknown tool) branch was exercised. | 400 |
| `POST /api/v1/quarantine/{qID}/approve and /reject (200 branch)` | external_quarantine has no rows on a fresh instance (GET /api/v1/quarantine -> {count: 0}); only the 400 (bad id) branch was exercised. | 400 |
| `GET /api/v1/sessions/{sessionId}/memory/{memoryId} (200 branch)` | the fixture session has no memory rows; the addressed memoryId is a synthetic UUID, so only the 404 branch was exercised. | 404 |
| `POST /api/v1/sessions/{sessionId}/message (LLM round trip)` | the documented 200 was reached and verified ({'session': ..., 'status': 'message_received'}). The downstream LLM CALL is out of T1 scope and was not made: the server was started with a dummy DEEPSEEK_API_KEY so session creation passes its configuration gate without any live provider traffic (T2 owns live-LLM assertions). | 200 |
| `SSE endpoints GET /api/v1/events, GET /global/event, GET /event` | status and Content-Type were recorded; the body is an open stream and was deliberately not drained. Frame-level assertions belong to C-196/C-244. | 200 |

## 10. Per-path matrix

Status is the authenticated probe (minimal request; `{}` body for write methods). `noauth` is the same request with
no credential. `match` compares the observed status against the `responses` keys of the served `/openapi.json`.

Full machine-readable record: `t1-openapi-sweep-2026-09-30.json` (every path, every operation, response bodies).

| path | method | documented | status | match | noauth | envelope |
|---|---|---|---|---|---|---|
| `/agent` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/api/v1/approvals` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/approvals/{approvalId}` | GET | `[200, 401, 403, 404, 500]` | 404 | yes | 401 | ok |
| `/api/v1/approvals/{approvalId}/review` | POST | `[200, 400, 401, 403, 404, 409, 500]` | 400 | yes | 401 | ok |
| `/api/v1/auth/keys` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/auth/keys` | POST | `[201, 400, 401, 403, 500]` | 400 | yes | 401 | ok |
| `/api/v1/auth/keys/{keyId}` | DELETE | `[200, 401, 403, 404, 500]` | 404 | yes | 401 | ok |
| `/api/v1/config` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/events` | GET | `[200, 401, 403]` | 200 | yes | 401 | - |
| `/api/v1/health` | GET | `[200]` | 200 | yes | 200 | - |
| `/api/v1/metrics` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/quarantine` | GET | `[200, 401, 403, 500, 503]` | 200 | yes | 401 | - |
| `/api/v1/quarantine/{qID}/approve` | POST | `[200, 400, 401, 403, 500, 503]` | 400 | yes | 401 | ok |
| `/api/v1/quarantine/{qID}/reject` | POST | `[200, 400, 401, 403, 500, 503]` | 400 | yes | 401 | ok |
| `/api/v1/sessions` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions` | POST | `[201, 400, 401, 403, 500]` | 400 | yes | 401 | ok |
| `/api/v1/sessions/{sessionId}` | GET | `[200, 401, 403, 404, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}` | PATCH | `[200, 400, 401, 403, 404, 409, 500]` | 400 | yes | 401 | ok |
| `/api/v1/sessions/{sessionId}` | DELETE | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/approvals` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/billing` | GET | `[200, 401, 403, 404, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/context` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/iterations` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/memory` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/memory/{memoryId}` | GET | `[200, 401, 403, 404, 500]` | 404 | yes | 401 | ok |
| `/api/v1/sessions/{sessionId}/message` | POST | `[200, 400, 401, 403, 404, 500]` | 400 | yes | 401 | ok |
| `/api/v1/sessions/{sessionId}/tasks` | GET | `[200, 401, 403, 500]` | 200 | yes | 401 | - |
| `/api/v1/sessions/{sessionId}/tasks` | POST | `[201, 400, 401, 403, 500]` | 400 | yes | 401 | ok |
| `/api/v1/skills` | GET | `[200, 500]` | 200 | yes | 401 | - |
| `/api/v1/skills/{skillName}` | GET | `[200, 404, 500]` | 404 | yes | 401 | ok |
| `/api/v1/tasks/{taskId}` | GET | `[200, 401, 403, 404, 500]` | 200 | yes | 401 | - |
| `/api/v1/tasks/{taskId}` | PATCH | `[200, 400, 401, 403, 404, 409, 500]` | 400 | yes | 401 | ok |
| `/api/v1/tasks/{taskId}/claim` | POST | `[200, 401, 404, 409, 500]` | 409 | yes | 401 | ok |
| `/api/v1/tools` | GET | `[200, 500]` | 200 | yes | 401 | - |
| `/api/v1/tools/{toolName}/execute` | POST | `[200, 400, 401, 403, 404, 409, 500]` | 400 | yes | 401 | ok |
| `/auth/{authId}` | PUT | `[200, 400, 401, 405]` | 200 | yes | 401 | - |
| `/config` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/config` | PATCH | `[200, 400, 401]` | 200 | yes | 401 | - |
| `/config/providers` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/doc` | GET | `[200]` | 200 | yes | 200 | - |
| `/event` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/experimental/tool` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/experimental/tool/ids` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/file/content` | GET | `[200, 400, 401, 500]` | 400 | yes | 401 | ok |
| `/file/status` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/find` | GET | `[200, 400, 401, 500]` | 400 | yes | 401 | ok |
| `/find/file` | GET | `[200, 400, 401, 500]` | 400 | yes | 401 | ok |
| `/find/symbol` | GET | `[501]` | 501 | yes | 401 | ok |
| `/global/event` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/global/health` | GET | `[200]` | 200 | yes | 200 | - |
| `/instance` | GET | `[200]` | 200 | yes | 200 | - |
| `/instance/path` | GET | `[200]` | 200 | yes | 200 | - |
| `/instance/vcs` | GET | `[200]` | 200 | yes | 200 | - |
| `/instance/vcs/diff` | GET | `[200]` | 200 | yes | 200 | - |
| `/lsp` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/mcp` | GET | `[200]` | 200 | yes | 200 | - |
| `/mcp` | POST | `[200, 202, 400, 401, 405, 501]` | 400 | yes | 400 | ok |
| `/permission` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/permission/{permissionId}` | GET | `[200, 401, 404]` | 404 | yes | 401 | ok |
| `/permission/{permissionId}/resolve` | POST | `[200, 400, 401, 404]` | 400 | yes | 401 | ok |
| `/project` | GET | `[401, 501]` | 501 | yes | 401 | ok |
| `/project/{projectId}` | GET | `[501]` | 404 | **NO** | 401 | **FAIL** |
| `/provider` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/session` | GET | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/session` | POST | `[200, 400, 401, 500]` | 200 | yes | 401 | - |
| `/session/{sessionId}` | GET | `[200, 401, 404]` | 200 | yes | 401 | - |
| `/session/{sessionId}` | PATCH | `[200, 400, 401]` | 400 | yes | 401 | ok |
| `/session/{sessionId}` | DELETE | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/session/{sessionId}/abort` | POST | `[200, 401, 500]` | 200 | yes | 401 | - |
| `/session/{sessionId}/children` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/session/{sessionId}/message` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/session/{sessionId}/message` | POST | `[200, 400, 401, 404]` | 400 | yes | 401 | ok |
| `/session/{sessionId}/message/{messageId}` | GET | `[200, 401, 404]` | 404 | yes | 401 | ok |
| `/tui/{action}` | GET | `[404, 501]` | 404 | yes | 401 | ok |
| `/vcs` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/vcs/diff` | GET | `[200, 401]` | 200 | yes | 401 | - |
| `/vcs/{vcsId}` | GET | `[501]` | 501 | yes | 401 | ok |
| `/webhooks/{source}` | POST | `[200, 202, 403, 404, 413, 429, 500]` | 404 | yes | 404 | ok |

## 11. Reproduce

```bash
cd /home/kara/consensus
go build -o /tmp/consensus-test-bin ./cmd/consensus
mkdir -p /tmp/t1-sweep/scratch && cd /tmp/t1-sweep/scratch
/tmp/consensus-test-bin init --db-url 'sqlite:///tmp/t1-sweep/scratch/t1.db?_journal_mode=WAL'   # prints the admin key once
DEEPSEEK_API_KEY=t1-sweep-dummy-not-a-live-key \
  /tmp/consensus-test-bin serve --port 8099 --hostname 127.0.0.1 \
  --db-url 'sqlite:///tmp/t1-sweep/scratch/t1.db?_journal_mode=WAL' &
curl -s http://127.0.0.1:8099/openapi.json -o /tmp/t1-sweep/openapi.json
python3 /tmp/t1-sweep/sweep.py   http://127.0.0.1:8099 <admin-key> /tmp/t1-sweep/fixtures.json /tmp/t1-sweep/raw.ndjson
python3 /tmp/t1-sweep/probes2.py http://127.0.0.1:8099 <admin-key> <session-id> /tmp/t1-sweep/probes2.json
python3 /tmp/t1-sweep/probes3.py http://127.0.0.1:8099 <admin-key> <session-id> /tmp/t1-sweep/probes3.json
```

The sweep is driven from the served `/openapi.json`, so it follows the contract wherever it grows — the path list is
never hand-enumerated.
