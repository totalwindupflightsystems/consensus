#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 — CLI smoke battery, part 2: remaining verbs + help tree.
set -u
export EDITOR=true
T=/tmp/r8/t-cli2.txt
BIN=/tmp/consensus-r8-bin
OUT_CAP=40
export CONSENSUS_SERVER=http://127.0.0.1:18090
export CONSENSUS_API_KEY=cs_ak_[REDACTED-scratch-admin-key]
: > "$T"

run() {
  local label="$1"; shift
  {
    echo "====================================================================="
    echo "# $label"
    echo "====================================================================="
    echo "\$ consensus $*"
    echo "---------------------------------------------------------------------"
  } >> "$T"
  local out rc lines
  out=$(timeout 25 "$BIN" "$@" < /dev/null 2>&1); rc=$?
  lines=$(printf '%s\n' "$out" | wc -l)
  printf '%s\n' "$out" | head -"$OUT_CAP" >> "$T"
  if [ "$lines" -gt "$OUT_CAP" ]; then
    printf '[... output truncated by the harness: %s lines total, %s shown ...]\n' "$lines" "$OUT_CAP" >> "$T"
  fi
  { echo "---------------------------------------------------------------------"; echo "[exit=$rc]"; echo ""; } >> "$T"
  printf '%-58s exit=%s\n' "$label" "$rc"
}

echo "CLI smoke battery part 2 — $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$T"

echo "### remaining verbs" | tee -a "$T"
run "help"                              help
run "help session"                      help session
run "mcp-stdio (stdin closed)"          mcp-stdio
run "migrate rollback"                  migrate rollback
run "migrate rollback --to 0"           migrate rollback --to 0
run "migrate run (again)"               migrate run
run "models sync --help"                models sync --help
run "approve (bare)"                    approve
run "reject (bare)"                     reject
run "reject (no args)"                  reject

echo ""
echo "part 2 complete" | tee -a "$T"
