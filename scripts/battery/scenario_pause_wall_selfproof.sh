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

# ── extract wait_idle + await_idle + last_turn_error + status_of verbatim from the runner ──
# (scripted extraction so the behavioral arms exercise the real source lines);
# emit() is stubbed to append JSON lines to CAPTURE so the detail is assertable.
awk '
  /^status_of\(\) \{/ {p=1}
  /^last_turn_error\(\) \{/ {p=1}
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
  # The paused fixture carries a pending approval row (the approval-wall shape);
  # SCN-CONSENSUS-025: a paused session WITHOUT one is the circuit-breaker shape
  # and gets its own behavioral arm below.
  sqlite3 "$1" \
    "CREATE TABLE sessions (id TEXT PRIMARY KEY, status TEXT); INSERT INTO sessions VALUES ('sid-1','$2');
     CREATE TABLE approval_requests (id TEXT PRIMARY KEY, session_id TEXT, status TEXT, target_tool TEXT);$( [ "$2" = paused ] && printf " INSERT INTO approval_requests VALUES ('apr-1','sid-1','pending','write');" )"
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

# ── BEHAVIORAL: leg attribution (SCN-CONSENSUS-025) ───────────────────────────
# A multi-leg scenario names its leg in the failure detail, so a 3-leg session
# failure is attributable from the runner output alone.
cat > "$TMP/leg_inner.sh" <<'INNER'
: > "${CAPTURE:?}"
emit() { printf '%s\n' "$*" >> "${CAPTURE:?}"; }
DB="${FIX_DB:?}"
source "${HELPERS:?}"
start=$(date +%s)
await_idle s2 "sid-1" 5 leg=read
echo "rc=$? elapsed=$(( $(date +%s) - start ))"
INNER
behavior "failed: leg-attributed detail, rc=1" failed 1 "$TMP/leg_inner.sh"
if [ -s "$TMP/capture.jsonl" ] && grep -qF "leg=read" "$TMP/capture.jsonl" \
   && grep -qF "timeout/non-idle: failed, sid=sid-1, leg=read" "$TMP/capture.jsonl"; then
  ok "failed: detail names the leg (leg=read) alongside sid and status"
else
  bad "failed: detail names the leg (leg=read) alongside sid and status" \
      "capture: $(cat "$TMP/capture.jsonl" 2>/dev/null)"
fi

# ── BEHAVIORAL: paused attribution (SCN-CONSENSUS-025) ────────────────────────
# A paused session with NO pending approval row is the consecutive-error circuit
# breaker, not the approval wall — the detail must name it instead of blaming
# the approval wall (live mislabel, 2026-11-28 proof run 3: an --auto-approve
# run where no tool was gated cannot be a wall).
cat > "$TMP/breaker_inner.sh" <<'INNER'
: > "${CAPTURE:?}"
emit() { printf '%s\n' "$*" >> "${CAPTURE:?}"; }
DB="${FIX_DB:?}"
source "${HELPERS:?}"
start=$(date +%s)
await_idle s2 "sid-1" 5 leg=edit
echo "rc=$? elapsed=$(( $(date +%s) - start ))"
INNER
bdb="$(fix_db breaker-paused)"
sqlite3 "$bdb" \
  "CREATE TABLE sessions (id TEXT PRIMARY KEY, status TEXT); INSERT INTO sessions VALUES ('sid-1','paused');
   CREATE TABLE approval_requests (id TEXT PRIMARY KEY, session_id TEXT, status TEXT, target_tool TEXT);"
bout="$( FIX_DB="$bdb" CAPTURE="$TMP/capture.jsonl" HELPERS="$TMP/helpers.sh" bash "$TMP/breaker_inner.sh" 2>&1 )"
brc="$(printf '%s' "$bout" | sed -n 's/^rc=\([0-9]*\).*/\1/p')"
belapsed="$(printf '%s' "$bout" | sed -n 's/^rc=[0-9]* elapsed=\([0-9]*\).*/\1/p')"
bproblems=""
[ "$brc" = "1" ] || bproblems="$problems rc=$brc (want 1)"
[ "${belapsed:-999}" -le 2 ] || bproblems="$problems elapsed=$belapsed (want <= 2)"
if [ -z "$bproblems" ] && grep -qF "paused: no pending approval row — consecutive-error circuit breaker" "$TMP/capture.jsonl" \
   && grep -qF "leg=edit" "$TMP/capture.jsonl"; then
  ok "paused without a pending approval: detail names the circuit breaker + leg"
else
  bad "paused without a pending approval: detail names the circuit breaker + leg" \
      "$bproblems" "output: $bout" "capture: $(cat "$TMP/capture.jsonl" 2>/dev/null)"
fi

# ── STATIC: the source pins what the behavioral arms can't reach ─────────────
wl_line="$(grep -n 'paused) return 2' "$SCN" | head -1 | cut -d: -f1)"
[ -n "$wl_line" ] && ok "static: wait_idle has the paused arm (line $wl_line)" \
                    || bad "static: wait_idle has the paused arm" "no 'paused) return 2' in $SCN"

calls="$(grep -c 'await_idle s[0-9] "\$sid" "\$LEG_TIMEOUT"' "$SCN")"
[ "$calls" -eq 8 ] && ok "static: all 8 scenario wait_idle call sites go through await_idle with the LEG_TIMEOUT budget" \
                   || bad "static: all 8 scenario wait_idle call sites go through await_idle with the LEG_TIMEOUT budget" "found $calls"

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
if [ "$rc" -eq 0 ] && [ "$(printf '%s\n' "$stdout" | grep -c '^scenario: ')" -eq 8 ]; then
  ok "invocation: --auto-approve --dry-run accepted, all eight scenarios selected"
else
  bad "invocation: --auto-approve --dry-run accepted, all eight scenarios selected" \
      "rc=$rc out=$stdout err=$(cat "$TMP/err")"
fi
CONSENSUS_E2E_AUTO_APPROVE=1 bash "$SCN" --dry-run 2>"$TMP/err2" >"$TMP/out2"; rc2=$?
if [ "$rc2" -eq 0 ] && [ "$(grep -c '^scenario: ' "$TMP/out2")" -eq 8 ]; then
  ok "invocation: CONSENSUS_E2E_AUTO_APPROVE=1 --dry-run accepted, all eight scenarios selected"
else
  bad "invocation: CONSENSUS_E2E_AUTO_APPROVE=1 --dry-run accepted, all eight scenarios selected" \
      "rc2=$rc2 out: $(cat "$TMP/out2") err: $(cat "$TMP/err2")"
fi
run --bogus-arg --dry-run
[ "$rc" -eq 2 ] && ok "invocation: an unknown arg still exits 2 (regression guard)" \
                 || bad "invocation: an unknown arg still exits 2 (regression guard)" "rc=$rc"

echo
echo "self-proof: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
