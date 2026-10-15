# ROUTE-ADD-023 — GET /api/permission/request (v2.permission.request.list)

ch:trace row=ROUTE-ADD-023
  source=SHIM-DRIFT-019 (id cited by the board row at generation time; see "Stale premise" — the id is positional and no longer labels this operation)
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.permission.request.list
  test=internal/shim/opencode/api_v2_lists_test.go, internal/shim/opencode/declared_vs_served_consistency_test.go
  evidence=docs/evidence/ROUTE-ADD-023-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-15T09:28:00-05:00
Branch: `wt/ROUTE-ADD-023`, base commit `d5a1b695a1b3b84364a088559b6266bcab14730f`.

## Stale premise — the route was already served

The board row asks for a handler on `GET /api/permission/request` because
"When: no shim route at all; net/http default 404". That was true when the row
was generated (2026-09-30). **It is not true at this base commit.** The route
was landed by **ROUTE-ADD-021** (commit `222bc47`, "feat(shim): serve the 7
declared v2 read-only lists per contract") together with six sibling v2 list
operations:

| Layer | Location |
|---|---|
| Route registration | `internal/shim/opencode/server.go:328` — exact pattern, never an `/api/*` catch-all |
| Handler | `internal/shim/opencode/api_v2_lists.go:419` `handleApiPermissionRequestList` |
| Tests | `internal/shim/opencode/api_v2_lists_test.go` (happy path, blank deepObject selector 400, store-failure 400, unauthenticated 401, undeclared-method 405) |
| Served surface | `specs/openapi/upstream/consensus-shim-served-surface.yaml:438` — outcome 200, evidence notes ROUTE-ADD-021 |
| Comparison | `covered` with `served_outcome: 200`; absent from `drift_declared_not_served` |

No handler was written for this row. Writing a second handler for an already
served exact-path pattern would have panicked net/http at mux registration
(duplicate pattern) — the row is a duplicate, and the reason it exists is worth
recording: the artifact's `SHIM-DRIFT-NNN` ids are **positional**, assigned in
row order, so every route closure renumbers the rows after it. ROUTE-ADD-021
retired the `SHIM-DRIFT-018`/`-019` rows it closed, and the ids then shifted onto
different operations — which is why this row and its siblings cite ids that no
longer label the routes they name.

## What the row's third criterion did find

Pass criterion (3) asks that "the declared-vs-served comparison is regenerated".
Regenerating it at this base commit exposed a real, live defect in the committed
artifact — introduced by the parallel ROUTE-ADD-019/-020 wave merge, not by this
row:

* `7ca0e79` ("Merge wt/ROUTE-ADD-020") recorded conflicts on both
  `internal/shim/opencode/server.go` **and**
  `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.
* ROUTE-ADD-019's commit `2c13465` had **removed** the
  `POST /api/integration/{integrationID}/connect/key` row from
  `drift_declared_not_served` when it served that operation.
* The conflict resolution **re-added it**, while keeping the `covered` row
  ROUTE-ADD-019 had added — so the committed artifact recorded one operation as
  *both* served-as-declared (`served_outcome: 204`) *and* "no shim route at all"
  (`class: NOT-SERVED`), and every positional drift id after it shifted by one.

The stale artifact also disagreed with the served surface it is generated from
(`served_route_entries: 143` vs 144 actual; `covered_operations: 102` vs 103
enumerated; `drift_declared_not_served: 67` vs 66 actual).

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/ra023-cmp.pdnkYF
```

Its generated `declared-vs-served.json` (`sha256
f5b9688c154041132e6d5d817b870fe6053de43319e5395d5180d321b68ed9cb`) was copied
**byte-for-byte** to `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`
(verified with `cmp`, identical). Result for the stable identity path
`/api/permission/request` + `GET` + operationId `v2.permission.request.list`:
present in `covered` with `served_outcome: 200`, absent from
`drift_declared_not_served`.

Counts moved only as the removal of the resurrected row implies:

| Count | Stale (HEAD) | Regenerated |
|---|---|---|
| `served_route_entries` | 143 | **144** |
| `covered_operations` | 102 | **103** |
| `drift_declared_not_served` | 67 | **66** |
| `drift_by_class.NOT-SERVED` | 63 | **62** |

## Live probe

Binary `/tmp/consensus-ra023`, sha256
`4d32178f6baebafe6ce536c3065ea9059bf3257a1c59593d507c2828ee84eb4b`, built with
`go build -o /tmp/consensus-ra023 ./cmd/consensus` from this worktree.

| Field | Value |
|---|---|
| Listener | `127.0.0.1:18723` (verified free before start; no listener remained after shutdown) |
| Database | scratch SQLite `/tmp/consensus-ra023-live.LHctJu/consensus.db` (491520 bytes after probe) |
| Config | explicit `/tmp/consensus-ra023-live.LHctJu/consensus.yaml` |
| Credential | bootstrap admin key from `consensus init`, captured to a mode-600 scratch file; raw value not logged or copied here |
| Server log | `/tmp/consensus-ra023-live.LHctJu/serve.log` |
| Transcript | `/tmp/consensus-ra023-live.LHctJu/probe-transcript.txt` |

The credential reaches the shim as `Authorization: Bearer <key>` — the shim's
`validateAuth` accepts a Bearer token (prefix + SHA-256 lookup in `api_keys`) or
Basic auth; it does **not** read `x-api-key`, and a probe that sends the key in
that header gets the declared 401 on every arm.

```text
READINESS=1
UNAUTH_GET=401 BODY={"_tag":"UnauthorizedError","message":"invalid credentials"}
BADKEY_GET=401 BODY={"_tag":"UnauthorizedError","message":"invalid credentials"}
AUTH_GET=200 BODY={"data":[],"location":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-023","project":{"directory":"/home/kara/worktrees/consensus-ROUTE-ADD-023","id":"consensus-bf43a9365950"}}}
AUTH_SELECTOR_GET=200 BODY={"data":[],"location":{"directory":"/home/kara","project":{"directory":"/home/kara","id":"consensus-7e66221cbb27"}}}
BLANK_SELECTOR_GET=400 BODY={"_tag":"InvalidRequestError","field":"location[directory]","kind":"Query","message":"query parameter \"location[directory]\" must not be blank"}
WRONG_METHOD_POST=405 BODY={"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}
```

Every declared arm is exercised: 401 with no credential and with a rejected
credential, 200 with the declared `{location, data}` envelope (and the declared
deepObject `location[directory]` selector honoured — the response directory
follows the selector, proving it is not ignored), and the declared 400
`InvalidRequestError` for the present-but-blank selector. The 405 is the sibling
route convention for an undeclared method, not a declared code. The declared
`data` array is empty and non-null because no `approval_requests` row can be
truthfully reshaped into the pinned `PermissionV2Request` schema (declared `id`
must match `^per`, approval ids are UUIDs; same for `^ses`), which the handler's
own evidence note in the served surface records.

## Regression test added

`internal/shim/opencode/declared_vs_served_consistency_test.go` —
`TestDeclaredVsServedArtifactIsSelfConsistent` pins the artifact's
self-consistency so this class cannot land silently again:

1. every declared count must equal the rows the artifact actually enumerates;
2. `drift_by_class` must sum to the drift row count;
3. no operation may appear in **both** `covered` and `drift_declared_not_served`
   (the merge signature), and no duplicate within `covered`;
4. each drift row must agree with its own class (`NOT-SERVED` names no served
   outcome; `OUTCOME-MISMATCH` must name one);
5. positional drift ids must be a contiguous, unique `SHIM-DRIFT-NNN` run;
6. `counts.served_route_entries` must equal the number of `routes:` entries in
   `consensus-shim-served-surface.yaml`.

The test refuses to pass vacuously on an empty artifact.

**RED proof** — the artifact alone was reverted to the committed HEAD revision
(`git show HEAD:specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`),
the new test alone left in place:

```text
=== RUN   TestDeclaredVsServedArtifactIsSelfConsistent
    declared_vs_served_consistency_test.go:140: counts.covered_operations = 102 but the artifact enumerates 103 row(s) — the artifact is stale; regenerate with scripts/compare-opencode-declared-served.py and copy the result to specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json
    declared_vs_served_consistency_test.go:171: POST /api/integration/{integrationID}/connect/key (v2.integration.connect.key) is recorded as BOTH covered (served outcome "204") and drift_declared_not_served (class "NOT-SERVED"): one operation cannot be served-as-declared and not served at all. This is the merge-resolved-artifact signature — regenerate the artifact rather than hand-editing it.
    declared_vs_served_consistency_test.go:244: counts.served_route_entries = 143 but ../../../specs/openapi/upstream/consensus-shim-served-surface.yaml enumerates 144 route(s) — the artifact was not regenerated after the served surface changed
--- FAIL: TestDeclaredVsServedArtifactIsSelfConsistent (0.01s)
```

**GREEN proof** — with the regenerated artifact restored (byte-identical to the
comparator output, `cmp` clean):

```text
--- PASS: TestDeclaredVsServedArtifactIsSelfConsistent (0.01s)
ok   github.com/wojons/consensus/internal/shim/opencode   0.021s
```

## Truthfulness

Nothing about the route changed in this row: it was already served, already
tested, and already recorded as covered before this branch. This row's
deliverable is the artifact regeneration its own third criterion mandates, plus
the regression test that pins what the regeneration caught. The empty `data`
array is the pre-existing, documented truthfulness decision of ROUTE-ADD-021,
not a new narrowing: the shim reads the real `approval_requests` store and
answers the declared `InvalidRequestError` 400 when the store fails, but emits
no row it cannot shape truthfully.

## Verification

- `go build ./...` — PASS.
- `go vet ./internal/shim/opencode/` — PASS.
- `go test ./internal/shim/opencode/... -count=1` — PASS (package ok).
- New consistency test — RED on the committed artifact, GREEN on the regenerated one.
- Artifact regeneration — byte-identical to `compare-opencode-declared-served.py` output.
- Live init + serve + curl probe — PASS for declared 401 (absent and rejected credential), 200 (bare and with the declared selector), 400 (blank selector), plus wrong-method 405.
- Scratch server shutdown — PASS; `ss` showed no listener remaining on port 18723.
