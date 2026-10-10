# ROUTE-FIX-031 live probe — POST /vcs/apply

Task: `ROUTE-FIX-031` (source item `SHIM-DRIFT-142`). Generated 2026-10-10.

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim route was driven over HTTP through the real serve wiring
(the API router with the shim mounted on its `MountPatterns`, exactly as
`cmd/consensus/main.go` mounts it). Every value below is copied from a live
HTTP response — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/rf031-consensus` (sha256 `e8e636769864e43d6ef4ca3f4f16e2b296f3d219a9a6620060ad8955e6e89fc7`) |
| base revision | `36059161a50dab972db548de95df9f11dc2c256e` (worktree `wt/ROUTE-FIX-031`, branch base = origin/master) |
| server | `127.0.0.1:18732`, backend `sqlite` |
| db | `sqlite:///tmp/rf031-run-2/rf031b.db?_journal_mode=WAL` (created by the boot's auto-migrate) |
| auth | bootstrap admin key (`created=true`, length 70) read from the server log; value never recorded |

## 2. Declared contract, live

| arm | request | observed |
|---|---|---|
| **declared 200** | `POST /vcs/apply` `{"patch":"--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n"}` | `200` body `{"applied":false}` |
| **declared 200** (valued selector) | `POST /vcs/apply?directory=/tmp/rf031-run-2` `{"patch":"x"}` | `200` body `{"applied":false}` |
| **declared 400** (declared field missing) | `POST /vcs/apply` `{}` | `400` `{"data":{"kind":"Payload","message":"required field patch must be a non-empty string"},"name":"BadRequest"}` |
| **declared 400** (field wrong type) | `POST /vcs/apply` `{"patch":42}` | `400` `{"data":{"kind":"Body","message":"request body must be an object containing only patch"},"name":"BadRequest"}` |
| **declared 400** (additionalProperties: false) | `POST /vcs/apply` `{"patch":"x","extra":1}` | `400` `{"data":{"kind":"Body","message":"request body must be an object containing only patch"},"name":"BadRequest"}` |
| **declared 400** (blank declared selector) | `POST /vcs/apply?directory=` `{"patch":"x"}` | `400` `{"data":{"kind":"Query","message":"query parameter \"directory\" must not be blank"},"name":"BadRequest"}` |
| **405** (method guard) | `GET /vcs/apply` | `405` `{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}` |
| **405** (method guard) | `PUT /vcs/apply` | `405` `{"error":{"code":"METHOD_NOT_ALLOWED","message":"use POST"}}` |
| **401** (auth) | `POST /vcs/apply` with no credential | `401` `{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` |
| neighbour unchanged | `GET /vcs/diff/raw` | `501` typed not_implemented envelope naming `vcs.diff.raw` |

Every declared response code — `200`, `400` — is reachable on the live surface,
plus the two answers the contract does not declare and the shim returns
deliberately: `405` for an undeclared method and `401` for a missing
credential.

## 3. Why the 200 body is `{"applied":false}` (no genuine apply analog exists)

The declared 200 schema is an object `{applied (required boolean)}` with
`additionalProperties: false` — the upstream document's representation of
"VCS patch applied". The Consensus shim has no VCS write capability: it runs no
`git apply`, holds no patch queue, and mutates no working tree. `false` is
therefore the only truthful value, the same convention the sibling detached-TUI
routes use (`tuiPublish`/`tuiShowToast` answer the declared body for an
operation this shim cannot perform).

A live check that no working tree was touched: `handleVCSApply` contains no
subprocess call at all — `sed -n '/^func (s \*Server) handleVCSApply/,/^}/p'
internal/shim/opencode/server.go | grep -n 'runGit\|exec\.\|Command('` returns
nothing — so the route cannot shell out to `git apply`. The worktree's
`git status --short` after the whole probe run lists only this row's own edits
(`server.go`, `server_test.go`, the two contract artifacts, this file) and no
patch artifact.

## 4. Contract artifacts

`scripts/compare-opencode-declared-served.py` regenerated against the updated
served surface (`specs/openapi/upstream/consensus-shim-served-surface.yaml`,
`/vcs/apply` outcome `501-typed` → `200`, auth `stub-skip-non-GET` → `api-key`):

| census field | before | after |
|---|---|---|
| `covered_operations` | 71 | **72** |
| `drift_declared_not_served` | 109 | **108** |
| `drift_by_class.OUTCOME-MISMATCH` | 7 | **6** |
| `drift_by_class.NOT-SERVED` | 102 | 102 |
| `drift_served_not_declared` | 0 | 0 |
| `shim_extension_operations` | 17 | 17 |

`vcs.apply` moves from `drift_declared_not_served` (class `OUTCOME-MISMATCH`,
`served_outcome: 501-typed`) to `covered` with `served_outcome: "200"`. The
regeneration is deterministic: a second run is byte-identical to the committed
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

## 5. Gates

| gate | result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `gofmt -l internal/` | empty |
| `go test -short ./internal/shim/opencode/ -count=1` | ok (58.9s) |
| `go test -short ./internal/api/ -count=1` | ok (spec/bundle parity unchanged) |
| `make bundle-spec` | ran redocly; `specs/openapi/bundled.yaml` byte-identical (no spec source changed) |
| RED proof | reverting only `internal/shim/opencode/server.go` fails all three new tests with the pre-fix answer: `got 501, want 200`, `got 501, want 400`, `got 501, want 405` (`{"error":"not_implemented","operation":"vcs.apply",…}`); restored byte-identical to the fixed file |
