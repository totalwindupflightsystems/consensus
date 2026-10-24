# Cutting a Consensus release

The release surface is mechanical: a `v*` tag push triggers
[.github/workflows/release.yml](../.github/workflows/release.yml), which runs
GoReleaser against [.goreleaser.yaml](../.goreleaser.yaml) (linux/darwin,
amd64/arm64, CGO_ENABLED=0, tar.gz archives + `checksums.txt`) and attaches the
assets to a GitHub Release. This document is the procedure for producing that
tag safely; `scripts/release.sh` is the local preflight that enforces steps 2
and 3.

No consumers are pinned yet — there are no downstream `go.mod require` pins,
no external deploy scripts tracking a release, and the GHCR image is not
anonymous-pullable — so a bad cut can be rolled back fully with step 7.

## Order of operations (do not reorder)

```
prep commit → CI green on that exact SHA → annotated tag → release.yml publishes
```

### 1. Prepare the release commit (docs/VERSION only)

The binary reads the repo-root `VERSION` file at compile time
(`version.go`, `go:embed`), and GoReleaser stamps `main.version` from the tag —
both must agree with the CHANGELOG before anything is tagged:

- `CHANGELOG.md`: promote every `[Unreleased]` bullet into a new
  `## [X.Y.Z] — YYYY-MM-DD` section (real cut date), then re-open an **empty**
  `## [Unreleased]` section directly above it.
- `VERSION`: set its single line to `X.Y.Z` (no `v`, no trailing whitespace).
- Nothing else ships in this commit. Commit message convention: `chore: release vX.Y.Z prep`.

`X.Y.Z` is a judgment call: pre-1.0, additive work bumps MINOR or PATCH; any
BREAKING marker in the promoted notes demands MAJOR (none so far in this
repo's history).

### 2. Push the prep commit and wait for green CI on that exact SHA

`git push origin main`, then verify the run belongs to the prep commit itself —
not to an older sha or a similarly-named branch:

```bash
git rev-parse HEAD                                # the SHA CI must be green on
gh run list --repo totalwindupflightsystems/consensus --commit "$(git rev-parse HEAD)" --workflow ci.yml
```

A cut is only ever made from a commit whose own SHA has a successful run.

### 3. Run the local preflight

```bash
bash scripts/release.sh --dry-run
```

The script fails closed, naming the failing check, if any of these is untrue:

1. **clean tree** — no uncommitted changes (staged or unstaged, untracked included);
2. **remote parity 0/0** — `HEAD == origin/main` after a fresh fetch (nothing
   unpushed, nothing unpulled);
3. **version consistency** — `VERSION` equals the newest numeric CHANGELOG
   heading, i.e. the prep commit from step 1 landed;
4. **CI green on HEAD** — a successful `ci.yml` GitHub Actions run exists for
   the exact HEAD SHA (queried by SHA, never by branch label).

On success it prints the exact tag command. Run it without `--dry-run` (or run
the printed command by hand) to create the local annotated tag.

### 4. Create and push the annotated tag

```bash
git tag -a "vX.Y.Z" -m "Release vX.Y.Z" && git push origin "vX.Y.Z"
```

The push is the point of no return for publishing: `release.yml` fires on `v*`
and runs GoReleaser with `contents: write`.

### 5. Confirm the release workflow ran

```bash
gh run list --repo totalwindupflightsystems/consensus --workflow release.yml
```

A green `Release / GoReleaser Release` run on the tagged SHA is the expected
outcome (the tag-validation step refuses any `GITHUB_REF_NAME` that does not
start with `v[0-9]`).

## Three-layer verification

All three must hold before announcing the release:

```bash
# Layer 1 — the tag is on the remote and points at the prep commit
git fetch --tags origin && git rev-parse "vX.Y.Z^{commit}"   # == prep-commit SHA

# Layer 2 — the Release object exists with non-empty assets
gh release view vX.Y.Z --repo totalwindupflightsystems/consensus
#   Assets must include consensus_X.Y.Z_{linux,darwin}_{amd64,arm64}.tar.gz + checksums.txt

# Layer 3 — the release workflow is green on the tagged SHA
gh run list --repo totalwindupflightsystems/consensus --workflow release.yml --commit "$(git rev-parse vX.Y.Z^{commit})"
```

Optional smoke: `go build ./cmd/consensus && ./bin/consensus --version` on a
fresh checkout of the tag prints `X.Y.Z` (or the GoReleaser-stamped tag via
`-X main.version`).

## Rollback

Because nothing consumes releases yet, rollback is complete and cheap:

```bash
# Remove the published Release object first (deleting only the tag would leave an orphaned release)
gh release delete vX.Y.Z --repo totalwindupflightsystems/consensus --yes

# Delete the tag on the remote, then locally
git push origin :refs/tags/vX.Y.Z
git tag -d vX.Y.Z
```

Fix the problem, re-run the preflight from step 3, and cut `vX.Y.(Z+1)`
(do not re-cut the same version number over changed content — reuse a version
only if the original cut published zero artifacts).

## Local simulation

`act -W .github/workflows/release.yml` exercises the GoReleaser matrix as a
snapshot build (no publish). See the header comment in
[.github/workflows/release.yml](../.github/workflows/release.yml) for its
constraints — publishing itself is only ever exercised by a real `v*` push.
