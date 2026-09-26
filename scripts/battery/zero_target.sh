#!/usr/bin/env bash
# zero_target.sh — C-187, made executable.
# Every battery step exits 2 when it audited ZERO targets. A battery that audits
# nothing is VOID: rc=0 with audited_targets==0 is the false-green class (this is
# the shape that once reported "Passed: 4 | Failed: 0" while auditing nothing).
#
# usage: zero_target.sh <audited_targets> [label]
# exit 0 = audited >= 1 target
# exit 2 = VOID: audited 0 targets (or the count is missing/not a number)
set -uo pipefail

N="${1:-}"
LABEL="${2:-battery step}"
case "$N" in
  ''|*[!0-9]*) echo "zero_target: VOID — ${LABEL} reported an unauditable count ('${N}')"; exit 2;;
esac
if [ "$N" -eq 0 ]; then
  echo "zero_target: VOID — ${LABEL} audited ZERO targets; rc must not be read as success"; exit 2
fi
echo "zero_target: ok — ${LABEL} audited ${N} target(s)"; exit 0
