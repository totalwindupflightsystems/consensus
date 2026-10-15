# ROUTE-ADD-020 — POST /api/integration/{integrationID}/connect/oauth live probe

ch:trace row=ROUTE-ADD-020
  source=SHIM-DRIFT-016
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.oauth
  test=internal/shim/opencode/api_integration_connect_oauth_test.go
  evidence=docs/evidence/ROUTE-ADD-020-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-15T08:34:09-05:00
Branch: `wt/ROUTE-ADD-020`, base commit `951448d654957aa53c1f91d303bd3480416c5230`.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra020` |
| Binary SHA-256 | `db21ffc323be8973824c1e8d073127fc23a73196254aa40eea1079ab88de11a3` |
| Listener | `127.0.0.1:52284` |
| Database | scratch SQLite `/tmp/consensus-ra020-live.J0FvZT/consensus.db` |
| Config | explicit `/tmp/consensus-ra020-live.J0FvZT/consensus.yaml` |
| Credential | bootstrap admin key captured in the scratch directory; raw value not recorded |
| Server log | `/tmp/consensus-ra020-live.J0FvZT/serve.log` |

The binary was built from this worktree with:

```text
go build -o /tmp/consensus-ra020 ./cmd/consensus
```

The scratch database was initialized with `consensus init`; the same binary was
then started with `consensus serve --adapter opencode`. The server was stopped
after the probe and the listener was verified closed.

## Contract exercised

The pinned upstream document declares `POST
/api/integration/{integrationID}/connect/oauth` as operation
`v2.integration.connect.oauth`. It requires a string `integrationID`, the
optional deepObject `location` selector, and one JSON object body:
`{methodID: string, inputs: object<string,string>, label?: string}`. Declared
responses are 200, 400 and 401.

The 200 body requires exactly `{location: LocationInfo, data:
IntegrationAttempt}`. `IntegrationAttempt` requires `attemptID`, `url`,
`instructions`, `mode`, and `time {created, expires}`.

## RED proof

Before the handler and registration existed, the focused test did not compile:

```text
internal/shim/opencode/api_integration_connect_oauth_test.go:137:4: s.handleApiIntegrationConnectOauth undefined (type *Server has no field or method handleApiIntegrationConnectOauth)
FAIL github.com/wojons/consensus/internal/shim/opencode [build failed]
```

Command:

```text
go test ./internal/shim/opencode/ -run 'RA020|ConnectOauth' -count=1
```

## Live results

The scratch server produced these asserted statuses:

```text
READINESS=200
AUTHENTICATED_POST=200
MISSING_METHOD_ID=400
UNAUTHENTICATED_POST=401
WRONG_METHOD_GET=405
```

The authenticated success body was:

```json
{"data":{"attemptID":"","instructions":"","mode":"","time":{"created":0,"expires":0},"url":""},"location":{"directory":"/tmp/consensus-ra020-live.J0FvZT","project":{"directory":"/tmp/consensus-ra020-live.J0FvZT","id":"consensus-00880e66abad"}}}
```

The response passed assertions for the exact top-level, `data`, and `time`
members; the selected location directory matched the real scratch workspace;
and every IntegrationAttempt value was empty or zero.

The malformed body answer used the declared 400 envelope:

```json
{"_tag":"InvalidRequestError","field":"methodID","kind":"Body","message":"required field \"methodID\" is missing"}
```

The unauthenticated answer used the declared v2 envelope:

```json
{"_tag":"UnauthorizedError","message":"invalid credentials"}
```

The wrong-method answer retained the sibling shim convention:

```json
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
```

## Truthfulness

Consensus has no integration registry, OAuth provider runtime, or
IntegrationAttempt store. The handler therefore reports only what the runtime
knows: the real selected location. It does not echo `integrationID`, `methodID`,
`inputs`, or `label` as created state. Every required IntegrationAttempt member
is present but empty/zero. In particular, `mode` is empty: selecting either
`auto` or `code` would falsely claim a mode for an OAuth attempt that was never
created. No attempt ID, authorization URL, instructions, or timestamps are
invented.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/consensus-ra020-compare-3842194
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`. The stable
identity `/api/integration/{integrationID}/connect/oauth` + POST +
`v2.integration.connect.oauth` moved from `drift_declared_not_served` to
`covered`; no other stable operation identity moved in either direction.
Counts changed from 101 to 102 covered operations and from 68 to 67
`drift_declared_not_served` operations. The generated target records
`served_outcome: 200` and declared responses `200,400,401`.

## Verification

- Focused RED proof: PASS — missing handler failed compilation as expected.
- Focused GREEN: `go test ./internal/shim/opencode/ -run 'RA020|ConnectOauth' -count=1` — PASS (`0.011s`).
- Whole package: `go test ./internal/shim/opencode/ -count=1` — PASS (`10.562s`).
- `go build ./...` — PASS.
- `go vet ./...` — PASS.
- `gofmt -l` on the two new Go files and `server.go` — empty.
- Scratch init + serve + curl probe: PASS for 200, 400, 401 and 405 arms.
