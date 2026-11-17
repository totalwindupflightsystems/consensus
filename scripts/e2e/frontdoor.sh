#!/usr/bin/env bash
# Consensus M0 frontdoor: real-provider write/read plus DF-68 recovery proof.
#
# One command (from any checkout): make test-e2e-frontdoor
#
# This is intentionally NOT the mock smoke test. The script builds this
# checkout into a scratch directory, boots a fresh DB on a loopback-only
# scratch port, asks a real provider to write and read a file, interrupts the
# executing claim, restarts with an expired lease, and verifies durable
# completion. A mock response, paused session, stub result, abandoned tool
# request, missing/wrong file, or missing provider billing is a hard failure.
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODEL="${FRONTDOOR_LLM_MODEL:-deepseek-flash}"
BASE_URL="${FRONTDOOR_LLM_BASE_URL:-https://api.deepseek.com/v1}"
TIMEOUT_SECONDS="${FRONTDOOR_TIMEOUT_SECONDS:-420}"
KEEP_ARTIFACTS="${FRONTDOOR_KEEP_ARTIFACTS:-0}"
EXPECTED_CONTENT="CONSENSUS-FRONTDOOR-OK"
TARGET_NAME="df_probe.txt"
SERVER_PID=""
SERVER_START=""
ADMIN_KEY=""
SESSION_ID=""
ORIGINAL_TOOL_ID=""
RUN_SUCCEEDED=0

log() { printf '[frontdoor] %s\n' "$*"; }
fail() { log "FAIL: $*"; exit 1; }

for cmd in go curl jq python3 awk grep mkfifo sha256sum cmp; do
    command -v "$cmd" >/dev/null 2>&1 || fail "missing prerequisite: $cmd"
done
[[ "$TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]] || fail "FRONTDOOR_TIMEOUT_SECONDS must be a positive integer"

# Credentials may be supplied directly or through the standard Hermes env
# file. Never print the value. There is deliberately no mock-provider fallback.
API_KEY="${FRONTDOOR_API_KEY:-${DEEPSEEK_API_KEY:-${CONSENSUS_API_KEY:-}}}"
if [[ -z "$API_KEY" && -r "$HOME/.hermes/.env" ]]; then
    API_KEY="$(awk -F= '/^(export )?DEEPSEEK_API_KEY=/{sub(/^(export )?DEEPSEEK_API_KEY=/, ""); print; exit}' "$HOME/.hermes/.env")"
    API_KEY="${API_KEY%$'\r'}"
    API_KEY="${API_KEY#${API_KEY%%[![:space:]]*}}"
    API_KEY="${API_KEY%${API_KEY##*[![:space:]]}}"
    if [[ ${#API_KEY} -ge 2 && ${API_KEY:0:1} == '"' && ${API_KEY: -1} == '"' ]]; then
        API_KEY="${API_KEY:1:${#API_KEY}-2}"
    elif [[ ${#API_KEY} -ge 2 && ${API_KEY:0:1} == "'" && ${API_KEY: -1} == "'" ]]; then
        API_KEY="${API_KEY:1:${#API_KEY}-2}"
    fi
fi
[[ -n "$API_KEY" ]] || fail "set DEEPSEEK_API_KEY (or FRONTDOOR_API_KEY); the M0 gate requires a real provider"

ARTIFACTS="$(mktemp -d "${TMPDIR:-/tmp}/consensus-frontdoor.XXXXXX")"
BIN="$ARTIFACTS/consensus"
DB="$ARTIFACTS/consensus.db"
CONFIG="$ARTIFACTS/consensus.yaml"
LOG_FILE="$ARTIFACTS/serve.log"
TRANSITIONS="$ARTIFACTS/status-transitions.jsonl"
WORKSPACES="$ARTIFACTS/workspaces"
EXPECTED_FILE="$ARTIFACTS/expected-content"
PORT="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
BASE="http://127.0.0.1:$PORT"
printf %s "$EXPECTED_CONTENT" > "$EXPECTED_FILE"
: > "$LOG_FILE"
: > "$TRANSITIONS"
mkdir -p "$WORKSPACES"

cat > "$CONFIG" <<EOF
database:
  url: sqlite://$DB
  max_open_conns: 4
server:
  hostname: 127.0.0.1
  port: $PORT
  pprof_addr: ""
compression:
  enabled: false
logging:
  level: info
EOF

process_is_ours() {
    [[ -n "$SERVER_PID" && -n "$SERVER_START" && -r "/proc/$SERVER_PID/stat" ]] || return 1
    [[ "$(awk '{print $22}' "/proc/$SERVER_PID/stat" 2>/dev/null)" == "$SERVER_START" ]]
}

port_is_closed() {
    python3 - "$PORT" <<'PY'
import socket
import sys
with socket.socket() as sock:
    sock.settimeout(0.25)
    raise SystemExit(1 if sock.connect_ex(("127.0.0.1", int(sys.argv[1]))) == 0 else 0)
PY
}

stop_server() {
    local signal="${1:-TERM}"
    local i
    if process_is_ours; then
        kill "-$signal" "$SERVER_PID" 2>/dev/null || true
        for i in $(seq 1 50); do
            process_is_ours || break
            sleep 0.1
        done
        if process_is_ours; then
            kill -KILL "$SERVER_PID" 2>/dev/null || true
        fi
    fi
    if [[ -n "$SERVER_PID" ]]; then
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    SERVER_PID=""
    SERVER_START=""
}

cleanup() {
    local rc=$?
    local cleanup_failed=0
    trap - EXIT
    set +e
    stop_server KILL
    if ! port_is_closed; then
        log "FAIL: scratch port $PORT still has a listener after teardown"
        cleanup_failed=1
    fi
    if [[ "$rc" -eq 0 && "$cleanup_failed" -ne 0 ]]; then
        rc=1
    fi
    if [[ "$RUN_SUCCEEDED" -eq 1 && "$rc" -eq 0 && "$KEEP_ARTIFACTS" != 1 ]]; then
        rm -rf "$ARTIFACTS"
    else
        log "artifacts: $ARTIFACTS"
    fi
    exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

redacted_log_tail() {
    tail -n 60 "$LOG_FILE" | sed -E \
        -e 's/sk_consensus_[A-Za-z0-9_-]+/[REDACTED_CONSENSUS_KEY]/g' \
        -e 's/cs_ak_[A-Za-z0-9_-]+/[REDACTED_CONSENSUS_KEY]/g' \
        -e 's/sk-[A-Za-z0-9_-]+/[REDACTED_PROVIDER_KEY]/g'
}

start_server() {
    local deadline key code
    log "starting real-provider server on 127.0.0.1:$PORT (model $MODEL)"
    env -u CONSENSUS_API_KEY -u CONSENSUS_ALLOW_MOCK_PROVIDER \
        CONSENSUS_MOCK_LLM=0 \
        CONSENSUS_DB_URL="sqlite://$DB" \
        CONSENSUS_TOOL_WORKSPACE="$WORKSPACES" \
        DEEPSEEK_API_KEY="$API_KEY" \
        CONSENSUS_LLM_PROVIDER=openai \
        CONSENSUS_LLM_MODEL="$MODEL" \
        CONSENSUS_LLM_BASE_URL="$BASE_URL" \
        "$BIN" serve --config "$CONFIG" --port "$PORT" >> "$LOG_FILE" 2>&1 &
    SERVER_PID=$!
    [[ -r "/proc/$SERVER_PID/stat" ]] || fail "server did not start"
    SERVER_START="$(awk '{print $22}' "/proc/$SERVER_PID/stat")"
    deadline=$((SECONDS + 45))
    while (( SECONDS < deadline )); do
        process_is_ours || { redacted_log_tail; fail "server exited during startup"; }
        key="$(grep -oE 'key=(sk_consensus|cs_ak)_[A-Za-z0-9_-]+' "$LOG_FILE" | tail -n 1 | cut -d= -f2 || true)"
        if [[ -n "$key" ]]; then
            code="$(curl --silent --max-time 2 -o "$ARTIFACTS/openapi.json" -w '%{http_code}' \
                -H "X-API-Key: $key" "$BASE/openapi.json" || true)"
            if [[ "$code" == 200 ]]; then
                ADMIN_KEY="$key"
                curl --silent --show-error --fail --max-time 5 \
                    -o "$ARTIFACTS/health.json" "$BASE/api/v1/health" \
                    || fail "health endpoint failed after openapi became ready"
                jq -e '.status == "ok"' "$ARTIFACTS/health.json" >/dev/null \
                    || fail "health endpoint returned a non-ok body"
                return 0
            fi
        fi
        sleep 0.2
    done
    redacted_log_tail
    fail "server did not become ready within 45s"
}

api_call() {
    local method="$1" path="$2" key="$3" body="$4" output="$5" code
    local args=(--silent --show-error --max-time 20 -o "$output" -w '%{http_code}'
        -X "$method" -H "X-API-Key: $key")
    if [[ -n "$body" ]]; then
        args+=(-H 'Content-Type: application/json' --data "$body")
    fi
    code="$(curl "${args[@]}" "$BASE$path")" || fail "HTTP $method $path transport error"
    printf '%s' "$code"
}

record_status() {
    local status="$1" iteration="$2"
    jq -cn --arg status "$status" --argjson iteration "$iteration" \
        '{elapsed_seconds: '"$SECONDS"', status: $status, iteration: $iteration}' >> "$TRANSITIONS"
    log "session $SESSION_ID status=$status iteration=$iteration elapsed=${SECONDS}s"
}

latest_write_request() {
    python3 - "$DB" "$SESSION_ID" <<'PY'
import sqlite3
import sys
conn = sqlite3.connect(sys.argv[1], timeout=0.2)
row = conn.execute(
    "SELECT id, status FROM tool_requests WHERE session_id=? AND tool_name='write' ORDER BY id LIMIT 1",
    (sys.argv[2],),
).fetchone()
print(f"{row[0]}|{row[1]}" if row else "|")
PY
}

session_snapshot() {
    local code
    code="$(api_call GET "/api/v1/sessions/$SESSION_ID" "$ADMIN_KEY" "" "$ARTIFACTS/session.json")"
    [[ "$code" == 200 ]] || fail "GET session returned HTTP $code"
    jq -r '[.status, (.iteration // 0)] | @tsv' "$ARTIFACTS/session.json"
}

log "building $ROOT into scratch binary"
(cd "$ROOT" && go build -o "$BIN" ./cmd/consensus)
start_server
if grep -qi 'mock llm client initialized' "$LOG_FILE"; then
    fail "mock provider was initialized; this gate requires real provider traffic"
fi

CREATE_BODY="$(jq -cn \
    --arg model "$MODEL" \
    --arg goal "M0 frontdoor: use the real filesystem tools to write and read one exact file." \
    '{agent_name:"frontdoor-m0", goal:$goal, model_id:$model, context_budget:64000, budget_limit_cents:200000}')"
CODE="$(api_call POST /api/v1/sessions "$ADMIN_KEY" "$CREATE_BODY" "$ARTIFACTS/create.json")"
[[ "$CODE" == 201 ]] || { jq -c . "$ARTIFACTS/create.json" 2>/dev/null || true; fail "create session returned HTTP $CODE"; }
SESSION_ID="$(jq -er '.id' "$ARTIFACTS/create.json")"
SESSION_KEY="$(jq -er '.api_key' "$ARTIFACTS/create.json")"
CODE="$(api_call GET "/api/v1/sessions/$SESSION_ID" "$ADMIN_KEY" "" "$ARTIFACTS/session.json")"
[[ "$CODE" == 200 ]] || fail "initial session readback returned HTTP $CODE"
jq -e --arg model "$MODEL" '.model_id == $model' "$ARTIFACTS/session.json" >/dev/null \
    || fail "session model differs from requested real model"
log "created session $SESSION_ID with real model $MODEL"

# A FIFO makes the provider-issued native write remain in `executing` until we
# interrupt the process. Without this deterministic crash window a fast write
# can finish before the gate can exercise DF-68. It is removed before restart,
# so successful recovery must create a regular file through the same request.
SESSION_WORKSPACE="$WORKSPACES"
mkdir -p "$SESSION_WORKSPACE"
[[ ! -e "$SESSION_WORKSPACE/$TARGET_NAME" ]] || fail "fresh workspace already contains target"
mkfifo "$SESSION_WORKSPACE/$TARGET_NAME"

PROMPT="Use the write tool now with filename '$TARGET_NAME' and content exactly '$EXPECTED_CONTENT' (no trailing newline). After the write completes, use the read tool on '$TARGET_NAME' and return its exact contents as final_message. Finish with memory_state_changes set to the empty list []; do not INSERT into audit_logs or any other table. Do not merely describe the file before using the tools and do not call list_sessions."
MESSAGE_BODY="$(jq -cn --arg content "$PROMPT" '{type:"user_instruction", content:$content}')"
CODE="$(api_call POST "/api/v1/sessions/$SESSION_ID/message" "$SESSION_KEY" "$MESSAGE_BODY" "$ARTIFACTS/message.json")"
[[ "$CODE" == 200 || "$CODE" == 201 || "$CODE" == 202 ]] || { jq -c . "$ARTIFACTS/message.json" 2>/dev/null || true; fail "post message returned HTTP $CODE"; }

log "waiting for the provider-issued write to enter executing"
DEADLINE=$((SECONDS + TIMEOUT_SECONDS))
LAST_STATUS=""
while (( SECONDS < DEADLINE )); do
    SNAPSHOT="$(session_snapshot)"
    STATUS="${SNAPSHOT%%$'\t'*}"
    ITERATION="${SNAPSHOT##*$'\t'}"
    if [[ "$STATUS" != "$LAST_STATUS" ]]; then
        record_status "$STATUS" "$ITERATION"
        LAST_STATUS="$STATUS"
    fi
    [[ "$STATUS" != paused && "$STATUS" != failed ]] || fail "session entered terminal error state $STATUS before crash injection"
    [[ "$STATUS" != idle && "$STATUS" != completed ]] || fail "session reached $STATUS without issuing write"

    REQUEST="$(latest_write_request)"
    REQUEST_ID="${REQUEST%%|*}"
    REQUEST_STATUS="${REQUEST##*|}"
    if [[ -n "$REQUEST_ID" && "$REQUEST_STATUS" == executing ]]; then
        ORIGINAL_TOOL_ID="$REQUEST_ID"
        break
    fi
    [[ "$REQUEST_STATUS" != completed ]] || fail "write completed before crash injection; FIFO did not hold the claim"
    sleep 0.1
done
[[ -n "$ORIGINAL_TOOL_ID" ]] || fail "no executing provider-issued write within ${TIMEOUT_SECONDS}s"
log "interrupting server with original write request $ORIGINAL_TOOL_ID in executing"
stop_server KILL
port_is_closed || fail "server port remained open after SIGKILL"
rm -f "$SESSION_WORKSPACE/$TARGET_NAME"

# Age only the lease timestamps after the process is dead. The request itself
# remains the provider-created row in `executing`; DF-68 must requeue THAT row.
python3 - "$DB" "$SESSION_ID" "$ORIGINAL_TOOL_ID" <<'PY'
import datetime
import sqlite3
import sys
old = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(minutes=10)).isoformat()
conn = sqlite3.connect(sys.argv[1])
row = conn.execute("SELECT status FROM tool_requests WHERE id=? AND session_id=?", (sys.argv[3], sys.argv[2])).fetchone()
assert row and row[0] == "executing", f"unexpected interrupted request state: {row}"
conn.execute("UPDATE sessions SET heartbeat_at=? WHERE id=?", (old, sys.argv[2]))
conn.execute("UPDATE tool_requests SET executed_at=? WHERE id=?", (old, sys.argv[3]))
conn.commit()
PY

start_server
log "restarted with an expired claim; waiting for recovery and readback"
DEADLINE=$((SECONDS + TIMEOUT_SECONDS))
while (( SECONDS < DEADLINE )); do
    SNAPSHOT="$(session_snapshot)"
    STATUS="${SNAPSHOT%%$'\t'*}"
    ITERATION="${SNAPSHOT##*$'\t'}"
    if [[ "$STATUS" != "$LAST_STATUS" ]]; then
        record_status "$STATUS" "$ITERATION"
        LAST_STATUS="$STATUS"
    fi
    case "$STATUS" in
        paused|failed)
            jq -c '{status,last_message,iteration}' "$ARTIFACTS/session.json"
            fail "session entered $STATUS during recovery"
            ;;
        idle|completed)
            break
            ;;
    esac
    sleep 1
done
[[ "$STATUS" == idle || "$STATUS" == completed ]] || fail "session did not reach terminal state within ${TIMEOUT_SECONDS}s (last=$STATUS)"
jq -e '.last_message != null and (.last_message | length > 0)' "$ARTIFACTS/session.json" >/dev/null \
    || fail "terminal session has no durable assistant message"

TARGET_FILE="$SESSION_WORKSPACE/$TARGET_NAME"
[[ -f "$TARGET_FILE" && ! -p "$TARGET_FILE" ]] || fail "recovered target is absent or not a regular file"
cmp -s "$EXPECTED_FILE" "$TARGET_FILE" || {
    log "expected bytes: $(sha256sum "$EXPECTED_FILE" | awk '{print $1}')"
    log "actual bytes:   $(sha256sum "$TARGET_FILE" | awk '{print $1}')"
    fail "target content differs from requested bytes"
}

# Durable DB assertions bind the visible file to the ORIGINAL interrupted
# request. If DF-68 is disabled, that row remains executing even if a model
# tries to hide the regression by issuing a duplicate write.
python3 - "$DB" "$SESSION_ID" "$ORIGINAL_TOOL_ID" "$MODEL" "$ARTIFACTS/db-assertions.json" <<'PY'
import json
import sqlite3
import sys

db_path, session_id, original_id, expected_model, output_path = sys.argv[1:]
conn = sqlite3.connect(db_path)
conn.row_factory = sqlite3.Row
rows = conn.execute(
    """SELECT tr.id, tr.tool_name, tr.status, tr.parameters,
              COALESCE(res.is_error, 0) AS is_error, COALESCE(res.output, '') AS output
       FROM tool_requests tr
       LEFT JOIN tool_results res ON res.request_id = tr.id
       WHERE tr.session_id=? ORDER BY tr.id""",
    (session_id,),
).fetchall()
original = [r for r in rows if str(r["id"]) == str(original_id)]
assert len(original) == 1, f"original interrupted request missing: {original_id}"
assert original[0]["status"] == "completed", f"DF-68 did not complete original request: {dict(original[0])}"
assert original[0]["tool_name"] == "write" and not original[0]["is_error"], f"original write failed: {dict(original[0])}"
nonterminal = [dict(r) for r in rows if r["status"] not in ("completed", "failed")]
assert not nonterminal, f"abandoned tool requests: {nonterminal}"
errors = [dict(r) for r in rows if r["is_error"] or r["status"] == "failed"]
assert not errors, f"tool errors: {errors}"
writes = [r for r in rows if r["tool_name"] == "write" and r["status"] == "completed" and not r["is_error"]]
reads = [r for r in rows if r["tool_name"] == "read" and r["status"] == "completed" and not r["is_error"]]
assert writes, "no successful write tool execution"
assert reads, "write succeeded but the provider never completed readback"
stub_markers = ("go_native stub", "not implemented", "placeholder output", "mock tool")
stubs = [dict(r) for r in rows if any(marker in r["output"].lower() for marker in stub_markers)]
assert not stubs, f"stub-shaped tool results: {stubs}"
billing = conn.execute(
    """SELECT model_id, COUNT(*) AS calls, COALESCE(SUM(prompt_tokens),0) AS input_tokens,
              COALESCE(SUM(completion_tokens),0) AS output_tokens
       FROM agent_billing WHERE session_id=? GROUP BY model_id""",
    (session_id,),
).fetchall()
input_tokens = sum(int(r["input_tokens"]) for r in billing)
output_tokens = sum(int(r["output_tokens"]) for r in billing)
assert billing and input_tokens + output_tokens > 0, "no real provider token billing recorded"
assert any(r["model_id"] == expected_model for r in billing), f"billing model mismatch: {[dict(r) for r in billing]}"
commits = conn.execute("SELECT COUNT(*) FROM iteration_commits WHERE session_id=?", (session_id,)).fetchone()[0]
memory_events = conn.execute("SELECT COUNT(*) FROM memory_events WHERE session_id=?", (session_id,)).fetchone()[0]
assert memory_events > 0, "no durable memory events"
summary = {
    "original_tool_id": int(original_id),
    "original_tool_status": original[0]["status"],
    "successful_writes": len(writes),
    "successful_reads": len(reads),
    "tool_requests": len(rows),
    "iteration_commits": commits,
    "memory_events": memory_events,
    "provider_calls": sum(int(r["calls"]) for r in billing),
    "input_tokens": input_tokens,
    "output_tokens": output_tokens,
    "billing_models": [r["model_id"] for r in billing],
}
with open(output_path, "w", encoding="utf-8") as handle:
    json.dump(summary, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(json.dumps(summary, sort_keys=True))
PY

CONTENT_SHA="$(sha256sum "$TARGET_FILE" | awk '{print $1}')"
SOURCE_COMMIT="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf unknown)"
jq -n \
    --arg session_id "$SESSION_ID" \
    --arg model "$MODEL" \
    --arg base_url "$BASE_URL" \
    --arg content_sha256 "$CONTENT_SHA" \
    --arg source_commit "$SOURCE_COMMIT" \
    --slurpfile db "$ARTIFACTS/db-assertions.json" \
    '{result:"PASS", gate:"real-provider-write-read-df68-recovery", session_id:$session_id,
      model:$model, base_url:$base_url, file:"'"$TARGET_NAME"'", content_sha256:$content_sha256,
      source_commit:$source_commit, assertions:$db[0]}' > "$ARTIFACTS/result.json"

RUN_SUCCEEDED=1
log "PASS: original interrupted write completed; readback, exact bytes, terminal state, and real billing verified"
jq -c . "$ARTIFACTS/result.json"
