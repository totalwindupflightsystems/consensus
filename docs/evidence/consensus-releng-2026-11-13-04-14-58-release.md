# Consensus release readiness evidence

Sweep: consensus-releng-2026-11-13-04-14-58
Repository: totalwindupflightsystems/consensus
Canonical remote: git@github.com:totalwindupflightsystems/consensus.git
Release remote alias: origin
Default branch: main
Certified candidate: 4c65e0a7a5fb8e75030f52cfaaf44697f31cdd9c
Candidate source: origin/main after fetch; local detached worktree at the certified SHA
Remote parity at census: 0/0

## Decision

NO CUT. The repository is technically ready for the proposed first release v0.1.0, but the canonical board still has RELEASE-CONSENSUS-3 blocked on explicit owner authorization naming this repository and version. A readiness/proposal row is not authorization. No tag, GitHub Release, or release asset was created.

The final candidate CI gate is green: run 43365237663 completed successfully on the exact certified SHA. This does not override the authorization gate. The candidate was re-anchored after the branch moved during the sweep; the earlier 773a86c and 2423bb3 measurements are superseded by this exact candidate.

## Release baseline and semver census

- No prior numeric release tag exists on the remote: `git ls-remote --tags origin` returned no refs.
- No GitHub Release object exists: `gh release list --repo totalwindupflightsystems/consensus --limit 20` returned no releases; the API release count is 0.
- First-cut proposal: v0.1.0. There is no previous tag range.
- Full reachable history at the certified candidate: 1,781 commits; 1,629 non-merge commits.
- Conventional-commit census over the full history: feat 114, fix 217, perf 2, other conventional 1,128, nonconventional 168.
- Breaking markers: bang subjects 0; `BREAKING CHANGE:` body footers 0.
- The two commits after the prior 2423bb3 tip are `ops:` commits only; no feat/fix/perf or breaking change entered the candidate delta. They are `773a86c ops: document consensus production compose surface` and `755907c ops: annotate QA-CONSENSUS-8 deploy probe`.
- Proposed bump remains v0.1.0 because this is the repository's first release, not a patch/minor increment from an existing tag. No major confirmation is currently relevant because no breaking marker was found.

## Version surfaces and repository-owned release path

- VERSION is 0.1.0.
- The newest numeric CHANGELOG heading is [0.1.0] — 2026-10-27.
- `bash scripts/release.sh --dry-run` passed all four checks at the certified candidate: clean tree, remote parity 0/0, VERSION/CHANGELOG consistency, and exact-head CI green on run 43365237663. It printed the v0.1.0 annotated-tag command and created no tag.
- `docs/release.md` documents the prep -> exact-SHA CI -> annotated tag -> release workflow order, verification layers, and rollback.
- `scripts/release.sh` is executable and is the repository-owned preflight/tag driver.
- `make release-dry` exists.
- `.github/workflows/release.yml` triggers on `v*` tags and runs GoReleaser with `contents: write`.
- `.goreleaser.yaml` is present and defines the Go release matrix, archives, and checksums.

## Exact-candidate CI

Run 43365237663, workflow CI, head SHA 755907cbb4a14bea1b18eec3b6499260b2b689b1, completed success.

Executed successful jobs: Build & Test; Postgres Integration; GoReleaser Config; GitReins Guard (Tier 1); Docker Build & Push; Build (linux/amd64); Build (linux/arm64); Build (darwin/amd64); Build (darwin/arm64); Build (windows/amd64).

Skipped by design: E2E (Real LLM) and GitReins Evaluator (Tier 2). These skipped jobs are not release-grade evidence for those optional legs and remain explicitly unverified.

## Build and artifact strategy

- Local `go build ./...` passed at the certified candidate.
- The targeted shim test `go test ./internal/shim/opencode/ -run TestDeclaredUnimplementedRoutesAnswerTypedEnvelope -count=1` passed.
- The release workflow owns the publish build: GoReleaser will create the declared cross-platform tar.gz assets and checksums after an authorized v0.1.0 tag. No publish build or asset upload was started because the authorization gate is absent.
- The Docker workflow job was green on the candidate and is configured for GHCR publication on the normal CI path; no tag-specific image publication was triggered.

## GitReins and dogfood

- No GitReins verdict is bound to the exact candidate SHA 755907cbb4a14bea1b18eec3b6499260b2b689b1. Historical verdicts are stale or bound to other task commits and are not candidate release evidence.
- The latest committed dogfood review is `docs/reviews/consensus-dogfood-review-2026-11-11.html`. It is stale relative to the candidate and was measured at f4ebedb. It reports the front door holding while the first real tool call still wedges: a live message is accepted, the iteration remains frozen, repeated `session already claimed by another worker` lines occur, and the requested file is not created.
- The current board still carries the relevant P0/P1 rows: DF-CONSENSUS-65 (tool-call planning wedge), DF-CONSENSUS-66 (fresh install tool surface), DF-CONSENSUS-67 (skills registry write path), WIRE-CONSENSUS-1/2/3, REVIEW-CONSENSUS-1, QA-CONSENSUS-REVIEW-1, QA-CONSENSUS-FOREMAN-1, and DF-CONSENSUS-068. These are measured open follow-ups, not red exact-candidate CI results, but they prevent a claim that the product has no known issues.
- No exact-candidate GitReins or fresh dogfood run was produced in this sweep.

## Known issues

- First real tool-calling sessions can wedge after the HTTP message is accepted; the dogfood workaround is to avoid tool-calling workflows. Tracking: DF-CONSENSUS-65 and WIRE-CONSENSUS-1.
- The fresh-install tool surface is incomplete and the skills registry write path is not wired according to the open board findings. Tracking: DF-CONSENSUS-66 and DF-CONSENSUS-67.
- Session claim cleanup does not cover the sessions table, so a dead worker can leave a session claimed. Tracking: REVIEW-CONSENSUS-1.
- The current dogfood review is stale at f4ebedb rather than an exact-candidate run; no current live workaround beyond the prior dogfood procedure is claimed. Tracking: consensus releng verification.

## Authorization and publication state

- RELEASE-CONSENSUS-3 is present on the fetched canonical board with status blocked and blocked_reason requiring Bane to explicitly authorize v0.1.0 for totalwindupflightsystems/consensus.
- No separate RELEASE-cut row or explicit owner authorization naming this repository and version was found in the fetched board/events.
- No tag, GitHub Release object, release artifact, checksum, or tag-triggered release CI exists yet.
- Next authorized action: re-fetch and verify the candidate again, then run `bash scripts/release.sh` only after explicit authorization. Push the resulting tag and verify the remote tag, non-empty Release assets, checksums/install smoke, and tag-triggered CI on the tag commit.

## Receipts

- Certified candidate: 4c65e0a7a5fb8e75030f52cfaaf44697f31cdd9c
- Candidate CI: 43365237663, completed success on the exact candidate
- Release preflight: `bash scripts/release.sh --dry-run`, exit 0
- Build: `go build ./...`, exit 0
- Targeted test: exit 0
- Board baseline validation: `boardctl validate`, RESULT: OK with 789 inherited warnings and 0 errors
- Evidence artifact is intended to be committed before the board carrier is updated.

## Pass 3 — final-tip delta re-anchor (2026-11-13)

The fetched canonical default branch advanced from 755907cbb4a14bea1b18eec3b6499260b2b689b1 to 4c65e0a7a5fb8e75030f52cfaaf44697f31cdd9c. The changed-path set outside `.coding-hermes/` and this readiness artifact is empty, so the prior material census and release-tooling checks carry by identical product-tree bytes. The candidate receipt was nevertheless re-queried at the new full SHA.

- Certified candidate: 4c65e0a7a5fb8e75030f52cfaaf44697f31cdd9c.
- Candidate CI: GitHub Actions run 43368905841, workflow CI, push to main, exact head SHA, completed success. Per-job readback: Build & Test, Postgres Integration, GoReleaser Config, GitReins Guard (Tier 1), Docker Build & Push, and all five cross-build jobs succeeded; E2E (Real LLM) and GitReins Evaluator (Tier 2) were skipped by design.
- Remote state rechecked: no v* tag refs and no GitHub Release objects. Existing tags were not moved and no release mutation was attempted.
- Repository-owned preflight: `bash scripts/release.sh --dry-run` exit 0; it passed clean-tree, parity 0/0, VERSION/CHANGELOG consistency, and exact-head CI. `go build ./...` exit 0. The targeted shim test exited 0.
- Full-history census at the certified candidate: 1,785 commits, 1,633 non-merges; feat 114, fix 217, perf 2, other conventional 1,132, nonconventional 168; bang subjects 0 and `BREAKING CHANGE:` footers 0. The four commits since the prior evidence candidate are bookkeeping/operations (`docs(releng)`, two `board(releng)`, and `qa`), with no product-path change and no semver materiality.
- GitReins: no verdict is bound to the exact candidate SHA. Historical verdicts remain stale and are not upgraded.
- Dogfood: the committed review remains at f4ebedb, stale relative to this candidate; it reports the first real tool-call wedge. Open P0/P1 rows remain, including DF-CONSENSUS-65, DF-CONSENSUS-66, DF-CONSENSUS-67, WIRE-CONSENSUS-1/2/3, REVIEW-CONSENSUS-1, QA-CONSENSUS-REVIEW-1, QA-CONSENSUS-FOREMAN-1, DF-CONSENSUS-068, QA-CONSENSUS-34, and QA-CONSENSUS-35.

Decision remains NO CUT. RELEASE-CONSENSUS-3 is still status blocked and requires explicit owner authorization naming totalwindupflightsystems/consensus and v0.1.0. No RELEASE-cut row or equivalent authorization was found in the fetched board/events. No tag, GitHub Release, asset, version surface, or board status was mutated by this sweep.
