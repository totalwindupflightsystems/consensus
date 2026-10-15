# ROUTE-ADD-025 — DELETE /api/permission/saved/{id} live probe

ch:trace row=ROUTE-ADD-025
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.permission.saved.remove
  test=internal/shim/opencode/api_permission_saved_remove_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-025-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-15, worktree `wt/ROUTE-ADD-025` (base `fac3923`).

**Source item:** the board brief and the committed artifact both name
`DELETE /api/permission/saved/{id}` / `v2.permission.saved.remove`; in the
committed artifact its positional drift id is `SHIM-DRIFT-001` (drift ids are
POSITIONAL — the comparator renumbers them on every regeneration, and
`TestNotImplementedTableMatchesDeclaredSurfaceArtifact` matches on the
method+path+operationId identity for exactly that reason).

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra025` |
| Binary SHA-256 | `26b371a47e73bc277b850b6aa3093fbf5e28b78977f12ef2748a5a77997bcea3` |
| Listener | `127.0.0.1:33117` (verified free before start via `ss -tln`) |
| Database | scratch SQLite `/tmp/consensus-ra025-live.2882264/consensus.db` |
| Credential | bootstrap admin key from `consensus init` (text output line `key=cs_ak_…`), kept in the scratch directory (`umask 077`); the raw value is not recorded here |
| Transcript | the probe script's own stdout (all arms below), scratch dir `/tmp/consensus-ra025-live.2882264` |

The binary was built from this worktree with `go build -o /tmp/consensus-ra025
./cmd/consensus`. `consensus init --db-url sqlite://<scratch>/consensus.db`
applied the embedded migrations, then `consensus serve --adapter opencode`
ran from the scratch directory on the free port above. The server process was
terminated after the probe (`SERVER_STOPPED_RC=0`, `LISTENER_CLOSED`).

## Declared contract

The pinned upstream document declares:

- `DELETE /api/permission/saved/{id}`
- operationId `v2.permission.saved.remove` ("Remove a saved permission by ID.")
- required plain-string path parameter `id` — NO pattern
- no requestBody, no query selector
- responses `204` No Content, `400` InvalidRequestError, `401` UnauthorizedError

Before ROUTE-ADD-025 there was no shim route. The RED regression run against
the unwired tree failed with the production answers: happy and blank-id
requests were `404 page not found`, `MountPatterns` lacked the pattern, and
unauthenticated access used the generic `UNAUTHENTICATED` envelope instead of
the declared v2 401 body.

## Live results

```text
INIT_RC=0
KEY_LEN=70
READY=1
UNAUTH_401=401 body={"_tag":"UnauthorizedError","message":"invalid credentials"}
HAPPY_204_call1=204 body_bytes=0
HAPPY_204_call2=204 body_bytes=0
BLANK_ID_400=400 body={"_tag":"InvalidRequestError","field":"id","kind":"Path","message":"id must not be blank"}
BLANK_ID_JQ=OK
WRONG_METHOD_GET=405 body={"error":{"code":"METHOD_NOT_ALLOWED","message":"use DELETE"}}
WRONG_METHOD_JQ=OK
SIBLING_LIST_GET=200 body={"data":[]}
SIBLING_JQ=OK
DEEPER_404=404
BAD_CRED_401=401 body={"_tag":"UnauthorizedError","message":"invalid credentials"}
BAD_CRED_JQ=OK
SERVER_STOPPED_RC=0
LISTENER_CLOSED
```

Every status and envelope was asserted with `jq -e` (`*_JQ=OK`); the
authenticated 204 body was asserted to be zero bytes via curl's
`size_download`, twice — the second call pins the idempotence (removing what
nothing holds answers the same way). The wrong-method request exercised the
exact same concrete path as the 204 arms, proving the handler rather than a
neighboring 404 answered them.

## Truthfulness

The runtime keeps NO saved-permission (reusable allow/deny rule) store — the
same truth the sibling list operation (`v2.permission.saved.list`,
ROUTE-ADD-021) documents: the migration set (001..026) has no such table and
`permission.reply` records an "always" answer as the resolved approval's
`review_notes` on the `approval_requests` row, not as a reusable rule. No
upstream id can address anything this runtime holds, so the declared 204 is an
idempotent removal of nothing: it carries no body, fabricates no stored rule,
and does not claim a deletion happened.

The declared 400 is reachable only for the one malformed input the pinned
document actually defines: a present-but-blank `{id}` segment (`%20`; an
actually-empty segment cannot match the route pattern and keeps net/http's
default 404). `id` is a plain required string with NO pattern and the
operation declares no requestBody and no query selector, so inventing further
malformed-id constraints would fabricate rules the document does not state
(the credentialRemove precedent, ROUTE-ADD-007).

## Declared-vs-served result

The served-surface row was added and the comparator was run with the pinned
surface, the updated served-surface YAML, and a scratch output directory. A
second run into another directory reproduces the committed artifact
byte-identically. The operation-key set-diff against the pre-change artifact
is exact:

- moved from `drift_declared_not_served` to `covered`:
  `DELETE /api/permission/saved/{id} (v2.permission.saved.remove)`
- no other operation key entered or left ANY bucket (covered, drift,
  narrowed_error_contract_only, intentional_narrowing, semantic_different,
  drift_served_not_declared, shim_extensions — all verified unchanged)

```text
counts before this change (the committed artifact):  covered_operations 104  drift_declared_not_served 65  NOT-SERVED 61
after this change (the regenerated artifact):        covered_operations 105  drift_declared_not_served 64  NOT-SERVED 60
narrowed_error_contract_only 16 (narrowed_unexplained 0) | intentional_narrowing 1 | semantic_different 2 | shim_extensions 17 | drift_served_not_declared 0
```

## Verification

- RED (route unregistered): `go test ./internal/shim/opencode/ -run '^TestApiPermissionSavedRemove' -count=1` — 4 tests FAIL with the pre-fix production answers (404 default, missing `MountPatterns` entry, v1 `UNAUTHENTICATED` 401 envelope).
- GREEN targeted: the same command after registration — PASS (4 tests).
- Whole package: `go test ./internal/shim/opencode/ -count=1` — PASS (128.9s).
- `go build ./...` — PASS. `go vet ./internal/shim/opencode/` — PASS.
- `gofmt -l` on the two new Go files and `server.go` — empty.
- Artifact: comparator output copied byte-for-byte to
  `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`; a second
  comparator run reproduces it byte-identically; the operation-key diff is
  exactly the one move; `TestDeclaredVsServedArtifactIsSelfConsistent` and the
  rest of the package confirm (counts equal their lists).
- Live scratch SQLite probe: PASS for the declared 401 (2 arms), empty-body
  204 (2 calls), blank-id 400, wrong-method 405, sibling list 200, deeper
  sub-path 404; listener closed after shutdown.
