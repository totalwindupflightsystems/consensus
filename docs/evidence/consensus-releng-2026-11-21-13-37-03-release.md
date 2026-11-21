# Consensus release readiness evidence

Sweep: consensus-releng-2026-11-21-13-37-03
Repository: totalwindupflightsystems/consensus
Canonical remote: git@github.com:totalwindupflightsystems/consensus.git (release remote alias `origin`)
Default branch: main
Certified candidate: 7a618fd923742e84d160b9109664f8036a62f684
Candidate source: `origin/main` after `git fetch origin --tags --prune`; tree `74dcbea099386c806eb3cd85f23bb31bfb3412b8`
Remote parity at census: 0/0 (`git rev-list --left-right --count origin/main...HEAD` -> `0	0`, local `HEAD` == `origin/main` == 7a618fd9)
Verification worktree: clean detached worktree at the candidate, `git status --porcelain` = 0 lines

## Decision

**NO CUT.** One release-mechanics gate that was red on 2026-11-15 is now **green**, but the
cut is still withheld. Three independent reasons, in priority order:

1. **Authorization is absent (unchanged, decisive).** RELEASE-CONSENSUS-3 remains `blocked`
   with `blocked_on: owner:release-cut-authorization`. No owner directive naming
   totalwindupflightsystems/consensus and v0.1.0 exists in the triggering message, the board,
   or the gitreins task list. A readiness sweep, a proposal row, or a satellite tick is **not**
   authorization, and silence is not authorization. Live proof that nothing has been published:
   `git tag -l | wc -l` -> 0, `git ls-remote --tags origin | wc -l` -> 0,
   `gh release list --limit 10` -> no rows.
2. **A P1 product defect on the core loop is open.** DF-CONSENSUS-084 (P1, filed 2026-11-18):
   on the successful-tool continuation path the billed assistant reply is never persisted —
   311 completion tokens billed, `planning: committing turn=1` logged, but no `text_block`
   lands in `memory_events`, `last_message` stays null, and `iteration_commits` is empty. A
   message sent mid-continuation is consumed with zero provider calls and no error surface.
   A first public release that silently drops a paid reply is not a release we cut without an
   owner decision.
3. **The readiness package the cut depends on has not landed.** RELEASE-CONSENSUS-007 (P1,
   `pending`) requires `docs/release/v0.1.0-notes.md` with release notes, a gate-evidence
   block and a dry-run section. `ls docs/release/` -> *No such file or directory*. REL-3's own
   acceptance criterion (1) ("RELEASE-CONSENSUS-007 is status=complete with its evidence
   cited") is therefore unmet.

### Gate movement since the 2026-11-15 sweep

| Gate | 2026-11-15 | 2026-11-21 (this sweep) |
|---|---|---|
| Owner authorization | RED | RED — still absent |
| Govulncheck (RELEASE-CONSENSUS-6) | **RED** — exit 3, 19 reachable, `toolchain go1.26.5`, `x/text v0.39.0` | **GREEN** — exit 0, "No vulnerabilities found", 0 reachable, `toolchain go1.26.11`, `golang.org/x/text v0.41.0` |
| Dogfood P0/P1 named by that sweep (DF-069/070/072) | RED | GREEN — all three `complete`; DF-069's premise was *reversed* (fail-closed confirmed) and DF-070/072 re-proved fixed in the 2026-11-18 run |
| New P1 on the same surface (DF-084) | n/a | RED — open |
| Readiness package (REL-007) | n/a | RED — not delivered |

The security fix is real and was verified live, not inherited: RELEASE-CONSENSUS-6 is
`complete` and both pins are on the candidate tree — `go.mod:9 toolchain go1.26.11`,
`go.mod:35 golang.org/x/text v0.41.0`. `govulncheck ./...` at the candidate exits **0** with
"No vulnerabilities found" (log sha256 `3016e51e4eac0d421674d2128bbbdefb2924b4646e0c14a1ab034977ad73fae5`),
against exit 3 / 19 reachable / log sha256 `fbb1c41e...` on 2026-11-15. The published binary
would no longer carry the vulnerable stdlib.

## Repository-owned release path (used as-is, per doctrine)

The repo ships its own toolchain and this sweep used it rather than inventing a path:

- `scripts/release.sh --dry-run` — four fail-closed checks (clean tree, remote parity 0/0,
  `VERSION` == newest numeric CHANGELOG heading, successful `ci.yml` run for the exact HEAD SHA).
- `make release-dry` — Makefile wrapper for the above.
- `.github/workflows/release.yml` — triggers on `tags: v*`, runs GoReleaser with `contents: write`.
- `.goreleaser.yaml` — GoReleaser v2, linux/darwin × amd64/arm64, `CGO_ENABLED=0`, tar.gz +
  `checksums.txt`, `-X main.version={{ .Version }}`.
- `docs/release.md` — documented order of operations, three-layer verification and rollback.

**Preflight result (run this sweep, in the clean detached worktree at the candidate):**

```
OK  [1/4] clean tree
OK  [2/4] remote parity 0/0 (HEAD == origin/main at 7a618fd92374)
OK  [3/4] VERSION (0.1.0) matches newest CHANGELOG heading ([0.1.0])
OK  [4/4] CI green on HEAD (run 44900256917 succeeded for 7a618fd92374)
Preflight passed for totalwindupflightsystems/consensus @ 7a618fd92374 — release v0.1.0 is ready to cut.
```

exit 0. It printed the annotated-tag command and created nothing.

Why the worktree: on the primary checkout the same script **refuses** check 1 —

```
REFUSED: check 'clean tree' failed — working tree has uncommitted changes:
     M .vfs/graph/.symbols_cache.json
    ?? .gitreins/review-state.json
    ?? .gitreins/review-state.lock
```

`.vfs/graph/.symbols_cache.json` is **tracked** (`git ls-files --error-unmatch` succeeds) and
churns (`git log -- .vfs/graph/.symbols_cache.json` shows repeated `chore: refresh symbol cache`
commits), and neither `.vfs/graph/.symbols_cache.json` nor `.gitreins/review-state.*` is
covered by `.gitignore`. A tracked regenerable cache means the ordinary checkout cannot pass
its own clean-tree gate. Filed as RELEASE-CONSENSUS-008 (P2, hygiene) — it is a workflow
defect, not a publication blocker, because a cut is properly made from a clean worktree.

## Release baseline and semver census

- **No release tag exists on the remote or locally**: `git ls-remote --tags origin` -> 0 refs;
  `git tag` -> empty; `git describe --tags --always` -> bare sha `7a618fd9`.
- **No GitHub Release object exists**: `gh release list --repo totalwindupflightsystems/consensus --limit 10` -> no rows.
- First-cut proposal remains **v0.1.0**. With no prior tag there is no last-tag range, no semver
  increment to derive, and the BREAKING-confirmation rule is unreachable — but v0.1.0 still
  needs explicit owner authorization because it is the first publication.
- `VERSION` = `0.1.0`; newest numeric CHANGELOG heading = `## [0.1.0] — 2026-10-27`. Consistent;
  neither surface was mutated by this sweep.
- Full-history conventional-commit census (1946 commits, the entire tree since no tag exists):

  | class | count |
  |---|---|
  | board | 725 |
  | fix | 247 |
  | chore | 224 |
  | feat | 114 |
  | docs | 101 |
  | merge | 49 |
  | test | 46 |
  | shim | 26 |
  | qa | 21 |
  | ci | 9 |
  | wave | 5 |
  | perf | 3 |
  | style | 2 |
  | **BREAKING CHANGE / `type!:`** | **0** |

  feat 114 / fix 247 / BREAKING 0. If this were a post-1.0 cut on a prior tag the shape would
  argue MAJOR only on a breaking marker — there is none in 1946 commits, so a bump decision
  would be MINOR; as a first tag it is simply v0.1.0. No tag was created and none was moved.

## CI evidence (exact SHA, not a branch label)

`gh run list --repo totalwindupflightsystems/consensus --limit 20` -> 20/20 `completed/success`;
the newest is the candidate's own run.

Run **44900256917** — workflow `CI`, `headSha` 7a618fd92374, conclusion **success**:

| job | result |
|---|---|
| Build & Test | success |
| Postgres Integration | success |
| GoReleaser Config | success |
| GitReins Guard (Tier 1) | success |
| Docker Build & Push | success |
| GolangCI-Lint | success |
| Build (linux/amd64) | success |
| Build (linux/arm64) | success |
| Build (darwin/amd64) | success |
| Build (darwin/arm64) | success |
| Build (windows/amd64) | success |
| GitReins Evaluator (Tier 2) | skipped by design |
| E2E (Real LLM) | skipped by design |

11 executed, 11 green, 2 skipped by design. No `continue-on-error` masking in the executed set.

## GitReins verdicts

`.gitreins/` carries 75 history entries and no verdict is bound to the candidate SHA.
One task is stranded in-progress: `REL-VERIFY-930` — "Evidence refresh for RELEASE-CONSENSUS-3:
verify build+CI+version stamp, confirm no authorization yet". This sweep performs exactly that
work; the task was **not** re-judged here because a Tier-2 evaluation in this checkout adds a
full suite run to a host already at load ~24 and would risk a suite-collision false FAIL. Left
for a quiet-window judge pass; noted rather than silently closed.

## Dogfood results

Newest run: `docs/dogfood/2026-11-18-tool-continuation-budget.md` (tick
`consensus-dogfood-2026-11-18-00-54-47`, HEAD `0e697977`). Verdict label 🟡 PROMISING-BUT-ROUGH.

- DF-CONSENSUS-070, -072, -6-R2, -069 re-proved **FIXED** on the 11-18 run (069's premise
  reversed: unregistered tool now records `status=failed` + `TOOL_ERROR` and is fed back to the
  model — fail-closed).
- **DF-CONSENSUS-084 (P1) is open**: continuation reply not persisted on the successful-tool
  path; queued message swallowed with no call and no answer.
- DF-CONSENSUS-073-R2 / -074-R2 recurrences were filed and are now `complete`; the original
  DF-073 (P2) and DF-074 (P3) rows still read `pending`, plus DF-CONSENSUS-076 (P2,
  `CONSENSUS_LLM_PROVIDER=deepseek` rejected / `CONSENSUS_LLM_MODEL` does not reach the session).
- **Board-hygiene finding:** DF-CONSENSUS-6-R2 is `pending` but the 11-18 dogfood explicitly
  re-proved multi-turn stateful (a later turn cited `17 × 23 = 391` from an earlier tool result
  unprompted; prompt grew 1764→1961 tokens, cached_tokens 1280–1408). That row looks
  **stale-by-repair**; it belongs to the dogfood lane, so this sweep reports it rather than
  closing it. Releng verifies; it does not develop.

Open P1/P2 at census: 6 P1 pending (QA-CONSENSUS-REVIEW-1 and QA-CONSENSUS-FOREMAN-1 are
bunker-disk / harness, INT-CI-CONSENSUS-002 is judge-binding infra, DF-CONSENSUS-6-R2 is the
stale-pending above, DF-CONSENSUS-084 is the product blocker, RELEASE-CONSENSUS-007 is prep),
8 P2 pending. Of these, exactly one is a product-code blocker: DF-CONSENSUS-084.

## Actions taken / not taken

Taken:
- Certified the candidate, ran the repo's own preflight 4/4 in a clean detached worktree.
- Re-measured the security gate live at the candidate (`govulncheck`, exit 0).
- Committed this evidence record and the board annotations below.
- Dispatched the one board-authorized, non-owner-gated P1 (RELEASE-CONSENSUS-007) as the
  tick's single worker; WAVE_BUDGET 2, one worker used.

Not taken (deliberately):
- **No tag.** `git tag -a v0.1.0` was not run: no owner authorization, and a P1 blocker is open.
- **No Release object, no asset, no GoReleaser run, no publish.**
- **No VERSION or CHANGELOG edit** — both surfaces are already consistent at 0.1.0.
- **No tag force-move.** There is no tag to move, and none was created.
- **No board row closed** outside what this sweep itself produced.

## Cut procedure on authorization (unchanged, ready)

```
bash scripts/release.sh --dry-run            # must be 0/4 in a clean tree at the candidate
gh release view v0.1.0 --repo totalwindupflightsystems/consensus   # must be "not found" before
git tag -a v0.1.0 -m "Release v0.1.0" && git push origin v0.1.0    # release.yml publishes
```

Then three-layer verification per `docs/release.md` (remote tag -> prep SHA, `gh release view`
with non-empty assets incl. `consensus_0.1.0_{linux,darwin}_{amd64,arm64}.tar.gz` + `checksums.txt`,
release workflow green on the tagged SHA). Rollback: `gh release delete` first, then delete the
tag remote + local — nothing consumes releases yet, so rollback stays complete and cheap.

**Precondition on the owner directive:** if it names v0.1.0 at a newer candidate than
7a618fd9, this certification does not carry — re-run the preflight at the named SHA, because
CI green on this SHA is not CI green on that one.

---

## Pass 13 addendum — readiness gap closed, cut still withheld (2026-11-21 14:31Z)

**Decision: still NO CUT.** The withholding reason did not weaken; the *mechanics* did change.

### What changed this pass

RELEASE-CONSENSUS-007 (the only board-authorized row this satellite could execute) landed a
v0.1.0 readiness package as `4390b3e2`, fast-forwarded onto `main` and pushed (parity `0/0`).

| Item | Before | After |
|---|---|---|
| `docs/release/v0.1.0-notes.md` | absent | 239 lines on `main` |
| `CHANGELOG.md` | unreleased section only, `[0.1.0]` dated 2026-10-27 | unreleased → `[0.1.0]` promotion, original 2026-10-27 facts preserved word-for-word |
| `VERSION` | `0.1.0` | `0.1.0` (correctly untouched — already correct) |
| Repo preflight | 4/4 only in a hand-made detached worktree | 4/4 `exit 0` on the landed tree `4390b3e2` |

### Gate evidence at `4390b3e2` (foreman-run, not worker-reported)

| # | Gate | Result |
|---|---|---|
| 1 | `bash scripts/release.sh --dry-run` in a clean detached worktree | `exit 0` — 4/4 OK |
| 2 | `go build ./...` | `exit 0` |
| 3 | `go vet ./...` | `exit 0` |
| 4 | `go test ./... -count=1` | `exit 0` — 40 packages, 37 `ok`, 3 no-test-files |
| 5 | `python3 scripts/audit/wirecheck.py .` | `RESULT: PASS` |
| 6 | CI on `4390b3e2` | run `44914197925` → success |

The script printed `release v0.1.0 is ready to cut`, emitted the exact tag command, and created
nothing. Tag count: 0 local, 0 remote. GitHub Releases: 0.

### Stale row closed

`DF-CONSENSUS-073` (iterations endpoint answering `[]`) was still `pending` and had already been
fixed by its own recurrence row `DF-CONSENSUS-073-R2`. Verified rather than assumed:
`git merge-base --is-ancestor 869bcaef origin/main` → true; R2 commit `363a39c4`; tier-2 run
`d90c4149` all-PASS. Closed as superseded so the board stops re-dispatching fixed work.

### New row filed

`RELEASE-CONSENSUS-009` (P1, CI-integrity): repo-local `core.hooksPath=.git/hooks` is **relative**,
and inside a linked worktree `git` reports `.git/hooks` while `.git` there is a *file*
(`gitdir: .../worktrees/<name>`), so `ls .git/hooks` is `Not a directory` and the pre-commit Tier-1
guard never executes. Every fleet worker commits from a worktree, and `guards.allow_skips: true`,
so nothing re-imposes the gate until `gitreins task complete`.

### Dispatch disclosure (seat truth, not label)

The tick's authoritative worker lane `openai/gpt-6.1-terra` via `xkiro` returned
`503 model_maintenance` until **2026-11-24T08:34Z** (same for `gpt-6-luna`), and the fallback
`9router :20128` was refused outright (`curl` exit 7). `deepseek-v4-pro` via `deepseek` was then
refused for credits, so the worker ran on `accounts/fireworks/models/deepseek-v4p1-flash-1017-spot`
via `fireworks` — confirmed from `agent.log` `API call #N: model=...` lines and `state.db`
`session_model_usage`, not from the dispatch label. One earlier `-z` attempt (session `local-777ece`)
silently fell back to `zai-org/GLM-4.7-Flash` mid-run; its process tree was killed before it could
write, and the worktree was verified clean (`dirty=0`) before the successful dispatch.

### Why the cut is still withheld

1. **Authorization is still absent (decisive).** `RELEASE-CONSENSUS-3` remains `blocked` with
   `blocked_on: owner:release-cut-authorization`. A readiness row landing is not a cut order.
2. **`DF-CONSENSUS-084` (P1) remains open** — a continuation reply billed to the provider but never
   persisted on the successful-tool path: data loss on the accounting path, not a release mechanical.

The cut is now exactly one authorized command away, and the command is printed in
`docs/release/v0.1.0-notes.md`. On an owner directive naming this SHA:
`git tag -a v0.1.0 -m "Release v0.1.0" && git push origin v0.1.0`, then `gh release create v0.1.0
--notes-file docs/release/v0.1.0-notes.md --verify-tag`, then confirm `gh release view v0.1.0` and
the release workflow green on the tagged SHA. If the directive names a newer candidate, this
certification does not carry.

## Fire 4, pass 5 — steward re-measure (2026-11-21T15:18Z / 10:18 local)

Same tick (`consensus-releng-2026-11-21-13-37-03`), fifth fire of the family: passes 1-4 are in the
ledger at `/releng/2026-11-21/tick-1337-v010-readiness*`. Steward rules applied: re-measure only the
decisive facts, carry the rest with its timestamp, append a delta instead of rewriting the lineage.

### Identity check (passed)

The candidate certified by passes 1-2 is unchanged as a *product* tree. The tip moved by one
bookkeeping commit after pass 4:

| fact | value |
|---|---|
| certified candidate | `7a618fd9` |
| tip now | `6746b0bc` (2026-11-21T10:05:25-05:00, "board: RELEASE-CONSENSUS-009 verification evidence (releng fire 3)") |
| product-path diff, candidate..tip | **0 files** |
| changed paths, candidate..tip | `.coding-hermes/board/tasks.jsonl`, `.coding-hermes/board/events.jsonl`, `CHANGELOG.md`, `docs/evidence/consensus-releng-2026-11-21-13-37-03-release.md`, `docs/release/v0.1.0-notes.md` |
| tags | 0 local, 0 remote |
| GitHub Releases | 0 (`gh release list` empty) |

### Exact-tip CI: discharged

The in-flight leg pass 4 left behind is closed. `6746b0bc` check-runs are terminal:
**11 success, 2 skipped, 0 failed** — CI push run `44917309229` (run_number 1054) green.

### Gitreins verdicts

0 of the repo's verdicts name `7a618fd9` or `6746b0bc`. Today's three verdicts (`06433492`,
`8700c114`, `ef3b2bd1`) grade `0bd9d862` and `39eee99e` — neither is the candidate. Unchanged from
pass 4: there is no external verdict on the release candidate, so this leg stays "None issued".

### Blockers re-checked (all still stand)

| item | state at 15:18Z |
|---|---|
| `RELEASE-CONSENSUS-3` | `blocked`, `blocked_on: owner:release-cut-authorization` — no authorization appeared |
| `DF-CONSENSUS-084` | P1 `pending` — continuation reply billed but not persisted |
| `RELEASE-CONSENSUS-008` | `pending` — clean-tree gate still broken **in the primary checkout**: `M .vfs/graph/.symbols_cache.json` tracked, plus untracked `.gitreins/review-state.json` / `.lock` |
| `RELEASE-CONSENSUS-009` | `pending` — branch `rel009-worktree-hooks` @ `a0a3727d` is pushed but **not an ancestor of main**; docs clause + merge + post-merge re-run outstanding |

### Carried, not re-measured

`govulncheck`, `go build`/`vet`, `wirecheck`, the dogfood run, and the 4/4 preflight all carry from
passes 1-2 at the same product tree. The only drift-prone preflight leg — gate [1/4] clean-tree — is
re-verified dirty above and is owned by `RELEASE-CONSENSUS-008`.

### Delta action: one wave worker on RELEASE-008

`WAVE_BUDGET: 2`, one worker used (the second slot is deliberately held: `RELEASE-009`'s remaining
work is a merge of already-verified code plus a docs clause — foreman acceptance, not worker authoring).

Seat disclosure (authoritative lane unavailable, brief's own escape clause invoked): `openai/gpt-6.1-terra`
via `xkiro` returned `503` twice — `agent.log` 10:09:17 (`local-a4ad824a`) and 10:14:59
(`20261121_101457_499c87`), message `Model "openai/gpt-6.1-terra" is tempora…`, both followed by
`Retrying API call in 600.0s`. `--provider 9router` was **not honored** — the client was still built
with `provider=xkiro` — so the 9router terra lane could not be reached by flag; no retry spent on it.
Both failing process trees were killed and the worktree verified clean (`git status` empty, HEAD still
`6746b0bc`) before re-dispatch. Worker ran on `accounts/fireworks/models/deepseek-v4p1-flash-1017-spot`
via `fireworks` — the same disclosed substitute pass 4 used.

### Decision: NO CUT (unchanged)

The readiness verdict does not move: authorization is absent and there are zero tags, so no release
may be created. The cut remains one authorized command chain away on candidate `7a618fd9`, printed in
`docs/release/v0.1.0-notes.md`.

---

## Fire 5, pass 6 — RELEASE-CONSENSUS-008 landed; preflight 4/4 (2026-11-21T15:55Z)

### What changed since pass 5

Pass 5 closed with `75fa94e4` (untrack `.vfs/graph/.symbols_cache.json`) judged
**necessary but not sufficient**: six other generated artifacts stayed tracked and
one `hilo graph search` still dirtied them. Wave worker 2 (`41cad9af`, branch
`rel008b-vfs-untrack`, worktree `wt-rel008b-1337`) untracked the remaining
generated set. Both commits are now on `main`.

    main: 6746b0bc..41cad9af (fast-forward, 2 commits) then 4654ff78 (board + evidence)

`git ls-tree -r origin/main -- .vfs` → `.vfs/manifest.yaml` only.

### Independent proof (releng probe, not the worker's self-report)

| Step | Command | Result |
|---|---|---|
| 1 | delete every `.vfs/graph/*` artifact, confirm tree clean | `git status --porcelain -uall` empty |
| 2 | `hilo graph warm` | 6194 edges / 492 files / 515 files coverage, rc=0 |
| 3 | `git status --porcelain -uall` after regeneration | **empty** — no tracked artifact moved |
| 4 | `git ls-files .vfs` | `.vfs/manifest.yaml` (1 path) |
| 5 | `git push origin main` | `6746b0bc..4654ff78`, HEAD == origin/main |
| 6 | `bash scripts/release.sh --dry-run` | **4/4 OK** |

### Preflight on `main` @ 4654ff78 (native tooling, unmodified)

    OK  [1/4] clean tree
    OK  [2/4] remote parity 0/0 (HEAD == origin/main at 4654ff787c1e)
    OK  [3/4] VERSION (0.1.0) matches newest CHANGELOG heading ([0.1.0])
    OK  [4/4] CI green on HEAD (run 44920879369 succeeded for 4654ff787c1e)

    Preflight passed for totalwindupflightsystems/consensus @ 4654ff787c1e — release v0.1.0 is ready to cut.

CI on the exact tip: 11 success, 2 skipped, 0 failed (workflow `CI`, run 1055,
`https://github.com/totalwindupflightsystems/consensus/actions/runs/44920879369`).

Note on check 4: the first dry-run refused with "no successful ci.yml run found"
while `gh api .../check-runs` already showed all 11 checks green — `gh run list
--status success` lags check-run completion by seconds. The gate is correct; it
is not racy in the wrong direction (it never passes on red).

### RELEASE-CONSENSUS-008 — closed complete

Row fields written: `status=complete`, `worker_status=complete`,
`commit_hash=41cad9af`, `branch=rel008b-vfs-untrack`,
`ci_result=success (11/2, run 44920879369 on 4654ff78)`,
`guard_result=PASS`, `detail` → this file.
Events: `1191 task_completed`, `1192 audit` (tick 956). Header `ticks_total=956`,
`last_commit=4654ff78`.

**Documented deviation.** The row's literal criterion `git ls-files .vfs == 0` is
not met — `.vfs/manifest.yaml` deliberately stays tracked. It is authored
configuration (fuse/ninep/cli/mcp interfaces, ports, mount points) and `hilo init`
never overwrites an existing manifest, so untracking it would ship a checkout
without project config. The criterion's intent — a cache refresh never dirties
the tree — is proven by steps 1–3 above.

### Release decision: NO CUT (unchanged), now one authorized command from the cut

- Authorization row `RELEASE-CONSENSUS-3` is still `blocked` on
  `owner:release-cut-authorization`. The satellite verifies; it does not execute
  an unauthorized cut.
- Repo state: 0 local tags, 0 remote tags, 0 GitHub Releases. `v0.1.0` would be
  the first release.
- Once authorized, the native script performs the cut unsupervised:
  `bash scripts/release.sh` (creates annotated `v0.1.0`, `release.yml` takes over
  on `git push origin v0.1.0`).
- If any commit lands first (e.g. the `RELEASE-CONSENSUS-009` worktree-hooks
  branch), HEAD moves and `VERSION`/`CHANGELOG`/CI on the new SHA must be
  re-proven — the gate will refuse otherwise, by design.

### Board state after this pass

| Row | Status | Note |
|---|---|---|
| RELEASE-CONSENSUS-008 | **complete** | landed 75fa94e4 + 41cad9af, 4/4 preflight |
| RELEASE-CONSENSUS-009 | pending | branch `rel009-worktree-hooks` @ a0a3727d pushed, **not an ancestor of main**; docs clause + merge + post-merge re-run outstanding |
| RELEASE-CONSENSUS-3 | blocked | `owner:release-cut-authorization` |
| DF-CONSENSUS-084 | pending (P1) | continuation reply billed but not persisted |
