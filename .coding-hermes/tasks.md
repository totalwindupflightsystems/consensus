
## Dogfood Findings (2026-09-01)
Verdict: PROMISING-BUT-ROUGH
Promise: {"entry_point":"CLI binary  (cmd/consensus, subcommands init/serve/mcp-stdio) that boots an HTTP server on :8090 exposing the REST API /api/v1 (sessions, memory, tasks, billing), SSE event stream, MCP server (SSE + stdio), embedded OpenAPI spec, and Chronicle dashboard at /chronicle/;

- [P0] LLM path dead on fresh checkout: shipped consensus.yaml pins base_url to DeepSeek and defeats both documented env overrides — With the shipped consensus.yaml present, OPENROUTER_API_KEY swaps the key but calls still hit api.deepseek.com and 401, contradicting the README's 'no separate CONSENSUS_LLM_BASE_URL required'; CONSEN
- [P1] POST /api/v1/sessions 400s on two undocumented required fields (agent_name, goal) — The documented curl example in docs/API.md returns 400; both agent_name and goal are required but absent from the docs. Creating the first session took two trial-and-error 400s — the documented quicks
- [P1] Message contract is async but docs promise a synchronous response; no poll-for-response recipe exists — POST /api/v1/sessions/{id}/message returns {"status":"message_received"} instantly while docs say it 'returns the agent response'. The agent reply is never observable via any documented path — SSE /ap
- [P2] MCP SSE routes undiscoverable: /api/v1/mcp 404s, /mcp is a 501 stub, real routes live only in docs/INTEGRATION.md — Route guessing cost real time; /mcp/sse + /mcp/message + _meta.authorization work once found (SSE handshake, tools/list, stdio auth error all verified) but are absent from the README — exactly the pro
- [P2] Free-port guidance is stale and its '404 page not found' tell-tale misses health-looking impostors — README's suggested ports 8090/8095/8091 were all occupied on this host (4 ports probed before 8124 worked), and the 8095 occupant answered /api/v1/health with plausible JSON (status:unhealthy, version
