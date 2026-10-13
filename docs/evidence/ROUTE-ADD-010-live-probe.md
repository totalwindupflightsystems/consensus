# ROUTE-ADD-010 — GET /api/fs/find live probe

## Run identity

| Field | Value |
|---|---|
| Source branch | `wt/ROUTE-ADD-010` |
| Base commit | `18f39aa5698fbd165fd494cf85bc3209f61144a4` |
| Binary | `/tmp/consensus-route-add-010` |
| Binary SHA-256 | `8a5cb9eed9a32d095f4cbbb35b9dab8b6f82ed1095342b33654dc5554034d8d8` |
| Go toolchain | `go1.26.5 linux/amd64` |
| Listener | `127.0.0.1:18910` |
| Database | scratch SQLite: `/tmp/route-add-010-live/consensus.db` |
| Search workspace | `/tmp/route-add-010-live/workspace` |
| Credential | bootstrap admin key captured to a mode-local scratch file; raw value not logged |
| Transcript | `/tmp/route-add-010-live/transcript.txt` |

The binary was built from the base commit plus the ROUTE-ADD-010 working-tree delta. The probe used an explicit scratch config and database; it did not read the repository's ambient configuration.

## Declared contract

The pinned upstream document is `specs/openapi/upstream/openapi-1.18.33.json`, operation `v2.fs.find`:

- `GET /api/fs/find`
- required query parameter: `query`
- optional query parameters: deep-object `location[directory]` / `location[workspace]`, `type=file|directory`, positive integer `limit`
- `200`: `{location: LocationInfo, data: FileSystemEntry[]}`
- `400`: `InvalidRequestError`
- `401`: `UnauthorizedError`

## RED proof

Before route registration and handler implementation, the targeted regression suite failed with the existing behavior:

- every authenticated happy-path and malformed-input request returned `404 page not found`;
- unauthenticated access returned the generic shim envelope `{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}`, not the declared v2 `UnauthorizedError`;
- the chi-mounted route returned 404.

Command: `go test ./internal/shim/opencode -run '^TestFSFind' -count=1`.

## GREEN live probe

| Arm | Request | Observed |
|---|---|---|
| Declared 401 | unauthenticated `GET /api/fs/find?query=alpha` | `401 {"_tag":"UnauthorizedError","message":"invalid credentials"}` |
| Declared 400 — required query | authenticated `GET /api/fs/find` | `400 {"_tag":"InvalidRequestError","field":"query","kind":"Query","message":"?query= is required"}` |
| Declared 400 — type enum | authenticated `GET /api/fs/find?query=alpha&type=socket` | `400 InvalidRequestError`, field `type` |
| Declared 200 | authenticated search rooted at the scratch workspace, query `alpha`, workspace `wrk-live` | `200`; `LocationInfo.directory` and `project.directory` matched the requested workspace; data contained `docs/alpha.md`, `src/alpha.go`, `alpha-dir/`, and `alpha-dir/nested.txt` with declared file/directory types |
| Declared 200 — filter + limit | authenticated `query=alpha&type=file&limit=1` | `200`; exactly one file entry |
| Neighbour guard | authenticated `GET /api/fs/list?path=.` | `404`; the exact registration did not claim the sibling v2 filesystem route |

The probe script asserted each status and JSON shape with `jq -e` before writing the transcript. It completed with exit code 0.

## Truthfulness and scope

The 200 response is produced from a real recursive walk of the selected directory. Paths are workspace-relative, `.git` is not traversed, type filtering and limits are applied after deterministic fuzzy ranking, and no filesystem entries are fabricated. The optional workspace ID is echoed only when it satisfies the declared `^wrk` shape; the singleton project ID is derived deterministically from the selected directory.

## Declared-vs-served result

The served-surface row was added and the comparator was rerun. The generated result was reconciled with the artifact's existing stable drift IDs:

- served route entries: `105 -> 106`
- covered operations: `77 -> 78`
- declared-not-served drift: `103 -> 102`
- `NOT-SERVED`: `99 -> 98`
- `/api/fs/find` is present in `covered` with responses `200,400,401`
- no `/api/fs/find` row remains in `drift_declared_not_served`
- the next historical source item, `/api/fs/list`, remains `SHIM-DRIFT-007` (no positional-ID reassignment)
