# api-surface — Layer 8: MCP server, SSE events, key authentication, OpenAPI spec (SPEC-015)

## Goal
Verify and implement REST API, MCP server, SSE event stream, and API key authentication (AC-043 through AC-047).

## Affected ACs
- AC-043: MCP server — initialize, list tools, create session
- AC-044: SSE event stream — subscribe, receive status changes
- AC-045: API key authentication — 4 scope types enforced
- AC-046: Health endpoint — no auth required (✅ already passed)
- AC-047: OpenAPI spec available

## Specs
- specs/015-api-and-mcp.md (full spec)
- specs/018-openapi-contract.md

## Steps
1. Test MCP server: POST /mcp/sse with initialize → assert capabilities → tools/list → assert 6 tools → tools/call(create_session)
2. Test SSE: connect to /api/v1/events → change session status → assert event received
3. Test auth scopes: admin_key (full), session_key (own session only), readonly_key (SELECT only), webhook_key (INSERT external_events only)
4. AC-046 already ✅ — health endpoint returns 200 without auth
5. Test OpenAPI: GET /openapi.json → assert valid spec with paths and components
