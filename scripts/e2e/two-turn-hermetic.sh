#!/usr/bin/env bash
# DF-CONSENSUS-6-R2 — hermetic two-turn acceptance arm.
#
# Starts a stdlib-only recorder stub (scripts/e2e/two_turn_recorder.py) on a
# free port, starts `consensus serve` on a FRESH sqlite file with a dummy
# DEEPSEEK_API_KEY and CONSENSUS_LLM_BASE_URL pointed at the stub, creates a
# session through the real HTTP API, sends turn 1 "My favorite number is 41"
# and turn 2 "what is my favorite number?", then asserts:
#
#   1. the recorded turn-2 request body literally contains the turn-1 user
#      text ("My favorite number is 41") AND the turn-1 assistant reply text;
#   2. GET /api/v1/sessions/{id}/memory returns both turns as persisted
#      events (2 user_message + 2 text_block, in conversation order).
#
# Exit 0 on pass (evidence printed); non-zero with the evidence on file on
# fail. Everything runs under a mktemp sandbox; the trap cleans it up.
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STUB_SCRIPT="$REPO_ROOT/scripts/e2e/two_turn_recorder.py"

TURN1='My favorite number is 41'
TURN2='what is my favorite number?'

TMP="$(mktemp -d /tmp/df006-e2e.XXXXXX)"
RECORD="$TMP/requests.jsonl"
SERVE_LOG="$TMP/serve.log"
STUB_LOG="$TMP/stub.log"
FAIL=0

cleanup() {
  [ -n "${SERVE_PID:-}" ] && kill "$SERVE_PID" 2>/dev/null
  [ -n "${STUB_PID:-}" ] && kill "$STUB_PID" 2>/dev/null
}
trap cleanup EXIT

die() {
  echo "FAIL: $*"
  echo "--- recorder JSONL ($RECORD) ---"
  cat "$RECORD" 2>/dev/null || echo "(no requests recorded)"
  echo "--- serve log (tail) ---"
  tail -40 "$SERVE_LOG" 2>/dev/null
  echo "--- artifacts kept at $TMP ---"
  exit 1
}

command -v python3 >/dev/null || die "python3 not found"

# 1. Build the serve binary.
( cd "$REPO_ROOT" && go build -o "$TMP/consensus" ./cmd/consensus ) || die "go build failed"

# 2. Start the recorder stub; wait for its PORT line.
python3 "$STUB_SCRIPT" "$RECORD" > "$STUB_LOG" 2>&1 &
STUB_PID=$!
for _ in $(seq 1 50); do
  if grep -q '^PORT=' "$STUB_LOG" 2>/dev/null; then break; fi
  kill -0 "$STUB_PID" 2>/dev/null || die "recorder stub died: $(cat "$STUB_LOG")"
  sleep 0.1
done
STUB_PORT="$(sed -n 's/^PORT=//p' "$STUB_LOG" | head -1)"
[ -n "$STUB_PORT" ] || die "stub never printed PORT"
echo "recorder stub on 127.0.0.1:$STUB_PORT (record: $RECORD)"

# 3. Pick a free port for serve and write a minimal config (pprof disabled so
# the arm cannot collide with a live instance's :18090 debug listener).
# Provider is "openai": the OpenAI-compatible chat.completions client — the
# recorder stub speaks exactly that wire format (the client accepts any base
# URL; CONSENSUS_LLM_BASE_URL below overrides the config default).
SERVE_PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
cat > "$TMP/consensus.yaml" <<EOF
server:
  hostname: 127.0.0.1
  port: $SERVE_PORT
  pprof_addr: ""
llm:
  provider: openai
EOF

# 4. Start serve on the fresh sqlite. HOME is sandboxed so the real
# ~/.consensus config is never read; CONSENSUS_CONFIG pins the minimal one.
# First-boot migration of the full schema on sqlite can take ~15s and has
# been observed to trip a busy-timeout under load — one retry on a fresh file.
start_serve() {
  rm -f "$TMP/e2e.db" "$TMP/e2e.db-wal" "$TMP/e2e.db-shm"
  HOME="$TMP" CONSENSUS_CONFIG="$TMP/consensus.yaml" \
    CONSENSUS_DB_URL="sqlite://$TMP/e2e.db" \
    DEEPSEEK_API_KEY="sk-e2e-dummy-not-real" \
    CONSENSUS_LLM_PROVIDER=openai \
    CONSENSUS_LLM_BASE_URL="http://127.0.0.1:$STUB_PORT/v1" \
    CONSENSUS_LLM_MODEL="e2e-stub" \
    "$TMP/consensus" serve --port "$SERVE_PORT" --hostname 127.0.0.1 \
    > "$SERVE_LOG" 2>&1 &
  SERVE_PID=$!
}
start_serve

# 5. Readiness: the bootstrap first-admin key line is the machine-parseable
# signal that migrations ran and the server came up (SPEC-016 §3).
ADMIN_KEY=""
for attempt in 1 2; do
  for _ in $(seq 1 400); do
    if ! kill -0 "$SERVE_PID" 2>/dev/null; then break; fi
    ADMIN_KEY="$(sed -n 's/^consensus: first_admin_key created=true key=\([^ ]*\) .*$/\1/p' "$SERVE_LOG" | head -1)"
    [ -n "$ADMIN_KEY" ] && break
    sleep 0.25
  done
  [ -n "$ADMIN_KEY" ] && break
  if [ "$attempt" = 1 ]; then
    echo "serve attempt 1 did not come up; retrying on a fresh sqlite"
    kill "$SERVE_PID" 2>/dev/null; wait "$SERVE_PID" 2>/dev/null
    start_serve
  fi
done
[ -n "$ADMIN_KEY" ] || die "serve never bootstrapped an admin key; log: $(tail -20 "$SERVE_LOG")"
HEALTH_OK=""
for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:$SERVE_PORT/api/v1/health" >/dev/null 2>&1; then
    HEALTH_OK=1; break
  fi
  sleep 0.25
done
[ -n "$HEALTH_OK" ] || die "serve health check failed; log: $(tail -20 "$SERVE_LOG")"
echo "serve on 127.0.0.1:$SERVE_PORT, admin key bootstrapped"

api() { # api METHOD PATH [BODY]
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -s -X "$method" -H "Authorization: Bearer $ADMIN_KEY" \
      -H "Content-Type: application/json" -d "$body" \
      "http://127.0.0.1:$SERVE_PORT$path"
  else
    curl -s -H "Authorization: Bearer $ADMIN_KEY" \
      "http://127.0.0.1:$SERVE_PORT$path"
  fi
}

# 6. Create a session through the real API.
CREATE_RESP="$(api POST /api/v1/sessions '{"agent_name":"e2e-two-turn","goal":"DF-CONSENSUS-6-R2 hermetic two-turn arm"}')"
SESSION_ID="$(printf '%s' "$CREATE_RESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("id",""))')"
[ -n "$SESSION_ID" ] || die "session creation failed: $CREATE_RESP"
echo "session: $SESSION_ID"

# 7. Send the two turns, waiting for the session to settle idle between them.
wait_idle() {
  for _ in $(seq 1 150); do
    STATUS="$(api GET "/api/v1/sessions/$SESSION_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))' 2>/dev/null)"
    [ "$STATUS" = "idle" ] && return 0
    sleep 0.2
  done
  return 1
}

api POST "/api/v1/sessions/$SESSION_ID/message" "{\"role\":\"user\",\"content\":\"$TURN1\"}" >/dev/null \
  || die "turn-1 POST failed"
wait_idle || die "session never settled after turn 1"
echo "turn 1 settled"

api POST "/api/v1/sessions/$SESSION_ID/message" "{\"role\":\"user\",\"content\":\"$TURN2\"}" >/dev/null \
  || die "turn-2 POST failed"
wait_idle || die "session never settled after turn 2"
echo "turn 2 settled"

# 8. Assert the recorded turn-2 request body carries the turn-1 exchange.
# 8. Assert the recorded turn-2 request body carries the turn-1 exchange.
# The checker programs are written to the sandbox (not piped on stdin) so the
# data files can be passed as argv.
cat > "$TMP/check_turn2.py" <<'PYEOF'
import json, sys
path, turn1, reply_marker = sys.argv[1], sys.argv[2], sys.argv[3]
body = json.load(open(path, encoding="utf-8"))
messages = body.get("messages", [])
print("--- recorded turn-2 request messages array ---")
print(json.dumps(messages, indent=2, ensure_ascii=False))
flat = json.dumps(messages, ensure_ascii=False)
ok = True
if turn1 not in flat:
    print(f"FAIL: turn-2 request does not contain the turn-1 user text {turn1!r}")
    ok = False
if reply_marker not in flat:
    print(f"FAIL: turn-2 request does not contain the turn-1 assistant reply {reply_marker!r}")
    ok = False
sys.exit(0 if ok else 1)
PYEOF

cat > "$TMP/check_memory.py" <<'PYEOF'
import json, sys
path, turn1, turn2, reply_marker = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
events = json.load(open(path, encoding="utf-8"))
conv = [e for e in events if e.get("type") in ("user_message", "text_block")]
conv.sort(key=lambda e: e.get("id", 0))  # endpoint returns newest-first; compare chronologically
print("--- GET /api/v1/sessions/{id}/memory conversational events ---")
print(json.dumps(conv, indent=2, ensure_ascii=False))
contents = [e.get("content", "") for e in conv]
types = [e.get("type") for e in conv]
expected = [turn1, reply_marker, turn2, reply_marker]
ok = True
if len(conv) != 4:
    print(f"FAIL: want 4 conversational events (2 user_message + 2 text_block), got {len(conv)}")
    ok = False
elif contents != expected:
    print(f"FAIL: conversation order/content = {contents}, want {expected}")
    ok = False
elif types != ["user_message", "text_block", "user_message", "text_block"]:
    print(f"FAIL: event types = {types}, want user_message/text_block alternating")
    ok = False
sys.exit(0 if ok else 1)
PYEOF

LINES="$(wc -l < "$RECORD")"
[ "$LINES" -ge 2 ] || die "recorder captured $LINES request(s), want >= 2"
sed -n '2p' "$RECORD" > "$TMP/turn2.json"
REPLY_TEXT="Ack: the assistant reply marker is E2E-REPLY-9f31."
python3 "$TMP/check_turn2.py" "$TMP/turn2.json" "$TURN1" "$REPLY_TEXT" || FAIL=1

# 9. Assert the memory read-back returns both turns as persisted events.
api GET "/api/v1/sessions/$SESSION_ID/memory" > "$TMP/memory.json"
python3 "$TMP/check_memory.py" "$TMP/memory.json" "$TURN1" "$TURN2" "$REPLY_TEXT" || FAIL=1

# 10. last_message must be a reply, not planner text.
LAST_MESSAGE="$(api GET "/api/v1/sessions/$SESSION_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("last_message") or "")')"
if [ "$LAST_MESSAGE" != "Ack: the assistant reply marker is E2E-REPLY-9f31." ]; then
  echo "FAIL: last_message = '$LAST_MESSAGE', want the assistant reply (not planner text)"
  FAIL=1
fi

if [ "$FAIL" -ne 0 ]; then
  echo "--- artifacts kept at $TMP ---"
  exit 1
fi

echo "PASS: turn-2 request carried the turn-1 exchange; memory read-back shows both turns; last_message is the reply"
exit 0
