# Product Context

## Why Consensus Exists
Bash/CLI AI agents are structurally flawed: non-deterministic state, no durable memory, escapable sandboxes, no rollback. Consensus pairs the LLM with the most deterministic system available (a relational database) to create agents that physically cannot break their own rules.

## How It Works
- Agent reads from `active_context_view` (a SQL VIEW that filters/compresses memory)
- Agent outputs JSON containing SQL statements and tool requests
- Harness executes SQL in a transaction (cognition phase)
- Harness runs tool requests asynchronously (tool execution phase)
- Results loop back into the next iteration's context
- All state lives in the database — no filesystem, no Redis, no message broker

## User Experience Goals
- **Supabase one-click deploy**: Sign up → run SQL script → system is live
- **PocketBase binary**: Download → double-click → agent running locally
- **CLI-first management**: All operations (session control, approvals, memory inspection, cost tracking, configuration) via `consensus` CLI
- **opencode TUI via shim**: `opencode attach` gives users a polished terminal chat interface backed by Consensus
- **MCP tool access**: Any MCP-compatible client can use Consensus as a tool provider
- **Web admin UI**: Future phase — API supports it, CLI proves the surface is complete, web dashboard designed later

## Key Differentiators vs LangChain/AutoGen
| Feature | LangChain/AutoGen | Consensus |
|---|---|---|
| State | External stores, lost between sessions | Append-only ledger with time-travel |
| Context | Monolithic prompt strings | Dynamic SQL VIEW with paging |
| Security | Python sandbox (escapable) | RLS at database kernel level |
| Error recovery | Manual state repair | Atomic transaction rollback |
| Memory cost | Full re-send every loop | Compressed pointers, vector-validated |
| Self-improvement | Prompt engineering | ALTER TABLE + JSON Schema constraints |