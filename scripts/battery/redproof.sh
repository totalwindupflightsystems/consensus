#!/usr/bin/env bash
# redproof.sh — SG-1's actual deliverable.
# "Prove the verifier can fail." Each guard is run against a DELIBERATELY BAD input
# and its exit code is captured IMMEDIATELY, before any other command can clobber $?.
# It ALSO asserts the guard emitted its own verdict voice (e.g. "rederive: VOID"), so a
# guard that crashes with the numerically-right code is caught rather than credited —
# a syntax error exits 2 and would otherwise impersonate a correct VOID.
#
# usage: redproof.sh            (self-contained; builds its own stubs)
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
STUB=$(mktemp -d)
trap 'rm -rf "$STUB"' EXIT

pass=0; fail=0
check() { # check <label> <expected_rc> <expected_marker> <cmd...>
  local label="$1" want="$2" marker="$3"; shift 3
  "$@" > "$STUB/last.out" 2>&1
  local got=$?
  local rc_ok=0 mk_ok=0
  [ "$want" -eq "$got" ] && rc_ok=1
  if [ -z "$marker" ]; then mk_ok=1; else grep -qF -- "$marker" "$STUB/last.out" && mk_ok=1; fi
  if [ "$rc_ok" -eq 1 ] && [ "$mk_ok" -eq 1 ]; then
    printf '  RED-PROOF OK     %-46s want=%-2s got=%-2s voice=%s\n' "$label" "$want" "$got" "'$marker'"
    pass=$((pass+1))
  else
    printf '  RED-PROOF BROKEN %-46s want=%-2s got=%-2s rc_ok=%s voice_ok=%s\n' "$label" "$want" "$got" "$rc_ok" "$mk_ok"
    printf '                   <-- GUARD DID NOT FAIL CORRECTLY\n'
    fail=$((fail+1))
  fi
  printf '                   %s\n' "$(tail -1 "$STUB/last.out")"
}

echo "== SG-1 red-proof: can the guards fail, and for the RIGHT reason? =="
echo

cat > "$STUB/results-stub.json" <<'JSON'
[
 {"tier":"T0","verdict":"PASS"},
 {"tier":"T2","verdict":"PASS"},
 {"tier":"T6","verdict":"VOID","tests_executed":0},
 {"tier":"T7","verdict":"VOID","tests_executed":0}
]
JSON
check "guard_void, tiers with 0 observables -> VOID" 2 "guard_void: VOID" bash "$HERE/guard_void.sh" "$STUB/results-stub.json"
check "zero_target, audited_targets=0 -> VOID"       2 "zero_target: VOID" bash "$HERE/zero_target.sh" 0 "stub battery cell"
check "zero_target, missing count -> VOID"           2 "zero_target: VOID" bash "$HERE/zero_target.sh" "" "stub missing count"

cat > "$STUB/board-stub.jsonl" <<'JSONL'
{"id":"STUB-OPEN","status":"pending"}
{"id":"STUB-DONE","status":"complete"}
{"id":"STUB-OPEN","status":"pending"}
JSONL
printf 'STUB-DONE\n' > "$STUB/ids-clean.txt"
printf 'STUB-OPEN\n' > "$STUB/ids-open.txt"
printf 'STUB-DONE\nSTUB-GONE\n' > "$STUB/ids-absent.txt"
: > "$STUB/ids-empty.txt"

check "defect_gate, all complete -> GREEN"  0 "defect_gate: GREEN" bash "$HERE/defect_gate.sh" "$STUB/board-stub.jsonl" "$STUB/ids-clean.txt"
check "defect_gate, an OPEN defect -> RED"  1 "defect_gate: RED"   bash "$HERE/defect_gate.sh" "$STUB/board-stub.jsonl" "$STUB/ids-open.txt"
check "defect_gate, absent id -> VOID"      2 "defect_gate: VOID"  bash "$HERE/defect_gate.sh" "$STUB/board-stub.jsonl" "$STUB/ids-absent.txt"
check "defect_gate, empty id list -> VOID"  2 "defect_gate: VOID"  bash "$HERE/defect_gate.sh" "$STUB/board-stub.jsonl" "$STUB/ids-empty.txt"

mkdir -p "$STUB/empty-run"
check "rederive over an empty run dir -> VOID" 2 "rederive: VOID" bash "$HERE/rederive.sh" "$STUB/empty-run"

cat > "$STUB/results-good.json" <<'JSON'
[{"tier":"T0","verdict":"PASS"},{"tier":"T1","verdict":"PASS"},{"tier":"T2","verdict":"PASS"},
 {"tier":"T3","verdict":"PASS"},{"tier":"T4","verdict":"PASS"},{"tier":"T5","verdict":"PASS"},
 {"tier":"T6","verdict":"PASS"},{"tier":"T7","verdict":"PASS"},{"tier":"T8","verdict":"PASS"},
 {"tier":"T9","verdict":"PASS"},{"tier":"T10","verdict":"PASS"},{"tier":"GATE","verdict":"PASS"},
 {"tier":"S1","verdict":"PASS"},{"tier":"S3","verdict":"PASS"},{"tier":"S4","verdict":"PASS"},
 {"tier":"S5","verdict":"PASS"}]
JSON
check "POSITIVE CTL guard_void, all observed -> ok" 0 "guard_void: ok" bash "$HERE/guard_void.sh" "$STUB/results-good.json"
check "POSITIVE CTL zero_target, 7 targets -> ok"   0 "zero_target: ok" bash "$HERE/zero_target.sh" 7 "positive control"
check "POSITIVE CTL rederive on a real dir -> ok"   0 "rederive: ok"    bash "$HERE/rederive.sh" "$HERE"

echo
echo "== SG-1 red-proof summary: ${pass} ok, ${fail} broken =="
if [ "$fail" -gt 0 ]; then
  echo "RESULT: FAIL — ${fail} guard(s) did not fail correctly on bad input. No green from this plan is trustworthy."
  exit 1
fi
echo "RESULT: PASS — every guard failed with its own VOID/RED verdict on bad input, and passed on good input."
exit 0
