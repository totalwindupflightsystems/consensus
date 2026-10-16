#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 verbatim HTTP probe helper.
# Usage: r8req.sh <transcript-file> <LABEL> <METHOD> <path> [json-body]
# Appends a reproducible, verbatim transcript block (request + response).
set -u
OUT="$1"; LABEL="$2"; METHOD="$3"; PATH_="$4"; BODY="${5:-}"
BASE="${BASE:-http://127.0.0.1:18090}"
KEY="${KEY:-}"

{
  echo "#####################################################################"
  echo "# $LABEL"
  echo "#####################################################################"
  echo "\$ curl -sS -i -X $METHOD ${BASE}${PATH_} \\"
  echo "    -H 'Authorization: Bearer \$CONSENSUS_API_KEY' \\"
  echo "    -H 'Content-Type: application/json' \\"
  if [ -n "$BODY" ]; then
    echo "    -d '$BODY'"
  else
    echo "    (no body)"
  fi
  echo ""
} >> "$OUT"

ARGS=(-sS -i -X "$METHOD" "${BASE}${PATH_}")
[ -n "$KEY" ] && ARGS+=(-H "Authorization: Bearer $KEY")
ARGS+=(-H "Content-Type: application/json")
[ -n "$BODY" ] && ARGS+=(-d "$BODY")

curl "${ARGS[@]}" >> "$OUT" 2>&1
echo "" >> "$OUT"
echo "" >> "$OUT"
echo "  -> $LABEL : $(tail -40 "$OUT" | grep -m1 '^HTTP/' | tr -d '\r')"
