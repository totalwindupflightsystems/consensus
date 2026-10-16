#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 — CLI smoke battery.
# Runs every consensus subcommand past init/serve/version with a minimal
# invocation against the scratch server, recording verbatim output + exit code.
set -u
export EDITOR=true
T=/tmp/r8/t-cli.txt
BIN=/tmp/consensus-r8-bin
OUT_CAP=40   # lines kept per invocation; the full line count is always recorded
export CONSENSUS_SERVER=http://127.0.0.1:18090
export CONSENSUS_API_KEY=cs_ak_[REDACTED-scratch-admin-key]
: > "$T"

run() {  # label, args...
  local label="$1"; shift
  {
    echo "====================================================================="
    echo "# $label"
    echo "====================================================================="
    echo "\$ consensus $*"
    echo "---------------------------------------------------------------------"
  } >> "$T"
  local out rc
  out=$(timeout 25 "$BIN" "$@" < /dev/null 2>&1); rc=$?
  local lines
  lines=$(printf '%s\n' "$out" | wc -l)
  printf '%s\n' "$out" | head -"$OUT_CAP" >> "$T"
  if [ "$lines" -gt "$OUT_CAP" ]; then
    printf '[... output truncated by the harness: %s lines total, %s shown ...]\n' "$lines" "$OUT_CAP" >> "$T"
  fi
  {
    echo "---------------------------------------------------------------------"
    echo "[exit=$rc]"
    echo ""
  } >> "$T"
  printf '%-58s exit=%s\n' "$label" "$rc"
}

echo "CLI smoke battery — $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$T"
echo "" >> "$T"

echo "### 1. approve family (real pending approval)" | tee -a "$T"
sqlite3 /tmp/r8/consensus.db "INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status) VALUES ('b0a129d3-7a0b-41c5-8c84-84db23bb0dff', 1, 'r8_approval_gated_echo', '{\"target\":\"production\",\"cli_run\":1}', 'pending');"
sleep 4
AID=$(sqlite3 /tmp/r8/consensus.db "SELECT id FROM approval_requests WHERE status='pending' ORDER BY created_at DESC LIMIT 1;")
printf '%s\n' "\$ sqlite3 /tmp/r8/consensus.db \"INSERT INTO tool_requests (... 'pending');\"   # -> gate created approval $AID" >> "$T"
run "approve list (table)"            approve list
run "approve list (json)"             approve list --format json
run "approve list --risk-level medium" approve list --risk-level medium
run "approve list --session <id>"     approve list --session b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "approve show <id>"               approve show "$AID"
run "approve show (bad id)"           approve show 00000000-0000-0000-0000-000000000000
run "approve show (no args)"          approve show
run "approve accept <id>"             approve accept "$AID" --notes "R8 CLI smoke: accepted"

echo "### 2. reject family (second pending approval)" | tee -a "$T"
sqlite3 /tmp/r8/consensus.db "INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status) VALUES ('b0a129d3-7a0b-41c5-8c84-84db23bb0dff', 1, 'r8_approval_gated_echo', '{\"target\":\"production\",\"cli_run\":2}', 'pending');"
sleep 4
AID2=$(sqlite3 /tmp/r8/consensus.db "SELECT id FROM approval_requests WHERE status='pending' ORDER BY created_at DESC LIMIT 1;")
run "approve reject <id>"             approve reject "$AID2" --reason "R8 CLI smoke: rejected via nested verb"
sqlite3 /tmp/r8/consensus.db "INSERT INTO tool_requests (session_id, iteration_id, tool_name, parameters, status) VALUES ('b0a129d3-7a0b-41c5-8c84-84db23bb0dff', 1, 'r8_approval_gated_echo', '{\"target\":\"production\",\"cli_run\":3}', 'pending');"
sleep 4
AID3=$(sqlite3 /tmp/r8/consensus.db "SELECT id FROM approval_requests WHERE status='pending' ORDER BY created_at DESC LIMIT 1;")
run "reject <id> (top-level verb)"    reject "$AID3" --reason "R8 CLI smoke: rejected via top-level verb"

echo "### 3. session family" | tee -a "$T"
run "session"                         session
run "session list"                    session list
run "session list --format json"      session list --format json
run "session show <id>"               session show b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session show (bad id)"           session show 00000000-0000-0000-0000-000000000000
run "session logs <id>"               session logs b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session cost <id>"               session cost b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session message <id> <text>"     session message b0a129d3-7a0b-41c5-8c84-84db23bb0dff "hello from the r8 CLI smoke check"
run "session pause <id>"              session pause b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session resume <id>"             session resume b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session cancel <id>"             session cancel b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "session create (real)"           session create --agent-name r8-cli-probe --goal "cli smoke"
run "session create (no flags)"       session create

echo "### 4. memory / skill / tool / status families" | tee -a "$T"
run "memory"                          memory
run "memory list <id>"                memory list b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "memory iterations <id>"          memory iterations b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "memory pages <id>"               memory pages b0a129d3-7a0b-41c5-8c84-84db23bb0dff
run "memory show <id> 5"              memory show b0a129d3-7a0b-41c5-8c84-84db23bb0dff 5
run "skill"                           skill
run "skill list"                      skill list
run "skill show <name>"               skill show nonexistent-skill
run "tool"                            tool
run "tool list"                       tool list
run "tool show <name>"                tool show r8_approval_gated_echo
run "tool show (unknown)"             tool show does_not_exist
run "status"                          status
run "status --format json"            status --format json

echo "### 5. config / models / migrate / completion" | tee -a "$T"
run "config"                          config
run "config list"                     config list
run "config get server.url"           config get server.url
run "config get (unknown key)"        config get not.a.key
run "config set server.port 18090"    config set server.port 18090
run "config edit (EDITOR=true)"       config edit
run "models"                          models
run "models sync"                     models sync
run "migrate"                         migrate
run "migrate version"                 migrate version
run "migrate create r8_probe"         migrate create r8_probe
run "migrate run"                     migrate run
run "completion"                      completion
run "completion bash"                 completion bash

echo ""
echo "CLI smoke battery complete" | tee -a "$T"
