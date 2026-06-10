# System Patterns

## Architecture Overview
**Database-as-Runtime**: All state lives in SQL tables. No Redis, no message brokers, no filesystem state. One Go binary containing harness loop, REST API, MCP server, and CLI.

## Key Technical Decisions
- **Two-Phase Execution**: Cognition transaction (fast) → async tool execution (no transaction). Never block a transaction on external I/O.
- **Append-Only Ledger**: `memory_events` is INSERT/SELECT only. Compression workers can update `summary_text` only.
- **Event Sourcing**: `iteration_commits` store active pointer arrays. Rollback = querying an older commit.
- **Progressive Disclosure**: Skill metadata is ~100 tokens; full instructions loaded on demand.
- **One Go binary** with `--db` flag selecting backend (`sqlite://path` or `postgres://connection-string`)
- **pg_cron optional** — Go binary has cron fallback for maintenance jobs

## Design Patterns in Use
- **RLS for security**: All tables scoped by `conscience.session_id` via `SET LOCAL` (Postgres) or Go-layer enforcement (SQLite)
- **Stored procedures for destructive ops**: `complete_session()`, `set_display_mode()`, `soft_delete()`
- **SQL injection mitigation**: Statement classifier (DML_READ/WRITE/DDL_CREATE/ALTER/DANGEROUS) + table whitelist + stored proc preference
- **Token caching**: System instructions (Layer 1) → Schema/Tools (Layer 2) → Active context (Layer 3)
- **Vector-validated compression**: Same embedding model for all tiers. LLM tier affects summarization, not embeddings.
- **Dynamic entity generator**: SECURITY DEFINER function for table creation. No raw DDL for agents.

## Data Flow
```
Harness reads active_context_view → formats Markdown → sends to LLM → LLM returns JSON
→ Harness parses JSON → Phase 1: cognition transaction (memory changes, system actions, pending tool requests)
→ Phase 2: async tool execution (each tool result in own mini-transaction)
→ Loop back to harness reads active_context_view
```

## Error Handling Strategy
- All state is in the database — errors roll back the transaction atomically
- Tool execution errors produce error events in the ledger, not crashes
- Interactive transactions have timeout and crash recovery
- Rollback-retry prevention prevents infinite loops

## Testing Strategy
- SQL schema tests with migration verification
- Database driver interface tests (Postgres + SQLite parity)
- Harness loop integration tests
- REST API contract tests
- End-to-end test with actual LLM calls
