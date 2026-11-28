#!/usr/bin/env bash
# harness-scenarios.sh — live end-to-end scenario runner for the Consensus harness.
#
# Boots a throwaway server (fresh SQLite DB, spare port, real provider), drives it
# through the public HTTP API, and asserts on the filesystem + tool tables.
# Unit tests prove internals; THIS proves the assembled system.
#
# Usage:  scripts/e2e/harness-scenarios.sh [--keep] [--scenarios s1,s3,s4]
#         [--auto-approve]
#         scripts/e2e/harness-scenarios.sh --scenarios s1 --dry-run
# Exit:   0 = every selected scenario passed, 1 = at least one failed,
#         2 = bad invocation (unknown arg, or a selector that names no known
#             scenario — a selector must never match nothing silently).
#
# --scenarios accepts a bare prefix (s1) or a full scenario id (s1_write_file),
# comma-separated. An unknown token, or a selector that ends up executing zero
# scenarios, is a hard error — never a green "failures: 0".
# --dry-run validates the selector and prints the selected scenario ids
# (`scenario: <fn>`, one per line) without building or booting anything.
#
# Env:    HARNESS_PROVIDER_KEY (otherwise read DEEPSEEK_API_KEY from ~/.hermes/.env),
#         HARNESS_PROVIDER_BASE_URL (optional OpenAI-compatible endpoint),
#         HARNESS_MODEL (model pin; takes precedence over CONSENSUS_MODEL),
#         CONSENSUS_MODEL (optional model pin),
#         CONSENSUS_E2E_AUTO_APPROVE=1 (same as --auto-approve), PORT (default
#         18460). When the fallback file is used, DEEPSEEK_API_KEY supplies the
#         provider key.
#
# Unattended runs and the approval wall (SCN-CONSENSUS-024):
#   --auto-approve / CONSENSUS_E2E_AUTO_APPROVE=1 arms the e2e approval path for
#   unattended runs: right after boot the runner seeds every enabled registry
#   tool with requires_approval=0 before any scenario creates a session, so a
#   requires_approval tool never parks a scenario at 'paused' waiting for a
#   human. Seeding at the registry, NOT a runtime approve-loop, is the clean
#   seam: the planning-path gate (planHasGatedTools) re-reads the registry on
#   every attempt and ignores already-approved approvals, so approving a parked
#   session just re-pauses it the next time the model re-issues the call.
#   wait_idle also fails the scenario FAST on a paused session — it never burns
#   the full 180s timeout on the approval wall, and the emitted detail names
#   "paused: approval wall" with the session id.

set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
PORT=${PORT:-18460}
KEEP=0
DRY_RUN=0
ONLY=""
AUTO_APPROVE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --keep) KEEP=1 ;;
    --scenarios) shift; ONLY="${1:-}" ;;
    --dry-run) DRY_RUN=1 ;;
    --auto-approve) AUTO_APPROVE=1 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done
if [ "${CONSENSUS_E2E_AUTO_APPROVE:-0}" = "1" ]; then
  AUTO_APPROVE=1
fi

log()  { printf '%s\n' "$*" >&2; }
emit() { printf '{"scenario":"%s","status":"%s","detail":%s}\n' "$1" "$2" "$(jq -Rn --arg d "$3" '$d')"; }

# ---------- scenario registry + selector (validated before anything boots) ----------
ALL="s1_write_file s2_read_file s3_bash s4_conversation_loop s5_containment s6_approve_resume s8_restart_durability s13_concurrent_sessions"

scenario_matches() { # $1=fn $2=selector token -> 0 when the token names this scenario
  local fn="$1" tok="$2"
  [ "$tok" = "${fn%%_*}" ] || [ "$tok" = "$fn" ]
}

known_scenarios() { # both accepted spellings, for the error message
  local fn out=""
  for fn in $ALL; do out="$out, ${fn%%_*}|$fn"; done
  printf '%s' "${out#, }"
}

scenario_selected() { # $1=fn -> 0 when no selector was given, or when a token names it
  local fn="$1" tok
  [ -n "$ONLY" ] || return 0
  for tok in "${SCEN_TOKENS[@]}"; do
    scenario_matches "$fn" "$tok" && return 0
  done
  return 1
}

# Split the selector once, trimming whitespace and dropping empty tokens
# (",," / a trailing comma is tolerated; a selector with no token at all is not).
SCEN_TOKENS=()
if [ -n "$ONLY" ]; then
  _ifs="$IFS"; IFS=','
  for _tok in $ONLY; do
    _tok=$(printf '%s' "$_tok" | tr -d '[:space:]')
    [ -n "$_tok" ] && SCEN_TOKENS+=("$_tok")
  done
  IFS="$_ifs"
  if [ "${#SCEN_TOKENS[@]}" -eq 0 ]; then
    log "error: --scenarios '$ONLY' names no scenario"
    log "known: $(known_scenarios)"
    exit 2
  fi
  for _tok in "${SCEN_TOKENS[@]}"; do
    _hit=0
    for _fn in $ALL; do scenario_matches "$_fn" "$_tok" && { _hit=1; break; }; done
    if [ "$_hit" -eq 0 ]; then
      log "error: unknown scenario '$_tok' (from --scenarios '$ONLY')"
      log "known: $(known_scenarios)"
      exit 2
    fi
  done
fi

if [ "$DRY_RUN" -eq 1 ]; then
  for _fn in $ALL; do
    scenario_selected "$_fn" && printf 'scenario: %s\n' "$_fn"
  done
  exit 0
fi

STATE=$(mktemp -d "${TMPDIR:-/tmp}/consensus-scn.XXXXXX") || exit 1
export TMPDIR="$STATE/tmp"; mkdir -p "$TMPDIR"
LOG="$STATE/serve.log"
SRV_PID=""
RESULTS=()
FAILS=0
RAN=0

cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null
  sleep 1
  if [ "$KEEP" -eq 1 ]; then echo "state kept: $STATE"; else rm -rf "$STATE"; fi
}
trap cleanup EXIT

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

# ---------- unattended approval path (SCN-CONSENSUS-024) ----------
# With --auto-approve the runner ungates every enabled registry tool BEFORE any
# scenario creates a session. Comment in the header for why this is seeding, not
# a runtime approve-loop: the planning-path gate re-reads requires_approval and
# ignores an already-approved approval, so approving only re-pauses.
if [ "$AUTO_APPROVE" -eq 1 ]; then
  if sqlite3 "$DB" "UPDATE tools_registry SET requires_approval = 0 WHERE enabled = 1;"; then
    log "auto-approve: seeded requires_approval=0 for every enabled registry tool"
  else
    emit boot fail "auto-approve: failed to seed requires_approval in $DB"
    exit 1
  fi
fi

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
wait_idle() { # $1=sid $2=timeout_s -> 0 if idle, 2 if paused (approval wall), 1 otherwise
  local i st
  for i in $(seq 1 "$2"); do
    st=$(status_of "$1")
    case "$st" in
      idle|completed) return 0 ;;
      paused) return 2 ;; # SCN-CONSENSUS-024: fail fast, never burn the timeout on the approval wall
      failed) return 1 ;;
    esac
    sleep 1
  done
  return 1
}
await_idle() { # $1=scenario-name $2=sid $3=timeout_s -> 0 if idle; emits the failure and returns 1 otherwise
  if wait_idle "$2" "$3"; then return 0; fi
  local st; st=$(status_of "$2")
  case "$st" in
    paused) emit "$1" fail "paused: approval wall, sid=$2 (arm --auto-approve to ungate tools)"; return 1 ;;
    *) emit "$1" fail "timeout/non-idle: $st"; return 1 ;;
  esac
}
wait_paused() { # $1=sid $2=timeout_s -> 0 if paused, 1 otherwise (s6: the park IS the expected state)
  local i st
  for i in $(seq 1 "$2"); do
    st=$(status_of "$1")
    case "$st" in
      paused) return 0 ;;
      failed|completed) return 1 ;;
    esac
    sleep 1
  done
  return 1
}
wait_terminal() { # $1=sid $2=timeout_s -> echoes the FIRST terminal status reached
  # (idle|completed|paused|failed), or 'timeout:<last-seen>' when none arrives
  # within the budget. Sessions are born 'booting' and a message POST flips
  # them to 'thinking', so any of the four terminal values ends the wait —
  # paused fails fast per the SCN-CONSENSUS-024 convention, never burning the
  # budget on the approval wall. Used by s13 to wait on 4 sessions at once.
  local i st=""
  for i in $(seq 1 "$2"); do
    st=$(status_of "$1")
    case "$st" in
      idle|completed|paused|failed) printf '%s' "$st"; return 0 ;;
    esac
    sleep 1
  done
  printf 'timeout:%s' "${st:-unknown}"
  return 0
}
ws_dir() { echo "$TMPDIR/consensus-workspaces/$1"; }

# ---------- scenarios ----------
s1_write_file() {
  local sid; sid=$(new_session "Write a file using your tools") || { emit s1 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: echo -n SCN-OK > scn1.txt (then report DONE)."
  await_idle s1 "$sid" 180 || return 1
  local f; f="$(ws_dir "$sid")/scn1.txt"
  [ -f "$f" ] && grep -q SCN-OK "$f" && { emit s1 pass "wrote $f"; return 0; }
  emit s1 fail "file missing or wrong content: $f"
  return 1
}

s2_read_file() {
  local sid; sid=$(new_session "Read, edit and glob files using your tools") || { emit s2 fail "no session"; return 1; }
  local d; d="$(ws_dir "$sid")"
  mkdir -p "$d/reports"
  printf 'SEED-READ-42\n' >"$d/seed.txt"
  printf 'STALE-DRAFT v1\n' >"$d/edit-target.txt"
  printf 'SEED-GLOB-99\n' >"$d/reports/glob-found.log"

  say "$sid" "Use your read tool to read the file seed.txt in your workspace and reply with the exact number you found."
  await_idle s2 "$sid" 180 || return 1

  say "$sid" "Use your edit tool to replace 'STALE-DRAFT v1' with 'SEED-EDIT-77' in the file edit-target.txt in your workspace. Then use your read tool to read edit-target.txt back and confirm the new text."
  await_idle s2 "$sid" 180 || return 1

  say "$sid" "Use your glob tool with the pattern 'reports/*.log' to list matching files in your workspace and reply with the filename you found."
  await_idle s2 "$sid" 180 || return 1

  # Three legs, each a sqlite3 count on tool_results (SCN-CONSENSUS-001):
  #   read  — a read tool result containing the seeded seed.txt content
  #   edit  — an edit tool call completed AND the seeded replacement visible in
  #           a tool result (the edit tool's own output reports the replacement
  #           count, not the content, so the content is proven via the read-back)
  #   glob  — a glob tool result containing the matched filename
  local leg_read leg_edit leg_edit_content leg_glob
  leg_read=$(sqlite3 "$DB" "SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id WHERE q.tool_name='read' AND r.is_error=0 AND r.output LIKE '%SEED-READ-42%';")
  leg_edit=$(sqlite3 "$DB" "SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id WHERE q.tool_name='edit' AND r.is_error=0;")
  leg_edit_content=$(sqlite3 "$DB" "SELECT count(*) FROM tool_results WHERE is_error=0 AND output LIKE '%SEED-EDIT-77%';")
  leg_glob=$(sqlite3 "$DB" "SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id WHERE q.tool_name='glob' AND r.is_error=0 AND r.output LIKE '%glob-found.log%';")
  if [ "$leg_read" -ge 1 ] && [ "$leg_edit" -ge 1 ] && [ "$leg_edit_content" -ge 1 ] && [ "$leg_glob" -ge 1 ]; then
    emit s2 pass "read+edit+glob tool_results: read=$leg_read edit_calls=$leg_edit edit_content=$leg_edit_content glob=$leg_glob"
    return 0
  fi
  emit s2 fail "tool_results proof failed: read=$leg_read edit_calls=$leg_edit edit_content=$leg_edit_content glob=$leg_glob"
  return 1
}

s3_bash() {
  local sid; sid=$(new_session "Run a shell command") || { emit s3 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: printf SCN-BASH > scn3.out ; then report DONE."
  await_idle s3 "$sid" 180 || return 1
  local f; f="$(ws_dir "$sid")/scn3.out"
  [ -f "$f" ] && grep -q SCN-BASH "$f" && { emit s3 pass "bash executed"; return 0; }
  emit s3 fail "bash output file missing: $f"
  return 1
}

s4_conversation_loop() {
  local sid; sid=$(new_session "Multi-turn tool use") || { emit s4 fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: echo -n ONE > turn1.txt"
  await_idle s4 "$sid" 180 || return 1
  say "$sid" "Now use your bash tool to run: echo -n TWO > turn2.txt"
  await_idle s4 "$sid" 180 || return 1
  local d; d=$(ws_dir "$sid")
  [ -f "$d/turn1.txt" ] && [ -f "$d/turn2.txt" ] && { emit s4 pass "two turns, two files"; return 0; }
  emit s4 fail "missing turn files in $d"
  return 1
}

s5_containment() {
  local sid; sid=$(new_session "Containment check") || { emit s5 fail "no session"; return 1; }
  local outside="$TMPDIR/consensus-workspaces/ESCAPED.txt"
  say "$sid" "Use your bash tool to run: echo -n ESCAPE > ../ESCAPED.txt ; report DONE either way."
  await_idle s5 "$sid" 180 || return 1
  if [ -f "$outside" ]; then emit s5 fail "ESCAPED the workspace: $outside"; return 1; fi
  emit s5 pass "no escape file outside the session workspace"
  return 0
}

s6_approve_resume() {
  # Attended approval gate (SCN-CONSENSUS-004): the one scenario that MUST run
  # with a human on the other side of the wall — never arm --auto-approve for
  # it (its own setup below re-gates the tool even if the flag is set).
  #
  # Scenario setup, not a product change: no enabled builtin defaults to
  # requires_approval=true, and the registry has no update API (POST /tools
  # 409s existing names), so gate exactly ONE tool — the write builtin — via
  # the same sqlite seam the --auto-approve seeding uses, before creating the
  # session.
  if ! sqlite3 "$DB" "UPDATE tools_registry SET requires_approval = 1 WHERE name = 'write' AND enabled = 1;"; then
    emit s6_approve_resume fail "could not seed requires_approval=1 on the write tool"
    return 1
  fi
  local gated; gated=$(sqlite3 "$DB" "SELECT count(*) FROM tools_registry WHERE name = 'write' AND enabled = 1 AND requires_approval = 1;")
  [ "$gated" -eq 1 ] || { emit s6_approve_resume fail "write tool not gated after seeding (rows=$gated)"; return 1; }

  local sid; sid=$(new_session "Approval gate check") || { emit s6_approve_resume fail "no session"; return 1; }
  say "$sid" "Use your write tool to write the file scn6.txt with the exact content HELLO-S6 (then report DONE)."

  # Leg 1: the session must PARK at the approval wall — never idle through it.
  wait_paused "$sid" 180 || { emit s6_approve_resume fail "session never reached paused (status=$(status_of "$sid"))"; return 1; }

  # Leg 2: a pending approvals row must exist for the session, targeting write.
  local appr pending aid
  appr=$(curl -s "http://127.0.0.1:$PORT/api/v1/sessions/$sid/approvals" -H "$H")
  pending=$(printf '%s' "$appr" | jq '[.[] | select(.status == "pending" and .target_tool == "write")] | length' 2>/dev/null)
  [ "${pending:-0}" -ge 1 ] || { emit s6_approve_resume fail "no pending approval row for write, sid=$sid: $appr"; return 1; }
  aid=$(printf '%s' "$appr" | jq -r '[.[] | select(.status == "pending" and .target_tool == "write")][0].id')

  # Leg 3: the human decision — POST the review with the admin bootstrap key.
  local reviewed
  reviewed=$(curl -s -X POST "http://127.0.0.1:$PORT/api/v1/approvals/$aid/review" -H "$H" \
    -H 'Content-Type: application/json' -d '{"decision":"approved"}')
  [ "$(printf '%s' "$reviewed" | jq -r '.status // empty')" = "approved" ] \
    || { emit s6_approve_resume fail "review POST did not approve $aid: $reviewed"; return 1; }

  # Wait for the linked successful result as well as terminal idle; idle alone
  # can be observed before the executor has run.
  local f count st i
  f="$(ws_dir "$sid")/scn6.txt"
  for i in $(seq 1 180); do
    st=$(status_of "$sid")
    count=$(sqlite3 "$DB" "SELECT count(*) FROM tool_requests q JOIN tool_results r ON r.request_id=q.id JOIN approval_requests a ON a.id=q.approval_request_id WHERE q.session_id='$sid' AND a.id='$aid' AND a.status='approved' AND q.tool_name='write' AND q.status='completed' AND r.is_error=0;")
    if [ "$st" = idle ] && [ "${count:-0}" -ge 1 ] && [ -f "$f" ] && grep -q HELLO-S6 "$f"; then
      emit s6_approve_resume pass "approved -> completed write request and successful result -> idle; wrote $f"
      return 0
    fi
    [ "$st" = failed ] && break
    sleep 1
  done
  emit s6_approve_resume fail "approved write did not complete with result and idle: status=$st successful_results=${count:-0} file=$f"
  return 1
}

s8_restart_durability() {
  # Restart durability (S8): SIGKILL the server mid-turn, restart the SAME
  # binary against the SAME SQLite DB and port, and prove the session is
  # re-armed and finishes the work it was killed on.
  #
  # Recovery after the crash is path-dependent — both paths are real:
  #
  #  - the killed turn had already queued a tool request but the dead
  #    process had not claimed it yet ('pending'): the restarted server's
  #    ToolExecutor claims pending requests straight from the table,
  #    re-runs it from the row's own parameters, and its hand-back
  #    (UPDATE ... WHERE status='tool_exec') flips the session to 'thinking'
  #    and wakes the planner for the continuation turn. Seconds.
  #  - the request was already 'executing' when the process died: nothing
  #    can claim it, so recovery goes through the claim-lease reaper
  #    (REVIEW-CONSENSUS-1 + DF-CONSENSUS-68): once the claim's heartbeat is
  #    older than the lease visibility timeout (5 minutes), the reaper hands
  #    the session back to 'tool_exec', requeues 'executing'→'pending', and
  #    wakes the executor. Minutes — the wait budget below covers it.
  #
  # Either way the assertion is the same: the request outstanding at kill time
  # is completed by the restarted process, and the file it was writing lands
  # on disk with the session idle. The kill lands during an executing-ish
  # window created by the 10s sleep; the runner reports which state it saw.
  #
  # The admin API key is stored hashed and is never reprinted, so the
  # restarted server logs no key: the runner keeps using the key captured on
  # the first boot (same DB, same key).
  local sc="s8_restart_durability"
  local marker="SCN-S8-RESTART-OK"
  local sid; sid=$(new_session "Restart durability check") || { emit "$sc" fail "no session"; return 1; }
  say "$sid" "Use your bash tool to run: sleep 10 && echo -n $marker > s8.txt (then report DONE)."

  # Kill mid-flight: only when the session sits in tool_exec with a bash
  # request still outstanding. Anything else (the model finished before we got
  # there, or the turn died) is a scenario setup failure, reported loudly.
  local i st rstatus
  st=""
  for i in $(seq 1 120); do
    st=$(status_of "$sid")
    rstatus=$(sqlite3 "$DB" "SELECT status FROM tool_requests WHERE session_id='$sid' AND status IN ('pending','executing') ORDER BY created_at ASC LIMIT 1;" 2>/dev/null)
    case "$st" in
      tool_exec) [ -n "$rstatus" ] && break ;;
      idle|completed) emit "$sc" fail "session settled before the mid-flight kill (status=$st) — nothing to resume"; return 1 ;;
      failed) emit "$sc" fail "session failed before the kill (sid=$sid)"; return 1 ;;
    esac
    sleep 0.5
  done
  [ "$st" = "tool_exec" ] && [ -n "$rstatus" ] \
    || { emit "$sc" fail "never caught the session mid-turn (status=$st outstanding=$rstatus)"; return 1; }

  local rid; rid=$(sqlite3 "$DB" "SELECT id FROM tool_requests WHERE session_id='$sid' AND status IN ('pending','executing') ORDER BY created_at ASC LIMIT 1;")
  [ -n "$rid" ] || { emit "$sc" fail "no outstanding tool request captured at kill time"; return 1; }

  # Crash, not shutdown: SIGKILL leaves no chance to settle the turn, and the
  # killed child tool process is never waited on.
  local dead_srv="$SRV_PID"
  kill -9 "$dead_srv" 2>/dev/null
  wait "$dead_srv" 2>/dev/null
  kill -0 "$dead_srv" 2>/dev/null \
    && { emit "$sc" fail "server pid $dead_srv still alive after SIGKILL"; return 1; }
  log "$sc: server (pid $dead_srv) killed mid-session: status=$st request=$rid/$rstatus"
  sleep 2 # let the listener release the port before the restart binds it

  # Restart the SAME binary against the SAME DB + port. Separate log so the
  # restarted server's recovery lines are greppable on their own.
  local LOG2="$STATE/serve2.log"
  CONSENSUS_API_KEY="$DK" DEEPSEEK_API_KEY="$DK" \
  CONSENSUS_DB_URL="sqlite://$DB" \
    "$BIN" serve --hostname 127.0.0.1 --port "$PORT" --log-level info >"$LOG2" 2>&1 &
  SRV_PID=$!
  local up=0
  for i in $(seq 1 60); do
    if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/openapi.json"; then up=1; break; fi
    kill -0 "$SRV_PID" 2>/dev/null || break
    sleep 1
  done
  [ "$up" -eq 1 ] \
    || { emit "$sc" fail "restarted server did not serve :$PORT (log tail: $(tail -c 400 "$LOG2" 2>/dev/null | tr '\n' ' '))"; return 1; }
  log "$sc: server restarted on :$PORT (pid $SRV_PID) against the same DB"

  # Lease expiry is wall-clock when recovery needs the reaper: allow 600s for
  # reap + re-execution + the planner continuation turn. The pending-request
  # path settles in seconds.
  await_idle "$sc" "$sid" 600 || return 1

  local reaps; reaps=$(grep -c "reclaimed stale session claim" "$LOG2" 2>/dev/null)
  log "$sc: claim-reaper reclaims in restarted log: ${reaps:-0} (0 = the killed request was still pending, so the restarted executor claimed it directly)"

  # Proof 1: the request that was outstanding when the process died reached a
  # successful result — only the restarted server could have moved it.
  # Proof 2: the requested file exists with the expected content.
  local req_end res_ok f
  req_end=$(sqlite3 "$DB" "SELECT status FROM tool_requests WHERE id='$rid';")
  res_ok=$(sqlite3 "$DB" "SELECT count(*) FROM tool_results WHERE request_id='$rid' AND is_error=0;")
  f="$(ws_dir "$sid")/s8.txt"
  if [ "$req_end" = "completed" ] && [ "${res_ok:-0}" -ge 1 ] && [ -f "$f" ] && grep -q "$marker" "$f"; then
    emit "$sc" pass "killed at status=$st request=$rid/$rstatus; restarted same DB; session idle; request completed; $f verified"
    return 0
  fi
  emit "$sc" fail "recovery incomplete: request_status=$req_end successful_results=${res_ok:-0} file=$f content=$(head -c 80 "$f" 2>/dev/null | tr '\n' ' ')"
  return 1
}

s13_concurrent_sessions() {
  # Concurrent sessions (S13): four sessions created up front, four message
  # POSTs dispatched back to back through the real HTTP API, and the server's
  # heartbeat loop (pollAndDispatch claims each active session into its own
  # goroutine) driving all four at the same time. Each session is told to
  # write its own marker file with its own distinct marker content.
  #
  # The proof is observed filesystem + database state, NOT ws_dir() path
  # arithmetic: four DISTINCT session ids read back from SQLite, four distinct
  # marker files, each file containing exactly its own marker, no session
  # workspace containing another session's marker (by file name or content),
  # and all four sessions reaching idle — or an explicitly named terminal
  # failure with a reason (paused fails fast, per the SCN-CONSENSUS-024
  # convention). Waits run in background subshells so one slow session cannot
  # serialize the other three.
  local sc="s13_concurrent_sessions"
  local -a sids=() markers=() pids=() files=() contents=() bad=()
  local n sid marker d f content st db_distinct

  # Leg 1: create all four sessions BEFORE dispatching or waiting, so the
  # server can claim them in parallel.
  for n in 1 2 3 4; do
    marker="SCN-S13-M$n"
    sid=$(new_session "Concurrent sessions check $n") || { emit "$sc" fail "no session (slot $n)"; return 1; }
    [ -n "$sid" ] || { emit "$sc" fail "empty session id (slot $n)"; return 1; }
    sids[$n]="$sid"
    markers[$n]="$marker"
  done

  # Leg 2: distinct session ids, read back from the DATABASE — not from the
  # runner's own array, which could only ever agree with itself.
  db_distinct=$(sqlite3 "$DB" "SELECT count(DISTINCT id) FROM sessions WHERE id IN ('${sids[1]}','${sids[2]}','${sids[3]}','${sids[4]}');")
  [ "${db_distinct:-0}" -eq 4 ] || { emit "$sc" fail "session ids not distinct in DB: ${db_distinct:-0} of 4"; return 1; }

  # Leg 3: dispatch all four instructions (four HTTP POSTs, back to back).
  for n in 1 2 3 4; do
    say "${sids[$n]}" "Use your bash tool to run: echo -n ${markers[$n]} > s13_marker_$n.txt (then report DONE)."
  done

  # Leg 4: wait for all four concurrently — one background waiter per session.
  for n in 1 2 3 4; do
    wait_terminal "${sids[$n]}" 300 >"$STATE/s13.status.$n" &
    pids[$n]=$!
  done
  for n in 1 2 3 4; do wait "${pids[$n]}"; done

  for n in 1 2 3 4; do
    st=$(cat "$STATE/s13.status.$n" 2>/dev/null)
    case "$st" in
      idle|completed) ;;
      paused) bad+=("sid=${sids[$n]} paused: approval wall (arm --auto-approve to ungate tools)") ;;
      failed) bad+=("sid=${sids[$n]} failed") ;;
      timeout:*) bad+=("sid=${sids[$n]} $st (non-idle)") ;;
      *) bad+=("sid=${sids[$n]} unknown status: ${st:-empty}") ;;
    esac
  done
  if [ "${#bad[@]}" -gt 0 ]; then
    emit "$sc" fail "not all sessions reached idle: ${bad[*]}"
    return 1
  fi

  # Leg 5: four distinct files, each holding exactly its own marker.
  for n in 1 2 3 4; do
    f="$(ws_dir "${sids[$n]}")/s13_marker_$n.txt"
    files[$n]="$f"
    if [ ! -f "$f" ]; then
      emit "$sc" fail "session ${sids[$n]}: marker file missing: $f"
      return 1
    fi
    content=$(cat "$f" 2>/dev/null)
    contents[$n]="$content"
    if [ "$content" != "${markers[$n]}" ]; then
      emit "$sc" fail "session ${sids[$n]}: $f content '$content' != own marker '${markers[$n]}'"
      return 1
    fi
  done
  if [ "$(printf '%s\n' "${files[@]}" | sort -u | wc -l)" -ne 4 ]; then
    emit "$sc" fail "marker files are not 4 distinct paths: ${files[*]}"
    return 1
  fi
  if [ "$(printf '%s\n' "${contents[@]}" | sort -u | wc -l)" -ne 4 ]; then
    emit "$sc" fail "marker contents are not 4 distinct values: ${contents[*]}"
    return 1
  fi

  # Leg 6: no cross-talk — no workspace holds another session's marker, under
  # either the marker FILE name or the marker CONTENT.
  for n in 1 2 3 4; do
    d="$(ws_dir "${sids[$n]}")"
    for m in 1 2 3 4; do
      [ "$m" = "$n" ] && continue
      if [ -f "$d/s13_marker_$m.txt" ]; then
        emit "$sc" fail "cross-talk: workspace of sid=${sids[$n]} holds session $m's marker FILE"
        return 1
      fi
      if grep -rqF -- "${markers[$m]}" "$d" 2>/dev/null; then
        emit "$sc" fail "cross-talk: workspace of sid=${sids[$n]} contains session $m's marker CONTENT"
        return 1
      fi
    done
  done

  emit "$sc" pass "4 sessions concurrent: ids distinct in DB, 4 distinct files, 4 distinct contents, no foreign marker in any workspace, all 4 idle"
  return 0
}

run_one() {
  local fn="$1"
  if ! scenario_selected "$fn"; then
    log "--- $fn (not selected)"
    return 0
  fi
  log "--- $fn"
  RAN=$((RAN+1))
  if "$fn"; then RESULTS+=("$fn=PASS"); else RESULTS+=("$fn=FAIL"); FAILS=$((FAILS+1)); fi
}

for fn in $ALL; do run_one "$fn"; done

# A selector that executed nothing must never present as green.
if [ "$RAN" -eq 0 ]; then
  emit selection fail "selector '$ONLY' matched no scenario (0 executed)"
  log "known: $(known_scenarios)"
  log "failures: n/a — no scenario was executed"
  exit 2
fi

log "=== summary ==="
printf '%s\n' "${RESULTS[@]}" >&2
log "scenarios run: $RAN"
log "failures: $FAILS"
[ "$FAILS" -eq 0 ]
