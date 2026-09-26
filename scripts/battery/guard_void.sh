#!/usr/bin/env bash
# guard_void.sh — C-123, made executable.
# A tier with ZERO observables is VOID (not GREEN). This is the guard the plan
# only asserted in prose, against a counter field that did not exist.
#
# usage: guard_void.sh <results.json> [tier_counters.tsv]
# results.json: JSON array of rows; each row needs "tier" and one of
#   "verdict" (PASS|FAIL|WARN|VOID) or "observables" (integer).
#
# exit 0 = every tier in the map has >=1 observable
# exit 1 = malformed input / cannot evaluate
# exit 2 = VOID: at least one tier has zero observables (the run must not be called green)
set -uo pipefail

RESULTS="${1:-}"
MAP="${2:-$(dirname "$0")/tier_counters.tsv}"

[ -n "$RESULTS" ] && [ -r "$RESULTS" ] || { echo "guard_void: results file unreadable: '${RESULTS}'" >&2; exit 1; }
[ -r "$MAP" ] || { echo "guard_void: counter map unreadable: '${MAP}'" >&2; exit 1; }
command -v jq >/dev/null || { echo "guard_void: jq is required (install it on the run host)" >&2; exit 1; }

jq -e 'type=="array"' "$RESULTS" >/dev/null 2>&1 || { echo "guard_void: $RESULTS is not a JSON array" >&2; exit 1; }

void_tiers=0
checked=0
printf '%-6s %-26s %-12s %s\n' TIER COUNTER OBSERVABLES VERDICT
while IFS=$'\t' read -r tier counter what; do
  case "$tier" in ''|\#*) continue;; esac
  n=$(jq --arg t "$tier" '[.[] | select(.tier==$t) | (if has("observables") then (.observables|tonumber) elif (.verdict=="PASS" or .verdict=="FAIL" or .verdict=="WARN") then 1 else 0 end)] | add // 0' "$RESULTS")
  # a row that is itself VOID contributes nothing: it measured nothing
  n_void=$(jq --arg t "$tier" '[.[] | select(.tier==$t and .verdict=="VOID")] | length' "$RESULTS")
  checked=$((checked+1))
  if [ "$n" -le 0 ]; then
    printf '%-6s %-26s %-12s %s\n' "$tier" "$counter" "$n" "VOID"
    void_tiers=$((void_tiers+1))
  else
    printf '%-6s %-26s %-12s %s\n' "$tier" "$counter" "$n" "ok"
  fi
  [ "$n_void" -gt 0 ] 2>/dev/null && printf '       (note: %s row(s) in %s self-report VOID)\n' "$n_void" "$tier"
done < "$MAP"

echo
if [ "$checked" -eq 0 ]; then
  echo "guard_void: the counter map listed no tiers — nothing was audited"; exit 1
fi
if [ "$void_tiers" -gt 0 ]; then
  echo "guard_void: VOID — ${void_tiers} tier(s) have ZERO observables. The run may not be reported green."; exit 2
fi
echo "guard_void: ok — all ${checked} tiers report at least one observable."; exit 0
