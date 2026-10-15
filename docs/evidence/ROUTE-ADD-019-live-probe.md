# ROUTE-ADD-019 — POST /api/integration/{integrationID}/connect/key live probe

ch:trace row=ROUTE-ADD-019
  source=SHIM-DRIFT-015
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.connect.key
  test=internal/shim/opencode/api_integration_connect_key_test.go
  evidence=docs/evidence/ROUTE-ADD-019-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-15T08:37:07-05:00
Branch: `wt/ROUTE-ADD-019`, base commit `951448d654957aa53c1f91d303bd3480416c5230`.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra019` |
| Binary SHA-256 | `4bcacf86de0a5efcc11bb3e6e53b1ecdb535177c7e373680904979c5e0d4682c` |
| Listener | `127.0.0.1:18619` (verified free before start; no listener remained after shutdown) |
| Database | scratch SQLite `/tmp/consensus-ra019-live.V46lmE/consensus.db` (491520 bytes after probe) |
| Config | explicit `/tmp/consensus-ra019-live.V46lmE/consensus.yaml` |
| Credential | bootstrap admin key captured to a mode-600 scratch file; raw value was not logged or copied here |
| Server log | `/tmp/consensus-ra019-live.V46lmE/serve.log` |
| Transcript | `/tmp/consensus-ra019-live.V46lmE/probe-transcript.txt` (359 bytes) |

The binary was built from this worktree with:

```text
go build -o /tmp/consensus-ra019 ./cmd/consensus
```

The scratch database was initialized with `consensus init`, using the explicit
config and `sqlite:///tmp/consensus-ra019-live.V46lmE/consensus.db`, before the
same binary was started with `consensus serve --adapter opencode`.

## Contract exercised

The pinned upstream document declares `POST
/api/integration/{integrationID}/connect/key` as operation
`v2.integration.connect.key`, with a required string `integrationID`, optional
deepObject `location[directory]` / `location[workspace]`, and a required
additionalProperties:false JSON object `{key: string, label?: string}`. The
declared responses are 204 No Content, 400 InvalidRequestError, and 401
UnauthorizedError.

ROUTE-ADD-019 also requires a missing or whitespace-only `key` to use the
declared 400 arm. An undeclared method follows the shim's sibling route
convention and answers 405 METHOD_NOT_ALLOWED.

## RED proof

Before the handler and route registration existed, the focused test command:

```text
go test ./internal/shim/opencode/ -run 'RA019|ConnectKey' -count=1
```

reproduced the production gap:

```text
POST /api/integration/github/connect/key: got 404, want 204. Body: 404 page not found
missing key: got 404, want 400. Body: 404 page not found
blank key: got 404, want 400. Body: 404 page not found
non-JSON body: got 404, want 400. Body: 404 page not found
/api/integration/{integrationID}/connect/key is not registered in MountPatterns
unauthenticated POST returned the shim UNAUTHENTICATED envelope instead of UnauthorizedError
```

## Live results

The scratch server produced and asserted this transcript:

```text
READINESS=200
UNAUTHENTICATED_POST=401 BODY={"_tag":"UnauthorizedError","message":"invalid credentials"}
AUTHENTICATED_POST=204 BODY_BYTES=0
MISSING_KEY_POST=400 BODY={"_tag":"InvalidRequestError","field":"key","kind":"Body","message":"required field \"key\" is missing"}
WRONG_METHOD_GET=405 BODY={"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
```

The probe asserted each status, asserted the 204 body was zero bytes, and parsed
the 400, 401, and 405 bodies with `jq` to verify their contract tags/codes.

## Truthfulness

Consensus keeps no integration registry and no credential-connect runtime. The
handler validates the credential-shaped request but does not retain its key or
label. The declared empty 204 is therefore the truthful accepted no-op: no
IntegrationAttempt or credential row is created, and the requested
`integrationID` is not echoed as a fabricated object. The 200 data-bearing
success shape belongs to `v2.integration.connect.oauth` (ROUTE-ADD-020), not
this operation.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/consensus-ra019-compare-3855746
```

Its generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`. The stable
source identity — path `/api/integration/{integrationID}/connect/key`, method
POST, operationId `v2.integration.connect.key` — is present in `covered` with
served outcome 204 and absent from `drift_declared_not_served`.

Counts changed only as expected: served route entries 142 to 143, covered
operations 101 to 102, declared-not-served drift 68 to 67, and NOT-SERVED 64 to
63. Drift IDs are positional: although the board row cites the historical
source item `SHIM-DRIFT-015`, this base artifact labeled the same stable
path/method/operation tuple `SHIM-DRIFT-001` before regeneration.

## Verification

- Focused RED proof: PASS (missing route reproduced as 404, wrong 401 envelope, and absent mount).
- Focused GREEN proof: `go test ./internal/shim/opencode/ -run 'RA019|ConnectKey' -count=1` — PASS.
- Package tests: `go test ./internal/shim/opencode/ -count=1` — PASS (13.342s after updating ROUTE-ADD-018's now-stale sibling-path 404 assertion to the new exact-route 405).
- Live init + serve + curl probe: PASS for declared 401, empty-body 204, declared 400, and wrong-method 405.
- Scratch server shutdown: PASS; `ss` showed no listener remaining on port 18619.
