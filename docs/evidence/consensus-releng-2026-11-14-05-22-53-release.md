# Consensus release readiness evidence

Sweep: consensus-releng-2026-11-14-05-22-53
Repository: totalwindupflightsystems/consensus
Canonical remote: git@github.com:totalwindupflightsystems/consensus.git
Release remote alias: origin
Default branch: main (GitHub API `defaultBranchRef.name`; the local `origin/HEAD` symref still points at the stale `master` branch, which exists at a3da94e — it is not the release branch)
Certified candidate: e561d9205872655f2f55dd2251283f9138db4f48
Candidate source: origin/main after fetch; detached verification worktree at the certified SHA
Remote parity at census: 0/0

## Decision

NO CUT. Two independent gates are red, in this order:

1. **Authorization is absent.** RELEASE-CONSENSUS-3 remains `blocked` and requires explicit owner authorization naming totalwindupflightsystems/consensus and v0.1.0. A readiness/proposal row is not authorization.
2. **A new security gate is red and would be shipped.** `govulncheck ./...` reports 19 reachable vulnerabilities at the certified candidate, all in the Go standard library plus golang.org/x/text. `go.mod` pins `toolchain go1.26.5`, so GoReleaser would compile the v0.1.0 binaries with that toolchain no matter which patch release the CI runner installs. Fixed-in versions run to go1.26.11, so the toolchain gate is `>= go1.26.11` plus `golang.org/x/text >= v0.41.0`.

Gate 2 is new in this sweep and is independent of gate 1: even an explicit authorization would not make this candidate publishable as-is. Filed as RELEASE-CONSENSUS-6.

No tag, GitHub Release, release asset, version surface, or board status was mutated by this sweep.

## Release baseline and semver census

- No numeric release tag exists on the remote: `git ls-remote --tags origin` returned 0 refs.
- No GitHub Release object exists: `gh release list --repo totalwindupflightsystems/consensus --limit 20` returned no rows.
- First-cut proposal: v0.1.0. There is no previous tag range, so no semver increment applies.
- Reachable history at the certified candidate: 1,793 commits, 1,641 non-merges.
- Conventional-commit census (non-merge subjects): feat 114, fix 218, perf 2, docs 90, chore 214, test 37, qa 18, shim 26, dogfood 24, merge 20, board 663, other conventional 127, nonconventional 168.
- Breaking markers: bang subjects 0; `BREAKING CHANGE:` body footers 0. No major-version proposal is relevant.
- Delta since the prior certified candidate 755907cbb4a14bea1b18eec3b6499260b2b689b1: 12 commits — `fix: persist DF-CONSENSUS-65 tool handoff before tool_exec` (73d3c97, product), `qa:`, `ops:`, `docs(releng):`, and 7 board bookkeeping commits. The one product-path change is a fix, so it carries no bump.

## Version surfaces and repository-owned release path

- VERSION is 0.1.0; the newest numeric CHANGELOG heading is `[0.1.0] — 2026-10-27`.
- `bash scripts/release.sh --dry-run` passed 4/4 at the certified candidate: clean tree, remote parity 0/0, VERSION/CHANGELOG consistency, exact-head CI green (run 43560513397 for e561d9205872). Exit 0; it printed the annotated-tag command and created nothing.
- `docs/release.md` documents prep -> exact-SHA CI -> annotated tag -> release workflow -> verification -> rollback.
- `make release-dry`, `.goreleaser.yaml`, and `.github/workflows/release.yml` (triggers on `v*`, GoReleaser, `contents: write`) are present and unchanged.

## Exact-candidate CI

Run 43560513397, workflow CI, head SHA e561d9205872655f2f55dd2251283f9138db4f48, completed `success`. The repository-owned preflight independently re-queried and accepted this run by SHA (not by branch label).

E2E (Real LLM) and GitReins Evaluator (Tier 2) remain skipped by design; they are not release-grade evidence and remain unverified.

## Security scan — new gate (govulncheck)

`govulncheck ./...` at the certified candidate exited 3 with:

> Your code is affected by 19 vulnerabilities from 1 module and the Go standard library.

Reachability is through real call paths, e.g. `internal/db/postgres/postgres.go` -> `xml.Unmarshal`, `internal/api/server.go` -> `http.Transport.RoundTrip`, `specs/embed.go` -> `asn1.Unmarshal`, `internal/compression/worker.go` -> `http.Client.Do`.

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

- `go.mod` carries `go 1.26.0` and `toolchain go1.26.5`. The toolchain line is authoritative for the build: `actions/setup-go` with `go-version: "1.26"` installs a latest-1.26 toolchain, but `go` re-resolves to the pinned 1.26.5, so the GoReleaser matrix emits binaries whose stdlib is the vulnerable one.
- crypto/x509 fixed-in go1.26.10 is the same patch line that drops trust in the compromised emSign/eMudhra intermediate chain. A v0.1.0 cut at go1.26.5 ships a TLS client that still trusts it.
- The scan also found 2 vulnerabilities in imported packages and 3 in required modules that are not reachable; those are informational only.

## Build and artifact strategy

- `go build ./...` exit 0 at the certified candidate.
- `go test ./internal/api -run TestDF65ToolCallHandoffRealBinary -count=1` PASS (3.87s) — a real-binary session reaches `status=idle`, `tool_request=completed`, `tool_results=1`. This is the first sweep at which the P0 tool-call wedge is independently reproduced as fixed.
- Publish build stays with GoReleaser after an authorized tag; nothing was built for publication and no asset was uploaded.
- Docker job was green on the candidate on the normal CI path; no tag-specific image publication was triggered.

## GitReins and dogfood

- No GitReins verdict is bound to e561d9205872655f2f55dd2251283f9138db4f48. `.gitreins/history/` stops at 2026-08-17 and there is no `verdicts/verdict.json`; historical verdicts are stale and are not candidate evidence.
- The committed dogfood review remains stale at f4ebedb (`docs/reviews/consensus-dogfood-review-2026-11-11.html`). Its headline — the first real tool call wedges — is now contradicted by 73d3c97 plus the passing real-binary integration test above, but no exact-candidate dogfood run exists, so end-to-end repair is **not** claimed.
- Board reconciliation observations (reported, not fixed by this lane):
  - ba3eb54 and c96ff4c are titled "record DF-CONSENSUS-65 landed verification", yet DF-CONSENSUS-65 is still `in_progress` at the candidate — the row needs a closer, not another annotation.
  - DF-CONSENSUS-068 requires `internal/harness/df65_planning_tool_dispatch_test.go`; that path does not exist. The dispatch-contract test that did land is `internal/api/df65_tool_handoff_integration_test.go`. The row's premise needs correcting by its owning lane.

## Known issues (refreshed against the candidate)

- DF-CONSENSUS-65 (P0, tool-call planning wedge): **fixed** at 73d3c97, verified by real-binary test at the candidate; the board row is not yet closed.
- DF-CONSENSUS-66 (fresh-install tool surface), DF-CONSENSUS-67 (skills registry write path), WIRE-CONSENSUS-1/2/3, REVIEW-CONSENSUS-1, QA-CONSENSUS-REVIEW-1, QA-CONSENSUS-FOREMAN-1, DF-CONSENSUS-068: open.
- New: the release toolchain is below the fixed Go patch level and would ship 19 reachable advisories (RELEASE-CONSENSUS-6).

## Authorization and publication state

- RELEASE-CONSENSUS-3 is `blocked` on explicit owner authorization naming this repository and v0.1.0; no RELEASE-cut row and no owner directive was found in the fetched board or events.
- Remote release surface: 0 tag refs, 0 Release objects. No existing tag was moved or deleted.
- After authorization the cut sequence is: re-fetch, re-run `bash scripts/release.sh --dry-run` (it re-checks exact-SHA CI), `bash scripts/release.sh`, `git push origin v0.1.0`, then verify four layers — remote tag ref, non-empty Release assets + checksums, tag-triggered release CI on the tag commit, and `version` smoke on a downloaded binary. RELEASE-CONSENSUS-6 must land first, otherwise the artifacts are published vulnerable.

## Receipts

- Certified candidate: e561d9205872655f2f55dd2251283f9138db4f48 (origin/main, parity 0/0)
- Candidate CI: run 43560513397, success on the exact candidate
- Repository preflight: `bash scripts/release.sh --dry-run` exit 0 (4/4)
- Build: `go build ./...` exit 0
- Targeted test: `go test ./internal/api -run TestDF65ToolCallHandoffRealBinary -count=1` PASS
- Security: `govulncheck ./...` exit 3, 19 reachable
- Remote release surface: `git ls-remote --tags origin` 0 refs; `gh release list` empty
- Evidence commit precedes the board carrier update; the carrier ships alone.

Measured 2026-11-14T05:36Z.

## Pass 2 — re-fire of tick consensus-releng-2026-11-14-05-22-53 (2026-11-14T05:52Z)

The lane re-fired on the same tick id 11 minutes after this record was written. Re-anchored on `origin/main` = `9248f474039fcc71e951845afc5ec87c5f596b66` (fetch at 05:52Z; local checkout `e561d92` sits 2 commits behind and was used only to host the read-only `govulncheck` build, whose result is tree-scoped, not HEAD-scoped).

Movement since the certified candidate: 2 commits, both bookkeeping — `1ecc3a1` (this record, under `docs/`) and `9248f47` (the board carrier). The material path set — everything outside `.coding-hermes/`, `.gitreins/`, `docs/`, `CHANGELOG.md`, `README.md` — is **empty**. The commit census, class counts, CI map and preflight above therefore carry unchanged; they are not re-quoted as a fresh measurement of a new tree.

New receipts this pass:

| Leg | Measurement |
|---|---|
| Carrier CI (`9248f47`) | run `43563003325`, workflow CI, completed/**success** — 10 jobs executed and green: Build & Test, Postgres Integration, GitReins Guard (Tier 1), GoReleaser Config, Docker Build & Push, Build linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. Skipped by design: GitReins Evaluator (Tier 2), E2E (Real LLM). |
| Evidence CI (`1ecc3a1`) | run `43562468162`, CI, completed/**success** |
| Security gate, independently re-measured | `govulncheck ./...` exit 3 — 19 reachable vulnerabilities; `go.mod` still `go 1.26.0` + `toolchain go1.26.5`; `golang.org/x/text v0.39.0` |
| Release surface | `git ls-remote --tags origin` = 0 refs; `gh release list` = 0 objects |
| Authorization | none found; RELEASE-CONSENSUS-3 still `blocked`; latest audit event id 1039 |
| Blocking fix | RELEASE-CONSENSUS-6 still `pending`; no fix branch — the only release heads are `wt/RELEASE-CONSENSUS-4` and `wt/RELEASE-CONSENSUS-5`, both **0 commits ahead** of main (spent worktrees, not a fix) |

Deliberately not done: no tag created, no Release published, no `VERSION` or `CHANGELOG.md` mutation, no GoReleaser invocation, no force-update of any ref.

`scripts/release.sh --dry-run` was **not** re-run this pass. The only checkout that could host it is 2 commits behind `origin/main`, and its exact-SHA CI check would fail there on the stale HEAD for reasons unrelated to the project — a false red. The 4/4 preflight above was measured on the byte-identical tree and is carried as such, not restated as new.

Decision unchanged: **NO CUT**. The single blocking gate remains RELEASE-CONSENSUS-6 — toolchain below the fixed Go patch level, 19 reachable advisories, including the emSign/eMudhra trust line that `crypto/x509` closes at go1.26.10. Authorization is still absent.

This pass is idempotent-bound by the marker `consensus-releng-2026-11-14-05-22-53/pass2` in the board event log.

Measured 2026-11-14T05:52Z.

## Pass 3 — third re-fire of tick consensus-releng-2026-11-14-05-22-53 (2026-11-14T06:45Z)

The lane re-fired a third time on the same tick id, 52 minutes after this record was first written. Re-anchored on `origin/main` = `05f89c37283613a4a14eab1c8f9cd03ab2782230` (fetch at 06:44Z). The local checkout `e561d92` sits 4 commits behind and still carries pass 1's superseded index, so it was read in place only — `git worktree add` for a clean detached tree was refused by the unattended-surface consent gate this pass, was not retried, and the carrier was built with a throwaway index instead.

Movement since the certified candidate: 3 paths, all bookkeeping — `.coding-hermes/board/events.jsonl`, `.coding-hermes/board/tasks.jsonl`, `docs/evidence/consensus-releng-2026-11-14-05-22-53-release.md`. The material path set (everything outside `.coding-hermes/`, `.gitreins/`, `docs/`, `CHANGELOG.md`, `README.md`) is **empty**. The commit census, class counts, CI map and preflight stand exactly as measured on `e561d92`; they are carried, not restated as a fresh measurement of a new tree.

New receipts this pass:

| Leg | Measurement |
|---|---|
| Carrier CI (`05f89c3`) | run `43564566738`, workflow CI, completed/**success** — 10 jobs green: GoReleaser Config, Build & Test, Docker Build & Push, GitReins Guard (Tier 1), Postgres Integration, and the five cross-builds (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64). Skipped by design: GitReins Evaluator (Tier 2), E2E (Real LLM). |
| Pass-2 evidence CI (`16bef13`) | run `43564380375`, CI, completed/**success** |
| Security gate, independently re-measured | `govulncheck ./...` exit **3**, `19` reachable vulnerabilities; `go.mod` still `go 1.26.0` + `toolchain go1.26.5`; `golang.org/x/text v0.39.0` |
| Release surface | `git ls-remote --tags origin` = 0 refs; `gh release list` = 0 objects |
| Authorization | still none; RELEASE-CONSENSUS-3 `blocked`; the marker-bearing audit event is id 1040 |
| Blocking fix | RELEASE-CONSENSUS-6 still `pending`; no branch above go1.26.5 — `wt/RELEASE-CONSENSUS-4` and `wt/RELEASE-CONSENSUS-5` are both 0 commits ahead of main |

Deliberately not done, again: no tag created, no Release published, no `VERSION` or `CHANGELOG.md` mutation, no GoReleaser invocation, no force-update of any ref, and no cleanup of the shared checkout's superseded index.

`scripts/release.sh --dry-run` was **not** re-run this pass. The only checkout that could host it is now 4 commits behind `origin/main` and carries uncommitted board/evidence changes; preflight check 1 (clean tree) and check 2 (parity 0/0) would both refuse for reasons that say nothing about the project — a false red. The 4/4 preflight above was measured on the byte-identical tree and is carried as such.

Decision unchanged: **NO CUT**. The single blocking gate remains RELEASE-CONSENSUS-6 — toolchain below the fixed Go patch level, 19 reachable advisories, including the emSign/eMudhra certificate line that `crypto/x509` closes at go1.26.10. Owner authorization for a v0.1.0 cut is still absent, and a third pass that finds no movement is not a substitute for it.

This pass is idempotent-bound by the marker `consensus-releng-2026-11-14-05-22-53/pass3` in the board event log.

Measured 2026-11-14T06:45Z.
