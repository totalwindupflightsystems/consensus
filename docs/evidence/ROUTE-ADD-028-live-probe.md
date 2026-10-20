# ROUTE-ADD-028 — GET /api/pty (v2.pty.list) live probe

ch:trace row=ROUTE-ADD-028
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.pty.list
  test=internal/shim/opencode/api_pty_list_test.go
  doc=specs/openapi/upstream/consensus-shim-served-surface.yaml
  evidence=docs/evidence/ROUTE-ADD-028-live-probe.md
  witness=none:unattended-worker-session

Date: 2026-10-20T00:24:00-05:00
Base commit: `c0d859f650be692114aa9c176c4c80f90ba766cd` (`main`).

**Source item:** the board row cites `SHIM-DRIFT-024`, which in the 2026-09-29
baseline report (`docs/reports/shim-source-002-baseline-diff-2026-09-29.md`,
line 396) is exactly `GET /api/pty` (v2.pty.list, declared responses
200,400,401). The committed artifact's drift ids are POSITIONAL — the
comparator renumbers them on every regeneration — and in the artifact this
worker started from the same operation carried `SHIM-DRIFT-001` (the same
situation ROUTE-ADD-025/026/027 record). The operation, the path and the
declared response set in the brief all name `GET /api/pty` = `v2.pty.list`, so
that is the row served here; the id mapping is recorded here and in the
served-surface evidence rather than being silently accepted.

## Run identity

| Field | Value |
|---|---|
| Binary | `/tmp/consensus-ra028` |
| Binary SHA-256 | `832ce028aeea0b193183f98a9b4e5a3eca74f5376486b1a3842e5f3be3ef3c1c` |
| Listener | `127.0.0.1:52841` |
| Database | scratch SQLite `/tmp/consensus-ra028-live/consensus.db` |
| Config | explicit `consensus.yaml` in the scratch directory (loopback-only; copied from the ROUTE-ADD-027 scratch template, port and db path re-pointed) |
| Credential | bootstrap admin key from `consensus init`, kept at `/tmp/consensus-ra028-live/admin.key1` (`chmod 600`); the raw value is not recorded in this document or in any committed file |
| Server logs | `/tmp/consensus-ra028-live/serve.log`, battery log `/tmp/ra028/live-battery.log` |
| Server process | Hermes-tracked background process `proc_7c175e040541`, stopped and verified closed after the probe |

The binary was built from this tree with `go build -o /tmp/consensus-ra028
./cmd/consensus`. The instance was initialized with `consensus init --config
/tmp/consensus-ra028-live/consensus.yaml` (migrations applied, bootstrap admin
key created) and started with `consensus serve --config
/tmp/consensus-ra028-live/consensus.yaml` (log: `consensus: opencode shim
enabled`). The listener was verified free before start (`ss -tln`) and closed
after shutdown.

## Contract exercised

The pinned document declares `GET /api/pty` as `v2.pty.list`: one optional
deepObject `location` selector (`location[directory]`, `location[workspace]`,
`additionalProperties: false`), and responses 200
`{location: LocationInfo, data: Pty[]}` ("List PTY sessions for a location,
including exited sessions retained until removal."), 400 `InvalidRequestError`,
401 `UnauthorizedError`. `Pty` requires `id` (pattern `^pty`), `title`,
`command`, `args`, `cwd`, `status` (`running|exited`) and `pid`; every 200
schema sets `additionalProperties: false`.

The exact-path registration also serves the declared sibling method on the
same path, `POST /api/pty` = `v2.pty.create` (baseline id SHIM-DRIFT-025), as
the typed 501 intentional narrowing (SHIM-GAP-002; the ROUTE-ADD-103
intentional narrowing beside bare `/pty`).

## RED proof

The new test battery was run against the pre-change tree (tests written
before the route existed):

```text
$ go test ./internal/shim/opencode/ -run 'ApiPtyList' -count=1
--- FAIL: TestApiPtyListServesDeclared200 (0.00s)
    api_pty_list_test.go:96: GET /api/pty: got 404, want 200. Body: 404 page not found
--- FAIL: TestApiPtyListServesSelectedWorkspace (0.00s)
    ... got 404, want 200. Body: 404 page not found
--- FAIL: TestApiPtyListDeclared400MalformedSelector (5 subtests)
    ... got 404, want the declared 400. Body: 404 page not found
--- FAIL: TestApiPtyListCreateAnswersTyped501 (0.00s)
    api_pty_list_test.go:194: POST /api/pty answered 404 — a registered route may not lie with 404/405 (SHIM-GAP-002). Body: 404 page not found
--- FAIL: TestApiPtyListMethodNotAllowed (0.00s)
    PUT/PATCH/DELETE: got 404, want 405. Body: 404 page not found
--- FAIL: TestApiPtyListServeDeclared401 (0.00s)
    response is missing required member "_tag" (got [error])  — the pre-fix v1 UNAUTHENTICATED envelope
--- FAIL: TestApiPtyListMountWiring (0.00s)
    /api/pty is not registered in MountPatterns — a parent chi mount would 404 the route (ROUTE-ADD-028)
FAIL    github.com/wojons/consensus/internal/shim/opencode
```

Full log: `/tmp/ra028-red-arm1.log`. The pre-fix answer for every arm is the
net/http default `404 page not found` (and the v1 UNAUTHENTICATED envelope for
unauthenticated requests) — the artifact's NOT-SERVED class.

## Live results

Full battery (`/tmp/ra028/live-battery.log`; `KEY` = bootstrap admin key,
`B = http://127.0.0.1:52841`):

```text
GET_200=200
GET_200_wellformed_selector=200
GET_400_blank_selector=400
GET_400_unknown_field=400
GET_400_bad_ws=400
POST_501=501
PUT_405=405
DELETE_405=405
GET_401_unauth=401
GET_401_badkey=401
POST_401_unauth=401
DEEPER_404=404
DEEPER2_404=404
SIBLING_V1_200=200
SIBLING_SHELLS_200=200
SIBLING_LIST_200=200
```

The authenticated success body (`Content-Type: application/json`) — the
declared envelope with the truthful empty terminal list for the real selected
workspace:

```json
{"data":[],"location":{"directory":"/tmp/consensus-ra028-live","project":{"directory":"/tmp/consensus-ra028-live","id":"consensus-e11979c69645"}}}
```

The declared error arms used the declared typed bodies:

```json
{"_tag":"InvalidRequestError","field":"location[directory]","kind":"Query","message":"query parameter \"location[directory]\" must not be blank"}
{"error":"not_implemented","operation":"v2.pty.create","detail":"v2.pty.create is not implemented: spawning pseudo-terminals is opencode-specific and Consensus keeps no pty registry; use the native shell tool inside an agent session"}
{"error":{"code":"METHOD_NOT_ALLOWED","message":"use GET or POST"}}
{"_tag":"UnauthorizedError","message":"invalid credentials"}
```

A well-formed `location[directory]` selector is honoured (`GET_200_wellformed_selector=200`),
so the 400 arm is the malformed-input branch, not a blanket refusal of the
selector. Deeper `/api/pty/pty1` and `/api/pty/a/b` keep the net/http default
404 (no catch-all was introduced), and the sibling surfaces are untouched: the
v1 `/pty` list and `/pty/shells` still answer their own 200s, and
`GET /api/provider` (ROUTE-ADD-026) still answers its own declared 200.

No response body anywhere in the probe contains a credential value. (One
probe-only mistake is recorded for honesty: the first battery answered
`400 Bad Request` on every authenticated request because the bootstrap key had
been extracted with a trailing prefix fragment from the init log, producing a
two-line key file whose embedded newline corrupted the HTTP header — curl
diagnosis, no server involvement; fixed by taking the first line and re-run.)

## Truthfulness

The declared `data` array is empty and non-null: this runtime keeps NO pty
registry and spawns no pseudo-terminals (the same truth ROUTE-ADD-102 records
for the v1 `pty.list`), and the declared `Pty` requires seven members (`id`
pattern `^pty`, `title`, `command`, `args`, `cwd`, `status` running|exited,
`pid`) that no runtime store could truthfully supply — any entry would assert
a terminal that does not exist. `location` is the REAL selected workspace
(`requestWorkspaceDir` — the `x-opencode-directory` header or the configured
workdir — overridden by the declared deepObject `location[directory]`;
`project.id = instanceID(directory)`), the same derivation the ROUTE-ADD-021
list family uses. The declared `POST` sibling is answered with the typed
not-implemented envelope naming `v2.pty.create` rather than a fabricated
spawned-terminal body or a lying 404 from a registered route.

## Declared-vs-served result

The comparator was run from the repository root:

```text
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/ra028/out2
```

The generated `declared-vs-served.json` was copied byte-for-byte to
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

```text
before: covered_operations 106, drift_declared_not_served 63 (NOT-SERVED 59), intentional_narrowing 1, served_route_entries 147
after:  covered_operations 107, drift_declared_not_served 61 (NOT-SERVED 57), intentional_narrowing 2, served_route_entries 149
```

`GET /api/pty` now appears in the artifact's `covered` list with
`served_outcome: 200`; no drift row for `v2.pty.list` remains, and the
sibling `POST /api/pty` (v2.pty.create) moved from NOT-SERVED to the
`intentional_narrowing` bucket (SHIM-NARROWED-U001 in the regenerated
artifact) instead of staying ordinary drift. `drift_served_not_declared`
stays 0.

## Gates

```text
go build ./...                                  ok
go vet ./...                                    ok
gofmt -l internal/shim/opencode/                clean
go test -short ./internal/shim/opencode/ -count=1   ok (19.552s, includes the artifact consistency gates)
go test -short ./internal/api/ -count=1         ok (2.275s, contract tests)
```

## Cleanup

The scratch server was stopped and the listener verified closed
(`ss -tln` → no `:52841`); the scratch database, config, key file and logs
remain under `/tmp/consensus-ra028-live/` and `/tmp/ra028/` for re-probing.
