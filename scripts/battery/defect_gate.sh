#!/usr/bin/env bash
# defect_gate.sh — C-122, made executable.
# Re-reads the board LAST-WINS for the run-blocking defect id list and emits an
# exit contract: GREEN / RED / VOID. The plan's version emitted a word with no
# exit code and passed even when the verdict it emitted was RED.
#
# usage: defect_gate.sh <board.jsonl> <ids-file>
#   board.jsonl : .coding-hermes/board/tasks.jsonl (append-only; LAST line per id wins)
#   ids-file    : one defect row id per line (the S3 run-blocking list)
#
# exit 0 = GREEN  (every listed defect is complete)
# exit 1 = RED    (at least one listed defect is still open)
# exit 2 = VOID   (a listed id is absent from the board, or nothing was listed)
set -uo pipefail

BOARD="${1:-}"; IDS="${2:-}"
[ -n "$BOARD" ] && [ -r "$BOARD" ] || { echo "defect_gate: board unreadable: '${BOARD}'" >&2; exit 2; }
[ -n "$IDS" ] && [ -r "$IDS" ] || { echo "defect_gate: ids file unreadable: '${IDS}'" >&2; exit 2; }
command -v jq >/dev/null || { echo "defect_gate: jq required" >&2; exit 2; }

listed=0; open=0; absent=0
# last-wins: keep the LAST occurrence of each id, matching boardctl semantics
LAST=$(mktemp)
trap 'rm -f "$LAST"' EXIT
while IFS= read -r line; do
  [ -z "$line" ] && continue
  printf '%s\n' "$line" | jq -c 'select(.id!=null) | {id:.id, status:.status}' 2>/dev/null | tail -1
done < "$BOARD" | jq -s 'reduce .[] as $r ({}; .[$r.id]=$r.status)' > "$LAST"

printf '%-24s %s\n' DEFECT STATUS
while IFS= read -r id; do
  case "$id" in ''|\#*) continue;; esac
  listed=$((listed+1))
  st=$(jq -r --arg i "$id" '.[$i] // "ABSENT"' "$LAST")
  case "$st" in
    complete) printf '%-24s %s\n' "$id" "complete";;
    ABSENT)   printf '%-24s %s\n' "$id" "ABSENT-FROM-BOARD"; absent=$((absent+1));;
    *)        printf '%-24s %s\n' "$id" "$st"; open=$((open+1));;
  esac
done < "$IDS"

echo
[ "$listed" -eq 0 ] && { echo "defect_gate: VOID — the id list was empty; nothing was evaluated"; exit 2; }
[ "$absent" -gt 0 ] && { echo "defect_gate: VOID — ${absent} listed id(s) are absent from the board (cannot judge)"; exit 2; }
[ "$open"   -gt 0 ] && { echo "defect_gate: RED — ${open} of ${listed} run-blocking defect(s) still open"; exit 1; }
echo "defect_gate: GREEN — all ${listed} run-blocking defect(s) complete"; exit 0
