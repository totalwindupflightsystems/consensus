# ROUTE-FIX-004 — Live Probe: GET /project serves the declared project.list contract

ch:trace row=ROUTE-FIX-004
  spec=specs/openapi/upstream/openapi-1.18.33.json#project.list
  test=internal/shim/opencode/project_list_test.go
  commit=wt/ROUTE-FIX-004-r2 (worktree consensus-wt-ROUTE-FIX-004-r2)
  doc=docs/evidence/ROUTE-FIX-004-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-004-live-probe.md
  witness=none:unattended worker session; probes are scripted curls against a locally built binary, transcripts preserved verbatim below

Date: 2026-10-04 (local, UTC-05)
Worktree: /home/kara/consensus-wt-ROUTE-FIX-004-r2 (branch wt/ROUTE-FIX-004-r2)

## What changed

`internal/shim/opencode/server.go` — the bare `/project` mount answered the
untyped 501 stub ("project is opencode-specific, not supported by Consensus
shim; use native tool API") for EVERY method before this fix
(SHIM-DRIFT-098, class OUTCOME-MISMATCH: declared 200,400, served 501).
Now:

| method | before | after |
|---|---|---|
| GET /project | 501 untyped stub | 200 Project[] from the runtime's `projects` table (migrations 014/015) |
| GET /project (store read failure) | 501 | 400 INVALID_REQUEST naming the failure (the declared vocabulary is 400 only) |
| GET /project (no projects table, pre-014 store) | 501 | 200 `[]` (an empty list is the honest translation) |
| GET /project unauthenticated | 501 (stub-skip reached it) | 401 UNAUTHENTICATED (unchanged — GET kept auth before and after; isStubPath covers only non-GET) |
| POST/DELETE/PATCH /project | 501 untyped stub | 501 untyped stub, byte-identical (undeclared methods; 405 is not in the declared set) |

Translation per row: `id` <- projects.id, `name` <- projects.name,
`worktree` <- the server's configured workdir (else process CWD — the same
single-workspace translation /vcs, /vcs/diff and /session/{id}/diff use),
`time` <- {created, updated} <- created_at (Unix seconds; numeric TEXT is
parsed — see finding below), `sandboxes` <- []. The optional
?directory=/?workspace= selectors are accepted but do not narrow: the runtime
has one workspace, no per-project path columns, and neither value is a
project id.

## Method

Binary built from this worktree, scratch DB initialized with the shipped
embedded migrations, one project row seeded, then scripted curl probes.

```text
go build -o /tmp/consensus-t004b ./cmd/consensus
/tmp/consensus-t004b init --db-url sqlite:///tmp/rf004-live.db   # fresh scratch DB, embedded migrations applied
sqlite3 /tmp/rf004-live.db "INSERT INTO projects (id, name, description, created_at)
  VALUES ('prj-live-1', 'LiveProbeProject', 'seeded for ROUTE-FIX-004 live probe', strftime('%s','now'));"
/tmp/consensus-t004b serve --db-url sqlite:///tmp/rf004-live.db --port 18095
# probes below, admin bearer = the first_admin_key minted by init
```

## Observed vs documented (documented set: {200, 400}, plus the mount's kept 401/501 arms)

**GET /project, authenticated (admin bearer):**

```text
[{"id":"prj-live-1","name":"LiveProbeProject","sandboxes":[],"time":{"created":1791098154,"updated":1791098154},"worktree":"/tmp"}]
HTTP_STATUS=200
```

Observed 200, declared 200. Every required Project field present (id,
worktree, time.created, time.updated, sandboxes), no undeclared fields.

**GET /project, no credentials:**

```text
{"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}
HTTP_STATUS=401
```

401 — GET keeps api-key auth on the bare mount (SPEC-017 §3.9; the shim smoke
test's 401 no-auth arm is preserved by isStubPath covering only non-GET).

**GET /project?directory=/tmp&workspace=ws1 (declared selectors):**

```text
[{"id":"prj-live-1","name":"LiveProbeProject","sandboxes":[],"time":{"created":1791098154,"updated":1791098154},"worktree":"/tmp"}]
HTTP_STATUS=200
```

200 — the selectors are accepted and do not narrow (documented in the handler
comment; there is nothing to narrow on).

**GET /project/current (neighbour — /project/{id} sub-paths untouched):**

```text
{"error":"not_implemented","operation":"project.current","detail":"project.current is not implemented: ..."}
HTTP_STATUS=501
```

The handleProjectByID typed-stub contract (SHIM-GAP-002) is intact.

**POST /project (undeclared method):**

```text
{"error":{"code":"NOT_IMPLEMENTED","message":"project is opencode-specific, not supported by Consensus shim; use native tool API"}}
HTTP_STATUS=501
```

The pre-existing stub answer, byte-identical.

**Freshly initialized store, no seeded projects (second scratch DB,
/tmp/rf004-empty.db, port 18096):**

```text
[]
HTTP_STATUS=200
```

200 with the declared empty array.

## Finding fixed during the probe: TEXT created_at

The first probe run returned `"time":{"created":0,"updated":0}` for the
seeded row: `strftime('%s','now')` wrote created_at as TEXT ("1791098154"),
and SQLite's NUMERIC affinity on a TIMESTAMPTZ-declared column did not coerce
it — `toInt64` (which handles only Go numerics) answered 0. The handler now
parses numeric TEXT through `projectTimeSeconds` (unreadable values answer 0,
never a fabricated stamp), pinned by
TestProjectListParsesTextTimestamps (TEXT parses to 1728000123, junk
answers 0). The re-probe above shows the corrected stamps.

## Ground truth established before writing the handler

- `sqlite3` probe: the migration DDL as written (`gen_random_uuid()`,
  `TIMESTAMPTZ ... DEFAULT now()`) applies on SQLite — the defaults are dead
  DDL, the table accepts explicit-value inserts.
- Bootstrap+migrate probe (a throwaway main under the repo, run then
  neutralized): the SHIPPED migration runner (Bootstrap + Up) creates the
  projects table on SQLite (24 migrations applied), and `db.DB.Query` returns
  map rows with the stored types.

## Tests

internal/shim/opencode/project_list_test.go — 8 tests, verified RED against
the pre-fix mount (every arm answered the 501 stub) and GREEN after:

- TestProjectListServesDeclaredContract (happy path over a real SQLite store;
  required fields; 401 unauthenticated + 200 via Basic admin key)
- TestProjectListEmptyStoreAnswersEmptyArray ([] never null)
- TestProjectListParsesTextTimestamps (the live-store finding)
- TestProjectListReadFailureAnswersDeclared400 (400 INVALID_REQUEST)
- TestProjectListNeverAnswersUndeclaredCodes (5-shape never-501 sweep)
- TestProjectListUndeclaredMethodKeepsStub (POST/DELETE/PATCH stay 501)
- TestProjectListChiMountMatchesProductionWiring (MountPatterns pass-through)
- TestProjectEndpointGetServesDeclaredList (server_test.go; renamed from
  TestProjectEndpointReturns501, which pinned the defect)

`go test -short -count=1 ./internal/shim/opencode/` green after the change;
the full GitReins guard ran in the commit hook and passed.
