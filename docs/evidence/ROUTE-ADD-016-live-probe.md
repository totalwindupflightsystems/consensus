# ROUTE-ADD-016 — GET /api/integration/attempt/{attemptID} live probe

## Run identity

| Field | Value |
|---|---|
| Source branch | `wt/ROUTE-ADD-016` |
| Base commit | `c280196c2e6e826ad2a61a450c703852ff8fcaf4` |
| Binary | `/tmp/consensus-ra016`, built from the ROUTE-ADD-016 working tree with `go build -o /tmp/consensus-ra016 ./cmd/consensus` |
| Binary SHA-256 | `81a56d39766b655a22ac153b44e17b86209c17400e0103f42f152131618643e8` |
| Go toolchain | `go1.26.5 linux/amd64` |
| Listener | `127.0.0.1:38016` (checked free before start) |
| Database | scratch SQLite at `/tmp/consensus-ra016-probe-a/consensus.db` |
| Credential | scratch bootstrap admin key; the raw value is not recorded here |

The scratch instance was initialized with the production binary and an explicit scratch config, then served with the opencode adapter. The server was stopped after the probe.

## Declared contract

Pinned source: `specs/openapi/upstream/openapi-1.18.33.json`, operation `v2.integration.attempt.status`:

- `GET /api/integration/attempt/{attemptID}`.
- Required path parameter `attemptID` (plain string, no declared pattern).
- Optional deepObject `location` query selector with only `directory` and `workspace` members (`additionalProperties: false`).
- Responses: `200`, `400 InvalidRequestError`, `401 UnauthorizedError`.
- The success envelope requires `location` and `data`; `location` is the selected runtime workspace.

## RED proof

Before the handler and route registration were added, the focused battery was run:

`go test -short -count=1 ./internal/shim/opencode/ -run 'TestApiIntegrationAttemptStatus'`

Observed failures pinned the original absence:

- well-formed GET: `got 404, want 200. Body: 404 page not found`;
- every malformed-input arm: `got 404, want declared 400`;
- POST, PUT, PATCH and DELETE: `got 404, want 405`;
- unauthenticated GET used the generic shim `UNAUTHENTICATED` envelope instead of the declared v2 `UnauthorizedError` body;
- the mount test reported `/api/integration/attempt/{attemptID} is not registered in MountPatterns`.

This is the comparator's original `SHIM-DRIFT-003` `NOT-SERVED` state: no shim route existed.

## GREEN live probe

| Arm | Request | Observed |
|---|---|---|
| Declared 401 | unauthenticated `GET /api/integration/attempt/att_live_probe` | `401 {"_tag":"UnauthorizedError","message":"invalid credentials"}` |
| Declared 200 | authenticated `GET /api/integration/attempt/att_live_probe` | `200 {"data":[],"location":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-016","project":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-016","id":"consensus-a41a4eb883c3"}}}` |
| Method guard | authenticated `POST /api/integration/attempt/att_live_probe` | `405 {"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}` |
| Declared 400 | authenticated GET with `location[workspace]=ses_bad` | `400 {"_tag":"InvalidRequestError","field":"location[workspace]","kind":"Query","message":"location[workspace] must start with wrk"}` |

The probe asserted the status codes and decoded each body before recording the result. The 200 body was additionally checked to contain exactly `data` and `location`, an empty non-null `data` array, and non-empty real `location.directory`, `location.project.id`, and `location.project.directory` values.

## Truthfulness

Consensus has no persisted OAuth integration-attempt store: no runtime row can support a pending, complete, failed, or expired attempt with the required status and created/expires timestamps. The handler therefore emits no attempt row. Its success response carries the real selected workspace through the shared v2 location resolver and the task-required empty, non-null `data` array; it never fabricates an attempt identifier, status, timestamp, or message. A non-empty result is reserved for a future real store whose row can be expressed by the upstream schema.

Malformed location selectors and a blank path identifier answer the declared `InvalidRequestError` 400. Authentication remains in the existing `/api/*` wall and answers the declared v2 401 body before the handler runs.

## Declared-vs-served reconciliation

The served-surface row was added and the comparator was run exactly as documented:

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/ROUTE-ADD-016-comparator-final-b
```

`/tmp/ROUTE-ADD-016-comparator-final-b/declared-vs-served.json` was copied byte-for-byte to `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` and verified with `cmp`.

- `served_route_entries`: 136 → 138. Two truthful entries describe the one exact handler: the implemented GET outcome and the still-unimplemented declared sibling DELETE's observed 405.
- `covered_operations`: 96 → 97.
- `drift_declared_not_served`: 73 → 72.
- The sole covered-set addition is `GET /api/integration/attempt/{attemptID}` / `v2.integration.attempt.status`.
- The target operation is absent from the regenerated drift array.
- The declared sibling `DELETE /api/integration/attempt/{attemptID}` remains unimplemented and changes classification from `NOT-SERVED` to `OUTCOME-MISMATCH`: this task intentionally enforces 405 on non-GET, and the served-surface row records that observed outcome without claiming the sibling's declared 204.
- `METHOD-MISSING`, `ROUTED-404`, and `STUB-501` remain absent from the drift classes; `drift_served_not_declared` and `narrowed_unexplained` remain 0.
