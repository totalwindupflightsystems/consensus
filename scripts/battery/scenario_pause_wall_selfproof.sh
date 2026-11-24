#!/usr/bin/env bash
# scenario_pause_wall_selfproof.sh — pin the SCN-CONSENSUS-024 pause-wall contract.
#
# Unattended e2e scenario runs used to park on status 'paused' (a requires_approval
# tool with no human) and wait_idle burned the FULL per-scenario timeout (180s)
# before failing as a bare "timeout/non-idle". This harness pins the two fixes:
#
#   1. Fail-fast on paused: wait_idle detected in the SCRIPT SOURCE ITSELF
#      (extracted verbatim, no hand-copied twin) returns rc=2 within ~1s of a
#      paused session and the await_idle wrapper emits a detail naming
#      "paused: approval wall" and the session id — while a still-running
#      session (thinking) still consumes the whole wait and any other terminal
#      status stays immediate.
#   2. Unattended auto-approve: --auto-approve and CONSENSUS_E2E_AUTO_APPROVE=1
#      are accepted (also with --dry-run), documented in the usage header, and
#      the seeding seam (UPDATE tools_registry SET requires_approval = 0)
#      stands between the DB assignment and the scenario helpers, i.e. it runs
#      after boot and before any scenario creates a session.
#
# Behavioral arms drive the ACTUAL extracted functions against a fixture sqlite
# DB — the same wait_idle/await_idle source lines the runner executes.
#
# usage: bash scripts/battery/scenario_pause_wall_selfproof.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCN="$REPO_ROOT/scripts/e2e/harness-scenarios.sh"
[ -r "$SCN" ] || { echo "selfproof: runner not found: $SCN" >&2; exit 2; }

TMP="$(mktemp -d -t scn-pause-selfproof.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0
ok()  { printf 'PASS  %s\n' "$1"; PASS=$((PASS+1)); }
bad() { local name="$1"; shift
        printf 'FAIL  %s\n' "$name"; printf '%s\n' "$@" | sed 's/^/      | /'; FAIL=$((FAIL+1)); }

# ── extract wait_idle + await_idle + status_of verbatim from the runner ──────
# (scripted extraction so the behavioral arms exercise the real source lines);
# emit() is stubbed to append JSON lines to CAPTURE so the detail is assertable.
awk '
  /^status_of\(\) \{/ {p=1}
  /^wait_idle\(\) \{/ {p=1}
  /^await_idle\(\) \{/ {p=1}
  p {print; if ($0 ~ /^\}/) p=0}
' "$SCN" > "$TMP/helpers.sh"
if bash -n "$TMP/helpers.sh" 2>/dev/null && grep -q '^wait_idle()' "$TMP/helpers.sh" \
   && grep -q '^await_idle()' "$TMP/helpers.sh"; then
  ok "static: wait_idle+await_idle extract cleanly (parseable, real source used)"
else
  bad "static: wait_idle+await_idle extract cleanly" \
      "helpers.sh head: $(head -3 "$TMP/helpers.sh")"
fi

fix_db() { echo "$TMP/fix-$1-$RANDOM-$(date +%s%N).db"; }

make_fixture() { # <db> <status>
  sqlite3 "$1" \
    "CREATE TABLE sessions (id TEXT PRIMARY KEY, status TEXT); INSERT INTO sessions VALUES ('sid-1','$2');"
}

cat > "$TMP/behavior_inner.sh" <<'INNER'
: > "${CAPTURE:?}"
emit() { printf '%s\n' "$*" >> "${CAPTURE:?}"; }
DB="${FIX_DB:?}"
source "${HELPERS:?}"
start=$(date +%s)
await_idle s1 "sid-1" 5
echo "rc=$? elapsed=$(( $(date +%s) - start ))"
INNER

# wait_idle-level arm: the raw paused detection (rc=2) the wrapper keys on.
cat > "$TMP/wait_inner.sh" <<'INNER'
DB="${FIX_DB:?}"
source "${HELPERS:?}"
start=$(date +%s)
wait_idle "sid-1" 5
echo "rc=$? elapsed=$(( $(date +%s) - start ))"
INNER

behavior() { # <name> <status> <expect-rc> [inner] ; timed
  local name="$1" status="$2" want_rc="$3" inner="${4:-$TMP/behavior_inner.sh}"
  local db; db="$(fix_db "$status")"
  make_fixture "$db" "$status"
  local out rc
  out="$( FIX_DB="$db" CAPTURE="$TMP/capture.jsonl" HELPERS="$TMP/helpers.sh" \
          bash "$inner" 2>&1 )"
  rc="$(printf '%s' "$out" | sed -n 's/^rc=\([0-9]*\).*/\1/p')"
  local elapsed; elapsed="$(printf '%s' "$out" | sed -n 's/^rc=[0-9]* elapsed=\([0-9]*\).*/\1/p')"
  local problems=""
  [ "$rc" = "$want_rc" ] || problems="$problems rc=$rc (want $want_rc)"
  case "$status" in
    paused|failed|idle) [ "${elapsed:-999}" -le 2 ] || problems="$problems elapsed=$elapsed (want <= 2 for a parked session)" ;;
    *) [ "${elapsed:-0}" -ge 4 ] || problems="$problems elapsed=$elapsed (want >= 4: a live wait must still consume the timeout)" ;;
  esac
  if [ -z "$problems" ]; then ok "$name -> rc=$rc elapsed=${elapsed}s"
  else bad "$name" "$problems" "output: $out" "capture: $(cat "$TMP/capture.jsonl" 2>/dev/null)"; fi
}

echo "=== harness-scenarios.sh pause-wall self-proof (SCN-CONSENSUS-024) ==="
echo "runner: $SCN"
echo

# ── BEHAVIORAL: the real wait_idle/await_idle source against a fixture DB ────
behavior "wait_idle paused: raw fail-fast, rc=2, under 2s" paused 2 "$TMP/wait_inner.sh"
behavior "await_idle paused: scenario seam fails fast, rc=1" paused 1
if [ -s "$TMP/capture.jsonl" ] && grep -qF "paused: approval wall, sid=sid-1" "$TMP/capture.jsonl"; then
  ok "paused: emitted detail names 'paused: approval wall' + sid"
else
  bad "paused: emitted detail names 'paused: approval wall' + sid" \
      "capture: $(cat "$TMP/capture.jsonl" 2>/dev/null)"
fi
behavior "failed: still immediate, rc=1" failed 1
if [ -s "$TMP/capture.jsonl" ] && grep -qF "timeout/non-idle: failed" "$TMP/capture.jsonl"; then
  ok "failed: emitted detail keeps the timeout/non-idle spelling"
else
  bad "failed: emitted detail keeps the timeout/non-idle spelling" \
      "capture: $(cat "$TMP/capture.jsonl" 2>/dev/null)"
fi
behavior "idle: immediate success, rc=0" idle 0
behavior "thinking: still consumes the full timeout (live wait preserved), rc=1" thinking 1

# ── STATIC: the source pins what the behavioral arms can't reach ─────────────
wl_line="$(grep -n 'paused) return 2' "$SCN" | head -1 | cut -d: -f1)"
[ -n "$wl_line" ] && ok "static: wait_idle has the paused arm (line $wl_line)" \
                    || bad "static: wait_idle has the paused arm" "no 'paused) return 2' in $SCN"

calls="$(grep -c 'await_idle s[0-9] "\$sid" 180 || return 1' "$SCN")"
[ "$calls" -eq 6 ] && ok "static: all 6 scenario wait_idle call sites go through await_idle" \
                     || bad "static: all 6 scenario wait_idle call sites go through await_idle" "found $calls"

seed_after_db="$(grep -n '^DB="\$STATE/db.sqlite"' "$SCN" | head -1 | cut -d: -f1)"
seed_line="$(grep -n 'UPDATE tools_registry SET requires_approval = 0' "$SCN" | head -1 | cut -d: -f1)"
first_new_session="$(grep -n '^new_session()' "$SCN" | head -1 | cut -d: -f1)"
if [ -n "$seed_after_db" ] && [ -n "$seed_line" ] && [ -n "$first_new_session" ] \
   && [ "$seed_after_db" -lt "$seed_line" ] && [ "$seed_line" -lt "$first_new_session" ]; then
  ok "static: auto-approve seeding sits after the DB is known and before any session is created"
else
  bad "static: auto-approve seeding sits after the DB is known and before any session is created" \
      "db_line=${seed_after_db:-none} seed_line=${seed_line:-none} new_session_line=${first_new_session:-none}"
fi

grep -q -- '--auto-approve' "$SCN" && grep -q 'CONSENSUS_E2E_AUTO_APPROVE' "$SCN" \
  && ok "static: usage header documents --auto-approve and CONSENSUS_E2E_AUTO_APPROVE" \
  || bad "static: usage header documents --auto-approve and CONSENSUS_E2E_AUTO_APPROVE" "missing in the Env/Usage block"

# ── INVOCATION: both spellings accepted, --dry-run stays side-effect free ────
run() { stdout="$(bash "$SCN" "$@" 2>"$TMP/err")"; rc=$?; }
run --auto-approve --dry-run
if [ "$rc" -eq 0 ] && [ "$(printf '%s\n' "$stdout" | grep -c '^scenario: ')" -eq 5 ]; then
  ok "invocation: --auto-approve --dry-run accepted, all five scenarios selected"
else
  bad "invocation: --auto-approve --dry-run accepted, all five scenarios selected" \
      "rc=$rc out=$stdout err=$(cat "$TMP/err")"
fi
CONSENSUS_E2E_AUTO_APPROVE=1 bash "$SCN" --dry-run 2>"$TMP/err2" >"$TMP/out2"; rc2=$?
if [ "$rc2" -eq 0 ] && [ "$(grep -c '^scenario: ' "$TMP/out2")" -eq 5 ]; then
  ok "invocation: CONSENSUS_E2E_AUTO_APPROVE=1 --dry-run accepted, all five scenarios selected"
else
  bad "invocation: CONSENSUS_E2E_AUTO_APPROVE=1 --dry-run accepted, all five scenarios selected" \
      "rc2=$rc2 out: $(cat "$TMP/out2") err: $(cat "$TMP/err2")"
fi
run --bogus-arg --dry-run
[ "$rc" -eq 2 ] && ok "invocation: an unknown arg still exits 2 (regression guard)" \
                 || bad "invocation: an unknown arg still exits 2 (regression guard)" "rc=$rc"

echo
echo "self-proof: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
