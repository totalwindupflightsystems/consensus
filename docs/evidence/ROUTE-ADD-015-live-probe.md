# ROUTE-ADD-015 — DELETE /api/integration/attempt/{attemptID} live probe

ch:trace row=ROUTE-ADD-015
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.integration.attempt.cancel
  test=internal/shim/opencode/api_integration_ra015_test.go
  evidence=docs/evidence/ROUTE-ADD-015-live-probe.md
  witness=none:unattended-worker-session

## Run identity

| Field | Value |
|---|---|
| Branch / base | `wt/ROUTE-ADD-015` / `c280196c2e6e826ad2a61a450c703852ff8fcaf4` |
| Binary | `/tmp/consensus-ra015` |
| Binary SHA-256 | `89280b5461fb8958ea0789f415cde1d6050325310aa70c8b3e264b2fb162b552` |
| Listener | `127.0.0.1:33021` |
| Database | scratch SQLite `/var/tmp/hermes-scratch/route-add-015-live-xk5p4p8r/consensus.db` |
| Config | explicit scratch config `/var/tmp/hermes-scratch/route-add-015-live-xk5p4p8r/consensus.yaml` |
| Credential | bootstrap admin key captured to a mode-600 scratch file; raw value was not logged or copied here |
| Transcript | `/var/tmp/hermes-scratch/route-add-015-live-xk5p4p8r/transcript.txt` |

The binary was built from this worktree with `go build -o /tmp/consensus-ra015
./cmd/consensus`. `consensus init` applied the embedded migrations to the scratch
SQLite database, then `consensus serve` ran from the scratch directory with the
explicit config, database URL, hostname, and free port above. The server process
was terminated after the probe.

## Declared contract

The pinned upstream document declares:

- `DELETE /api/integration/attempt/{attemptID}`
- operationId `v2.integration.attempt.cancel`
- required plain-string path parameter `attemptID`
- optional deepObject `location` query selector with only `directory` and
  `workspace` members (`additionalProperties: false`)
- responses `204` No Content, `400` InvalidRequestError, and `401`
  UnauthorizedError

Before ROUTE-ADD-015 there was no shim route. The RED regression run against base
behavior failed with the production answers: happy and malformed requests were
`404 page not found`, the chi mount lacked the pattern, and unauthenticated access
used the generic `UNAUTHENTICATED` envelope instead of the declared v2 401 body.

## Live results

```text
unauthenticated DELETE -> 401 {"_tag":"UnauthorizedError","message":"invalid credentials"}
authenticated DELETE -> 204 body_bytes=0
authenticated GET -> 405 {"error":{"code":"METHOD_NOT_ALLOWED","message":"use DELETE"}}
malformed selector DELETE -> 400 {"_tag":"InvalidRequestError","field":"location[workspace]","kind":"Query","message":"query parameter \"location[workspace]\" must not be blank"}
LIVE_PROBE_PASS
```

The probe asserted each status and envelope with `jq -e`; the authenticated 204
body was also asserted to be zero bytes. The wrong-method request exercised the
exact same concrete path, proving the handler rather than a neighboring 404
answered it.

## Truthfulness

Consensus has no integration OAuth-attempt store and no runtime resource keyed by
the upstream `attemptID`. The declared 204 is therefore an idempotent cancellation
of nothing: it carries no body, fabricates no attempt row, and does not claim that
a stored attempt existed. The handler still validates the declared `location`
selector and lets the existing `/api/*` auth wall answer the declared 401 before
handler dispatch.

## Declared-vs-served result

The served-surface row was added and the comparator was run with the pinned
surface, the committed served-surface YAML, and a scratch output directory. Its
`declared-vs-served.json` was copied byte-for-byte into the committed artifact.
The operation-key diff is exact:

- added to `covered`: `/api/integration/attempt/{attemptID}|DELETE|v2.integration.attempt.cancel`
- removed from `drift_declared_not_served`: the same key
- no other operation key entered or left either bucket
- served route entries: `136 -> 138` (the DELETE success row plus an explicit
  record of the exact path's routed GET 405)
- covered operations: `96 -> 97`
- declared-not-served drift: `73 -> 72`
- the sibling GET `v2.integration.attempt.status` remains drift as
  `OUTCOME-MISMATCH` (`405` vs declared `200,400,401`); recording that actual
  routed outcome keeps the artifact free of the forbidden implicit
  `METHOD-MISSING` class

## Verification

- RED: `go test -short -count=1 ./internal/shim/opencode/ -run '^TestApiIntegrationRA015'` failed on the pre-fix 404, missing mount, and generic 401 envelope.
- GREEN targeted regression: the same command passed.
- `gofmt -l` on all changed Go files: empty / PASS.
- `go build ./...`: PASS.
- `go vet ./internal/shim/opencode/ ./internal/api/`: PASS.
- `go test -short ./internal/shim/opencode/`: PASS.
- `go test -short ./internal/api/`: PASS.
- `git diff --check`: PASS.
- Live scratch SQLite probe: PASS for declared 401, declared empty-body 204, malformed-selector 400, and wrong-method 405.
