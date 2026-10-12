# ROUTE-ADD-104 live probe — GET /pty/shells

ch:trace row=ROUTE-ADD-104
  spec=specs/024-shim-support-matrix.md#intentional-narrowing
  test=internal/shim/opencode/pty_shells_test.go
  doc=docs/evidence/opencode-upstream-v1.18.33/surface-diff-report.md
  evidence=docs/evidence/ROUTE-ADD-104-live-probe.md
  witness=none:self-verified-in-worktree — scripted curls against a locally built
    binary; verdict=/commit= are deliberately absent because this probe ran
    before the implementation commit.

Date: 2026-10-12T11:10:03Z (UTC)
Worktree: `/home/kara/totalwindupflightsystems/consensus` at base `0302f81`.

The binary was built from the working tree with `go build -o
/tmp/consensus-route-add-104 ./cmd/consensus` and served against a fresh SQLite
state using the explicit scratch config `/tmp/route-add-104-live/consensus.yaml`.
The server bound `127.0.0.1:18604`; the bootstrap key was read from the local
init log into a shell variable and is not recorded here.

Binary SHA-256: `ed27c87a283ffb89bd57cd66323d8ba0399d2f915267eea8b346125d96653168`

## Declared arms and live observations

The pinned upstream contract for `GET /pty/shells` declares `200` with an
array of `{path, name, acceptable}` shell items and `400` with
`BadRequestError`. The probe asserted the status and decoded body shape for
both arms, plus auth and neighbouring PTY routes.

```text
health 200 /global/health
shells 200 /pty/shells
shells_existing 200 /pty/shells?directory=/tmp
shells_missing 400 /pty/shells?directory=/no/such/route-add-104
shells_blank 400 /pty/shells?workspace=
shells_no_auth 401 /pty/shells
pty_neighbor 200 /pty
pty_id_neighbor 404 /pty/pty_1
probe assertions: PASS
```

The successful response was a real host-backed list, not a fabricated PTY
registry:

```json
[{"path":"/bin/sh","name":"sh","acceptable":true},{"path":"/usr/bin/sh","name":"sh","acceptable":true},{"path":"/bin/bash","name":"bash","acceptable":true},{"path":"/usr/bin/bash","name":"bash","acceptable":true},{"path":"/bin/rbash","name":"rbash","acceptable":true},{"path":"/usr/bin/rbash","name":"rbash","acceptable":true},{"path":"/usr/bin/dash","name":"dash","acceptable":true},{"path":"/usr/bin/screen","name":"screen","acceptable":true},{"path":"/usr/bin/tmux","name":"tmux","acceptable":true}]
```

The declared errors were observed as:

```json
{"error":{"code":"INVALID_REQUEST","message":"directory \"/no/such/route-add-104\" does not exist or is not a directory"}}
{"error":{"code":"INVALID_REQUEST","message":"query parameter \"workspace\" must not be blank"}}
```

An unauthenticated request remained protected:

```json
{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}
```

## Declared-vs-served flip

The route comparator was run from the repository script against the patched
served-surface YAML. Its output was copied to the canonical route comparison
artifact by that generated-output workflow and validated with `jq empty`.
`GET /pty/shells` is in `covered` with `served_outcome: "200"` and is absent
from `drift_declared_not_served`.

```text
{"declared_paths":162,"declared_operations":188,"served_route_entries":103,"covered_operations":74,"narrowed_error_contract_only":6,"intentional_narrowing":1,"semantic_different":2,"drift_declared_not_served":105,"drift_by_class":{"NOT-SERVED":101,"OUTCOME-MISMATCH":4},"drift_served_not_declared":0,"served_drift_by_class":{},"shim_extension_operations":17}
```

The parent chi mount was also exercised by `TestPtyShellsChiMount`; the live
scratch probe confirmed the direct server route and the `/pty` and
`/pty/{ptyID}` neighbours remained reachable with their prior behavior.
