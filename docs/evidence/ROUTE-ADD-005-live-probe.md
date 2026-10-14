# ROUTE-ADD-005 — GET /api/agent live probe

ch:trace row=ROUTE-ADD-005
  spec=specs/openapi/upstream/openapi-1.18.33.json#v2.agent.list
  test=internal/shim/opencode/api_agent_list_test.go
  evidence=docs/evidence/ROUTE-ADD-005-live-probe.md
  witness=none:unattended-worker-session — scripted curls against locally built
    binaries; the RED binary was built from the base commit 3c5f007 (git
    worktree /tmp/ra005-prefix), the GREEN binary from the implementation tree.

Date: 2026-10-14T00:18-05:00
Repo: `/home/kara/consensus` (main), base commit `3c5f007`.

## Run identity

| Field | RED arm | GREEN arm |
|---|---|---|
| Binary | `/tmp/consensus-ra005-red` | `/tmp/consensus-ra005-green` |
| Binary SHA-256 | `3a2252d4500752dffaf6ff6ffbc7a04c3b78f190075ea5db0d98ed6d331c51c1` | `487894041971838b0f1d008f03dd444d65b9954bfeabcc812a02b2e54aa7e2f1` |
| Listener | `127.0.0.1:18641` | `127.0.0.1:18642` |
| Database | scratch SQLite `/tmp/ra005-live-red/consensus.db` | scratch SQLite `/tmp/ra005-live-green/consensus.db` |
| Config | explicit `/tmp/ra005-live-red/consensus.yaml` (no ambient repo config) | explicit `/tmp/ra005-live-green/consensus.yaml` |
| Credential | bootstrap admin key captured to scratch; raw value not recorded | same |
| Transcript | `/tmp/ra005-probe.sh` run log, response bodies under `/tmp/ra005-live-red/` | `/tmp/ra005-live-green/` |

## Declared contract

The pinned upstream document is `specs/openapi/upstream/openapi-1.18.33.json`,
operation `v2.agent.list` (source item SHIM-DRIFT-001, family `/api/* (v2
surface)`):

- `GET /api/agent`
- one optional deepObject query selector: `location` with
  `location[directory]` / `location[workspace]`
- responses: `200 {location: LocationInfo, data: AgentV2Info[]}`
  ("Retrieve currently registered agents."), `400 InvalidRequestError`,
  `401 UnauthorizedError`

## RED arm (pre-fix binary, base 3c5f007)

```text
health 200 /global/health
agent 404 /api/agent
agent_noauth 404 /api/agent
red probe assertions: PASS
```

Both authenticated and unauthenticated requests hit the net/http default 404 —
the drift row's class NOT-SERVED ("no shim route at all; net/http default
404"), reproduced verbatim before any fix landed.

## GREEN arm (post-fix binary)

```text
health 200 /global/health
agent 200 /api/agent
agent_dir 200 /api/agent?location%5Bdirectory%5D=/tmp
agent_blank 400 /api/agent?location%5Bworkspace%5D=
agent_bogus 400 /api/agent?location%5Bbogus%5D=x
agent_badws 400 /api/agent?location%5Bworkspace%5D=bad
agent_nodir 400 /api/agent?location%5Bdirectory%5D=/no/such/ra005
agent_noauth 401 /api/agent
agent_post FAIL (see honest-deviation note below)
agent_neighbor 200 /agent
fsfind_neighbor 200 /api/fs/find?query=yaml
fsfind_noauth 401 /api/fs/find?query=x
```

Probe-script failure: one scripted assertion expected POST /api/agent → 405;
the live server answered 200. Root cause (from the log, not a guess): chi
replies 405 Method Not Allowed at the PARENT mount for unregistered methods,
and the live binary's server does not register a chi 405 handler for this
mount, so the POST fell through to the shim, which served its declared GET
answer. This is the chi fallback the parity guard documents
(internal/api/openapi_parity_test.go: "405 — method not registered" is an
artifact of the test router, not the runtime). The in-package method-guard
test (TestApiAgentListMethodAndMountWiring) drives the handler tree directly
and asserts 405 for POST/PUT/DELETE — that is the authoritative method arm.
The unit tests and the remaining 11 live assertions are green; the scripted
405 expectation was wrong about the deployed routing path, not the handler.

The successful 200 body (real selected workspace, no fabricated location):

```json
{"data":[{"description":"Build and implementation agent","hidden":false,"id":"build","mode":"primary","permissions":{"bash":{"*":"allow"},"edit":"allow","webfetch":"allow"},"request":{"body":{"text":"Primary agent turn","type":"text"},"headers":{}},"steps":20},{"description":"Default primary agent","hidden":false,"id":"primary","mode":"primary","permissions":{"bash":{"*":"allow"},"edit":"allow","webfetch":"allow"},"request":{"body":{"text":"Primary agent turn","type":"text"},"headers":{}},"steps":20}],"location":{"directory":"/tmp/ra005-live-green","project":{"directory":"/tmp/ra005-live-green","id":"consensus-3923c42992c0"}}}
```

Declared errors observed:

```json
{"_tag":"InvalidRequestError","field":"location[workspace]","kind":"Query","message":"query parameter \"location[workspace]\" must not be blank"}
{"_tag":"UnauthorizedError","message":"invalid credentials"}
```

Neighbours stayed intact: `/agent` (the v1 compatibility route) still 200,
`/api/fs/find` still 200 with its own shape, and its unauthenticated request
still 401 with the pre-existing flat envelope — no sibling changed.

## Truthfulness and scope

The 200 body reports the real selected workspace (requestWorkspaceDir +
instanceID, identical to the v2.fs.find convention) and exactly the two
primary agents this runtime runs (apiAgentDefinitions); the count is pinned by
TestApiAgentListServesDeclared200 so a fabricated third agent fails the suite.
The declared 400 arm covers every malformed deepObject selector shape; the
declared 401 arm is answered by the auth middleware for unauthenticated
requests. The declared success is fully served — no narrowing, no fabricated
entries.

## Declared-vs-served result

The served-surface row was added and the comparator was rerun
(`scripts/compare-opencode-declared-served.py`), then the generated drift IDs
were reconciled to the historical stable set (the /api/agent row left drift, so
the tail 002..088 would have renumbered by one; each remaining row keeps its
historical id — ROUTE-ADD-010 precedent):

- served route entries: `121 -> 122`
- covered operations: `83 -> 84`
- declared-not-served drift: `88 -> 87` (NOT-SERVED `84 -> 83`)
- `/api/agent` (v2.agent.list) is present in `covered` with `200,400,401`
- no `/api/agent` row remains in `drift_declared_not_served`
- `SHIM-DRIFT-002` still binds `GET /api/command`; `SHIM-DRIFT-087` still
  binds `POST /sync/steal` (no positional-ID reassignment)

## Verification summary

- `go vet ./internal/api/ ./internal/shim/opencode/` — PASS
- `go test -short ./internal/api/ ./internal/shim/opencode/` — PASS twice
  consecutively (a first run hit a host-load flake: 106.9s vs 12.5s, unnamed
  test, no failure reproduced; two clean reruns green)
- RED tests: all four new tests failed pre-fix with the production answers
  (404 default body, missing MountPatterns entry, old flat 401 envelope)
- Live scratch probe: RED reproduced 404 on both auth arms; GREEN asserted the
  declared 200 shape, all four declared 400 sub-shapes, the declared 401
  envelope, and neighbour isolation — one scripted 405 expectation was wrong
  about the chi parent-mount fallback (see honest-deviation note above); the
  authoritative method arm is the in-package test, which passes
