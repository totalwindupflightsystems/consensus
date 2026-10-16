#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 — dashboard / UI surface smoke.
set -u
T=/tmp/r8/t-dashboard.txt
BASE=http://127.0.0.1:18090
: > "$T"

probe() {  # label, path
  local label="$1" path="$2"
  {
    echo "====================================================================="
    echo "# $label"
    echo "====================================================================="
    echo "\$ curl -sS -i ${BASE}${path}"
    echo "---------------------------------------------------------------------"
  } >> "$T"
  local body status
  body=$(curl -sS -m 10 -w '\n__STATUS__%{http_code}__SIZE__%{size_download}' "${BASE}${path}")
  status=$(printf '%s' "$body" | tail -1)
  printf '%s\n' "$body" | sed '$d' | head -18 >> "$T"
  echo "..." >> "$T"
  printf '[%s]\n\n' "$status" >> "$T"
  printf '%-40s %s\n' "$label" "$status"
}

probe "GET /ui/ (dashboard index)"     "/ui/"
probe "GET /ui/dashboard"              "/ui/dashboard"
probe "GET /ui/sessions"               "/ui/sessions"
probe "GET /ui/memory"                 "/ui/memory"
probe "GET /ui/health"                 "/ui/health"
probe "GET /ui/does-not-exist"         "/ui/does-not-exist"
probe "GET /chronicle/"                "/chronicle/"
probe "GET /mcp"                       "/mcp"
probe "GET /webhooks/r8-quarantine-probe (GET on webhook route)" "/webhooks/r8-quarantine-probe"

# Markers inside the rendered pages (the HTML itself is not readable evidence of wiring)
{
  echo "====================================================================="
  echo "# dashboard wiring markers (fetched separately, first 3 matches per page)"
  echo "====================================================================="
  for p in /ui/ /ui/dashboard /ui/sessions /ui/memory /ui/health; do
    echo "-- ${BASE}${p}"
    curl -sS -m 10 "${BASE}${p}" | grep -o -E '<title>[^<]*</title>|/api/v1/[a-z/-]+|data-page="[a-z-]+"' | sort -u | head -6
    echo ""
  done
} >> "$T"
echo "dashboard smoke captured"
