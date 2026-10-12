#!/usr/bin/env bash
# t0_sweep.sh — SG-8 T0 build/unit remeasurement.
#
# Runs one env-clean, uncached full Go suite (not a duplicate short suite), plus
# format/build/vet/fresh-binary/smoke checks. It emits machine-readable counters
# and raw logs. A dirty checkout or any failed mandatory cell makes the tier RED;
# nonzero package/test counts prove only that the run was non-vacuous.
#
# Usage: bash scripts/battery/t0_sweep.sh [evidence-dir]
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
OUT=${1:-"$ROOT/docs/testing/evidence/sg8-t0"}
TARGET_SHA=${TARGET_SHA:-$(git -C "$ROOT" rev-parse HEAD)}
TEST_TIMEOUT_SECONDS=${T0_TEST_TIMEOUT_SECONDS:-600}
SMOKE_TIMEOUT_SECONDS=${T0_SMOKE_TIMEOUT_SECONDS:-90}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/consensus-sg8-t0.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

status_for_rc() {
  if [ "$1" -eq 0 ]; then printf 'PASS'; else printf 'FAIL'; fi
}

for tool in git go gofmt jq make sha256sum timeout; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf 't0_sweep: BLOCKED — required tool missing: %s\n' "$tool" >&2
    exit 2
  fi
done

# Capture the checkout before creating the evidence directory. The script itself
# may be an in-flight scoped change; the raw clean-tree contract still sees it.
git -C "$ROOT" status --porcelain=v1 >"$TMP/dirty-before.txt"
head_sha=$(git -C "$ROOT" rev-parse HEAD)
origin_sha=$(git -C "$ROOT" rev-parse origin/master 2>/dev/null || true)
mkdir -p "$OUT"
cp "$TMP/dirty-before.txt" "$OUT/dirty-before.txt"

printf '%s\n' "$head_sha" >"$OUT/head.txt"
printf '%s\n' "$origin_sha" >"$OUT/origin-master.txt"
printf '%s\n' "$TARGET_SHA" >"$OUT/target-sha.txt"
printf 'go_version=%s\n' "$(go version)" >"$OUT/toolchain.txt"
printf 'test_timeout_seconds=%s\nsmoke_timeout_seconds=%s\n' \
  "$TEST_TIMEOUT_SECONDS" "$SMOKE_TIMEOUT_SECONDS" >>"$OUT/toolchain.txt"

provenance_rc=0
[ "$head_sha" = "$TARGET_SHA" ] || provenance_rc=1
[ -s "$OUT/dirty-before.txt" ] && provenance_rc=1

root_binary_before=false
[ -e "$ROOT/consensus" ] && root_binary_before=true
printf 'root_binary_before=%s\n' "$root_binary_before" >"$OUT/fresh-binary.txt"

set +e
(
  cd "$ROOT" || exit 1
  go list ./...
) >"$OUT/package-list.txt" 2>"$OUT/go-list.stderr.log"
list_rc=$?
set -e
package_targets=$(wc -l <"$OUT/package-list.txt" | tr -d ' ')
printf 'command=go list ./...\nexit_code=%s\npackages_targeted=%s\n' \
  "$list_rc" "$package_targets" >"$OUT/go-list.result.txt"

mapfile -t go_files < <(git -C "$ROOT" ls-files '*.go')
set +e
gofmt -l "${go_files[@]}" >"$OUT/gofmt.log" 2>&1
gofmt_cmd_rc=$?
set -e
gofmt_drift=$(wc -l <"$OUT/gofmt.log" | tr -d ' ')
gofmt_rc=$gofmt_cmd_rc
[ "$gofmt_drift" -eq 0 ] || gofmt_rc=1
printf 'command=gofmt -l <all tracked Go files>\nexit_code=%s\ndrift_files=%s\n' \
  "$gofmt_cmd_rc" "$gofmt_drift" >"$OUT/gofmt.result.txt"

set +e
(
  cd "$ROOT" || exit 1
  CGO_ENABLED=0 go build ./...
) >"$OUT/go-build.log" 2>&1
build_rc=$?
set -e
printf 'command=CGO_ENABLED=0 go build ./...\nexit_code=%s\npackages_targeted=%s\n' \
  "$build_rc" "$package_targets" >"$OUT/go-build.result.txt"

# Keep the evidence shape complete even when the fresh build fails before it
# can produce a binary or version string.
: >"$OUT/version.txt"
: >"$OUT/binary-sha256.txt"
set +e
(
  cd "$ROOT" || exit 1
  make fresh
) >"$OUT/make-fresh.log" 2>&1
fresh_rc=$?
set -e
version_rc=1
root_binary_after=true
if [ "$fresh_rc" -eq 0 ] && [ -x "$ROOT/bin/consensus" ]; then
  set +e
  "$ROOT/bin/consensus" --version >"$OUT/version.txt" 2>&1
  version_rc=$?
  set -e
  sha256sum "$ROOT/bin/consensus" >"$OUT/binary-sha256.txt"
fi
[ -e "$ROOT/consensus" ] || root_binary_after=false
[ "$root_binary_after" = false ] || fresh_rc=1
[ "$version_rc" -eq 0 ] || fresh_rc=1
printf 'root_binary_after=%s\nmake_fresh_exit_code=%s\nversion_exit_code=%s\n' \
  "$root_binary_after" "$fresh_rc" "$version_rc" >>"$OUT/fresh-binary.txt"

set +e
(
  cd "$ROOT" || exit 1
  go vet ./...
) >"$OUT/go-vet.log" 2>&1
vet_rc=$?
set -e
printf 'command=go vet ./...\nexit_code=%s\npackages_targeted=%s\n' \
  "$vet_rc" "$package_targets" >"$OUT/go-vet.result.txt"

mkdir -p "$TMP/go-tmp"
set +e
(
  cd "$ROOT" || exit 1
  env -i \
    HOME="$HOME" \
    PATH="$PATH" \
    TMPDIR="$TMP/go-tmp" \
    CGO_ENABLED=0 \
    timeout "$TEST_TIMEOUT_SECONDS" go test -json -count=1 ./...
) >"$OUT/go-test.jsonl" 2>&1
test_rc=$?
set -e

json_stream_rc=0
jq -s '.' "$OUT/go-test.jsonl" >/dev/null 2>&1 || json_stream_rc=1
if [ "$json_stream_rc" -eq 0 ]; then
  package_passes=$(jq -s '[.[] | select(.Action=="pass" and ((.Test? // "")=="")) | .Package] | unique | length' "$OUT/go-test.jsonl")
  package_failures=$(jq -s '[.[] | select(.Action=="fail" and ((.Test? // "")=="")) | .Package] | unique | length' "$OUT/go-test.jsonl")
  no_test_packages=$(jq -s '[.[] | select(.Action=="output" and ((.Output? // "")|contains("[no test files]"))) | .Package] | unique | length' "$OUT/go-test.jsonl")
  test_events_passed=$(jq -s '[.[] | select(.Action=="pass" and ((.Test? // "")!=""))] | length' "$OUT/go-test.jsonl")
  test_events_failed=$(jq -s '[.[] | select(.Action=="fail" and ((.Test? // "")!=""))] | length' "$OUT/go-test.jsonl")
  test_events_skipped=$(jq -s '[.[] | select(.Action=="skip" and ((.Test? // "")!=""))] | length' "$OUT/go-test.jsonl")
  top_level_tests_passed=$(jq -s '[.[] | select(.Action=="pass" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$OUT/go-test.jsonl")
  top_level_tests_failed=$(jq -s '[.[] | select(.Action=="fail" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$OUT/go-test.jsonl")
  top_level_tests_skipped=$(jq -s '[.[] | select(.Action=="skip" and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$OUT/go-test.jsonl")
  top_level_tests_observed=$((top_level_tests_passed + top_level_tests_failed + top_level_tests_skipped))
else
  package_passes=0
  package_failures=0
  no_test_packages=0
  test_events_passed=0
  test_events_failed=0
  test_events_skipped=0
  top_level_tests_passed=0
  top_level_tests_failed=0
  top_level_tests_skipped=0
  top_level_tests_observed=0
  test_rc=1
fi
# The dollars below document the command literally; they are not expansions.
# shellcheck disable=SC2016
printf 'command=env -i HOME=$HOME PATH=$PATH TMPDIR=<scratch> CGO_ENABLED=0 timeout %s go test -json -count=1 ./...\nexit_code=%s\njson_stream_parse_exit_code=%s\npackages_with_tests_passed=%s\npackages_no_test_files=%s\npackages_failed=%s\ntest_events_passed=%s\ntest_events_failed=%s\ntest_events_skipped=%s\ntop_level_tests_passed=%s\ntop_level_tests_failed=%s\ntop_level_tests_skipped=%s\ntop_level_tests_observed=%s\n' \
  "$TEST_TIMEOUT_SECONDS" "$test_rc" "$json_stream_rc" "$package_passes" "$no_test_packages" "$package_failures" \
  "$test_events_passed" "$test_events_failed" "$test_events_skipped" \
  "$top_level_tests_passed" "$top_level_tests_failed" "$top_level_tests_skipped" "$top_level_tests_observed" \
  >"$OUT/go-test.result.txt"

set +e
(
  cd "$ROOT" || exit 1
  timeout "$SMOKE_TIMEOUT_SECONDS" go test -json -count=1 -run Smoke ./demo/
) >"$OUT/smoke.jsonl" 2>&1
smoke_rc=$?
set -e
smoke_tests=0
smoke_mock_calls=0
if jq -s '.' "$OUT/smoke.jsonl" >/dev/null 2>&1; then
  smoke_tests=$(jq -s '[.[] | select((.Action=="pass" or .Action=="fail") and ((.Test? // "")!="") and ((.Test|contains("/"))|not))] | length' "$OUT/smoke.jsonl")
  smoke_mock_calls=$(jq -rs '[.[] | (.Output? // "") | try (capture("after (?<n>[0-9]+) mocked LLM call").n|tonumber) catch empty] | max // 0' "$OUT/smoke.jsonl")
fi
[ "$smoke_mock_calls" -gt 0 ] || smoke_rc=1
printf 'command=timeout %s go test -json -count=1 -run Smoke ./demo/\nexit_code=%s\ntop_level_tests_observed=%s\nmocked_llm_calls=%s\n' \
  "$SMOKE_TIMEOUT_SECONDS" "$smoke_rc" "$smoke_tests" "$smoke_mock_calls" >"$OUT/smoke.result.txt"

# Execute the manifest's exact C-235 selector. At this HEAD it discovers zero
# matching tests; rc=0 must therefore be recorded as VOID, not as coverage.
set +e
(
  cd "$ROOT" || exit 1
  go test -json -count=1 -run TestToolRegistry ./internal/harness/ ./internal/api/
) >"$OUT/tool-registry-selector.jsonl" 2>&1
tool_selector_rc=$?
set -e
tool_selector_tests=0
if jq -s '.' "$OUT/tool-registry-selector.jsonl" >/dev/null 2>&1; then
  tool_selector_tests=$(jq -s '[.[] | select((.Action=="pass" or .Action=="fail" or .Action=="skip") and ((.Test? // "")!=""))] | length' "$OUT/tool-registry-selector.jsonl")
fi
tool_selector_verdict=VOID
if [ "$tool_selector_rc" -ne 0 ]; then tool_selector_verdict=FAIL; fi
if [ "$tool_selector_tests" -gt 0 ] && [ "$tool_selector_rc" -eq 0 ]; then tool_selector_verdict=PASS; fi
printf 'command=go test -json -count=1 -run TestToolRegistry ./internal/harness/ ./internal/api/\nexit_code=%s\ntest_events_observed=%s\nverdict=%s\n' \
  "$tool_selector_rc" "$tool_selector_tests" "$tool_selector_verdict" >"$OUT/tool-registry-selector.result.txt"

set +e
git -C "$ROOT" merge-base --is-ancestor ad8afd8 HEAD
pool_ancestor_rc=$?
set -e
printf 'command=git merge-base --is-ancestor ad8afd8 HEAD\nexit_code=%s\n' \
  "$pool_ancestor_rc" >"$OUT/pool-fix.txt"

fresh_db_rc=0
[ ! -e "$ROOT/dev.db" ] || fresh_db_rc=1
printf 'repo_dev_db_present=%s\nscratch_db_present=false\n' \
  "$([ -e "$ROOT/dev.db" ] && printf true || printf false)" >"$OUT/fresh-db.txt"

core_rc=0
for rc in "$list_rc" "$gofmt_rc" "$build_rc" "$fresh_rc" "$vet_rc" "$test_rc" "$smoke_rc" "$pool_ancestor_rc" "$fresh_db_rc"; do
  [ "$rc" -eq 0 ] || core_rc=1
done
core_verdict=$(status_for_rc "$core_rc")
overall_rc=$core_rc
[ "$provenance_rc" -eq 0 ] || overall_rc=1
overall_verdict=$(status_for_rc "$overall_rc")
packages_ok=$((package_passes + no_test_packages))
observed_test_events=$((test_events_passed + test_events_failed + test_events_skipped))

jq -n \
  --arg schema "consensus-sg8-t0-v1" \
  --arg tier "T0" \
  --arg head "$head_sha" \
  --arg origin_master "$origin_sha" \
  --arg target_sha "$TARGET_SHA" \
  --arg verdict "$overall_verdict" \
  --arg core_verdict "$core_verdict" \
  --argjson packages_targeted "$package_targets" \
  --argjson packages_ok "$packages_ok" \
  --argjson packages_with_tests_passed "$package_passes" \
  --argjson packages_no_test_files "$no_test_packages" \
  --argjson packages_failed "$package_failures" \
  --argjson top_level_tests_observed "$top_level_tests_observed" \
  --argjson top_level_tests_passed "$top_level_tests_passed" \
  --argjson top_level_tests_failed "$top_level_tests_failed" \
  --argjson top_level_tests_skipped "$top_level_tests_skipped" \
  --argjson test_events_observed "$observed_test_events" \
  --argjson test_events_passed "$test_events_passed" \
  --argjson test_events_failed "$test_events_failed" \
  --argjson test_events_skipped "$test_events_skipped" \
  --argjson smoke_mock_calls "$smoke_mock_calls" \
  --argjson dirty_paths "$(jq -R -s 'split("\n") | map(select(length>0))' "$OUT/dirty-before.txt")" \
  --arg provenance "$(status_for_rc "$provenance_rc")" \
  --arg gofmt "$(status_for_rc "$gofmt_rc")" \
  --arg build "$(status_for_rc "$build_rc")" \
  --arg fresh_binary "$(status_for_rc "$fresh_rc")" \
  --arg vet "$(status_for_rc "$vet_rc")" \
  --arg tests "$(status_for_rc "$test_rc")" \
  --arg smoke "$(status_for_rc "$smoke_rc")" \
  --arg pool_ancestor "$(status_for_rc "$pool_ancestor_rc")" \
  --arg fresh_db "$(status_for_rc "$fresh_db_rc")" \
  --arg tool_selector_verdict "$tool_selector_verdict" \
  --argjson tool_selector_tests "$tool_selector_tests" \
  '{schema:$schema,tier:$tier,head:$head,origin_master:$origin_master,target_sha:$target_sha,
    verdict:$verdict,core_build_unit_verdict:$core_verdict,
    counters:{packages_targeted:$packages_targeted,packages_ok:$packages_ok,
      packages_with_tests_passed:$packages_with_tests_passed,packages_no_test_files:$packages_no_test_files,
      packages_failed:$packages_failed,top_level_tests_observed:$top_level_tests_observed,
      top_level_tests_passed:$top_level_tests_passed,top_level_tests_failed:$top_level_tests_failed,
      top_level_tests_skipped:$top_level_tests_skipped,test_events_observed:$test_events_observed,
      test_events_passed:$test_events_passed,test_events_failed:$test_events_failed,
      test_events_skipped:$test_events_skipped,smoke_mock_calls:$smoke_mock_calls},
    cells:[
      {id:"provenance-clean-tree",verdict:$provenance,dirty_paths:$dirty_paths},
      {id:"gofmt",verdict:$gofmt},{id:"build-all",verdict:$build},
      {id:"fresh-binary",verdict:$fresh_binary},{id:"vet-all",verdict:$vet},
      {id:"env-clean-full-suite",verdict:$tests},{id:"keyless-smoke",verdict:$smoke},
      {id:"pool-fix-ancestor",verdict:$pool_ancestor},{id:"fresh-db",verdict:$fresh_db},
      {id:"tool-registry-selector",verdict:$tool_selector_verdict,test_events_observed:$tool_selector_tests}
    ],
    unexecuted:[
      {manifest_ids:["C-063","C-128","C-190","C-233"],reason:"duplicate short-suite variants were not rerun after the stronger env-clean full suite; no verdict assigned"},
      {manifest_ids:["C-234"],reason:"scripts/probe-tvf.go does not exist at this HEAD; manifest command unsupported; no substitute fabricated"},
      {manifest_ids:["C-236"],reason:"gitreins guard is staged-diff dependent and is run separately before commit; this sweep records gofmt only"}
    ]}' >"$OUT/t0.json"

jq -n --arg verdict "$overall_verdict" --argjson observables "$packages_ok" \
  '[{tier:"T0",id:"sg8-build-unit",verdict:$verdict,observables:$observables}]' \
  >"$OUT/t0-results.json"
printf '# tier\tcounter_name\twhat a nonzero value proves\nT0\tpackages_ok\tgo build/vet + unit packages that compiled and passed\n' \
  >"$OUT/t0-counter.tsv"
set +e
bash "$ROOT/scripts/battery/guard_void.sh" "$OUT/t0-results.json" "$OUT/t0-counter.tsv" \
  >"$OUT/guard-void.log" 2>&1
guard_void_rc=$?
set -e
printf 'guard_void_exit_code=%s\n' "$guard_void_rc" >"$OUT/guard-void.result.txt"

(
  cd "$OUT" || exit 1
  sha256sum -- \
    binary-sha256.txt dirty-before.txt go-build.log go-test.jsonl go-vet.log \
    smoke.jsonl t0.json tool-registry-selector.jsonl version.txt \
    >MANIFEST.txt
)

printf 'T0 %s: packages_ok=%s/%s top_level_tests_observed=%s test_events=%s smoke_mock_calls=%s provenance=%s\n' \
  "$overall_verdict" "$packages_ok" "$package_targets" "$top_level_tests_observed" \
  "$observed_test_events" "$smoke_mock_calls" "$(status_for_rc "$provenance_rc")"
exit "$overall_rc"
