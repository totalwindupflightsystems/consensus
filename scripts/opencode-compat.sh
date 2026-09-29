#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/consensus-opencode-compat.XXXXXX")
RESULTS="$TMP_DIR/results.tsv"
WITH_LIVE_LLM=0

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT HUP INT TERM

usage() {
  cat <<'EOF'
Usage: scripts/opencode-compat.sh [--with-live-llm]

Re-runs the 46-check Go opencode compatibility inventory cited in
`docs/testing/consensus-full-test-plan-v1.md` and prints pass/fail counts.

The default keyless grade runs 44 checks with an offline placeholder endpoint:
39 graded checks plus the five tracked C19/C20 exclusions. C09 and the real-LLM
lifecycle check are reported as not run because both require DEEPSEEK_API_KEY.

Options:
  --with-live-llm  also run C09 and TestShimRealLLMSessionLifecycle (costs API
                   credits; requires DEEPSEEK_API_KEY)
  -h, --help       show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --with-live-llm) WITH_LIVE_LLM=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

command -v go >/dev/null 2>&1 || {
  echo "ERROR: go is required" >&2
  exit 2
}

if [ "$WITH_LIVE_LLM" -eq 1 ] && [ -z "${DEEPSEEK_API_KEY:-}" ]; then
  echo "ERROR: --with-live-llm requires DEEPSEEK_API_KEY" >&2
  exit 2
fi

cd "$ROOT"
: >"$RESULTS"
required_run_failed=0

record_results() {
  log=$1
  awk '
    /"Action":"(pass|fail|skip)"/ && /"Test":"/ {
      action = $0
      sub(/^.*"Action":"/, "", action)
      sub(/".*$/, "", action)

      test = $0
      sub(/^.*"Test":"/, "", test)
      sub(/".*$/, "", test)

      if (index(test, "/") > 0 ||
          test == "TestShimEndpoints" ||
          test == "TestShimHealthEndpoint" ||
          test == "TestShimRealLLMSessionLifecycle") {
        print action "\t" test
      }
    }
  ' "$log" >>"$RESULTS"
}

run_keyless() {
  label=$1
  selector=$2
  required=$3
  log="$TMP_DIR/$label.json"

  printf 'RUN %-18s %s\n' "$label" "$selector"
  if env \
    DEEPSEEK_API_KEY=offline \
    CONSENSUS_LLM_BASE_URL=http://127.0.0.1:1 \
    go test -count=1 -json ./internal/chronicle -run "$selector" >"$log"; then
    rc=0
  else
    rc=$?
  fi
  record_results "$log"

  if [ "$required" -eq 1 ] && [ "$rc" -ne 0 ]; then
    echo "ERROR: $label exited $rc" >&2
    required_run_failed=1
  fi
}

# The keyless inventory is split so the tracked C19/C20 exclusions cannot hide
# a failure in a graded test, and so C09 never waits on an unavailable LLM.
run_keyless contract \
  '^(TestOpenCodeContract_|TestFullContract_(HealthDoc|SessionLifecycle|Config|FileOperations|PermissionsQuestions|ErrorShapes|SSEEvents|SDKCompatibility))' \
  1
run_keyless messages-keyless '^TestFullContract_Messages$/^C1[0-2]:' 1
run_keyless known-exclusions '^TestFullContract_InstanceVCS$' 0
run_keyless smoke '^(TestShimEndpoints|TestShimHealthEndpoint)$' 1

if [ "$WITH_LIVE_LLM" -eq 1 ]; then
  for live_case in \
    '^TestFullContract_Messages$/^C09:' \
    '^TestShimRealLLMSessionLifecycle$'
  do
    log="$TMP_DIR/live-$WITH_LIVE_LLM.json"
    printf 'RUN %-18s %s\n' live-llm "$live_case"
    if go test -count=1 -json ./internal/chronicle -run "$live_case" >"$log"; then
      rc=0
    else
      rc=$?
    fi
    record_results "$log"
    WITH_LIVE_LLM=$((WITH_LIVE_LLM + 1))
    if [ "$rc" -ne 0 ]; then
      echo "ERROR: live-llm exited $rc for $live_case" >&2
      required_run_failed=1
    fi
  done
fi

stats=$(awk -F '\t' '
  BEGIN { pass=0; fail=0; skip=0; excluded=0; ep=0; ef=0; es=0 }
  $2 ~ /\/C19:|\/C20:/ {
    excluded++
    if ($1 == "pass") ep++
    else if ($1 == "fail") ef++
    else if ($1 == "skip") es++
    next
  }
  $1 == "pass" { pass++ }
  $1 == "fail" { fail++ }
  $1 == "skip" { skip++ }
  END { print pass, fail, skip, excluded, ep, ef, es }
' "$RESULTS")
IFS=' ' read -r passed failed skipped excluded excluded_passed excluded_failed excluded_skipped <<EOF
$stats
EOF

if [ "$WITH_LIVE_LLM" -eq 0 ]; then
  live_key_not_run=2
  expected_observed=44
  expected_graded=39
else
  live_key_not_run=0
  expected_observed=46
  expected_graded=41
fi
observed=$((passed + failed + skipped + excluded))

printf '\nKNOWN EXCLUSIONS C19/C20: %s (pass=%s fail=%s skip=%s)\n' \
  "$excluded" "$excluded_passed" "$excluded_failed" "$excluded_skipped"
printf 'SUMMARY pass=%s fail=%s skip=%s excluded=%s live_key_not_run=%s inventory=46\n' \
  "$passed" "$failed" "$skipped" "$excluded" "$live_key_not_run"
printf 'GRADE %s/%s graded checks passed\n' "$passed" "$expected_graded"

if [ "$observed" -ne "$expected_observed" ]; then
  echo "ERROR: observed $observed checks; expected $expected_observed" >&2
  exit 1
fi
if [ "$excluded" -ne 5 ]; then
  echo "ERROR: observed $excluded C19/C20 exclusions; expected 5" >&2
  exit 1
fi
if [ "$failed" -ne 0 ] || [ "$skipped" -ne 0 ] || [ "$passed" -ne "$expected_graded" ]; then
  awk -F '\t' '$1 == "fail" || $1 == "skip" { print toupper($1) " " $2 }' "$RESULTS" >&2
  exit 1
fi
if [ "$required_run_failed" -ne 0 ]; then
  exit 1
fi
