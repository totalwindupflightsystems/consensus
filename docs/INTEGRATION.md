# Consensus Integration Guide

How to connect external systems to Consensus: **MCP clients** (SSE and stdio
transports) and the **H3 brain-swap adapter**. All examples below were checked
against the code in this repository (see the "Verified against" notes) — no
invented endpoints, no invented JSON.

- REST API: see [API.md](API.md) — every `/api/v1/*` endpoint with examples.
- MCP spec: `specs/015-api-and-mcp.md` §5.
- H3 protocol types: `internal/shim/h3/server.go` (this is the source of truth;
  there is no H3 spec file yet).

The server listens on `127.0.0.1:8090` by default (`server.port` in
`consensus.yaml`, `CONSENSUS_PORT` env override).

---

## 1. MCP (Model Context Protocol)

Consensus exposes its agent runtime as an MCP server with two transports:

| Transport | How to reach it |
|---|---|
| SSE over HTTP | `GET /mcp/sse` + `POST /mcp/message?sessionId=...` on a running `consensus serve` |
| stdio | `consensus mcp-stdio` (JSON-RPC on stdin/stdout) |

Both serve the same JSON-RPC 2.0 surface. *Verified against:*
`internal/mcp/server.go` (`Handler()` registers `/mcp/sse` and
`/mcp/message`), `cmd/consensus/main.go` (mounts the MCP handler under
`/mcp/*` on the API router).

### 1.1 Authentication

MCP uses the same API keys as the REST API (`cs_ak_...` secrets created by
`consensus init`, see [API.md](API.md)). The key is configured on the
**client** side and forwarded to the server inside the `initialize` request's
`_meta.authorization` field:

- `--api-key cs_ak_...` flag on `consensus mcp-stdio`, **or**
- `CONSENSUS_API_KEY` environment variable, **or**
- `server.api_key` in `consensus.yaml`

*Verified against:* `internal/mcp/auth.go` (`validateAuth` reads
`params._meta.authorization`, accepts an optional `Bearer ` prefix, and checks
the key against the `api_keys` table).

### 1.2 SSE transport, step by step (curl)

**Step 1 — start the server** (SQLite, no API key needed to *boot*):

```bash
consensus init --db-url sqlite:///tmp/cs-mcp.db
consensus serve --db-url sqlite:///tmp/cs-mcp.db   # prints the admin key once
```

**Step 2 — open the SSE stream.** The server assigns a session and immediately
sends the message endpoint URL:

```bash
curl -N http://127.0.0.1:8090/mcp/sse
```

```
event: endpoint
data: /mcp/message?sessionId=1f2e3d4c5b6a7988
```

**Step 3 — POST JSON-RPC messages to that endpoint** (each in its own curl,
same `sessionId`). The wire format is JSON-RPC 2.0:
`{"jsonrpc":"2.0","id":<any>,"method":"...","params":{...}}`.

Initialize (this is where auth happens — the key goes in `_meta.authorization`):

```bash
curl -s -X POST 'http://127.0.0.1:8090/mcp/message?sessionId=1f2e3d4c5b6a7988' \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0", "id": 1, "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "curl", "version": "1.0"},
      "_meta": {"authorization": "cs_ak_..."}
    }
  }'
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {"tools": {"listChanged": false}, "resources": {"subscribe": false, "listChanged": false}, "prompts": {"listChanged": false}},
    "serverInfo": {"name": "consensus-mcp", "version": "0.1.0"}
  }
}
```

List the tools:

```bash
curl -s -X POST 'http://127.0.0.1:8090/mcp/message?sessionId=1f2e3d4c5b6a7988' \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

The server registers six tools: `create_session`, `send_message`,
`get_session_status`, `list_memory`, `review_approval`, `query_tool`
(*verified against:* `internal/mcp/tools.go`).

Create a session, then message it:

```bash
# tools/call create_session {"agent_name":"demo","goal":"list my memory"}
# → result.sessionId  (the Consensus session id)
#
# tools/call send_message {"session_id":"<id>","message":"start"}
```

*Verified against:* `internal/mcp/server.go` — requests without an `id` are
treated as notifications (no response); `POST` only; a missing `sessionId`
query param yields 400, an unknown one yields 404.

### 1.3 stdio transport

```bash
consensus mcp-stdio --api-key cs_ak_...
```

Then speak JSON-RPC 2.0 over stdin, one message per line; responses arrive on
stdout. Any MCP client that supports stdio (Claude Desktop, IDE MCP plugins)
can point at `consensus mcp-stdio --api-key cs_ak_...` directly. *Verified
against:* `internal/cli/mcp_stdio.go` and the dogfood run (2026-08-04).

---

## 2. H3 adapter (brain-swap protocol)

The H3 shim (`internal/shim/h3/`) lets any H3-compliant harness use Consensus
as an external agent brain. It translates H3's `/v1/process` (send a message)
and `/v1/result` (feed back a tool result) calls into Consensus session
operations, and answers with H3 **decision** objects.

> **Status: library, not yet mounted.** As of this writing the shim is a
> self-contained Go package with zero wiring in `cmd/consensus` — `consensus
> serve` does **not** expose `/v1/*` H3 endpoints yet. Section 2.3 shows the
> exact mount contract and a complete runnable example.

### 2.1 Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/v1/health` | GET | Liveness + protocol capability advertisement |
| `/v1/process` | POST | Send a user message; returns a decision |
| `/v1/result` | POST | Feed a tool result back; returns the next decision |
| `/v1/cancel` | POST | Acknowledge cancellation (`{"status":"cancelled"}`) |

*Verified against:* `internal/shim/h3/server.go` (`NewServer` route table).

`/v1/health` response:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "transport": "rest",
  "protocol_version": "1.0",
  "capabilities": ["text", "tool_call", "llm_call", "end"]
}
```

### 2.2 The decision object

Every `/v1/process` and `/v1/result` response is a decision:

| `decision` | Meaning | Fields |
|---|---|---|
| `text` | Agent produced prose | `text.content`, `text.finished` |
| `tool_call` | Agent wants a tool run | `tool_call.name`, `tool_call.params`, `tool_call.reasoning` |
| `llm_call` | Raw LLM invocation (reserved) | `llm_call.*` |
| `wait` | Agent is waiting | `wait.reason` |
| `delegate` | Agent delegates to another agent | `delegate.*` |
| `end` | Session over | `end.reason` (`task_complete`/`user_requested`/`error`/`timeout`), `end.summary` |

All decisions carry a `decision_id` (UUID). Errors are `end` decisions with
`end.reason = "error"`, or an HTTP 400 envelope
`{"error":{"code":"INVALID_REQUEST","message":"..."}}` for malformed bodies.

### 2.3 Mounting the shim

The shim exposes one constructor and one handler:

```go
srv := h3.NewServer(database, svc) // database db.DB, svc SessionService
http.Handle("/", srv.Handler())    // or mount on your router, e.g. apiMux.Handle("/h3/*", ...)
```

`SessionService` is the minimal Consensus surface the shim needs
(`internal/shim/h3/server.go`):

```go
type SessionService interface {
    CreateSession(ctx, agentName, goal, modelID, projectID string, contextBudget int) (sessionID, status string, err error)
    GetSession(ctx, id string) (status string, err error)
    ProcessMessage(ctx, sessionID, message string) (response string, err error)
    FeedToolResult(ctx, sessionID, toolName string, success bool, data any) (response string, err error)
}
```

The shim maps an H3 session id to a Consensus session on first `/v1/process`
(agent name `h3-<user_name>`, goal = message content, model = first entry of
`context.models` or `deepseek-v4-pro`, context budget 200000) and reuses it
for every later call on the same H3 session id.

**Runnable example (keyless, canned responses).** Drop this into
`cmd/h3shim/main.go` in this repo and run
`timeout 30 go run ./cmd/h3shim` — it serves the H3 endpoints on `:8095`
with a stub service that plays back a realistic conversation, so you can
exercise the protocol without an LLM key:

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/wojons/consensus/internal/shim/h3"
)

// stub implements h3.SessionService with a canned two-step conversation:
// process → "searching..." (text), first result → tool call, second result → DONE.
type stub struct{ calls int }

func (s *stub) CreateSession(ctx context.Context, agentName, goal, modelID, projectID string, budget int) (string, string, error) {
	return "cs-stub-1", "active", nil
}
func (s *stub) GetSession(ctx context.Context, id string) (string, error) { return "active", nil }
func (s *stub) ProcessMessage(ctx context.Context, sessionID, message string) (string, error) {
	return "searching...", nil
}
func (s *stub) FeedToolResult(ctx context.Context, sessionID, toolName string, ok bool, data any) (string, error) {
	s.calls++
	if s.calls == 1 {
		return `{"internal_monologue":"need more data","tool_requests":[{"tool_name":"search","parameters":{"q":"next"}}]}`, nil
	}
	return "wrapped up DONE", nil
}

func main() {
	log.Println("h3 shim demo on :8095 — try: curl -s localhost:8095/v1/health")
	log.Fatal(http.ListenAndServe("127.0.0.1:8095", h3.NewServer(nil, &stub{}).Handler()))
}
```

> Note: in production, adapt `internal/api`'s `SessionService` (its
> `CreateSession` takes an input struct — the adapter maps fields across) and
> pass a real `db.DB`. Production wiring into `cmd/consensus` is the natural
> next step and will make the H3 endpoints available on `consensus serve`
> directly.

### 2.4 Worked session (text → tool_call → end)

All bodies below match the actual request/response types in
`internal/shim/h3/server.go`. `$H3 = http://127.0.0.1:8095`.

**1. `/v1/process` — new session, agent answers with text:**

```bash
curl -s -X POST $H3/v1/process -H 'Content-Type: application/json' -d '{
  "session_id": "hermes-42",
  "message": {"role": "user", "content": "find the consensus docs", "timestamp": "2026-08-14T10:00:00Z"},
  "identity": {"platform": "hermes", "chat_id": "c1", "user_name": "alice", "user_id": "u1"},
  "context": {
    "history": [],
    "tools": [],
    "models": [{"name": "deepseek-v4-flash", "provider": "openai", "context_window": 128000}],
    "config": {"max_iterations": 5, "timeout_seconds": 60},
    "session_state": {"turn_count": 0, "total_tool_calls": 0, "total_llm_calls": 0, "cost_so_far": 0}
  }
}'
```

```json
{
  "decision": "text",
  "decision_id": "9f3c...",
  "text": {"content": "searching...", "finished": false}
}
```

`finished` flips to `true` when the response contains `DONE` or the turn count
reaches `context.config.max_iterations`.

**2. `/v1/result` — feed the tool result; the agent requests another tool:**

```bash
curl -s -X POST $H3/v1/result -H 'Content-Type: application/json' -d '{
  "session_id": "hermes-42",
  "decision_id": "9f3c...",
  "result": {"type": "tool_result", "tool_name": "search", "data": {"hits": 3}, "duration_ms": 12.5, "success": true}
}'
```

```json
{
  "decision": "tool_call",
  "decision_id": "71ab...",
  "tool_call": {"name": "search", "params": {"q": "next"}, "reasoning": "need more data"}
}
```

(The agent signals a tool request by returning JSON with a non-empty
`tool_requests[]` array; `reasoning` comes from `internal_monologue`.)

**3. `/v1/result` — agent reports completion:**

```bash
curl -s -X POST $H3/v1/result -H 'Content-Type: application/json' -d '{
  "session_id": "hermes-42",
  "decision_id": "71ab...",
  "result": {"type": "tool_result", "tool_name": "search", "data": {"hits": 3}, "success": true}
}'
```

```json
{
  "decision": "end",
  "decision_id": "c02d...",
  "end": {"reason": "task_complete", "summary": "wrapped up DONE"}
}
```

A `/v1/result` for an unknown H3 session returns `end` with
`"summary": "session not found: <id>"` and `end.reason = "error"`.

---

## 3. Verifying your setup

```bash
# REST + MCP surfaces live
curl -s http://127.0.0.1:8090/api/v1/health | head -c 200

# H3 shim (runnable example from §2.3)
curl -s http://127.0.0.1:8095/v1/health

# Keyless end-to-end harness loop (mocked LLM) — the repo's own smoke test:
make smoke          # or: timeout 60 go test -run Smoke ./demo/
```

---

## Related

- [HTTP API Reference](API.md)
- [Quickstart (cross-platform)](quickstart-cross-platform.md)
- [MCP + API spec](specs/015-api-and-mcp.md) (SPEC-015, §5 = MCP)
- [OpenAPI contract](specs/018-openapi-contract.md)
