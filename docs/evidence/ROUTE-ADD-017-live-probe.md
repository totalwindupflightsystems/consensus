# ROUTE-ADD-017 — POST /api/integration/attempt/{attemptID}/complete live probe

ch:trace row=ROUTE-ADD-017
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.complete
  test=internal/shim/opencode/api_integration_ra017_test.go
  evidence=docs/evidence/ROUTE-ADD-017-live-probe.md
  witness=none:unattended-worker-session — scripted curls against a locally
    built binary and a fresh scratch SQLite database.

Date: 2026-10-14T12:27:45-05:00
Worktree: `/home/kara/worktrees/consensus-ROUTE-ADD-017`
Branch/base: `wt/ROUTE-ADD-017` at base `c280196c2e6e826ad2a61a450c703852ff8fcaf4`

## Run identity

| Property | Value |
|---|---|
| Binary | `/tmp/consensus-ra017` |
| Build | `go build -o /tmp/consensus-ra017 ./cmd/consensus` |
| Binary SHA-256 | `ceb4413970ecb70811ad6fdb12348b000376587fb68ab294774330a16cea0d8f` |
| Listener | `127.0.0.1:18617` (preflight confirmed free; released after probe) |
| Database | fresh scratch SQLite `/tmp/ra017-live/consensus.db`, initialized by the built binary |
| Config | explicit scratch `/tmp/ra017-consensus.yaml`; no ambient repository config |
| Authentication | bootstrap admin bearer captured from init output into a shell variable; the raw value was never printed or copied here |
| Transcript | `/tmp/ra017-live/transcript.txt`; response bodies under `/tmp/ra017-live/` |

## Declared contract

Pinned source: `specs/openapi/upstream/openapi-1.18.33.json`, operationId
`v2.integration.attempt.complete`, source item `SHIM-DRIFT-004`.

- `POST /api/integration/attempt/{attemptID}/complete`
- required JSON object body; only optional property `code` of type string;
  `additionalProperties: false`
- optional deepObject `location` query selector with `directory` and `workspace`
- declared responses: `204 No Content`, `400 InvalidRequestError`,
  `401 UnauthorizedError`

Before ROUTE-ADD-017 the exact path had no shim route and net/http answered the
default 404. The new route is exact and parameterized; it does not add an
`/api/integration/*` catch-all.

## Live results

The probe drove the real `consensus serve` wiring after `consensus init` created
the scratch database and bootstrap key. This is the transcript copied verbatim:

```text
health=200
unauthenticated_post=401 envelope={"_tag":"UnauthorizedError","message":"invalid credentials"}
authenticated_post=204 body_bytes=0
wrong_method_get=405 envelope={"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}
malformed_code=400 envelope={"_tag":"InvalidRequestError","field":"code","kind":"Body","message":"field \"code\" must be a string"}
LIVE_PROBE_PASS
```

Assertions:

| Arm | Observed | Result |
|---|---|---|
| Unauthenticated `POST` with `{}` | `401`, declared `UnauthorizedError` body | PASS |
| Authenticated `POST` with `{"code":"probe-code"}` | `204`, response body exactly 0 bytes | PASS |
| Authenticated undeclared `GET` | `405`, sibling `METHOD_NOT_ALLOWED` envelope | PASS |
| Authenticated malformed `code` number | `400`, declared `InvalidRequestError` naming `code` | PASS |

## Truthfulness

Consensus has no integration-attempt table, no integration-credential table,
and no OAuth integration-attempt producer. The handler therefore does not
store, exchange, or echo the supplied code and does not fabricate an attempt or
credential row. A well-formed completion is an idempotent no-op acknowledged by
the declared empty 204, matching the store-less `v2.credential.remove`
precedent. Malformed request shapes are refused with the declared 400 rather
than silently accepted.

## Contract artifact

The served-surface row records `outcome: 204` and the comparator was regenerated
with:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/consensus-route-add-017-compare
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`. The target
operation is now in `covered` with served outcome `204`; no target row remains
in `drift_declared_not_served`.
