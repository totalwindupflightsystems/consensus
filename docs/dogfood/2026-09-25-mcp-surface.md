# Dogfood Integration Report — MCP Surface — 2026-09-25

Angle: first dogfood of the streamable-HTTP/legacy MCP surface since MCP-DIRECT-001
(df33c39) landed. Prior 2026-09-25 run (morning) covered burst sends + multi-turn via
REST; this run deliberately did NOT repeat those.

Environment: control host scratch instance on :8127 (SQLite `/tmp`, config-file path,
provider openai→api.deepseek.com/v1, model deepseek-chat, real key probed first per
ENV-CONSENSUS-1: 1 of 11 candidates live). Binary built from HEAD 8a6512d.

## What was exercised (real client, curl + python driver)

1. Legacy handshake: GET /mcp/sse → endpoint event → POST /mcp/message initialize
   (key in `_meta.authorization`) → notifications/initialized → tools/list.
   WORKS. 8 tools: create_session, send_message, get_session_status, list_memory,
   review_approval, query_tool, list_tasks, claim_task (list_tasks/claim_task new).
2. Full workflow over MCP tools only: create_session → send_message → status →
   list_memory. Tools return proper JSON-RPC results, ~10ms tool latency.
3. list_tasks/claim_task: both answer correctly (empty task set — no board rows in DB).
4. REST cross-path control: same flow via /api/v1/sessions for A/B.

## Findings (full rows on the board, commit f596c04)

- DF-CONSENSUS-23 (P0): POST/GET /mcp (the discoverable streamable mount the spec
  documents) 404s in the production binary. The unit test mounts /mcp directly;
  production mounts only /mcp/*. Reproduced on a fresh bunker clone too.
- DF-CONSENSUS-24 (P0): MCP send_message on a fresh ('booting') session is acked
  `{"sent": true}` but the session never runs — wake condition in
  internal/mcp/tools.go:287 covers idle/paused while REST (service.go:433) also
  covers booting. The same session wakes instantly on one REST message.
- DF-CONSENSUS-25 (P1): legacy endpoint returns plain-text 404 "session not found"
  (not JSON-RPC) when the SSE session was reaped after client disconnect.
- DF-CONSENSUS-26 (P2): fresh-machine docs gap (no `consensus version`; Go toolchain
  requirement unstated in README quickstart).
- PERF-CONSENSUS-12 (P2): MCP tool round trip 1058ms warm (REST control 1075ms);
  cold first turn 41.9s was 100% DF-CONSENSUS-24 dead-letter wait, not compute.

## The right way to drive consensus over MCP today (until DF-23/24 fix)

- Use the legacy transport: GET /mcp/sse, keep the stream OPEN while working
  (sessions are reaped shortly after disconnect), POST JSON-RPC to
  /mcp/message?sessionId=... with the key in initialize._meta.authorization.
- Never let an MCP-created session's FIRST message be its last resort: either
  create the session via REST then use MCP tools, or send one REST message to wake it.
- The bootstrap admin key prints ONCE on serve stdout (cs_ak_...) — capture it.
- Health endpoint is /api/v1/health (not /health).
