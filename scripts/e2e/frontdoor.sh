#!/usr/bin/env bash
# Consensus front-door battery: build, boot, mutate, read back, and tear down.
# ch:trace row=WIRE-CONSENSUS-1 spec=none:front-door-wiring wave=none:serial test=scripts/e2e/frontdoor.sh doc=none:not-doc-task prompt=/home/kara/.hermes/cache/scratch/consensus-foreman-2026-11-15-18-06-03/brief-WIRE-CONSENSUS-1.txt evidence=none:runtime-output-captured-separately witness=local-command verdict=none:foreman-judge commit=none:pending

set -Eeuo pipefail

fail() {
    printf 'frontdoor: FAIL: %s\n' "$*" >&2
    exit 1
}

need() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

for command_name in awk bash curl go grep jq kill mktemp rm sed seq sleep ss; do
    need "$command_name"
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# The HTTP mutation below must exercise the production wake path. A source
# guard makes removal of that wiring fail immediately instead of waiting for a
# long heartbeat interval and accidentally reporting a false green result.
grep -Fq 'whStore.SetWake(h.RequestWake)' cmd/consensus/main.go || \
    fail 'production message wake wiring is absent: expected whStore.SetWake(h.RequestWake)'

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/consensus-frontdoor.XXXXXX")"
DB_PATH="$TMP_DIR/consensus.db"
CONFIG_PATH="$TMP_DIR/consensus.yaml"
LOG_PATH="$TMP_DIR/server.log"
HEALTH_RESPONSE="$TMP_DIR/health.json"
SESSION_RESPONSE="$TMP_DIR/session.json"
SESSION_READBACK="$TMP_DIR/session-readback.json"
MESSAGE_RESPONSE="$TMP_DIR/message.json"
SERVER_PID=""
PORT=""

cleanup() {
    local status=$?
    if [[ -n "$SERVER_PID" ]]; then
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            kill -TERM "$SERVER_PID" 2>/dev/null || true
            for _ in $(seq 1 100); do
                kill -0 "$SERVER_PID" 2>/dev/null || break
                sleep 0.1
            done
            if kill -0 "$SERVER_PID" 2>/dev/null; then
                kill -KILL "$SERVER_PID" 2>/dev/null || true
            fi
        fi
        wait "$SERVER_PID" 2>/dev/null || true
    fi

    if [[ -n "$PORT" ]] && ss -H -ltn | awk -v port=":$PORT" '$4 ~ port "$" {found=1} END {exit found ? 0 : 1}'; then
        printf 'frontdoor: FAIL: port %s is still listening after PID %s teardown\n' "$PORT" "$SERVER_PID" >&2
        status=1
    elif [[ -n "$PORT" ]]; then
        printf 'frontdoor: port %s is free after PID %s teardown\n' "$PORT" "${SERVER_PID:-unknown}"
    fi

    if [[ "$status" -ne 0 && -f "$LOG_PATH" ]]; then
        printf 'frontdoor: server output:\n' >&2
        sed -n '1,240p' "$LOG_PATH" >&2 || true
    fi
    rm -rf "$TMP_DIR"
    exit "$status"
}
trap cleanup EXIT INT TERM

# Keep the server isolated from developer configuration and disable pprof so
# the only listener under test is the front-door HTTP API.
printf '%s\n' \
    'server:' \
    '  hostname: 127.0.0.1' \
    '  pprof_addr: ""' \
    'llm:' \
    '  provider: mock' \
    '  default_model: mock-model' \
    'harness:' \
    '  heartbeat_interval_seconds: 60' \
    '  planning_timeout_seconds: 15' \
    'compression:' \
    '  enabled: false' \
    'database:' \
    "  url: sqlite://$DB_PATH" \
    '  max_open_conns: 4' > "$CONFIG_PATH"

# Pick a currently unused TCP port, then re-check it before starting the
# server. All client traffic below is explicitly pinned to loopback.
for _ in $(seq 1 50); do
    candidate="$(awk 'BEGIN { srand(); print 20000 + int(rand() * 20000) }')"
    if ! ss -H -ltn | awk -v port=":$candidate" '$4 ~ port "$" {found=1} END {exit found ? 0 : 1}'; then
        PORT="$candidate"
        break
    fi
done
[[ -n "$PORT" ]] || fail 'could not find a free TCP port'

if ss -H -ltn | awk -v port=":$PORT" '$4 ~ port "$" {found=1} END {exit found ? 0 : 1}'; then
    fail "selected port $PORT became occupied before startup"
fi

printf 'frontdoor: building ./cmd/consensus -> /tmp/consensus\n'
go build -o /tmp/consensus ./cmd/consensus

printf 'frontdoor: starting fresh server on 127.0.0.1:%s\n' "$PORT"
CONSENSUS_CONFIG="$CONFIG_PATH" \
CONSENSUS_DB_URL="sqlite://$DB_PATH" \
CONSENSUS_LLM_PROVIDER=mock \
CONSENSUS_MOCK_LLM=1 \
    /tmp/consensus serve --port "$PORT" >"$LOG_PATH" 2>&1 &
SERVER_PID=$!
BASE_URL="http://127.0.0.1:$PORT"

health_code=""
for _ in $(seq 1 240); do
    health_code="$(curl --silent --show-error --max-time 1 -o "$HEALTH_RESPONSE" -w '%{http_code}' "$BASE_URL/api/v1/health" || true)"
    if [[ "$health_code" == "200" ]]; then
        printf 'frontdoor: health returned 200\n'
        break
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        fail "server PID $SERVER_PID exited before health"
    fi
    sleep 0.25
done
[[ "$health_code" == "200" ]] || fail "health did not return 200 (last status: $health_code)"

admin_key="$(awk '{ for (i = 1; i <= NF; i++) if ($i ~ /^key=cs_ak_/) { sub(/^key=/, "", $i); print $i; exit } }' "$LOG_PATH")"
[[ -n "$admin_key" ]] || fail "fresh-server admin key was not emitted"

create_code="$(curl --silent --show-error --max-time 10 -o "$SESSION_RESPONSE" -w '%{http_code}' \
    -X POST "$BASE_URL/api/v1/sessions" \
    -H "Authorization: Bearer $admin_key" \
    -H 'Content-Type: application/json' \
    --data '{"agent_name":"frontdoor-battery","goal":"Return the deterministic mock response."}')"
[[ "$create_code" == "201" ]] || fail "session creation returned HTTP $create_code: $(cat "$SESSION_RESPONSE")"

session_id="$(jq -er '.id' "$SESSION_RESPONSE")" || fail "session response has no id"
printf 'frontdoor: session created over HTTP: %s\n' "$session_id"

read_code="$(curl --silent --show-error --max-time 10 -o "$SESSION_READBACK" -w '%{http_code}' \
    -H "Authorization: Bearer $admin_key" \
    "$BASE_URL/api/v1/sessions/$session_id")"
[[ "$read_code" == "200" ]] || fail "session readback returned HTTP $read_code: $(cat "$SESSION_READBACK")"
jq -e --arg id "$session_id" '.id == $id and .agent_name == "frontdoor-battery"' "$SESSION_READBACK" >/dev/null || \
    fail "created session was not readable over API"
printf 'frontdoor: session creation persisted and read back over HTTP\n'

message_code="$(curl --silent --show-error --max-time 10 -o "$MESSAGE_RESPONSE" -w '%{http_code}' \
    -X POST "$BASE_URL/api/v1/sessions/$session_id/message" \
    -H "Authorization: Bearer $admin_key" \
    -H 'Content-Type: application/json' \
    --data '{"content":"frontdoor durable mutation"}')"
[[ "$message_code" == "200" ]] || fail "message mutation returned HTTP $message_code: $(cat "$MESSAGE_RESPONSE")"
jq -e --arg id "$session_id" '.status == "message_received" and .session == $id' "$MESSAGE_RESPONSE" >/dev/null || \
    fail "message response did not acknowledge the target session"
printf 'frontdoor: message mutation accepted over HTTP\n'

expected_message='mock reasoning'
mutation_ok=0
for _ in $(seq 1 40); do
    read_code="$(curl --silent --show-error --max-time 5 -o "$SESSION_READBACK" -w '%{http_code}' \
        -H "Authorization: Bearer $admin_key" \
        "$BASE_URL/api/v1/sessions/$session_id" || true)"
    if [[ "$read_code" == "200" ]] && jq -e --arg expected "$expected_message" \
        '.status == "idle" and .last_message == $expected' "$SESSION_READBACK" >/dev/null 2>&1; then
        mutation_ok=1
        break
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        fail "server PID $SERVER_PID exited during mutation processing"
    fi
    sleep 0.25
done
[[ "$mutation_ok" == "1" ]] || fail "durable assistant mutation was not visible over HTTP: $(cat "$SESSION_READBACK")"
printf 'frontdoor: PASS: durable assistant mutation read back over HTTP\n'
