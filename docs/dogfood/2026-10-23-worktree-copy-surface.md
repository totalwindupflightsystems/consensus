# Dogfood 2026-10-23 — the new `/experimental/*` shim families in real use

Target: `consensus` @ HEAD `1482437` (build `go build ./cmd/consensus`, 2.3 s warm; scratch
sqlite instance, no LLM key — this run deliberately drives the protocol-shim surface, which
needs no model).
Angle: the routes merged since the previous dogfood (2026-10-20) were never used by a human:
`/experimental/worktree` ×4 (`ROUTE-ADD-083..086`, merges `ae94992`/`db944ae`/`fcb4516`),
`/experimental/project/{projectID}/copy` ×2 (`ROUTE-ADD-069/070`), `/experimental/console` ×3
and `/experimental/control-plane/move-session` (`ROUTE-ADD-065..068`), plus the `/api/pty/{ptyID}`
family (`ROUTE-ADD-030..032`, merged the same morning).

Promise being tested (from the shim contract): an opencode-shaped client can create a worktree,
work in it, reset it and remove it; list console state; copy a project.

## Environment / provenance

| item | value |
|---|---|
| product HEAD | `1482437db1de0bad81616bef03286cac89d20316` (clean tree) |
| binary | `/tmp/consensus-df-1023/consensus`, built from that HEAD, `go1.26.5` |
| server A | port `18391`, cwd `/tmp/consensus-df-1023` (**not** a git repo — the realistic systemd-launch case) |
| server B | port `18392`, cwd `/tmp/consensus-df-1023/proj` (a git repo) |
| workspace repo | `/tmp/consensus-df-1023/proj`, one commit `68bb4d5` |
| auth | fresh `cs_ak_…` admin key from `consensus init`; shim routes answer `401` without it (verified) |
| isolation | `HOME=/tmp/consensus-df-1023/home`, DB `/tmp/consensus-df-1023/dogfood.db` |
| schema | `schema_version 32`, 38 tables, 31 migrations |

Same binary on both ports — the only difference is the process CWD, which is what makes the two
findings below attributive rather than speculative.

## What works (verified live, not from tests)

| operation | arm | result |
|---|---|---|
| `GET /experimental/worktree` | authed, `?directory=<repo>` | `200 ["/…/proj"]`; then lists the created worktree too |
| `GET /experimental/worktree` | no auth | `401` |
| `POST /experimental/worktree` | `{"name":"df-wt-1"}` | `200 {"directory":"…/.opencode-worktrees/df-wt-1","name":"df-wt-1"}` — a **real** `git worktree` on disk (`git worktree list` shows it, detached HEAD) |
| `POST /experimental/worktree` | `{}` (name optional) | `200`, name generated (`wt-1729cc2a`) |
| `POST /experimental/worktree` | `{"name":"x","bogus":true}` | `400 InvalidRequestError kind=Body "unknown field \"bogus\" (… allows only name and startCommand)"` |
| `POST /experimental/worktree/reset` | `{"directory":"<worktree>"}` after dirtying it (`M README.md`, `?? newfile.txt`) | `200 true`; modification reverted **and** the untracked file removed |
| `POST /experimental/worktree/reset` | no body | `400 InvalidRequestError kind=Body (WorktreeResetInput, additionalProperties:false): EOF` |
| `DELETE /experimental/worktree` | `?directory=<linked worktree>` (server B) | `200 true`, worktree gone from disk and from `git worktree list` |
| `DELETE /experimental/worktree` | `?directory=<primary checkout>` | `400 WorktreeRemoveFailedError "… is the primary checkout of …; worktree.remove only removes linked worktrees"` |
| `DELETE /experimental/worktree` | `?directory=/etc` | `400 InvalidRequestError field=directory kind=Query "resolves outside the selected workspace"` |
| `GET /experimental/console` | — | `200 {"consoleManagedProviders":[],"switchableOrgCount":0}` — truthful empty, matches declared `ConsoleState` required fields |
| `GET /experimental/console/orgs` | — | `200 {"orgs":[]}` |
| `POST /experimental/console/switch` | `{"accountID":"acct_1","orgID":"org_1"}` | `200 false` (nothing switched — truthful) |
| `POST /experimental/project/{id}/copy` | `{"strategy":"git_worktree","directory":"<repo>","name":"df-copy-1"}` | `200 {"directory":"…/proj/df-copy-1"}` — real worktree created |
| `POST /experimental/project/{id}/copy` | `{"strategy":"local",…}` | `400 ProjectCopyError "… registered strategy: \"git_worktree\""` — honest capability statement |
| `POST /experimental/project/{id}/copy` | missing `directory` / extra field | `400 InvalidRequestError kind=Payload` / `kind=Body` naming the declared field set |
| `DELETE /experimental/project/{id}/copy` | `{"directory":…,"force":true}` | `204`, nothing removed (no copy registered) |
| `POST /experimental/control-plane/move-session` | `ses_bogus` | `400 MoveSessionError "no session … nothing to move"` |

The error vocabulary across these families is the best part of the surface: every refusal is a
typed, named error saying which field and why, and the destructive arms (primary checkout, path
escape) refuse **before** doing anything. The `?directory=/etc` escape and the
`additionalProperties:false` rejections are exactly the kind of guard that is usually missing.

## Finding 1 (P1) — `worktree.remove` does not implement the declared input contract

`DF-CONSENSUS-63`. The declared operation carries **both** optional `directory`/`workspace` query
selectors **and** a required request body `WorktreeRemoveInput {directory}`. Two deviations,
A/B-proven on the same binary:

**(A) The declared body is ignored.** Server B (cwd inside the repo), worktree `df-wt-1` existing:

```
DELETE /experimental/worktree   body {"directory":"…/.opencode-worktrees/df-wt-1"}   -> 400
  WorktreeRemoveFailedError "worktree.remove needs an explicit directory query parameter:
  without one the only candidate is the primary checkout, which this runtime refuses to remove"
DELETE /experimental/worktree?directory=…/.opencode-worktrees/df-wt-1   (no body)     -> 200 true
  …and `git worktree list` + the directory itself are gone
```

Sibling `reset` is the opposite — it *requires* the body and rejects a body-less call. So one
family treats the same declared body two incompatible ways.

**(B) The declared workspace selectors are not consulted.** Server A (cwd outside the repo — what
a systemd unit with no `WorkingDirectory` looks like):

```
POST /experimental/worktree?directory=<repo>  -> 200, real worktree created     (create honours it)
GET  /experimental/worktree?directory=<repo>  -> 200, lists it                  (list honours it)
DELETE /experimental/worktree?directory=<that worktree> -> 400 WorktreeNotGitError
  "workspace /tmp/consensus-df-1023 is not a git repository: exit status 128"   (remove does not)
POST /experimental/worktree/reset (body, same selector) -> 400 the same way     (reset does not)
```

The message names the server's launch directory, so a client author reads it as a server fault
rather than a selector problem. Workarounds proven live (both `200 true` + real disk removal):
header `x-opencode-directory: <repo>`, or `?location[directory]=<repo>` — the bracketed selector
is what `apiV2LocationSelector` actually reads (`api_v2_lists.go:87-111`), and it is **not** what
these four operations declare.

Provider lines: `internal/shim/opencode/experimental_worktree_list_remove.go`
(`handleExperimentalWorktreeRemove`, target = `q.Get("directory")` only);
`internal/shim/opencode/api_v2_lists.go:87-111` (workspace = `location[directory]` or
`requestWorkspaceDir`); contrast `internal/shim/opencode/experimental_worktree_create.go:229-240`
(create/list parse the declared plain `directory`). `SPEC-017 §3.10` pins the shim workspace order
as *x-opencode-directory → declared `?directory=` → `?workspace=` → configured workdir*; the middle
two are missing from remove/reset only.

Net user impact: a client that creates worktrees through the documented selector can never remove
them unless the server happens to be launched inside the repo — worktrees accumulate on disk and
`worktree.list` keeps returning them.

## Finding 2 (P2) — `startCommand` is accepted and never executed, disclosed only in the server log

`DF-CONSENSUS-64`. The declared `WorktreeCreateInput.startCommand` means "additional startup
script to run after the project's start command". Live:

```
POST /experimental/worktree?directory=<repo>
  {"name":"df-wt-sc","startCommand":"touch /tmp/consensus-df-1023/evidence/startcommand-ran"}
-> 200 {"directory":"…/.opencode-worktrees/df-wt-sc","name":"df-wt-sc"}
3 s later: ls evidence/startcommand-ran -> ENOENT      (no child process either)
logs/serve.log: level=WARN msg="opencode-shim: worktree startCommand accepted but not executed"
```

The response is the declared success shape, so the client believes the worktree is coming up
running; the only disclosure is a log line an HTTP client never sees. The project's convention for
an unsupported declared capability is the typed `not_implemented` narrowing registered in
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` → `intentional_narrowing`
(2 entries today: `POST /api/pty`, `POST /pty`). `startCommand` is registered nowhere, so the
drift report cannot show it either. Source: `experimental_worktree_create.go:88` (design note),
`:149-152` (the `slog.Warn`).

## Not findings (checked, and honestly fine)

- **`POST /experimental/project/{projectID}/copy/generate-name` and `.../copy/refresh` answer a bare
  `404 page not found`.** These are declared-but-unserved and already tracked as open rows
  `ROUTE-ADD-071` / `ROUTE-ADD-072` and as `NOT-SERVED` entries in the drift artifact (47 today) —
  not re-filed. Worth noting for whoever works them: the 404 is untyped, so a generated client
  cannot tell "unsupported" from "wrong path".
- **`move-session` does not enforce the declared `sessionID` pattern `^ses`** (`"nope"` reaches the
  lookup and answers the same `400 MoveSessionError`). No user-visible difference; left in the log
  rather than turned into a row.
- **`GET /project` returns `[]` on a workspace where worktrees exist.** Consistent with
  `SPEC-017` (ROUTE-FIX-004/005: the list comes from the `projects` table, a fresh DB has none);
  `/instance/*` and `/session` carry the single-workspace identity instead.
- **`consensus init` wrote a `.memory-bank/` into the server's CWD.** Expected for a memory-bank
  runtime, but a server launched from `$HOME` will create `~/.memory-bank` — noted, not filed.

## Performance (Step 2b) — nothing slow enough for a user to notice, so no PERF row

`hyperfine -w 1 -r 10` against the scratch instance (numbers include curl's own ~6-7 ms process
spawn, i.e. they are upper bounds):

| operation | mean ± σ | range |
|---|---|---|
| `GET /experimental/worktree` | 9.7 ms ± 0.8 | 8.9 – 10.9 ms |
| `GET /experimental/console` | 6.8 ms ± 0.4 | 6.3 – 7.5 ms |
| `POST worktree` + `DELETE worktree` (a real `git worktree add` + `remove`) | 22.4 ms ± 9.2 | 17.2 – 46.9 ms |

One-shot bootstrap: `consensus init` 2.4 s (migrations applied, key minted), first `200` from
`/api/v1/health` ≈ 5 s after launch in the same second as the shim banner. Single-request cold
`POST` from a freshly started process: 9.2 ms. Nothing here justifies a profile — no PERF row
filed (a win nobody can feel is not a finding).

## Verdict

🟡 **PROMISING-BUT-ROUGH** for this surface. The new families are *served for real* — they create
and destroy actual git worktrees, they refuse path escapes and unknown fields with typed errors,
and the console/copy families answer honestly instead of fabricating capability. What is rough is
the input contract of the two destructive worktree operations: `remove` ignores the declared body
and both `remove`/`reset` ignore the declared workspace selectors, so the create→remove loop that
a real client performs only closes on a server launched inside the repository.

## Reproduce

```bash
go build -o /tmp/df/consensus ./cmd/consensus
HOME=/tmp/df/home /tmp/df/consensus init --db-url sqlite:///tmp/df/dogfood.db
git init -q /tmp/df/proj && (cd /tmp/df/proj && git commit -q --allow-empty -m init)

# server A: launched OUTSIDE the repo
(cd /tmp/df && /tmp/df/consensus serve --db-url sqlite:///tmp/df/dogfood.db --port 18391 &)
KEY=…   # cs_ak_… printed once by init
P=/tmp/df/proj
curl -s -X POST -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"name":"w1"}' "http://127.0.0.1:18391/experimental/worktree?directory=$P"   # 200 + real worktree
curl -s -X DELETE -H "Authorization: Bearer $KEY" \
  "http://127.0.0.1:18391/experimental/worktree?directory=$P/.opencode-worktrees/w1"
# -> 400 WorktreeNotGitError "workspace /tmp/df is not a git repository"

# server B: launched INSIDE the repo
(cd /tmp/df/proj && /tmp/df/consensus serve --db-url sqlite:///tmp/df/s2.db --port 18392 &)
curl -s -X DELETE -H "Authorization: Bearer $KEY2" -H 'Content-Type: application/json' \
  -d "{\"directory\":\"$P/.opencode-worktrees/w1\"}" "http://127.0.0.1:18392/experimental/worktree"
# -> 400 WorktreeRemoveFailedError: the declared body is ignored
```
