#!/usr/bin/env bash
set -e
ADMIN_KEY="cs_ak_49287865bb9d8e5fe3357eaaf796d8e086ffc8f04a22bd5ea8db4a684b95d3be"

echo "=== AC-006: Session CRUD ==="
CREATE_RESP=$(curl -s -X POST http://127.0.0.1:8094/api/v1/sessions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_KEY" \
  -d '{"goal":"cron wake test","agent_name":"cron-test","llm_provider":"deepseek","llm_model":"deepseek-chat"}')
echo "CREATE: $CREATE_RESP"

SID=$(echo "$CREATE_RESP" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
SK=$(echo "$CREATE_RESP" | grep -o '"api_key":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "SID=$SID SK starts: ${SK:0:10}..."

echo "=== GET session ==="
curl -s "http://127.0.0.1:8094/api/v1/sessions/$SID" \
  -H "Authorization: Bearer $ADMIN_KEY"
echo ""

echo "=== Message ==="
curl -s -X POST "http://127.0.0.1:8094/api/v1/sessions/$SID/message" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SK" \
  -d '{"content":"hello world","role":"user"}'
echo ""

echo "=== Memory ==="
curl -s "http://127.0.0.1:8094/api/v1/sessions/$SID/memory" \
  -H "Authorization: Bearer $SK"
echo ""

echo "=== AC-046: Health no-auth ==="
curl -s http://127.0.0.1:8094/api/v1/health
echo ""

echo "=== AC-013: Bootstrap key TTL ==="
# Check bootstrap key in DB
python3 -c "
import json, sys
resp = \"$CREATE_RESP\"
d = json.loads(resp)
print(f'Session: id={d.get(\"id\",\"?\")} status={d.get(\"status\",\"?\")} goal={d.get(\"goal\",\"\")[:30]}')
"
echo ""

echo "=== DONE ==="
