#!/usr/bin/env bash
# harness-scenarios.sh — live end-to-end scenario runner for the Consensus harness.
#
# Boots a throwaway server (fresh SQLite DB, spare port, real provider), drives it
# through the public HTTP API, and asserts on the filesystem + tool tables.
# Unit tests prove internals; THIS proves the assembled system.
#
# Usage:  scripts/e2e/harness-scenarios.sh [--keep] [--scenarios s1,s3,s4]
# Exit:   0 = every selected scenario passed, 1 = at least one failed.
#
# Env:    HARNESS_PROVIDER_KEY (otherwise read DEEPSEEK_API_KEY from ~/.hermes/.env),
#         HARNESS_PROVIDER_BASE_URL (optional OpenAI-compatible endpoint),
#         HARNESS_MODEL (model pin; takes precedence over CONSENSUS_MODEL),
#         CONSENSUS_MODEL (optional model pin), PORT (default 18460). When the
#         fallback file is used, DEEPSEEK_API_KEY supplies the provider key.

set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
PORT=${PORT:-18460}
KEEP=0
ONLY=""
while [ $# -gt 0 ]; do
  case "$1" in
    --keep) KEEP=1 ;;
    --scenarios) shift; ONLY="${1:-}" ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done

STATE=$(mktemp -d "${TMPDIR:-/tmp}/consensus-scn.XXXXXX") || exit 1
export TMPDIR="$STATE/tmp"; mkdir -p "$TMPDIR"
LOG="$STATE/serve.log"
SRV_PID=""
RESULTS=()
FAILS=0

cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null
  sleep 1
  if [ "$KEEP" -eq 1 ]; then echo "state kept: $STATE"; else rm -rf "$STATE"; fi
}
trap cleanup EXIT

log()  { printf '%s\n' "$*" >&2; }
emit() { printf '{"scenario":"%s","status":"%s","detail":%s}\n' "$1" "$2" "$(jq -Rn --arg d "$3" '$d')"; }

# ---------- build ----------
BIN="$STATE/consensus"
if ! ( cd "$ROOT" && CGO_ENABLED=0 go build -o "$BIN" ./cmd/consensus ); then
  emit build fail "go build failed"
  exit 1
fi

# ---------- boot ----------
DK=${HARNESS_PROVIDER_KEY:-}
if [ -z "$DK" ]; then
  DK=$(grep -m1 -oP '^DEEPSEEK_API_KEY=\K.*' "$HOME/.hermes/.env" 2>/dev/null)
fi
[ -n "$DK" ] || { emit boot fail "no provider key: set HARNESS_PROVIDER_KEY (or DEEPSEEK_API_KEY)"; exit 1; }
if [ -n "${HARNESS_PROVIDER_BASE_URL:-}" ]; then
  export CONSENSUS_LLM_BASE_URL="$HARNESS_PROVIDER_BASE_URL"
fi
if [ -n "${HARNESS_MODEL:-${CONSENSUS_MODEL:-}}" ]; then
  export CONSENSUS_LLM_MODEL="${HARNESS_MODEL:-$CONSENSUS_MODEL}"
fi
# CONSENSUS_API_KEY outranks inherited provider-specific keys in config resolution.
unset OPENROUTER_API_KEY
CONSENSUS_API_KEY="$DK" DEEPSEEK_API_KEY="$DK" \
CONSENSUS_DB_URL="sqlite://$STATE/db.sqlite" \
  "$BIN" serve --hostname 127.0.0.1 --port "$PORT" --log-level info >"$LOG" 2>&1 &
SRV_PID=$!

API=""
for _ in $(seq 1 40); do
  API=$(grep -m1 -oE 'cs_ak_[A-Za-z0-9_-]{10,}' "$LOG" 2>/dev/null)
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/openapi.json" && [ -n "$API" ] && break
  sleep 1
done
[ -n "$API" ] || { emit boot fail "server did not expose an API key"; exit 1; }
log "server up on :$PORT (pid $SRV_PID)"

DB="$STATE/db.sqlite"
H="Authorization: Bearer $API"

# ---------- helpers ----------
new_session() { # $1=goal -> echoes session id
  curl -s -X POST "http://127.0.0.1:$PORT/api/v1/sessions" -H "$H" \
    -H 'Content-Type: application/json' -d "$(jq -cn --arg g "$1" '{agent_name:"scenario",goal:$g}')" \
    | jq -r '.id // empty'
}
say() { # $1=sid $2=content
  curl -s -X POST "http://127.0.0.1:$PORT/api/v1/sessions/$1/message" -H "$H" \
    -H 'Content-Type: application/json' \
    -d "$(jq -cn --arg c "$2" '{content:$c,type:"user_instruction"}')" >/dev/null
}
status_of() { sqlite3 "$DB" "SELECT status FROM sessions WHERE id='$1';" 2>/dev/null; }
wait_idle() { # $1=sid $2=timeout_s -> 0 if idle
  local i; for i in $(seq 1 "$2"); do
    case "$(status_of "$1")" in
      idle|completed) return 0 ;;
      failed) return 1 ;;
    esac
    sleep 1
  done
  return 1
}
ws_dir() { echo "$TMPDIR/consensus-workspaces/$1"; }

# ---------- scenarios ----------
s1_write_file() {
  local sid; sid=$(new_session "Write a file using your tools") || { emit s1 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: echo -n SCN-OK > scn1.txt (then report DONE)."
  wait_idle "$sid" 180 || { emit s1 fail "timeout/non-idle: $(status_of "$sid")"; return 1; }
  local f; f="$(ws_dir "$sid")/scn1.txt"
  [ -f "$f" ] && grep -q SCN-OK "$f" && { emit s1 pass "wrote $f"; return 0; }
  emit s1 fail "file missing or wrong content: $f"
  return 1
}

s2_read_file() {
  local sid; sid=$(new_session "Read a file using your tools") || { emit s2 fail "no session"; return 1; }
  mkdir -p "$(ws_dir "$sid")"
  printf 'SEED-READ-42\n' >"$(ws_dir "$sid")/seed.txt"
  say "$sid" "Read the file seed.txt in your workspace with your read tool and reply with the exact number you found."
  wait_idle "$sid" 180 || { emit s2 fail "timeout/non-idle: $(status_of "$sid")"; return 1; }
  if sqlite3 "$DB" "SELECT count(*) FROM tool_results WHERE output LIKE '%SEED-READ-42%';" | grep -q '^[1-9]'; then
    emit s2 pass "read tool returned the seeded content"; return 0
  fi
  emit s2 fail "no tool result contains the seeded content"
  return 1
}

s3_bash() {
  local sid; sid=$(new_session "Run a shell command") || { emit s3 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: printf SCN-BASH > scn3.out ; then report DONE."
  wait_idle "$sid" 180 || { emit s3 fail "timeout/non-idle: $(status_of "$sid")"; return 1; }
  local f; f="$(ws_dir "$sid")/scn3.out"
  [ -f "$f" ] && grep -q SCN-BASH "$f" && { emit s3 pass "bash executed"; return 0; }
  emit s3 fail "bash output file missing: $f"
  return 1
}

s4_conversation_loop() {
  local sid; sid=$(new_session "Multi-turn tool use") || { emit s4 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: echo -n ONE > turn1.txt"
  wait_idle "$sid" 180 || { emit s4 fail "turn1 timeout: $(status_of "$sid")"; return 1; }
  say "$sid" "Now use your bash tool to run: echo -n TWO > turn2.txt"
  wait_idle "$sid" 180 || { emit s4 fail "turn2 timeout: $(status_of "$sid")"; return 1; }
  local d; d=$(ws_dir "$sid")
  [ -f "$d/turn1.txt" ] && [ -f "$d/turn2.txt" ] && { emit s4 pass "two turns, two files"; return 0; }
  emit s4 fail "missing turn files in $d"
  return 1
}

s5_containment() {
  local sid; sid=$(new_session "Containment check") || { emit s5 fail "no session"; return 1; }
  local outside="$TMPDIR/consensus-workspaces/ESCAPED.txt"
  say "$sid" "Use your bash tool to run: echo -n ESCAPE > ../ESCAPED.txt ; report DONE either way."
  wait_idle "$sid" 180 || { emit s5 fail "timeout/non-idle: $(status_of "$sid")"; return 1; }
  if [ -f "$outside" ]; then emit s5 fail "ESCAPED the workspace: $outside"; return 1; fi
  emit s5 pass "no escape file outside the session workspace"
  return 0
}

ALL="s1_write_file s2_read_file s3_bash s4_conversation_loop s5_containment"
run_one() {
  local fn="$1"
  if [ -n "$ONLY" ]; then
    local id="${fn%%_*}"
    case ",$ONLY," in *",$id,"*) ;; *) return 0 ;; esac
  fi
  log "--- $fn"
  if "$fn"; then RESULTS+=("$fn=PASS"); else RESULTS+=("$fn=FAIL"); FAILS=$((FAILS+1)); fi
}

for fn in $ALL; do run_one "$fn"; done

log "=== summary ==="
printf '%s\n' "${RESULTS[@]}" >&2
log "failures: $FAILS"
[ "$FAILS" -eq 0 ]
