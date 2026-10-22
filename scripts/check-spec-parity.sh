#!/usr/bin/env bash
# axiom:trace work_item=SHIM-SOURCE-003 spec=specs/024-shim-support-matrix.md impl=scripts/check-spec-parity.sh test=internal/api/openapi_parity_test.go
# check-spec-parity.sh — OpenAPI declared-vs-served parity guard (SHIM-SOURCE-003).
#
# specs/024 §A.3 requires the declared surface and the *served* surface to be
# compared mechanically (declared == served, per path). This script owns the
# part that needs a live server: it builds a scratch binary from the working
# tree, starts it on a free loopback port with a throwaway SQLite database,
# waits until /openapi.json answers, then hands the base URL to
# scripts/spec-parity-check.py (the comparator) and tears the server down.
#
# `GET /openapi.json` needs no API key, so the guard runs unattended.
# Exit codes: 0 = parity holds, 1 = drift found (named on stdout),
#             2 = the guard could not run (no server, bad spec, missing tool).
# Every run records the revision it ran at (`git rev-parse HEAD`).
#
# Usage: bash scripts/check-spec-parity.sh [--binary PATH] [--no-build]
#          [--commit SHA] [--spec-root DIR] [--keep-scratch] [--no-runtime]
set -euo pipefail

REPO="$(git rev-parse --show-toplevel)"
COMMIT="${CONSENSUS_SPEC_PARITY_COMMIT:-$(git -C "$REPO" rev-parse HEAD)}"
BINARY=""
BUILD=1
SPEC_ROOT=""
KEEP=0
EXTRA=()

while [ $# -gt 0 ]; do
  case "$1" in
    --binary)     BINARY="$2"; shift 2 ;;
    --no-build)   BUILD=0; shift ;;
    --commit)     COMMIT="$2"; shift 2 ;;
    --spec-root)  SPEC_ROOT="$2"; shift 2 ;;
    --keep-scratch) KEEP=1; shift ;;
    --no-runtime) EXTRA+=("--no-runtime"); shift ;;
    -h|--help)    sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "check-spec-parity: unknown option $1" >&2; exit 2 ;;
  esac
done

for tool in go python3 curl git; do
  command -v "$tool" >/dev/null 2>&1 || { echo "check-spec-parity: $tool is required" >&2; exit 2; }
done
python3 -c 'import yaml' 2>/dev/null || {
  echo "check-spec-parity: PyYAML is required (python3 -m pip install pyyaml)" >&2
  exit 2
}

SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/consensus-spec-parity.XXXXXX")"
PID=""
cleanup() {
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  # The serve log carries the admin key the server mints at startup; never
  # leave it behind unless the caller explicitly asked to keep the scratch dir.
  if [ "$KEEP" = "1" ]; then
    echo "check-spec-parity: scratch dir kept at $SCRATCH"
  else
    rm -rf "$SCRATCH"
  fi
}
trap cleanup EXIT

# A free loopback port, so a sibling server (or a docker-published one) can
# never be the thing we measure. The readiness loop below re-checks that OUR
# process is still alive, which catches the bind-failure race.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"

BIN="$BINARY"
if [ -z "$BIN" ]; then
  BIN="$SCRATCH/consensus"
  BUILD=1
fi
if [ "$BUILD" = "1" ]; then
  echo "check-spec-parity: building scratch binary at commit $COMMIT" >&2
  ( cd "$REPO" && CGO_ENABLED=0 go build -o "$BIN" ./cmd/consensus ) || {
    echo "check-spec-parity: go build ./cmd/consensus failed — the guard cannot run" >&2
    exit 2
  }
fi
[ -x "$BIN" ] || { echo "check-spec-parity: $BIN is not executable" >&2; exit 2; }

echo "check-spec-parity: starting scratch server on 127.0.0.1:$PORT (sqlite, mock LLM, no API key needed)"
CONSENSUS_PORT="$PORT" \
CONSENSUS_HOSTNAME="127.0.0.1" \
CONSENSUS_DB_URL="sqlite://$SCRATCH/parity.db" \
CONSENSUS_LLM_PROVIDER="mock" \
CONSENSUS_MOCK_LLM="1" \
  "$BIN" serve --db-url "sqlite://$SCRATCH/parity.db" > "$SCRATCH/serve.log" 2>&1 &
PID=$!

READY=0
for _ in $(seq 1 120); do
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "check-spec-parity: server exited before it served /openapi.json — log:" >&2
    sed -E 's/cs_[a-z]{2}_[0-9a-f]+/cs_[REDACTED]/g' "$SCRATCH/serve.log" | tail -20 >&2
    exit 2
  fi
  if [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://127.0.0.1:$PORT/openapi.json" || true)" = "200" ]; then
    READY=1
    break
  fi
  sleep 0.5
done
if [ "$READY" != "1" ]; then
  echo "check-spec-parity: /openapi.json did not answer 200 within 60s — log:" >&2
  sed -E 's/cs_[a-z]{2}_[0-9a-f]+/cs_[REDACTED]/g' "$SCRATCH/serve.log" | tail -20 >&2
  exit 2
fi

set +e
if [ -n "$SPEC_ROOT" ]; then
  python3 "$REPO/scripts/spec-parity-check.py" \
    --base-url "http://127.0.0.1:$PORT" --commit "$COMMIT" --spec-root "$SPEC_ROOT" "${EXTRA[@]+"${EXTRA[@]}"}"
else
  python3 "$REPO/scripts/spec-parity-check.py" \
    --base-url "http://127.0.0.1:$PORT" --commit "$COMMIT" "${EXTRA[@]+"${EXTRA[@]}"}"
fi
RC=$?
set -e

if [ "$RC" = "0" ]; then
  echo "check-spec-parity: OK — declared surface matches the served surface of the built server (commit $COMMIT)"
else
  echo "check-spec-parity: FAILED — drift at commit $COMMIT (see findings above)" >&2
  echo "  The declared contract is specs/openapi/openapi.yaml + paths/*.yaml." >&2
  echo "  The served contract is the embedded specs/openapi/bundled.yaml of the built binary." >&2
  echo "  Fix: re-run \`make bundle-spec\` and rebuild, or file the drift as a row (specs/024 §A.3)." >&2
fi
exit "$RC"
