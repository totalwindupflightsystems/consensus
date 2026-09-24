#!/bin/sh
set -eu

ROOT=$(git -C "$(dirname -- "$0")/.." rev-parse --show-toplevel)
EVIDENCE_DIR=${1:-"$ROOT/docs/evidence/opencode-cli-v1.18.29"}
BASE_URL=${CONSENSUS_OPENCODE_BASE_URL:-}
LLM_BASE_URL=${CONSENSUS_LLM_BASE_URL:-}
OUTPUT="$EVIDENCE_DIR/summary.md"

if [ -z "$BASE_URL" ]; then
  echo "ERROR: CONSENSUS_OPENCODE_BASE_URL is required" >&2
  exit 2
fi
mkdir -p "$EVIDENCE_DIR"
set +e
CONSENSUS_LLM_BASE_URL="$LLM_BASE_URL" \
  timeout 60 opencode run --attach "$BASE_URL" --pure --format json \
    "Return exactly the word CLI_PROOF_OK; do not call tools." \
    >"$EVIDENCE_DIR/raw.log" 2>&1
rc=$?
set -e
sed -E \
  -e 's/cs_(ak|sk)_[A-Za-z0-9_-]+/[REDACTED]/g' \
  -e 's/(authorization[^:]*:[[:space:]]*)[^[:space:]]+/\1[REDACTED]/Ig' \
  "$EVIDENCE_DIR/raw.log" >"$EVIDENCE_DIR/output.log"
rm -f "$EVIDENCE_DIR/raw.log"
{
  echo "# OpenCode CLI compatibility proof"
  echo
  echo "- CLI version: \`$(opencode --version 2>/dev/null || printf '%s' unknown)\`"
  echo "- Command: \`opencode run --attach <CONSENSUS_OPENCODE_BASE_URL> --pure --format json\`"
  echo "- Shim URL: \`$BASE_URL\`"
  echo "- LLM endpoint: \`$LLM_BASE_URL\`"
  echo "- Exit code: \`$rc\`"
  echo "- Sanitized output: [output.log](output.log)"
  echo
  if [ "$rc" -eq 0 ] && grep -q 'CLI_PROOF_OK' "$EVIDENCE_DIR/output.log"; then
    echo "Result: PASS — the installed OpenCode CLI completed a real attached session."
  else
    echo "Result: EXPLICIT DIVERGENCE — the attached CLI session did not complete the requested proof."
    echo "No failure was converted into a pass; inspect output.log and file a follow-up board row."
  fi
} >"$OUTPUT"
exit "$rc"
