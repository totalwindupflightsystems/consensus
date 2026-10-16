#!/usr/bin/env bash
# Snapshot the scratch DB's approval/tool/session state verbatim into the transcript.
set -u
OUT="$1"; LABEL="$2"
DB=/tmp/r8/consensus.db
{
  echo "#####################################################################"
  echo "# $LABEL"
  echo "#####################################################################"
  echo "\$ sqlite3 -header -column $DB \"SELECT id,status,iteration FROM sessions ...\""
  echo ""
  echo "-- sessions:"
  sqlite3 -header -column "$DB" "SELECT id,status,iteration FROM sessions ORDER BY created_at;"
  echo "-- tool_requests:"
  sqlite3 -header -column "$DB" "SELECT id,session_id,iteration_id,tool_name,status,ifnull(approval_request_id,'(null)') AS approval_request_id FROM tool_requests ORDER BY id;"
  echo "-- approval_requests:"
  sqlite3 -header -column "$DB" "SELECT id,request_type,target_tool,status,ifnull(reviewer_id,'(null)') AS reviewer_id FROM approval_requests ORDER BY created_at;"
  echo "-- tool_results:"
  sqlite3 -header -column "$DB" "SELECT request_id,ifnull(output,'(null)') AS output,is_error FROM tool_results ORDER BY id;"
  echo "-- server log (approval gate lines):"
  grep -E 'approval gate|tool_executor|harness: found active|planning: tool call' /tmp/r8/serve3.log | tail -12
  echo ""
} >> "$OUT"
echo "snapshot appended: $LABEL"
