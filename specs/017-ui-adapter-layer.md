# SPEC-017: TUI Protocol Shims — Borrow the Frontend

**Status:** Draft
**Depends On:** SPEC-015 (API & External Interface Layer), SPEC-008 (Harness), SPEC-003 (Database Schema), SPEC-010 (Tools)
**Created:** 2026-04-12

---

## 1. Overview

Consensus is a headless agent runtime with no built-in UI. It has its **own native HTTP protocol** — the REST API defined in SPEC-015. That API is the source of truth. Everything else is built on top of it.

On top of the native API, Consensus runs **protocol shim layers** that translate other tools' server protocols into Consensus's native API calls. This lets users borrow existing TUIs (like opencode's) and connect them to Consensus, without Consensus owning or maintaining any UI code.

**Core principle:** Consensus has its own API. Shims translate external protocols into it. Don't build UIs — shim to existing ones.

This gives users three interaction paths:
1. **Native API** — Consensus's own REST API (SPEC-015). The primary interface. All other paths are built on this.
2. **TUI shims** — Translate another tool's server protocol into Consensus API calls. Example: the opencode shim lets `opencode attach http://localhost:8090` work.
3. **MCP server** — Expose Consensus as a tool provider inside any MCP-compatible AI tool.

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    USER'S EXISTING TOOLS                  │
│                                                          │
│  opencode TUI          Claude Code        custom UI      │
│  (via attach)          (via MCP tools)    (via REST API) │
└───────┬────────────────────┬──────────────────┬──────────┘
        │                    │                  │
        │ HTTP (opencode     │ MCP protocol     │ HTTP (native
        │ server protocol)   │                  │ Consensus API)
        │                    │                  │
 ┌──────▼────────────────────▼──────────────────▼──────────┐
│                                                           │
│  ┌────────────────────────┐  ┌─────────────────────────┐ │
│  │  opencode Shim         │  │  MCP Server             │ │
│  │  (translates opencode  │  │  (SPEC-015 §5)          │ │
│  │   protocol → native    │  │                         │ │
│  │   Consensus API)      │  │  create_session         │ │
│  │                        │  │  send_message           │ │
│  │  /session → API shim   │  │  get_session_status     │ │
│  │  /message → API shim   │  │  list_memory            │ │
│  │  /event  → API shim    │  │  review_approval        │ │
│  └──────────┬─────────────┘  └────────────┬────────────┘ │
│             │                              │              │
│             └──────────────┬───────────────┘              │
│                            │ all roads lead here          │
│                            ▼                              │
│  ┌──────────────────────────────────────────────────────┐│
│  │        Native Consensus REST API (SPEC-015)         ││
│  │                                                      ││
│  │  POST /api/v1/sessions          (create session)     ││
│  │  POST /api/v1/sessions/:id/message (send message)    ││
│  │  GET  /api/v1/approvals         (list approvals)     ││
│  │  POST /api/v1/approvals/:id/review (approve/reject)  ││
│  │  GET  /api/v1/sessions/:id/memory (inspect memory)   ││
│  │  GET  /api/v1/metrics           (cost/status)        ││
│  │  ...                                                 ││
│  └──────────────────────────┬───────────────────────────┘│
│                             │                              │
│  ┌──────────────────────────▼───────────────────────────┐│
│  │              Consensus Runtime                       ││
│  │  SPEC-008 Harness + SPEC-003 Database                ││
│  │                                                      ││
│  │  Harness → read context → call LLM → execute SQL     ││
│  │          → run tools → write memory → loop            ││
│  └──────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────┘
```

**Key point:** The opencode shim does NOT bypass the native API. It calls it. The shim is a translation layer — opencode protocol in, native Consensus API out. This means the native API is always complete and functional on its own, and shims are optional additions.

---

## 3. Shim: opencode Server Protocol

opencode has a strict client/server split. The TUI is just an HTTP client that talks to the server. By implementing the same server API, Consensus becomes a backend that `opencode attach` can connect to.

### 3.1 Why This Works

- opencode's server publishes an OpenAPI 3.1 spec at `/doc`
- The TUI (`opencode`), desktop app, web UI, IDE plugins, and SDK all talk to the same HTTP API
- `opencode attach http://host:port` starts only the TUI client and connects it to a remote server
- Authentication via `OPENCODE_SERVER_PASSWORD` (HTTP Basic Auth)
- The server handles sessions, messages, tool execution, file access, events — exactly what Consensus does

### 3.2 How the Shim Works

The opencode shim is a **translation layer**. It receives requests in opencode's server protocol format, translates them into calls against Consensus's native REST API (SPEC-015), and translates the responses back into opencode's format.

```
opencode TUI sends:  POST /session/:id/message { parts: [...] }
        │
        ▼
opencode shim receives the request
        │
        ▼
shim translates → calls native Consensus API:
  POST /api/v1/sessions/:id/message { content: "...", type: "user_instruction" }
        │
        ▼
Consensus harness runs the agent iteration
        │
        ▼
native API returns: agent response
        │
        ▼
shim translates → returns opencode-format response:
  { info: {...}, parts: [{ type: "text", text: "..." }] }
        │
        ▼
opencode TUI renders the response
```

The shim does NOT have its own session management, its own database access, or its own business logic. It is purely a protocol translator. Every shim endpoint calls through to the native Consensus API. This means:

- The native API is always complete and usable on its own
- Shims are optional — disable them and Consensus still works
- Any shim bug only affects that shim, not the core system
- New shims for new tools can be added without touching core code

### 3.3 Concept Mapping

| opencode Concept | Consensus Concept | Mapping |
|---|---|---|
| Session | Session | Direct 1:1 |
| Message (user) | `memory_events` (type = `user_instruction`) | User message → INSERT into memory_events |
| Message (assistant) | `memory_events` (type = `agent_response`) | Agent response → SELECT from memory_events |
| Message parts (tool calls, text, etc.) | `memory_events` (type = `tool_result`) + JSON parts | Mapped from Consensus's structured output |
| Event stream | Postgres LISTEN/NOTIFY or SSE | Consensus's real-time event system |
| File operations | Consensus tools (read, write, edit, grep, glob) | Shim delegates to native tool execution API |
| Provider | LLM config in Consensus | Shim reports Consensus's configured LLM |
| Agent | Agent config (build/plan modes) | Mapped to Consensus session parameters |
| Tool | Consensus tools_registry | Shim exposes Consensus tools as opencode tools |
| Permission request | HITL approval_request | Direct mapping — approval shows as permission prompt |
| Config | `consensus.yaml` | Shim reads Consensus config |

#### Shim Endpoints

These are the opencode server endpoints the shim must handle. Each one translates to a native Consensus API call:

**Global:**

| Method | Path | Maps To |
|---|---|---|
| `GET` | `/global/health` | Static `{ healthy: true, version: "consensus-x.y.z" }` |
| `GET` | `/global/event` | Consensus real-time event stream (SSE) |

**Sessions:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/session` | `GET /api/v1/sessions` |
| `POST` | `/session` | `POST /api/v1/sessions` |
| `GET` | `/session/:id` | `GET /api/v1/sessions/:id` |
| `DELETE` | `/session/:id` | `DELETE /api/v1/sessions/:id` |
| `PATCH` | `/session/:id` | `PATCH /api/v1/sessions/:id` |
| `GET` | `/session/:id/children` | `GET /api/v1/sessions/:id` (filter by parent) |
| `POST` | `/session/:id/abort` | `PATCH /api/v1/sessions/:id` `{ status: "failed" }` |

**Messages:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/session/:id/message` | `GET /api/v1/sessions/:id/memory` |
| `POST` | `/session/:id/message` | `POST /api/v1/sessions/:id/message` → harness runs → response translated |
| `GET` | `/session/:id/message/:messageID` | `GET /api/v1/sessions/:id/memory/:mid` |

**This is the critical endpoint.** When the TUI sends a message:

```
opencode TUI → POST /session/:id/message { parts: [{ type: "text", text: "Fix the auth bug" }] }
    │
    ▼
opencode shim receives request
    │
    ▼
shim calls native API:
  POST /api/v1/sessions/:id/message { content: "Fix the auth bug", type: "user_instruction" }
    │
    ▼
Consensus harness runs (native API handles the rest):
  1. INSERT INTO memory_events (type='user_instruction', content=text)
  2. Run agent iteration (SPEC-008 core loop)
  3. Wait for agent_response memory event
    │
    ▼
native API returns agent response
    │
    ▼
shim translates → returns opencode-format response:
  { info: {...}, parts: [{ type: "text", text: "I'll fix the auth bug by..." }] }
    │
    ▼
opencode TUI displays the response
```

**Files:**

| Method | Path | Maps To |
|---|---|---|
| `GET` | `/find?pattern=<pat>` | `POST /api/v1/tools/grep/execute` |
| `GET` | `/find/file?query=<q>` | `POST /api/v1/tools/glob/execute` |
| `GET` | `/find/symbol?query=<q>` | LSP integration (if available) |
| `GET` | `/file/content?path=<p>` | `POST /api/v1/tools/read/execute` |
| `GET` | `/file/status` | `POST /api/v1/tools/git_status/execute` |

**Config:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/config` | `GET /api/v1/config` |
| `PATCH` | `/config` | `PATCH /api/v1/config` |
| `GET` | `/config/providers` | `GET /api/v1/config/providers` |

**Provider:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/provider` | `GET /api/v1/config/providers` (reformatted) |
| `PUT` | `/auth/:id` | `PUT /api/v1/auth/:id` |

**Agents:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/agent` | `GET /api/v1/agents` (returns agent types from config) |

**Tools:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/experimental/tool/ids` | `GET /api/v1/tools` (names only) |
| `GET` | `/experimental/tool` | `GET /api/v1/tools` (full schemas) |

**LSP & MCP:**

| Method | Shim Path | Native API Call |
|---|---|---|
| `GET` | `/lsp` | Read config (LSP status) |
| `GET` | `/mcp` | Read config (MCP status) |
| `POST` | `/mcp` | `POST /api/v1/mcp` |

**TUI Control:**

| Method | Shim Path | Notes |
|---|---|---|
| `POST` | `/tui/append-prompt` | Shim-only (queue text for TUI) |
| `POST` | `/tui/submit-prompt` | Shim-only (submit queued prompt) |
| `POST` | `/tui/execute-command` | Shim-only (execute TUI command) |
| `POST` | `/tui/show-toast` | Shim-only (show notification in TUI) |

**Events:**

| Method | Path | Maps To |
|---|---|---|
| `GET` | `/event` | SSE stream of Consensus events |

**Docs:**

| Method | Path | Maps To |
|---|---|---|
| `GET` | `/doc` | Serve OpenAPI 3.1 spec (SPEC-018) |

### 3.3 Authentication

opencode uses HTTP Basic Auth via `OPENCODE_SERVER_PASSWORD`. The shim translates this to Consensus's native API key auth:

```
opencode client sends: Authorization: Basic base64("opencode:" + password)
    │
    ▼
shim translates:
  - Password = Consensus admin API key
  - Validates against api_keys table (SPEC-015 §2)
  - Calls native API with the resolved API key
```

### 3.4 Message Format Mapping

opencode messages have a specific structure with `info` and `parts`. The shim translates between opencode's format and Consensus's native API request/response shapes.

**User message (shim translates opencode format → native API call):**

```json
// opencode TUI sends to shim:
{
  "parts": [
    { "type": "text", "text": "Fix the auth bug in @src/auth.ts" }
  ]
}

// shim translates and calls native API:
// POST /api/v1/sessions/:id/message
// { "content": "Fix the auth bug in @src/auth.ts", "type": "user_instruction" }
```

**Assistant message (shim translates native API response → opencode format):**

```json
// native API returns:
// { "type": "agent_response", "content": "I'll fix the auth bug..." }
// plus tool_result events

// shim translates to opencode format:
{
  "info": {
    "id": "msg-uuid",
    "role": "assistant",
    "createdAt": 1712928000000
  },
  "parts": [
    { "type": "text", "text": "I'll fix the auth bug by adding token validation..." },
    {
      "type": "tool-invocation",
      "toolInvocation": {
        "toolCallId": "call-1",
        "toolName": "file_edit",
        "args": { "path": "src/auth.ts", "changes": "..." },
        "state": "result",
        "result": { "success": true }
      }
    }
  ]
}
```

### 3.5 Event Stream Mapping

opencode's TUI subscribes to `/event` (SSE) for real-time updates. The shim subscribes to Consensus's native event stream (SPEC-015 §4) and translates events:

| Consensus Event | opencode Event |
|---|---|
| Session status change | `session.updated` |
| New memory_event | `message.created` |
| Tool execution start | `tool.started` |
| Tool execution complete | `tool.completed` |
| HITL approval requested | `permission.requested` |
| Approval resolved | `permission.resolved` |

```typescript
// Shim translates Consensus native events → opencode event format
function translateEvent(nativeEvent: ConscienceEvent): OpenCodeEvent {
    switch (consensusEvent.type) {
        case 'session_status_change':
            return { type: 'session.updated', properties: { sessionID: consensusEvent.session_id, status: consensusEvent.status } };
        case 'memory_event_created':
            return { type: 'message.created', properties: { sessionID: consensusEvent.session_id, messageID: consensusEvent.id } };
        case 'tool_execution_start':
            return { type: 'tool.started', properties: { sessionID: consensusEvent.session_id, toolName: consensusEvent.tool_name } };
        case 'approval_requested':
            return { type: 'permission.requested', properties: { permissionID: consensusEvent.approval_id, message: consensusEvent.description } };
    }
}
```

### 3.6 Tool Execution Through the Shim

When the agent (running inside Consensus's harness) uses a tool like `file_edit`, the shim needs to format the tool result as an opencode message part so the TUI renders it correctly.

The key insight: **Consensus executes tools via its native tool system (SPEC-010).** The shim just translates the results into opencode's display format. Consensus's harness handles the actual execution. The shim is a read-only translator for display purposes.

```
Consensus harness iteration (same as always):
  → Agent decides to edit a file
  → Harness executes tool: file_edit(path="src/auth.ts", ...)
  → Tool result written to memory_events
  → Native API returns the result

Shim reads the result from native API:
  → GET /api/v1/sessions/:id/memory (gets tool_result events)
  → Translates to opencode tool-invocation part format
  → Returns complete message with text + tool parts

opencode TUI renders:
  → Shows agent's explanation (text part)
  → Shows file edit with diff (tool-invocation part)
```

### 3.7 HITL as opencode Permission Requests

Consensus's HITL approval system (SPEC-014) maps to opencode's permission system through the shim. When the agent pauses for approval:

```
Consensus: INSERT INTO approval_requests (status='pending', ...)
    │
    ▼
Native API: approval appears in GET /api/v1/approvals
    │
    ▼
Shim translates to opencode permission.requested event
    │
    ▼
opencode TUI: shows permission prompt dialog
    "Agent wants to: DELETE FROM temp_cache"
    [Allow] [Deny]
    │
    ▼
opencode sends: POST /session/:id/permissions/:id { response: "allow" }
    │
    ▼
Shim translates → calls native API:
  POST /api/v1/approvals/:id/review { decision: "approved" }
    │
    ▼
Consensus: session resumes, agent continues
```

### 3.8 Connection Flow

```bash
# 1. Start Consensus server with opencode shim enabled
consensus serve --port 8090 --adapter opencode

# 2. From another terminal (or another machine), attach opencode
opencode attach http://localhost:8090

# 3. opencode TUI connects, authenticates, creates/restores session
# 4. User chats normally through opencode's TUI
# 5. Consensus handles all the cognitive architecture behind the scenes
```

### 3.9 Shim Endpoint Exclusions

These opencode endpoints relate to opencode's own internal LLM calling and aren't needed — Consensus handles LLM interaction through its native harness (SPEC-008). The shim returns `501 Not Implemented` for these:

| Endpoint | Why Skip |
|---|---|
| `POST /session/:id/prompt_async` | Consensus runs the harness loop synchronously per message |
| `POST /session/:id/shell` | Consensus doesn't expose raw shell — it uses its own tool system |
| `POST /session/:id/command` | Slash commands are opencode-specific; Consensus has its own commands |
| `POST /session/:id/share` | Sharing is opencode's feature; Consensus has its own export (SPEC-015) |
| `POST /session/:id/summarize` | Consensus does its own memory compression |
| `POST /session/:id/init` | Consensus has its own bootstrapping |
| `POST /session/:id/fork` | Consensus has iteration rollback via iteration_commits |
| `POST /session/:id/revert` | Consensus has its own undo via transaction rollback |
| `GET /project` | Project concept is opencode-specific |
| `GET /vcs` | VCS access through Consensus tools, not direct |

These return `501 Not Implemented` or are mapped to Consensus equivalents where sensible.

The `/instance/*` surface is NOT part of this exclusion list — it is implemented as real opencode-protocol translation endpoints, see §3.10.

### 3.10 Instance Endpoints (opencode-protocol translation)

C-GAP-031: `/instance/*` translates the opencode server protocol into native Consensus calls instead of returning 501. The Consensus server is a singleton instance rooted at the server process working directory.

**Auth policy:** `/instance/*` is fully public — no auth — because the opencode contract probes these endpoints unauthenticated and expects 200 with real workspace data (full-contract suite C19). `isStubPath` no longer covers `/instance`; the auth skip lives directly in the auth middleware.

| Endpoint | Response |
|---|---|
| `GET /instance` | Singleton instance list: `[{id, path, createdAt, updatedAt}]` — `id` is a stable hash of the workspace directory; timestamps come from the server process. |
| `GET /instance/path` | opencode `PathInfo`: `{home, state, config, worktree, directory}` — `worktree` is the git top-level when the workspace is a repo, otherwise the directory itself. |
| `GET /instance/vcs` | opencode `Vcs.Info`: `{branch?, default_branch?}` read live from git (`branch --show-current`, origin HEAD / `init.defaultBranch`); non-git workspaces return `{}` — never an error. |
| `GET /instance/vcs/diff` | opencode `Vcs.FileDiff[]`: `[{file, additions, deletions, status?}]` built from `git status --porcelain` + `git diff HEAD --numstat` (untracked files count their own lines); clean or non-git workspaces return `[]`. `patch` is omitted — optional in the upstream schema. |

Other upstream `/instance/*` sub-paths (`dispose`, `vcs/status`, `vcs/diff/raw`, `vcs/apply`, `command`, `agent`, `skill`, `lsp`, `formatter`) return `501 NOT_IMPLEMENTED` (same convention as `/session` subpaths in §3.9); unknown sub-paths return `404 NOT_FOUND`.

Response shapes mirror the upstream opencode server protocol (`sst/opencode` `packages/opencode/src/server/routes/instance/httpapi/groups/instance.ts` and `src/project/vcs.ts`) so bridged opencode clients are indistinguishable from a native server.

---

## 4. MCP Server (Tool-Level Integration)

For users who want Consensus capabilities inside their existing AI tool without replacing the tool's backend. The MCP server exposes Consensus as a set of tools any MCP client can call.

This is already defined in SPEC-015 §5. The key tools:

| MCP Tool | What It Does |
|---|---|
| `create_session` | Create a new Consensus agent session |
| `send_message` | Send a message to a running agent |
| `get_session_status` | Check what an agent is doing |
| `list_memory` | Browse agent memory |
| `review_approval` | Approve/reject HITL requests |
| `query_tool` | Execute an internal Consensus tool |

**Usage example (Claude Code):**

```bash
# Add Consensus as an MCP tool server
claude mcp add consensus --transport http http://localhost:8090/mcp/sse

# Now inside Claude Code:
> Create a Consensus session to analyze my database schema
> What is the agent doing now?
> Approve the pending schema change request
```

**Usage example (opencode):**

```json
{
  "mcp": {
    "consensus": {
      "type": "remote",
      "url": "http://localhost:8090/mcp/sse",
      "headers": { "Authorization": "Bearer cs_sk_..." }
    }
  }
}
```

The user keeps their existing AI tool (with its own LLM, its own UI) but gains access to Consensus's persistent memory, database-backed agents, and HITL system as tools.

---

## 5. Future Shims

### 5.1 pi-agent

If pi-agent (or similar tools) expose a client/server architecture with a detachable TUI, the same shim pattern applies: implement their server protocol as a translation layer on top of Consensus's native API.

Research needed: Does pi-agent have a `serve` + `attach` model like opencode? If so, the shim follows the exact same pattern as Section 3 — pi-agent protocol in, native Consensus API out.

### 5.2 Aider

Aider doesn't have a detachable TUI — it's a monolithic CLI. No shim opportunity. Aider users would use Consensus through MCP tools instead.

### 5.3 Continue.dev / Cursor / IDE Extensions

These are editor-embedded UIs, not detachable TUIs. They connect through MCP tools (Section 4) or directly through the native REST API (SPEC-015).

---

## 6. Implementation Priority

### Phase 1: MCP Server
- Implement SPEC-015 §5 MCP server
- Users can talk to Consensus from opencode, Claude Code, any MCP client
- No TUI borrowing yet — just tool-level access
- Uses native API directly (no shim needed)

### Phase 2: opencode Shim (Core)
- Implement the opencode server endpoints the TUI needs as a translation layer over the native API:
  - `/global/health`, `/global/event`
  - `/session`, `/session/:id`, `/session/:id/message`
  - `/config`, `/provider`, `/agent`
- Basic `opencode attach` works
- Message format translation (opencode ↔ native API)
- Event stream translation

### Phase 3: opencode Shim (Full)
- File operation translations (opencode file endpoints → native tool execution API)
- HITL permission request translation
- TUI control endpoints
- Tool invocation display formatting
- Full `/doc` OpenAPI spec served at the shim root

### Phase 4: Additional Shims
- pi-agent shim (if protocol available)
- Any future tool with a detachable TUI

---

## 7. Shim Implementation Pattern

Each shim follows the same structure: receive external protocol request → translate → call native Consensus API → translate response → return.

```typescript
// shims/opencode/index.ts
export class OpenCodeShim {
    constructor(private nativeAPI: ConscienceAPI) {}

    // Shim endpoint → native API call → format translation
    async listSessions(): Promise<OpenCodeSession[]> {
        const sessions = await this.nativeAPI.get('/api/v1/sessions');
        return sessions.map(this.translateSession);
    }

    async createSession(body: CreateSessionBody): Promise<OpenCodeSession> {
        const session = await this.nativeAPI.post('/api/v1/sessions', {
            agent_name: body.title || 'default',
            goal: '', // opencode doesn't set a goal upfront
        });
        return this.translateSession(session);
    }

    async sendMessage(sessionId: string, body: SendMessageBody): Promise<OpenCodeMessage> {
        // Translate opencode parts → native API message
        const text = body.parts.filter(p => p.type === 'text').map(p => p.text).join('\n');
        const response = await this.nativeAPI.post(`/api/v1/sessions/${sessionId}/message`, {
            content: text,
            type: 'user_instruction',
        });
        // Translate native API response → opencode message format
        return this.translateMessage(response);
    }

    // Format translators (opencode format ↔ native API format)
    private translateSession(s: ConscienceSession): OpenCodeSession { ... }
    private translateMessage(m: ConscienceMemoryEvent): OpenCodeMessage { ... }
    private translateEvent(e: ConscienceEvent): OpenCodeEvent { ... }
}
```

```go
// shims/opencode/shim.go (PocketBase)
type OpenCodeShim struct {
    nativeAPI *ConscienceClient
}

func (s *OpenCodeShim) RegisterRoutes(r *gin.Engine) {
    r.GET("/global/health", s.health)
    r.GET("/global/event", s.eventStream)
    r.GET("/session", s.listSessions)
    r.POST("/session", s.createSession)
    r.GET("/session/:id", s.getSession)
    r.POST("/session/:id/message", s.sendMessage)
    // ... etc
}

// Every handler calls nativeAPI internally
func (s *OpenCodeShim) sendMessage(c *gin.Context) {
    // Parse opencode request
    var req OpenCodeMessageRequest
    c.BindJSON(&req)
    // Call native API
    resp, err := s.nativeAPI.PostMessage(sessionID, text)
    // Translate and return
    c.JSON(200, translateMessage(resp))
}
```

---

## 8. Shim Session Mapping

When `opencode attach` connects, it may create sessions or resume existing ones. The shim maps opencode session IDs to Consensus sessions:

```sql
CREATE TABLE shim_session_map (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shim_type       TEXT NOT NULL CHECK (shim_type IN ('opencode', 'pi-agent')),
    external_id     TEXT NOT NULL,       -- opencode's session ID
    session_id      UUID NOT NULL REFERENCES sessions(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_shim_session_map ON shim_session_map(shim_type, external_id);
```

This table is owned by the shim layer only. The native API does not reference it.

---

## 9. PocketBase Parity

| Feature | Postgres Backend | SQLite Backend |
|---|---|---|
| Native REST API | Go HTTP router | Same — shared code |
| opencode shim routes | Go HTTP handlers (translating to native API) | Same |
| MCP Server | Go handler at `/mcp/sse` | Same |
| Event stream | Postgres LISTEN/NOTIFY → SSE | Go channels → SSE |
| Shim session mapping | `shim_session_map` table | Same table (SQLite) |
| Auth translation | HTTP Basic Auth → api_keys lookup | Same |

---

## 10. Open Questions

1. **opencode version compatibility**: opencode's server API may change between versions. How do we track which version's API to implement? Pin to a specific opencode version?
2. **File system access**: opencode expects the server to have filesystem access (read/write files). If Consensus runs on a separate machine from the codebase, how do file operations work? Proxy through a local agent?
3. **Tool schema format**: Consensus tools have their own schema format (SPEC-010). Does this map cleanly to opencode's tool format, or do we need translation?
4. **Streaming responses**: opencode expects streaming message parts. Consensus's harness runs a full iteration then returns. Do we stream intermediate tool calls, or only return the final response?
5. **pi-agent protocol**: Need to research pi-agent's architecture to determine if the same shim pattern applies.
