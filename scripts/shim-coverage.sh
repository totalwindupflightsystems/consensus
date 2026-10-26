#!/usr/bin/env bash
# axiom:trace row=SHIM-GAP-008 spec=specs/024-shim-support-matrix.md#coverage impl=scripts/shim-coverage.sh test=scripts/shim-coverage-test.sh make=Makefile:shim-coverage
# shim-coverage.sh — publish the shim coverage percentage as a rerunnable
# artifact (SHIM-GAP-008).
#
# specs/024 §A.3 already owns the mechanical declared-vs-served comparison
# (scripts/compare-opencode-declared-served.py, SHIM-SOURCE-003). This script
# adds NO second comparison: it runs the existing comparator into a temporary
# directory — never overwriting the checked-in artifact
# specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json — reads the
# protocol pin from specs/openapi/opencode-pin.yaml, records the commit it ran
# at, and prints machine-readable key=value lines on stdout.
#
# Fail closed: a missing tool or input file, a comparator failure, invalid
# counts JSON, a missing pin field, or zero declared operations abort with a
# non-zero exit and print no artifact — nothing is published from a comparison
# that did not run.
#
# Output (stdout, one key per line):
#   declared_operations=<n>   operations the pinned document declares
#   covered_operations=<n>    operations the shim serves as declared
#   coverage_pct=<n.nn>       covered/declared as a percentage, two decimals
#   pin=<sha>                 upstream protocol pin (opencode-pin.yaml)
#   pin_version=<v>           pinned upstream version
#   commit=<sha>              the consensus commit this artifact was computed at
#
# Usage: bash scripts/shim-coverage.sh   (usually via `make shim-coverage`)
# Regression proof: bash scripts/shim-coverage-test.sh
set -euo pipefail

COMPARATOR="scripts/compare-opencode-declared-served.py"
DECLARED_SURFACE="specs/openapi/upstream/openapi-1.18.33.surface.json"
SERVED_SURFACE="specs/openapi/upstream/consensus-shim-served-surface.yaml"
PIN_FILE="specs/openapi/opencode-pin.yaml"

fail() { echo "shim-coverage: $*" >&2; exit 1; }

command -v git >/dev/null 2>&1 || fail "git is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
python3 -c 'import yaml' >/dev/null 2>&1 || \
  fail "PyYAML is required (python3 -m pip install pyyaml)"

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || \
  fail "not inside a git work tree; run from the consensus repository"
cd "$REPO"

for f in "$COMPARATOR" "$DECLARED_SURFACE" "$SERVED_SURFACE" "$PIN_FILE"; do
  [ -f "$f" ] || fail "required input missing: $REPO/$f"
done

# The protocol pin is read from the existing pin file, never duplicated here
# (specs/024 §A.1: the pin is recorded in-repo, never implied).
pin_values="$(python3 - "$PIN_FILE" <<'PY'
import sys
import yaml

try:
    doc = yaml.safe_load(open(sys.argv[1]))
except (OSError, yaml.YAMLError) as e:
    sys.exit("cannot read protocol pin file: %s" % e)
pin = (doc or {}).get("pin") or {}
commit = str(pin.get("commit") or "").strip()
version = str(pin.get("version") or "").strip()
if not commit:
    sys.exit("pin.commit is missing or empty in %s" % sys.argv[1])
if not version:
    sys.exit("pin.version is missing or empty in %s" % sys.argv[1])
print(commit)
print(version)
PY
)" || fail "protocol pin unusable (see above)"
PIN_COMMIT="$(printf '%s\n' "$pin_values" | sed -n '1p')"
PIN_VERSION="$(printf '%s\n' "$pin_values" | sed -n '2p')"

COMMIT="$(git rev-parse HEAD 2>/dev/null)" || fail "git rev-parse HEAD failed"
[ -n "$COMMIT" ] || fail "git rev-parse HEAD produced an empty commit"

# The comparator owns the comparison; its stdout IS the counts document this
# artifact publishes. Generated appendix files land in the temp dir and are
# removed on exit.
OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/consensus-shim-coverage.XXXXXX")"
cleanup() { rm -rf "$OUT_DIR"; }
trap cleanup EXIT

counts_file="$OUT_DIR/comparator-counts.json"
python3 "$COMPARATOR" "$DECLARED_SURFACE" "$SERVED_SURFACE" "$OUT_DIR" \
  > "$counts_file" || fail "declared-vs-served comparator failed (see above)"

counts_line="$(python3 - "$counts_file" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path) as f:
        counts = json.load(f)
except (OSError, ValueError) as e:
    sys.exit("comparator counts are not valid JSON: %s" % e)
if not isinstance(counts, dict):
    sys.exit("comparator counts are not a JSON object")
covered = counts.get("covered_operations")
declared = counts.get("declared_operations")
for name, v in (("covered_operations", covered), ("declared_operations", declared)):
    if not isinstance(v, int) or isinstance(v, bool):
        sys.exit("comparator counts lack an integer %s" % name)
if declared <= 0:
    sys.exit("declared_operations is %d; refusing to divide by zero" % declared)
print(covered, declared, "%.2f" % (covered / declared * 100.0))
PY
)" || fail "comparator counts unusable (see above)"
read -r COVERED DECLARED PCT <<<"$counts_line"
[ -n "$COVERED" ] && [ -n "$DECLARED" ] && [ -n "$PCT" ] || \
  fail "failed to parse covered/declared/percentage from the comparator counts"

printf 'declared_operations=%s\n' "$DECLARED"
printf 'covered_operations=%s\n' "$COVERED"
printf 'coverage_pct=%s\n' "$PCT"
printf 'pin=%s\n' "$PIN_COMMIT"
printf 'pin_version=%s\n' "$PIN_VERSION"
printf 'commit=%s\n' "$COMMIT"
