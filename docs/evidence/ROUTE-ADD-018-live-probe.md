# ROUTE-ADD-018 — GET /api/integration/{integrationID} live probe

ch:trace row=ROUTE-ADD-018
  source=SHIM-DRIFT-005
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.get
  test=internal/shim/opencode/api_integration_ra018_test.go
  evidence=docs/evidence/ROUTE-ADD-018-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-14T12:26:47-05:00
Branch: `wt/ROUTE-ADD-018`, base commit `c280196c2e6e826ad2a61a450c703852ff8fcaf4`.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra018` |
| Binary SHA-256 | `51f7fdeef51894888462030e6ed9736a121eb3e264c1f987a738099b93adf31e` |
| Listener | `127.0.0.1:52283` |
| Database | scratch SQLite `/tmp/consensus-ra018-live.1xnKZN/consensus.db` |
| Config | explicit `/tmp/consensus-ra018-live.1xnKZN/consensus.yaml` |
| Credential | bootstrap admin key captured in scratch output; raw value not recorded |
| Server log | `/tmp/consensus-ra018-live.1xnKZN/serve.log` |

The binary was built from this worktree with:

```text
go build -o /tmp/consensus-ra018 ./cmd/consensus
```

The scratch database was initialized with `consensus init` before the same
binary was started with `consensus serve --adapter opencode`.

## Contract exercised

The pinned upstream document declares `GET
/api/integration/{integrationID}` as operation `v2.integration.get`, with a
required string `integrationID`, the same optional deepObject `location`
selector declared by `v2.integration.list`, and response statuses 200, 400 and
401. ROUTE-ADD-018 requires the list-compatible success envelope
`{location: LocationInfo, data: IntegrationInfo[]}` for the truthful no-store
answer.

The route enforces GET. Malformed `location[directory]` /
`location[workspace]` input uses the declared `InvalidRequestError` 400 helper;
unauthenticated requests rely on the existing `/api/*` auth wall and its
`UnauthorizedError` body.

## RED proof

Before the handler and registration existed, the focused tests reproduced the
production gap:

```text
GET /api/integration/github: got 404, want 200. Body: 404 page not found
GET with declared location selector: got 404, want 200. Body: 404 page not found
/api/integration/{integrationID} is not registered in MountPatterns
401 body = map[error:map[code:UNAUTHENTICATED message:invalid credentials]], want declared UnauthorizedError
```

Command:

```text
go test -short -count=1 -run 'TestApiIntegrationRA018' ./internal/shim/opencode/
```

## Live results

The scratch server produced these asserted statuses:

```text
READINESS=200
UNAUTHENTICATED_GET=401
AUTHENTICATED_GET=200
WRONG_METHOD_POST=405
```

The unauthenticated body was the declared v2 envelope:

```json
{"_tag":"UnauthorizedError","message":"invalid credentials"}
```

The authenticated 200 body passed these structural assertions:

```text
GET_DATA_LENGTH=0
GET_LOCATION_PRESENT=true
GET_TOP_LEVEL_KEYS=["data","location"]
```

The wrong-method response used the sibling ROUTE-ADD-005 envelope:

```json
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
```

## Truthfulness

Consensus has no integration registry: no integration table exists in the
migration set and no runtime component registers rows with the required
`IntegrationInfo` members (`id`, `name`, `methods`, `connections`). The 200
therefore carries the selected real workspace in `location` and an empty,
non-null `data` array. The requested `integrationID` is not echoed into a
fabricated row.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/consensus-ra018-compare.J99BiC
```

Its `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` (`cmp` PASS).
The source item identity — path `/api/integration/{integrationID}`, method GET,
operationId `v2.integration.get` — is present in `covered` with declared
responses `200,400,401` and absent from `drift_declared_not_served`.

Counts changed as expected: served route entries 136 to 137, covered operations
96 to 97, and declared-not-served drift 73 to 72 (NOT-SERVED 69 to 68). Drift
IDs in this generated artifact are positional; after this operation leaves the
list, `SHIM-DRIFT-005` labels the next remaining integration route. The stable
source identity is the method + path + operationId tuple above.

## Verification

- Focused RED proof: PASS (all four tests failed against the missing route for the expected reasons).
- Focused GREEN proof: `go test -short -count=1 -run 'TestApiIntegrationRA018' ./internal/shim/opencode/` — PASS.
- Package tests: `go test -short -count=1 ./internal/shim/opencode/` — PASS (10.652s).
- Package tests: `go test -short -count=1 ./internal/api/` — PASS (1.283s).
- Live init + serve + curl probe: PASS for 401, 200 and 405 arms.
