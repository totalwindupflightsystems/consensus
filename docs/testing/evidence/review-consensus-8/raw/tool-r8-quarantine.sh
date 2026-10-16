#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 — quarantine arm.
# Q1: valid HMAC signature + threat content  -> quarantined by the scanner
# Q2: INVALID signature + benign content     -> quarantined by the signature check
set -u
export BASE=http://127.0.0.1:18090
SID=$(cat /tmp/r8/sid2.txt)
T=/tmp/r8/t-quarantine.txt
SECRET=r8-webhook-secret
: > "$T"

echo "\$ sqlite3 /tmp/r8/consensus.db \"INSERT INTO webhook_registrations (id,name,source,url_path,secret,event_types,target_session_id,enabled,created_at) VALUES ('r8-reg-1','r8-quarantine-probe','r8-quarantine-probe','/webhooks/r8-quarantine-probe','$SECRET','{}','$SID',1,datetime('now'));\"" >> "$T"
sqlite3 /tmp/r8/consensus.db "INSERT INTO webhook_registrations (id,name,source,url_path,secret,event_types,target_session_id,enabled,created_at) VALUES ('r8-reg-1','r8-quarantine-probe','r8-quarantine-probe','/webhooks/r8-quarantine-probe','$SECRET','{}','$SID',1,datetime('now'));" \
  && echo "-- registration seeded (no API/CLI verb creates webhook registrations) --" >> "$T"

Q1_BODY='{"tool":"db_admin","sql":"DROP TABLE users; -- r8 probe malicious external content"}'
Q2_BODY='{"article":"A perfectly ordinary status update from the monitoring service."}'
Q1_SIG=$(printf '%s' "$Q1_BODY" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')
Q2_SIG=$(printf '%s' "$Q2_BODY" | openssl dgst -sha256 -hmac "wrong-secret-on-purpose" | awk '{print $2}')

req() {  # label, url, body, sig-header-value, delivery-id
  local label="$1" url="$2" body="$3" sig="$4" did="$5"
  {
    echo "#####################################################################"
    echo "# $label"
    echo "#####################################################################"
    echo "\$ curl -sS -i -X POST ${BASE}${url} -H 'Content-Type: application/json' \\"
    echo "    -H 'X-Event-Type: r8.probe' -H 'X-Delivery-ID: $did' -H 'X-Hub-Signature-256: sha256=<hmac>' \\"
    echo "    -d '$body'"
    echo ""
  } >> "$T"
  curl -sS -i -X POST "${BASE}${url}" -H 'Content-Type: application/json' \
    -H 'X-Event-Type: r8.probe' -H "X-Delivery-ID: $did" -H "X-Hub-Signature-256: sha256=$sig" \
    -d "$body" >> "$T" 2>&1
  echo "" >> "$T"; echo "" >> "$T"
}

req "Q1. POST /webhooks/r8-quarantine-probe — VALID signature + SQL-injection content (scanner arm)" \
    "/webhooks/r8-quarantine-probe" "$Q1_BODY" "$Q1_SIG" "r8-delivery-1"
req "Q2. POST /webhooks/r8-quarantine-probe — INVALID signature + benign content (signature arm)" \
    "/webhooks/r8-quarantine-probe" "$Q2_BODY" "$Q2_SIG" "r8-delivery-2"

sleep 1
{
  echo "#####################################################################"
  echo "# Q3. database state: external_events + external_quarantine"
  echo "#####################################################################"
  echo "\$ sqlite3 -header -column /tmp/r8/consensus.db \"SELECT id,source,event_type,signature_valid,status FROM external_events; SELECT id,session_id,source_type,validation_status,substr(raw_content,1,60) FROM external_quarantine;\""
  echo ""
  echo "-- external_events:"
  sqlite3 -header -column /tmp/r8/consensus.db "SELECT id,source,source_id,event_type,signature_valid,status FROM external_events ORDER BY id;"
  echo "-- external_quarantine:"
  sqlite3 -header -column /tmp/r8/consensus.db "SELECT id,session_id,source_type,validation_status,substr(raw_content,1,52) AS raw_content FROM external_quarantine ORDER BY id;"
  echo "-- webhook scan log:"
  grep 'webhook: quarantining\|webhook: failed to insert quarantine' /tmp/r8/serve3.log | tail -6
  echo ""
} >> "$T"
echo "quarantine arm captured"
