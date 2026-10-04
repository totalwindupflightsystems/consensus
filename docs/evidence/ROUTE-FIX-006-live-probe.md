# ROUTE-FIX-006 live probe — POST /project/git/init

ch:trace row=ROUTE-FIX-006
  spec=specs/openapi/upstream/openapi-1.18.33.json#project.initGit
  test=internal/shim/opencode/project_init_test.go (TestProjectInitGit*)
  doc=docs/evidence/ROUTE-FIX-006-live-probe.md
  evidence=docs/evidence/ROUTE-FIX-006-live-probe.md
  witness=none:unattended-worker-session — scripted curls against a locally
    built binary; every value below is copied verbatim from the probe
    transcripts (/tmp/t006-run3/probe.txt, probe2.txt). verdict=/commit= are
    deliberately absent in-tree: a verdict binds to a commit that already
    exists, so it is recorded by the commit footer / foreman verification,
    never self-referenced by the change it judges.

Date: 2026-10-04 (local, UTC-05) / boot 2026-10-04T13:51:55Z
Worktree: /home/kara/worktrees/consensus-ROUTE-FIX-006 (branch
`wt/ROUTE-FIX-006`, based on master); the probe ran with this change applied
to the worktree — the committed diff is identical to the probed tree.

A locally built `consensus` binary was served on a **fresh SQLite database**
and the opencode shim route was driven over HTTP. The server process ran with
its CWD in a plain non-git directory (`/tmp/t006-run3/ws`) so the happy path
performs a REAL `git init` — verifiable before/after with live git commands.
Every value below is copied from a live HTTP response, a live `git` invocation,
or the boot log — none is hand-typed.

## 1. Run identity

| property | value |
|---|---|
| binary | `/tmp/t006-consensus` (sha256 `dcf35e645ce61fcd1112038dee4e6e7e68e95e5c738bef078d85d19e596bf0e0`) |
| build | `go build -o /tmp/t006-consensus ./cmd/consensus`, from the worktree root |
| worktree / branch | `/home/kara/worktrees/consensus-ROUTE-FIX-006` @ `wt/ROUTE-FIX-006` |
| server | `127.0.0.1:18619`, backend `sqlite`; `/global/health` = `{"healthy":true,"version":"consensus-0.1.0"}` |
| db | `sqlite:///tmp/t006-run3/t006.db?_journal_mode=WAL`, created by the boot's auto-migrate |
| process CWD (workspace) | `/tmp/t006-run3/ws` — NOT a git repository before the probe (`git rev-parse --is-inside-work-tree` → `fatal: not a git repository`) |
| auth | bootstrap admin key (Bearer); captured from the boot's stdout and used from a curl header — the value is never printed or recorded in this document |

## 2. Declared contract, live

`<code> <body>` below is curl's HTTP status and the response body, verbatim.

| arm | request | observed |
|---|---|---|
| no auth header | `POST /project/git/init` | `401 {"error":{"code":"UNAUTHENTICATED","message":"invalid credentials"}}` — the sub-path is served for real now, so it left `isStubPath`'s auth exemption (see §5) |
| happy path (declared 200, REAL git init) | `POST /project/git/init` | `200 {"id":"consensus-5836d0db2aa6","name":"ws","sandboxes":[],"time":{"created":1791121915000,"updated":1791121915000},"vcs":"git","worktree":"/tmp/t006-run3/ws"}` |
| effect proof, before | `git -C /tmp/t006-run3/ws rev-parse --is-inside-work-tree` | `fatal: not a git repository (or any of the parent directories): .git` |
| effect proof, after | same command | `true` |
| effect proof, git dir | `git -C /tmp/t006-run3/ws rev-parse --git-dir` | `.git` |
| effect proof, no commit | `git -C /tmp/t006-run3/ws rev-parse --verify HEAD` | `fatal: Needed a single revision` — init created no HEAD, exactly what `git init` alone does |
| re-init arm (idempotent) | `POST /project/git/init` again | `200`, and `rev-parse --verify HEAD` STILL `fatal: Needed a single revision` — the second POST did not create refs |
| blank `?directory=` (declared 400) | `POST /project/git/init?directory=` | `400 {"data":{"kind":"Query","message":"query parameter \"directory\" must not be blank"},"name":"BadRequest"}` — the declared BadRequestError envelope |
| whitespace `?workspace=` (declared 400) | `POST /project/git/init?workspace=%20` | `400 {"data":{"kind":"Query","message":"query parameter \"workspace\" must not be blank"},"name":"BadRequest"}` |
| valued selectors | `POST /project/git/init?directory=/tmp&workspace=default` | `200` (accepted, scope nothing — one workspace) |
| `x-opencode-directory` header (real init in the header workspace) | header `/tmp/t006-run3/other` | `200 {"id":"consensus-cedf31ad29a3","name":"other",…,"worktree":"/tmp/t006-run3/other"}`; live `git -C /tmp/t006-run3/other rev-parse --is-inside-work-tree` → `true` — the init ran in the HEADER workspace |
| init failure (declared 400) | header `/tmp/t006-fail/notadir` (a FILE, git init exits 128) | `400 {"error":{"code":"INVALID_REQUEST","message":"could not initialize git in the workspace: exit status 128 "}}` |
| body is not part of the contract | `POST` with `{"unexpected":true}` | `200` (the operation declares no requestBody; the query selectors are the input surface) |

Neighbours (must be unchanged by this route):

| probe | observed |
|---|---|
| `GET /project/current` | `200` — the sibling served route, and afterwards it reports the SAME id `consensus-5836d0db2aa6` for the same workspace (singleton-id cross-check) |
| `GET /project/prj_missing` | `404` (typed ProjectNotFoundError, pre-existing) |
| `GET /project/prj_missing/directories` | `501` (still unserved — ROUTE-FIX-007's row) |
| `POST /project/current` (undeclared method) | `501 {"error":"not_implemented","operation":"project.current","detail":"project.current is declared as a GET operation; the shim translates GET only"}` (typed envelope kept) |
| `GET /vcs` | `200 {"branch":"master"}` (read neighbour) |

## 3. Which declared codes are reachable

`project.initGit` declares exactly `200` (Project) and `400` (BadRequest).

- **200 is reachable** — the happy path answers the declared Project with
  every required key present (`id`, `worktree`, `time`, `sandboxes`), no key
  outside the declared set, and the effect is real (`.git` exists afterwards,
  live git commands above).
- **400 is reachable** three ways, all live above: a present-but-blank (or
  whitespace-only) declared query selector answers the declared
  `BadRequestError` NamedError envelope (`{name:"BadRequest", data:{kind,
  message}}`, kind `Query`); a `git init` that cannot run answers
  `INVALID_REQUEST`; an unreadable schema_versions ledger answers
  `INVALID_REQUEST` (unit-tested, `TestProjectInitGit…`; the operation
  declares no 5xx).
- **No 501 is served for POST** — the pre-fix answer to every request was the
  typed not-implemented envelope; the in-repo battery
  `TestProjectInitGitNeverAnswers501` pins this. Non-POST keeps the typed 501
  on purpose: 405 is not in the declared response set, and an honest "not
  translated" beats a false method error.

## 4. Why the declared 200 is a real effect plus a translation, not a fabrication

Upstream runs `git init` in the project's worktree. The shim performs the same
filesystem effect on the single real workspace the instance serves (the
singleton convention of `/instance`, `/path` and `project.current`, resolved by
`x-opencode-directory`), then derives the declared Project exactly the way
`project.current` does (shared `deriveProject` helper: id <- `instanceID(dir)`,
worktree <- `gitWorktree(dir)`, name <- base name, `vcs` <- `"git"` only when
`rev-parse --is-inside-work-tree` succeeds, `commands` <- consensus.json when
present, `time` <- schema_versions applied_at bounds in epoch millis,
`sandboxes` `[]`). Nothing is invented: the mutation is a real `git init` the
probe verified with git itself, and every field comes from the workspace
directory or a row the database actually holds.

## 5. Auth change (isStubPath carve-out)

The stub exemption `isStubPath` granted every non-GET `/project/*` request
existed because stubs answer "NOT_IMPLEMENTED with zero data, so there is
nothing to protect" (SPEC-017 §3.9). `POST /project/git/init` now serves real
data and performs a real workspace mutation, so it left that exemption and
keeps api-key auth like every other implemented route — live `401` above
without credentials, `200` with them. `GET /vcs` and the other stub sub-paths
are unchanged (`GET /project/prj_missing/directories` still `501`).

## 6. RED proof

Reverting only the dispatch arm + handler (server.go restored from
`HEAD:internal/shim/opencode/server.go`, byte-verified sha256 both before and
after: `8c67f68e3493cce6511438ba02985d24f38cf8866f6aa9d7a431c086b2f31d4b`) and
running the new battery against the unfixed tree:

```
--- FAIL: TestProjectInitGitServesDeclared200 (0.05s)
--- FAIL: TestProjectInitGitErrorArmsAnswerDeclaredCodes (0.04s)
--- FAIL: TestProjectInitGitNeverAnswers501 (0.04s)
FAIL
```

(The full-tree revert also fails `TestProjectCurrentSiblingsUntouched`'s
git/init arm in its pre-fix 501 expectation — that arm was updated to expect
the declared codes, and `TestNotImplementedTableMatchesDriftArtifact` /
`TestDeclaredUnimplementedRoutesAnswerTypedEnvelope` were re-counted from 14
to 13 with the artifact regenerated.)
