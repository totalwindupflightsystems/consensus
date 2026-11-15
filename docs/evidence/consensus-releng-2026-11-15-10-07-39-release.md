# Consensus release readiness evidence

Sweep: consensus-releng-2026-11-15-10-07-39
Repository: totalwindupflightsystems/consensus
Canonical remote: git@github.com:totalwindupflightsystems/consensus.git
Release remote alias: origin
Default branch: main
Certified candidate: beb6bd4a5964897d0d426141b043a44597cda0ca
Candidate source: origin/main after fetch; detached verification worktree at the certified SHA (tree e362324b7c54ed975cdf84e3d847725f85a165a5, `git status --porcelain` = 0 lines)
Remote parity at census: 0/0 (local `HEAD` == `origin/main` == beb6bd4a)

## Decision

NO CUT. Three independent gates are red, in this order:

1. **Authorization is absent.** RELEASE-CONSENSUS-3 remains `blocked` and requires an explicit owner directive naming totalwindupflightsystems/consensus and v0.1.0. A readiness row, a proposal, or a lane sweep is not authorization. Silent absence of a denial is likewise not authorization.
2. **The release toolchain would ship 19 reachable vulns.** `govulncheck ./...` exits 3 at the certified candidate: "Your code is affected by 19 vulnerabilities from 1 module and the Go standard library." `go.mod` pins `go 1.26.0` + `toolchain go1.26.5`, and the toolchain directive is authoritative for the GoReleaser build, so the published binaries carry the vulnerable stdlib regardless of which patch the CI runner installs. Fixed-in runs to **go1.26.11**, plus `golang.org/x/text >= v0.41.0`. Tracked as RELEASE-CONSENSUS-6, still `open` with no fix branch.
3. **The product's headline promise is broken live on a fresh install.** The most recent dogfood run (2026-11-14b, HEAD 94f0342c) returned 🔴 BROKEN AT THE PROMISE with open rows DF-CONSENSUS-069 (P0: a tool request naming a tool absent from `tools_registry` is recorded `completed` without executing — silent false success on the core loop), DF-CONSENSUS-070 (P1: turn ends with no assistant reply), DF-CONSENSUS-072 (P1: `session.last_message` returns the planner's private reasoning verbatim), DF-CONSENSUS-073 (P2), DF-CONSENSUS-071/074 (P3). Product source is **byte-identical** between that dogfooded HEAD and the candidate: `git diff --quiet 94f0342c beb6bd4a -- internal cmd pkg specs demo` exits 0, so the findings are not stale-by-repair.

Gates 2 and 3 are independent of gate 1: an explicit authorization today would still not make this candidate publishable.

Nothing was tagged, released, version-bumped, or force-updated by this sweep. RELEASE-CONSENSUS-4 and -5 remain spent heads (`origin/wt/RELEASE-CONSENSUS-4` at be22220e, `origin/wt/RELEASE-CONSENSUS-5` at 038821b2, both **0 commits ahead** of main) — neither is a toolchain fix.

## Release baseline and semver census

- No numeric release tag exists on the remote: `git ls-remote --tags origin` returned 0 refs; 0 local tags.
- No GitHub Release object exists: `gh release list --repo totalwindupflightsystems/consensus --limit 20` returned no rows.
- First-cut proposal: v0.1.0. There is no previous tag range, so no semver increment applies and the BREAKING-confirmation rule is not reachable.
- `VERSION` is 0.1.0; the newest numeric CHANGELOG heading is `## [0.1.0] — 2026-10-27`. Both surfaces are consistent and neither was mutated.
- Delta since the prior certified candidate e561d9205872655f2f55dd2251283f9138db4f48: **13 commits, 13 non-merges, 0 product-path files**. Path set: `.coding-hermes/board/{events,tasks}.jsonl`, `.coding-hermes/dogfood-log.md`, `README.md`, `docker-compose.yml`, `docker-compose.prod.yml`, and two `docs/evidence/` records. Subject census of the delta: board 4, docs 4, dogfood 3, qa: 1, fix 1. The single fix (`docker-compose`: make the postgres host port overridable, DF-CONSENSUS-075) is a patch to the deploy surface, carries no bump, and is the only non-bookkeeping code change in the range.
- Because the material path set of the delta is empty of product source, the class counts, CI map and preflight from the 2026-11-14 sweep are **carried**, not re-quoted as a fresh measurement of a new tree.

## Version surfaces and repository-owned release path

- `bash scripts/release.sh --dry-run` was run **this sweep** in a clean detached worktree at the certified candidate (tree e362324b, 0 modified paths) and exited **0** — all four preflight checks pass (clean tree, remote parity 0/0, VERSION/CHANGELOG consistency, exact-head CI green). It printed the annotated-tag command and created nothing.
- `docs/release.md` documents prep -> exact-SHA CI -> annotated tag -> release workflow -> verification -> rollback.
- `make release-dry`, `.goreleaser.yaml`, and `.github/workflows/release.yml` (triggers on `v*`, GoReleaser, `contents: write`) are present and unchanged.
- GoReleaser is the publish path; nothing was built for publication and no asset was uploaded.

## Exact-candidate CI

Run `43683729251`, workflow CI, head SHA `beb6bd4a5964897d0d426141b043a44597cda0ca`, completed **success**. Job-level read (not a branch-label read):

| Job | Conclusion |
|---|---|
| Build & Test | success |
| Postgres Integration | success |
| GitReins Guard (Tier 1) | success |
| GoReleaser Config | success |
| Docker Build & Push | success |
| Build (linux/amd64) | success |
| Build (linux/arm64) | success |
| Build (darwin/amd64) | success |
| Build (darwin/arm64) | success |
| Build (windows/amd64) | success |
| GitReins Evaluator (Tier 2) | skipped by design |
| E2E (Real LLM) | skipped by design |

The two skipped legs are not release-grade evidence and remain unverified at this candidate.

## Security scan — gate 2

`govulncheck ./...` at the certified candidate, run from the clean detached worktree, exited **3**. Full log: `fbb1c41eccdb59208005caa65b317f77e0ed52040597b1025c47e8d0c759b1ac`.

Reachability is through real call paths: `internal/db/postgres` -> `xml.Unmarshal`, `internal/api/server.go` -> `http.Transport.RoundTrip`, `specs/embed.go` -> `asn1.Unmarshal`, `internal/compression/worker.go` -> `http.Client.Do`.

| Package | Advisories | Found in | Fixed in |
|---|---|---|---|
| net/http | GO-2026-6859 | go1.26.5 | go1.26.11 |
| net/http | GO-2026-6603, 6605, 6609, 6610, 6611, 6612, 6613, 6617 (8) | go1.26.5 | go1.26.9 |
| net/http | GO-2026-5026, GO-2026-6089 | go1.26.5 | go1.26.6 |
| crypto/tls | GO-2026-6607 | go1.26.5 | go1.26.9 |
| crypto/tls | GO-2026-6090 | go1.26.5 | go1.26.6 |
| crypto/x509 | GO-2026-6809 | go1.26.5 | go1.26.10 |
| mime/multipart | GO-2026-6608 | go1.26.5 | go1.26.9 |
| net/url | GO-2026-6218 | go1.26.5 | go1.26.6 |
| encoding/xml | GO-2026-6088 | go1.26.5 | go1.26.6 |
| encoding/asn1 | GO-2026-5972 | go1.26.5 | go1.26.6 |
| golang.org/x/text | GO-2026-6629 | v0.39.0 | v0.41.0 |

Why this blocks a cut rather than being advisory:

- `go.mod` carries `go 1.26.0` and `toolchain go1.26.5`. The toolchain line is authoritative: `actions/setup-go` may install a newer 1.26 patch, but `go` re-resolves to the pinned 1.26.5 for the build, so GoReleaser emits vulnerable stdlib binaries.
- crypto/x509 fixed-in go1.26.10 is the same patch line that drops trust in the compromised emSign/eMudhra intermediate chain; a v0.1.0 cut at go1.26.5 would ship a TLS client that still trusts it.
- The scan also reports 2 vulnerabilities in imported packages and 3 in required modules that are **not reachable**; those are informational only.

## GitReins and dogfood

- No GitReins verdict is bound to the certified candidate. The newest history entry is 2026-11-14 for task `qa-consensus-8-ops-verify` at commit `8d38f8b17453`: tier 1 **FAIL** (the `tests` step: `demo` smoke keyless start timed out at 20s and `internal/shim/opencode` failed — both long-running suites at high machine load; `internal/harness` passed in 232s and `opencode` ran 192s), tier 2 **PASS** (all three QA-CONSENSUS-8 arms reproduced, `no_concerns_found`). That verdict grades an ops row on a different commit and is therefore **not** candidate evidence either way. Release-grade GitReins status at beb6bd4a: unverified.
- Dogfood: latest run is 2026-11-14b (tick consensus-dogfood-2026-11-14-23-03-21, HEAD 94f0342c). Its 2026-11-15 00:37Z entry is a re-fire closeout of the same label, not a new run. Verdict on the promise: 🔴 BROKEN. Open rows it filed: DF-CONSENSUS-069 (P0), -070 (P1), -072 (P1), -073 (P2), -071 (P3), -074 (P3); -075 (compose host port) was fixed by `beb6bd4a` itself. Product source is byte-identical between 94f0342c and the candidate, so none of these is stale-by-repair.
- DF-CONSENSUS-6-R2 (P1, multi-turn chat stateless, constant `messages=3`) is a separate recurrence row, not a duplicate of the toolchain gate.

## Known issues at the candidate

- DF-CONSENSUS-069 (P0, silent false success for unregistered tools): open, no fix branch.
- DF-CONSENSUS-070, DF-CONSENSUS-072 (P1: missing assistant reply; private-reasoning leak via `session.last_message`), DF-CONSENSUS-6-R2 (P1): open.
- DF-CONSENSUS-073 (P2), DF-CONSENSUS-071, DF-CONSENSUS-074 (P3): open.
- RELEASE-CONSENSUS-6 (P1, release toolchain): open; this is the only release-mechanical blocker.
- QA-CONSENSUS-8/33 and the ops rows are complete; no release dependency sits on them.

## Authorization and publication state

- RELEASE-CONSENSUS-3 is `blocked` on an explicit owner directive. Board events 1032-1049 (the whole window since the 2026-11-14 sweep, latest id 1049 at 2026-11-15T06:02:20Z) contain only dogfood/ops/qa task creations and audits — no authorization of any kind.
- Remote release surface: 0 tag refs, 0 Release objects. No existing tag was moved or deleted; a force-move never becomes necessary here because no tag exists.
- After authorization the cut sequence is: re-fetch, re-run `bash scripts/release.sh --dry-run` from a clean tree (it re-checks exact-SHA CI), `bash scripts/release.sh`, `git push origin v0.1.0`, then verify four layers — remote tag ref, non-empty Release assets + `checksums.txt`, tag-triggered release CI green on the tag commit, and a `version` smoke on a downloaded binary. **RELEASE-CONSENSUS-6 must land first**, and the P0 must be repaired or explicitly waived by the owner, otherwise the artifacts are published vulnerable and with a known silent-failure core loop.

## Receipts

- Certified candidate: `beb6bd4a5964897d0d426141b043a44597cda0ca` (origin/main, parity 0/0), tree `e362324b7c54ed975cdf84e3d847725f85a165a5`
- Candidate CI: run `43683729251`, success on the exact SHA, 10 jobs green / 2 skipped by design
- Repository preflight: `bash scripts/release.sh --dry-run` exit 0 in a clean detached worktree at the candidate
- Security: `govulncheck ./...` exit 3, 19 reachable; log sha256 `fbb1c41eccdb59208005caa65b317f77e0ed52040597b1025c47e8d0c759b1ac`
- Toolchain surfaces: `go 1.26.0` / `toolchain go1.26.5`; `golang.org/x/text v0.39.0`
- Dogfood: 2026-11-14b, HEAD 94f0342c, 🔴 broken; product source identical to candidate (`git diff --quiet ... -- internal cmd pkg specs demo` rc 0)
- Remote release surface: `git ls-remote --tags origin` 0 refs; `gh release list` 0 objects
- No tag, Release object, VERSION/CHANGELOG edit, GoReleaser run, or ref force-update occurred in this sweep

Decision: **NO CUT** — authorization absent, toolchain gate open, P0 dogfood promise broken.

Measured 2026-11-15T10:0xZ by the consensus-releng satellite.
