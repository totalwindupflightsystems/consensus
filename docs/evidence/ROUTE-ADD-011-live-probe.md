# ROUTE-ADD-011 — GET /api/fs/list + GET /api/fs/read/* live probe

## Run identity

| Field | Value |
|---|---|
| Source branch | `wt/ROUTE-ADD-011` |
| Base commit | `95d58ecf045bfeaea45fe845d419a8a0077785f8` |
| Working-tree delta | the ROUTE-ADD-011 change set (handlers, tests, served-surface rows, regenerated artifact) |
| Binary | the demo harness build of `./cmd/consensus` (`demo.TestMain`, `go build -o <tmp>/consensus-demo ./cmd/consensus/`) — the real production binary, not a test handler |
| Go toolchain | `go1.26.5 linux/amd64` |
| Listener | `127.0.0.1:39621` (loopback, per-run reserved port) |
| Database | scratch SQLite written under the probe's `t.TempDir()` |
| Workspace | scratch directory with `alpha.txt`, `docs/guide.md` and an `outside-secret.txt` NEGATIVE CONTROL one level above it |
| Credential | bootstrap admin key emitted by the server (`cs_ak_e364…`); the raw value is not recorded here |
| Probe | scratch `demo/zz_routeadd011_live_probe_test.go`, run with `go test -count=1 -run TestRouteAdd011LiveProbe -v ./demo/`, then deleted — the deliverable is the handlers, their tests and the reconciliation rows |

## Declared contract

The pinned upstream document is `specs/openapi/upstream/openapi-1.18.33.json`:

- `v2.fs.list` — `GET /api/fs/list`, optional deepObject `location` (`directory`, `workspace`; `additionalProperties: false`) and optional string `path`; `200` `{location: LocationInfo, data: FileSystemEntry[]}` ("List direct children of one directory relative to the requested location."), `400` `InvalidRequestError`, `401` `UnauthorizedError`.
- `v2.fs.read` — `GET /api/fs/read/*`, optional deepObject `location`; `200` `application/octet-stream` (binary string, "Serve one file relative to the requested location."), `400` `InvalidRequestError`, `401` `UnauthorizedError`.

Neither operation declares a 404.

## RED proof

Stage A — before route registration and handlers, with the new tests in the tree:

- command: `go test -count=1 -short -run 'TestFSList|TestFSRead|TestFSFind' ./internal/shim/opencode/`
- every arm failed with `404 page not found`: `GET /api/fs/list: got 404, want 200`, `GET /api/fs/read/alpha.txt: got 404, want 200`, every malformed arm `got 404, want 400`, and the stale sibling assertion `GET /api/fs/list: got 404, want 200 (ROUTE-ADD-011 serves the sibling)`. This is the artifact's NOT-SERVED class: no shim route at all.

Stage B — routes registered, traversal guard removed from `Server.Handler` (one-line revert):

- `--- FAIL: TestFSReadRejectsTraversal/plain_parent_reference` with `GET /api/fs/read/../outside-secret.txt: got 404, want the declared 400` — net/http's ServeMux canonicalizes the non-canonical path and redirects (308) before the route is matched, so the request lands on the cleaned, unclaimed path. Every other spelling reached the handler and passed. The guard is therefore load-bearing, not decoration: without it the declared `400` arm is unreachable for the plain `../` spelling the contract exists for.

## GREEN live probe

| Arm | Request (authenticated unless stated) | Observed |
|---|---|---|
| Declared 401 — list | unauthenticated `GET /api/fs/list?location[directory]=<ws>` | `401 {"_tag":"UnauthorizedError","message":"invalid credentials"}` |
| Declared 401 — read | unauthenticated `GET /api/fs/read/alpha.txt?location[directory]=<ws>` | `401 {"_tag":"UnauthorizedError","message":"invalid credentials"}` |
| Declared 200 — list | `GET /api/fs/list?location[directory]=<ws>` | `200`; `data` = exactly `[{"path":"alpha.txt","type":"file"},{"path":"docs/","type":"directory"}]`; `location.directory` and `location.project.directory` = the requested workspace; no `workspaceID` (none was named) |
| Declared 200 — read | `GET /api/fs/read/alpha.txt?location[directory]=<ws>` | `200`, `application/octet-stream`, body `alpha` |
| Declared 200 — nested read | `GET /api/fs/read/docs/guide.md?location[directory]=<ws>` | `200`, body `guide` |
| Traversal — plain | `GET /api/fs/read/../outside-secret.txt?location[directory]=<ws>` | `400 {"_tag":"InvalidRequestError","kind":"Path","message":"path escapes the requested location: \"/api/fs/read/../outside-secret.txt\" contains a parent reference"}` |
| Traversal — percent-encoded | `GET /api/fs/read/%2e%2e/outside-secret.txt?…` | same declared `400` envelope |
| Traversal — encoded slash | `GET /api/fs/read/..%2Foutside-secret.txt?…` | same declared `400` envelope |
| Traversal — nested | `GET /api/fs/read/docs/..%2F..%2Foutside-secret.txt?…` | same declared `400` envelope |
| Traversal — absolute | `GET /api/fs/read/%2Fetc%2Fhostname?…` | `400 {"_tag":"InvalidRequestError","kind":"Path","message":"path \"/etc/hostname\" is absolute; the v2 filesystem operations address paths relative to the requested location"}` |
| Declared 400 — blank selector | `GET /api/fs/list?location[directory]=` | `400`, `field location[directory]`, `kind Query` |
| Declared 400 — unknown selector field | `GET /api/fs/list?location[bogus]=x` | `400`, `field location[bogus]`, `kind Query` |
| Declared 400 — blank path | `GET /api/fs/list?path=%20&location[directory]=<ws>` | `400`, `field path`, `kind Query` |
| Declared 400 — traversing list path | `GET /api/fs/list?path=..&location[directory]=<ws>` | `400`, `field path`, `kind Query`, message quotes the parent reference |
| Declared 400 — absent file | `GET /api/fs/read/missing.txt?location[directory]=<ws>` | `400 InvalidRequestError` (the operation declares no 404) |
| Declared 400 — directory | `GET /api/fs/read/docs?location[directory]=<ws>` | `400 InvalidRequestError` |
| Declared 400 — empty suffix | `GET /api/fs/read/?location[directory]=<ws>` | `400 InvalidRequestError` |
| Method guard — list | `POST /api/fs/list?location[directory]=<ws>` | `405 {"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}` |
| Method guard — read | `DELETE /api/fs/read/alpha.txt?location[directory]=<ws>` | `405 {"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET"}}` |
| Neighbour isolation | `GET /api/agent`; `GET /api/fs/find?query=alpha&location[directory]=<ws>` | both `200` |

No arm returned the negative-control content: the probe asserts `OUTSIDE-SECRET` never appears in any traversal response body.

## Truthfulness and scope

Both payloads come from the real host filesystem: the list is an actual `os.ReadDir` of the requested directory and the read is an actual `os.ReadFile` of the requested file, served as the declared binary media type with no JSON envelope and no re-encoding (a 256-byte `0x00`–`0xff` file round-trips byte-for-byte in `TestFSReadServesBinaryBytes`). Nothing is fabricated: no entry, no field and no location member is invented, an empty directory answers `[]` rather than null, and the project id is the deterministic `instanceID` of the selected directory. Paths that leave the requested location are refused, never served, on three independent layers — the lexical check (`resolveWorkspacePath`), the symlink-resolved containment check (`ensureNoSymlinkEscape`, proven by `TestFSReadRejectsTraversal/symlink_escaping_the_workspace` and its positive control `TestFSReadServesSymlinkInsideTheWorkspace`), and the pre-ServeMux guard (`dispatchWithV2FSTraversalGuard`).

## Declared-vs-served result

The two served-surface rows were added and the comparator rerun:

```
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/dvs-route-add-011
```

- `drift_declared_not_served`: `87` → `85` (both rows leave the NOT-SERVED class)
- `drift_by_class.NOT-SERVED`: `83` → `81`
- `covered_operations`: `84` → `86`
- `served_route_entries`: `122` → `124`
- `drift_served_not_declared` stays `0`; `narrowed_unexplained` stays `0`

`/tmp/dvs-route-add-011/declared-vs-served.json` was copied byte-for-byte over the committed artifact (no re-serialization). The artifact's remaining diff is the positional `SHIM-DRIFT-NNN` renumbering of the surviving rows, which is how the comparator assigns ids.
