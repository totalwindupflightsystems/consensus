# ROUTE-FIX-005 live probe — GET /project/current

ch:trace row=ROUTE-FIX-005
  spec=specs/openapi/upstream/openapi-1.18.33.json#project.current
  test=internal/shim/opencode/project_current_test.go (TestProjectCurrent*)
  doc=docs/evidence/ROUTE-FIX-005-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-005-live-probe.md
  witness=none:unattended-worker-session — scripted curls against a locally
    built binary; every value below is copied verbatim from the probe
    transcript. verdict=/commit= are deliberately absent in-tree: a verdict
    binds to a commit that already exists, so it is recorded by the commit
    footer / foreman verification, never self-referenced by the change it
    judges.

Date: 2026-10-04 (local, UTC-05) / boot 2026-10-04T07:10:28Z
Worktree: `/home/kara/consensus-wt-ROUTE-FIX-005` (branch `wt/ROUTE-FIX-005`,
base `05a52a2`); the probe ran with this change applied to the worktree — the
committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim route was driven over HTTP. Every value below is copied
from a live HTTP response, a live `sqlite3` invocation, or the boot log —
none is hand-typed. (Probe context: a first attempt at this row had its
worktree reaped mid-flight and the board briefly carried a fabricated
"merged a2c7d8e" close; this probe is from the re-created worktree against
the real commit.)

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/t005-consensus` (sha256 `bd44c42fd45978bf…` full: `bd44c42fd45978bfd8a8c1c590932220e97151112e8c07676c48fab515686164`) |
| build | `go build -o /tmp/t005-consensus ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/consensus-wt-ROUTE-FIX-005` @ `wt/ROUTE-FIX-005`, base `05a52a2` |
| server | `127.0.0.1:18612`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/t005-run/t005.db`, created by the boot's auto-migrate (38 tables) |
| sqlite3 | 3.53.4 (used only to read back) |
| auth | bootstrap admin key (Bearer); captured from the boot's stdout and used from a curl header — the value is never printed or recorded in this document |

## 2. Declared contract, live

`<code> <body>` below is curl's HTTP status and the response body, verbatim.

| arm | request | observed |
|---|---|---|
| no auth header | `GET /project/current` | `401` (auth middleware, pre-existing) |
| happy path (declared 200) | `GET /project/current` | `200 application/json` — `{"id":"consensus-fb71fe67d3c7","name":"consensus-wt-ROUTE-FIX-005","sandboxes":[],"time":{"created":1791097828000,"updated":1791097828000},"vcs":"git","worktree":"/home/kara/consensus-wt-ROUTE-FIX-005"}` |
| declared `?directory=` selector | `GET /project/current?directory=/tmp` | `200` |
| declared `?workspace=` selector | `GET /project/current?workspace=default` | `200` |
| both selectors | `GET /project/current?directory=/tmp&workspace=default` | `200` |
| blank `?directory=` (declared 400) | `GET /project/current?directory=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"directory\" must not be blank"}}` |
| whitespace-only `?directory=` (declared 400) | `GET /project/current?directory=%20` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"directory\" must not be blank"}}` |
| blank `?workspace=` (declared 400) | `GET /project/current?workspace=` | `400 {"error":{"code":"INVALID_REQUEST","message":"query parameter \"workspace\" must not be blank"}}` |
| `x-opencode-directory` header workspace | `GET /project/current` + header `/tmp/t005-run/ws` | `200 {"id":"consensus-867a32d914a4","name":"ws","sandboxes":[],"time":{…same ledger…},"worktree":"/tmp/t005-run/ws"}` — no `vcs` key (plain dir), id/name/worktree all switch to the header workspace |
| undeclared method (only GET is declared; 405 is not in the declared set) | `POST /project/current` | `501 {"error":"not_implemented","operation":"project.current","detail":"project.current is declared as a GET operation; the shim translates GET only"}` |

Neighbours (must be unchanged by this route):

| probe | observed |
|---|---|
| `GET /project/project_missing` | `404 {"_tag":"ProjectNotFoundError","message":"Project not found: project_missing","projectID":"project_missing"}` (typed, DF-CONSENSUS-47) |
| `POST /project/git/init` | `501` (still unserved — ROUTE-FIX-006's row) |
| `GET /project/project_missing/directories` | `501` (still unserved — ROUTE-FIX-007's row) |
| `GET /project/current/unknown-sub` | `404` (non-vacuity control) |
| `GET /instance` id | `"id":"consensus-fb71fe67d3c7"` — the same id `project.current` reports for the same workspace (singleton-id cross-check) |
| happy path again after every error arm | `200` — `SELECT count(*) FROM schema_versions` = `24` before and after (no-effect proof: no arm mutated the ledger) |

## 3. Which declared codes are reachable

`project.current` declares exactly `200` (Project) and `400` (BadRequest).

- **200 is reachable** — the happy path answers the declared Project with
  every required key present (`id`, `worktree`, `time`, `sandboxes`) and no
  key outside the declared set.
- **400 is reachable** — a present-but-blank (or whitespace-only) declared
  query selector `directory`/`workspace` (live above). The operation declares
  no requestBody, so the selectors are what keep this arm reachable;
  validation runs before any store read.
- **No 501 is served for GET** — the pre-fix answer to every request was the
  typed not-implemented envelope; the route no longer answers 501 for any
  GET arm (the in-repo battery `TestProjectCurrentNeverAnswers501` pins
  this). POST keeps the typed 501 on purpose: 405 is not in the declared
  response set, and an honest "not translated" beats a false method error.

## 4. Why the declared 200 is a translation, not a fabrication

The runtime keeps exactly one workspace per instance (the singleton
convention of `/instance` and `/path`); the declared Project fields are all
derived from that real workspace directory:

| declared field | source | live value |
|---|---|---|
| `id` | `instanceID(dir)` — the singleton GET /instance id (short sha256 of the workspace dir) | `consensus-fb71fe67d3c7`, equal to GET /instance's id (cross-checked above) |
| `worktree` | `git rev-parse --show-toplevel`, else the directory (the /path convention) | `/home/kara/consensus-wt-ROUTE-FIX-005` |
| `name` | the workspace base name | `consensus-wt-ROUTE-FIX-005` |
| `vcs` | `"git"` only when `git rev-parse --is-inside-work-tree` succeeds (declared enum `["git"]`; absent otherwise) | `"git"` (worktree); **absent** for the header `/tmp/t005-run/ws` probe — the key is never asserted without evidence |
| `commands` | the workspace `consensus.json` `"commands"` object when present; absent otherwise (optional, never synthesized) | absent in this run (no consensus.json in the workspace); unit-tested both ways |
| `time.created` / `time.updated` | earliest / latest `applied_at` from `schema_versions` (the migration ledger — the project-state clock), epoch millis (the ProjectTime unit) | `1791097828000` = `2026-10-04T07:10:28Z` = the ledger's `2026-10-04T02:10:28-05:00` rows (all 24 stamped at boot) |
| `sandboxes` | `[]` (required key; the runtime spawns no sandboxes) | `[]` |
| `icon` | omitted (optional; nothing asserts it) | — |

Nothing here is invented: every field comes from the workspace directory or a
row the database actually holds, and an unreadable ledger answers the declared
400 (`TestProjectCurrentErrorArmsAnswerDeclaredCodes/unreadable_ledger…`)
rather than a fabricated timestamp.

## 5. RED proof

Reverting only the dispatch arm (`projectID == "current"` back to the typed
501 envelope, byte-verified restore of the fixed file afterwards,
sha256 `5076c097…` both before and after) and running the new battery against
the unfixed tree:

```
--- FAIL: TestProjectCurrentServesDeclared200 (0.16s)
--- FAIL: TestProjectCurrentErrorArmsAnswerDeclaredCodes (0.10s)
--- FAIL: TestProjectCurrentNeverAnswers501 (0.05s)
FAIL
```

The tests fail on the pre-fix behaviour and pass on the fixed tree — they pin
the fix, not the implementation.

## 6. Declared-vs-served flip

`specs/openapi/upstream/consensus-shim-served-surface.yaml` registers the
route (`outcome: 200`, with the per-arm evidence) and
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` was
regenerated by `scripts/compare-opencode-declared-served.py` (the pre-change
artifact was byte-reproducible from the pre-change inputs, so the regenerated
diff is exactly this route's flip):

| measure | before | after |
|---|---|---|
| `covered_operations` | 52 | **53** (`project.current` enters) |
| `drift_declared_not_served` | 128 (NOT-SERVED 103, OUTCOME-MISMATCH 25) | **127** (NOT-SERVED 103, OUTCOME-MISMATCH 24 — SHIM-DRIFT-099 leaves) |
| `served_route_entries` | 101 | **102** (the GET route) |
| `narrowed_error_contract_only` | 6 | 6 (unchanged) |
| `drift_served_not_declared` | 17 (PATH-ABSENT 16, METHOD-ABSENT 1) | 17 (unchanged) |

The `SHIM-DRIFT-NNN` ids after 099 shift down by one — the ids are positional
(enumeration order), as documented in the artifact header; the Go test table
matches on method + path shape, not id, and was re-pinned (15 → 14 rows).
