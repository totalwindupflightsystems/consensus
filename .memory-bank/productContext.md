# Product Context

## Problem Space
Bash/CLI AI agents are structurally flawed: non-deterministic state, no durable memory, escapable sandboxes, no rollback. Every tool call mutates the filesystem with no transaction boundary. Agents cannot guarantee they follow their own rules.

Conscience pairs the LLM with the most deterministic system available (a relational database) to create agents that physically cannot break their own rules.

## User Experience Goals
- **Supabase one-click deploy**: Sign up → run SQL script → system is live
- **Local binary**: Download → double-click → agent running locally
- **CLI-first management**: All operations via `conscience` CLI
- **opencode TUI via shim**: `opencode attach` gives users a polished terminal chat interface
- **MCP tool access**: Any MCP-compatible client can use Conscience as a tool provider
- **Web admin UI**: Future phase

## How It Works
- Agent reads from `active_context_view` (SQL VIEW filtering/compressing memory)
- Agent outputs JSON containing SQL statements and tool requests
- Harness executes SQL in a transaction (cognition phase)
- Harness runs tool requests asynchronously (tool execution phase)
- Results loop back into the next iteration
- All state lives in the database — no filesystem, no Redis, no message broker

## Key Differentiators vs LangChain/AutoGen
| Feature | LangChain/AutoGen | Conscience |
|---|---|---|
| State | External stores, lost between sessions | Append-only ledger with time-travel |
| Context | Monolithic prompt strings | Dynamic SQL VIEW with paging |
| Security | Python sandbox (escapable) | RLS at database kernel level |
| Error recovery | Manual state repair | Atomic transaction rollback |
| Memory cost | Full re-send every loop | Compressed pointers, vector-validated |
| Self-improvement | Prompt engineering | ALTER TABLE + JSON Schema constraints |

## Domain Terminology
- **Cognition transaction**: Phase 1 — memory changes, system actions, pending tool requests executed in a transaction
- **Tool execution phase**: Phase 2 — async tool execution, each result in its own mini-transaction
- **active_context_view**: SQL VIEW that filters/compresses memory for the LLM
- **Unbypassable constraints**: Database-level CHECK/RLS constraints rather than prompt-level rules
