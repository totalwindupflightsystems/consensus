# ROUTE-ADD-014 — GET /api/integration live probe

ch:trace row=ROUTE-ADD-014
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.list
  test=internal/shim/opencode/api_integration_list_test.go
  evidence=docs/evidence/ROUTE-ADD-014-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-14T12:25:31-05:00
Branch: `wt/ROUTE-ADD-014`
Base commit: `c280196c2e6e826ad2a61a450c703852ff8fcaf4`

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra014` |
| Binary SHA-256 | `f4bc55dbd15fa298d16672eee0320883e7859f2695bcfdaf64e5f77001b3d1bd` |
| Build command | `go build -o /tmp/consensus-ra014 ./cmd/consensus` |
| Listener | `127.0.0.1:18614` (verified free before start) |
| Database | scratch SQLite `/tmp/consensus-ra014-live-2/consensus.db` |
| Config | explicit scratch config `/tmp/consensus-ra014-live-2/consensus.yaml` |
| Initialization | `/tmp/consensus-ra014 --config ... init --db-url sqlite:///tmp/consensus-ra014-live-2/consensus.db` (exit 0; embedded migrations applied) |
| Server | `/tmp/consensus-ra014 --config ... serve --db-url sqlite:///tmp/consensus-ra014-live-2/consensus.db --hostname 127.0.0.1 --port 18614 --adapter opencode --log-level warn` |
| Credential | bootstrap admin key read from the scratch init log; raw value is not recorded |
| Transcript | `/tmp/consensus-ra014-live-2/probe-transcript.txt` |

The readiness check called `GET /global/health`, required HTTP success, and
asserted `.healthy == true` before any route probe.

## Declared contract

The pinned upstream document declares:

- `GET /api/integration`
- operationId `v2.integration.list`
- optional deepObject `location` query parameter with only
  `location[directory]` and `location[workspace]`
- `200` body `{location: LocationInfo, data: IntegrationInfo[]}`
- `400` body `InvalidRequestError`
- `401` body `UnauthorizedError`

`IntegrationInfo` requires all four properties: `id`, `name`, `methods`, and
`connections`; `additionalProperties` is false.

## RED regression arm

Before the handler and route registrations were added, the focused regression
battery failed against base `c280196` with the production behavior:

| Probe | Observed before fix | Required |
|---|---:|---:|
| auth-bypassed handler-tree `GET /api/integration` | 404 `404 page not found` | 200 |
| auth-bypassed malformed location selectors | 404 `404 page not found` | 400 |
| unauthenticated `GET /api/integration` | 401 legacy `UNAUTHENTICATED` envelope | 401 declared `UnauthorizedError` |
| wrong methods | 404 `404 page not found` | 405 `METHOD_NOT_ALLOWED` |
| `MountPatterns` membership | absent | exact path present |

Command: `go test -short -count=1 -run '^TestApiIntegrationList' ./internal/shim/opencode/`.

## Live GREEN probe

The locally built binary was run against the initialized scratch database. The
probe asserted status and JSON shape, not status alone:

```text
unauthenticated_get=401 body={"_tag":"UnauthorizedError","message":"invalid credentials"}
authenticated_get=200 body={"data":[],"location":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-014","project":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-014","id":"consensus-efde1bc1a4ab"}}}
authenticated_post=405 body={"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
assertions=PASS
```

Assertions:

1. Unauthenticated GET is exactly the declared `UnauthorizedError` object
   (`_tag` and `message`, no undeclared members).
2. Authenticated GET is exactly the declared top-level shape (`data` and
   `location`), `data` is an empty non-null array, and the location carries a
   non-empty real directory plus project id/directory.
3. Authenticated POST on the exact path is 405 with the sibling
   `METHOD_NOT_ALLOWED` envelope and `use GET` message.

## Truthfulness

The runtime keeps no integration registry or connection store. Provider-auth
routes do not retain an upstream `IntegrationInfo` row with the required
identity, authentication-method descriptors, and connections. A non-empty row
would therefore fabricate runtime state. The declared 200 is served with the
real selected workspace and `data: []`; no integration identity or connection
is invented.

Malformed deepObject selectors are tested separately and answer the declared
400 through the shared `apiV2LocationSelector` contract.

## Declared-vs-served artifact

Comparator command:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/consensus-route-add-014-compare-1
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` and verified
with `cmp -s`.

| Measure | Before | After |
|---|---:|---:|
| served route entries | 136 | 137 |
| covered operations | 96 | 97 |
| declared-not-served drift | 73 | 72 |
| NOT-SERVED drift | 69 | 68 |

A `(path, method, operationId)` key-set diff shows exactly one move:
`GET /api/integration` / `v2.integration.list` was removed from
`drift_declared_not_served` (source item `SHIM-DRIFT-001`) and added to
`covered`. No other operation changed bucket.
