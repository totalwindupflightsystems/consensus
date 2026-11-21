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
