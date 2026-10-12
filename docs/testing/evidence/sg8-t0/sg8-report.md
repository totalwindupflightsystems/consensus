# SG-8 T0 remeasurement — non-vacuous, but RED

Date: 2026-10-12 · Task: `TEST-CONSENSUS-001` · Tier: T0 build/unit

Commit under test: `5a28bf0403552c8434eb54b8715f60399ea624a2` on `master`, equal to
`origin/master` when the sweep started. SG-6 T4 and SG-7 T10 were not rerun.

## Verdict

**T0 is RED, not GREEN.** The actual build, vet, env-clean full Go suite, fresh binary, and
keyless smoke all passed with nonzero observations. The tier still fails three explicit plan
contracts, and one manifest selector is VOID:

1. `gofmt -l $(git ls-files '*.go')` found committed format drift in
   `internal/api/sessions_test.go` (`gofmt.log`, byte-preserved patch in `gofmt-diff.txt.gz`). The file was clean
   relative to HEAD before this task; it was not changed because product-code repair is outside this
   evidence-only scope.
2. The fresh-DB precondition fails because ignored `dev.db` already exists at repo root (471040
   bytes, mtime 2026-08-31). It was not removed or mutated (`dev-db-stat.txt`).
3. The checkout was not clean. Before the evidence directory existed, `git status --porcelain`
   showed the pre-existing modified `.gitreins/tasks.yaml`, the unrelated untracked
   `.vfs/graph/.symbols_cache.json`, and the in-flight scoped `t0_sweep.sh`. The first two were
   present before this task and were neither staged nor modified.
4. Manifest cell C-235's exact selector exits 0 while executing **zero tests**. It is recorded as
   `VOID`, not PASS (`tool-registry-selector.result.txt`).

The SG-1 non-vacuity guard passes only its narrow claim: `packages_ok=40` is nonzero. It does not
convert the RED tier to green.

## Executed counts

The env-clean command was one uncached **full** suite, not a short-suite duplicate:

```text
env -i HOME=$HOME PATH=$PATH TMPDIR=<scratch> CGO_ENABLED=0 \
  timeout 600 go test -json -count=1 ./...
```

Observed from `go-test.jsonl`:

- package targets: **40**
- packages with tests passing: **36**
- packages with no test files: **4**
- failed packages: **0**
- top-level tests: **1793 passed, 0 failed, 37 skipped** (**1830 observed**)
- all terminal test events including subtests: **2217 passed, 0 failed, 37 skipped** (**2254 observed**)
- keyless smoke: **2 top-level tests observed**, **1 mocked LLM call**, exit 0

`skipped-top-level-tests.tsv` names every skipped top-level test. The skips are not silently counted
as passes. `no-test-packages.txt` names the four packages contributing to the 40-package target
without test files.

## Commands and exit codes

| Cell | Command | Exit | Evidence / disposition |
|---|---|---:|---|
| package inventory | `go list ./...` | 0 | 40 targets (`package-list.txt`) |
| format | `gofmt -l <all tracked Go files>` | command 0, cell FAIL | 1 drifted file (`gofmt.log`) |
| build | `CGO_ENABLED=0 go build ./...` | 0 | 40 targets, empty error log |
| fresh binary | `make fresh`; `bin/consensus --version` | 0 / 0 | stale root binary removed; version `0.1.0`; SHA-256 recorded |
| vet | `go vet ./...` | 0 | 40 targets, empty error log |
| env-clean full suite | command above | 0 | 1830 top-level terminal events; 0 failures |
| keyless smoke | `timeout 90 go test -json -count=1 -run Smoke ./demo/` | 0 | 1 actual mocked LLM call |
| pool-fix ancestry | `git merge-base --is-ancestor ad8afd8 HEAD` | 0 | required fix is in tree |
| fresh DB | `test ! -e dev.db` | 1 | pre-existing ignored DB is present; cell FAIL |
| C-235 selector | `go test -json -count=1 -run TestToolRegistry ./internal/harness/ ./internal/api/` | 0 | 0 test events; VOID |
| C-234 prerequisite | `test -e scripts/probe-tvf.go` | 1 | script absent; unsupported, no substitute fabricated |
| non-vacuity guard | `guard_void.sh t0-results.json t0-counter.tsv` | 0 | 40 observables; proves non-VOID only |
| GitReins (default diff mode) | `gitreins guard` on the staged evidence/script diff | 2 | DEGRADED PASS: no Go files staged, so build/lint/tests skipped; not green evidence |
| GitReins (whole tree) | `gitreins guard --full` | 0 | PASS; secrets, static analysis, Go build, Go lint, and Go tests all executed |
| sweep | `bash scripts/battery/t0_sweep.sh docs/testing/evidence/sg8-t0` | 1 | expected aggregate RED |

C-063/C-128/C-190/C-233 are duplicate short-suite variants and were not rerun after the stronger
env-clean full suite; they retain no new verdict. C-236 was executed separately after staging. Its
default diff-mode guard was DEGRADED because no Go file was staged; the stronger whole-tree guard
passed, but the gofmt half still found drift, so the combined cell is not green.

## Raw and recomputable evidence

- `t0.json` — machine-readable verdict, counters, cells, and unexecuted/unsupported rows.
- `go-test.jsonl` — complete Go JSON event stream, not a hand-written count.
- `go-test.result.txt` — derived counter summary and exact command.
- `smoke.jsonl` — complete smoke event stream with the mock-call output.
- `MANIFEST.txt` — SHA-256 pins for the decision-bearing evidence.

Recompute the headline counts without trusting this report:

```bash
EV=docs/testing/evidence/sg8-t0
jq -s '[.[] | select(.Action=="pass" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$EV/go-test.jsonl"
jq -s '[.[] | select(.Action=="fail" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$EV/go-test.jsonl"
jq -s '[.[] | select(.Action=="skip" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$EV/go-test.jsonl"
jq '.counters, .cells, .unexecuted, .verdict' "$EV/t0.json"
sha256sum -c "$EV/MANIFEST.txt"
```

## Residual blocker

SG-8 T0 remains uncompleted/RED. The evidence proves that compilation, vet, unit execution, and the
keyless smoke work at the frozen SHA, but a clean rerun cannot be called green until the committed
format drift and stale root DB are resolved and C-235 points at tests that actually exist. The
pre-existing `.gitreins/tasks.yaml` change and unrelated `.vfs` cache also prevent the exact
clean-tree provenance cell in this shared checkout; this task intentionally did not modify them.

No persistent Consensus process was started by the sweep. A postcheck found an unrelated existing
`/tmp/readme-consensus-bin/consensus serve` process; it was not touched.
