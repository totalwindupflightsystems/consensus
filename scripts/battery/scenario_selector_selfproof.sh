#!/usr/bin/env bash
# scenario_selector_selfproof.sh — prove the e2e scenario selector cannot go vacuous.
#
# `scripts/e2e/harness-scenarios.sh --scenarios <tok,...>` used to compare each token
# against the scenario's bare prefix only (s1..s5). Any other token -- a typo, a bare
# `s8`, or even the documented full id `s4_conversation_loop` -- matched NO function,
# so zero scenarios ran, FAILS stayed 0 and the script exited 0 with a green
# "failures: 0" summary (SCN-CONSENSUS-023, measured 2026-11-21).
#
# This harness pins the whole selector contract, cheaply and without a provider:
#
#   RED    --scenarios s8                  exit 2, names s8, no green summary
#   RED    --scenarios s4_conversation     exit 2, names the token (typo arm)
#   RED    --scenarios s1,s8               exit 2 before running s1 (no partial run)
#   RED    --scenarios ,                   exit 2, "names no scenario"
#   GREEN  --scenarios s4_conversation_loop --dry-run   full id is accepted (s4 only)
#   GREEN  --scenarios s4 --dry-run                     bare prefix (s4 only)
#   GREEN  --scenarios s1,s3 --dry-run                  multi-selector = exactly s1,s3
#   GREEN  --dry-run                                    no selector = all five
#   GREEN  --scenarios ' s1 , s5 ,' --dry-run           whitespace + trailing comma
#   ORDER  unknown token is rejected BEFORE the build (stub `go` never invoked)
#   ORDER  a known token passes validation INTO the build (stub `go` invoked)
#   STATIC validation sits above the build in the source; run_one goes through it
#
# usage: bash scripts/battery/scenario_selector_selfproof.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCN="$REPO_ROOT/scripts/e2e/harness-scenarios.sh"
[ -r "$SCN" ] || { echo "selfproof: runner not found: $SCN" >&2; exit 2; }

TMP="$(mktemp -d -t scn-selector-selfproof.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()  { printf 'PASS  %s\n' "$1"; PASS=$((PASS+1)); }
bad() { local name="$1"; shift
        printf 'FAIL  %s\n' "$name"; printf '%s\n' "$@" | sed 's/^/      | /'; FAIL=$((FAIL+1)); }

RC=0; OUT=""; ERR=""
run() { # run <runner args...> -> RC, OUT (stdout), ERR (stderr)
    OUT="$(bash "$SCN" "$@" 2>"$TMP/err")"; RC=$?
    ERR="$(cat "$TMP/err")"
}

# ── stub `go`: proves whether the run got past selector validation ───────────
mkdir -p "$TMP/stub"
cat > "$TMP/stub/go" <<'STUB'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "${STUB_GO_MARK:?}"
echo "stub-go: build intentionally not performed" >&2
exit 1
STUB
chmod +x "$TMP/stub/go"
export STUB_GO_MARK="$TMP/go-invoked"
: > "$STUB_GO_MARK"

arm_reject() { # <name> <token> <expect-error-substring> <args...>
    local name="$1" tok="$2" want="$3"; shift 3
    run "$@"
    local all problems=""
    all="$OUT$ERR"
    [ "$RC" -eq 2 ] || problems="$problems rc=$RC (want 2)"
    printf '%s' "$ERR" | grep -qF "$want" || problems="$problems missing-error[$want]"
    printf '%s' "$all" | grep -qF 'unknown scenario' || problems="$problems no-unknown-error"
    printf '%s' "$all" | grep -q 'failures:' && problems="$problems green-summary-printed"
    printf '%s' "$all" | grep -q '"scenario"' && problems="$problems scenario-emitted"
    if [ -z "$problems" ]; then ok "$name -> rc=$RC '$want'"
    else bad "$name" "$problems" "stdout: $OUT" "stderr: $ERR"; fi
}

arm_selection() { # <name> <expected fn list> <args...>
    local name="$1" want="$2"; shift 2
    run "$@"
    local got sorted_want
    got="$(printf '%s\n' "$OUT" | sed -n 's/^scenario: //p' | sort | tr '\n' ' ' | sed 's/ *$//')"
    sorted_want="$(printf '%s\n' $want | sort | tr '\n' ' ' | sed 's/ *$//')"
    if [ "$RC" -eq 0 ] && [ -n "$got" ] && [ "$got" = "$sorted_want" ]; then
        ok "$name -> [$got]"
    else
        bad "$name" "rc=$RC (want 0)" "selected=[$got] (want [$sorted_want])" "stderr: $ERR"
    fi
}

echo "=== harness-scenarios.sh selector self-proof (SCN-CONSENSUS-023) ==="
echo "runner: $SCN"
echo

# ── RED: selectors that name nothing must be refused, loudly, exit 2 ────────
arm_reject "unknown: s8"                  s8              "unknown scenario 's8'"            --scenarios s8
arm_reject "unknown: full-id typo"        s4_conversation "unknown scenario 's4_conversation'" --scenarios s4_conversation
arm_reject "unknown: mixed s1,s8"         s8              "unknown scenario 's8'"            --scenarios s1,s8
rm -f "$TMP/err"; run --scenarios ,
if [ "$RC" -eq 2 ] && printf '%s' "$ERR" | grep -qF "names no scenario" \
   && ! printf '%s' "$OUT$ERR" | grep -q 'failures:'; then
    ok "unknown: empty selector (',') -> rc=2 'names no scenario'"
else
    bad "unknown: empty selector (',')" "rc=$RC (want 2)" "stdout: $OUT" "stderr: $ERR"
fi

# ── GREEN: every documented spelling selects the scenarios it names ─────────
arm_selection "full id accepted: s4_conversation_loop" "s4_conversation_loop" --scenarios s4_conversation_loop --dry-run
arm_selection "bare prefix: s4"                        "s4_conversation_loop" --scenarios s4 --dry-run
arm_selection "multi-selector: s1,s3"                  "s1_write_file s3_bash" --scenarios s1,s3 --dry-run
arm_selection "no selector: all five" \
    "s1_write_file s2_read_file s3_bash s4_conversation_loop s5_containment" --dry-run
arm_selection "whitespace + trailing comma: ' s1 , s5 ,'" "s1_write_file s5_containment" \
    --scenarios ' s1 , s5 ,' --dry-run

# ── ORDER: validation runs before the build (stub go is the tripwire) ───────
: > "$STUB_GO_MARK"
RC=0; OUT="$(PATH="$TMP/stub:$PATH" bash "$SCN" --scenarios s8 2>"$TMP/err")"; RC=$?
ERR="$(cat "$TMP/err")"
if [ "$RC" -eq 2 ] && printf '%s' "$ERR" | grep -qF "unknown scenario 's8'" \
   && [ ! -s "$STUB_GO_MARK" ]; then
    ok "order: unknown token rejected before the build (go never invoked)"
else
    bad "order: unknown token rejected before the build" \
        "rc=$RC (want 2)" "go invocations: $(wc -l <"$STUB_GO_MARK")" "stdout: $OUT" "stderr: $ERR"
fi

: > "$STUB_GO_MARK"
RC=0; OUT="$(PATH="$TMP/stub:$PATH" bash "$SCN" --scenarios s1,s3 2>"$TMP/err")"; RC=$?
ERR="$(cat "$TMP/err")"
if [ "$RC" -eq 1 ] && [ -s "$STUB_GO_MARK" ] \
   && printf '%s' "$OUT$ERR" | grep -q 'go build failed' \
   && ! printf '%s' "$ERR" | grep -q 'unknown scenario'; then
    ok "order: a known selector passes validation into the build (rc=1 at build, not 2)"
else
    bad "order: a known selector passes validation into the build" \
        "rc=$RC (want 1)" "go invocations: $(wc -l <"$STUB_GO_MARK")" "stdout: $OUT" "stderr: $ERR"
fi

# ── STATIC: single decision point, validated above the build ───────────────
sel_line="$(grep -n "unknown scenario '" "$SCN" | head -1 | cut -d: -f1)"
build_line="$(grep -n 'go build' "$SCN" | head -1 | cut -d: -f1)"
if [ -n "$sel_line" ] && [ -n "$build_line" ] && [ "$sel_line" -lt "$build_line" ] \
   && grep -q 'scenario_selected "\$fn"' "$SCN"; then
    ok "static: validation (line $sel_line) precedes build (line $build_line); run_one uses scenario_selected"
else
    bad "static: validation precedes build / run_one uses the selector" \
        "sel_line=${sel_line:-none} build_line=${build_line:-none}"
fi

echo
echo "self-proof: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
