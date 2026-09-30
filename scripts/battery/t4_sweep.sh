#!/usr/bin/env bash
# t4_sweep.sh — SG-6. T4: non-vacuous grading over a REAL conversation.
#
# Quorum #1 subgoal tree §4 SG-6 (docs/testing/quorum-q1-merged-verdicts.md:97):
#   done-when: one real round trip yields session status idle with tokens>0 AND
#              >=1 agent_billing row with cost>0; the sweep driver prints
#              audited_targets and FAILS on a non-idle session.
#   falsifier: the driver again reports rows over zero conversations -> the
#              RUNNER is the defect; file the runner row, do not grade the product.
#
# Why this grading is non-vacuous (the 2026-09-24 baseline graded ZERO
# conversations: types={} / ('failed', 0, 0) / 0 billing rows):
#   - the audit runs AFTER one real round trip (create -> message -> settle);
#   - every number is parsed from a live HTTP response, never hand-typed;
#   - audited_targets enumerates WHERE each number comes from (route + table);
#   - a non-idle session is a mandated FAIL, not a footnote;
#   - the checks are red-proven: CONTROL_MODE=mock (session idles with zero
#     tokens and zero-cost billing) and CONTROL_MODE=badkey (LLM 401, session
#     never settles idle) must both produce verdict FAIL.
#
# Usage (from the repo root; requires go + jq + curl):
#   bash scripts/battery/t4_sweep.sh                    # the real SG-6 leg
#   CONTROL_MODE=mock    bash scripts/battery/t4_sweep.sh   # red control 1
#   CONTROL_MODE=badkey T4_DEEPSEEK_KEY=cs_invalid bash scripts/battery/t4_sweep.sh
#
# The real leg needs DEEPSEEK_API_KEY in the environment (SG-2 live-key
# preflight). Key VALUES are never echoed, never written to the evidence file.

set -uo pipefail

BATTERY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$BATTERY_DIR/run-env.sh"

CONTROL_MODE="${CONTROL_MODE:-}"
P="${P:-18492}"                      # do NOT collide with a dev server on 8090
BASE="http://127.0.0.1:$P"
BIN="${BIN:-/tmp/consensus-t4-bin}"
RUN_ROOT="${RUN_ROOT:-/tmp/t4-rerun}"
EV="$RUN_ROOT"
mkdir -p "$RUN_ROOT"

TIER="T4"
TARGET_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
GENERATED_AT="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

OUT="$EV/t4.json"
EXPECTED_VERDICT="PASS"
CONTROL_JSON="null"
# Key-source ladder (SG-2 live-key preflight): the canonical name first, then
# the host-local alias. Only the SOURCE NAME is recorded in the evidence —
# never the value.
LLM_KEY="${DEEPSEEK_API_KEY:-}"
KEY_SOURCE="env:DEEPSEEK_API_KEY"
if [ -z "$LLM_KEY" ] && [ -n "${T4_DEEPSEEK_KEY:-}" ]; then
  LLM_KEY="$T4_DEEPSEEK_KEY"
  KEY_SOURCE="env:T4_DEEPSEEK_KEY"
fi
case "$CONTROL_MODE" in
  mock)
    OUT="$EV/t4-control-mock.json"
    EXPECTED_VERDICT="FAIL"
    CONTROL_JSON='{"mode":"mock","why":"CONSENSUS_MOCK_LLM: the session can reach idle with zero tokens and zero-cost billing; proves the token and cost checks are not vacuous","expected_verdict":"FAIL"}'
    ;;
  badkey)
    OUT="$EV/t4-control-badkey.json"
    EXPECTED_VERDICT="FAIL"
    CONTROL_JSON='{"mode":"badkey","why":"an invalid provider key makes every LLM call fail, so the session never settles idle; proves the idle check is not vacuous","expected_verdict":"FAIL"}'
    LLM_KEY="${T4_DEEPSEEK_KEY:-cs-invalid-key-for-red-control}"
    ;;
  "") : ;;
  *) echo "[t4] unknown CONTROL_MODE '$CONTROL_MODE' (use mock|badkey|empty)" >&2; exit 64 ;;
esac

log() { echo "[t4] $*"; }
SRV_PID=""
cleanup() { [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null; }
trap cleanup EXIT

# redact any credential-shaped substring before it can reach the evidence file
redact() { sed -e 's/cs_ak_[a-f0-9]*/[REDACTED]/g' -e 's/cs_sk_[a-f0-9]*/[REDACTED]/g' \
               -e 's/cs_rk_[a-f0-9]*/[REDACTED]/g' -e 's/cs_wk_[a-f0-9]*/[REDACTED]/g'; }

# ---------- 0. preflight ----------
if [ -z "$LLM_KEY" ]; then
  jq -n --arg now "$GENERATED_AT" --arg sha "$TARGET_SHA" \
    '{schema:"consensus-tier-sweep-v2",tier:"T4",subgoal:"SG-6",generated_at:$now,target_sha:$sha,
      control:null,verdict:"BLOCKED",
      blocked:{reason:"no live LLM key in the environment; the real round trip cannot run. Fix: export DEEPSEEK_API_KEY (or T4_DEEPSEEK_KEY) per the SG-2 live-key preflight, then rerun."}}' > "$OUT"
  log "BLOCKED — no live LLM key; wrote $OUT"
  exit 2
fi

# ---------- 1. build ----------
if [ ! -x "$BIN" ] || [ "${T4_FORCE_BUILD:-0}" = "1" ]; then
  log "building $BIN"
  (cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/consensus) || { log "build FAILED"; exit 1; }
fi

# ---------- 2. start the server on a scratch DB ----------
DB_FILE="$RUN_ROOT/t4-$CONTROL_MODE.db"
DB_URL="sqlite://$DB_FILE?_journal_mode=WAL"
SRV_LOG="$RUN_ROOT/server-$CONTROL_MODE.log"
rm -f "$DB_FILE" "$DB_FILE-wal" "$DB_FILE-shm"

SERVER_ENV=(CONSENSUS_DB_URL="$DB_URL" CONSENSUS_PORT="$P" DEEPSEEK_API_KEY="$LLM_KEY" CONSENSUS_BOOTSTRAP_KEY_TTL_HOURS=1)
[ "$CONTROL_MODE" = "mock" ] && SERVER_ENV+=(CONSENSUS_LLM_PROVIDER=mock CONSENSUS_MOCK_LLM=1)

log "starting consensus on :$P (control=${CONTROL_MODE:-none})"
( cd "$REPO_ROOT" && env "${SERVER_ENV[@]}" "$BIN" ) >"$SRV_LOG" 2>&1 &
SRV_PID=$!

UP=0
for _ in $(seq 1 60); do
  CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$BASE/api/v1/health" 2>/dev/null || echo 000)"
  if [ "$CODE" = "200" ] || [ "$CODE" = "401" ]; then UP=1; break; fi
  sleep 0.5
done
if [ "$UP" != "1" ]; then
  log "FAIL: server did not come up on :$P — log tail:"; tail -5 "$SRV_LOG" | redact
  exit 1
fi

ADMIN_KEY="$(grep -o 'key=cs_ak_[a-f0-9]*' "$SRV_LOG" | head -1 | cut -d= -f2)"
if [ -z "$ADMIN_KEY" ]; then
  log "FAIL: no bootstrap admin key in server log — log tail:"; tail -5 "$SRV_LOG" | redact
  exit 1
fi

api() { # api METHOD PATH [extra curl args] -> body
  local m="$1" p="$2"; shift 2
  curl -sS --max-time 20 -X "$m" \
    -H "Authorization: Bearer $ADMIN_KEY" -H "Content-Type: application/json" \
    "$@" "$BASE$p"
}
api_code() { # api_code METHOD PATH [extra curl args] -> "code<TAB>body"
  local m="$1" p="$2" body code; shift 2
  body="$(curl -sS --max-time 20 -X "$m" \
    -H "Authorization: Bearer $ADMIN_KEY" -H "Content-Type: application/json" \
    -w $'\n%{http_code}' "$@" "$BASE$p")" || { echo "000	"; return; }
  code="$(printf '%s\n' "$body" | tail -1)"
  printf '%s\t%s' "$code" "$(printf '%s\n' "$body" | sed '$d')"
}

# ---------- 3. the ONE real round trip ----------
read -r CREATE_CODE CREATE_BODY <<EOF2
$(api_code POST /api/v1/sessions -d '{"agent_name":"consensus-t4-rerun","goal":"Answer the user next message in one short sentence. Plain text only."}')
EOF2
S4="$(jq -r '.id // empty' <<<"$CREATE_BODY" 2>/dev/null)"
if [ -z "$S4" ]; then
  log "FAIL: session create did not return an id (http $CREATE_CODE): $(redact <<<"$CREATE_BODY" | head -c 300)"
  exit 1
fi
log "session created: $S4 (http $CREATE_CODE)"

read -r MSG_CODE MSG_BODY <<EOF2
$(api_code POST "/api/v1/sessions/$S4/message" -d '{"content":"Please answer with exactly one short sentence: what are you? Do not call any tools.","type":"user_instruction"}')
EOF2
log "message sent (http $MSG_CODE): $(jq -r '.status // "?"' <<<"$MSG_BODY" 2>/dev/null)"

POLL_START="$(date +%s)"
STATUS="unknown"
for _ in $(seq 1 50); do
  SESS_RAW="$(api GET "/api/v1/sessions/$S4")"
  STATUS="$(jq -r '.status // "unknown"' <<<"$SESS_RAW" 2>/dev/null)"
  case "$STATUS" in
    idle) break ;;
    failed|paused) log "session reached terminal status '$STATUS' — stopping the poll"; break ;;
  esac
  sleep 3
done
POLL_SECONDS=$(( $(date +%s) - POLL_START ))
log "poll finished after ${POLL_SECONDS}s with status=$STATUS"

# settle window: the usage flush commits microseconds before the last idle
# write, but poll once more so the recorded numbers are the settled ones
for _ in $(seq 1 10); do
  SESS_RAW="$(api GET "/api/v1/sessions/$S4")"
  T_IN="$(jq -r '.tokens_used_in // 0' <<<"$SESS_RAW" 2>/dev/null)"
  T_OUT="$(jq -r '.tokens_used_out // 0' <<<"$SESS_RAW" 2>/dev/null)"
  [ "$(( T_IN + T_OUT ))" -gt 0 ] 2>/dev/null && break
  sleep 1
done
STATUS="$(jq -r '.status // "unknown"' <<<"$SESS_RAW" 2>/dev/null)"

SESS_JSON="$(jq '{status, model_id, iteration, tokens_used_in, tokens_used_out}' <<<"$SESS_RAW" 2>/dev/null || echo '{}')"

# ---------- 4. audit the named targets ----------
BILL_RAW="$(api GET "/api/v1/sessions/$S4/billing")"
BILL_JSON="$(jq '{billing_rows:(.entries|length),
                  billing_rows_cost_gt0:([.entries[]|select(.cost_usd>0)]|length),
                  total_cost_usd:.total_cost_usd,
                  total_prompt_tokens:.total_prompt_tokens,
                  total_completion_tokens:.total_completion_tokens,
                  rows_cost_gt0:[.entries[]|select(.cost_usd>0)|{iteration,model_id,prompt_tokens,completion_tokens,cost_usd}]}' \
                <<<"$BILL_RAW" 2>/dev/null || echo '{}')"

LEDGER_RAW="$(api GET "/api/v1/sessions/$S4/memory?limit=500")"
LEDGER_JSON="$(jq 'if type=="array"
                   then {total_rows:length, types:((group_by(.type)|map({(.[0].type):length})|add) // {})}
                   else {total_rows:0, types:{}, unexpected_shape:(.)} end' \
                <<<"$LEDGER_RAW" 2>/dev/null || echo '{total_rows:0,types:{}}')"

AUDITED_TARGETS_JSON="[
 {\"id\":\"session\",\"kind\":\"sessions\",\"table\":\"sessions\",\"location\":\"GET $BASE/api/v1/sessions/$S4\",\"why\":\"status + tokens_used_in/out after the round trip\"},
 {\"id\":\"ledger\",\"kind\":\"memory_events\",\"table\":\"memory_events\",\"location\":\"GET $BASE/api/v1/sessions/$S4/memory?limit=500\",\"why\":\"the conversation actually persisted (user_message + assistant text_block)\"},
 {\"id\":\"billing\",\"kind\":\"agent_billing\",\"table\":\"agent_billing\",\"location\":\"GET $BASE/api/v1/sessions/$S4/billing\",\"why\":\"per-iteration cost rows written by the harness usage flush\"}
]"

ROUND_JSON="$(jq -n --arg create "$CREATE_CODE" --arg msg "$MSG_CODE" --arg s4 "$S4" \
                   --arg status "$STATUS" --argjson secs "$POLL_SECONDS" \
                   '{session_id:$s4, create_http_status:($create|tonumber), message_http_status:($msg|tonumber), poll_seconds:$secs, final_status:$status}')"

# ---------- 5. checks (mandated flags mark the SG-6 done-when rows) ----------
mk_check() { # id expected observed pass mandated
  jq -n --arg id "$1" --arg expected "$2" --arg observed "$3" \
        --argjson pass "$4" --argjson mandated "$5" \
        '{id:$id, expected:$expected, observed:$observed,
          verdict:(if $pass then "PASS" else "FAIL" end), mandated:$mandated}'
}
C1_PASS="$([ "$STATUS" = "idle" ] && echo true || echo false)"
C2_PASS="$(jq -r 'if ((.tokens_used_in // 0) + (.tokens_used_out // 0)) > 0 then "true" else "false" end' <<<"$SESS_JSON")"
C3_PASS="$(jq -r 'if (.billing_rows_cost_gt0 // 0) >= 1 then "true" else "false" end' <<<"$BILL_JSON")"
C4_PASS="$(jq -r 'if (.total_rows // 0) >= 1 then "true" else "false" end' <<<"$LEDGER_JSON")"

CHECKS_JSON="[$(mk_check "session-idle" "status == idle after the round trip" "$STATUS" "$C1_PASS" true),
              $(mk_check "session-tokens" "tokens_used_in + tokens_used_out > 0" "$(jq -r '"in=\(.tokens_used_in) out=\(.tokens_used_out)"' <<<"$SESS_JSON")" "$C2_PASS" true),
              $(mk_check "billing-rows" ">=1 agent_billing row with cost_usd > 0" "$(jq -r '"\(.billing_rows) rows, \(.billing_rows_cost_gt0) with cost>0, total=$\(.total_cost_usd)"' <<<"$BILL_JSON")" "$C3_PASS" true),
              $(mk_check "ledger-types" "memory_events persisted for the conversation (non-vacuity: rows > 0)" "$(jq -r '"\(.total_rows) rows types=\(.types|tojson)"' <<<"$LEDGER_JSON")" "$C4_PASS" false)]"

# ---------- 6. falsifier ----------
FALSIFIER_JSON='{"triggered":false}'
if [ "$(jq -r '.billing_rows // 0' <<<"$BILL_JSON")" = "0" ] && [ "$(jq -r '.total_rows // 0' <<<"$LEDGER_JSON")" = "0" ]; then
  FALSIFIER_JSON='{"triggered":true,"reading":"rows over zero conversations: agent_billing=0 AND memory_events=0 after a real round trip. Per SG-6 the RUNNER is the defect — file the runner row; do NOT grade the product on this output."}'
fi

LOG_TAIL="$(tail -c 1200 "$SRV_LOG" | redact)"

# ---------- 7. verdict + evidence ----------
jq -n \
  --arg schema "consensus-tier-sweep-v2" --arg tier "$TIER" --arg subgoal "SG-6" \
  --arg run_id "$RUN_ID" --arg generated_at "$GENERATED_AT" --arg target_sha "$TARGET_SHA" \
  --arg base "$BASE" --arg out "$OUT" --arg key_source "$KEY_SOURCE" \
  --argjson audited_targets "$AUDITED_TARGETS_JSON" \
  --argjson round_trip "$ROUND_JSON" \
  --argjson session "$SESS_JSON" \
  --argjson billing "$BILL_JSON" \
  --argjson ledger "$LEDGER_JSON" \
  --argjson checks "$CHECKS_JSON" \
  --argjson control "$CONTROL_JSON" \
  --argjson falsifier "$FALSIFIER_JSON" \
  --arg server_log_tail "$LOG_TAIL" \
  '{schema:$schema, tier:$tier, subgoal:$subgoal, run_id:$run_id, generated_at:$generated_at,
    target_sha:$target_sha, base_url:$base, llm_key_source:$key_source, control:$control,
    audited_targets:$audited_targets,
    round_trip:$round_trip, session:$session, billing:$billing, ledger:$ledger,
    checks:$checks,
    verdict:(if ([$checks[] | select(.mandated == true and .verdict != "PASS")] | length) > 0
             then "FAIL" else "PASS" end),
    falsifier:$falsifier,
    server_log_tail:$server_log_tail,
    recompute:("bash scripts/battery/t4_sweep.sh  # from the repo root, with DEEPSEEK_API_KEY set; fresh scratch DB + random session each run")}' > "$OUT.tmp" \
  && mv "$OUT.tmp" "$OUT"

VERDICT="$(jq -r .verdict "$OUT")"

# done-when: the driver PRINTS audited_targets
echo "audited_targets:"
jq -r '.audited_targets[] | "  - \(.id): \(.location) (table \(.table))"' "$OUT"
jq -r '.checks[] | "[\(.verdict)] \(.id): observed=\(.observed)"' "$OUT"
log "verdict=$VERDICT (expected=$EXPECTED_VERDICT) -> $OUT"

[ "$VERDICT" = "$EXPECTED_VERDICT" ] && exit 0 || exit 1
