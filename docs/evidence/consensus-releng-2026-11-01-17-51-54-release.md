# Consensus release readiness evidence

Sweep: consensus-releng-2026-11-01-17-51-54
Repository: totalwindupflightsystems/consensus
Canonical remote: git@github.com:totalwindupflightsystems/consensus.git
Default branch: main
Candidate SHA: d97e42bbab9fe4e7198fe371ae10e3341cbe8354
Candidate status: origin/main == candidate; clean detached audit worktree

Decision: NO-GO for publication. Technical preflight is ready for the proposed first release v0.1.0, but no explicit owner authorization or RELEASE-cut board row naming this repository and version is present. This readiness row does not authorize tagging.

Release baseline and version:
- No prior numeric release tag exists on origin (git ls-remote --tags origin 'v*' returned none).
- No GitHub Release object exists; gh release view v0.1.0 returned not found.
- First-cut proposal: v0.1.0.
- Full reachable history at candidate: 1,725 commits; feat 114; fix/perf 220; BREAKING CHANGE footer/subject markers 0; other 1,391.
- VERSION is 0.1.0 and matches the newest numeric CHANGELOG heading [0.1.0].

Repository release contract:
- scripts/release.sh exists and is the repository-owned preflight/tag driver.
- docs/release.md documents prep commit -> exact-SHA CI green -> annotated tag -> tag workflow, three-layer verification, and rollback.
- Makefile release-dry target exists.
- .github/workflows/release.yml is active and triggers on v* tags; it runs GoReleaser with contents:write.
- .goreleaser.yaml builds linux/darwin amd64/arm64 CGO_ENABLED=0 tar.gz archives and checksums.txt.
- Candidate dry-run: bash scripts/release.sh --dry-run exited 0; all four checks passed and no tag was created.

Exact-candidate CI:
- Run 41261184464, workflow CI, head d97e42bbab9fe4e7198fe371ae10e3341cbe8354, completed success.
- Executed jobs: 10 success, including Build & Test, Postgres Integration, Docker Build & Push, GoReleaser Config, GitReins Guard (Tier 1), and five cross-build jobs.
- Skipped by design: E2E (Real LLM), GitReins Evaluator (Tier 2). The skipped jobs have zero steps; they are not green release gates and remain explicitly unverified.
- Branch protection: GitHub reports protected=false.

GitReins and dogfood:
- No GitReins verdict is bound to the candidate SHA. Historical latest verdicts include older route-task failures and passes; they are not candidate release evidence.
- Open release-adjacent dogfood/QA rows include DF-CONSENSUS-32 (P2 skipped install/bunker), DF-CONSENSUS-52 (P3 skipped install/bunker), QA-CONSENSUS-8 (P3 deploy probe 000), QA-CONSENSUS-9 (P3 3GB cap), and QA-CONSENSUS-31 (P3 vacuous UI/chaos coverage). These are not red exact-head CI results, but remain open follow-ups.
- Existing RELEASE-CONSENSUS-3 remains pending and is the authorization/readiness proposal; no RELEASE-cut row was found.

Publication state:
- No tag, Release object, or release artifact was created by this sweep.
- Next authorized action: re-read this evidence at cut time, confirm candidate and CI have not moved, then run the repository-owned release driver only after an explicit authorization naming v0.1.0. Verify remote tag, non-empty Release assets, downloaded artifact checksums/execution, and tag-triggered CI on the tag commit.
