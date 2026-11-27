#!/usr/bin/env bash
# scenario_s6_selector_selfproof.sh — pin the s6 selector contract (SCN-CONSENSUS-004).
#
# s6_approve_resume (approval gate / pause-resume) joined the scenario registry.
# This pins, dry-run level and with NO server boot and NO provider key:
#
#   1. token s6 selects exactly s6_approve_resume (short-prefix matching);
#   2. the full id s6_approve_resume selects itself;
#   3. a comma selector s1,s6 still selects both scenarios;
#   4. an unknown token s99 is a hard error (exit 2), never a silent zero-match;
#   5. s6_approve_resume is registered in ALL and never waits through await_idle
#      for the park — the park is awaited with wait_paused (SCN-CONSENSUS-024
#      made await_idle fail fast on paused, so s6 must expect the pause).
#
# usage: bash scripts/battery/scenario_s6_selector_selfproof.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCN="$REPO_ROOT/scripts/e2e/harness-scenarios.sh"
[ -r "$SCN" ] || { echo "selfproof: runner not found: $SCN" >&2; exit 2; }

TMP="$(mktemp -d -t scn-s6-selfproof.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()  { printf 'PASS  %s\n' "$1"; PASS=$((PASS+1)); }
bad() { local name="$1"; shift
        printf 'FAIL  %s\n' "$name"; printf '%s\n' "$@" | sed 's/^/      | /'; FAIL=$((FAIL+1)); }

run() { stdout="$(bash "$SCN" "$@" 2>"$TMP/err")"; rc=$?; }

# ── INVOCATION: both spellings select s6; s99 is a hard error ────────────────
run --scenarios s6 --dry-run
[ "$rc" -eq 0 ] && [ "$stdout" = "scenario: s6_approve_resume" ] \
  && ok "invocation: --scenarios s6 --dry-run selects s6_approve_resume" \
  || bad "invocation: --scenarios s6 --dry-run selects s6_approve_resume" \
       "rc=$rc out=$stdout err=$(cat "$TMP/err")"

run --scenarios s6_approve_resume --dry-run
[ "$rc" -eq 0 ] && [ "$stdout" = "scenario: s6_approve_resume" ] \
  && ok "invocation: --scenarios s6_approve_resume --dry-run selects itself" \
  || bad "invocation: --scenarios s6_approve_resume --dry-run selects itself" \
       "rc=$rc out=$stdout err=$(cat "$TMP/err")"

run --scenarios s1,s6 --dry-run
if [ "$rc" -eq 0 ] \
   && [ "$(printf '%s\n' "$stdout" | grep -c '^scenario: ')" -eq 2 ] \
   && printf '%s\n' "$stdout" | grep -q '^scenario: s6_approve_resume$'; then
  ok "invocation: comma selector s1,s6 selects both"
else
  bad "invocation: comma selector s1,s6 selects both" "rc=$rc out=$stdout err=$(cat "$TMP/err")"
fi

run --scenarios s99 --dry-run
[ "$rc" -eq 2 ] && ok "invocation: --scenarios s99 --dry-run exits 2 (never a silent zero-match)" \
                || bad "invocation: --scenarios s99 --dry-run exits 2 (never a silent zero-match)" \
                     "rc=$rc err=$(cat "$TMP/err")"

# ── STATIC: s6 registered, and the park goes through wait_paused ─────────────
grep -q 's6_approve_resume' "$SCN" \
  && ok "static: s6_approve_resume is registered in ALL" \
  || bad "static: s6_approve_resume is registered in ALL" "not found in $SCN"

grep -q '^wait_paused()' "$SCN" \
  && ok "static: wait_paused helper exists (the pause is awaited, not failed through)" \
  || bad "static: wait_paused helper exists (the pause is awaited, not failed through)" "no wait_paused in $SCN"

if grep -q 'wait_paused "$sid" 180' "$SCN" \
   && [ "$(grep -c 'await_idle s6 ' "$SCN")" -eq 0 ]; then
  ok "static: s6 parks via wait_paused and never awaits the wall through await_idle"
else
  bad "static: s6 parks via wait_paused and never awaits the wall through await_idle" \
      "wait_paused call: $(grep -c 'wait_paused "$sid" 180' "$SCN"), await_idle s6 call sites: $(grep -c 'await_idle s6 ' "$SCN")"
fi

grep -q 'approvals/.*/review\|/review" -H "$H"' "$SCN" || grep -q 'decision":"approved"' "$SCN" \
  && ok "static: s6 drives the approvals review API (decision: approved)" \
  || bad "static: s6 drives the approvals review API (decision: approved)" "no review POST in s6"

echo
echo "self-proof: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
