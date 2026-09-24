#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel)
ASSET_DIR="$ROOT/scripts/opencode-upstream"
MANIFEST="$ASSET_DIR/manifest.json"
PATCH="$ASSET_DIR/adapter.patch"
PRELOAD="$ASSET_DIR/preload.mjs"
REVISION=16747470f976aca3d362ad730bcd3fe82ecc2c9a
VERSION=1.18.29
REPOSITORY=https://github.com/anomalyco/opencode.git
DEFAULT_CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/consensus/opencode-$REVISION"
CACHE_DIR=$DEFAULT_CACHE
CHECKOUT=
BASE_URL=${CONSENSUS_OPENCODE_BASE_URL:-}
EVIDENCE_DIR="$ROOT/docs/evidence/opencode-upstream-v$VERSION"
SELF_TEST=0

usage() {
  cat <<'EOF'
Usage: scripts/test-opencode-upstream.sh --base-url URL [options]

Executes the four pinned upstream opencode TypeScript suites against a live
Consensus opencode shim. A failing upstream assertion remains a failing suite;
full sanitized logs and a summary are written as durable evidence.

Options:
  --base-url URL       live Consensus shim URL (or CONSENSUS_OPENCODE_BASE_URL)
  --checkout DIR       use an existing pristine checkout at the pinned revision
  --cache-dir DIR      clone/install cache (default: XDG cache)
  --evidence-dir DIR   output directory for summary and full suite logs
  --self-test          offline runner/adapter contract check
  -h, --help           show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-url) BASE_URL=${2:?missing URL}; shift 2 ;;
    --checkout) CHECKOUT=${2:?missing directory}; shift 2 ;;
    --cache-dir) CACHE_DIR=${2:?missing directory}; shift 2 ;;
    --evidence-dir) EVIDENCE_DIR=${2:?missing directory}; shift 2 ;;
    --self-test) SELF_TEST=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: required command not found: $1" >&2
    exit 2
  }
}

manifest_value() {
  node -e 'const m=require(process.argv[1]); console.log(m[process.argv[2]])' "$MANIFEST" "$1"
}

self_test() {
  require node
  require git
  require sha256sum
  node --test "$ASSET_DIR/preload.test.mjs"
  [ "$(manifest_value revision)" = "$REVISION" ] || {
    echo "ERROR: runner revision and manifest revision differ" >&2
    exit 2
  }
  [ "$(manifest_value version)" = "$VERSION" ] || {
    echo "ERROR: runner version and manifest version differ" >&2
    exit 2
  }
  [ "$(manifest_value repository)" = "$REPOSITORY" ] || {
    echo "ERROR: runner repository and manifest repository differ" >&2
    exit 2
  }
  node - "$PATCH" <<'EOF'
const fs = require("node:fs")
const patch = fs.readFileSync(process.argv[2], "utf8")
if (!patch.includes("CONSENSUS_ADAPTER")) throw new Error("adapter marker missing")
for (const line of patch.split("\n")) {
  if (!/^[+-]/.test(line) || /^(---|\+\+\+)/.test(line)) continue
  if (/(expect\(|describe\(|test\(|it\.live\(|it\.instance\()/.test(line)) {
    throw new Error(`adapter mutates an upstream assertion or test registration: ${line}`)
  }
}
EOF
  echo "self-test: ok"
}

if [ "$SELF_TEST" -eq 1 ]; then
  self_test
  exit 0
fi

require git
require bun
require node
require sha256sum
require timeout
self_test >/dev/null

[ -n "$BASE_URL" ] || {
  echo "ERROR: --base-url or CONSENSUS_OPENCODE_BASE_URL is required" >&2
  exit 2
}
case "$BASE_URL" in
  http://*|https://*) ;;
  *) echo "ERROR: base URL must begin with http:// or https://" >&2; exit 2 ;;
esac

if [ -z "$CHECKOUT" ]; then
  CHECKOUT=$CACHE_DIR
  if [ ! -d "$CHECKOUT/.git" ]; then
    mkdir -p "$(dirname -- "$CHECKOUT")"
    git clone --filter=blob:none --no-checkout "$REPOSITORY" "$CHECKOUT"
  fi
  git -C "$CHECKOUT" fetch --depth 1 origin "$REVISION"
  git -C "$CHECKOUT" checkout --detach "$REVISION"
fi

actual_revision=$(git -C "$CHECKOUT" rev-parse HEAD)
[ "$actual_revision" = "$REVISION" ] || {
  echo "ERROR: checkout revision $actual_revision does not match pinned $REVISION" >&2
  exit 2
}

verify_file() {
  expected=$1
  relative=$2
  actual=$(sha256sum "$CHECKOUT/$relative" | cut -d ' ' -f 1)
  [ "$actual" = "$expected" ] || {
    echo "ERROR: pinned source drift: $relative" >&2
    echo "       expected $expected" >&2
    echo "       actual   $actual" >&2
    exit 2
  }
}

lock_hash=$(manifest_value lock_sha256)
verify_file "$lock_hash" bun.lock
node - "$MANIFEST" "$CHECKOUT" <<'EOF'
const fs = require("node:fs")
const crypto = require("node:crypto")
const path = require("node:path")
const manifest = require(process.argv[2])
const checkout = process.argv[3]
for (const suite of manifest.suites) {
  const content = fs.readFileSync(path.join(checkout, suite.path))
  const actual = crypto.createHash("sha256").update(content).digest("hex")
  if (actual !== suite.sha256) {
    console.error(`ERROR: pinned source drift: ${suite.path}`)
    console.error(`       expected ${suite.sha256}`)
    console.error(`       actual   ${actual}`)
    process.exit(2)
  }
}
EOF

package_version=$(node -e 'console.log(require(process.argv[1]).version)' "$CHECKOUT/packages/opencode/package.json")
[ "$package_version" = "$VERSION" ] || {
  echo "ERROR: package version $package_version does not match pinned $VERSION" >&2
  exit 2
}

# Frozen install is the dependency pin. Re-running is intentional: Bun checks
# the committed lock and makes no mutable-latest resolution.
(
  cd "$CHECKOUT"
  bun install --frozen-lockfile
)

# The checkout is a dedicated cache or an explicitly supplied pristine clone.
# Back up only the two transport-adapted files and restore them on every exit.
INSTANCE_TEST="$CHECKOUT/packages/opencode/test/server/httpapi-instance.test.ts"
ERROR_TEST="$CHECKOUT/packages/opencode/test/server/sdk-error-shape.test.ts"
BACKUP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/consensus-opencode-adapter.XXXXXX")
cp "$INSTANCE_TEST" "$BACKUP_DIR/httpapi-instance.test.ts"
cp "$ERROR_TEST" "$BACKUP_DIR/sdk-error-shape.test.ts"
restore_checkout() {
  cp "$BACKUP_DIR/httpapi-instance.test.ts" "$INSTANCE_TEST"
  cp "$BACKUP_DIR/sdk-error-shape.test.ts" "$ERROR_TEST"
  rm -rf "$BACKUP_DIR"
}
trap restore_checkout EXIT HUP INT TERM

git -C "$CHECKOUT" apply --check "$PATCH"
git -C "$CHECKOUT" apply "$PATCH"

mkdir -p "$EVIDENCE_DIR"
SUMMARY="$EVIDENCE_DIR/summary.md"
RESULTS="$EVIDENCE_DIR/results.tsv"
: >"$RESULTS"

sanitize() {
  sed -E \
    -e 's/cs_(ak|sk)_[A-Za-z0-9_-]+/[REDACTED]/g' \
    -e 's/(authorization: (Basic|Bearer) )[A-Za-z0-9+\/_=.-]+/\1[REDACTED]/Ig'
}

suite_index=0
SUITE_LIST="$BACKUP_DIR/suites.txt"
node -e 'for (const s of require(process.argv[1]).suites) console.log(s.path)' "$MANIFEST" >"$SUITE_LIST"
while IFS= read -r suite; do
  suite_index=$((suite_index + 1))
  slug=$(printf '%s' "$suite" | tr '/.' '__')
  raw_log="$BACKUP_DIR/$slug.raw.log"
  log="$EVIDENCE_DIR/$slug.log"
  set +e
  (
    cd "$CHECKOUT"
    CONSENSUS_OPENCODE_BASE_URL="$BASE_URL" \
      timeout 180 bun test --timeout 15000 --preload "$PRELOAD" "$suite"
  ) >"$raw_log" 2>&1
  rc=$?
  set -e
  sanitize <"$raw_log" >"$log"
  tests=$(sed -nE 's/^Ran ([0-9]+) tests?.*/\1/p' "$raw_log" | awk 'END {print}')
  passed=$(sed -nE 's/^([0-9]+) pass.*/\1/p' "$raw_log" | awk 'END {print}')
  failed=$(sed -nE 's/^([0-9]+) fail.*/\1/p' "$raw_log" | awk 'END {print}')
  [ -n "$tests" ] || tests=unknown
  [ -n "$passed" ] || passed=0
  [ -n "$failed" ] || failed=0
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$suite_index" "$suite" "$rc" "$tests" "$passed" "$failed" >>"$RESULTS"
done <"$SUITE_LIST"

adapter_hash=$(sha256sum "$PATCH" | cut -d ' ' -f 1)
preload_hash=$(sha256sum "$PRELOAD" | cut -d ' ' -f 1)
{
  echo "# Pinned opencode upstream compatibility evidence"
  echo
  echo "- Upstream: \`$REPOSITORY\`"
  echo "- Version: \`$VERSION\`"
  echo "- Revision: \`$REVISION\`"
  echo "- Lock SHA-256: \`$lock_hash\`"
  echo "- Adapter patch SHA-256: \`$adapter_hash\`"
  echo "- Fetch preload SHA-256: \`$preload_hash\`"
  echo "- Shim base URL: \`$BASE_URL\`"
  echo "- Invocation: \`scripts/test-opencode-upstream.sh --base-url $BASE_URL\`"
  echo
  echo "The source hashes were verified before the transport-only patch was applied."
  echo "No upstream assertion or test registration is edited by the adapter."
  echo
  echo "## Suite results"
  echo
  echo "| # | Actual upstream suite | Exit | Tests | Pass | Fail | Full log |"
  echo "|---:|---|---:|---:|---:|---:|---|"
  while IFS="	" read -r index suite rc tests passed failed; do
    slug=$(printf '%s' "$suite" | tr '/.' '__')
    echo "| $index | \`$suite\` | $rc | $tests | $passed | $failed | [$slug.log]($slug.log) |"
  done <"$RESULTS"
  echo
  echo "## Explicit divergences"
  echo
  if awk -F '\t' '$3 != 0 { found=1 } END { exit !found }' "$RESULTS"; then
    echo "The following upstream-owned assertions failed. They are compatibility gaps, not skips:"
    echo
    while IFS="	" read -r index suite rc tests passed failed; do
      [ "$rc" -ne 0 ] || continue
      slug=$(printf '%s' "$suite" | tr '/.' '__')
      echo "### \`$suite\`"
      echo
      echo "Exit $rc; tests=$tests, pass=$passed, fail=$failed. Full evidence: [$slug.log]($slug.log)."
      echo
      echo '```text'
      sed -nE '/\(fail\)|error:|Expected:|Received:|timed out|Timeout|ECONNREFUSED|FAIL/p' "$EVIDENCE_DIR/$slug.log" | awk 'NR <= 80'
      echo '```'
      echo
    done <"$RESULTS"
  else
    echo "None. All named upstream suites passed against the shim."
  fi
} >"$SUMMARY"

status=0
if awk -F '\t' '$3 != 0 { found=1 } END { exit(found ? 0 : 1) }' "$RESULTS"; then
  status=1
fi
rm -f "$RESULTS"
exit "$status"
