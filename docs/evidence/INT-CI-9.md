# INT-CI-9 — Replace rate-limited public ECR base images (Docker Build & Push)

## Failure being fixed

GitHub Actions run **39625937328** (commit `d7d05cd684c277bef79e3d2356d48751dfd22c65`)
failed in the **Docker Build & Push** job: the `public.ecr.aws` manifest request for
the `alpine:3.21` base image returned HTTP **429 `toomanyrequests: Data limit
exceeded`**. The retry step (`nick-fields/retry@v3`, `max_attempts: 3`) re-ran the
build against the **same rate-limited source** and also failed; every other CI job
passed. This is the same recurring class recorded by INT-CI-002 / INT-CI-006 /
INT-CI-7 — retrying the same endpoint is not a durable fix, so the shipped default
source is changed.

No repository spec anchors the base-image registry (checked `specs/` — the only
base-image references are unqualified official names in `specs/021-repository-layout.md`
and `specs/026-dashboard-ui.md`), so switching to official Docker Hub references does
not falsify any spec surface.

## Source change (exact)

`Dockerfile` lines 6–7 — ARG defaults only; both build stages (`FROM ${GO_IMAGE} AS
builder`, `FROM ${ALPINE_IMAGE}`) and the ARG override mechanism are unchanged:

```diff
-ARG GO_IMAGE=public.ecr.aws/docker/library/golang:1.26-alpine
-ARG ALPINE_IMAGE=public.ecr.aws/docker/library/alpine:3.21
+ARG GO_IMAGE=docker.io/library/golang:1.26-alpine
+ARG ALPINE_IMAGE=docker.io/library/alpine:3.21
```

`.github/workflows/ci.yml` — Docker job primary buildx invocation (build-args, lines
275–276) and the retry invocation (`--build-arg` flags, lines 303–304) now pass the
same `docker.io/library/...` values; the retry step itself is preserved and now
retries the new source. The explanatory comment above the retry step was rewritten to
describe the new source and the historical 429 class (run 39625937328). No other CI
job, registry push/auth behavior, or application code was touched.

## Verification (exact commands and observed results)

All commands run in the worktree `/home/kara/worktrees/consensus-INT-CI-9`
(branch `wt/INT-CI-9`) against the edited files.

| # | Command | Observed result |
|---|---------|-----------------|
| 1 | `docker build --check .` | `Check complete, no warnings found.` — exit 0. Buildkit resolved live registry metadata for both new bases (`load metadata for docker.io/library/golang:1.26-alpine`, `...alpine:3.21`) with no 429. |
| 2 | `python3 /tmp/int-ci9-validate.py` (YAML parse + content assertions: ci.yml parses; primary build-args block == `{GO_IMAGE=docker.io/library/golang:1.26-alpine, ALPINE_IMAGE=docker.io/library/alpine:3.21}`; exactly 2 retry `--build-arg` flags with the same values; zero `public.ecr.aws` lines in Dockerfile and ci.yml; both Dockerfile ARGs present with the new defaults) | `ALL CHECKS PASSED` — exit 0 |
| 3 | `git diff --check` | clean (no whitespace/conflict-marker errors) |
| 4 | `grep -c "public.ecr.aws" Dockerfile .github/workflows/ci.yml` | `Dockerfile:0` / `.github/workflows/ci.yml:0` |
| 5 | `grep -n "docker.io/library" Dockerfile .github/workflows/ci.yml` | 7 hits: Dockerfile:6,7; ci.yml:275,276 (build-args), 282 (comment), 303,304 (retry `--build-arg`) |
| 6 | `go build ./...` | exit 0, no output |
| 7 | `go vet ./...` | exit 0, no output |
| 8 | `go test ./... -count=1 -short` | exit 0 — 37 packages `ok`, 0 `FAIL` (log: `/tmp/int-ci9-go-test.log`) |

The repository has no established CI/config-test surface for workflow files (no
workflow-parsing tests exist under `internal/`), so per the acceptance contract this
task ships the scripted content validation above (item 2, checks 1–4 mirror the
contract's acceptance points) rather than a new Go test package. Hosted CI has NOT
been run from this commit; this artifact records local verification only.

## Not changed (deliberate)

- Both build stages and the `FROM ${VAR}` indirection (ARG overrides remain
  available for future mirror pinning).
- The retry step (`nick-fields/retry@v3`) — kept, now retrying docker.io.
- All other CI jobs, GHCR login/metadata/push behavior, and all application code.

ch:trace row=INT-CI-9 spec=none:no repository spec anchor identified wave=none:serial test=.github/workflows/ci.yml::base-image-source-check doc=docs/evidence/INT-CI-9.md prompt=/tmp/consensus-foreman-2026-10-21-15-37-47-INT-CI-9-brief.txt evidence=docs/evidence/INT-CI-9.md witness=github:run/39625937328 verdict=none:foreman-runs-judge-after-commit commit=881fd07ddba9129ef37046e9ca7c637ede460f2f memory=none:not-written

STATUS: complete
TIER: T3 — locally verified end to end without hosted CI: `docker build --check .`
passed with live docker.io metadata resolution (result 1); scripted YAML + content
validation passed all assertions (result 2); `git diff --check` clean (result 3);
`go build ./...` and `go vet ./...` clean (results 6–7); `go test ./... -count=1
-short` exit 0 with 37 packages ok, 0 FAIL (result 8). Hosted GitHub Actions was not
invoked from this commit; the foreman runs the judge after commit.

EVIDENCE: docs/evidence/INT-CI-9.md
