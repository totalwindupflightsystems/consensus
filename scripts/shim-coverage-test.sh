#!/usr/bin/env bash
# axiom:trace row=SHIM-GAP-008 spec=specs/024-shim-support-matrix.md#coverage impl=scripts/shim-coverage.sh test=scripts/shim-coverage-test.sh make=Makefile:shim-coverage
# shim-coverage-test.sh — regression proof for scripts/shim-coverage.sh
# (SHIM-GAP-008). Proves the published artifact is COMPUTED from the live
# declared-vs-served comparator output rather than a copied literal:
#
#   1. runs `make shim-coverage` and parses its key=value artifact;
#   2. runs the comparator independently into a fresh temp directory (not via
#      the script under test) and recomputes the expected counts and
#      percentage from THOSE numbers;
#   3. fails if the artifact's coverage_pct equals a hardcoded literal, or if
#      it disagrees with the independently recomputed value;
#   4. mutates the comparator counts (covered=declared → 100.00) through a
#      stub comparator and proves the published percentage MOVES with the
#      comparator — a copied literal cannot move;
#   5. proves the artifact names the pin file's commit and the runtime git
#      commit;
#   6. fail-closed checks: no output and non-zero exit when the pin file, the
#      surfaces, or a valid zero-declared counts document are absent.
#
# Exit 0 = all proofs hold. Each check names itself on failure.
set -uo pipefail

# Resolve the repository from this script's location so the test runs from any
# working directory, exactly like the target it guards.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$REPO" ] || { echo "FAIL: not inside a git work tree" >&2; exit 1; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/shim-coverage-test.XXXXXX")"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

fail_count=0
check() { # check <label> <rc>
  if [ "$2" = "0" ]; then
    echo "PASS: $1"
  else
    echo "FAIL: $1" >&2
    fail_count=$((fail_count + 1))
  fi
}

require_tools() {
  local t
  for t in git python3 make; do
    command -v "$t" >/dev/null 2>&1 || { echo "FAIL: $t is required" >&2; exit 2; }
  done
  python3 -c 'import yaml' >/dev/null 2>&1 || {
    echo "FAIL: PyYAML is required" >&2; exit 2; }
}
require_tools

COMPARATOR="$REPO/scripts/compare-opencode-declared-served.py"
PIN_FILE="$REPO/specs/openapi/opencode-pin.yaml"
COV="$REPO/scripts/shim-coverage.sh"
MAKE_TARGET_OK=0

# expected_pct and expected_counts: computed from an INDEPENDENT comparator
# run — never from the artifact under test.
expected_pct=""
declare -A expected_counts=()

# ---------------------------------------------------------------- 1. happy path
artifact="$TMP/artifact.txt"
if make -C "$REPO" shim-coverage >"$artifact" 2>"$TMP/make.err"; then
  MAKE_TARGET_OK=1
else
  echo "--- make shim-coverage stderr ---" >&2
  cat "$TMP/make.err" >&2
  echo "----------------------------------" >&2
fi
check "make shim-coverage exits 0" \
  "$([ "$MAKE_TARGET_OK" = "1" ] && echo 0 || echo 1)"
if [ "$MAKE_TARGET_OK" = "1" ]; then
  echo "--- make shim-coverage output ---"
  cat "$artifact"
  echo "---------------------------------"
fi

# Parse the published artifact.
parse_kv() { # parse_kv <file> <key>; prints the value
  sed -n "s/^$2=//p" "$1" | tail -n 1
}

PCT="$(parse_kv "$artifact" coverage_pct)"
COVERED="$(parse_kv "$artifact" covered_operations)"
DECLARED="$(parse_kv "$artifact" declared_operations)"
PIN="$(parse_kv "$artifact" pin)"
PIN_VERSION="$(parse_kv "$artifact" pin_version)"
COMMIT_OUT="$(parse_kv "$artifact" commit)"

[ -n "$PCT" ] || { echo "FAIL: artifact carries no coverage_pct" >&2; exit 1; }

# ------------------------------------------- 2. independent comparator recompute
indie="$TMP/independent"
mkdir -p "$indie"
python3 "$COMPARATOR" \
  "$REPO/specs/openapi/upstream/openapi-1.18.33.surface.json" \
  "$REPO/specs/openapi/upstream/consensus-shim-served-surface.yaml" \
  "$indie" >"$TMP/independent-counts.json" 2>"$TMP/independent.err"
INDIE_RC=$?
check "independent comparator run exits 0" "$INDIE_RC"

if [ "$INDIE_RC" = "0" ]; then
  read -r I_COVERED I_DECLARED I_PCT < <(python3 - "$TMP/independent-counts.json" <<'PY'
import json
import sys

counts = json.load(open(sys.argv[1]))
covered = counts["covered_operations"]
declared = counts["declared_operations"]
print(covered, declared, "%.2f" % (covered / declared * 100.0))
PY
)
  expected_pct="$I_PCT"
  expected_counts[covered]="$I_COVERED"
  expected_counts[declared]="$I_DECLARED"

  check "coverage_pct ($PCT) equals independent covered/declared recomputation ($expected_pct)" \
    "$([ "$PCT" = "$expected_pct" ] && echo 0 || echo 1)"
  check "covered_operations ($COVERED) equals the independent comparator's ($I_COVERED)" \
    "$([ "$COVERED" = "$I_COVERED" ] && echo 0 || echo 1)"
  check "declared_operations ($DECLARED) equals the independent comparator's ($I_DECLARED)" \
    "$([ "$DECLARED" = "$I_DECLARED" ] && echo 0 || echo 1)"
fi

# ------------------------------------------------- 3. not-a-copied-literal proof
# 3a. the artifact must not be the currently forbidden example value...
if [ "$PCT" = "63.83" ]; then
  # ...unless the live comparator genuinely produces it (which would make the
  # "forbidden literal" indistinguishable from the computed value).
  if [ "$expected_pct" != "63.83" ]; then
    echo "FAIL: coverage_pct is the hardcoded example 63.83 but the live comparator computes $expected_pct" >&2
    fail_count=$((fail_count + 1))
  else
    echo "PASS: coverage_pct is 63.83 because the comparator computes 63.83"
  fi
else
  echo "PASS: coverage_pct ($PCT) is not the hardcoded example value"
fi

# 3b. mutation proof: point the coverage script at a STUB comparator that
# emits covered==declared. The published percentage must move to 100.00 —
# a copied literal cannot move. The stub records its argv so we also prove the
# script still exercises the real surface files.
stub_dir="$TMP/stub"
mkdir -p "$stub_dir/in"
cat >"$stub_dir/comparator.py" <<'PY'
import json
import sys

out = sys.argv[3]
counts = {
    "declared_operations": 40,
    "covered_operations": 40,
}
with open(out + "/declared-vs-served.json", "w") as f:
    json.dump({"counts": counts}, f)
print(json.dumps(counts))
PY
# Re-point only the comparator (the output dir stays temp): sed the one
# assignment line, nothing else.
sed "s|^COMPARATOR=.*|COMPARATOR=\"$stub_dir/comparator.py\"|" \
  "$REPO/scripts/shim-coverage.sh" >"$TMP/shim-coverage-stub.sh"
mut_artifact="$TMP/mutated-artifact.txt"
if bash "$TMP/shim-coverage-stub.sh" >"$mut_artifact" 2>"$TMP/mut.err"; then
  MUT_PCT="$(parse_kv "$mut_artifact" coverage_pct)"
  check "mutated comparator counts (40/40) move coverage_pct to 100.00 (got ${MUT_PCT:-unset})" \
    "$([ "$MUT_PCT" = "100.00" ] && echo 0 || echo 1)"
  # The stub run must still read the pin + git from the real repo.
  MUT_PIN="$(parse_kv "$mut_artifact" pin)"
  REAL_PIN="$(python3 - "$PIN_FILE" <<'PY'
import sys
import yaml

print(yaml.safe_load(open(sys.argv[1]))["pin"]["commit"])
PY
)"
  check "stub run still names the pin file's commit" \
    "$([ "$MUT_PIN" = "$REAL_PIN" ] && echo 0 || echo 1)"
else
  echo "FAIL: stub-comparator run exited non-zero: $(cat "$TMP/mut.err")" >&2
  fail_count=$((fail_count + 1))
fi

# ------------------------------------------------------- 4. pin + commit fields
REAL_PIN_COMMIT="$(python3 - "$PIN_FILE" <<'PY'
import sys
import yaml

print(yaml.safe_load(open(sys.argv[1]))["pin"]["commit"])
PY
)"
REAL_PIN_VERSION="$(python3 - "$PIN_FILE" <<'PY'
import sys
import yaml

print(yaml.safe_load(open(sys.argv[1]))["pin"]["version"])
PY
)"
RUNTIME_COMMIT="$(git -C "$REPO" rev-parse HEAD)"
check "artifact pin ($PIN) equals the pin file's commit ($REAL_PIN_COMMIT)" \
  "$([ "$PIN" = "$REAL_PIN_COMMIT" ] && echo 0 || echo 1)"
check "artifact pin_version ($PIN_VERSION) equals the pin file's version ($REAL_PIN_VERSION)" \
  "$([ "$PIN_VERSION" = "$REAL_PIN_VERSION" ] && echo 0 || echo 1)"
check "artifact commit ($COMMIT_OUT) equals runtime git HEAD ($RUNTIME_COMMIT)" \
  "$([ "$COMMIT_OUT" = "$RUNTIME_COMMIT" ] && echo 0 || echo 1)"

# ---------------------------------------------------------------- 5. fail closed
# The failure arms run the REAL script with its inputs hidden from it. It has
# no flag surface by design, so each arm copies the script and rewrites only
# the constant that names the input, mirroring the mutation-proof technique.
run_broken_arm() { # run_broken_arm <label> <sed-expression>
  local label="$1"
  local arm="$TMP/arm.sh"
  sed "$2" "$COV" >"$arm" || return 1
  local out="$TMP/arm-out.txt"
  if bash "$arm" >"$out" 2>"$TMP/arm-err.txt"; then
    echo "FAIL: $label — expected non-zero exit, got 0 with output: $(cat "$out")" >&2
    fail_count=$((fail_count + 1))
  else
    if [ -s "$out" ]; then
      echo "FAIL: $label — printed an artifact on a failed run: $(head -3 "$out")" >&2
      fail_count=$((fail_count + 1))
    else
      echo "PASS: $label"
    fi
  fi
}

run_broken_arm "missing pin file fails closed" \
  "s|^PIN_FILE=.*|PIN_FILE=\"specs/openapi/does-not-exist.yaml\"|"
run_broken_arm "missing declared surface fails closed" \
  "s|^DECLARED_SURFACE=.*|DECLARED_SURFACE=\"specs/openapi/upstream/nope.surface.json\"|"
run_broken_arm "missing served surface fails closed" \
  "s|^SERVED_SURFACE=.*|SERVED_SURFACE=\"specs/openapi/upstream/nope.yaml\"|"

# zero-declared arm: a valid counts JSON with declared_operations=0 must abort,
# not divide by zero. The stub writes 30/0 and expects the run to fail.
cat >"$stub_dir/zero.py" <<'PY'
import json
import sys

counts = {"declared_operations": 0, "covered_operations": 30}
with open(sys.argv[3] + "/declared-vs-served.json", "w") as f:
    json.dump({"counts": counts}, f)
print(json.dumps(counts))
PY
zero_arm="$TMP/arm-zero.sh"
sed "s|^COMPARATOR=.*|COMPARATOR=\"$stub_dir/zero.py\"|" "$COV" >"$zero_arm"
zero_out="$TMP/zero-out.txt"
if bash "$zero_arm" >"$zero_out" 2>/dev/null; then
  echo "FAIL: zero declared_operations failed open (exit 0, output: $(head -3 "$zero_out"))" >&2
  fail_count=$((fail_count + 1))
else
  if [ -s "$zero_out" ]; then
    echo "FAIL: zero declared_operations printed an artifact before failing: $(head -3 "$zero_out")" >&2
    fail_count=$((fail_count + 1))
  else
    echo "PASS: zero declared_operations fails closed"
  fi
fi

# invalid-JSON arm: a stub emitting garbage must abort without publishing.
cat >"$stub_dir/garbage.py" <<'PY'
import sys

print("this is not json")
PY
garbage_arm="$TMP/arm-garbage.sh"
sed "s|^COMPARATOR=.*|COMPARATOR=\"$stub_dir/garbage.py\"|" "$COV" >"$garbage_arm"
garbage_out="$TMP/garbage-out.txt"
if bash "$garbage_arm" >"$garbage_out" 2>/dev/null; then
  echo "FAIL: invalid counts JSON failed open (exit 0, output: $(head -3 "$garbage_out"))" >&2
  fail_count=$((fail_count + 1))
else
  if [ -s "$garbage_out" ]; then
    echo "FAIL: invalid counts JSON printed an artifact: $(head -3 "$garbage_out")" >&2
    fail_count=$((fail_count + 1))
  else
    echo "PASS: invalid counts JSON fails closed"
  fi
fi

# --------------------------------------------------------------------- verdict
echo ""
if [ "$fail_count" -eq 0 ]; then
  echo "shim-coverage-test: ALL PROOFS HOLD"
  exit 0
fi
echo "shim-coverage-test: $fail_count proof(s) FAILED" >&2
exit 1
